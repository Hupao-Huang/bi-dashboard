package handler

// 快递仓储分析 (LogisticsAnalysis / WarehouseFlow)
// v0.56: 仓储老大要看 "商品都卖到哪些省, 都从哪个仓发出"
// 数据源: trade_YYYYMM (主) + trade_goods_YYYYMM (SKU 行) 按月分表
//   - 主表: state(省), warehouse_name, shop_name, trade_status_explain
//   - 商品: sell_count(件数), divide_sell_total(摊销销额), goods_no, goods_name, is_gift
// 口径:
//   - 件数(qty)  = SUM(sell_count)
//   - 销额(amt)  = SUM(divide_sell_total)  摊销折扣后真销额
//   - 单数(ord)  = COUNT(DISTINCT trade_id)
//   - 赠品计入   (仓储看实物出货)
//   - 排取消单   (trade_status_explain NOT LIKE '%取消%'  实测 1-4 月数据 0 取消)

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// provinceNormSQL — DB 里 state 列脏数据严重("上海"/"上海市", "广东省"/"广东", "宁夏回族自治区银川市..."),
// 必须 normalize 才能和 ECharts 中国地图 GeoJSON name 匹配 (datav 用全名: "广东省"/"上海市"/"广西壮族自治区"...)
// 用法: SELECT %s AS state ...  (%s 替换为 provinceNormSQL)
const provinceNormSQL = `CASE
	WHEN t.state IN ('北京','北京市') THEN '北京市'
	WHEN t.state IN ('上海','上海市') THEN '上海市'
	WHEN t.state IN ('天津','天津市') THEN '天津市'
	WHEN t.state IN ('重庆','重庆市') THEN '重庆市'
	WHEN t.state LIKE '广东%' THEN '广东省'
	WHEN t.state LIKE '广西%' THEN '广西壮族自治区'
	WHEN t.state LIKE '宁夏%' THEN '宁夏回族自治区'
	WHEN t.state LIKE '内蒙古%' THEN '内蒙古自治区'
	WHEN t.state LIKE '新疆%' THEN '新疆维吾尔自治区'
	WHEN t.state LIKE '西藏%' THEN '西藏自治区'
	WHEN t.state LIKE '香港%' THEN '香港特别行政区'
	WHEN t.state LIKE '澳门%' THEN '澳门特别行政区'
	WHEN t.state LIKE '台湾%' THEN '台湾省'
	WHEN t.state IN ('江苏','江苏省') THEN '江苏省'
	WHEN t.state IN ('浙江','浙江省') THEN '浙江省'
	WHEN t.state IN ('山东','山东省') THEN '山东省'
	WHEN t.state IN ('福建','福建省') THEN '福建省'
	WHEN t.state IN ('湖南','湖南省') THEN '湖南省'
	WHEN t.state IN ('湖北','湖北省') THEN '湖北省'
	WHEN t.state IN ('河南','河南省') THEN '河南省'
	WHEN t.state IN ('河北','河北省') THEN '河北省'
	WHEN t.state IN ('山西','山西省') THEN '山西省'
	WHEN t.state IN ('陕西','陕西省') THEN '陕西省'
	WHEN t.state IN ('四川','四川省') THEN '四川省'
	WHEN t.state IN ('安徽','安徽省') THEN '安徽省'
	WHEN t.state IN ('江西','江西省') THEN '江西省'
	WHEN t.state IN ('辽宁','辽宁省') THEN '辽宁省'
	WHEN t.state IN ('吉林','吉林省') THEN '吉林省'
	WHEN t.state IN ('黑龙江','黑龙江省') THEN '黑龙江省'
	WHEN t.state IN ('云南','云南省') THEN '云南省'
	WHEN t.state IN ('贵州','贵州省') THEN '贵州省'
	WHEN t.state IN ('甘肃','甘肃省') THEN '甘肃省'
	WHEN t.state IN ('青海','青海省') THEN '青海省'
	WHEN t.state IN ('海南','海南省') THEN '海南省'
	ELSE t.state
END`

// allMetricsSelect — v0.56.3 仓储真口径: 订单数 + 包裹数, 前端切 metric 纯本地 re-render
//   t = 主表 trade_YYYYMM  p = trade_package_YYYYMM (LEFT JOIN, 一单可能多包裹/无包裹)
//   - orders   = COUNT(DISTINCT t.trade_id)        接单工作量
//   - packages = COUNT(DISTINCT trade+logistic_no) 实际发货包裹数 (NULL 自动忽略)
const allMetricsSelect = `COUNT(DISTINCT t.trade_id) AS orders,
	COUNT(DISTINCT CONCAT(t.trade_id, '|', p.logistic_no)) AS packages`

// resolveYM 把 ?ym=YYYY-MM 转换成 (yyyymm, error)。空值默认上月或最新表。
func resolveYM(db *sql.DB, ym string) (string, error) {
	if ym == "" {
		// 默认: 取 information_schema 里最新的 trade_YYYYMM 表
		row := db.QueryRow(`SELECT TABLE_NAME FROM information_schema.TABLES
			WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME REGEXP '^trade_[0-9]{6}$'
			ORDER BY TABLE_NAME DESC LIMIT 1`)
		var t string
		if err := row.Scan(&t); err != nil {
			return "", fmt.Errorf("no trade_* table found")
		}
		return strings.TrimPrefix(t, "trade_"), nil
	}
	tt, err := time.Parse("2006-01", ym)
	if err != nil {
		return "", fmt.Errorf("invalid ym format, want YYYY-MM")
	}
	return tt.Format("200601"), nil
}

// flowFilter 把过滤条件拼成 WHERE 片段 + 参数, 表 alias t 是主表 trade_YYYYMM
type flowFilter struct {
	ym        string // 200601
	shop      string
	warehouse string
	province  string
	skuKw     string // 关键字, 走 g.goods_name LIKE 或 g.goods_no =
	skuNo     string // 精确 goods_no
}

func parseFlowFilter(r *http.Request, ym string) flowFilter {
	q := r.URL.Query()
	return flowFilter{
		ym:        ym,
		shop:      strings.TrimSpace(q.Get("shop")),
		warehouse: strings.TrimSpace(q.Get("warehouse")),
		province:  strings.TrimSpace(q.Get("province")),
		skuKw:     strings.TrimSpace(q.Get("sku_keyword")),
		skuNo:     strings.TrimSpace(q.Get("goods_no")),
	}
}

// needsGoods 仅在 SKU 过滤激活时才需要 JOIN trade_goods (v0.56.5 性能优化)
func (f flowFilter) needsGoods() bool {
	return f.skuKw != "" || f.skuNo != ""
}

// canUseSummary 判断能否走物化预聚合表 warehouse_flow_summary
// 没 SKU 过滤 + 该 ym 已物化 才走物化路径(切月场景 7s → <50ms)
// 未物化的 ym 自动降级到原 SQL 兜底(探测查询 <5ms)
func (f flowFilter) canUseSummary(db *sql.DB, ym string) bool {
	if f.needsGoods() {
		return false
	}
	var cnt int
	db.QueryRow(`SELECT COUNT(*) FROM warehouse_flow_summary WHERE ym = ? LIMIT 1`,
		ym[:4]+"-"+ym[4:]).Scan(&cnt)
	return cnt > 0
}

// buildSummaryWhere 物化表过滤(ym必传 + 可选 shop/warehouse/province)
// 注意: ym 用 'YYYY-MM' 格式而非 'YYYYMM'
func (f flowFilter) buildSummaryWhere(ym string) (string, []interface{}) {
	var sb strings.Builder
	args := []interface{}{}
	sb.WriteString(" AND ym = ?")
	args = append(args, ym[:4]+"-"+ym[4:])
	if f.shop != "" {
		sb.WriteString(" AND shop_name = ?")
		args = append(args, f.shop)
	}
	if f.warehouse != "" {
		sb.WriteString(" AND warehouse_name = ?")
		args = append(args, f.warehouse)
	}
	if f.province != "" {
		sb.WriteString(" AND province = ?")
		args = append(args, f.province)
	}
	return sb.String(), args
}

// buildJoins 动态拼装 JOIN 子句, 永远 LEFT JOIN trade_package, 仅 SKU 过滤激活时 JOIN trade_goods
func (f flowFilter) buildJoins(tradeT, goodsT, pkgT string) string {
	joins := fmt.Sprintf("FROM %s t LEFT JOIN %s p ON p.trade_id = t.trade_id", tradeT, pkgT)
	if f.needsGoods() {
		joins += fmt.Sprintf(" JOIN %s g ON g.trade_id = t.trade_id", goodsT)
	}
	return joins
}

// buildWhere 返回 WHERE 子句 (含开头 " AND " 否则空) 和 args.
//   - 主表过滤: shop / warehouse / province / 排取消 / 7 仓白名单
//   - 商品过滤: skuKw / skuNo  (作用于 g)
func (f flowFilter) buildWhere() (string, []interface{}) {
	var sb strings.Builder
	args := []interface{}{}
	sb.WriteString(" AND t.trade_status_explain NOT LIKE '%取消%'")
	sb.WriteString(" AND t.state IS NOT NULL AND t.state != ''")
	// v0.56.4: 排除 trade_type 8/12 (补差/对账特殊单, 不产生物流包裹, sync-detail 也显式排除 12)
	// 实测: type 1/2/7/9/10 包裹覆盖率 100%, type 8/12 = 0%, 不算入仓储发货分析
	sb.WriteString(" AND t.trade_type NOT IN (8, 12)")
	// v0.56.2: 7 仓白名单 (与计划看板/库存预警共用 planWarehouses, 定义在 supply_chain.go)
	// 同时把不合格仓/虚拟仓/原料仓/平台仓全部排除 (它们都不在白名单里)
	whCond, whArgs := buildPlanWarehouseFilter("t.warehouse_name")
	sb.WriteString(whCond)
	args = append(args, whArgs...)
	if f.shop != "" {
		sb.WriteString(" AND t.shop_name = ?")
		args = append(args, f.shop)
	}
	if f.warehouse != "" {
		sb.WriteString(" AND t.warehouse_name = ?")
		args = append(args, f.warehouse)
	}
	if f.province != "" {
		sb.WriteString(" AND (" + provinceNormSQL + ") = ?")
		args = append(args, f.province)
	}
	if f.skuNo != "" {
		sb.WriteString(" AND g.goods_no = ?")
		args = append(args, f.skuNo)
	} else if f.skuKw != "" {
		sb.WriteString(" AND (g.goods_name LIKE ? OR g.goods_no LIKE ?)")
		args = append(args, "%"+f.skuKw+"%", "%"+f.skuKw+"%")
	}
	return sb.String(), args
}

// GetWarehouseFlowOverview KPI + 省份分布 + 仓库分布 (一次返回 3 指标全量, 前端切 metric 不重查)
//   GET /api/warehouse-flow/overview?ym=YYYY-MM&shop=&warehouse=&province=&sku_keyword=&goods_no=
func (h *DashboardHandler) GetWarehouseFlowOverview(w http.ResponseWriter, r *http.Request) {
	ym, err := resolveYM(h.DB, r.URL.Query().Get("ym"))
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	flt := parseFlowFilter(r, ym)

	// v0.60: 双轨路由 — 没 SKU 过滤走物化表 warehouse_flow_summary, 切月 7s → <200ms
	useSummary := flt.canUseSummary(h.DB, ym)

	var kpiSQL, provSQL, whSQL string
	var args []interface{}

	if useSummary {
		// 物化路径
		var sumWhere string
		sumWhere, args = flt.buildSummaryWhere(ym)
		// KPI: SUM(orders) + SUM(packages) + COUNT DISTINCT province/warehouse
		kpiSQL = `SELECT SUM(orders), SUM(packages),
			COUNT(DISTINCT province), COUNT(DISTINCT warehouse_name)
			FROM warehouse_flow_summary WHERE 1=1` + sumWhere
		provSQL = `SELECT province, SUM(orders), SUM(packages)
			FROM warehouse_flow_summary WHERE 1=1` + sumWhere +
			` GROUP BY province ORDER BY 2 DESC`
		whSQL = `SELECT warehouse_name, SUM(orders), SUM(packages)
			FROM warehouse_flow_summary WHERE 1=1` + sumWhere +
			` GROUP BY warehouse_name ORDER BY 2 DESC`
	} else {
		// 原 SQL 路径(SKU 过滤激活时降级)
		var where string
		where, args = flt.buildWhere()
		tradeT := "trade_" + ym
		goodsT := "trade_goods_" + ym
		pkgT := "trade_package_" + ym
		joins := flt.buildJoins(tradeT, goodsT, pkgT)
		kpiSQL = fmt.Sprintf(`
			SELECT
				COUNT(DISTINCT t.trade_id), COUNT(DISTINCT CONCAT(t.trade_id, '|', p.logistic_no)),
				COUNT(DISTINCT (%s)), COUNT(DISTINCT t.warehouse_name)
			%s WHERE 1=1 %s`, provinceNormSQL, joins, where)
		provSQL = fmt.Sprintf(`SELECT (%s) AS prov, %s
			%s WHERE 1=1 %s GROUP BY prov ORDER BY orders DESC`,
			provinceNormSQL, allMetricsSelect, joins, where)
		whSQL = fmt.Sprintf(`SELECT t.warehouse_name, %s
			%s WHERE 1=1 %s GROUP BY t.warehouse_name ORDER BY orders DESC`,
			allMetricsSelect, joins, where)
	}

	var kpi struct {
		Orders       int64 `json:"orders"`
		Packages     int64 `json:"packages"`
		ProvinceCnt  int   `json:"provinceCnt"`
		WarehouseCnt int   `json:"warehouseCnt"`
	}

	type rowMM struct {
		Name     string `json:"name"`
		Orders   int64  `json:"orders"`
		Packages int64  `json:"packages"`
	}

	// 渠道下拉/ymList 走原表(物化里 shop_name 也有但要保持口径一致, 历史 trade 表全)
	tradeT := "trade_" + ym

	// === 渠道(shop) 分布 - 用于筛选下拉 (限定 7 仓白名单内有销售单的渠道) ===
	whCondForShop, whArgsForShop := buildPlanWarehouseFilter("t.warehouse_name")
	shopSQL := fmt.Sprintf(`SELECT DISTINCT t.shop_name FROM %s t
		WHERE t.shop_name IS NOT NULL AND t.shop_name != ''%s
		ORDER BY t.shop_name`, tradeT, whCondForShop)

	// v0.56.5: 5 个 SQL 并发执行 (KPI + 省 + 仓 + shop 下拉 + ym 列表)
	// 串行 14s+ -> 并发 max(各SQL) ≈ 4-5s
	var (
		provinces  []rowMM
		warehouses []rowMM
		shops      []string
		ymList     []string
		kpiErr     error
		wg         sync.WaitGroup
	)
	wg.Add(5)

	// 1. KPI
	go func() {
		defer wg.Done()
		kpiErr = h.DB.QueryRow(kpiSQL, args...).Scan(&kpi.Orders, &kpi.Packages, &kpi.ProvinceCnt, &kpi.WarehouseCnt)
	}()
	// 2. 省份分布
	go func() {
		defer wg.Done()
		rs, err := h.DB.Query(provSQL, args...)
		if err != nil {
			return
		}
		defer rs.Close()
		for rs.Next() {
			var p rowMM
			if rs.Scan(&p.Name, &p.Orders, &p.Packages) == nil {
				provinces = append(provinces, p)
			}
		}
	}()
	// 3. 仓库分布
	go func() {
		defer wg.Done()
		rs, err := h.DB.Query(whSQL, args...)
		if err != nil {
			return
		}
		defer rs.Close()
		for rs.Next() {
			var p rowMM
			if rs.Scan(&p.Name, &p.Orders, &p.Packages) == nil {
				warehouses = append(warehouses, p)
			}
		}
	}()
	// 4. 渠道下拉
	go func() {
		defer wg.Done()
		rs, err := h.DB.Query(shopSQL, whArgsForShop...)
		if err != nil {
			return
		}
		defer rs.Close()
		for rs.Next() {
			var s string
			if rs.Scan(&s) == nil {
				shops = append(shops, s)
			}
		}
	}()
	// 5. 可选 ym 列表
	go func() {
		defer wg.Done()
		rs, err := h.DB.Query(`SELECT TABLE_NAME FROM information_schema.TABLES
			WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME REGEXP '^trade_[0-9]{6}$'
			ORDER BY TABLE_NAME DESC`)
		if err != nil {
			return
		}
		defer rs.Close()
		for rs.Next() {
			var n string
			if rs.Scan(&n) == nil {
				s := strings.TrimPrefix(n, "trade_")
				if len(s) == 6 {
					ymList = append(ymList, s[:4]+"-"+s[4:])
				}
			}
		}
	}()
	wg.Wait()

	if kpiErr != nil {
		writeError(w, 500, "kpi query failed: "+kpiErr.Error())
		return
	}

	writeJSON(w, map[string]interface{}{
		"ym":         ym[:4] + "-" + ym[4:],
		"kpi":        kpi,
		"provinces":  provinces,
		"warehouses": warehouses,
		"shops":      shops,
		"ymList":     ymList,
	})
}

// GetWarehouseFlowMatrix 仓 × 省 流向矩阵
//   GET /api/warehouse-flow/matrix?ym=YYYY-MM&metric=...&top_provinces=10
//   返回: { warehouses, provinces, values[wh_idx][prov_idx], rowTotals, colTotals, grand }
func (h *DashboardHandler) GetWarehouseFlowMatrix(w http.ResponseWriter, r *http.Request) {
	ym, err := resolveYM(h.DB, r.URL.Query().Get("ym"))
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	flt := parseFlowFilter(r, ym)

	// v0.60: 双轨路由 — 没 SKU 过滤走物化表 (7s → <50ms)
	var rawSQL string
	var args []interface{}
	if flt.canUseSummary(h.DB, ym) {
		var sumWhere string
		sumWhere, args = flt.buildSummaryWhere(ym)
		rawSQL = `SELECT warehouse_name, province, SUM(orders), SUM(packages)
			FROM warehouse_flow_summary WHERE 1=1` + sumWhere +
			` GROUP BY warehouse_name, province`
	} else {
		var where string
		where, args = flt.buildWhere()
		tradeT := "trade_" + ym
		goodsT := "trade_goods_" + ym
		pkgT := "trade_package_" + ym
		joins := flt.buildJoins(tradeT, goodsT, pkgT)
		rawSQL = fmt.Sprintf(`
			SELECT t.warehouse_name, (%s) AS prov, %s
			%s WHERE 1=1 %s
			GROUP BY t.warehouse_name, prov`, provinceNormSQL, allMetricsSelect, joins, where)
	}

	rs, err := h.DB.Query(rawSQL, args...)
	if err != nil {
		writeServerError(w, 500, "查询仓库流向失败", err)
		return
	}
	defer rs.Close()

	type cellRow struct {
		Warehouse string `json:"warehouse"`
		Province  string `json:"province"`
		Orders    int64  `json:"orders"`
		Packages  int64  `json:"packages"`
	}
	cells := []cellRow{}
	for rs.Next() {
		var c cellRow
		if rs.Scan(&c.Warehouse, &c.Province, &c.Orders, &c.Packages) != nil {
			continue
		}
		cells = append(cells, c)
	}

	writeJSON(w, map[string]interface{}{
		"ym":    ym[:4] + "-" + ym[4:],
		"cells": cells,
	})
}

