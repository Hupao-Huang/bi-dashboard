package yingdao

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newStubServer 起一个假影刀服务: token 接口固定返回 token, 其余路径交给 handler
func newStubServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/oapi/token/") {
			_, _ = io.WriteString(w, `{"code":200,"success":true,"data":{"accessToken":"tok","expiresIn":7200}}`)
			return
		}
		handler(w, r)
	}))
}

// TestStopJob 成功场景: 路径正确 + 请求体带 jobUuid + 返回 nil
func TestStopJob(t *testing.T) {
	var gotPath, gotBody string
	srv := newStubServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = io.WriteString(w, `{"code":200,"success":true}`)
	})
	defer srv.Close()

	c := NewClient("ak", "sk", srv.URL, srv.URL, "acct@x")
	if err := c.StopJob(context.Background(), "job-123"); err != nil {
		t.Fatalf("StopJob 成功场景应返回 nil, 实际: %v", err)
	}
	if gotPath != "/oapi/dispatch/v2/job/stop" {
		t.Errorf("接口路径应为 /oapi/dispatch/v2/job/stop, 实际 %q", gotPath)
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(gotBody), &parsed); err != nil {
		t.Fatalf("请求体不是合法 JSON: %v (%s)", err, gotBody)
	}
	if parsed["jobUuid"] != "job-123" {
		t.Errorf("请求体应带 jobUuid=job-123, 实际 %q", gotBody)
	}
}

// TestStopJobError 影刀返回非 200 时应报错
func TestStopJobError(t *testing.T) {
	srv := newStubServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"code":50000,"success":false,"msg":"任务不存在"}`)
	})
	defer srv.Close()

	c := NewClient("ak", "sk", srv.URL, srv.URL, "acct@x")
	if err := c.StopJob(context.Background(), "bad"); err == nil {
		t.Fatal("StopJob 失败场景应返回 error, 实际 nil")
	}
}
