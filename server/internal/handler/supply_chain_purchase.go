package handler

import (
	"database/sql"
	"log"
	"net/http"
)

func (h *DashboardHandler) GetPurchasePlan(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDomainAccess(r, "supply_chain")) {
		return
	}

	// === 1. KPI 4 个 ===
	type kpiData struct {
		UrgentSKU            int     `json:"urgentSku"`
		InTransitOrders      int     `json:"inTransitOrders"`
		InTransitSubcontract int     `json:"inTransitSubcontract"`
		Recent30Amount       float64 `json:"recent30Amount"`
	}
	var kpi kpiData

	// 紧急 SKU 数 (成品可售天数 < 7) — 必须按 SKU 聚合再算, 不能 row-level
	// row-level 会因为同 SKU 散在多仓而把虚高计数, 实际全公司库存充裕也会算紧急
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM (
		SELECT goods_no, SUM(current_qty - locked_qty) AS stock_total, SUM(month_qty)/30 AS daily_avg
		FROM stock_quantity
		WHERE goods_attr=1 AND month_qty > 0
		GROUP BY goods_no
		HAVING stock_total > 0 AND stock_total / daily_avg < 7
	) t`).Scan(&kpi.UrgentSKU); err != nil {
		log.Printf("kpi urgent: %v", err)
	}

	// 在途采购订单数 (v0.52: 排除安徽香松组织)
	h.DB.QueryRow(`SELECT COUNT(DISTINCT id) FROM ys_purchase_orders
		WHERE purchase_orders_in_wh_status IN (2,3)` + excludeAnhuiOrgWHERE).Scan(&kpi.InTransitOrders)

	// 在途委外订单数 (v0.52: 排除安徽香松组织)
	h.DB.QueryRow(`SELECT COUNT(DISTINCT id) FROM ys_subcontract_orders
		WHERE status NOT IN (2)` + excludeAnhuiOrgWHERE).Scan(&kpi.InTransitSubcontract)

	// 最近 30 天采购金额 (相对 DB 内 MAX(vouchdate) 滚动) (v0.52: 排除安徽香松)
	var amt sql.NullFloat64
	h.DB.QueryRow(`SELECT SUM(ori_sum) FROM ys_purchase_orders
		WHERE vouchdate >= DATE_SUB((SELECT MAX(vouchdate) FROM ys_purchase_orders), INTERVAL 30 DAY)` + excludeAnhuiOrgWHERE).Scan(&amt)
	kpi.Recent30Amount = amt.Float64

	// === 2. 月度趋势 (近 6 个月采购金额) ===
	type monthRow struct {
		Month  string  `json:"month"`
		Amount float64 `json:"amount"`
	}
	monthlyTrend := []monthRow{}
	mRows, _ := h.DB.Query(`SELECT DATE_FORMAT(vouchdate, '%Y-%m') AS month,
		ROUND(SUM(ori_sum), 0) AS amount
		FROM ys_purchase_orders
		WHERE vouchdate >= DATE_SUB((SELECT MAX(vouchdate) FROM ys_purchase_orders), INTERVAL 6 MONTH)` + excludeAnhuiOrgWHERE + `
		GROUP BY DATE_FORMAT(vouchdate, '%Y-%m') ORDER BY month`)
	if mRows != nil {
		for mRows.Next() {
			var m monthRow
			if err := mRows.Scan(&m.Month, &m.Amount); err == nil {
				monthlyTrend = append(monthlyTrend, m)
			}
		}
		mRows.Close()
	}

	// === 3. TOP 10 供应商 (按采购金额) ===
	type vendorRow struct {
		VendorName string  `json:"vendorName"`
		Amount     float64 `json:"amount"`
		OrderCount int     `json:"orderCount"`
	}
	topVendors := []vendorRow{}
	vRows, _ := h.DB.Query(`SELECT vendor_name, ROUND(SUM(ori_sum), 0) AS amount,
		COUNT(DISTINCT id) AS order_count
		FROM ys_purchase_orders WHERE vendor_name IS NOT NULL` + excludeAnhuiOrgWHERE + `
		GROUP BY vendor_name ORDER BY amount DESC LIMIT 10`)
	if vRows != nil {
		for vRows.Next() {
			var v vendorRow
			if err := vRows.Scan(&v.VendorName, &v.Amount, &v.OrderCount); err == nil {
				topVendors = append(topVendors, v)
			}
		}
		vRows.Close()
	}

	// === 4. 建议采购清单 (UNION 成品 + 包材, 按建议量倒序) ===
	// v0.51: 在途量按 recieve_date <= today+90天 过滤 (远期/超期排除); 加 nextArriveDate 显示最近到货
	// 编码两套并存: jkyCode + ysCode 通过 goods.sku_code 映射
	// v0.62 改: 成品段限定 7 仓白名单(planWarehouses), 不含京东/天猫超市/朴朴外仓+采购外仓+不合格仓
	//          展示全部 SKU(去掉 HAVING > 0 过滤), 跑哥要核对
	planSqWhCond, planSqWhArgs := buildPlanWarehouseFilter("sq.warehouse_name")
	prodExclCond, prodExclArgs := buildExcludeGoodsFilter("sq.goods_no")
	type suggestRow struct {
		Type                 string  `json:"type"`    // 成品 / 包材
		JkyCode              string  `json:"jkyCode"` // 吉客云编码
		YsCode               string  `json:"ysCode"`  // 用友编码
		GoodsName            string  `json:"goodsName"`
		Stock                float64 `json:"stock"`
		DailyAvg             float64 `json:"dailyAvg"`
		InTransit            float64 `json:"inTransit"`            // 在途采购量
		InTransitSubcontract float64 `json:"inTransitSubcontract"` // v0.54: 在途委外量 (委外加工未完工)
		Status               string  `json:"status"`         // 紧急 / 偏低 / 正常 / 积压
		SellableDays         float64 `json:"sellableDays"`   // 可售天数
		NextArriveDate       string  `json:"nextArriveDate"` // 最近一笔在途(采购+委外)到货日期
		NextArriveDays       int     `json:"nextArriveDays"` // 距今天数 (负=已逾期, NULL→999)
		YsClassName          string  `json:"ysClassName"`    // YS 分类(固态/液态/标签/纸箱 等)
		Position             string  `json:"position"`       // v0.81: 产品定位 (S/A/B/C/D)
		CateName             string  `json:"cateName"`       // v0.81: 吉客云分类
	}
	suggested := []suggestRow{}

	// 4a. 成品 (goods_attr=1, 目标 45 天) — 主表 stock_quantity, 通过 goods.sku_code (吉客云外部编码) 映射 YS 编码
	// v0.54: 加在途委外量 (sc 子查询) + next_arrive 综合采购+委外两种到货
	// 公式: max(0, 45 × 吉客云日均 - 吉客云库存 - YS 在途采购 - YS 在途委外)
	prodSQL := `SELECT '成品/半成品' AS t,
		sq.goods_no AS jky_code,
		IFNULL(MAX(gm.ys_code), '') AS ys_code,
		sq.goods_name,
		ROUND(SUM(sq.current_qty - sq.locked_qty), 0) AS stock,
		ROUND(SUM(sq.month_qty)/30, 1) AS daily_avg,
		IFNULL(ROUND(MAX(po.in_transit_qty), 0), 0) AS in_transit,
		IFNULL(ROUND(MAX(sc.in_transit_qty), 0), 0) AS in_transit_subcontract,
		COALESCE(NULLIF(MAX(gm.ys_class_name), ''), MAX(ys_direct.direct_class_name), '') AS ys_class_name,
		CASE
		  WHEN SUM(sq.month_qty) > 0 AND (SUM(sq.current_qty - sq.locked_qty)) <= 0 THEN -1
		  WHEN SUM(sq.month_qty) > 0 THEN ROUND(SUM(sq.current_qty - sq.locked_qty) / (SUM(sq.month_qty)/30), 1)
		  ELSE 9999 END AS sellable_days,
		CASE WHEN LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')) = '9999-12-31' THEN ''
		     ELSE DATE_FORMAT(LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')), '%Y-%m-%d') END AS next_arrive_date,
		CASE WHEN LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')) = '9999-12-31' THEN 999
		     ELSE DATEDIFF(LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')), CURDATE()) END AS next_arrive_days,
		IFNULL(MAX(gm.position), '') AS position,
		IFNULL(MAX(gm.cate_name), '') AS cate_name
		FROM stock_quantity sq
		LEFT JOIN (
		  -- v0.54 fix: ys_purchase_orders.product_c_code 是 YS 编码, 必须通过 goods.sku_code 桥接到吉客云 goods_no
		  SELECT g.goods_no AS jky_no, SUM(p.qty - IFNULL(p.total_in_qty, 0)) AS in_transit_qty
		  FROM ys_purchase_orders p JOIN goods g ON g.sku_code = p.product_c_code
		  WHERE p.purchase_orders_in_wh_status IN (2,3) AND p.qty > IFNULL(p.total_in_qty, 0)
		    AND (p.recieve_date IS NULL OR p.recieve_date <= DATE_ADD(CURDATE(), INTERVAL 90 DAY))
		    AND p.org_name != '安徽香松自然调味品有限公司'
		  GROUP BY g.goods_no
		) po ON po.jky_no = sq.goods_no
		LEFT JOIN (
		  SELECT g.goods_no AS jky_no, MIN(IFNULL(p.recieve_date, DATE_ADD(p.vouchdate, INTERVAL 30 DAY))) AS next_arrive
		  FROM ys_purchase_orders p JOIN goods g ON g.sku_code = p.product_c_code
		  WHERE p.purchase_orders_in_wh_status IN (2,3) AND p.qty > IFNULL(p.total_in_qty, 0)
		    AND p.org_name != '安徽香松自然调味品有限公司'
		  GROUP BY g.goods_no
		) po_arr ON po_arr.jky_no = sq.goods_no
		LEFT JOIN (
		  SELECT g.goods_no AS jky_no,
		    SUM(s.order_product_subcontract_quantity_mu - IFNULL(s.order_product_incoming_quantity, 0)) AS in_transit_qty
		  FROM ys_subcontract_orders s JOIN goods g ON g.sku_code = s.order_product_material_code
		  WHERE s.status NOT IN (2)
		    AND s.order_product_subcontract_quantity_mu > IFNULL(s.order_product_incoming_quantity, 0)
		    AND (s.order_product_delivery_date IS NULL OR s.order_product_delivery_date <= DATE_ADD(CURDATE(), INTERVAL 90 DAY))
		    AND s.org_name != '安徽香松自然调味品有限公司'
		  GROUP BY g.goods_no
		) sc ON sc.jky_no = sq.goods_no
		LEFT JOIN (
		  SELECT g.goods_no AS jky_no,
		    MIN(IFNULL(s.order_product_delivery_date, DATE_ADD(s.vouchdate, INTERVAL 30 DAY))) AS next_arrive
		  FROM ys_subcontract_orders s JOIN goods g ON g.sku_code = s.order_product_material_code
		  WHERE s.status NOT IN (2)
		    AND s.order_product_subcontract_quantity_mu > IFNULL(s.order_product_incoming_quantity, 0)
		    AND s.org_name != '安徽香松自然调味品有限公司'
		  GROUP BY g.goods_no
		) sc_arr ON sc_arr.jky_no = sq.goods_no
		LEFT JOIN (
		  SELECT g.goods_no,
		    MAX(NULLIF(g.sku_code,'')) AS ys_code,
		    MAX(yc.manage_class_name) AS ys_class_name,
		    MAX(yc.manage_class_code) AS ys_class_code,
		    MAX(NULLIF(g.goods_field7,'')) AS position,
		    MAX(NULLIF(g.cate_name,'')) AS cate_name
		  FROM goods g
		  LEFT JOIN (SELECT product_code,
		                    MAX(manage_class_name) AS manage_class_name,
		                    MAX(manage_class_code) AS manage_class_code
		             FROM ys_stock GROUP BY product_code) yc ON yc.product_code = g.sku_code
		  WHERE g.sku_code IS NOT NULL AND g.sku_code != '' GROUP BY g.goods_no
		) gm ON gm.goods_no = sq.goods_no
		LEFT JOIN (
		  -- v0.65 直连兜底: 当 goods.sku_code 缺失时, 用 sq.goods_no 直接对 ys_stock.product_code
		  SELECT product_code,
		    MAX(manage_class_name) AS direct_class_name,
		    MAX(manage_class_code) AS direct_class_code
		  FROM ys_stock GROUP BY product_code
		) ys_direct ON ys_direct.product_code = sq.goods_no
		WHERE sq.goods_attr = 1 AND sq.month_qty > 0
		  AND IFNULL(gm.ys_class_code, '') NOT LIKE '05%'
		  AND IFNULL(ys_direct.direct_class_code, '') NOT LIKE '05%'` + planSqWhCond + prodExclCond + `
		GROUP BY sq.goods_no, sq.goods_name`

	// 4b. 包材/原料 (v0.49 改用 YS 现存量) — 主表 ys_stock, 反向通过 goods.sku_code 映射回吉客云 goods_no
	// v0.54: 加在途委外 (sc) 子查询 + next_arrive 综合采购+委外
	// 公式: max(0, 90 × YS日均 - YS库存 - YS在途采购 - YS在途委外)
	matSQL := `SELECT '原材料/包材' AS t,
		IFNULL(MAX(gm.goods_no), '') AS jky_code,
		ys.product_code AS ys_code,
		MAX(ys.product_name) AS goods_name,
		ROUND(SUM(ys.currentqty), 0) AS stock,
		ROUND(IFNULL(MAX(mo.daily_avg), 0), 1) AS daily_avg,
		IFNULL(ROUND(MAX(po.in_transit_qty), 0), 0) AS in_transit,
		IFNULL(ROUND(MAX(sc.in_transit_qty), 0), 0) AS in_transit_subcontract,
		IFNULL(MAX(ys.manage_class_name), '') AS ys_class_name,
		CASE
		  WHEN IFNULL(MAX(mo.daily_avg), 0) > 0 AND SUM(ys.currentqty) <= 0 THEN -1
		  WHEN IFNULL(MAX(mo.daily_avg), 0) > 0 THEN ROUND(SUM(ys.currentqty) / MAX(mo.daily_avg), 1)
		  ELSE 9999 END AS sellable_days,
		CASE WHEN LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')) = '9999-12-31' THEN ''
		     ELSE DATE_FORMAT(LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')), '%Y-%m-%d') END AS next_arrive_date,
		CASE WHEN LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')) = '9999-12-31' THEN 999
		     ELSE DATEDIFF(LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')), CURDATE()) END AS next_arrive_days,
		IFNULL(MAX(gm.position), '') AS position,
		IFNULL(MAX(gm.cate_name), '') AS cate_name
		FROM ys_stock ys
		LEFT JOIN (
		  SELECT product_c_code, SUM(qty)/30 AS daily_avg FROM ys_material_out
		  WHERE vouchdate >= DATE_SUB((SELECT MAX(vouchdate) FROM ys_material_out), INTERVAL 30 DAY)` + excludeAnhuiOrgWHERE + `
		  GROUP BY product_c_code
		) mo ON mo.product_c_code = ys.product_code
		LEFT JOIN (
		  SELECT product_c_code, SUM(qty - IFNULL(total_in_qty, 0)) AS in_transit_qty
		  FROM ys_purchase_orders
		  WHERE purchase_orders_in_wh_status IN (2,3) AND qty > IFNULL(total_in_qty, 0)
		    AND (recieve_date IS NULL OR recieve_date <= DATE_ADD(CURDATE(), INTERVAL 90 DAY))` + excludeAnhuiOrgWHERE + `
		  GROUP BY product_c_code
		) po ON po.product_c_code = ys.product_code
		LEFT JOIN (
		  SELECT product_c_code, MIN(IFNULL(recieve_date, DATE_ADD(vouchdate, INTERVAL 30 DAY))) AS next_arrive
		  FROM ys_purchase_orders
		  WHERE purchase_orders_in_wh_status IN (2,3) AND qty > IFNULL(total_in_qty, 0)` + excludeAnhuiOrgWHERE + `
		  GROUP BY product_c_code
		) po_arr ON po_arr.product_c_code = ys.product_code
		LEFT JOIN (
		  SELECT order_product_material_code AS pcode,
		    SUM(order_product_subcontract_quantity_mu - IFNULL(order_product_incoming_quantity, 0)) AS in_transit_qty
		  FROM ys_subcontract_orders
		  WHERE status NOT IN (2)
		    AND order_product_subcontract_quantity_mu > IFNULL(order_product_incoming_quantity, 0)
		    AND (order_product_delivery_date IS NULL OR order_product_delivery_date <= DATE_ADD(CURDATE(), INTERVAL 90 DAY))` + excludeAnhuiOrgWHERE + `
		  GROUP BY order_product_material_code
		) sc ON sc.pcode = ys.product_code
		LEFT JOIN (
		  SELECT order_product_material_code AS pcode,
		    MIN(IFNULL(order_product_delivery_date, DATE_ADD(vouchdate, INTERVAL 30 DAY))) AS next_arrive
		  FROM ys_subcontract_orders
		  WHERE status NOT IN (2)
		    AND order_product_subcontract_quantity_mu > IFNULL(order_product_incoming_quantity, 0)` + excludeAnhuiOrgWHERE + `
		  GROUP BY order_product_material_code
		) sc_arr ON sc_arr.pcode = ys.product_code
		LEFT JOIN (
		  SELECT sku_code,
		    MAX(goods_no) AS goods_no,
		    MAX(NULLIF(goods_field7,'')) AS position,
		    MAX(NULLIF(cate_name,'')) AS cate_name
		  FROM goods
		  WHERE sku_code IS NOT NULL AND sku_code != '' GROUP BY sku_code
		) gm ON gm.sku_code = ys.product_code
		WHERE (ys.manage_class_code LIKE '01%' OR ys.manage_class_code LIKE '02%')` + excludeAnhuiOrgYsWHERE + `
		GROUP BY ys.product_code`

	// 4c. 其他 (含广宣品/周边品/物流易耗品/其它) — v0.64 新增
	// 跑哥指示: 用吉客云的库存和销量 (业务对广宣品的"消耗"走销售出库, 不走YS生产领料)
	// 公式: max(0, 45 × 吉客云日均 - 吉客云库存 - YS在途采购 - YS在途委外)
	// 范围: ys_stock manage_class_code LIKE '05%' 圈定 SKU, stock_quantity 取 7 仓白名单, 有月销
	otherPlanSqWhCond, otherPlanSqWhArgs := buildPlanWarehouseFilter("sq.warehouse_name")
	otherProdExclCond, otherProdExclArgs := buildExcludeGoodsFilter("sq.goods_no")
	otherSQL := `SELECT '其他' AS t,
		sq.goods_no AS jky_code,
		IFNULL(MAX(gm.ys_code), '') AS ys_code,
		sq.goods_name,
		ROUND(SUM(sq.current_qty - sq.locked_qty), 0) AS stock,
		ROUND(SUM(sq.month_qty)/30, 1) AS daily_avg,
		IFNULL(ROUND(MAX(po.in_transit_qty), 0), 0) AS in_transit,
		IFNULL(ROUND(MAX(sc.in_transit_qty), 0), 0) AS in_transit_subcontract,
		COALESCE(NULLIF(MAX(gm.ys_class_name), ''), MAX(ys_direct.direct_class_name), '') AS ys_class_name,
		CASE
		  WHEN SUM(sq.month_qty) > 0 AND (SUM(sq.current_qty - sq.locked_qty)) <= 0 THEN -1
		  WHEN SUM(sq.month_qty) > 0 THEN ROUND(SUM(sq.current_qty - sq.locked_qty) / (SUM(sq.month_qty)/30), 1)
		  ELSE 9999 END AS sellable_days,
		CASE WHEN LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')) = '9999-12-31' THEN ''
		     ELSE DATE_FORMAT(LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')), '%Y-%m-%d') END AS next_arrive_date,
		CASE WHEN LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')) = '9999-12-31' THEN 999
		     ELSE DATEDIFF(LEAST(IFNULL(MAX(po_arr.next_arrive), '9999-12-31'), IFNULL(MAX(sc_arr.next_arrive), '9999-12-31')), CURDATE()) END AS next_arrive_days,
		IFNULL(MAX(gm.position), '') AS position,
		IFNULL(MAX(gm.cate_name), '') AS cate_name
		FROM stock_quantity sq
		LEFT JOIN (
		  SELECT g.goods_no AS jky_no, SUM(p.qty - IFNULL(p.total_in_qty, 0)) AS in_transit_qty
		  FROM ys_purchase_orders p JOIN goods g ON g.sku_code = p.product_c_code
		  WHERE p.purchase_orders_in_wh_status IN (2,3) AND p.qty > IFNULL(p.total_in_qty, 0)
		    AND (p.recieve_date IS NULL OR p.recieve_date <= DATE_ADD(CURDATE(), INTERVAL 90 DAY))
		    AND p.org_name != '安徽香松自然调味品有限公司'
		  GROUP BY g.goods_no
		) po ON po.jky_no = sq.goods_no
		LEFT JOIN (
		  SELECT g.goods_no AS jky_no, MIN(IFNULL(p.recieve_date, DATE_ADD(p.vouchdate, INTERVAL 30 DAY))) AS next_arrive
		  FROM ys_purchase_orders p JOIN goods g ON g.sku_code = p.product_c_code
		  WHERE p.purchase_orders_in_wh_status IN (2,3) AND p.qty > IFNULL(p.total_in_qty, 0)
		    AND p.org_name != '安徽香松自然调味品有限公司'
		  GROUP BY g.goods_no
		) po_arr ON po_arr.jky_no = sq.goods_no
		LEFT JOIN (
		  SELECT g.goods_no AS jky_no,
		    SUM(s.order_product_subcontract_quantity_mu - IFNULL(s.order_product_incoming_quantity, 0)) AS in_transit_qty
		  FROM ys_subcontract_orders s JOIN goods g ON g.sku_code = s.order_product_material_code
		  WHERE s.status NOT IN (2)
		    AND s.order_product_subcontract_quantity_mu > IFNULL(s.order_product_incoming_quantity, 0)
		    AND (s.order_product_delivery_date IS NULL OR s.order_product_delivery_date <= DATE_ADD(CURDATE(), INTERVAL 90 DAY))
		    AND s.org_name != '安徽香松自然调味品有限公司'
		  GROUP BY g.goods_no
		) sc ON sc.jky_no = sq.goods_no
		LEFT JOIN (
		  SELECT g.goods_no AS jky_no,
		    MIN(IFNULL(s.order_product_delivery_date, DATE_ADD(s.vouchdate, INTERVAL 30 DAY))) AS next_arrive
		  FROM ys_subcontract_orders s JOIN goods g ON g.sku_code = s.order_product_material_code
		  WHERE s.status NOT IN (2)
		    AND s.order_product_subcontract_quantity_mu > IFNULL(s.order_product_incoming_quantity, 0)
		    AND s.org_name != '安徽香松自然调味品有限公司'
		  GROUP BY g.goods_no
		) sc_arr ON sc_arr.jky_no = sq.goods_no
		LEFT JOIN (
		  SELECT g.goods_no,
		    MAX(NULLIF(g.sku_code,'')) AS ys_code,
		    MAX(yc.manage_class_name) AS ys_class_name,
		    MAX(yc.manage_class_code) AS ys_class_code,
		    MAX(NULLIF(g.goods_field7,'')) AS position,
		    MAX(NULLIF(g.cate_name,'')) AS cate_name
		  FROM goods g
		  LEFT JOIN (SELECT product_code,
		                    MAX(manage_class_name) AS manage_class_name,
		                    MAX(manage_class_code) AS manage_class_code
		             FROM ys_stock GROUP BY product_code) yc ON yc.product_code = g.sku_code
		  WHERE g.sku_code IS NOT NULL AND g.sku_code != '' GROUP BY g.goods_no
		) gm ON gm.goods_no = sq.goods_no
		LEFT JOIN (
		  -- v0.65 直连兜底: 当 goods.sku_code 缺失时, 用 sq.goods_no 直接对 ys_stock.product_code
		  SELECT product_code,
		    MAX(manage_class_name) AS direct_class_name,
		    MAX(manage_class_code) AS direct_class_code
		  FROM ys_stock GROUP BY product_code
		) ys_direct ON ys_direct.product_code = sq.goods_no
		WHERE sq.month_qty > 0
		  AND (gm.ys_class_code LIKE '05%' OR ys_direct.direct_class_code LIKE '05%')` + otherPlanSqWhCond + otherProdExclCond + `
		GROUP BY sq.goods_no, sq.goods_name`

	type queryWithArgs struct {
		sql  string
		args []interface{}
	}
	prodArgs := append([]interface{}{}, planSqWhArgs...)
	prodArgs = append(prodArgs, prodExclArgs...)
	otherArgs := append([]interface{}{}, otherPlanSqWhArgs...)
	otherArgs = append(otherArgs, otherProdExclArgs...)
	for _, qa := range []queryWithArgs{
		{prodSQL, prodArgs},   // 成品/半成品 7 仓白名单 + 虚拟品排除 + 排除广宣品(05%)
		{matSQL, nil},         // 原材料/包材 YS 全仓 (限定 01%/02%)
		{otherSQL, otherArgs}, // 其他 7 仓白名单 + 限定广宣品(05%)
	} {
		sRows, err := h.DB.Query(qa.sql, qa.args...)
		if err != nil {
			log.Printf("suggest query err: %v", err)
			continue
		}
		for sRows.Next() {
			var s suggestRow
			if err := sRows.Scan(&s.Type, &s.JkyCode, &s.YsCode, &s.GoodsName, &s.Stock, &s.DailyAvg,
				&s.InTransit, &s.InTransitSubcontract, &s.YsClassName, &s.SellableDays,
				&s.NextArriveDate, &s.NextArriveDays, &s.Position, &s.CateName); err != nil {
				log.Printf("[suggest] scan err: %v", err)
				continue
			}
			// 判断 status
			switch {
			case s.SellableDays < 0:
				s.Status = "断货"
			case s.SellableDays < 7:
				s.Status = "紧急"
			case s.SellableDays < 14:
				s.Status = "偏低"
			case s.SellableDays > 90:
				s.Status = "积压"
			default:
				s.Status = "正常"
			}
			suggested = append(suggested, s)
		}
		sRows.Close()
	}

	// v0.83: 按库存降序 (库存多的排前)
	for i := 0; i < len(suggested); i++ {
		for j := i + 1; j < len(suggested); j++ {
			if suggested[j].Stock > suggested[i].Stock {
				suggested[i], suggested[j] = suggested[j], suggested[i]
			}
		}
	}

	writeJSON(w, map[string]interface{}{
		"kpis":         kpi,
		"monthlyTrend": monthlyTrend,
		"topVendors":   topVendors,
		"suggested":    suggested,
		"params": map[string]interface{}{
			"finishedGoodsTargetDays":  45,
			"materialTargetDays":       90,
			"urgentThresholdDays":      7,
			"lowThresholdDays":         14,
			"overstockThresholdDays":   90,
		},
	})
}

// SyncYSStock 同步触发: 7 重防御职责划分 (v1.00 设计文档化, 不删)
//
// 前端 PurchasePlan.tsx handleSync 5 重:
//  1. syncing 状态防御         — 当前 tab 已在同步, 拦截重复点
//  2. syncProgress Modal 防御  — 上一轮完成态 Modal 没关, 拦截连击
//  3. lastSyncEndRef 60s       — 同 tab 异步竞态/误双击 (跟后端对齐)
//  4. polling race fix (v0.78) — 防 1.5s 轮询把 done=true 状态覆盖回 running=true
//  5. 二次确认弹窗 (v0.80)     — 同步耗时 4-6 分钟, 防误点
//
// 后端 2 重 (本文件):
//  6. syncYSStockMu + Running  — 进程内并发互斥, 同一时刻只能跑一轮
//  7. syncYSLastEndTime 60s    — 全局 cooldown, 兜底前端 5 重失效场景
//                                 (双 tab / 浏览器扩展 / 页面刷新清前端 ref / 直接 curl)
//
// 真 bug 历史: v0.78 polling race 是最后一个代码层 bug, 已修.
// 剩余 6 重均为兜底用户行为, 代码层无可"消除根因", 不要再当冗余删.
// 调查方法: grep "sync-ys-stock|SyncYSStock" 全代码, 仅 PurchasePlan.tsx:248 一处调用且只在按钮 onClick.
