package jackyun

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"
)

// FlexString 兼容吉客云返回的 string/number 不一致问题
type FlexString string

func (f *FlexString) UnmarshalJSON(data []byte) error {
	// 尝试 string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = FlexString(s)
		return nil
	}
	// 尝试 number
	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		*f = FlexString(n.String())
		return nil
	}
	// null
	*f = ""
	return nil
}

func (f FlexString) String() string { return string(f) }

// FlexFloat 兼容吉客云返回的 float/string 不一致问题
type FlexFloat float64

func (f *FlexFloat) UnmarshalJSON(data []byte) error {
	var n float64
	if err := json.Unmarshal(data, &n); err == nil {
		*f = FlexFloat(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s == "" {
			*f = 0
			return nil
		}
		n, err := strconv.ParseFloat(s, 64)
		if err == nil {
			*f = FlexFloat(n)
			return nil
		}
		// v1.71.2: 兼容百分比字符串 "55.30%" / "-100%" 等, 按字面值存 (跑哥 2026-05-22 决策)
		// 吉客云 sales_summary.go 的 GrossProfitRate / TaxGrossProfitRate 等百分率字段返这种格式
		// 字段类型 DECIMAL(8,4) 完全能装 -1076.36 这种负退款率
		if strings.HasSuffix(s, "%") {
			trimmed := strings.TrimSuffix(s, "%")
			if n, err := strconv.ParseFloat(trimmed, 64); err == nil {
				*f = FlexFloat(n)
				return nil
			}
		}
	}
	// v1.71.1: 上游格式异常 (既不是 float 也不是合法 string 数字), 默认 0 但 log 警告
	// 区分"业务真 0" vs "数据污染", 销售明细金额/数量被静默吞成 0 时能从日志查到
	log.Printf("[jackyun.FlexFloat] 格式异常返0: %.200s", string(data))
	*f = 0
	return nil
}

func (f FlexFloat) Float64() float64 { return float64(f) }

// FlexInt 兼容吉客云返回的 int/string 不一致问题
type FlexInt int

func (f *FlexInt) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*f = FlexInt(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s == "" {
			*f = 0
			return nil
		}
		n, err := strconv.Atoi(s)
		if err == nil {
			*f = FlexInt(n)
			return nil
		}
		// v1.71.2: 兼容 "55%" 之类的百分比字符串 (按字面值存), 与 FlexFloat 一致
		if strings.HasSuffix(s, "%") {
			trimmed := strings.TrimSuffix(s, "%")
			if n, err := strconv.Atoi(trimmed); err == nil {
				*f = FlexInt(n)
				return nil
			}
		}
	}
	// v1.71.1: 同 FlexFloat, 格式异常加 log
	log.Printf("[jackyun.FlexInt] 格式异常返0: %.200s", string(data))
	*f = 0
	return nil
}

func (f FlexInt) Int() int { return int(f) }

// TradeQuery 销售单查询请求参数
type TradeQuery struct {
	StartModified    string `json:"startModified,omitempty"`
	EndModified      string `json:"endModified,omitempty"`
	StartCreated     string `json:"startCreated,omitempty"`
	EndCreated       string `json:"endCreated,omitempty"`
	StartConsignTime string `json:"startConsignTime,omitempty"` // 发货时间（起始）
	EndConsignTime   string `json:"endConsignTime,omitempty"`   // 发货时间（截止）
	TradeStatus      int    `json:"tradeStatus,omitempty"`
	TradeStatusList  []int  `json:"tradeStatusList,omitempty"`  // 状态列表
	PageIndex        int    `json:"pageIndex"`
	PageSize         int    `json:"pageSize"`
	HasTotal         int    `json:"hasTotal"`
	ScrollId         string `json:"scrollId,omitempty"`
	Fields           string `json:"fields,omitempty"`
	IsDelete         string `json:"isDelete,omitempty"` // 0:未删除
}

// TradeResult 销售单查询结果
type TradeResult struct {
	TotalResults int     `json:"totalResults"`
	Trades       []Trade `json:"trades"`
	ScrollId     string  `json:"scrollId"`
}

// Trade 销售单
type Trade struct {
	TradeNo        FlexString  `json:"tradeNo"`
	TradeId        FlexString  `json:"tradeId"`
	SourceTradeNo  FlexString  `json:"sourceTradeNo"`
	TradeStatus    FlexInt     `json:"tradeStatus"`
	TradeType      FlexInt     `json:"tradeType"`
	ShopId         FlexString  `json:"shopId"`
	ShopName       FlexString  `json:"shopName"`
	WarehouseId    FlexString  `json:"warehouseId"`
	WarehouseName  FlexString  `json:"warehouseName"`
	PayType        FlexInt     `json:"payType"`
	PayNo          FlexString  `json:"payNo"`
	ChargeCurrency FlexString  `json:"chargeCurrency"`
	CheckTotal     FlexFloat   `json:"checkTotal"`
	OtherFee       FlexFloat   `json:"otherFee"`
	SellerMemo     FlexString  `json:"sellerMemo"`
	BuyerMemo      FlexString  `json:"buyerMemo"`
	TradeTime      FlexString  `json:"tradeTime"`
	GmtCreate      FlexString  `json:"gmtCreate"`
	AuditTime      FlexString  `json:"auditTime"`
	ConsignTime    FlexString  `json:"consignTime"`
	CompleteTime   FlexString  `json:"completeTime"`
	GmtModified    FlexString  `json:"gmtModified"`
	BillDate       FlexString  `json:"billDate"`       // 发货时间（通过billDate字段返回）
	GoodsDetail    []GoodsItem `json:"goodsDetail"`
}

// GoodsItem 商品明细
type GoodsItem struct {
	GoodsId     FlexString `json:"goodsId"`
	GoodsNo     FlexString `json:"goodsNo"`
	GoodsName   FlexString `json:"goodsName"`
	SpecName    FlexString `json:"specName"`
	Barcode     FlexString `json:"barcode"`
	SkuId       FlexString `json:"skuId"`
	SellCount   FlexFloat  `json:"sellCount"`
	SellPrice   FlexFloat  `json:"sellPrice"`
	SellTotal   FlexFloat  `json:"sellTotal"`
	Cost        FlexFloat  `json:"cost"`
	DiscountFee FlexFloat  `json:"discountFee"`
	TaxFee      FlexFloat  `json:"taxFee"`
	CateName    FlexString `json:"cateName"`
	BrandName   FlexString `json:"brandName"`
	Unit        FlexString `json:"unit"`
}

