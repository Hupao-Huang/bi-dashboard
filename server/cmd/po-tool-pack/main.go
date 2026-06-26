// po-tool-pack: 把 config.json 里的用友密钥用共享口令加密成 secret.dat。
//
// 独立采购订单工具 po-tool 携带这个 secret.dat 分发。没有口令, secret.dat 解不出密钥,
// 所以 exe + secret.dat 即使流出, 拿不到口令也调不了用友。
//
// 用法(在 server/ 目录下):
//
//	go run ./cmd/po-tool-pack -password <你的口令>
//	→ 生成 secret.dat (默认当前目录)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/potoolcrypto"
)

// poToolSecret 打进加密 blob 的用友连接信息(po-tool 解密后用它 NewClient)。
type poToolSecret struct {
	AppKey    string `json:"appkey"`
	AppSecret string `json:"appsecret"`
	BaseURL   string `json:"base_url"`
}

func main() {
	password := flag.String("password", "", "共享口令(留空则读环境变量 BI_POTOOL_PASSWORD, 更安全不留命令行历史)")
	configPath := flag.String("config", "config.json", "BI config.json 路径(读用友密钥)")
	out := flag.String("out", "secret.dat", "输出的加密密钥文件路径")
	flag.Parse()

	// 优先环境变量, 避免口令出现在命令行/PowerShell 历史/进程列表里
	pwd := *password
	if pwd == "" {
		pwd = os.Getenv("BI_POTOOL_PASSWORD")
	}
	if pwd == "" {
		log.Fatal("请用环境变量 BI_POTOOL_PASSWORD 提供口令(推荐), 或用 -password 参数")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("读 config(%s): %v", *configPath, err)
	}
	if cfg.YonSuite.AppKey == "" || cfg.YonSuite.AppSecret == "" {
		log.Fatal("config.json 里 yonsuite.appkey / appsecret 为空, 无法打包")
	}

	plain, err := json.Marshal(poToolSecret{
		AppKey:    cfg.YonSuite.AppKey,
		AppSecret: cfg.YonSuite.AppSecret,
		BaseURL:   cfg.YonSuite.BaseURL,
	})
	if err != nil {
		log.Fatalf("序列化密钥: %v", err)
	}
	blob, err := potoolcrypto.Encrypt(plain, pwd)
	if err != nil {
		log.Fatalf("加密: %v", err)
	}
	if err := os.WriteFile(*out, []byte(blob), 0600); err != nil {
		log.Fatalf("写 %s: %v", *out, err)
	}

	// 自检: 从磁盘回读刚写出的文件再解密, 端到端确认落盘没坏(部分写/磁盘满/编码异常都能查出)
	written, err := os.ReadFile(*out)
	if err != nil {
		log.Fatalf("自检回读 %s 失败: %v", *out, err)
	}
	if _, err := potoolcrypto.Decrypt(string(written), pwd); err != nil {
		log.Fatalf("自检解密失败(落盘文件无效, 请重新生成): %v", err)
	}

	fmt.Printf("✅ 已生成 %s (落盘回读自检通过)\n", *out)
	fmt.Printf("   用友密钥已用口令加密, 没口令解不出。目标环境 BaseURL=%s\n", cfg.YonSuite.BaseURL)
	fmt.Println("   下一步: 把 secret.dat 跟 po-tool.exe / vendors.json 一起打包发出去; 口令单独口头告知使用人。")
}
