package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"
)

// salesReportWarehouses 销售日报只算这 4 个仓(跑哥定)
var salesReportWarehouses = []string{
	"南京委外成品仓-公司仓-委外",
	"天津委外仓-公司仓-外仓",
	"松鲜鲜&大地密码云仓",
	"长沙委外成品仓-公司仓-外仓",
}

// whereWarehouseArgs 返回 4 仓 IN 占位与参数(拼在发货日区间之后)
func whereWarehouseArgs() (string, []interface{}) {
	ph := "?,?,?,?"
	args := make([]interface{}, len(salesReportWarehouses))
	for i, w := range salesReportWarehouses {
		args[i] = w
	}
	return ph, args
}

// 当日/当月累计并排(对齐 Excel 版式): 一行同时带 Today + Month 两组数, 榜单按当月排。

// ChannelStat 渠道一组统计(当日或当月)
type ChannelStat struct {
	Orders   int     `json:"orders"`   // 发货单数
	Bottles  float64 `json:"bottles"`  // 发货件数(汇总账 goods_qty×销售规格, 排除退货/仅退款)
	WeightKg float64 `json:"weightKg"` // 重量(kg)
}

// ChannelRow 渠道汇总行(平台/渠道维度), 当日+当月两组
type ChannelRow struct {
	Platform   string      `json:"platform"`
	Channel    string      `json:"channel"`
	Today      ChannelStat `json:"today"`
	Month      ChannelStat `json:"month"`
	PrevOrders int         `json:"prevOrders"` // 前一发货日发货量(订单数), 前端算环比用
}

type channelOrderAgg struct {
	Platform    string
	Channel     string
	TodayOrders int
	TodayWeight float64
	PrevOrders  int
	MonthOrders int
	MonthWeight float64
}

// GoodsStat 单品一组统计
type GoodsStat struct {
	Orders  int     `json:"orders"`
	Bottles float64 `json:"bottles"` // 发货件数(汇总账 goods_qty×销售规格, 排除退货/仅退款)
	Boxes   float64 `json:"boxes"`   // 发货箱数=件数÷装箱规格
	Pallets float64 `json:"pallets"` // 托数=箱数÷托规
}

// GoodsRow TOP10 单品行(按当月单瓶排), 当日+当月两组
type GoodsRow struct {
	GoodsNo     string    `json:"goodsNo"`
	GoodsName   string    `json:"goodsName"`
	BoxQty      float64   `json:"boxQty"` // 装箱规格(每箱瓶数), 对齐 Excel「箱规」列 = 箱数分母
	Today       GoodsStat `json:"today"`
	Month       GoodsStat `json:"month"`
	PrevBottles float64   `json:"prevBottles"` // 前一发货日发货件数, 前端算环比用(单品环比按件数)
}

// ComboStat 组合一组统计
type ComboStat struct {
	Orders   int     `json:"orders"`
	Bottles  float64 `json:"bottles"`
	WeightKg float64 `json:"weightKg"`
}

// ComboRow TOP10 货品组合行(按当月订单数排), 当日+当月两组
type ComboRow struct {
	Display    string    `json:"display"`
	Today      ComboStat `json:"today"`
	Month      ComboStat `json:"month"`
	PrevOrders int       `json:"prevOrders"` // 前一发货日发货量(订单数), 前端算环比用
}

// ratio 占比(total=0 返 0,防 #DIV/0!)
func ratio(part, total float64) float64 {
	if total == 0 {
		return 0
	}
	return part / total
}

// perOrder 单均(orders=0 返 0)
func perOrder(v float64, orders int) float64 {
	if orders == 0 {
		return 0
	}
	return v / float64(orders)
}

// palletsOf 托数=箱数÷托规(无托规[≤0]返 0)
func palletsOf(boxes, palletBoxQty float64) float64 {
	if palletBoxQty <= 0 {
		return 0
	}
	return boxes / palletBoxQty
}

var platformOrder = map[string]int{"社媒": 0, "电商": 1, "其他": 2}

// rollupPlatforms 渠道明细行 → 每平台前插「X合计」,末尾加「总计」;
// 平台按 社媒/电商/其他 排,平台内按 当月 Bottles 降序; 合计/总计累加 当日+当月两组
func rollupPlatforms(rows []ChannelRow) []ChannelRow {
	sort.SliceStable(rows, func(i, j int) bool {
		oi, oj := platformOrder[rows[i].Platform], platformOrder[rows[j].Platform]
		if oi != oj {
			return oi < oj
		}
		return rows[i].Month.Bottles > rows[j].Month.Bottles
	})
	addInto := func(dst *ChannelStat, s ChannelStat) {
		dst.Orders += s.Orders
		dst.Bottles += s.Bottles
		dst.WeightKg += s.WeightKg
	}
	var out []ChannelRow
	grand := ChannelRow{Channel: "总计"}
	i := 0
	for i < len(rows) {
		p := rows[i].Platform
		sum := ChannelRow{Platform: p, Channel: p + "合计"}
		j := i
		for j < len(rows) && rows[j].Platform == p {
			addInto(&sum.Today, rows[j].Today)
			addInto(&sum.Month, rows[j].Month)
			sum.PrevOrders += rows[j].PrevOrders
			j++
		}
		out = append(out, sum)
		out = append(out, rows[i:j]...)
		addInto(&grand.Today, sum.Today)
		addInto(&grand.Month, sum.Month)
		grand.PrevOrders += sum.PrevOrders
		i = j
	}
	out = append(out, grand)
	return out
}

// ---- 接口层 ----

// GetSalesDailyReport 供应链-销售日报(发货日报): 按发货日/4仓, 三块 当日+当月累计
// GET /api/supply-chain/sales-daily-report?date=YYYY-MM-DD
func (h *DashboardHandler) GetSalesDailyReport(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = h.latestConsignDate()
	}
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		writeError(w, 400, "date 格式应为 YYYY-MM-DD")
		return
	}
	part := "trade_" + d.Format("200601")
	partG := "trade_goods_" + d.Format("200601")
	dayStart := d.Format("2006-01-02")
	dayEnd := d.AddDate(0, 0, 1).Format("2006-01-02")
	prevDay := d.AddDate(0, 0, -1).Format("2006-01-02") // 前一自然日, 算环比分母(对齐 Excel $A$2-1)
	monStart := time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")

	ctx := r.Context()
	// 三块都跑「当月区间 + CASE WHEN 当日/前一日」条件聚合, 一行同时出当日/当月/环比分母, 榜单按当月排
	var channels []ChannelRow
	var goods []GoodsRow
	var combos []ComboRow
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		channels = h.queryChannel(ctx, part, partG, prevDay, dayStart, monStart, dayEnd)
	}()
	go func() {
		defer wg.Done()
		goods = h.queryTopGoods(ctx, part, partG, prevDay, dayStart, monStart, dayEnd)
	}()
	go func() {
		defer wg.Done()
		combos = h.queryTopCombos(ctx, part, partG, prevDay, dayStart, monStart, dayEnd)
	}()
	wg.Wait()
	resp := map[string]interface{}{
		"date":     date,
		"channels": channels,
		"goods":    goods,
		"combos":   combos,
	}
	writeJSON(w, resp)
}

// latestConsignDate 最近有发货数据的日期(4仓/销售)。先查当月分区, 月初当月还没发货数据
// (或当月表还没建)时回退查上月分区, 而不是无脑退"昨天"(那样月初会跳到上月末且分区可能对不上)。
func (h *DashboardHandler) latestConsignDate() string {
	whPH, whArgs := whereWarehouseArgs()
	tryMonth := func(t time.Time) string {
		part := "trade_" + t.Format("200601")
		q := fmt.Sprintf(`SELECT IFNULL(DATE_FORMAT(MAX(consign_time),'%%Y-%%m-%%d'),'')
			FROM %s WHERE trade_type NOT IN (8,12) AND warehouse_name IN (%s)`, part, whPH)
		var s string
		if err := h.DB.QueryRow(q, whArgs...).Scan(&s); err != nil {
			return "" // 表不存在等, 交给上层回退上月
		}
		return s
	}
	now := time.Now()
	if s := tryMonth(now); s != "" {
		return s
	}
	if s := tryMonth(now.AddDate(0, 0, -1)); s != "" { // 上月末(可能是上一个分区)
		return s
	}
	return now.AddDate(0, 0, -1).Format("2006-01-02")
}

// queryChannel 渠道块: 当月区间跑, CASE WHEN 当日/前一日 拆各组。
// 订单+重量仍按销售单明细的订单粒度; 发货件数用吉客云汇总账拆订单类型后的 goods_qty×销售规格, 排除退货/仅退款。
// prevDay~dayStart=前一发货日(环比分母, 双边界确保月初1号时落在WHERE外→prev=0对齐Excel #DIV/0!)。
func (h *DashboardHandler) queryChannel(ctx context.Context, part, partG, prevDay, dayStart, monStart, end string) []ChannelRow {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	whPH, whArgs := whereWarehouseArgs()

	// 发货件数: 以吉客云汇总账 goods_qty 为准, 再乘销售规格 box_qty 还原单瓶/单包件数。
	qBottles := fmt.Sprintf(`SELECT COALESCE(m.platform,'其他') AS platform, COALESCE(m.channel,'未分类') AS channel,
		IFNULL(SUM(CASE WHEN s.stat_date=? THEN s.goods_qty*COALESCE(p.box_qty,1) ELSE 0 END),0) AS today_bottles,
		IFNULL(SUM(s.goods_qty*COALESCE(p.box_qty,1)),0) AS month_bottles
		FROM sales_goods_summary_by_order_type s
		LEFT JOIN dim_sales_channel_map m ON m.shop_name=s.shop_name
		LEFT JOIN dim_goods_pack_spec p ON p.goods_no=s.goods_no
		WHERE s.trade_order_type NOT IN ('8','12') AND s.stat_date>=? AND s.stat_date<? AND s.warehouse_name IN (%s)
		GROUP BY COALESCE(m.platform,'其他'), COALESCE(m.channel,'未分类')`, whPH)

	bArgs := append([]interface{}{dayStart, monStart, end}, whArgs...)

	byCh := map[string]*ChannelRow{}
	orderRows, err := h.queryChannelOrdersSummary(ctx, prevDay, dayStart, monStart, end)
	if err != nil {
		log.Printf("[sales-daily-report] queryChannel orders summary 失败, 回退明细查询 (%s %s~%s): %v", part, monStart, end, err)
		orderRows, err = h.queryChannelOrdersRaw(ctx, part, prevDay, dayStart, monStart, end)
		if err != nil {
			log.Printf("[sales-daily-report] queryChannel qOrders 失败 (%s %s~%s): %v", part, monStart, end, err)
			return nil
		}
	}
	for _, o := range orderRows {
		c := ChannelRow{
			Platform:   o.Platform,
			Channel:    o.Channel,
			Today:      ChannelStat{Orders: o.TodayOrders, WeightKg: o.TodayWeight},
			Month:      ChannelStat{Orders: o.MonthOrders, WeightKg: o.MonthWeight},
			PrevOrders: o.PrevOrders,
		}
		byCh[c.Platform+"|"+c.Channel] = &c
	}

	brows, err := h.DB.QueryContext(ctx, qBottles, bArgs...)
	if err != nil {
		log.Printf("[sales-daily-report] queryChannel qBottles 失败 (%s %s~%s): %v", part, monStart, end, err)
		return nil
	}
	for brows.Next() {
		var plat, ch string
		var todayB, monthB float64
		if err := brows.Scan(&plat, &ch, &todayB, &monthB); err != nil {
			brows.Close()
			return nil
		}
		if c, ok := byCh[plat+"|"+ch]; ok {
			c.Today.Bottles = todayB
			c.Month.Bottles = monthB
		} else {
			byCh[plat+"|"+ch] = &ChannelRow{
				Platform: plat,
				Channel:  ch,
				Today:    ChannelStat{Bottles: todayB},
				Month:    ChannelStat{Bottles: monthB},
			}
		}
	}
	brows.Close()

	out := make([]ChannelRow, 0, len(byCh))
	for _, c := range byCh {
		out = append(out, *c)
	}
	return rollupPlatforms(out)
}

func (h *DashboardHandler) queryChannelOrdersSummary(ctx context.Context, prevDay, dayStart, monStart, end string) ([]channelOrderAgg, error) {
	var dayRows int
	if err := h.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sales_daily_shop_order_summary WHERE stat_date=?`, dayStart).Scan(&dayRows); err != nil {
		return nil, err
	}
	if dayRows == 0 {
		return nil, fmt.Errorf("sales_daily_shop_order_summary 缺少当天数据: %s", dayStart)
	}
	rows, err := h.DB.QueryContext(ctx, `SELECT COALESCE(m.platform,'其他') AS platform, COALESCE(m.channel,'未分类') AS channel,
		IFNULL(SUM(CASE WHEN s.stat_date=? THEN s.orders ELSE 0 END),0) AS today_orders,
		IFNULL(SUM(CASE WHEN s.stat_date=? THEN s.weight_kg ELSE 0 END),0) AS today_weight,
		IFNULL(SUM(CASE WHEN s.stat_date>=? AND s.stat_date<? THEN s.orders ELSE 0 END),0) AS prev_orders,
		IFNULL(SUM(s.orders),0) AS month_orders,
		IFNULL(SUM(s.weight_kg),0) AS month_weight
		FROM sales_daily_shop_order_summary s
		LEFT JOIN dim_sales_channel_map m ON m.shop_name=s.shop_name
		WHERE s.stat_date>=? AND s.stat_date<?
		GROUP BY COALESCE(m.platform,'其他'), COALESCE(m.channel,'未分类')`, dayStart, dayStart, prevDay, dayStart, monStart, end)
	if err != nil {
		return nil, err
	}
	return scanChannelOrderAggRows(rows)
}

func (h *DashboardHandler) queryChannelOrdersRaw(ctx context.Context, part, prevDay, dayStart, monStart, end string) ([]channelOrderAgg, error) {
	whPH, whArgs := whereWarehouseArgs()
	q := fmt.Sprintf(`SELECT COALESCE(m.platform,'其他') AS platform, COALESCE(m.channel,'未分类') AS channel,
		SUM(CASE WHEN t.consign_time>=? THEN 1 ELSE 0 END) AS today_orders,
		IFNULL(SUM(CASE WHEN t.consign_time>=? THEN t.estimate_weight ELSE 0 END),0)/1000 AS today_weight,
		SUM(CASE WHEN t.consign_time>=? AND t.consign_time<? THEN 1 ELSE 0 END) AS prev_orders,
		COUNT(*) AS month_orders,
		IFNULL(SUM(t.estimate_weight),0)/1000 AS month_weight
		FROM %s t LEFT JOIN dim_sales_channel_map m ON m.shop_name=t.shop_name
		WHERE t.trade_type NOT IN (8,12) AND t.consign_time>=? AND t.consign_time<? AND t.warehouse_name IN (%s)
		GROUP BY COALESCE(m.platform,'其他'), COALESCE(m.channel,'未分类')`, part, whPH)
	args := append([]interface{}{dayStart, dayStart, prevDay, dayStart, monStart, end}, whArgs...)
	rows, err := h.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return scanChannelOrderAggRows(rows)
}

func scanChannelOrderAggRows(rows *sql.Rows) ([]channelOrderAgg, error) {
	defer rows.Close()
	var out []channelOrderAgg
	for rows.Next() {
		var o channelOrderAgg
		var todayOrders, prevOrders, monthOrders float64
		if err := rows.Scan(&o.Platform, &o.Channel, &todayOrders, &o.TodayWeight, &prevOrders, &monthOrders, &o.MonthWeight); err != nil {
			return nil, err
		}
		o.TodayOrders = int(math.Round(todayOrders))
		o.PrevOrders = int(math.Round(prevOrders))
		o.MonthOrders = int(math.Round(monthOrders))
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// queryTopGoods TOP10 单品: 按当月发货件数排 TOP10, 每行同时出当日/当月两组 + 前一日件数(环比)。
// 件数=吉客云汇总账 goods_qty×销售规格(按订单类型拆分后排除 8/12); 箱数=件数÷装箱规格carton_pieces。
func (h *DashboardHandler) queryTopGoods(ctx context.Context, part, partG, prevDay, dayStart, monStart, end string) []GoodsRow {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	whPH, whArgs := whereWarehouseArgs()
	q := fmt.Sprintf(`SELECT s.goods_no, MAX(s.goods_name) AS nm,
		MAX(COALESCE(p.carton_pieces, p.box_qty, 1)) AS carton_pieces,
		IFNULL(SUM(CASE WHEN s.stat_date=? THEN s.so_qty ELSE 0 END),0) AS today_orders,
		IFNULL(SUM(CASE WHEN s.stat_date=? THEN s.goods_qty*COALESCE(p.box_qty,1) ELSE 0 END),0) AS today_bottles,
		IFNULL(SUM(CASE WHEN s.stat_date=? THEN s.goods_qty*COALESCE(p.box_qty,1) ELSE 0 END),0) AS prev_bottles,
		IFNULL(SUM(s.so_qty),0) AS month_orders,
		IFNULL(SUM(s.goods_qty*COALESCE(p.box_qty,1)),0) AS month_bottles,
		MAX(p.pallet_box_qty) AS pallet_box_qty
		FROM sales_goods_summary_by_order_type s
		LEFT JOIN dim_goods_pack_spec p ON p.goods_no=s.goods_no
		WHERE s.trade_order_type NOT IN ('8','12') AND s.stat_date>=? AND s.stat_date<? AND s.warehouse_name IN (%s)
		GROUP BY s.goods_no ORDER BY month_bottles DESC LIMIT 10`, whPH)
	args := append([]interface{}{dayStart, dayStart, prevDay, monStart, end}, whArgs...)
	rows, err := h.DB.QueryContext(ctx, q, args...)
	if err != nil {
		log.Printf("[sales-daily-report] queryTopGoods 失败 (%s %s~%s): %v", part, monStart, end, err)
		return nil
	}
	defer rows.Close()
	var out []GoodsRow
	for rows.Next() {
		var g GoodsRow
		var cartonPieces, pallet sql.NullFloat64
		var todayOrders, monthOrders float64
		if err := rows.Scan(&g.GoodsNo, &g.GoodsName, &cartonPieces,
			&todayOrders, &g.Today.Bottles, &g.PrevBottles,
			&monthOrders, &g.Month.Bottles, &pallet); err != nil {
			log.Printf("[sales-daily-report] queryTopGoods scan 失败: %v", err)
			return nil
		}
		g.Today.Orders = int(math.Round(todayOrders))
		g.Month.Orders = int(math.Round(monthOrders))
		// 装箱规格(每箱瓶数): 已由 SQL COALESCE 兜底为 box_qty/1, 一定>0; 箱数=件数÷它
		cp := 1.0
		if cartonPieces.Valid && cartonPieces.Float64 > 0 {
			cp = cartonPieces.Float64
		}
		g.BoxQty = cp
		g.Today.Boxes = g.Today.Bottles / cp
		g.Month.Boxes = g.Month.Bottles / cp
		if pallet.Valid {
			g.Today.Pallets = palletsOf(g.Today.Boxes, pallet.Float64)
			g.Month.Pallets = palletsOf(g.Month.Boxes, pallet.Float64)
		}
		out = append(out, g)
	}
	return out
}

// queryTopCombos TOP10 货品组合: 按当月订单数排 TOP10, 每行出当日/当月两组 + 前一日订单数(环比)。
func (h *DashboardHandler) queryTopCombos(ctx context.Context, part, partG, prevDay, dayStart, monStart, end string) []ComboRow {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var dayRows int
	if err := h.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sales_daily_combo_summary WHERE stat_date=?`, dayStart).Scan(&dayRows); err != nil {
		log.Printf("[sales-daily-report] queryTopCombos summary 检查失败, 回退明细查询 (%s %s): %v", part, dayStart, err)
		return h.queryTopCombosRaw(ctx, part, partG, prevDay, dayStart, monStart, end)
	}
	if dayRows == 0 {
		log.Printf("[sales-daily-report] queryTopCombos summary 缺少当天数据, 回退明细查询 (%s %s)", part, dayStart)
		return h.queryTopCombosRaw(ctx, part, partG, prevDay, dayStart, monStart, end)
	}
	rows, err := h.DB.QueryContext(ctx, `SELECT MAX(combo_display) AS combo_display,
		SUM(CASE WHEN stat_date=? THEN orders ELSE 0 END) AS today_orders,
		IFNULL(SUM(CASE WHEN stat_date=? THEN bottles ELSE 0 END),0) AS today_bottles,
		IFNULL(SUM(CASE WHEN stat_date=? THEN weight_kg ELSE 0 END),0) AS today_weight,
		SUM(CASE WHEN stat_date=? THEN orders ELSE 0 END) AS prev_orders,
		SUM(orders) AS month_orders,
		IFNULL(SUM(bottles),0) AS month_bottles,
		IFNULL(SUM(weight_kg),0) AS month_weight
		FROM sales_daily_combo_summary
		WHERE stat_date>=? AND stat_date<?
		GROUP BY combo_hash, combo_sig
		ORDER BY month_orders DESC LIMIT 10`, dayStart, dayStart, dayStart, prevDay, monStart, end)
	if err != nil {
		log.Printf("[sales-daily-report] queryTopCombos summary 查询失败, 回退明细查询 (%s %s~%s): %v", part, monStart, end, err)
		return h.queryTopCombosRaw(ctx, part, partG, prevDay, dayStart, monStart, end)
	}
	defer rows.Close()
	var out []ComboRow
	for rows.Next() {
		var c ComboRow
		if err := rows.Scan(&c.Display, &c.Today.Orders, &c.Today.Bottles, &c.Today.WeightKg, &c.PrevOrders,
			&c.Month.Orders, &c.Month.Bottles, &c.Month.WeightKg); err != nil {
			log.Printf("[sales-daily-report] queryTopCombos summary scan 失败: %v", err)
			return h.queryTopCombosRaw(ctx, part, partG, prevDay, dayStart, monStart, end)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[sales-daily-report] queryTopCombos summary rows 失败: %v", err)
		return h.queryTopCombosRaw(ctx, part, partG, prevDay, dayStart, monStart, end)
	}
	if len(out) > 0 {
		return out
	}
	log.Printf("[sales-daily-report] queryTopCombos summary 无数据, 回退明细查询 (%s %s~%s)", part, monStart, end)
	return h.queryTopCombosRaw(ctx, part, partG, prevDay, dayStart, monStart, end)
}

func (h *DashboardHandler) queryTopCombosRaw(ctx context.Context, part, partG, prevDay, dayStart, monStart, end string) []ComboRow {
	// SET SESSION 与后续查询必须落在同一物理连接: h.DB 是连接池, ExecContext 设的 session 变量
	// 可能作用在别的连接上, 导致长篮子 GROUP_CONCAT 仍按默认 1024 截断。用 Conn() 锁定单连接。
	conn, err := h.DB.Conn(ctx)
	if err != nil {
		log.Printf("[sales-daily-report] queryTopCombos 取连接失败: %v", err)
		return nil
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "SET SESSION group_concat_max_len = 100000"); err != nil {
		log.Printf("[sales-daily-report] queryTopCombos SET group_concat_max_len 失败: %v", err)
		return nil
	}
	whPH, whArgs := whereWarehouseArgs()
	// sig 用 SIGNED (退货/改数场景 sell_count 可能负/小数, UNSIGNED 会环绕成天文数字污染签名)
	q := fmt.Sprintf(`SELECT MAX(sig_display) AS sig_display,
		SUM(is_today) AS today_orders,
		IFNULL(SUM(CASE WHEN is_today=1 THEN ob ELSE 0 END),0) AS today_bottles,
		IFNULL(SUM(CASE WHEN is_today=1 THEN ow ELSE 0 END),0)/1000 AS today_weight,
		SUM(is_prev) AS prev_orders,
		COUNT(*) AS month_orders,
		IFNULL(SUM(ob),0) AS month_bottles, IFNULL(SUM(ow),0)/1000 AS month_weight
		FROM (
		  SELECT t.trade_id, MAX(t.estimate_weight) AS ow,
		    CASE WHEN MAX(t.consign_time)>=? THEN 1 ELSE 0 END AS is_today,
		    CASE WHEN MAX(t.consign_time)>=? AND MAX(t.consign_time)<? THEN 1 ELSE 0 END AS is_prev,
		    GROUP_CONCAT(tg.goods_no,'#',CAST(tg.sell_count AS SIGNED) ORDER BY tg.goods_no SEPARATOR '|') AS sig,
		    GROUP_CONCAT(tg.goods_name,'(',CAST(tg.sell_count AS SIGNED),')' ORDER BY tg.goods_no SEPARATOR ', ') AS sig_display,
		    SUM(tg.sell_count*COALESCE(p.box_qty,1)) AS ob
		  FROM %s t JOIN %s tg ON tg.trade_id=t.trade_id
		  LEFT JOIN dim_goods_pack_spec p ON p.goods_no=tg.goods_no
		  WHERE t.trade_type NOT IN (8,12) AND t.consign_time>=? AND t.consign_time<? AND t.warehouse_name IN (%s)
		  GROUP BY t.trade_id
		) o GROUP BY sig ORDER BY month_orders DESC LIMIT 10`, part, partG, whPH)
	// 参数: is_today CASE(dayStart) + is_prev CASE(prevDay,dayStart) + WHERE(monStart,end) + wh
	args := append([]interface{}{dayStart, prevDay, dayStart, monStart, end}, whArgs...)
	rows, err := conn.QueryContext(ctx, q, args...)
	if err != nil {
		log.Printf("[sales-daily-report] queryTopCombos 查询失败 (%s %s~%s): %v", part, monStart, end, err)
		return nil
	}
	defer rows.Close()
	var out []ComboRow
	for rows.Next() {
		var c ComboRow
		if err := rows.Scan(&c.Display, &c.Today.Orders, &c.Today.Bottles, &c.Today.WeightKg, &c.PrevOrders,
			&c.Month.Orders, &c.Month.Bottles, &c.Month.WeightKg); err != nil {
			log.Printf("[sales-daily-report] queryTopCombos scan 失败: %v", err)
			return nil
		}
		out = append(out, c)
	}
	return out
}
