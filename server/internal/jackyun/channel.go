package jackyun

import (
	"encoding/json"
	"fmt"
)

// ChannelQuery 销售渠道查询参数
type ChannelQuery struct {
	PageIndex       int    `json:"pageIndex"`
	PageSize        int    `json:"pageSize"`
	IsBlockup       int    `json:"isBlockup,omitempty"`       // 是否停用 0-否 1-是
	IsDelete        int    `json:"isDelete,omitempty"`         // 是否删除 0-否 1-是
	CateNames       string `json:"cateNames,omitempty"`        // 渠道分类
	ChannelCodes    string `json:"channelCodes,omitempty"`     // 渠道编号，逗号分隔
	ChannelIds      string `json:"channelIds,omitempty"`       // 渠道ID，逗号分隔
	GmtModifiedStart string `json:"gmtModifiedStart,omitempty"`
	GmtModifiedEnd   string `json:"gmtModifiedEnd,omitempty"`
}

// Channel 销售渠道信息
type Channel struct {
	ChannelId           json.Number `json:"channelId"`
	ChannelCode         string      `json:"channelCode"`
	ChannelName         string      `json:"channelName"`
	ChannelType         json.Number `json:"channelType"`         // 0:分销办公室 1:直营网店 2:直营门店 3:销售办公室 4:货主虚拟店 5:分销虚拟店 6:加盟门店 7:内部交易渠道
	OnlinePlatTypeCode  string      `json:"onlinePlatTypeCode"`  // 平台编码
	OnlinePlatTypeName  string      `json:"onlinePlatTypeName"`  // 平台名称(淘宝/京东/抖音...)
	ChannelDepartId     json.Number `json:"channelDepartId"`     // 负责部门ID
	ChannelDepartName   string      `json:"channelDepartName"`   // 负责部门名称
	CateId              json.Number `json:"cateId"`              // 渠道分类ID
	CateName            string      `json:"cateName"`            // 渠道分类名称
	CompanyId           json.Number `json:"companyId"`
	CompanyName         string `json:"companyName"`
	CompanyCode         string `json:"companyCode"`
	DepartCode          string `json:"departCode"`
	WarehouseCode       string `json:"warehouseCode"`
	WarehouseName       string `json:"warehouseName"`
	LinkMan             string `json:"linkMan"`
	LinkTel             string `json:"linkTel"`
	Memo                string `json:"memo"`
	PlatShopId          string `json:"platShopId"`
	PlatShopName        string `json:"platShopName"`
	ResponsibleUserName string      `json:"responsibleUserName"`
	ChargeType          json.Number `json:"chargeType"`          // 收费类型
	Email               string      `json:"email"`               // 邮箱
	GroupId             json.Number `json:"groupId"`             // 分组ID
	OfficeAddress       string      `json:"officeAddress"`       // 办公地址
	Postcode            string      `json:"postcode"`            // 邮编
	CityId              json.Number `json:"cityId"`              // 城市ID
	CityName            string      `json:"cityName"`            // 城市名称
	CountryId           json.Number `json:"countryId"`           // 国家ID
	CountryName         string      `json:"countryName"`         // 国家名称
	ProvinceId          json.Number `json:"provinceId"`          // 省份ID
	ProvinceName        string      `json:"provinceName"`        // 省份名称
	StreetId            json.Number `json:"streetId"`            // 街道ID
	StreetName          string      `json:"streetName"`          // 街道名称
	TownId              json.Number `json:"townId"`              // 镇ID
	TownName            string      `json:"townName"`            // 镇名称
	Field1              string      `json:"field1"`              // 自定义字段1
	Field2              string      `json:"field2"`              // 自定义字段2
	Field3              string      `json:"field3"`              // 自定义字段3
	Field4              string      `json:"field4"`              // 自定义字段4
	Field5              string      `json:"field5"`              // 自定义字段5
	Field6              string      `json:"field6"`              // 自定义字段6
	Field7              string      `json:"field7"`              // 自定义字段7
	Field8              string      `json:"field8"`              // 自定义字段8
	Field9              string      `json:"field9"`              // 自定义字段9
	Field10             string      `json:"field10"`             // 自定义字段10
	Field11             string      `json:"field11"`             // 自定义字段11
	Field12             string      `json:"field12"`             // 自定义字段12
	Field13             string      `json:"field13"`             // 自定义字段13
	Field14             string      `json:"field14"`             // 自定义字段14
	Field15             string      `json:"field15"`             // 自定义字段15
	Field16             string      `json:"field16"`             // 自定义字段16
	Field17             string      `json:"field17"`             // 自定义字段17
	Field18             string      `json:"field18"`             // 自定义字段18
	Field19             string      `json:"field19"`             // 自定义字段19
	Field20             string      `json:"field20"`             // 自定义字段20
	Field21             string      `json:"field21"`             // 自定义字段21
	Field22             string      `json:"field22"`             // 自定义字段22
	Field23             string      `json:"field23"`             // 自定义字段23
	Field24             string      `json:"field24"`             // 自定义字段24
	Field25             string      `json:"field25"`             // 自定义字段25
	Field26             string      `json:"field26"`             // 自定义字段26
	Field27             string      `json:"field27"`             // 自定义字段27
	Field28             string      `json:"field28"`             // 自定义字段28
	Field29             string      `json:"field29"`             // 自定义字段29
	Field30             string      `json:"field30"`             // 自定义字段30
}

// ChannelTypeName 渠道类型中文
var ChannelTypeName = map[string]string{
	"0": "分销办公室",
	"1": "直营网店",
	"2": "直营门店",
	"3": "销售办公室",
	"4": "货主虚拟店",
	"5": "分销虚拟店",
	"6": "加盟门店",
	"7": "内部交易渠道",
}

// FetchChannels 拉取所有销售渠道（分页）
func (c *Client) FetchChannels(callback func([]Channel) error) error {
	page := 0
	for {
		query := ChannelQuery{
			PageIndex: page,
			PageSize:  50,
			IsBlockup: 0,
			IsDelete:  0,
		}

		resp, err := c.Call("erp.sales.get", query)
		if err != nil {
			return fmt.Errorf("call channel api: %w", err)
		}
		if resp.Code != 200 {
			return fmt.Errorf("channel api error: code=%d msg=%s", resp.Code, resp.Msg)
		}

		var wrapper struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(resp.Result, &wrapper); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}

		// data 里是 salesChannelInfo 数组
		var dataWrapper struct {
			SalesChannelInfo []Channel `json:"salesChannelInfo"`
		}
		if err := json.Unmarshal(wrapper.Data, &dataWrapper); err != nil {
			// 可能 data 直接就是数组
			var items []Channel
			if err2 := json.Unmarshal(wrapper.Data, &items); err2 != nil {
				return fmt.Errorf("unmarshal channels: %w (also tried array: %w)", err, err2)
			}
			dataWrapper.SalesChannelInfo = items
		}

		items := dataWrapper.SalesChannelInfo
		if len(items) == 0 {
			break
		}

		if err := callback(items); err != nil {
			return fmt.Errorf("callback: %w", err)
		}

		if len(items) < 50 {
			break
		}
		page++
	}

	return nil
}
