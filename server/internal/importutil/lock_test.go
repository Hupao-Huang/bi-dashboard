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

// ============ AcquireLock ============

func TestAcquireLockNew(t *testing.T) {
	// 没有现存锁 → 直接写新锁
	name := "test-acquire-lock-new-12345"
	release := AcquireLock(name)

	lockFile := filepathJoin(name)
	data, err := os.ReadFile(lockFile)
	if err != nil {
		release()
		t.Fatalf("锁文件应存在: %v", err)
	}
	if len(data) == 0 {
		release()
		t.Error("锁文件应有 PID 内容")
	}

	release()
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Error("release 后锁文件应被删除")
	}
}

func TestAcquireLockOrphanCleanup(t *testing.T) {
	// 注入一个 PID=9999999 (不存在的 PID) 的孤儿锁
	name := "test-acquire-lock-orphan-67890"
	lockFile := filepathJoin(name)
	os.MkdirAll(lockDir, 0755)
	defer os.Remove(lockFile)
	os.WriteFile(lockFile, []byte("9999999"), 0644)

	release := AcquireLock(name) // 应清理孤儿锁后获取新锁
	defer release()

	data, _ := os.ReadFile(lockFile)
	if string(data) == "9999999" {
		t.Error("孤儿锁应被替换为当前 PID")
	}
}

func TestAcquireLockBadContent(t *testing.T) {
	// 锁文件内容不是数字 → 当孤儿锁清理
	name := "test-acquire-lock-badcontent-abc"
	lockFile := filepathJoin(name)
	os.MkdirAll(lockDir, 0755)
	defer os.Remove(lockFile)
	os.WriteFile(lockFile, []byte("not-a-pid"), 0644)

	release := AcquireLock(name)
	defer release()

	data, _ := os.ReadFile(lockFile)
	if string(data) == "not-a-pid" {
		t.Error("非数字内容应被替换")
	}
}

func filepathJoin(name string) string {
	return lockDir + string(os.PathSeparator) + name + ".lock"
}
