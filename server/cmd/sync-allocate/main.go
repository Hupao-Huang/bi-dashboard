// sync-allocate.exe: 拉吉客云调拨单 → allocate_orders + allocate_details
// 只拉 3 个外仓 (0019/0057/0110) 对应京东自营/天猫超市寄售/朴朴
// 用法: sync-allocate.exe [--start=YYYY-MM-DD] [--end=YYYY-MM-DD] [--days=N]
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"bi-dashboard/internal/jackyun"
)

type Config struct {
	Database struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
		Dbname   string `json:"dbname"`
	} `json:"database"`
	Jackyun struct {
		Appkey string `json:"appkey"`
		Secret string `json:"secret"`
		APIURL string `json:"api_url"`
	} `json:"jackyun"`
}

// 3 个外仓 → 渠道映射
var warehouseMap = map[string]struct {
	Code        string
	ChannelKey  string // 京东/猫超/朴朴
	ChannelName string // 渠道全称
}{
	"0057": {"0057", "京东", "ds-京东-清心湖自营"},
	"0019": {"0019", "猫超", "ds-天猫超市-寄售"},
	"0110": {"0110", "朴朴", "js-即时零售事业一部（世创）-朴朴"},
}

type AllocateQuery struct {
	PageIndex       int    `json:"pageIndex"`
	PageSize        int    `json:"pageSize"`
	StartCreateTime string `json:"startCreateTime,omitempty"`
	EndCreateTime   string `json:"endCreateTime,omitempty"`
	InWarehouseCode string `json:"inWarehouseCode,omitempty"`
}

type AllocateOrder struct {
	AllocateNo               string           `json:"allocateNo"`
	AllocateID               string           `json:"allocateId"`
	IntWarehouseName         string           `json:"intWarehouseName"`
	IntWarehouseCode         string           `json:"intWarehouseCode"`
	OutWarehouseCode         string           `json:"outWarehouseCode"`
	Status                   interface{}      `json:"status"`
	InStatus                 interface{}      `json:"inStatus"`
	OutStatus                interface{}      `json:"outStatus"`
	GmtCreate                interface{}      `json:"gmtCreate"`
	GmtModified              interface{}      `json:"gmtModified"`
	AuditDate                interface{}      `json:"auditDate"`
	TotalAmount              interface{}      `json:"totalAmount"`
	SkuCount                 interface{}      `json:"skuCount"`
	SourceNo                 string           `json:"sourceNo"`
	StockAllocateDetailViews []AllocateDetail `json:"stockAllocateDetailViews"`
}

type AllocateDetail struct {
	OutSkuCode  string      `json:"outSkuCode"`
	GoodsNo     string      `json:"goodsNo"`
	GoodsName   string      `json:"goodsName"`
	SkuName     string      `json:"skuName"`
	SkuBarcode  string      `json:"skuBarcode"`
	SkuCount    interface{} `json:"skuCount"`
	OutCount    interface{} `json:"outCount"`
	InCount     interface{} `json:"inCount"`
	SkuPrice    interface{} `json:"skuPrice"`
	TotalAmount interface{} `json:"totalAmount"`
}

type AllocateResult struct {
	TotalCount    int             `json:"totalCount"`
	StockAllocate []AllocateOrder `json:"stockAllocate"`
}

type PriceLookup struct {
	GoodsNoMap map[string]float64 // channel_key|goods_no → price
	BarcodeMap map[string]float64 // channel_key|barcode → price
}

func main() {
	configPath := flag.String("config", `C:\Users\Administrator\bi-dashboard\server\config.json`, "配置")
	startStr := flag.String("start", "", "开始日期 yyyy-MM-dd (默认 7 天前)")
	endStr := flag.String("end", "", "结束日期 yyyy-MM-dd (默认今天)")
	days := flag.Int("days", 7, "默认拉最近 N 天")
	flag.Parse()

	end := time.Now()
	if *endStr != "" {
		end, _ = time.Parse("2006-01-02", *endStr)
	}
	start := end.AddDate(0, 0, -*days)
	if *startStr != "" {
		start, _ = time.Parse("2006-01-02", *startStr)
	}

	bs, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatal("读 config 失败:", err)
	}
	var cfg Config
	if err := json.Unmarshal(bs, &cfg); err != nil {
		log.Fatal("解析 config 失败:", err)
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		cfg.Database.User, cfg.Database.Password,
		cfg.Database.Host, cfg.Database.Port, cfg.Database.Dbname)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal("连数据库失败:", err)
	}
	defer db.Close()

	// 加载 Excel 价格表
	priceLookup, err := loadPrices(db)
	if err != nil {
		log.Fatal("加载价格表失败:", err)
	}
	fmt.Printf("📊 价格表加载: 编码索引 %d 条, 条码索引 %d 条\n",
		len(priceLookup.GoodsNoMap), len(priceLookup.BarcodeMap))

	cli := jackyun.NewClient(cfg.Jackyun.Appkey, cfg.Jackyun.Secret, cfg.Jackyun.APIURL)

	totalOrders := 0
	totalDetails := 0
	for _, w := range warehouseMap {
		fmt.Printf("\n========================================\n")
		fmt.Printf("📦 拉仓 %s (%s)  时间 %s ~ %s\n",
			w.Code, w.ChannelName, start.Format("2006-01-02"), end.Format("2006-01-02"))
		fmt.Printf("========================================\n")

		orders, err := fetchAllocates(cli, start, end, w.Code)
		if err != nil {
			log.Printf("❌ 拉仓 %s 失败: %v", w.Code, err)
			continue
		}
		fmt.Printf("   → 拉到 %d 单\n", len(orders))

		for _, o := range orders {
			if err := upsertOrder(db, &o, w.ChannelKey, priceLookup); err != nil {
				log.Printf("❌ 写单 %s 失败: %v", o.AllocateNo, err)
				continue
			}
			totalOrders++
			totalDetails += len(o.StockAllocateDetailViews)
		}
	}

	fmt.Printf("\n🎯 同步完成: %d 单 / %d 行明细\n", totalOrders, totalDetails)
}

func fetchAllocates(cli *jackyun.Client, start, end time.Time, whCode string) ([]AllocateOrder, error) {
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
		if resp.Code != 200 {
			return nil, fmt.Errorf("api code=%d msg=%s", resp.Code, resp.Msg)
		}

		var w struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(resp.Result, &w); err != nil {
			return nil, fmt.Errorf("unmarshal result: %w", err)
		}
		var page AllocateResult
		if err := json.Unmarshal(w.Data, &page); err != nil {
			return nil, fmt.Errorf("unmarshal data: %w", err)
		}
		if len(page.StockAllocate) == 0 {
			break
		}
		all = append(all, page.StockAllocate...)
		pageIndex++
		if pageIndex > 200 {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	return all, nil
}

func upsertOrder(db *sql.DB, o *AllocateOrder, channelKey string, lookup *PriceLookup) error {
	gmtCreate := parseTime(o.GmtCreate)
	gmtModified := parseTime(o.GmtModified)
	auditDate := parseTime(o.AuditDate)

	var statDate *string
	inStatus := toInt(o.InStatus)
	if inStatus == 3 && !gmtModified.IsZero() {
		s := gmtModified.Format("2006-01-02")
		statDate = &s
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO allocate_orders
		(allocate_no, allocate_id, in_warehouse_code, in_warehouse_name, out_warehouse_code,
		 status, in_status, out_status, total_amount, sku_count, source_no,
		 gmt_create, gmt_modified, audit_date, stat_date, channel_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		in_warehouse_name=VALUES(in_warehouse_name), out_warehouse_code=VALUES(out_warehouse_code),
		status=VALUES(status), in_status=VALUES(in_status), out_status=VALUES(out_status),
		total_amount=VALUES(total_amount), sku_count=VALUES(sku_count), source_no=VALUES(source_no),
		gmt_modified=VALUES(gmt_modified), audit_date=VALUES(audit_date),
		stat_date=VALUES(stat_date), channel_key=VALUES(channel_key)`,
		o.AllocateNo, o.AllocateID, o.IntWarehouseCode, o.IntWarehouseName, o.OutWarehouseCode,
		toInt(o.Status), inStatus, toInt(o.OutStatus), toFloat(o.TotalAmount), toInt(o.SkuCount), o.SourceNo,
		nullTime(gmtCreate), nullTime(gmtModified), nullTime(auditDate), statDate, channelKey)
	if err != nil {
		return fmt.Errorf("upsert order: %w", err)
	}

	// 删旧明细再插, 防止接口去掉某行后旧数据残留
	if _, err := tx.Exec("DELETE FROM allocate_details WHERE allocate_no=?", o.AllocateNo); err != nil {
		return fmt.Errorf("clear old details: %w", err)
	}

	for _, d := range o.StockAllocateDetailViews {
		excelPrice, source := lookup.find(channelKey, d.GoodsNo, d.SkuBarcode)
		qty := toFloat(d.SkuCount)
		excelAmount := excelPrice * qty

		_, err := tx.Exec(`INSERT INTO allocate_details
			(allocate_no, out_sku_code, goods_no, goods_name, sku_name, sku_barcode,
			 sku_count, out_count, in_count, sku_price, total_amount,
			 excel_price, excel_amount, price_source, channel_key, stat_date, in_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			o.AllocateNo, d.OutSkuCode, d.GoodsNo, d.GoodsName, d.SkuName, d.SkuBarcode,
			qty, toFloat(d.OutCount), toFloat(d.InCount), toFloat(d.SkuPrice), toFloat(d.TotalAmount),
			excelPrice, excelAmount, source, channelKey, statDate, inStatus)
		if err != nil {
			return fmt.Errorf("insert detail: %w", err)
		}
	}

	return tx.Commit()
}

func loadPrices(db *sql.DB) (*PriceLookup, error) {
	rows, err := db.Query("SELECT channel_key, goods_no, barcode, price FROM channel_special_price")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lookup := &PriceLookup{
		GoodsNoMap: map[string]float64{},
		BarcodeMap: map[string]float64{},
	}
	for rows.Next() {
		var ck, gn, bc string
		var price float64
		if err := rows.Scan(&ck, &gn, &bc, &price); err != nil {
			return nil, err
		}
		if gn != "" {
			lookup.GoodsNoMap[ck+"|"+gn] = price
		}
		if bc != "" {
			lookup.BarcodeMap[ck+"|"+bc] = price
		}
	}
	return lookup, nil
}

func (l *PriceLookup) find(channelKey, goodsNo, barcode string) (float64, string) {
	if p, ok := l.GoodsNoMap[channelKey+"|"+goodsNo]; ok {
		return p, "excel"
	}
	if barcode != "" {
		if p, ok := l.BarcodeMap[channelKey+"|"+barcode]; ok {
			return p, "excel"
		}
	}
	return 0, "missing"
}

// parseTime 兼容多种时间格式: unix ms / "2006-01-02 15:04:05" / float
func parseTime(v interface{}) time.Time {
	switch x := v.(type) {
	case string:
		if x == "" {
			return time.Time{}
		}
		// 先按字符串解析 yyyy-MM-dd HH:mm:ss
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", x, time.Local); err == nil {
			return t
		}
		// 不行就试 unix ms
		if ms, err := strconv.ParseInt(x, 10, 64); err == nil {
			return time.Unix(0, ms*int64(time.Millisecond))
		}
	case float64:
		ms := int64(x)
		if ms > 1e12 {
			return time.Unix(0, ms*int64(time.Millisecond))
		}
		return time.Unix(ms, 0)
	case int64:
		return time.Unix(0, x*int64(time.Millisecond))
	case json.Number:
		ms, _ := x.Int64()
		if ms > 1e12 {
			return time.Unix(0, ms*int64(time.Millisecond))
		}
		return time.Unix(ms, 0)
	}
	return time.Time{}
}

func toFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	case json.Number:
		f, _ := x.Float64()
		return f
	}
	return 0
}

func toInt(v interface{}) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	}
	return 0
}

func nullTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}
