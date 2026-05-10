package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
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
	Phone       string   `json:"phone"`
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
	"ecommerce":      "电商部门",
	"social":         "社媒部门",
	"offline":        "线下部门",
	"distribution":   "分销部门",
	"instant_retail": "即时零售部",
	"finance":        "财务部门",
	"supply_chain":   "供应链管理",
	"supply-chain":   "供应链管理",
	"other":          "其他",
	"excluded":       "不计算销售",
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
