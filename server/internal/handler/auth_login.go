package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type loginAttempt struct {
	count    int
	lockedAt time.Time
}

var (
	loginAttempts = make(map[string]*loginAttempt)
	loginMu       sync.Mutex
)

func checkLoginLock(ip string) (bool, int) {
	loginMu.Lock()
	defer loginMu.Unlock()
	attempt, ok := loginAttempts[ip]
	if !ok {
		return false, 0
	}
	if !attempt.lockedAt.IsZero() && time.Now().Before(attempt.lockedAt.Add(lockDuration)) {
		remaining := int(time.Until(attempt.lockedAt.Add(lockDuration)).Minutes()) + 1
		return true, remaining
	}
	if !attempt.lockedAt.IsZero() && time.Now().After(attempt.lockedAt.Add(lockDuration)) {
		delete(loginAttempts, ip)
		return false, 0
	}
	return false, 0
}

func recordLoginFailure(ip string) int {
	loginMu.Lock()
	defer loginMu.Unlock()
	attempt, ok := loginAttempts[ip]
	if !ok {
		attempt = &loginAttempt{}
		loginAttempts[ip] = attempt
	}
	attempt.count++
	if attempt.count >= maxLoginAttempts {
		attempt.lockedAt = time.Now()
		return 0
	}
	return maxLoginAttempts - attempt.count
}

func clearLoginFailure(ip string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	delete(loginAttempts, ip)
}


func (h *DashboardHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ip := clientIP(r)
	if locked, remaining := checkLoginLock(ip); locked {
		writeError(w, http.StatusTooManyRequests, fmt.Sprintf("登录失败次数过多，请 %d 分钟后重试", remaining))
		return
	}

	var req struct {
		Username      string `json:"username"`
		Password      string `json:"password"`
		Remember      bool   `json:"remember"`
		CaptchaID     string `json:"captchaId"`
		CaptchaAnswer int    `json:"captchaAnswer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "请输入账号和密码")
		return
	}

	if req.CaptchaID == "" {
		writeError(w, http.StatusBadRequest, "请完成验证码")
		return
	}
	if !verifyCaptcha(req.CaptchaID, req.CaptchaAnswer) {
		writeError(w, http.StatusBadRequest, "验证码错误或已过期")
		return
	}

	var (
		userID       int64
		passwordHash string
		realName     sql.NullString
		status       string
	)
	err := h.DB.QueryRow(
		`SELECT id, password_hash, real_name, status FROM users WHERE username = ?`,
		req.Username,
	).Scan(&userID, &passwordHash, &realName, &status)
	if errors.Is(err, sql.ErrNoRows) {
		remaining := recordLoginFailure(ip)
		if remaining > 0 {
			writeError(w, http.StatusUnauthorized, fmt.Sprintf("账号或密码错误，还剩 %d 次机会", remaining))
		} else {
			writeError(w, http.StatusTooManyRequests, fmt.Sprintf("登录失败次数过多，请 %d 分钟后重试", int(lockDuration.Minutes())))
		}
		return
	}
	if writeDatabaseError(w, err) {
		return
	}
	if status != "active" {
		writeError(w, http.StatusForbidden, "账号已被禁用，请联系管理员")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		remaining := recordLoginFailure(ip)
		if remaining > 0 {
			writeError(w, http.StatusUnauthorized, fmt.Sprintf("账号或密码错误，还剩 %d 次机会", remaining))
		} else {
			writeError(w, http.StatusTooManyRequests, fmt.Sprintf("登录失败次数过多，请 %d 分钟后重试", int(lockDuration.Minutes())))
		}
		return
	}

	clearLoginFailure(ip)

	token, err := generateSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "创建会话失败")
		return
	}
	tokenHash := hashSessionToken(token)
	duration := sessionDuration
	if !req.Remember {
		duration = shortSessionDuration
	}
	expiresAt := time.Now().Add(duration)

	if _, err := h.DB.Exec(
		`INSERT INTO user_sessions (user_id, token_hash, expires_at, ip, user_agent) VALUES (?, ?, ?, ?, ?)`,
		userID, tokenHash, expiresAt, ip, truncateString(r.UserAgent(), 255),
	); writeDatabaseError(w, err) {
		return
	}

	if _, err := h.DB.Exec(`UPDATE users SET last_login_at = NOW() WHERE id = ?`, userID); err != nil {
		log.Printf("update last_login_at failed: %v", err)
	}

	h.logAuditNoRequest(userID, req.Username, realName.String, "login", "密码登录", "", ip, truncateString(r.UserAgent(), 255))

	setSessionCookie(w, token, expiresAt, isSecureRequest(r))
	payload, err := h.loadAuthPayload(userID)
	if writeDatabaseError(w, err) {
		return
	}

	writeJSON(w, payload)
}

func (h *DashboardHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		if _, execErr := h.DB.Exec(`DELETE FROM user_sessions WHERE token_hash = ?`, hashSessionToken(cookie.Value)); execErr != nil {
			log.Printf("delete session failed: %v", execErr)
		}
	}

	// audit: logout
	payload, _ := authPayloadFromContext(r)
	if payload != nil {
		h.logAudit(r, "logout", "退出登录", nil)
	}
	clearSessionCookie(w)
	writeJSON(w, map[string]string{"message": "ok"})
}

func (h *DashboardHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
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
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if err := validatePassword(req.NewPassword, payload.User.Username); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var currentHash string
	err := h.DB.QueryRow("SELECT IFNULL(password_hash,'') FROM users WHERE id=?", payload.User.ID).Scan(&currentHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "查询用户失败")
		return
	}

	if currentHash != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.OldPassword)); err != nil {
			writeError(w, http.StatusBadRequest, "当前密码错误")
			return
		}
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "密码加密失败")
		return
	}

	if _, err := h.DB.Exec("UPDATE users SET password_hash=?, must_change_password=0 WHERE id=?", string(newHash), payload.User.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "更新密码失败")
		return
	}
	if err := h.revokeUserSessions(payload.User.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "会话失效失败")
		return
	}
	clearSessionCookie(w)
	writeJSON(w, map[string]string{"message": "密码修改成功，请重新登录"})
}

func validatePassword(password, username string) error {
	if len(password) < 8 {
		return fmt.Errorf("密码至少8位")
	}
	if strings.EqualFold(password, username) {
		return fmt.Errorf("密码不能和用户名相同")
	}
	hasUpper, hasLower, hasDigit := false, false, false
	for _, c := range password {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit {
		return fmt.Errorf("密码必须包含大写字母、小写字母和数字")
	}
	return nil
}

func (h *DashboardHandler) Me(w http.ResponseWriter, r *http.Request) {
	payload, ok := authPayloadFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, payload)
}
