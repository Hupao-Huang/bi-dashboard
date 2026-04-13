package jackyun

import (
	"encoding/json"
	"fmt"
)

// SalesSummaryQuery 销售货品汇总帐查询参数
type SalesSummaryQuery struct {
	TimeType          int    `json:"timeType"`                    // 2:按月, 3:按天
	StartTime         string `json:"startTime"`                   // 开始时间
	EndTime           string `json:"endTime"`                     // 截止时间
	FilterTimeType    int    `json:"filterTimeType"`              // 1:下单时间 2:发货时间 3:审核时间 4:完成时间 5:签收时间 6:开票时间
	AssemblyDimension int    `json:"assemblyDimension,omitempty"` // 0:不按组装 1:按单品
	PageIndex         int    `json:"pageIndex"`
	PageSize          int    `json:"pageSize"`
	SummaryType       string `json:"summaryType,omitempty"`       // 汇总方式: 1:时间 2:销售渠道 3:订单类型 4:业务员 5:仓库
	OrderType         string `json:"orderType,omitempty"`         // 订单类型
	ShopId            string `json:"shopId,omitempty"`
	TradeStatus       string `json:"tradeStatus,omitempty"`       // 订单状态
	IsSkuStatistic    int    `json:"isSkuStatistic,omitempty"`    // 1:按规格 2:按商品汇总
	IsCancelTrade     string `json:"isCancelTrade,omitempty"`     // 0:不包含取消单 1:包含
	IsAssembly        string `json:"isAssembly,omitempty"`        // 0:不按组装 1:按组装 2:全部
	ExcludeFlag       string `json:"excludeFlag,omitempty"`       // 1:排除特殊单
}

// SalesSummaryItem 汇总结果条目
type SalesSummaryItem struct {
	ShopId                FlexString `json:"shopId"`
	ShopName              FlexString `json:"shopName"`
	ShopCode              FlexString `json:"shopCode"`
	WarehouseId           FlexString `json:"warehouseId"`
	WarehouseName         FlexString `json:"warehouseName"`
	WarehouseCode         FlexString `json:"warehouseCode"`
	Time                  FlexString `json:"time"`
	GoodsId               FlexString `json:"goodsId"`
	GoodsNo               FlexString `json:"goodsNo"`
	GoodsName             FlexString `json:"goodsName"`
	BrandName             FlexString `json:"brandName"`
	CateName              FlexString `json:"cateName"`
	SkuId                 FlexString `json:"skuId"`
	SkuName               FlexString `json:"skuName"`
	SkuBarcode            FlexString `json:"skuBarcode"`
	Unit                  FlexString `json:"unit"`
	ChargeCurrencyCode    FlexString `json:"chargeCurrencyCode"`
	GoodsQty              FlexFloat  `json:"goodsQty"`
	GoodsAmt              FlexFloat  `json:"goodsAmt"`
	LocalCurrencyGoodsAmt FlexFloat  `json:"localCurrencyGoodsAmt"`
	GoodsCost             FlexFloat  `json:"goodsCost"`
	TaxFee                FlexFloat  `json:"taxFee"`
	TaxAmt                FlexFloat  `json:"taxAmt"`
	GrossProfit           FlexFloat  `json:"grossProfit"`
	TaxGrossProfit        FlexFloat  `json:"taxGrossProfit"`
	AvgPrice              FlexFloat  `json:"avgPrice"`
	SellTotal             FlexFloat  `json:"sellTotal"`
	ShareSalesExpense     FlexFloat  `json:"shareSalesExpense"`
	GoodsNameEn           FlexString `json:"goodsNameEn"`
	GrossProfitRate       FlexFloat  `json:"grossProfitRate"`
	TaxGrossProfitRate    FlexFloat  `json:"taxGrossProfitRate"`
	TaxUnitPrice          FlexFloat  `json:"taxUnitPrice"`
	FixedCost             FlexFloat  `json:"fixedCost"`
	RetailPrice           FlexFloat  `json:"retailPrice"`
	SoQty                 FlexFloat  `json:"soQty"`
	SellerId              FlexString `json:"sellerId"`
	SellerName            FlexString `json:"sellerName"`
	TradeOrderType        FlexString `json:"tradeOrderType"`
	TradeOrderTypeName    FlexString `json:"tradeOrderTypeName"`
	CateFullName          FlexString `json:"cateFullName"`
	ColorName             FlexString `json:"colorName"`
	SizeName              FlexString `json:"sizeName"`
	GoodsAlias            FlexString `json:"goodsAlias"`
	MaterialName          FlexString `json:"materialName"`
	MainBarcode           FlexString `json:"mainBarcode"`
	ImgUrl                FlexString `json:"imgUrl"`
	SkuNo                 FlexString `json:"skuNo"`
	SkuGmtCreate          FlexString `json:"skuGmtCreate"`
	GoodsGmtCreate        FlexString `json:"goodsGmtCreate"`
	ShopCateName          FlexString `json:"shopCateName"`
	ShopCompanyCode       FlexString `json:"shopCompanyCode"`
	CurrencyName          FlexString `json:"currencyName"`
	LocalCurrencyShareSalesExpense FlexFloat `json:"localCurrencyShareSalesExpense"`
	LocalCurrencyTaxFee   FlexFloat  `json:"localCurrencyTaxFee"`
	GoodsExtendMap        FlexString `json:"goodsExtendMap"`
	PriceExtendMap        FlexString `json:"priceExtendMap"`
	SkuExtendMap          FlexString `json:"skuExtendMap"`
	AssistInfo            FlexString `json:"assistInfo"`
	GoodsFlagData         FlexString `json:"goodsFlagData"`
	DefaultVendName       FlexString `json:"defaultVendName"`
	EstimateWeight        FlexFloat  `json:"estimateWeight"`
	DefaultVendId         FlexString `json:"defaultVendId"`
	UniqueId              FlexString `json:"uniqueId"`
	UniqueSkuId           FlexString `json:"uniqueSkuId"`
}

// FetchSalesSummary 拉取销售货品汇总数据（分页）
func (c *Client) FetchSalesSummary(query SalesSummaryQuery, callback func([]SalesSummaryItem) error) error {
	page := query.PageIndex
	total := 0
	for {
		query.PageIndex = page
		if query.PageSize == 0 {
			query.PageSize = 50
		}

		resp, err := c.Call("birc.report.salesGoodsSummary", query)
		if err != nil {
			return fmt.Errorf("call sales summary api page %d: %w", page, err)
		}
		if resp.Code != 200 {
			return fmt.Errorf("sales summary api error page %d: code=%d msg=%s", page, resp.Code, resp.Msg)
		}

		var wrapper struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(resp.Result, &wrapper); err != nil {
			return fmt.Errorf("unmarshal result page %d: %w", page, err)
		}

		var items []SalesSummaryItem
		if err := json.Unmarshal(wrapper.Data, &items); err != nil {
			return fmt.Errorf("unmarshal data page %d: %w", page, err)
		}

		if len(items) == 0 {
			break
		}

		if err := callback(items); err != nil {
			return fmt.Errorf("callback page %d: %w", page, err)
		}

		total += len(items)
		fmt.Printf("    汇总已拉取 %d 条\n", total)

		if len(items) < query.PageSize {
			break
		}
		page++
	}

	return nil
}
