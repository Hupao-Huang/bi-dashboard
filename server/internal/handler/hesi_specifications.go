package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

