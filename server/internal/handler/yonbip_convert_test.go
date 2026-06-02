package handler

import "testing"

// 批次转换: before/after 只有 batchno 不同(A→B), 库存状态两行相同。
func TestYbBuildMorphConvBody_Batch(t *testing.T) {
	it := ybConvItem{
		Type: "batch", OrgID: ybDefaultOrgID, WarehouseCode: "W1", ProductCode: "P1",
		UnitID: "U1", StockUnitID: "U1", Qty: "30", Batchno: "B1", ToBatch: "B2",
		StockStatusDoc: "2448706971278246078",
	}
	body := ybBuildMorphConvBody(it, "2026-05-31")
	data := body["data"].(map[string]interface{})
	if data["conversionType"] != "1" || data["mcType"] != "1" || data["businesstypeId"] != "A70002" {
		t.Fatalf("header wrong: %+v", data)
	}
	if data["beforeWarehouse"] != "W1" || data["afterWarehouse"] != "W1" {
		t.Fatalf("warehouse wrong: %+v", data)
	}
	detail := data["morphologyconversiondetail"].([]map[string]interface{})
	if len(detail) != 2 {
		t.Fatalf("want 2 detail lines, got %d", len(detail))
	}
	before, after := detail[0], detail[1]
	if before["lineType"] != "1" || before["batchno"] != "B1" {
		t.Fatalf("before wrong: %+v", before)
	}
	if after["lineType"] != "2" || after["batchno"] != "B2" {
		t.Fatalf("after wrong: %+v", after)
	}
	// 批次转换库存状态不变
	if before["stockStatusDoc"] != "2448706971278246078" || after["stockStatusDoc"] != "2448706971278246078" {
		t.Fatalf("status should stay same: before=%v after=%v", before["stockStatusDoc"], after["stockStatusDoc"])
	}
}

// 状态转换: before/after 只有 stockStatusDoc 不同(A→B), 批次两行相同。
func TestYbBuildMorphConvBody_Status(t *testing.T) {
	it := ybConvItem{
		Type: "status", OrgID: ybDefaultOrgID, WarehouseCode: "W1", ProductCode: "P1",
		UnitID: "U1", Qty: "10", Batchno: "B1",
		StockStatusDoc: "2448706971278246078", ToStatusDoc: "2448706971278246081",
	}
	body := ybBuildMorphConvBody(it, "2026-05-31")
	data := body["data"].(map[string]interface{})
	detail := data["morphologyconversiondetail"].([]map[string]interface{})
	before, after := detail[0], detail[1]
	// 批次不变
	if before["batchno"] != "B1" || after["batchno"] != "B1" {
		t.Fatalf("batch should stay same: before=%v after=%v", before["batchno"], after["batchno"])
	}
	// 状态变
	if before["stockStatusDoc"] != "2448706971278246078" || after["stockStatusDoc"] != "2448706971278246081" {
		t.Fatalf("status conversion wrong: before=%v after=%v", before["stockStatusDoc"], after["stockStatusDoc"])
	}
	if before["lineType"] != "1" || after["lineType"] != "2" {
		t.Fatalf("lineType wrong")
	}
}

// 生产/到期日期带上时, before/after 都要带, 且补 00:00:00。
func TestYbBuildMorphConvBody_Dates(t *testing.T) {
	it := ybConvItem{
		Type: "batch", WarehouseCode: "W1", ProductCode: "P1", UnitID: "U1", Qty: "5",
		Batchno: "B1", ToBatch: "B2", Producedate: "2026-01-02", Invaliddate: "2027-01-02",
	}
	body := ybBuildMorphConvBody(it, "20260531")
	data := body["data"].(map[string]interface{})
	if data["vouchdate"] != "2026-05-31 00:00:00" {
		t.Fatalf("vouchdate fmt wrong: %v", data["vouchdate"])
	}
	detail := data["morphologyconversiondetail"].([]map[string]interface{})
	for _, line := range detail {
		if line["producedate"] != "2026-01-02 00:00:00" {
			t.Fatalf("producedate wrong: %v", line["producedate"])
		}
		if line["invaliddate"] != "2027-01-02 00:00:00" {
			t.Fatalf("invaliddate wrong: %v", line["invaliddate"])
		}
	}
}

func TestYbStatusName(t *testing.T) {
	if ybStatusName("2448706971278246078") != "合格" {
		t.Fatal("合格 doc name wrong")
	}
	if ybStatusName("2448706971278246081") != "不合格" {
		t.Fatal("不合格 doc name wrong")
	}
	if ybStatusName("xxx") != "xxx" {
		t.Fatal("unknown doc should pass through")
	}
}

func TestYbConvQtyNum(t *testing.T) {
	if ybConvQtyNum("30") != 30 || ybConvQtyNum(" 12.5 ") != 12.5 || ybConvQtyNum("abc") != 0 {
		t.Fatal("qty parse wrong")
	}
}
