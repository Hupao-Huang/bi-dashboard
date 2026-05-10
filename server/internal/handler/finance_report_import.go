package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bi-dashboard/internal/finance"
)

// ImportFinancePreview 第一步：上传 + 解析（不写库）+ 生成 token + 返回预览 diff
// POST /api/finance/report/import/preview
// 表单: file (xlsx), mode ("full" | "incremental"), year (可选)
func (h *DashboardHandler) ImportFinancePreview(w http.ResponseWriter, r *http.Request) {
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
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".xlsx" {
		writeError(w, 400, "仅支持 .xlsx 格式")
		return
	}

	mode := strings.TrimSpace(r.FormValue("mode"))
	if mode == "" {
		mode = finance.ImportModeFull
	}
	if mode != finance.ImportModeFull && mode != finance.ImportModeIncremental {
		writeError(w, 400, "mode 必须是 full 或 incremental")
		return
	}

	year := finance.ParseYearFromFilename(header.Filename)
	if yv := strings.TrimSpace(r.FormValue("year")); yv != "" {
		if y, err := strconv.Atoi(yv); err == nil {
			year = y
		}
	}
	if year == 0 {
		writeError(w, 400, "无法推断年份，请用 YYYY年财务管理报表.xlsx 命名或传 year 参数")
		return
	}
	if year < 2000 || year > 2100 {
		writeError(w, 400, fmt.Sprintf("年份 %d 不合理，请检查文件名", year))
		return
	}

	tmpDir := filepath.Join(os.TempDir(), "bi-finance-import")
	os.MkdirAll(tmpDir, 0755)
	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("upload-%d-%s", time.Now().UnixMilli(), filepath.Base(header.Filename)))
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

	userID := 0
	if payload, ok := authPayloadFromContext(r); ok && payload != nil {
		userID = int(payload.User.ID)
	}

	dict, err := finance.LoadSubjectDict(h.DB)
	if err != nil {
		writeServerError(w, 500, "加载字典失败", err)
		return
	}
	result, err := finance.ParseFile(tmpPath, year, dict)
	if err != nil {
		writeServerError(w, 500, "Excel 解析失败", err)
		return
	}
	result.Mode = mode

	diff, err := finance.ComputeDiff(h.DB, result)
	if err != nil {
		writeServerError(w, 500, "计算变更预览失败", err)
		return
	}

	// 缓存解析结果到磁盘
	token := newPreviewToken()
	payload := &previewPayload{
		Year:       year,
		Mode:       mode,
		Filename:   header.Filename,
		UserID:     userID,
		UploadedAt: time.Now(),
		Result:     result,
	}
	cachePath := filepath.Join(financePreviewDir(), token+".json")
	cacheBytes, _ := json.Marshal(payload)
	if err := os.WriteFile(cachePath, cacheBytes, 0600); err != nil {
		writeServerError(w, 500, "缓存预览失败", err)
		return
	}

	// 顺手清理过期缓存（best-effort）
	go cleanupExpiredPreviews()

	writeJSON(w, map[string]interface{}{
		"token":       token,
		"year":        year,
		"mode":        mode,
		"filename":    header.Filename,
		"sheetCount":  result.SheetCount,
		"rowCount":    result.RowCount,
		"departments": result.Departments,
		"unmapped":    result.UnmappedSubjects,
		"diff":        diff,
		"expiresAt":   time.Now().Add(financePreviewTTL).Format(time.RFC3339),
	})
}

// ImportFinanceConfirm 第二步：凭 token 触发实际写库
// POST /api/finance/report/import/confirm
// JSON: { token: string }
func (h *DashboardHandler) ImportFinanceConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	if !financeImportMu.TryLock() {
		writeError(w, 409, "有其他导入任务进行中，请稍后再试")
		return
	}
	defer financeImportMu.Unlock()

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

	cachePath := filepath.Join(financePreviewDir(), req.Token+".json")
	cacheBytes, err := os.ReadFile(cachePath)
	if err != nil {
		writeError(w, 404, "预览已过期或不存在，请重新上传")
		return
	}
	var payload previewPayload
	if err := json.Unmarshal(cacheBytes, &payload); err != nil {
		writeServerError(w, 500, "缓存损坏", err)
		return
	}
	if time.Since(payload.UploadedAt) > financePreviewTTL {
		os.Remove(cachePath)
		writeError(w, 410, "预览已过期（30分钟），请重新上传")
		return
	}

	if payload.Result == nil {
		writeError(w, 500, "缓存内容异常")
		return
	}
	payload.Result.Mode = payload.Mode

	if err := finance.WriteResult(h.DB, payload.Result); err != nil {
		_ = finance.LogImport(h.DB, payload.Filename, payload.Year, payload.Result, payload.UserID, "failed", err.Error())
		writeServerError(w, 500, "入库失败", err)
		return
	}

	status := "success"
	if len(payload.Result.UnmappedSubjects) > 0 {
		status = "partial"
	}
	_ = finance.LogImport(h.DB, payload.Filename, payload.Year, payload.Result, payload.UserID, status, "mode="+payload.Mode)

	// 写库成功后清理缓存
	os.Remove(cachePath)

	writeJSON(w, map[string]interface{}{
		"year":        payload.Year,
		"mode":        payload.Mode,
		"sheetCount":  payload.Result.SheetCount,
		"rowCount":    payload.Result.RowCount,
		"departments": payload.Result.Departments,
		"unmapped":    payload.Result.UnmappedSubjects,
		"status":      status,
	})
}

// cleanupExpiredPreviews 清理过期的预览缓存文件
func cleanupExpiredPreviews() {
	dir := financePreviewDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > financePreviewTTL {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
