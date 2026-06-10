package handler

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// 防重指纹必须: 同一笔(同字段)→同指纹(重发能命中跳过); 任一关键字段变→指纹变(不误挡新业务)。
func TestYbConvFingerprint_StableAndSensitive(t *testing.T) {
	base := ybConvItem{
		Type: "batch", OrgID: ybDefaultOrgID, WarehouseCode: "W1", ProductCode: "P1",
		Qty: "30", Batchno: "B1", ToBatch: "B2", StockStatusDoc: "2448706971278246078",
	}
	fp := ybConvFingerprint(base, "2026-06-10")

	// 同输入 → 同指纹 (这正是"用户重发整批"被跳过的前提)
	if fp != ybConvFingerprint(base, "2026-06-10") {
		t.Fatal("同一笔转换两次指纹应相同, 否则重发不会被防重命中")
	}

	// 关键字段任一变, 指纹必须变 (否则会误把不同业务当重复挡掉)
	mutators := map[string]func(ybConvItem) ybConvItem{
		"数量": func(it ybConvItem) ybConvItem { it.Qty = "31"; return it },
		"源批次": func(it ybConvItem) ybConvItem { it.Batchno = "BX"; return it },
		"目标批次": func(it ybConvItem) ybConvItem { it.ToBatch = "BY"; return it },
		"仓库": func(it ybConvItem) ybConvItem { it.WarehouseCode = "W2"; return it },
		"货品": func(it ybConvItem) ybConvItem { it.ProductCode = "P2"; return it },
		"生产日期": func(it ybConvItem) ybConvItem { it.Producedate = "2026-01-01"; return it },
		"到期日期": func(it ybConvItem) ybConvItem { it.Invaliddate = "2027-01-01"; return it },
		"sku": func(it ybConvItem) ybConvItem { it.ProductskuID = "SKU9"; return it },
		"目标状态": func(it ybConvItem) ybConvItem { it.ToStatusDoc = "2448706971278246081"; return it },
	}
	for name, mut := range mutators {
		if ybConvFingerprint(mut(base), "2026-06-10") == fp {
			t.Fatalf("改了[%s]指纹却没变, 会误挡不同业务", name)
		}
	}
	// 单据日期变, 指纹也要变
	if ybConvFingerprint(base, "2026-06-11") == fp {
		t.Fatal("单据日期变指纹应变")
	}
}

// 回归核心: 同一笔在防重窗口内已提交过 → ybExecuteConvert 必须跳过, 且【绝不调用用友】。
// 这里 h.YS=nil, 若没跳过走到 MorphologyConversionSave 会 panic; 不 panic + Skipped=true 即证明防重生效。
func TestYbExecuteConvert_SkipsDuplicateWithoutCallingYS(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	// ① ensureSubmitLog 建表
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS yonbip_submit_log").
		WillReturnResult(sqlmock.NewResult(0, 0))
	// ② 防重查询命中 (返回已有单号) → 视为重复
	mock.ExpectQuery("yonbip_submit_log").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"doc_code"}).AddRow("OUT-EXIST-001"))

	h := &DashboardHandler{DB: db} // YS 故意留空: 一旦真去建单就会 panic
	items := []ybConvItem{{
		Type: "batch", OrgID: ybDefaultOrgID, WarehouseCode: "W1", ProductCode: "P1",
		Qty: "30", Batchno: "B1", ToBatch: "B2", StockStatusDoc: "2448706971278246078",
	}}

	results := h.ybExecuteConvert(context.Background(), "2026-06-10", items)
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if !results[0].Skipped {
		t.Fatalf("重复提交应被跳过(Skipped=true), 实际: %+v", results[0])
	}
	if results[0].DocCode != "OUT-EXIST-001" {
		t.Fatalf("跳过时应回显已有单号, 实际: %q", results[0].DocCode)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql 预期未满足: %v", err)
	}
}

// 客户端断开(ctx 取消) → 立即停手, 不调用用友。同样靠 YS=nil 不 panic 来证明。
func TestYbExecuteConvert_StopsOnCanceledContext(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS yonbip_submit_log").
		WillReturnResult(sqlmock.NewResult(0, 0))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 模拟客户端已断开

	h := &DashboardHandler{DB: db}
	items := []ybConvItem{{
		Type: "batch", OrgID: ybDefaultOrgID, WarehouseCode: "W1", ProductCode: "P1",
		Qty: "30", Batchno: "B1", ToBatch: "B2", StockStatusDoc: "2448706971278246078",
	}}

	results := h.ybExecuteConvert(ctx, "2026-06-10", items)
	if len(results) != 1 || results[0].Error == "" {
		t.Fatalf("ctx 取消后应标注中断且不建单, 实际: %+v", results)
	}
	if results[0].Skipped {
		t.Fatal("ctx 中断不是防重跳过, 不应置 Skipped")
	}
}
