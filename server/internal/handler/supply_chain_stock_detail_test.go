package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ---------- GetStockWarehouseDetail (原材料/包材 当前库存按仓库下钻) ----------

func TestGetStockWarehouseDetailHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// 口径同 matSQL: ys_stock 按 product_code, 排除安徽香松, 各仓库 SUM(currentqty)
	mock.ExpectQuery(`FROM ys_stock ys\s+WHERE ys\.product_code = \?`).
		WithArgs("01010055").
		WillReturnRows(sqlmock.NewRows([]string{"warehouse_name", "org_name", "qty"}).
			AddRow("润松-郎溪原料仓-公司仓", "杭州润松自然调味品有限公司", 597000.0).
			AddRow("润松-采购外仓-溧阳市天目湖调味品有限公司", "杭州润松自然调味品有限公司", 23929.0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/supply-chain/stock-detail?productCode=01010055", nil)
	(&DashboardHandler{DB: db}).GetStockWarehouseDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if resp["productCode"] != "01010055" {
		t.Errorf("productCode 应 01010055, got %v", resp["productCode"])
	}
	whs, _ := resp["warehouses"].([]interface{})
	if len(whs) != 2 {
		t.Fatalf("warehouses 应 2 条, got %d", len(whs))
	}
	// 合计应 = 两仓之和 (跟列里显示的"当前库存"对得上)
	if total, _ := resp["total"].(float64); total != 620929.0 {
		t.Errorf("total 应 620929, got %v", total)
	}
	// 第一条应是库存最多的润松郎溪仓
	first, _ := whs[0].(map[string]interface{})
	if first["warehouseName"] != "润松-郎溪原料仓-公司仓" {
		t.Errorf("第一条应是库存最多的郎溪仓, got %v", first["warehouseName"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestGetStockWarehouseDetailMissingCode(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/supply-chain/stock-detail", nil)
	(&DashboardHandler{DB: db}).GetStockWarehouseDetail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺 productCode 应 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}
