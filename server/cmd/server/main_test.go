package main

import (
	"net/http"
	"testing"
	"time"
)

// deadlineWriter 模拟底层连接: 实现 SetWriteDeadline (真 *http.response 也有)。
type deadlineWriter struct {
	http.ResponseWriter
	setCalled bool
}

func (d *deadlineWriter) SetWriteDeadline(t time.Time) error {
	d.setCalled = true
	return nil
}

type nopRW struct{}

func (nopRW) Header() http.Header        { return http.Header{} }
func (nopRW) Write(b []byte) (int, error) { return len(b), nil }
func (nopRW) WriteHeader(int)            {}

// 回归: statusRecorder 必须能让 http.NewResponseController 穿透拿到底层连接,
// 否则用友出库/转换接口清写超时静默失败 → 大批量仍被 120s 掐断 → 重复提交风险。
func TestStatusRecorder_UnwrapReachesWriteDeadline(t *testing.T) {
	base := &deadlineWriter{ResponseWriter: nopRW{}}
	rec := &statusRecorder{ResponseWriter: base, status: 200}

	if err := http.NewResponseController(rec).SetWriteDeadline(time.Time{}); err != nil {
		t.Fatalf("清写超时应穿透 statusRecorder 成功, 实际报错: %v (说明缺 Unwrap, 120s 掐断没被解除)", err)
	}
	if !base.setCalled {
		t.Fatal("SetWriteDeadline 没打到底层连接, Unwrap 链路断了")
	}
}
