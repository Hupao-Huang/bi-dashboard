package handler

import (
	"net/http"
	"strings"
)

// GetFeiguaData 飞瓜看板数据
func (h *DashboardHandler) GetFeiguaData(w http.ResponseWriter, r *http.Request) {
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)
	platform := strings.TrimSpace(r.URL.Query().Get("platform")) // 可选: 抖音/快手/小红书
	if writeScopeError(w, requireDomainAccess(r, "ops")) || writeScopeError(w, requireDeptAccess(r, "social")) {
		return
	}

	payload, _ := authPayloadFromContext(r)
	allowedPlatformKeys := []string{}
	if payload != nil && !payload.IsSuperAdmin {
		allowedPlatformKeys = payload.DataScopes.Platforms
	}
	feiguaPlatformToScopeKey := map[string]string{
		"抖音":  "douyin",
		"快手":  "kuaishou",
		"小红书": "xiaohongshu",
	}
	feiguaScopeKeyToPlatform := map[string]string{
		"douyin":      "抖音",
		"kuaishou":    "快手",
		"xiaohongshu": "小红书",
	}

	platCond := ""
	var platArgs []interface{}
	if platform != "" && platform != "all" {
		if len(allowedPlatformKeys) > 0 {
			scopeKey, ok := feiguaPlatformToScopeKey[platform]
			if !ok || !containsString(allowedPlatformKeys, scopeKey) {
				writeError(w, http.StatusForbidden, "forbidden by data scope")
				return
			}
		}
		platCond = " AND platform = ?"
		platArgs = []interface{}{platform}
	} else if len(allowedPlatformKeys) > 0 {
		allowedPlatforms := []string{}
		for _, key := range uniqueSortedStrings(allowedPlatformKeys) {
			if platformName, ok := feiguaScopeKeyToPlatform[key]; ok {
				allowedPlatforms = append(allowedPlatforms, platformName)
			}
		}
		allowedPlatforms = uniqueSortedStrings(allowedPlatforms)
		if len(allowedPlatforms) == 0 {
			writeError(w, http.StatusForbidden, "forbidden by data scope")
			return
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(allowedPlatforms)), ",")
		platCond = " AND platform IN (" + placeholders + ")"
		platArgs = toInterfaceSlice(allowedPlatforms)
	}

	// 1. 每日GMV趋势（按平台分组）
	type DailyGMV struct {
		Date     string  `json:"date"`
		Platform string  `json:"platform"`
		GMV      float64 `json:"gmv"`
		Orders   int     `json:"orders"`
		Creators int     `json:"creators"`
	}
	var dailyGmv []DailyGMV
	tArgs := append([]interface{}{trendStart, trendEnd}, platArgs...)
	tRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'), platform,
			ROUND(SUM(gmv),2), SUM(order_count), COUNT(DISTINCT creator_name)
		FROM fg_creator_daily
		WHERE stat_date BETWEEN ? AND ?`+platCond+`
		GROUP BY stat_date, platform ORDER BY stat_date, platform`, tArgs...)
	if !ok {
		return
	}
	defer tRows.Close()
	for tRows.Next() {
		var d DailyGMV
		if writeDatabaseError(w, tRows.Scan(&d.Date, &d.Platform, &d.GMV, &d.Orders, &d.Creators)) {
			return
		}
		dailyGmv = append(dailyGmv, d)
	}
	if writeDatabaseError(w, tRows.Err()) {
		return
	}

	// 2. 汇总指标（用原始日期范围）
	type Summary struct {
		TotalGMV      float64 `json:"totalGmv"`
		TotalOrders   int     `json:"totalOrders"`
		TotalCreators int     `json:"totalCreators"`
		Commission    float64 `json:"commission"`
		RefundOrders  int     `json:"refundOrders"`
	}
	var summary Summary
	sArgs := append([]interface{}{start, end}, platArgs...)
	if err := h.DB.QueryRow(`
		SELECT IFNULL(ROUND(SUM(gmv),2),0), IFNULL(SUM(order_count),0),
			COUNT(DISTINCT creator_name), IFNULL(ROUND(SUM(commission),2),0),
			IFNULL(SUM(refund_orders),0)
		FROM fg_creator_daily WHERE stat_date BETWEEN ? AND ?`+platCond, sArgs...).
		Scan(&summary.TotalGMV, &summary.TotalOrders, &summary.TotalCreators,
			&summary.Commission, &summary.RefundOrders); err != nil {
		writeError(w, 500, "database query failed")
		return
	}

	// 3. 达人出单排行TOP20（用原始日期范围）
	type CreatorRank struct {
		CreatorName string  `json:"creatorName"`
		Platform    string  `json:"platform"`
		GMV         float64 `json:"gmv"`
		Orders      int     `json:"orders"`
		Commission  float64 `json:"commission"`
		Products    int     `json:"products"`
		Follower    string  `json:"follower"`
	}
	var creators []CreatorRank
	cArgs := append([]interface{}{start, end}, platArgs...)
	cRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT creator_name, platform, ROUND(SUM(gmv),2), SUM(order_count),
			ROUND(SUM(commission),2), SUM(product_count), IFNULL(MAX(follower), '未分配')
		FROM fg_creator_daily WHERE stat_date BETWEEN ? AND ?`+platCond+`
		GROUP BY creator_name, platform ORDER BY SUM(gmv) DESC LIMIT 20`, cArgs...)
	if !ok {
		return
	}
	defer cRows.Close()
	for cRows.Next() {
		var c CreatorRank
		if writeDatabaseError(w, cRows.Scan(&c.CreatorName, &c.Platform, &c.GMV, &c.Orders,
			&c.Commission, &c.Products, &c.Follower)) {
			return
		}
		creators = append(creators, c)
	}
	if writeDatabaseError(w, cRows.Err()) {
		return
	}

	// 4. 跟进人业绩排行
	type FollowerRank struct {
		Follower   string  `json:"follower"`
		GMV        float64 `json:"gmv"`
		Orders     int     `json:"orders"`
		CreatorCnt int     `json:"creatorCount"`
		Commission float64 `json:"commission"`
	}
	var followers []FollowerRank
	fArgs := append([]interface{}{start, end}, platArgs...)
	fRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT IFNULL(follower,'未分配'), ROUND(SUM(gmv),2), SUM(order_count),
			COUNT(DISTINCT creator_name), ROUND(SUM(commission),2)
		FROM fg_creator_daily WHERE stat_date BETWEEN ? AND ?`+platCond+`
		GROUP BY follower ORDER BY SUM(gmv) DESC LIMIT 20`, fArgs...)
	if !ok {
		return
	}
	defer fRows.Close()
	for fRows.Next() {
		var f FollowerRank
		if writeDatabaseError(w, fRows.Scan(&f.Follower, &f.GMV, &f.Orders, &f.CreatorCnt, &f.Commission)) {
			return
		}
		followers = append(followers, f)
	}
	if writeDatabaseError(w, fRows.Err()) {
		return
	}

	// 5. 平台占比
	type PlatformShare struct {
		Platform string  `json:"platform"`
		GMV      float64 `json:"gmv"`
		Orders   int     `json:"orders"`
		Creators int     `json:"creators"`
	}
	var platforms []PlatformShare
	pArgs := append([]interface{}{start, end}, platArgs...)
	pRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT platform, ROUND(SUM(gmv),2), SUM(order_count), COUNT(DISTINCT creator_name)
		FROM fg_creator_daily WHERE stat_date BETWEEN ? AND ?`+platCond+`
		GROUP BY platform ORDER BY SUM(gmv) DESC`, pArgs...)
	if !ok {
		return
	}
	defer pRows.Close()
	for pRows.Next() {
		var p PlatformShare
		if writeDatabaseError(w, pRows.Scan(&p.Platform, &p.GMV, &p.Orders, &p.Creators)) {
			return
		}
		platforms = append(platforms, p)
	}
	if writeDatabaseError(w, pRows.Err()) {
		return
	}

	// 6. 达人资源库统计
	type RosterStat struct {
		Platform  string `json:"platform"`
		Total     int    `json:"total"`
		Connected int    `json:"connected"`
	}
	var roster []RosterStat
	rArgs := append([]interface{}{}, platArgs...)
	rRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT platform, COUNT(*), SUM(CASE WHEN contact_status='已建联' THEN 1 ELSE 0 END)
		FROM fg_creator_roster
		WHERE 1 = 1`+platCond+`
		GROUP BY platform`, rArgs...)
	if !ok {
		return
	}
	defer rRows.Close()
	for rRows.Next() {
		var r RosterStat
		if writeDatabaseError(w, rRows.Scan(&r.Platform, &r.Total, &r.Connected)) {
			return
		}
		roster = append(roster, r)
	}
	if writeDatabaseError(w, rRows.Err()) {
		return
	}

	writeJSON(w, map[string]interface{}{
		"dailyGmv":  dailyGmv,
		"summary":   summary,
		"creators":  creators,
		"followers": followers,
		"platforms": platforms,
		"roster":    roster,
		"dateRange": map[string]string{"start": start, "end": end},
	})
}
