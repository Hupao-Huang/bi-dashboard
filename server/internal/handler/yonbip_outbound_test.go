package handler

import (
	"testing"

	"bi-dashboard/internal/yonsuite"
)

func ybFakeStock(data map[string][]yonsuite.StockRow) func(string, string) []yonsuite.StockRow {
	return func(oid, productCode string) []yonsuite.StockRow {
		return data[oid+"|"+productCode]
	}
}

// 直出: 目标批次有现货且够 → 1 张直出 shipment, 不转换。
func TestYbPlanExport_DirectOut(t *testing.T) {
	org := ybDefaultOrgID
	stock := map[string][]yonsuite.StockRow{
		org + "|P1": {{
			WarehouseCode: "W1", WarehouseName: "南京委外成品仓-公司仓-委外",
			ProductCode: "P1", ProductName: "货品1", Batchno: "B1",
			AvailableQty: 100, StockStatusDoc: "2448706971278246078", UnitID: "U1", StockUnitID: "U1",
		}},
	}
	rows := []ybRow{{ProductCode: "P1", Qty: "30", TargetBatch: "B1", WarehouseName: "南京委外成品仓-公司仓-委外"}}
	plans := ybPlanExport(ybFakeStock(stock), rows)
	if len(plans) != 1 {
		t.Fatalf("want 1 plan, got %d", len(plans))
	}
	p := plans[0]
	if p.NeededQty != 30 || p.FulfilledQty != 30 || p.RemainingQty != 0 {
		t.Fatalf("qty mismatch: need=%d ful=%d rem=%d", p.NeededQty, p.FulfilledQty, p.RemainingQty)
	}
	if len(p.Shipments) != 1 {
		t.Fatalf("want 1 shipment, got %d", len(p.Shipments))
	}
	sh := p.Shipments[0]
	if sh.Qty != 30 || sh.ConvertQty != 0 || sh.OutBatch != "B1" || len(sh.ConvertSources) != 0 {
		t.Fatalf("direct shipment wrong: %+v", sh)
	}
}

// 批次转换: 目标批次无现货, 别的批次有 → 1 张转换 shipment (B1→B2)。
func TestYbPlanExport_BatchConversion(t *testing.T) {
	org := ybDefaultOrgID
	stock := map[string][]yonsuite.StockRow{
		org + "|P1": {{
			WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1",
			Batchno: "B1", AvailableQty: 50, StockStatusDoc: "2448706971278246078",
		}},
	}
	rows := []ybRow{{ProductCode: "P1", Qty: "20", TargetBatch: "B2", WarehouseName: "仓A"}}
	plans := ybPlanExport(ybFakeStock(stock), rows)
	sh := plans[0].Shipments
	if len(sh) != 1 {
		t.Fatalf("want 1 shipment got %d", len(sh))
	}
	if sh[0].ConvertQty != 20 || sh[0].OutBatch != "B2" ||
		len(sh[0].ConvertSources) != 1 || sh[0].ConvertSources[0].FromBatch != "B1" {
		t.Fatalf("conversion shipment wrong: %+v", sh[0])
	}
}

// 跨组织: org1 只有 5, org2 有 100, 要 30 → 5(org1) + 25(org2)。
func TestYbPlanExport_CrossOrg(t *testing.T) {
	o1 := ybOrgPriority[0].ID
	o2 := ybOrgPriority[1].ID
	stock := map[string][]yonsuite.StockRow{
		o1 + "|P1": {{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B1", AvailableQty: 5, StockStatusDoc: "2448706971278246078"}},
		o2 + "|P1": {{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B9", AvailableQty: 100, StockStatusDoc: "2448706971278246078"}},
	}
	rows := []ybRow{{ProductCode: "P1", Qty: "30", WarehouseName: "仓A"}}
	plans := ybPlanExport(ybFakeStock(stock), rows)
	p := plans[0]
	if p.FulfilledQty != 30 || p.RemainingQty != 0 {
		t.Fatalf("cross-org fill wrong: ful=%d rem=%d", p.FulfilledQty, p.RemainingQty)
	}
	if len(p.Shipments) != 2 {
		t.Fatalf("want 2 shipments got %d", len(p.Shipments))
	}
	if p.Shipments[0].Qty != 5 || p.Shipments[0].OrgID != o1 {
		t.Fatalf("ship0 wrong: %+v", p.Shipments[0])
	}
	if p.Shipments[1].Qty != 25 || p.Shipments[1].OrgID != o2 {
		t.Fatalf("ship1 wrong: %+v", p.Shipments[1])
	}
}

// 缺口: 要 30 只有 10 → fulfilled 10, remaining 20。
func TestYbPlanExport_Shortfall(t *testing.T) {
	org := ybDefaultOrgID
	stock := map[string][]yonsuite.StockRow{
		org + "|P1": {{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B1", AvailableQty: 10, StockStatusDoc: "2448706971278246078"}},
	}
	rows := []ybRow{{ProductCode: "P1", Qty: "30", WarehouseName: "仓A"}}
	plans := ybPlanExport(ybFakeStock(stock), rows)
	if plans[0].FulfilledQty != 10 || plans[0].RemainingQty != 20 {
		t.Fatalf("shortfall wrong: ful=%d rem=%d", plans[0].FulfilledQty, plans[0].RemainingQty)
	}
}

// 库存池跨行共享: 同货品同仓同批次 pool=30, 两行各要 20 → 第二行只能凑 10。
func TestYbPlanExport_PoolSharedAcrossRows(t *testing.T) {
	org := ybDefaultOrgID
	stock := map[string][]yonsuite.StockRow{
		org + "|P1": {{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B1", AvailableQty: 30, StockStatusDoc: "2448706971278246078"}},
	}
	rows := []ybRow{
		{ProductCode: "P1", Qty: "20", TargetBatch: "B1", WarehouseName: "仓A"},
		{ProductCode: "P1", Qty: "20", TargetBatch: "B1", WarehouseName: "仓A"},
	}
	plans := ybPlanExport(ybFakeStock(stock), rows)
	if plans[0].FulfilledQty != 20 {
		t.Fatalf("row1 want 20 got %d", plans[0].FulfilledQty)
	}
	if plans[1].FulfilledQty != 10 || plans[1].RemainingQty != 10 {
		t.Fatalf("row2 pool-shared wrong: ful=%d rem=%d", plans[1].FulfilledQty, plans[1].RemainingQty)
	}
}

// 状态优先: 合格(优先级0) 应排在 不合格(1) 前面先出。
func TestYbPlanExport_StatusPriority(t *testing.T) {
	org := ybDefaultOrgID
	stock := map[string][]yonsuite.StockRow{
		org + "|P1": {
			{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "BAD", AvailableQty: 100, StockStatusDoc: "2448706971278246081"}, // 不合格
			{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "OK", AvailableQty: 100, StockStatusDoc: "2448706971278246078"},  // 合格
		},
	}
	rows := []ybRow{{ProductCode: "P1", Qty: "10", WarehouseName: "仓A"}}
	plans := ybPlanExport(ybFakeStock(stock), rows)
	if len(plans[0].Shipments) == 0 || plans[0].Shipments[0].StockStatusDoc != "2448706971278246078" {
		t.Fatalf("status priority wrong, first ship: %+v", plans[0].Shipments)
	}
}

const (
	ybTestQualified = "2448706971278246078" // 合格
	ybTestDefect    = "2448706971278246081" // 不合格
	ybTestScrap     = "2448706971278246082" // 废品
)

// 不合格货就在目标批次: 不能直出, 要状态转换成合格(出库一律合格)。
func TestYbPlanExport_StatusConvert_DefectInTargetBatch(t *testing.T) {
	org := ybDefaultOrgID
	stock := map[string][]yonsuite.StockRow{
		org + "|P1": {{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B1", AvailableQty: 50, StockStatusDoc: ybTestDefect}},
	}
	rows := []ybRow{{ProductCode: "P1", Qty: "20", TargetBatch: "B1", WarehouseName: "仓A"}}
	sh := ybPlanExport(ybFakeStock(stock), rows)[0].Shipments
	if len(sh) != 1 || sh[0].ConvertQty != 20 || len(sh[0].ConvertSources) != 1 {
		t.Fatalf("不合格目标批次应走状态转换(非直出): %+v", sh)
	}
	if sh[0].StockStatusDoc != ybQualifiedStatusDoc {
		t.Fatalf("出库状态应为合格, got %s", sh[0].StockStatusDoc)
	}
	if sh[0].ConvertSources[0].StockStatusDoc != ybTestDefect {
		t.Fatalf("转换来源状态应为不合格, got %s", sh[0].ConvertSources[0].StockStatusDoc)
	}
}

// 合格优先: 合格够时直出, 不合格库存不动。
func TestYbPlanExport_QualifiedFirst_DefectUntouched(t *testing.T) {
	org := ybDefaultOrgID
	stock := map[string][]yonsuite.StockRow{
		org + "|P1": {
			{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B1", AvailableQty: 100, StockStatusDoc: ybTestQualified},
			{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B1", AvailableQty: 100, StockStatusDoc: ybTestDefect},
		},
	}
	rows := []ybRow{{ProductCode: "P1", Qty: "30", TargetBatch: "B1", WarehouseName: "仓A"}}
	sh := ybPlanExport(ybFakeStock(stock), rows)[0].Shipments
	if len(sh) != 1 || sh[0].Qty != 30 || len(sh[0].ConvertSources) != 0 {
		t.Fatalf("合格够时应直出30不转, got %+v", sh)
	}
}

// 合格不够: 先直出合格, 再状态转换不合格补齐。
func TestYbPlanExport_QualifiedThenDefect(t *testing.T) {
	org := ybDefaultOrgID
	stock := map[string][]yonsuite.StockRow{
		org + "|P1": {
			{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B1", AvailableQty: 10, StockStatusDoc: ybTestQualified},
			{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B1", AvailableQty: 100, StockStatusDoc: ybTestDefect},
		},
	}
	rows := []ybRow{{ProductCode: "P1", Qty: "30", TargetBatch: "B1", WarehouseName: "仓A"}}
	p := ybPlanExport(ybFakeStock(stock), rows)[0]
	if p.RemainingQty != 0 || len(p.Shipments) != 2 {
		t.Fatalf("应凑齐(合格10+不合格20), got rem=%d ships=%+v", p.RemainingQty, p.Shipments)
	}
	if p.Shipments[0].Qty != 10 || len(p.Shipments[0].ConvertSources) != 0 {
		t.Fatalf("第一张应直出合格10: %+v", p.Shipments[0])
	}
	if p.Shipments[1].Qty != 20 || p.Shipments[1].ConvertSources[0].StockStatusDoc != ybTestDefect {
		t.Fatalf("第二张应状态转换不合格20: %+v", p.Shipments[1])
	}
}

// 档内不合格优先于废品: 两者都够时全用不合格。
func TestYbPlanExport_DefectBeforeScrap(t *testing.T) {
	org := ybDefaultOrgID
	stock := map[string][]yonsuite.StockRow{
		org + "|P1": {
			{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B1", AvailableQty: 100, StockStatusDoc: ybTestScrap},
			{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B1", AvailableQty: 100, StockStatusDoc: ybTestDefect},
		},
	}
	rows := []ybRow{{ProductCode: "P1", Qty: "30", TargetBatch: "B1", WarehouseName: "仓A"}}
	sh := ybPlanExport(ybFakeStock(stock), rows)[0].Shipments
	if len(sh) != 1 || sh[0].ConvertSources[0].StockStatusDoc != ybTestDefect {
		t.Fatalf("应优先用不合格(非废品), got %+v", sh)
	}
}

// 未知状态(非合格/不合格/废品)不消费、不自动转 —— 业务含义不明的库存(冻结/在途/质押)留着算缺口。
func TestYbPlanExport_UnknownStatus_NotConsumed(t *testing.T) {
	org := ybDefaultOrgID
	stock := map[string][]yonsuite.StockRow{
		org + "|P1": {{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B1", AvailableQty: 100, StockStatusDoc: "9999999999999999999"}}, // 未知状态
	}
	rows := []ybRow{{ProductCode: "P1", Qty: "30", TargetBatch: "B1", WarehouseName: "仓A"}}
	p := ybPlanExport(ybFakeStock(stock), rows)[0]
	if len(p.Shipments) != 0 || p.RemainingQty != 30 {
		t.Fatalf("未知状态不该被消费/转换, 应全缺口30: ships=%+v rem=%d", p.Shipments, p.RemainingQty)
	}
}

// 单据级缺货: 同一单号一行齐、一行缺 → 整号两行都标 BillShort(整单不出)。
func TestYbPlanExport_BillShort_WholeBillNotOut(t *testing.T) {
	org := ybDefaultOrgID
	stock := map[string][]yonsuite.StockRow{
		org + "|P1": {{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B1", AvailableQty: 100, StockStatusDoc: ybTestQualified}},
		org + "|P2": {{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P2", Batchno: "B1", AvailableQty: 5, StockStatusDoc: ybTestQualified}}, // 不够
	}
	rows := []ybRow{
		{ProductCode: "P1", Qty: "30", TargetBatch: "B1", WarehouseName: "仓A", BillNo: "CRK001"}, // 齐
		{ProductCode: "P2", Qty: "30", TargetBatch: "B1", WarehouseName: "仓A", BillNo: "CRK001"}, // 缺25
	}
	plans := ybPlanExport(ybFakeStock(stock), rows)
	if !plans[0].BillShort || !plans[1].BillShort {
		t.Fatalf("同单号一行缺→整号两行都该 BillShort: p0=%v p1=%v", plans[0].BillShort, plans[1].BillShort)
	}
}

// 不同单号互不牵连: A 单齐全不受 B 单缺货影响。
func TestYbPlanExport_BillShort_OtherBillUnaffected(t *testing.T) {
	org := ybDefaultOrgID
	stock := map[string][]yonsuite.StockRow{
		org + "|P1": {{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B1", AvailableQty: 100, StockStatusDoc: ybTestQualified}},
		org + "|P2": {{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P2", Batchno: "B1", AvailableQty: 5, StockStatusDoc: ybTestQualified}},
	}
	rows := []ybRow{
		{ProductCode: "P1", Qty: "30", TargetBatch: "B1", WarehouseName: "仓A", BillNo: "CRK_A"}, // 齐
		{ProductCode: "P2", Qty: "30", TargetBatch: "B1", WarehouseName: "仓A", BillNo: "CRK_B"}, // 缺
	}
	plans := ybPlanExport(ybFakeStock(stock), rows)
	if plans[0].BillShort {
		t.Fatalf("CRK_A 齐全不该被 CRK_B 牵连: %+v", plans[0])
	}
	if !plans[1].BillShort {
		t.Fatalf("CRK_B 缺货应 BillShort")
	}
}

// BillShort 单的转换不算"待转换"(否则永远挡住②出库)。
func TestYbPlanHasUnconverted_ExcludesBillShort(t *testing.T) {
	shortWithConv := []ybPlan{{
		BillShort: true,
		Shipments: []ybShipment{{Qty: 5, ConvertSources: []ybConvertSource{{FromBatch: "B1", Qty: 5}}}},
	}}
	if ybPlanHasUnconverted(shortWithConv) {
		t.Fatal("缺货单的转换不该算待转换(它整单不出/不转)")
	}
}

// phase=out 防呆: 计划里还有 ConvertSources(未转) 就该被挡。
func TestYbPlanHasUnconverted(t *testing.T) {
	noConv := []ybPlan{{Shipments: []ybShipment{{Qty: 5, ConvertSources: []ybConvertSource{}}}}}
	if ybPlanHasUnconverted(noConv) {
		t.Fatal("无转换的计划不该被判为未转")
	}
	withConv := []ybPlan{{Shipments: []ybShipment{
		{Qty: 5, ConvertSources: []ybConvertSource{}},
		{Qty: 5, ConvertSources: []ybConvertSource{{FromBatch: "B1", Qty: 5}}},
	}}}
	if !ybPlanHasUnconverted(withConv) {
		t.Fatal("有转换的计划应被判为未转, 挡住直接出库")
	}
}

// 形态转换单拆分(用友不允许一张单批次+状态同时改):
func ybDetail(doc map[string]interface{}) (before, after map[string]interface{}) {
	d := doc["data"].(map[string]interface{})["morphologyconversiondetail"].([]map[string]interface{})
	return d[0], d[1]
}

// 不合格 B1 → 合格 B2: 批次+状态都变 → 拆 2 张(先状态B1不合格→B1合格, 再批次B1合格→B2合格)。
func TestYbBuildConversionDocs_StatusAndBatch_SplitsTwo(t *testing.T) {
	sh := ybShipment{OrgID: ybDefaultOrgID, WarehouseCode: "W1", ProductCode: "P1", StockStatusDoc: ybQualifiedStatusDoc}
	cs := ybConvertSource{FromBatch: "B1", Qty: 20, StockStatusDoc: ybTestDefect}
	docs := ybBuildConversionDocs(sh, "2026-06-10", cs, "B2")
	if len(docs) != 2 {
		t.Fatalf("批次+状态都变应拆 2 张, got %d", len(docs))
	}
	// ①状态转换: B1不合格 → B1合格 (批次不变)
	b1, a1 := ybDetail(docs[0])
	if b1["batchno"] != "B1" || a1["batchno"] != "B1" || b1["stockStatusDoc"] != ybTestDefect || a1["stockStatusDoc"] != ybQualifiedStatusDoc {
		t.Fatalf("①应状态转换 B1不合格→B1合格: before=%v/%v after=%v/%v", b1["batchno"], b1["stockStatusDoc"], a1["batchno"], a1["stockStatusDoc"])
	}
	// ②批次转换: B1合格 → B2合格 (状态不变)
	b2, a2 := ybDetail(docs[1])
	if b2["batchno"] != "B1" || a2["batchno"] != "B2" || b2["stockStatusDoc"] != ybQualifiedStatusDoc || a2["stockStatusDoc"] != ybQualifiedStatusDoc {
		t.Fatalf("②应批次转换 B1合格→B2合格: before=%v/%v after=%v/%v", b2["batchno"], b2["stockStatusDoc"], a2["batchno"], a2["stockStatusDoc"])
	}
}

// 只改一样 → 1 张。合格 B1→B2(纯批次); 不合格 B1→B1(纯状态, 目标批次空)。
func TestYbBuildConversionDocs_SingleChange_OneDoc(t *testing.T) {
	sh := ybShipment{OrgID: ybDefaultOrgID, WarehouseCode: "W1", ProductCode: "P1", StockStatusDoc: ybQualifiedStatusDoc}
	// 纯批次转换: 来源合格
	if d := ybBuildConversionDocs(sh, "2026-06-10", ybConvertSource{FromBatch: "B1", Qty: 5, StockStatusDoc: ybTestQualified}, "B2"); len(d) != 1 {
		t.Fatalf("纯批次转换应 1 张, got %d", len(d))
	}
	// 纯状态转换: 目标批次空 → 批次不变
	d := ybBuildConversionDocs(sh, "2026-06-10", ybConvertSource{FromBatch: "B1", Qty: 5, StockStatusDoc: ybTestDefect}, "")
	if len(d) != 1 {
		t.Fatalf("纯状态转换应 1 张, got %d", len(d))
	}
	b, a := ybDetail(d[0])
	if b["batchno"] != "B1" || a["batchno"] != "B1" || a["stockStatusDoc"] != ybQualifiedStatusDoc {
		t.Fatalf("纯状态转换应 B1不合格→B1合格: %v/%v → %v/%v", b["batchno"], b["stockStatusDoc"], a["batchno"], a["stockStatusDoc"])
	}
}

// 进度计数: 拆 2 张的源算 2, 拆 1 张的算 1。
func TestYbConvDocCount(t *testing.T) {
	sh := ybShipment{StockStatusDoc: ybQualifiedStatusDoc}
	if n := ybConvDocCount(sh, ybConvertSource{FromBatch: "B1", StockStatusDoc: ybTestDefect}, "B2"); n != 2 {
		t.Fatalf("不合格+换批次应 2, got %d", n)
	}
	if n := ybConvDocCount(sh, ybConvertSource{FromBatch: "B1", StockStatusDoc: ybTestQualified}, "B2"); n != 1 {
		t.Fatalf("合格换批次应 1, got %d", n)
	}
	if n := ybConvDocCount(sh, ybConvertSource{FromBatch: "B1", StockStatusDoc: ybTestDefect}, "B1"); n != 1 {
		t.Fatalf("不合格同批次(只改状态)应 1, got %d", n)
	}
}

func TestYbWhMatch(t *testing.T) {
	if !ybWhMatch("南京委外成品仓-公司仓-委外", "南京委外成品仓-公司仓-委外") {
		t.Fatal("exact match failed")
	}
	if !ybWhMatch("南京委外成品仓-公司仓-委外", "南京委外成品仓") {
		t.Fatal("normalized match failed")
	}
	if ybWhMatch("成品仓", "南京委外成品仓") {
		t.Fatal("substring should NOT match")
	}
}

func TestYbFmtDate(t *testing.T) {
	cases := map[string]string{
		"20260531":   "2026-05-31",
		"2026-05-31": "2026-05-31",
		"2026/05/31": "2026-05-31",
	}
	for in, want := range cases {
		if got := ybFmtDate(in); got != want {
			t.Fatalf("ybFmtDate(%q)=%q want %q", in, got, want)
		}
	}
}

func TestYbNormalizeCategory(t *testing.T) {
	if ybNormalizeCategory("调拨出库") != "27" {
		t.Fatal("调拨出库 should map to 27")
	}
	if ybNormalizeCategory("29") != "29" {
		t.Fatal("digit code should pass through")
	}
	if ybNormalizeCategory("不存在的类别") != "" {
		t.Fatal("unknown should be empty")
	}
}
