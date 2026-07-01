package handler

import "testing"

func TestValidatePackSpecRow(t *testing.T) {
	// 合法: 销售规格必填>0, 装箱规格/托规可空(=0)
	if err := validatePackSpecRow("twl35g", 1, 0, 0); err != nil {
		t.Errorf("销售规格1装箱规格0托规0应合法(装箱规格/托规可空): %v", err)
	}
	if err := validatePackSpecRow("twl35g", 1, 150, 64); err != nil {
		t.Errorf("单品散卖 销售规格1装箱规格150托规64 应合法: %v", err)
	}
	if err := validatePackSpecRow("sxxtwl-480g", 36, 36, 180); err != nil {
		t.Errorf("合法行不该报错: %v", err)
	}
	// 货品编码空
	if err := validatePackSpecRow("  ", 1, 0, 0); err == nil {
		t.Error("货品编码空应报错")
	}
	// 销售规格<=0
	if err := validatePackSpecRow("x", 0, 0, 0); err == nil {
		t.Error("销售规格0应报错")
	}
	if err := validatePackSpecRow("x", -5, 0, 0); err == nil {
		t.Error("销售规格负应报错")
	}
	// 装箱规格负应报错
	if err := validatePackSpecRow("x", 1, -2, 0); err == nil {
		t.Error("装箱规格负应报错")
	}
	// 托规负应报错
	if err := validatePackSpecRow("x", 1, 0, -3); err == nil {
		t.Error("托规负应报错")
	}
}
