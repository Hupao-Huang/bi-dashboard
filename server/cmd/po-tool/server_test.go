package main

import "testing"

// 守卫: 只认本机 Host / 同源 Origin, 挡 DNS rebinding 与跨站驱动。
func TestHostOriginGuard(t *testing.T) {
	a := newApp("", nil, nil, nil, []string{"127.0.0.1:18900", "localhost:18900"})

	if !a.hostAllowed("127.0.0.1:18900") {
		t.Error("本机 Host 应放行")
	}
	if !a.hostAllowed("localhost:18900") {
		t.Error("localhost Host 应放行")
	}
	if a.hostAllowed("evil.com") {
		t.Error("外部 Host 必须拒(DNS rebinding 防线)")
	}
	if a.hostAllowed("127.0.0.1:9999") {
		t.Error("非监听端口的 Host 应拒")
	}

	if !a.originAllowed("http://127.0.0.1:18900") {
		t.Error("同源 Origin 应放行")
	}
	if a.originAllowed("http://evil.com") {
		t.Error("跨站 Origin 必须拒")
	}
	if a.originAllowed("https://127.0.0.1.evil.com:18900") {
		t.Error("伪装前缀的 Origin 必须拒")
	}
}
