// po-tool-vendors: 从 BI 数据库 ys_vendor_dict 导出"供应商名称→编码"名单成 vendors.json。
//
// 独立采购订单工具 po-tool 离网运行, 连不到数据库, 所以随 exe 带一份名单快照。
// 用友不支持按"名称"实时查供应商, 只能靠这份本地名单翻译。供应商有增减时重跑一次发新名单:
//
//	go run ./cmd/po-tool-vendors            (在 server/ 目录, 默认输出 vendors.json)
//
// 组织、物料是 po-tool 实时连用友查的, 不会过期; 只有供应商靠这份名单。
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"bi-dashboard/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	configPath := flag.String("config", "config.json", "BI config.json 路径")
	out := flag.String("out", "vendors.json", "输出供应商名单文件")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("读 config(%s): %v", *configPath, err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连数据库: %v", err)
	}
	defer db.Close()

	// ORDER BY code: 固定行序, 同名供应商每次都保留同一条编码(否则行序不定→不同次导出翻成不同编码→可能建错供应商的不可逆单)
	rows, err := db.Query("SELECT name, code FROM ys_vendor_dict WHERE name <> '' AND code <> '' ORDER BY code")
	if err != nil {
		log.Fatalf("查 ys_vendor_dict: %v", err)
	}
	defer rows.Close()

	m := make(map[string]string)
	conflicts := map[string][]string{} // 同名不同编码的供应商, 收集出来告警
	for rows.Next() {
		var name, code string
		if err := rows.Scan(&name, &code); err != nil {
			log.Fatalf("scan: %v", err)
		}
		if prev, ok := m[name]; ok {
			if prev != code {
				conflicts[name] = appendUniq(appendUniq(conflicts[name], prev), code)
			}
			continue // 已有该名, 保留先到的(ORDER BY code 下=编码最小那条, 确定性)
		}
		m[name] = code
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("遍历结果: %v", err)
	}
	if len(m) == 0 {
		log.Fatal("ys_vendor_dict 没查到供应商, 先跑 sync-ys-vendors 拉取")
	}

	data, err := json.Marshal(m) // 紧凑(10万家文件不小, 不缩进)
	if err != nil {
		log.Fatalf("序列化: %v", err)
	}
	if err := os.WriteFile(*out, data, 0644); err != nil {
		log.Fatalf("写 %s: %v", *out, err)
	}
	fmt.Printf("✅ 已导出 %d 家供应商 → %s (%.1f MB)\n", len(m), *out, float64(len(data))/1024/1024)
	if len(conflicts) > 0 {
		fmt.Printf("⚠️ 有 %d 个供应商名对应多个编码(已保留编码最小的一条, 翻译可能挑错主体):\n", len(conflicts))
		for name, codes := range conflicts {
			fmt.Printf("   「%s」→ %s (保留 %s)\n", name, strings.Join(codes, " / "), m[name])
		}
		fmt.Println("   若这些供应商会出现在采购模板里, 请在用友核对正确编码后手工处理, 不要直接依赖名单翻译。")
	}
}

// appendUniq 追加且去重(保持顺序)。
func appendUniq(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
