package importutil

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

const lockDir = `C:\Users\Administrator\bi-dashboard\server`

// AcquireLock 获取文件锁，返回释放函数。如果锁已存在则退出程序。
func AcquireLock(name string) func() {
	lockFile := filepath.Join(lockDir, name+".lock")
	if _, err := os.Stat(lockFile); err == nil {
		log.Fatalf("任务 %s 正在运行中（锁文件 %s 已存在），跳过本次执行", name, lockFile)
	}
	os.WriteFile(lockFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
	return func() {
		os.Remove(lockFile)
	}
}
