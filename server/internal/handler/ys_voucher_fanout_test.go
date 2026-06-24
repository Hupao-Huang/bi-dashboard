package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"bi-dashboard/internal/yonsuite"
)

func TestFlattenVoucherRecordsTagsAccbook(t *testing.T) {
	recs := []map[string]interface{}{
		{
			"header": map[string]interface{}{
				"id":              "1234567890123456789",
				"period":          "2026-06",
				"displayname":     "转-1",
				"vouchertype":     map[string]interface{}{"name": "转账凭证"},
				"description":     "计提工资",
				"totaldebit_org":  float64(1000),
				"totalcredit_org": float64(1000),
				"srcsystem":       "总账",
				"maker":           map[string]interface{}{"name": "张三"},
				"voucherstatus":   "04",
				"maketime":        "2026-06-30",
				"attachedbill":    "2",
			},
			"body": []interface{}{
				map[string]interface{}{
					"recordnumber":   "1",
					"description":    "计提工资",
					"accsubject":     map[string]interface{}{"code": "6601", "name": "管理费用"},
					"auxiliaryShow":  "行政部",
					"debit_original": float64(1000),
				},
			},
		},
	}
	rows := flattenVoucherRecords(recs, "ZJ001", "浙江松鲜鲜")
	if len(rows) != 1 {
		t.Fatalf("应抽平 1 行, got %d", len(rows))
	}
	r := rows[0]
	if r.AccbookCode != "ZJ001" || r.AccbookName != "浙江松鲜鲜" {
		t.Errorf("账簿标记错: code=%q name=%q", r.AccbookCode, r.AccbookName)
	}
	if r.ID != "1234567890123456789" {
		t.Errorf("19 位 id 应原样 string, got %q", r.ID)
	}
	if r.VoucherNo != "转-1" || r.VoucherType != "转账凭证" {
		t.Errorf("字号/类型错: %q %q", r.VoucherNo, r.VoucherType)
	}
	if r.Status != "已记账" {
		t.Errorf("状态 04 应→已记账, got %q", r.Status)
	}
	if r.TotalDebit != 1000 || r.TotalCredit != 1000 {
		t.Errorf("借贷合计错: %v %v", r.TotalDebit, r.TotalCredit)
	}
	if len(r.Lines) != 1 || r.Lines[0].SubjectName != "管理费用" {
		t.Errorf("分录错: %+v", r.Lines)
	}
}

// dummyVoucherRecords 造 n 条最小 record (fanOut 只数行数+读 RecordCount, 不关心字段)
func dummyVoucherRecords(n int) []map[string]interface{} {
	out := make([]map[string]interface{}, n)
	for i := range out {
		out[i] = map[string]interface{}{"header": map[string]interface{}{"id": "x"}}
	}
	return out
}

func voucherResp(recordCount, n int) *yonsuite.VoucherListResp {
	r := &yonsuite.VoucherListResp{}
	r.Data.RecordCount = recordCount
	r.Data.RecordList = dummyVoucherRecords(n)
	return r
}

func TestFanOutVouchersFastMode(t *testing.T) {
	nameOf := map[string]string{"A": "甲账簿", "B": "乙账簿"}
	calls := 0
	fetch := func(code string, pi, ps int) (*yonsuite.VoucherListResp, error) {
		calls++
		if ps != voucherPerBookLimit {
			t.Errorf("快模式每本应只拉 %d 条, got pageSize=%d", voucherPerBookLimit, ps)
		}
		switch code {
		case "A":
			return voucherResp(50, 20), nil // 甲账簿共 50, 只拉到 20
		case "B":
			return voucherResp(5, 5), nil
		}
		return voucherResp(0, 0), nil
	}
	rows, meta, truncated := fanOutVouchers([]string{"A", "B"}, nameOf, false, fetch)
	if calls != 2 {
		t.Errorf("快模式 2 本账应只调 2 次, got %d", calls)
	}
	if len(rows) != 25 {
		t.Errorf("合并行数应 20+5=25, got %d", len(rows))
	}
	if truncated {
		t.Error("快模式不应标 truncated")
	}
	if meta[0].RecordCount != 50 || meta[0].Fetched != 20 || meta[0].Name != "甲账簿" {
		t.Errorf("甲账簿 meta 错: %+v", meta[0])
	}
	if meta[1].RecordCount != 5 || meta[1].Fetched != 5 {
		t.Errorf("乙账簿 meta 错: %+v", meta[1])
	}
}

func TestFanOutVouchersFullModePaginates(t *testing.T) {
	nameOf := map[string]string{"A": "甲"}
	pages := map[int]int{1: 200, 2: 200, 3: 50} // 共 450
	fetch := func(code string, pi, ps int) (*yonsuite.VoucherListResp, error) {
		if ps != voucherFullPageSize {
			t.Errorf("拉全每页应 %d, got %d", voucherFullPageSize, ps)
		}
		return voucherResp(450, pages[pi]), nil
	}
	rows, meta, truncated := fanOutVouchers([]string{"A"}, nameOf, true, fetch)
	if len(rows) != 450 {
		t.Errorf("拉全应翻页到 450, got %d", len(rows))
	}
	if truncated {
		t.Error("450 行未触顶, 不应 truncated")
	}
	if meta[0].Fetched != 450 || meta[0].RecordCount != 450 {
		t.Errorf("meta 错: %+v", meta[0])
	}
}

func TestFanOutVouchersCapTruncates(t *testing.T) {
	nameOf := map[string]string{"A": "甲"}
	fetch := func(code string, pi, ps int) (*yonsuite.VoucherListResp, error) {
		return voucherResp(1000000, voucherFullPageSize), nil // 永远还有更多
	}
	rows, _, truncated := fanOutVouchers([]string{"A"}, nameOf, true, fetch)
	if !truncated {
		t.Error("超大账簿拉全应触顶 truncated")
	}
	if len(rows) > voucherMaxRows {
		t.Errorf("行数不应超过闸值 %d, got %d", voucherMaxRows, len(rows))
	}
}

func TestFanOutVouchersPerBookErrorIsolated(t *testing.T) {
	nameOf := map[string]string{"A": "甲", "B": "乙"}
	fetch := func(code string, pi, ps int) (*yonsuite.VoucherListResp, error) {
		if code == "A" {
			return nil, errVoucherTest
		}
		return voucherResp(3, 3), nil
	}
	rows, meta, _ := fanOutVouchers([]string{"A", "B"}, nameOf, false, fetch)
	if len(rows) != 3 {
		t.Errorf("甲失败应只剩乙的 3 行, got %d", len(rows))
	}
	if meta[0].Error == "" {
		t.Error("甲账簿应记 error")
	}
	if meta[1].Error != "" || meta[1].Fetched != 3 {
		t.Errorf("乙账簿不应受影响: %+v", meta[1])
	}
}

var errVoucherTest = &voucherTestErr{}

type voucherTestErr struct{}

func (*voucherTestErr) Error() string { return "用友连接失败(测试)" }

func TestGetVoucherListEmptyCodes400(t *testing.T) {
	// YS 非 nil(用 NewClient 构造但永不拨号), accbookCodes 为空应在调用用友前就 400
	h := &DashboardHandler{YS: yonsuite.NewClient("k", "s", "http://127.0.0.1:0")}
	req := httptest.NewRequest("POST", "/api/finance/voucher/list",
		strings.NewReader(`{"accbookCodes":[],"periodStart":"2026-06","periodEnd":"2026-06"}`))
	w := httptest.NewRecorder()
	h.GetVoucherList(w, req)
	if w.Code != 400 {
		t.Fatalf("空账簿应返回 400, got %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "请选择账簿") {
		t.Errorf("应提示请选择账簿, got %s", w.Body.String())
	}
}
