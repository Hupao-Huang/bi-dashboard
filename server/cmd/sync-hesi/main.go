// sync-hesi: 同步合思费控单据数据到MySQL
// 用法: sync-hesi [--type expense|loan|requisition|custom] [--state paid|approving|...]
// 不传参数则拉取全部类型全部状态
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"bi-dashboard/internal/config"
)

const (
	hesiAPIBase  = "https://app.ekuaibao.com"
	hesiAppKey   = "4b7bb534-d6be-4dde-953f-6cf7f9077272"
	hesiSecret   = "efdb8e9c-b93e-47ca-af75-cdaf289336d4"
	pageSize     = 100
	attachBatch  = 50 // 附件接口每次最多100个，保守用50
)

var httpClient = &http.Client{Timeout: 60 * time.Second}

// 获取accessToken
func getAccessToken() (string, error) {
	body := map[string]string{
		"appKey":      hesiAppKey,
		"appSecurity": hesiSecret,
	}
	b, _ := json.Marshal(body)
	resp, err := httpClient.Post(hesiAPIBase+"/api/openapi/v1/auth/getAccessToken", "application/json", bytes.NewReader(b))
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
		return "", fmt.Errorf("解析授权失败: %w, body: %s", err, string(data[:min(len(data), 200)]))
	}
	if result.Value.AccessToken == "" {
		return "", fmt.Errorf("accessToken为空, body: %s", string(data[:min(len(data), 200)]))
	}
	log.Printf("获取授权成功, corporationId=%s", result.Value.CorporationId)
	return result.Value.AccessToken, nil
}

// 获取单据列表
func getFlowList(token, formType string, start, count int) (int, []map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/openapi/v1.1/docs/getApplyList?type=%s&start=%d&count=%d&accessToken=%s",
		hesiAPIBase, formType, start, count, token)
	resp, err := httpClient.Get(url)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var result struct {
		Count int                      `json:"count"`
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, nil, fmt.Errorf("解析失败: %w, body: %s", err, string(data[:min(len(data), 300)]))
	}
	return result.Count, result.Items, nil
}

// 获取附件信息
func getAttachments(token string, flowIds []string) ([]map[string]interface{}, error) {
	body := map[string]interface{}{"flowIds": flowIds}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/openapi/v1/flowDetails/attachment?accessToken=%s", hesiAPIBase, token), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var result struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("解析附件失败: %w", err)
	}
	return result.Items, nil
}

// 获取发票主体信息（批量，最多100个）
func getInvoiceDetails(token string, invoiceIds []string, objectId string) ([]map[string]interface{}, error) {
	body := map[string]interface{}{"ids": invoiceIds}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/openapi/v2/extension/INVOICE/object/%s/search?accessToken=%s", hesiAPIBase, objectId, token), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var result struct {
		Count int                      `json:"count"`
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("解析发票主体失败: %w", err)
	}
	return result.Items, nil
}

// 保存发票主体信息到数据库
func saveInvoiceDetails(db *sql.DB, items []map[string]interface{}) int {
	updated := 0
	for _, inv := range items {
		invId := getStr(inv, "id")
		if invId == "" {
			continue
		}
		// 兼容不同发票类型的字段名
		invNumber := getStr(inv, "E_system_发票主体_发票号码")
		if invNumber == "" {
			invNumber = getStr(inv, "E_system_非税收入类票据_票据号码")
		}
		invCode := getStr(inv, "E_system_发票主体_发票代码")
		if invCode == "" {
			invCode = getStr(inv, "E_system_非税收入类票据_票据代码")
		}
		invDate := getInt64(inv, "E_system_发票主体_发票日期")
		if invDate == 0 {
			invDate = getInt64(inv, "E_system_非税收入类票据_开票日期")
		}
		invStatus := getStr(inv, "E_system_发票主体_发票状态")
		invType := getStr(inv, "E_system_发票主体_发票类别")
		if invType == "" {
			invType = getStr(inv, "E_system_非税收入类票据_发票类别")
		}
		buyerName := getStr(inv, "E_system_发票主体_购买方名称")
		buyerTaxNo := getStr(inv, "E_system_发票主体_购买方纳税人识别号")
		sellerName := getStr(inv, "E_system_发票主体_销售方名称")
		if sellerName == "" {
			sellerName = getStr(inv, "E_system_非税收入类票据_收款单位")
		}
		sellerTaxNo := getStr(inv, "E_system_发票主体_销售方纳税人识别号")
		verified := 0
		if v, ok := inv["E_system_发票主体_验真"]; ok && v == true {
			verified = 1
		}
		invAmount := getMoney(inv, "E_system_发票主体_发票金额")
		totalAmount := getMoney(inv, "E_system_发票主体_价税合计")
		if !totalAmount.Valid {
			totalAmount = getMoney(inv, "E_system_非税收入类票据_金额合计")
		}
		taxAmount := getMoney(inv, "E_税额")
		if !taxAmount.Valid {
			taxAmount = getMoney(inv, "E_system_发票主体_税额")
		}

		result, err := db.Exec(`UPDATE hesi_flow_invoice SET
			invoice_number=?, invoice_code=?, invoice_date=?, invoice_amount=?, total_amount=?,
			tax_amount=?, invoice_status=?, invoice_type=?,
			buyer_name=?, buyer_tax_no=?, seller_name=?, seller_tax_no=?, is_verified=?
			WHERE invoice_id=?`,
			nullStr(invNumber), nullStr(invCode), nullInt64(invDate), invAmount, totalAmount,
			taxAmount, nullStr(invStatus), nullStr(invType),
			nullStr(buyerName), nullStr(buyerTaxNo), nullStr(sellerName), nullStr(sellerTaxNo), verified,
			invId)
		if err == nil {
			if n, _ := result.RowsAffected(); n > 0 {
				updated++
			}
		}
	}
	return updated
}

func getStr(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func getInt64(m map[string]interface{}, key string) int64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}

func getMoney(m map[string]interface{}, key string) sql.NullFloat64 {
	v, ok := m[key]
	if !ok || v == nil {
		return sql.NullFloat64{}
	}
	moneyMap, ok := v.(map[string]interface{})
	if !ok {
		return sql.NullFloat64{}
	}
	std := getStr(moneyMap, "standard")
	if std == "" {
		return sql.NullFloat64{}
	}
	f, err := strconv.ParseFloat(std, 64)
	if err != nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: f, Valid: true}
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullInt64(n int64) sql.NullInt64 {
	if n == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: n, Valid: true}
}

func isActive(item map[string]interface{}) bool {
	v, ok := item["active"]
	if !ok {
		return true
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return true
}

func saveFlow(db *sql.DB, item map[string]interface{}) error {
	flowId := getStr(item, "id")
	formType := getStr(item, "formType")
	state := getStr(item, "state")
	ownerId := getStr(item, "ownerId")
	ownerDept := getStr(item, "ownerDefaultDepartment")
	createTime := getInt64(item, "createTime")
	updateTime := getInt64(item, "updateTime")
	corpId := getStr(item, "corporationId")
	active := 1
	if !isActive(item) {
		active = 0
	}

	form, _ := item["form"].(map[string]interface{})
	if form == nil {
		return fmt.Errorf("form为空, flowId=%s", flowId)
	}

	code := getStr(form, "code")
	title := getStr(form, "title")
	submitterId := getStr(form, "submitterId")
	submitDate := getInt64(form, "submitDate")
	payDate := getInt64(form, "payDate")
	flowEndTime := getInt64(form, "flowEndTime")
	voucherNo := getStr(form, "voucherNo")
	voucherStatus := getStr(form, "voucherStatus")
	specId := getStr(form, "specificationId")

	// 部门：不同类型字段名不同
	deptId := getStr(form, "expenseDepartment")
	if deptId == "" {
		deptId = getStr(form, "loanDepartment")
	}

	payMoney := getMoney(form, "payMoney")
	expenseMoney := getMoney(form, "expenseMoney")
	loanMoney := getMoney(form, "loanMoney")
	receiptMoney := getMoney(form, "receiptMoney")

	rawJSON, _ := json.Marshal(form)

	_, err := db.Exec(`INSERT INTO hesi_flow
		(flow_id, code, title, form_type, state, owner_id, owner_department,
		 submitter_id, department_id, pay_money, expense_money, loan_money, receipt_money,
		 create_time, update_time, submit_date, pay_date, flow_end_time,
		 voucher_no, voucher_status, corporation_id, specification_id, active, raw_json)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
			title=VALUES(title), state=VALUES(state), active=VALUES(active),
			pay_money=VALUES(pay_money), expense_money=VALUES(expense_money),
			loan_money=VALUES(loan_money), receipt_money=VALUES(receipt_money),
			update_time=VALUES(update_time), pay_date=VALUES(pay_date),
			flow_end_time=VALUES(flow_end_time),
			voucher_no=VALUES(voucher_no), voucher_status=VALUES(voucher_status),
			raw_json=VALUES(raw_json)`,
		flowId, code, title, formType, state, nullStr(ownerId), nullStr(ownerDept),
		nullStr(submitterId), nullStr(deptId), payMoney, expenseMoney, loanMoney, receiptMoney,
		nullInt64(createTime), nullInt64(updateTime), nullInt64(submitDate), nullInt64(payDate), nullInt64(flowEndTime),
		nullStr(voucherNo), nullStr(voucherStatus), nullStr(corpId), nullStr(specId), active, string(rawJSON))
	return err
}

func saveDetails(db *sql.DB, flowId string, form map[string]interface{}) error {
	details, ok := form["details"]
	if !ok || details == nil {
		return nil
	}
	detailList, ok := details.([]interface{})
	if !ok {
		return nil
	}

	for i, d := range detailList {
		det, ok := d.(map[string]interface{})
		if !ok {
			continue
		}
		feeTypeId := getStr(det, "feeTypeId")
		specId := getStr(det, "specificationId")

		ftf, _ := det["feeTypeForm"].(map[string]interface{})
		if ftf == nil {
			continue
		}

		detailId := getStr(ftf, "detailId")
		amount := getMoney(ftf, "amount")
		feeDate := getInt64(ftf, "feeDate")
		reasons := getStr(ftf, "consumptionReasons")
		detailNo := i + 1
		if dn := getStr(ftf, "detailNo"); dn != "" {
			if n, err := strconv.Atoi(dn); err == nil {
				detailNo = n
			}
		}

		// 发票状态
		invoiceCount := 0
		invoiceStr := getStr(ftf, "invoice")
		if invoiceStr != "" && invoiceStr != "0" {
			invoiceCount, _ = strconv.Atoi(invoiceStr)
		}
		invoiceStatus := "noExist"
		if invForm, ok := ftf["invoiceForm"].(map[string]interface{}); ok {
			if getStr(invForm, "type") == "exist" {
				invoiceStatus = "exist"
			}
			// 保存发票关联
			if invoices, ok := invForm["invoices"].([]interface{}); ok {
				for _, inv := range invoices {
					invMap, ok := inv.(map[string]interface{})
					if !ok {
						continue
					}
					invoiceId := getStr(invMap, "invoiceId")
					taxAmt := getMoney(invMap, "taxAmount")
					approveAmt := getMoney(invMap, "approveAmount")

					db.Exec(`INSERT INTO hesi_flow_invoice
						(flow_id, detail_id, invoice_id, tax_amount, approve_amount)
						VALUES (?,?,?,?,?)
						ON DUPLICATE KEY UPDATE
						tax_amount=VALUES(tax_amount), approve_amount=VALUES(approve_amount)`,
						flowId, nullStr(detailId), nullStr(invoiceId), taxAmt, approveAmt)
				}
			}
		}

		rawDetail, _ := json.Marshal(det)
		db.Exec(`INSERT INTO hesi_flow_detail
			(flow_id, detail_id, detail_no, fee_type_id, specification_id,
			 amount, fee_date, invoice_count, invoice_status, consumption_reasons, raw_json)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE
			detail_no=VALUES(detail_no), amount=VALUES(amount), fee_date=VALUES(fee_date),
			invoice_count=VALUES(invoice_count), invoice_status=VALUES(invoice_status),
			consumption_reasons=VALUES(consumption_reasons), raw_json=VALUES(raw_json)`,
			flowId, nullStr(detailId), detailNo, nullStr(feeTypeId), nullStr(specId),
			amount, nullInt64(feeDate), invoiceCount, invoiceStatus, nullStr(reasons), string(rawDetail))
	}
	return nil
}

func saveAttachments(db *sql.DB, items []map[string]interface{}) {
	for _, item := range items {
		flowId := getStr(item, "flowId")
		attList, ok := item["attachmentList"].([]interface{})
		if !ok {
			continue
		}

		// 先删旧附件记录
		db.Exec("DELETE FROM hesi_flow_attachment WHERE flow_id=?", flowId)

		for _, att := range attList {
			attMap, ok := att.(map[string]interface{})
			if !ok {
				continue
			}
			attType := getStr(attMap, "type")
			freeId := getStr(attMap, "freeId")

			// 普通附件
			if urls, ok := attMap["attachmentUrls"].([]interface{}); ok {
				for _, u := range urls {
					urlMap, ok := u.(map[string]interface{})
					if !ok {
						continue
					}
					db.Exec(`INSERT INTO hesi_flow_attachment
						(flow_id, attachment_type, file_id, file_name, is_invoice, free_id)
						VALUES (?,?,?,?,0,?)`,
						flowId, attType, nullStr(getStr(urlMap, "fileId")), nullStr(getStr(urlMap, "fileName")), nullStr(freeId))
				}
			}

			// 发票文件
			if urls, ok := attMap["invoiceUrls"].([]interface{}); ok {
				for _, u := range urls {
					urlMap, ok := u.(map[string]interface{})
					if !ok {
						continue
					}
					db.Exec(`INSERT INTO hesi_flow_attachment
						(flow_id, attachment_type, file_id, file_name, is_invoice, invoice_number, invoice_code, free_id)
						VALUES (?,?,?,?,1,?,?,?)`,
						flowId, attType, nullStr(getStr(urlMap, "fileId")), nullStr(getStr(urlMap, "fileName")),
						nullStr(getStr(urlMap, "invoiceNumber")), nullStr(getStr(urlMap, "invoiceCode")), nullStr(freeId))
				}
			}

			// 回单
			if urls, ok := attMap["receiptUrls"].([]interface{}); ok {
				for _, u := range urls {
					urlMap, ok := u.(map[string]interface{})
					if !ok {
						continue
					}
					db.Exec(`INSERT INTO hesi_flow_attachment
						(flow_id, attachment_type, file_id, file_name, is_invoice, free_id)
						VALUES (?,?,?,?,0,?)`,
						flowId, attType, nullStr(getStr(urlMap, "key")), nullStr(getStr(urlMap, "key")), nullStr(freeId))
				}
			}
		}
	}
}

// syncFlows 同步指定类型和状态的单据，maxCount限制最多拉取条数（0=不限）
func syncFlows(db *sql.DB, token, formType, state string, maxCount int) (int, []string) {
	total, _, err := getFlowListWithState(token, formType, state, 0, 1)
	if err != nil {
		log.Printf("[%s/%s] 查询失败: %v", formType, state, err)
		return 0, nil
	}
	if total == 0 {
		return 0, nil
	}
	limit := total
	if maxCount > 0 && limit > maxCount {
		limit = maxCount
	}
	fmt.Printf("  %s(%s): %d 条(拉取%d)\n", formType, state, total, limit)

	var flowIds []string
	count := 0
	for start := 0; start < limit; start += pageSize {
		_, items, err := getFlowListWithState(token, formType, state, start, pageSize)
		if err != nil {
			log.Printf("[%s/%s] 第%d页失败: %v", formType, state, start/pageSize+1, err)
			continue
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			if !isActive(item) {
				continue
			}
			flowId := getStr(item, "id")
			if err := saveFlow(db, item); err != nil {
				log.Printf("保存失败 %s: %v", flowId, err)
				continue
			}
			form, _ := item["form"].(map[string]interface{})
			if form != nil {
				saveDetails(db, flowId, form)
			}
			flowIds = append(flowIds, flowId)
			count++
		}
		time.Sleep(200 * time.Millisecond)
	}
	return count, flowIds
}

// getFlowListWithState 带状态筛选的列表请求
func getFlowListWithState(token, formType, state string, start, count int) (int, []map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/openapi/v1.1/docs/getApplyList?type=%s&start=%d&count=%d&accessToken=%s",
		hesiAPIBase, formType, start, count, token)
	if state != "" {
		url += "&state=" + state
	}
	resp, err := httpClient.Get(url)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var result struct {
		Count int                      `json:"count"`
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, nil, fmt.Errorf("解析失败: %w", err)
	}
	return result.Count, result.Items, nil
}

func syncAttachments(db *sql.DB, token string, flowIds []string) int {
	totalAttachments := 0
	fmt.Printf("\n=== 同步附件元信息 (%d个单据) ===\n", len(flowIds))

	for i := 0; i < len(flowIds); i += attachBatch {
		end := i + attachBatch
		if end > len(flowIds) {
			end = len(flowIds)
		}
		batch := flowIds[i:end]

		items, err := getAttachments(token, batch)
		if err != nil {
			log.Printf("获取附件失败(batch %d-%d): %v", i, end, err)
			continue
		}
		saveAttachments(db, items)

		for _, item := range items {
			if attList, ok := item["attachmentList"].([]interface{}); ok {
				for _, att := range attList {
					if attMap, ok := att.(map[string]interface{}); ok {
						if urls, ok := attMap["attachmentUrls"].([]interface{}); ok {
							totalAttachments += len(urls)
						}
						if urls, ok := attMap["invoiceUrls"].([]interface{}); ok {
							totalAttachments += len(urls)
						}
						if urls, ok := attMap["receiptUrls"].([]interface{}); ok {
							totalAttachments += len(urls)
						}
					}
				}
			}
		}

		if len(flowIds) > attachBatch {
			fmt.Printf("  附件进度 %d/%d\n", end, len(flowIds))
		}
		time.Sleep(300 * time.Millisecond)
	}
	return totalAttachments
}

func main() {
	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)

	token, err := getAccessToken()
	if err != nil {
		log.Fatalf("获取授权失败: %v", err)
	}

	// 解析参数: --mode full|incremental (默认incremental)
	mode := "incremental"
	for i, arg := range os.Args[1:] {
		if arg == "--mode" && i+1 < len(os.Args)-1 {
			mode = os.Args[i+2]
		}
		if arg == "--full" {
			mode = "full"
		}
	}

	types := []string{"expense", "loan", "requisition", "custom"}
	totalFlows := 0
	var allFlowIds []string

	if mode == "full" {
		// 全量模式：拉所有非草稿状态
		fmt.Println("========== 全量同步 ==========")
		allStates := []string{"approving", "paying", "pending", "PROCESSING", "paid", "archived", "rejected"}
		for _, formType := range types {
			fmt.Printf("\n=== %s ===\n", formType)
			for _, st := range allStates {
				count, ids := syncFlows(db, token, formType, st, 0)
				totalFlows += count
				allFlowIds = append(allFlowIds, ids...)
			}
		}
	} else {
		// 增量模式：
		// 1. 活跃单据全拉（approving/paying/pending/PROCESSING）
		// 2. 已完成的只拉最近200条（捕获刚变更的）
		fmt.Println("========== 增量同步 ==========")
		activeStates := []string{"approving", "paying", "pending", "PROCESSING"}
		recentStates := []string{"paid", "archived", "rejected"}

		for _, formType := range types {
			fmt.Printf("\n=== %s ===\n", formType)
			for _, st := range activeStates {
				count, ids := syncFlows(db, token, formType, st, 0)
				totalFlows += count
				allFlowIds = append(allFlowIds, ids...)
			}
			for _, st := range recentStates {
				count, ids := syncFlows(db, token, formType, st, 200)
				totalFlows += count
				allFlowIds = append(allFlowIds, ids...)
			}
		}
	}

	// 同步附件
	totalAttachments := syncAttachments(db, token, allFlowIds)

	// 同步发票主体信息（补充发票代码、号码、购销方等）
	fmt.Printf("\n=== 同步发票主体信息 ===\n")
	var invoiceIds []string
	{
		rows, _ := db.Query("SELECT DISTINCT invoice_id FROM hesi_flow_invoice WHERE invoice_id IS NOT NULL AND invoice_id != '' AND (invoice_number IS NULL OR invoice_number = '') LIMIT 10000")
		if rows != nil {
			for rows.Next() {
				var id string
				rows.Scan(&id)
				invoiceIds = append(invoiceIds, id)
			}
			rows.Close()
		}
	}
	fmt.Printf("需要补充发票信息: %d 条\n", len(invoiceIds))

	// 遍历所有发票类型查询
	invoiceTypes := []string{"invoice", "noTaxIncome", "taxi", "fixed", "train", "flightItinerary", "tolls", "machinePrint", "other", "passengerCar", "shopping", "medical", "overseasInvoice"}
	totalInvoiceUpdated := 0
	remaining := make([]string, len(invoiceIds))
	copy(remaining, invoiceIds)

	for _, invType := range invoiceTypes {
		if len(remaining) == 0 {
			break
		}
		typeUpdated := 0
		var notFound []string
		for i := 0; i < len(remaining); i += 100 {
			end := i + 100
			if end > len(remaining) {
				end = len(remaining)
			}
			batch := remaining[i:end]
			items, err := getInvoiceDetails(token, batch, invType)
			if err != nil {
				log.Printf("获取发票主体失败(%s, batch %d-%d): %v", invType, i, end, err)
				notFound = append(notFound, batch...)
				continue
			}
			updated := saveInvoiceDetails(db, items)
			typeUpdated += updated

			// 找出未匹配的ID
			matchedIds := make(map[string]bool)
			for _, item := range items {
				matchedIds[getStr(item, "id")] = true
			}
			for _, id := range batch {
				if !matchedIds[id] {
					notFound = append(notFound, id)
				}
			}
			time.Sleep(300 * time.Millisecond)
		}
		if typeUpdated > 0 {
			fmt.Printf("  %s: 更新 %d 条\n", invType, typeUpdated)
		}
		totalInvoiceUpdated += typeUpdated
		remaining = notFound
	}
	fmt.Printf("发票主体信息更新: %d 条, 未匹配: %d 条\n", totalInvoiceUpdated, len(remaining))

	// 删除草稿数据（如果之前全量导入过）
	result, _ := db.Exec("DELETE FROM hesi_flow WHERE state='draft'")
	if result != nil {
		deleted, _ := result.RowsAffected()
		if deleted > 0 {
			fmt.Printf("\n已清理 %d 条草稿数据\n", deleted)
			// 清理关联数据
			db.Exec("DELETE d FROM hesi_flow_detail d LEFT JOIN hesi_flow f ON d.flow_id=f.flow_id WHERE f.flow_id IS NULL")
			db.Exec("DELETE i FROM hesi_flow_invoice i LEFT JOIN hesi_flow f ON i.flow_id=f.flow_id WHERE f.flow_id IS NULL")
			db.Exec("DELETE a FROM hesi_flow_attachment a LEFT JOIN hesi_flow f ON a.flow_id=f.flow_id WHERE f.flow_id IS NULL")
		}
	}

	fmt.Printf("\n========== 同步完成 (%s模式) ==========\n", mode)
	fmt.Printf("单据: %d 条\n", totalFlows)
	fmt.Printf("附件: %d 个\n", totalAttachments)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
