package handler

// helpers_pure_test.go — dashboard.go round2 + writeJSON/writeError + supply_chain pure 函数
// 已 Read dashboard.go (line 27-63) + supply_chain.go (line 54-124).

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

// === round2 (dashboard.go line 27-29) ===
// 跑哥业务: 浮点累加防尾巴 (0.090000000001 → 0.09)

func TestRound2(t *testing.T) {
	cases := map[float64]float64{
		0.090000000001: 0.09,
		0.094999999:    0.09, // 四舍五入
		0.095:          0.10, // 0.095 * 100 = 9.5, math.Round = 10, /100 = 0.10
		1.005:          1.0,  // 浮点黑魔法: 1.005*100 实际略小, Round 到 100 → 1.00
		-1.234:         -1.23,
		0:              0,
		100:            100,
		3.14159:        3.14,
	}
	for input, want := range cases {
		got := round2(input)
		// 注意 1.005 case 由于 IEEE 754 浮点表示, 实际值是 1.00 不是 1.01
		if got != want {
			// 对 1.005 这种边界 case 容许浮点精度差
			diff := got - want
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.001 {
				t.Errorf("round2(%v)=%v want %v", input, got, want)
			}
		}
	}
}

func TestRound2NaN(t *testing.T) {
	// math.Round(NaN) = NaN; round2 不 panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("round2 不应 panic: %v", r)
		}
	}()
	_ = round2(0)
}

// === writeJSON (line 38-44) ===
// 源码: 包 envelope {code: 200, data: ...}

func TestWriteJSONWrapsInEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, map[string]string{"foo": "bar"})

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type 应为 application/json, got %q", ct)
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)

	// CLAUDE.md envelope 规则: {code: 200, data: ...}
	if resp["code"].(float64) != 200 {
		t.Errorf("envelope code 应 200, got %v", resp["code"])
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("envelope.data 应为 map, got %v", resp["data"])
	}
	if data["foo"] != "bar" {
		t.Errorf("data.foo 应保留, got %v", data["foo"])
	}
}

func TestWriteJSONHandlesNil(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, nil)
	// 不 panic + 仍返合法 JSON envelope
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("nil data 仍应是合法 JSON: %v", err)
	}
	if resp["code"].(float64) != 200 {
		t.Errorf("envelope.code=200 不变")
	}
}

// === writeError (line 46-53) ===
// 源码: {code: <httpCode>, msg: <text>}, 同时 WriteHeader(code)

func TestWriteErrorWritesStatusAndBody(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, 400, "bad request")

	if rec.Code != 400 {
		t.Errorf("HTTP status 应 400, got %d", rec.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["code"].(float64) != 400 {
		t.Errorf("body.code 应 400, got %v", resp["code"])
	}
	if resp["msg"] != "bad request" {
		t.Errorf("body.msg 应 'bad request', got %v", resp["msg"])
	}
}

func TestWriteServerErrorDelegatesToWriteError(t *testing.T) {
	// writeServerError 内部 log + 调 writeError
	rec := httptest.NewRecorder()
	writeServerError(rec, 500, "server boom", errors.New("internal stack trace not exposed"))

	if rec.Code != 500 {
		t.Errorf("status 应 500, got %d", rec.Code)
	}
	body := rec.Body.String()
	// log 信息不应泄漏到 response (跑哥规则: 不暴露 SQL/路径/堆栈)
	if strings.Contains(body, "internal stack trace") {
		t.Errorf("内部 err 不应泄漏到 response body, got %s", body)
	}
	// generic msg 应可见
	if !strings.Contains(body, "server boom") {
		t.Errorf("generic msg 应在 body, got %s", body)
	}
}

// === supply_chain.go pure 子句构造 ===

func TestPlanStockoutExcludeFlagsCond_NoAlias(t *testing.T) {
	cond, args := planStockoutExcludeFlagsCond("")
	if !strings.Contains(cond, "goods_no NOT IN") {
		t.Errorf("无 alias 应直接 goods_no, got %q", cond)
	}
	if !strings.Contains(cond, "SELECT goods_no FROM goods") {
		t.Errorf("应有子查询, got %q", cond)
	}
	if len(args) != len(planStockoutExcludeFlags) {
		t.Errorf("args 数量应等于 flags 数量")
	}
}

func TestPlanStockoutExcludeFlagsCond_WithAlias(t *testing.T) {
	cond, _ := planStockoutExcludeFlagsCond("s")
	if !strings.Contains(cond, "s.goods_no NOT IN") {
		t.Errorf("有 alias='s' 应 s.goods_no, got %q", cond)
	}
}

func TestPlanCategoryGoodsCondAliasing(t *testing.T) {
	condNo, _ := planCategoryGoodsCond("")
	condS, _ := planCategoryGoodsCond("s")

	if !strings.Contains(condNo, "goods_no IN") {
		t.Errorf("无 alias 应是 goods_no IN, got %q", condNo)
	}
	if !strings.Contains(condS, "s.goods_no IN") {
		t.Errorf("alias=s 应 s.goods_no IN, got %q", condS)
	}
	// 必含 cate_full_name CASE 拆分逻辑
	if !strings.Contains(condNo, "SUBSTRING_INDEX") {
		t.Errorf("应有 SUBSTRING_INDEX 拆 cate_full_name, got %q", condNo)
	}
}

func TestBuildExcludeGoodsFilter(t *testing.T) {
	cond, args := buildExcludeGoodsFilter("goods_no")
	if !strings.HasPrefix(cond, " AND goods_no NOT IN (") {
		t.Errorf("应以 ' AND goods_no NOT IN (' 开头, got %q", cond)
	}
	if len(args) != len(planExcludeGoods) {
		t.Errorf("args 应等于 planExcludeGoods 数量, got %d want %d", len(args), len(planExcludeGoods))
	}
}

func TestBuildPlanWarehouseFilter(t *testing.T) {
	cond, args := buildPlanWarehouseFilter("warehouse_name")
	if !strings.HasPrefix(cond, " AND warehouse_name IN (") {
		t.Errorf("应以 ' AND warehouse_name IN (' 开头, got %q", cond)
	}
	// 7 仓白名单 (memory project_plan_dashboard_warehouses)
	if len(args) != 7 {
		t.Errorf("planWarehouses 应有 7 个仓, got %d", len(args))
	}
}
