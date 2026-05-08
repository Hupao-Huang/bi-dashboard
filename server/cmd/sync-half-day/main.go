// sync-half-day: 按半天拉取销售单（绕过吉客云某些日期单天报错的问题）
// 用法: sync-half-day 2026-03-03
package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"bi-dashboard/internal/jackyun"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	unlock := importutil.AcquireLock("sync-half-day")
	defer unlock()

	if len(os.Args) < 2 {
		log.Fatal("用法: sync-half-day YYYY-MM-DD")
	}
	dateStr := os.Args[1]
	d, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		log.Fatalf("日期格式错误: %v", err)
	}
	tableMonth := d.Format("200601")

	cfg, err := config.Load("C:\\Users\\Administrator\\bi-dashboard\\server\\config.json")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(20)
	client := jackyun.NewClient(cfg.JackYunTrade.AppKey, cfg.JackYunTrade.Secret, cfg.JackYun.APIURL)

	// 时间区间：按小时拆分全天
	var windows [][]string
	for h := 0; h < 24; h++ {
		windows = append(windows, []string{
			fmt.Sprintf("%s %02d:00:00", dateStr, h),
			fmt.Sprintf("%s %02d:59:59", dateStr, h),
		})
	}

	fields := "tradeNo,tradeStatus,tradeStatusExplain,tradeType,shopName,shopId,shopcode,warehouseName,warehouseId,warehouseCode," +
		"goodsDetail.goodsNo,goodsDetail.goodsName,goodsDetail.goodsId,goodsDetail.sellCount,goodsDetail.sellPrice,goodsDetail.sellTotal," +
		"goodsDetail.specName,goodsDetail.barcode,goodsDetail.cost,goodsDetail.cateName,goodsDetail.brandName,goodsDetail.unit," +
		"goodsDetail.subTradeId,goodsDetail.isFit,goodsDetail.isGift,goodsDetail.specId,goodsDetail.discountFee,goodsDetail.taxFee," +
		"goodsDetail.goodsMemo,goodsDetail.outerId,goodsDetail.platGoodsId,goodsDetail.platSkuId,goodsDetail.skuImgUrl," +
		"goodsDetail.divideSellTotal,goodsDetail.goodsSeller,goodsDetail.goodsPlatDiscountFee,goodsDetail.isPresell," +
		"goodsDetail.shareOrderDiscountFee,goodsDetail.shareOrderPlatDiscountFee," +
		"sourceTradeNo,onlineTradeNo,scrollId,billDate,consignTime,tradeTime,orderNo,customerName," +
		"logisticName,mainPostid,checkTotal,totalFee,payment,postFee,discountFee,otherFee," +
		"tradeCount,isDelete,tradeFrom,sellerMemo,buyerMemo,payTime,payType,payStatus,payNo,chargeCurrency," +
		"grossProfit,couponFee,realFee,taxFee,taxRate,departName,companyName,gmtCreate,gmtModified," +
		"auditTime,completeTime,flagIds,flagNames,state,city,district,town,country,zip," +
		"customerDiscountFee,customerPostFee,customerDiscount,customerTotalFee," +
		"customerAccount,customerCode,customerGradeName,customerTags,buyerOpenUid,blackList," +
		"invoiceAmount,invoiceType,invoiceCode,chargeExchangeRate,chargeCurrencyCode,localCurrencyCode," +
		"firstPayment,finalPayment,receivedTotal,firstPaytime,finalPaytime,finReceiptTime," +
		"payerName,payerPhone,payerRegno,payerBankAccount,payerBankName," +
		"logisticCode,logisticType,extraLogisticNo,packageWeight,estimateWeight,estimateVolume," +
		"stockoutNo,lastShipTime,signingTime,reviewTime,confirmTime,activationTime,notifyPickTime," +
		"settleAuditTime,platCompleteTime,reviewer,auditor,register,seller," +
		"shopTypeCode,agentShopName,sourceAfterNo,countryCode,cityCode," +
		"sysFlagIds,specialReminding,abnormalDescription,appendMemo,ticketCodeList,allCompassSourceContentType," +
		"packageDetail.logisticNo,packageDetail.logisticName,packageDetail.logisticCode," +
		"packageDetail.warehouseName,packageDetail.sellCount,packageDetail.isGift,packageDetail.isPlatGift," +
		"packageDetail.barcode,packageDetail.tradeNo,packageDetail.sourceTradeNo,packageDetail.buyerMemo,packageDetail.sellerMemo"

	totalAll := 0
	for _, w := range windows {
		log.Printf("\n=== 拉取窗口 %s ~ %s ===", w[0], w[1])
		scrollId := ""
		windowTotal := 0
		page := 0
		for {
			biz := map[string]interface{}{
				"startConsignTime": w[0],
				"endConsignTime":   w[1],
				"isDelete":         "0",
				"pageSize":         200,
				"scrollId":         scrollId,
				"fields":           fields,
			}
			var resp *jackyun.APIResponse
			for retry := 0; retry < 5; retry++ {
				resp, err = client.Call("jackyun.tradenotsensitiveinfos.list.get", biz)
				if err == nil {
					break
				}
				time.Sleep(time.Duration((retry+1)*10) * time.Second)
				log.Printf("调用失败重试 %d/5: %v", retry+1, err)
			}
			if err != nil {
				log.Printf("放弃此窗口")
				break
			}
			if resp.Code != 200 {
				log.Printf("接口报错 page=%d code=%d msg=%s", page, resp.Code, resp.Msg)
				break
			}
			var wrapper struct {
				Data     json.RawMessage `json:"data"`
				ScrollId string          `json:"scrollId"`
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
				Trades   []json.RawMessage `json:"Trades"`
				ScrollId string            `json:"ScrollId"`
			}
			if err := json.Unmarshal(dataBytes, &result); err != nil {
				log.Printf("解析失败: %v", err)
				break
			}
			if len(result.Trades) == 0 {
				break
			}

			for _, raw := range result.Trades {
				var t map[string]interface{}
				dec := json.NewDecoder(bytes.NewReader(raw))
				dec.UseNumber()
				dec.Decode(&t)
				tradeId := gs(t, "TradeId")
				tradeNo := gs(t, "TradeNo")
				if gs(t, "IsDelete") == "1" {
					continue
				}
				// 只写主表 + 明细 + 包裹（核心数据）
				err = writeTrade(db, tableMonth, tradeId, tradeNo, t)
				if err != nil {
					log.Printf("写入失败 %s: %v", tradeNo, err)
					continue
				}
				windowTotal++
			}

			if len(result.Trades) < 200 {
				break
			}
			// 游标翻页：先wrapper层，再data层
			if wrapper.ScrollId != "" {
				scrollId = wrapper.ScrollId
			} else if result.ScrollId != "" {
				scrollId = result.ScrollId
			} else {
				log.Printf("[警告] 无 scrollId 返回, 停止翻页 (窗口 %s ~ %s, page=%d, 累计 %d 条; 可能漏数据, 请按更小时段补拉)", w[0], w[1], page, windowTotal)
				break
			}
			page++
			log.Printf("窗口已拉 %d 条 (page=%d)", windowTotal, page)
		}
		log.Printf("窗口完成: %d 条", windowTotal)
		totalAll += windowTotal
	}

	log.Printf("\n=== 半天拉取完成: 共 %d 条 ===", totalAll)
}

func writeTrade(db *sql.DB, tableMonth, tradeId, tradeNo string, t map[string]interface{}) error {
	// 简化版：只插入主表 + 明细，使用 ON DUPLICATE KEY UPDATE 幂等
	_, err := db.Exec(fmt.Sprintf(`INSERT INTO trade_%s
		(trade_id, trade_no, order_no, source_trade_no, online_trade_no,
		 trade_status, trade_status_explain, trade_type,
		 shop_id, shop_name, shop_code, warehouse_id, warehouse_name, warehouse_code,
		 pay_type, pay_no, charge_currency, pay_time, pay_status,
		 check_total, total_fee, payment, post_fee, discount_fee, other_fee,
		 gross_profit, coupon_fee, real_fee, tax_fee, tax_rate,
		 trade_count, trade_from, seller_memo, buyer_memo,
		 trade_time, bill_date, consign_time, audit_time, complete_time,
		 gmt_create, gmt_modified,
		 logistic_name, main_postid, customer_name, depart_name, company_name,
		 is_delete, flag_ids, flag_names,
		 state, city, district, town, country, zip)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
			shop_id=VALUES(shop_id), shop_name=VALUES(shop_name),
			payment=VALUES(payment), check_total=VALUES(check_total),
			total_fee=VALUES(total_fee),
			gmt_modified=VALUES(gmt_modified)`, tableMonth),
		tradeId, tradeNo, gn(t, "OrderNo"), gn(t, "SourceTradeNo"), gn(t, "OnlineTradeNo"),
		gn(t, "TradeStatus"), gn(t, "TradeStatusExplain"), gn(t, "TradeType"),
		gs(t, "ShopId"), gn(t, "ShopName"), gn(t, "Shopcode"), gs(t, "WarehouseId"), gn(t, "WarehouseName"), gn(t, "WarehouseCode"),
		gn(t, "PayType"), gn(t, "PayNo"), gn(t, "ChargeCurrency"), gn(t, "PayTime"), gn(t, "PayStatus"),
		gn(t, "CheckTotal"), gn(t, "TotalFee"), gn(t, "Payment"), gn(t, "PostFee"), gn(t, "DiscountFee"), gn(t, "OtherFee"),
		gn(t, "GrossProfit"), gn(t, "CouponFee"), gn(t, "RealFee"), gn(t, "TaxFee"), gn(t, "TaxRate"),
		gn(t, "TradeCount"), gn(t, "TradeFrom"), gn(t, "SellerMemo"), gn(t, "BuyerMemo"),
		gn(t, "TradeTime"), gn(t, "BillDate"), gn(t, "ConsignTime"), gn(t, "AuditTime"), gn(t, "CompleteTime"),
		gn(t, "GmtCreate"), gn(t, "GmtModified"),
		gn(t, "LogisticName"), gn(t, "MainPostid"), gn(t, "CustomerName"), gn(t, "DepartName"), gn(t, "CompanyName"),
		gn(t, "IsDelete"), gn(t, "FlagIds"), gn(t, "FlagNames"),
		gn(t, "State"), gn(t, "City"), gn(t, "District"), gn(t, "Town"), gn(t, "Country"), gn(t, "Zip"),
	)
	if err != nil {
		return err
	}

	// 写商品明细
	if gd, ok := t["GoodsDetail"]; ok && gd != nil {
		if gdList, ok := gd.([]interface{}); ok {
			for _, item := range gdList {
				g, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				db.Exec(fmt.Sprintf(`INSERT INTO trade_goods_%s
					(trade_id, trade_no, sub_trade_id, goods_id, goods_no, goods_name,
					 spec_id, spec_name, barcode,
					 sell_count, sell_price, sell_total, cost, discount_fee, tax_fee,
					 category_name, brand_name, unit, is_gift, is_fit,
					 shop_id, bill_date, trade_type)
					VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
					ON DUPLICATE KEY UPDATE sell_count=VALUES(sell_count), sell_total=VALUES(sell_total)`, tableMonth),
					tradeId, tradeNo, gs(g, "SubTradeId"), gs(g, "GoodsId"), gn(g, "GoodsNo"), gn(g, "GoodsName"),
					gs(g, "SpecId"), gn(g, "SpecName"), gn(g, "Barcode"),
					gn(g, "SellCount"), gn(g, "SellPrice"), gn(g, "SellTotal"), gn(g, "Cost"), gn(g, "DiscountFee"), gn(g, "TaxFee"),
					gn(g, "CateName"), gn(g, "BrandName"), gn(g, "Unit"), gn(g, "IsGift"), gn(g, "IsFit"),
					gs(t, "ShopId"), gn(t, "BillDate"), gn(t, "TradeType"),
				)
			}
		}
	}
	return nil
}

func gs(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return "0"
	}
	return fmt.Sprintf("%v", v)
}

func gn(m map[string]interface{}, key string) interface{} {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	s := strings.TrimSpace(fmt.Sprintf("%v", v))
	if s == "" {
		return nil
	}
	return s
}
