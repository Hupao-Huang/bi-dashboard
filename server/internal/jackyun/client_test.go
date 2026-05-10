package jackyun

import "testing"

// 跑哥业务: 吉客云签名算法守护
// memory feedback_test_and_verify: 写 test 前必须 Read 源码理解所有分支, 不允许猜
//
// sign() 算法 (来自 client.go:91 源码逐行看过):
//   1. 排除 key='sign' 和 key='contextid'
//   2. 剩余 key 按字母序排序
//   3. 拼接: secret + key1 + value1 + key2 + value2 + ... + secret
//   4. 整体转小写
//   5. MD5 hex

// property 1: 同 input 必同 output (md5 确定性)
func TestSignIsDeterministic(t *testing.T) {
	c := &Client{Secret: "test_secret"}
	params := map[string]string{
		"method":      "test.method",
		"appkey":      "12345",
		"version":     "v1.0",
		"contenttype": "json",
		"timestamp":   "2026-05-10 12:00:00",
		"bizcontent":  `{"x":1}`,
	}
	a := c.sign(params)
	b := c.sign(params)
	if a != b {
		t.Fatalf("sign 必须确定: a=%s b=%s", a, b)
	}
	if len(a) != 32 {
		t.Fatalf("md5 hex 长度必须 32, got %d", len(a))
	}
}

// property 2: sign 和 contextid 自身参与不影响结果 (源码 line 94: continue)
func TestSignExcludesSignAndContextid(t *testing.T) {
	c := &Client{Secret: "secret"}
	base := map[string]string{
		"method": "x",
		"appkey": "1",
	}
	withSign := map[string]string{
		"method": "x",
		"appkey": "1",
		"sign":   "anything-here",
	}
	withContextid := map[string]string{
		"method":    "x",
		"appkey":    "1",
		"contextid": "ctx-123",
	}
	if c.sign(base) != c.sign(withSign) {
		t.Fatal("加 sign 字段不应影响计算结果 (sign 自身不参与签名)")
	}
	if c.sign(base) != c.sign(withContextid) {
		t.Fatal("加 contextid 字段不应影响计算结果")
	}
}

// property 3: secret 变 → sign 变 (敏感性)
func TestSignSensitiveToSecretChange(t *testing.T) {
	params := map[string]string{"method": "x", "appkey": "1"}
	c1 := &Client{Secret: "secret_a"}
	c2 := &Client{Secret: "secret_b"}
	if c1.sign(params) == c2.sign(params) {
		t.Fatal("不同 secret 必须产生不同 sign")
	}
}

// property 4: 任何普通参数变 → sign 变
func TestSignSensitiveToParamChange(t *testing.T) {
	c := &Client{Secret: "secret"}
	a := c.sign(map[string]string{"method": "x", "appkey": "1"})
	b := c.sign(map[string]string{"method": "y", "appkey": "1"})
	d := c.sign(map[string]string{"method": "x", "appkey": "2"})
	if a == b || a == d || b == d {
		t.Fatalf("不同 params 必须产生不同 sign: a=%s b=%s d=%s", a, b, d)
	}
}

// property 5: 同一组 params, 不论 map 迭代顺序如何, sign 一致 (源码 sort.Strings)
// Go map 迭代顺序随机, 已经隐式覆盖. 这里反复跑 100 次确保算法本身排序稳定.
func TestSignKeyOrderInsensitive(t *testing.T) {
	c := &Client{Secret: "secret"}
	params := map[string]string{
		"a1": "v1", "z9": "v9", "m5": "v5", "b2": "v2", "y8": "v8",
		"c3": "v3", "x7": "v7", "n6": "v6", "d4": "v4",
	}
	first := c.sign(params)
	for i := 0; i < 100; i++ {
		if got := c.sign(params); got != first {
			t.Fatalf("第 %d 次 sign 不一致 (排序应稳定): want=%s got=%s", i, first, got)
		}
	}
}

// 测试输出全小写 (源码 line 110: strings.ToLower)
func TestSignAllLowercase(t *testing.T) {
	c := &Client{Secret: "ABCDEF"}
	params := map[string]string{
		"Method": "TEST.UPPER",
		"AppKey": "ABC123",
	}
	out := c.sign(params)
	for _, r := range out {
		if r >= 'A' && r <= 'Z' {
			t.Fatalf("sign 输出必须全小写 (md5 hex), 含大写 %c: %s", r, out)
		}
	}
}
