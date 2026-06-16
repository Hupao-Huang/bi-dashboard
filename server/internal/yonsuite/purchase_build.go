package yonsuite

// 采购订单"组装"纯逻辑(无 I/O): 算价 / 报文拼装 / 幂等键 / 日期归一。
// handler 和 CLI 共用同一份, 避免不可逆财务单据逻辑在两处分叉(二审 altitude 项)。

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strconv"
	"strings"
	"time"
)

// roundN 四舍五入到 n 位小数。
func roundN(x float64, n int) float64 {
	p := math.Pow10(n)
	return math.Round(x*p) / p
}

// POHeaderInput 采购订单表头输入(已翻译成编码)。
type POHeaderInput struct {
	OrgCode    string
	VendorCode string
	VouchDate  string // 已归一为 yyyy-MM-dd HH:mm:ss
}

// POLineInput 采购订单明细行输入(已翻译成编码)。
type POLineInput struct {
	ProductCode      string
	UnitCode         string // 主计量编码
	PurUOMCode       string // 采购单位编码
	PriceUOMCode     string // 计价单位编码
	TaxitemsCode     string // 税目编码
	Qty              float64
	TaxInclUnitPrice float64 // 含税单价
	TaxRatePct       float64 // 税率(百分数, 如 13)
}

// LinePrices 一行的价税分解(全部以"含税金额"为锚点保持自洽)。
type LinePrices struct {
	OriSum          float64 // 含税金额
	OriMoney        float64 // 无税金额
	OriTax          float64 // 税额
	OriUnitPrice    float64 // 无税单价(高精度, 保证 ×数量≈无税金额)
	OriTaxUnitPrice float64 // 含税单价
}

// ComputeLinePrices 算价。含税金额=含税单价×数量(锚点); 无税=含税/(1+税率); 税额=含税-无税。
// 无税单价 = 无税金额/数量, 保留 8 位(不舍到 2 位), 否则 单价×数量≠金额 用友拒单(二审 #1)。
func ComputeLinePrices(taxInclUnitPrice, qty, taxRatePct float64) LinePrices {
	r := taxRatePct / 100.0
	oriSum := roundN(taxInclUnitPrice*qty, 2)
	oriMoney := oriSum
	if r > 0 {
		oriMoney = roundN(oriSum/(1+r), 2)
	}
	oriTax := roundN(oriSum-oriMoney, 2)
	oriUnitPrice := 0.0
	if qty != 0 {
		oriUnitPrice = roundN(oriMoney/qty, 8)
	}
	return LinePrices{
		OriSum:          oriSum,
		OriMoney:        oriMoney,
		OriTax:          oriTax,
		OriUnitPrice:    oriUnitPrice,
		OriTaxUnitPrice: taxInclUnitPrice,
	}
}

// BuildPurchaseOrderPayload 把表头+明细组装成用友 singleSave_v1 报文 {"data":{...}}。
// 单位相同(采购=计价=主计量)情形下换算率=1、转换方式=固定; 本币=原币(CNY)。
// resubmitCheckKey 由内容算出(同内容同 key, 用友侧挡重复)。
func BuildPurchaseOrderPayload(h POHeaderInput, lines []POLineInput) map[string]interface{} {
	poLines := make([]map[string]interface{}, 0, len(lines))
	keyParts := []string{h.OrgCode, h.VendorCode, h.VouchDate}
	for i, ln := range lines {
		p := ComputeLinePrices(ln.TaxInclUnitPrice, ln.Qty, ln.TaxRatePct)
		keyParts = append(keyParts,
			ln.ProductCode,
			strconv.FormatFloat(ln.Qty, 'f', -1, 64),
			strconv.FormatFloat(ln.TaxInclUnitPrice, 'f', -1, 64),
		)
		poLines = append(poLines, map[string]interface{}{
			"_status":               "Insert",
			"rowno":                 strconv.Itoa((i + 1) * 10),
			"inOrg_code":            h.OrgCode,
			"inInvoiceOrg_code":     h.OrgCode,
			"product_cCode":         ln.ProductCode,
			"unit_code":             ln.UnitCode,
			"purUOM_Code":           ln.PurUOMCode,
			"priceUOM_Code":         ln.PriceUOMCode,
			"taxitems_code":         ln.TaxitemsCode,
			"qty":                   ln.Qty,
			"subQty":                ln.Qty,
			"priceQty":              ln.Qty,
			"invExchRate":           1,
			"invPriceExchRate":      1,
			"unitExchangeType":      0,
			"unitExchangeTypePrice": 0,
			"oriTaxUnitPrice":       p.OriTaxUnitPrice,
			"oriUnitPrice":          p.OriUnitPrice,
			"oriSum":                p.OriSum,
			"oriMoney":              p.OriMoney,
			"oriTax":                p.OriTax,
			"natTaxUnitPrice":       p.OriTaxUnitPrice,
			"natUnitPrice":          p.OriUnitPrice,
			"natSum":                p.OriSum,
			"natMoney":              p.OriMoney,
			"natTax":                p.OriTax,
			"isGiftProduct":         false,
		})
	}
	return map[string]interface{}{
		"data": map[string]interface{}{
			"_status":            "Insert",
			"resubmitCheckKey":   ResubmitKey(keyParts...),
			"bustype_code":       BustypeNormalPurchase,
			"org_code":           h.OrgCode,
			"vendor_code":        h.VendorCode,
			"invoiceVendor_code": h.VendorCode,
			"currency_code":      "CNY",
			"natCurrency_code":   "CNY",
			"exchRate":           1,
			"exchRateType":       ExchRateTypeBase,
			"vouchdate":          h.VouchDate,
			"purchaseOrders":     poLines,
		},
	}
}

// 采购订单建单常量(本租户)。
const (
	BustypeNormalPurchase = "A20001"   // 普通采购
	ExchRateTypeBase      = "sdpimdz5" // 基准汇率类型
)

// ResubmitKey 幂等键: 同内容→同 key(≤32位), 用友按 resubmitCheckKey 挡重复提交。
func ResubmitKey(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return "PO" + hex.EncodeToString(h[:])[:30]
}

// NormDate 日期归一为 yyyy-MM-dd HH:mm:ss。excelize 读日期格子返回显示格式字符串(可能美式),
// 多 layout 兜底; 解析不了返回空串(由调用方当作"日期非法"拦截, 绝不把垃圾发给用友, 二审 #7)。
func NormDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	layouts := []string{
		"2006-01-02 15:04:05", "2006-01-02", "2006/1/2", "2006/01/02",
		"2006.01.02", "01-02-06", "01/02/2006", "1/2/2006",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.Format("2006-01-02 15:04:05")
		}
	}
	return ""
}
