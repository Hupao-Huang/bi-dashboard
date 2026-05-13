package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
)

// HesiApprovalQueue 查队列状态 (GET /api/hesi-bot/approve/queue)
// 返回: 当前用户的 queued/running 单 + 最近完成 20 条 + 全局排队总数
// admin 可加 ?all=1 查全部
func (h *DashboardHandler) HesiApprovalQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, 405, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, 401, "unauthorized")
		return
	}

	isAdmin := hasPermission(payload, "user.manage")
	showAll := r.URL.Query().Get("all") == "1" && isAdmin

	// 全局队列长度 (任何登录用户可看, 评估等待时间)
	var totalQueued, totalRunning int
	h.DB.QueryRow(`SELECT COUNT(*) FROM hesi_approval_queue WHERE status='queued'`).Scan(&totalQueued)
	h.DB.QueryRow(`SELECT COUNT(*) FROM hesi_approval_queue WHERE status='running'`).Scan(&totalRunning)

	// 拉用户自己 (或全部) 的 active + 最近完成
	whereUser := ""
	args := []interface{}{}
	if !showAll {
		whereUser = " AND user_id = ?"
		args = append(args, payload.User.ID)
	}

	type queueRow struct {
		ID         int64   `json:"id"`
		UserID     int64   `json:"userId"`
		RealName   string  `json:"realName"`
		FlowID     string  `json:"flowId"`
		FlowCode   string  `json:"flowCode"`
		FlowTitle  string  `json:"flowTitle"`
		Action     string  `json:"action"`
		Comment    string  `json:"comment"`
		Status     string  `json:"status"`
		ErrorMsg   *string `json:"errorMsg"`
		CreatedAt  string  `json:"createdAt"`
		FinishedAt *string `json:"finishedAt"`
	}

	queryFn := func(extraWhere string) []queueRow {
		q := `SELECT id, user_id, real_name, flow_id, flow_code, flow_title, action,
		             IFNULL(comment,''), status, error_msg,
		             DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s'),
		             DATE_FORMAT(finished_at,'%Y-%m-%d %H:%i:%s')
		      FROM hesi_approval_queue
		      WHERE 1=1 ` + extraWhere + whereUser + ` ORDER BY id DESC LIMIT 50`
		rows, err := h.DB.Query(q, args...)
		if err != nil {
			return []queueRow{}
		}
		defer rows.Close()
		var out []queueRow
		for rows.Next() {
			var it queueRow
			var errMsg, finishedAt sql.NullString
			if err := rows.Scan(&it.ID, &it.UserID, &it.RealName, &it.FlowID, &it.FlowCode,
				&it.FlowTitle, &it.Action, &it.Comment, &it.Status, &errMsg,
				&it.CreatedAt, &finishedAt); err != nil {
				continue
			}
			if errMsg.Valid {
				s := errMsg.String
				if len(s) > 300 {
					s = s[:300] + "..."
				}
				it.ErrorMsg = &s
			}
			if finishedAt.Valid {
				it.FinishedAt = &finishedAt.String
			}
			out = append(out, it)
		}
		if out == nil {
			out = []queueRow{}
		}
		return out
	}

	active := queryFn(" AND status IN ('queued','running') ")
	recent := queryFn(" AND status IN ('success','failed') ")

	// 估算等待时间: 全局 queued+running × 65s
	estimateSeconds := (totalQueued + totalRunning) * 65

	writeJSON(w, map[string]interface{}{
		"active":          active,
		"recent":          recent,
		"totalQueued":     totalQueued,
		"totalRunning":    totalRunning,
		"estimateSeconds": estimateSeconds,
		"isAdmin":         isAdmin,
		"showingAll":      showAll,
	})
}

// HesiApprovalQueueItem 查单条队列状态 (GET /api/hesi-bot/approve/queue/{id})
func (h *DashboardHandler) HesiApprovalQueueItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, 405, "method not allowed")
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/hesi-bot/approve/queue/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, 400, "invalid id")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, 401, "unauthorized")
		return
	}

	var (
		uid                       int64
		realName, flowCode, title string
		action, status            string
		errMsg, result            sql.NullString
		createdAt, finishedAt     sql.NullString
	)
	err = h.DB.QueryRow(`
		SELECT user_id, real_name, flow_code, flow_title, action, status, error_msg, result,
		       DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s'),
		       DATE_FORMAT(finished_at,'%Y-%m-%d %H:%i:%s')
		FROM hesi_approval_queue WHERE id=?`, id).
		Scan(&uid, &realName, &flowCode, &title, &action, &status, &errMsg, &result, &createdAt, &finishedAt)
	if err == sql.ErrNoRows {
		writeError(w, 404, "not found")
		return
	}
	if err != nil {
		writeServerError(w, 500, "查询失败", err)
		return
	}
	// 非 admin 只能查自己的
	if uid != payload.User.ID && !hasPermission(payload, "user.manage") {
		writeError(w, 403, "无权访问")
		return
	}

	resp := map[string]interface{}{
		"id":        id,
		"userId":    uid,
		"realName":  realName,
		"flowCode":  flowCode,
		"flowTitle": title,
		"action":    action,
		"status":    status,
	}
	if errMsg.Valid {
		resp["errorMsg"] = errMsg.String
	}
	if result.Valid {
		resp["result"] = result.String
	}
	if createdAt.Valid {
		resp["createdAt"] = createdAt.String
	}
	if finishedAt.Valid {
		resp["finishedAt"] = finishedAt.String
	}
	writeJSON(w, resp)
}
