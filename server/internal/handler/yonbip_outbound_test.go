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
		o1 + "|P1": {{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B1", AvailableQty: 5, StockStatusDoc: "s0"}},
		o2 + "|P1": {{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B9", AvailableQty: 100, StockStatusDoc: "s0"}},
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
		org + "|P1": {{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B1", AvailableQty: 10, StockStatusDoc: "s0"}},
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
		org + "|P1": {{WarehouseCode: "W1", WarehouseName: "仓A", ProductCode: "P1", Batchno: "B1", AvailableQty: 30, StockStatusDoc: "s0"}},
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
