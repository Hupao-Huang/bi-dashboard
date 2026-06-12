package jackyun

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
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

// FetchTrades 拉取销售单（支持游标分页）
// progressFn 可选，用于打印翻页进度
// 注意: 当前无生产调用方 (仅单测覆盖) — sync-daily-trades 用自己的按小时拆窗循环拉单,
// 且自带 scrollId 缺失防护。本函数的 emptyScroll 守卫是库函数加固, 供未来调用方继承。
func (c *Client) FetchTrades(start, end time.Time, callback func([]Trade) error, progressFn ...func(fetched, total int)) error {
	scrollId := ""
	pageIndex := 0
	fetched := 0
	emptyScroll := 0 // 连续几页没回 scrollId (游标不前进时防原地重拉死循环)

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
			emptyScroll = 0
		} else {
			// 游标没回来 (空 scrollId 真实发生过, 见 cmd/probe-trade-empty-scrollid):
			// 整页满 = 数据没拉完, 旧游标再查只会原地重拉同一页; 连续两轮缺失 = 游标已卡死
			// 报错让同步任务失败重跑, 不能无限循环也不能静默截断
			emptyScroll++
			if len(result.Trades) >= 200 || emptyScroll >= 2 {
				return fmt.Errorf("scrollId 缺失 page %d (本页 %d 条, 已拉 %d 条), 中止防原地重拉", pageIndex, len(result.Trades), fetched)
			}
			// 非整页 + 首次缺失: 大概率已是最后一页, 按原行为再查一次等空页正常收尾
		}
		pageIndex++
	}

	return nil
}
