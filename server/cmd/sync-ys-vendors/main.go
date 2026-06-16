// sync-ys-vendors 全量拉用友供应商档案 → ys_vendor_dict (名称→编码字典)。
// 供应商挂"企业账号级"共享, 无名称过滤接口, 故全量翻页落本地表, 采购订单工具按名称本地匹配。
// 幂等: code 为主键, REPLACE 覆盖。建议每天定时刷一次。
package main

import (
	"database/sql"
	"log"
	"strings"
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

	// 供应商企业级共享, 同一家按使用组织在多页重复出现(实测10万行→2千多家)。
	// 先内存去重(按编码), 再批量 upsert, 避免几十倍冗余的单条 DB 往返(二审#7)。
	uniq := map[string]string{} // code → name
	pageCount := 1
	for page := 1; page <= pageCount; page++ {
		vendors, pc, err := c.QueryVendorsPage(page, 500)
		if err != nil {
			log.Fatalf("query vendors page %d: %v", page, err)
		}
		// pageCount 兜底: 接口给了就用, 没给但本页满 500 就继续翻(防 pageCount=0 早停, 二审#1)
		if pc > 0 {
			pageCount = pc
		} else if len(vendors) >= 500 && page >= pageCount {
			pageCount = page + 1
		}
		for _, v := range vendors {
			if v.Code != "" {
				uniq[v.Code] = v.Name
			}
		}
		log.Printf("[sync-ys-vendors] page %d/%d, 去重后累计 %d 家", page, pageCount, len(uniq))
	}

	// 批量 upsert, 每批 500 行
	codes := make([]string, 0, len(uniq))
	for code := range uniq {
		codes = append(codes, code)
	}
	const batch = 500
	for start := 0; start < len(codes); start += batch {
		end := start + batch
		if end > len(codes) {
			end = len(codes)
		}
		var sb strings.Builder
		sb.WriteString("INSERT INTO ys_vendor_dict (code, name) VALUES ")
		args := make([]interface{}, 0, (end-start)*2)
		for i, code := range codes[start:end] {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString("(?,?)")
			args = append(args, code, uniq[code])
		}
		sb.WriteString(" ON DUPLICATE KEY UPDATE name=VALUES(name)")
		if _, err := db.Exec(sb.String(), args...); err != nil {
			log.Fatalf("batch upsert [%d:%d]: %v", start, end, err)
		}
	}

	log.Printf("[sync-ys-vendors] 完成: 共同步 %d 家供应商 @ %s", len(uniq), time.Now().Format("2006-01-02 15:04:05"))
}
