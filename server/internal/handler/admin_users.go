package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func (h *DashboardHandler) adminUsersList(w http.ResponseWriter) {
	rows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT u.id, u.username, IFNULL(u.real_name,''), IFNULL(u.phone,''), u.status,
			IFNULL(DATE_FORMAT(u.last_login_at, '%Y-%m-%d %H:%i:%s'),''),
			IFNULL(GROUP_CONCAT(r.code ORDER BY r.code SEPARATOR ','), ''),
			IFNULL(u.remark,'')
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		GROUP BY u.id, u.username, u.real_name, u.phone, u.status, u.last_login_at, u.remark
		ORDER BY u.id`)
	if !ok {
		return
	}
	defer rows.Close()

	users := []adminUserListItem{}
	for rows.Next() {
		var user adminUserListItem
		var roleCodes string
		if writeDatabaseError(w, rows.Scan(&user.ID, &user.Username, &user.RealName, &user.Phone, &user.Status, &user.LastLoginAt, &roleCodes, &user.Remark)) {
			return
		}
		if roleCodes != "" {
			user.Roles = strings.Split(roleCodes, ",")
		} else {
			user.Roles = []string{}
		}
		users = append(users, user)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}

	writeJSON(w, map[string]interface{}{"list": users})
}

func (h *DashboardHandler) adminUsersCreate(w http.ResponseWriter, r *http.Request) {
	var req adminCreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.RealName = strings.TrimSpace(req.RealName)
	req.Status = normalizeUserStatus(req.Status)
	if req.Username == "" || req.Password == "" || req.RealName == "" {
		writeError(w, http.StatusBadRequest, "username, realName and password are required")
		return
	}
	if err := validatePassword(req.Password, req.Username); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	payload, _ := authPayloadFromContext(r)
	if containsString(req.RoleCodes, "super_admin") && (payload == nil || !payload.IsSuperAdmin) {
		writeError(w, http.StatusForbidden, "只有超级管理员才能分配super_admin角色")
		return
	}

	tx, err := h.DB.Begin()
	if writeDatabaseError(w, err) {
		return
	}
	defer tx.Rollback()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create password")
		return
	}

	result, err := tx.Exec(
		`INSERT INTO users (username, password_hash, real_name, status) VALUES (?, ?, ?, ?)`,
		req.Username, string(passwordHash), req.RealName, req.Status,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, "username already exists")
		return
	}

	userID, err := result.LastInsertId()
	if writeDatabaseError(w, err) {
		return
	}
	if err := h.saveUserAccessTx(tx, userID, req.RoleCodes, req.DataScopes); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		writeDatabaseError(w, err)
		return
	}

	access, err := h.loadAdminUserAccess(userID)
	if writeDatabaseError(w, err) {
		return
	}
	writeJSON(w, access)
}

func (h *DashboardHandler) adminUserAccessGet(w http.ResponseWriter, userID int64) {
	access, err := h.loadAdminUserAccess(userID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if writeDatabaseError(w, err) {
		return
	}
	writeJSON(w, access)
}

func (h *DashboardHandler) adminUserAccessUpdate(w http.ResponseWriter, r *http.Request, userID int64) {
	var req adminUpdateAccessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	payload, _ := authPayloadFromContext(r)
	targetIsSuperAdmin, err := h.isUserSuperAdmin(userID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if writeDatabaseError(w, err) {
		return
	}

	removingSuperAdmin := targetIsSuperAdmin && !containsString(req.RoleCodes, "super_admin")
	if payload != nil && payload.User.ID == userID && removingSuperAdmin {
		writeError(w, http.StatusBadRequest, "cannot remove your own super_admin role")
		return
	}
	// 非超管不能分配 super_admin 角色
	if payload != nil && !payload.IsSuperAdmin && containsString(req.RoleCodes, "super_admin") {
		writeError(w, http.StatusForbidden, "只有超级管理员才能分配super_admin角色")
		return
	}
	if removingSuperAdmin {
		remaining, err := h.countOtherActiveSuperAdmins(userID)
		if writeDatabaseError(w, err) {
			return
		}
		if remaining == 0 {
			writeError(w, http.StatusBadRequest, "cannot remove super_admin from the last active super_admin user")
			return
		}
	}

	tx, err := h.DB.Begin()
	if writeDatabaseError(w, err) {
		return
	}
	defer tx.Rollback()

	if err := h.saveUserAccessTx(tx, userID, req.RoleCodes, req.DataScopes); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		writeDatabaseError(w, err)
		return
	}

	h.logAudit(r, "permission_change", fmt.Sprintf("用户权限变更 #%d", userID), map[string]interface{}{"userId": userID, "roles": req.RoleCodes})

	access, err := h.loadAdminUserAccess(userID)
	if writeDatabaseError(w, err) {
		return
	}
	writeJSON(w, access)
}

func (h *DashboardHandler) adminUserStatusUpdate(w http.ResponseWriter, r *http.Request, userID int64) {
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	status := normalizeUserStatus(req.Status)
	payload, _ := authPayloadFromContext(r)
	if payload != nil && payload.User.ID == userID && status != "active" {
		writeError(w, http.StatusBadRequest, "cannot disable your own account")
		return
	}

	var existingStatus string
	err := h.DB.QueryRow("SELECT status FROM users WHERE id = ?", userID).Scan(&existingStatus)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if writeDatabaseError(w, err) {
		return
	}

	if status != "active" && existingStatus == "active" {
		targetIsSuperAdmin, err := h.isUserSuperAdmin(userID)
		if writeDatabaseError(w, err) {
			return
		}
		if targetIsSuperAdmin {
			remaining, err := h.countOtherActiveSuperAdmins(userID)
			if writeDatabaseError(w, err) {
				return
			}
			if remaining == 0 {
				writeError(w, http.StatusBadRequest, "cannot disable the last active super_admin user")
				return
			}
		}
	}

	if _, err := h.DB.Exec(`UPDATE users SET status = ? WHERE id = ?`, status, userID); writeDatabaseError(w, err) {
		return
	}

	writeJSON(w, map[string]string{"status": status})
}

func (h *DashboardHandler) adminUserPasswordUpdate(w http.ResponseWriter, r *http.Request, userID int64) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Password = strings.TrimSpace(req.Password)
	var username string
	err := h.DB.QueryRow(`SELECT username FROM users WHERE id = ?`, userID).Scan(&username)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if writeDatabaseError(w, err) {
		return
	}

	if err := validatePassword(req.Password, username); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create password")
		return
	}

	result, err := h.DB.Exec(`UPDATE users SET password_hash = ?, must_change_password = 1 WHERE id = ?`, string(passwordHash), userID)
	if writeDatabaseError(w, err) {
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err := h.revokeUserSessions(userID); err != nil {
		writeDatabaseError(w, err)
		return
	}

	writeJSON(w, map[string]string{"message": "ok"})
}

func (h *DashboardHandler) adminUserDelete(w http.ResponseWriter, r *http.Request, userID int64) {
	payload, _ := authPayloadFromContext(r)
	if payload != nil && payload.User.ID == userID {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	var exists int
	if err := h.DB.QueryRow("SELECT COUNT(*) FROM users WHERE id=?", userID).Scan(&exists); writeDatabaseError(w, err) {
		return
	}
	if exists == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	var targetIsSuperAdmin int
	if err := h.DB.QueryRow(`
		SELECT COUNT(*)
		FROM user_roles ur
		INNER JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = ? AND r.code = 'super_admin'`,
		userID,
	).Scan(&targetIsSuperAdmin); writeDatabaseError(w, err) {
		return
	}
	if targetIsSuperAdmin > 0 {
		var remainingSuperAdmins int
		if err := h.DB.QueryRow(`
			SELECT COUNT(DISTINCT ur.user_id)
			FROM user_roles ur
			INNER JOIN roles r ON r.id = ur.role_id
			WHERE r.code = 'super_admin' AND ur.user_id <> ?`,
			userID,
		).Scan(&remainingSuperAdmins); writeDatabaseError(w, err) {
			return
		}
		if remainingSuperAdmins == 0 {
			writeError(w, http.StatusBadRequest, "cannot delete the last super_admin user")
			return
		}
	}

	tx, err := h.DB.Begin()
	if writeDatabaseError(w, err) {
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM user_sessions WHERE user_id = ?`, userID); err != nil {
		writeDatabaseError(w, err)
		return
	}
	if _, err := tx.Exec(`DELETE FROM user_roles WHERE user_id = ?`, userID); err != nil {
		writeDatabaseError(w, err)
		return
	}
	if _, err := tx.Exec(`DELETE FROM data_scopes WHERE subject_type = 'user' AND subject_id = ?`, userID); err != nil {
		writeDatabaseError(w, err)
		return
	}
	result, err := tx.Exec(`DELETE FROM users WHERE id = ?`, userID)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if err := tx.Commit(); err != nil {
		writeDatabaseError(w, err)
		return
	}
	writeJSON(w, map[string]string{"message": "用户已删除"})
}

func (h *DashboardHandler) loadAdminUserAccess(userID int64) (*adminUserAccessResponse, error) {
	var resp adminUserAccessResponse
	resp.DataScopes = authDataScopes{
		Depts:      []string{},
		Platforms:  []string{},
		Shops:      []string{},
		Warehouses: []string{},
		Domains:    []string{},
	}

	if err := h.DB.QueryRow(
		`SELECT id, username, IFNULL(real_name,''), status FROM users WHERE id = ?`,
		userID,
	).Scan(&resp.UserID, &resp.Username, &resp.RealName, &resp.Status); err != nil {
		return nil, err
	}

	roleRows, err := h.DB.Query(
		`SELECT r.code
		 FROM roles r
		 INNER JOIN user_roles ur ON ur.role_id = r.id
		 WHERE ur.user_id = ?
		 ORDER BY r.code`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer roleRows.Close()
	for roleRows.Next() {
		var roleCode string
		if err := roleRows.Scan(&roleCode); err != nil {
			return nil, err
		}
		resp.RoleCodes = append(resp.RoleCodes, roleCode)
	}
	if err := roleRows.Err(); err != nil {
		return nil, err
	}

	scopeRows, err := h.DB.Query(
		`SELECT scope_type, scope_value FROM data_scopes WHERE subject_type = 'user' AND subject_id = ? ORDER BY scope_type, scope_value`,
		userID,
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

	resp.RoleCodes = uniqueSortedStrings(resp.RoleCodes)
	resp.DataScopes.Depts = uniqueSortedStrings(resp.DataScopes.Depts)
	resp.DataScopes.Platforms = uniqueSortedStrings(resp.DataScopes.Platforms)
	resp.DataScopes.Shops = uniqueSortedStrings(resp.DataScopes.Shops)
	resp.DataScopes.Warehouses = uniqueSortedStrings(resp.DataScopes.Warehouses)
	resp.DataScopes.Domains = uniqueSortedStrings(resp.DataScopes.Domains)

	return &resp, nil
}

func (h *DashboardHandler) saveUserAccessTx(tx *sql.Tx, userID int64, roleCodes []string, scopes authDataScopes) error {
	roleCodes = uniqueSortedStrings(roleCodes)
	scopeRows := [][]string{
		{"dept", strings.Join(uniqueSortedStrings(scopes.Depts), "\x1f")},
		{"platform", strings.Join(uniqueSortedStrings(scopes.Platforms), "\x1f")},
		{"shop", strings.Join(uniqueSortedStrings(scopes.Shops), "\x1f")},
		{"warehouse", strings.Join(uniqueSortedStrings(scopes.Warehouses), "\x1f")},
		{"domain", strings.Join(uniqueSortedStrings(scopes.Domains), "\x1f")},
	}

	if _, err := tx.Exec(`DELETE FROM user_roles WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM data_scopes WHERE subject_type = 'user' AND subject_id = ?`, userID); err != nil {
		return err
	}

	for _, roleCode := range roleCodes {
		var roleID int64
		if err := tx.QueryRow(`SELECT id FROM roles WHERE code = ?`, roleCode).Scan(&roleID); err != nil {
			return errors.New("invalid role code")
		}
		if _, err := tx.Exec(`INSERT INTO user_roles (user_id, role_id) VALUES (?, ?)`, userID, roleID); err != nil {
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
				`INSERT INTO data_scopes (subject_type, subject_id, scope_type, scope_value) VALUES ('user', ?, ?, ?)`,
				userID, scopeType, scopeValue,
			); err != nil {
				return err
			}
		}
	}

	return nil
}
