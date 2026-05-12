package handler

// v1.60.2 同步钉钉企业通讯录真名到 users.dingtalk_real_name
// - 单同步: POST /api/profile/sync-dingtalk (当前用户)
// - 一键全员: POST /api/admin/sync-all-dingtalk-names (管理员)
// - 登录自动: DingtalkLogin 成功后异步触发, 不阻塞登录

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
)

// fetchDingtalkRealName 用钉钉 unionId 拉企业通讯录真名 + 手机号
// 失败时返回空字符串, 不报错(异步同步用, 不阻塞主流程)
func (h *DashboardHandler) fetchDingtalkRealName(unionId string) (name, mobile string) {
	if unionId == "" || h.DingClientID == "" || h.DingClientSecret == "" {
		return
	}
	// 1. 企业 access_token
	tokenBody, _ := json.Marshal(map[string]string{"appKey": h.DingClientID, "appSecret": h.DingClientSecret})
	tokenResp, err := http.Post("https://api.dingtalk.com/v1.0/oauth2/accessToken", "application/json", bytes.NewReader(tokenBody))
	if err != nil {
		log.Printf("[ding-sync] corp token error: %v", err)
		return
	}
	defer tokenResp.Body.Close()
	tokenBytes, _ := io.ReadAll(tokenResp.Body)
	var corpToken struct {
		AccessToken string `json:"accessToken"`
	}
	json.Unmarshal(tokenBytes, &corpToken)
	if corpToken.AccessToken == "" {
		log.Printf("[ding-sync] corp token empty: %s", string(tokenBytes))
		return
	}
	// 2. unionId → userid
	unionBody, _ := json.Marshal(map[string]string{"unionid": unionId})
	unionResp, err := http.Post("https://oapi.dingtalk.com/topapi/user/getbyunionid?access_token="+corpToken.AccessToken, "application/json", bytes.NewReader(unionBody))
	if err != nil {
		log.Printf("[ding-sync] getbyunionid error: %v", err)
		return
	}
	defer unionResp.Body.Close()
	unionBytes, _ := io.ReadAll(unionResp.Body)
	var unionData struct {
		Result struct {
			UserId string `json:"userid"`
		} `json:"result"`
	}
	json.Unmarshal(unionBytes, &unionData)
	if unionData.Result.UserId == "" {
		log.Printf("[ding-sync] getbyunionid empty userid: %s", string(unionBytes))
		return
	}
	// 3. userid → name + mobile
	detailBody, _ := json.Marshal(map[string]string{"userid": unionData.Result.UserId})
	detailResp, err := http.Post("https://oapi.dingtalk.com/topapi/v2/user/get?access_token="+corpToken.AccessToken, "application/json", bytes.NewReader(detailBody))
	if err != nil {
		log.Printf("[ding-sync] user/get error: %v", err)
		return
	}
	defer detailResp.Body.Close()
	detailBytes, _ := io.ReadAll(detailResp.Body)
	var detailData struct {
		Result struct {
			Name   string `json:"name"`
			Mobile string `json:"mobile"`
		} `json:"result"`
	}
	json.Unmarshal(detailBytes, &detailData)
	return strings.TrimSpace(detailData.Result.Name), strings.TrimSpace(detailData.Result.Mobile)
}

// asyncSyncDingtalkName 异步同步, 失败只打日志(用于登录后无阻塞触发)
func (h *DashboardHandler) asyncSyncDingtalkName(userID int64, unionId string) {
	go func() {
		name, _ := h.fetchDingtalkRealName(unionId)
		if name == "" {
			return
		}
		if _, err := h.DB.Exec(`UPDATE users SET dingtalk_real_name=? WHERE id=? AND (dingtalk_real_name IS NULL OR dingtalk_real_name='' OR dingtalk_real_name<>?)`, name, userID, name); err != nil {
			log.Printf("[ding-sync] update user_id=%d failed: %v", userID, err)
		}
	}()
}

// SyncMyDingtalk POST /api/profile/sync-dingtalk
func (h *DashboardHandler) SyncMyDingtalk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var unionId string
	if err := h.DB.QueryRow(`SELECT IFNULL(dingtalk_userid,'') FROM users WHERE id=?`, payload.User.ID).Scan(&unionId); err != nil {
		writeServerError(w, 500, "查用户失败", err)
		return
	}
	unionId = strings.TrimSpace(unionId)
	if unionId == "" {
		writeError(w, http.StatusBadRequest, "您账号未绑定钉钉, 请先用钉钉扫码登录")
		return
	}
	name, mobile := h.fetchDingtalkRealName(unionId)
	if name == "" {
		writeError(w, http.StatusBadGateway, "钉钉接口未返姓名(可能您不在企业通讯录或应用权限不足)")
		return
	}
	if _, err := h.DB.Exec(`UPDATE users SET dingtalk_real_name=? WHERE id=?`, name, payload.User.ID); err != nil {
		writeServerError(w, 500, "写入失败", err)
		return
	}
	writeJSON(w, map[string]interface{}{
		"realName": name,
		"mobile":   mobile,
		"ok":       true,
	})
}

// SyncAllDingtalk POST /api/admin/sync-all-dingtalk-names
func (h *DashboardHandler) SyncAllDingtalk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !hasPermission(payload, "user.manage") {
		writeError(w, http.StatusForbidden, "无权限")
		return
	}
	rows, err := h.DB.Query(`SELECT id, IFNULL(dingtalk_userid,''), IFNULL(real_name,'') FROM users WHERE dingtalk_userid IS NOT NULL AND dingtalk_userid<>''`)
	if err != nil {
		writeServerError(w, 500, "查用户列表失败", err)
		return
	}
	defer rows.Close()

	type userRow struct {
		ID      int64
		UnionId string
		Nick    string
	}
	var us []userRow
	for rows.Next() {
		var u userRow
		if err := rows.Scan(&u.ID, &u.UnionId, &u.Nick); err == nil {
			us = append(us, u)
		}
	}

	success := 0
	failed := []string{}
	for _, u := range us {
		name, _ := h.fetchDingtalkRealName(u.UnionId)
		if name == "" {
			failed = append(failed, u.Nick)
			continue
		}
		if _, err := h.DB.Exec(`UPDATE users SET dingtalk_real_name=? WHERE id=?`, name, u.ID); err == nil {
			success++
		}
	}
	writeJSON(w, map[string]interface{}{
		"total":   len(us),
		"success": success,
		"failed":  failed,
	})
}
