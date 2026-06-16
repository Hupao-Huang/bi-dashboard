// po-dryrun 采购订单导入"空跑": 读 Excel 模板 → 翻译编码(组织/物料/供应商)→ 算价 →
// 按"组织+供应商+单据日期"合并组装用友采购订单报文 → 打印(不建单)。
// 验证整条链路用, 给跑哥看"模板 → 用友报文"。用法: po-dryrun <xlsx路径>
package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/yonsuite"

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

// resubmitKey 幂等键: 同内容→同key, 用友侧按 resubmitCheckKey 挡重复提交(≤32位)。
// 同一张模板重复导入 → 同样的订单算出同样的 key → 用友拒绝重复建单, 防呆。
func resubmitKey(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return "PO" + hex.EncodeToString(h[:])[:30]
}

const (
	bustypeNormalPurchase = "A20001"   // 普通采购 (跑哥确认)
	exchRateTypeBase      = "sdpimdz5" // 基准汇率类型 (本租户, 历史确认)
)

func round2(x float64) float64 { return math.Round(x*100) / 100 }

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

// 单据日期/计划到货日期 统一成 yyyy-MM-dd HH:mm:ss。
// excelize 读日期格子会按显示格式给字符串(如美式 "06-16-26"=MM-DD-YY), 多格式兜底解析。
func normDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	layouts := []string{
		"2006-01-02 15:04:05", "2006-01-02", "2006/1/2", "2006/01/02",
		"01-02-06", "1/2/2006", "01/02/2006", "2006.01.02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.Format("2006-01-02 15:04:05")
		}
	}
	return s // 解析不了原样返回 (会在报文里暴露, 便于发现)
}

type tmplRow struct {
	OrgName, Bustype, VouchDate, VendorName        string
	ProductCode, ProductName                       string
	Qty, UnitName, MainUnit                        string
	TaxInclPrice, TaxInclAmount, TaxRate, ArriveDt string
	rowNo                                          int
}

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("用法: po-dryrun <xlsx路径> [--commit-one]")
	}
	xlsxPath := os.Args[1]
	commitOne := false // 默认只打印不建单; --commit-one 才真发第一张(测试用)
	for _, a := range os.Args[2:] {
		if a == "--commit-one" {
			commitOne = true
		}
	}

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	ys := yonsuite.NewClient(cfg.YonSuite.AppKey, cfg.YonSuite.AppSecret, cfg.YonSuite.BaseURL)

	// 1) 读 Excel
	rows := readTemplate(xlsxPath)
	fmt.Printf("读到 %d 行明细\n", len(rows))

	// 2) 翻译组织 (名称→编码)
	orgs, err := ys.QueryPurchaseOrgs()
	if err != nil {
		log.Fatalf("查组织: %v", err)
	}
	orgCodeByName := map[string]string{}
	for _, o := range orgs {
		orgCodeByName[o.Name] = o.Code
	}

	// 3) 翻译物料 (编码→单位/税目), 按组织批量查
	orgCodeForProducts := ""
	codeSet := map[string]bool{}
	var codes []string
	for _, r := range rows {
		if orgCodeForProducts == "" {
			orgCodeForProducts = orgCodeByName[r.OrgName]
		}
		if !codeSet[r.ProductCode] {
			codeSet[r.ProductCode] = true
			codes = append(codes, r.ProductCode)
		}
	}
	prodByCode := map[string]*yonsuite.ProductDetail{}
	if orgCodeForProducts != "" {
		prodByCode, err = ys.QueryProductDetails(orgCodeForProducts, codes)
		if err != nil {
			log.Fatalf("查物料详情: %v", err)
		}
	}

	// 4) 翻译供应商 (名称→编码), 查本地字典表 ys_vendor_dict
	vendorCodeByName := map[string]string{}
	for _, r := range rows {
		if _, ok := vendorCodeByName[r.VendorName]; ok {
			continue
		}
		var code string
		err := db.QueryRow("SELECT code FROM ys_vendor_dict WHERE name = ? LIMIT 1", r.VendorName).Scan(&code)
		if err == nil {
			vendorCodeByName[r.VendorName] = code
		} else {
			vendorCodeByName[r.VendorName] = "" // 查不到
		}
	}

	// 5) 逐行翻译+算价, 打印对照表
	fmt.Println("\n===== 逐行翻译 + 算价 (绿=OK 红=缺) =====")
	type lineCalc struct {
		row              tmplRow
		orgCode          string
		vendorCode       string
		prod             *yonsuite.ProductDetail
		oriSum, oriMoney float64
		oriTax, oriUP    float64
		taxRate          float64
		problems         []string
	}
	var calcs []lineCalc
	for _, r := range rows {
		lc := lineCalc{row: r}
		lc.orgCode = orgCodeByName[r.OrgName]
		lc.vendorCode = vendorCodeByName[r.VendorName]
		lc.prod = prodByCode[r.ProductCode]

		if lc.orgCode == "" {
			lc.problems = append(lc.problems, "组织查不到编码")
		}
		if lc.vendorCode == "" {
			lc.problems = append(lc.problems, "供应商查不到编码")
		}
		if lc.prod == nil {
			lc.problems = append(lc.problems, "物料查不到(单位/税目)")
		}

		// 算价: 含税金额 = 含税单价 × 采购数量; 无税 = 含税/(1+税率); 税额 = 含税-无税
		qty := atof(r.Qty)
		taxPrice := atof(r.TaxInclPrice)
		lc.taxRate = atof(r.TaxRate) / 100.0
		lc.oriSum = round2(taxPrice * qty)
		if amt := atof(r.TaxInclAmount); amt > 0 {
			lc.oriSum = round2(amt) // 模板有含税金额就以它为准
		}
		if lc.taxRate > 0 {
			lc.oriMoney = round2(lc.oriSum / (1 + lc.taxRate))
		} else {
			lc.oriMoney = lc.oriSum
		}
		lc.oriTax = round2(lc.oriSum - lc.oriMoney)
		if qty != 0 {
			lc.oriUP = round2(lc.oriMoney / qty)
		}
		calcs = append(calcs, lc)

		status := "✅"
		if len(lc.problems) > 0 {
			status = "❌ " + strings.Join(lc.problems, ",")
		}
		taxitems := ""
		unit := ""
		if lc.prod != nil {
			taxitems = lc.prod.TaxitemsCode
			unit = lc.prod.PurUOMCode
		}
		fmt.Printf("行%d %s\n   组织 %s→%s | 供应商 %s→%s | 物料 %s→单位%s/税目%s\n   含税单价%.4f×%.0f=含税%.2f 无税%.2f 税额%.2f\n",
			r.rowNo, status, r.OrgName, lc.orgCode, r.VendorName, lc.vendorCode, r.ProductCode, unit, taxitems,
			taxPrice, qty, lc.oriSum, lc.oriMoney, lc.oriTax)
	}

	// 6) 按 组织+供应商+单据日期 合并成订单, 组装用友报文
	fmt.Println("\n===== 组装用友采购订单报文 (按 组织+供应商+日期 合并) =====")
	type grpKey struct{ org, vendor, date string }
	groups := map[grpKey][]lineCalc{}
	var order []grpKey
	for _, lc := range calcs {
		k := grpKey{lc.orgCode, lc.vendorCode, normDate(lc.row.VouchDate)}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], lc)
	}

	for gi, k := range order {
		lines := groups[k]
		poLines := []map[string]interface{}{}
		keyParts := []string{k.org, k.vendor, k.date}
		for i, lc := range lines {
			qty := atof(lc.row.Qty)
			taxPrice := atof(lc.row.TaxInclPrice)
			var unitCode, purUOM, priceUOM, taxitems string
			if lc.prod != nil {
				unitCode, purUOM, priceUOM, taxitems = lc.prod.UnitCode, lc.prod.PurUOMCode, lc.prod.PriceUOMCode, lc.prod.TaxitemsCode
			}
			keyParts = append(keyParts, lc.row.ProductCode, strconv.FormatFloat(qty, 'f', -1, 64), strconv.FormatFloat(taxPrice, 'f', -1, 64))
			poLines = append(poLines, map[string]interface{}{
				"_status":               "Insert",
				"rowno":                 strconv.Itoa((i + 1) * 10),
				"inOrg_code":            k.org,
				"inInvoiceOrg_code":     k.org,
				"product_cCode":         lc.row.ProductCode,
				"unit_code":             unitCode,
				"purUOM_Code":           purUOM,
				"priceUOM_Code":         priceUOM,
				"taxitems_code":         taxitems,
				"qty":                   qty,
				"subQty":                qty,
				"priceQty":              qty,
				"invExchRate":           1,
				"invPriceExchRate":      1,
				"unitExchangeType":      0,
				"unitExchangeTypePrice": 0,
				"oriTaxUnitPrice":       taxPrice,
				"oriUnitPrice":          lc.oriUP,
				"oriSum":                lc.oriSum,
				"oriMoney":              lc.oriMoney,
				"oriTax":                lc.oriTax,
				"natTaxUnitPrice":       taxPrice,
				"natUnitPrice":          lc.oriUP,
				"natSum":                lc.oriSum,
				"natMoney":              lc.oriMoney,
				"natTax":                lc.oriTax,
				"isGiftProduct":         false,
			})
		}
		payload := map[string]interface{}{
			"data": map[string]interface{}{
				"_status":            "Insert",
				"resubmitCheckKey":   resubmitKey(keyParts...),
				"bustype_code":       bustypeNormalPurchase,
				"org_code":           k.org,
				"vendor_code":        k.vendor,
				"invoiceVendor_code": k.vendor,
				"currency_code":      "CNY",
				"natCurrency_code":   "CNY",
				"exchRate":           1,
				"exchRateType":       exchRateTypeBase,
				"vouchdate":          k.date,
				"purchaseOrders":     poLines,
			},
		}

		// 有缺失编码的订单绝不发(防止建出脏单)
		hasProblem := false
		for _, lc := range lines {
			if len(lc.problems) > 0 {
				hasProblem = true
			}
		}

		js, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Printf("\n--- 订单%d: 组织%s 供应商%s 日期%s (%d 行) ---\n%s\n", gi+1, k.org, k.vendor, k.date, len(lines), string(js))

		// --commit-one: 只真发第一张, 且必须无缺失编码; 写用友不可逆
		if commitOne && gi == 0 {
			if hasProblem {
				fmt.Println(">>> 第一张订单有缺失编码, 拒绝建单。请先补齐再试。")
				return
			}
			fmt.Println(">>> [真建单] 发送第一张订单到用友 ...")
			resp, err := ys.PurchaseOrderSingleSave(payload)
			if err != nil {
				fmt.Printf(">>> ❌ 建单失败: %v\n", err)
				if resp != nil {
					fmt.Printf(">>> 用友返回: code=%s msg=%s data=%s\n", resp.Code, resp.Message, string(resp.Data))
				}
				return
			}
			fmt.Printf(">>> ✅ 建单成功! 用友返回: code=%s msg=%s\n>>> data=%s\n", resp.Code, resp.Message, string(resp.Data))
			return // 测试只发一张
		}
	}
}

func readTemplate(path string) []tmplRow {
	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Fatalf("打开Excel: %v", err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	allRows, err := f.GetRows(sheet)
	if err != nil {
		log.Fatalf("读Excel行: %v", err)
	}
	if len(allRows) < 2 {
		log.Fatalf("Excel无数据行")
	}
	// 表头列名→列号
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
	var out []tmplRow
	for ri := 1; ri < len(allRows); ri++ {
		r := allRows[ri]
		if get(r, "物料编码") == "" && get(r, "供货供应商") == "" {
			continue
		}
		out = append(out, tmplRow{
			OrgName:       get(r, "采购组织"),
			Bustype:       get(r, "交易类型"),
			VouchDate:     get(r, "单据日期"),
			VendorName:    get(r, "供货供应商"),
			ProductCode:   get(r, "物料编码"),
			ProductName:   get(r, "物料名称"),
			Qty:           get(r, "采购数量"),
			UnitName:      get(r, "采购单位名称"),
			MainUnit:      get(r, "主计量"),
			TaxInclPrice:  get(r, "含税单价"),
			TaxInclAmount: get(r, "含税金额"),
			TaxRate:       get(r, "税率"),
			ArriveDt:      get(r, "计划到货日期"),
			rowNo:         ri + 1,
		})
	}
	return out
}
