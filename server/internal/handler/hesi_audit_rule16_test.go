package handler

// 规则 16 企业支付行程防重复报销 (跑哥 2026-06-11)
// 同车次+同乘车人+票面价=发票金额 → 驳回; 票价不同 → 人工核; 乘车人不同 → 不触发

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// mkR16Handler 按 ruleCorpPaidDuplicate 查询顺序挂期望:
// ① hesi_flow code 反查 ② hesi_travel_order ③ hesi_flow_invoice
func mkR16Handler(t *testing.T, orderRows, invRows *sqlmock.Rows) (*DashboardHandler, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	mock.ExpectQuery(`SELECT code FROM hesi_flow WHERE flow_id=\?`).
		WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("S26001802"))
	mock.ExpectQuery(`FROM hesi_travel_order`).WillReturnRows(orderRows)
	mock.ExpectQuery(`FROM hesi_flow_invoice WHERE flow_id=\?`).WillReturnRows(invRows)
	return &DashboardHandler{DB: db}, func() { db.Close() }
}

func r16OrderRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"trip_no", "traveler", "req_code", "corp_pay", "raw_json"})
}

func r16InvRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"detail_id", "train_no", "passenger", "total_amount"})
}

func r16Raw() map[string]interface{} {
	return map[string]interface{}{
		"expenseLinks": []interface{}{"ID01REQFLOW1"},
		"details": []interface{}{
			map[string]interface{}{
				"feeTypeId": "ID01TRAVEL",
				"feeTypeForm": map[string]interface{}{
					"detailId": "D-train", "detailNo": float64(3),
				},
			},
		},
	}
}

func TestRule16ExactFareMatchRejects(t *testing.T) {
	// 公司企业支付 G7581 票面价119 (企业付122含服务费), 发票同车次同人 ¥119 → 驳回
	orders := r16OrderRows().AddRow("G7581", "郑华坤", "S26001802", 122.00, `{"票面价":{"standard":"119.00"}}`)
	inv := r16InvRows().AddRow("D-train", "G7581", "郑华坤", 119.00)
	h, done := mkR16Handler(t, orders, inv)
	defer done()
	rej, warn := h.ruleCorpPaidDuplicate(r16Raw(), "F16")
	got := strings.Join(rej, "; ")
	if !strings.Contains(got, "规则 16") || !strings.Contains(got, "明细#3") {
		t.Errorf("票面价一致应驳回且标明细号, rej=%q warn=%v", got, warn)
	}
}

func TestRule16FareMismatchWarns(t *testing.T) {
	// 同车次同人但票价不同 (118 vs 85) → 转人工核, 不直接驳回
	orders := r16OrderRows().AddRow("D7788", "孙东禹", "S26001802", 88.00, `{"票面价":{"standard":"85.00"}}`)
	inv := r16InvRows().AddRow("D-train", "D7788", "孙东禹", 118.00)
	h, done := mkR16Handler(t, orders, inv)
	defer done()
	rej, warn := h.ruleCorpPaidDuplicate(r16Raw(), "F16")
	if len(rej) != 0 {
		t.Errorf("票价不同不应直接驳回, rej=%v", rej)
	}
	if !strings.Contains(strings.Join(warn, "; "), "人工核") {
		t.Errorf("票价不同应转人工核, warn=%v", warn)
	}
}

func TestRule16DifferentPassengerNoHit(t *testing.T) {
	// 同车次但乘车人不同 → 不触发 (同事同行各自买票是正常的)
	orders := r16OrderRows().AddRow("G7581", "郑华坤", "S26001802", 122.00, `{"票面价":{"standard":"119.00"}}`)
	inv := r16InvRows().AddRow("D-train", "G7581", "李四", 119.00)
	h, done := mkR16Handler(t, orders, inv)
	defer done()
	rej, warn := h.ruleCorpPaidDuplicate(r16Raw(), "F16")
	if len(rej) != 0 || len(warn) != 0 {
		t.Errorf("乘车人不同不应触发: rej=%v warn=%v", rej, warn)
	}
}

func TestRule16OneOrderConsumedOnce(t *testing.T) {
	// 1张企业支付订单只够对1张发票; 第2张同车次发票没订单可对 → 只报1条
	orders := r16OrderRows().AddRow("G7581", "郑华坤", "S26001802", 122.00, `{"票面价":{"standard":"119.00"}}`)
	inv := r16InvRows().
		AddRow("D-train", "G7581", "郑华坤", 119.00).
		AddRow("D-train", "G7581", "郑华坤", 119.00)
	h, done := mkR16Handler(t, orders, inv)
	defer done()
	rej, warn := h.ruleCorpPaidDuplicate(r16Raw(), "F16")
	if len(rej)+len(warn) != 1 {
		t.Errorf("1张订单只应对上1张发票, rej=%v warn=%v", rej, warn)
	}
}

func TestRule16NoLinksNoQueries(t *testing.T) {
	// 没关联申请单 → 直接返回, 不查库
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := &DashboardHandler{DB: db}
	raw := map[string]interface{}{"details": []interface{}{}}
	rej, warn := h.ruleCorpPaidDuplicate(raw, "F16")
	if len(rej) != 0 || len(warn) != 0 {
		t.Errorf("无关联申请单不应触发: rej=%v warn=%v", rej, warn)
	}
}
