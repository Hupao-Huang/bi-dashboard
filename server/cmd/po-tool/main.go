// po-tool: 独立"新增采购订单"工具(发给连不上内网的人用)。
//
// 双击 exe → 起一个本地网页 → 自动打开浏览器 → 输口令解锁 → 传 Excel → 看红绿预览 → 确认建单。
// 自带加密的用友密钥(secret.dat, 口令解密)+ 供应商名单(vendors.json)。
// 组织/物料实时连用友查, 供应商查本地名单。建单防重记本机文件 po-submit-log.json。
//
// 分发: po-tool.exe + secret.dat + vendors.json + 使用说明.txt 一起打包。口令单独口头告知。
package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed web/index.html
var indexHTML []byte

func main() {
	port := flag.Int("port", 0, "本地服务端口(默认 0 = 系统随机分配空闲端口; 调试时可固定)")
	flag.Parse()

	dir := dataDir()
	blob, err := os.ReadFile(filepath.Join(dir, "secret.dat"))
	if err != nil {
		fatal("找不到 secret.dat(加密的用友密钥), 它要跟 exe 放在同一个文件夹: %v", err)
	}
	vendors, err := loadVendors(filepath.Join(dir, "vendors.json"))
	if err != nil {
		fatal("读 vendors.json(供应商名单)失败, 它要跟 exe 放在同一个文件夹: %v", err)
	}
	idemp := newIdempStore(dir)

	// 默认只监听本机回环 + 系统分配的空闲端口(不怕端口被占); -port 可固定
	addr := "127.0.0.1:0"
	if *port > 0 {
		addr = fmt.Sprintf("127.0.0.1:%d", *port)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fatal("启动本地服务失败: %v", err)
	}
	// 允许的 Host: 我们实际监听的 127.0.0.1:PORT, 外加用户可能手敲的 localhost:PORT
	hostPort := ln.Addr().String()
	_, portStr, _ := net.SplitHostPort(hostPort)
	allowHosts := []string{hostPort, "localhost:" + portStr}

	a := newApp(string(blob), vendors, idemp, indexHTML, allowHosts)
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/api/unlock", a.guardAPI(a.handleUnlock))
	mux.HandleFunc("/api/preview", a.guardAPI(a.handlePreview))
	mux.HandleFunc("/api/commit", a.guardAPI(a.handleCommit))

	url := fmt.Sprintf("http://%s/", hostPort)

	fmt.Println("==================================================")
	fmt.Println("  新增采购订单工具  已启动")
	fmt.Println("  浏览器地址:", url)
	fmt.Println("  (浏览器会自动打开; 没弹出就手动复制上面这行地址)")
	fmt.Printf("  已加载供应商名单 %d 家\n", len(vendors))
	fmt.Println("  用完直接关掉这个黑窗口即退出。")
	fmt.Println("==================================================")

	openBrowser(url)
	if err := http.Serve(ln, mux); err != nil {
		fatal("服务异常退出: %v", err)
	}
}

// dataDir 数据文件(secret.dat/vendors.json/防重日志)所在目录。
// 分发形态: exe 所在目录; 开发 go run 时 exe 在临时目录, 回退当前工作目录。
func dataDir() string {
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(d, "secret.dat")); err == nil {
			return d
		}
	}
	wd, _ := os.Getwd()
	return wd
}

func loadVendors(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// openBrowser Windows 下用 rundll32 打开默认浏览器(不弹多余的命令行窗口)。
func openBrowser(url string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

// fatal 打印错误并等回车再退出, 避免黑窗口一闪就关、用户看不到原因。
func fatal(format string, args ...interface{}) {
	fmt.Printf("\n❌ "+format+"\n\n按回车键退出...", args...)
	_, _ = fmt.Scanln()
	os.Exit(1)
}
