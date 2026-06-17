// dry-run-payment-audit: 张俊付款单/预付款单 AI 审批建议 dry-run 回测探针
// 用途: 对当前待张俊审批的付款单/预付款单跑 AuditPayment 引擎，统计三档动作分布及规则命中频次
// 只读库，不写库，不重启服务
package main

import (
	"database/sql"
	"encoding/json"
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
	Hesi struct {
		AppKey string `json:"appkey"`
		Secret string `json:"secret"`
	} `json:"hesi"`
}

func main() {
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

	// 实例化 DashboardHandler (照 dry-run-fan-audit 套路)
	h := &handler.DashboardHandler{DB: db, HesiAppKey: cfg.Hesi.AppKey, HesiSecret: cfg.Hesi.Secret}

	// 查当前待张俊审批的付款单+预付款单 (active=1)
	rows, err := db.Query(`
		SELECT flow_id, IFNULL(specification_id,''), IFNULL(raw_json,''), IFNULL(submit_date,0)
		FROM hesi_flow
		WHERE current_approver_name='张俊'
		  AND active=1
		  AND (specification_id LIKE 'ID01KgaO6dcZtR%' OR specification_id LIKE 'ID01FhdI9II9A3%')
		ORDER BY submit_date DESC
	`)
	if err != nil {
		fmt.Println("查单据失败:", err)
		os.Exit(1)
	}
	defer rows.Close()

	type record struct {
		FlowID     string
		SpecID     string
		RawJSON    string
		SubmitDate int64
		Tmpl       string // "付款单" or "预付款单"
		Action     string
		FirstReason string
	}

	var all []record
	counts := map[string]int{"agree": 0, "manual": 0, "reject": 0}
	tmplCounts := map[string]int{"付款单": 0, "预付款单": 0}

	for rows.Next() {
		var r record
		if err := rows.Scan(&r.FlowID, &r.SpecID, &r.RawJSON, &r.SubmitDate); err != nil {
			fmt.Println("scan 失败:", err)
			continue
		}
		// 判模板类型
		if strings.HasPrefix(r.SpecID, "ID01KgaO6dcZtR") {
			r.Tmpl = "付款单"
		} else {
			r.Tmpl = "预付款单"
		}
		tmplCounts[r.Tmpl]++

		sug := h.AuditPayment(r.FlowID, r.SpecID, r.RawJSON, r.SubmitDate)
		r.Action = sug.Action
		if len(sug.Reasons) > 0 {
			r.FirstReason = sug.Reasons[0]
		}
		counts[sug.Action]++
		all = append(all, r)
	}

	total := len(all)

	// ① 总数 + 按模板分组
	fmt.Printf("\n========== 张俊 付款单/预付款单 AI 审批建议 dry-run ==========\n")
	fmt.Printf("总单数: %d  (付款单: %d, 预付款单: %d)\n", total, tmplCounts["付款单"], tmplCounts["预付款单"])

	// ② 三档动作计数 + 占比
	fmt.Printf("\n--- 三档动作计数 ---\n")
	fmt.Printf("agree  (建议通过): %d", counts["agree"])
	if total > 0 {
		fmt.Printf("  (%.1f%%)", float64(counts["agree"])*100/float64(total))
	}
	fmt.Println()
	fmt.Printf("manual (转人工核): %d", counts["manual"])
	if total > 0 {
		fmt.Printf("  (%.1f%%)", float64(counts["manual"])*100/float64(total))
	}
	fmt.Println()
	fmt.Printf("reject (建议驳回): %d", counts["reject"])
	if total > 0 {
		fmt.Printf("  (%.1f%%)", float64(counts["reject"])*100/float64(total))
	}
	fmt.Println()

	// ③ 每单一行: flow_id | 模板 | action | 第一条理由
	fmt.Printf("\n--- 每单明细 ---\n")
	for _, r := range all {
		reason := r.FirstReason
		if len(reason) > 80 {
			reason = reason[:80] + "…"
		}
		fmt.Printf("%-32s | %-6s | %-6s | %s\n", r.FlowID, r.Tmpl, r.Action, reason)
	}

	// ④ 规则命中频次汇总 (哪条规则命中最多, 降序)
	ruleHit := map[string]int{}
	for _, r := range all {
		// 重新调一次引擎拿完整 reasons, 用于统计规则
		sug := h.AuditPayment(r.FlowID, r.SpecID, r.RawJSON, r.SubmitDate)
		if sug == nil {
			continue
		}
		seen := map[string]bool{}
		for _, msg := range sug.Reasons {
			// 提取括号内规则标签 如 (A1) (B1/B2) 等
			start := strings.LastIndex(msg, "(")
			if start < 0 {
				continue
			}
			end := strings.LastIndex(msg, ")")
			if end < 0 || end <= start {
				continue
			}
			tag := strings.TrimSpace(msg[start+1 : end])
			// 过滤掉非规则标签 (如括号包裹的数字金额等)
			if tag == "" || tag == "0" {
				continue
			}
			// 只统计以字母开头的规则标签 A1/B1/B2 等
			if len(tag) >= 2 && (tag[0] == 'A' || tag[0] == 'B') {
				if !seen[tag] {
					ruleHit[tag]++
					seen[tag] = true
				}
			}
		}
	}

	if len(ruleHit) > 0 {
		fmt.Printf("\n--- 规则命中频次 (按单去重, 降序) ---\n")
		type kv struct {
			k string
			v int
		}
		kvs := make([]kv, 0, len(ruleHit))
		for k, v := range ruleHit {
			kvs = append(kvs, kv{k, v})
		}
		sort.Slice(kvs, func(i, j int) bool {
			if kvs[i].v != kvs[j].v {
				return kvs[i].v > kvs[j].v
			}
			return kvs[i].k < kvs[j].k
		})
		for _, p := range kvs {
			fmt.Printf("  %s: %d 单\n", p.k, p.v)
		}
	} else {
		fmt.Println("\n无规则命中统计 (全部通过或无数据)")
	}

	fmt.Printf("\n========== 回测完毕 ==========\n")
}
