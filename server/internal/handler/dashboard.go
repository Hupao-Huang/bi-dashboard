package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type DashboardHandler struct {
	DB               *sql.DB
	DingToken        string
	DingSecret       string
	DingClientID     string
	DingClientSecret string
	HesiAppKey       string
	HesiSecret       string
	WebhookSecret    string
}

type overviewCacheEntry struct {
	data      map[string]interface{}
	expiresAt time.Time
}

var (
	overviewCacheMu sync.RWMutex
	overviewCache   = map[string]overviewCacheEntry{}
)

const (
	overviewCacheTTL      = 30 * time.Second
	deptCacheTTL          = 5 * time.Minute
	overviewCacheMaxItems = 1024
)

// ClearOverviewCache 清空所有接口缓存（同步脚本完成后调用，立即反映最新数据）
func ClearOverviewCache() int {
	overviewCacheMu.Lock()
	defer overviewCacheMu.Unlock()
	n := len(overviewCache)
	overviewCache = map[string]overviewCacheEntry{}
	return n
}

func getOverviewCache(key string) (map[string]interface{}, bool) {
	now := time.Now()
	overviewCacheMu.RLock()
	entry, ok := overviewCache[key]
	overviewCacheMu.RUnlock()
	if !ok || now.After(entry.expiresAt) {
		return nil, false
	}
	return entry.data, true
}

func setOverviewCache(key string, data map[string]interface{}) {
	setCacheWithTTL(key, data, overviewCacheTTL)
}

func setCacheWithTTL(key string, data map[string]interface{}, ttl time.Duration) {
	now := time.Now()

	overviewCacheMu.Lock()
	defer overviewCacheMu.Unlock()

	if len(overviewCache) >= overviewCacheMaxItems {
		for cacheKey, entry := range overviewCache {
			if now.After(entry.expiresAt) {
				delete(overviewCache, cacheKey)
			}
		}
		if len(overviewCache) >= overviewCacheMaxItems {
			overviewCache = map[string]overviewCacheEntry{}
		}
	}

	overviewCache[key] = overviewCacheEntry{
		data:      data,
		expiresAt: now.Add(ttl),
	}
}

func (h *DashboardHandler) WithCache(ttl time.Duration, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		payload, _ := authPayloadFromContext(r)
		uid := int64(0)
		if payload != nil {
			uid = payload.User.ID
		}
		key := fmt.Sprintf("api|%s?%s|u:%d", r.URL.Path, r.URL.RawQuery, uid)
		if cached, ok := getOverviewCache(key); ok {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cached)
			return
		}
		rec := &cacheResponseRecorder{ResponseWriter: w, statusCode: 200}
		handler(rec, r)
		if rec.statusCode == 200 && len(rec.body) > 0 {
			var parsed map[string]interface{}
			if json.Unmarshal(rec.body, &parsed) == nil {
				setCacheWithTTL(key, parsed, ttl)
			}
		}
	}
}

type cacheResponseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       []byte
}

func (r *cacheResponseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *cacheResponseRecorder) Write(b []byte) (int, error) {
	if r.statusCode == 200 {
		r.body = append(r.body, b...)
	}
	return r.ResponseWriter.Write(b)
}

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

// GetOverview 综合看板
func (h *DashboardHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getOverviewTrendRange(r, start, end)
	scopeCond, scopeArgs, err := buildSalesDataScopeCond(r, "", "", "")
	if writeScopeError(w, err) {
		return
	}

	cacheKey := buildOverviewCacheKey(r, start, end, trendStart, trendEnd)
	if cached, ok := getOverviewCache(cacheKey); ok {
		writeJSON(w, cached)
		return
	}

	// 1. 各部门汇总（含未映射部门，归入other）
	deptArgs := append([]interface{}{start, end}, scopeArgs...)
	deptRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT CASE WHEN department IS NULL OR department = '' THEN 'other' ELSE department END as dept,
			ROUND(SUM(goods_amt), 2) as sales,
			ROUND(SUM(goods_qty), 0) as qty,
			ROUND(SUM(gross_profit), 2) as profit,
			ROUND(SUM(goods_cost), 2) as cost,
			COUNT(DISTINCT goods_id) as sku_count
		FROM sales_goods_summary
		WHERE stat_date BETWEEN ? AND ?`+scopeCond+`
		GROUP BY dept
		ORDER BY sales DESC`, deptArgs...)
	if !ok {
		return
	}
	defer deptRows.Close()

	type DeptSummary struct {
		Department string  `json:"department"`
		Sales      float64 `json:"sales"`
		Qty        float64 `json:"qty"`
		Profit     float64 `json:"profit"`
		Cost       float64 `json:"cost"`
		SkuCount   int     `json:"skuCount"`
	}
	deptMap := map[string]DeptSummary{}
	for deptRows.Next() {
		var d DeptSummary
		if writeDatabaseError(w, deptRows.Scan(&d.Department, &d.Sales, &d.Qty, &d.Profit, &d.Cost, &d.SkuCount)) {
			return
		}
		deptMap[d.Department] = d
	}
	if writeDatabaseError(w, deptRows.Err()) {
		return
	}
	// 确保4个部门都返回（没数据的补0）
	allDepts := []string{"ecommerce", "social", "offline", "distribution"}
	var deptList []DeptSummary
	for _, dept := range allDepts {
		if d, ok := deptMap[dept]; ok {
			deptList = append(deptList, d)
		} else {
			deptList = append(deptList, DeptSummary{Department: dept})
		}
	}
	// 加上其他未知部门
	for dept, d := range deptMap {
		found := false
		for _, ad := range allDepts {
			if dept == ad {
				found = true
				break
			}
		}
		if !found {
			deptList = append(deptList, d)
		}
	}

	// 2. 每日销售趋势（含未映射部门，归入other）
	trendArgs := append([]interface{}{trendStart, trendEnd}, scopeArgs...)
	trendRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date, '%Y-%m-%d') as d,
			CASE WHEN department IS NULL OR department = '' THEN 'other' ELSE department END as dept,
			ROUND(SUM(goods_amt), 2) as sales,
			ROUND(SUM(goods_qty), 0) as qty
		FROM sales_goods_summary
		WHERE stat_date BETWEEN ? AND ?`+scopeCond+`
		GROUP BY stat_date, dept
		ORDER BY stat_date`, trendArgs...)
	if !ok {
		return
	}
	defer trendRows.Close()

	type TrendPoint struct {
		Date       string  `json:"date"`
		Department string  `json:"department"`
		Sales      float64 `json:"sales"`
		Qty        float64 `json:"qty"`
	}
	var trend []TrendPoint
	for trendRows.Next() {
		var t TrendPoint
		if writeDatabaseError(w, trendRows.Scan(&t.Date, &t.Department, &t.Sales, &t.Qty)) {
			return
		}
		trend = append(trend, t)
	}
	if writeDatabaseError(w, trendRows.Err()) {
		return
	}

	// 3. 商品销售排行 TOP15
	goodsArgs := append([]interface{}{start, end}, scopeArgs...)
	goodsRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT s.goods_no, s.goods_name, s.brand_name, s.cate_name,
			IFNULL(g.goods_field7,'') as grade,
			ROUND(SUM(s.goods_amt), 2) as sales,
			ROUND(SUM(s.goods_qty), 0) as qty,
			ROUND(SUM(s.gross_profit), 2) as profit
		FROM sales_goods_summary s
		LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
		WHERE s.goods_no IS NOT NULL AND s.goods_no != ''
		  AND s.stat_date BETWEEN ? AND ?`+strings.ReplaceAll(scopeCond, "shop_name", "s.shop_name")+`
		GROUP BY s.goods_no, s.goods_name, s.brand_name, s.cate_name, g.goods_field7
		ORDER BY sales DESC LIMIT 15`, goodsArgs...)
	if !ok {
		return
	}
	defer goodsRows.Close()

	type GoodsRank struct {
		GoodsNo  string  `json:"goodsNo"`
		Name     string  `json:"goodsName"`
		Brand    string  `json:"brand"`
		Category string  `json:"category"`
		Grade    string  `json:"grade"`
		Sales    float64 `json:"sales"`
		Qty      float64 `json:"qty"`
		Profit   float64 `json:"profit"`
	}
	var topGoods []GoodsRank
	for goodsRows.Next() {
		var g GoodsRank
		if writeDatabaseError(w, goodsRows.Scan(&g.GoodsNo, &g.Name, &g.Brand, &g.Category, &g.Grade, &g.Sales, &g.Qty, &g.Profit)) {
			return
		}
		topGoods = append(topGoods, g)
	}
	if writeDatabaseError(w, goodsRows.Err()) {
		return
	}

	// 3.5 商品渠道分布
	type OverviewChannelSales struct {
		ShopName string  `json:"shopName"`
		Sales    float64 `json:"sales"`
		Qty      float64 `json:"qty"`
	}
	overviewGoodsChannels := map[string][]OverviewChannelSales{}
	if len(topGoods) > 0 {
		ph := make([]string, len(topGoods))
		chArgs := []interface{}{start, end}
		for i, g := range topGoods {
			ph[i] = "?"
			chArgs = append(chArgs, g.GoodsNo)
		}
		chArgs = append(chArgs, scopeArgs...)
		chRows, ok := queryRowsOrWriteError(w, h.DB, `
			SELECT goods_no, shop_name,
				ROUND(SUM(goods_amt), 2) as sales,
				ROUND(SUM(goods_qty), 0) as qty
			FROM sales_goods_summary
			WHERE stat_date BETWEEN ? AND ?
			  AND goods_no IN (`+joinStrings(ph, ",")+`)`+scopeCond+`
			GROUP BY goods_no, shop_name
			ORDER BY goods_no, sales DESC`, chArgs...)
		if !ok {
			return
		}
		defer chRows.Close()
		for chRows.Next() {
			var goodsNo, shopName string
			var sales, qty float64
			if writeDatabaseError(w, chRows.Scan(&goodsNo, &shopName, &sales, &qty)) {
				return
			}
			overviewGoodsChannels[goodsNo] = append(overviewGoodsChannels[goodsNo], OverviewChannelSales{ShopName: shopName, Sales: sales, Qty: qty})
		}
		if writeDatabaseError(w, chRows.Err()) {
			return
		}
	}

	// 4. 店铺/渠道排行 TOP15
	shopArgs := append([]interface{}{start, end}, scopeArgs...)
	shopRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT shop_name, department,
			ROUND(SUM(goods_amt), 2) as sales,
			ROUND(SUM(goods_qty), 0) as qty
		FROM sales_goods_summary
		WHERE shop_name IS NOT NULL AND shop_name != ''
		  AND stat_date BETWEEN ? AND ?`+scopeCond+`
		GROUP BY shop_name, department
		ORDER BY sales DESC LIMIT 15`, shopArgs...)
	if !ok {
		return
	}
	defer shopRows.Close()

	type ShopRank struct {
		ShopName   string  `json:"shopName"`
		Department string  `json:"department"`
		Sales      float64 `json:"sales"`
		Qty        float64 `json:"qty"`
	}
	var topShops []ShopRank
	for shopRows.Next() {
		var s ShopRank
		if writeDatabaseError(w, shopRows.Scan(&s.ShopName, &s.Department, &s.Sales, &s.Qty)) {
			return
		}
		topShops = append(topShops, s)
	}
	if writeDatabaseError(w, shopRows.Err()) {
		return
	}

	// 5. 产品定位分布
	type GradeDist struct {
		Grade string  `json:"grade"`
		Sales float64 `json:"sales"`
	}
	var grades []GradeDist
	gradeArgs := append([]interface{}{start, end}, scopeArgs...)
	gradeRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT IFNULL(g.goods_field7,'未设置') as grade,
			ROUND(SUM(s.goods_amt), 2) as sales
		FROM sales_goods_summary s
		LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
		WHERE s.stat_date BETWEEN ? AND ?`+strings.ReplaceAll(scopeCond, "shop_name", "s.shop_name")+`
		GROUP BY g.goods_field7
		ORDER BY FIELD(g.goods_field7,'S','A','B','C','D'), sales DESC`, gradeArgs...)
	if !ok {
		return
	}
	defer gradeRows.Close()
	for gradeRows.Next() {
		var gd GradeDist
		if writeDatabaseError(w, gradeRows.Scan(&gd.Grade, &gd.Sales)) {
			return
		}
		grades = append(grades, gd)
	}
	if writeDatabaseError(w, gradeRows.Err()) {
		return
	}

	// 6. 产品定位 × 部门明细（用于 hover 展开）
	type GradeDeptSales struct {
		Grade      string  `json:"grade"`
		Department string  `json:"department"`
		Sales      float64 `json:"sales"`
	}
	var gradeDeptSales []GradeDeptSales
	gdArgs := append([]interface{}{start, end}, scopeArgs...)
	gdRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT IFNULL(g.goods_field7,'未设置') as grade,
			IFNULL(s.department,'其他') as department,
			ROUND(SUM(s.goods_amt), 2) as sales
		FROM sales_goods_summary s
		LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
		WHERE s.stat_date BETWEEN ? AND ?`+strings.ReplaceAll(scopeCond, "shop_name", "s.shop_name")+`
		GROUP BY g.goods_field7, s.department
		ORDER BY FIELD(g.goods_field7,'S','A','B','C','D'), sales DESC`, gdArgs...)
	if !ok {
		return
	}
	defer gdRows.Close()
	for gdRows.Next() {
		var gd GradeDeptSales
		if writeDatabaseError(w, gdRows.Scan(&gd.Grade, &gd.Department, &gd.Sales)) {
			return
		}
		gradeDeptSales = append(gradeDeptSales, gd)
	}
	if writeDatabaseError(w, gdRows.Err()) {
		return
	}

	// 7. 可选日期范围
	var minDate, maxDate string
	_ = h.DB.QueryRow("SELECT IFNULL(MIN(stat_date),''), IFNULL(MAX(stat_date),'') FROM sales_goods_summary").Scan(&minDate, &maxDate)

	response := map[string]interface{}{
		"departments":    deptList,
		"trend":          trend,
		"topGoods":       topGoods,
		"goodsChannels":  overviewGoodsChannels,
		"topShops":       topShops,
		"grades":         grades,
		"gradeDeptSales": gradeDeptSales,
		"dateRange":      map[string]string{"start": start, "end": end, "min": minDate, "max": maxDate},
		"trendRange":     map[string]string{"start": trendStart, "end": trendEnd},
	}
	setOverviewCache(cacheKey, response)
	writeJSON(w, response)
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

// GetDepartmentDetail 部门详情
func (h *DashboardHandler) GetDepartmentDetail(w http.ResponseWriter, r *http.Request) {
	dept := r.URL.Query().Get("dept")
	if dept == "" {
		writeError(w, 400, "dept is required")
		return
	}
	start, end := getDateRange(r, h.DB)
	shop := r.URL.Query().Get("shop")         // 可选：按店铺过滤
	platform := r.URL.Query().Get("platform") // 可选：按平台过滤

	// 线下大区合并：shop_name → 大区名映射
	const offlineRegionExpr = `CASE
		WHEN shop_name LIKE '%华东大区%' THEN '华东大区'
		WHEN shop_name LIKE '%华北大区%' THEN '华北大区'
		WHEN shop_name LIKE '%华南大区%' THEN '华南大区'
		WHEN shop_name LIKE '%华中大区%' THEN '华中大区'
		WHEN shop_name LIKE '%西北大区%' THEN '西北大区'
		WHEN shop_name LIKE '%西南大区%' THEN '西南大区'
		WHEN shop_name LIKE '%东北大区%' THEN '东北大区'
		WHEN shop_name LIKE '%山东大区%' OR shop_name LIKE '%山东省区%' THEN '山东大区'
		WHEN shop_name LIKE '%重客系统%' THEN '重客'
		ELSE NULL END`
	offlineRegionCond := map[string]string{
		"华东大区": "shop_name LIKE '%华东大区%'",
		"华北大区": "shop_name LIKE '%华北大区%'",
		"华南大区": "shop_name LIKE '%华南大区%'",
		"华中大区": "shop_name LIKE '%华中大区%'",
		"西北大区": "shop_name LIKE '%西北大区%'",
		"西南大区": "shop_name LIKE '%西南大区%'",
		"东北大区": "shop_name LIKE '%东北大区%'",
		"山东大区": "(shop_name LIKE '%山东大区%' OR shop_name LIKE '%山东省区%')",
		"重客":    "shop_name LIKE '%重客系统%'",
	}

	// 构建额外条件
	shopCond := ""
	extraArgs := []interface{}{}
	if shop != "" {
		if dept == "offline" {
			if cond, ok := offlineRegionCond[shop]; ok {
				shopCond = " AND " + cond
			}
		} else {
			shopCond = " AND shop_name = ?"
			extraArgs = append(extraArgs, shop)
		}
	}
	platCond, platArgs := buildPlatformCond(dept, platform)
	extraArgs = append(extraArgs, platArgs...)
	scopeCond, scopeArgs, err := buildSalesDataScopeCond(r, dept, platform, shop)
	if writeScopeError(w, err) {
		return
	}
	extraArgs = append(extraArgs, scopeArgs...)

	// 1. 每日趋势（短范围自动扩展）
	trendStart, trendEnd := getTrendDateRange(start, end)
	trendArgs := append([]interface{}{dept, trendStart, trendEnd}, extraArgs...)
	trendRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date, '%Y-%m-%d'),
			ROUND(SUM(local_goods_amt), 2) as sales,
			ROUND(SUM(goods_qty), 0) as qty,
			ROUND(SUM(gross_profit), 2) as profit,
			ROUND(SUM(IFNULL(goods_cost,0) + IFNULL(fixed_cost,0)), 2) as cost
		FROM sales_goods_summary
		WHERE department = ? AND stat_date BETWEEN ? AND ?`+shopCond+platCond+`
		`+scopeCond+`
		GROUP BY stat_date ORDER BY stat_date`, trendArgs...)
	if !ok {
		return
	}
	defer trendRows.Close()

	type DailyData struct {
		Date   string  `json:"date"`
		Sales  float64 `json:"sales"`
		Qty    float64 `json:"qty"`
		Profit float64 `json:"profit"`
		Cost   float64 `json:"cost"`
	}
	var daily []DailyData
	for trendRows.Next() {
		var d DailyData
		if writeDatabaseError(w, trendRows.Scan(&d.Date, &d.Sales, &d.Qty, &d.Profit, &d.Cost)) {
			return
		}
		daily = append(daily, d)
	}
	if writeDatabaseError(w, trendRows.Err()) {
		return
	}

	// 2. 店铺/大区排行（offline 按大区合并，其余按 shop_name）
	shopListArgs := append([]interface{}{dept, start, end}, platArgs...)
	shopListArgs = append(shopListArgs, scopeArgs...)
	var shopListSQL string
	if dept == "offline" {
		shopListSQL = `SELECT ` + offlineRegionExpr + ` as shop_name,
			ROUND(SUM(local_goods_amt), 2) as sales,
			ROUND(SUM(goods_qty), 0) as qty,
			ROUND(SUM(gross_profit), 2) as profit
		FROM sales_goods_summary
		WHERE department = ? AND shop_name IS NOT NULL
		  AND stat_date BETWEEN ? AND ?` + scopeCond + `
		GROUP BY shop_name HAVING shop_name IS NOT NULL ORDER BY sales DESC`
	} else {
		shopListSQL = `SELECT shop_name,
			ROUND(SUM(local_goods_amt), 2) as sales,
			ROUND(SUM(goods_qty), 0) as qty,
			ROUND(SUM(gross_profit), 2) as profit
		FROM sales_goods_summary
		WHERE department = ? AND shop_name IS NOT NULL
		  AND stat_date BETWEEN ? AND ?` + platCond + scopeCond + `
		GROUP BY shop_name ORDER BY sales DESC LIMIT 20`
	}
	shopRows, ok := queryRowsOrWriteError(w, h.DB, shopListSQL, shopListArgs...)
	if !ok {
		return
	}
	defer shopRows.Close()

	type ShopData struct {
		ShopName string  `json:"shopName"`
		Sales    float64 `json:"sales"`
		Qty      float64 `json:"qty"`
		Profit   float64 `json:"profit"`
	}
	var shops []ShopData
	for shopRows.Next() {
		var s ShopData
		if writeDatabaseError(w, shopRows.Scan(&s.ShopName, &s.Sales, &s.Qty, &s.Profit)) {
			return
		}
		shops = append(shops, s)
	}
	if writeDatabaseError(w, shopRows.Err()) {
		return
	}

	// 3. 商品排行
	goodsArgs := append([]interface{}{dept, start, end}, extraArgs...)
	goodsRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT s.goods_no, s.goods_name, s.brand_name, IFNULL(s.cate_name,''),
			ROUND(SUM(s.local_goods_amt), 2) as sales,
			ROUND(SUM(s.goods_qty), 0) as qty,
			ROUND(SUM(s.gross_profit), 2) as profit,
			IFNULL(g.goods_field7,'') as grade
		FROM sales_goods_summary s
		LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods WHERE goods_field7 IS NOT NULL AND goods_field7 != '') g ON g.goods_no = s.goods_no
		WHERE s.department = ? AND s.goods_no IS NOT NULL
		  AND s.stat_date BETWEEN ? AND ?`+strings.ReplaceAll(shopCond+platCond+scopeCond, "shop_name", "s.shop_name")+`
		GROUP BY s.goods_no, s.goods_name, s.brand_name, s.cate_name, g.goods_field7
		ORDER BY sales DESC LIMIT 15`, goodsArgs...)
	if !ok {
		return
	}
	defer goodsRows.Close()

	type GoodsData struct {
		GoodsNo  string  `json:"goodsNo"`
		Name     string  `json:"goodsName"`
		Brand    string  `json:"brand"`
		Category string  `json:"category"`
		Sales    float64 `json:"sales"`
		Qty      float64 `json:"qty"`
		Profit   float64 `json:"profit"`
		Grade    string  `json:"grade"`
	}
	var goods []GoodsData
	for goodsRows.Next() {
		var g GoodsData
		if writeDatabaseError(w, goodsRows.Scan(&g.GoodsNo, &g.Name, &g.Brand, &g.Category, &g.Sales, &g.Qty, &g.Profit, &g.Grade)) {
			return
		}
		goods = append(goods, g)
	}
	if writeDatabaseError(w, goodsRows.Err()) {
		return
	}

	// 3.5 商品渠道分布（为TOP15每个商品查各渠道销售额）
	type ChannelSales struct {
		ShopName string  `json:"shopName"`
		Sales    float64 `json:"sales"`
		Qty      float64 `json:"qty"`
	}
	goodsChannels := map[string][]ChannelSales{}
	if len(goods) > 0 {
		placeholders := make([]string, len(goods))
		chArgs := []interface{}{dept, start, end}
		for i, g := range goods {
			placeholders[i] = "?"
			chArgs = append(chArgs, g.GoodsNo)
		}
		chArgs = append(chArgs, extraArgs...)
		chRows, ok := queryRowsOrWriteError(w, h.DB, `
			SELECT goods_no, shop_name,
				ROUND(SUM(local_goods_amt), 2) as sales,
				ROUND(SUM(goods_qty), 0) as qty
			FROM sales_goods_summary
			WHERE department = ? AND stat_date BETWEEN ? AND ?
			  AND goods_no IN (`+joinStrings(placeholders, ",")+`)
			`+shopCond+platCond+scopeCond+`
			GROUP BY goods_no, shop_name
			ORDER BY goods_no, sales DESC`, chArgs...)
		if !ok {
			return
		}
		defer chRows.Close()
		for chRows.Next() {
			var goodsNo, shopName string
			var sales, qty float64
			if writeDatabaseError(w, chRows.Scan(&goodsNo, &shopName, &sales, &qty)) {
				return
			}
			goodsChannels[goodsNo] = append(goodsChannels[goodsNo], ChannelSales{ShopName: shopName, Sales: sales, Qty: qty})
		}
		if writeDatabaseError(w, chRows.Err()) {
			return
		}
	}

	// 4. 品牌分布
	brandArgs := append([]interface{}{dept, start, end}, extraArgs...)
	brandRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT IFNULL(brand_name,'未知') as brand,
			ROUND(SUM(local_goods_amt), 2) as sales
		FROM sales_goods_summary
		WHERE department = ? AND stat_date BETWEEN ? AND ?`+shopCond+platCond+scopeCond+`
		GROUP BY brand_name ORDER BY sales DESC LIMIT 10`, brandArgs...)
	if !ok {
		return
	}
	defer brandRows.Close()

	type BrandData struct {
		Brand string  `json:"brand"`
		Sales float64 `json:"sales"`
	}
	var brands []BrandData
	for brandRows.Next() {
		var b BrandData
		if writeDatabaseError(w, brandRows.Scan(&b.Brand, &b.Sales)) {
			return
		}
		brands = append(brands, b)
	}
	if writeDatabaseError(w, brandRows.Err()) {
		return
	}

	// 4.5 产品定位分布
	type GradeData struct {
		Grade string  `json:"grade"`
		Sales float64 `json:"sales"`
	}
	var grades []GradeData
	gradeArgs := append([]interface{}{dept, start, end}, extraArgs...)
	gradeRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT IFNULL(g.goods_field7,'未设置') as grade,
			ROUND(SUM(s.local_goods_amt), 2) as sales
		FROM sales_goods_summary s
		LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
		WHERE s.department = ? AND s.stat_date BETWEEN ? AND ?`+strings.ReplaceAll(shopCond+platCond+scopeCond, "shop_name", "s.shop_name")+`
		GROUP BY g.goods_field7
		ORDER BY FIELD(g.goods_field7,'S','A','B','C','D'), sales DESC`, gradeArgs...)
	if !ok {
		return
	}
	defer gradeRows.Close()
	for gradeRows.Next() {
		var gd GradeData
		if writeDatabaseError(w, gradeRows.Scan(&gd.Grade, &gd.Sales)) {
			return
		}
		grades = append(grades, gd)
	}
	if writeDatabaseError(w, gradeRows.Err()) {
		return
	}

	// 4.6 产品定位×平台销售分布（电商+社媒部门，平台维度通过 sales_channel.online_plat_name）
	type GradePlatItem struct {
		Grade    string  `json:"grade"`
		Platform string  `json:"platform"`
		Sales    float64 `json:"sales"`
	}
	var gradePlatSales []GradePlatItem
	if dept == "ecommerce" || dept == "social" {
		gpRows, ok := queryRowsOrWriteError(w, h.DB, `
			SELECT IFNULL(g.goods_field7,'未设置') as grade,
			CASE
				WHEN s.shop_name LIKE '%即时零售%' THEN '即时零售'
				WHEN sc.online_plat_name IS NULL OR sc.online_plat_name = '' THEN '其他'
				ELSE sc.online_plat_name
			END AS plat,
			ROUND(SUM(s.local_goods_amt),2) as sales
			FROM sales_goods_summary s
			LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
			LEFT JOIN sales_channel sc ON s.shop_name = sc.channel_name AND sc.department = s.department
			WHERE s.department = ? AND s.stat_date BETWEEN ? AND ?`+scopeCond+`
			GROUP BY grade, plat
			ORDER BY FIELD(grade,'S','A','B','C','D'), sales DESC`, dept, start, end)
		if !ok {
			return
		}
		defer gpRows.Close()
		platLabelMap := map[string]string{
			"天猫商城": "天猫", "天猫超市": "天猫超市", "京东": "京东",
			"拼多多": "拼多多", "唯品会MP": "唯品会", "唯品会JIT": "唯品会",
			"抖音超市": "即时零售", "淘宝": "淘宝",
			"放心购（抖音小店）": "抖音", "快手小店": "快手",
			"小红书": "小红书", "有赞": "有赞", "微店": "微店",
			"微信视频号小店": "视频号",
		}
		gpKey := func(grade, plat string) string { return grade + "|" + plat }
		gpMap := map[string]*GradePlatItem{}
		for gpRows.Next() {
			var grade, rawPlat string
			var sales float64
			if writeDatabaseError(w, gpRows.Scan(&grade, &rawPlat, &sales)) {
				return
			}
			label := rawPlat
			if l, ok := platLabelMap[rawPlat]; ok {
				label = l
			}
			key := gpKey(grade, label)
			if ps, ok := gpMap[key]; ok {
				ps.Sales += sales
			} else {
				gpMap[key] = &GradePlatItem{Grade: grade, Platform: label, Sales: sales}
			}
		}
		if writeDatabaseError(w, gpRows.Err()) {
			return
		}
		for _, ps := range gpMap {
			gradePlatSales = append(gradePlatSales, *ps)
		}
		sort.Slice(gradePlatSales, func(i, j int) bool {
			gradeOrder := map[string]int{"S": 0, "A": 1, "B": 2, "C": 3, "D": 4, "未设置": 5}
			gi, gj := gradeOrder[gradePlatSales[i].Grade], gradeOrder[gradePlatSales[j].Grade]
			if gi != gj {
				return gi < gj
			}
			return gradePlatSales[i].Sales > gradePlatSales[j].Sales
		})
	} else if dept == "offline" || dept == "distribution" {
		// 线下：按大区合并；分销：保留原 shop_name
		var gpSQL string
		if dept == "offline" {
			gpSQL = `SELECT IFNULL(g.goods_field7,'未设置') as grade,
				` + offlineRegionExpr + ` as channel,
				ROUND(SUM(s.local_goods_amt),2) as sales
				FROM sales_goods_summary s
				LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
				WHERE s.department = ? AND s.stat_date BETWEEN ? AND ?` + scopeCond + `
				GROUP BY grade, channel HAVING channel IS NOT NULL
				ORDER BY FIELD(grade,'S','A','B','C','D'), sales DESC`
		} else {
			gpSQL = `SELECT IFNULL(g.goods_field7,'未设置') as grade, s.shop_name as channel,
				ROUND(SUM(s.local_goods_amt),2) as sales
				FROM sales_goods_summary s
				LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
				WHERE s.department = ? AND s.stat_date BETWEEN ? AND ?` + scopeCond + `
				GROUP BY grade, s.shop_name
				ORDER BY FIELD(grade,'S','A','B','C','D'), sales DESC`
		}
		gpRows, ok := queryRowsOrWriteError(w, h.DB, gpSQL, dept, start, end)
		if !ok {
			return
		}
		defer gpRows.Close()
		for gpRows.Next() {
			var grade, channel string
			var sales float64
			if writeDatabaseError(w, gpRows.Scan(&grade, &channel, &sales)) {
				return
			}
			gradePlatSales = append(gradePlatSales, GradePlatItem{Grade: grade, Platform: channel, Sales: sales})
		}
		if writeDatabaseError(w, gpRows.Err()) {
			return
		}
	}

	// 5. 平台列表（合并后，只返回有销售数据的）
	// 先查有数据的原始平台名
	platRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DISTINCT sc.online_plat_name
		FROM sales_channel sc
		INNER JOIN sales_goods_summary s ON s.shop_name = sc.channel_name
		WHERE sc.department = ? AND sc.online_plat_name IS NOT NULL AND sc.online_plat_name != ''
		  AND s.stat_date BETWEEN ? AND ? AND s.department = ?`, dept, start, end, dept)
	if !ok {
		return
	}
	defer platRows.Close()
	rawPlats := map[string]bool{}
	for platRows.Next() {
		var p string
		if writeDatabaseError(w, platRows.Scan(&p)) {
			return
		}
		rawPlats[p] = true
	}
	if writeDatabaseError(w, platRows.Err()) {
		return
	}

	// 即时零售特殊检查：按店铺名匹配
	var instantCount int
	if writeDatabaseError(w, h.DB.QueryRow(`SELECT COUNT(DISTINCT shop_name) FROM sales_goods_summary
		WHERE shop_name LIKE '%即时零售%' AND stat_date BETWEEN ? AND ?
		AND shop_name IN (SELECT channel_name FROM sales_channel WHERE department = ?)
		AND department = ?`, start, end, dept, dept).Scan(&instantCount)) {
		return
	}

	// 按合并规则生成平台Tab列表
	type PlatTab struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	}
	platTabDefs := []PlatTab{
		{"tmall", "天猫"}, {"tmall_cs", "天猫超市"}, {"jd", "京东"}, {"pdd", "拼多多"},
		{"vip", "唯品会"}, {"taobao", "淘宝"}, {"instant", "即时零售"},
		{"douyin", "抖音"}, {"kuaishou", "快手"}, {"xiaohongshu", "小红书"},
		{"youzan", "有赞"}, {"weidian", "微店"}, {"shipinhao", "视频号"},
	}
	var platforms []PlatTab
	for _, pt := range platTabDefs {
		if !isPlatformAllowedForDept(dept, pt.Key) {
			continue
		}
		if pt.Key == "instant" {
			if instantCount > 0 {
				platforms = append(platforms, pt)
			}
			continue
		}
		plats := platformToPlats[pt.Key]
		hasData := false
		for _, p := range plats {
			if rawPlats[p] {
				hasData = true
				break
			}
		}
		if hasData {
			platforms = append(platforms, pt)
		}
	}

	// 6. 平台销售额分布
	type PlatSales struct {
		Platform string  `json:"platform"`
		Sales    float64 `json:"sales"`
		Qty      float64 `json:"qty"`
	}
	var platformSales []PlatSales
	platSalesRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT CASE
			WHEN s.shop_name LIKE '%即时零售%' THEN '即时零售'
			WHEN sc.online_plat_name IS NULL OR sc.online_plat_name = '' THEN '其他'
			ELSE sc.online_plat_name
		END AS plat,
		ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0)
		FROM sales_goods_summary s
		LEFT JOIN sales_channel sc ON s.shop_name = sc.channel_name AND sc.department = s.department
		WHERE s.department = ? AND s.stat_date BETWEEN ? AND ?`+scopeCond+`
		GROUP BY plat
		ORDER BY SUM(s.local_goods_amt) DESC`, dept, start, end)
	if !ok {
		return
	}
	defer platSalesRows.Close()
	platSalesMap := map[string]*PlatSales{}
	for platSalesRows.Next() {
		var rawPlat string
		var sales, qty float64
		if writeDatabaseError(w, platSalesRows.Scan(&rawPlat, &sales, &qty)) {
			return
		}
		// 合并平台：通过 platformToPlats 反查分组
		label := rawPlat
		for key, plats := range platformToPlats {
			for _, p := range plats {
				if p == rawPlat {
					for _, pt := range platTabDefs {
						if pt.Key == key {
							label = pt.Label
							break
						}
					}
					break
				}
			}
		}
		if ps, ok := platSalesMap[label]; ok {
			ps.Sales += sales
			ps.Qty += qty
		} else {
			platSalesMap[label] = &PlatSales{Platform: label, Sales: sales, Qty: qty}
		}
	}
	if writeDatabaseError(w, platSalesRows.Err()) {
		return
	}
	for _, ps := range platSalesMap {
		platformSales = append(platformSales, *ps)
	}
	// 按销售额降序
	sort.Slice(platformSales, func(i, j int) bool {
		return platformSales[i].Sales > platformSales[j].Sales
	})

	writeJSON(w, map[string]interface{}{
		"daily":           daily,
		"shops":           shops,
		"goods":           goods,
		"goodsChannels":   goodsChannels,
		"brands":          brands,
		"grades":          grades,
		"platforms":       platforms,
		"platformSales":   platformSales,
		"gradePlatSales":  gradePlatSales,
		"dateRange":       map[string]string{"start": start, "end": end},
		"trendRange":      map[string]string{"start": trendStart, "end": trendEnd},
	})
}

// GetTmallOps 天猫运营数据（流量转化/推广投放/会员复购）
func (h *DashboardHandler) GetTmallOps(w http.ResponseWriter, r *http.Request) {
	shop := r.URL.Query().Get("shop")
	if shop == "" {
		writeError(w, 400, "shop is required")
		return
	}
	if writeScopeError(w, requireDomainAccess(r, "ops")) || writeScopeError(w, requirePlatformAccess(r, "tmall")) || writeScopeError(w, requireShopAccess(r, shop)) {
		return
	}
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getOverviewTrendRange(r, start, end)
	if trendStart == start && trendEnd == end {
		// 兼容未传 trendStart/trendEnd 的旧前端：仅单日查询时扩展趋势
		if start == end {
			if eDate, err := time.Parse("2006-01-02", end); err == nil {
				trendStart = eDate.AddDate(0, 0, -13).Format("2006-01-02")
				trendEnd = end
			}
		}
	}

	// 1. 店铺流量转化日趋势 (生意参谋)
	type TrafficDaily struct {
		Date        string  `json:"date"`
		Visitors    int     `json:"visitors"`
		PageViews   int     `json:"pageViews"`
		CartBuyers  int     `json:"cartBuyers"`
		PayBuyers   int     `json:"payBuyers"`
		PayAmount   float64 `json:"payAmount"`
		PayConvRate float64 `json:"payConvRate"`
		UvValue     float64 `json:"uvValue"`
		BounceRate  float64 `json:"bounceRate"`
	}
	var traffic []TrafficDaily
	tRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'), visitors, page_views,
			cart_buyers, pay_buyers, pay_amount, pay_conv_rate, uv_value, bounce_rate
		FROM op_tmall_shop_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer tRows.Close()
	for tRows.Next() {
		var t TrafficDaily
		if writeDatabaseError(w, tRows.Scan(&t.Date, &t.Visitors, &t.PageViews, &t.CartBuyers,
			&t.PayBuyers, &t.PayAmount, &t.PayConvRate, &t.UvValue, &t.BounceRate)) {
			return
		}
		traffic = append(traffic, t)
	}
	if writeDatabaseError(w, tRows.Err()) {
		return
	}

	// 2. CPC推广汇总(万象台) - 按天汇总所有场景
	type CampaignDaily struct {
		Date      string  `json:"date"`
		Cost      float64 `json:"cost"`
		PayAmount float64 `json:"payAmount"`
		ROI       float64 `json:"roi"`
		Clicks    int     `json:"clicks"`
		Impr      int     `json:"impressions"`
	}
	var campaigns []CampaignDaily
	cRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			ROUND(SUM(cost),2), ROUND(SUM(total_pay_amount),2),
			CASE WHEN SUM(cost)>0 THEN ROUND(SUM(total_pay_amount)/SUM(cost),2) ELSE 0 END,
			SUM(clicks), SUM(impressions)
		FROM op_tmall_campaign_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY stat_date ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer cRows.Close()
	for cRows.Next() {
		var c CampaignDaily
		if writeDatabaseError(w, cRows.Scan(&c.Date, &c.Cost, &c.PayAmount, &c.ROI, &c.Clicks, &c.Impr)) {
			return
		}
		campaigns = append(campaigns, c)
	}
	if writeDatabaseError(w, cRows.Err()) {
		return
	}

	// 3. CPC场景分布(万象台) - 按场景汇总(用原始范围)
	type SceneSummary struct {
		SceneName string  `json:"sceneName"`
		Cost      float64 `json:"cost"`
		PayAmount float64 `json:"payAmount"`
		ROI       float64 `json:"roi"`
		Clicks    int     `json:"clicks"`
	}
	var scenes []SceneSummary
	sRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT scene_name, ROUND(SUM(cost),2), ROUND(SUM(total_pay_amount),2),
			CASE WHEN SUM(cost)>0 THEN ROUND(SUM(total_pay_amount)/SUM(cost),2) ELSE 0 END,
			SUM(clicks)
		FROM op_tmall_campaign_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY scene_name ORDER BY SUM(cost) DESC`, shop, start, end)
	if !ok {
		return
	}
	defer sRows.Close()
	for sRows.Next() {
		var s SceneSummary
		if writeDatabaseError(w, sRows.Scan(&s.SceneName, &s.Cost, &s.PayAmount, &s.ROI, &s.Clicks)) {
			return
		}
		scenes = append(scenes, s)
	}
	if writeDatabaseError(w, sRows.Err()) {
		return
	}

	// 4. CPS推广(淘宝联盟) - 按天汇总
	type CPSDaily struct {
		Date          string  `json:"date"`
		PayAmount     float64 `json:"payAmount"`
		PayCommission float64 `json:"payCommission"`
		PayUsers      int     `json:"payUsers"`
	}
	var cps []CPSDaily
	cpsRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			ROUND(SUM(settle_amount),2), ROUND(SUM(settle_total_cost),2), SUM(pay_users)
		FROM op_tmall_cps_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY stat_date ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer cpsRows.Close()
	for cpsRows.Next() {
		var c CPSDaily
		if writeDatabaseError(w, cpsRows.Scan(&c.Date, &c.PayAmount, &c.PayCommission, &c.PayUsers)) {
			return
		}
		cps = append(cps, c)
	}
	if writeDatabaseError(w, cpsRows.Err()) {
		return
	}

	// 5. CPS按计划分布
	type CPSPlan struct {
		PlanName      string  `json:"planName"`
		PayAmount     float64 `json:"payAmount"`
		PayCommission float64 `json:"payCommission"`
		PayUsers      int     `json:"payUsers"`
	}
	var cpsPlans []CPSPlan
	cpRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT plan_name, ROUND(SUM(settle_amount),2), ROUND(SUM(settle_total_cost),2), SUM(pay_users)
		FROM op_tmall_cps_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY plan_name ORDER BY SUM(settle_amount) DESC`, shop, start, end)
	if !ok {
		return
	}
	defer cpRows.Close()
	for cpRows.Next() {
		var c CPSPlan
		if writeDatabaseError(w, cpRows.Scan(&c.PlanName, &c.PayAmount, &c.PayCommission, &c.PayUsers)) {
			return
		}
		cpsPlans = append(cpsPlans, c)
	}
	if writeDatabaseError(w, cpRows.Err()) {
		return
	}

	// 6. 会员数据
	type MemberDaily struct {
		Date            string  `json:"date"`
		PaidMemberCnt   int     `json:"paidMemberCnt"`
		MemberPayAmt    float64 `json:"memberPayAmt"`
		MemberUnitPrice float64 `json:"memberUnitPrice"`
		TotalMemberCnt  int     `json:"totalMemberCnt"`
		RepurchaseRate  float64 `json:"repurchaseRate"`
	}
	var members []MemberDaily
	mRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			paid_member_cnt, member_pay_amount, member_unit_price,
			total_member_cnt, repurchase_rate
		FROM op_tmall_member_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer mRows.Close()
	for mRows.Next() {
		var m MemberDaily
		if writeDatabaseError(w, mRows.Scan(&m.Date, &m.PaidMemberCnt, &m.MemberPayAmt,
			&m.MemberUnitPrice, &m.TotalMemberCnt, &m.RepurchaseRate)) {
			return
		}
		members = append(members, m)
	}
	if writeDatabaseError(w, mRows.Err()) {
		return
	}

	// 7. 商品TOP10 (生意参谋-商品销售)
	type GoodsItem struct {
		ProductName string  `json:"productName"`
		Visitors    int     `json:"visitors"`
		CartBuyers  int     `json:"cartBuyers"`
		PayQty      int     `json:"payQty"`
		PayAmount   float64 `json:"payAmount"`
		PayConvRate string  `json:"payConvRate"`
		PayBuyers   int     `json:"payBuyers"`
		RefundAmt   float64 `json:"refundAmount"`
	}
	var goodsTop []GoodsItem
	gRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT product_name, SUM(visitors), SUM(cart_buyers), SUM(pay_qty),
			ROUND(SUM(pay_amount),2),
			CASE WHEN SUM(visitors)>0 THEN CONCAT(ROUND(SUM(pay_buyers)/SUM(visitors)*100,2),'%%') ELSE '0%%' END,
			SUM(pay_buyers), ROUND(SUM(refund_amount),2)
		FROM op_tmall_goods_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY product_name ORDER BY SUM(pay_amount) DESC LIMIT 10`, shop, start, end)
	if !ok {
		return
	}
	defer gRows.Close()
	for gRows.Next() {
		var g GoodsItem
		if writeDatabaseError(w, gRows.Scan(&g.ProductName, &g.Visitors, &g.CartBuyers, &g.PayQty, &g.PayAmount, &g.PayConvRate, &g.PayBuyers, &g.RefundAmt)) {
			return
		}
		goodsTop = append(goodsTop, g)
	}

	// 8. 品牌数据趋势 (数据银行)
	type BrandDaily struct {
		Date           string  `json:"date"`
		MemberPayAmt   float64 `json:"memberPayAmt"`
		CustomerVolume int     `json:"customerVolume"`
		LoyalVolume    int     `json:"loyalVolume"`
		InterestVolume int     `json:"interestVolume"`
		PurchaseVolume int     `json:"purchaseVolume"`
		DeepenRatio    float64 `json:"deepenRatio"`
	}
	var brandDaily []BrandDaily
	bRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'), member_pay_amount,
			customer_volume, loyal_volume, interest_volume, purchase_volume, deepen_ratio
		FROM op_tmall_brand_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer bRows.Close()
	for bRows.Next() {
		var b BrandDaily
		if writeDatabaseError(w, bRows.Scan(&b.Date, &b.MemberPayAmt, &b.CustomerVolume, &b.LoyalVolume, &b.InterestVolume, &b.PurchaseVolume, &b.DeepenRatio)) {
			return
		}
		brandDaily = append(brandDaily, b)
	}

	// 9. 人群覆盖趋势 (达摩盘)
	type CrowdDaily struct {
		Date        string  `json:"date"`
		Coverage    int     `json:"coverage"`
		Concentrate float64 `json:"concentrate"`
		PayAmount   float64 `json:"payAmount"`
		PayUV       int     `json:"payUV"`
	}
	var crowdDaily []CrowdDaily
	crRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'), coverage,
			ta_concentrate_ratio, shop_alipay_amount, shop_alipay_uv
		FROM op_tmall_crowd_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer crRows.Close()
	for crRows.Next() {
		var c CrowdDaily
		if writeDatabaseError(w, crRows.Scan(&c.Date, &c.Coverage, &c.Concentrate, &c.PayAmount, &c.PayUV)) {
			return
		}
		crowdDaily = append(crowdDaily, c)
	}

	// 10. 行业月报 (集客)
	type IndustryMonthly struct {
		Month           string  `json:"month"`
		Category        string  `json:"category"`
		NewRatio        float64 `json:"newRatio"`
		NewSalesRatio   float64 `json:"newSalesRatio"`
		NewRepurchase30 float64 `json:"newRepurchase30d"`
		OldRatio        float64 `json:"oldRatio"`
		OldSalesRatio   float64 `json:"oldSalesRatio"`
		OldRepurchase30 float64 `json:"oldRepurchase30d"`
		UnitPrice       float64 `json:"unitPrice"`
	}
	var industry []IndustryMonthly
	iRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT stat_month, IFNULL(category,''), new_ratio, new_sales_ratio, new_repurchase_30d,
			old_ratio, old_sales_ratio, old_repurchase_30d, unit_price
		FROM op_tmall_industry_monthly
		WHERE shop_name = ? ORDER BY stat_month DESC LIMIT 12`, shop)
	if !ok {
		return
	}
	defer iRows.Close()
	for iRows.Next() {
		var i IndustryMonthly
		if writeDatabaseError(w, iRows.Scan(&i.Month, &i.Category, &i.NewRatio, &i.NewSalesRatio, &i.NewRepurchase30, &i.OldRatio, &i.OldSalesRatio, &i.OldRepurchase30, &i.UnitPrice)) {
			return
		}
		industry = append(industry, i)
	}

	// 11. 复购月报 (集客)
	type RepurchaseMonthly struct {
		Month              string  `json:"month"`
		Category           string  `json:"category"`
		ShopRepurchase30   float64 `json:"shopRepurchase30d"`
		ShopRepurchase180  float64 `json:"shopRepurchase180d"`
		ShopRepurchase360  float64 `json:"shopRepurchase360d"`
		LostRepurchase     float64 `json:"lostRepurchaseRate"`
		LastRepurchaseDays float64 `json:"lastRepurchaseDays"`
	}
	var repurchase []RepurchaseMonthly
	rRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT stat_month, IFNULL(category,''), shop_repurchase_30d, shop_repurchase_180d,
			shop_repurchase_360d, lost_repurchase_rate, last_repurchase_days
		FROM op_tmall_repurchase_monthly
		WHERE shop_name = ? ORDER BY stat_month DESC LIMIT 12`, shop)
	if !ok {
		return
	}
	defer rRows.Close()
	for rRows.Next() {
		var rp RepurchaseMonthly
		if writeDatabaseError(w, rRows.Scan(&rp.Month, &rp.Category, &rp.ShopRepurchase30, &rp.ShopRepurchase180, &rp.ShopRepurchase360, &rp.LostRepurchase, &rp.LastRepurchaseDays)) {
			return
		}
		repurchase = append(repurchase, rp)
	}

	writeJSON(w, map[string]interface{}{
		"traffic":    traffic,
		"campaigns":  campaigns,
		"scenes":     scenes,
		"cps":        cps,
		"cpsPlans":   cpsPlans,
		"members":    members,
		"goodsTop":   goodsTop,
		"brandDaily": brandDaily,
		"crowdDaily": crowdDaily,
		"industry":   industry,
		"repurchase": repurchase,
	})
}

// GetTmallcsOps 天猫超市运营数据
// 包含：经营概况(每日)、智多星推广(已在campaign_daily)、行业热词、市场品牌排名
func (h *DashboardHandler) GetTmallcsOps(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDomainAccess(r, "ops")) || writeScopeError(w, requirePlatformAccess(r, "tmall_cs")) {
		return
	}
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)

	// 1. 经营概况日趋势
	type BusinessDaily struct {
		Date             string  `json:"date"`
		PayAmount        float64 `json:"payAmount"`
		SubOrderAvgPrice float64 `json:"subOrderAvgPrice"`
		AvgPrice         float64 `json:"avgPrice"`
		IpvUv            int     `json:"ipvUv"`
		PaySubOrders     int     `json:"paySubOrders"`
		PayQty           int     `json:"payQty"`
		ConvRate         float64 `json:"convRate"`
		PayUsers         int     `json:"payUsers"`
	}
	var business []BusinessDaily
	bRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			pay_amount, sub_order_avg_price, avg_price, ipv_uv,
			pay_sub_orders, pay_qty, conv_rate, pay_users
		FROM op_tmall_cs_shop_daily
		WHERE stat_date BETWEEN ? AND ?
		ORDER BY stat_date`, trendStart, trendEnd)
	if !ok {
		return
	}
	defer bRows.Close()
	for bRows.Next() {
		var b BusinessDaily
		if writeDatabaseError(w, bRows.Scan(&b.Date, &b.PayAmount, &b.SubOrderAvgPrice, &b.AvgPrice, &b.IpvUv,
			&b.PaySubOrders, &b.PayQty, &b.ConvRate, &b.PayUsers)) {
			return
		}
		business = append(business, b)
	}

	// 2. 推广汇总(智多星/无界场景/淘客) - 按天
	type CampaignDaily struct {
		Date      string  `json:"date"`
		PromoType string  `json:"promoType"`
		Cost      float64 `json:"cost"`
		PayAmount float64 `json:"payAmount"`
		ROI       float64 `json:"roi"`
		Clicks    int     `json:"clicks"`
		Impr      int     `json:"impressions"`
	}
	var campaigns []CampaignDaily
	cRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'), promo_type,
			ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
			CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
			SUM(clicks), SUM(impressions)
		FROM op_tmall_cs_campaign_daily
		WHERE stat_date BETWEEN ? AND ?
		GROUP BY stat_date, promo_type ORDER BY stat_date`, trendStart, trendEnd)
	if !ok {
		return
	}
	defer cRows.Close()
	for cRows.Next() {
		var c CampaignDaily
		if writeDatabaseError(w, cRows.Scan(&c.Date, &c.PromoType, &c.Cost, &c.PayAmount, &c.ROI, &c.Clicks, &c.Impr)) {
			return
		}
		campaigns = append(campaigns, c)
	}

	// 3. 行业热词TOP30 (按搜索曝光热度)
	type IndustryKeyword struct {
		Keyword          string  `json:"keyword"`
		SearchImpression float64 `json:"searchImpression"`
		TradeHeat        float64 `json:"tradeHeat"`
		TradeScale       float64 `json:"tradeScale"`
		ConvIndex        float64 `json:"convIndex"`
		VisitHeat        float64 `json:"visitHeat"`
	}
	var keywords []IndustryKeyword
	kRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT keyword, ROUND(AVG(search_impression),2), ROUND(AVG(trade_heat),2),
			ROUND(AVG(trade_scale),2), ROUND(AVG(conv_index),2), ROUND(AVG(visit_heat),2)
		FROM op_tmall_cs_industry_keyword
		WHERE stat_date BETWEEN ? AND ? AND dimension='day' AND channel='整体'
		GROUP BY keyword
		ORDER BY AVG(search_impression) DESC LIMIT 30`, start, end)
	if !ok {
		return
	}
	defer kRows.Close()
	for kRows.Next() {
		var k IndustryKeyword
		if writeDatabaseError(w, kRows.Scan(&k.Keyword, &k.SearchImpression, &k.TradeHeat,
			&k.TradeScale, &k.ConvIndex, &k.VisitHeat)) {
			return
		}
		keywords = append(keywords, k)
	}

	// 4. 市场品牌排名 - 按品类返回
	type MarketRank struct {
		Category        string  `json:"category"`
		BrandName       string  `json:"brandName"`
		TradeHeat       float64 `json:"tradeHeat"`
		TradePopularity float64 `json:"tradePopularity"`
		VisitHeat       float64 `json:"visitHeat"`
		ConvIndex       float64 `json:"convIndex"`
		TradeIndex      float64 `json:"tradeIndex"`
	}
	var ranks []MarketRank
	rRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT category, brand_name,
			ROUND(AVG(trade_heat),2), ROUND(AVG(trade_popularity),2),
			ROUND(AVG(visit_heat),2), ROUND(AVG(conv_index),2), ROUND(AVG(trade_index),2)
		FROM op_tmall_cs_market_rank
		WHERE stat_date BETWEEN ? AND ?
		GROUP BY category, brand_name
		ORDER BY category, AVG(trade_heat) DESC`, start, end)
	if !ok {
		return
	}
	defer rRows.Close()
	for rRows.Next() {
		var r MarketRank
		if writeDatabaseError(w, rRows.Scan(&r.Category, &r.BrandName, &r.TradeHeat, &r.TradePopularity,
			&r.VisitHeat, &r.ConvIndex, &r.TradeIndex)) {
			return
		}
		ranks = append(ranks, r)
	}

	writeJSON(w, map[string]interface{}{
		"business":  business,
		"campaigns": campaigns,
		"keywords":  keywords,
		"ranks":     ranks,
	})
}

// GetSProducts S品渠道销售分析
// 参数: dept=ecommerce (按部门过滤)
func (h *DashboardHandler) GetSProducts(w http.ResponseWriter, r *http.Request) {
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)
	dept := r.URL.Query().Get("dept")
	platform := r.URL.Query().Get("platform")
	deptCond := ""
	var deptArgs []interface{}
	if dept != "" && dept != "all" {
		deptCond = " AND s.department = ?"
		deptArgs = append(deptArgs, dept)
	}
	// 店铺过滤（优先）
	shop := r.URL.Query().Get("shop")
	if shop != "" && shop != "all" {
		deptCond += " AND s.shop_name = ?"
		deptArgs = append(deptArgs, shop)
	} else if platform != "" && platform != "all" {
		// 平台过滤：根据平台code匹配shop_name前缀
		platPrefixMap := map[string]string{
			"tmall": "ds-天猫-", "tmall_cs": "ds-天猫超市-", "jd": "ds-京东-",
			"pdd": "ds-拼多多-", "vip": "ds-唯品会-",
		}
		if prefix, ok := platPrefixMap[platform]; ok {
			deptCond += " AND s.shop_name LIKE ?"
			deptArgs = append(deptArgs, prefix+"%")
		}
	}

	// 1. S品渠道销售排行（全部平台时按平台汇总，选了平台时按店铺）
	type ShopSales struct {
		ShopName   string  `json:"shopName"`
		Department string  `json:"department"`
		Sales      float64 `json:"sales"`
		Qty        float64 `json:"qty"`
	}
	var shopRank []ShopSales
	args1 := append([]interface{}{start, end}, deptArgs...)
	groupByPlatform := platform == "" || platform == "all"
	var shopSQL string
	if groupByPlatform {
		// 按平台汇总：从shop_name提取平台名（ds-天猫-xxx → 天猫）
		shopSQL = `
		SELECT
			CASE
				WHEN s.shop_name LIKE 'ds-天猫超市%' THEN '天猫超市'
				WHEN s.shop_name LIKE 'ds-天猫-%' THEN '天猫'
				WHEN s.shop_name LIKE 'ds-京东-%' THEN '京东'
				WHEN s.shop_name LIKE 'ds-拼多多-%' THEN '拼多多'
				WHEN s.shop_name LIKE 'ds-唯品会-%' THEN '唯品会'
				WHEN s.shop_name LIKE 'js-%' THEN '即时零售'
				ELSE '其他'
			END as platform_name,
			s.department,
			ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0)
		FROM sales_goods_summary s
		JOIN goods g ON g.goods_no = s.goods_no AND g.goods_field7 = 'S'
		WHERE s.stat_date BETWEEN ? AND ?
		  AND s.shop_name IS NOT NULL AND s.shop_name != ''` + deptCond + `
		GROUP BY platform_name, s.department
		ORDER BY SUM(s.local_goods_amt) DESC`
	} else {
		shopSQL = `
		SELECT s.shop_name, s.department, ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0)
		FROM sales_goods_summary s
		JOIN goods g ON g.goods_no = s.goods_no AND g.goods_field7 = 'S'
		WHERE s.stat_date BETWEEN ? AND ?
		  AND s.shop_name IS NOT NULL AND s.shop_name != ''` + deptCond + `
		GROUP BY s.shop_name, s.department
		ORDER BY SUM(s.local_goods_amt) DESC LIMIT 20`
	}
	sRows, ok := queryRowsOrWriteError(w, h.DB, shopSQL, args1...)
	if !ok {
		return
	}
	defer sRows.Close()
	for sRows.Next() {
		var s ShopSales
		if writeDatabaseError(w, sRows.Scan(&s.ShopName, &s.Department, &s.Sales, &s.Qty)) {
			return
		}
		shopRank = append(shopRank, s)
	}

	// 2. S品单品销售排行
	type GoodsSales struct {
		GoodsNo   string  `json:"goodsNo"`
		GoodsName string  `json:"goodsName"`
		Sales     float64 `json:"sales"`
		Qty       float64 `json:"qty"`
		ShopCount int     `json:"shopCount"`
	}
	var goodsRank []GoodsSales
	args2 := append([]interface{}{start, end}, deptArgs...)
	var goodsSQL string
	if groupByPlatform {
		goodsSQL = `
		SELECT s.goods_no, g.goods_name, ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0),
			COUNT(DISTINCT CASE
				WHEN s.shop_name LIKE 'ds-天猫超市%' THEN '天猫超市'
				WHEN s.shop_name LIKE 'ds-天猫-%' THEN '天猫'
				WHEN s.shop_name LIKE 'ds-京东-%' THEN '京东'
				WHEN s.shop_name LIKE 'ds-拼多多-%' THEN '拼多多'
				WHEN s.shop_name LIKE 'ds-唯品会-%' THEN '唯品会'
				WHEN s.shop_name LIKE 'js-%' THEN '即时零售'
				ELSE '其他'
			END)
		FROM sales_goods_summary s
		JOIN goods g ON g.goods_no = s.goods_no AND g.goods_field7 = 'S'
		WHERE s.stat_date BETWEEN ? AND ?` + deptCond + `
		GROUP BY s.goods_no, g.goods_name
		ORDER BY SUM(s.local_goods_amt) DESC`
	} else {
		goodsSQL = `
		SELECT s.goods_no, g.goods_name, ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0),
			COUNT(DISTINCT s.shop_name)
		FROM sales_goods_summary s
		JOIN goods g ON g.goods_no = s.goods_no AND g.goods_field7 = 'S'
		WHERE s.stat_date BETWEEN ? AND ?` + deptCond + `
		GROUP BY s.goods_no, g.goods_name
		ORDER BY SUM(s.local_goods_amt) DESC`
	}
	gRows, ok := queryRowsOrWriteError(w, h.DB, goodsSQL, args2...)
	if !ok {
		return
	}
	defer gRows.Close()
	for gRows.Next() {
		var g GoodsSales
		if writeDatabaseError(w, gRows.Scan(&g.GoodsNo, &g.GoodsName, &g.Sales, &g.Qty, &g.ShopCount)) {
			return
		}
		goodsRank = append(goodsRank, g)
	}

	// 3. S品每日销售趋势
	type DailyTrend struct {
		Date  string  `json:"date"`
		Sales float64 `json:"sales"`
		Qty   float64 `json:"qty"`
	}
	var trend []DailyTrend
	args3 := append([]interface{}{trendStart, trendEnd}, deptArgs...)
	tRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(s.stat_date,'%Y-%m-%d'), ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0)
		FROM sales_goods_summary s
		JOIN goods g ON g.goods_no = s.goods_no AND g.goods_field7 = 'S'
		WHERE s.stat_date BETWEEN ? AND ?`+deptCond+`
		GROUP BY s.stat_date ORDER BY s.stat_date`, args3...)
	if !ok {
		return
	}
	defer tRows.Close()
	for tRows.Next() {
		var t DailyTrend
		if writeDatabaseError(w, tRows.Scan(&t.Date, &t.Sales, &t.Qty)) {
			return
		}
		trend = append(trend, t)
	}

	// 4. S品各单品明细（全部平台时按平台汇总，选了平台时按店铺）
	type GoodsShopDetail struct {
		GoodsName string  `json:"goodsName"`
		ShopName  string  `json:"shopName"`
		Sales     float64 `json:"sales"`
		Qty       float64 `json:"qty"`
	}
	var details []GoodsShopDetail
	args4 := append([]interface{}{start, end}, deptArgs...)
	var detailSQL string
	if groupByPlatform {
		detailSQL = `
		SELECT g.goods_name,
			CASE
				WHEN s.shop_name LIKE 'ds-天猫超市%' THEN '天猫超市'
				WHEN s.shop_name LIKE 'ds-天猫-%' THEN '天猫'
				WHEN s.shop_name LIKE 'ds-京东-%' THEN '京东'
				WHEN s.shop_name LIKE 'ds-拼多多-%' THEN '拼多多'
				WHEN s.shop_name LIKE 'ds-唯品会-%' THEN '唯品会'
				WHEN s.shop_name LIKE 'js-%' THEN '即时零售'
				ELSE '其他'
			END as platform_name,
			ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0)
		FROM sales_goods_summary s
		JOIN goods g ON g.goods_no = s.goods_no AND g.goods_field7 = 'S'
		WHERE s.stat_date BETWEEN ? AND ?
		  AND s.shop_name IS NOT NULL AND s.shop_name != ''` + deptCond + `
		GROUP BY g.goods_name, platform_name
		HAVING SUM(s.local_goods_amt) > 0
		ORDER BY g.goods_name, SUM(s.local_goods_amt) DESC`
	} else {
		detailSQL = `
		SELECT g.goods_name, s.shop_name, ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0)
		FROM sales_goods_summary s
		JOIN goods g ON g.goods_no = s.goods_no AND g.goods_field7 = 'S'
		WHERE s.stat_date BETWEEN ? AND ?
		  AND s.shop_name IS NOT NULL AND s.shop_name != ''` + deptCond + `
		GROUP BY g.goods_name, s.shop_name
		HAVING SUM(s.local_goods_amt) > 0
		ORDER BY g.goods_name, SUM(s.local_goods_amt) DESC`
	}
	dRows, ok := queryRowsOrWriteError(w, h.DB, detailSQL, args4...)
	if !ok {
		return
	}
	defer dRows.Close()
	for dRows.Next() {
		var d GoodsShopDetail
		if writeDatabaseError(w, dRows.Scan(&d.GoodsName, &d.ShopName, &d.Sales, &d.Qty)) {
			return
		}
		details = append(details, d)
	}

	writeJSON(w, map[string]interface{}{
		"shopRank":  shopRank,
		"goodsRank": goodsRank,
		"trend":     trend,
		"details":   details,
	})
}

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

// GetPddOps 拼多多运营数据（店铺经营+商品数据+短视频）
func (h *DashboardHandler) GetPddOps(w http.ResponseWriter, r *http.Request) {
	shop := r.URL.Query().Get("shop")
	if shop == "" {
		writeError(w, 400, "shop is required")
		return
	}
	if writeScopeError(w, requireDomainAccess(r, "ops")) || writeScopeError(w, requirePlatformAccess(r, "pdd")) || writeScopeError(w, requireShopAccess(r, shop)) {
		return
	}
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)

	// 店铺经营数据
	type ShopDaily struct {
		Date         string  `json:"date"`
		PayAmount    float64 `json:"payAmount"`
		PayCount     int     `json:"payCount"`
		PayOrders    int     `json:"payOrders"`
		ConvRate     float64 `json:"convRate"`
		UnitPrice    float64 `json:"unitPrice"`
		PayOrdersPct float64 `json:"payOrdersPct"`
	}
	var shopDaily []ShopDaily
	rows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			IFNULL(SUM(pay_amount),0), IFNULL(SUM(pay_count),0), IFNULL(SUM(pay_orders),0),
			IFNULL(AVG(conv_rate),0), IFNULL(AVG(unit_price),0), IFNULL(AVG(pay_orders_pct),0)
		FROM op_pdd_shop_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY stat_date ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var d ShopDaily
		if writeDatabaseError(w, rows.Scan(&d.Date, &d.PayAmount, &d.PayCount, &d.PayOrders, &d.ConvRate, &d.UnitPrice, &d.PayOrdersPct)) {
			return
		}
		shopDaily = append(shopDaily, d)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}

	// 商品数据
	type GoodsDaily struct {
		Date           string  `json:"date"`
		GoodsVisitors  int     `json:"goodsVisitors"`
		GoodsViews     int     `json:"goodsViews"`
		GoodsCollect   int     `json:"goodsCollect"`
		SaleGoodsCount int     `json:"saleGoodsCount"`
		PayAmount      float64 `json:"payAmount"`
		PayCount       int     `json:"payCount"`
	}
	var goodsDaily []GoodsDaily
	rows2, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			IFNULL(SUM(goods_visitors),0), IFNULL(SUM(goods_views),0), IFNULL(SUM(goods_collect),0),
			IFNULL(SUM(sale_goods_count),0), IFNULL(SUM(pay_amount),0), IFNULL(SUM(pay_count),0)
		FROM op_pdd_goods_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY stat_date ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var d GoodsDaily
		if writeDatabaseError(w, rows2.Scan(&d.Date, &d.GoodsVisitors, &d.GoodsViews, &d.GoodsCollect, &d.SaleGoodsCount, &d.PayAmount, &d.PayCount)) {
			return
		}
		goodsDaily = append(goodsDaily, d)
	}
	if writeDatabaseError(w, rows2.Err()) {
		return
	}

	// 短视频数据
	type VideoDaily struct {
		Date          string  `json:"date"`
		TotalGmv      float64 `json:"totalGmv"`
		OrderCount    int     `json:"orderCount"`
		OrderUv       int     `json:"orderUv"`
		FeedCount     int     `json:"feedCount"`
		VideoViewCnt  int     `json:"videoViewCnt"`
		GoodsClickCnt int     `json:"goodsClickCnt"`
	}
	var videoDaily []VideoDaily
	rows3, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			IFNULL(SUM(total_gmv),0), IFNULL(SUM(order_count),0), IFNULL(SUM(order_uv),0),
			IFNULL(SUM(feed_count),0), IFNULL(SUM(video_view_cnt),0), IFNULL(SUM(goods_click_cnt),0)
		FROM op_pdd_video_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY stat_date ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer rows3.Close()
	for rows3.Next() {
		var d VideoDaily
		if writeDatabaseError(w, rows3.Scan(&d.Date, &d.TotalGmv, &d.OrderCount, &d.OrderUv, &d.FeedCount, &d.VideoViewCnt, &d.GoodsClickCnt)) {
			return
		}
		videoDaily = append(videoDaily, d)
	}
	if writeDatabaseError(w, rows3.Err()) {
		return
	}

	writeJSON(w, map[string]interface{}{
		"shop":      shopDaily,
		"goods":     goodsDaily,
		"video":     videoDaily,
		"dateRange": map[string]string{"start": start, "end": end},
	})
}

// GetJdOps 京东运营数据（店铺经营+客户分析）
func (h *DashboardHandler) GetJdOps(w http.ResponseWriter, r *http.Request) {
	shop := r.URL.Query().Get("shop")
	if shop == "" {
		writeError(w, 400, "shop is required")
		return
	}
	if writeScopeError(w, requireDomainAccess(r, "ops")) || writeScopeError(w, requirePlatformAccess(r, "jd")) || writeScopeError(w, requireShopAccess(r, shop)) {
		return
	}
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)

	// 店铺经营数据
	type ShopDaily struct {
		Date         string  `json:"date"`
		Visitors     int     `json:"visitors"`
		PageViews    int     `json:"pageViews"`
		PayCustomers int     `json:"payCustomers"`
		PayAmount    float64 `json:"payAmount"`
		PayCount     int     `json:"payCount"`
		PayOrders    int     `json:"payOrders"`
		UnitPrice    float64 `json:"unitPrice"`
		ConvRate     float64 `json:"convRate"`
		UvValue      float64 `json:"uvValue"`
		BounceRate   float64 `json:"bounceRate"`
		RefundAmount float64 `json:"refundAmount"`
	}
	var shopDaily []ShopDaily
	rows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			IFNULL(SUM(visitors),0), IFNULL(SUM(page_views),0),
			IFNULL(SUM(pay_customers),0), IFNULL(SUM(pay_amount),0),
			IFNULL(SUM(pay_count),0), IFNULL(SUM(pay_orders),0),
			IFNULL(AVG(unit_price),0), IFNULL(AVG(conv_rate),0),
			IFNULL(AVG(uv_value),0), IFNULL(AVG(bounce_rate),0),
			IFNULL(SUM(refund_amount),0)
		FROM op_jd_shop_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY stat_date ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var d ShopDaily
		if writeDatabaseError(w, rows.Scan(&d.Date, &d.Visitors, &d.PageViews, &d.PayCustomers, &d.PayAmount,
			&d.PayCount, &d.PayOrders, &d.UnitPrice, &d.ConvRate, &d.UvValue, &d.BounceRate, &d.RefundAmount)) {
			return
		}
		shopDaily = append(shopDaily, d)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}

	// 客户分析数据
	type CustomerDaily struct {
		Date                string `json:"date"`
		BrowseCustomers     int    `json:"browseCustomers"`
		CartCustomers       int    `json:"cartCustomers"`
		OrderCustomers      int    `json:"orderCustomers"`
		PayCustomers        int    `json:"payCustomers"`
		RepurchaseCustomers int    `json:"repurchaseCustomers"`
		LostCustomers       int    `json:"lostCustomers"`
	}
	var customerDaily []CustomerDaily
	rows2, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			IFNULL(SUM(browse_customers),0), IFNULL(SUM(cart_customers),0),
			IFNULL(SUM(order_customers),0), IFNULL(SUM(pay_customers),0),
			IFNULL(SUM(repurchase_customers),0), IFNULL(SUM(lost_customers),0)
		FROM op_jd_customer_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY stat_date ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var d CustomerDaily
		if writeDatabaseError(w, rows2.Scan(&d.Date, &d.BrowseCustomers, &d.CartCustomers, &d.OrderCustomers,
			&d.PayCustomers, &d.RepurchaseCustomers, &d.LostCustomers)) {
			return
		}
		customerDaily = append(customerDaily, d)
	}
	if writeDatabaseError(w, rows2.Err()) {
		return
	}

	// 3. 新老客分析
	type CustomerType struct {
		Date         string  `json:"date"`
		CustomerType string  `json:"customerType"`
		PayCustomers int     `json:"payCustomers"`
		PayPct       float64 `json:"payPct"`
		ConvRate     float64 `json:"convRate"`
		UnitPrice    float64 `json:"unitPrice"`
	}
	var customerTypes []CustomerType
	ctRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'), customer_type, pay_customers,
			pay_pct, conv_rate, unit_price
		FROM op_jd_customer_type_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		ORDER BY stat_date, customer_type`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer ctRows.Close()
	for ctRows.Next() {
		var c CustomerType
		if writeDatabaseError(w, ctRows.Scan(&c.Date, &c.CustomerType, &c.PayCustomers, &c.PayPct, &c.ConvRate, &c.UnitPrice)) {
			return
		}
		customerTypes = append(customerTypes, c)
	}

	// 4. 行业热词TOP20
	type IndustryKeyword struct {
		Keyword        string `json:"keyword"`
		SearchRank     string `json:"searchRank"`
		CompeteRank    string `json:"competeRank"`
		ClickRank      string `json:"clickRank"`
		PayAmountRange string `json:"payAmountRange"`
		TopBrand       string `json:"topBrand"`
	}
	var keywords []IndustryKeyword
	kwRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT keyword, search_rank, compete_rank, click_rank,
			IFNULL(pay_amount_range,''), IFNULL(top_brand,'')
		FROM op_jd_industry_keyword
		WHERE shop_name = ? AND stat_date = (
			SELECT MAX(stat_date) FROM op_jd_industry_keyword WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		) LIMIT 20`, shop, shop, start, end)
	if !ok {
		return
	}
	defer kwRows.Close()
	for kwRows.Next() {
		var k IndustryKeyword
		if writeDatabaseError(w, kwRows.Scan(&k.Keyword, &k.SearchRank, &k.CompeteRank, &k.ClickRank, &k.PayAmountRange, &k.TopBrand)) {
			return
		}
		keywords = append(keywords, k)
	}

	// 5. 促销活动汇总
	type PromoSummary struct {
		PromoType string  `json:"promoType"`
		PayAmount float64 `json:"payAmount"`
		PayUsers  int     `json:"payUsers"`
		PayCount  int     `json:"payCount"`
		ConvRate  float64 `json:"convRate"`
		UV        int     `json:"uv"`
	}
	var promos []PromoSummary
	pRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT promo_type, ROUND(SUM(pay_amount),2), SUM(pay_users), SUM(pay_count),
			CASE WHEN SUM(uv)>0 THEN ROUND(SUM(pay_users)/SUM(uv)*100,2) ELSE 0 END, SUM(uv)
		FROM op_jd_promo_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY promo_type ORDER BY SUM(pay_amount) DESC`, shop, start, end)
	if !ok {
		return
	}
	defer pRows.Close()
	for pRows.Next() {
		var p PromoSummary
		if writeDatabaseError(w, pRows.Scan(&p.PromoType, &p.PayAmount, &p.PayUsers, &p.PayCount, &p.ConvRate, &p.UV)) {
			return
		}
		promos = append(promos, p)
	}

	// 6. 促销商品TOP10
	type PromoSku struct {
		GoodsName string  `json:"goodsName"`
		PromoType string  `json:"promoType"`
		UV        int     `json:"uv"`
		PayAmount float64 `json:"payAmount"`
		PayUsers  int     `json:"payUsers"`
		PayCount  int     `json:"payCount"`
	}
	var promoSkus []PromoSku
	psRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT goods_name, promo_type, SUM(uv), ROUND(SUM(pay_amount),2), SUM(pay_users), SUM(pay_count)
		FROM op_jd_promo_sku_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		GROUP BY goods_name, promo_type ORDER BY SUM(pay_amount) DESC LIMIT 10`, shop, start, end)
	if !ok {
		return
	}
	defer psRows.Close()
	for psRows.Next() {
		var p PromoSku
		if writeDatabaseError(w, psRows.Scan(&p.GoodsName, &p.PromoType, &p.UV, &p.PayAmount, &p.PayUsers, &p.PayCount)) {
			return
		}
		promoSkus = append(promoSkus, p)
	}

	writeJSON(w, map[string]interface{}{
		"shop":          shopDaily,
		"customer":      customerDaily,
		"customerTypes": customerTypes,
		"keywords":      keywords,
		"promos":        promos,
		"promoSkus":     promoSkus,
		"dateRange":     map[string]string{"start": start, "end": end},
	})
}

// GetVipOps 唯品会运营数据（流量转化）
func (h *DashboardHandler) GetVipOps(w http.ResponseWriter, r *http.Request) {
	shop := r.URL.Query().Get("shop")
	if shop == "" {
		writeError(w, 400, "shop is required")
		return
	}
	if writeScopeError(w, requireDomainAccess(r, "ops")) || writeScopeError(w, requirePlatformAccess(r, "vip")) || writeScopeError(w, requireShopAccess(r, shop)) {
		return
	}
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)

	type VipDaily struct {
		Date          string  `json:"date"`
		Impressions   int     `json:"impressions"`
		PageViews     int     `json:"pageViews"`
		DetailUV      int     `json:"detailUv"`
		DetailUVValue float64 `json:"detailUvValue"`
		CartBuyers    int     `json:"cartBuyers"`
		CollectBuyers int     `json:"collectBuyers"`
		PayAmount     float64 `json:"payAmount"`
		PayCount      int     `json:"payCount"`
		PayOrders     int     `json:"payOrders"`
		Visitors      int     `json:"visitors"`
		ARPU          float64 `json:"arpu"`
		CartConvRate  string  `json:"cartConvRate"`
		PayConvRate   string  `json:"payConvRate"`
		CancelAmount  float64 `json:"cancelAmount"`
	}
	var daily []VipDaily
	rows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
			impressions, page_views, detail_uv, detail_uv_value,
			cart_buyers, collect_buyers,
			pay_amount, pay_count, pay_orders, visitors, arpu,
			IFNULL(cart_conv_rate,''), IFNULL(pay_conv_rate,''), cancel_amount
		FROM op_vip_shop_daily
		WHERE shop_name = ? AND stat_date BETWEEN ? AND ?
		ORDER BY stat_date`, shop, trendStart, trendEnd)
	if !ok {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var d VipDaily
		if writeDatabaseError(w, rows.Scan(&d.Date, &d.Impressions, &d.PageViews, &d.DetailUV, &d.DetailUVValue,
			&d.CartBuyers, &d.CollectBuyers,
			&d.PayAmount, &d.PayCount, &d.PayOrders, &d.Visitors, &d.ARPU,
			&d.CartConvRate, &d.PayConvRate, &d.CancelAmount)) {
			return
		}
		daily = append(daily, d)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}

	writeJSON(w, map[string]interface{}{
		"daily":     daily,
		"dateRange": map[string]string{"start": start, "end": end},
	})
}

// GetChannels 渠道管理列表
func (h *DashboardHandler) GetChannels(w http.ResponseWriter, r *http.Request) {
	dept := r.URL.Query().Get("dept")
	if dept == "" {
		writeError(w, 400, "dept is required")
		return
	}
	scopeCond, scopeArgs, err := buildSalesDataScopeCond(r, dept, "", "")
	if writeScopeError(w, err) {
		return
	}

	// 渠道类型名映射
	typeNames := map[string]string{
		"0": "分销办公室", "1": "直营网店", "2": "直营门店", "3": "销售办公室",
		"4": "货主虚拟店", "5": "分销虚拟店", "6": "加盟门店", "7": "内部交易渠道",
	}

	// 平台分组映射（原始平台名 -> 分组key + 分组label）
	platGroupMap := map[string][2]string{
		"天猫商城": {"tmall", "天猫"}, "天猫超市": {"tmall_cs", "天猫超市"},
		"京东": {"jd", "京东"}, "拼多多": {"pdd", "拼多多"},
		"唯品会MP": {"vip", "唯品会"}, "唯品会JIT": {"vip", "唯品会"},
		"淘宝": {"taobao", "淘宝"}, "抖音超市": {"instant", "即时零售"},
	}

	rows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT sc.channel_id, sc.channel_name, IFNULL(sc.online_plat_name,'') as plat,
			IFNULL(sc.channel_type,'') as ctype, IFNULL(sc.channel_depart_name,'') as depart,
			IFNULL(SUM(s.local_goods_amt),0) as total_sales, IFNULL(SUM(s.goods_qty),0) as total_qty
		FROM sales_channel sc
		LEFT JOIN sales_goods_summary s ON s.shop_name = sc.channel_name`+strings.ReplaceAll(strings.ReplaceAll(scopeCond, "department", "s.department"), "shop_name", "s.shop_name")+`
		WHERE sc.department = ?
		GROUP BY sc.channel_id, sc.channel_name, sc.online_plat_name, sc.channel_type, sc.channel_depart_name
		ORDER BY total_sales DESC`, append([]interface{}{dept}, scopeArgs...)...)
	if !ok {
		return
	}
	defer rows.Close()

	type Channel struct {
		ChannelId      string  `json:"channelId"`
		ChannelName    string  `json:"channelName"`
		PlatName       string  `json:"platName"`
		PlatGroup      string  `json:"platGroup"`
		PlatGroupLabel string  `json:"platGroupLabel"`
		ChannelType    string  `json:"channelType"`
		DepartName     string  `json:"departName"`
		TotalSales     float64 `json:"totalSales"`
		TotalQty       float64 `json:"totalQty"`
	}
	var channels []Channel
	for rows.Next() {
		var ch Channel
		var ctype string
		if writeDatabaseError(w, rows.Scan(&ch.ChannelId, &ch.ChannelName, &ch.PlatName, &ctype, &ch.DepartName, &ch.TotalSales, &ch.TotalQty)) {
			return
		}
		ch.ChannelType = typeNames[ctype]
		if ch.ChannelType == "" {
			ch.ChannelType = ctype
		}

		// 平台分组
		if pg, ok := platGroupMap[ch.PlatName]; ok {
			ch.PlatGroup = pg[0]
			ch.PlatGroupLabel = pg[1]
		} else if ch.ChannelName != "" && len(ch.ChannelName) > 3 && ch.ChannelName[:3] == "js-" {
			ch.PlatGroup = "instant"
			ch.PlatGroupLabel = "即时零售"
		} else {
			ch.PlatGroup = "other"
			ch.PlatGroupLabel = "其他"
		}

		channels = append(channels, ch)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}

	writeJSON(w, map[string]interface{}{"channels": channels})
}

// GetMarketingCost 营销费用（多平台CPC+CPS跨店铺汇总）
// 参数: platform=all|tmall|jd|pdd|tmall_cs, shop=店铺名(可选)
func (h *DashboardHandler) GetMarketingCost(w http.ResponseWriter, r *http.Request) {
	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)
	shop := r.URL.Query().Get("shop")
	platform := r.URL.Query().Get("platform")
	if platform == "" {
		platform = "all"
	}
	if writeScopeError(w, requireDomainAccess(r, "ops", "finance")) || writeScopeError(w, requireShopAccess(r, shop)) {
		return
	}

	payload, _ := authPayloadFromContext(r)
	allowedPlatforms := []string{}
	allowedShops := []string{}
	if payload != nil && !payload.IsSuperAdmin {
		allowedPlatforms = payload.DataScopes.Platforms
		allowedShops = payload.DataScopes.Shops
	}

	shopCond := ""
	var shopArgs []interface{}
	if shop != "" && shop != "all" {
		shopCond = " AND shop_name = ?"
		shopArgs = []interface{}{shop}
	}
	if shopCond == "" && len(allowedShops) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(allowedShops)), ",")
		shopCond = " AND shop_name IN (" + placeholders + ")"
		for _, allowedShop := range allowedShops {
			shopArgs = append(shopArgs, allowedShop)
		}
	}

	// 通用结构(CpcDaily/CpsDaily 用包级别定义)
	type ShopCost struct {
		ShopName  string  `json:"shopName"`
		Cost      float64 `json:"cost"`
		PayAmount float64 `json:"payAmount"`
		ROI       float64 `json:"roi"`
		Clicks    int     `json:"clicks"`
	}
	type DetailRow struct {
		Platform  string  `json:"platform"`
		Name      string  `json:"name"`
		Cost      float64 `json:"cost"`
		PayAmount float64 `json:"payAmount"`
		ROI       float64 `json:"roi"`
		Clicks    int     `json:"clicks"`
		AvgCpc    float64 `json:"avgCpc"`
	}

	var cpcDaily []CpcDaily
	var cpsDaily []CpsDaily
	var shopCosts []ShopCost
	var details []DetailRow
	var shops []string
	hasCps := false

	// 根据平台查不同的表
	queryPlatforms := []string{platform}
	if platform == "all" {
		queryPlatforms = []string{"tmall", "jd", "pdd", "tmall_cs"}
	}
	if len(allowedPlatforms) > 0 {
		filtered := []string{}
		for _, plat := range queryPlatforms {
			if containsString(allowedPlatforms, plat) {
				filtered = append(filtered, plat)
			}
		}
		if platform != "all" && len(filtered) == 0 {
			writeError(w, http.StatusForbidden, "forbidden by data scope")
			return
		}
		queryPlatforms = filtered
	}
	if len(queryPlatforms) == 0 {
		writeError(w, http.StatusForbidden, "forbidden by data scope")
		return
	}

	// 趋势用扩展范围，汇总用原始范围
	trendShopArgs := append([]interface{}{trendStart, trendEnd}, shopArgs...)
	_ = trendShopArgs // 预定义，下面按需用

	for _, plat := range queryPlatforms {
		switch plat {
		case "tmall":
			// 天猫CPC(万象台)
			args := append([]interface{}{trendStart, trendEnd}, shopArgs...)
			rows, ok := queryRowsOrWriteError(w, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
				ROUND(SUM(cost),2), ROUND(SUM(total_pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(total_pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks), SUM(impressions)
				FROM op_tmall_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY stat_date ORDER BY stat_date`, args...)
			if !ok {
				return
			}
			if rows != nil {
				for rows.Next() {
					var c CpcDaily
					if writeDatabaseError(w, rows.Scan(&c.Date, &c.Cost, &c.PayAmount, &c.ROI, &c.Clicks, &c.Impr)) {
						rows.Close()
						return
					}
					cpcDaily = mergeCpcDaily(cpcDaily, c)
				}
				if writeDatabaseError(w, rows.Err()) {
					rows.Close()
					return
				}
				rows.Close()
			}
			// 天猫CPS(淘宝联盟)
			hasCps = true
			args2 := append([]interface{}{trendStart, trendEnd}, shopArgs...)
			rows2, ok := queryRowsOrWriteError(w, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
				ROUND(SUM(settle_amount),2), ROUND(SUM(settle_total_cost),2), SUM(pay_users)
				FROM op_tmall_cps_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY stat_date ORDER BY stat_date`, args2...)
			if !ok {
				return
			}
			if rows2 != nil {
				for rows2.Next() {
					var c CpsDaily
					if writeDatabaseError(w, rows2.Scan(&c.Date, &c.PayAmount, &c.PayCommission, &c.PayUsers)) {
						rows2.Close()
						return
					}
					cpsDaily = mergeCpsDaily(cpsDaily, c)
				}
				if writeDatabaseError(w, rows2.Err()) {
					rows2.Close()
					return
				}
				rows2.Close()
			}
			// 天猫店铺CPC
			sArgs := append([]interface{}{start, end}, shopArgs...)
			sRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT shop_name, ROUND(SUM(cost),2), ROUND(SUM(total_pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(total_pay_amount)/SUM(cost),2) ELSE 0 END, SUM(clicks)
				FROM op_tmall_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY shop_name ORDER BY SUM(cost) DESC`, sArgs...)
			if !ok {
				return
			}
			if sRows != nil {
				for sRows.Next() {
					var s ShopCost
					if writeDatabaseError(w, sRows.Scan(&s.ShopName, &s.Cost, &s.PayAmount, &s.ROI, &s.Clicks)) {
						sRows.Close()
						return
					}
					shopCosts = append(shopCosts, s)
				}
				if writeDatabaseError(w, sRows.Err()) {
					sRows.Close()
					return
				}
				sRows.Close()
			}
			// 天猫场景明细
			dArgs := append([]interface{}{start, end}, shopArgs...)
			dRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT scene_name, ROUND(SUM(cost),2), ROUND(SUM(total_pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(total_pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks),
				CASE WHEN SUM(clicks)>0 THEN ROUND(SUM(cost)/SUM(clicks),2) ELSE 0 END
				FROM op_tmall_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY scene_name ORDER BY SUM(cost) DESC`, dArgs...)
			if !ok {
				return
			}
			if dRows != nil {
				for dRows.Next() {
					var d DetailRow
					d.Platform = "天猫"
					if writeDatabaseError(w, dRows.Scan(&d.Name, &d.Cost, &d.PayAmount, &d.ROI, &d.Clicks, &d.AvgCpc)) {
						dRows.Close()
						return
					}
					details = append(details, d)
				}
				if writeDatabaseError(w, dRows.Err()) {
					dRows.Close()
					return
				}
				dRows.Close()
			}

		case "jd":
			// 京东CPC(京准通)
			args := append([]interface{}{trendStart, trendEnd}, shopArgs...)
			rows, ok := queryRowsOrWriteError(w, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
				ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks), SUM(impressions)
				FROM op_jd_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY stat_date ORDER BY stat_date`, args...)
			if !ok {
				return
			}
			if rows != nil {
				for rows.Next() {
					var c CpcDaily
					if writeDatabaseError(w, rows.Scan(&c.Date, &c.Cost, &c.PayAmount, &c.ROI, &c.Clicks, &c.Impr)) {
						rows.Close()
						return
					}
					cpcDaily = mergeCpcDaily(cpcDaily, c)
				}
				if writeDatabaseError(w, rows.Err()) {
					rows.Close()
					return
				}
				rows.Close()
			}
			// 京东CPS(京东联盟)
			hasCps = true
			args2 := append([]interface{}{trendStart, trendEnd}, shopArgs...)
			rows2, ok := queryRowsOrWriteError(w, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
				ROUND(SUM(complete_amount),2), ROUND(SUM(actual_commission),2), SUM(complete_buyers)
				FROM op_jd_affiliate_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY stat_date ORDER BY stat_date`, args2...)
			if !ok {
				return
			}
			if rows2 != nil {
				for rows2.Next() {
					var c CpsDaily
					if writeDatabaseError(w, rows2.Scan(&c.Date, &c.PayAmount, &c.PayCommission, &c.PayUsers)) {
						rows2.Close()
						return
					}
					cpsDaily = mergeCpsDaily(cpsDaily, c)
				}
				if writeDatabaseError(w, rows2.Err()) {
					rows2.Close()
					return
				}
				rows2.Close()
			}
			// 京东店铺CPC
			sArgs := append([]interface{}{start, end}, shopArgs...)
			sRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT shop_name, ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END, SUM(clicks)
				FROM op_jd_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY shop_name ORDER BY SUM(cost) DESC`, sArgs...)
			if !ok {
				return
			}
			if sRows != nil {
				for sRows.Next() {
					var s ShopCost
					if writeDatabaseError(w, sRows.Scan(&s.ShopName, &s.Cost, &s.PayAmount, &s.ROI, &s.Clicks)) {
						sRows.Close()
						return
					}
					shopCosts = append(shopCosts, s)
				}
				if writeDatabaseError(w, sRows.Err()) {
					sRows.Close()
					return
				}
				sRows.Close()
			}
			// 京东推广类型明细
			dArgs := append([]interface{}{start, end}, shopArgs...)
			dRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT promo_type, ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks),
				CASE WHEN SUM(clicks)>0 THEN ROUND(SUM(cost)/SUM(clicks),2) ELSE 0 END
				FROM op_jd_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY promo_type ORDER BY SUM(cost) DESC`, dArgs...)
			if !ok {
				return
			}
			if dRows != nil {
				for dRows.Next() {
					var d DetailRow
					d.Platform = "京东"
					if writeDatabaseError(w, dRows.Scan(&d.Name, &d.Cost, &d.PayAmount, &d.ROI, &d.Clicks, &d.AvgCpc)) {
						dRows.Close()
						return
					}
					details = append(details, d)
				}
				if writeDatabaseError(w, dRows.Err()) {
					dRows.Close()
					return
				}
				dRows.Close()
			}

		case "pdd":
			// 拼多多CPC
			args := append([]interface{}{trendStart, trendEnd}, shopArgs...)
			rows, ok := queryRowsOrWriteError(w, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
				ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks), SUM(impressions)
				FROM op_pdd_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY stat_date ORDER BY stat_date`, args...)
			if !ok {
				return
			}
			if rows != nil {
				for rows.Next() {
					var c CpcDaily
					if writeDatabaseError(w, rows.Scan(&c.Date, &c.Cost, &c.PayAmount, &c.ROI, &c.Clicks, &c.Impr)) {
						rows.Close()
						return
					}
					cpcDaily = mergeCpcDaily(cpcDaily, c)
				}
				if writeDatabaseError(w, rows.Err()) {
					rows.Close()
					return
				}
				rows.Close()
			}
			// 拼多多店铺CPC
			sArgs := append([]interface{}{start, end}, shopArgs...)
			sRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT shop_name, ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END, SUM(clicks)
				FROM op_pdd_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY shop_name ORDER BY SUM(cost) DESC`, sArgs...)
			if !ok {
				return
			}
			if sRows != nil {
				for sRows.Next() {
					var s ShopCost
					if writeDatabaseError(w, sRows.Scan(&s.ShopName, &s.Cost, &s.PayAmount, &s.ROI, &s.Clicks)) {
						sRows.Close()
						return
					}
					shopCosts = append(shopCosts, s)
				}
				if writeDatabaseError(w, sRows.Err()) {
					sRows.Close()
					return
				}
				sRows.Close()
			}
			// 拼多多推广类型明细
			dArgs := append([]interface{}{start, end}, shopArgs...)
			dRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT promo_type, ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks),
				CASE WHEN SUM(clicks)>0 THEN ROUND(SUM(cost)/SUM(clicks),2) ELSE 0 END
				FROM op_pdd_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY promo_type ORDER BY SUM(cost) DESC`, dArgs...)
			if !ok {
				return
			}
			if dRows != nil {
				for dRows.Next() {
					var d DetailRow
					d.Platform = "拼多多"
					if writeDatabaseError(w, dRows.Scan(&d.Name, &d.Cost, &d.PayAmount, &d.ROI, &d.Clicks, &d.AvgCpc)) {
						dRows.Close()
						return
					}
					details = append(details, d)
				}
				if writeDatabaseError(w, dRows.Err()) {
					dRows.Close()
					return
				}
				dRows.Close()
			}

		case "tmall_cs":
			// 天猫超市CPC
			args := append([]interface{}{trendStart, trendEnd}, shopArgs...)
			rows, ok := queryRowsOrWriteError(w, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
				ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks), SUM(impressions)
				FROM op_tmall_cs_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY stat_date ORDER BY stat_date`, args...)
			if !ok {
				return
			}
			if rows != nil {
				for rows.Next() {
					var c CpcDaily
					if writeDatabaseError(w, rows.Scan(&c.Date, &c.Cost, &c.PayAmount, &c.ROI, &c.Clicks, &c.Impr)) {
						rows.Close()
						return
					}
					cpcDaily = mergeCpcDaily(cpcDaily, c)
				}
				if writeDatabaseError(w, rows.Err()) {
					rows.Close()
					return
				}
				rows.Close()
			}
			// 天猫超市店铺CPC(一盘货/寄售双店对比)
			sArgs := append([]interface{}{start, end}, shopArgs...)
			sRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT shop_name, ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END, SUM(clicks)
				FROM op_tmall_cs_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY shop_name ORDER BY SUM(cost) DESC`, sArgs...)
			if !ok {
				return
			}
			if sRows != nil {
				for sRows.Next() {
					var s ShopCost
					if writeDatabaseError(w, sRows.Scan(&s.ShopName, &s.Cost, &s.PayAmount, &s.ROI, &s.Clicks)) {
						sRows.Close()
						return
					}
					shopCosts = append(shopCosts, s)
				}
				if writeDatabaseError(w, sRows.Err()) {
					sRows.Close()
					return
				}
				sRows.Close()
			}
			// 天猫超市推广类型明细
			dArgs := append([]interface{}{start, end}, shopArgs...)
			dRows, ok := queryRowsOrWriteError(w, h.DB, `SELECT promo_type, ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks),
				CASE WHEN SUM(clicks)>0 THEN ROUND(SUM(cost)/SUM(clicks),2) ELSE 0 END
				FROM op_tmall_cs_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				GROUP BY promo_type ORDER BY SUM(cost) DESC`, dArgs...)
			if !ok {
				return
			}
			if dRows != nil {
				for dRows.Next() {
					var d DetailRow
					d.Platform = "天猫超市"
					if writeDatabaseError(w, dRows.Scan(&d.Name, &d.Cost, &d.PayAmount, &d.ROI, &d.Clicks, &d.AvgCpc)) {
						dRows.Close()
						return
					}
					details = append(details, d)
				}
				if writeDatabaseError(w, dRows.Err()) {
					dRows.Close()
					return
				}
				dRows.Close()
			}
		}
	}

	// 店铺列表(用于筛选，不受shop参数影响，但保留权限过滤)
	shopListCond := ""
	var shopListArgs []interface{}
	if len(allowedShops) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(allowedShops)), ",")
		shopListCond = " AND shop_name IN (" + placeholders + ")"
		for _, s := range allowedShops {
			shopListArgs = append(shopListArgs, s)
		}
	}
	shopQueries := []string{}
	shopQueryArgs := []interface{}{}
	for _, plat := range queryPlatforms {
		switch plat {
		case "tmall":
			shopQueries = append(shopQueries, "SELECT DISTINCT shop_name FROM op_tmall_campaign_daily WHERE stat_date BETWEEN ? AND ?"+shopListCond)
			shopQueryArgs = append(shopQueryArgs, start, end)
			shopQueryArgs = append(shopQueryArgs, shopListArgs...)
			shopQueries = append(shopQueries, "SELECT DISTINCT shop_name FROM op_tmall_cps_daily WHERE stat_date BETWEEN ? AND ?"+shopListCond)
			shopQueryArgs = append(shopQueryArgs, start, end)
			shopQueryArgs = append(shopQueryArgs, shopListArgs...)
		case "jd":
			shopQueries = append(shopQueries, "SELECT DISTINCT shop_name FROM op_jd_campaign_daily WHERE stat_date BETWEEN ? AND ?"+shopListCond)
			shopQueryArgs = append(shopQueryArgs, start, end)
			shopQueryArgs = append(shopQueryArgs, shopListArgs...)
			shopQueries = append(shopQueries, "SELECT DISTINCT shop_name FROM op_jd_affiliate_daily WHERE stat_date BETWEEN ? AND ?"+shopListCond)
			shopQueryArgs = append(shopQueryArgs, start, end)
			shopQueryArgs = append(shopQueryArgs, shopListArgs...)
		case "pdd":
			shopQueries = append(shopQueries, "SELECT DISTINCT shop_name FROM op_pdd_campaign_daily WHERE stat_date BETWEEN ? AND ?"+shopListCond)
			shopQueryArgs = append(shopQueryArgs, start, end)
			shopQueryArgs = append(shopQueryArgs, shopListArgs...)
		case "tmall_cs":
			shopQueries = append(shopQueries, "SELECT DISTINCT shop_name FROM op_tmall_cs_campaign_daily WHERE stat_date BETWEEN ? AND ?"+shopListCond)
			shopQueryArgs = append(shopQueryArgs, start, end)
			shopQueryArgs = append(shopQueryArgs, shopListArgs...)
		}
	}
	if len(shopQueries) > 0 {
		fullQuery := joinStrings(shopQueries, " UNION ") + " ORDER BY 1"
		sRows, ok := queryRowsOrWriteError(w, h.DB, fullQuery, shopQueryArgs...)
		if !ok {
			return
		}
		if sRows != nil {
			for sRows.Next() {
				var s string
				if writeDatabaseError(w, sRows.Scan(&s)) {
					sRows.Close()
					return
				}
				shops = append(shops, s)
			}
			if writeDatabaseError(w, sRows.Err()) {
				sRows.Close()
				return
			}
			sRows.Close()
		}
	}

	sort.Slice(cpcDaily, func(i, j int) bool { return cpcDaily[i].Date < cpcDaily[j].Date })
	sort.Slice(cpsDaily, func(i, j int) bool { return cpsDaily[i].Date < cpsDaily[j].Date })

	writeJSON(w, map[string]interface{}{
		"cpcDaily":   cpcDaily,
		"cpsDaily":   cpsDaily,
		"shopCosts":  shopCosts,
		"details":    details,
		"shops":      shops,
		"hasCps":     hasCps,
		"dateRange":  map[string]string{"start": start, "end": end},
		"trendRange": map[string]string{"start": trendStart, "end": trendEnd},
	})
}

// mergeCpcDaily 合并同日CPC数据(跨平台汇总时)
func mergeCpcDaily(arr []CpcDaily, item CpcDaily) []CpcDaily {
	for i, a := range arr {
		if a.Date == item.Date {
			arr[i].Cost += item.Cost
			arr[i].PayAmount += item.PayAmount
			arr[i].Clicks += item.Clicks
			arr[i].Impr += item.Impr
			if arr[i].Cost > 0 {
				arr[i].ROI = float64(int(arr[i].PayAmount/arr[i].Cost*100)) / 100
			}
			return arr
		}
	}
	return append(arr, item)
}

type CpcDaily struct {
	Date      string  `json:"date"`
	Cost      float64 `json:"cost"`
	PayAmount float64 `json:"payAmount"`
	ROI       float64 `json:"roi"`
	Clicks    int     `json:"clicks"`
	Impr      int64   `json:"impressions"`
}

// mergeCpsDaily 合并同日CPS数据
func mergeCpsDaily(arr []CpsDaily, item CpsDaily) []CpsDaily {
	for i, a := range arr {
		if a.Date == item.Date {
			arr[i].PayAmount += item.PayAmount
			arr[i].PayCommission += item.PayCommission
			arr[i].PayUsers += item.PayUsers
			return arr
		}
	}
	return append(arr, item)
}

type CpsDaily struct {
	Date          string  `json:"date"`
	PayAmount     float64 `json:"payAmount"`
	PayCommission float64 `json:"payCommission"`
	PayUsers      int     `json:"payUsers"`
}

type customerMetricRecord struct {
	Platform         string
	Date             string
	ShopName         string
	ConsultUsers     float64
	InquiryUsers     float64
	PayUsers         float64
	SalesAmount      float64
	FirstRespSeconds float64
	ResponseSeconds  float64
	SatisfactionRate float64
	ConvRate         float64
}

type customerMetricAgg struct {
	RecordCount       int
	ConsultUsers      float64
	InquiryUsers      float64
	PayUsers          float64
	SalesAmount       float64
	FirstRespSeconds  float64
	FirstRespCount    int
	ResponseSeconds   float64
	ResponseCount     int
	SatisfactionRate  float64
	SatisfactionCount int
	ConvRate          float64
	ConvCount         int
}

type customerPlatformStat struct {
	Platform            string  `json:"platform"`
	RecordCount         int     `json:"recordCount"`
	ShopCount           int     `json:"shopCount"`
	ConsultUsers        float64 `json:"consultUsers"`
	InquiryUsers        float64 `json:"inquiryUsers"`
	PayUsers            float64 `json:"payUsers"`
	SalesAmount         float64 `json:"salesAmount"`
	AvgFirstRespSeconds float64 `json:"avgFirstRespSeconds"`
	AvgResponseSeconds  float64 `json:"avgResponseSeconds"`
	AvgSatisfactionRate float64 `json:"avgSatisfactionRate"`
	AvgConvRate         float64 `json:"avgConvRate"`
}

type customerTrendPoint struct {
	Date                string  `json:"date"`
	ConsultUsers        float64 `json:"consultUsers"`
	InquiryUsers        float64 `json:"inquiryUsers"`
	PayUsers            float64 `json:"payUsers"`
	SalesAmount         float64 `json:"salesAmount"`
	AvgFirstRespSeconds float64 `json:"avgFirstRespSeconds"`
	AvgResponseSeconds  float64 `json:"avgResponseSeconds"`
	AvgSatisfactionRate float64 `json:"avgSatisfactionRate"`
	AvgConvRate         float64 `json:"avgConvRate"`
}

type customerShopStat struct {
	Platform            string  `json:"platform"`
	ShopName            string  `json:"shopName"`
	RecordCount         int     `json:"recordCount"`
	ConsultUsers        float64 `json:"consultUsers"`
	InquiryUsers        float64 `json:"inquiryUsers"`
	PayUsers            float64 `json:"payUsers"`
	SalesAmount         float64 `json:"salesAmount"`
	AvgFirstRespSeconds float64 `json:"avgFirstRespSeconds"`
	AvgResponseSeconds  float64 `json:"avgResponseSeconds"`
	AvgSatisfactionRate float64 `json:"avgSatisfactionRate"`
	AvgConvRate         float64 `json:"avgConvRate"`
}

func nullFloat(v sql.NullFloat64) float64 {
	if v.Valid {
		return v.Float64
	}
	return 0
}

func normalizeRate(v float64) float64 {
	if v <= 0 {
		return 0
	}
	if v <= 1 {
		return v * 100
	}
	return v
}

func roundFloat(v float64, digits int) float64 {
	if digits < 0 {
		return v
	}
	pow := math.Pow(10, float64(digits))
	return math.Round(v*pow) / pow
}

func (a *customerMetricAgg) add(rec customerMetricRecord) {
	a.RecordCount++
	a.ConsultUsers += rec.ConsultUsers
	a.InquiryUsers += rec.InquiryUsers
	a.PayUsers += rec.PayUsers
	a.SalesAmount += rec.SalesAmount
	if rec.FirstRespSeconds > 0 {
		a.FirstRespSeconds += rec.FirstRespSeconds
		a.FirstRespCount++
	}
	if rec.ResponseSeconds > 0 {
		a.ResponseSeconds += rec.ResponseSeconds
		a.ResponseCount++
	}
	if rec.SatisfactionRate > 0 {
		a.SatisfactionRate += normalizeRate(rec.SatisfactionRate)
		a.SatisfactionCount++
	}
	if rec.ConvRate > 0 {
		a.ConvRate += normalizeRate(rec.ConvRate)
		a.ConvCount++
	}
}

func (a *customerMetricAgg) avgFirstRespSeconds() float64 {
	if a.FirstRespCount == 0 {
		return 0
	}
	return a.FirstRespSeconds / float64(a.FirstRespCount)
}

func (a *customerMetricAgg) avgResponseSeconds() float64 {
	if a.ResponseCount == 0 {
		return 0
	}
	return a.ResponseSeconds / float64(a.ResponseCount)
}

func (a *customerMetricAgg) avgSatisfactionRate() float64 {
	if a.SatisfactionCount == 0 {
		return 0
	}
	return a.SatisfactionRate / float64(a.SatisfactionCount)
}

func (a *customerMetricAgg) avgConvRate() float64 {
	if a.ConvCount == 0 {
		return 0
	}
	return a.ConvRate / float64(a.ConvCount)
}

// GetCustomerOverview 客服总览（跨平台统一指标）
func (h *DashboardHandler) GetCustomerOverview(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDomainAccess(r, "ops")) {
		return
	}

	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)
	platformFilter := strings.TrimSpace(r.URL.Query().Get("platform"))
	args := []interface{}{
		trendStart, trendEnd, // tmall
		trendStart, trendEnd, // pdd
		trendStart, trendEnd, // jd
		trendStart, trendEnd, // douyin
		trendStart, trendEnd, // kuaishou
		trendStart, trendEnd, // xhs
		platformFilter,
		platformFilter,
	}

	rows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT platform, stat_date, shop_name, consult_users, inquiry_users, pay_users, sales_amount, first_response_seconds, response_seconds, satisfaction_rate, conv_rate
		FROM (
			SELECT
				'天猫' AS platform,
				DATE_FORMAT(tc.stat_date, '%Y-%m-%d') AS stat_date,
				tc.shop_name,
				IFNULL(tc.consult_users, 0) AS consult_users,
				IFNULL(ti.inquiry_users, 0) AS inquiry_users,
				IFNULL(ti.final_pay_users, 0) AS pay_users,
				IFNULL(ta.sales_amount, 0) AS sales_amount,
				IFNULL(tc.first_resp_sec, 0) AS first_response_seconds,
				IFNULL(tc.avg_response_sec, 0) AS response_seconds,
				IFNULL(te.total_satisfaction_rate, 0) AS satisfaction_rate,
				CASE
					WHEN IFNULL(ti.daily_conv_rate, 0) <= 1 THEN IFNULL(ti.daily_conv_rate, 0) * 100
					ELSE IFNULL(ti.daily_conv_rate, 0)
				END AS conv_rate
			FROM op_tmall_service_consult tc
			LEFT JOIN op_tmall_service_inquiry ti ON ti.stat_date = tc.stat_date AND ti.shop_name = tc.shop_name
			LEFT JOIN op_tmall_service_avgprice ta ON ta.stat_date = tc.stat_date AND ta.shop_name = tc.shop_name
			LEFT JOIN op_tmall_service_evaluation te ON te.stat_date = tc.stat_date AND te.shop_name = tc.shop_name
			WHERE tc.stat_date BETWEEN ? AND ?

			UNION ALL

			SELECT
				'拼多多' AS platform,
				DATE_FORMAT(pbase.stat_date, '%Y-%m-%d') AS stat_date,
				pbase.shop_name,
				IFNULL(ps.inquiry_users, 0) AS consult_users,
				IFNULL(ps.inquiry_users, 0) AS inquiry_users,
				IFNULL(ps.final_group_users, 0) AS pay_users,
				IFNULL(ps.cs_sales_amount, 0) AS sales_amount,
				0 AS first_response_seconds,
				0 AS response_seconds,
				CASE
					WHEN IFNULL(px.three_min_reply_rate_823, 0) <= 1 THEN IFNULL(px.three_min_reply_rate_823, 0) * 100
					ELSE IFNULL(px.three_min_reply_rate_823, 0)
				END AS satisfaction_rate,
				IFNULL(ps.inquiry_conv_rate, 0) AS conv_rate
			FROM (
				SELECT stat_date, shop_name FROM op_pdd_cs_service_daily
				UNION
				SELECT stat_date, shop_name FROM op_pdd_cs_sales_daily
			) pbase
			LEFT JOIN op_pdd_cs_service_daily px ON px.stat_date = pbase.stat_date AND px.shop_name = pbase.shop_name
			LEFT JOIN op_pdd_cs_sales_daily ps ON ps.stat_date = pbase.stat_date AND ps.shop_name = pbase.shop_name
			WHERE pbase.stat_date BETWEEN ? AND ?

			UNION ALL

			SELECT
				'京东' AS platform,
				DATE_FORMAT(jbase.stat_date, '%Y-%m-%d') AS stat_date,
				jbase.shop_name,
				IFNULL(js.presale_receive_users, IFNULL(jw.message_consult_count, IFNULL(jw.consult_count, 0))) AS consult_users,
				IFNULL(js.presale_receive_users, IFNULL(jw.message_consult_count, IFNULL(jw.consult_count, 0))) AS inquiry_users,
				IFNULL(js.order_users, 0) AS pay_users,
				IFNULL(js.order_goods_amount, 0) AS sales_amount,
				IFNULL(jw.first_avg_resp_seconds, 0) AS first_response_seconds,
				IFNULL(jw.new_avg_resp_seconds, 0) AS response_seconds,
				IFNULL(jw.satisfaction_rate, 0) AS satisfaction_rate,
				IFNULL(js.consult_to_order_rate, 0) AS conv_rate
			FROM (
				SELECT stat_date, shop_name FROM op_jd_cs_workload_daily
				UNION
				SELECT stat_date, shop_name FROM op_jd_cs_sales_perf_daily
			) jbase
			LEFT JOIN op_jd_cs_workload_daily jw ON jw.stat_date = jbase.stat_date AND jw.shop_name = jbase.shop_name
			LEFT JOIN op_jd_cs_sales_perf_daily js ON js.stat_date = jbase.stat_date AND js.shop_name = jbase.shop_name
			WHERE jbase.stat_date BETWEEN ? AND ?

			UNION ALL

			SELECT
				'抖音' AS platform,
				DATE_FORMAT(stat_date, '%Y-%m-%d') AS stat_date,
				shop_name,
				IFNULL(inquiry_users, IFNULL(received_users, 0)) AS consult_users,
				IFNULL(inquiry_users, IFNULL(received_users, 0)) AS inquiry_users,
				IFNULL(pay_users, 0) AS pay_users,
				IFNULL(inquiry_pay_amount, 0) AS sales_amount,
				IFNULL(all_day_first_reply_seconds, 0) AS first_response_seconds,
				IFNULL(all_day_avg_reply_seconds, 0) AS response_seconds,
				IFNULL(all_day_satisfaction_rate, 0) AS satisfaction_rate,
				IFNULL(inquiry_conv_rate, 0) AS conv_rate
			FROM op_douyin_cs_feige_daily
			WHERE stat_date BETWEEN ? AND ?

			UNION ALL

			SELECT
				'快手' AS platform,
				DATE_FORMAT(stat_date, '%Y-%m-%d') AS stat_date,
				shop_name,
				IFNULL(consult_users, 0) AS consult_users,
				IFNULL(consult_users, 0) AS inquiry_users,
				IFNULL(pay_users, 0) AS pay_users,
				IFNULL(cs_sales_amount, 0) AS sales_amount,
				0 AS first_response_seconds,
				CASE
					WHEN IFNULL(reply_3min_rate_person, 0) <= 1 THEN IFNULL(reply_3min_rate_person, 0) * 100
					ELSE IFNULL(reply_3min_rate_person, 0)
				END AS response_seconds,
				CASE
					WHEN IFNULL(good_rate_person, 0) <= 1 THEN IFNULL(good_rate_person, 0) * 100
					ELSE IFNULL(good_rate_person, 0)
				END AS satisfaction_rate,
				CASE
					WHEN IFNULL(inquiry_conv_rate, 0) <= 1 THEN IFNULL(inquiry_conv_rate, 0) * 100
					ELSE IFNULL(inquiry_conv_rate, 0)
				END AS conv_rate
			FROM op_kuaishou_cs_assessment_daily
			WHERE stat_date BETWEEN ? AND ?

			UNION ALL

			SELECT
				'小红书' AS platform,
				DATE_FORMAT(stat_date, '%Y-%m-%d') AS stat_date,
				shop_name,
				IFNULL(case_count, 0) AS consult_users,
				IFNULL(case_count, 0) AS inquiry_users,
				IFNULL(inquiry_pay_pkg_count, 0) AS pay_users,
				IFNULL(inquiry_pay_gmv, 0) AS sales_amount,
				0 AS first_response_seconds,
				CASE
					WHEN IFNULL(reply_in_3min_case_ratio, 0) <= 1 THEN IFNULL(reply_in_3min_case_ratio, 0) * 100
					ELSE IFNULL(reply_in_3min_case_ratio, 0)
				END AS response_seconds,
				CASE
					WHEN IFNULL(positive_case_ratio, 0) <= 1 THEN IFNULL(positive_case_ratio, 0) * 100
					ELSE IFNULL(positive_case_ratio, 0)
				END AS satisfaction_rate,
				CASE
					WHEN IFNULL(inquiry_pay_case_ratio, 0) <= 1 THEN IFNULL(inquiry_pay_case_ratio, 0) * 100
					ELSE IFNULL(inquiry_pay_case_ratio, 0)
				END AS conv_rate
			FROM op_xhs_cs_analysis_daily
			WHERE stat_date BETWEEN ? AND ?
		) metrics
		WHERE (? = '' OR platform = ?)
		ORDER BY stat_date, platform, shop_name
	`, args...)
	if !ok {
		return
	}
	defer rows.Close()

	records := make([]customerMetricRecord, 0)
	for rows.Next() {
		var rec customerMetricRecord
		var consultUsers sql.NullFloat64
		var inquiryUsers sql.NullFloat64
		var payUsers sql.NullFloat64
		var salesAmount sql.NullFloat64
		var firstRespSeconds sql.NullFloat64
		var responseSeconds sql.NullFloat64
		var satisfactionRate sql.NullFloat64
		var convRate sql.NullFloat64

		if writeDatabaseError(w, rows.Scan(
			&rec.Platform,
			&rec.Date,
			&rec.ShopName,
			&consultUsers,
			&inquiryUsers,
			&payUsers,
			&salesAmount,
			&firstRespSeconds,
			&responseSeconds,
			&satisfactionRate,
			&convRate,
		)) {
			return
		}

		rec.ConsultUsers = nullFloat(consultUsers)
		rec.InquiryUsers = nullFloat(inquiryUsers)
		rec.PayUsers = nullFloat(payUsers)
		rec.SalesAmount = nullFloat(salesAmount)
		rec.FirstRespSeconds = nullFloat(firstRespSeconds)
		rec.ResponseSeconds = nullFloat(responseSeconds)
		rec.SatisfactionRate = nullFloat(satisfactionRate)
		rec.ConvRate = nullFloat(convRate)

		if platformFilter != "" && rec.Platform != platformFilter {
			continue
		}

		if rec.Platform == "天猫" &&
			rec.ConsultUsers <= 0 &&
			rec.SalesAmount <= 0 &&
			rec.FirstRespSeconds <= 0 &&
			rec.ResponseSeconds <= 0 &&
			rec.SatisfactionRate <= 0 &&
			rec.ConvRate <= 0 {
			continue
		}

		records = append(records, rec)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}

	totalAgg := customerMetricAgg{}
	platformAgg := map[string]*customerMetricAgg{}
	platformShopSets := map[string]map[string]struct{}{}
	trendAgg := map[string]*customerMetricAgg{}
	shopAgg := map[string]*customerMetricAgg{}
	shopMeta := map[string]customerShopStat{}
	allShops := map[string]struct{}{}
	latestDate := ""

	for _, rec := range records {
		dAgg, ok := trendAgg[rec.Date]
		if !ok {
			dAgg = &customerMetricAgg{}
			trendAgg[rec.Date] = dAgg
		}
		dAgg.add(rec)

		if rec.Date < start || rec.Date > end {
			continue
		}
		totalAgg.add(rec)
		if rec.Date > latestDate {
			latestDate = rec.Date
		}
		allShops[rec.Platform+"|"+rec.ShopName] = struct{}{}

		pAgg, ok := platformAgg[rec.Platform]
		if !ok {
			pAgg = &customerMetricAgg{}
			platformAgg[rec.Platform] = pAgg
		}
		pAgg.add(rec)
		if _, ok := platformShopSets[rec.Platform]; !ok {
			platformShopSets[rec.Platform] = map[string]struct{}{}
		}
		platformShopSets[rec.Platform][rec.ShopName] = struct{}{}

		shopKey := rec.Platform + "|" + rec.ShopName
		sAgg, ok := shopAgg[shopKey]
		if !ok {
			sAgg = &customerMetricAgg{}
			shopAgg[shopKey] = sAgg
			shopMeta[shopKey] = customerShopStat{
				Platform: rec.Platform,
				ShopName: rec.ShopName,
			}
		}
		sAgg.add(rec)
	}

	platformStats := make([]customerPlatformStat, 0, len(platformAgg))
	for platform, agg := range platformAgg {
		platformStats = append(platformStats, customerPlatformStat{
			Platform:            platform,
			RecordCount:         agg.RecordCount,
			ShopCount:           len(platformShopSets[platform]),
			ConsultUsers:        roundFloat(agg.ConsultUsers, 0),
			InquiryUsers:        roundFloat(agg.InquiryUsers, 0),
			PayUsers:            roundFloat(agg.PayUsers, 0),
			SalesAmount:         roundFloat(agg.SalesAmount, 2),
			AvgFirstRespSeconds: roundFloat(agg.avgFirstRespSeconds(), 1),
			AvgResponseSeconds:  roundFloat(agg.avgResponseSeconds(), 1),
			AvgSatisfactionRate: roundFloat(agg.avgSatisfactionRate(), 2),
			AvgConvRate:         roundFloat(agg.avgConvRate(), 2),
		})
	}
	sort.Slice(platformStats, func(i, j int) bool {
		return platformStats[i].SalesAmount > platformStats[j].SalesAmount
	})

	trendDates := make([]string, 0, len(trendAgg))
	for date := range trendAgg {
		trendDates = append(trendDates, date)
	}
	sort.Strings(trendDates)
	trend := make([]customerTrendPoint, 0, len(trendDates))
	for _, date := range trendDates {
		agg := trendAgg[date]
		trend = append(trend, customerTrendPoint{
			Date:                date,
			ConsultUsers:        roundFloat(agg.ConsultUsers, 0),
			InquiryUsers:        roundFloat(agg.InquiryUsers, 0),
			PayUsers:            roundFloat(agg.PayUsers, 0),
			SalesAmount:         roundFloat(agg.SalesAmount, 2),
			AvgFirstRespSeconds: roundFloat(agg.avgFirstRespSeconds(), 1),
			AvgResponseSeconds:  roundFloat(agg.avgResponseSeconds(), 1),
			AvgSatisfactionRate: roundFloat(agg.avgSatisfactionRate(), 2),
			AvgConvRate:         roundFloat(agg.avgConvRate(), 2),
		})
	}

	shopRanking := make([]customerShopStat, 0, len(shopAgg))
	for key, agg := range shopAgg {
		meta := shopMeta[key]
		shopRanking = append(shopRanking, customerShopStat{
			Platform:            meta.Platform,
			ShopName:            meta.ShopName,
			RecordCount:         agg.RecordCount,
			ConsultUsers:        roundFloat(agg.ConsultUsers, 0),
			InquiryUsers:        roundFloat(agg.InquiryUsers, 0),
			PayUsers:            roundFloat(agg.PayUsers, 0),
			SalesAmount:         roundFloat(agg.SalesAmount, 2),
			AvgFirstRespSeconds: roundFloat(agg.avgFirstRespSeconds(), 1),
			AvgResponseSeconds:  roundFloat(agg.avgResponseSeconds(), 1),
			AvgSatisfactionRate: roundFloat(agg.avgSatisfactionRate(), 2),
			AvgConvRate:         roundFloat(agg.avgConvRate(), 2),
		})
	}
	sort.Slice(shopRanking, func(i, j int) bool {
		return shopRanking[i].SalesAmount > shopRanking[j].SalesAmount
	})
	if len(shopRanking) > 30 {
		shopRanking = shopRanking[:30]
	}

	payUserConsultRatio := 0.0
	salesPerConsultUser := 0.0
	salesPerPayUser := 0.0
	if totalAgg.ConsultUsers > 0 {
		payUserConsultRatio = totalAgg.PayUsers / totalAgg.ConsultUsers * 100
		salesPerConsultUser = totalAgg.SalesAmount / totalAgg.ConsultUsers
	}
	if totalAgg.PayUsers > 0 {
		salesPerPayUser = totalAgg.SalesAmount / totalAgg.PayUsers
	}

	writeJSON(w, map[string]interface{}{
		"summary": map[string]interface{}{
			"platformCount":       len(platformStats),
			"shopCount":           len(allShops),
			"recordCount":         totalAgg.RecordCount,
			"consultUsers":        roundFloat(totalAgg.ConsultUsers, 0),
			"payUsers":            roundFloat(totalAgg.PayUsers, 0),
			"salesAmount":         roundFloat(totalAgg.SalesAmount, 2),
			"avgFirstRespSeconds": roundFloat(totalAgg.avgFirstRespSeconds(), 1),
			"avgResponseSeconds":  roundFloat(totalAgg.avgResponseSeconds(), 1),
			"avgSatisfactionRate": roundFloat(totalAgg.avgSatisfactionRate(), 2),
			"avgConversionRate":   roundFloat(totalAgg.avgConvRate(), 2),
			"payUserConsultRatio": roundFloat(payUserConsultRatio, 2),
			"salesPerConsultUser": roundFloat(salesPerConsultUser, 2),
			"salesPerPayUser":     roundFloat(salesPerPayUser, 2),
			"latestDate":          latestDate,
		},
		"platformStats": platformStats,
		"trend":         trend,
		"shopRanking":   shopRanking,
		"dateRange": map[string]string{
			"start": start,
			"end":   end,
		},
		"trendRange": map[string]string{
			"start": trendStart,
			"end":   trendEnd,
		},
	})
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": 200,
		"data": data,
	})
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": code,
		"msg":  msg,
	})
}
