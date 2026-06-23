package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"bi-dashboard/internal/business"
)

var businessImportMu sync.Mutex

const businessPreviewTTL = 30 * time.Minute

var reYearOnly = regexpMustCompileBusinessYear()

func regexpMustCompileBusinessYear() *regexp.Regexp {
	return regexp.MustCompile(`(\d{4})\s*年`)
}

func businessPreviewDir() string {
	d := filepath.Join(os.TempDir(), "bi-business-import")
	os.MkdirAll(d, 0755)
	return d
}

func newBusinessToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type businessPreviewPayload struct {
	SnapshotYear  int                   `json:"snapshotYear"`
	SnapshotMonth int                   `json:"snapshotMonth"`
	Mode          string                `json:"mode"`
	Filename      string                `json:"filename"`
	UserID        int                   `json:"userID"`
	UploadedAt    time.Time             `json:"uploadedAt"`
	Result        *business.ParseResult `json:"result"`
}

// ImportBusinessReportPreview 第一步：上传+解析+算diff+存token
// POST /api/finance/business-report/import/preview  表单: file, mode(full|incremental), 可选 snapshotMonth
func (h *DashboardHandler) ImportBusinessReportPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		writeError(w, 400, "解析表单失败: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, 400, "请选择 xlsx 文件")
		return
	}
	defer file.Close()
	if strings.ToLower(filepath.Ext(header.Filename)) != ".xlsx" {
		writeError(w, 400, "仅支持 .xlsx 格式")
		return
	}

	mode := strings.TrimSpace(r.FormValue("mode"))
	if mode == "" {
		mode = business.ImportModeFull
	}
	if mode != business.ImportModeFull && mode != business.ImportModeIncremental {
		writeError(w, 400, "mode 必须是 full 或 incremental")
		return
	}

	year, month := business.ParseSnapshotFromFilename(header.Filename)
	if mv := strings.TrimSpace(r.FormValue("snapshotMonth")); mv != "" {
		if m, e := strconv.Atoi(mv); e == nil && m >= 1 && m <= 12 {
			month = m
		}
	}
	// 年份兜底：文件名只有年的情况
	if year == 0 {
		if yv := reYearOnly.FindStringSubmatch(header.Filename); yv != nil {
			year, _ = strconv.Atoi(yv[1])
		}
	}
	if year < 2020 || year > 2050 || month < 1 || month > 12 {
		writeError(w, 400, "无法确定快照年月，请用「YYYY年MM月业务预决算报表.xlsx」命名或手选月份")
		return
	}

	tmpPath := filepath.Join(businessPreviewDir(), fmt.Sprintf("upload-%d-%s", time.Now().UnixMilli(), filepath.Base(header.Filename)))
	dst, err := os.Create(tmpPath)
	if err != nil {
		writeError(w, 500, "创建临时文件失败")
		return
	}
	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		writeError(w, 500, "保存文件失败")
		return
	}
	dst.Close()
	defer os.Remove(tmpPath)

	result, err := business.ParseFile(tmpPath, year, month, year)
	if err != nil {
		writeServerError(w, 500, "Excel 解析失败", err)
		return
	}
	result.Mode = mode
	result.SourceFile = header.Filename // 存原名，不是临时路径

	diff, err := business.ComputeDiff(h.DB, result)
	if err != nil {
		writeServerError(w, 500, "计算变更预览失败", err)
		return
	}

	userID := 0
	if payload, ok := authPayloadFromContext(r); ok && payload != nil {
		userID = int(payload.User.ID)
	}

	token := newBusinessToken()
	payload := &businessPreviewPayload{
		SnapshotYear:  year,
		SnapshotMonth: month,
		Mode:          mode,
		Filename:      header.Filename,
		UserID:        userID,
		UploadedAt:    time.Now(),
		Result:        result,
	}
	cacheBytes, _ := json.Marshal(payload)
	if err := os.WriteFile(filepath.Join(businessPreviewDir(), token+".json"), cacheBytes, 0600); err != nil {
		writeServerError(w, 500, "缓存预览失败", err)
		return
	}
	go cleanupExpiredBusinessPreviews()

	writeJSON(w, map[string]interface{}{
		"token":         token,
		"snapshotYear":  year,
		"snapshotMonth": month,
		"mode":          mode,
		"filename":      header.Filename,
		"rowCount":      result.RowCount,
		"diff":          diff,
		"expiresAt":     time.Now().Add(businessPreviewTTL).Format(time.RFC3339),
	})
}

// ImportBusinessReportConfirm 第二步：凭 token 写库
// POST /api/finance/business-report/import/confirm  JSON {token}
func (h *DashboardHandler) ImportBusinessReportConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	if !businessImportMu.TryLock() {
		writeError(w, 409, "有其他导入任务进行中，请稍后再试")
		return
	}
	defer businessImportMu.Unlock()

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		writeError(w, 400, "缺少 token")
		return
	}
	// token 校验：只允许 hex（防路径穿越）
	for _, c := range req.Token {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			writeError(w, 400, "token 格式非法")
			return
		}
	}
	if len(req.Token) != 32 {
		writeError(w, 400, "token 长度非法")
		return
	}

	cachePath := filepath.Join(businessPreviewDir(), req.Token+".json")
	cacheBytes, err := os.ReadFile(cachePath)
	if err != nil {
		writeError(w, 404, "预览已过期或不存在，请重新上传")
		return
	}
	var payload businessPreviewPayload
	if err := json.Unmarshal(cacheBytes, &payload); err != nil {
		writeServerError(w, 500, "缓存损坏", err)
		return
	}
	if time.Since(payload.UploadedAt) > businessPreviewTTL {
		os.Remove(cachePath)
		writeError(w, 410, "预览已过期（30分钟），请重新上传")
		return
	}
	if payload.Result == nil {
		writeError(w, 500, "缓存内容异常")
		return
	}
	payload.Result.Mode = payload.Mode
	if uname := businessUsername(h, payload.UserID); uname != "" {
		payload.Result.ImportedBy = uname
	}

	if err := business.WriteResult(h.DB, payload.Result); err != nil {
		writeServerError(w, 500, "入库失败", err)
		return
	}
	os.Remove(cachePath)

	writeJSON(w, map[string]interface{}{
		"snapshotYear":  payload.SnapshotYear,
		"snapshotMonth": payload.SnapshotMonth,
		"mode":          payload.Mode,
		"rowCount":      payload.Result.RowCount,
	})
}

func cleanupExpiredBusinessPreviews() {
	entries, err := os.ReadDir(businessPreviewDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > businessPreviewTTL {
			os.Remove(filepath.Join(businessPreviewDir(), e.Name()))
		}
	}
}

// businessUsername 取用户名作 imported_by（查不到返回空，WriteResult 会回落 "cli"）
func businessUsername(h *DashboardHandler, userID int) string {
	if userID == 0 || h.DB == nil {
		return ""
	}
	var name string
	if err := h.DB.QueryRow(`SELECT username FROM users WHERE id=?`, userID).Scan(&name); err != nil {
		return ""
	}
	return name
}
