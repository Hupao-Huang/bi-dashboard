package handler

import (
	"bytes"
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
	fromClause := "hesi_flow f"
	selectFields := "DISTINCT f.flow_id, f.code, f.title, f.form_type, f.state, f.owner_id, f.department_id, f.submitter_id, f.pay_money, f.expense_money, f.loan_money, f.create_time, f.update_time, f.submit_date, f.pay_date, f.flow_end_time, f.voucher_no, f.voucher_status, JSON_UNQUOTE(JSON_EXTRACT(f.raw_json, '$.preApprovedNodeName')) AS pre_approved_node, JSON_UNQUOTE(JSON_EXTRACT(f.raw_json, '$.preNodeApprovedTime')) AS pre_approved_time, f.current_stage_name, f.current_approver_name, f.current_approver_code, f.specification_id"

	if hasInvoiceFilter {
		fromClause += " JOIN hesi_flow_detail d ON f.flow_id = d.flow_id"
		where += " AND d.invoice_status=?"
		args = append(args, invoiceStatus)
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
		VoucherNo         *string `json:"voucherNo"`
		VoucherStatus     *string `json:"voucherStatus"`
		SpecificationId   *string `json:"specificationId"`
		SpecificationName string  `json:"specificationName"`
	}
	err := h.DB.QueryRow(`SELECT flow_id, code, title, form_type, state, owner_id, department_id, submitter_id,
		pay_money, expense_money, loan_money, create_time, update_time, submit_date, pay_date, flow_end_time,
		voucher_no, voucher_status, specification_id, raw_json FROM hesi_flow WHERE flow_id=?`, flowId).Scan(
		&flow.FlowId, &flow.Code, &flow.Title, &flow.FormType, &flow.State,
		&flow.OwnerId, &flow.DepartmentId, &flow.SubmitterId,
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

	// 明细
	type DetailItem struct {
		DetailId      *string  `json:"detailId"`
		DetailNo      *int     `json:"detailNo"`
		FeeTypeId     *string  `json:"feeTypeId"`
		Amount        *float64 `json:"amount"`
		FeeDate       *int64   `json:"feeDate"`
		InvoiceCount  int      `json:"invoiceCount"`
		InvoiceStatus string   `json:"invoiceStatus"`
		Reasons       *string  `json:"consumptionReasons"`
	}
	var details []DetailItem
	drows, err := h.DB.Query("SELECT detail_id, detail_no, fee_type_id, amount, fee_date, invoice_count, invoice_status, consumption_reasons FROM hesi_flow_detail WHERE flow_id=? ORDER BY detail_no", flowId)
	if writeDatabaseError(w, err) {
		return
	}
	if drows != nil {
		defer drows.Close()
		for drows.Next() {
			var d DetailItem
			if writeDatabaseError(w, drows.Scan(&d.DetailId, &d.DetailNo, &d.FeeTypeId, &d.Amount, &d.FeeDate, &d.InvoiceCount, &d.InvoiceStatus, &d.Reasons)) {
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(data)
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
	if writeDatabaseError(w, h.DB.QueryRow(`SELECT COUNT(DISTINCT f.flow_id) FROM hesi_flow f
		JOIN hesi_flow_detail d ON f.flow_id=d.flow_id
		WHERE f.active=1 AND f.state='paid' AND d.invoice_status='noExist'`).Scan(&paidNoInvoice)) {
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
