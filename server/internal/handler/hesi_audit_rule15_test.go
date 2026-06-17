package handler

// 规则 15 线下(世创/世用)专属规则集单测 (樊雪娇 2026-06-11 口径)
// 重点盖二审修的三处: 私车日与补贴期间求交集 / feeDatePeriod 脏数据封顶 / 旧行为回退

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const r15Day = int64(86400000)

// 2026-06-01 00:00 UTC (本地 +08 为 08:00, 不踩日界)
const r15June1 = int64(1780272000000)

const r15SubsidyFeeType = "ID01Fk0MQBAAQ7"

// mkOfflineHandler 造 handler + sqlmock, 并按 ruleOfflineExtras 的查询顺序挂期望:
// ① hesi_flow_invoice ② hesi_employee_contract_company.position
func mkOfflineHandler(t *testing.T, invRows *sqlmock.Rows, position string) (*DashboardHandler, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	mock.ExpectQuery(`SELECT IFNULL\(detail_id,''\), IFNULL\(invoice_type,''\)`).WillReturnRows(invRows)
	mock.ExpectQuery(`SELECT IFNULL\(position,''\) FROM hesi_employee_contract_company`).
		WillReturnRows(sqlmock.NewRows([]string{"position"}).AddRow(position))
	return &DashboardHandler{DB: db}, func() { db.Close() }
}

func emptyInvRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"detail_id", "invoice_type", "total_amount", "approve_amount", "toll_pass_start"})
}

// mkSubsidyDetail 补贴明细: 金额/天数 + 可选 feeDatePeriod (startMs=0 表示不带日期)
func mkSubsidyDetail(no int, amt string, days string, startMs, endMs int64) map[string]interface{} {
	form := map[string]interface{}{
		"detailId": "D-sub-" + amt,
		"detailNo": float64(no),
		"u_天数":     days,
		"u_出差补贴金额": map[string]interface{}{"standard": amt},
		"amount":   map[string]interface{}{"standard": amt},
	}
	if startMs > 0 {
		form["feeDatePeriod"] = map[string]interface{}{"start": float64(startMs), "end": float64(endMs)}
	}
	return map[string]interface{}{"feeTypeId": r15SubsidyFeeType, "feeTypeForm": form}
}

// mkDriveDetail 私车公用明细: 单天 feeDate
func mkDriveDetail(no int, amt string, feeDateMs int64) map[string]interface{} {
	form := map[string]interface{}{
		"detailId": "D-drive",
		"detailNo": float64(no),
		"amount":   map[string]interface{}{"standard": amt},
	}
	if feeDateMs > 0 {
		form["feeDate"] = float64(feeDateMs)
	}
	return map[string]interface{}{"feeTypeId": driveFeeTypeID, "feeTypeForm": form}
}

func rawOf(details ...map[string]interface{}) map[string]interface{} {
	arr := make([]interface{}, 0, len(details))
	for _, d := range details {
		arr = append(arr, d)
	}
	return map[string]interface{}{"details": arr}
}

// ===== 15-3 私车日 × 补贴期间 交集 (二审修) =====

func TestRule153DriveOutsideSubsidyPeriodNotPenalized(t *testing.T) {
	// 补贴 6/1-6/5 ¥600 (集团经理 50+70=120/天 cap=600), 私车日 6/20 在期间外 → 不扣交通补, 不驳回
	h, done := mkOfflineHandler(t, emptyInvRows(), "集团经理")
	defer done()
	raw := rawOf(
		mkSubsidyDetail(1, "600.00", "5", r15June1, r15June1+4*r15Day),
		mkDriveDetail(2, "", r15June1+19*r15Day), // 6/20
	)
	rej, warn := h.ruleOfflineExtras(raw, "F15", "S1")
	if len(rej) != 0 || len(warn) != 0 {
		t.Errorf("期间外私车日不应扣交通补: rej=%v warn=%v", rej, warn)
	}
}

func TestRule153DriveInsideSubsidyPeriodRejects(t *testing.T) {
	// 补贴 6/1-6/5 ¥600, 私车日 6/3 在期间内 → cap=50×5+70×4=530 → ¥600 驳回
	h, done := mkOfflineHandler(t, emptyInvRows(), "集团经理")
	defer done()
	raw := rawOf(
		mkSubsidyDetail(1, "600.00", "5", r15June1, r15June1+4*r15Day),
		mkDriveDetail(2, "", r15June1+2*r15Day), // 6/3
	)
	rej, _ := h.ruleOfflineExtras(raw, "F15", "S1")
	got := strings.Join(rej, "; ")
	if !strings.Contains(got, "规则 15-3") {
		t.Errorf("期间内私车日应触发 15-3 驳回, got %q", got)
	}
}

func TestRule153NoSubsidyDatesFallbackGlobalDriveDays(t *testing.T) {
	// 补贴明细没填日期 → 退回全单私车日数 (旧行为): cap=530 → ¥600 驳回
	h, done := mkOfflineHandler(t, emptyInvRows(), "集团经理")
	defer done()
	raw := rawOf(
		mkSubsidyDetail(1, "600.00", "5", 0, 0),
		mkDriveDetail(2, "", r15June1+19*r15Day),
	)
	rej, _ := h.ruleOfflineExtras(raw, "F15", "S1")
	if !strings.Contains(strings.Join(rej, "; "), "规则 15-3") {
		t.Errorf("无日期补贴应退回旧行为(全单私车日扣减), got %v", rej)
	}
}

func TestRule153UnknownPositionWarnsNotRejects(t *testing.T) {
	// 花名册查不到职级 → 按其他员工档(50交通)算, 超标转人工核不驳回
	h, done := mkOfflineHandler(t, emptyInvRows(), "")
	defer done()
	raw := rawOf(mkSubsidyDetail(1, "700.00", "5", 0, 0)) // cap=50×5+50×5=500
	rej, warn := h.ruleOfflineExtras(raw, "F15", "S1")
	if len(rej) != 0 {
		t.Errorf("职级未知不应直接驳回, got %v", rej)
	}
	if !strings.Contains(strings.Join(warn, "; "), "人工核") {
		t.Errorf("职级未知超标应转人工核, got %v", warn)
	}
}

// ===== feeDatePeriod 脏数据封顶 (二审修) =====

func TestRule15DirtyFeeDatePeriodCapped(t *testing.T) {
	// 私车明细的 end 是脏的远期时间戳 (约 +1000 年) → 区间被丢弃, 不跑飞;
	// 补贴无日期 → driveDays=0 → cap=600 → ¥600 通过
	h, done := mkOfflineHandler(t, emptyInvRows(), "集团经理")
	defer done()
	drive := mkDriveDetail(2, "", 0)
	drive["feeTypeForm"].(map[string]interface{})["feeDatePeriod"] = map[string]interface{}{
		"start": float64(r15June1), "end": float64(r15June1 + 365000*r15Day),
	}
	raw := rawOf(mkSubsidyDetail(1, "600.00", "5", 0, 0), drive)
	rej, warn := h.ruleOfflineExtras(raw, "F15", "S1")
	if len(rej) != 0 || len(warn) != 0 {
		t.Errorf("脏 feeDatePeriod 应整段丢弃且不影响判定: rej=%v warn=%v", rej, warn)
	}
}

// ===== 15-2 一分不差 + 15-1.2 付款截图 + 15-4 油费发票 =====

func TestRule152AmountMustMatchInvoice(t *testing.T) {
	// 非豁免明细 ¥100.00 vs 发票合计 ¥100.01 → 驳回; 有付款截图避开 15-1.2 干扰
	inv := emptyInvRows().AddRow("D-x", "DIGITAL_NORMAL", 100.01, 0, 0)
	h, done := mkOfflineHandler(t, inv, "集团经理")
	defer done()
	raw := rawOf(map[string]interface{}{
		"feeTypeId": "ID01OTHER",
		"feeTypeForm": map[string]interface{}{
			"detailId": "D-x", "detailNo": float64(1),
			"amount": map[string]interface{}{"standard": "100.00"},
			"u_付款截图": "att://1",
		},
	})
	rej, _ := h.ruleOfflineExtras(raw, "F15", "S1")
	got := strings.Join(rej, "; ")
	if !strings.Contains(got, "规则 15-2") {
		t.Errorf("差一分应驳回 15-2, got %q", got)
	}
}

func TestRule1512NonSpecialInvoiceNeedsPayShot(t *testing.T) {
	// 非专票 + 没传付款截图 → 15-1.2 驳回; 金额一致避开 15-2
	inv := emptyInvRows().AddRow("D-x", "DIGITAL_NORMAL", 88.00, 0, 0)
	h, done := mkOfflineHandler(t, inv, "集团经理")
	defer done()
	raw := rawOf(map[string]interface{}{
		"feeTypeId": "ID01OTHER",
		"feeTypeForm": map[string]interface{}{
			"detailId": "D-x", "detailNo": float64(1),
			"amount": map[string]interface{}{"standard": "88.00"},
		},
	})
	rej, _ := h.ruleOfflineExtras(raw, "F15", "S1")
	got := strings.Join(rej, "; ")
	if !strings.Contains(got, "规则 15-1.2") {
		t.Errorf("非专票无付款截图应驳回 15-1.2, got %q", got)
	}
	if strings.Contains(got, "规则 15-2") {
		t.Errorf("金额一致不应触发 15-2, got %q", got)
	}
}

func TestRule1512SpecialInvoiceNoPayShotOK(t *testing.T) {
	// 全专票 → 不要求付款截图
	inv := emptyInvRows().AddRow("D-x", "PAPER_SPECIAL", 88.00, 0, 0)
	h, done := mkOfflineHandler(t, inv, "集团经理")
	defer done()
	raw := rawOf(map[string]interface{}{
		"feeTypeId": "ID01OTHER",
		"feeTypeForm": map[string]interface{}{
			"detailId": "D-x", "detailNo": float64(1),
			"amount": map[string]interface{}{"standard": "88.00"},
		},
	})
	rej, warn := h.ruleOfflineExtras(raw, "F15", "S1")
	if len(rej) != 0 || len(warn) != 0 {
		t.Errorf("全专票不应触发 15-1.2: rej=%v warn=%v", rej, warn)
	}
}

func TestRule154AggregateAcrossDetails(t *testing.T) {
	// 两条私车明细(100+100), 一张¥210大油票挂在第一条 → 整单聚合 210≥200 → 过
	// (6/12 修: 按明细判会误驳没挂发票的第二条; 樊雪娇口径是"总额"判)
	inv := emptyInvRows().AddRow("D-drive-a", "DIGITAL_NORMAL", 210.00, 0, 0)
	h, done := mkOfflineHandler(t, inv, "集团经理")
	defer done()
	mk := func(did string, no int) map[string]interface{} {
		return map[string]interface{}{
			"feeTypeId": driveFeeTypeID,
			"feeTypeForm": map[string]interface{}{
				"detailId": did, "detailNo": float64(no),
				"amount": map[string]interface{}{"standard": "100.00"},
				"u_付款截图": "att://1",
			},
		}
	}
	rej, _ := h.ruleOfflineExtras(rawOf(mk("D-drive-a", 1), mk("D-drive-b", 2)), "F15", "S1")
	for _, r := range rej {
		if strings.Contains(r, "规则 15-4") {
			t.Errorf("整单油票足额不应驳回 15-4, got %v", rej)
		}
	}
}

func TestRule154DriveFuelInvoiceMustCover(t *testing.T) {
	// 私车公用 ¥200, 油费发票合计 ¥150 → 驳回 15-4; 补足 ¥200 → 通过
	inv := emptyInvRows().AddRow("D-drive", "DIGITAL_NORMAL", 150.00, 0, 0)
	h, done := mkOfflineHandler(t, inv, "集团经理")
	defer done()
	raw := rawOf(mkDriveDetail(1, "200.00", r15June1))
	rej, _ := h.ruleOfflineExtras(raw, "F15", "S1")
	if !strings.Contains(strings.Join(rej, "; "), "规则 15-4") {
		t.Errorf("油费发票不足应驳回 15-4, got %v", rej)
	}

	inv2 := emptyInvRows().
		AddRow("D-drive", "DIGITAL_NORMAL", 150.00, 0, 0).
		AddRow("D-drive", "DIGITAL_NORMAL", 50.00, 0, 0)
	h2, done2 := mkOfflineHandler(t, inv2, "集团经理")
	defer done2()
	rej2, _ := h2.ruleOfflineExtras(rawOf(mkDriveDetail(1, "200.00", r15June1)), "F15", "S1")
	for _, r := range rej2 {
		if strings.Contains(r, "规则 15-4") {
			t.Errorf("油费发票足额不应驳回 15-4, got %v", rej2)
		}
	}
}
