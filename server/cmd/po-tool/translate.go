package main

// 采购订单 Excel → 翻译 → 算价 → 红绿预览 → 按"组织+供应商+日期"合并成订单。
//
// 逻辑搬自 BI 服务器 handler.YonbipPOPreview, 唯一区别: 供应商改查本地名单 vendors.json
// (离网运行连不到数据库)。组织、物料仍实时连用友查, 不会过期。

import (
	"fmt"
	"strconv"
	"strings"

	"bi-dashboard/internal/yonsuite"

	"github.com/xuri/excelize/v2"
)

// previewRow 回前端展示的一行(翻译+算价结果, 红=problems 绿=ok)。
type previewRow struct {
	RowNo        int      `json:"rowNo"`
	OrderIndex   int      `json:"orderIndex"`
	OrgName      string   `json:"orgName"`
	OrgCode      string   `json:"orgCode"`
	VendorName   string   `json:"vendorName"`
	VendorCode   string   `json:"vendorCode"`
	ProductCode  string   `json:"productCode"`
	ProductName  string   `json:"productName"`
	UnitCode     string   `json:"unitCode"`
	TaxitemsCode string   `json:"taxitemsCode"`
	TaxRatePct   float64  `json:"taxRatePct"`
	TaxRateName  string   `json:"taxRateName"`
	Qty          float64  `json:"qty"`
	TaxInclPrice float64  `json:"taxInclPrice"`
	OriSum       float64  `json:"oriSum"`
	OriMoney     float64  `json:"oriMoney"`
	OriTax       float64  `json:"oriTax"`
	ArriveDate   string   `json:"arriveDate"`
	Problems     []string `json:"problems"`
	Warnings     []string `json:"warnings"`
}

// previewOrder 合并后的一张订单(Lines 给建单用, 不回前端)。
type previewOrder struct {
	OrgCode    string                 `json:"orgCode"`
	OrgName    string                 `json:"orgName"`
	VendorCode string                 `json:"vendorCode"`
	VendorName string                 `json:"vendorName"`
	VouchDate  string                 `json:"vouchDate"`
	Lines      []yonsuite.POLineInput `json:"-"`
	LineCount  int                    `json:"lineCount"`
	TotalSum   float64                `json:"totalSum"`
	HasProblem bool                   `json:"hasProblem"`
}

// tmplRow Excel 模板的一行原始文本。
type tmplRow struct {
	OrgName, VouchDate, VendorName            string
	ProductCode, ProductName                  string
	Qty, TaxInclPrice, TaxInclAmount, TaxRate string
	ArriveDt                                  string
	rowNo                                     int
}

func parseTemplate(f *excelize.File) ([]tmplRow, error) {
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
	var out []tmplRow
	for ri := 1; ri < len(allRows); ri++ {
		r := allRows[ri]
		if get(r, "物料编码") == "" && get(r, "供货供应商") == "" {
			continue
		}
		out = append(out, tmplRow{
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

// translateAndPrice 翻译(组织/物料实时查用友, 供应商查本地名单)+ 算价 + 查错 + 合并订单。
// 返回 前端展示行 + 订单列表(含建单 Lines)。任一用友接口失败 → 整体报错(不带半截数据建单)。
func translateAndPrice(rows []tmplRow, ys *yonsuite.Client, vendors map[string]string) ([]previewRow, []previewOrder, error) {
	// 组织名→编码(实时查用友)
	orgs, err := ys.QueryPurchaseOrgs()
	if err != nil {
		return nil, nil, fmt.Errorf("查询用友组织失败: %w", err)
	}
	orgCodeByName := map[string]string{}
	for _, o := range orgs {
		orgCodeByName[o.Name] = o.Code
	}

	// 物料编码→单位/税目(按组织分桶批量实时查用友)
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
		m, err := ys.QueryProductDetails(oc, codes)
		if err != nil {
			return nil, nil, fmt.Errorf("查询用友物料详情失败(组织%s): %w", oc, err)
		}
		prodByOrg[oc] = m
	}

	// 供应商名→编码(本地名单)
	vendorCodeByName := map[string]string{}
	for _, r := range rows {
		if _, ok := vendorCodeByName[r.VendorName]; ok {
			continue
		}
		vendorCodeByName[r.VendorName] = vendors[r.VendorName]
	}

	// 逐行翻译+算价+查错
	type rowCalc struct {
		row      tmplRow
		dto      previewRow
		line     yonsuite.POLineInput
		problems []string
	}
	var calcs []rowCalc
	previewRows := make([]previewRow, 0, len(rows))
	for _, r := range rows {
		orgCode := orgCodeByName[r.OrgName]
		vendorCode := vendorCodeByName[r.VendorName]
		var prod *yonsuite.ProductDetail
		if orgCode != "" {
			prod = prodByOrg[orgCode][r.ProductCode]
		}
		vouchDate := yonsuite.NormDate(r.VouchDate)
		qty := atof(r.Qty)
		taxPrice := atof(r.TaxInclPrice)
		tmplRate := atof(r.TaxRate)
		// 税率以货品档案为准(权威); 货品查不到才退回模板(此时本就因物料缺失标红)
		taxRate := tmplRate
		if prod != nil && prod.TaxRatePct > 0 {
			taxRate = prod.TaxRatePct
		}
		prices := yonsuite.ComputeLinePrices(taxPrice, qty, taxRate)

		var probs, warns []string
		if prod != nil && prod.TaxRatePct > 0 && tmplRate > 0 && tmplRate != prod.TaxRatePct {
			warns = append(warns, fmt.Sprintf("模板填%.0f%%与货品档案%.0f%%不符, 已按货品%.0f%%算", tmplRate, prod.TaxRatePct, prod.TaxRatePct))
		}
		if orgCode == "" {
			probs = append(probs, "组织查不到编码")
		}
		if vendorCode == "" {
			probs = append(probs, "供应商查不到编码(名单里没有, 可能要重导名单)")
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
		if taxPrice <= 0 {
			probs = append(probs, "含税单价≤0(可能没填或填错), 不建零元单")
		}
		if amt := atof(r.TaxInclAmount); amt != 0 && absFloat(amt-prices.OriSum) > 0.01 {
			probs = append(probs, fmt.Sprintf("含税金额%.2f≠单价×数量%.2f", amt, prices.OriSum))
		}
		// 到货日期填了但解析不出来 → 告警(不拦建单, 但订单上不会带到货日期, 别让用户以为带上了)
		if strings.TrimSpace(r.ArriveDt) != "" && yonsuite.NormDate(r.ArriveDt) == "" {
			warns = append(warns, "计划到货日期识别不了, 已忽略(订单上不会带到货日期)")
		}

		unit, taxitems, taxName := "", "", ""
		if prod != nil {
			unit, taxitems, taxName = prod.PurUOMCode, prod.TaxitemsCode, prod.TaxRateName
		}
		dto := previewRow{
			RowNo: r.rowNo, OrgName: r.OrgName, OrgCode: orgCode,
			VendorName: r.VendorName, VendorCode: vendorCode,
			ProductCode: r.ProductCode, ProductName: r.ProductName,
			UnitCode: unit, TaxitemsCode: taxitems, TaxRatePct: taxRate, TaxRateName: taxName,
			Qty: qty, TaxInclPrice: taxPrice,
			OriSum: prices.OriSum, OriMoney: prices.OriMoney, OriTax: prices.OriTax,
			ArriveDate: yonsuite.NormDate(r.ArriveDt), Problems: probs, Warnings: warns,
		}
		previewRows = append(previewRows, dto)

		var line yonsuite.POLineInput
		if prod != nil {
			line = yonsuite.POLineInput{
				ProductCode: r.ProductCode, UnitCode: prod.UnitCode, PurUOMCode: prod.PurUOMCode,
				PriceUOMCode: prod.PriceUOMCode, TaxitemsCode: prod.TaxitemsCode,
				Qty: qty, TaxInclUnitPrice: taxPrice, TaxRatePct: taxRate,
				ArriveDate: yonsuite.NormDate(r.ArriveDt),
			}
		}
		calcs = append(calcs, rowCalc{row: r, dto: dto, line: line, problems: probs})
	}

	// 按 组织+供应商+日期 合并成订单。
	// 翻译失败(编码为空)时按"原名"分组, 否则不同组织/供应商会因空码被错并进同一张预览单(虽因 HasProblem 不会真建, 但展示误导)。
	type gk struct{ org, vendor, date string }
	idx := map[gk]int{}
	var orders []previewOrder
	for ci, c := range calcs {
		vouchDate := yonsuite.NormDate(c.row.VouchDate)
		orgKey := orgCodeByName[c.row.OrgName]
		if orgKey == "" {
			orgKey = "name:" + c.row.OrgName
		}
		vendorKey := vendorCodeByName[c.row.VendorName]
		if vendorKey == "" {
			vendorKey = "vname:" + c.row.VendorName
		}
		k := gk{orgKey, vendorKey, vouchDate}
		i, ok := idx[k]
		if !ok {
			i = len(orders)
			idx[k] = i
			// 存真实编码(翻译失败时为空, 不能存 "name:" 前缀的分组键, 否则建单会把假码发给用友)
			orders = append(orders, previewOrder{
				OrgCode: orgCodeByName[c.row.OrgName], OrgName: c.row.OrgName,
				VendorCode: vendorCodeByName[c.row.VendorName], VendorName: c.row.VendorName, VouchDate: vouchDate,
			})
		}
		previewRows[ci].OrderIndex = i
		orders[i].Lines = append(orders[i].Lines, c.line)
		orders[i].LineCount++
		orders[i].TotalSum = roundMoney(orders[i].TotalSum + c.dto.OriSum)
		if len(c.problems) > 0 {
			orders[i].HasProblem = true
		}
	}
	return previewRows, orders, nil
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
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
