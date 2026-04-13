package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/jackyun"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

// DetailTrade 带商品明细的订单
type DetailTrade struct {
	TradeId     json.Number   `json:"tradeId"`
	TradeNo     string        `json:"tradeNo"`
	GoodsDetail []GoodsDetail `json:"goodsDetail"`
}

type GoodsDetail struct {
	SubTradeId   json.Number `json:"subTradeId"`
	GoodsId      json.Number `json:"goodsId"`
	GoodsNo      string      `json:"goodsNo"`
	GoodsName    string      `json:"goodsName"`
	SpecId       json.Number `json:"specId"`
	SpecName     string      `json:"specName"`
	Barcode      string      `json:"barcode"`
	SellCount    json.Number `json:"sellCount"`
	SellPrice    json.Number `json:"sellPrice"`
	SellTotal    json.Number `json:"sellTotal"`
	Cost         json.Number `json:"cost"`
	DiscountFee  json.Number `json:"discountFee"`
	TaxFee       json.Number `json:"taxFee"`
	CateName     string      `json:"cateName"`
	BrandName    string      `json:"brandName"`
	Unit         string      `json:"unit"`
	IsGift       json.Number `json:"isGift"`
	IsFit        json.Number `json:"isFit"`
}

const batchSize = 30 // 每批查30个订单

func main() {
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(20)

	client := jackyun.NewClient(cfg.JackYun.AppKey, cfg.JackYun.Secret, cfg.JackYun.APIURL)

	tableMonth := "202601"

	// 第一步：从数据库读取所有待拆明细的订单（排除仅退款type=12）
	rows, err := db.Query(fmt.Sprintf(
		"SELECT trade_no, trade_id, shop_id, bill_date, trade_type FROM trade_%s WHERE trade_type != 12 ORDER BY id", tableMonth))
	if err != nil {
		log.Fatalf("查询订单失败: %v", err)
	}

	type OrderInfo struct {
		TradeNo   string
		TradeId   string
		ShopId    sql.NullString
		BillDate  sql.NullString
		TradeType sql.NullInt64
	}

	var orders []OrderInfo
	for rows.Next() {
		var o OrderInfo
		rows.Scan(&o.TradeNo, &o.TradeId, &o.ShopId, &o.BillDate, &o.TradeType)
		orders = append(orders, o)
	}
	rows.Close()

	fmt.Printf("共 %d 个订单需要拆明细，每批 %d 个\n\n", len(orders), batchSize)

	// 建立 tradeNo -> OrderInfo 的映射
	orderMap := make(map[string]OrderInfo)
	for _, o := range orders {
		orderMap[o.TradeNo] = o
	}

	// goodsDetail 需要的 fields
	fields := "tradeNo,tradeId," +
		"goodsDetail.subTradeId,goodsDetail.goodsId,goodsDetail.goodsNo,goodsDetail.goodsName," +
		"goodsDetail.specId,goodsDetail.specName,goodsDetail.barcode," +
		"goodsDetail.sellCount,goodsDetail.sellPrice,goodsDetail.sellTotal," +
		"goodsDetail.cost,goodsDetail.discountFee,goodsDetail.taxFee," +
		"goodsDetail.cateName,goodsDetail.brandName,goodsDetail.unit," +
		"goodsDetail.isGift,goodsDetail.isFit"

	totalDetail := 0
	noDetailCount := 0

	// 第二步：分批查询
	for i := 0; i < len(orders); i += batchSize {
		end := i + batchSize
		if end > len(orders) {
			end = len(orders)
		}
		batch := orders[i:end]

		// 拼接 tradeNo
		tradeNos := make([]string, len(batch))
		for j, o := range batch {
			tradeNos[j] = o.TradeNo
		}

		biz := map[string]interface{}{
			"tradeNo":  strings.Join(tradeNos, ","),
			"pageIndex": 0,
			"pageSize":  batchSize,
			"hasTotal":  1,
			"fields":    fields,
		}

		resp, err := client.Call("oms.trade.fullinfoget", biz)
		if err != nil {
			log.Printf("批次%d调用失败: %v", i/batchSize, err)
			continue
		}
		if resp.Code != 200 {
			log.Printf("批次%d接口报错: code=%d msg=%s", i/batchSize, resp.Code, resp.Msg)
			continue
		}

		var wrapper struct {
			Data json.RawMessage `json:"data"`
		}
		json.Unmarshal(resp.Result, &wrapper)

		var result struct {
			Trades []DetailTrade `json:"trades"`
		}
		if err := json.Unmarshal(wrapper.Data, &result); err != nil {
			log.Printf("批次%d解析失败: %v", i/batchSize, err)
			continue
		}

		// 写入明细
		batchDetail := 0
		for _, t := range result.Trades {
			info := orderMap[t.TradeNo]

			if len(t.GoodsDetail) == 0 {
				noDetailCount++
				continue
			}

			for _, g := range t.GoodsDetail {
				_, err := db.Exec(fmt.Sprintf(`
					INSERT INTO trade_goods_%s
						(trade_id, trade_no, sub_trade_id, goods_id, goods_no, goods_name,
						 spec_id, spec_name, barcode,
						 sell_count, sell_price, sell_total, cost, discount_fee, tax_fee,
						 category_name, brand_name, unit, is_gift, is_fit,
						 shop_id, bill_date, trade_type)
					VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
				ON DUPLICATE KEY UPDATE sell_count=VALUES(sell_count), sell_price=VALUES(sell_price), sell_total=VALUES(sell_total), cost=VALUES(cost)`, tableMonth),
					info.TradeId, t.TradeNo,
					ns(g.SubTradeId), ns(g.GoodsId), g.GoodsNo, g.GoodsName,
					ns(g.SpecId), g.SpecName, g.Barcode,
					nf(g.SellCount), nf(g.SellPrice), nf(g.SellTotal),
					nf(g.Cost), nf(g.DiscountFee), nf(g.TaxFee),
					ne(g.CateName), ne(g.BrandName), ne(g.Unit),
					ni(g.IsGift), ni(g.IsFit),
					nullStr(info.ShopId), nullStr(info.BillDate), info.TradeType.Int64,
				)
				if err != nil {
					log.Printf("写入明细 %s 失败: %v", t.TradeNo, err)
				}
				batchDetail++
			}
		}

		totalDetail += batchDetail
		fmt.Printf("  批次 %d/%d: 查了%d单, 返回%d单, 写入%d条明细 (累计%d)\n",
			i/batchSize+1, (len(orders)+batchSize-1)/batchSize,
			len(batch), len(result.Trades), batchDetail, totalDetail)
	}

	fmt.Printf("\n拆分完成！共 %d 条明细, %d 个订单无明细\n", totalDetail, noDetailCount)
}

func ns(n json.Number) string { return n.String() }
func ni(n json.Number) interface{} {
	s := n.String()
	if s == "" { return 0 }
	return s
}
func nf(n json.Number) interface{} {
	s := n.String()
	if s == "" { return 0 }
	return s
}
func ne(s string) interface{} {
	if s == "" { return nil }
	return s
}
func nullStr(s sql.NullString) interface{} {
	if !s.Valid || s.String == "" { return nil }
	return s.String
}
