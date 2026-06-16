// sync-ys-vendors 全量拉用友供应商档案 → ys_vendor_dict (名称→编码字典)。
// 供应商挂"企业账号级"共享, 无名称过滤接口, 故全量翻页落本地表, 采购订单工具按名称本地匹配。
// 幂等: code 为主键, REPLACE 覆盖。建议每天定时刷一次。
package main

import (
	"database/sql"
	"log"
	"time"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/yonsuite"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS ys_vendor_dict (
			code VARCHAR(64) NOT NULL COMMENT '供应商编码',
			name VARCHAR(255) NOT NULL COMMENT '供应商名称',
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '同步时间',
			PRIMARY KEY (code),
			KEY idx_name (name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用友供应商档案字典(名称→编码), sync-ys-vendors全量同步'
	`); err != nil {
		log.Fatalf("create table: %v", err)
	}

	c := yonsuite.NewClient(cfg.YonSuite.AppKey, cfg.YonSuite.AppSecret, cfg.YonSuite.BaseURL)

	const pageSize = 1000
	total := 0
	pageCount := 1
	for page := 1; page <= pageCount; page++ {
		vendors, pc, err := c.QueryVendorsPage(page, pageSize)
		if err != nil {
			log.Fatalf("query vendors page %d: %v", page, err)
		}
		if pc > 0 {
			pageCount = pc
		}
		for _, v := range vendors {
			if v.Code == "" {
				continue
			}
			if _, err := db.Exec(
				"REPLACE INTO ys_vendor_dict (code, name) VALUES (?, ?)",
				v.Code, v.Name,
			); err != nil {
				log.Printf("upsert vendor %s: %v", v.Code, err)
				continue
			}
			total++
		}
		log.Printf("[sync-ys-vendors] page %d/%d done, 累计 %d", page, pageCount, total)
	}

	log.Printf("[sync-ys-vendors] 完成: 共同步 %d 个供应商 @ %s", total, time.Now().Format("2006-01-02 15:04:05"))
}
