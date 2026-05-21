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
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
)

const (
	hesiAPIBase = "https://app.ekuaibao.com"
	pageSize    = 100
	// v1.70.3: 50 → 200, 22,698 单据从 454 batch × 4s = 30min+ 降到 114 batch × 2s = 4min
	// 合思 API flowIds 在请求体里, 没硬性 batch 上限 (实测 200 OK)
	attachBatch = 200
)

var (
	httpClient = &http.Client{Timeout: 60 * time.Second}
	hesiAppKey string
	hesiSecret string
)

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

// v1.58.0: 拉单据当前审批节点 + 审批人信息
// GET /api/openapi/v2/approveStates/[id1,id2,...]?accessToken=xxx
// Response: {items:[{flowId, stageName, operators:[{id,name,code}], delegateData}]}
// 注意: path 参数必须含 [], 例如 /approveStates/[ID01xxx,ID01yyy]
type ApproveState struct {
	FlowID    string
	StageName string
	OpID      string
	OpName    string
	OpCode    string
}

func fetchApproveStates(token string, flowIds []string) (map[string]ApproveState, error) {
	result := make(map[string]ApproveState, len(flowIds))
	if len(flowIds) == 0 {
		return result, nil
	}
	const batchSize = 20 // 合思接口 path 长度有限, 单批不能太大
	for i := 0; i < len(flowIds); i += batchSize {
		end := i + batchSize
		if end > len(flowIds) {
			end = len(flowIds)
		}
		batch := flowIds[i:end]
		// path 形如 [id1,id2,...]
		idsParam := "[" + joinStrings(batch, ",") + "]"
		url := fmt.Sprintf("%s/api/openapi/v2/approveStates/%s?accessToken=%s", hesiAPIBase, idsParam, token)
		resp, err := httpClient.Get(url)
		if err != nil {
			log.Printf("[approveStates] HTTP 失败 batch=%d: %v", i, err)
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var r struct {
			Items []struct {
				FlowID    string `json:"flowId"`
				StageName string `json:"stageName"`
				Operators []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
					Code string `json:"code"`
				} `json:"operators"`
			} `json:"items"`
		}
		if err := json.Unmarshal(data, &r); err != nil {
			log.Printf("[approveStates] 解析失败 batch=%d body=%s: %v", i, string(data[:min(len(data), 200)]), err)
			continue
		}
		for _, it := range r.Items {
			st := ApproveState{FlowID: it.FlowID, StageName: it.StageName}
			if len(it.Operators) > 0 {
				st.OpID = it.Operators[0].ID
				st.OpName = it.Operators[0].Name
				st.OpCode = it.Operators[0].Code
				// 多审批人时拼接显示 (例: 张三+李四)
				if len(it.Operators) > 1 {
					names := make([]string, len(it.Operators))
					for j, op := range it.Operators {
						names[j] = op.Name
					}
					st.OpName = joinStrings(names, "+")
				}
			}
			result[it.FlowID] = st
		}
		time.Sleep(200 * time.Millisecond) // 限流: 每批间 200ms 间隔
	}
	return result, nil
}

func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for i := 1; i < len(ss); i++ {
		out += sep + ss[i]
	}
	return out
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
					db.Exec(`INSERT IGNORE INTO hesi_flow_attachment
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
					db.Exec(`INSERT IGNORE INTO hesi_flow_attachment
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
					db.Exec(`INSERT IGNORE INTO hesi_flow_attachment
						(flow_id, attachment_type, file_id, file_name, is_invoice, free_id)
						VALUES (?,?,?,?,0,?)`,
						flowId, attType, nullStr(getStr(urlMap, "key")), nullStr(getStr(urlMap, "key")), nullStr(freeId))
				}
			}
		}
	}
}

// syncFlows 同步指定类型和状态的单据，maxCount 限制最多拉取条数（0=不限），startDate=yyyy-MM-dd HH:mm:ss 时间过滤（空=不限）
func syncFlows(db *sql.DB, token, formType, state string, maxCount int, startDate string) (int, []string) {
	total, _, err := getFlowListWithState(token, formType, state, 0, 1, startDate)
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
	suffix := ""
	if startDate != "" {
		suffix = fmt.Sprintf(" startDate>=%s", startDate)
	}
	log.Printf("  %s(%s): %d 条(拉取%d)%s", formType, state, total, limit, suffix)

	var flowIds []string
	count := 0
	for start := 0; start < limit; start += pageSize {
		_, items, err := getFlowListWithState(token, formType, state, start, pageSize, startDate)
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

// getFlowListWithState 带状态筛选+起始时间过滤的列表请求 (startDate 空=不限)
func getFlowListWithState(token, formType, state string, start, count int, startDate string) (int, []map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/openapi/v1.1/docs/getApplyList?type=%s&start=%d&count=%d&accessToken=%s",
		hesiAPIBase, formType, start, count, token)
	if state != "" {
		url += "&state=" + state
	}
	if startDate != "" {
		// 合思 v1.1 文档: orderBy=updateTime 默认查最近 1 年; 加 startDate 过滤 updateTime>=startDate
		url += "&orderBy=updateTime&startDate=" + strings.ReplaceAll(startDate, " ", "%20")
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
	log.Printf("=== 同步附件元信息 (%d个单据) ===", len(flowIds))

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
			log.Printf("  附件进度 %d/%d", end, len(flowIds))
		}
		time.Sleep(100 * time.Millisecond) // v1.70.3: 300ms → 100ms (省 ~2 分钟)
	}
	return totalAttachments
}

// v1.70.5: 借款包(预付款核销追踪)
// 合思预付款单(form_type=loan)出纳付款后会生成借款包(loanInfo),
// 借款包 remain 归零 → 单据 archived. 此函数拉借款包余额数据存 hesi_loan_info 表
// 接口文档: https://docs.ekuaibao.com/docs/open-api/flows/get-loanInfo-ByFlowId
// !!! 注意 URL 路径 $flowId 里的 $ 是字面字符不是模板变量, 必须保留 $ !!!

type LoanInfo struct {
	Id            string
	FlowId        string
	Total         float64
	Reserved      float64
	Remain        float64
	Repayment     float64
	State         string
	OwnerId       string
	CorporationId string
	LoanDate      int64
	RepaymentDate int64
	Active        bool
	RawJSON       string
}

func loanInfoToFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	}
	return 0
}

func fetchLoanInfo(token, flowId string) (*LoanInfo, error) {
	// URL 必须带字面 $ 前缀, 去掉会 404
	url := fmt.Sprintf("%s/api/openapi/v1/loans/getLoanInfoByFlowId/$%s?accessToken=%s",
		hesiAPIBase, flowId, token)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 400 {
		// 文档定义的 400: 该单据没有生成借款包, 跳过算正常
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(data)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, snippet)
	}
	var parsed struct {
		Value struct {
			Id            string      `json:"id"`
			FlowId        string      `json:"flowId"`
			Total         interface{} `json:"total"`
			Reserved      interface{} `json:"reserved"`
			Remain        interface{} `json:"remain"`
			Repayment     interface{} `json:"repayment"`
			State         string      `json:"state"`
			OwnerId       string      `json:"ownerId"`
			CorporationId string      `json:"corporationId"`
			LoanDate      int64       `json:"loanDate"`
			RepaymentDate int64       `json:"repaymentDate"`
			Active        bool        `json:"active"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("解析失败: %w", err)
	}
	v := parsed.Value
	if v.Id == "" {
		return nil, nil
	}
	return &LoanInfo{
		Id: v.Id, FlowId: v.FlowId,
		Total: loanInfoToFloat(v.Total), Reserved: loanInfoToFloat(v.Reserved),
		Remain: loanInfoToFloat(v.Remain), Repayment: loanInfoToFloat(v.Repayment),
		State: v.State, OwnerId: v.OwnerId, CorporationId: v.CorporationId,
		LoanDate: v.LoanDate, RepaymentDate: v.RepaymentDate, Active: v.Active,
		RawJSON: string(data),
	}, nil
}

func saveLoanInfo(db *sql.DB, li *LoanInfo) error {
	active := 0
	if li.Active {
		active = 1
	}
	_, err := db.Exec(`REPLACE INTO hesi_loan_info
		(loan_info_id, flow_id, total, reserved, remain, repayment, state,
		 owner_id, corporation_id, loan_date, repayment_date, active, raw_json)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		li.Id, li.FlowId, li.Total, li.Reserved, li.Remain, li.Repayment, li.State,
		nullStr(li.OwnerId), nullStr(li.CorporationId),
		li.LoanDate, li.RepaymentDate, active, li.RawJSON)
	return err
}

// syncLoanInfos 拉 form_type=loan 的单据对应的借款包
// 增量模式: 只拉 paid/paying/archived (借款包活跃状态)
// 全量模式: 拉所有 loan 单 (含 approving/rejected 等)
func syncLoanInfos(db *sql.DB, token string, fullMode bool) (int, int) {
	var q string
	if fullMode {
		q = `SELECT flow_id FROM hesi_flow WHERE active=1 AND form_type='loan'`
	} else {
		// 增量: paid/paying/archived 才有借款包数据;
		// archived 的虽然 PAID(已还清) 不会再变, 但首次同步也要拉一次确认 remain=0
		q = `SELECT flow_id FROM hesi_flow WHERE active=1 AND form_type='loan' AND state IN ('paid','paying','archived')`
	}
	rows, err := db.Query(q)
	if err != nil {
		log.Printf("[loanInfo] 查 loan 单失败: %v", err)
		return 0, 0
	}
	var flowIds []string
	for rows.Next() {
		var fid string
		if err := rows.Scan(&fid); err == nil {
			flowIds = append(flowIds, fid)
		}
	}
	rows.Close()

	log.Printf("待同步借款包: %d 单", len(flowIds))
	ok, skip, fail := 0, 0, 0
	for i, fid := range flowIds {
		li, err := fetchLoanInfo(token, fid)
		if err != nil {
			log.Printf("[loanInfo] fetch %s 失败: %v", fid, err)
			fail++
			continue
		}
		if li == nil {
			skip++
			continue
		}
		if err := saveLoanInfo(db, li); err != nil {
			log.Printf("[loanInfo] save %s 失败: %v", fid, err)
			fail++
			continue
		}
		ok++
		if (i+1)%100 == 0 {
			log.Printf("  借款包进度 %d/%d", i+1, len(flowIds))
		}
	}
	log.Printf("借款包同步完成: 成功 %d, 跳过(无借款包) %d, 失败 %d", ok, skip, fail)
	return ok, skip
}

func main() {
	// v1.57.1: 日志双写 — 既写固定 sync-hesi.log 又走 stdout
	// 这样 schtasks 通过 vbs 触发(写 sync-hesi.log) 跟 bi-server 触发(读 stdout 进 manual-*.log) 都能拿到日志
	logFile, err := os.OpenFile(`C:\Users\Administrator\bi-dashboard\server\sync-hesi.log`, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(io.MultiWriter(logFile, os.Stdout))
		defer logFile.Close()
	}

	// 整体超时保护：45 分钟未结束强制退出
	// 防止合思 API 卡死/死循环导致 schtasks 显示 Running 数小时
	// (2026-05-09 实测过 13 小时卡死, vbs 同步等待无法回收)
	// v1.70.3: 30 → 45 分钟, full 实测 22K 单据+附件正常 ~10 分钟跑完, 留 35 分钟兜底
	go func() {
		time.Sleep(45 * time.Minute)
		log.Fatalf("[sync-hesi] 超过 45 分钟未结束, 强制退出（防卡死）")
	}()

	unlock := importutil.AcquireLock("sync-hesi")
	defer unlock()

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	hesiAppKey = cfg.Hesi.AppKey
	hesiSecret = cfg.Hesi.Secret
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

	// 解析参数: --mode full|incremental (默认incremental), --loan-only 跳过单据/附件只同步借款包
	mode := "incremental"
	loanOnly := false
	for i, arg := range os.Args[1:] {
		if arg == "--mode" && i+1 < len(os.Args)-1 {
			mode = os.Args[i+2]
		}
		if arg == "--full" {
			mode = "full"
		}
		if arg == "--loan-only" {
			loanOnly = true
		}
	}

	// v1.70.5: --loan-only 只跑借款包同步 (回填/补漏用), 跳过单据/附件/发票
	if loanOnly {
		log.Println("========== --loan-only 模式: 仅同步借款包 ==========")
		loanOk, loanSkip := syncLoanInfos(db, token, mode == "full")
		log.Printf("借款包: %d 条 (无借款包 %d)", loanOk, loanSkip)
		return
	}

	types := []string{"expense", "loan", "requisition", "custom"}
	totalFlows := 0
	var allFlowIds []string

	if mode == "full" {
		// 全量模式：拉所有非草稿状态（无时间过滤，跑哥周末手动 --full 兜底）
		log.Println("========== 全量同步 ==========")
		allStates := []string{"approving", "paying", "pending", "PROCESSING", "paid", "archived", "rejected"}
		for _, formType := range types {
			log.Printf("=== %s ===", formType)
			for _, st := range allStates {
				count, ids := syncFlows(db, token, formType, st, 0, "")
				totalFlows += count
				allFlowIds = append(allFlowIds, ids...)
			}
		}
	} else {
		// 增量模式 (v1.62.x 优化):
		// 1. 活跃单据全拉 (approving/paying/pending/PROCESSING) — 大概率 1k 内
		// 2. 已完成只拉 paid/rejected 最近 7 天 (覆盖凭证回写 + 流转回退场景)
		// 3. archived 不拉 — 永久归档不会变, 周末 --full 兜底
		log.Println("========== 增量同步 (v1.62.x: archived 不拉, paid/rejected 7天内) ==========")
		activeStates := []string{"approving", "paying", "pending", "PROCESSING"}
		recentStates := []string{"paid", "rejected"}
		sevenDaysAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02 15:04:05")

		for _, formType := range types {
			log.Printf("=== %s ===", formType)
			for _, st := range activeStates {
				count, ids := syncFlows(db, token, formType, st, 0, "")
				totalFlows += count
				allFlowIds = append(allFlowIds, ids...)
			}
			for _, st := range recentStates {
				count, ids := syncFlows(db, token, formType, st, 0, sevenDaysAgo)
				totalFlows += count
				allFlowIds = append(allFlowIds, ids...)
			}
		}
	}

	// 同步附件
	totalAttachments := syncAttachments(db, token, allFlowIds)

	// 同步发票主体信息（补充发票代码、号码、购销方等）
	log.Printf("=== 同步发票主体信息 ===")
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
	log.Printf("需要补充发票信息: %d 条", len(invoiceIds))

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
			log.Printf("  %s: 更新 %d 条", invType, typeUpdated)
		}
		totalInvoiceUpdated += typeUpdated
		remaining = notFound
	}
	log.Printf("发票主体信息更新: %d 条, 未匹配: %d 条", totalInvoiceUpdated, len(remaining))

	// 删除草稿数据（如果之前全量导入过）
	result, _ := db.Exec("DELETE FROM hesi_flow WHERE state='draft'")
	if result != nil {
		deleted, _ := result.RowsAffected()
		if deleted > 0 {
			log.Printf("已清理 %d 条草稿数据", deleted)
			// 清理关联数据
			db.Exec("DELETE d FROM hesi_flow_detail d LEFT JOIN hesi_flow f ON d.flow_id=f.flow_id WHERE f.flow_id IS NULL")
			db.Exec("DELETE i FROM hesi_flow_invoice i LEFT JOIN hesi_flow f ON i.flow_id=f.flow_id WHERE f.flow_id IS NULL")
			db.Exec("DELETE a FROM hesi_flow_attachment a LEFT JOIN hesi_flow f ON a.flow_id=f.flow_id WHERE f.flow_id IS NULL")
		}
	}

	// v1.58.0: 对所有进行中状态的单据拉当前审批节点 + 审批人
	log.Println("========== 拉取当前审批节点 + 审批人信息 ==========")
	activeRows, err := db.Query(`SELECT flow_id FROM hesi_flow WHERE active=1 AND state NOT IN ('paid','archived','rejected','draft')`)
	if err != nil {
		log.Printf("[approveStates] 查 active flow 失败: %v", err)
	} else {
		var activeFlowIds []string
		for activeRows.Next() {
			var fid string
			if err := activeRows.Scan(&fid); err == nil {
				activeFlowIds = append(activeFlowIds, fid)
			}
		}
		activeRows.Close()
		log.Printf("待拉取审批状态: %d 单", len(activeFlowIds))
		if len(activeFlowIds) > 0 {
			states, err := fetchApproveStates(token, activeFlowIds)
			if err != nil {
				log.Printf("[approveStates] 拉取失败: %v", err)
			} else {
				updateCount := 0
				for _, st := range states {
					_, err := db.Exec(`UPDATE hesi_flow SET current_stage_name=?, current_approver_id=?, current_approver_name=?, current_approver_code=? WHERE flow_id=?`,
						nullStr(st.StageName), nullStr(st.OpID), nullStr(st.OpName), nullStr(st.OpCode), st.FlowID)
					if err != nil {
						log.Printf("[approveStates] 更新 %s 失败: %v", st.FlowID, err)
						continue
					}
					updateCount++
				}
				log.Printf("审批状态更新: %d 条", updateCount)
			}
		}
	}

	// v1.70.5: 同步借款包 (预付款核销追踪)
	log.Println("========== 同步借款包信息 (预付款核销追踪) ==========")
	loanOk, loanSkip := syncLoanInfos(db, token, mode == "full")

	log.Printf("========== 同步完成 (%s模式) ==========", mode)
	log.Printf("单据: %d 条", totalFlows)
	log.Printf("附件: %d 个", totalAttachments)
	log.Printf("借款包: %d 条 (无借款包 %d)", loanOk, loanSkip)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
