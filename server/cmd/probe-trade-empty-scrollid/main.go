// probe-trade-empty-scrollid: 连续翻页拉 5月1日 09:00-09:59 销售单, 直到吉客云
// 返回空 scrollId, 完整打印每次的 request form + raw response body, 用作吉客云客服证据.
package main

import (
	"bi-dashboard/internal/config"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const tradeFields = "tradeNo,tradeStatus,tradeType,shopName,billDate,consignTime,goodsDetail.goodsNo,goodsDetail.subTradeId,customizeGoodsColumn3,customizeGoodsColumn4"

func main() {
	startTime := "2026-05-01 09:00:00"
	endTime := "2026-05-01 09:59:59"
	if len(os.Args) >= 3 {
		startTime = os.Args[1]
		endTime = os.Args[2]
	}

	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	appkey := cfg.JackYunTrade.AppKey
	secret := cfg.JackYunTrade.Secret
	apiURL := cfg.JackYun.APIURL

	httpClient := &http.Client{Timeout: 120 * time.Second}
	scrollId := ""

	fmt.Println("================================================================")
	fmt.Printf("吉客云销售单接口探针 (用于排查 scrollId 失效)\n")
	fmt.Printf("接口: jackyun.tradenotsensitiveinfos.list.get\n")
	fmt.Printf("AppKey: %s\n", appkey)
	fmt.Printf("时段: %s ~ %s\n", startTime, endTime)
	fmt.Printf("规则: 每次 pageSize=200, 用上次返回的 scrollId 翻页, 直到返回空\n")
	fmt.Println("================================================================")

	for page := 1; page <= 30; page++ {
		biz := map[string]interface{}{
			"startConsignTime": startTime,
			"endConsignTime":   endTime,
			"isDelete":         "0",
			"pageSize":         200,
			"scrollId":         scrollId,
			"fields":           tradeFields,
		}
		bizBytes, _ := json.Marshal(biz)

		timestamp := time.Now().Format("2006-01-02 15:04:05")
		params := map[string]string{
			"method":      "jackyun.tradenotsensitiveinfos.list.get",
			"appkey":      appkey,
			"version":     "v1.0",
			"contenttype": "json",
			"timestamp":   timestamp,
			"bizcontent":  string(bizBytes),
		}
		params["sign"] = sign(params, secret)

		form := url.Values{}
		for k, v := range params {
			form.Set(k, v)
		}

		fmt.Println()
		fmt.Printf("======== 第 %d 页 (timestamp=%s) ========\n", page, timestamp)
		fmt.Println("[REQUEST]")
		fmt.Printf("  POST %s\n", apiURL)
		fmt.Printf("  form.method      = %s\n", params["method"])
		fmt.Printf("  form.appkey      = %s\n", params["appkey"])
		fmt.Printf("  form.version     = %s\n", params["version"])
		fmt.Printf("  form.contenttype = %s\n", params["contenttype"])
		fmt.Printf("  form.timestamp   = %s\n", params["timestamp"])
		fmt.Printf("  form.sign        = %s...(脱敏)\n", params["sign"][:8])
		fmt.Printf("  form.bizcontent  = %s\n", params["bizcontent"])

		resp, err := httpClient.PostForm(apiURL, form)
		if err != nil {
			fmt.Printf("[ERROR] HTTP POST 失败: %v\n", err)
			break
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		fmt.Println("[RESPONSE]")
		fmt.Printf("  HTTP Status: %s\n", resp.Status)
		fmt.Printf("  Body length: %d bytes\n", len(body))
		fmt.Printf("  Body: %s\n", truncBody(string(body), 2000))

		var apiResp struct {
			Code    int             `json:"code"`
			Msg     string          `json:"msg"`
			SubCode string          `json:"subCode"`
			Result  json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(body, &apiResp); err != nil {
			fmt.Printf("[PARSE-ERROR] 顶层 JSON 解析失败: %v\n", err)
			break
		}
		fmt.Printf("  parsed.code=%d, msg=%q, subCode=%q\n", apiResp.Code, apiResp.Msg, apiResp.SubCode)

		var wrapper struct {
			Data     json.RawMessage `json:"data"`
			ScrollId string          `json:"scrollId"`
		}
		json.Unmarshal(apiResp.Result, &wrapper)

		var dataBytes []byte
		var dataStr string
		if err := json.Unmarshal(wrapper.Data, &dataStr); err == nil {
			dataBytes = []byte(dataStr)
		} else {
			dataBytes = wrapper.Data
		}
		var dataObj struct {
			TotalResults int               `json:"TotalResults"`
			Trades       []json.RawMessage `json:"Trades"`
			ScrollId     string            `json:"ScrollId"`
		}
		json.Unmarshal(dataBytes, &dataObj)

		newScrollId := wrapper.ScrollId
		if newScrollId == "" {
			newScrollId = dataObj.ScrollId
		}

		fmt.Printf("  本页 Trades 数: %d, 累计 ScrollId(wrapper)=%q, ScrollId(data)=%q\n",
			len(dataObj.Trades), wrapper.ScrollId, dataObj.ScrollId)

		if newScrollId == "" {
			fmt.Println()
			fmt.Println("================================================================")
			fmt.Printf("!!! 第 %d 页吉客云返回空 scrollId, 翻页中断 !!!\n", page)
			fmt.Printf("    (本页 Trades 数: %d)\n", len(dataObj.Trades))
			fmt.Println("================================================================")
			break
		}

		scrollId = newScrollId
		time.Sleep(1100 * time.Millisecond) // 限流
	}
}

func sign(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "sign" || k == "contextid" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf strings.Builder
	buf.WriteString(secret)
	for _, k := range keys {
		buf.WriteString(k)
		buf.WriteString(params[k])
	}
	buf.WriteString(secret)
	lower := strings.ToLower(buf.String())
	hash := md5.Sum([]byte(lower))
	return hex.EncodeToString(hash[:])
}

func truncBody(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("...(共 %d 字节, 已截断显示前 %d)", len(s), max)
}
