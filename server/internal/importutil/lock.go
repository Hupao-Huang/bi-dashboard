//go:build windows

package importutil

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const lockDir = `C:\Users\Administrator\bi-dashboard\server`

// processAlive 检测指定 PID 是否仍在运行 (Windows 专用).
// 用 OpenProcess + GetExitCodeProcess: ExitCode 259 = STILL_ACTIVE 表示进程未退出.
func processAlive(pid int) bool {
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var exitCode uint32
	if err := syscall.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	return exitCode == 259
}

// AcquireLock 获取文件锁，返回释放函数。
// 锁文件内容是当前 PID. 如果锁存在但 PID 已死(孤儿锁), 自动清理后继续;
// 如果 PID 仍活着, log.Fatalf 退出避免并发跑同一个 import 工具.
func AcquireLock(name string) func() {
	lockFile := filepath.Join(lockDir, name+".lock")
	if data, err := os.ReadFile(lockFile); err == nil {
		pidStr := strings.TrimSpace(string(data))
		if pid, perr := strconv.Atoi(pidStr); perr == nil {
			if processAlive(pid) {
				log.Fatalf("任务 %s 正在运行中（锁文件 %s 已存在, PID %d 仍在运行），跳过本次执行", name, lockFile, pid)
			}
			log.Printf("[%s] 检测到孤儿锁 (PID %d 已退出), 自动清理后继续", name, pid)
			os.Remove(lockFile)
		} else {
			log.Printf("[%s] 锁文件内容异常 (%q), 当孤儿锁处理", name, pidStr)
			os.Remove(lockFile)
		}
	}
	os.WriteFile(lockFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
	return func() {
		os.Remove(lockFile)
	}
}
