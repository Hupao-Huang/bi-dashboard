package dingtalk

// notify_http_test.go — Notifier 4 个 method (getAccessToken / resolveStaffID / SendText / SendTextAsync)
// 已 Read notify.go 全文 (234 行) 含 3 const URL (gettoken / getbyunionid / batchSend).
// 用 mockTransport 拦截 httpClient 的 Round-trip, 按 URL.Path 路由响应.

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// mockTransport 按 URL.Path 路由 mock response
type mockTransport struct {
	handlers   map[string]string // path 子串 → response body
	statusCode int               // 0 默认 200
	calls      int32             // 调用次数计数 (用 atomic 防 race)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	atomic.AddInt32(&m.calls, 1)
	for path, body := range m.handlers {
		if strings.Contains(req.URL.Path, path) {
			sc := 200
			if m.statusCode > 0 {
				sc = m.statusCode
			}
			return &http.Response{
				StatusCode: sc,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}
	}
	return &http.Response{
		StatusCode: 404,
		Body:       io.NopCloser(strings.NewReader(`{"errcode":404,"errmsg":"no mock"}`)),
		Header:     make(http.Header),
	}, nil
}

func newTestNotifier(handlers map[string]string) (*Notifier, *mockTransport) {
	n := NewNotifier("appkey", "secret", "robot-code")
	mt := &mockTransport{handlers: handlers}
	n.httpClient = &http.Client{Transport: mt, Timeout: time.Second}
	return n, mt
}

// ---------- getAccessToken ----------

func TestGetAccessTokenHappyPath(t *testing.T) {
	n, _ := newTestNotifier(map[string]string{
		"/gettoken": `{"errcode":0,"access_token":"tok-123","expires_in":7200}`,
	})

	tok, err := n.getAccessToken()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if tok != "tok-123" {
		t.Errorf("token=%q want tok-123", tok)
	}
	// expiresAt = now + (7200-200)s = ~7000s in future
	if !n.expiresAt.After(time.Now().Add(time.Hour)) {
		t.Errorf("expiresAt 应在 1h 后, got %v", n.expiresAt)
	}
}

func TestGetAccessTokenErrCode(t *testing.T) {
	n, _ := newTestNotifier(map[string]string{
		"/gettoken": `{"errcode":40001,"errmsg":"invalid appkey"}`,
	})

	_, err := n.getAccessToken()
	if err == nil {
		t.Error("errcode != 0 应返 err")
	}
	if !strings.Contains(err.Error(), "40001") {
		t.Errorf("err 应含 errcode, got %v", err)
	}
}

func TestGetAccessTokenCacheHit(t *testing.T) {
	n, mt := newTestNotifier(map[string]string{
		"/gettoken": `{"errcode":0,"access_token":"cached","expires_in":7200}`,
	})

	t1, _ := n.getAccessToken()
	t2, _ := n.getAccessToken()
	if t1 != t2 || t1 != "cached" {
		t.Errorf("两次应一致 cached, got %q %q", t1, t2)
	}
	if mt.calls != 1 {
		t.Errorf("第二次应 cache hit, 实际 HTTP 调 %d 次", mt.calls)
	}
}

func TestGetAccessTokenMinExpiresFloor(t *testing.T) {
	// expires_in - 200 < 60 → 应 floor 到 60s (源码 line 149-151)
	n, _ := newTestNotifier(map[string]string{
		"/gettoken": `{"errcode":0,"access_token":"short","expires_in":100}`,
	})
	_, err := n.getAccessToken()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// 应至少 60s 后过期
	expected := time.Now().Add(60 * time.Second)
	if n.expiresAt.Before(expected.Add(-2 * time.Second)) {
		t.Errorf("expiresAt 应至少 60s 后, got delta=%v", time.Until(n.expiresAt))
	}
}

func TestGetAccessTokenInvalidJSON(t *testing.T) {
	n, _ := newTestNotifier(map[string]string{
		"/gettoken": `not json`,
	})
	_, err := n.getAccessToken()
	if err == nil {
		t.Error("无效 JSON 应返 err")
	}
}

// ---------- resolveStaffID ----------

func TestResolveStaffIDHappyPath(t *testing.T) {
	n, _ := newTestNotifier(map[string]string{
		"/gettoken":      `{"errcode":0,"access_token":"tok","expires_in":7200}`,
		"/getbyunionid":  `{"errcode":0,"result":{"userid":"staff-001"}}`,
	})

	sid, err := n.resolveStaffID("union-aaa")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if sid != "staff-001" {
		t.Errorf("staffID=%q want staff-001", sid)
	}
	// cache 命中
	if cached := n.staffIDCache["union-aaa"]; cached != "staff-001" {
		t.Errorf("cache 应有 union-aaa→staff-001, got %q", cached)
	}
}

func TestResolveStaffIDEmptyUnionID(t *testing.T) {
	n, _ := newTestNotifier(map[string]string{})

	_, err := n.resolveStaffID("")
	if err == nil {
		t.Error("空 unionID 应返 err")
	}
}

func TestResolveStaffIDErrCode(t *testing.T) {
	n, _ := newTestNotifier(map[string]string{
		"/gettoken":     `{"errcode":0,"access_token":"tok","expires_in":7200}`,
		"/getbyunionid": `{"errcode":60121,"errmsg":"用户不存在"}`,
	})

	_, err := n.resolveStaffID("union-bad")
	if err == nil {
		t.Error("errcode != 0 应返 err")
	}
}

func TestResolveStaffIDEmptyUserID(t *testing.T) {
	n, _ := newTestNotifier(map[string]string{
		"/gettoken":     `{"errcode":0,"access_token":"tok","expires_in":7200}`,
		"/getbyunionid": `{"errcode":0,"result":{"userid":""}}`,
	})

	_, err := n.resolveStaffID("union-empty")
	if err == nil {
		t.Error("空 userid 应返 err")
	}
}

func TestResolveStaffIDCacheHit(t *testing.T) {
	n, mt := newTestNotifier(map[string]string{
		"/gettoken":     `{"errcode":0,"access_token":"tok","expires_in":7200}`,
		"/getbyunionid": `{"errcode":0,"result":{"userid":"staff-x"}}`,
	})

	_, _ = n.resolveStaffID("uid-1") // 第 1 次 = 2 个 HTTP 调用 (token+getbyunionid)
	firstCalls := atomic.LoadInt32(&mt.calls)
	_, _ = n.resolveStaffID("uid-1") // 第 2 次 cache 命中 = 0 HTTP
	secondCalls := atomic.LoadInt32(&mt.calls)
	if secondCalls != firstCalls {
		t.Errorf("staffID cache 应命中, 第 2 次不该多 HTTP. firstCalls=%d secondCalls=%d", firstCalls, secondCalls)
	}
}

// ---------- SendText ----------

func TestSendTextHappyPath(t *testing.T) {
	n, _ := newTestNotifier(map[string]string{
		"/gettoken":      `{"errcode":0,"access_token":"tok","expires_in":7200}`,
		"/getbyunionid":  `{"errcode":0,"result":{"userid":"staff-001"}}`,
		"/oToMessages/batchSend": `{"processQueryKey":"abc","invalidStaffIdList":[],"flowControlledStaffIdList":[]}`,
	})

	err := n.SendText([]string{"union-aaa"}, "Hello 跑哥")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestSendTextEmptyUnionIDs(t *testing.T) {
	n, _ := newTestNotifier(map[string]string{})

	err := n.SendText(nil, "anything")
	if err == nil {
		t.Error("空 unionIDs 应返 err")
	}
	err = n.SendText([]string{}, "anything")
	if err == nil {
		t.Error("空 slice 应返 err")
	}
}

func TestSendTextEmptyContent(t *testing.T) {
	n, _ := newTestNotifier(map[string]string{})

	err := n.SendText([]string{"u1"}, "")
	if err == nil {
		t.Error("空 content 应返 err")
	}
}

func TestSendTextAllResolveFail(t *testing.T) {
	// 所有 unionID resolve 都失败 → "无有效 staffId"
	n, _ := newTestNotifier(map[string]string{
		"/gettoken":     `{"errcode":0,"access_token":"tok","expires_in":7200}`,
		"/getbyunionid": `{"errcode":60121,"errmsg":"不存在"}`,
	})

	err := n.SendText([]string{"u1", "u2"}, "msg")
	if err == nil {
		t.Error("所有 resolve 失败应返 err")
	}
	if !strings.Contains(err.Error(), "无有效") {
		t.Errorf("err 应含'无有效', got %v", err)
	}
}

func TestSendTextSendMessageStatusNon200(t *testing.T) {
	n, mt := newTestNotifier(map[string]string{
		"/gettoken":              `{"errcode":0,"access_token":"tok","expires_in":7200}`,
		"/getbyunionid":          `{"errcode":0,"result":{"userid":"staff-001"}}`,
		"/oToMessages/batchSend": `{"errcode":403,"errmsg":"forbidden"}`,
	})
	mt.statusCode = 403

	err := n.SendText([]string{"union-aaa"}, "msg")
	if err == nil {
		t.Error("status != 200 应返 err")
	}
}

// ---------- SendTextAsync ----------

func TestSendTextAsyncDoesNotBlock(t *testing.T) {
	// 异步发送, 即使后端慢/失败也不阻塞调用者
	// 我们调用即返回, 然后 sleep 短时间让 goroutine 跑完
	n, _ := newTestNotifier(map[string]string{
		"/gettoken":              `{"errcode":0,"access_token":"tok","expires_in":7200}`,
		"/getbyunionid":          `{"errcode":0,"result":{"userid":"s1"}}`,
		"/oToMessages/batchSend": `{"processQueryKey":"x"}`,
	})

	start := time.Now()
	n.SendTextAsync([]string{"u1"}, "async msg")
	elapsed := time.Since(start)
	// SendTextAsync 应 ~immediately 返回 (远小于 timeout)
	if elapsed > 50*time.Millisecond {
		t.Errorf("SendTextAsync 不应阻塞, 实际 %v", elapsed)
	}
}
