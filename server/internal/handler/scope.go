package handler

import (
	"errors"
	"net/http"
	"strings"
)

var errDataScopeForbidden = errors.New("forbidden by data scope")

func buildPlatformCondForKeys(platforms []string, shopColumn string) (string, []interface{}) {
	keys := uniqueSortedStrings(platforms)
	if len(keys) == 0 {
		return "", nil
	}

	containsInstant := false
	platNames := []string{}
	for _, key := range keys {
		if key == "instant" {
			containsInstant = true
			continue
		}
		platNames = append(platNames, platformToPlats[key]...)
	}
	platNames = uniqueSortedStrings(platNames)

	parts := []string{}
	args := []interface{}{}
	if len(platNames) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(platNames)), ",")
		parts = append(parts, shopColumn+" IN (SELECT channel_name FROM sales_channel WHERE online_plat_name IN ("+placeholders+"))")
		for _, platName := range platNames {
			args = append(args, platName)
		}
	}
	if containsInstant {
		parts = append(parts, shopColumn+" LIKE '%即时零售%'")
	}
	if len(parts) == 0 {
		return "", nil
	}
	return " AND (" + strings.Join(parts, " OR ") + ")", args
}

func buildSalesDataScopeCond(r *http.Request, requestedDept, requestedPlatform, requestedShop string) (string, []interface{}, error) {
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil || payload.IsSuperAdmin {
		return "", nil, nil
	}
	if len(payload.DataScopes.Domains) > 0 && !containsString(payload.DataScopes.Domains, "sales") {
		return "", nil, errDataScopeForbidden
	}

	conditions := []string{}
	args := []interface{}{}
	scopes := payload.DataScopes

	if len(scopes.Depts) > 0 {
		if requestedDept != "" {
			if !containsString(scopes.Depts, requestedDept) {
				return "", nil, errDataScopeForbidden
			}
		} else {
			placeholders := strings.TrimSuffix(strings.Repeat("?,", len(scopes.Depts)), ",")
			conditions = append(conditions, "department IN ("+placeholders+")")
			for _, dept := range scopes.Depts {
				args = append(args, dept)
			}
		}
	}

	if len(scopes.Platforms) > 0 {
		if requestedPlatform != "" && requestedPlatform != "all" {
			if !containsString(scopes.Platforms, requestedPlatform) {
				return "", nil, errDataScopeForbidden
			}
		} else {
			cond, condArgs := buildPlatformCondForKeys(scopes.Platforms, "shop_name")
			if cond != "" {
				conditions = append(conditions, strings.TrimPrefix(cond, " AND "))
				args = append(args, condArgs...)
			}
		}
	}

	if len(scopes.Shops) > 0 {
		if requestedShop != "" && requestedShop != "all" {
			if !containsString(scopes.Shops, requestedShop) {
				return "", nil, errDataScopeForbidden
			}
		} else {
			placeholders := strings.TrimSuffix(strings.Repeat("?,", len(scopes.Shops)), ",")
			conditions = append(conditions, "shop_name IN ("+placeholders+")")
			for _, shop := range scopes.Shops {
				args = append(args, shop)
			}
		}
	}

	if len(conditions) == 0 {
		return "", nil, nil
	}
	return " AND " + strings.Join(conditions, " AND "), args, nil
}

func requireDomainAccess(r *http.Request, allowedDomains ...string) error {
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil || payload.IsSuperAdmin {
		return nil
	}
	if len(payload.DataScopes.Domains) == 0 {
		// 当用户已配置其它数据范围但未配置 domain 时，避免跨域数据“默认放行”
		if len(payload.DataScopes.Depts) > 0 ||
			len(payload.DataScopes.Platforms) > 0 ||
			len(payload.DataScopes.Shops) > 0 ||
			len(payload.DataScopes.Warehouses) > 0 {
			return errDataScopeForbidden
		}
		return nil
	}
	for _, domain := range allowedDomains {
		if containsString(payload.DataScopes.Domains, domain) {
			return nil
		}
	}
	return errDataScopeForbidden
}

func requirePlatformAccess(r *http.Request, platform string) error {
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil || payload.IsSuperAdmin || len(payload.DataScopes.Platforms) == 0 || platform == "" || platform == "all" {
		return nil
	}
	if containsString(payload.DataScopes.Platforms, platform) {
		return nil
	}
	return errDataScopeForbidden
}

func requireShopAccess(r *http.Request, shop string) error {
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil || payload.IsSuperAdmin || len(payload.DataScopes.Shops) == 0 || shop == "" || shop == "all" {
		return nil
	}
	if containsString(payload.DataScopes.Shops, shop) {
		return nil
	}
	return errDataScopeForbidden
}

func requireWarehouseAccess(r *http.Request, warehouse string) error {
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil || payload.IsSuperAdmin || len(payload.DataScopes.Warehouses) == 0 || warehouse == "" || warehouse == "all" {
		return nil
	}
	if containsString(payload.DataScopes.Warehouses, warehouse) {
		return nil
	}
	return errDataScopeForbidden
}

func buildWarehouseScopeCond(r *http.Request, requestedWarehouse, warehouseColumn string) (string, []interface{}, error) {
	payload, ok := authPayloadFromContext(r)
	if requestedWarehouse != "" && requestedWarehouse != "all" {
		if ok && payload != nil && !payload.IsSuperAdmin && len(payload.DataScopes.Warehouses) > 0 &&
			!containsString(payload.DataScopes.Warehouses, requestedWarehouse) {
			return "", nil, errDataScopeForbidden
		}
		return " AND " + warehouseColumn + " = ?", []interface{}{requestedWarehouse}, nil
	}

	if !ok || payload == nil || payload.IsSuperAdmin || len(payload.DataScopes.Warehouses) == 0 {
		return "", nil, nil
	}

	allowedWarehouses := uniqueSortedStrings(payload.DataScopes.Warehouses)
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(allowedWarehouses)), ",")
	return " AND " + warehouseColumn + " IN (" + placeholders + ")", toInterfaceSlice(allowedWarehouses), nil
}

func toInterfaceSlice(values []string) []interface{} {
	result := make([]interface{}, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func requireDeptAccess(r *http.Request, dept string) error {
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil || payload.IsSuperAdmin || len(payload.DataScopes.Depts) == 0 || dept == "" {
		return nil
	}
	if containsString(payload.DataScopes.Depts, dept) {
		return nil
	}
	return errDataScopeForbidden
}

func writeScopeError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errDataScopeForbidden) {
		writeError(w, http.StatusForbidden, "forbidden by data scope")
		return true
	}
	writeDatabaseError(w, err)
	return true
}
