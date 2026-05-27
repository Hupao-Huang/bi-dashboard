// dry-run-fan-audit: 樊雪娇日常报销单 AI 建议 dry-run, 跑历史 archived 单跑统计
// 用途: codex 二审打补丁后, 验证 reject/manual/agree 数字变化
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"bi-dashboard/internal/handler"

	_ "github.com/go-sql-driver/mysql"
)

type Config struct {
	Database struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
		Dbname   string `json:"dbname"`
	} `json:"database"`
}

func main() {
	limit := flag.Int("limit", 200, "拉取多少单 (按 submit_date desc)")
	verbose := flag.Bool("v", false, "打印每单详情")
	flag.Parse()

	cfgBytes, err := os.ReadFile("config.json")
	if err != nil {
		fmt.Println("读 config.json 失败:", err)
		os.Exit(1)
	}
	var cfg Config
	if err := json.Unmarshal(cfgBytes, &cfg); err != nil {
		fmt.Println("解析 config 失败:", err)
		os.Exit(1)
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		cfg.Database.User, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port, cfg.Database.Dbname)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		fmt.Println("连 DB 失败:", err)
		os.Exit(1)
	}
	defer db.Close()

	h := &handler.DashboardHandler{DB: db}

	// 樊雪娇 approver_id = ID01FfMgoeP7cz:ID01Fp0nSy11h5
	// 拉她审批过的日常报销单 (archived + paid + rejected) — 她真实见过的单 + 此次实测样本
	rows, err := db.Query(`
		SELECT flow_id, code, IFNULL(title,''),
		       IFNULL(owner_department,''), IFNULL(department_id,''),
		       IFNULL(submitter_id,''),
		       IFNULL(expense_money,0), IFNULL(raw_json,''),
		       state
		FROM hesi_flow
		WHERE specification_id LIKE 'ID01Fk3qJYYFvp%'
		  AND current_approver_id LIKE '%ID01Fp0nSy11h5%'
		  AND state IN ('approving','paying','archived','paid','rejected')
		ORDER BY submit_date DESC, create_time DESC
		LIMIT ?`, *limit)
	if err != nil {
		fmt.Println("查待审批失败:", err)
		os.Exit(1)
	}
	defer rows.Close()

	type result struct {
		FlowID, Code, Title, State string
		ExpenseMoney               float64
		Suggestion                 *handler.AuditSuggestion
	}
	var all []result

	counts := map[string]int{"agree": 0, "manual": 0, "reject": 0}
	for rows.Next() {
		var r result
		var ownerDept, deptID, submitterID, rawJSON string
		if err := rows.Scan(&r.FlowID, &r.Code, &r.Title, &ownerDept, &deptID, &submitterID, &r.ExpenseMoney, &rawJSON, &r.State); err != nil {
			fmt.Println("scan 失败:", err)
			continue
		}
		sug := h.AuditDailyExpense(ownerDept, deptID, submitterID, r.FlowID, r.ExpenseMoney, rawJSON)
		r.Suggestion = sug
		all = append(all, r)
		counts[sug.Action]++
	}

	total := len(all)
	fmt.Printf("\n========== 樊雪娇 日常报销单 AI 建议 dry-run ==========\n")
	fmt.Printf("总单数: %d  agree=%d  manual=%d  reject=%d\n",
		total, counts["agree"], counts["manual"], counts["reject"])
	if total > 0 {
		fmt.Printf("通过率 agree=%.1f%%  人工=%.1f%%  驳回=%.1f%%\n",
			float64(counts["agree"])*100/float64(total),
			float64(counts["manual"])*100/float64(total),
			float64(counts["reject"])*100/float64(total))
	}

	// 规则触发频率统计 (按 reasons 关键字)
	ruleHit := map[string]int{}
	for _, r := range all {
		if r.Suggestion == nil {
			continue
		}
		seen := map[string]bool{}
		for _, msg := range r.Suggestion.Reasons {
			// 提取 "规则 X" 标签
			start := strings.Index(msg, "规则 ")
			if start < 0 {
				continue
			}
			end := strings.IndexAny(msg[start:], ")。;,")
			if end < 0 {
				end = len(msg) - start
			}
			tag := strings.TrimSpace(msg[start : start+end])
			if !seen[tag] {
				ruleHit[tag]++
				seen[tag] = true
			}
		}
	}
	if len(ruleHit) > 0 {
		fmt.Println("\n--- 各规则命中单数 (去重每单) ---")
		type kv struct {
			k string
			v int
		}
		kvs := make([]kv, 0, len(ruleHit))
		for k, v := range ruleHit {
			kvs = append(kvs, kv{k, v})
		}
		sort.Slice(kvs, func(i, j int) bool { return kvs[i].v > kvs[j].v })
		for _, p := range kvs {
			fmt.Printf("  %s: %d\n", p.k, p.v)
		}
	}

	if *verbose {
		fmt.Println("\n--- 单据详情 ---")
		for _, r := range all {
			if r.Suggestion == nil {
				continue
			}
			fmt.Printf("\n[%s] %s | %s | ¥%.2f | %s\n", r.State, r.Code, r.Title, r.ExpenseMoney, r.Suggestion.Action)
			for _, msg := range r.Suggestion.Reasons {
				fmt.Println("  •", msg)
			}
		}
	}
}
