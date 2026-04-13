package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/jackyun"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	cfg, _ := config.Load("config.json")
	client := jackyun.NewClient(cfg.JackYun.AppKey, cfg.JackYun.Secret, cfg.JackYun.APIURL)
	db, _ := sql.Open("mysql", cfg.Database.DSN())
	defer db.Close()

	client2 := jackyun.NewClient(cfg.JackYunTrade.AppKey, cfg.JackYunTrade.Secret, cfg.JackYun.APIURL)

	fmt.Println("========== 1. erp.stockquantity.get => stock_quantity ==========")
	checkAPI(client, "erp.stockquantity.get", map[string]interface{}{
		"pageIndex": 0, "pageSize": 2, "isBlockup": 2, "isNotQueryBatchStock": "1",
	}, "goodsStockQuantity", db, "stock_quantity")

	fmt.Println("\n========== 2. erp.batchstockquantity.get => stock_batch ==========")
	var whCode string
	db.QueryRow("SELECT warehouse_code FROM stock_quantity WHERE current_qty>0 AND warehouse_code!='' LIMIT 1").Scan(&whCode)
	checkAPI(client, "erp.batchstockquantity.get", map[string]interface{}{
		"warehouseCode": whCode, "pageIndex": 0, "pageSize": 2,
	}, "goodsStockQuantity", db, "stock_batch")

	fmt.Println("\n========== 3. erp.sales.get => sales_channel ==========")
	checkAPI(client, "erp.sales.get", map[string]interface{}{
		"pageIndex": 0, "pageSize": 2,
	}, "salesChannelList", db, "sales_channel")

	fmt.Println("\n========== 4. erp.storage.goodslist => goods ==========")
	checkAPI(client, "erp.storage.goodslist", map[string]interface{}{
		"pageIndex": 0, "pageSize": 2,
	}, "goodsInfoList", db, "goods")

	fmt.Println("\n========== 5. birc.report.salesGoodsSummary => sales_goods_summary ==========")
	checkSummaryAPI(client, db)

	fmt.Println("\n========== 6. trade (定制接口) => trade_YYYYMM + trade_goods_YYYYMM ==========")
	checkTradeAPI(client2, db)
}

func checkAPI(client *jackyun.Client, method string, biz map[string]interface{}, listKey string, db *sql.DB, table string) {
	resp, err := client.Call(method, biz)
	if err != nil {
		log.Printf("调用失败: %v", err)
		return
	}
	if resp.Code != 200 {
		log.Printf("接口错误: %d %s", resp.Code, resp.Msg)
		return
	}

	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	json.Unmarshal(resp.Result, &wrapper)
	var dataBytes []byte
	var dataStr string
	if err := json.Unmarshal(wrapper.Data, &dataStr); err == nil {
		dataBytes = []byte(dataStr)
	} else {
		dataBytes = wrapper.Data
	}

	var raw map[string]json.RawMessage
	json.Unmarshal(dataBytes, &raw)

	var items []map[string]interface{}
	if listData, ok := raw[listKey]; ok {
		json.Unmarshal(listData, &items)
	}
	if len(items) == 0 {
		fmt.Println("无数据")
		return
	}

	apiFields := collectKeys(items[0], "")
	dbFields := getDBColumns(db, table)
	printDiff(apiFields, dbFields)
}

func checkSummaryAPI(client *jackyun.Client, db *sql.DB) {
	query := jackyun.SalesSummaryQuery{
		TimeType: 3, StartTime: "2026-03-01", EndTime: "2026-03-01",
		FilterTimeType: 2, AssemblyDimension: 1, IsSkuStatistic: 1,
		SummaryType: "1,2,5", PageIndex: 0, PageSize: 2,
		IsCancelTrade: "0", IsAssembly: "2",
	}
	resp, err := client.Call("birc.report.salesGoodsSummary", query)
	if err != nil {
		log.Printf("调用失败: %v", err)
		return
	}
	if resp.Code != 200 {
		log.Printf("接口错误: %d %s", resp.Code, resp.Msg)
		return
	}

	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	json.Unmarshal(resp.Result, &wrapper)
	var items []map[string]interface{}
	json.Unmarshal(wrapper.Data, &items)
	if len(items) == 0 {
		fmt.Println("无数据")
		return
	}

	apiFields := collectKeys(items[0], "")
	dbFields := getDBColumns(db, "sales_goods_summary")
	printDiff(apiFields, dbFields)
}

func checkTradeAPI(client *jackyun.Client, db *sql.DB) {
	fields := "tradeNo,tradeStatus,tradeStatusExplain,tradeType,shopName,shopId,shopcode,warehouseName,warehouseId,warehouseCode," +
		"goodsDetail.goodsNo,goodsDetail.goodsName,goodsDetail.goodsId,goodsDetail.sellCount," +
		"goodsDetail.sellPrice,goodsDetail.sellTotal,goodsDetail.specName,goodsDetail.barcode," +
		"goodsDetail.cost,goodsDetail.cateName,goodsDetail.brandName,goodsDetail.unit," +
		"goodsDetail.subTradeId,goodsDetail.isFit,goodsDetail.isGift,goodsDetail.specId," +
		"goodsDetail.discountFee,goodsDetail.taxFee,goodsDetail.goodsMemo,goodsDetail.outerId,goodsDetail.platGoodsId," +
		"goodsDetail.platSkuId,goodsDetail.skuImgUrl,goodsDetail.divideSellTotal,goodsDetail.goodsSeller," +
		"sourceTradeNo,onlineTradeNo,scrollId,billDate,consignTime,tradeTime,orderNo,customerName," +
		"logisticName,mainPostid,checkTotal,totalFee,payment,postFee,discountFee,otherFee," +
		"tradeCount,isDelete,tradeFrom,sellerMemo,buyerMemo," +
		"payTime,payType,payStatus,payNo,chargeCurrency," +
		"grossProfit,couponFee,realFee,taxFee,taxRate," +
		"departName,companyName,gmtCreate,gmtModified," +
		"auditTime,completeTime,flagIds,flagNames," +
		"state,city,district,town,country,zip"

	biz := map[string]interface{}{
		"startConsignTime": "2026-03-01 00:00:00",
		"endConsignTime":   "2026-03-01 23:59:59",
		"isDelete":         "0",
		"pageIndex":        0,
		"pageSize":         2,
		"hasTotal":         1,
		"fields":           fields,
	}

	resp, err := client.Call("jackyun.tradenotsensitiveinfos.list.get", biz)
	if err != nil {
		log.Printf("调用失败: %v", err)
		return
	}
	if resp.Code != 200 {
		log.Printf("接口错误: %d %s", resp.Code, resp.Msg)
		return
	}

	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	json.Unmarshal(resp.Result, &wrapper)
	var dataBytes []byte
	var dataStr string
	if err := json.Unmarshal(wrapper.Data, &dataStr); err == nil {
		dataBytes = []byte(dataStr)
	} else {
		dataBytes = wrapper.Data
	}
	var result struct {
		Trades []map[string]interface{} `json:"Trades"`
	}
	json.Unmarshal(dataBytes, &result)
	if len(result.Trades) == 0 {
		fmt.Println("无数据")
		return
	}

	trade := result.Trades[0]

	// 主表字段（排除嵌套对象）
	fmt.Println("\n--- 主表 trade_YYYYMM ---")
	mainFields := []string{}
	for k, v := range trade {
		if k == "GoodsDetail" || k == "ScrollId" {
			continue
		}
		switch v.(type) {
		case []interface{}, map[string]interface{}:
			continue
		}
		mainFields = append(mainFields, k)
	}
	sort.Strings(mainFields)
	dbTradeFields := getDBColumns(db, "trade_202601")
	printDiff(mainFields, dbTradeFields)

	// 明细字段
	fmt.Println("\n--- 明细 trade_goods_YYYYMM ---")
	if gd, ok := trade["GoodsDetail"]; ok {
		if gdList, ok := gd.([]interface{}); ok && len(gdList) > 0 {
			if g, ok := gdList[0].(map[string]interface{}); ok {
				goodsFields := collectKeys(g, "")
				dbGoodsFields := getDBColumns(db, "trade_goods_202601")
				printDiff(goodsFields, dbGoodsFields)
			}
		}
	}
}

func collectKeys(m map[string]interface{}, prefix string) []string {
	keys := []string{}
	for k := range m {
		keys = append(keys, prefix+k)
	}
	sort.Strings(keys)
	return keys
}

func getDBColumns(db *sql.DB, table string) []string {
	rows, err := db.Query("SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? ORDER BY ORDINAL_POSITION", table)
	if err != nil {
		return nil
	}
	defer rows.Close()
	cols := []string{}
	for rows.Next() {
		var c string
		rows.Scan(&c)
		cols = append(cols, c)
	}
	return cols
}

func printDiff(apiFields, dbFields []string) {
	dbSet := map[string]bool{}
	for _, f := range dbFields {
		dbSet[strings.ToLower(f)] = true
	}

	fmt.Printf("API返回 %d 个字段, DB有 %d 个列\n", len(apiFields), len(dbFields))
	missing := []string{}
	for _, f := range apiFields {
		snake := camelToSnake(f)
		if !dbSet[snake] && !dbSet[strings.ToLower(f)] {
			missing = append(missing, fmt.Sprintf("  %s (=> %s)", f, snake))
		}
	}
	if len(missing) > 0 {
		fmt.Printf("API有但DB没有的字段(%d个):\n", len(missing))
		for _, m := range missing {
			fmt.Println(m)
		}
	} else {
		fmt.Println("全部覆盖")
	}
}

func camelToSnake(s string) string {
	var result []byte
	for i, c := range s {
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				result = append(result, '_')
			}
			result = append(result, byte(c)+32)
		} else {
			result = append(result, byte(c))
		}
	}
	return string(result)
}
