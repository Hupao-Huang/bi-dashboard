package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/jackyun"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type syncWindow struct {
	start string
	end   string
	table string
}

func main() {

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

	// 用新appkey调定制接口
	client := jackyun.NewClient(cfg.JackYunTrade.AppKey, cfg.JackYunTrade.Secret, cfg.JackYun.APIURL)

	months, err := loadSyncWindows()
	if err != nil {
		log.Fatalf("同步范围配置错误: %v", err)
	}

	for _, m := range months {
	startTime, _ := time.Parse("2006-01-02", m.start)
	endTime, _ := time.Parse("2006-01-02", m.end)
	tableMonth := m.table

	fmt.Printf("\n开始同步（定制接口）: %s ~ %s\n", startTime.Format("2006-01-02"), endTime.Format("2006-01-02"))
	fmt.Printf("目标表: trade_%s / trade_goods_%s / trade_package_%s\n\n", tableMonth, tableMonth, tableMonth)

	ensureTable(db, "trade_"+tableMonth, "trade_202601")
	ensureTable(db, "trade_goods_"+tableMonth, "trade_goods_202601")
	ensureTable(db, "trade_package_"+tableMonth, "trade_package_202601")

	fields := "tradeNo,tradeStatus,tradeStatusExplain,tradeType,shopName,shopId,shopcode,warehouseName,warehouseId,warehouseCode," +
		"goodsDetail.goodsNo,goodsDetail.goodsName,goodsDetail.goodsId,goodsDetail.sellCount," +
		"goodsDetail.sellPrice,goodsDetail.sellTotal,goodsDetail.specName,goodsDetail.barcode," +
		"goodsDetail.cost,goodsDetail.cateName,goodsDetail.brandName,goodsDetail.unit," +
		"goodsDetail.subTradeId,goodsDetail.isFit,goodsDetail.isGift,goodsDetail.specId," +
		"goodsDetail.discountFee,goodsDetail.taxFee,goodsDetail.goodsMemo,goodsDetail.outerId,goodsDetail.platGoodsId," +
		"goodsDetail.platSkuId,goodsDetail.skuImgUrl,goodsDetail.divideSellTotal,goodsDetail.goodsSeller," +
		"goodsDetail.goodsPlatDiscountFee,goodsDetail.isPresell," +
		"goodsDetail.shareOrderDiscountFee,goodsDetail.shareOrderPlatDiscountFee," +
		"sourceTradeNo,onlineTradeNo,scrollId,billDate,consignTime,tradeTime,orderNo,customerName," +
		"logisticName,mainPostid,checkTotal,totalFee,payment,postFee,discountFee,otherFee," +
		"tradeCount,isDelete,tradeFrom,sellerMemo,buyerMemo," +
		"payTime,payType,payStatus,payNo,chargeCurrency," +
		"grossProfit,couponFee,realFee,taxFee,taxRate," +
		"departName,companyName,gmtCreate,gmtModified," +
		"auditTime,completeTime,flagIds,flagNames," +
		"state,city,district,town,country,zip," +
		"customerDiscountFee,customerPostFee,customerDiscount,customerTotalFee," +
		"customerAccount,customerCode,customerGradeName,customerTags," +
		"buyerOpenUid,blackList," +
		"invoiceAmount,invoiceType,invoiceCode," +
		"chargeExchangeRate,chargeCurrencyCode,localCurrencyCode," +
		"firstPayment,finalPayment,receivedTotal," +
		"firstPaytime,finalPaytime,finReceiptTime," +
		"payerName,payerPhone,payerRegno,payerBankAccount,payerBankName," +
		"logisticCode,logisticType,extraLogisticNo,packageWeight,estimateWeight,estimateVolume," +
		"stockoutNo,lastShipTime," +
		"signingTime,reviewTime,confirmTime,activationTime,notifyPickTime," +
		"settleAuditTime,platCompleteTime," +
		"reviewer,auditor,register,seller," +
		"shopTypeCode,agentShopName,sourceAfterNo,countryCode,cityCode," +
		"sysFlagIds,specialReminding,abnormalDescription,appendMemo," +
		"ticketCodeList,allCompassSourceContentType," +
		"packageDetail.logisticNo,packageDetail.logisticName,packageDetail.logisticCode," +
		"packageDetail.warehouseName,packageDetail.sellCount,packageDetail.isGift,packageDetail.isPlatGift," +
		"packageDetail.barcode,packageDetail.tradeNo,packageDetail.sourceTradeNo," +
		"packageDetail.buyerMemo,packageDetail.sellerMemo"

	totalTrades := 0
	totalGoods := 0
	dayStart := startTime

	for !dayStart.After(endTime) {
		dayEnd := dayStart
		ds := dayStart.Format("2006-01-02") + " 00:00:00"
		de := dayEnd.Format("2006-01-02") + " 23:59:59"


		fmt.Printf("[%s] ", dayStart.Format("2006-01-02"))

		scrollId := ""
		parseRetry := 0
		dayTrades := 0
		dayGoods := 0

		for {
			biz := map[string]interface{}{
				"startConsignTime": ds,
				"endConsignTime":   de,
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
				wait := time.Duration((retry+1)*10) * time.Second
				log.Printf("调用失败(重试%d/5，等待%s): %v", retry+1, wait, err)
				time.Sleep(wait)
			}
			if err != nil {
				log.Printf("5次重试均失败，跳过当天剩余数据")
				break
			}
			if resp.Code != 200 {
				log.Printf("接口报错: code=%d msg=%s", resp.Code, resp.Msg)
				break
			}

			// 解析 result -> data (字符串需二次解析)
			var wrapper struct {
				Data     json.RawMessage `json:"data"`
				ScrollId string          `json:"scrollId"`
			}
			json.Unmarshal(resp.Result, &wrapper)
			if scrollId == "" && wrapper.ScrollId != "" {
				log.Printf("wrapper层scrollId: %s", wrapper.ScrollId[:min(len(wrapper.ScrollId), 32)])
			}

			var dataBytes []byte
			var dataStr string
			if err := json.Unmarshal(wrapper.Data, &dataStr); err == nil {
				dataBytes = []byte(dataStr)
			} else {
				dataBytes = wrapper.Data
			}

			var result struct {
				TotalResults int               `json:"TotalResults"`
				Trades       []json.RawMessage `json:"Trades"`
				ScrollId     string            `json:"ScrollId"`
			}
			if err := json.Unmarshal(dataBytes, &result); err != nil {
				parseRetry++
				if parseRetry >= 3 {
					log.Printf("解析失败已重试3次，跳过: %v", err)
					break
				}
				log.Printf("解析失败(重试%d/3): %v，等待10秒后重试", parseRetry, err)
				time.Sleep(10 * time.Second)
				continue
			}
			parseRetry = 0

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
				isDelete := gs(t, "IsDelete")
				if isDelete == "1" {
					continue
				}

				// 写入订单主表
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
					 state, city, district, town, country, zip,
					 customer_discount_fee, customer_post_fee, customer_discount, customer_total_fee,
					 customer_account, customer_code, customer_grade_name, customer_tags,
					 buyer_open_uid, black_list,
					 invoice_amount, invoice_type, invoice_code,
					 charge_exchange_rate, charge_currency_code, local_currency_code,
					 first_payment, final_payment, received_total,
					 first_paytime, final_paytime, fin_receipt_time,
					 payer_name, payer_phone, payer_regno, payer_bank_account, payer_bank_name,
					 logistic_code, logistic_type, extra_logistic_no, package_weight, estimate_weight, estimate_volume,
					 stockout_no, last_ship_time,
					 signing_time, review_time, confirm_time, activation_time, notify_pick_time,
					 settle_audit_time, plat_complete_time,
					 reviewer, auditor, register, seller,
					 shop_type_code, agent_shop_name, source_after_no, country_code, city_code,
					 sys_flag_ids, special_reminding, abnormal_description, append_memo,
					 ticket_code_list, all_compass_source_content_type)
					VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
					ON DUPLICATE KEY UPDATE
						shop_id=VALUES(shop_id), warehouse_id=VALUES(warehouse_id),
						buyer_open_uid=VALUES(buyer_open_uid), customer_account=VALUES(customer_account),
						trade_status=VALUES(trade_status), trade_type=VALUES(trade_type),
						check_total=VALUES(check_total), total_fee=VALUES(total_fee),
						payment=VALUES(payment), discount_fee=VALUES(discount_fee),
						gross_profit=VALUES(gross_profit), coupon_fee=VALUES(coupon_fee),
						real_fee=VALUES(real_fee), tax_fee=VALUES(tax_fee), tax_rate=VALUES(tax_rate),
						seller_memo=VALUES(seller_memo), buyer_memo=VALUES(buyer_memo),
						pay_type=VALUES(pay_type), pay_no=VALUES(pay_no), pay_status=VALUES(pay_status),
						charge_currency=VALUES(charge_currency),
						audit_time=VALUES(audit_time), complete_time=VALUES(complete_time),
						gmt_modified=VALUES(gmt_modified),
						flag_ids=VALUES(flag_ids), flag_names=VALUES(flag_names),
						is_delete=VALUES(is_delete),
						state=VALUES(state), city=VALUES(city), district=VALUES(district),
						town=VALUES(town), country=VALUES(country), zip=VALUES(zip),
						customer_discount_fee=VALUES(customer_discount_fee), customer_post_fee=VALUES(customer_post_fee),
						customer_discount=VALUES(customer_discount), customer_total_fee=VALUES(customer_total_fee),
						customer_account=VALUES(customer_account), customer_code=VALUES(customer_code),
						customer_grade_name=VALUES(customer_grade_name), customer_tags=VALUES(customer_tags),
						buyer_open_uid=VALUES(buyer_open_uid), black_list=VALUES(black_list),
						invoice_amount=VALUES(invoice_amount), invoice_type=VALUES(invoice_type), invoice_code=VALUES(invoice_code),
						charge_exchange_rate=VALUES(charge_exchange_rate), charge_currency_code=VALUES(charge_currency_code),
						local_currency_code=VALUES(local_currency_code),
						first_payment=VALUES(first_payment), final_payment=VALUES(final_payment), received_total=VALUES(received_total),
						first_paytime=VALUES(first_paytime), final_paytime=VALUES(final_paytime), fin_receipt_time=VALUES(fin_receipt_time),
						payer_name=VALUES(payer_name), payer_phone=VALUES(payer_phone), payer_regno=VALUES(payer_regno),
						payer_bank_account=VALUES(payer_bank_account), payer_bank_name=VALUES(payer_bank_name),
						logistic_code=VALUES(logistic_code), logistic_type=VALUES(logistic_type),
						extra_logistic_no=VALUES(extra_logistic_no), package_weight=VALUES(package_weight),
						estimate_weight=VALUES(estimate_weight), estimate_volume=VALUES(estimate_volume),
						stockout_no=VALUES(stockout_no), last_ship_time=VALUES(last_ship_time),
						signing_time=VALUES(signing_time), review_time=VALUES(review_time),
						confirm_time=VALUES(confirm_time), activation_time=VALUES(activation_time),
						notify_pick_time=VALUES(notify_pick_time),
						settle_audit_time=VALUES(settle_audit_time), plat_complete_time=VALUES(plat_complete_time),
						reviewer=VALUES(reviewer), auditor=VALUES(auditor), register=VALUES(register), seller=VALUES(seller),
						shop_type_code=VALUES(shop_type_code), agent_shop_name=VALUES(agent_shop_name),
						source_after_no=VALUES(source_after_no), country_code=VALUES(country_code), city_code=VALUES(city_code),
						sys_flag_ids=VALUES(sys_flag_ids), special_reminding=VALUES(special_reminding),
						abnormal_description=VALUES(abnormal_description), append_memo=VALUES(append_memo),
						ticket_code_list=VALUES(ticket_code_list),
						all_compass_source_content_type=VALUES(all_compass_source_content_type)`, tableMonth),
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
					gn(t, "CustomerDiscountFee"), gn(t, "CustomerPostFee"), gn(t, "CustomerDiscount"), gn(t, "CustomerTotalFee"),
					gn(t, "CustomerAccount"), gn(t, "CustomerCode"), gn(t, "CustomerGradeName"), gn(t, "CustomerTags"),
					gn(t, "BuyerOpenUid"), gn(t, "BlackList"),
					gn(t, "InvoiceAmount"), gn(t, "InvoiceType"), gn(t, "InvoiceCode"),
					gn(t, "ChargeExchangeRate"), gn(t, "ChargeCurrencyCode"), gn(t, "LocalCurrencyCode"),
					gn(t, "FirstPayment"), gn(t, "FinalPayment"), gn(t, "ReceivedTotal"),
					gn(t, "FirstPaytime"), gn(t, "FinalPaytime"), gn(t, "FinReceiptTime"),
					gn(t, "PayerName"), gn(t, "PayerPhone"), gn(t, "PayerRegno"), gn(t, "PayerBankAccount"), gn(t, "PayerBankName"),
					gn(t, "LogisticCode"), gn(t, "LogisticType"), gn(t, "ExtraLogisticNo"), gn(t, "PackageWeight"), gn(t, "EstimateWeight"), gn(t, "EstimateVolume"),
					gn(t, "StockoutNo"), gn(t, "LastShipTime"),
					gn(t, "SigningTime"), gn(t, "ReviewTime"), gn(t, "ConfirmTime"), gn(t, "ActivationTime"), gn(t, "NotifyPickTime"),
					gn(t, "SettleAuditTime"), gn(t, "PlatCompleteTime"),
					gn(t, "Reviewer"), gn(t, "Auditor"), gn(t, "Register"), gn(t, "Seller"),
					gn(t, "ShopTypeCode"), gn(t, "AgentShopName"), gn(t, "SourceAfterNo"), gn(t, "CountryCode"), gn(t, "CityCode"),
					gn(t, "SysFlagIds"), gn(t, "SpecialReminding"), gn(t, "AbnormalDescription"), gn(t, "AppendMemo"),
					gn(t, "TicketCodeList"), gn(t, "AllCompassSourceContentType"))
				if err != nil {
					log.Printf("订单写入失败 %s: %v", tradeNo, err)
				}
				dayTrades++

				// 写入商品明细
				if gd, ok := t["GoodsDetail"]; ok && gd != nil {
					if gdList, ok := gd.([]interface{}); ok {
						for _, item := range gdList {
							g, ok := item.(map[string]interface{})
							if !ok {
								continue
							}
							_, err := db.Exec(fmt.Sprintf(`INSERT INTO trade_goods_%s
								(trade_id, trade_no, sub_trade_id, goods_id, goods_no, goods_name,
								 spec_id, spec_name, barcode,
								 sell_count, sell_price, sell_total, cost, discount_fee, tax_fee,
								 category_name, brand_name, unit, is_gift, is_fit,
								 goods_memo, outer_id, plat_goods_id, plat_sku_id, sku_img_url,
								 divide_sell_total, goods_seller,
								 shop_id, bill_date, trade_type,
								 goods_plat_discount_fee, is_presell, share_order_discount_fee, share_order_plat_discount_fee)
								VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
							ON DUPLICATE KEY UPDATE goods_id=VALUES(goods_id), shop_id=VALUES(shop_id),
								sell_count=VALUES(sell_count), sell_price=VALUES(sell_price),
								sell_total=VALUES(sell_total), cost=VALUES(cost), discount_fee=VALUES(discount_fee),
								goods_memo=VALUES(goods_memo), divide_sell_total=VALUES(divide_sell_total),
								goods_seller=VALUES(goods_seller),
								goods_plat_discount_fee=VALUES(goods_plat_discount_fee), is_presell=VALUES(is_presell),
								share_order_discount_fee=VALUES(share_order_discount_fee),
								share_order_plat_discount_fee=VALUES(share_order_plat_discount_fee)`, tableMonth),
								tradeId, tradeNo, gs(g, "SubTradeId"), gs(g, "GoodsId"), gn(g, "GoodsNo"), gn(g, "GoodsName"),
								gs(g, "SpecId"), gn(g, "SpecName"), gn(g, "Barcode"),
								gn(g, "SellCount"), gn(g, "SellPrice"), gn(g, "SellTotal"), gn(g, "Cost"), gn(g, "DiscountFee"), gn(g, "TaxFee"),
								gn(g, "CateName"), gn(g, "BrandName"), gn(g, "Unit"), gn(g, "IsGift"), gn(g, "IsFit"),
								gn(g, "GoodsMemo"), gn(g, "OuterId"), gn(g, "PlatGoodsId"), gn(g, "PlatSkuId"), gn(g, "SkuImgUrl"),
								gn(g, "DivideSellTotal"), gn(g, "GoodsSeller"),
								gs(t, "ShopId"), gn(t, "BillDate"), gn(t, "TradeType"),
								gn(g, "GoodsPlatDiscountFee"), gn(g, "IsPresell"), gn(g, "ShareOrderDiscountFee"), gn(g, "ShareOrderPlatDiscountFee"))
							if err != nil {
								log.Printf("明细写入失败 %s: %v", tradeNo, err)
							}
							dayGoods++
						}
					}
				}
			}

			// 写入包裹详情
			for _, raw := range result.Trades {
				var tp map[string]interface{}
				json.Unmarshal(raw, &tp)
				tid := gs(tp, "TradeId")
				tno := gs(tp, "TradeNo")
				if pd, ok := tp["PackageDetail"]; ok && pd != nil {
					if pdList, ok := pd.([]interface{}); ok {
						for _, item := range pdList {
							p, ok := item.(map[string]interface{})
							if !ok {
								continue
							}
							db.Exec(fmt.Sprintf(`INSERT IGNORE INTO trade_package_%s
								(trade_id, trade_no, logistic_no, logistic_name, logistic_code,
								 warehouse_name, sell_count, is_gift, is_plat_gift, barcode,
								 source_trade_no, buyer_memo, seller_memo)
								VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, tableMonth),
								tid, tno, gn(p, "LogisticNo"), gn(p, "LogisticName"), gn(p, "LogisticCode"),
								gn(p, "WarehouseName"), gn(p, "SellCount"), gn(p, "IsGift"), gn(p, "IsPlatGift"),
								gn(p, "Barcode"), gn(p, "SourceTradeNo"), gn(p, "BuyerMemo"), gn(p, "SellerMemo"))
						}
					}
				}
			}

			if len(result.Trades) < 200 {
				break
			}

			// 游标翻页
			if wrapper.ScrollId != "" {
				scrollId = wrapper.ScrollId
			} else if result.ScrollId != "" {
				scrollId = result.ScrollId
			} else {
				log.Printf("警告: 无scrollId返回，停止翻页")
				break
			}
		}

		totalTrades += dayTrades
		totalGoods += dayGoods
		fmt.Printf("订单:%d 明细:%d (累计 %d/%d)\n", dayTrades, dayGoods, totalTrades, totalGoods)

		dayStart = dayStart.AddDate(0, 0, 1)
		time.Sleep(300 * time.Millisecond)
	}

	fmt.Printf("\n同步完成！共 %d 笔订单, %d 条明细\n", totalTrades, totalGoods)
	} // end months loop
}

func loadSyncWindows() ([]syncWindow, error) {
	startStr := os.Getenv("TRADE_SYNC_START_DATE")
	endStr := os.Getenv("TRADE_SYNC_END_DATE")
	if startStr == "" && endStr == "" {
		return []syncWindow{
			{start: "2026-01-01", end: "2026-01-31", table: "202601"},
			{start: "2026-02-01", end: "2026-02-28", table: "202602"},
			{start: "2026-03-01", end: "2026-03-26", table: "202603"},
		}, nil
	}
	if startStr == "" || endStr == "" {
		return nil, fmt.Errorf("TRADE_SYNC_START_DATE 和 TRADE_SYNC_END_DATE 必须同时设置")
	}

	startTime, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		return nil, fmt.Errorf("TRADE_SYNC_START_DATE 格式错误: %w", err)
	}
	endTime, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		return nil, fmt.Errorf("TRADE_SYNC_END_DATE 格式错误: %w", err)
	}
	if endTime.Before(startTime) {
		return nil, fmt.Errorf("结束日期不能早于开始日期")
	}

	var windows []syncWindow
	current := startTime
	for !current.After(endTime) {
		monthStart := current
		monthEnd := time.Date(current.Year(), current.Month()+1, 0, 0, 0, 0, 0, current.Location())
		if monthEnd.After(endTime) {
			monthEnd = endTime
		}
		windows = append(windows, syncWindow{
			start: monthStart.Format("2006-01-02"),
			end:   monthEnd.Format("2006-01-02"),
			table: monthStart.Format("200601"),
		})
		current = monthEnd.AddDate(0, 0, 1)
	}

	return windows, nil
}

func ensureTable(db *sql.DB, tableName, likeTable string) {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", tableName).Scan(&count)
	if count == 0 {
		db.Exec(fmt.Sprintf("CREATE TABLE %s LIKE %s", tableName, likeTable))
		fmt.Printf("建表: %s\n", tableName)
	}
}

func gs(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil { return "0" }
	return fmt.Sprintf("%v", v)
}

func gn(m map[string]interface{}, key string) interface{} {
	v, ok := m[key]
	if !ok || v == nil { return nil }
	s := fmt.Sprintf("%v", v)
	if s == "" { return nil }
	return s
}
