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

// 两步拆分: phase 控制只转换/只出库。用 dedup 命中让两阶段都跳过真用友调用(YS=nil 不 panic),
// 借此证明 phase 闸门: convert 不碰出库, out 不碰转换。
func ybOnePlanWithConvert() []ybPlan {
	return []ybPlan{{
		Row: ybRow{ProductCode: "P1", Qty: "5", TargetBatch: "B2"},
		Shipments: []ybShipment{{
			OrgID: ybDefaultOrgID, WarehouseCode: "W1", ProductCode: "P1", Qty: 5,
			OutBatch: "B2", StockStatusDoc: "2448706971278246078",
			ConvertSources: []ybConvertSource{{FromBatch: "B1", Qty: 5}},
		}},
	}}
}

func TestYbExecute_PhaseConvertOnly_SkipsOutbound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS yonbip_submit_log").WillReturnResult(sqlmock.NewResult(0, 0))
	// Phase1 转换的防重查询命中 → 跳过, 不调用友
	mock.ExpectQuery("yonbip_submit_log").WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"doc_code"}).AddRow("CONV-EXIST"))

	h := &DashboardHandler{DB: db} // YS=nil: 真去建单就 panic
	out := h.ybExecute(context.Background(), "2026-06-10", ybOnePlanWithConvert(), true, "convert", nil)
	if len(out) != 1 {
		t.Fatalf("want 1 plan result, got %d", len(out))
	}
	subs := out[0]["shipments"].([]*ybShipLog)
	if len(subs) != 1 || len(subs[0].Conversions) != 1 || !subs[0].Conversions[0].Skipped {
		t.Fatalf("convert 阶段应有1条跳过的转换, got %+v", subs)
	}
	if subs[0].OutDocCode != "" || subs[0].OutSkipped {
		t.Fatalf("phase=convert 不该碰出库, 实际 out 有动作: %+v", subs[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql 预期未满足(可能多调了出库的防重查询): %v", err)
	}
}

func TestYbExecute_PhaseOutOnly_SkipsConvert(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS yonbip_submit_log").WillReturnResult(sqlmock.NewResult(0, 0))
	// 只应有 Phase2 出库的防重查询命中(Phase1 被 phase 闸门跳过, 不查转换防重)
	mock.ExpectQuery("yonbip_submit_log").WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"doc_code"}).AddRow("OUT-EXIST"))

	h := &DashboardHandler{DB: db}
	out := h.ybExecute(context.Background(), "2026-06-10", ybOnePlanWithConvert(), true, "out", nil)
	subs := out[0]["shipments"].([]*ybShipLog)
	if len(subs[0].Conversions) != 0 {
		t.Fatalf("phase=out 不该做批次转换, 实际有转换记录: %+v", subs[0].Conversions)
	}
	if !subs[0].OutSkipped || subs[0].OutDocCode != "OUT-EXIST" {
		t.Fatalf("phase=out 应执行出库(此处命中防重跳过), 实际: %+v", subs[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql 预期未满足(可能误做了转换的防重查询): %v", err)
	}
}

// 进度回调: convert 阶段每处理一笔转换回调一次 done/total(SSE 推前端的源头)。
func TestYbExecute_EmitsProgress(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS yonbip_submit_log").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("yonbip_submit_log").WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"doc_code"}).AddRow("CONV-EXIST"))

	h := &DashboardHandler{DB: db}
	var n, gotDone, gotTotal int
	h.ybExecute(context.Background(), "2026-06-10", ybOnePlanWithConvert(), true, "convert",
		func(done, total int, _ string) { n++; gotDone = done; gotTotal = total })
	if n != 1 || gotDone != 1 || gotTotal != 1 {
		t.Fatalf("进度应回调1次 done=1 total=1(1笔转换), 实际 n=%d done=%d total=%d", n, gotDone, gotTotal)
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
