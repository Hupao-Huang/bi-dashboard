package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"
)

type adminMetaOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type adminRoleOption struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type adminPermissionOption struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type adminUserListItem struct {
	ID          int64    `json:"id"`
	Username    string   `json:"username"`
	RealName    string   `json:"realName"`
	Status      string   `json:"status"`
	LastLoginAt string   `json:"lastLoginAt"`
	Roles       []string `json:"roles"`
	Remark      string   `json:"remark,omitempty"`
}

type adminUserAccessResponse struct {
	DataScopes authDataScopes `json:"dataScopes"`
	RealName   string         `json:"realName"`
	RoleCodes  []string       `json:"roleCodes"`
	Status     string         `json:"status"`
	UserID     int64          `json:"userId"`
	Username   string         `json:"username"`
}

type adminCreateUserRequest struct {
	DataScopes authDataScopes `json:"dataScopes"`
	Password   string         `json:"password"`
	RealName   string         `json:"realName"`
	RoleCodes  []string       `json:"roleCodes"`
	Status     string         `json:"status"`
	Username   string         `json:"username"`
}

type adminUpdateAccessRequest struct {
	DataScopes authDataScopes `json:"dataScopes"`
	RoleCodes  []string       `json:"roleCodes"`
}

type adminRoleListItem struct {
	Builtin         bool   `json:"builtin"`
	Code            string `json:"code"`
	Description     string `json:"description"`
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	PermissionCount int64  `json:"permissionCount"`
	UserCount       int64  `json:"userCount"`
}

type adminRoleDetailResponse struct {
	Builtin     bool           `json:"builtin"`
	Code        string         `json:"code"`
	DataScopes  authDataScopes `json:"dataScopes"`
	Description string         `json:"description"`
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	Permissions []string       `json:"permissions"`
}

type adminUpdateRoleRequest struct {
	DataScopes  authDataScopes `json:"dataScopes"`
	Description string         `json:"description"`
	Name        string         `json:"name"`
	Permissions []string       `json:"permissions"`
}

var adminDeptLabelMap = map[string]string{
	"ecommerce":    "电商部门",
	"social":       "社媒部门",
	"offline":      "线下部门",
	"distribution": "分销部门",
	"finance":      "财务部门",
	"supply_chain": "供应链管理",
	"supply-chain": "供应链管理",
}

var builtInRoleCodes = func() map[string]struct{} {
	result := make(map[string]struct{}, len(roleSeeds))
	for _, seed := range roleSeeds {
		result[seed.Code] = struct{}{}
	}
	return result
}()

func adminDeptLabel(value string) string {
	if label, ok := adminDeptLabelMap[value]; ok {
		return label
	}
	return value
}

func isBuiltInRole(code string) bool {
	_, ok := builtInRoleCodes[code]
	return ok
}

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

func (h *DashboardHandler) AdminUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.adminUsersList(w)
	case http.MethodPost:
		h.adminUsersCreate(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *DashboardHandler) AdminRoles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// 列表继续走下面
	case http.MethodPost:
		h.adminRoleCreate(w, r)
		return
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT r.id, r.code, r.name, IFNULL(r.description,''),
			COUNT(DISTINCT rp.permission_id) AS permission_count,
			COUNT(DISTINCT ur.user_id) AS user_count
		FROM roles r
		LEFT JOIN role_permissions rp ON rp.role_id = r.id
		LEFT JOIN user_roles ur ON ur.role_id = r.id
		GROUP BY r.id, r.code, r.name, r.description
		ORDER BY r.id`)
	if !ok {
		return
	}
	defer rows.Close()

	roles := []adminRoleListItem{}
	for rows.Next() {
		var role adminRoleListItem
		if writeDatabaseError(w, rows.Scan(
			&role.ID,
			&role.Code,
			&role.Name,
			&role.Description,
			&role.PermissionCount,
			&role.UserCount,
		)) {
			return
		}
		role.Builtin = isBuiltInRole(role.Code)
		roles = append(roles, role)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}

	writeJSON(w, map[string]interface{}{"list": roles})
}

func (h *DashboardHandler) adminUsersList(w http.ResponseWriter) {
	rows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT u.id, u.username, IFNULL(u.real_name,''), u.status,
			IFNULL(DATE_FORMAT(u.last_login_at, '%Y-%m-%d %H:%i:%s'),''),
			IFNULL(GROUP_CONCAT(r.code ORDER BY r.code SEPARATOR ','), ''),
			IFNULL(u.remark,'')
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		GROUP BY u.id, u.username, u.real_name, u.status, u.last_login_at, u.remark
		ORDER BY u.id`)
	if !ok {
		return
	}
	defer rows.Close()

	users := []adminUserListItem{}
	for rows.Next() {
		var user adminUserListItem
		var roleCodes string
		if writeDatabaseError(w, rows.Scan(&user.ID, &user.Username, &user.RealName, &user.Status, &user.LastLoginAt, &roleCodes, &user.Remark)) {
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

func (h *DashboardHandler) AdminUserByPath(w http.ResponseWriter, r *http.Request) {
	userID, action, ok := parseAdminUserPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	switch action {
	case "":
		if r.Method != http.MethodDelete {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.adminUserDelete(w, r, userID)
	case "access":
		switch r.Method {
		case http.MethodGet:
			h.adminUserAccessGet(w, userID)
		case http.MethodPut:
			h.adminUserAccessUpdate(w, r, userID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "status":
		if r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.adminUserStatusUpdate(w, r, userID)
	case "password":
		if r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.adminUserPasswordUpdate(w, r, userID)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (h *DashboardHandler) AdminRoleByPath(w http.ResponseWriter, r *http.Request) {
	roleID, ok := parseAdminRolePath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.adminRoleGet(w, roleID)
	case http.MethodPut:
		h.adminRoleUpdate(w, r, roleID)
	case http.MethodDelete:
		h.adminRoleDelete(w, roleID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func parseAdminUserPath(path string) (int64, string, bool) {
	const prefix = "/api/admin/users/"
	if !strings.HasPrefix(path, prefix) {
		return 0, "", false
	}
	rest := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	parts := strings.Split(rest, "/")
	if len(parts) < 1 || len(parts) > 2 {
		return 0, "", false
	}
	userID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", false
	}
	if len(parts) == 1 {
		return userID, "", true
	}
	return userID, parts[1], true
}

func parseAdminRolePath(path string) (int64, bool) {
	const prefix = "/api/admin/roles/"
	if !strings.HasPrefix(path, prefix) {
		return 0, false
	}
	rest := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if rest == "" || strings.Contains(rest, "/") {
		return 0, false
	}
	roleID, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return 0, false
	}
	return roleID, true
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
		writeError(w, http.StatusInternalServerError, "创建失败: "+err.Error())
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

	role, err := h.loadAdminRoleDetail(roleID)
	if writeDatabaseError(w, err) {
		return
	}
	writeJSON(w, role)
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

func normalizeUserStatus(status string) string {
	if strings.EqualFold(status, "disabled") {
		return "disabled"
	}
	return "active"
}

func (h *DashboardHandler) isUserSuperAdmin(userID int64) (bool, error) {
	var exists int
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, userID).Scan(&exists); err != nil {
		return false, err
	}
	if exists == 0 {
		return false, sql.ErrNoRows
	}

	var cnt int
	if err := h.DB.QueryRow(`
		SELECT COUNT(*)
		FROM user_roles ur
		INNER JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = ? AND r.code = 'super_admin'`,
		userID,
	).Scan(&cnt); err != nil {
		return false, err
	}
	return cnt > 0, nil
}

func (h *DashboardHandler) countOtherActiveSuperAdmins(excludeUserID int64) (int, error) {
	var cnt int
	err := h.DB.QueryRow(`
		SELECT COUNT(DISTINCT u.id)
		FROM users u
		INNER JOIN user_roles ur ON ur.user_id = u.id
		INNER JOIN roles r ON r.id = ur.role_id
		WHERE u.status = 'active' AND r.code = 'super_admin' AND u.id <> ?`,
		excludeUserID,
	).Scan(&cnt)
	return cnt, err
}

// ==================== 批量导入用户 ====================

type batchImportRow struct {
	Row        int    `json:"row"`
	RealName   string `json:"realName"`
	Phone      string `json:"phone"`
	Department string `json:"department"`
	EmployeeID string `json:"employeeId"`
	Username   string `json:"username"`
	Valid      bool   `json:"valid"`
	Error      string `json:"error,omitempty"`
}

func (h *DashboardHandler) AdminUsersBatchImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "文件过大或格式错误")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "请上传Excel文件")
		return
	}
	defer file.Close()

	password := strings.TrimSpace(r.FormValue("password"))
	roleCodesJSON := r.FormValue("roleCodes")
	dryRun := r.FormValue("dryRun") == "true"

	if password == "" {
		writeError(w, http.StatusBadRequest, "请设置初始密码")
		return
	}
	if err := validatePassword(password, ""); err != nil {
		writeError(w, http.StatusBadRequest, "初始密码不符合要求: "+err.Error())
		return
	}

	var roleCodes []string
	if roleCodesJSON != "" {
		if err := json.Unmarshal([]byte(roleCodesJSON), &roleCodes); err != nil {
			writeError(w, http.StatusBadRequest, "角色参数格式错误")
			return
		}
	}

	// 解析 Excel
	f, err := excelize.OpenReader(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无法读取Excel文件")
		return
	}
	defer f.Close()

	sheetName := f.GetSheetName(0)
	allRows, err := f.GetRows(sheetName)
	if err != nil || len(allRows) < 2 {
		writeError(w, http.StatusBadRequest, "Excel为空或无数据行")
		return
	}

	// 按表头名称匹配列
	headerMap := map[string]string{
		"姓名": "realName", "名字": "realName", "员工姓名": "realName",
		"手机号": "phone", "手机": "phone", "手机号码": "phone", "联系电话": "phone",
		"部门": "department", "所在部门": "department",
		"工号": "employeeId", "员工工号": "employeeId",
	}
	colIndex := map[string]int{}
	for i, cell := range allRows[0] {
		cell = strings.TrimSpace(cell)
		if field, ok := headerMap[cell]; ok {
			colIndex[field] = i
		}
	}

	if _, ok := colIndex["realName"]; !ok {
		writeError(w, http.StatusBadRequest, "Excel中未找到[姓名]列")
		return
	}
	if _, ok := colIndex["phone"]; !ok {
		writeError(w, http.StatusBadRequest, "Excel中未找到[手机号]列")
		return
	}

	// 解析数据行
	rows := make([]batchImportRow, 0, len(allRows)-1)
	for i := 1; i < len(allRows); i++ {
		row := allRows[i]
		getCol := func(field string) string {
			if idx, ok := colIndex[field]; ok && idx < len(row) {
				return strings.TrimSpace(row[idx])
			}
			return ""
		}

		realName := getCol("realName")
		phone := getCol("phone")
		department := getCol("department")
		employeeId := getCol("employeeId")

		if realName == "" && phone == "" {
			continue // 跳过空行
		}

		item := batchImportRow{
			Row:        i + 1,
			RealName:   realName,
			Phone:      phone,
			Department: department,
			EmployeeID: employeeId,
			Username:   phone,
			Valid:      true,
		}

		if realName == "" {
			item.Valid = false
			item.Error = "姓名为空"
		} else if phone == "" {
			item.Valid = false
			item.Error = "手机号为空"
		} else if len(phone) < 11 {
			item.Valid = false
			item.Error = "手机号格式不正确"
		}

		rows = append(rows, item)
	}

	if len(rows) == 0 {
		writeError(w, http.StatusBadRequest, "没有有效的数据行")
		return
	}

	// 查重：检查已存在的用户名
	usernames := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Valid {
			usernames = append(usernames, row.Username)
		}
	}
	existingUsers := map[string]bool{}
	if len(usernames) > 0 {
		placeholders := strings.Repeat("?,", len(usernames))
		placeholders = placeholders[:len(placeholders)-1]
		args := make([]interface{}, len(usernames))
		for i, u := range usernames {
			args[i] = u
		}
		existRows, err := h.DB.Query(
			fmt.Sprintf(`SELECT username FROM users WHERE username IN (%s)`, placeholders),
			args...,
		)
		if err == nil {
			defer existRows.Close()
			for existRows.Next() {
				var u string
				existRows.Scan(&u)
				existingUsers[u] = true
			}
		}
	}

	// 检查导入列表内部重复
	seenPhones := map[string]int{}
	for i := range rows {
		if !rows[i].Valid {
			continue
		}
		if existingUsers[rows[i].Username] {
			rows[i].Valid = false
			rows[i].Error = "用户名已存在"
		} else if prevRow, dup := seenPhones[rows[i].Phone]; dup {
			rows[i].Valid = false
			rows[i].Error = fmt.Sprintf("手机号与第%d行重复", prevRow)
		} else {
			seenPhones[rows[i].Phone] = rows[i].Row
		}
	}

	// 统计
	validCount := 0
	errorItems := make([]batchImportRow, 0)
	for _, row := range rows {
		if row.Valid {
			validCount++
		} else {
			errorItems = append(errorItems, row)
		}
	}

	// 预览模式
	if dryRun {
		writeJSON(w, map[string]interface{}{
			"total":   len(rows),
			"valid":   validCount,
			"errors":  errorItems,
			"preview": rows,
		})
		return
	}

	if validCount == 0 {
		writeError(w, http.StatusBadRequest, "没有可导入的有效数据")
		return
	}

	// 正式导入：bcrypt 只算一次
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "密码加密失败")
		return
	}

	tx, err := h.DB.Begin()
	if writeDatabaseError(w, err) {
		return
	}
	defer tx.Rollback()

	imported := 0
	importErrors := make([]batchImportRow, 0)
	for i := range rows {
		if !rows[i].Valid {
			continue
		}

		result, err := tx.Exec(
			`INSERT INTO users (username, password_hash, real_name, phone, department, employee_id, must_change_password, status) VALUES (?, ?, ?, ?, ?, ?, 1, 'active')`,
			rows[i].Username, string(passwordHash), rows[i].RealName, rows[i].Phone, rows[i].Department, rows[i].EmployeeID,
		)
		if err != nil {
			rows[i].Valid = false
			rows[i].Error = "插入失败: " + err.Error()
			importErrors = append(importErrors, rows[i])
			continue
		}

		userID, _ := result.LastInsertId()
		if len(roleCodes) > 0 {
			if err := h.saveUserAccessTx(tx, userID, roleCodes, authDataScopes{}); err != nil {
				rows[i].Valid = false
				rows[i].Error = "分配角色失败: " + err.Error()
				importErrors = append(importErrors, rows[i])
				continue
			}
		}
		imported++
	}

	if err := tx.Commit(); err != nil {
		writeDatabaseError(w, err)
		return
	}

	writeJSON(w, map[string]interface{}{
		"total":    len(rows),
		"valid":    validCount,
		"imported": imported,
		"errors":   append(errorItems, importErrors...),
	})
}
