package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

func (h *DashboardHandler) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		payload, err := h.authPayloadFromRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		ctx := context.WithValue(r.Context(), currentAuthPayloadKey, payload)
		next(w, r.WithContext(ctx))
	}
}

func (h *DashboardHandler) RequirePermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return h.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		payload, ok := authPayloadFromContext(r)
		if !ok || !hasPermission(payload, permission) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		next(w, r)
	})
}

func (h *DashboardHandler) RequireAnyPermission(next http.HandlerFunc, permissions ...string) http.HandlerFunc {
	return h.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		payload, ok := authPayloadFromContext(r)
		if !ok {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		for _, permission := range permissions {
			if hasPermission(payload, permission) {
				next(w, r)
				return
			}
		}
		writeError(w, http.StatusForbidden, "forbidden")
	})
}

func (h *DashboardHandler) RequireAllPermissions(next http.HandlerFunc, permissions ...string) http.HandlerFunc {
	return h.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		payload, ok := authPayloadFromContext(r)
		if !ok {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		for _, permission := range permissions {
			if !hasPermission(payload, permission) {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
		}
		next(w, r)
	})
}

func authPayloadFromContext(r *http.Request) (*authPayload, bool) {
	payload, ok := r.Context().Value(currentAuthPayloadKey).(*authPayload)
	return payload, ok
}

func (h *DashboardHandler) authPayloadFromRequest(r *http.Request) (*authPayload, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, errors.New("missing session cookie")
	}

	tokenHash := hashSessionToken(cookie.Value)

	var userID int64
	var lastActiveAt time.Time
	err = h.DB.QueryRow(
		`SELECT user_id, IFNULL(last_active_at, created_at) FROM user_sessions WHERE token_hash = ? AND expires_at > NOW()`,
		tokenHash,
	).Scan(&userID, &lastActiveAt)
	if err != nil {
		return nil, err
	}

	payload, err := h.loadAuthPayload(userID)
	if err != nil {
		return nil, err
	}

	if !payload.IsSuperAdmin && time.Since(lastActiveAt) > idleTimeout {
		h.DB.Exec(`DELETE FROM user_sessions WHERE token_hash = ?`, tokenHash)
		return nil, errors.New("session idle timeout")
	}

	if _, execErr := h.DB.Exec(
		`UPDATE user_sessions SET last_active_at = NOW() WHERE token_hash = ?`,
		tokenHash,
	); execErr != nil {
		log.Printf("update session activity failed: %v", execErr)
	}

	return payload, nil
}

func (h *DashboardHandler) loadAuthPayload(userID int64) (*authPayload, error) {
	payload := &authPayload{
		DataScopes: authDataScopes{
			Depts:      []string{},
			Platforms:  []string{},
			Shops:      []string{},
			Warehouses: []string{},
			Domains:    []string{},
		},
	}

	var realName, dingtalkRealName sql.NullString
	if err := h.DB.QueryRow(
		`SELECT id, username, real_name, IFNULL(dingtalk_real_name,''), must_change_password FROM users WHERE id = ? AND status = 'active'`,
		userID,
	).Scan(&payload.User.ID, &payload.User.Username, &realName, &dingtalkRealName, &payload.MustChangePassword); err != nil {
		return nil, err
	}
	payload.User.RealName = realName.String
	payload.User.DingtalkRealName = dingtalkRealName.String

	roleIDs := []int64{}
	roleRows, err := h.DB.Query(
		`SELECT r.id, r.code
		 FROM roles r
		 INNER JOIN user_roles ur ON ur.role_id = r.id
		 WHERE ur.user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer roleRows.Close()

	for roleRows.Next() {
		var roleID int64
		var roleCode string
		if err := roleRows.Scan(&roleID, &roleCode); err != nil {
			return nil, err
		}
		roleIDs = append(roleIDs, roleID)
		payload.Roles = append(payload.Roles, roleCode)
	}
	if err := roleRows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(payload.Roles)

	payload.IsSuperAdmin = containsString(payload.Roles, "super_admin")
	if payload.IsSuperAdmin {
		payload.Permissions = allPermissionCodes()
		sort.Strings(payload.Permissions)
		return payload, nil
	}

	permissionRows, err := h.DB.Query(
		`SELECT DISTINCT p.code
		 FROM permissions p
		 INNER JOIN role_permissions rp ON rp.permission_id = p.id
		 INNER JOIN user_roles ur ON ur.role_id = rp.role_id
		 WHERE ur.user_id = ?
		 ORDER BY p.code`,
		userID,
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
		payload.Permissions = append(payload.Permissions, code)
	}
	if err := permissionRows.Err(); err != nil {
		return nil, err
	}

	scopes, err := h.loadDataScopes(userID, roleIDs)
	if err != nil {
		return nil, err
	}
	payload.DataScopes = scopes

	return payload, nil
}

func (h *DashboardHandler) loadDataScopes(userID int64, roleIDs []int64) (authDataScopes, error) {
	scopes := authDataScopes{}
	query := `SELECT scope_type, scope_value FROM data_scopes WHERE (subject_type = 'user' AND subject_id = ?)`
	args := []interface{}{userID}

	if len(roleIDs) > 0 {
		placeholders := make([]string, 0, len(roleIDs))
		for _, roleID := range roleIDs {
			placeholders = append(placeholders, "?")
			args = append(args, roleID)
		}
		query += fmt.Sprintf(" OR (subject_type = 'role' AND subject_id IN (%s))", strings.Join(placeholders, ","))
	}
	query += " ORDER BY scope_type, scope_value"

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		return scopes, err
	}
	defer rows.Close()

	for rows.Next() {
		var scopeType string
		var scopeValue string
		if err := rows.Scan(&scopeType, &scopeValue); err != nil {
			return scopes, err
		}
		switch scopeType {
		case "dept":
			scopes.Depts = append(scopes.Depts, scopeValue)
		case "platform":
			scopes.Platforms = append(scopes.Platforms, scopeValue)
		case "shop":
			scopes.Shops = append(scopes.Shops, scopeValue)
		case "warehouse":
			scopes.Warehouses = append(scopes.Warehouses, scopeValue)
		case "domain":
			scopes.Domains = append(scopes.Domains, scopeValue)
		}
	}
	if err := rows.Err(); err != nil {
		return scopes, err
	}

	scopes.Depts = uniqueSortedStrings(scopes.Depts)
	scopes.Platforms = uniqueSortedStrings(scopes.Platforms)
	scopes.Shops = uniqueSortedStrings(scopes.Shops)
	scopes.Warehouses = uniqueSortedStrings(scopes.Warehouses)
	scopes.Domains = uniqueSortedStrings(scopes.Domains)

	return scopes, nil
}

func generateSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

