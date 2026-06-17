package handler

// 规则 10 无票判定 — 私车公用无发票豁免 (跑哥 2026-06-17)
// 复用 hesi_audit_fix20260612_test.go 的 invSelectPattern / invCols

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRule10PrivateCarNoInvoiceExempt(t *testing.T) {
	// 私车公用明细无发票 → 豁免不驳回 (走规则12-1行车记录核账, 本就无发票)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(invSelectPattern).WillReturnRows(sqlmock.NewRows(invCols())) // 无发票
	h := &DashboardHandler{DB: db}
	raw := map[string]interface{}{"details": []interface{}{
		map[string]interface{}{
			"feeTypeId": "ID01Fr2mX8KP2T", // 私车公用
			"feeTypeForm": map[string]interface{}{
				"detailId": "D1", "detailNo": float64(1),
				"amount": map[string]interface{}{"standard": "100"},
			},
		},
	}}
	rej, _ := h.ruleInvoiceChecks(raw, "", "F1")
	for _, r := range rej {
		if strings.Contains(r, "规则 10") {
			t.Errorf("私车公用无发票应豁免, 不应触发规则10驳回, got %q", r)
		}
	}
}

func TestRule10NonExemptNoInvoiceStillRejects(t *testing.T) {
	// 对照: 非豁免类型(交通费)无发票 → 仍按规则10驳回
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(invSelectPattern).WillReturnRows(sqlmock.NewRows(invCols()))
	h := &DashboardHandler{DB: db}
	raw := map[string]interface{}{"details": []interface{}{
		map[string]interface{}{
			"feeTypeId": "ID01Fk0STRw38z", // 交通费(非豁免)
			"feeTypeForm": map[string]interface{}{
				"detailId": "D1", "detailNo": float64(1),
				"amount": map[string]interface{}{"standard": "100"},
			},
		},
	}}
	rej, _ := h.ruleInvoiceChecks(raw, "", "F1")
	hit := false
	for _, r := range rej {
		if strings.Contains(r, "规则 10") {
			hit = true
		}
	}
	if !hit {
		t.Errorf("交通费无发票应按规则10驳回, got %v", rej)
	}
}
