package handler

import (
	"net/http"
)

func (h *DashboardHandler) AdminMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	roleRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT code, name FROM roles ORDER BY id`)
	if !ok {
		return
	}
	defer roleRows.Close()

	roles := []adminRoleOption{}
	for roleRows.Next() {
		var role adminRoleOption
		if writeDatabaseError(w, roleRows.Scan(&role.Code, &role.Name)) {
			return
		}
		roles = append(roles, role)
	}
	if writeDatabaseError(w, roleRows.Err()) {
		return
	}

	permissionRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT code, name, type FROM permissions ORDER BY type, id`)
	if !ok {
		return
	}
	defer permissionRows.Close()

	permissions := []adminPermissionOption{}
	for permissionRows.Next() {
		var permission adminPermissionOption
		if writeDatabaseError(w, permissionRows.Scan(&permission.Code, &permission.Name, &permission.Type)) {
			return
		}
		permissions = append(permissions, permission)
	}
	if writeDatabaseError(w, permissionRows.Err()) {
		return
	}

	deptRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DISTINCT department
		FROM sales_channel
		WHERE department IS NOT NULL AND department != ''
		ORDER BY department`)
	if !ok {
		return
	}
	defer deptRows.Close()

	depts := []adminMetaOption{}
	for deptRows.Next() {
		var value string
		if writeDatabaseError(w, deptRows.Scan(&value)) {
			return
		}
		depts = append(depts, adminMetaOption{Value: value, Label: adminDeptLabel(value)})
	}
	if writeDatabaseError(w, deptRows.Err()) {
		return
	}

	platforms := []adminMetaOption{
		{Value: "tmall", Label: "天猫"},
		{Value: "tmall_cs", Label: "天猫超市"},
		{Value: "jd", Label: "京东"},
		{Value: "pdd", Label: "拼多多"},
		{Value: "vip", Label: "唯品会"},
		{Value: "instant", Label: "即时零售"},
		{Value: "taobao", Label: "淘宝"},
		{Value: "douyin", Label: "抖音"},
		{Value: "kuaishou", Label: "快手"},
		{Value: "xiaohongshu", Label: "小红书"},
		{Value: "youzan", Label: "有赞"},
		{Value: "weidian", Label: "微店"},
		{Value: "shipinhao", Label: "视频号"},
	}

	shopRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DISTINCT channel_name, IFNULL(department, '')
		FROM sales_channel
		WHERE channel_name IS NOT NULL AND channel_name != ''
		ORDER BY IFNULL(department, ''), channel_name`)
	if !ok {
		return
	}
	defer shopRows.Close()

	shops := []adminMetaOption{}
	for shopRows.Next() {
		var shopName string
		var dept string
		if writeDatabaseError(w, shopRows.Scan(&shopName, &dept)) {
			return
		}
		label := shopName
		if dept != "" {
			label = adminDeptLabel(dept) + " / " + shopName
		}
		shops = append(shops, adminMetaOption{Value: shopName, Label: label})
	}
	if writeDatabaseError(w, shopRows.Err()) {
		return
	}

	warehouseRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT warehouse_name
		FROM (
			SELECT DISTINCT TRIM(warehouse_name) AS warehouse_name
			FROM stock_quantity
			WHERE warehouse_name IS NOT NULL AND TRIM(warehouse_name) != ''
			UNION
			SELECT DISTINCT TRIM(warehouse_name) AS warehouse_name
			FROM stock_batch
			WHERE warehouse_name IS NOT NULL AND TRIM(warehouse_name) != ''
		) w
		ORDER BY warehouse_name`)
	if !ok {
		return
	}
	defer warehouseRows.Close()

	warehouses := []adminMetaOption{}
	for warehouseRows.Next() {
		var warehouseName string
		if writeDatabaseError(w, warehouseRows.Scan(&warehouseName)) {
			return
		}
		warehouses = append(warehouses, adminMetaOption{Value: warehouseName, Label: warehouseName})
	}
	if writeDatabaseError(w, warehouseRows.Err()) {
		return
	}

	domains := []adminMetaOption{
		{Value: "sales", Label: "销售"},
		{Value: "ops", Label: "运营"},
		{Value: "finance", Label: "财务"},
		{Value: "supply_chain", Label: "供应链"},
	}

	writeJSON(w, map[string]interface{}{
		"roles":       roles,
		"permissions": permissions,
		"depts":       depts,
		"platforms":   platforms,
		"shops":       shops,
		"warehouses":  warehouses,
		"domains":     domains,
	})
}
