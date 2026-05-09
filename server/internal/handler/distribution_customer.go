package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ============================================
// 分销客户名单 + 客户分析 (v1.29)
// 表 distribution_high_value_customers (v1.28 全量入库 59081 客户)
// ============================================

type distributionCustomer struct {
	ID            int64   `json:"id"`
	CustomerCode  string  `json:"customerCode"`
	CustomerName  string  `json:"customerName"`
	Grade         string  `json:"grade"`
	Remark        string  `json:"remark"`
	FirstOrderAt  string  `json:"firstOrderAt"`
	LastOrderAt   string  `json:"lastOrderAt"`
	TotalAmount   float64 `json:"totalAmount"`
	TotalOrders   int     `json:"totalOrders"`
	CreatedBy     string  `json:"createdBy"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}

// ListDistributionCustomers GET /api/distribution/customers/list
// 参数: page, pageSize, search(客户名/编码模糊), grade(S/A/all/none), sortBy, sortOrder
func (h *DashboardHandler) ListDistributionCustomers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	search := strings.TrimSpace(q.Get("search"))
	grade := strings.TrimSpace(q.Get("grade"))
	sortBy := q.Get("sortBy")
	if sortBy == "" {
		sortBy = "total_amount"
	}
	// 白名单防 SQL 注入
	allowedSort := map[string]bool{"total_amount": true, "total_orders": true, "last_order_at": true, "first_order_at": true, "created_at": true}
	if !allowedSort[sortBy] {
		sortBy = "total_amount"
	}
	order := strings.ToLower(q.Get("sortOrder"))
	if order != "asc" {
		order = "desc"
	}

	conds := []string{"1=1"}
	args := []interface{}{}
	if search != "" {
		conds = append(conds, "(customer_name LIKE ? OR customer_code LIKE ?)")
		kw := "%" + search + "%"
		args = append(args, kw, kw)
	}
	if grade == "none" {
		conds = append(conds, "(grade IS NULL OR grade='')")
	} else if grade != "" && grade != "all" {
		conds = append(conds, "grade = ?")
		args = append(args, grade)
	}
	where := strings.Join(conds, " AND ")

	var total int
	if writeDatabaseError(w, h.DB.QueryRow("SELECT COUNT(*) FROM distribution_high_value_customers WHERE "+where, args...).Scan(&total)) {
		return
	}

	offset := (page - 1) * pageSize
	querySQL := fmt.Sprintf(`SELECT id, customer_code, customer_name, IFNULL(grade,''), IFNULL(remark,''),
		IFNULL(DATE_FORMAT(first_order_at,'%%Y-%%m-%%d'),''), IFNULL(DATE_FORMAT(last_order_at,'%%Y-%%m-%%d'),''),
		IFNULL(total_amount,0), IFNULL(total_orders,0),
		created_by, DATE_FORMAT(created_at,'%%Y-%%m-%%d %%H:%%i:%%s'), DATE_FORMAT(updated_at,'%%Y-%%m-%%d %%H:%%i:%%s')
		FROM distribution_high_value_customers WHERE %s
		ORDER BY %s %s, id ASC LIMIT ? OFFSET ?`, where, sortBy, order)
	args = append(args, pageSize, offset)
	rows, err := h.DB.Query(querySQL, args...)
	if writeDatabaseError(w, err) {
		return
	}
	defer rows.Close()

	list := []distributionCustomer{}
	for rows.Next() {
		var c distributionCustomer
		if err := rows.Scan(&c.ID, &c.CustomerCode, &c.CustomerName, &c.Grade, &c.Remark,
			&c.FirstOrderAt, &c.LastOrderAt, &c.TotalAmount, &c.TotalOrders,
			&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			continue
		}
		list = append(list, c)
	}

	writeJSON(w, map[string]interface{}{
		"list":     list,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// SetDistributionCustomerGrade POST /api/distribution/customers/grade
// body: {customerCode, grade, remark} — grade 传空字符串表示清除等级
func (h *DashboardHandler) SetDistributionCustomerGrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, 405, "method not allowed")
		return
	}
	var req struct {
		CustomerCode string `json:"customerCode"`
		Grade        string `json:"grade"`
		Remark       string `json:"remark"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "请求体格式错误")
		return
	}
	if req.CustomerCode == "" {
		writeError(w, 400, "客户编码必填")
		return
	}
	if req.Grade != "" && req.Grade != "S" && req.Grade != "A" && req.Grade != "SA" {
		writeError(w, 400, "等级只能是 S/A/SA (留空表示清除)")
		return
	}

	var grade interface{}
	if req.Grade == "" {
		grade = nil
	} else {
		grade = req.Grade
	}

	res, err := h.DB.Exec(
		"UPDATE distribution_high_value_customers SET grade=?, remark=? WHERE customer_code=?",
		grade, req.Remark, req.CustomerCode,
	)
	if writeDatabaseError(w, err) {
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, 404, "客户不存在")
		return
	}
	writeJSON(w, map[string]interface{}{"message": "更新成功"})
}

// BatchSetDistributionCustomerGrade POST /api/distribution/customers/grade-batch
// body: {items: [{customerCode, grade}], remark?}
func (h *DashboardHandler) BatchSetDistributionCustomerGrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, 405, "method not allowed")
		return
	}
	var req struct {
		Items []struct {
			CustomerCode string `json:"customerCode"`
			Grade        string `json:"grade"`
		} `json:"items"`
		Remark string `json:"remark"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "请求体格式错误")
		return
	}
	if len(req.Items) == 0 {
		writeError(w, 400, "items 不能为空")
		return
	}

	tx, err := h.DB.Begin()
	if writeDatabaseError(w, err) {
		return
	}
	defer tx.Rollback()

	updated := 0
	notFound := []string{}
	for _, item := range req.Items {
		if item.CustomerCode == "" {
			continue
		}
		if item.Grade != "" && item.Grade != "S" && item.Grade != "A" && item.Grade != "SA" {
			continue
		}
		var grade interface{}
		if item.Grade == "" {
			grade = nil
		} else {
			grade = item.Grade
		}
		res, err := tx.Exec(
			"UPDATE distribution_high_value_customers SET grade=?, remark=COALESCE(NULLIF(?,''), remark) WHERE customer_code=?",
			grade, req.Remark, item.CustomerCode,
		)
		if err != nil {
			writeDatabaseError(w, err)
			return
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			updated++
		} else {
			notFound = append(notFound, item.CustomerCode)
		}
	}

	if writeDatabaseError(w, tx.Commit()) {
		return
	}
	writeJSON(w, map[string]interface{}{
		"updated":  updated,
		"notFound": notFound,
	})
}

// DistributionCustomerAnalysisKPI GET /api/distribution/customer-analysis/kpi
// 参数: startDate, endDate (默认本月)
// 返回: 高价值客户数 + 高价值贡献销售额 + 分销总销售额 + 占比 + 月环比
func (h *DashboardHandler) DistributionCustomerAnalysisKPI(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	startDate := q.Get("startDate")
	endDate := q.Get("endDate")
	if startDate == "" || endDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	}

	// 查询当期 + 上期(用于月环比), 跨多张 trade_YYYYMM 月表
	curTotal, curHV, _ := h.queryDistributionPeriodAmount(startDate, endDate)
	prevStart, prevEnd := previousPeriod(startDate, endDate)
	_, prevHV, _ := h.queryDistributionPeriodAmount(prevStart, prevEnd)

	// 名单中高价值客户数(全量, 不依赖时间)
	var hvCustomerCount int
	h.DB.QueryRow("SELECT COUNT(*) FROM distribution_high_value_customers WHERE grade IN ('S','A','SA')").Scan(&hvCustomerCount)

	hvShare := 0.0
	if curTotal > 0 {
		hvShare = curHV / curTotal * 100
	}
	momChange := 0.0
	if prevHV > 0 {
		momChange = (curHV - prevHV) / prevHV * 100
	}

	writeJSON(w, map[string]interface{}{
		"hvCustomers":  hvCustomerCount,
		"hvAmount":     curHV,
		"totalAmount":  curTotal,
		"hvShare":      hvShare,
		"momChange":    momChange,
		"startDate":    startDate,
		"endDate":      endDate,
	})
}

// queryDistributionPeriodAmount 跨多张 trade 月表查询分销期内总销售额 + 高价值客户销售额
func (h *DashboardHandler) queryDistributionPeriodAmount(startDate, endDate string) (total, hv float64, err error) {
	months := monthsBetween(startDate, endDate)
	for _, ym := range months {
		var t, v float64
		err = h.DB.QueryRow(fmt.Sprintf(`
			SELECT IFNULL(SUM(t.payment),0),
			       IFNULL(SUM(CASE WHEN d.grade IN ('S','A','SA') THEN t.payment ELSE 0 END),0)
			FROM trade_%s t
			INNER JOIN sales_channel sc ON sc.channel_name=t.shop_name
			LEFT JOIN distribution_high_value_customers d ON d.customer_code=t.customer_code
			WHERE sc.department='distribution'
			  AND t.consign_time >= ? AND t.consign_time < DATE_ADD(?, INTERVAL 1 DAY)
		`, ym), startDate, endDate).Scan(&t, &v)
		if err == nil {
			total += t
			hv += v
		}
	}
	return total, hv, nil
}

// DistributionHVCustomerList GET /api/distribution/customer-analysis/list
// 仅高价值客户(grade S/A) + 期间销售额排名
// 参数: startDate, endDate, page, pageSize
func (h *DashboardHandler) DistributionHVCustomerList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	startDate := q.Get("startDate")
	endDate := q.Get("endDate")
	if startDate == "" || endDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 跨多月聚合每个高价值客户的期间销售额, 内存合并
	months := monthsBetween(startDate, endDate)
	agg := map[string]struct {
		amount float64
		orders int
		name   string
		grade  string
	}{}

	// 先加载所有 grade S/A 的客户基础信息
	rows, err := h.DB.Query("SELECT customer_code, customer_name, grade FROM distribution_high_value_customers WHERE grade IN ('S','A','SA')")
	if writeDatabaseError(w, err) {
		return
	}
	for rows.Next() {
		var code, name, grade string
		rows.Scan(&code, &name, &grade)
		agg[code] = struct {
			amount float64
			orders int
			name   string
			grade  string
		}{0, 0, name, grade}
	}
	rows.Close()

	if len(agg) == 0 {
		writeJSON(w, map[string]interface{}{
			"list":     []interface{}{},
			"total":    0,
			"page":     page,
			"pageSize": pageSize,
		})
		return
	}

	// 跨月累加, 只统计已在 agg 中的客户(高价值名单)
	for _, ym := range months {
		mrows, err := h.DB.Query(fmt.Sprintf(`
			SELECT t.customer_code, IFNULL(SUM(t.payment),0), COUNT(*)
			FROM trade_%s t
			INNER JOIN sales_channel sc ON sc.channel_name=t.shop_name
			INNER JOIN distribution_high_value_customers d ON d.customer_code=t.customer_code
			WHERE sc.department='distribution' AND d.grade IN ('S','A','SA')
			  AND t.consign_time >= ? AND t.consign_time < DATE_ADD(?, INTERVAL 1 DAY)
			GROUP BY t.customer_code
		`, ym), startDate, endDate)
		if err != nil {
			continue
		}
		for mrows.Next() {
			var code string
			var amt float64
			var cnt int
			mrows.Scan(&code, &amt, &cnt)
			if v, ok := agg[code]; ok {
				v.amount += amt
				v.orders += cnt
				agg[code] = v
			}
		}
		mrows.Close()
	}

	// 排序
	type item struct {
		CustomerCode string  `json:"customerCode"`
		CustomerName string  `json:"customerName"`
		Grade        string  `json:"grade"`
		Amount       float64 `json:"amount"`
		Orders       int     `json:"orders"`
	}
	all := make([]item, 0, len(agg))
	for code, v := range agg {
		all = append(all, item{
			CustomerCode: code,
			CustomerName: v.name,
			Grade:        v.grade,
			Amount:       v.amount,
			Orders:       v.orders,
		})
	}
	// 销售额降序
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].Amount > all[i].Amount {
				all[i], all[j] = all[j], all[i]
			}
		}
	}

	total := len(all)
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	writeJSON(w, map[string]interface{}{
		"list":     all[start:end],
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// DistributionCustomerMonthly GET /api/distribution/customer-analysis/monthly
// 单客户跨月销售时序 (同时支撑月趋势 + 历年对比, 前端自行拆分按年展示)
// 参数: customerCode, startMonth(yyyy-MM), endMonth
func (h *DashboardHandler) DistributionCustomerMonthly(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	code := strings.TrimSpace(q.Get("customerCode"))
	if code == "" {
		writeError(w, 400, "customerCode 必填")
		return
	}
	startMonth := q.Get("startMonth")
	endMonth := q.Get("endMonth")
	if startMonth == "" {
		startMonth = "2025-01"
	}
	if endMonth == "" {
		endMonth = time.Now().Format("2006-01")
	}

	months := []string{}
	cur, _ := time.Parse("2006-01", startMonth)
	endT, _ := time.Parse("2006-01", endMonth)
	for !cur.After(endT) {
		months = append(months, cur.Format("200601"))
		cur = cur.AddDate(0, 1, 0)
	}

	type monthRow struct {
		Month  string  `json:"month"`
		Amount float64 `json:"amount"`
		Orders int     `json:"orders"`
	}
	results := []monthRow{}
	for _, ym := range months {
		var amt float64
		var cnt int
		h.DB.QueryRow(fmt.Sprintf(
			`SELECT IFNULL(SUM(payment),0), COUNT(*) FROM trade_%s WHERE customer_code=?`,
			ym,
		), code).Scan(&amt, &cnt)
		// 月份显示格式 yyyy-MM
		displayMonth := ym[:4] + "-" + ym[4:]
		results = append(results, monthRow{Month: displayMonth, Amount: amt, Orders: cnt})
	}

	// 客户基础信息
	var name, grade string
	h.DB.QueryRow("SELECT customer_name, IFNULL(grade,'') FROM distribution_high_value_customers WHERE customer_code=?", code).Scan(&name, &grade)

	writeJSON(w, map[string]interface{}{
		"customerCode": code,
		"customerName": name,
		"grade":        grade,
		"months":       results,
	})
}

// monthsBetween 生成 startDate-endDate 涉及的所有 yyyymm
func monthsBetween(startDate, endDate string) []string {
	startT, err1 := time.Parse("2006-01-02", startDate)
	endT, err2 := time.Parse("2006-01-02", endDate)
	if err1 != nil || err2 != nil {
		return nil
	}
	months := []string{}
	cur := time.Date(startT.Year(), startT.Month(), 1, 0, 0, 0, 0, time.Local)
	for !cur.After(endT) {
		months = append(months, cur.Format("200601"))
		cur = cur.AddDate(0, 1, 0)
	}
	return months
}

// previousPeriod 给定时间段, 返回同长度的上期
func previousPeriod(startDate, endDate string) (string, string) {
	s, _ := time.Parse("2006-01-02", startDate)
	e, _ := time.Parse("2006-01-02", endDate)
	days := int(e.Sub(s).Hours()/24) + 1
	prevEnd := s.AddDate(0, 0, -1)
	prevStart := prevEnd.AddDate(0, 0, -(days - 1))
	return prevStart.Format("2006-01-02"), prevEnd.Format("2006-01-02")
}

// DistributionCustomerSkus GET /api/distribution/customer-analysis/skus
// 单客户期间 SKU 销售明细 (商品维度聚合, 销售额降序)
// 参数: customerCode, startDate(yyyy-MM-dd), endDate (跟主页 DateFilter 一致)
func (h *DashboardHandler) DistributionCustomerSkus(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	code := strings.TrimSpace(q.Get("customerCode"))
	if code == "" {
		writeError(w, 400, "customerCode 必填")
		return
	}
	startDate := q.Get("startDate")
	endDate := q.Get("endDate")
	if startDate == "" || endDate == "" {
		now := time.Now()
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	}
	months := monthsBetween(startDate, endDate)

	type skuAgg struct {
		goodsNo    string
		goodsName  string
		qty        float64
		amount     float64
		orderCount int
		isPackage  int // 0 单品 / 1 组合装 (来自 goods.is_package_good)
	}
	agg := map[string]*skuAgg{}

	for _, ym := range months {
		// 跨月 trade_goods JOIN trade 按 customer_code + 时间区间 聚合 SKU
		// LEFT JOIN goods 取 is_package_good 标记组合装/单品
		rows, err := h.DB.Query(fmt.Sprintf(`
			SELECT IFNULL(tg.goods_no,''), IFNULL(tg.goods_name,''),
			       IFNULL(SUM(tg.sell_count),0), IFNULL(SUM(tg.sell_total),0),
			       COUNT(DISTINCT tg.trade_id),
			       IFNULL(MAX(g.is_package_good),0) AS is_package
			FROM trade_goods_%s tg
			INNER JOIN trade_%s t ON t.trade_id=tg.trade_id
			LEFT JOIN goods g ON g.goods_no=tg.goods_no
			WHERE t.customer_code = ?
			  AND t.consign_time >= ? AND t.consign_time < DATE_ADD(?, INTERVAL 1 DAY)
			GROUP BY tg.goods_no, tg.goods_name
		`, ym, ym), code, startDate, endDate)
		if err != nil {
			continue
		}
		for rows.Next() {
			var no, name string
			var qty, amt float64
			var oc, pkg int
			rows.Scan(&no, &name, &qty, &amt, &oc, &pkg)
			key := no + "|" + name
			if v, ok := agg[key]; ok {
				v.qty += qty
				v.amount += amt
				v.orderCount += oc
				if pkg > v.isPackage {
					v.isPackage = pkg
				}
			} else {
				agg[key] = &skuAgg{goodsNo: no, goodsName: name, qty: qty, amount: amt, orderCount: oc, isPackage: pkg}
			}
		}
		rows.Close()
	}

	type childRow struct {
		ChildGoodsNo   string  `json:"childGoodsNo"`
		ChildGoodsName string  `json:"childGoodsName"`
		ChildSpecName  string  `json:"childSpecName"`
		GoodsAmount    float64 `json:"goodsAmount"`
		UnitName       string  `json:"unitName"`
		ShareAmount    float64 `json:"shareAmount"`
	}
	type skuRow struct {
		GoodsNo    string     `json:"goodsNo"`
		GoodsName  string     `json:"goodsName"`
		Qty        float64    `json:"qty"`
		Amount     float64    `json:"amount"`
		OrderCount int        `json:"orderCount"`
		IsPackage  int        `json:"isPackage"` // 1=组合装, 0=单品
		PackageChildren []childRow `json:"packageChildren"` // 组合装子件 (字段名避开 antd Table 自动 TreeData 识别)
	}
	list := make([]skuRow, 0, len(agg))
	for _, v := range agg {
		row := skuRow{
			GoodsNo:    v.goodsNo,
			GoodsName:  v.goodsName,
			Qty:        v.qty,
			Amount:     v.amount,
			OrderCount: v.orderCount,
			IsPackage:  v.isPackage,
			PackageChildren: []childRow{},
		}
		// 组合装查 BOM 子件
		if v.isPackage == 1 {
			cRows, cErr := h.DB.Query(`
				SELECT IFNULL(child_goods_no,''), IFNULL(child_goods_name,''),
				       IFNULL(child_spec_name,''), IFNULL(goods_amount,0),
				       IFNULL(unit_name,''), IFNULL(share_amount,0)
				FROM goods_blend_detail WHERE parent_goods_no=?`, v.goodsNo)
			if cErr == nil {
				for cRows.Next() {
					var ch childRow
					if scanErr := cRows.Scan(&ch.ChildGoodsNo, &ch.ChildGoodsName, &ch.ChildSpecName,
						&ch.GoodsAmount, &ch.UnitName, &ch.ShareAmount); scanErr == nil {
						row.PackageChildren = append(row.PackageChildren, ch)
					}
				}
				cRows.Close()
			}
		}
		list = append(list, row)
	}
	// 销售额降序
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].Amount > list[i].Amount {
				list[i], list[j] = list[j], list[i]
			}
		}
	}

	writeJSON(w, map[string]interface{}{
		"customerCode": code,
		"startDate":    startDate,
		"endDate":      endDate,
		"list":         list,
	})
}
