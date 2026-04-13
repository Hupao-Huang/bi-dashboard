package main

// 调试工具：复现3月3日报错，打印请求参数和原始HTTP响应

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	appKey   = "73983197"
	secret   = "607f395d615d452abade78e3241b2433"
	apiURL   = "https://open.jackyun.com/open/openapi/do"
	method   = "jackyun.tradenotsensitiveinfos.list.get"
	startDay = "2026-03-03 16:07:00"
	endDay   = "2026-03-03 16:07:59"
)

func sign(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "sign" || k == "contextid" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf strings.Builder
	buf.WriteString(secret)
	for _, k := range keys {
		buf.WriteString(k)
		buf.WriteString(params[k])
	}
	buf.WriteString(secret)
	hash := md5.Sum([]byte(strings.ToLower(buf.String())))
	return hex.EncodeToString(hash[:])
}

func main() {
	// 模拟翻页直到报错
	scrollId := ""
	pageIndex := 0
	totalPulled := 0

	fields := "tradeNo,tradeStatus,tradeType,shopName,shopId,shopcode,warehouseName,warehouseId,warehouseCode," +
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

	for {
		biz := map[string]interface{}{
			"startConsignTime": startDay,
			"endConsignTime":   endDay,
			"isDelete":         "0",
			"pageIndex":        pageIndex,
			"pageSize":         200,
			"fields":           fields,
		}
		if scrollId != "" {
			biz["scrollId"] = scrollId
		}

		bizBytes, _ := json.Marshal(biz)
		params := map[string]string{
			"method":      method,
			"appkey":      appKey,
			"version":     "v1.0",
			"contenttype": "json",
			"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
			"bizcontent":  string(bizBytes),
		}
		params["sign"] = sign(params)

		form := url.Values{}
		for k, v := range params {
			form.Set(k, v)
		}

		resp, err := http.PostForm(apiURL, form)
		if err != nil {
			fmt.Printf("HTTP错误: %v\n", err)
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var apiResp struct {
			Code int             `json:"code"`
			Msg  string          `json:"msg"`
			Data json.RawMessage `json:"result"`
		}
		json.Unmarshal(body, &apiResp)

		if apiResp.Code != 200 {
			fmt.Println("\n========== 报错了！打印请求和返回 ==========")
			fmt.Printf("\n【请求URL】%s\n", apiURL)
			fmt.Printf("\n【请求方式】POST (form-urlencoded)\n")
			fmt.Printf("\n【请求参数】\n")
			fmt.Printf("  method = %s\n", params["method"])
			fmt.Printf("  appkey = %s\n", params["appkey"])
			fmt.Printf("  version = %s\n", params["version"])
			fmt.Printf("  contenttype = %s\n", params["contenttype"])
			fmt.Printf("  timestamp = %s\n", params["timestamp"])
			fmt.Printf("  sign = %s\n", params["sign"])
			fmt.Printf("\n【bizcontent】(导致报错的具体业务参数):\n")
			var pretty map[string]interface{}
			json.Unmarshal(bizBytes, &pretty)
			prettyJSON, _ := json.MarshalIndent(pretty, "  ", "  ")
			fmt.Printf("  %s\n", string(prettyJSON))
			fmt.Printf("\n【已成功拉取条数】%d\n", totalPulled)
			fmt.Printf("\n【出错时翻页位置】pageIndex=%d (即第%d页, 每页200条)\n", pageIndex, pageIndex+1)
			fmt.Printf("\n【原始HTTP响应】\n%s\n", string(body))
			return
		}

		// 解析当前数据
		var wrapper struct {
			Data json.RawMessage `json:"data"`
		}
		json.Unmarshal(apiResp.Data, &wrapper)
		var dataBytes []byte
		var dataStr string
		if err := json.Unmarshal(wrapper.Data, &dataStr); err == nil {
			dataBytes = []byte(dataStr)
		} else {
			dataBytes = wrapper.Data
		}
		var result struct {
			Trades []json.RawMessage `json:"Trades"`
		}
		json.Unmarshal(dataBytes, &result)

		if len(result.Trades) == 0 {
			fmt.Println("拉取完成，无报错")
			return
		}
		totalPulled += len(result.Trades)
		fmt.Printf("第%d页 拉取%d条 (累计%d)\n", pageIndex, len(result.Trades), totalPulled)

		if len(result.Trades) < 200 {
			fmt.Println("拉取完成，无报错")
			return
		}
		pageIndex++
	}
}
