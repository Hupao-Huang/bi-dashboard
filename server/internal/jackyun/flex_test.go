package jackyun

// flex_test.go — FlexString / FlexFloat / FlexInt UnmarshalJSON 兼容性测试
// 已 Read trade.go line 10-85.
// 业务背景: 吉客云 API 返字段类型不稳定 (string vs number 混用), 这层兼容必须 100% 测.

import (
	"encoding/json"
	"testing"
)

// === FlexString ===

func TestFlexStringFromString(t *testing.T) {
	var f FlexString
	if err := json.Unmarshal([]byte(`"hello"`), &f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if f.String() != "hello" {
		t.Errorf("got %q want hello", f)
	}
}

func TestFlexStringFromNumber(t *testing.T) {
	// API 返 number, 应转 string
	var f FlexString
	if err := json.Unmarshal([]byte(`12345`), &f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if f.String() != "12345" {
		t.Errorf("got %q want '12345'", f)
	}
}

func TestFlexStringFromLongID(t *testing.T) {
	// 19 位 long id (memory feedback_go_long_id_precision: 必须保精度)
	// json.Number 转 string 不丢精度
	var f FlexString
	if err := json.Unmarshal([]byte(`1234567890123456789`), &f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if f.String() != "1234567890123456789" {
		t.Errorf("19 位 long id 必须保精度, got %q", f)
	}
}

func TestFlexStringFromNull(t *testing.T) {
	// 源码 line 27-28: 都失败时设 "" 返 nil
	var f FlexString
	if err := json.Unmarshal([]byte(`null`), &f); err != nil {
		t.Fatalf("null 不应 err: %v", err)
	}
	if f.String() != "" {
		t.Errorf("null 应 → '', got %q", f)
	}
}

// === FlexFloat ===

func TestFlexFloatFromNumber(t *testing.T) {
	var f FlexFloat
	if err := json.Unmarshal([]byte(`123.45`), &f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if f.Float64() != 123.45 {
		t.Errorf("got %v want 123.45", f.Float64())
	}
}

func TestFlexFloatFromString(t *testing.T) {
	var f FlexFloat
	if err := json.Unmarshal([]byte(`"99.99"`), &f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if f.Float64() != 99.99 {
		t.Errorf("got %v want 99.99", f.Float64())
	}
}

func TestFlexFloatFromEmptyString(t *testing.T) {
	// 源码 line 44-46: 空 string → 0
	var f FlexFloat
	if err := json.Unmarshal([]byte(`""`), &f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if f.Float64() != 0 {
		t.Errorf("空 string 应 → 0, got %v", f.Float64())
	}
}

func TestFlexFloatFromInvalidStringDefaultsZero(t *testing.T) {
	// 源码 line 53-54: 都失败 → 0
	var f FlexFloat
	json.Unmarshal([]byte(`"not-a-number"`), &f)
	if f.Float64() != 0 {
		t.Errorf("非法 string 应 → 0, got %v", f.Float64())
	}
}

// === FlexInt ===

func TestFlexIntFromNumber(t *testing.T) {
	var f FlexInt
	if err := json.Unmarshal([]byte(`42`), &f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if f.Int() != 42 {
		t.Errorf("got %d want 42", f.Int())
	}
}

func TestFlexIntFromString(t *testing.T) {
	var f FlexInt
	if err := json.Unmarshal([]byte(`"100"`), &f); err != nil {
		t.Fatalf("err: %v", err)
	}
	if f.Int() != 100 {
		t.Errorf("got %d want 100", f.Int())
	}
}

func TestFlexIntFromEmptyString(t *testing.T) {
	var f FlexInt
	json.Unmarshal([]byte(`""`), &f)
	if f.Int() != 0 {
		t.Errorf("空 string → 0, got %d", f.Int())
	}
}

// === 嵌套 struct UnmarshalJSON 应正常工作 ===

func TestFlexInStruct(t *testing.T) {
	type item struct {
		Name FlexString `json:"name"`
		Qty  FlexInt    `json:"qty"`
		Amt  FlexFloat  `json:"amt"`
	}
	// 混合类型: name=number, qty=string, amt=number
	jsonStr := `{"name": 999, "qty": "5", "amt": 19.99}`
	var it item
	if err := json.Unmarshal([]byte(jsonStr), &it); err != nil {
		t.Fatalf("err: %v", err)
	}
	if it.Name.String() != "999" {
		t.Errorf("Name(number→str): got %q", it.Name)
	}
	if it.Qty.Int() != 5 {
		t.Errorf("Qty(string→int): got %d", it.Qty.Int())
	}
	if it.Amt.Float64() != 19.99 {
		t.Errorf("Amt: got %v", it.Amt.Float64())
	}
}

// === min helper (client.go line 115-120) ===

func TestMinHelper(t *testing.T) {
	cases := []struct {
		a, b, want int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{5, 5, 5},
		{-1, -2, -2},
		{0, 0, 0},
	}
	for _, c := range cases {
		if got := min(c.a, c.b); got != c.want {
			t.Errorf("min(%d,%d)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}
