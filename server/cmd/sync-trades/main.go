package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/jackyun"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// TradeRow 接口返回的销售单（宽松解析，全用 json.RawMessage 再手动取值）
type TradeRow struct {
	TradeId            json.Number `json:"tradeId"`
	TradeNo            string      `json:"tradeNo"`
	OrderNo            string      `json:"orderNo"`
	SourceTradeNo      string      `json:"sourceTradeNo"`
	TradeStatus        json.Number `json:"tradeStatus"`
	TradeStatusExplain string      `json:"tradeStatusExplain"`
	TradeType          json.Number `json:"tradeType"`
	ShopId             json.Number `json:"shopId"`
	ShopName           string      `json:"shopName"`
	WarehouseId        json.Number `json:"warehouseId"`
	WarehouseName      string      `json:"warehouseName"`
	PayType            json.Number `json:"payType"`
	PayNo              string      `json:"payNo"`
	ChargeCurrency     string      `json:"chargeCurrency"`
	CheckTotal         json.Number `json:"checkTotal"`
	TotalFee           json.Number `json:"totalFee"`
	Payment            json.Number `json:"payment"`
	PostFee            json.Number `json:"postFee"`
	DiscountFee        json.Number `json:"discountFee"`
	OtherFee           json.Number `json:"otherFee"`
	TradeCount         json.Number `json:"tradeCount"`
	SellerMemo         string      `json:"sellerMemo"`
	BuyerMemo          string      `json:"buyerMemo"`
	TradeFrom          json.Number `json:"tradeFrom"`
	TradeTime          string      `json:"tradeTime"`
	BillDate           string      `json:"billDate"`
	ConsignTime        string      `json:"consignTime"`
	GmtCreate          string      `json:"gmtCreate"`
	AuditTime          string      `json:"auditTime"`
	CompleteTime       string      `json:"completeTime"`
	GmtModified        string      `json:"gmtModified"`
	LogisticName       string      `json:"logisticName"`
	MainPostid         string      `json:"mainPostid"`
	CustomerName       string      `json:"customerName"`
	IsDelete           json.Number `json:"isDelete"`
	ScrollId           string      `json:"scrollId"`
}

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

	// 同步2月整月
	startTime, _ := time.Parse("2006-01-02 15:04:05", "2026-02-01 00:00:00")
	endTime, _ := time.Parse("2006-01-02 15:04:05", "2026-02-28 23:59:59")
	tableMonth := "202602"

	fmt.Printf("开始同步销售单列表: %s ~ %s\n", startTime.Format("2006-01-02"), endTime.Format("2006-01-02"))
	fmt.Printf("目标表: trade_%s（只拉订单列表，不拆明细）\n\n", tableMonth)

	// 只请求主表字段，不请求 goodsDetail
	fields := "tradeNo,orderNo,sourceTradeNo,tradeStatus,tradeStatusExplain,tradeType," +
		"shopId,shopName,warehouseId,warehouseName," +
		"payType,payNo,chargeCurrency,checkTotal,totalFee,payment,postFee,discountFee,otherFee," +
		"tradeCount,sellerMemo,buyerMemo,tradeFrom," +
		"tradeTime,billDate,consignTime,auditTime,completeTime," +
		"logisticName,mainPostid,customerName,isDelete,scrollId"

	total := 0

	// 按天循环（接口限制时间跨度不能超过7天）
	dayStart := startTime
	for dayStart.Before(endTime) {
		dayEnd := dayStart.Add(24*time.Hour - time.Second)
		if dayEnd.After(endTime) {
			dayEnd = endTime
		}

		fmt.Printf("[%s] 拉取中...\n", dayStart.Format("2006-01-02"))
		scrollId := ""
		pageIndex := 0
		dayTotal := 0

		for {
			biz := map[string]interface{}{
				"startConsignTime": dayStart.Format("2006-01-02 15:04:05"),
				"endConsignTime":   dayEnd.Format("2006-01-02 15:04:05"),
				"pageIndex":        pageIndex,
				"pageSize":         200,
				"hasTotal":         1,
				"fields":           fields,
			}
			if scrollId != "" {
				biz["scrollId"] = scrollId
			}

			resp, err := client.Call("oms.trade.fullinfoget", biz)
			if err != nil {
				log.Printf("  第%d页调用失败: %v", pageIndex, err)
				break
			}
			if resp.Code != 200 {
				log.Printf("  第%d页接口报错: code=%d msg=%s", pageIndex, resp.Code, resp.Msg)
				break
			}

			var wrapper struct {
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(resp.Result, &wrapper); err != nil {
				log.Printf("  解析result失败: %v", err)
				break
			}

			var result struct {
				TotalResults int        `json:"totalResults"`
				Trades       []TradeRow `json:"trades"`
			}
			if err := json.Unmarshal(wrapper.Data, &result); err != nil {
				log.Printf("  第%d页解析data失败: %v", pageIndex, err)
				break
			}

			if len(result.Trades) == 0 {
				break
			}

			for _, t := range result.Trades {
				_, err := db.Exec(fmt.Sprintf(`
					INSERT INTO trade_%s
						(trade_id, trade_no, order_no, source_trade_no,
						 trade_status, trade_status_explain, trade_type,
						 shop_id, shop_name, warehouse_id, warehouse_name,
						 pay_type, pay_no, charge_currency,
						 check_total, total_fee, payment, post_fee, discount_fee, other_fee,
						 trade_count, seller_memo, buyer_memo, trade_from,
						 trade_time, bill_date, consign_time, created_time, audit_time,
						 complete_time, modified_time,
						 logistic_name, main_postid, customer_name, is_delete)
					VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
					ON DUPLICATE KEY UPDATE
						trade_status=VALUES(trade_status), trade_type=VALUES(trade_type),
						modified_time=VALUES(modified_time)`, tableMonth),
					ns(t.TradeId), t.TradeNo, ne(t.OrderNo), ne(t.SourceTradeNo),
					ni(t.TradeStatus), ne(t.TradeStatusExplain), ni(t.TradeType),
					ns(t.ShopId), t.ShopName, ns(t.WarehouseId), t.WarehouseName,
					ni(t.PayType), ne(t.PayNo), ne(t.ChargeCurrency),
					nf(t.CheckTotal), nf(t.TotalFee), nf(t.Payment), nf(t.PostFee), nf(t.DiscountFee), nf(t.OtherFee),
					ni(t.TradeCount), ne(t.SellerMemo), ne(t.BuyerMemo), ns(t.TradeFrom),
					ne(t.TradeTime), ne(t.BillDate), ne(t.ConsignTime), ne(t.GmtCreate), ne(t.AuditTime),
					ne(t.CompleteTime), ne(t.GmtModified),
					ne(t.LogisticName), ne(t.MainPostid), ne(t.CustomerName), ni(t.IsDelete),
				)
				if err != nil {
					log.Printf("写入 %s 失败: %v", t.TradeNo, err)
				}
			}

			dayTotal += len(result.Trades)

			if result.TotalResults > 0 && dayTotal >= result.TotalResults {
				break
			}
			if len(result.Trades) < 200 {
				break
			}

			lastTrade := result.Trades[len(result.Trades)-1]
			if lastTrade.ScrollId != "" {
				scrollId = lastTrade.ScrollId
			}
			pageIndex++
		}

		total += dayTotal
		fmt.Printf("  完成 %d 条 (累计 %d)\n", dayTotal, total)
		dayStart = dayStart.Add(24 * time.Hour)
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Printf("\n同步完成！共 %d 笔订单\n", total)

	// 统计
	var orderCount int
	db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM trade_%s", tableMonth)).Scan(&orderCount)
	fmt.Printf("数据库中订单数: %d\n", orderCount)

	// 按订单类型统计
	rows, _ := db.Query(fmt.Sprintf("SELECT trade_type, trade_status_explain, COUNT(*) as cnt FROM trade_%s GROUP BY trade_type, trade_status_explain ORDER BY cnt DESC LIMIT 20", tableMonth))
	if rows != nil {
		defer rows.Close()
		fmt.Println("\n按订单类型统计:")
		for rows.Next() {
			var tt, explain sql.NullString
			var cnt int
			rows.Scan(&tt, &explain, &cnt)
			fmt.Printf("  类型=%s 状态=%s: %d\n", tt.String, explain.String, cnt)
		}
	}
}

// ns: json.Number to string
func ns(n json.Number) string { return n.String() }

// ni: json.Number to nullable int
func ni(n json.Number) interface{} {
	s := n.String()
	if s == "" || s == "0" {
		return 0
	}
	return s
}

// nf: json.Number to nullable float
func nf(n json.Number) interface{} {
	s := n.String()
	if s == "" {
		return 0
	}
	return s
}

// ne: string to nullable
func ne(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
