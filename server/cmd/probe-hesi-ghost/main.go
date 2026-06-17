// probe-hesi-ghost: 一次性探针 — 对照合思 getApplyList, 把我们库里的"待审"单分成三类:
//   ✅ 合思仍待审(真单)  🔄 合思已流转 paid/rejected(同步滞后)  👻 合思已无(删除/归档=真僵尸)
// 只读, 不改库。用于评估"第二批真实节点名但长期冻结"的单到底有多少是真僵尸 (跑哥 2026-06-17)。
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"bi-dashboard/internal/config"
)

const (
	hesiAPIBase = "https://app.ekuaibao.com"
	pageSize    = 100
)

var httpClient = &http.Client{Timeout: 60 * time.Second}

func getStr(m map[string]interface{}, k string) string {
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getToken(appKey, secret string) (string, error) {
	body, _ := json.Marshal(map[string]string{"appKey": appKey, "appSecurity": secret})
	resp, err := httpClient.Post(hesiAPIBase+"/api/openapi/v1/auth/getAccessToken", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var r struct {
		Value struct {
			AccessToken string `json:"accessToken"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return "", fmt.Errorf("token解析失败: %w, body=%.200s", err, string(data))
	}
	return r.Value.AccessToken, nil
}

func pageCall(token, formType, state string, start, count int) (int, []map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/openapi/v1.1/docs/getApplyList?type=%s&start=%d&count=%d&accessToken=%s&state=%s",
		hesiAPIBase, formType, start, count, token, state)
	resp, err := httpClient.Get(url)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var r struct {
		Count int                      `json:"count"`
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return 0, nil, fmt.Errorf("list解析失败: %w, body=%.200s", err, string(data))
	}
	return r.Count, r.Items, nil
}

// listIDs 拉某 type+state 的全部 flow_id (分页). ok=false 表示这趟有失败页(集合不完整, 调用方据此避免误判)
func listIDs(token, formType, state string, out map[string]bool) bool {
	total, items, err := pageCall(token, formType, state, 0, 1)
	if err != nil {
		fmt.Printf("  [警告] %s/%s 首页失败: %v\n", formType, state, err)
		return false
	}
	for _, it := range items {
		if id := getStr(it, "id"); id != "" {
			out[id] = true
		}
	}
	ok := true
	for start := 0; start < total; start += pageSize {
		_, items, err := pageCall(token, formType, state, start, pageSize)
		if err != nil {
			fmt.Printf("  [警告] %s/%s 第%d页失败: %v\n", formType, state, start/pageSize+1, err)
			ok = false
			continue
		}
		for _, it := range items {
			if id := getStr(it, "id"); id != "" {
				out[id] = true
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return ok
}

func main() {
	cfg, err := config.Load("config.json")
	if err != nil {
		panic(err)
	}
	token, err := getToken(cfg.Hesi.AppKey, cfg.Hesi.Secret)
	if err != nil {
		panic(err)
	}
	fmt.Println("合思授权 OK, 开始拉取实时单据列表...")

	types := []string{"expense", "loan", "requisition", "custom"}
	activeStates := []string{"approving", "paying", "pending", "PROCESSING"}
	termStates := []string{"paid", "rejected"}

	liveActive := map[string]bool{}
	activeComplete := true
	for _, t := range types {
		for _, s := range activeStates {
			if !listIDs(token, t, s, liveActive) {
				activeComplete = false
			}
		}
	}
	fmt.Printf("合思当前活跃单(approving/paying/pending/PROCESSING): %d 张  (完整=%v)\n", len(liveActive), activeComplete)

	liveTerm := map[string]bool{}
	for _, t := range types {
		for _, s := range termStates {
			listIDs(token, t, s, liveTerm)
		}
	}
	fmt.Printf("合思近期已流转(paid/rejected): %d 张\n", len(liveTerm))

	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		panic(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT flow_id, code, IFNULL(current_stage_name,''), state, gmt_sync
		FROM hesi_flow WHERE active=1 AND state IN ('approving','paying','pending','PROCESSING')`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	type doc struct{ flowID, code, node, state, sync string }
	var pending, moved, ghosts []doc
	for rows.Next() {
		var d doc
		if err := rows.Scan(&d.flowID, &d.code, &d.node, &d.state, &d.sync); err != nil {
			continue
		}
		switch {
		case liveActive[d.flowID]:
			pending = append(pending, d)
		case liveTerm[d.flowID]:
			moved = append(moved, d)
		default:
			ghosts = append(ghosts, d)
		}
	}

	fmt.Printf("\n========== 库里 %d 张待审单 对照合思 ==========\n", len(pending)+len(moved)+len(ghosts))
	fmt.Printf("✅ 合思仍待审(真单, 别动):      %d\n", len(pending))
	fmt.Printf("🔄 合思已流转 paid/rejected(同步滞后, 下次同步自愈): %d\n", len(moved))
	fmt.Printf("👻 合思列表已无(删除/归档=真僵尸): %d\n", len(ghosts))
	if !activeComplete {
		fmt.Println("⚠️  活跃单列表有失败页, 上面👻数偏多, 别据此删!")
	}

	sort.Slice(ghosts, func(i, j int) bool { return ghosts[i].sync < ghosts[j].sync })
	fmt.Println("\n--- 👻 真僵尸明细(按最后同步升序) ---")
	for _, d := range ghosts {
		fmt.Printf("  %-11s %-10s 节点=%-12s 最后同步=%s\n", d.code, d.state, d.node, d.sync)
	}
	sort.Slice(moved, func(i, j int) bool { return moved[i].sync < moved[j].sync })
	if len(moved) > 0 {
		fmt.Println("\n--- 🔄 已流转(同步滞后)明细 ---")
		for _, d := range moved {
			fmt.Printf("  %-11s %-10s 节点=%-12s 最后同步=%s\n", d.code, d.state, d.node, d.sync)
		}
	}
}
