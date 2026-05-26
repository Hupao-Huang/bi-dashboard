package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	hesiAPIBase = "https://app.ekuaibao.com"
)

var hesiHTTP = &http.Client{Timeout: 30 * time.Second}

func (h *DashboardHandler) getHesiToken() (string, error) {
	body := map[string]string{"appKey": h.HesiAppKey, "appSecurity": h.HesiSecret}
	b, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	resp, err := hesiHTTP.Post(hesiAPIBase+"/api/openapi/v1/auth/getAccessToken", "application/json", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("获取合思token失败: http %d", resp.StatusCode)
	}
	var result struct {
		Value struct {
			AccessToken string `json:"accessToken"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	if result.Value.AccessToken == "" {
		return "", fmt.Errorf("获取合思token失败")
	}
	return result.Value.AccessToken, nil
}

// GetHesiFlows 获取合思单据列表（支持筛选/分页/搜索）
func (h *DashboardHandler) GetHesiFlows(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	formType := q.Get("formType")
	state := q.Get("state")
	invoiceStatus := q.Get("invoiceStatus")
	keyword := q.Get("keyword")
	approver := q.Get("approver") // v1.58.2: 当前审批人姓名 LIKE 搜索
	specificationID := q.Get("specificationId") // v1.63.x: 单据模板筛选 (字典 id 是 specification_id 的前缀)
	startDate := q.Get("startDate")
	endDate := q.Get("endDate")

	// 构建查询（排除已删除的单据）
	where := "f.active=1"
	args := []interface{}{}

	if formType != "" {
		where += " AND f.form_type=?"
		args = append(args, formType)
	}
	if state != "" {
		// v1.62.x: 支持分组别名 active=未结束 / terminal=已结束 / 其他=精确匹配
		switch state {
		case "active":
			where += " AND f.state IN ('approving','paying','pending','PROCESSING')"
		case "terminal":
			where += " AND f.state IN ('paid','archived','rejected')"
		default:
			where += " AND f.state=?"
			args = append(args, state)
		}
	}
	if keyword != "" {
		where += " AND (f.title LIKE ? OR f.code LIKE ?)"
		kw := "%" + keyword + "%"
		args = append(args, kw, kw)
	}
	if approver != "" {
		// v1.58.2: 按当前审批人姓名 LIKE 搜索 (含多人拼接 张三+李四)
		where += " AND f.current_approver_name LIKE ?"
		args = append(args, "%"+approver+"%")
	}
	if specificationID != "" {
		// v1.63.x: 单据模板筛选 — 字典 id 是 specification_id 的前缀 (LIKE 'ID01xxx%')
		where += " AND f.specification_id LIKE ?"
		args = append(args, specificationID+"%")
	}
	if startDate != "" {
		// 转毫秒时间戳
		t, err := time.Parse("2006-01-02", startDate)
		if err == nil {
			where += " AND f.create_time >= ?"
			args = append(args, t.UnixMilli())
		}
	}
	if endDate != "" {
		t, err := time.Parse("2006-01-02", endDate)
		if err == nil {
			where += " AND f.create_time < ?"
			args = append(args, t.AddDate(0, 0, 1).UnixMilli())
		}
	}

	// 如果筛选发票状态，需要JOIN明细表
	hasInvoiceFilter := invoiceStatus != ""
	// v1.70.5: 借款待还款 KPI 卡点击, JOIN 借款包表过滤 state=REPAID + active=1
	loanRepaid := q.Get("loanRepaid") == "1"
	fromClause := "hesi_flow f"
	selectFields := "DISTINCT f.flow_id, f.code, f.title, f.form_type, f.state, f.owner_id, f.department_id, f.submitter_id, f.pay_money, f.expense_money, f.loan_money, f.create_time, f.update_time, f.submit_date, f.pay_date, f.flow_end_time, f.voucher_no, f.voucher_status, JSON_UNQUOTE(JSON_EXTRACT(f.raw_json, '$.preApprovedNodeName')) AS pre_approved_node, JSON_UNQUOTE(JSON_EXTRACT(f.raw_json, '$.preNodeApprovedTime')) AS pre_approved_time, f.current_stage_name, f.current_approver_name, f.current_approver_code, f.specification_id"

	if hasInvoiceFilter {
		fromClause += " JOIN hesi_flow_detail d ON f.flow_id = d.flow_id"
		where += " AND d.invoice_status=?"
		args = append(args, invoiceStatus)
	}
	if loanRepaid {
		fromClause += " JOIN hesi_loan_info li ON f.flow_id = li.flow_id"
		where += " AND li.active=1 AND li.state='REPAID'"
	}

	// 总数
	var total int
	countSQL := fmt.Sprintf("SELECT COUNT(DISTINCT f.flow_id) FROM %s WHERE %s", fromClause, where)
	if writeDatabaseError(w, h.DB.QueryRow(countSQL, args...).Scan(&total)) {
		return
	}

	// 数据
	dataSQL := fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY f.create_time DESC LIMIT ? OFFSET ?", selectFields, fromClause, where)
	dataArgs := append(args, pageSize, offset)
	rows, err := h.DB.Query(dataSQL, dataArgs...)
	if err != nil {
		writeServerError(w, 500, "查询费控数据失败", err)
		return
	}
	defer rows.Close()

	type FlowItem struct {
		FlowId        string   `json:"flowId"`
		Code          string   `json:"code"`
		Title         string   `json:"title"`
		FormType      string   `json:"formType"`
		State         string   `json:"state"`
		OwnerId       *string  `json:"ownerId"`
		DepartmentId  *string  `json:"departmentId"`
		SubmitterId   *string  `json:"submitterId"`
		PayMoney      *float64 `json:"payMoney"`
		ExpenseMoney  *float64 `json:"expenseMoney"`
		LoanMoney     *float64 `json:"loanMoney"`
		CreateTime    *int64   `json:"createTime"`
		UpdateTime    *int64   `json:"updateTime"`
		SubmitDate    *int64   `json:"submitDate"`
		PayDate       *int64   `json:"payDate"`
		FlowEndTime   *int64   `json:"flowEndTime"`
		VoucherNo     *string  `json:"voucherNo"`
		VoucherStatus *string  `json:"voucherStatus"`
		// v1.57.2: 审批流进度 (从 raw_json 解析, 来自合思 API)
		PreApprovedNode *string `json:"preApprovedNode"` // 上一步已审批通过的节点名 (岗位级, 例: 直属上级/资金预算负责人)
		PreApprovedTime *string `json:"preApprovedTime"` // 上一步通过时间 (毫秒时间戳字符串)
		// v1.58.0: 当前审批节点 + 真实审批人姓名 (来自合思 /v2/approveStates 接口)
		CurrentStageName    *string `json:"currentStageName"`    // 当前审批节点 (例: 总经理/出纳支付)
		CurrentApproverName *string `json:"currentApproverName"` // 当前审批人姓名 (例: 易子涵, 多人时拼接 张三+李四)
		CurrentApproverCode *string `json:"currentApproverCode"` // 当前审批人工号
		// v1.62.x: 单据模板
		SpecificationId   *string `json:"specificationId"`
		SpecificationName string  `json:"specificationName"`
		// 明细汇总
		DetailCount     int `json:"detailCount"`
		InvoiceExist    int `json:"invoiceExist"`
		InvoiceMissing  int `json:"invoiceMissing"`
		AttachmentCount int `json:"attachmentCount"`
	}

	var items []FlowItem
	for rows.Next() {
		var item FlowItem
		if writeDatabaseError(w, rows.Scan(&item.FlowId, &item.Code, &item.Title, &item.FormType, &item.State,
			&item.OwnerId, &item.DepartmentId, &item.SubmitterId,
			&item.PayMoney, &item.ExpenseMoney, &item.LoanMoney,
			&item.CreateTime, &item.UpdateTime, &item.SubmitDate, &item.PayDate, &item.FlowEndTime,
			&item.VoucherNo, &item.VoucherStatus,
			&item.PreApprovedNode, &item.PreApprovedTime,
			&item.CurrentStageName, &item.CurrentApproverName, &item.CurrentApproverCode,
			&item.SpecificationId)) {
			return
		}
		if item.SpecificationId != nil && *item.SpecificationId != "" {
			item.SpecificationName = h.LookupSpecName(*item.SpecificationId)
		}
		items = append(items, item)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}

	// 批量查明细和附件统计：IN+GROUP BY 一次查完，避免 N+1（原循环每条 2 次 SQL，pageSize=20 时多 40 次往返）
	if len(items) > 0 {
		flowIds := make([]interface{}, len(items))
		placeholders := make([]string, len(items))
		for i := range items {
			flowIds[i] = items[i].FlowId
			placeholders[i] = "?"
		}
		ph := strings.Join(placeholders, ",")

		// 明细统计：count / 已开票 / 未开票
		type detailStat struct {
			count, exist, missing int
		}
		detailMap := make(map[string]detailStat, len(items))
		drows, err := h.DB.Query(fmt.Sprintf(
			`SELECT flow_id, COUNT(*), COALESCE(SUM(invoice_status='exist'),0), COALESCE(SUM(invoice_status='noExist'),0)
			 FROM hesi_flow_detail WHERE flow_id IN (%s) GROUP BY flow_id`, ph), flowIds...)
		if writeDatabaseError(w, err) {
			return
		}
		for drows.Next() {
			var fid string
			var s detailStat
			if err := drows.Scan(&fid, &s.count, &s.exist, &s.missing); err != nil {
				drows.Close()
				writeServerError(w, 500, "扫描明细统计失败", err)
				return
			}
			detailMap[fid] = s
		}
		drows.Close()

		// 附件统计
		attachMap := make(map[string]int, len(items))
		arows, err := h.DB.Query(fmt.Sprintf(
			`SELECT flow_id, COUNT(*) FROM hesi_flow_attachment WHERE flow_id IN (%s) GROUP BY flow_id`, ph), flowIds...)
		if writeDatabaseError(w, err) {
			return
		}
		for arows.Next() {
			var fid string
			var c int
			if err := arows.Scan(&fid, &c); err != nil {
				arows.Close()
				writeServerError(w, 500, "扫描附件统计失败", err)
				return
			}
			attachMap[fid] = c
		}
		arows.Close()

		for i := range items {
			if s, ok := detailMap[items[i].FlowId]; ok {
				items[i].DetailCount = s.count
				items[i].InvoiceExist = s.exist
				items[i].InvoiceMissing = s.missing
			}
			items[i].AttachmentCount = attachMap[items[i].FlowId]
		}
	}

	writeJSON(w, map[string]interface{}{
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
		"items":    items,
	})
}

// GetHesiFlowDetail 获取单据详情（含明细、发票、附件元信息）
func (h *DashboardHandler) GetHesiFlowDetail(w http.ResponseWriter, r *http.Request) {
	flowId := r.URL.Query().Get("flowId")
	if flowId == "" {
		writeError(w, 400, "缺少flowId参数")
		return
	}

	// 主表
	var rawJSON *string
	var flow struct {
		FlowId       string   `json:"flowId"`
		Code         string   `json:"code"`
		Title        string   `json:"title"`
		FormType     string   `json:"formType"`
		State        string   `json:"state"`
		OwnerId      *string  `json:"ownerId"`
		DepartmentId *string  `json:"departmentId"`
		// v1.74.9: owner_department 字段以前没 SELECT, 现在补上 (发起人所在部门, 跟 department_id=报销部门 不同)
		OwnerDepartment   *string  `json:"ownerDepartmentId"`
		SubmitterId       *string  `json:"submitterId"`
		PayMoney          *float64 `json:"payMoney"`
		ExpenseMoney      *float64 `json:"expenseMoney"`
		LoanMoney         *float64 `json:"loanMoney"`
		CreateTime        *int64   `json:"createTime"`
		UpdateTime        *int64   `json:"updateTime"`
		SubmitDate        *int64   `json:"submitDate"`
		PayDate           *int64   `json:"payDate"`
		FlowEndTime       *int64   `json:"flowEndTime"`
		VoucherNo         *string  `json:"voucherNo"`
		VoucherStatus     *string  `json:"voucherStatus"`
		SpecificationId   *string  `json:"specificationId"`
		SpecificationName string   `json:"specificationName"`
		// v1.74.9: 字典查询补名字, 单据详情弹窗用 (合思 API 设计返 ID, 名字得另查字典)
		OwnerName           string `json:"ownerName"`
		SubmitterName       string `json:"submitterName"`
		DepartmentName      string `json:"departmentName"`      // 报销/借款部门名
		OwnerDepartmentName string `json:"ownerDepartmentName"` // 发起人部门名
		LegalEntityId       string `json:"legalEntityId"`       // raw_json 里 "法人实体" 字段 (合思自定义维度)
		LegalEntityName     string `json:"legalEntityName"`     // 跑哥要的"公司名称"
		// v1.75.0: 钉钉花名册合同公司校验 (仅"日常报销单"模板触发)
		// EntityCheck = "ok"(一致) | "mismatch"(不一致) | "no_data"(钉钉无数据) | ""(不适用)
		EntityCheck         string `json:"entityCheck"`
		EntityCheckExpected string `json:"entityCheckExpected"` // 应为的公司名 (mismatch 时填)
		EntityCheckReason   string `json:"entityCheckReason"`   // 解释文案 (前端 Tooltip 用)
	}
	err := h.DB.QueryRow(`SELECT flow_id, code, title, form_type, state, owner_id, department_id, owner_department, submitter_id,
		pay_money, expense_money, loan_money, create_time, update_time, submit_date, pay_date, flow_end_time,
		voucher_no, voucher_status, specification_id, raw_json FROM hesi_flow WHERE flow_id=?`, flowId).Scan(
		&flow.FlowId, &flow.Code, &flow.Title, &flow.FormType, &flow.State,
		&flow.OwnerId, &flow.DepartmentId, &flow.OwnerDepartment, &flow.SubmitterId,
		&flow.PayMoney, &flow.ExpenseMoney, &flow.LoanMoney,
		&flow.CreateTime, &flow.UpdateTime, &flow.SubmitDate, &flow.PayDate, &flow.FlowEndTime,
		&flow.VoucherNo, &flow.VoucherStatus, &flow.SpecificationId, &rawJSON)
	if err != nil {
		writeError(w, 404, "单据不存在")
		return
	}

	// v1.62.x: 查模板名称 (60s 缓存)
	if flow.SpecificationId != nil && *flow.SpecificationId != "" {
		flow.SpecificationName = h.LookupSpecName(*flow.SpecificationId)
	}

	// v1.74.9: 查员工/部门/法人实体名字 (5min 缓存, 字典首次拉合思 ~800ms, 之后内存)
	if flow.OwnerId != nil {
		flow.OwnerName = h.LookupStaffName(*flow.OwnerId)
	}
	if flow.SubmitterId != nil {
		flow.SubmitterName = h.LookupStaffName(*flow.SubmitterId)
	}
	if flow.DepartmentId != nil {
		flow.DepartmentName = h.LookupDeptName(*flow.DepartmentId)
	}
	if flow.OwnerDepartment != nil {
		flow.OwnerDepartmentName = h.LookupDeptName(*flow.OwnerDepartment)
	}

	// v1.75.0: entityCheck 校验代码挪到 raw_json 解析后 (因为依赖 flow.LegalEntityName)
	// 见下方 "// 原始form JSON" 之后

	// 明细
	// v1.74.5: 加返 specificationId + rawJson, 让前端展开行显示合思 API 原始字段
	// (币种/自定义字段/差旅城市/补贴明细等 — 不同费用类型字段不一样)
	type DetailItem struct {
		DetailId        *string         `json:"detailId"`
		DetailNo        *int            `json:"detailNo"`
		FeeTypeId       *string         `json:"feeTypeId"`
		Amount          *float64        `json:"amount"`
		FeeDate         *int64          `json:"feeDate"`
		InvoiceCount    int             `json:"invoiceCount"`
		InvoiceStatus   string          `json:"invoiceStatus"`
		Reasons         *string         `json:"consumptionReasons"`
		SpecificationId *string         `json:"specificationId"`
		RawJson         json.RawMessage `json:"rawJson,omitempty"`
	}
	var details []DetailItem
	drows, err := h.DB.Query(`SELECT detail_id, detail_no, fee_type_id, amount, fee_date,
		invoice_count, invoice_status, consumption_reasons, specification_id,
		IFNULL(raw_json, '{}') AS raw_json
		FROM hesi_flow_detail WHERE flow_id=? ORDER BY detail_no`, flowId)
	if writeDatabaseError(w, err) {
		return
	}
	if drows != nil {
		defer drows.Close()
		for drows.Next() {
			var d DetailItem
			if writeDatabaseError(w, drows.Scan(&d.DetailId, &d.DetailNo, &d.FeeTypeId, &d.Amount, &d.FeeDate,
				&d.InvoiceCount, &d.InvoiceStatus, &d.Reasons, &d.SpecificationId, &d.RawJson)) {
				return
			}
			details = append(details, d)
		}
		if writeDatabaseError(w, drows.Err()) {
			return
		}
	}

	// 发票
	type InvoiceItem struct {
		InvoiceId     *string  `json:"invoiceId"`
		InvoiceNumber *string  `json:"invoiceNumber"`
		InvoiceCode   *string  `json:"invoiceCode"`
		InvoiceDate   *int64   `json:"invoiceDate"`
		InvoiceAmount *float64 `json:"invoiceAmount"`
		TotalAmount   *float64 `json:"totalAmount"`
		TaxAmount     *float64 `json:"taxAmount"`
		ApproveAmount *float64 `json:"approveAmount"`
		InvoiceStatus *string  `json:"invoiceStatus"`
		InvoiceType   *string  `json:"invoiceType"`
		BuyerName     *string  `json:"buyerName"`
		BuyerTaxNo    *string  `json:"buyerTaxNo"`
		SellerName    *string  `json:"sellerName"`
		SellerTaxNo   *string  `json:"sellerTaxNo"`
		IsVerified    *int     `json:"isVerified"`
	}
	var invoices []InvoiceItem
	irows, err := h.DB.Query(`SELECT invoice_id, invoice_number, invoice_code,
		invoice_date, invoice_amount, total_amount, tax_amount, approve_amount,
		invoice_status, invoice_type, buyer_name, buyer_tax_no, seller_name, seller_tax_no, is_verified
		FROM hesi_flow_invoice WHERE flow_id=?`, flowId)
	if writeDatabaseError(w, err) {
		return
	}
	if irows != nil {
		defer irows.Close()
		for irows.Next() {
			var inv InvoiceItem
			if writeDatabaseError(w, irows.Scan(&inv.InvoiceId, &inv.InvoiceNumber, &inv.InvoiceCode,
				&inv.InvoiceDate, &inv.InvoiceAmount, &inv.TotalAmount, &inv.TaxAmount, &inv.ApproveAmount,
				&inv.InvoiceStatus, &inv.InvoiceType, &inv.BuyerName, &inv.BuyerTaxNo, &inv.SellerName, &inv.SellerTaxNo, &inv.IsVerified)) {
				return
			}
			invoices = append(invoices, inv)
		}
		if writeDatabaseError(w, irows.Err()) {
			return
		}
	}

	// 附件元信息
	type AttachItem struct {
		AttachmentType string  `json:"attachmentType"`
		FileId         *string `json:"fileId"`
		FileName       *string `json:"fileName"`
		IsInvoice      int     `json:"isInvoice"`
		InvoiceNumber  *string `json:"invoiceNumber"`
		InvoiceCode    *string `json:"invoiceCode"`
	}
	var attachments []AttachItem
	arows, err := h.DB.Query("SELECT attachment_type, file_id, file_name, is_invoice, invoice_number, invoice_code FROM hesi_flow_attachment WHERE flow_id=?", flowId)
	if writeDatabaseError(w, err) {
		return
	}
	if arows != nil {
		defer arows.Close()
		for arows.Next() {
			var a AttachItem
			if writeDatabaseError(w, arows.Scan(&a.AttachmentType, &a.FileId, &a.FileName, &a.IsInvoice, &a.InvoiceNumber, &a.InvoiceCode)) {
				return
			}
			attachments = append(attachments, a)
		}
		if writeDatabaseError(w, arows.Err()) {
			return
		}
	}

	// 原始form JSON
	var formData interface{}
	if rawJSON != nil {
		if err := json.Unmarshal([]byte(*rawJSON), &formData); err != nil {
			formData = nil
		}
	}

	// v1.74.9: 从 raw_json 抽 "法人实体" 字段 + 查字典拿公司名
	// (合思的"法人实体"是自定义维度, 不是 corporation_id; 跑哥要看的"公司"按这个字段)
	if m, ok := formData.(map[string]interface{}); ok {
		if le, ok := m["法人实体"].(string); ok && le != "" {
			flow.LegalEntityId = le
			flow.LegalEntityName = h.LookupLegalEntityName(le)
		}
	}

	// v1.75.3: 主体校验扩展到所有 expense 类单据
	// v1.75.4: 付款单排除 (主体=费用归属公司 ≠ 申请人合同公司, 校验无意义)
	// 当前覆盖 3 个模板:
	//   - 日常报销单 (ID01Fk3qJYYFvp, 2744 单)
	//   - 费用核销申请单 (ID01Fk8AefXZzp, 3062 单)
	//   - 银行支付申请单 (ID01FhdI9II931, 1188 单)
	// 跳过模板 (主体语义不是申请人合同公司):
	//   - 付款单/票到付款 (ID01KgaO6dcZtR, 3762 单) — 主体=被付款方/费用归属
	skipEntityCheckSpecPrefixes := []string{
		"ID01KgaO6dcZtR", // 付款单（票到付款/票到核销）
	}
	shouldEntityCheck := flow.FormType == "expense" && flow.OwnerId != nil
	if shouldEntityCheck && flow.SpecificationId != nil {
		for _, p := range skipEntityCheckSpecPrefixes {
			if strings.HasPrefix(*flow.SpecificationId, p) {
				shouldEntityCheck = false
				break
			}
		}
	}
	if shouldEntityCheck {
		var expectedCompany sql.NullString
		var matchMethod sql.NullString
		queryErr := h.DB.QueryRow(
			`SELECT contract_company_name, match_method FROM hesi_employee_contract_company WHERE hesi_staff_id = ?`,
			*flow.OwnerId,
		).Scan(&expectedCompany, &matchMethod)
		switch {
		case queryErr == sql.ErrNoRows || !expectedCompany.Valid || expectedCompany.String == "":
			flow.EntityCheck = "no_data"
			if matchMethod.Valid && matchMethod.String == "none" {
				flow.EntityCheckReason = "该员工在钉钉花名册中未找到 (可能已离职或外部账号)"
			} else {
				flow.EntityCheckReason = "钉钉花名册的'合同信息→合同公司'字段未填, 请联系 HR 在钉钉智能人事补全"
			}
		case queryErr != nil:
			flow.EntityCheck = ""
		case flow.LegalEntityName == "":
			flow.EntityCheck = ""
		case flow.LegalEntityName == expectedCompany.String:
			flow.EntityCheck = "ok"
			flow.EntityCheckReason = "已核对: 跟钉钉花名册的合同公司一致"
		default:
			flow.EntityCheck = "mismatch"
			flow.EntityCheckExpected = expectedCompany.String
			flow.EntityCheckReason = "申请人填的'法人实体'跟钉钉花名册的'合同公司'不一致, 请核实主体选择"
		}
	}

	writeJSON(w, map[string]interface{}{
		"flow":        flow,
		"details":     details,
		"invoices":    invoices,
		"attachments": attachments,
		"formData":    formData,
	})
}

// GetHesiAttachmentURLs 实时获取附件下载URL
func (h *DashboardHandler) GetHesiAttachmentURLs(w http.ResponseWriter, r *http.Request) {
	flowId := r.URL.Query().Get("flowId")
	if flowId == "" {
		writeError(w, 400, "缺少flowId参数")
		return
	}

	token, err := h.getHesiToken()
	if err != nil {
		writeServerError(w, 500, "获取合思授权失败", err)
		return
	}

	body := map[string]interface{}{"flowIds": []string{flowId}}
	b, err := json.Marshal(body)
	if err != nil {
		writeError(w, 500, "获取附件失败: 请求构造失败")
		return
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/openapi/v1/flowDetails/attachment?accessToken=%s", hesiAPIBase, token), bytes.NewReader(b))
	if err != nil {
		writeError(w, 500, "获取附件失败: 请求构造失败")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hesiHTTP.Do(req)
	if err != nil {
		writeServerError(w, 500, "获取附件失败", err)
		return
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, 500, "获取附件失败: 读取响应失败")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeError(w, 502, fmt.Sprintf("获取附件失败: 上游状态码%d", resp.StatusCode))
		return
	}

	// 合思响应是 {items:[...], fromType:"..."} 直接结构, 这里平铺包成 BI 看板的
	// {code:200, items:..., fromType:...} 给前端 (前端校验 json.code===200 && json.items)
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		writeError(w, 500, "获取附件失败: 响应解析失败")
		return
	}
	payload["code"] = 200
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}

// GetHesiStats 获取合思数据统计概览
func (h *DashboardHandler) GetHesiStats(w http.ResponseWriter, r *http.Request) {
	var totalFlows, totalExpense, totalLoan, totalRequisition, totalCustom int
	var paidNoInvoice, approving, paying int
	var totalAttachments, totalInvoiceFiles int

	if writeDatabaseError(w, h.DB.QueryRow("SELECT COUNT(*) FROM hesi_flow WHERE active=1").Scan(&totalFlows)) {
		return
	}
	if writeDatabaseError(w, h.DB.QueryRow("SELECT COUNT(*) FROM hesi_flow WHERE active=1 AND form_type='expense'").Scan(&totalExpense)) {
		return
	}
	if writeDatabaseError(w, h.DB.QueryRow("SELECT COUNT(*) FROM hesi_flow WHERE active=1 AND form_type='loan'").Scan(&totalLoan)) {
		return
	}
	if writeDatabaseError(w, h.DB.QueryRow("SELECT COUNT(*) FROM hesi_flow WHERE active=1 AND form_type='requisition'").Scan(&totalRequisition)) {
		return
	}
	if writeDatabaseError(w, h.DB.QueryRow("SELECT COUNT(*) FROM hesi_flow WHERE active=1 AND form_type='custom'").Scan(&totalCustom)) {
		return
	}
	// v1.70.5: 改为查合思借款包待还款数 (跟合思后台对齐)
	// 旧 SQL 按 invoice_status='noExist' 错把内部往来/差旅核销等无票流程都算进来 (1742 笔), 跟合思后台 219 差太多
	// 预付款核销机制: 出纳付款 → 生成借款包(loanInfo) → 报销单关联核销借款包 → 借款包 state PAID → 单据 archived
	// 借款包 state='REPAID' = 待还款 (含员工借款+预付款), 跟合思后台数字一致
	if writeDatabaseError(w, h.DB.QueryRow(`SELECT COUNT(*) FROM hesi_loan_info WHERE active=1 AND state='REPAID'`).Scan(&paidNoInvoice)) {
		return
	}
	if writeDatabaseError(w, h.DB.QueryRow("SELECT COUNT(*) FROM hesi_flow WHERE active=1 AND state='approving'").Scan(&approving)) {
		return
	}
	if writeDatabaseError(w, h.DB.QueryRow("SELECT COUNT(*) FROM hesi_flow WHERE active=1 AND state='paying'").Scan(&paying)) {
		return
	}
	if writeDatabaseError(w, h.DB.QueryRow(`SELECT COUNT(*) FROM hesi_flow_attachment a
		JOIN hesi_flow f ON f.flow_id = a.flow_id
		WHERE f.active=1`).Scan(&totalAttachments)) {
		return
	}
	if writeDatabaseError(w, h.DB.QueryRow(`SELECT COUNT(*) FROM hesi_flow_attachment a
		JOIN hesi_flow f ON f.flow_id = a.flow_id
		WHERE f.active=1 AND a.is_invoice=1`).Scan(&totalInvoiceFiles)) {
		return
	}

	// 按状态分布
	type StateCount struct {
		State string `json:"state"`
		Count int    `json:"count"`
	}
	var stateDistribution []StateCount
	srows, err := h.DB.Query("SELECT state, COUNT(*) as cnt FROM hesi_flow WHERE active=1 GROUP BY state ORDER BY cnt DESC")
	if writeDatabaseError(w, err) {
		return
	}
	if srows != nil {
		defer srows.Close()
		for srows.Next() {
			var sc StateCount
			if writeDatabaseError(w, srows.Scan(&sc.State, &sc.Count)) {
				return
			}
			stateDistribution = append(stateDistribution, sc)
		}
		if writeDatabaseError(w, srows.Err()) {
			return
		}
	}

	// 按类型分布
	type TypeCount struct {
		FormType string `json:"formType"`
		Count    int    `json:"count"`
	}
	var typeDistribution []TypeCount
	trows, err := h.DB.Query("SELECT form_type, COUNT(*) as cnt FROM hesi_flow WHERE active=1 GROUP BY form_type ORDER BY cnt DESC")
	if writeDatabaseError(w, err) {
		return
	}
	if trows != nil {
		defer trows.Close()
		for trows.Next() {
			var tc TypeCount
			if writeDatabaseError(w, trows.Scan(&tc.FormType, &tc.Count)) {
				return
			}
			typeDistribution = append(typeDistribution, tc)
		}
		if writeDatabaseError(w, trows.Err()) {
			return
		}
	}

	// 近30天每天的单据数
	type DailyCount struct {
		Date  string `json:"date"`
		Count int    `json:"count"`
	}
	var dailyTrend []DailyCount
	drows, err := h.DB.Query(`SELECT DATE(FROM_UNIXTIME(create_time/1000)) as dt, COUNT(*) as cnt
		FROM hesi_flow WHERE active=1 AND create_time >= ?
		GROUP BY dt ORDER BY dt`,
		time.Now().AddDate(0, 0, -30).UnixMilli())
	if writeDatabaseError(w, err) {
		return
	}
	if drows != nil {
		defer drows.Close()
		for drows.Next() {
			var dc DailyCount
			if writeDatabaseError(w, drows.Scan(&dc.Date, &dc.Count)) {
				return
			}
			dailyTrend = append(dailyTrend, dc)
		}
		if writeDatabaseError(w, drows.Err()) {
			return
		}
	}

	writeJSON(w, map[string]interface{}{
		"totalFlows":        totalFlows,
		"totalExpense":      totalExpense,
		"totalLoan":         totalLoan,
		"totalRequisition":  totalRequisition,
		"totalCustom":       totalCustom,
		"paidNoInvoice":     paidNoInvoice,
		"approving":         approving,
		"paying":            paying,
		"totalAttachments":  totalAttachments,
		"totalInvoiceFiles": totalInvoiceFiles,
		"stateDistribution": stateDistribution,
		"typeDistribution":  typeDistribution,
		"dailyTrend":        dailyTrend,
	})
}
