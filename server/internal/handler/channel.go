package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
)

type ChannelInfo struct {
	ID              int64  `json:"id"`
	ChannelID       string `json:"channelId"`
	ChannelName     string `json:"channelName"`
	ChannelCode     string `json:"channelCode"`
	OnlinePlatName  string `json:"onlinePlatName"`
	CateName        string `json:"cateName"`
	ChannelTypeName string `json:"channelTypeName"`
	DepartName      string `json:"departName"`
	CompanyName     string `json:"companyName"`
	ResponsibleUser string `json:"responsibleUser"`
	Department      string `json:"department"`
}

// AdminChannels 渠道列表（支持搜索、筛选）
func (h *DashboardHandler) AdminChannels(w http.ResponseWriter, r *http.Request) {
	keyword := r.URL.Query().Get("keyword")
	dept := r.URL.Query().Get("department")
	plat := r.URL.Query().Get("platform")
	unmapped := r.URL.Query().Get("unmapped")

	query := `SELECT id, IFNULL(channel_id,''), IFNULL(channel_name,''), IFNULL(channel_code,''),
		IFNULL(online_plat_name,''), IFNULL(cate_name,''), IFNULL(channel_type_name,''),
		IFNULL(channel_depart_name,''), IFNULL(company_name,''), IFNULL(responsible_user,''),
		IFNULL(department,'')
		FROM sales_channel WHERE 1=1`
	args := []interface{}{}

	if keyword != "" {
		query += " AND (channel_name LIKE ? OR channel_code LIKE ? OR responsible_user LIKE ?)"
		kw := "%" + keyword + "%"
		args = append(args, kw, kw, kw)
	}
	if dept != "" {
		if dept == "unmapped" {
			query += " AND (department IS NULL OR department = '')"
		} else {
			query += " AND department = ?"
			args = append(args, dept)
		}
	}
	if unmapped == "1" {
		query += " AND (department IS NULL OR department = '')"
	}
	if plat != "" {
		query += " AND online_plat_name = ?"
		args = append(args, plat)
	}
	query += " ORDER BY department, online_plat_name, channel_name"

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer rows.Close()

	var channels []ChannelInfo
	for rows.Next() {
		var c ChannelInfo
		if err := rows.Scan(&c.ID, &c.ChannelID, &c.ChannelName, &c.ChannelCode,
			&c.OnlinePlatName, &c.CateName, &c.ChannelTypeName,
			&c.DepartName, &c.CompanyName, &c.ResponsibleUser, &c.Department); err != nil {
			writeError(w, 500, err.Error())
			return
		}
		channels = append(channels, c)
	}
	if channels == nil {
		channels = []ChannelInfo{}
	}

	// 统计信息
	var total, unmappedCount int
	if err := h.DB.QueryRow("SELECT COUNT(*) FROM sales_channel").Scan(&total); err != nil {
		log.Printf("channel stats total 查询失败: %v", err)
	}
	if err := h.DB.QueryRow("SELECT COUNT(*) FROM sales_channel WHERE department IS NULL OR department = ''").Scan(&unmappedCount); err != nil {
		log.Printf("channel stats unmapped 查询失败: %v", err)
	}

	// 平台列表（用于筛选下拉）
	platRows, err := h.DB.Query("SELECT DISTINCT online_plat_name FROM sales_channel WHERE online_plat_name IS NOT NULL AND online_plat_name != '' ORDER BY online_plat_name")
	var platforms []string
	if err != nil {
		log.Printf("平台列表查询失败: %v", err)
	} else if platRows != nil {
		defer platRows.Close()
		for platRows.Next() {
			var p string
			if err := platRows.Scan(&p); err != nil {
				log.Printf("平台名扫描失败: %v", err)
				continue
			}
			platforms = append(platforms, p)
		}
	}

	writeJSON(w, map[string]interface{}{
		"channels":      channels,
		"total":         total,
		"unmappedCount": unmappedCount,
		"platforms":     platforms,
	})
}

// ChannelByPath 处理 /api/admin/channels/{id} 的PUT请求
func (h *DashboardHandler) ChannelByPath(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/channels/")
	if path == "" {
		writeError(w, 400, "缺少渠道ID")
		return
	}
	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		writeError(w, 400, "无效的渠道ID")
		return
	}

	switch r.Method {
	case http.MethodPut:
		h.UpdateChannelDepartment(w, r, id)
	default:
		writeError(w, 405, "不支持的请求方法")
	}
}

// UpdateChannelDepartment 修改渠道的BI部门归属
func (h *DashboardHandler) UpdateChannelDepartment(w http.ResponseWriter, r *http.Request, id int64) {
	var req struct {
		Department string `json:"department"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "请求格式错误")
		return
	}

	// 验证部门值
	validDepts := map[string]bool{"ecommerce": true, "social": true, "offline": true, "distribution": true, "": true}
	if !validDepts[req.Department] {
		writeError(w, 400, "无效的部门值")
		return
	}

	// 两表更新包事务，保证 sales_channel 和 sales_goods_summary 的 department 强一致
	tx, err := h.DB.Begin()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer tx.Rollback()

	result, err := tx.Exec("UPDATE sales_channel SET department = ? WHERE id = ?", req.Department, id)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, 404, "渠道不存在")
		return
	}

	// 同步更新 sales_goods_summary 中对应渠道的 department
	var channelID string
	if err := tx.QueryRow("SELECT channel_id FROM sales_channel WHERE id = ?", id).Scan(&channelID); err != nil {
		writeError(w, 500, "查询channel_id失败: "+err.Error())
		return
	}
	if channelID != "" {
		if _, err := tx.Exec("UPDATE sales_goods_summary SET department = ? WHERE shop_id = ?", req.Department, channelID); err != nil {
			writeError(w, 500, "同步sales_goods_summary部门失败: "+err.Error())
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, 500, "commit failed: "+err.Error())
		return
	}

	writeJSON(w, map[string]interface{}{"message": "更新成功"})
}

// SyncChannels 从吉客云同步渠道
func (h *DashboardHandler) SyncChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "不支持的请求方法")
		return
	}

	cmd := exec.Command(`C:\Users\Administrator\bi-dashboard\server\sync-channels.exe`)
	cmd.Dir = `C:\Users\Administrator\bi-dashboard\server`
	output, err := cmd.CombinedOutput()
	if err != nil {
		writeError(w, 500, "同步失败: "+err.Error()+"\n"+string(output))
		return
	}

	writeJSON(w, map[string]interface{}{
		"message": "同步完成",
		"output":  string(output),
	})
}
