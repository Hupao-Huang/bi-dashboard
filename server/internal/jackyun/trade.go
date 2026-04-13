package jackyun

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
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
	}
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
	}
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

// FetchTrades 拉取销售单（支持游标分页）
// progressFn 可选，用于打印翻页进度
func (c *Client) FetchTrades(start, end time.Time, callback func([]Trade) error, progressFn ...func(fetched, total int)) error {
	scrollId := ""
	pageIndex := 0
	fetched := 0

	fields := "tradeNo,tradeStatus,tradeType,shopName,shopId,warehouseName,warehouseId," +
		"goodsDetail.goodsNo,goodsDetail.goodsName,goodsDetail.goodsId," +
		"goodsDetail.sellCount,goodsDetail.sellPrice,goodsDetail.specName," +
		"goodsDetail.barcode,goodsDetail.skuId,goodsDetail.cost," +
		"goodsDetail.discountFee,goodsDetail.taxFee,goodsDetail.cateName," +
		"goodsDetail.brandName,goodsDetail.unit,goodsDetail.sellTotal," +
		"sourceTradeNo,scrollId,billDate"

	// 按发货时间查询，不过滤状态——有发货时间就说明已发货，状态过滤反而会漏数据
	for {
		query := TradeQuery{
			StartConsignTime: start.Format("2006-01-02 15:04:05"),
			EndConsignTime:   end.Format("2006-01-02 15:04:05"),
			PageIndex:        pageIndex,
			PageSize:         200,
			HasTotal:         1,
			ScrollId:         scrollId,
			Fields:           fields,
		}

		resp, err := c.Call("oms.trade.fullinfoget", query)
		if err != nil {
			return fmt.Errorf("call trade api page %d: %w", pageIndex, err)
		}
		if resp.Code != 200 {
			return fmt.Errorf("trade api error page %d: code=%d msg=%s", pageIndex, resp.Code, resp.Msg)
		}

		// 解析 result -> data
		var wrapper struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(resp.Result, &wrapper); err != nil {
			return fmt.Errorf("unmarshal result page %d: %w", pageIndex, err)
		}

		var result TradeResult
		if err := json.Unmarshal(wrapper.Data, &result); err != nil {
			return fmt.Errorf("unmarshal data page %d: %w", pageIndex, err)
		}

		if len(result.Trades) == 0 {
			break
		}

		if err := callback(result.Trades); err != nil {
			return fmt.Errorf("callback page %d: %w", pageIndex, err)
		}

		fetched += len(result.Trades)

		// 打印进度
		if len(progressFn) > 0 && progressFn[0] != nil {
			progressFn[0](fetched, result.TotalResults)
		}

		if result.TotalResults > 0 && fetched >= result.TotalResults {
			break
		}

		// 使用游标翻页
		if result.ScrollId != "" {
			scrollId = result.ScrollId
		}
		pageIndex++
	}

	return nil
}
