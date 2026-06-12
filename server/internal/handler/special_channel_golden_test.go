package handler

// 渠道清单收敛 (2026-06-12 第三批) 黄金测试:
// 从 specialchannel 注册表生成的 SQL 片段必须与收敛前的硬编码字面量逐字相同

import "testing"

func TestEcommerceExcludeAllotCondGolden(t *testing.T) {
	const legacy = ` AND shop_name NOT IN ('ds-京东-清心湖自营','ds-天猫超市-寄售')`
	if ecommerceExcludeAllotCond != legacy {
		t.Errorf("电商排除条件漂移:\n got  %q\n want %q", ecommerceExcludeAllotCond, legacy)
	}
	const legacyAlias = ` AND s.shop_name NOT IN ('ds-京东-清心湖自营','ds-天猫超市-寄售')`
	if ecommerceExcludeAllotCondAlias != legacyAlias {
		t.Errorf("电商排除条件(别名)漂移:\n got  %q\n want %q", ecommerceExcludeAllotCondAlias, legacyAlias)
	}
}

func TestOverviewShopConstsGolden(t *testing.T) {
	if jdShopID != "1819610592561398400" || tmcsShopID != "1819610591915475584" {
		t.Errorf("shopID 漂移: jd=%q tmcs=%q", jdShopID, tmcsShopID)
	}
	if jdShopName != "ds-京东-清心湖自营" || tmcsShopNm != "ds-天猫超市-寄售" {
		t.Errorf("店名漂移: jd=%q tmcs=%q", jdShopName, tmcsShopNm)
	}
	if jdChanKey != "京东" || tmcsChanKey != "猫超" {
		t.Errorf("渠道 key 漂移: jd=%q tmcs=%q", jdChanKey, tmcsChanKey)
	}
	want := map[string]string{
		"朴朴": "js-即时零售事业一部（世创）-朴朴",
		"小象": "js-即时零售事业一部（世创）-小象",
		"叮咚": "js-即时零售事业一部（杭州松鲜鲜）-叮咚",
	}
	for k, v := range want {
		if instantRetailAllotShop[k] != v {
			t.Errorf("即时零售店名漂移 %s: %q", k, instantRetailAllotShop[k])
		}
	}
	if len(instantRetailAllotShop) != len(want) {
		t.Errorf("即时零售渠道数漂移: %d", len(instantRetailAllotShop))
	}
}
