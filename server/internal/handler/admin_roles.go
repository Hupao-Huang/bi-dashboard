package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

func (h *DashboardHandler) adminRoleGet(w http.ResponseWriter, roleID int64) {
	role, err := h.loadAdminRoleDetail(roleID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "role not found")
		return
	}
	if writeDatabaseError(w, err) {
		return
	}
	writeJSON(w, role)
}

func (h *DashboardHandler) adminRoleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Code        string `json:"code"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Code = strings.TrimSpace(req.Code)
	if req.Name == "" || req.Code == "" {
		writeError(w, http.StatusBadRequest, "角色名称和编码不能为空")
		return
	}

	result, err := h.DB.Exec(
		`INSERT INTO roles (code, name, description) VALUES (?, ?, ?)`,
		req.Code, req.Name, req.Description,
	)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			writeError(w, http.StatusConflict, "角色编码已存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "创建失败，请重试")
		return
	}

	id, _ := result.LastInsertId()
	writeJSON(w, map[string]interface{}{"id": id, "message": "创建成功"})
}

func (h *DashboardHandler) adminRoleDelete(w http.ResponseWriter, roleID int64) {
	// 检查是否内置角色
	var code string
	err := h.DB.QueryRow("SELECT code FROM roles WHERE id = ?", roleID).Scan(&code)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "role not found")
		return
	}
	if writeDatabaseError(w, err) {
		return
	}
	if isBuiltInRole(code) {
		writeError(w, http.StatusForbidden, "内置角色不可删除")
		return
	}

	// 检查是否有用户在使用
	var userCount int
	if err := h.DB.QueryRow("SELECT COUNT(*) FROM user_roles WHERE role_id = ?", roleID).Scan(&userCount); writeDatabaseError(w, err) {
		return
	}
	if userCount > 0 {
		writeError(w, http.StatusConflict, fmt.Sprintf("该角色下有 %d 个用户，请先移除", userCount))
		return
	}

	tx, err := h.DB.Begin()
	if writeDatabaseError(w, err) {
		return
	}
	defer tx.Rollback()

	// 删除角色权限和数据作用域
	if _, err := tx.Exec("DELETE FROM role_permissions WHERE role_id = ?", roleID); err != nil {
		writeDatabaseError(w, err)
		return
	}
	if _, err := tx.Exec("DELETE FROM data_scopes WHERE subject_type = 'role' AND subject_id = ?", roleID); err != nil {
		writeDatabaseError(w, err)
		return
	}
	result, err := tx.Exec("DELETE FROM roles WHERE id = ?", roleID)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeError(w, http.StatusNotFound, "role not found")
		return
	}

	if err := tx.Commit(); err != nil {
		writeDatabaseError(w, err)
		return
	}

	writeJSON(w, map[string]string{"message": "删除成功"})
}

func (h *DashboardHandler) adminRoleUpdate(w http.ResponseWriter, r *http.Request, roleID int64) {
	var req adminUpdateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "role name is required")
		return
	}

	currentRole, err := h.loadAdminRoleDetail(roleID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "role not found")
		return
	}
	if writeDatabaseError(w, err) {
		return
	}
	if currentRole.Code == "super_admin" {
		writeError(w, http.StatusBadRequest, "super_admin role is read only")
		return
	}

	tx, err := h.DB.Begin()
	if writeDatabaseError(w, err) {
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE roles SET name = ?, description = ? WHERE id = ?`, req.Name, req.Description, roleID); err != nil {
		writeDatabaseError(w, err)
		return
	}

	if err := h.saveRoleAccessTx(tx, roleID, req.Permissions, req.DataScopes); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		writeDatabaseError(w, err)
		return
	}

	h.logAudit(r, "permission_change", fmt.Sprintf("角色权限变更 #%d", roleID), map[string]interface{}{"roleId": roleID, "permissions": req.Permissions})

	role, err := h.loadAdminRoleDetail(roleID)
	if writeDatabaseError(w, err) {
		return
	}
	writeJSON(w, role)
}

func (h *DashboardHandler) loadAdminRoleDetail(roleID int64) (*adminRoleDetailResponse, error) {
	resp := &adminRoleDetailResponse{
		DataScopes: authDataScopes{
			Depts:      []string{},
			Platforms:  []string{},
			Shops:      []string{},
			Warehouses: []string{},
			Domains:    []string{},
		},
		Permissions: []string{},
	}

	if err := h.DB.QueryRow(
		`SELECT id, code, name, IFNULL(description,'') FROM roles WHERE id = ?`,
		roleID,
	).Scan(&resp.ID, &resp.Code, &resp.Name, &resp.Description); err != nil {
		return nil, err
	}
	resp.Builtin = isBuiltInRole(resp.Code)

	permissionRows, err := h.DB.Query(
		`SELECT p.code
		 FROM permissions p
		 INNER JOIN role_permissions rp ON rp.permission_id = p.id
		 WHERE rp.role_id = ?
		 ORDER BY p.code`,
		roleID,
	)
	if err != nil {
		return nil, err
	}
	defer permissionRows.Close()
	for permissionRows.Next() {
		var code string
		if err := permissionRows.Scan(&code); err != nil {
			return nil, err
		}
		resp.Permissions = append(resp.Permissions, code)
	}
	if err := permissionRows.Err(); err != nil {
		return nil, err
	}

	scopeRows, err := h.DB.Query(
		`SELECT scope_type, scope_value FROM data_scopes WHERE subject_type = 'role' AND subject_id = ? ORDER BY scope_type, scope_value`,
		roleID,
	)
	if err != nil {
		return nil, err
	}
	defer scopeRows.Close()
	for scopeRows.Next() {
		var scopeType string
		var scopeValue string
		if err := scopeRows.Scan(&scopeType, &scopeValue); err != nil {
			return nil, err
		}
		switch scopeType {
		case "dept":
			resp.DataScopes.Depts = append(resp.DataScopes.Depts, scopeValue)
		case "platform":
			resp.DataScopes.Platforms = append(resp.DataScopes.Platforms, scopeValue)
		case "shop":
			resp.DataScopes.Shops = append(resp.DataScopes.Shops, scopeValue)
		case "warehouse":
			resp.DataScopes.Warehouses = append(resp.DataScopes.Warehouses, scopeValue)
		case "domain":
			resp.DataScopes.Domains = append(resp.DataScopes.Domains, scopeValue)
		}
	}
	if err := scopeRows.Err(); err != nil {
		return nil, err
	}

	resp.Permissions = uniqueSortedStrings(resp.Permissions)
	resp.DataScopes.Depts = uniqueSortedStrings(resp.DataScopes.Depts)
	resp.DataScopes.Platforms = uniqueSortedStrings(resp.DataScopes.Platforms)
	resp.DataScopes.Shops = uniqueSortedStrings(resp.DataScopes.Shops)
	resp.DataScopes.Warehouses = uniqueSortedStrings(resp.DataScopes.Warehouses)
	resp.DataScopes.Domains = uniqueSortedStrings(resp.DataScopes.Domains)

	return resp, nil
}

func (h *DashboardHandler) saveRoleAccessTx(tx *sql.Tx, roleID int64, permissionCodes []string, scopes authDataScopes) error {
	permissionCodes = uniqueSortedStrings(permissionCodes)
	scopeRows := [][]string{
		{"dept", strings.Join(uniqueSortedStrings(scopes.Depts), "\x1f")},
		{"platform", strings.Join(uniqueSortedStrings(scopes.Platforms), "\x1f")},
		{"shop", strings.Join(uniqueSortedStrings(scopes.Shops), "\x1f")},
		{"warehouse", strings.Join(uniqueSortedStrings(scopes.Warehouses), "\x1f")},
		{"domain", strings.Join(uniqueSortedStrings(scopes.Domains), "\x1f")},
	}

	if _, err := tx.Exec(`DELETE FROM role_permissions WHERE role_id = ?`, roleID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM data_scopes WHERE subject_type = 'role' AND subject_id = ?`, roleID); err != nil {
		return err
	}

	for _, permissionCode := range permissionCodes {
		var permissionID int64
		if err := tx.QueryRow(`SELECT id FROM permissions WHERE code = ?`, permissionCode).Scan(&permissionID); err != nil {
			return errors.New("invalid permission code")
		}
		if _, err := tx.Exec(`INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)`, roleID, permissionID); err != nil {
			return err
		}
	}

	for _, scopeRow := range scopeRows {
		scopeType := scopeRow[0]
		if scopeRow[1] == "" {
			continue
		}
		for _, scopeValue := range strings.Split(scopeRow[1], "\x1f") {
			if _, err := tx.Exec(
				`INSERT INTO data_scopes (subject_type, subject_id, scope_type, scope_value) VALUES ('role', ?, ?, ?)`,
				roleID, scopeType, scopeValue,
			); err != nil {
				return err
			}
		}
	}

	return nil
}
