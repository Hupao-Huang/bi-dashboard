// po-dryrun 采购订单导入"空跑": 读 Excel 模板 → 翻译编码(组织/物料/供应商)→ 算价 →
// 按"组织+供应商+单据日期"合并组装用友采购订单报文 → 打印(不建单)。
// 核心算价/组装/幂等逻辑在 internal/yonsuite/purchase_build.go(与正式 handler 共用)。
// 用法: po-dryrun <xlsx路径> [--commit-one]   --commit-one 才真发第一张(测试, 不可逆)。
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/yonsuite"

	_ "github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
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

	rows := readTemplate(xlsxPath)
	fmt.Printf("读到 %d 行明细\n", len(rows))

	// 翻译组织 (名称→编码)
	orgs, err := ys.QueryPurchaseOrgs()
	if err != nil {
		log.Fatalf("查组织: %v", err)
	}
	orgCodeByName := map[string]string{}
	for _, o := range orgs {
		orgCodeByName[o.Name] = o.Code
	}

	// 翻译物料 (编码→单位/税目) — 按每行各自组织分桶批量查(支持多组织)
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
	prodByOrgCode := map[string]map[string]*yonsuite.ProductDetail{} // org→(code→detail)
	for oc, set := range codesByOrg {
		var codes []string
		for c := range set {
			codes = append(codes, c)
		}
		m, err := ys.QueryProductDetails(oc, codes)
		if err != nil {
			log.Fatalf("查物料详情(org=%s): %v", oc, err)
		}
		prodByOrgCode[oc] = m
	}

	// 翻译供应商 (名称→编码), 查本地字典表; 区分"查无此家"与"数据库报错"(二审#10)
	vendorCodeByName := map[string]string{}
	for _, r := range rows {
		if _, ok := vendorCodeByName[r.VendorName]; ok {
			continue
		}
		var code string
		switch err := db.QueryRow("SELECT code FROM ys_vendor_dict WHERE name = ? LIMIT 1", r.VendorName).Scan(&code); err {
		case nil:
			vendorCodeByName[r.VendorName] = code
		case sql.ErrNoRows:
			vendorCodeByName[r.VendorName] = "" // 字典里确实没有
		default:
			log.Fatalf("查供应商字典失败(可能DB异常, 勿当作'查无此家'): %v", err)
		}
	}

	// 逐行翻译+算价+查错, 打印对照
	fmt.Println("\n===== 逐行翻译 + 算价 (绿=OK 红=缺/错) =====")
	type lineCalc struct {
		row        tmplRow
		orgCode    string
		vendorCode string
		vouchDate  string
		prod       *yonsuite.ProductDetail
		prices     yonsuite.LinePrices
		problems   []string
	}
	var calcs []lineCalc
	for _, r := range rows {
		lc := lineCalc{row: r}
		lc.orgCode = orgCodeByName[r.OrgName]
		lc.vendorCode = vendorCodeByName[r.VendorName]
		lc.vouchDate = yonsuite.NormDate(r.VouchDate)
		if lc.orgCode != "" {
			lc.prod = prodByOrgCode[lc.orgCode][r.ProductCode]
		}
		qty := atof(r.Qty)
		taxPrice := atof(r.TaxInclPrice)
		lc.prices = yonsuite.ComputeLinePrices(taxPrice, qty, atof(r.TaxRate))

		if lc.orgCode == "" {
			lc.problems = append(lc.problems, "组织查不到编码")
		}
		if lc.vendorCode == "" {
			lc.problems = append(lc.problems, "供应商查不到编码")
		}
		if lc.prod == nil {
			lc.problems = append(lc.problems, "物料查不到(单位/税目)")
		}
		if lc.vouchDate == "" {
			lc.problems = append(lc.problems, "单据日期非法/无法解析")
		}
		if qty <= 0 {
			lc.problems = append(lc.problems, "采购数量≤0")
		}
		if taxPrice < 0 {
			lc.problems = append(lc.problems, "含税单价为负")
		}
		// 模板填了含税金额却跟 单价×数量 对不上(>1分)→ 提示, 防止填错列
		if amt := atof(r.TaxInclAmount); amt != 0 && absf(amt-lc.prices.OriSum) > 0.01 {
			lc.problems = append(lc.problems, fmt.Sprintf("含税金额%.2f≠单价×数量%.2f", amt, lc.prices.OriSum))
		}
		calcs = append(calcs, lc)

		status := "✅"
		if len(lc.problems) > 0 {
			status = "❌ " + strings.Join(lc.problems, ",")
		}
		unit, taxitems := "", ""
		if lc.prod != nil {
			unit, taxitems = lc.prod.PurUOMCode, lc.prod.TaxitemsCode
		}
		fmt.Printf("行%d %s\n   组织 %s→%s | 供应商 %s→%s | 物料 %s→单位%s/税目%s\n   含税%.2f 无税%.2f 税额%.2f 无税单价%.8f\n",
			r.rowNo, status, r.OrgName, lc.orgCode, r.VendorName, lc.vendorCode, r.ProductCode, unit, taxitems,
			lc.prices.OriSum, lc.prices.OriMoney, lc.prices.OriTax, lc.prices.OriUnitPrice)
	}

	// 按 组织+供应商+单据日期 合并成订单
	fmt.Println("\n===== 组装用友采购订单报文 (按 组织+供应商+日期 合并) =====")
	type grpKey struct{ org, vendor, date string }
	groups := map[grpKey][]lineCalc{}
	var order []grpKey
	for _, lc := range calcs {
		k := grpKey{lc.orgCode, lc.vendorCode, lc.vouchDate}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], lc)
	}

	for gi, k := range order {
		lines := groups[k]
		hasProblem := false
		poLines := make([]yonsuite.POLineInput, 0, len(lines))
		for _, lc := range lines {
			if len(lc.problems) > 0 {
				hasProblem = true
			}
			var unit, pur, price, tax string
			if lc.prod != nil {
				unit, pur, price, tax = lc.prod.UnitCode, lc.prod.PurUOMCode, lc.prod.PriceUOMCode, lc.prod.TaxitemsCode
			}
			poLines = append(poLines, yonsuite.POLineInput{
				ProductCode:      lc.row.ProductCode,
				UnitCode:         unit,
				PurUOMCode:       pur,
				PriceUOMCode:     price,
				TaxitemsCode:     tax,
				Qty:              atof(lc.row.Qty),
				TaxInclUnitPrice: atof(lc.row.TaxInclPrice),
				TaxRatePct:       atof(lc.row.TaxRate),
			})
		}
		payload := yonsuite.BuildPurchaseOrderPayload(
			yonsuite.POHeaderInput{OrgCode: k.org, VendorCode: k.vendor, VouchDate: k.date},
			poLines,
		)
		js, _ := json.MarshalIndent(payload, "", "  ")
		flag := "✅可建"
		if hasProblem {
			flag = "❌有问题不可建"
		}
		fmt.Printf("\n--- 订单%d [%s]: 组织%s 供应商%s 日期%s (%d 行) ---\n%s\n", gi+1, flag, k.org, k.vendor, k.date, len(lines), string(js))

		// --commit-one: 只真发第一张, 必须无任何问题; 写用友不可逆
		if commitOne && gi == 0 {
			if hasProblem {
				fmt.Println(">>> 第一张订单有问题, 拒绝建单。请先补齐再试。")
				return
			}
			fmt.Println(">>> [真建单] 发送第一张订单到用友 ...")
			id, resp, err := ys.SavePurchaseOrder(payload)
			if err != nil {
				fmt.Printf(">>> ❌ 建单失败: %v\n", err)
				if resp != nil {
					fmt.Printf(">>> 用友返回: code=%s msg=%s\n", resp.Code, resp.Message)
				}
				return
			}
			fmt.Printf(">>> ✅ 建单成功! 采购订单id=%s (开立态, 可在用友删除)\n", id)
			return // 测试只发一张
		}
	}
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
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
