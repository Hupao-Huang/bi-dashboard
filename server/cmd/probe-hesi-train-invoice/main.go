// 一次性探针: 拉火车票发票主体的全部字段, 看合思有没有"席别/座位等级"
// 用法: cd server && go run ./cmd/probe-hesi-train-invoice
package main

import (
	"bi-dashboard/internal/config"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const apiBase = "https://app.ekuaibao.com"

// 这单(B26003394 朱磊5月差旅费)的 8 张火车票 invoice_id
var trainInvoiceIDs = []string{
	"ID01FfMgoeP7cz::26219151977000141040",
	"ID01FfMgoeP7cz::26219153876001499830",
	"ID01FfMgoeP7cz::26219153885001326292",
	"ID01FfMgoeP7cz::26219153885001326293",
	"ID01FfMgoeP7cz::26219153885001326294",
	"ID01FfMgoeP7cz::26229154116000902312",
	"ID01FfMgoeP7cz::26229154116000902313",
	"ID01FfMgoeP7cz::26239153469000642726",
}

func main() {
	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		fmt.Println("加载配置失败:", err)
		return
	}
	client := &http.Client{Timeout: 30 * time.Second}

	// 1. 授权
	authBody, _ := json.Marshal(map[string]string{"appKey": cfg.Hesi.AppKey, "appSecurity": cfg.Hesi.Secret})
	resp, err := client.Post(apiBase+"/api/openapi/v1/auth/getAccessToken", "application/json", bytes.NewReader(authBody))
	if err != nil {
		fmt.Println("授权请求失败:", err)
		return
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var auth struct {
		Value struct {
			AccessToken string `json:"accessToken"`
		} `json:"value"`
	}
	json.Unmarshal(data, &auth)
	if auth.Value.AccessToken == "" {
		fmt.Println("拿不到 token:", string(data[:min(len(data), 200)]))
		return
	}
	token := auth.Value.AccessToken

	// 2. 挨个发票类型试, 找到能返回这些 ID 的 objectId
	invoiceTypes := []string{"invoice", "train", "noTaxIncome", "taxi", "fixed", "flightItinerary", "tolls", "machinePrint", "other", "passengerCar", "shopping", "medical", "overseasInvoice"}
	var result struct {
		Count int                      `json:"count"`
		Items []map[string]interface{} `json:"items"`
	}
	hitType := ""
	for _, t := range invoiceTypes {
		body, _ := json.Marshal(map[string]interface{}{"ids": trainInvoiceIDs})
		req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/openapi/v2/extension/INVOICE/object/%s/search?accessToken=%s", apiBase, t, token), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r, err := client.Do(req)
		if err != nil {
			continue
		}
		d, _ := io.ReadAll(r.Body)
		r.Body.Close()
		var tmp struct {
			Count int                      `json:"count"`
			Items []map[string]interface{} `json:"items"`
		}
		json.Unmarshal(d, &tmp)
		fmt.Printf("类型 %-16s 返回 %d 张\n", t, len(tmp.Items))
		if len(tmp.Items) > 0 && hitType == "" {
			result = tmp
			hitType = t
		}
		time.Sleep(300 * time.Millisecond)
	}
	fmt.Printf("\n命中类型: %s, 拉到 %d 张\n\n", hitType, len(result.Items))
	if len(result.Items) == 0 {
		fmt.Println("所有类型都没返回, 可能 invoice_id 格式或权限问题")
		return
	}

	// 3. 逐张列出关键出行信息 (车次/座位类型/席位/区间/乘车人/金额)
	fmt.Println("===== 8 张火车票的座位等级(座位类型) =====")
	for i, inv := range result.Items {
		fmt.Printf("%d. 票号 %s | %s次 %s→%s | 座位类型=%s | 席位=%s | 车厢=%s | 乘车人=%s | 金额%s\n",
			i+1,
			pickSuffix(inv, "_发票号码"),
			pickSuffix(inv, "_车次"),
			pickSuffix(inv, "_上车车站"),
			pickSuffix(inv, "_下车车站"),
			pickSuffix(inv, "_座位类型"),
			pickSuffix(inv, "_席位"),
			pickSuffix(inv, "_车厢"),
			pickSuffix(inv, "_乘车人姓名"),
			pickMoney(inv, "_价税合计"),
		)
	}
	_ = sort.Strings
}

func pickMoney(m map[string]interface{}, suffix string) string {
	for k, v := range m {
		if strings.HasSuffix(k, suffix) {
			if mm, ok := v.(map[string]interface{}); ok {
				if s, ok := mm["standard"].(string); ok {
					return "¥" + s
				}
			}
		}
	}
	return "?"
}

func pickSuffix(m map[string]interface{}, suffix string) string {
	for k, v := range m {
		if strings.HasSuffix(k, suffix) {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return "?"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
