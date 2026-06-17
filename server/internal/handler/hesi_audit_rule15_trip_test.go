package handler

// 规则 15-3 条4: 私车/过路消费日只有落在出差申请单期间内才扣交通补 (跑哥 2026-06-17)。
// 复用 hesi_audit_rule15_test.go 的 emptyInvRows / mkSubsidyDetail / mkDriveDetail / rawOf / r15* 常量。
// 集团经理: 交通补 70/天; 补贴 5 天, 不扣 cap=50×5+70×5=600, 扣 1 天 cap=50×5+70×4=530。

import (
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRule153DeductOnlyInsideTripRequisition(t *testing.T) {
	// 私车日 6/3 在补贴期内, 但出差申请单是 6/10~6/15 → 私车日落在出差期间外 → 不扣交通补 → ¥600 通过
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	// 查询顺序: 发票 → 出差申请单(tripDateSpans) → position
	mock.ExpectQuery(`SELECT IFNULL\(detail_id,''\), IFNULL\(invoice_type,''\)`).WillReturnRows(emptyInvRows())
	reqRaw := fmt.Sprintf(`{"u_出差起止日期":{"start":%d,"end":%d}}`, r15June1+9*r15Day, r15June1+14*r15Day)
	mock.ExpectQuery(`SELECT IFNULL\(raw_json,''\) FROM hesi_flow`).
		WillReturnRows(sqlmock.NewRows([]string{"raw_json"}).AddRow(reqRaw))
	mock.ExpectQuery(`SELECT IFNULL\(position,''\) FROM hesi_employee_contract_company`).
		WillReturnRows(sqlmock.NewRows([]string{"position"}).AddRow("集团经理"))

	h := &DashboardHandler{DB: db}
	raw := rawOf(
		mkSubsidyDetail(1, "600.00", "5", r15June1, r15June1+4*r15Day), // 补贴 6/1~6/5
		mkDriveDetail(2, "", r15June1+2*r15Day),                        // 私车日 6/3
	)
	raw["expenseLinks"] = []interface{}{"REQ1"}
	rej, warn := h.ruleOfflineExtras(raw, "F15", "S1")
	if len(rej) != 0 || len(warn) != 0 {
		t.Errorf("私车日落在出差申请单期间外不应扣交通补(条4): rej=%v warn=%v", rej, warn)
	}
}

func TestRule153DeductWhenInsideTripRequisition(t *testing.T) {
	// 对照: 出差申请单 6/1~6/5, 私车日 6/3 落在其内 → 扣交通补 → cap=530 → ¥600 超标驳回
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT IFNULL\(detail_id,''\), IFNULL\(invoice_type,''\)`).WillReturnRows(emptyInvRows())
	reqRaw := fmt.Sprintf(`{"u_出差起止日期":{"start":%d,"end":%d}}`, r15June1, r15June1+4*r15Day)
	mock.ExpectQuery(`SELECT IFNULL\(raw_json,''\) FROM hesi_flow`).
		WillReturnRows(sqlmock.NewRows([]string{"raw_json"}).AddRow(reqRaw))
	mock.ExpectQuery(`SELECT IFNULL\(position,''\) FROM hesi_employee_contract_company`).
		WillReturnRows(sqlmock.NewRows([]string{"position"}).AddRow("集团经理"))

	h := &DashboardHandler{DB: db}
	raw := rawOf(
		mkSubsidyDetail(1, "600.00", "5", r15June1, r15June1+4*r15Day),
		mkDriveDetail(2, "", r15June1+2*r15Day), // 6/3 在出差申请单内
	)
	raw["expenseLinks"] = []interface{}{"REQ1"}
	rej, _ := h.ruleOfflineExtras(raw, "F15", "S1")
	if len(rej) == 0 {
		t.Error("私车日落在出差申请单期间内应扣交通补→¥600超cap=530驳回(条4), got 无驳回")
	}
}
