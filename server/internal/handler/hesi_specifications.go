package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// GetHesiLastSync GET /api/hesi/last-sync
// 返回 sync-hesi.log 最后修改时间作为"上次同步时间"
// 权限: finance.expense:view (跟费控管理页面同级)
func (h *DashboardHandler) GetHesiLastSync(w http.ResponseWriter, r *http.Request) {
	// 候选路径: server/sync-hesi.log → ../sync-hesi.log → 当前 exe 同级
	candidates := []string{
		"sync-hesi.log",
		filepath.Join("server", "sync-hesi.log"),
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "sync-hesi.log"))
	}
	var info os.FileInfo
	var usedPath string
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			info = fi
			usedPath = p
			break
		}
	}
	if info == nil {
		writeJSON(w, map[string]interface{}{
			"lastSyncAt": nil,
			"message":    "未找到 sync-hesi.log",
		})
		return
	}
	t := info.ModTime()
	writeJSON(w, map[string]interface{}{
		"lastSyncAt":     t.Format("2006-01-02 15:04:05"),
		"lastSyncMillis": t.UnixMilli(),
		"logPath":        usedPath,
	})
}

// 合思单据模板字典 (19 条左右, 内存缓存 60s)
type hesiSpec struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

var (
	hesiSpecCache    []hesiSpec
	hesiSpecCacheAt  time.Time
	hesiSpecCacheMu  sync.Mutex
	hesiSpecTTL      = 60 * time.Second
)

// GetHesiSpecifications GET /api/hesi/specifications
// 拉合思单据模板字典 (id/name/type), 60s 内存缓存
func (h *DashboardHandler) GetHesiSpecifications(w http.ResponseWriter, r *http.Request) {
	specs, err := h.fetchHesiSpecifications()
	if err != nil {
		writeServerError(w, 500, "拉取合思模板字典失败", err)
		return
	}
	writeJSON(w, map[string]interface{}{"items": specs, "count": len(specs)})
}

// fetchHesiSpecifications 60s 缓存包装
func (h *DashboardHandler) fetchHesiSpecifications() ([]hesiSpec, error) {
	hesiSpecCacheMu.Lock()
	defer hesiSpecCacheMu.Unlock()
	if time.Since(hesiSpecCacheAt) < hesiSpecTTL && len(hesiSpecCache) > 0 {
		return hesiSpecCache, nil
	}

	token, err := h.getHesiToken()
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/api/openapi/v2/specifications?accessToken=%s", hesiAPIBase, token)
	resp, err := hesiHTTP.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(data)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("合思返回 HTTP %d: %s", resp.StatusCode, snippet)
	}
	var parsed struct {
		Items []hesiSpec `json:"items"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	hesiSpecCache = parsed.Items
	hesiSpecCacheAt = time.Now()
	return parsed.Items, nil
}

// LookupSpecName 根据 specification_id (格式 corp:hash 或 corp:PRESET_xxx:hash) 前缀匹配字典
// 返回匹配到的模板名称, 没匹配返回空串
func (h *DashboardHandler) LookupSpecName(specID string) string {
	if specID == "" {
		return ""
	}
	specs, err := h.fetchHesiSpecifications()
	if err != nil {
		return ""
	}
	// 字典 id 可能是 "ID01xxx" 或 "ID01xxx:PRESET_yyy"
	// 匹配方式: 字典 id 是 specification_id 的前缀
	for _, s := range specs {
		if s.ID == "" {
			continue
		}
		// 完整匹配 specification_id 以 s.ID + ":" 开头, 或完全相等
		if specID == s.ID || (len(specID) > len(s.ID) && specID[:len(s.ID)] == s.ID && specID[len(s.ID)] == ':') {
			return s.Name
		}
	}
	return ""
}

// ============================================================================
// v1.74.9: 合思员工/部门/法人实体字典 (单据详情弹窗展示名字, 而非 ID)
// 合思 OpenAPI 设计是返 ID, 名字得调字典接口. 仿 LookupSpecName 模式, 3 个字典各自缓存.
// 字典体量: 员工 880 / 部门 511 / 法人实体 48, 一次拉完, 5min TTL 平衡新鲜度与性能.
// ============================================================================

type hesiDictItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var (
	hesiStaffCache       map[string]string
	hesiStaffCacheAt     time.Time
	hesiStaffCacheMu     sync.Mutex
	hesiDeptCache        map[string]string
	hesiDeptCacheAt      time.Time
	hesiDeptCacheMu      sync.Mutex
	hesiLegalEntityCache map[string]string
	hesiLegalCacheAt     time.Time
	hesiLegalCacheMu     sync.Mutex
	hesiDictTTL          = 5 * time.Minute
)

// fetchHesiDictMap 通用字典拉取: 调合思接口, parse items, 返 id→name map
// 用 sync.Mutex 包外部, 内部不锁 (避免外层已持锁双重加锁)
func (h *DashboardHandler) fetchHesiDictMap(path string) (map[string]string, error) {
	token, err := h.getHesiToken()
	if err != nil {
		log.Printf("[hesi-dict] getHesiToken failed: %v", err)
		return nil, err
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	// count 合思上限 1000, 超了返 400 "count参数不能大于1000"
	// 当前规模: 员工 880 / 部门 511 / 法人实体 48, 都 < 1000. 超 1000 时需分页 (TODO)
	url := fmt.Sprintf("%s%s%saccessToken=%s&start=0&count=1000", hesiAPIBase, path, sep, token)
	resp, err := hesiHTTP.Get(url)
	if err != nil {
		log.Printf("[hesi-dict] GET %s failed: %v", path, err)
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(data)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		log.Printf("[hesi-dict] HTTP %d on %s: %s", resp.StatusCode, path, snippet)
		return nil, fmt.Errorf("合思返回 HTTP %d: %s", resp.StatusCode, snippet)
	}
	var parsed struct {
		Items []hesiDictItem `json:"items"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		log.Printf("[hesi-dict] unmarshal %s failed: %v, body=%s", path, err, string(data[:min(200, len(data))]))
		return nil, err
	}
	m := make(map[string]string, len(parsed.Items))
	for _, it := range parsed.Items {
		if it.ID != "" {
			m[it.ID] = it.Name
		}
	}
	log.Printf("[hesi-dict] %s loaded %d items", path, len(m))
	return m, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// LookupStaffName 员工 ID → 姓名. staff_id 格式 "ID01FfMgoeP7cz:ID01Fp0xxx"
func (h *DashboardHandler) LookupStaffName(staffID string) string {
	if staffID == "" {
		return ""
	}
	hesiStaffCacheMu.Lock()
	defer hesiStaffCacheMu.Unlock()
	if time.Since(hesiStaffCacheAt) >= hesiDictTTL || hesiStaffCache == nil {
		m, err := h.fetchHesiDictMap("/api/openapi/v2/staffs")
		if err != nil {
			return ""
		}
		hesiStaffCache = m
		hesiStaffCacheAt = time.Now()
	}
	return hesiStaffCache[staffID]
}

// LookupDeptName 部门 ID → 部门名. dept_id 格式 "ID01FfMgoeP7cz:ID01Fp0xxx"
func (h *DashboardHandler) LookupDeptName(deptID string) string {
	if deptID == "" {
		return ""
	}
	hesiDeptCacheMu.Lock()
	defer hesiDeptCacheMu.Unlock()
	if time.Since(hesiDeptCacheAt) >= hesiDictTTL || hesiDeptCache == nil {
		m, err := h.fetchHesiDictMap("/api/openapi/v2/departments")
		if err != nil {
			return ""
		}
		hesiDeptCache = m
		hesiDeptCacheAt = time.Now()
	}
	return hesiDeptCache[deptID]
}

// LookupLegalEntityName 法人实体 ID → 公司名 (合思 dimensions 自定义维度 "法人实体")
// 注意: 法人实体 ID 是无 corp prefix 的纯 ID, 如 "ID01KiKNGdLTLF"
// corp prefix (从 token 第二段取) 是 dimensions 接口的 dimensionId 前缀
func (h *DashboardHandler) LookupLegalEntityName(entityID string) string {
	if entityID == "" {
		return ""
	}
	hesiLegalCacheMu.Lock()
	defer hesiLegalCacheMu.Unlock()
	if time.Since(hesiLegalCacheAt) >= hesiDictTTL || hesiLegalEntityCache == nil {
		// 从 token 中提取 corp ID (token 格式 "xxx:corpId")
		token, err := h.getHesiToken()
		if err != nil {
			return ""
		}
		corpID := token
		if idx := strings.Index(token, ":"); idx > 0 {
			corpID = token[idx+1:]
		}
		// URL encode "法人实体"
		dimID := corpID + ":" + "法人实体"
		path := "/api/openapi/v2/dimensions/items?dimensionId=" + neturl.QueryEscape(dimID)
		m, err := h.fetchHesiDictMap(path)
		if err != nil {
			return ""
		}
		hesiLegalEntityCache = m
		hesiLegalCacheAt = time.Now()
	}
	return hesiLegalEntityCache[entityID]
}

