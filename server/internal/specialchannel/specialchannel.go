// Package specialchannel 是"特殊渠道调拨当销售"业务的单一事实来源。
//
// 背景: 京东/猫超/朴朴/小象/叮咚 这些渠道按调拨单算销售额, 渠道清单原来散落 9 处硬编码
// (special_channel.go / special_channel_price.go / dashboard_department.go / dashboard_overview.go /
// cmd/sync-allocate / cmd/import-channel-price), 加一个渠道要改 6-9 个文件, 漏一处数据就不一致
// (2026-06-12 全仓审查收敛, 详 CODE_REVIEW_20260612.md 第 10 条)。
//
// 加新渠道(例"七鲜")只需:
//  1. All 里追加一行 (Key/Dept/ChannelName/WarehouseCode, 有销售单的渠道再配 ShopID)
//  2. 重新编译部署 bi-server + sync-allocate.exe + import-channel-price.exe
//  3. 对账页价格表里给商品配价 (PureAllot=false 的渠道配了价才自动进看板)
package specialchannel

// Channel 一个特殊渠道的全部口径字段
type Channel struct {
	Key           string // 渠道短名 (前端 Tab / 价格表 sheet 名 / channel_special_price.channel_key)
	Dept          string // 归属部门: ecommerce / instant_retail
	ChannelName   string // 调拨单渠道全称 (= allocate_orders.channel_name = 店铺看板店名)
	ShopID        string // sales_goods_summary.shop_id — 注意: 跟 ChannelName 是两套不同维度的标识, 仅有销售单的渠道需要
	WarehouseCode string // 吉客云调拨外仓编码 (sync-allocate 按此拉单)
	PureAllot     bool   // true=纯调拨无销售单(朴朴): 不依赖价格表始终纳入看板; false=有销售单: 配了价格表才纳入(防金额0件数真导致客单价失真)
}

const (
	DeptEcommerce     = "ecommerce"
	DeptInstantRetail = "instant_retail"
)

// All 渠道注册表 — 顺序即前端展示顺序, 加渠道只改这里
var All = []Channel{
	{Key: "京东", Dept: DeptEcommerce, ChannelName: "ds-京东-清心湖自营", ShopID: "1819610592561398400", WarehouseCode: "0057"},
	{Key: "猫超", Dept: DeptEcommerce, ChannelName: "ds-天猫超市-寄售", ShopID: "1819610591915475584", WarehouseCode: "0019"},
	{Key: "朴朴", Dept: DeptInstantRetail, ChannelName: "js-即时零售事业一部（世创）-朴朴", WarehouseCode: "0110", PureAllot: true},
	{Key: "小象", Dept: DeptInstantRetail, ChannelName: "js-即时零售事业一部（世创）-小象", WarehouseCode: "0112"},
	{Key: "叮咚", Dept: DeptInstantRetail, ChannelName: "js-即时零售事业一部（杭州松鲜鲜）-叮咚", WarehouseCode: "0111"},
}

// ByDept 按部门取渠道 (保持 All 的顺序); dept 为空返回全部
func ByDept(dept string) []Channel {
	if dept == "" {
		return All
	}
	out := make([]Channel, 0, len(All))
	for _, c := range All {
		if c.Dept == dept {
			out = append(out, c)
		}
	}
	return out
}

// KeysByDept 按部门取渠道短名列表 (保持顺序); dept 为空返回全部
func KeysByDept(dept string) []string {
	chans := ByDept(dept)
	out := make([]string, 0, len(chans))
	for _, c := range chans {
		out = append(out, c.Key)
	}
	return out
}

// ByKey 按短名取渠道
func ByKey(key string) (Channel, bool) {
	for _, c := range All {
		if c.Key == key {
			return c, true
		}
	}
	return Channel{}, false
}

// IsValidKey 短名是否是合法渠道
func IsValidKey(key string) bool {
	_, ok := ByKey(key)
	return ok
}

// ShopNamesByDept 按部门取渠道全称(店名)列表 (保持顺序)
func ShopNamesByDept(dept string) []string {
	chans := ByDept(dept)
	out := make([]string, 0, len(chans))
	for _, c := range chans {
		out = append(out, c.ChannelName)
	}
	return out
}

// ShopNameByKey 短名 → 渠道全称(店名) map (dept 为空取全部)
func ShopNameByKey(dept string) map[string]string {
	m := map[string]string{}
	for _, c := range ByDept(dept) {
		m[c.Key] = c.ChannelName
	}
	return m
}

// PureAllotKeys 纯调拨渠道短名 (始终纳入看板, 不依赖价格表)
func PureAllotKeys(dept string) []string {
	out := []string{}
	for _, c := range ByDept(dept) {
		if c.PureAllot {
			out = append(out, c.Key)
		}
	}
	return out
}

// PriceGatedKeys 有销售单的渠道短名 (配了价格表才纳入看板)
func PriceGatedKeys(dept string) []string {
	out := []string{}
	for _, c := range ByDept(dept) {
		if !c.PureAllot {
			out = append(out, c.Key)
		}
	}
	return out
}
