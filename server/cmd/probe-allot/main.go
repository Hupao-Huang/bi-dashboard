// probe-allot.exe: 一次性 audit, 直接调吉客云调拨接口拉数据
// 1) 调 erp.allocate.get 拉一段时间的调拨单
// 2) 过滤 3 个目标外仓
// 3) 提取调拨明细中所有 SKU
// 4) 跟 价格体系.xlsx 对比, 列出缺失 SKU
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"bi-dashboard/internal/jackyun"
)

type Config struct {
	Jackyun struct {
		Appkey  string `json:"appkey"`
		Secret  string `json:"secret"`
		APIURL  string `json:"api_url"`
	} `json:"jackyun"`
	JackyunTrade struct {
		Appkey string `json:"appkey"`
		Secret string `json:"secret"`
	} `json:"jackyun_trade"`
}

type AllocateQuery struct {
	PageIndex       int    `json:"pageIndex"`
	PageSize        int    `json:"pageSize"`
	StartCreateTime string `json:"startCreateTime,omitempty"`
	EndCreateTime   string `json:"endCreateTime,omitempty"`
	Status          string `json:"status,omitempty"`
	InWarehouseCode string `json:"inWarehouseCode,omitempty"`
}

// AllocateOrder 调拨单(只拿对账要用的字段)
type AllocateOrder struct {
	AllocateNo         string                  `json:"allocateNo"`
	IntWarehouseName   string                  `json:"intWarehouseName"`
	IntWarehouseCode   string                  `json:"intWarehouseCode"`
	OutWarehouseCode   string                  `json:"outWarehouseCode"`
	Status             interface{}             `json:"status"`
	InStatus           interface{}             `json:"inStatus"`
	OutStatus          interface{}             `json:"outStatus"`
	GmtCreate          interface{}             `json:"gmtCreate"`
	GmtModified        interface{}             `json:"gmtModified"`
	AuditDate          interface{}             `json:"auditDate"`
	TotalAmount        interface{}             `json:"totalAmount"`
	SkuCount           interface{}             `json:"skuCount"`
	StockAllocateDetailViews []AllocateDetail `json:"stockAllocateDetailViews"`
}

type AllocateDetail struct {
	OutSkuCode  string      `json:"outSkuCode"`
	GoodsName   string      `json:"goodsName"`
	GoodsNo     string      `json:"goodsNo"`
	SkuName     string      `json:"skuName"`
	SkuCount    interface{} `json:"skuCount"`
	OutCount    interface{} `json:"outCount"`
	InCount     interface{} `json:"inCount"`
	SkuPrice    interface{} `json:"skuPrice"`
	TotalAmount interface{} `json:"totalAmount"`
	SkuBarcode  string      `json:"skuBarcode"`
}

type AllocateResult struct {
	TotalCount     int             `json:"totalCount"`
	StockAllocate  []AllocateOrder `json:"stockAllocate"`
}

type ExcelSKU struct {
	Sheet   string
	GoodsNo string
	Barcode string
	Name    string
	Price   string
}

func main() {
	configPath := flag.String("config", `C:\Users\Administrator\bi-dashboard\server\config.json`, "配置文件")
	xlsxPath := flag.String("xlsx", `C:\Users\Administrator\Desktop\价格体系.xlsx`, "价格体系")
	startStr := flag.String("start", "", "开始时间 yyyy-MM-dd (默认 60 天前)")
	endStr := flag.String("end", "", "结束时间 yyyy-MM-dd (默认今天)")
	useTrade := flag.Bool("use-trade-app", false, "用 jackyun_trade 凭证 (默认主 appkey)")
	verbose := flag.Bool("v", false, "详细日志")
	whCode := flag.String("wh-code", "", "只查这个入库仓 code (调试用)")
	flag.Parse()

	// 时间范围
	end := time.Now()
	if *endStr != "" {
		end, _ = time.Parse("2006-01-02", *endStr)
	}
	start := end.AddDate(0, 0, -60)
	if *startStr != "" {
		start, _ = time.Parse("2006-01-02", *startStr)
	}
	fmt.Printf("📅 时间范围: %s ~ %s\n", start.Format("2006-01-02"), end.Format("2006-01-02"))

	// 读 Excel
	skus, err := readPriceXlsx(*xlsxPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读 Excel 失败:", err)
		os.Exit(1)
	}
	bySheet := map[string][]ExcelSKU{}
	for _, s := range skus {
		bySheet[s.Sheet] = append(bySheet[s.Sheet], s)
	}
	fmt.Printf("📊 价格体系.xlsx 读到 %d 条 SKU\n", len(skus))
	for k, v := range bySheet {
		fmt.Printf("   - Sheet %s: %d 条\n", k, len(v))
	}

	// 配置
	bs, err := os.ReadFile(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读 config 失败:", err)
		os.Exit(1)
	}
	var cfg Config
	if err := json.Unmarshal(bs, &cfg); err != nil {
		fmt.Fprintln(os.Stderr, "解析 config 失败:", err)
		os.Exit(1)
	}

	appkey, secret := cfg.Jackyun.Appkey, cfg.Jackyun.Secret
	if *useTrade {
		appkey, secret = cfg.JackyunTrade.Appkey, cfg.JackyunTrade.Secret
		fmt.Printf("🔑 使用 jackyun_trade appkey=%s\n", appkey)
	} else {
		fmt.Printf("🔑 使用主 appkey=%s\n", appkey)
	}

	cli := jackyun.NewClient(appkey, secret, cfg.Jackyun.APIURL)

	// 拉调拨单
	orders, err := fetchAllAllocates(cli, start, end, *whCode, *verbose)
	if err != nil {
		fmt.Fprintln(os.Stderr, "拉调拨单失败:", err)
		os.Exit(1)
	}
	fmt.Printf("\n📦 共拉到 %d 条调拨单\n", len(orders))

	// 3 个目标外仓 (按入库仓名匹配)
	targetWarehouses := map[string]string{
		"朴朴自营仓-公司平台仓-外仓":    "朴朴",
		"京东自营仓-公司平台仓-外仓":    "京东",
		"天猫超市-公司平台仓-外仓":     "猫超",
	}

	// 按目标仓分组 + 收集 SKU
	type SKUItem struct {
		GoodsNo  string
		Barcode  string
		Name     string
		Count    int     // 出现单据数
		QtyTotal float64 // 累计调拨数量
		SkuPrice float64 // 接口最新单价
	}
	bucket := map[string]map[string]*SKUItem{} // sheet -> goodsNo -> item

	allWarehouses := map[string]int{} // 调试用: 看实际所有入库仓名

	for _, o := range orders {
		allWarehouses[o.IntWarehouseName]++
		sheet, ok := targetWarehouses[o.IntWarehouseName]
		if !ok {
			continue
		}
		if bucket[sheet] == nil {
			bucket[sheet] = map[string]*SKUItem{}
		}
		for _, d := range o.StockAllocateDetailViews {
			key := d.GoodsNo
			if key == "" {
				key = d.OutSkuCode
			}
			if key == "" {
				continue
			}
			it := bucket[sheet][key]
			if it == nil {
				it = &SKUItem{GoodsNo: key, Barcode: d.SkuBarcode, Name: d.GoodsName}
				bucket[sheet][key] = it
			}
			it.Count++
			it.QtyTotal += toFloat(d.SkuCount)
			if p := toFloat(d.SkuPrice); p > 0 {
				it.SkuPrice = p
			}
		}
	}

	if *verbose {
		fmt.Println("\n📋 实际拉到的所有入库仓名(前 20):")
		type wh struct {
			Name  string
			Count int
		}
		var whs []wh
		for k, v := range allWarehouses {
			whs = append(whs, wh{k, v})
		}
		sort.Slice(whs, func(i, j int) bool { return whs[i].Count > whs[j].Count })
		for i, w := range whs {
			if i >= 20 {
				break
			}
			fmt.Printf("   - %-50s %d 单\n", w.Name, w.Count)
		}
	}

	// 对每个目标渠道做对比
	channelOrder := []struct {
		Sheet   string
		Channel string
	}{
		{"朴朴", "js-即时零售事业一部（世创）-朴朴"},
		{"京东", "ds-京东-清心湖自营"},
		{"猫超", "ds-天猫超市-寄售"},
	}

	for _, co := range channelOrder {
		fmt.Printf("\n========================================\n")
		fmt.Printf("🔍 渠道: %s   入库仓: %s\n", co.Channel, findWarehouseBySheet(targetWarehouses, co.Sheet))
		fmt.Printf("========================================\n")

		items := bucket[co.Sheet]
		if len(items) == 0 {
			fmt.Println("⚠️  这个仓没拉到任何调拨明细")
			continue
		}

		// 按数量排序
		var list []*SKUItem
		for _, v := range items {
			list = append(list, v)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].QtyTotal > list[j].QtyTotal })

		fmt.Printf("📦 调拨单 SKU 共 %d 个\n", len(list))

		// Excel 索引
		egn := map[string]ExcelSKU{}
		ebc := map[string]ExcelSKU{}
		for _, e := range bySheet[co.Sheet] {
			if e.GoodsNo != "" {
				egn[e.GoodsNo] = e
			}
			if e.Barcode != "" {
				ebc[e.Barcode] = e
			}
		}

		// 对比
		var missing []*SKUItem
		matched := 0
		for _, it := range list {
			if _, ok := egn[it.GoodsNo]; ok {
				matched++
				continue
			}
			if it.Barcode != "" {
				if _, ok := ebc[it.Barcode]; ok {
					matched++
					continue
				}
			}
			missing = append(missing, it)
		}

		fmt.Printf("✅ Excel 已匹配: %d 个\n", matched)
		fmt.Printf("❌ Excel 缺失: %d 个\n", len(missing))
		if len(missing) > 0 {
			for i, m := range missing {
				fmt.Printf("   [%d] goods_no=%-15s barcode=%-15s 出现%d单 累计%.0f件 接口价=%.2f 名称=%s\n",
					i+1, m.GoodsNo, m.Barcode, m.Count, m.QtyTotal, m.SkuPrice, m.Name)
			}
		}

		// Excel 有但调拨单没出现过
		used := map[string]bool{}
		for _, it := range list {
			used[it.GoodsNo] = true
		}
		var unused []ExcelSKU
		for _, e := range bySheet[co.Sheet] {
			if !used[e.GoodsNo] {
				unused = append(unused, e)
			}
		}
		if len(unused) > 0 {
			fmt.Printf("⚠️  Excel 有但调拨单 60 天内没出现: %d 个\n", len(unused))
			for i, u := range unused {
				if i >= 15 {
					fmt.Printf("   ... 还有 %d 个\n", len(unused)-15)
					break
				}
				fmt.Printf("   - %s  %s  价=%s\n", u.GoodsNo, u.Name, u.Price)
			}
		}
	}
}

func findWarehouseBySheet(m map[string]string, sheet string) string {
	for k, v := range m {
		if v == sheet {
			return k
		}
	}
	return "?"
}

func fetchAllAllocates(cli *jackyun.Client, start, end time.Time, whCode string, verbose bool) ([]AllocateOrder, error) {
	var all []AllocateOrder
	pageIndex := 0
	pageSize := 50
	for {
		q := AllocateQuery{
			PageIndex:       pageIndex,
			PageSize:        pageSize,
			StartCreateTime: start.Format("2006-01-02 15:04:05"),
			EndCreateTime:   end.Format("2006-01-02 15:04:05"),
			InWarehouseCode: whCode,
		}
		resp, err := cli.Call("erp.allocate.get", q)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", pageIndex, err)
		}
		if verbose && pageIndex == 0 {
			fmt.Printf("   [raw page0] code=%d msg=%s subCode=%s result=%s\n",
				resp.Code, resp.Msg, resp.SubCode, string(resp.Result[:min(len(resp.Result), 1500)]))
		}
		if resp.Code != 200 {
			return nil, fmt.Errorf("api code=%d msg=%s subCode=%s", resp.Code, resp.Msg, resp.SubCode)
		}

		// result -> data -> stockAllocateVOs
		var w1 struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(resp.Result, &w1); err != nil {
			return nil, fmt.Errorf("unmarshal result: %w body=%s", err, string(resp.Result[:min(len(resp.Result), 500)]))
		}
		var page AllocateResult
		if err := json.Unmarshal(w1.Data, &page); err != nil {
			return nil, fmt.Errorf("unmarshal data: %w body=%s", err, string(w1.Data[:min(len(w1.Data), 500)]))
		}

		if verbose {
			fmt.Printf("   page %d → %d 单 (累计 %d / 总 %d)\n", pageIndex, len(page.StockAllocate), len(all)+len(page.StockAllocate), page.TotalCount)
		}

		if len(page.StockAllocate) == 0 {
			break
		}
		all = append(all, page.StockAllocate...)

		if page.TotalCount > 0 && len(all) >= page.TotalCount {
			break
		}
		pageIndex++
		if pageIndex > 200 { // 安全栓
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	return all, nil
}

func readPriceXlsx(path string) ([]ExcelSKU, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var skus []ExcelSKU
	for _, sheetName := range f.GetSheetList() {
		rows, _ := f.GetRows(sheetName)
		if len(rows) < 2 {
			continue
		}
		header := rows[0]
		idxBC, idxGN, idxNM, idxPR := -1, -1, -1, -1
		for i, h := range header {
			h = strings.TrimSpace(h)
			switch h {
			case "erp条码", "条码":
				idxBC = i
			case "编码", "商品编码":
				idxGN = i
			case "名称", "商品名称":
				idxNM = i
			case "采购价", "单价":
				idxPR = i
			}
		}
		for ri, r := range rows {
			if ri == 0 {
				continue
			}
			s := ExcelSKU{Sheet: sheetName}
			if idxBC >= 0 && idxBC < len(r) {
				s.Barcode = strings.TrimSpace(r[idxBC])
			}
			if idxGN >= 0 && idxGN < len(r) {
				s.GoodsNo = strings.TrimSpace(r[idxGN])
			}
			if idxNM >= 0 && idxNM < len(r) {
				s.Name = strings.TrimSpace(r[idxNM])
			}
			if idxPR >= 0 && idxPR < len(r) {
				s.Price = strings.TrimSpace(r[idxPR])
			}
			if s.GoodsNo == "" && s.Barcode == "" {
				continue
			}
			skus = append(skus, s)
		}
	}
	return skus, nil
}

func toFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case string:
		var f float64
		fmt.Sscanf(x, "%f", &f)
		return f
	case json.Number:
		f, _ := x.Float64()
		return f
	}
	return 0
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
