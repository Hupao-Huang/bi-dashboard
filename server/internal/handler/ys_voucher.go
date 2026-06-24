// ys_voucher.go 凭证查询 (财务, 仅超管) — 实时透传用友 YS 凭证, 不入库
// 账簿下拉 + 按账期/凭证号/状态查凭证头, 点开看分录明细 (借贷)
package handler

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"bi-dashboard/internal/yonsuite"
)

// 账簿清单缓存 (账簿很少变, 缓存 24h, 省掉每次开页都打用友)
var (
	accbookCacheMu   sync.Mutex
	accbookCache     []yonsuite.Accbook
	accbookCacheTime time.Time
)

// GetVoucherAccbooks 账簿下拉数据 (缓存 24h)
func (h *DashboardHandler) GetVoucherAccbooks(w http.ResponseWriter, r *http.Request) {
	if h.YS == nil {
		writeError(w, 503, "用友 YS 未配置")
		return
	}

	accbookCacheMu.Lock()
	if accbookCache != nil && time.Since(accbookCacheTime) < 24*time.Hour {
		cached := accbookCache
		accbookCacheMu.Unlock()
		writeJSON(w, cached)
		return
	}
	accbookCacheMu.Unlock()

	list, err := h.YS.QueryAccbookList()
	if err != nil {
		writeServerError(w, 500, "查询账簿失败", err)
		return
	}

	accbookCacheMu.Lock()
	accbookCache = list
	accbookCacheTime = time.Now()
	accbookCacheMu.Unlock()

	writeJSON(w, list)
}

// voucherLine 凭证分录 (抽平后给前端)
type voucherLine struct {
	RecordNumber string  `json:"recordNumber"` // 行号
	Description  string  `json:"description"`  // 摘要
	SubjectCode  string  `json:"subjectCode"`  // 科目编码
	SubjectName  string  `json:"subjectName"`  // 科目名称
	Auxiliary    string  `json:"auxiliary"`    // 辅助核算 (供应商/客户等)
	Debit        float64 `json:"debit"`        // 借方金额 (原币)
	Credit       float64 `json:"credit"`       // 贷方金额 (原币)
}

// voucherRow 凭证 (头 + 分录, 抽平后给前端)
type voucherRow struct {
	AccbookCode string        `json:"accbookCode"` // 账簿编码 (多账簿合并用)
	AccbookName string        `json:"accbookName"` // 账簿名称
	ID          string        `json:"id"`
	Period      string        `json:"period"`      // 账期 yyyy-MM
	VoucherNo   string        `json:"voucherNo"`   // 凭证字号 如 "转-1"
	VoucherType string        `json:"voucherType"` // 凭证类型 转账凭证/收款凭证...
	Description string        `json:"description"` // 摘要 (头空则取第一条分录)
	TotalDebit  float64       `json:"totalDebit"`  // 借方合计
	TotalCredit float64       `json:"totalCredit"` // 贷方合计
	SrcSystem   string        `json:"srcSystem"`   // 来源系统 应付管理...
	Maker       string        `json:"maker"`       // 制单人
	Auditor     string        `json:"auditor"`     // 审核人
	Tallyman    string        `json:"tallyman"`    // 记账人
	Status      string        `json:"status"`      // 状态 (中文)
	MakeTime    string        `json:"makeTime"`    // 制单日期
	Attached    string        `json:"attached"`    // 附单据数
	Lines       []voucherLine `json:"lines"`       // 分录明细
}

// flattenVoucherRecords 把用友 recordList 抽平成 voucherRow, 每行打上账簿标记
func flattenVoucherRecords(recordList []map[string]interface{}, accbookCode, accbookName string) []voucherRow {
	rows := make([]voucherRow, 0, len(recordList))
	for _, rec := range recordList {
		header := mapObj(rec, "header")
		row := voucherRow{
			AccbookCode: accbookCode,
			AccbookName: accbookName,
			ID:          ysMapStr(header, "id"),
			Period:      ysMapStr(header, "period"),
			VoucherNo:   ysMapStr(header, "displayname"),
			VoucherType: ysMapStr(mapObj(header, "vouchertype"), "name"),
			Description: ysMapStr(header, "description"),
			TotalDebit:  ysMapFloat(header, "totaldebit_org"),
			TotalCredit: ysMapFloat(header, "totalcredit_org"),
			SrcSystem:   ysMapStr(header, "srcsystem"),
			Maker:       ysMapStr(mapObj(header, "maker"), "name"),
			Auditor:     ysMapStr(mapObj(header, "auditor"), "name"),
			Tallyman:    ysMapStr(mapObj(header, "tallyman"), "name"),
			Status:      voucherStatusText(ysMapStr(header, "voucherstatus")),
			MakeTime:    ysMapStr(header, "maketime"),
			Attached:    ysMapStr(header, "attachedbill"),
		}
		if bodyArr, ok := rec["body"].([]interface{}); ok {
			for _, bi := range bodyArr {
				lm, ok := bi.(map[string]interface{})
				if !ok {
					continue
				}
				row.Lines = append(row.Lines, voucherLine{
					RecordNumber: ysMapStr(lm, "recordnumber"),
					Description:  ysMapStr(lm, "description"),
					SubjectCode:  ysMapStr(mapObj(lm, "accsubject"), "code"),
					SubjectName:  ysMapStr(mapObj(lm, "accsubject"), "name"),
					Auxiliary:    ysMapStr(lm, "auxiliaryShow"),
					Debit:        ysMapFloat(lm, "debit_original"),
					Credit:       ysMapFloat(lm, "credit_original"),
				})
			}
		}
		// 凭证头摘要为空时, 用第一条分录摘要兜底
		if row.Description == "" && len(row.Lines) > 0 {
			row.Description = row.Lines[0].Description
		}
		rows = append(rows, row)
	}
	return rows
}

// GetVoucherList 实时查用友凭证 (POST)
func (h *DashboardHandler) GetVoucherList(w http.ResponseWriter, r *http.Request) {
	if h.YS == nil {
		writeError(w, 503, "用友 YS 未配置")
		return
	}
	if r.Method != "POST" {
		writeError(w, 405, "method not allowed")
		return
	}

	var body struct {
		AccbookCode   string `json:"accbookCode"`
		PeriodStart   string `json:"periodStart"`
		PeriodEnd     string `json:"periodEnd"`
		VoucherStatus string `json:"voucherStatus"` // "" 全部 / "01" 保存 / "04" 已记账
		BillcodeMin   int    `json:"billcodeMin"`
		BillcodeMax   int    `json:"billcodeMax"`
		PageIndex     int    `json:"pageIndex"`
		PageSize      int    `json:"pageSize"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, 400, "请求参数解析失败")
		return
	}
	if body.AccbookCode == "" {
		writeError(w, 400, "请选择账簿")
		return
	}
	if body.PageIndex <= 0 {
		body.PageIndex = 1
	}
	if body.PageSize <= 0 || body.PageSize > 200 {
		body.PageSize = 20
	}

	req := &yonsuite.VoucherListReq{
		AccbookCode: body.AccbookCode,
		PeriodStart: body.PeriodStart,
		PeriodEnd:   body.PeriodEnd,
		BillcodeMin: body.BillcodeMin,
		BillcodeMax: body.BillcodeMax,
	}
	req.Pager.PageIndex = body.PageIndex
	req.Pager.PageSize = body.PageSize
	if body.VoucherStatus != "" {
		req.VoucherStatusList = []string{body.VoucherStatus}
	}

	resp, err := h.YS.QueryVoucherList(req)
	if err != nil {
		writeServerError(w, 500, "查询凭证失败", err)
		return
	}

	rows := flattenVoucherRecords(resp.Data.RecordList, body.AccbookCode, "")

	writeJSON(w, map[string]interface{}{
		"list":        rows,
		"recordCount": resp.Data.RecordCount,
		"pageIndex":   resp.Data.PageIndex,
		"pageSize":    resp.Data.PageSize,
	})
}

// voucherStatusText 凭证状态码 → 中文 (01 保存 / 04 已记账, 其余原样返码)
func voucherStatusText(code string) string {
	switch code {
	case "01":
		return "保存"
	case "04":
		return "已记账"
	case "":
		return ""
	default:
		return code
	}
}

// --- 安全取值 helper (recordList 用 UseNumber 解析, 数字是 json.Number) ---

func mapObj(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return nil
	}
	if v, ok := m[key].(map[string]interface{}); ok {
		return v
	}
	return nil
}

// ysMapStr 取 m[key] 并统一成 string。读 nil map 得零值 nil, JSONString(nil)="" — 无需单独判空。
// 类型转换 (json.Number / string / nil → string) 与 yonsuite.JSONString 共用一处, 单一事实来源。
func ysMapStr(m map[string]interface{}, key string) string {
	return yonsuite.JSONString(m[key])
}

func ysMapFloat(m map[string]interface{}, key string) float64 {
	if m == nil {
		return 0
	}
	switch t := m[key].(type) {
	case json.Number:
		f, _ := t.Float64()
		return f
	case float64:
		return t
	default:
		return 0
	}
}
