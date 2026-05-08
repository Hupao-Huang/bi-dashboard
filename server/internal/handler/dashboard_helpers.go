package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func getOverviewTrendRange(r *http.Request, start, end string) (string, string) {
	trendStart := strings.TrimSpace(r.URL.Query().Get("trendStart"))
	trendEnd := strings.TrimSpace(r.URL.Query().Get("trendEnd"))
	if trendStart == "" || trendEnd == "" {
		return start, end
	}
	if _, err := time.Parse("2006-01-02", trendStart); err != nil {
		return start, end
	}
	if _, err := time.Parse("2006-01-02", trendEnd); err != nil {
		return start, end
	}
	if trendStart > trendEnd {
		return start, end
	}
	return trendStart, trendEnd
}

func buildOverviewCacheKey(r *http.Request, start, end, trendStart, trendEnd string) string {
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		return fmt.Sprintf("anon|%s|%s|%s|%s", start, end, trendStart, trendEnd)
	}

	return fmt.Sprintf(
		"u:%d|sa:%t|%s|%s|%s|%s|d:%s|p:%s|s:%s|w:%s|dom:%s",
		payload.User.ID,
		payload.IsSuperAdmin,
		start,
		end,
		trendStart,
		trendEnd,
		strings.Join(payload.DataScopes.Depts, ","),
		strings.Join(payload.DataScopes.Platforms, ","),
		strings.Join(payload.DataScopes.Shops, ","),
		strings.Join(payload.DataScopes.Warehouses, ","),
		strings.Join(payload.DataScopes.Domains, ","),
	)
}

// getDateRange 从请求中获取日期范围，默认返回全部数据的范围
func getDateRange(r *http.Request, db *sql.DB) (string, string) {
	start := strings.TrimSpace(r.URL.Query().Get("start"))
	end := strings.TrimSpace(r.URL.Query().Get("end"))
	if start != "" && end != "" {
		_, startErr := time.Parse("2006-01-02", start)
		_, endErr := time.Parse("2006-01-02", end)
		if startErr == nil && endErr == nil && start <= end {
			return start, end
		}
	}

	_ = db
	// 动态默认值：本月1号~昨天
	now := time.Now()
	end = now.AddDate(0, 0, -1).Format("2006-01-02")
	start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	// 如果是月初（1号），昨天是上月，start用上月1号
	if now.Day() == 1 {
		start = now.AddDate(0, -1, 0).Format("2006-01-02")
		start = start[:8] + "01"
	}
	return start, end
}

// getTrendDateRange 趋势图日期范围：当选中范围<=7天时自动扩展到至少14天
// 返回 (trendStart, trendEnd)，汇总指标仍用原始 start/end
func getTrendDateRange(start, end string) (string, string) {
	s, err1 := time.Parse("2006-01-02", start)
	e, err2 := time.Parse("2006-01-02", end)
	if err1 != nil || err2 != nil {
		return start, end
	}
	days := int(e.Sub(s).Hours()/24) + 1
	if days <= 7 {
		// 往前推，保证至少14天
		trendStart := e.AddDate(0, 0, -13)
		return trendStart.Format("2006-01-02"), end
	}
	return start, end
}

// platformToPlats 平台Tab对应的online_plat_name列表
var platformToPlats = map[string][]string{
	"tmall":       {"天猫商城"},
	"tmall_cs":    {"天猫超市"},
	"jd":          {"京东"},
	"pdd":         {"拼多多"},
	"vip":         {"唯品会MP", "唯品会JIT"},
	"instant":     {"抖音超市"},
	"taobao":      {"淘宝"},
	"douyin":      {"放心购（抖音小店）"},
	"kuaishou":    {"快手小店"},
	"xiaohongshu": {"小红书"},
	"youzan":      {"有赞"},
	"weidian":     {"微店"},
	"shipinhao":   {"微信视频号小店"},
}

var deptPlatformTabWhitelist = map[string]map[string]struct{}{
	"social": {
		"douyin":      {},
		"kuaishou":    {},
		"xiaohongshu": {},
		"shipinhao":   {},
		"youzan":      {},
		"weidian":     {},
	},
}

func isPlatformAllowedForDept(dept, platform string) bool {
	allowed, ok := deptPlatformTabWhitelist[dept]
	if !ok {
		return true
	}
	_, exists := allowed[platform]
	return exists
}

// buildPlatformCond 根据platform参数构建SQL条件
func buildPlatformCond(dept, platform string) (string, []interface{}) {
	if platform == "" || platform == "all" {
		return "", nil
	}
	if !isPlatformAllowedForDept(dept, platform) {
		return " AND 1=0", nil
	}
	// 即时零售特殊处理：按店铺名模糊匹配（这些店在吉客云里没有平台名称）
	if platform == "instant" {
		return " AND shop_name LIKE '%即时零售%'", nil
	}
	plats, ok := platformToPlats[platform]
	if !ok {
		return "", nil
	}
	placeholders := make([]string, len(plats))
	args := make([]interface{}, len(plats))
	for i, p := range plats {
		placeholders[i] = "?"
		args[i] = p
	}
	cond := " AND shop_name IN (SELECT channel_name FROM sales_channel WHERE department = ? AND online_plat_name IN (" +
		joinStrings(placeholders, ",") + "))"
	return cond, append([]interface{}{dept}, args...)
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

