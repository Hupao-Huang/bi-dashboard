package handler

import (
	"log"
	"net/http"
	"strings"
)

// GetStockWarehouseDetail 当前库存按仓库下钻 — 采购计划点击"当前库存"数字弹窗用。
//
// 两套口径(随 kind 切换, 各自严格对齐 GetPurchasePlan 同段, 各仓 SUM 相加 = 列里显示的"当前库存"):
//   - kind=finished: 成品/半成品 + 其他(广宣品)。源 stock_quantity 按 goods_no, 8 仓白名单(planWarehouses),
//     SUM(current_qty - locked_qty)。对齐 prodSQL/otherSQL。
//   - 其它/空(material): 原材料/包材。源 ys_stock 按 product_code, 排香松, 限 01/02(原料/包材) 分类。对齐 matSQL。
func (h *DashboardHandler) GetStockWarehouseDetail(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDomainAccess(r, "supply_chain")) {
		return
	}
	productCode := strings.TrimSpace(r.URL.Query().Get("productCode"))
	if productCode == "" {
		writeError(w, http.StatusBadRequest, "productCode required")
		return
	}
	// kind: "finished"=成品/半成品+其他(走吉客云 stock_quantity 8 仓); 空/其它=原材料/包材(走 ys_stock)
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))

	type warehouseRow struct {
		WarehouseName string  `json:"warehouseName"`
		OrgName       string  `json:"orgName"`
		Qty           float64 `json:"qty"`
	}
	warehouses := []warehouseRow{}
	var total float64

	// HAVING qty <> 0: 只展示有货的仓库(0 库存仓不贡献合计, 不展示)
	var sqlStr string
	var args []interface{}
	if kind == "finished" {
		// 成品/半成品 + 其他(广宣品): 口径对齐 prodSQL/otherSQL —— stock_quantity 按 goods_no, 8 仓白名单,
		// SUM(current_qty - locked_qty)。各仓相加 = 列里"当前库存"(8 仓外的库存不计, 与列一致)。
		// 吉客云库存无组织维度, org_name 留空。
		whCond, whArgs := buildPlanWarehouseFilter("sq.warehouse_name")
		sqlStr = `SELECT
			IFNULL(sq.warehouse_name, '(未分仓)') AS warehouse_name,
			'' AS org_name,
			ROUND(SUM(sq.current_qty - sq.locked_qty), 0) AS qty
			FROM stock_quantity sq
			WHERE sq.goods_no = ?` + whCond + `
			GROUP BY sq.warehouse_name
			HAVING qty <> 0
			ORDER BY qty DESC`
		args = append([]interface{}{productCode}, whArgs...)
	} else {
		// 原材料/包材: 口径与 matSQL 一致 —— ys_stock 按 product_code, 排香松, 限 01/02 分类; 按仓库汇总(跨批次)
		sqlStr = `SELECT
			IFNULL(ys.warehouse_name, '(未分仓)') AS warehouse_name,
			IFNULL(ys.org_name, '') AS org_name,
			ROUND(SUM(ys.currentqty), 2) AS qty
			FROM ys_stock ys
			WHERE ys.product_code = ?
			  AND (ys.manage_class_code LIKE '01%' OR ys.manage_class_code LIKE '02%')` + excludeAnhuiOrgYsWHERE + `
			GROUP BY ys.warehouse_name, ys.org_name
			HAVING qty <> 0
			ORDER BY qty DESC`
		args = []interface{}{productCode}
	}
	rows, err := h.DB.Query(sqlStr, args...)
	if err != nil {
		log.Printf("stock-detail query err: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var wr warehouseRow
		if err := rows.Scan(&wr.WarehouseName, &wr.OrgName, &wr.Qty); err == nil {
			warehouses = append(warehouses, wr)
			total += wr.Qty
		}
	}

	writeJSON(w, map[string]interface{}{
		"productCode": productCode,
		"warehouses":  warehouses,
		"total":       total,
	})
}
