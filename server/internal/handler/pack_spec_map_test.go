package handler

import "testing"

func TestValidatePackSpecRow(t *testing.T) {
	// 合法: 箱规必填>0, 托规可空
	if err := validatePackSpecRow("twl35g", 1, 0); err != nil {
		t.Errorf("箱规1托规0应合法(托规可空): %v", err)
	}
	if err := validatePackSpecRow("sxxtwl-480g", 36, 180); err != nil {
		t.Errorf("合法行不该报错: %v", err)
	}
	// 货品编码空
	if err := validatePackSpecRow("  ", 1, 0); err == nil {
		t.Error("货品编码空应报错")
	}
	// 箱规<=0
	if err := validatePackSpecRow("x", 0, 0); err == nil {
		t.Error("箱规0应报错")
	}
	if err := validatePackSpecRow("x", -5, 0); err == nil {
		t.Error("箱规负应报错")
	}
	// 托规负(有值时必须>0)
	if err := validatePackSpecRow("x", 1, -3); err == nil {
		t.Error("托规负应报错")
	}
}
