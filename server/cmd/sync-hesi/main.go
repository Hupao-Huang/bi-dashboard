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
	"regexp"
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
	// 6/11 翻车收正: 5/20 调到 200 后接口实际全批报错(返回中文错误文本非JSON),
	// 5/20-6/11 附件静默漏同步 7359 批。合思附件接口上限就是 100 (v0.16 原注释是对的), 别再调大
	attachBatch = 100
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
		// 带上响应体片段 — 5/20 这里只报 invalid character, 接口真实报错被吞了三周
		snip := string(data)
		if len(snip) > 200 {
			snip = snip[:200]
		}
		return nil, fmt.Errorf("解析附件失败: %w (HTTP %d, body: %s)", err, resp.StatusCode, snip)
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

// pickFirstStr 按字段后缀通用匹配 (兼容机打发票/出租车/火车/机票等不同 invoice 类型)
// 合思字段命名 E_system_{发票类型中文名}_{字段中文名}, 不同类型前缀不同, 后缀一致
func pickFirstStr(inv map[string]interface{}, suffix string) string {
	for k, v := range inv {
		if !strings.HasSuffix(k, suffix) {
			continue
		}
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func pickFirstInt64(inv map[string]interface{}, suffix string) int64 {
	for k, v := range inv {
		if !strings.HasSuffix(k, suffix) {
			continue
		}
		switch x := v.(type) {
		case float64:
			if x != 0 {
				return int64(x)
			}
		case int64:
			if x != 0 {
				return x
			}
		}
	}
	return 0
}

func pickFirstMoney(inv map[string]interface{}, suffix string) sql.NullFloat64 {
	for k, v := range inv {
		if !strings.HasSuffix(k, suffix) {
			continue
		}
		if m, ok := v.(map[string]interface{}); ok {
			if std, ok := m["standard"].(string); ok && std != "" {
				if f, err := strconv.ParseFloat(std, 64); err == nil && f != 0 {
					return sql.NullFloat64{Float64: f, Valid: true}
				}
			}
		}
	}
	return sql.NullFloat64{}
}

// tollPassRe 匹配过路费(ETC)发票备注里的"通行日期起/止：起/止"(冒号兼容中英文)。
// 备注样例: 车牌号：鲁V7BC76\n车辆类型：客车\n通行日期起/止：2026-05-24 11:25:11/2026-05-24 11:48:07\n入/出口站：...
var tollPassRe = regexp.MustCompile(`通行日期起/止[:：]\s*(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\s*/\s*(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})`)

// cstZone 固定北京时间 UTC+8 (中国无夏令时); 不依赖宿主 time.Local, 防上云迁 UTC 时区把通行时间解析跨天 (二审)。
var cstZone = time.FixedZone("CST", 8*3600)

// parseTollPassDates 从过路费发票备注提取"通行日期起/止"的起止毫秒戳; 非过路费/无此字段返回 0,0。
func parseTollPassDates(remark string) (start, end int64) {
	m := tollPassRe.FindStringSubmatch(remark)
	if len(m) != 3 {
		return 0, 0
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", m[1], cstZone); err == nil {
		start = t.UnixMilli()
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", m[2], cstZone); err == nil {
		end = t.UnixMilli()
	}
	return start, end
}

// invDetailLine 发票货物明细行(只取过路费要用的通行日期; 用 interface{} 兼容数字/字符串, 防类型异常整批解析失败)。
type invDetailLine struct {
	MasterID  string      `json:"masterId"`
	PassStart interface{} `json:"E_system_发票明细_通行日期起"`
	PassEnd   interface{} `json:"E_system_发票明细_通行日期止"`
}

// asMs 把合思明细行的通行日期字段转毫秒戳, 兼容 float64/json.Number/字符串; 解析不出返回 0。
func asMs(v interface{}) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case json.Number:
		n, _ := x.Int64()
		return n
	case string:
		if f, err := strconv.ParseFloat(x, 64); err == nil {
			return int64(f)
		}
	}
	return 0
}

// fetchTollPassFromDetails 批量拉发票货物明细行的"通行日期起/止"(毫秒)。
// 背景: 过路费有两种票 —— 全电(FULL_DIGITAl_NORMAL)通行日期在发票备注(parseTollPassDates 解析);
//       普票(ELECTRONIC_PAPER_FEE 等)备注为空, 通行日期在货物明细行 E_system_发票明细_通行日期起/止 (跑哥 2026-06-17)。
// 返回 invoiceID(masterId) → [起, 止] 毫秒; 同一发票多明细行取最早的"起"(及其对应"止")。
func fetchTollPassFromDetails(token string, invoiceIDs []string) map[string][2]int64 {
	out := map[string][2]int64{}
	for i := 0; i < len(invoiceIDs); i += 100 { // detailBatch 上限 100 (同发票主体口径)
		end := i + 100
		if end > len(invoiceIDs) {
			end = len(invoiceIDs)
		}
		body, _ := json.Marshal(map[string]interface{}{"invoiceIds": invoiceIDs[i:end]})
		resp, err := httpClient.Post(
			fmt.Sprintf("%s/api/openapi/v2/extension/INVOICE/object/invoice/detailBatch?accessToken=%s", hesiAPIBase, token),
			"application/json", bytes.NewReader(body))
		if err != nil {
			log.Printf("过路费明细行拉取失败(batch %d-%d): %v", i, end, err)
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			log.Printf("过路费明细行 HTTP %d: %s", resp.StatusCode, string(data[:min(len(data), 200)]))
			continue
		}
		var parsed struct {
			Items []invDetailLine `json:"items"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			log.Printf("过路费明细行解析失败: %v (body: %s)", err, string(data[:min(len(data), 200)]))
			continue
		}
		mergeTollPass(out, parsed.Items)
		time.Sleep(300 * time.Millisecond)
	}
	return out
}

// mergeTollPass 把明细行的通行日期合并进 out (invoiceID → [起,止] 毫秒): 跳过无起始日的(全电票明细行无此字段),
// 同一发票多明细行取最早的"起"。抽成纯函数便于单测。
func mergeTollPass(out map[string][2]int64, items []invDetailLine) {
	for _, it := range items {
		if it.MasterID == "" {
			continue
		}
		start := asMs(it.PassStart)
		if start <= 0 { // 全电票明细行无此字段, 跳过(它走备注)
			continue
		}
		pair := [2]int64{start, asMs(it.PassEnd)}
		if cur, ok := out[it.MasterID]; !ok || start < cur[0] { // 多明细行取最早起
			out[it.MasterID] = pair
		}
	}
}

// backfillTollPassFromDetails 对待审单里"过路费明细 + toll_pass_start 仍为空"的发票(主要是普票),
// 调 detailBatch 取货物明细行的通行日期补进 toll_pass_start/end。
// 只补待审单(规则只对待审单生效)且只在 toll_pass_start 为空时写(幂等, 不覆盖备注已解析的全电票)。
func backfillTollPassFromDetails(db *sql.DB) {
	rows, err := db.Query(`SELECT DISTINCT i.invoice_id FROM hesi_flow_invoice i
		JOIN hesi_flow_detail d ON i.detail_id = d.detail_id
		JOIN hesi_flow f ON d.flow_id = f.flow_id
		WHERE d.fee_type_id='ID01KhLSijR8FV' AND f.state IN ('approving','pending')
		  AND i.toll_pass_start IS NULL
		  AND i.invoice_id IS NOT NULL AND i.invoice_id != ''
		LIMIT 5000`)
	if err != nil {
		log.Printf("过路费明细行回补查询失败: %v", err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if len(ids) == 0 {
		return
	}
	// 本阶段在 sync 末尾, 距开头取 token 已隔附件+发票主体两个长阶段(full 可达 20+ 分钟), token 可能过期
	// → detailBatch 静默 401 致普票全补不上。这里重取保证本阶段 token 新鲜 (二审)。
	token, err := getAccessToken()
	if err != nil {
		log.Printf("过路费回补取 token 失败, 跳过本轮: %v", err)
		return
	}
	log.Printf("=== 过路费普票通行日期回补(货物明细行): 待补 %d 张 ===", len(ids))
	passMap := fetchTollPassFromDetails(token, ids)
	updated := 0
	for invID, pair := range passMap {
		// AND toll_pass_start IS NULL: 幂等 + 不覆盖全电备注已解析的; toll_pass_end 为 0 时存 NULL
		res, err := db.Exec(`UPDATE hesi_flow_invoice SET toll_pass_start=?, toll_pass_end=?
			WHERE invoice_id=? AND toll_pass_start IS NULL`,
			nullInt64(pair[0]), nullInt64(pair[1]), invID)
		if err != nil {
			log.Printf("过路费通行日期回补失败 inv=%s: %v", invID, err)
			continue
		}
		if n, _ := res.RowsAffected(); n > 0 {
			updated++
		}
	}
	log.Printf("过路费普票通行日期回补完成: 命中 %d 张 / 写入 %d 行", len(passMap), updated)
}

// 保存发票主体信息到数据库
func saveInvoiceDetails(db *sql.DB, items []map[string]interface{}) int {
	updated := 0
	for _, inv := range items {
		invId := getStr(inv, "id")
		if invId == "" {
			continue
		}
		// 通用 pattern 兼容所有发票类型 (发票主体/非税收入类票据/机打发票/出租车发票/火车发票/...)
		invNumber := pickFirstStr(inv, "_发票号码")
		if invNumber == "" {
			invNumber = pickFirstStr(inv, "_票据号码")
		}
		invCode := pickFirstStr(inv, "_发票代码")
		if invCode == "" {
			invCode = pickFirstStr(inv, "_票据代码")
		}
		invDate := pickFirstInt64(inv, "_发票日期")
		if invDate == 0 {
			invDate = pickFirstInt64(inv, "_开票日期")
		}
		if invDate == 0 {
			invDate = pickFirstInt64(inv, "_时间")
		}
		invStatus := pickFirstStr(inv, "_发票状态")
		invType := pickFirstStr(inv, "_发票类别")
		if invType == "" {
			invType = pickFirstStr(inv, "_发票种类")
		}
		buyerName := pickFirstStr(inv, "_购买方名称")
		buyerTaxNo := pickFirstStr(inv, "_购买方税号")
		if buyerTaxNo == "" {
			buyerTaxNo = pickFirstStr(inv, "_购买方纳税人识别号")
		}
		sellerName := pickFirstStr(inv, "_销售方名称")
		if sellerName == "" {
			sellerName = pickFirstStr(inv, "_收款单位")
		}
		sellerTaxNo := pickFirstStr(inv, "_销售方税号")
		if sellerTaxNo == "" {
			sellerTaxNo = pickFirstStr(inv, "_销售方纳税人识别号")
		}
		verified := 0
		if v, ok := inv["E_system_发票主体_验真"]; ok && v == true {
			verified = 1
		}
		invAmount := pickFirstMoney(inv, "_发票金额")
		totalAmount := pickFirstMoney(inv, "_价税合计")
		if !totalAmount.Valid {
			totalAmount = pickFirstMoney(inv, "_金额合计")
		}
		if !totalAmount.Valid {
			totalAmount = pickFirstMoney(inv, "_金额")
		}
		taxAmount := getMoney(inv, "E_税额")
		if !taxAmount.Valid {
			taxAmount = pickFirstMoney(inv, "_税额")
		}

		// 火车票出行信息 (合思发票主体 OCR, 用于规则 7-2 座位等级自动判 + 发票 tab 展示)
		seatType := pickFirstStr(inv, "_座位类型")
		trainNo := pickFirstStr(inv, "_车次")
		carriage := pickFirstStr(inv, "_车厢")
		seatNo := pickFirstStr(inv, "_席位")
		fromStation := pickFirstStr(inv, "_上车车站")
		toStation := pickFirstStr(inv, "_下车车站")
		passenger := pickFirstStr(inv, "_乘车人姓名")

		// 过路费(ETC)通行日期: 从发票备注"通行日期起/止"解析, 供补贴规则按通行日判 (跑哥 2026-06-17)
		tollStart, tollEnd := parseTollPassDates(pickFirstStr(inv, "_备注"))

		result, err := db.Exec(`UPDATE hesi_flow_invoice SET
			invoice_number=?, invoice_code=?, invoice_date=?, invoice_amount=?, total_amount=?,
			tax_amount=?, invoice_status=?, invoice_type=?,
			buyer_name=?, buyer_tax_no=?, seller_name=?, seller_tax_no=?, is_verified=?,
			seat_type=?, train_no=?, carriage=?, seat_no=?, from_station=?, to_station=?, passenger=?,
			toll_pass_start=COALESCE(?,toll_pass_start), toll_pass_end=COALESCE(?,toll_pass_end)
			WHERE invoice_id=?`,
			nullStr(invNumber), nullStr(invCode), nullInt64(invDate), invAmount, totalAmount,
			taxAmount, nullStr(invStatus), nullStr(invType),
			nullStr(buyerName), nullStr(buyerTaxNo), nullStr(sellerName), nullStr(sellerTaxNo), verified,
			nullStr(seatType), nullStr(trainNo), nullStr(carriage), nullStr(seatNo), nullStr(fromStation), nullStr(toStation), nullStr(passenger),
			nullInt64(tollStart), nullInt64(tollEnd),
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

// retireGhostFlows 幽灵单退场: 合思把单据删除/撤回到草稿后, getApplyList 直接不再返回该单,
// 增量同步无从感知, 单据就冻结在"待审"(approving/paying/...)状态, 详情页与 AI 建议永远读旧 raw_json。
// approveStates 接口仍会把这类单的当前环节报成"已删除"/"尚未提交", 这是权威信号 ——
// 据此把它们 active=0 退出待审池(详情/列表/统计/审批拉单都按 active=1 过滤)。
// 自愈: 单据若后来重新提交, getApplyList 会再返回, saveFlow 的 active=VALUES(active)=1 自动复活, 无需人工。
func retireGhostFlows(db *sql.DB) (int64, error) {
	res, err := db.Exec(`UPDATE hesi_flow SET active=0
		WHERE active=1 AND state IN ('approving','paying','pending','PROCESSING')
		AND current_stage_name IN ('已删除','尚未提交')`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
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

	var flowInvKeys [][2]string // 全单当前所有 (detailId, invoiceId), 循环后删不在其中的发票残留 (跑哥 2026-06-17)
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
			// 保存发票关联 + 收集当前 (明细,发票) 对 (供循环后单据级删残留)
			if invoices, ok := invForm["invoices"].([]interface{}); ok {
				for _, inv := range invoices {
					invMap, ok := inv.(map[string]interface{})
					if !ok {
						continue
					}
					invoiceId := getStr(invMap, "invoiceId")
					if invoiceId != "" && detailId != "" {
						flowInvKeys = append(flowInvKeys, [2]string{detailId, invoiceId})
					}
					taxAmt := getMoney(invMap, "taxAmount")
					approveAmt := getMoney(invMap, "approveAmount")

					// v1.71.0: UPSERT 发票关联失败 → 丢一条, log 即可
					if _, err := db.Exec(`INSERT INTO hesi_flow_invoice
						(flow_id, detail_id, invoice_id, tax_amount, approve_amount)
						VALUES (?,?,?,?,?)
						ON DUPLICATE KEY UPDATE
						tax_amount=VALUES(tax_amount), approve_amount=VALUES(approve_amount)`,
						flowId, nullStr(detailId), nullStr(invoiceId), taxAmt, approveAmt); err != nil {
						log.Printf("[sync-hesi] UPSERT invoice 失败 flow=%s detail=%s invoice=%s: %v", flowId, detailId, invoiceId, err)
					}
				}
			}
		}

		rawDetail, _ := json.Marshal(det)
		// v1.71.0: UPSERT 明细失败 → 丢一行, log + 计数(暂用 log)
		if _, err := db.Exec(`INSERT INTO hesi_flow_detail
			(flow_id, detail_id, detail_no, fee_type_id, specification_id,
			 amount, fee_date, invoice_count, invoice_status, consumption_reasons, raw_json)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE
			detail_no=VALUES(detail_no), amount=VALUES(amount), fee_date=VALUES(fee_date),
			invoice_count=VALUES(invoice_count), invoice_status=VALUES(invoice_status),
			consumption_reasons=VALUES(consumption_reasons), raw_json=VALUES(raw_json)`,
			flowId, nullStr(detailId), detailNo, nullStr(feeTypeId), nullStr(specId),
			amount, nullInt64(feeDate), invoiceCount, invoiceStatus, nullStr(reasons), string(rawDetail)); err != nil {
			log.Printf("[sync-hesi] UPSERT detail 失败 flow=%s detail=%s: %v", flowId, detailId, err)
		}
	}
	// 单据级删残留: 删掉该单"当前 raw_json 已没有"的发票(含整条明细被删的)。
	// 防驳回换发票后旧发票残留致规则8-1/8-2误报。仅当解析到≥1张当前发票才清(全单无发票时保守不动,
	// 防合思偶发不返发票误删全单) (跑哥 2026-06-17)。
	pruneFlowInvoices(db, flowId, flowInvKeys)
	return nil
}

// pruneFlowInvoices 删除某单"合思已移除"的发票残留, 保持 hesi_flow_invoice 与单据当前发票完全一致。
// keys = 当前 raw_json 全部 (detailId, invoiceId) 对。按 (detail_id, invoice_id) 整对 NOT IN 删,
// 既清"某明细下被换掉的发票", 也清"整条明细被删后遗留的发票"。
// keys 空(全单当前无发票)→ 保守不删, 防合思偶发不返发票把整单发票误删 (跑哥 2026-06-17)。
func pruneFlowInvoices(db *sql.DB, flowId string, keys [][2]string) {
	if flowId == "" || len(keys) == 0 {
		return
	}
	tuples := strings.TrimRight(strings.Repeat("(?,?),", len(keys)), ",")
	args := make([]interface{}, 0, len(keys)*2+1)
	args = append(args, flowId)
	for _, k := range keys {
		args = append(args, k[0], k[1])
	}
	if _, err := db.Exec(`DELETE FROM hesi_flow_invoice WHERE flow_id=? AND (detail_id, invoice_id) NOT IN (`+tuples+`)`, args...); err != nil {
		log.Printf("[sync-hesi] prune flow invoice 失败 flow=%s: %v", flowId, err)
	}
}

func saveAttachments(db *sql.DB, items []map[string]interface{}) {
	for _, item := range items {
		flowId := getStr(item, "flowId")
		attList, ok := item["attachmentList"].([]interface{})
		if !ok {
			continue
		}

		// 先删旧附件记录
		// v1.71.0: DELETE 失败 → 旧附件残留 + 新一批 INSERT IGNORE 跳过, log 排查
		if _, err := db.Exec("DELETE FROM hesi_flow_attachment WHERE flow_id=?", flowId); err != nil {
			log.Printf("[sync-hesi] DELETE old attachments 失败 flow=%s: %v", flowId, err)
		}

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
					// v1.71.0: INSERT 附件失败 → 丢一个附件链接, log 排查
					if _, err := db.Exec(`INSERT IGNORE INTO hesi_flow_attachment
						(flow_id, attachment_type, file_id, file_name, is_invoice, free_id)
						VALUES (?,?,?,?,0,?)`,
						flowId, attType, nullStr(getStr(urlMap, "fileId")), nullStr(getStr(urlMap, "fileName")), nullStr(freeId)); err != nil {
						log.Printf("[sync-hesi] INSERT attachment 失败 flow=%s file=%s: %v", flowId, getStr(urlMap, "fileId"), err)
					}
				}
			}

			// 发票文件
			if urls, ok := attMap["invoiceUrls"].([]interface{}); ok {
				for _, u := range urls {
					urlMap, ok := u.(map[string]interface{})
					if !ok {
						continue
					}
					if _, err := db.Exec(`INSERT IGNORE INTO hesi_flow_attachment
						(flow_id, attachment_type, file_id, file_name, is_invoice, invoice_number, invoice_code, free_id)
						VALUES (?,?,?,?,1,?,?,?)`,
						flowId, attType, nullStr(getStr(urlMap, "fileId")), nullStr(getStr(urlMap, "fileName")),
						nullStr(getStr(urlMap, "invoiceNumber")), nullStr(getStr(urlMap, "invoiceCode")), nullStr(freeId)); err != nil {
						log.Printf("[sync-hesi] INSERT invoice attachment 失败 flow=%s file=%s: %v", flowId, getStr(urlMap, "fileId"), err)
					}
				}
			}

			// 回单
			if urls, ok := attMap["receiptUrls"].([]interface{}); ok {
				for _, u := range urls {
					urlMap, ok := u.(map[string]interface{})
					if !ok {
						continue
					}
					if _, err := db.Exec(`INSERT IGNORE INTO hesi_flow_attachment
						(flow_id, attachment_type, file_id, file_name, is_invoice, free_id)
						VALUES (?,?,?,?,0,?)`,
						flowId, attType, nullStr(getStr(urlMap, "key")), nullStr(getStr(urlMap, "key")), nullStr(freeId)); err != nil {
						log.Printf("[sync-hesi] INSERT receipt 失败 flow=%s key=%s: %v", flowId, getStr(urlMap, "key"), err)
					}
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

	// 解析参数: --mode full|incremental (默认incremental), --loan-only 跳过单据/附件只同步借款包,
	// --active-only 轻量高频模式(跑哥 2026-06-17): 只同步活跃单(approving/pending 等)的 flow+明细+新发票抬头+审批节点,
	// 跳过附件/历史 paid-rejected/借款包/草稿清理。配 15min schtasks 缩短"单据改后审批页还显示旧内容"的窗口(原 hourly→15min)。
	mode := "incremental"
	loanOnly := false
	activeOnly := false
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
		if arg == "--active-only" {
			activeOnly = true
		}
	}

	// v1.70.5: --loan-only 只跑借款包同步 (回填/补漏用), 跳过单据/附件/发票
	if loanOnly {
		log.Println("========== --loan-only 模式: 仅同步借款包 ==========")
		loanOk, loanSkip := syncLoanInfos(db, token, mode == "full")
		log.Printf("借款包: %d 条 (无借款包 %d)", loanOk, loanSkip)
		return
	}
	if activeOnly {
		log.Println("========== --active-only 轻量同步: 仅活跃单内容+审批节点, 跳过附件/历史/借款包/草稿清理 ==========")
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
			if !activeOnly { // 轻量模式只拉活跃单, 跳过历史 paid/rejected
				for _, st := range recentStates {
					count, ids := syncFlows(db, token, formType, st, 0, sevenDaysAgo)
					totalFlows += count
					allFlowIds = append(allFlowIds, ids...)
				}
			}
		}
	}

	// 同步附件 (轻量模式跳过, 附件只在 hourly/full 跑)
	totalAttachments := 0
	if !activeOnly {
		totalAttachments = syncAttachments(db, token, allFlowIds)
	}

	// 同步发票主体信息（补充发票代码、号码、购销方等）
	log.Printf("=== 同步发票主体信息 ===")
	var invoiceIds []string
	{
		// 拉主体: 未拉过的(invoice_number空) + 过路费发票通行日期还没解析的(toll_pass_start空, 补历史; 跑哥 2026-06-17)。
		// 过路费回补只针对待审单(state in approving/pending): 规则只对待审单生效, 历史已审单不必补, 避免解析不出的票每次sync无限重拉 (二审)
		var rows *sql.Rows
		if activeOnly {
			// 轻量模式: 只补活跃单(approving/pending)的新发票(让抬头/税号新鲜), 不扫全表历史空发票
			rows, _ = db.Query(`SELECT DISTINCT i.invoice_id FROM hesi_flow_invoice i
				JOIN hesi_flow f ON i.flow_id = f.flow_id
				WHERE i.invoice_id IS NOT NULL AND i.invoice_id != ''
				AND f.state IN ('approving','pending')
				AND ((i.invoice_number IS NULL OR i.invoice_number = '')
				   OR (i.toll_pass_start IS NULL AND i.detail_id IN (
				       SELECT d.detail_id FROM hesi_flow_detail d WHERE d.fee_type_id='ID01KhLSijR8FV')))
				LIMIT 5000`)
		} else {
			rows, _ = db.Query(`SELECT DISTINCT i.invoice_id FROM hesi_flow_invoice i
				WHERE i.invoice_id IS NOT NULL AND i.invoice_id != ''
				AND ((i.invoice_number IS NULL OR i.invoice_number = '')
				   OR (i.toll_pass_start IS NULL AND i.detail_id IN (
				       SELECT d.detail_id FROM hesi_flow_detail d
				       JOIN hesi_flow f ON d.flow_id = f.flow_id
				       WHERE d.fee_type_id='ID01KhLSijR8FV' AND f.state IN ('approving','pending'))))
				LIMIT 10000`)
		}
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

	// 普票过路费通行日期回补: 备注没有(普票), 从货物明细行取 (跑哥 2026-06-17)。内部重取 token 防末尾过期
	backfillTollPassFromDetails(db)

	// 删除草稿数据（如果之前全量导入过）—— 轻量模式跳过(低频清理即可)
	if !activeOnly {
		result, _ := db.Exec("DELETE FROM hesi_flow WHERE state='draft'")
		if result != nil {
			deleted, _ := result.RowsAffected()
			if deleted > 0 {
				log.Printf("已清理 %d 条草稿数据", deleted)
				// 清理关联数据 (v1.71.0: 失败下次重跑能补, log 即可)
				if _, err := db.Exec("DELETE d FROM hesi_flow_detail d LEFT JOIN hesi_flow f ON d.flow_id=f.flow_id WHERE f.flow_id IS NULL"); err != nil {
					log.Printf("[sync-hesi] cleanup detail 孤儿失败: %v", err)
				}
				if _, err := db.Exec("DELETE i FROM hesi_flow_invoice i LEFT JOIN hesi_flow f ON i.flow_id=f.flow_id WHERE f.flow_id IS NULL"); err != nil {
					log.Printf("[sync-hesi] cleanup invoice 孤儿失败: %v", err)
				}
				if _, err := db.Exec("DELETE a FROM hesi_flow_attachment a LEFT JOIN hesi_flow f ON a.flow_id=f.flow_id WHERE f.flow_id IS NULL"); err != nil {
					log.Printf("[sync-hesi] cleanup attachment 孤儿失败: %v", err)
				}
			}
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

	// 幽灵单退场 (跑哥 2026-06-17): 合思删单/撤回到草稿后 getApplyList 不再返回该单,
	// 单据冻结在"待审", 详情页/AI建议永远读旧 raw_json (例 B26002569 卡在 5-13)。
	// 紧接 approveStates 之后跑: 此时 current_stage_name 刚刷新, 据"已删除/尚未提交"退场。
	if n, err := retireGhostFlows(db); err != nil {
		log.Printf("[ghost] 退场幽灵单失败: %v", err)
	} else if n > 0 {
		log.Printf("[ghost] 退场幽灵单(已删除/尚未提交): %d 单", n)
	}

	// v1.70.5: 同步借款包 (预付款核销追踪) —— 轻量模式跳过(借款包变动慢, hourly 同步足够)
	loanOk, loanSkip := 0, 0
	if !activeOnly {
		log.Println("========== 同步借款包信息 (预付款核销追踪) ==========")
		loanOk, loanSkip = syncLoanInfos(db, token, mode == "full")
	}

	log.Printf("========== 同步完成 (%s模式) ==========", mode)
	log.Printf("单据: %d 条", totalFlows)
	log.Printf("附件: %d 个", totalAttachments)
	log.Printf("借款包: %d 条 (无借款包 %d)", loanOk, loanSkip)

	// v1.73.2: sync 完通知 bi-server 清 hesi cache, 让用户立刻看到新数据 (15min cache 兜底, 这里追求立即)
	clearHesiCache(cfg.Webhook.Secret)
}

// clearHesiCache 通过 webhook 让 bi-server 清掉 /api/hesi/ 前缀的 cache
// 失败只 log, 不影响 sync 成功 (有 15min cache TTL 兜底)
func clearHesiCache(secret string) {
	if secret == "" {
		log.Println("[clear-cache] webhook.secret 未配, 跳过")
		return
	}
	url := "http://127.0.0.1:8080/api/webhook/clear-cache?prefix=api%7C/api/hesi/"
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		log.Printf("[clear-cache] 建请求失败: %v", err)
		return
	}
	req.Header.Set("X-Webhook-Secret", secret)
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[clear-cache] 调 bi-server 失败 (服务可能未启): %v", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	log.Printf("[clear-cache] bi-server 响应: %d %s", resp.StatusCode, string(body))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
