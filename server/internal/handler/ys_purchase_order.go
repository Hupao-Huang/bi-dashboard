// ys_purchase_order.go 工具箱"新增采购订单": 导入 Excel → 翻译编码 → 算价 → 组装 → 建用友采购订单。
// 两步: ①Preview 上传解析+翻译+算价+预览(不建单, 存 token) ②Commit 确认建单(防重流水+逐单成败)。
// 纯算价/组装逻辑在 internal/yonsuite/purchase_build.go; 防重复用 yonbip_idemp.go 的 yonbip_submit_log。
// 写用友=不可逆。
package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"bi-dashboard/internal/yonsuite"

	"github.com/xuri/excelize/v2"
)

// ---- 两步之间的预览缓存 (token → 已翻译组装好的订单, TTL 30min) ----

type poPreviewOrder struct {
	OrgCode    string                 `json:"orgCode"`
	OrgName    string                 `json:"orgName"`
	VendorCode string                 `json:"vendorCode"`
	VendorName string                 `json:"vendorName"`
	VouchDate  string                 `json:"vouchDate"`
	Lines      []yonsuite.POLineInput `json:"-"` // 建单用, 不回前端
	LineCount  int                    `json:"lineCount"`
	TotalSum   float64                `json:"totalSum"` // 含税合计
	HasProblem bool                   `json:"hasProblem"`
}

type poPreview struct {
	orders    []poPreviewOrder
	createdAt time.Time
}

var (
	poPreviewMu    sync.Mutex
	poPreviewStore = map[string]*poPreview{}
)

const poPreviewTTL = 30 * time.Minute

func poNewToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func poGCPreview() {
	for k, v := range poPreviewStore {
		if time.Since(v.createdAt) > poPreviewTTL {
			delete(poPreviewStore, k)
		}
	}
}

// ---- Excel 模板解析 ----

type poTmplRow struct {
	OrgName, VouchDate, VendorName              string
	ProductCode, ProductName                    string
	Qty, TaxInclPrice, TaxInclAmount, TaxRate   string
	ArriveDt                                    string
	rowNo                                       int
}

func poParseTemplate(f *excelize.File) ([]poTmplRow, error) {
	sheet := f.GetSheetName(0)
	allRows, err := f.GetRows(sheet)
	if err != nil {
		return nil, err
	}
	if len(allRows) < 2 {
		return nil, fmt.Errorf("Excel 没有数据行")
	}
	col := map[string]int{}
	for i, h := range allRows[0] {
		col[strings.TrimSpace(h)] = i
	}
	get := func(r []string, name string) string {
		if i, ok := col[name]; ok && i < len(r) {
			return strings.TrimSpace(r[i])
		}
		return ""
	}
	var out []poTmplRow
	for ri := 1; ri < len(allRows); ri++ {
		r := allRows[ri]
		if get(r, "物料编码") == "" && get(r, "供货供应商") == "" {
			continue
		}
		out = append(out, poTmplRow{
			OrgName:       get(r, "采购组织"),
			VouchDate:     get(r, "单据日期"),
			VendorName:    get(r, "供货供应商"),
			ProductCode:   get(r, "物料编码"),
			ProductName:   get(r, "物料名称"),
			Qty:           get(r, "采购数量"),
			TaxInclPrice:  get(r, "含税单价"),
			TaxInclAmount: get(r, "含税金额"),
			TaxRate:       get(r, "税率"),
			ArriveDt:      get(r, "计划到货日期"),
			rowNo:         ri + 1,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("Excel 没有有效明细行")
	}
	return out, nil
}

func poAtof(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

// ---- 预览行(回前端展示) ----

type poPreviewRowDTO struct {
	RowNo        int      `json:"rowNo"`
	OrderIndex   int      `json:"orderIndex"` // 属于第几张订单(前端点订单筛明细用)
	OrgName      string   `json:"orgName"`
	OrgCode      string   `json:"orgCode"`
	VendorName   string   `json:"vendorName"`
	VendorCode   string   `json:"vendorCode"`
	ProductCode  string   `json:"productCode"`
	ProductName  string   `json:"productName"`
	UnitCode     string   `json:"unitCode"`
	TaxitemsCode string   `json:"taxitemsCode"`
	Qty          float64  `json:"qty"`
	TaxInclPrice float64  `json:"taxInclPrice"`
	OriSum       float64  `json:"oriSum"`
	OriMoney     float64  `json:"oriMoney"`
	OriTax       float64  `json:"oriTax"`
	ArriveDate   string   `json:"arriveDate"`
	Problems     []string `json:"problems"`
}

// YonbipPOPreview 上传 Excel → 翻译+算价+组装 → 预览(不建单)。
func (h *DashboardHandler) YonbipPOPreview(w http.ResponseWriter, r *http.Request) {
	if h.YS == nil {
		writeError(w, 503, "用友未配置")
		return
	}
	if r.Method != "POST" {
		writeError(w, 405, "method not allowed")
		return
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, 400, "上传解析失败")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, 400, "请上传 Excel 文件(字段名 file)")
		return
	}
	defer file.Close()

	xf, err := excelize.OpenReader(file)
	if err != nil {
		writeError(w, 400, "不是有效的 Excel 文件")
		return
	}
	defer xf.Close()

	rows, err := poParseTemplate(xf)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	// 翻译组织
	orgs, err := h.YS.QueryPurchaseOrgs()
	if err != nil {
		writeServerError(w, 500, "查询用友组织失败", err)
		return
	}
	orgCodeByName := map[string]string{}
	for _, o := range orgs {
		orgCodeByName[o.Name] = o.Code
	}

	// 翻译物料(按组织分桶批量查)
	codesByOrg := map[string]map[string]bool{}
	for _, r := range rows {
		oc := orgCodeByName[r.OrgName]
		if oc == "" {
			continue
		}
		if codesByOrg[oc] == nil {
			codesByOrg[oc] = map[string]bool{}
		}
		codesByOrg[oc][r.ProductCode] = true
	}
	prodByOrg := map[string]map[string]*yonsuite.ProductDetail{}
	for oc, set := range codesByOrg {
		var codes []string
		for c := range set {
			codes = append(codes, c)
		}
		m, err := h.YS.QueryProductDetails(oc, codes)
		if err != nil {
			writeServerError(w, 500, "查询用友物料详情失败", err)
			return
		}
		prodByOrg[oc] = m
	}

	// 翻译供应商(本地字典)
	vendorCodeByName := map[string]string{}
	for _, r := range rows {
		if _, ok := vendorCodeByName[r.VendorName]; ok {
			continue
		}
		vendorCodeByName[r.VendorName] = h.poVendorCode(r.VendorName)
	}

	// 逐行翻译+算价+查错
	type rowCalc struct {
		row      poTmplRow
		dto      poPreviewRowDTO
		line     yonsuite.POLineInput
		problems []string
	}
	var calcs []rowCalc
	previewRows := make([]poPreviewRowDTO, 0, len(rows))
	for _, r := range rows {
		orgCode := orgCodeByName[r.OrgName]
		vendorCode := vendorCodeByName[r.VendorName]
		var prod *yonsuite.ProductDetail
		if orgCode != "" {
			prod = prodByOrg[orgCode][r.ProductCode]
		}
		vouchDate := yonsuite.NormDate(r.VouchDate)
		qty := poAtof(r.Qty)
		taxPrice := poAtof(r.TaxInclPrice)
		prices := yonsuite.ComputeLinePrices(taxPrice, qty, poAtof(r.TaxRate))

		var probs []string
		if orgCode == "" {
			probs = append(probs, "组织查不到编码")
		}
		if vendorCode == "" {
			probs = append(probs, "供应商查不到编码")
		}
		if prod == nil {
			probs = append(probs, "物料查不到(单位/税目)")
		}
		if vouchDate == "" {
			probs = append(probs, "单据日期非法")
		}
		if qty <= 0 {
			probs = append(probs, "采购数量≤0")
		}
		if taxPrice < 0 {
			probs = append(probs, "含税单价为负")
		}
		if amt := poAtof(r.TaxInclAmount); amt != 0 && absFloat(amt-prices.OriSum) > 0.01 {
			probs = append(probs, fmt.Sprintf("含税金额%.2f≠单价×数量%.2f", amt, prices.OriSum))
		}

		unit, taxitems := "", ""
		if prod != nil {
			unit, taxitems = prod.PurUOMCode, prod.TaxitemsCode
		}
		dto := poPreviewRowDTO{
			RowNo: r.rowNo, OrgName: r.OrgName, OrgCode: orgCode,
			VendorName: r.VendorName, VendorCode: vendorCode,
			ProductCode: r.ProductCode, ProductName: r.ProductName,
			UnitCode: unit, TaxitemsCode: taxitems, Qty: qty, TaxInclPrice: taxPrice,
			OriSum: prices.OriSum, OriMoney: prices.OriMoney, OriTax: prices.OriTax,
			ArriveDate: yonsuite.NormDate(r.ArriveDt), Problems: probs,
		}
		previewRows = append(previewRows, dto)

		var line yonsuite.POLineInput
		if prod != nil {
			line = yonsuite.POLineInput{
				ProductCode: r.ProductCode, UnitCode: prod.UnitCode, PurUOMCode: prod.PurUOMCode,
				PriceUOMCode: prod.PriceUOMCode, TaxitemsCode: prod.TaxitemsCode,
				Qty: qty, TaxInclUnitPrice: taxPrice, TaxRatePct: poAtof(r.TaxRate),
				ArriveDate: yonsuite.NormDate(r.ArriveDt),
			}
		}
		calcs = append(calcs, rowCalc{row: r, dto: dto, line: line, problems: probs})
	}

	// 按 组织+供应商+日期 合并成订单
	type gk struct{ org, vendor, date string }
	idx := map[gk]int{}
	var orders []poPreviewOrder
	for ci, c := range calcs {
		vouchDate := yonsuite.NormDate(c.row.VouchDate)
		k := gk{orgCodeByName[c.row.OrgName], vendorCodeByName[c.row.VendorName], vouchDate}
		i, ok := idx[k]
		if !ok {
			i = len(orders)
			idx[k] = i
			orders = append(orders, poPreviewOrder{
				OrgCode: k.org, OrgName: c.row.OrgName,
				VendorCode: k.vendor, VendorName: c.row.VendorName, VouchDate: k.date,
			})
		}
		previewRows[ci].OrderIndex = i // 回填: 该明细行属于第 i 张订单
		orders[i].Lines = append(orders[i].Lines, c.line)
		orders[i].LineCount++
		orders[i].TotalSum = roundMoney(orders[i].TotalSum + c.dto.OriSum)
		if len(c.problems) > 0 {
			orders[i].HasProblem = true
		}
	}

	token := poNewToken()
	poPreviewMu.Lock()
	poGCPreview()
	poPreviewStore[token] = &poPreview{orders: orders, createdAt: time.Now()}
	poPreviewMu.Unlock()

	writeJSON(w, map[string]interface{}{
		"token":  token,
		"rows":   previewRows,
		"orders": orders,
	})
}

// poVendorCode 本地字典查供应商编码; 区分"查无此家"与 DB 异常(异常返回空但不当查无)。
func (h *DashboardHandler) poVendorCode(name string) string {
	if h.DB == nil || name == "" {
		return ""
	}
	var code string
	err := h.DB.QueryRow("SELECT code FROM ys_vendor_dict WHERE name = ? LIMIT 1", name).Scan(&code)
	if err != nil {
		return "" // 查无 / DB 异常都返回空; 预览会标红, 不会误建
	}
	return code
}

// poCommitResult 单张订单建单结果。
type poCommitResult struct {
	OrgCode    string `json:"orgCode"`
	VendorName string `json:"vendorName"`
	VouchDate  string `json:"vouchDate"`
	LineCount  int    `json:"lineCount"`
	OK         bool   `json:"ok"`
	Skipped    bool   `json:"skipped"` // 防重跳过
	OrderID    string `json:"orderId"`
	Error      string `json:"error,omitempty"`
}

// YonbipPOCommit 按 token 取预览, 逐单建用友采购订单(防重+逐单成败+客户端断开即停)。
func (h *DashboardHandler) YonbipPOCommit(w http.ResponseWriter, r *http.Request) {
	if h.YS == nil {
		writeError(w, 503, "用友未配置")
		return
	}
	if r.Method != "POST" {
		writeError(w, 405, "method not allowed")
		return
	}
	var req struct {
		Token string `json:"token"`
		Force bool   `json:"force"` // 强制重发: 跳过防重(前端勾确认才带)
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		writeError(w, 400, "缺少 token")
		return
	}

	poPreviewMu.Lock()
	pv := poPreviewStore[req.Token]
	poPreviewMu.Unlock()
	if pv == nil {
		writeError(w, 400, "预览已过期, 请重新上传")
		return
	}

	h.ybEnsureSubmitLog()
	ctx := r.Context()

	results := make([]poCommitResult, 0, len(pv.orders))
	for _, o := range pv.orders {
		res := poCommitResult{
			OrgCode: o.OrgCode, VendorName: o.VendorName, VouchDate: o.VouchDate, LineCount: o.LineCount,
		}
		if ctx.Err() != nil { // 客户端断开 → 停手, 不再往用友写
			res.Error = "已取消(连接断开)"
			results = append(results, res)
			continue
		}
		if o.HasProblem {
			res.Error = "该订单有未解决的问题, 已跳过"
			results = append(results, res)
			continue
		}

		payload := yonsuite.BuildPurchaseOrderPayload(
			yonsuite.POHeaderInput{OrgCode: o.OrgCode, VendorCode: o.VendorCode, VouchDate: o.VouchDate},
			o.Lines,
		)
		// 防重指纹 = 订单内容(与 resubmitCheckKey 同源, 但本地先挡一道)
		fpParts := []string{"po", o.OrgCode, o.VendorCode, o.VouchDate}
		for _, ln := range o.Lines {
			fpParts = append(fpParts, ln.ProductCode,
				strconv.FormatFloat(ln.Qty, 'f', -1, 64),
				strconv.FormatFloat(ln.TaxInclUnitPrice, 'f', -1, 64), ln.ArriveDate)
		}
		fp := ybFingerprint(fpParts...)
		if !req.Force {
			if dup, prevDoc := h.ybRecentSubmit(fp); dup {
				res.Skipped = true
				res.OrderID = prevDoc
				res.Error = "10分钟内已提交过, 已自动跳过防止重复建单"
				results = append(results, res)
				continue
			}
		}

		id, _, err := h.YS.SavePurchaseOrder(payload)
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		// 建成(不可逆) → 立即落防重流水
		h.ybRecordSubmit(fp, "po", id, o.VouchDate)
		res.OK = true
		res.OrderID = id
		results = append(results, res)
	}

	// 用过即删, 防止重复点确认
	poPreviewMu.Lock()
	delete(poPreviewStore, req.Token)
	poPreviewMu.Unlock()

	writeJSON(w, map[string]interface{}{"results": results})
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func roundMoney(x float64) float64 {
	return float64(int64(x*100+0.5)) / 100
}
