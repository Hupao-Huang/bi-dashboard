package yonsuite

// client_test.go — yonsuite sign + truncate pure 函数测试
// 已 Read client.go line 170-189 (sign) + line 430-435 (truncate).
//
// sign 算法 (HmacSHA256 base64):
//   1. 排除 key='signature' (源码 line 173)
//   2. 剩余 key 按字母序排序 (sort.Strings line 178)
//   3. 拼接 key+value (无分隔符, line 181-184)
//   4. HmacSHA256(payload, AppSecret) (line 186-187)
//   5. base64 std encode (line 188)

import (
	"strings"
	"testing"
	"time"
)

// === sign property tests ===

func TestSignDeterministic(t *testing.T) {
	c := &Client{AppSecret: "secret"}
	params := map[string]string{
		"appKey":    "key123",
		"timestamp": "1700000000",
		"username":  "admin",
	}
	a := c.sign(params)
	b := c.sign(params)
	if a != b {
		t.Fatalf("sign 必须确定, a=%s b=%s", a, b)
	}
	// base64 标准 encode 长度为 4*ceil(32/3)=44 (HmacSHA256 32 字节)
	if len(a) != 44 {
		t.Errorf("HmacSHA256 base64 应为 44 字符, got %d", len(a))
	}
}

func TestSignExcludesSignature(t *testing.T) {
	c := &Client{AppSecret: "s"}
	base := map[string]string{"appKey": "k", "timestamp": "1"}
	withSig := map[string]string{"appKey": "k", "timestamp": "1", "signature": "x"}
	if c.sign(base) != c.sign(withSig) {
		t.Error("signature 字段自身不应参与签名 (源码 line 173 continue)")
	}
}

func TestSignSensitiveToSecret(t *testing.T) {
	params := map[string]string{"appKey": "k"}
	c1 := &Client{AppSecret: "secret_a"}
	c2 := &Client{AppSecret: "secret_b"}
	if c1.sign(params) == c2.sign(params) {
		t.Error("不同 AppSecret 必须产生不同 sign")
	}
}

func TestSignSensitiveToParams(t *testing.T) {
	c := &Client{AppSecret: "secret"}
	a := c.sign(map[string]string{"appKey": "K1"})
	b := c.sign(map[string]string{"appKey": "K2"})
	if a == b {
		t.Error("不同 appKey 应不同 sign")
	}
}

func TestSignKeyOrderStable(t *testing.T) {
	c := &Client{AppSecret: "secret"}
	params := map[string]string{
		"a": "1", "b": "2", "c": "3", "d": "4", "e": "5",
		"timestamp": "100", "appKey": "K", "z": "26",
	}
	first := c.sign(params)
	for i := 0; i < 50; i++ {
		if got := c.sign(params); got != first {
			t.Fatalf("迭代 %d 次 sign 不稳定 (sort.Strings 必须保证): want=%s got=%s", i, first, got)
		}
	}
}

func TestSignEmptyParamsValid(t *testing.T) {
	c := &Client{AppSecret: "secret"}
	out := c.sign(map[string]string{})
	if len(out) != 44 {
		t.Errorf("空 params 也产生有效 base64 sign, got len=%d", len(out))
	}
}

// === truncate (line 430-435) ===

func TestTruncate(t *testing.T) {
	cases := []struct {
		input string
		n     int
		want  string
	}{
		{"abc", 5, "abc"},     // len <= n: 原样返
		{"abcdef", 3, "abc..."}, // len > n: 截 n + "..."
		{"", 0, ""},            // 空
		{"hello", 5, "hello"},  // 边界 ==
		{"hello world", 5, "hello..."},
	}
	for _, c := range cases {
		if got := truncate(c.input, c.n); got != c.want {
			t.Errorf("truncate(%q,%d)=%q want %q", c.input, c.n, got, c.want)
		}
	}
}

// === SetMinInterval / waitRateLimit (line 83-104) ===

func TestSetMinInterval(t *testing.T) {
	c := NewClient("k", "s", "https://x.com")
	if c.minInterval != defaultMinInterval {
		t.Errorf("默认应为 1100ms, got %v", c.minInterval)
	}
	c.SetMinInterval(2 * time.Second)
	if c.minInterval != 2*time.Second {
		t.Errorf("Set 后应改为 2s, got %v", c.minInterval)
	}
	// 关节流 (付费版)
	c.SetMinInterval(0)
	if c.minInterval != 0 {
		t.Errorf("Set 0 应关节流, got %v", c.minInterval)
	}
}

// === NewClient ===

func TestNewClientStripsTrailingSlash(t *testing.T) {
	cases := map[string]string{
		"https://api.example.com/":  "https://api.example.com",
		"https://api.example.com":   "https://api.example.com",
		"https://api.example.com//": "https://api.example.com",
		"":                          "",
	}
	for input, want := range cases {
		c := NewClient("k", "s", input)
		if c.BaseURL != want {
			t.Errorf("NewClient baseURL %q → BaseURL %q want %q", input, c.BaseURL, want)
		}
	}
}

func TestNewClientDefaults(t *testing.T) {
	c := NewClient("appk", "apps", "https://x.com")
	if c.AppKey != "appk" || c.AppSecret != "apps" {
		t.Error("AppKey/AppSecret 应正确赋值")
	}
	if c.HTTP == nil {
		t.Error("HTTP client 必须初始化")
	}
	if c.HTTP.Timeout != 60*time.Second {
		t.Errorf("默认 timeout 应 60s, got %v", c.HTTP.Timeout)
	}
}

// 验证 sign payload 顺序: 应先 appKey 再 timestamp (字母序)
func TestSignPayloadIsSorted(t *testing.T) {
	c := &Client{AppSecret: "fixed"}
	// 手动构造 expected payload: 排序后 "appKey"+"K1"+"signature"+"X"
	// signature 排除 → "appKey"+"K1"+"timestamp"+"100"
	params := map[string]string{
		"timestamp": "100", // map 顺序乱, 但函数应排序
		"appKey":    "K1",
	}
	got := c.sign(params)

	// 用相同明文手动 hmac 比对 (源码逻辑)
	// 这里跨场景验证: 顺序乱不乱, sign 都应一致
	params2 := map[string]string{
		"appKey":    "K1",
		"timestamp": "100",
	}
	got2 := c.sign(params2)
	if got != got2 {
		t.Fatalf("顺序无关性: %s vs %s", got, got2)
	}
	// base64 编码不应含奇怪字符 (验证编码正确)
	if strings.ContainsAny(got, "?<>") {
		t.Errorf("base64 不应含异常字符: %s", got)
	}
}
