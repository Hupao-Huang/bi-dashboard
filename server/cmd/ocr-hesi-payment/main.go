// ocr-hesi-payment: 后台OCR跑批工具
// 从合思下载付款截图(名含付款/支付/转账/回单/账单), 识别金额并缓存到 hesi_payment_ocr 表。
// 用法:
//   ocr-hesi-payment.exe             # 生产模式: 扫所有待办截图
//   ocr-hesi-payment.exe -flow CODE  # 测试模式: 只处理指定单据号(如 B26003890)
//
// IMPORTANT: 合思/阿里云OSS均为国内域名, 所有http.Client必须 Proxy:nil, 否则xray代理环境下请求失败。
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/handler"
	"bi-dashboard/internal/ocr"
)

const (
	hesiAPIBase    = "https://app.ekuaibao.com"
	attachBatchMax = 50 // 每批最多50个flowId
)

// noProxyClient 所有国内API请求(合思+阿里云OSS)必须用此client, 不走xray代理
func noProxyClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{Proxy: nil},
	}
}

// getAccessToken 获取合思accessToken
func getAccessToken(appKey, secret string) (string, error) {
	body := map[string]string{
		"appKey":      appKey,
		"appSecurity": secret,
	}
	b, _ := json.Marshal(body)
	client := noProxyClient(30 * time.Second)
	resp, err := client.Post(hesiAPIBase+"/api/openapi/v1/auth/getAccessToken", "application/json", bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("请求授权失败: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var result struct {
		Value struct {
			AccessToken   string `json:"accessToken"`
			CorporationId string `json:"corporationId"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		snip := string(data)
		if len(snip) > 200 {
			snip = snip[:200]
		}
		return "", fmt.Errorf("解析授权失败: %w, body: %s", err, snip)
	}
	if result.Value.AccessToken == "" {
		snip := string(data)
		if len(snip) > 200 {
			snip = snip[:200]
		}
		return "", fmt.Errorf("accessToken为空, body: %s", snip)
	}
	log.Printf("获取授权成功, corporationId=%s", result.Value.CorporationId)
	return result.Value.AccessToken, nil
}

// attachInfo 从attachmentUrls提取的文件信息
type attachInfo struct {
	FileID   string
	FileName string
	URL      string
}

// getAttachmentURLs 批量查询合思附件下载链接, 返回 fileId -> attachInfo 的 map
// 合理批次大小: ≤50 flowId/call
func getAttachmentURLs(token string, flowIDs []string) (map[string]attachInfo, error) {
	result := make(map[string]attachInfo)
	client := noProxyClient(60 * time.Second)

	for i := 0; i < len(flowIDs); i += attachBatchMax {
		end := i + attachBatchMax
		if end > len(flowIDs) {
			end = len(flowIDs)
		}
		batch := flowIDs[i:end]

		body := map[string]interface{}{"flowIds": batch}
		b, _ := json.Marshal(body)
		req, err := http.NewRequest("POST",
			fmt.Sprintf("%s/api/openapi/v1/flowDetails/attachment?accessToken=%s", hesiAPIBase, token),
			bytes.NewReader(b))
		if err != nil {
			log.Printf("[ocr-hesi-payment] 构造附件请求失败 batch: %v", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[attachments] batch %d-%d HTTP失败: %v", i, end, err)
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var parsed struct {
			Items []struct {
				FlowID         string `json:"flowId"`
				AttachmentList []struct {
					AttachmentUrls []struct {
						FileID   string `json:"fileId"`
						FileName string `json:"fileName"`
						URL      string `json:"url"`
					} `json:"attachmentUrls"`
				} `json:"attachmentList"`
			} `json:"items"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			snip := string(data)
			if len(snip) > 200 {
				snip = snip[:200]
			}
			log.Printf("[attachments] batch %d-%d 解析失败: %v (HTTP %d, body: %s)", i, end, err, resp.StatusCode, snip)
			continue
		}

		for _, item := range parsed.Items {
			for _, attGroup := range item.AttachmentList {
				for _, u := range attGroup.AttachmentUrls {
					if u.FileID != "" && u.URL != "" {
						result[u.FileID] = attachInfo{
							FileID:   u.FileID,
							FileName: u.FileName,
							URL:      u.URL,
						}
					}
				}
			}
		}

		if len(flowIDs) > attachBatchMax {
			log.Printf("[attachments] 进度 %d/%d", end, len(flowIDs))
		}
		time.Sleep(100 * time.Millisecond)
	}
	return result, nil
}

// downloadImage 下载图片字节, 使用 Proxy:nil client
func downloadImage(url string) ([]byte, error) {
	client := noProxyClient(60 * time.Second)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("下载图片失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("下载图片HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取图片内容失败: %w", err)
	}
	return data, nil
}

// truncate 限制为最多 maxRunes 个字符(按rune, 不切断多字节中文; 配合 VARCHAR(500))
func truncate(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) > maxRunes {
		return string(r[:maxRunes])
	}
	return s
}

// paymentNameKeywords 付款截图文件名关键词 (与下方查询 SQL 的 LIKE 粗筛一致)。
var paymentNameKeywords = []string{"付款", "支付", "转账", "回单", "账单"}

// isPaymentScreenshotName 判定文件名是否为"应OCR的付款截图"(权威判定, 扫表后逐条过)。
// 命中任一付款关键词 且 不含"对账" 才算。排除"对账单": 它含"账单"会被关键词误命中,
// 但金额是对账总额而非单次实付, 拿去比发票会误判超付转人工 (潍坊中百对账单案例, 跑哥 2026-06-25)。
func isPaymentScreenshotName(name string) bool {
	if strings.Contains(name, "对账") {
		return false
	}
	for _, kw := range paymentNameKeywords {
		if strings.Contains(name, kw) {
			return true
		}
	}
	return false
}

// pendingRow 是待OCR的一行记录
type pendingRow struct {
	FlowID   string
	FileID   string
	FileName string
}

func main() {
	// 日志写 ocr-hesi-payment.log + stdout 双写
	logFile, err := os.OpenFile(`C:\Users\Administrator\bi-dashboard\server\ocr-hesi-payment.log`,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(io.MultiWriter(logFile, os.Stdout))
		defer logFile.Close()
	}

	// flags
	flowCode := flag.String("flow", "", "只处理指定单据号(如 B26003890), 留空=生产全扫")
	flag.Parse()

	log.Printf("========== ocr-hesi-payment 启动 (flow=%q) ==========", *flowCode)

	// 加载配置
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if cfg.DashScope.APIKey == "" {
		log.Fatalf("dashscope.api_key 未配置, 退出")
	}

	// 连接数据库
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(5)

	// 建表(幂等)
	if err := handler.EnsurePaymentOcrTable(db); err != nil {
		log.Fatalf("建表失败: %v", err)
	}

	// 查询待OCR的付款截图
	baseSQL := `SELECT a.flow_id, a.file_id, a.file_name
FROM hesi_flow_attachment a
JOIN hesi_flow f ON f.flow_id = a.flow_id
WHERE f.form_type='expense' AND f.state IN ('approving','pending')
  AND a.is_invoice=0
  AND (LOWER(a.file_name) LIKE '%.jpg' OR LOWER(a.file_name) LIKE '%.jpeg' OR LOWER(a.file_name) LIKE '%.png')
  AND (a.file_name LIKE '%付款%' OR a.file_name LIKE '%支付%' OR a.file_name LIKE '%转账%' OR a.file_name LIKE '%回单%' OR a.file_name LIKE '%账单%')
  AND a.file_name NOT LIKE '%对账%'
  AND a.file_id NOT IN (SELECT file_id FROM hesi_payment_ocr WHERE status='ok')`

	var rows *sql.Rows
	if *flowCode != "" {
		rows, err = db.Query(baseSQL+" AND f.code = ?", *flowCode)
	} else {
		rows, err = db.Query(baseSQL)
	}
	if err != nil {
		log.Fatalf("查询待办截图失败: %v", err)
	}

	var pending []pendingRow
	flowIDSet := make(map[string]bool)
	for rows.Next() {
		var r pendingRow
		if err := rows.Scan(&r.FlowID, &r.FileID, &r.FileName); err != nil {
			log.Printf("Scan失败: %v", err)
			continue
		}
		// 权威判定: 排除"对账单"等被关键词误命中的非付款截图 (SQL 已粗筛, 这里收口)
		if !isPaymentScreenshotName(r.FileName) {
			continue
		}
		pending = append(pending, r)
		flowIDSet[r.FlowID] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		log.Fatalf("查询行错误: %v", err)
	}

	if len(pending) == 0 {
		log.Println("无待OCR截图, 退出")
		return
	}
	log.Printf("待OCR截图: %d 张, 涉及 %d 个单据", len(pending), len(flowIDSet))

	// 收集唯一 flowIDs
	flowIDs := make([]string, 0, len(flowIDSet))
	for fid := range flowIDSet {
		flowIDs = append(flowIDs, fid)
	}

	// 获取合思 accessToken
	token, err := getAccessToken(cfg.Hesi.AppKey, cfg.Hesi.Secret)
	if err != nil {
		log.Fatalf("获取合思授权失败: %v", err)
	}

	// 批量拉附件签名URL, 建 fileId -> attachInfo map
	log.Printf("=== 拉取附件签名URL (%d个单据) ===", len(flowIDs))
	urlMap, err := getAttachmentURLs(token, flowIDs)
	if err != nil {
		// getAttachmentURLs内部批次失败只log, 此处err实际不返回, 但保留逻辑
		log.Printf("拉取附件URL部分失败: %v", err)
	}
	log.Printf("拿到签名URL: %d 条", len(urlMap))

	// 逐张OCR
	nOK, nFail, nSkip := 0, 0, 0
	ctx := context.Background()

	for i, row := range pending {
		att, ok := urlMap[row.FileID]
		if !ok {
			log.Printf("[%d/%d] file_id=%s 无签名URL, 跳过 (flow=%s, name=%s)",
				i+1, len(pending), row.FileID, row.FlowID, row.FileName)
			nSkip++
			continue
		}

		log.Printf("[%d/%d] OCR: flow=%s file=%s name=%s", i+1, len(pending), row.FlowID, row.FileID, row.FileName)

		// 下载图片
		imgBytes, err := downloadImage(att.URL)
		if err != nil {
			log.Printf("  下载失败: %v", err)
			if upsertErr := handler.UpsertPaymentOcr(db, row.FileID, row.FlowID, row.FileName, 0, "fail", truncate(err.Error(), 500)); upsertErr != nil {
				log.Printf("  upsert fail记录失败: %v", upsertErr)
			}
			nFail++
			time.Sleep(300 * time.Millisecond)
			continue
		}

		// OCR识别
		amount, raw, err := ocr.RecognizePaymentAmount(ctx, cfg.DashScope.APIKey, imgBytes)
		if err != nil {
			log.Printf("  OCR失败: %v", err)
			if upsertErr := handler.UpsertPaymentOcr(db, row.FileID, row.FlowID, row.FileName, 0, "fail", truncate(err.Error(), 500)); upsertErr != nil {
				log.Printf("  upsert fail记录失败: %v", upsertErr)
			}
			nFail++
			time.Sleep(300 * time.Millisecond)
			continue
		}

		log.Printf("  识别成功: amount=%.4f raw=%q", amount, truncate(raw, 80))
		if upsertErr := handler.UpsertPaymentOcr(db, row.FileID, row.FlowID, row.FileName, amount, "ok", truncate(raw, 500)); upsertErr != nil {
			log.Printf("  upsert ok记录失败: %v", upsertErr)
		}
		nOK++

		// 限速: OCR QPS保护
		time.Sleep(300 * time.Millisecond)
	}

	log.Printf("========== 跑批完成: 成功 %d / 失败 %d / 跳过(无URL) %d ==========", nOK, nFail, nSkip)
}
