package handler

// auth_handlers_test.go — auth.go 未测的 handler + 纯函数 + captcha + lock 逻辑
// 已 Read auth.go:
//   - 纯函数 (1574-1697): setSessionCookie / clearSessionCookie / uniqueSortedStrings /
//     containsString / hasPermission / isSecureRequest / isTrustedProxyRemoteAddr / clientIP /
//     truncateString
//   - Captcha (286-331): verifyCaptcha / preVerifyCaptcha (memory: 一次性, 防爆破销毁)
//   - Lock (366-410): checkLoginLock / recordLoginFailure / clearLoginFailure
//   - Handler (191/334/1208/1230/1311): GetCaptcha / VerifyCaptchaOnly / Logout / ChangePassword / Me
//   - DingtalkAuthURL (1701)

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ 纯函数 ============

func TestUniqueSortedStringsDedupesAndSorts(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{[]string{"b", "a", "c"}, []string{"a", "b", "c"}},
		{[]string{"a", "a", "b"}, []string{"a", "b"}},                 // 去重
		{[]string{"", "a", "", "b"}, []string{"a", "b"}},               // 跳空
		{[]string{}, []string{}},
		{nil, []string{}},
		{[]string{"z", "a", "z", "", "m"}, []string{"a", "m", "z"}},
	}
	for _, tc := range cases {
		got := uniqueSortedStrings(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("uniqueSortedStrings(%v) len=%d want %d (got %v)", tc.in, len(got), len(tc.want), got)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("uniqueSortedStrings(%v)[%d]=%q want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

// containsString 已在 scope_test.go 测过

func TestHasPermission(t *testing.T) {
	// nil payload → false
	if hasPermission(nil, "anything") {
		t.Error("nil payload 应返 false")
	}

	// IsSuperAdmin = true → 任意 permission 都过
	sa := &authPayload{IsSuperAdmin: true}
	if !hasPermission(sa, "any.permission") {
		t.Error("super admin 应有所有权限")
	}

	// Roles 含 super_admin → 等同 super
	sr := &authPayload{Roles: []string{"super_admin"}}
	if !hasPermission(sr, "x") {
		t.Error("Roles 含 super_admin 应通行")
	}

	// 空 permission + 非 super → false
	p := &authPayload{Roles: []string{"user"}, Permissions: []string{"a"}}
	if hasPermission(p, "") {
		t.Error("空 permission 非 super 应 false")
	}

	// 普通用户 + 含权限 → true
	if !hasPermission(p, "a") {
		t.Error("含 permission 应 true")
	}
	if hasPermission(p, "b") {
		t.Error("不含 permission 应 false")
	}
}

func TestTruncateString(t *testing.T) {
	if got := truncateString("abc", 10); got != "abc" {
		t.Errorf("len <= max 应原样返, got %q", got)
	}
	if got := truncateString("abcdefghij", 5); len(got) > 5 {
		t.Errorf("应截断到 ≤5, got %q (len %d)", got, len(got))
	}
	// 中文 rune 安全
	cn := "中文截断测试" // 6 rune × 3 byte = 18 byte
	got := truncateString(cn, 6)
	if len([]byte(got)) > 6 {
		t.Errorf("中文截断应按字节 ≤6, got %q (len %d)", got, len([]byte(got)))
	}
}

func TestIsTrustedProxyRemoteAddr(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:1234":   true,  // loopback IPv4
		"127.0.0.1":        true,
		"[::1]:5678":       true,  // loopback IPv6
		"::1":              true,
		"8.8.8.8:9000":     false, // 公网
		"8.8.8.8":          false,
		"":                 false, // 空
		"not-an-ip:1234":   false, // bad
		"10.0.0.1":         false, // 内网但不是 loopback
	}
	for in, want := range cases {
		if got := isTrustedProxyRemoteAddr(in); got != want {
			t.Errorf("isTrustedProxyRemoteAddr(%q)=%v want %v", in, got, want)
		}
	}
}

func TestIsSecureRequest(t *testing.T) {
	// nil request
	if isSecureRequest(nil) {
		t.Error("nil request 应 false")
	}

	// 来自 loopback proxy + X-Forwarded-Proto=https → true
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "127.0.0.1:9999"
	req2.Header.Set("X-Forwarded-Proto", "https")
	if !isSecureRequest(req2) {
		t.Error("loopback + X-Forwarded-Proto=https 应 true")
	}

	// 公网 IP + X-Forwarded-Proto=https → false (not trusted)
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.RemoteAddr = "8.8.8.8:80"
	req3.Header.Set("X-Forwarded-Proto", "https")
	if isSecureRequest(req3) {
		t.Error("公网 IP + X-Forwarded-Proto 不应 trust")
	}

	// loopback 但 X-Forwarded-Proto=http → false
	req4 := httptest.NewRequest(http.MethodGet, "/", nil)
	req4.RemoteAddr = "127.0.0.1:9999"
	req4.Header.Set("X-Forwarded-Proto", "http")
	if isSecureRequest(req4) {
		t.Error("X-Forwarded-Proto=http 应 false")
	}

	// TLS 直连分支需要真实 TLS conn, 跳过 (主路径 X-Forwarded-Proto 已覆盖)
}

func TestClientIP(t *testing.T) {
	// nil request
	if got := clientIP(nil); got != "" {
		t.Errorf("nil 应空, got %q", got)
	}

	// loopback proxy + X-Forwarded-For → 取 forwarded
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
	if got := clientIP(req); got != "203.0.113.5" {
		t.Errorf("应取 X-Forwarded-For 第一个, got %q", got)
	}

	// loopback + X-Real-IP (无 Forwarded-For)
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "127.0.0.1:1234"
	req2.Header.Set("X-Real-IP", "198.51.100.7")
	if got := clientIP(req2); got != "198.51.100.7" {
		t.Errorf("应取 X-Real-IP, got %q", got)
	}

	// 公网 RemoteAddr (不 trust forwarded headers)
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.RemoteAddr = "8.8.8.8:5555"
	req3.Header.Set("X-Forwarded-For", "EVIL.0.0.1") // 应被忽略
	got := clientIP(req3)
	if got != "8.8.8.8" {
		t.Errorf("公网 RemoteAddr 应剥端口取 IP, 不取 Forwarded, got %q", got)
	}
}

// ============ Cookie helpers ============

func TestSetSessionCookieFields(t *testing.T) {
	rec := httptest.NewRecorder()
	expires := time.Now().Add(24 * time.Hour)
	setSessionCookie(rec, "tok-abc", expires, true)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("应 1 个 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != sessionCookieName {
		t.Errorf("cookie name=%q want %q", c.Name, sessionCookieName)
	}
	if c.Value != "tok-abc" {
		t.Errorf("value=%q want tok-abc", c.Value)
	}
	if !c.HttpOnly {
		t.Error("应 HttpOnly")
	}
	if !c.Secure {
		t.Error("传 secure=true 应保留")
	}
	if c.Path != "/" {
		t.Errorf("path=%q want /", c.Path)
	}
}

func TestClearSessionCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	clearSessionCookie(rec)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("应 1 个 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.Value != "" {
		t.Errorf("clear 后 value 应空, got %q", c.Value)
	}
	if c.MaxAge != -1 {
		t.Errorf("MaxAge 应 -1 (立即过期), got %d", c.MaxAge)
	}
}

// ============ Captcha (preVerifyCaptcha + verifyCaptcha) ============

func TestPreVerifyCaptchaNotFound(t *testing.T) {
	if preVerifyCaptcha("nonexistent-id-123", 100) {
		t.Error("不存在 id 应返 false")
	}
}

func TestPreVerifyCaptchaWrongAnswerDestroysEntry(t *testing.T) {
	// memory feedback_captcha_security: 失败即销毁防爆破
	captchaMu.Lock()
	captchaStore["test-wrong"] = captchaEntry{targetX: 100, expiresAt: time.Now().Add(time.Minute)}
	captchaMu.Unlock()

	if preVerifyCaptcha("test-wrong", 200) { // 偏差 100 > tolerance(5)
		t.Error("偏差大应返 false")
	}
	// 销毁验证: 再次试也失败 (entry 不存在)
	if preVerifyCaptcha("test-wrong", 100) {
		t.Error("失败后 entry 应已销毁, 再次 verify 应 false")
	}
}

func TestPreVerifyCaptchaCorrectAnswerMarksVerified(t *testing.T) {
	captchaMu.Lock()
	captchaStore["test-ok"] = captchaEntry{targetX: 150, expiresAt: time.Now().Add(time.Minute)}
	captchaMu.Unlock()

	if !preVerifyCaptcha("test-ok", 152) { // 偏差 2 ≤ tolerance(5)
		t.Error("偏差小应返 true")
	}
	// preVerify 不消耗, entry 应仍存在且 verified=true
	captchaMu.Lock()
	entry, ok := captchaStore["test-ok"]
	captchaMu.Unlock()
	if !ok {
		t.Fatal("preVerify 不应消耗 entry")
	}
	if !entry.verified {
		t.Error("preVerify 通过后 verified 应 true")
	}

	// cleanup
	captchaMu.Lock()
	delete(captchaStore, "test-ok")
	captchaMu.Unlock()
}

func TestPreVerifyCaptchaExpired(t *testing.T) {
	captchaMu.Lock()
	captchaStore["test-expired"] = captchaEntry{targetX: 100, expiresAt: time.Now().Add(-time.Second)}
	captchaMu.Unlock()

	if preVerifyCaptcha("test-expired", 100) {
		t.Error("过期 entry 应返 false")
	}
}

func TestVerifyCaptchaRequiresPreVerify(t *testing.T) {
	// 直接 verifyCaptcha 没经过 preVerify → false (verified=false)
	captchaMu.Lock()
	captchaStore["test-noprev"] = captchaEntry{targetX: 100, expiresAt: time.Now().Add(time.Minute)}
	captchaMu.Unlock()

	if verifyCaptcha("test-noprev", 100) {
		t.Error("没 preVerify 直接 verify 应 false (entry.verified=false)")
	}
}

func TestVerifyCaptchaConsumesEntry(t *testing.T) {
	// 先设置 verified=true 然后 verify, entry 应被消耗 (delete)
	captchaMu.Lock()
	captchaStore["test-consume"] = captchaEntry{targetX: 100, expiresAt: time.Now().Add(time.Minute), verified: true}
	captchaMu.Unlock()

	if !verifyCaptcha("test-consume", 100) {
		t.Error("verified entry + 正确 answer 应 true")
	}
	// 再次 verify 应找不到 entry
	if verifyCaptcha("test-consume", 100) {
		t.Error("verify 后 entry 应已销毁")
	}
}

// ============ Login Lock ============

func TestCheckLoginLockNoAttempt(t *testing.T) {
	// 干净 IP
	loginMu.Lock()
	delete(loginAttempts, "test-ip-clean")
	loginMu.Unlock()

	locked, _ := checkLoginLock("test-ip-clean")
	if locked {
		t.Error("无记录 IP 应不锁")
	}
}

func TestCheckLoginLockLocked(t *testing.T) {
	loginMu.Lock()
	loginAttempts["test-ip-locked"] = &loginAttempt{count: 5, lockedAt: time.Now()}
	loginMu.Unlock()
	defer func() {
		loginMu.Lock()
		delete(loginAttempts, "test-ip-locked")
		loginMu.Unlock()
	}()

	locked, remaining := checkLoginLock("test-ip-locked")
	if !locked {
		t.Error("刚锁定 IP 应 locked=true")
	}
	if remaining < 1 || remaining > 6 {
		t.Errorf("remaining 应 1-6 分钟之间, got %d", remaining)
	}
}

func TestCheckLoginLockExpiredAutoCleanup(t *testing.T) {
	loginMu.Lock()
	loginAttempts["test-ip-expired"] = &loginAttempt{count: 5, lockedAt: time.Now().Add(-10 * time.Minute)}
	loginMu.Unlock()

	locked, _ := checkLoginLock("test-ip-expired")
	if locked {
		t.Error("过期锁应自动清, locked 应 false")
	}
	// 验证已删除
	loginMu.Lock()
	_, exists := loginAttempts["test-ip-expired"]
	loginMu.Unlock()
	if exists {
		t.Error("过期锁应已 delete")
	}
}

func TestRecordLoginFailureAndClear(t *testing.T) {
	// 防 polluting 全局
	const testIP = "test-ip-record"
	loginMu.Lock()
	delete(loginAttempts, testIP)
	loginMu.Unlock()

	// recordLoginFailure 返回剩余次数 = maxLoginAttempts - count, 应递减
	rem1 := recordLoginFailure(testIP)
	rem2 := recordLoginFailure(testIP)
	rem3 := recordLoginFailure(testIP)
	if rem1 <= rem2 || rem2 <= rem3 {
		t.Errorf("剩余次数应递减, got %d/%d/%d", rem1, rem2, rem3)
	}
	if rem1 != rem2+1 || rem2 != rem3+1 {
		t.Errorf("剩余次数每次应 -1, got %d/%d/%d", rem1, rem2, rem3)
	}

	clearLoginFailure(testIP)
	loginMu.Lock()
	_, exists := loginAttempts[testIP]
	loginMu.Unlock()
	if exists {
		t.Error("clear 后 IP 记录应已删")
	}
}

// 测试 captchaMu 全局确实是 sync.Mutex (sanity check, 防止编译期改成别的类型)
func TestCaptchaMuIsMutex(t *testing.T) {
	var _ *sync.Mutex = &captchaMu
}

// ============ Handler 边界 ============

func TestGetCaptchaMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captcha", nil)
	(&DashboardHandler{DB: db}).GetCaptcha(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST 应 405, got %d", rec.Code)
	}
}

func TestGetCaptchaHappyPath(t *testing.T) {
	// GetCaptcha 实际生成 320x160 png + base64 编码, 慢但能跑
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/captcha", nil)
	(&DashboardHandler{DB: db}).GetCaptcha(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if resp["id"] == "" {
		t.Error("captcha id 应非空")
	}
	bg, _ := resp["bg"].(string)
	if !strings.HasPrefix(bg, "data:image/png;base64,") {
		t.Errorf("bg 应是 base64 png data URL, got %q (前 80 字符: %s)", bg[:min(80, len(bg))], bg[:min(80, len(bg))])
	}
}

func TestVerifyCaptchaOnlyMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/captcha/verify", nil)
	(&DashboardHandler{DB: db}).VerifyCaptchaOnly(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

func TestVerifyCaptchaOnlyBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captcha/verify", bytes.NewReader([]byte(`not json`)))
	(&DashboardHandler{DB: db}).VerifyCaptchaOnly(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json 应 400, got %d", rec.Code)
	}
}

func TestVerifyCaptchaOnlyVerifyFail(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"captchaId":"nonexistent","captchaAnswer":100}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captcha/verify", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).VerifyCaptchaOnly(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("不存在 captchaId 应 400, got %d", rec.Code)
	}
}

func TestVerifyCaptchaOnlyHappyPath(t *testing.T) {
	// 先注入一个 captcha entry, 再调 VerifyCaptchaOnly
	captchaMu.Lock()
	captchaStore["test-verify-ok"] = captchaEntry{targetX: 100, expiresAt: time.Now().Add(time.Minute)}
	captchaMu.Unlock()
	defer func() {
		captchaMu.Lock()
		delete(captchaStore, "test-verify-ok")
		captchaMu.Unlock()
	}()

	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"captchaId":"test-verify-ok","captchaAnswer":102}`) // 偏差 2 ≤ 5
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/captcha/verify", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).VerifyCaptchaOnly(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("正确 answer 应 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ Logout ============

func TestLogoutMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/logout", nil)
	(&DashboardHandler{DB: db}).Logout(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

func TestLogoutNoCookie(t *testing.T) {
	// 无 cookie 也应 200 (clearSessionCookie + writeJSON)
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	(&DashboardHandler{DB: db}).Logout(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("无 cookie 应 200 (登出无害), got %d", rec.Code)
	}
}

func TestLogoutWithCookieDeletesSession(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`DELETE FROM user_sessions WHERE token_hash = \?`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "any-token"})
	(&DashboardHandler{DB: db}).Logout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("应 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("DELETE 应被调用: %v", err)
	}
}

// ============ Me / ChangePassword 401 ============

func TestMeNoAuth(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	(&DashboardHandler{DB: db}).Me(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 auth 应 401, got %d", rec.Code)
	}
}

func TestChangePasswordNoAuth(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/change-password", bytes.NewReader([]byte(`{}`)))
	(&DashboardHandler{DB: db}).ChangePassword(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 auth 应 401, got %d", rec.Code)
	}
}

func TestChangePasswordMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/change-password", nil)
	(&DashboardHandler{DB: db}).ChangePassword(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

// ============ DingtalkAuthURL 边界 ============

func TestDingtalkAuthURLMissingClientID(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// DingClientID 空 → 400
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/dingtalk/url", nil)
	(&DashboardHandler{DB: db, DingClientID: ""}).DingtalkAuthURL(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("无 DingClientID 应 400, got %d", rec.Code)
	}
}
