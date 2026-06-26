package main

// apply-notice 一次性工具：执行一个 .sql 公告文件（取消旧置顶 + INSERT 新公告）。
// 复用项目 config 包读 server/config.json 的 DSN（不在命令行裸露密码）。
// 用法: go run ./cmd/apply-notice <path-to-sql>
// 注意: 按 ";" 切分语句逐条 Exec, 所以 .sql 内容里别用分号(公告正文用中文标点)。

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	"bi-dashboard/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("用法: go run ./cmd/apply-notice <path-to-sql>")
	}
	sqlPath := os.Args[1]

	raw, err := os.ReadFile(sqlPath)
	if err != nil {
		log.Fatalf("读 SQL 文件失败: %v", err)
	}

	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()

	for _, stmt := range strings.Split(string(raw), ";") {
		s := strings.TrimSpace(stmt)
		// 跳过空白和纯注释行
		if s == "" {
			continue
		}
		lines := []string{}
		for _, ln := range strings.Split(s, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(ln), "--") {
				lines = append(lines, ln)
			}
		}
		s = strings.TrimSpace(strings.Join(lines, "\n"))
		if s == "" {
			continue
		}
		res, err := db.Exec(s)
		if err != nil {
			log.Fatalf("执行失败:\n%s\n错误: %v", s, err)
		}
		n, _ := res.RowsAffected()
		preview := s
		if len(preview) > 60 {
			preview = preview[:60]
		}
		fmt.Printf("OK 影响 %d 行 <- %s...\n", n, strings.ReplaceAll(preview, "\n", " "))
	}
	fmt.Println("公告 SQL 执行完成")
}
