package dingtalk

// notify_test.go — Notifier 构造逻辑测试
// 已 Read notify.go line 47-61 (NewNotifier)

import "testing"

// NewNotifier 凭证空 → 返 nil; 否则正常构造
func TestNewNotifierReturnsNilWhenCredsEmpty(t *testing.T) {
	cases := []struct {
		appKey, appSecret string
		wantNil           bool
	}{
		{"", "", true},          // 都空
		{"", "secret", true},    // appKey 空
		{"key", "", true},       // appSecret 空
		{"key", "secret", false}, // 都非空 → 构造
	}
	for _, c := range cases {
		got := NewNotifier(c.appKey, c.appSecret, "")
		if c.wantNil && got != nil {
			t.Errorf("NewNotifier(%q,%q,%q) 应返 nil, got %v", c.appKey, c.appSecret, "", got)
		}
		if !c.wantNil && got == nil {
			t.Errorf("NewNotifier(%q,%q,%q) 应返实例, got nil", c.appKey, c.appSecret, "")
		}
	}
}

// 源码 line 51-53: robotCode 空 → fallback 用 appKey
func TestNewNotifierRobotCodeFallbackToAppKey(t *testing.T) {
	n := NewNotifier("myKey", "mySecret", "")
	if n == nil {
		t.Fatal("应返实例")
	}
	if n.robotCode != "myKey" {
		t.Errorf("robotCode 空时应 fallback 到 appKey, got %q", n.robotCode)
	}
}

func TestNewNotifierUsesProvidedRobotCode(t *testing.T) {
	n := NewNotifier("myKey", "mySecret", "myRobot")
	if n.robotCode != "myRobot" {
		t.Errorf("应用指定 robotCode, got %q", n.robotCode)
	}
}

// 内部 cache map / httpClient 必初始化
func TestNewNotifierInternalState(t *testing.T) {
	n := NewNotifier("k", "s", "r")
	if n.staffIDCache == nil {
		t.Error("staffIDCache map 必须初始化")
	}
	if n.httpClient == nil {
		t.Error("httpClient 必须初始化")
	}
	if n.httpClient.Timeout != httpTimeout {
		t.Errorf("Timeout 应 %v, got %v", httpTimeout, n.httpClient.Timeout)
	}
}
