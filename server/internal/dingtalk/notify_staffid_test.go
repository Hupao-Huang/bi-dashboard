package dingtalk

// notify_staffid_test.go — SendTextToStaffIDs 直发 staffId 路径 (合思下一审批人通知用)
// 关键差异: 不走 getbyunionid 转换 (桥接表存的就是企业 userid)

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSendTextToStaffIDsSkipsUnionIDResolve(t *testing.T) {
	n, mt := newTestNotifier(map[string]string{
		"/gettoken":              `{"errcode":0,"access_token":"tok","expires_in":7200}`,
		"/oToMessages/batchSend": `{"processQueryKey":"ok"}`,
		// 故意不 mock getbyunionid: 若走了转换会 404 报错
	})

	if err := n.SendTextToStaffIDs([]string{"123456"}, "下一环节通知"); err != nil {
		t.Fatalf("直发 staffId 应成功 (不应走 unionId 转换): %v", err)
	}
	// gettoken + batchSend = 2 次调用, 没有 getbyunionid
	if c := atomic.LoadInt32(&mt.calls); c != 2 {
		t.Errorf("HTTP 调用次数=%d want 2 (gettoken+batchSend)", c)
	}
}

func TestSendTextToStaffIDsEmptyFiltered(t *testing.T) {
	n, _ := newTestNotifier(nil)
	if err := n.SendTextToStaffIDs([]string{"", ""}, "msg"); err == nil {
		t.Error("全空 staffIds 应返 err")
	}
	if err := n.SendTextToStaffIDs([]string{"1"}, ""); err == nil {
		t.Error("空 content 应返 err")
	}
}

func TestSendTextToStaffIDsAsyncDoesNotBlock(t *testing.T) {
	n, _ := newTestNotifier(map[string]string{
		"/gettoken":              `{"errcode":0,"access_token":"tok","expires_in":7200}`,
		"/oToMessages/batchSend": `{"processQueryKey":"ok"}`,
	})
	start := time.Now()
	n.SendTextToStaffIDsAsync([]string{"123456"}, "async")
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("Async 不应阻塞, 实际 %v", elapsed)
	}
	time.Sleep(100 * time.Millisecond) // 等 goroutine 跑完防 leak 报警
}

// 防回归: SendText(unionId 路径) 仍要走转换
func TestSendTextStillResolvesUnionID(t *testing.T) {
	n, _ := newTestNotifier(map[string]string{
		"/gettoken":              `{"errcode":0,"access_token":"tok","expires_in":7200}`,
		"/getbyunionid":          `{"errcode":0,"result":{"userid":"staff-9"}}`,
		"/oToMessages/batchSend": `{"processQueryKey":"ok"}`,
	})
	if err := n.SendText([]string{"union-abc"}, "unionId 路径"); err != nil {
		t.Fatalf("unionId 路径应照常工作: %v", err)
	}
	if sid := n.staffIDCache["union-abc"]; !strings.Contains(sid, "staff-9") {
		t.Errorf("应缓存转换结果, got %q", sid)
	}
}
