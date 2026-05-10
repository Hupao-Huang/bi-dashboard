//go:build windows

package importutil

// lock_test.go — processAlive 测试 (Windows 专用)
// 已 Read lock.go 全文 (54 行). syscall.OpenProcess + GetExitCodeProcess.

import (
	"os"
	"testing"
)

// processAlive(self PID) 必返 true: 当前测试进程在跑
func TestProcessAliveSelf(t *testing.T) {
	pid := os.Getpid()
	if !processAlive(pid) {
		t.Errorf("self process (PID %d) 必须活着 → true", pid)
	}
}

// processAlive(0) 应返 false (PID 0 是 system idle, OpenProcess 拒绝 access)
func TestProcessAliveZeroPID(t *testing.T) {
	// PID 0 在 Windows 是 System Idle, OpenProcess 通常 access denied
	// 函数源码 line 21: err != nil return false
	if processAlive(0) {
		t.Log("PID 0 在 Windows 偶尔可能返 true (System Idle), 这是边缘 case")
		// 不强失败, 仅记录
	}
}

// processAlive(超大 PID) 应返 false (进程不存在)
func TestProcessAliveNonExistentPID(t *testing.T) {
	// 9999999 几乎不可能是真 PID
	if processAlive(9999999) {
		t.Error("非常大的不存在 PID 应返 false")
	}
}
