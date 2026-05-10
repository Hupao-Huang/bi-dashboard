package handler

// auth_random_test.go — 随机生成函数 + hash + status 标准化
// 已 Read auth.go: generateCaptchaID(77-81), generateSessionToken(1561-1567),
// hashSessionToken(1569-1572), normalizeUserStatus(1205-1210)

import (
	"strings"
	"testing"
)

// generateCaptchaID — 16 字节随机 → hex (32 char)
func TestGenerateCaptchaIDLengthAndHex(t *testing.T) {
	id := generateCaptchaID()
	if len(id) != 32 { // 16 bytes hex = 32 chars
		t.Errorf("captcha ID 应为 32 char hex, got %d", len(id))
	}
	for _, r := range id {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Errorf("captcha ID 必须全 hex 小写, 含异常字符 %q", r)
		}
	}
}

func TestGenerateCaptchaIDUniqueness(t *testing.T) {
	// 防止退化到固定值: 1000 次生成应几乎全唯一
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id := generateCaptchaID()
		if _, dup := seen[id]; dup {
			t.Fatalf("第 %d 次生成 captcha ID 重复, 随机性异常: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}

// generateSessionToken — 32 字节随机 → hex (64 char)
func TestGenerateSessionTokenLengthAndUniqueness(t *testing.T) {
	tok, err := generateSessionToken()
	if err != nil {
		t.Fatalf("got err=%v", err)
	}
	if len(tok) != 64 { // 32 bytes hex = 64 chars
		t.Errorf("session token 应为 64 char, got %d", len(tok))
	}
	// 唯一性
	tok2, _ := generateSessionToken()
	if tok == tok2 {
		t.Error("两次 session token 应不同")
	}
}

// hashSessionToken — SHA-256(token) → 64 char hex
func TestHashSessionToken(t *testing.T) {
	hash := hashSessionToken("any-input")
	if len(hash) != 64 {
		t.Errorf("SHA-256 hex 应 64 char, got %d", len(hash))
	}
	// 同 input 必同 hash
	if hashSessionToken("X") != hashSessionToken("X") {
		t.Error("hash 必须确定")
	}
	// 不同 input 必不同 hash
	if hashSessionToken("X") == hashSessionToken("Y") {
		t.Error("不同 input hash 必不同")
	}
	// 全小写 hex
	if strings.ToLower(hash) != hash {
		t.Errorf("SHA-256 hex 应全小写, got %s", hash)
	}
}

// normalizeUserStatus — 'disabled' (case-insensitive) → 'disabled', 否则 'active'
func TestNormalizeUserStatus(t *testing.T) {
	cases := map[string]string{
		"disabled": "disabled",
		"DISABLED": "disabled",
		"Disabled": "disabled",
		"active":   "active",
		"":         "active", // fallback (源码: else return "active")
		"unknown":  "active",
		"banned":   "active",
	}
	for input, want := range cases {
		if got := normalizeUserStatus(input); got != want {
			t.Errorf("normalizeUserStatus(%q)=%q want %q", input, got, want)
		}
	}
}

// isBuiltInRole — 内置角色 code 检测
// 已 Read auth.go line 512-519: 6 个内置 role
func TestIsBuiltInRole(t *testing.T) {
	builtIn := []string{"super_admin", "management", "dept_manager", "operator", "finance", "supply_chain"}
	for _, code := range builtIn {
		if !isBuiltInRole(code) {
			t.Errorf("%q 是内置 role, 应返 true", code)
		}
	}
	// 自定义 role 不算
	notBuiltIn := []string{"", "custom_role_xyz", "admin", "guest"}
	for _, code := range notBuiltIn {
		if isBuiltInRole(code) {
			t.Errorf("%q 不是内置 role, 应返 false", code)
		}
	}
}
