package handler

import (
	"errors"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookieName    = "bi_dashboard_session"
	defaultAdminUsername = "admin"
	defaultAdminPassword = "Admin@123456"
	maxLoginAttempts     = 5
	lockDuration         = 5 * time.Minute
	captchaExpiry        = 5 * time.Minute
	shortSessionDuration = 8 * time.Hour
	idleTimeout          = 2 * time.Hour
)

var sessionDuration = 7 * 24 * time.Hour

type dingtalkPendingUser struct {
	Nick       string
	UnionId    string
	OpenId     string
	Mobile     string
	Department string
	Expires    time.Time
}

var dingtalkPendingUsers = sync.Map{}

type authContextKey string

const currentAuthPayloadKey authContextKey = "currentAuthPayload"

type authPayload struct {
	User               authUser       `json:"user"`
	Roles              []string       `json:"roles"`
	Permissions        []string       `json:"permissions"`
	DataScopes         authDataScopes `json:"dataScopes"`
	IsSuperAdmin       bool           `json:"isSuperAdmin"`
	MustChangePassword bool           `json:"mustChangePassword"`
}

type authUser struct {
	ID               int64  `json:"id"`
	Username         string `json:"username"`
	RealName         string `json:"realName"`
	DingtalkRealName string `json:"dingtalkRealName"`
}

type authDataScopes struct {
	Depts      []string `json:"depts"`
	Platforms  []string `json:"platforms"`
	Shops      []string `json:"shops"`
	Warehouses []string `json:"warehouses"`
	Domains    []string `json:"domains"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func hasPermission(payload *authPayload, permission string) bool {
	if payload == nil {
		return false
	}
	if payload.IsSuperAdmin || containsString(payload.Roles, "super_admin") {
		return true
	}
	if permission == "" {
		return false
	}
	return containsString(payload.Permissions, permission)
}

func isSecureRequest(r *http.Request) bool {
	if r != nil && r.TLS != nil {
		return true
	}
	if r == nil {
		return false
	}
	if !isTrustedProxyRemoteAddr(r.RemoteAddr) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func isTrustedProxyRemoteAddr(remoteAddr string) bool {
	host := strings.TrimSpace(remoteAddr)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}

	if isTrustedProxyRemoteAddr(r.RemoteAddr) {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
			return truncateString(forwarded, 64)
		}
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			return truncateString(realIP, 64)
		}
	}

	host := strings.TrimSpace(r.RemoteAddr)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	return truncateString(host, 64)
}

func (h *DashboardHandler) revokeUserSessions(userID int64) error {
	if h == nil || h.DB == nil {
		return errors.New("db is not initialized")
	}
	_, err := h.DB.Exec("DELETE FROM user_sessions WHERE user_id = ?", userID)
	return err
}

func truncateString(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	runes := []rune(value)
	for len(string(runes)) > maxLen {
		runes = runes[:len(runes)-1]
	}
	return string(runes)
}
