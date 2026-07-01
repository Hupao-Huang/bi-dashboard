package handler

import "testing"

func TestValidateChannelMapRow(t *testing.T) {
	// 合法
	if err := validateChannelMapRow("ds-京东-某店", "京东", "电商"); err != nil {
		t.Errorf("合法行不该报错: %v", err)
	}
	// 店铺空
	if err := validateChannelMapRow("  ", "京东", "电商"); err == nil {
		t.Error("店铺空应报错")
	}
	// 渠道空
	if err := validateChannelMapRow("ds-京东-某店", "", "电商"); err == nil {
		t.Error("渠道空应报错")
	}
	// 平台非法
	if err := validateChannelMapRow("ds-京东-某店", "京东", "乱写"); err == nil {
		t.Error("平台非三选一应报错")
	}
	// 平台三个合法值
	for _, p := range []string{"社媒", "电商", "其他"} {
		if err := validateChannelMapRow("店", "渠道", p); err != nil {
			t.Errorf("平台 %q 应合法: %v", p, err)
		}
	}
}
