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
	// v1.71.0: 解析失败用零值 → AccessToken 为空 → 下游 if 兜底, 加 log 利于排查
	if err := json.Unmarshal(tokenRespBody, &tokenData); err != nil {
		log.Printf("[dingtalk-login] token 响应解析失败: %v body=%s", err, string(tokenRespBody))
	}
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
	// v1.71.0: 解析失败下游 if 兜底, 加 log
	if err := json.Unmarshal(meRespBody, &dtUser); err != nil {
		log.Printf("[dingtalk-login] me 响应解析失败: %v body=%s", err, string(meRespBody))
	}
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
				// v1.70.6: 备注写库失败必须告知用户, 否则用户以为申请了管理员看不到 remark
				if _, err := h.DB.Exec(`UPDATE users SET remark = ? WHERE id = ?`, remark, userID); err != nil {
					log.Printf("[dingtalk-login] 写 remark 失败 user_id=%d: %v", userID, err)
					writeServerError(w, 500, "提交申请失败, 请重试或联系管理员", err)
					return
				}
			}
			writeJSON(w, map[string]interface{}{"pending": true, "message": "注册申请已提交，请等待管理员审批开通"})
			return
		}
		if userStatus != "active" {
			writeError(w, http.StatusForbidden, "账号已被禁用")
			return
		}
		// v1.60.2 登录后异步同步钉钉真名(不阻塞登录)
		h.asyncSyncDingtalkName(userID, dtID)
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
			// v1.70.6: 自动绑定写库失败必须告知, 否则用户下次扫码又被认成新人重复绑
			if _, exErr := h.DB.Exec(`UPDATE users SET dingtalk_userid = ? WHERE id = ?`, dtID, userID); exErr != nil {
				log.Printf("[dingtalk-login] 自动绑定失败 user_id=%d dtID=%s: %v", userID, dtID, exErr)
				writeServerError(w, 500, "钉钉账号自动绑定失败, 请联系管理员", exErr)
				return
			}
			if userStatus != "active" {
				writeError(w, http.StatusForbidden, "账号已被禁用")
				return
			}
			// v1.60.2 登录后异步同步钉钉真名(不阻塞登录)
			h.asyncSyncDingtalkName(userID, dtID)
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
	// v1.71.0: 加 err 检查利于上游排查 (下游 if 已兜底)
	if err := json.Unmarshal(tokenRespBody, &corpToken); err != nil {
		log.Printf("[dingtalk-dept] corpToken 解析失败: %v body=%s", err, string(tokenRespBody))
	}
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
	if err := json.Unmarshal(unionRespBody, &unionData); err != nil {
		log.Printf("[dingtalk-dept] getbyunionid 解析失败: %v body=%s", err, string(unionRespBody))
	}
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
	if err := json.Unmarshal(detailRespBody, &detailData); err != nil {
		log.Printf("[dingtalk-dept] user/get 解析失败: %v body=%s", err, string(detailRespBody))
	}
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
		if err := json.Unmarshal(deptRespBody, &deptData); err != nil {
			log.Printf("[dingtalk-dept] department/get 解析失败: %v body=%s", err, string(deptRespBody))
		}
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
	// v1.70.6: 最后登录时间是审计辅助字段, 失败只记日志不阻塞登录
	if _, err := h.DB.Exec(`UPDATE users SET last_login_at = NOW() WHERE id = ?`, userID); err != nil {
		log.Printf("[session] 更新 last_login_at 失败 user_id=%d: %v", userID, err)
	}

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
		// v1.70.6: 解绑失败必须告知, 否则用户以为解了实际还绑着
		if _, err := h.DB.Exec(`UPDATE users SET dingtalk_userid = '' WHERE id = ?`, payload.User.ID); err != nil {
			log.Printf("[dingtalk-bind] 解绑失败 user_id=%d: %v", payload.User.ID, err)
			writeServerError(w, 500, "解绑失败, 请重试", err)
			return
		}
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
	// v1.71.0: 加 err 检查 (下游 if 已兜底)
	if err := json.Unmarshal(tokenRespBody, &tokenData); err != nil {
		log.Printf("[dingtalk-bind] token 解析失败: %v body=%s", err, string(tokenRespBody))
	}
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
	if err := json.Unmarshal(meBody, &dtUser); err != nil {
		log.Printf("[dingtalk-bind] me 解析失败: %v body=%s", err, string(meBody))
	}
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
	// v1.71.0: 绑定 UPDATE 失败必须告诉用户, 不能"假绑定成功"
	if _, err := h.DB.Exec(`UPDATE users SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		log.Printf("[dingtalk-bind] UPDATE 失败 user_id=%d: %v", payload.User.ID, err)
		writeServerError(w, 500, "钉钉绑定写库失败, 请重试", err)
		return
	}
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

		// v1.71.0: cleanup 任务, 失败只记日志不阻塞
		if _, err := h.DB.Exec("DELETE FROM user_sessions WHERE expires_at < ?", now); err != nil {
			log.Printf("[cleanup] 清理过期 session 失败: %v", err)
		}
	}
}
