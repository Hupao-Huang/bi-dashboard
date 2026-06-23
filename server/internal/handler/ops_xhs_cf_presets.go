package handler

// 乘风「自定义指标」常用方案：每个用户可存多套命名的指标列方案（含顺序）。
// 表 cf_metric_presets：user_id+name 唯一；metric_keys 存有序 JSON 数组字符串。
// 三接口都按当前登录用户隔离，只读/改自己的方案。

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

var cfPresetTableOnce sync.Once

// ensureCfPresetTable 懒建表（首次调用任一接口时建一次）
func (h *DashboardHandler) ensureCfPresetTable() {
	cfPresetTableOnce.Do(func() {
		_, _ = h.DB.Exec(`CREATE TABLE IF NOT EXISTS cf_metric_presets (
			id BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键',
			user_id BIGINT NOT NULL COMMENT '所属用户ID',
			name VARCHAR(20) NOT NULL COMMENT '方案名称',
			metric_keys TEXT NOT NULL COMMENT '有序指标Key的JSON数组',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
			UNIQUE KEY uk_user_name (user_id, name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='乘风自定义指标常用方案'`)
	})
}

type cfPreset struct {
	ID   int64    `json:"id"`
	Name string   `json:"name"`
	Keys []string `json:"keys"`
}

// ListCfPresets GET /api/xiaohongshu/chengfeng/presets —— 当前用户的常用方案列表
func (h *DashboardHandler) ListCfPresets(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDeptAccess(r, "social")) {
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	h.ensureCfPresetTable()
	rows, err := h.DB.Query(`SELECT id, name, metric_keys FROM cf_metric_presets WHERE user_id=? ORDER BY updated_at DESC, id DESC`, payload.User.ID)
	if err != nil {
		writeServerError(w, 500, "查常用方案失败", err)
		return
	}
	defer rows.Close()
	presets := []cfPreset{}
	for rows.Next() {
		var p cfPreset
		var keysRaw string
		if err := rows.Scan(&p.ID, &p.Name, &keysRaw); err != nil {
			continue
		}
		if json.Unmarshal([]byte(keysRaw), &p.Keys) != nil {
			p.Keys = []string{}
		}
		presets = append(presets, p)
	}
	writeJSON(w, map[string]interface{}{"presets": presets})
}

// SaveCfPreset POST /api/xiaohongshu/chengfeng/presets/save —— 新增/覆盖同名方案
func (h *DashboardHandler) SaveCfPreset(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDeptAccess(r, "social")) {
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Name string   `json:"name"`
		Keys []string `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "方案名称不能为空")
		return
	}
	if len([]rune(body.Name)) > 10 {
		writeError(w, http.StatusBadRequest, "方案名称最多10个字")
		return
	}
	if len(body.Keys) == 0 {
		writeError(w, http.StatusBadRequest, "至少选择一个指标")
		return
	}
	keysJSON, _ := json.Marshal(body.Keys)
	h.ensureCfPresetTable()
	// user_id+name 唯一 → 同名覆盖（更新 keys）
	if _, err := h.DB.Exec(`INSERT INTO cf_metric_presets (user_id, name, metric_keys) VALUES (?,?,?)
		ON DUPLICATE KEY UPDATE metric_keys=VALUES(metric_keys)`, payload.User.ID, body.Name, string(keysJSON)); err != nil {
		writeServerError(w, 500, "保存常用方案失败", err)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}

// DeleteCfPreset POST /api/xiaohongshu/chengfeng/presets/delete —— 删除自己的某个方案
func (h *DashboardHandler) DeleteCfPreset(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDeptAccess(r, "social")) {
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID <= 0 {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	h.ensureCfPresetTable()
	if _, err := h.DB.Exec(`DELETE FROM cf_metric_presets WHERE id=? AND user_id=?`, body.ID, payload.User.ID); err != nil {
		writeServerError(w, 500, "删除常用方案失败", err)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}
