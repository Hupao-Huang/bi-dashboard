package handler

// v1.60.0 合思机器人规则 CRUD
// 用户自定义自动审批规则: 字段+操作符+值的 AND 条件组, 配金额上限护栏
// dry_run=1 默认干跑 (v1.61 才扫描匹配), enabled=0 默认关 (v1.62 才真审批)

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type hesiRule struct {
	ID             int64           `json:"id"`
	UserID         int64           `json:"userId"`
	Name           string          `json:"name"`
	Enabled        bool            `json:"enabled"`
	DryRun         bool            `json:"dryRun"`
	ActionType     string          `json:"actionType"`     // agree | reject
	ApproveComment string          `json:"approveComment"`
	MaxAmount      float64         `json:"maxAmount"`
	Conditions     json.RawMessage `json:"conditions"`
	Priority       int             `json:"priority"`
	MatchedCount   int             `json:"matchedCount"`
	ApprovedCount  int             `json:"approvedCount"`
	LastMatchedAt  *string         `json:"lastMatchedAt"`
	CreatedAt      string          `json:"createdAt"`
	UpdatedAt      string          `json:"updatedAt"`
}

// ListMyHesiRules GET /api/profile/hesi-rules
func (h *DashboardHandler) ListMyHesiRules(w http.ResponseWriter, r *http.Request) {
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	rows, err := h.DB.Query(`SELECT id, user_id, name, enabled, dry_run, action_type, approve_comment,
		max_amount, conditions_json, priority, matched_count, approved_count,
		DATE_FORMAT(last_matched_at,'%Y-%m-%d %H:%i:%s'),
		DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s'),
		DATE_FORMAT(updated_at,'%Y-%m-%d %H:%i:%s')
		FROM hesi_auto_rule WHERE user_id=? ORDER BY priority ASC, id DESC`, payload.User.ID)
	if err != nil {
		writeServerError(w, 500, "查规则失败", err)
		return
	}
	defer rows.Close()
	items := []hesiRule{}
	for rows.Next() {
		var ru hesiRule
		var condRaw sql.NullString
		var enabled, dryRun int
		if err := rows.Scan(&ru.ID, &ru.UserID, &ru.Name, &enabled, &dryRun, &ru.ActionType, &ru.ApproveComment,
			&ru.MaxAmount, &condRaw, &ru.Priority, &ru.MatchedCount, &ru.ApprovedCount,
			&ru.LastMatchedAt, &ru.CreatedAt, &ru.UpdatedAt); err != nil {
			continue
		}
		ru.Enabled = enabled == 1
		ru.DryRun = dryRun == 1
		if condRaw.Valid {
			ru.Conditions = json.RawMessage(condRaw.String)
		} else {
			ru.Conditions = json.RawMessage("[]")
		}
		items = append(items, ru)
	}
	writeJSON(w, map[string]interface{}{"items": items, "count": len(items)})
}

type hesiRuleReq struct {
	Name           string          `json:"name"`
	Enabled        bool            `json:"enabled"`
	DryRun         *bool           `json:"dryRun"`
	ActionType     string          `json:"actionType"`
	ApproveComment string          `json:"approveComment"`
	MaxAmount      float64         `json:"maxAmount"`
	Conditions     json.RawMessage `json:"conditions"`
	Priority       int             `json:"priority"`
}

func validateRule(req *hesiRuleReq) error {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return fmt.Errorf("规则名不能为空")
	}
	if len(req.Name) > 100 {
		return fmt.Errorf("规则名太长(限100字)")
	}
	if req.ActionType != "agree" && req.ActionType != "reject" {
		req.ActionType = "agree"
	}
	if req.MaxAmount < 0 {
		req.MaxAmount = 0
	}
	if req.Priority < 1 || req.Priority > 9999 {
		req.Priority = 100
	}
	if len(req.Conditions) == 0 {
		req.Conditions = json.RawMessage("[]")
	} else {
		// 验证 JSON
		var arr []map[string]interface{}
		if err := json.Unmarshal(req.Conditions, &arr); err != nil {
			return fmt.Errorf("条件 JSON 格式错: %v", err)
		}
	}
	return nil
}

// CreateMyHesiRule POST /api/profile/hesi-rules
func (h *DashboardHandler) CreateMyHesiRule(w http.ResponseWriter, r *http.Request) {
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req hesiRuleReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错")
		return
	}
	if err := validateRule(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	dryRun := 1
	if req.DryRun != nil && !*req.DryRun {
		dryRun = 0
	}
	enabled := 0
	if req.Enabled {
		enabled = 1
	}
	res, err := h.DB.Exec(`INSERT INTO hesi_auto_rule
		(user_id, name, enabled, dry_run, action_type, approve_comment, max_amount, conditions_json, priority)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		payload.User.ID, req.Name, enabled, dryRun, req.ActionType, req.ApproveComment,
		req.MaxAmount, string(req.Conditions), req.Priority)
	if err != nil {
		writeServerError(w, 500, "创建规则失败", err)
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, map[string]interface{}{"id": id})
}

// HesiRuleByPath PUT / DELETE /api/profile/hesi-rules/{id}
func (h *DashboardHandler) HesiRuleByPath(w http.ResponseWriter, r *http.Request) {
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/profile/hesi-rules/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "无效 ID")
		return
	}
	// 鉴权: 只能改自己的
	var ownerID int64
	if err := h.DB.QueryRow(`SELECT user_id FROM hesi_auto_rule WHERE id=?`, id).Scan(&ownerID); err != nil {
		writeError(w, http.StatusNotFound, "规则不存在")
		return
	}
	if ownerID != payload.User.ID {
		writeError(w, http.StatusForbidden, "无权限修改他人规则")
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if _, err := h.DB.Exec(`DELETE FROM hesi_auto_rule WHERE id=?`, id); err != nil {
			writeServerError(w, 500, "删除失败", err)
			return
		}
		writeJSON(w, map[string]interface{}{"ok": true})
	case http.MethodPut:
		var req hesiRuleReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "请求格式错")
			return
		}
		if err := validateRule(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		dryRun := 1
		if req.DryRun != nil && !*req.DryRun {
			dryRun = 0
		}
		enabled := 0
		if req.Enabled {
			enabled = 1
		}
		if _, err := h.DB.Exec(`UPDATE hesi_auto_rule SET
			name=?, enabled=?, dry_run=?, action_type=?, approve_comment=?, max_amount=?,
			conditions_json=?, priority=? WHERE id=?`,
			req.Name, enabled, dryRun, req.ActionType, req.ApproveComment, req.MaxAmount,
			string(req.Conditions), req.Priority, id); err != nil {
			writeServerError(w, 500, "更新失败", err)
			return
		}
		writeJSON(w, map[string]interface{}{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
