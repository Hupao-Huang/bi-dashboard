package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (h *DashboardHandler) DingtalkAuthURL(w http.ResponseWriter, r *http.Request) {
	if h.DingClientID == "" {
		writeError(w, http.StatusBadRequest, "钉钉登录未配置")
		return
	}
	// 优先用 config 配置的回调域名（必须在钉钉应用后台白名单里）
	// 兜底：未配置时回退到 Referer，最后兜底到固定默认值
	host := strings.TrimRight(h.DingCallbackHost, "/")
	if host == "" {
		if referer := r.Header.Get("Referer"); referer != "" {
			if parsed, err := url.Parse(referer); err == nil && parsed.Host != "" {
				host = parsed.Scheme + "://" + parsed.Host
			}
		}
	}
	if host == "" {
		host = "http://192.168.200.48:3000"
	}
	redirectURI := host + "/dingtalk/callback"

	state := r.URL.Query().Get("state")
	if state != "login" && state != "bind" {
		state = "login"
	}
	authURL := fmt.Sprintf(
		"https://login.dingtalk.com/oauth2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=openid+corpid&prompt=consent&state=%s",
		h.DingClientID, url.QueryEscape(redirectURI), url.QueryEscape(state),
	)
	writeJSON(w, map[string]string{"url": authURL})
}

func (h *DashboardHandler) DingtalkLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Code         string `json:"code"`
		Remark       string `json:"remark"`
		PendingToken string `json:"pendingToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if req.PendingToken != "" {
		val, ok := dingtalkPendingUsers.LoadAndDelete(req.PendingToken)
		if !ok {
			writeError(w, http.StatusBadRequest, "注册信息已过期，请重新扫码")
			return
		}
		pending := val.(*dingtalkPendingUser)
		if time.Now().After(pending.Expires) {
			writeError(w, http.StatusBadRequest, "注册信息已过期，请重新扫码")
			return
		}
		remark := strings.TrimSpace(req.Remark)
		if pending.Department != "" {
			remark = "【" + pending.Department + "】" + remark
		}
		dtID := pending.UnionId
		if dtID == "" {
			dtID = pending.OpenId
		}
		mobile := strings.TrimSpace(pending.Mobile)
		username := mobile
		if username == "" {
			if len(dtID) >= 12 {
				username = "dt_" + dtID[:12]
			} else {
				username = "dt_" + dtID
			}
		}
		result, err := h.DB.Exec(
			`INSERT INTO users (username, password_hash, real_name, phone, dingtalk_userid, status, remark) VALUES (?, '', ?, ?, ?, 'pending', ?)`,
			username, pending.Nick, mobile, dtID, remark,
		)
		if err != nil {
			writeError(w, http.StatusConflict, "该手机号已注册，请联系管理员")
			return
		}
		newID, _ := result.LastInsertId()
		log.Printf("dingtalk new user registered: id=%d name=%s phone=%s", newID, pending.Nick, mobile)
		writeJSON(w, map[string]interface{}{"pending": true, "message": "注册申请已提交，请等待管理员审批开通"})
		return
	}

	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	// 1. 用 code 换 access_token
	tokenBody, _ := json.Marshal(map[string]string{
		"clientId":     h.DingClientID,
		"clientSecret": h.DingClientSecret,
		"code":         req.Code,
		"grantType":    "authorization_code",
	})
	tokenResp, err := http.Post("https://api.dingtalk.com/v1.0/oauth2/userAccessToken", "application/json", bytes.NewReader(tokenBody))
	if err != nil {
		writeError(w, http.StatusBadGateway, "钉钉服务器连接失败")
		return
	}
	defer tokenResp.Body.Close()
	tokenRespBody, _ := io.ReadAll(tokenResp.Body)

	var tokenData struct {
		AccessToken string `json:"accessToken"`
	}
	json.Unmarshal(tokenRespBody, &tokenData)
	if tokenData.AccessToken == "" {
		log.Printf("dingtalk token error: %s", string(tokenRespBody))
		writeError(w, http.StatusBadRequest, "钉钉授权失败，请重试")
		return
	}

	// 2. 用 access_token 获取用户信息
	meReq, _ := http.NewRequest("GET", "https://api.dingtalk.com/v1.0/contact/users/me", nil)
	meReq.Header.Set("x-acs-dingtalk-access-token", tokenData.AccessToken)
	meResp, err := http.DefaultClient.Do(meReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "获取钉钉用户信息失败")
		return
	}
	defer meResp.Body.Close()
	meRespBody, _ := io.ReadAll(meResp.Body)

	var dtUser struct {
		Nick    string `json:"nick"`
		UnionId string `json:"unionId"`
		OpenId  string `json:"openId"`
		Mobile  string `json:"mobile"`
	}
	json.Unmarshal(meRespBody, &dtUser)
	if dtUser.UnionId == "" && dtUser.OpenId == "" {
		log.Printf("dingtalk user error: %s", string(meRespBody))
		writeError(w, http.StatusBadRequest, "获取钉钉身份失败")
		return
	}
	dtID := dtUser.UnionId
	if dtID == "" {
		dtID = dtUser.OpenId
	}

	// 3. 查 dingtalk_userid 是否已绑定
	var userID int64
	var userStatus string
	err = h.DB.QueryRow(
		`SELECT id, status FROM users WHERE dingtalk_userid = ? AND dingtalk_userid != ''`, dtID,
	).Scan(&userID, &userStatus)

	if err == nil {
		// 已绑定
		if userStatus == "pending" {
			remark := strings.TrimSpace(req.Remark)
			if remark != "" {
				h.DB.Exec(`UPDATE users SET remark = ? WHERE id = ?`, remark, userID)
			}
			writeJSON(w, map[string]interface{}{"pending": true, "message": "注册申请已提交，请等待管理员审批开通"})
			return
		}
		if userStatus != "active" {
			writeError(w, http.StatusForbidden, "账号已被禁用")
			return
		}
		h.createSessionAndRespond(w, r, userID)
		return
	}

	// 4. 用手机号匹配已有账号
	mobile := strings.TrimSpace(dtUser.Mobile)
	if mobile != "" {
		err = h.DB.QueryRow(
			`SELECT id, status FROM users WHERE (phone = ? OR username = ?) AND dingtalk_userid = ''`, mobile, mobile,
		).Scan(&userID, &userStatus)
		if err == nil {
			// 匹配到 → 自动绑定
			h.DB.Exec(`UPDATE users SET dingtalk_userid = ? WHERE id = ?`, dtID, userID)
			if userStatus != "active" {
				writeError(w, http.StatusForbidden, "账号已被禁用")
				return
			}
			h.createSessionAndRespond(w, r, userID)
			return
		}
	}

	// 5. 都没找到 → 查部门 + 缓存信息等待填写备注
	dept := h.getDingtalkDepartment(dtUser.UnionId)
	pendingToken, _ := generateSessionToken()
	dingtalkPendingUsers.Store(pendingToken, &dingtalkPendingUser{
		Nick:       dtUser.Nick,
		UnionId:    dtUser.UnionId,
		OpenId:     dtUser.OpenId,
		Mobile:     dtUser.Mobile,
		Department: dept,
		Expires:    time.Now().Add(10 * time.Minute),
	})
	writeJSON(w, map[string]interface{}{"needRemark": true, "nick": dtUser.Nick, "department": dept, "pendingToken": pendingToken})
	return
}

func (h *DashboardHandler) getDingtalkDepartment(unionId string) string {
	// 1. 获取企业 access_token
	tokenBody, _ := json.Marshal(map[string]string{
		"appKey":    h.DingClientID,
		"appSecret": h.DingClientSecret,
	})
	tokenResp, err := http.Post("https://api.dingtalk.com/v1.0/oauth2/accessToken", "application/json", bytes.NewReader(tokenBody))
	if err != nil {
		log.Printf("dingtalk corp token error: %v", err)
		return ""
	}
	defer tokenResp.Body.Close()
	tokenRespBody, _ := io.ReadAll(tokenResp.Body)
	var corpToken struct {
		AccessToken string `json:"accessToken"`
	}
	json.Unmarshal(tokenRespBody, &corpToken)
	if corpToken.AccessToken == "" {
		log.Printf("dingtalk corp token empty: %s", string(tokenRespBody))
		return ""
	}

	// 2. 通过 unionId 查 userid
	unionBody, _ := json.Marshal(map[string]string{"unionid": unionId})
	unionReq, _ := http.NewRequest("POST", "https://oapi.dingtalk.com/topapi/user/getbyunionid?access_token="+corpToken.AccessToken, bytes.NewReader(unionBody))
	unionReq.Header.Set("Content-Type", "application/json")
	unionResp, err := http.DefaultClient.Do(unionReq)
	if err != nil {
		log.Printf("dingtalk getbyunionid error: %v", err)
		return ""
	}
	defer unionResp.Body.Close()
	unionRespBody, _ := io.ReadAll(unionResp.Body)
	var unionData struct {
		ErrCode int `json:"errcode"`
		Result  struct {
			UserId string `json:"userid"`
		} `json:"result"`
	}
	json.Unmarshal(unionRespBody, &unionData)
	if unionData.Result.UserId == "" {
		log.Printf("dingtalk getbyunionid empty: %s", string(unionRespBody))
		return ""
	}

	// 3. 查用户详情获取部门ID
	detailBody, _ := json.Marshal(map[string]string{"userid": unionData.Result.UserId})
	detailReq, _ := http.NewRequest("POST", "https://oapi.dingtalk.com/topapi/v2/user/get?access_token="+corpToken.AccessToken, bytes.NewReader(detailBody))
	detailReq.Header.Set("Content-Type", "application/json")
	detailResp, err := http.DefaultClient.Do(detailReq)
	if err != nil {
		log.Printf("dingtalk user detail error: %v", err)
		return ""
	}
	defer detailResp.Body.Close()
	detailRespBody, _ := io.ReadAll(detailResp.Body)
	var detailData struct {
		ErrCode int `json:"errcode"`
		Result  struct {
			DeptIdList []int64 `json:"dept_id_list"`
		} `json:"result"`
	}
	json.Unmarshal(detailRespBody, &detailData)
	if len(detailData.Result.DeptIdList) == 0 {
		log.Printf("dingtalk user detail no dept: %s", string(detailRespBody))
		return ""
	}

	// 4. 查部门名称
	deptNames := []string{}
	for _, deptId := range detailData.Result.DeptIdList {
		deptBody, _ := json.Marshal(map[string]interface{}{"dept_id": deptId})
		deptReq, _ := http.NewRequest("POST", "https://oapi.dingtalk.com/topapi/v2/department/get?access_token="+corpToken.AccessToken, bytes.NewReader(deptBody))
		deptReq.Header.Set("Content-Type", "application/json")
		deptResp, err := http.DefaultClient.Do(deptReq)
		if err != nil {
			continue
		}
		deptRespBody, _ := io.ReadAll(deptResp.Body)
		deptResp.Body.Close()
		var deptData struct {
			ErrCode int `json:"errcode"`
			Result  struct {
				Name string `json:"name"`
			} `json:"result"`
		}
		json.Unmarshal(deptRespBody, &deptData)
		if deptData.Result.Name != "" {
			deptNames = append(deptNames, deptData.Result.Name)
		}
	}

	result := strings.Join(deptNames, " / ")
	log.Printf("dingtalk dept for %s: %s", unionId, result)
	return result
}

func (h *DashboardHandler) createSessionAndRespond(w http.ResponseWriter, r *http.Request, userID int64) {
	token, err := generateSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "生成会话失败")
		return
	}
	tokenHash := hashSessionToken(token)
	expiresAt := time.Now().Add(sessionDuration)
	ip := clientIP(r)

	if _, err := h.DB.Exec(
		`INSERT INTO user_sessions (user_id, token_hash, expires_at, ip, user_agent) VALUES (?, ?, ?, ?, ?)`,
		userID, tokenHash, expiresAt, ip, truncateString(r.UserAgent(), 255),
	); err != nil {
		writeError(w, http.StatusInternalServerError, "创建会话失败")
		return
	}
	h.DB.Exec(`UPDATE users SET last_login_at = NOW() WHERE id = ?`, userID)

	// 先加载 payload, 才能拿到 username/real_name 喂给 audit
	// 之前 audit 用空串导致钉钉扫码登录的审计记录"用户"列空白(虎跑等用户登录看不到)
	payload, err := h.loadAuthPayload(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "加载用户信息失败")
		return
	}

	h.logAuditNoRequest(userID, payload.User.Username, payload.User.RealName, "login", "钉钉扫码登录", "", ip, truncateString(r.UserAgent(), 255))
	setSessionCookie(w, token, expiresAt, isSecureRequest(r))
	writeJSON(w, payload)
}

func (h *DashboardHandler) DingtalkBind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Code   string `json:"code"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	// 解绑
	if req.Action == "unbind" {
		h.DB.Exec(`UPDATE users SET dingtalk_userid = '' WHERE id = ?`, payload.User.ID)
		writeJSON(w, map[string]string{"message": "已解绑钉钉"})
		return
	}

	// 绑定：用 code 换钉钉信息
	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	tokenBody, _ := json.Marshal(map[string]string{
		"clientId":     h.DingClientID,
		"clientSecret": h.DingClientSecret,
		"code":         req.Code,
		"grantType":    "authorization_code",
	})
	tokenResp, err := http.Post("https://api.dingtalk.com/v1.0/oauth2/userAccessToken", "application/json", bytes.NewReader(tokenBody))
	if err != nil {
		writeError(w, http.StatusBadGateway, "钉钉服务器连接失败")
		return
	}
	defer tokenResp.Body.Close()
	tokenRespBody, _ := io.ReadAll(tokenResp.Body)

	var tokenData struct {
		AccessToken string `json:"accessToken"`
	}
	json.Unmarshal(tokenRespBody, &tokenData)
	if tokenData.AccessToken == "" {
		writeError(w, http.StatusBadRequest, "钉钉授权失败")
		return
	}

	meReq, _ := http.NewRequest("GET", "https://api.dingtalk.com/v1.0/contact/users/me", nil)
	meReq.Header.Set("x-acs-dingtalk-access-token", tokenData.AccessToken)
	meResp, err := http.DefaultClient.Do(meReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "获取钉钉用户信息失败")
		return
	}
	defer meResp.Body.Close()
	meBody, _ := io.ReadAll(meResp.Body)

	var dtUser struct {
		Nick    string `json:"nick"`
		UnionId string `json:"unionId"`
		OpenId  string `json:"openId"`
		Mobile  string `json:"mobile"`
	}
	json.Unmarshal(meBody, &dtUser)
	dtID := dtUser.UnionId
	if dtID == "" {
		dtID = dtUser.OpenId
	}
	if dtID == "" {
		writeError(w, http.StatusBadRequest, "获取钉钉身份失败")
		return
	}

	// 检查是否已被其他账号绑定
	var existID int64
	err = h.DB.QueryRow(`SELECT id FROM users WHERE dingtalk_userid = ? AND id != ?`, dtID, payload.User.ID).Scan(&existID)
	if err == nil {
		writeError(w, http.StatusConflict, "该钉钉账号已绑定其他用户")
		return
	}

	sets := []string{"dingtalk_userid = ?"}
	args := []interface{}{dtID}
	if strings.TrimSpace(dtUser.Nick) != "" {
		sets = append(sets, "real_name = ?")
		args = append(args, strings.TrimSpace(dtUser.Nick))
	}
	if strings.TrimSpace(dtUser.Mobile) != "" {
		sets = append(sets, "phone = ?")
		args = append(args, strings.TrimSpace(dtUser.Mobile))
	}
	args = append(args, payload.User.ID)
	h.DB.Exec(`UPDATE users SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	writeJSON(w, map[string]string{"message": "钉钉绑定成功", "nick": dtUser.Nick, "mobile": dtUser.Mobile})
}

func (h *DashboardHandler) StartCleanupRoutines() {
	ticker := time.NewTicker(10 * time.Minute)
	for range ticker.C {
		now := time.Now()

		captchaMu.Lock()
		for id, entry := range captchaStore {
			if now.After(entry.expiresAt) {
				delete(captchaStore, id)
			}
		}
		captchaMu.Unlock()

		loginMu.Lock()
		for ip, attempt := range loginAttempts {
			if !attempt.lockedAt.IsZero() && now.Sub(attempt.lockedAt) > 30*time.Minute {
				delete(loginAttempts, ip)
			}
		}
		loginMu.Unlock()

		dingtalkPendingUsers.Range(func(key, value interface{}) bool {
			if entry, ok := value.(*dingtalkPendingUser); ok {
				if now.After(entry.Expires) {
					dingtalkPendingUsers.Delete(key)
				}
			}
			return true
		})

		h.DB.Exec("DELETE FROM user_sessions WHERE expires_at < ?", now)
	}
}
