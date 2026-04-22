package jackyun

import (
	"encoding/json"
	"fmt"
)

// StockIOQuery 出入库查询参数(入库和出库接口公用)
type StockIOQuery struct {
	Cols             string `json:"cols"`                       // 需要查询的字段
	PageIndex        int    `json:"pageIndex"`
	PageSize         int    `json:"pageSize"`
	InOutDateStart   string `json:"inOutDateStart,omitempty"`   // 出入库时间(起) yyyy-MM-dd HH:mm:ss
	InOutDateEnd     string `json:"inOutDateEnd,omitempty"`     // 出入库时间(止)
	InoutTypes       string `json:"inouttypes,omitempty"`       // 类型，多个逗号分隔
	GoodsNo          string `json:"goodsNo,omitempty"`
	BatchNo          string `json:"batchNo,omitempty"`
	WarehouseCode    string `json:"warehouseCode,omitempty"`
	Archived         int    `json:"archived,omitempty"`         // 0未归档 1归档
	IsCertified      *int   `json:"isCertified,omitempty"`      // 正次品 1正品 0次品
}

// StockIOItem 出入库单明细(入库/出库共用结构)
type StockIOItem struct {
	GoodsdocNo               FlexString `json:"goodsdocNo"`
	InOutDate                FlexString `json:"inOutDate"`
	InouttypeName            FlexString `json:"inouttypeName"`
	WarehouseCode            FlexString `json:"warehouseCode"`
	WarehouseName            FlexString `json:"warehouseName"`
	BillNo                   FlexString `json:"billNo"`
	SourceBillNo             FlexString `json:"sourceBillNo"`
	VendCode                 FlexString `json:"vendCode"`
	VendCustomerName         FlexString `json:"vendCustomerName"`
	ChannelCode              FlexString `json:"channelCode"`
	CreateUserName           FlexString `json:"createUserName"`
	GoodsdocRemark           FlexString `json:"goodsdocRemark"`
	GoodsNo                  FlexString `json:"goodsNo"`
	GoodsName                FlexString `json:"goodsName"`
	SkuName                  FlexString `json:"skuName"`
	SkuBarcode               FlexString `json:"skuBarcode"`
	UnitName                 FlexString `json:"unitName"`
	BatchNo                  FlexString `json:"batchNo"`
	Quantity                 FlexFloat  `json:"quantity"`
	BaceCurrencyCostPrice    FlexFloat  `json:"baceCurrencyCostPrice"`
	BaceCurrencyCostAmount   FlexFloat  `json:"baceCurrencyCostAmount"`
	BaceCurrencyNoTaxPrice   FlexFloat  `json:"baceCurrencyNoTaxPrice"`
	BaceCurrencyWithTaxPrice FlexFloat  `json:"baceCurrencyWithTaxPrice"`
	IsCertified              FlexInt    `json:"isCertified"`
	GoodsDetailRemark        FlexString `json:"goodsDetailRemark"`
	RecId                    FlexString `json:"recId"`
}

// FetchStockIO 通用拉取出入库流水(method: erp-busiorder.goodsdocin.search 或 .goodsdocout.search)
func (c *Client) FetchStockIO(method string, query StockIOQuery, callback func([]StockIOItem) error) error {
	page := query.PageIndex
	total := 0
	for {
		query.PageIndex = page
		if query.PageSize == 0 {
			query.PageSize = 50
		}
		if query.Cols == "" {
			query.Cols = "goodsdocNo,inOutDate,inouttypeName,warehouseCode,warehouseName,billNo,vendCode,vendCustomerName,channelCode,createUserName,goodsdocRemark,goodsNo,goodsName,skuName,skuBarcode,unitName,batchNo,quantity,baceCurrencyCostPrice,baceCurrencyCostAmount,baceCurrencyNoTaxPrice,baceCurrencyWithTaxPrice,isCertified,goodsDetailRemark,recId"
		}

		resp, err := c.Call(method, query)
		if err != nil {
			return fmt.Errorf("call %s page %d: %w", method, page, err)
		}
		if resp.Code != 200 {
			return fmt.Errorf("%s error page %d: code=%d msg=%s", method, page, resp.Code, resp.Msg)
		}

		var wrapper struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(resp.Result, &wrapper); err != nil {
			return fmt.Errorf("unmarshal result page %d: %w", page, err)
		}

		var items []StockIOItem
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
		fmt.Printf("    已拉取 %d 条\n", total)

		if len(items) < query.PageSize {
			break
		}
		page++
	}
	return nil
}
