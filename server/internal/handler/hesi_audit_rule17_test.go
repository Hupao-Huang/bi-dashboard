package handler

// 规则 17 补贴日期须在出差申请单起止内 + 规则 18 广告费发票项目名称 (财务 2026-06-12)
// + 15-1.2 修正: 交通及差旅费类型豁免付款截图

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const r17Day = int64(86400000)
const r17June1 = int64(1780272000000) // 2026-06-01 00:00 UTC

// mkR17Raw 报销单: 1条补贴明细(日期区间) + 1个关联申请单
func mkR17Raw(subStart, subEnd int64) map[string]interface{} {
	return map[string]interface{}{
		"expenseLinks": []interface{}{"ID01REQFLOW1"},
		"details": []interface{}{
			map[string]interface{}{
				"feeTypeId": "ID01Fk0MQBAAQ7",
				"feeTypeForm": map[string]interface{}{
					"detailId": "D-sub", "detailNo": float64(2),
					"feeDatePeriod": map[string]interface{}{"start": float64(subStart), "end": float64(subEnd)},
				},
			},
		},
	}
}

// mkR17Handler 申请单 raw_json 带出差起止日期 6/1~6/5
func mkR17Handler(t *testing.T, tripStart, tripEnd int64) (*DashboardHandler, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	reqRaw := fmt.Sprintf(`{"u_出差起止日期":{"start":%d,"end":%d}}`, tripStart, tripEnd)
	mock.ExpectQuery(`SELECT code, IFNULL\(raw_json,''\) FROM hesi_flow WHERE flow_id=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"code", "raw_json"}).AddRow("S26001802", reqRaw))
	return &DashboardHandler{DB: db}, func() { db.Close() }
}

func TestRule17PeriodOverlapOK(t *testing.T) {
	// 补贴区间 6/2-6/8 与出差 6/1-6/5 有交集 → 通过 (按月报销惯例, 区间宽容判)
	h, done := mkR17Handler(t, r17June1, r17June1+4*r17Day)
	defer done()
	rej := h.ruleSubsidyDateInTrip(mkR17Raw(r17June1+r17Day, r17June1+7*r17Day))
	if len(rej) != 0 {
		t.Errorf("区间有交集不应驳回: %v", rej)
	}
}

func TestRule17PeriodNoOverlapRejects(t *testing.T) {
	// 补贴区间 6/10-6/12 与出差 6/1-6/5 完全不重叠 → 驳回
	h, done := mkR17Handler(t, r17June1, r17June1+4*r17Day)
	defer done()
	rej := h.ruleSubsidyDateInTrip(mkR17Raw(r17June1+9*r17Day, r17June1+11*r17Day))
	got := strings.Join(rej, "; ")
	if !strings.Contains(got, "规则 17") || !strings.Contains(got, "完全不重叠") {
		t.Errorf("区间完全不重叠应驳回, got %q", got)
	}
}

func TestRule17SingleDateOutsideRejects(t *testing.T) {
	// 单日补贴 6/8 不在出差 6/1-6/5 内 → 驳回 (单日严格判)
	h, done := mkR17Handler(t, r17June1, r17June1+4*r17Day)
	defer done()
	raw := map[string]interface{}{
		"expenseLinks": []interface{}{"ID01REQFLOW1"},
		"details": []interface{}{
			map[string]interface{}{
				"feeTypeId": "ID01Fk0MQBAAQ7",
				"feeTypeForm": map[string]interface{}{
					"detailId": "D-sub", "detailNo": float64(3),
					"feeDate": float64(r17June1 + 7*r17Day),
				},
			},
		},
	}
	rej := h.ruleSubsidyDateInTrip(raw)
	got := strings.Join(rej, "; ")
	if !strings.Contains(got, "规则 17") || !strings.Contains(got, "06-08") {
		t.Errorf("单日超范围应驳回, got %q", got)
	}
}

func TestRule17NoTripDatesSkips(t *testing.T) {
	// 申请单没有出差起止日期 → 不判
	db, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer db.Close()
	mock.ExpectQuery(`SELECT code, IFNULL\(raw_json,''\) FROM hesi_flow WHERE flow_id=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"code", "raw_json"}).AddRow("S26001802", `{"title":"无起止日期"}`))
	h := &DashboardHandler{DB: db}
	if rej := h.ruleSubsidyDateInTrip(mkR17Raw(r17June1, r17June1+r17Day)); len(rej) != 0 {
		t.Errorf("申请单无起止日期不应判, got %v", rej)
	}
}

func TestRule17NoSubsidyDateSkips(t *testing.T) {
	// 补贴明细没填日期 → 不判, 不查库
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := &DashboardHandler{DB: db}
	raw := map[string]interface{}{
		"expenseLinks": []interface{}{"ID01REQFLOW1"},
		"details": []interface{}{
			map[string]interface{}{
				"feeTypeId":   "ID01Fk0MQBAAQ7",
				"feeTypeForm": map[string]interface{}{"detailId": "D-sub", "detailNo": float64(1)},
			},
		},
	}
	if rej := h.ruleSubsidyDateInTrip(raw); len(rej) != 0 {
		t.Errorf("无日期补贴不应判, got %v", rej)
	}
}

// ===== 规则 18 判定层 =====

func TestRule18NoAdKeywordRejects(t *testing.T) {
	// 发票明细全是印刷品 → 驳回 (财务给的真实案例 B26002472)
	invs := []adInvRef{{no: 1, invoiceID: "INV1", number: "26412000001346794456"}}
	names := map[string][]string{"INV1": {"*印刷品*木质展示架", "*印刷品*KT板立牌"}}
	rej := checkAdInvoiceItems(invs, names)
	got := strings.Join(rej, "; ")
	if !strings.Contains(got, "规则 18") || !strings.Contains(got, "46794456") {
		t.Errorf("项目名称无广告/推广应驳回并带发票尾号, got %q", got)
	}
}

func TestRule18AdKeywordPasses(t *testing.T) {
	// 任一明细行含"广告"或"推广" → 通过
	invs := []adInvRef{
		{no: 1, invoiceID: "INV1", number: "111"},
		{no: 2, invoiceID: "INV2", number: "222"},
	}
	names := map[string][]string{
		"INV1": {"*现代服务*广告设计费"},
		"INV2": {"*信息技术服务*线上推广服务费"},
	}
	if rej := checkAdInvoiceItems(invs, names); len(rej) != 0 {
		t.Errorf("含广告/推广不应驳回, got %v", rej)
	}
}

func TestRule18NoItemsSkips(t *testing.T) {
	// 接口没回该发票的明细 → 不冤枉, 跳过
	invs := []adInvRef{{no: 1, invoiceID: "INV1", number: "111"}}
	if rej := checkAdInvoiceItems(invs, map[string][]string{}); len(rej) != 0 {
		t.Errorf("无明细数据不应驳回, got %v", rej)
	}
}

// ===== 15-1.2 修正: 交通及差旅费豁免付款截图 =====

func TestRule1512TravelFeeTypeExempt(t *testing.T) {
	// 火车费用类型 + 铁路电子票(非专票) + 没传付款截图 → 不再驳回 (财务 6/12)
	inv := emptyInvRows().AddRow("D-x", "ELECTRONIC_TRAIN_INVOICE", 88.00, 0)
	h, done := mkOfflineHandler(t, inv, "集团经理")
	defer done()
	raw := rawOf(map[string]interface{}{
		"feeTypeId": "ID01Fk0IZFCb03", // 火车 (交通及差旅费子类)
		"feeTypeForm": map[string]interface{}{
			"detailId": "D-x", "detailNo": float64(1),
			"amount": map[string]interface{}{"standard": "88.00"},
		},
	})
	rej, _ := h.ruleOfflineExtras(raw, "F15", "S1")
	if strings.Contains(strings.Join(rej, "; "), "规则 15-1.2") {
		t.Errorf("交通及差旅费类型应豁免付款截图, got %v", rej)
	}
}
