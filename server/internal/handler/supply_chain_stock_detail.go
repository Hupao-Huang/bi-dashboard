package handler

import (
	"log"
	"net/http"
	"strings"
)

// GetStockWarehouseDetail 当前库存按仓库下钻 — 原材料/包材点击"当前库存"数字弹窗用。
//
// 口径严格对齐 GetPurchasePlan 的 matSQL(原材料/包材段): ys_stock 按 product_code,
// 排除安徽香松组织, 限定 01/02(原料/包材) 分类。各仓库 SUM(currentqty) 相加 = 列里
// 显示的"当前库存"(一个料在用友里按仓库+批次分多行存, 这里按仓库汇总)。
func (h *DashboardHandler) GetStockWarehouseDetail(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDomainAccess(r, "supply_chain")) {
		return
	}
	productCode := strings.TrimSpace(r.URL.Query().Get("productCode"))
	if productCode == "" {
		writeError(w, http.StatusBadRequest, "productCode required")
		return
	}

	type warehouseRow struct {
		WarehouseName string  `json:"warehouseName"`
		OrgName       string  `json:"orgName"`
		Qty           float64 `json:"qty"`
	}
	warehouses := []warehouseRow{}
	var total float64

	// 口径与 matSQL 一致: 排除安徽香松 + 限定 01/02(原料/包材) 分类; 按仓库汇总(跨批次)
	// HAVING qty <> 0: 只展示有货的仓库(0 库存仓不贡献合计, 不展示)
	sqlStr := `SELECT
		IFNULL(ys.warehouse_name, '(未分仓)') AS warehouse_name,
		IFNULL(ys.org_name, '') AS org_name,
		ROUND(SUM(ys.currentqty), 2) AS qty
		FROM ys_stock ys
		WHERE ys.product_code = ?
		  AND (ys.manage_class_code LIKE '01%' OR ys.manage_class_code LIKE '02%')` + excludeAnhuiOrgYsWHERE + `
		GROUP BY ys.warehouse_name, ys.org_name
		HAVING qty <> 0
		ORDER BY qty DESC`
	rows, err := h.DB.Query(sqlStr, productCode)
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
