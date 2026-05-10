package yonsuite

// types_test.go — yonsuite 请求/响应 struct JSON 序列化测试
// 已 Read client.go line 192-226: SimpleVO / QueryOrder / PurchaseListReq / PurchaseListResp
//
// 业务场景: memory feedback_ys_simplevos_required - 日期过滤必须用 simpleVOs.
// JSON tag 不能错, 否则 YS API 不识别字段.

import (
	"encoding/json"
	"strings"
	"testing"
)

// SimpleVO JSON tag 必须严格 (memory: top-level vouchdate 静默失效)
func TestSimpleVOJSONTags(t *testing.T) {
	v := SimpleVO{
		Field:  "vouchdate",
		Op:     "between",
		Value1: "2026-05-01",
		Value2: "2026-05-31",
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	// 必含 4 个 tag
	for _, frag := range []string{`"field":"vouchdate"`, `"op":"between"`, `"value1":"2026-05-01"`, `"value2":"2026-05-31"`} {
		if !strings.Contains(got, frag) {
			t.Errorf("缺 fragment %q in %q", frag, got)
		}
	}
}

func TestSimpleVOValue2OmitEmpty(t *testing.T) {
	// Value2 omitempty: 没 between 时不出现
	v := SimpleVO{Field: "id", Op: "eq", Value1: "123"}
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "value2") {
		t.Errorf("空 Value2 必须 omitempty, got %s", string(b))
	}
}

func TestQueryOrderJSON(t *testing.T) {
	q := QueryOrder{Field: "vouchdate", Order: "desc"}
	b, _ := json.Marshal(q)
	got := string(b)
	if !strings.Contains(got, `"field":"vouchdate"`) {
		t.Errorf("缺 field, got %s", got)
	}
	if !strings.Contains(got, `"order":"desc"`) {
		t.Errorf("缺 order, got %s", got)
	}
}

// PurchaseListReq 完整序列化 (含嵌套 SimpleVO + QueryOrder)
func TestPurchaseListReqFullSerialize(t *testing.T) {
	req := PurchaseListReq{
		PageIndex: 1,
		PageSize:  100,
		IsSum:     false,
		SimpleVOs: []SimpleVO{
			{Field: "vouchdate", Op: "between", Value1: "2026-05-01", Value2: "2026-05-31"},
		},
		QueryOrders: []QueryOrder{
			{Field: "vouchdate", Order: "desc"},
		},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := string(b)
	for _, frag := range []string{`"pageIndex":1`, `"pageSize":100`, `"isSum":false`, `"simpleVOs"`, `"queryOrders"`} {
		if !strings.Contains(got, frag) {
			t.Errorf("缺 %q in %q", frag, got)
		}
	}
}

// PurchaseListResp Unmarshal: data.recordList 必须是 []map[string]interface{}
// (memory feedback_ys_stock_data_array: 现存量是直接 array, 不是 data.recordList)
// 注意: 采购订单是 recordList, 跟 stock 不同
func TestPurchaseListRespUnmarshal(t *testing.T) {
	respJSON := `{
		"code": "200",
		"message": "ok",
		"data": {
			"pageIndex": 1,
			"pageSize": 100,
			"recordCount": 250,
			"pageCount": 3,
			"recordList": [
				{"id": "12345", "vouchdate": "2026-05-01", "amount": 99.99}
			]
		}
	}`
	var resp PurchaseListResp
	if err := json.Unmarshal([]byte(respJSON), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Code != "200" || resp.Message != "ok" {
		t.Errorf("code/message 解析失败")
	}
	if resp.Data.PageCount != 3 || resp.Data.RecordCount != 250 {
		t.Errorf("分页字段失败: %+v", resp.Data)
	}
	if len(resp.Data.RecordList) != 1 {
		t.Fatalf("recordList 应有 1 条, got %d", len(resp.Data.RecordList))
	}
	rec := resp.Data.RecordList[0]
	if rec["id"] != "12345" {
		t.Errorf("recordList[0].id 应保留原始字段, got %v", rec["id"])
	}
}
