package specialchannel

// 黄金测试: 锁定 2026-06-12 收敛时各散落点的原始口径值
// 注册表任何字段漂移(改错店名/仓号/部门归属)都会在这里炸, 而不是静默改了业务口径

import (
	"reflect"
	"testing"
)

func TestRegistryGoldenValues(t *testing.T) {
	// 原 special_channel.go channelMapByDept + special_channel_price.go priceChannelsByDept
	if got := KeysByDept(DeptEcommerce); !reflect.DeepEqual(got, []string{"京东", "猫超"}) {
		t.Errorf("电商渠道 key 漂移: %v", got)
	}
	if got := KeysByDept(DeptInstantRetail); !reflect.DeepEqual(got, []string{"朴朴", "小象", "叮咚"}) {
		t.Errorf("即时零售渠道 key 漂移: %v", got)
	}
	// 原 sync-allocate warehouseMap (仓号 → 渠道)
	wantWh := map[string]string{"京东": "0057", "猫超": "0019", "朴朴": "0110", "小象": "0112", "叮咚": "0111"}
	for k, wh := range wantWh {
		c, ok := ByKey(k)
		if !ok || c.WarehouseCode != wh {
			t.Errorf("渠道 %s 仓号漂移: got %q want %q", k, c.WarehouseCode, wh)
		}
	}
	// 原 dashboard_overview.go jdShopID/tmcsShopID
	if c, _ := ByKey("京东"); c.ShopID != "1819610592561398400" {
		t.Errorf("京东 shopID 漂移: %q", c.ShopID)
	}
	if c, _ := ByKey("猫超"); c.ShopID != "1819610591915475584" {
		t.Errorf("猫超 shopID 漂移: %q", c.ShopID)
	}
	// 原 dashboard_department.go instantRetailAllotShop (渠道 → 店名)
	wantShop := map[string]string{
		"朴朴": "js-即时零售事业一部（世创）-朴朴",
		"小象": "js-即时零售事业一部（世创）-小象",
		"叮咚": "js-即时零售事业一部（杭州松鲜鲜）-叮咚",
	}
	if got := ShopNameByKey(DeptInstantRetail); !reflect.DeepEqual(got, wantShop) {
		t.Errorf("即时零售店名映射漂移: %v", got)
	}
	// 原 ecommerceExcludeAllotCond 的 NOT IN 清单
	if got := ShopNamesByDept(DeptEcommerce); !reflect.DeepEqual(got, []string{"ds-京东-清心湖自营", "ds-天猫超市-寄售"}) {
		t.Errorf("电商排除店名漂移: %v", got)
	}
	// 朴朴纯调拨语义 (始终纳入) vs 小象/叮咚价格门控
	if got := PureAllotKeys(DeptInstantRetail); !reflect.DeepEqual(got, []string{"朴朴"}) {
		t.Errorf("纯调拨渠道漂移: %v", got)
	}
	if got := PriceGatedKeys(DeptInstantRetail); !reflect.DeepEqual(got, []string{"小象", "叮咚"}) {
		t.Errorf("价格门控渠道漂移: %v", got)
	}
	// 合法 key 集合
	for _, k := range []string{"京东", "猫超", "朴朴", "小象", "叮咚"} {
		if !IsValidKey(k) {
			t.Errorf("%s 应是合法渠道", k)
		}
	}
	if IsValidKey("七鲜") {
		t.Error("七鲜还没加, 不应合法 (加的时候改 All + 本测试)")
	}
}
