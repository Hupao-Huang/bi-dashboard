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
	DingCallbackHost string
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

// ClearCacheByPrefix 精准清理：只删 key 以 prefix 开头的缓存条目
// prefix 示例: "api|/api/stock/" 匹配所有 stock 相关接口（不影响别的模块）
// WithCache 的 key 格式: "api|<path>?<query>|u:<uid>"
func ClearCacheByPrefix(prefix string) int {
	overviewCacheMu.Lock()
	defer overviewCacheMu.Unlock()
	n := 0
	for k := range overviewCache {
		if strings.HasPrefix(k, prefix) {
			delete(overviewCache, k)
			n++
		}
	}
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
			ROUND(SUM(IFNULL(local_goods_amt, goods_amt)), 2) as sales,
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
			ROUND(SUM(IFNULL(local_goods_amt, goods_amt)), 2) as sales,
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
				ROUND(SUM(IFNULL(local_goods_amt, goods_amt)), 2) as sales,
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
			ROUND(SUM(IFNULL(local_goods_amt, goods_amt)), 2) as sales,
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

	// 6. 产品定位 × 部门明细（含毛利，总览矩阵表用）
	type GradeDeptSales struct {
		Grade      string  `json:"grade"`
		Department string  `json:"department"`
		Sales      float64 `json:"sales"`
		Profit     float64 `json:"profit"`
	}
	var gradeDeptSales []GradeDeptSales
	gdArgs := append([]interface{}{start, end}, scopeArgs...)
	gdRows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT IFNULL(g.goods_field7,'未设置') as grade,
			IFNULL(s.department,'其他') as department,
			ROUND(SUM(s.goods_amt), 2) as sales,
			ROUND(SUM(s.gross_profit), 2) as profit
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
		if writeDatabaseError(w, gdRows.Scan(&gd.Grade, &gd.Department, &gd.Sales, &gd.Profit)) {
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
		"重客":   "shop_name LIKE '%重客系统%'",
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
	const offlineRegionPrefilter = ` AND (shop_name LIKE '%大区%' OR shop_name LIKE '%省区%' OR shop_name LIKE '%重客系统%')`
	if dept == "offline" {
		shopListSQL = `SELECT ` + offlineRegionExpr + ` as shop_name,
			ROUND(SUM(local_goods_amt), 2) as sales,
			ROUND(SUM(goods_qty), 0) as qty,
			ROUND(SUM(gross_profit), 2) as profit
		FROM sales_goods_summary
		WHERE department = ? AND shop_name IS NOT NULL
		  AND stat_date BETWEEN ? AND ?` + offlineRegionPrefilter + scopeCond + `
		GROUP BY 1 ORDER BY sales DESC`
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
	// crossDept=1: 跨 4 部门聚合渠道分布（财务·产品利润页用于看商品在各部门/各渠道的全口径分布）
	crossDept := r.URL.Query().Get("crossDept") == "1"
	type ChannelSales struct {
		ShopName string  `json:"shopName"`
		Sales    float64 `json:"sales"`
		Qty      float64 `json:"qty"`
	}
	goodsChannels := map[string][]ChannelSales{}
	if len(goods) > 0 {
		placeholders := make([]string, len(goods))
		for i := range goods {
			placeholders[i] = "?"
		}
		var chSQL string
		var chArgs []interface{}
		if crossDept {
			// 跨部门：忽略 dept/shop/platform/scope 过滤，按 stat_date+goods_no 全口径聚合
			chArgs = []interface{}{start, end}
			for _, g := range goods {
				chArgs = append(chArgs, g.GoodsNo)
			}
			chSQL = `SELECT goods_no, shop_name,
				ROUND(SUM(local_goods_amt), 2) as sales,
				ROUND(SUM(goods_qty), 0) as qty
			FROM sales_goods_summary
			WHERE stat_date BETWEEN ? AND ?
			  AND goods_no IN (` + joinStrings(placeholders, ",") + `)
			GROUP BY goods_no, shop_name
			ORDER BY goods_no, sales DESC`
		} else {
			chArgs = []interface{}{dept, start, end}
			for _, g := range goods {
				chArgs = append(chArgs, g.GoodsNo)
			}
			chArgs = append(chArgs, extraArgs...)
			if dept == "offline" {
				chSQL = `SELECT goods_no, ` + offlineRegionExpr + ` as shop_name,
					ROUND(SUM(local_goods_amt), 2) as sales,
					ROUND(SUM(goods_qty), 0) as qty
				FROM sales_goods_summary
				WHERE department = ? AND stat_date BETWEEN ? AND ?
				  AND goods_no IN (` + joinStrings(placeholders, ",") + `)` +
					offlineRegionPrefilter + shopCond + scopeCond + `
				GROUP BY goods_no, 2
				ORDER BY goods_no, sales DESC`
			} else {
				chSQL = `SELECT goods_no, shop_name,
					ROUND(SUM(local_goods_amt), 2) as sales,
					ROUND(SUM(goods_qty), 0) as qty
				FROM sales_goods_summary
				WHERE department = ? AND stat_date BETWEEN ? AND ?
				  AND goods_no IN (` + joinStrings(placeholders, ",") + `)` +
					shopCond + platCond + scopeCond + `
				GROUP BY goods_no, shop_name
				ORDER BY goods_no, sales DESC`
			}
		}
		chRows, ok := queryRowsOrWriteError(w, h.DB, chSQL, chArgs...)
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
			LEFT JOIN (SELECT channel_name, department, MAX(online_plat_name) AS online_plat_name FROM sales_channel GROUP BY channel_name, department) sc ON sc.channel_name = s.shop_name AND sc.department = s.department
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
				WHERE s.department = ? AND s.stat_date BETWEEN ? AND ?` + offlineRegionPrefilter + scopeCond + `
				GROUP BY 1, 2
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
		LEFT JOIN (SELECT channel_name, department, MAX(online_plat_name) AS online_plat_name FROM sales_channel GROUP BY channel_name, department) sc ON sc.channel_name = s.shop_name AND sc.department = s.department
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

	// offline 补充：查询日期范围内各月目标累加
	regionTargets := map[string]float64{}
	if dept == "offline" {
		// 解析 start/end 年月
		startTime, _ := time.Parse("2006-01-02", start)
		endTime, _ := time.Parse("2006-01-02", end)
		if !startTime.IsZero() && !endTime.IsZero() {
			tRows, tOk := queryRowsOrWriteError(w, h.DB, `
				SELECT region, SUM(target)
				FROM offline_region_target
				WHERE (year*100+month) BETWEEN ? AND ?
				GROUP BY region`,
				startTime.Year()*100+int(startTime.Month()),
				endTime.Year()*100+int(endTime.Month()),
			)
			if tOk {
				defer tRows.Close()
				for tRows.Next() {
					var reg string
					var tgt float64
					if writeDatabaseError(w, tRows.Scan(&reg, &tgt)) {
						return
					}
					regionTargets[reg] = tgt
				}
			}
		}
	}

	// crossDept=1 额外返回：产品定位×部门 + 产品定位×店铺 全口径聚合（含毛利）
	type GradeDeptItem struct {
		Grade      string  `json:"grade"`
		Department string  `json:"department"`
		Sales      float64 `json:"sales"`
		Profit     float64 `json:"profit"`
	}
	type GradeShopItem struct {
		Grade      string  `json:"grade"`
		Department string  `json:"department"`
		ShopName   string  `json:"shopName"`
		Sales      float64 `json:"sales"`
		Profit     float64 `json:"profit"`
	}
	var gradeDeptSalesAll []GradeDeptItem
	var gradeShopSalesAll []GradeShopItem
	if crossDept {
		gdRows, ok := queryRowsOrWriteError(w, h.DB, `
			SELECT IFNULL(g.goods_field7,'未设置') as grade, s.department,
				ROUND(SUM(s.local_goods_amt),2) as sales,
				ROUND(SUM(s.gross_profit),2) as profit
			FROM sales_goods_summary s
			LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
			WHERE s.stat_date BETWEEN ? AND ?
			GROUP BY grade, s.department
			ORDER BY FIELD(grade,'S','A','B','C','D','未设置'), sales DESC`, start, end)
		if !ok {
			return
		}
		defer gdRows.Close()
		for gdRows.Next() {
			var it GradeDeptItem
			if writeDatabaseError(w, gdRows.Scan(&it.Grade, &it.Department, &it.Sales, &it.Profit)) {
				return
			}
			gradeDeptSalesAll = append(gradeDeptSalesAll, it)
		}
		if writeDatabaseError(w, gdRows.Err()) {
			return
		}

		gsRows, ok := queryRowsOrWriteError(w, h.DB, `
			SELECT IFNULL(g.goods_field7,'未设置') as grade,
				IFNULL(s.department,'其他') as department,
				s.shop_name,
				ROUND(SUM(s.local_goods_amt),2) as sales,
				ROUND(SUM(s.gross_profit),2) as profit
			FROM sales_goods_summary s
			LEFT JOIN (SELECT DISTINCT goods_no, goods_field7 FROM goods) g ON g.goods_no = s.goods_no
			WHERE s.stat_date BETWEEN ? AND ?
			GROUP BY grade, s.department, s.shop_name
			ORDER BY FIELD(grade,'S','A','B','C','D','未设置'), sales DESC`, start, end)
		if !ok {
			return
		}
		defer gsRows.Close()
		for gsRows.Next() {
			var it GradeShopItem
			if writeDatabaseError(w, gsRows.Scan(&it.Grade, &it.Department, &it.ShopName, &it.Sales, &it.Profit)) {
				return
			}
			gradeShopSalesAll = append(gradeShopSalesAll, it)
		}
		if writeDatabaseError(w, gsRows.Err()) {
			return
		}
	}

	writeJSON(w, map[string]interface{}{
		"daily":             daily,
		"shops":             shops,
		"goods":             goods,
		"goodsChannels":     goodsChannels,
		"brands":            brands,
		"grades":            grades,
		"platforms":         platforms,
		"platformSales":     platformSales,
		"gradePlatSales":    gradePlatSales,
		"gradeDeptSalesAll": gradeDeptSalesAll,
		"gradeShopSalesAll": gradeShopSalesAll,
		"regionTargets":     regionTargets,
		"dateRange":         map[string]string{"start": start, "end": end},
		"trendRange":        map[string]string{"start": trendStart, "end": trendEnd},
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
		INNER JOIN (SELECT DISTINCT goods_no FROM goods WHERE goods_field7 = 'S') g ON g.goods_no = s.goods_no
		WHERE s.stat_date BETWEEN ? AND ?
		  AND s.shop_name IS NOT NULL AND s.shop_name != ''` + deptCond + `
		GROUP BY platform_name, s.department
		ORDER BY SUM(s.local_goods_amt) DESC`
	} else {
		shopSQL = `
		SELECT s.shop_name, s.department, ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0)
		FROM sales_goods_summary s
		INNER JOIN (SELECT DISTINCT goods_no FROM goods WHERE goods_field7 = 'S') g ON g.goods_no = s.goods_no
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
		INNER JOIN (SELECT goods_no, MAX(goods_name) AS goods_name FROM goods WHERE goods_field7 = 'S' GROUP BY goods_no) g ON g.goods_no = s.goods_no
		WHERE s.stat_date BETWEEN ? AND ?` + deptCond + `
		GROUP BY s.goods_no, g.goods_name
		ORDER BY SUM(s.local_goods_amt) DESC`
	} else {
		goodsSQL = `
		SELECT s.goods_no, g.goods_name, ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0),
			COUNT(DISTINCT s.shop_name)
		FROM sales_goods_summary s
		INNER JOIN (SELECT goods_no, MAX(goods_name) AS goods_name FROM goods WHERE goods_field7 = 'S' GROUP BY goods_no) g ON g.goods_no = s.goods_no
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
		INNER JOIN (SELECT DISTINCT goods_no FROM goods WHERE goods_field7 = 'S') g ON g.goods_no = s.goods_no
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
		INNER JOIN (SELECT goods_no, MAX(goods_name) AS goods_name FROM goods WHERE goods_field7 = 'S' GROUP BY goods_no) g ON g.goods_no = s.goods_no
		WHERE s.stat_date BETWEEN ? AND ?
		  AND s.shop_name IS NOT NULL AND s.shop_name != ''` + deptCond + `
		GROUP BY g.goods_name, platform_name
		HAVING SUM(s.local_goods_amt) > 0
		ORDER BY g.goods_name, SUM(s.local_goods_amt) DESC`
	} else {
		detailSQL = `
		SELECT g.goods_name, s.shop_name, ROUND(SUM(s.local_goods_amt),2), ROUND(SUM(s.goods_qty),0)
		FROM sales_goods_summary s
		INNER JOIN (SELECT goods_no, MAX(goods_name) AS goods_name FROM goods WHERE goods_field7 = 'S' GROUP BY goods_no) g ON g.goods_no = s.goods_no
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
		IsContent bool    `json:"isContent"` // true=内容推广(无投放费用，cost/roi/cpc 不参与对比)
	}
	type SkuTopRow struct {
		ShopName    string  `json:"shopName"`
		ProductID   string  `json:"productId"`
		ProductName string  `json:"productName"`
		Cost        float64 `json:"cost"`
		PayAmount   float64 `json:"payAmount"`
		ROI         float64 `json:"roi"`
		Clicks      int     `json:"clicks"`
	}
	var tmallSkuTop []SkuTopRow
	var pddSkuTop []SkuTopRow

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
			// 天猫万象台 SKU Top 20：仅在用户单选"天猫"平台时返回（all 时不返回避免视觉干扰）
			// 按 (店铺, 商品) 分组，避免同商品在不同店合并失真
			if platform == "tmall" {
				skuArgs := append([]interface{}{start, end}, shopArgs...)
				skuRows, skuErr := h.DB.Query(`SELECT shop_name, product_id,
					MAX(product_name) AS product_name,
					ROUND(SUM(cost),2) AS cost,
					ROUND(SUM(total_pay_amount),2) AS pay,
					CASE WHEN SUM(cost)>0 THEN ROUND(SUM(total_pay_amount)/SUM(cost),2) ELSE 0 END AS roi,
					SUM(clicks) AS clicks
					FROM op_tmall_campaign_detail_daily
					WHERE stat_date BETWEEN ? AND ?`+shopCond+`
					AND entity_type='商品'
					GROUP BY shop_name, product_id
					HAVING cost > 0
					ORDER BY cost DESC LIMIT 20`, skuArgs...)
				if skuErr == nil {
					for skuRows.Next() {
						var s SkuTopRow
						if err := skuRows.Scan(&s.ShopName, &s.ProductID, &s.ProductName, &s.Cost, &s.PayAmount, &s.ROI, &s.Clicks); err != nil {
							continue
						}
						tmallSkuTop = append(tmallSkuTop, s)
					}
					skuRows.Close()
				}
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

			// 拼多多 多多视频（内容推广，无投放费用）
			vArgs := append([]interface{}{start, end}, shopArgs...)
			var vGmv sql.NullFloat64
			var vClicks sql.NullInt64
			var vOrders sql.NullInt64
			vErr := h.DB.QueryRow(`SELECT ROUND(SUM(total_gmv),2), SUM(goods_click_cnt), SUM(order_count)
				FROM op_pdd_video_daily WHERE stat_date BETWEEN ? AND ?`+shopCond, vArgs...).
				Scan(&vGmv, &vClicks, &vOrders)
			if vErr == nil && (vGmv.Float64 > 0 || vOrders.Int64 > 0) {
				details = append(details, DetailRow{
					Platform:  "拼多多",
					Name:      "多多视频（内容）",
					Cost:      0,
					PayAmount: vGmv.Float64,
					ROI:       0,
					Clicks:    int(vClicks.Int64),
					AvgCpc:    0,
					IsContent: true,
				})
			}

			// 拼多多商品推广 SKU Top 20：仅在用户单选"拼多多"平台时返回
			if platform == "pdd" {
				skuArgs := append([]interface{}{start, end}, shopArgs...)
				skuRows, skuErr := h.DB.Query(`SELECT shop_name, goods_id,
					MAX(goods_name) AS goods_name,
					ROUND(SUM(cost_total),2) AS cost,
					ROUND(SUM(pay_amount),2) AS pay,
					CASE WHEN SUM(cost_total)>0 THEN ROUND(SUM(pay_amount)/SUM(cost_total),2) ELSE 0 END AS roi,
					SUM(clicks) AS clicks
					FROM op_pdd_campaign_goods_daily
					WHERE stat_date BETWEEN ? AND ?`+shopCond+`
					GROUP BY shop_name, goods_id
					HAVING cost > 0
					ORDER BY cost DESC LIMIT 20`, skuArgs...)
				if skuErr == nil {
					for skuRows.Next() {
						var s SkuTopRow
						if err := skuRows.Scan(&s.ShopName, &s.ProductID, &s.ProductName, &s.Cost, &s.PayAmount, &s.ROI, &s.Clicks); err != nil {
							continue
						}
						pddSkuTop = append(pddSkuTop, s)
					}
					skuRows.Close()
				}
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
			// 天猫超市推广明细：无界/智多星 拆到场景级（每个场景独立行，name 带括号标父类），淘客单独一行
			// 无界场景：op_tmall_cs_wujie_scene_daily
			wujieArgs := append([]interface{}{start, end}, shopArgs...)
			wRows, wErr := h.DB.Query(`SELECT scene_name, ROUND(SUM(cost),2), ROUND(SUM(total_pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(total_pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks),
				CASE WHEN SUM(clicks)>0 THEN ROUND(SUM(cost)/SUM(clicks),2) ELSE 0 END
				FROM op_tmall_cs_wujie_scene_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				AND scene_name IS NOT NULL AND scene_name != ''
				GROUP BY scene_name ORDER BY SUM(cost) DESC`, wujieArgs...)
			if wErr == nil {
				for wRows.Next() {
					var d DetailRow
					var sceneName string
					if err := wRows.Scan(&sceneName, &d.Cost, &d.PayAmount, &d.ROI, &d.Clicks, &d.AvgCpc); err != nil {
						continue
					}
					d.Platform = "天猫超市"
					d.Name = sceneName + "（无界）"
					details = append(details, d)
				}
				wRows.Close()
			}

			// 智多星场景：op_tmall_cs_smart_plan_daily
			smartArgs := append([]interface{}{start, end}, shopArgs...)
			spRows, spErr := h.DB.Query(`SELECT campaign_scene, ROUND(SUM(cost),2), ROUND(SUM(total_pay_amount),2),
				CASE WHEN SUM(cost)>0 THEN ROUND(SUM(total_pay_amount)/SUM(cost),2) ELSE 0 END,
				SUM(clicks),
				CASE WHEN SUM(clicks)>0 THEN ROUND(SUM(cost)/SUM(clicks),2) ELSE 0 END
				FROM op_tmall_cs_smart_plan_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				AND campaign_scene IS NOT NULL AND campaign_scene != ''
				GROUP BY campaign_scene ORDER BY SUM(cost) DESC`, smartArgs...)
			if spErr == nil {
				for spRows.Next() {
					var d DetailRow
					var sceneName string
					if err := spRows.Scan(&sceneName, &d.Cost, &d.PayAmount, &d.ROI, &d.Clicks, &d.AvgCpc); err != nil {
						continue
					}
					d.Platform = "天猫超市"
					d.Name = sceneName + "（智多星）"
					details = append(details, d)
				}
				spRows.Close()
			}

			// 淘客单独一行（无场景维度，从 op_tmall_cs_campaign_daily promo_type='淘客' 取）
			tkArgs := append([]interface{}{start, end}, shopArgs...)
			var tkCost, tkPay sql.NullFloat64
			var tkClicks sql.NullInt64
			tkErr := h.DB.QueryRow(`SELECT ROUND(SUM(cost),2), ROUND(SUM(pay_amount),2), SUM(clicks)
				FROM op_tmall_cs_campaign_daily WHERE stat_date BETWEEN ? AND ?`+shopCond+`
				AND promo_type='淘客'`, tkArgs...).Scan(&tkCost, &tkPay, &tkClicks)
			if tkErr == nil && (tkCost.Float64 > 0 || tkPay.Float64 > 0) {
				roi := 0.0
				if tkCost.Float64 > 0 {
					roi = tkPay.Float64 / tkCost.Float64
				}
				cpc := 0.0
				if tkClicks.Int64 > 0 {
					cpc = tkCost.Float64 / float64(tkClicks.Int64)
				}
				details = append(details, DetailRow{
					Platform:  "天猫超市",
					Name:      "淘客",
					Cost:      tkCost.Float64,
					PayAmount: tkPay.Float64,
					ROI:       roi,
					Clicks:    int(tkClicks.Int64),
					AvgCpc:    cpc,
				})
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
		"cpcDaily":     cpcDaily,
		"cpsDaily":     cpsDaily,
		"shopCosts":    shopCosts,
		"details":      details,
		"tmallSkuTop":  tmallSkuTop,
		"pddSkuTop":    pddSkuTop,
		"shops":        shops,
		"hasCps":       hasCps,
		"dateRange":    map[string]string{"start": start, "end": end},
		"trendRange":   map[string]string{"start": trendStart, "end": trendEnd},
	})
}

// mergeCpcDaily 合并同日CPC数据(跨平台汇总时)
func mergeCpcDaily(arr []CpcDaily, item CpcDaily) []CpcDaily {
	for i, a := range arr {
		if a.Date == item.Date {
			arr[i].Cost = round2(arr[i].Cost + item.Cost)
			arr[i].PayAmount = round2(arr[i].PayAmount + item.PayAmount)
			arr[i].Clicks += item.Clicks
			arr[i].Impr += item.Impr
			if arr[i].Cost > 0 {
				arr[i].ROI = float64(int(arr[i].PayAmount/arr[i].Cost*100)) / 100
			}
			return arr
		}
	}
	item.Cost = round2(item.Cost)
	item.PayAmount = round2(item.PayAmount)
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
			arr[i].PayAmount = round2(arr[i].PayAmount + item.PayAmount)
			arr[i].PayCommission = round2(arr[i].PayCommission + item.PayCommission)
			arr[i].PayUsers += item.PayUsers
			return arr
		}
	}
	item.PayAmount = round2(item.PayAmount)
	item.PayCommission = round2(item.PayCommission)
	return append(arr, item)
}

// round2 浮点数累加防精度尾巴(0.090000000001 → 0.09)
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

type CpsDaily struct {
	Date          string  `json:"date"`
	PayAmount     float64 `json:"payAmount"`
	PayCommission float64 `json:"payCommission"`
	PayUsers      int     `json:"payUsers"`
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
