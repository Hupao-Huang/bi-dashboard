package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sort"
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
	Bottles  float64 `json:"bottles"`  // 单瓶数(sell_count×箱规)
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

// GoodsStat 单品一组统计
type GoodsStat struct {
	Orders  int     `json:"orders"`
	Bottles float64 `json:"bottles"`
	Boxes   float64 `json:"boxes"`   // 箱数=Σsell_count
	Pallets float64 `json:"pallets"` // 托数=箱数÷托规
}

// GoodsRow TOP10 单品行(按当月单瓶排), 当日+当月两组
type GoodsRow struct {
	GoodsNo    string    `json:"goodsNo"`
	GoodsName  string    `json:"goodsName"`
	BoxQty     float64   `json:"boxQty"`     // 箱规(每箱瓶数), 对齐 Excel「箱规」列
	Today      GoodsStat `json:"today"`
	Month      GoodsStat `json:"month"`
	PrevBottles float64  `json:"prevBottles"` // 前一发货日发货件数, 前端算环比用(单品环比按件数)
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
	resp := map[string]interface{}{
		"date":     date,
		"channels": h.queryChannel(ctx, part, partG, prevDay, dayStart, monStart, dayEnd),
		"goods":    h.queryTopGoods(ctx, part, partG, prevDay, dayStart, monStart, dayEnd),
		"combos":   h.queryTopCombos(ctx, part, partG, prevDay, dayStart, monStart, dayEnd),
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

// queryChannel 渠道块: 当月区间跑, CASE WHEN 当日/前一日 拆各组; 订单+重量按订单粒度, 单瓶按明细粒度。
// prevDay~dayStart=前一发货日(环比分母, 双边界确保月初1号时落在WHERE外→prev=0对齐Excel #DIV/0!)。
func (h *DashboardHandler) queryChannel(ctx context.Context, part, partG, prevDay, dayStart, monStart, end string) []ChannelRow {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	whPH, whArgs := whereWarehouseArgs()

	// 订单数 + 重量: 当月 COUNT/SUM, 当日/前一日用 CASE WHEN。LEFT JOIN 兜底未映射店铺。
	qOrders := fmt.Sprintf(`SELECT COALESCE(m.platform,'其他') AS platform, COALESCE(m.channel,'未分类') AS channel,
		SUM(CASE WHEN t.consign_time>=? THEN 1 ELSE 0 END) AS today_orders,
		IFNULL(SUM(CASE WHEN t.consign_time>=? THEN t.estimate_weight ELSE 0 END),0)/1000 AS today_weight,
		SUM(CASE WHEN t.consign_time>=? AND t.consign_time<? THEN 1 ELSE 0 END) AS prev_orders,
		COUNT(*) AS month_orders,
		IFNULL(SUM(t.estimate_weight),0)/1000 AS month_weight
		FROM %s t LEFT JOIN dim_sales_channel_map m ON m.shop_name=t.shop_name
		WHERE t.trade_type NOT IN (8,12) AND t.consign_time>=? AND t.consign_time<? AND t.warehouse_name IN (%s)
		GROUP BY COALESCE(m.platform,'其他'), COALESCE(m.channel,'未分类')`, part, whPH)
	// 单瓶数(sell_count×箱规, 缺箱规×1), 平台+渠道复合(防同渠道跨平台串), 当日/当月各一组。
	qBottles := fmt.Sprintf(`SELECT COALESCE(m.platform,'其他') AS platform, COALESCE(m.channel,'未分类') AS channel,
		IFNULL(SUM(CASE WHEN t.consign_time>=? THEN tg.sell_count*COALESCE(p.box_qty,1) ELSE 0 END),0) AS today_bottles,
		IFNULL(SUM(tg.sell_count*COALESCE(p.box_qty,1)),0) AS month_bottles
		FROM %s t JOIN %s tg ON tg.trade_id=t.trade_id
		LEFT JOIN dim_sales_channel_map m ON m.shop_name=t.shop_name
		LEFT JOIN dim_goods_pack_spec p ON p.goods_no=tg.goods_no
		WHERE t.trade_type NOT IN (8,12) AND t.consign_time>=? AND t.consign_time<? AND t.warehouse_name IN (%s)
		GROUP BY COALESCE(m.platform,'其他'), COALESCE(m.channel,'未分类')`, part, partG, whPH)

	// qOrders 参数: today CASE(dayStart) + today_weight CASE(dayStart) + prev CASE(prevDay,dayStart) + WHERE(monStart,end) + wh
	oArgs := append([]interface{}{dayStart, dayStart, prevDay, dayStart, monStart, end}, whArgs...)
	bArgs := append([]interface{}{dayStart, monStart, end}, whArgs...)

	byCh := map[string]*ChannelRow{}
	rows, err := h.DB.QueryContext(ctx, qOrders, oArgs...)
	if err != nil {
		log.Printf("[sales-daily-report] queryChannel qOrders 失败 (%s %s~%s): %v", part, monStart, end, err)
		return nil
	}
	for rows.Next() {
		var c ChannelRow
		if err := rows.Scan(&c.Platform, &c.Channel, &c.Today.Orders, &c.Today.WeightKg, &c.PrevOrders, &c.Month.Orders, &c.Month.WeightKg); err != nil {
			rows.Close()
			return nil
		}
		cp := c
		byCh[c.Platform+"|"+c.Channel] = &cp
	}
	rows.Close()

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
		}
	}
	brows.Close()

	out := make([]ChannelRow, 0, len(byCh))
	for _, c := range byCh {
		out = append(out, *c)
	}
	return rollupPlatforms(out)
}

// queryTopGoods TOP10 单品: 按当月单瓶排 TOP10(对齐 Excel 榜单口径), 每行同时出当日/当月两组 + 前一日件数(环比)。
func (h *DashboardHandler) queryTopGoods(ctx context.Context, part, partG, prevDay, dayStart, monStart, end string) []GoodsRow {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	whPH, whArgs := whereWarehouseArgs()
	q := fmt.Sprintf(`SELECT tg.goods_no, MAX(tg.goods_name) AS nm, MAX(p.box_qty) AS box_qty,
		COUNT(DISTINCT CASE WHEN t.consign_time>=? THEN t.trade_id END) AS today_orders,
		IFNULL(SUM(CASE WHEN t.consign_time>=? THEN tg.sell_count*COALESCE(p.box_qty,1) ELSE 0 END),0) AS today_bottles,
		IFNULL(SUM(CASE WHEN t.consign_time>=? THEN tg.sell_count ELSE 0 END),0) AS today_boxes,
		IFNULL(SUM(CASE WHEN t.consign_time>=? AND t.consign_time<? THEN tg.sell_count*COALESCE(p.box_qty,1) ELSE 0 END),0) AS prev_bottles,
		COUNT(DISTINCT t.trade_id) AS month_orders,
		IFNULL(SUM(tg.sell_count*COALESCE(p.box_qty,1)),0) AS month_bottles,
		IFNULL(SUM(tg.sell_count),0) AS month_boxes,
		MAX(p.pallet_box_qty) AS pallet_box_qty
		FROM %s t JOIN %s tg ON tg.trade_id=t.trade_id
		LEFT JOIN dim_goods_pack_spec p ON p.goods_no=tg.goods_no
		WHERE t.trade_type NOT IN (8,12) AND t.consign_time>=? AND t.consign_time<? AND t.warehouse_name IN (%s)
		GROUP BY tg.goods_no ORDER BY month_bottles DESC LIMIT 10`, part, partG, whPH)
	args := append([]interface{}{dayStart, dayStart, dayStart, prevDay, dayStart, monStart, end}, whArgs...)
	rows, err := h.DB.QueryContext(ctx, q, args...)
	if err != nil {
		log.Printf("[sales-daily-report] queryTopGoods 失败 (%s %s~%s): %v", part, monStart, end, err)
		return nil
	}
	defer rows.Close()
	var out []GoodsRow
	for rows.Next() {
		var g GoodsRow
		var boxQty, pallet sql.NullFloat64
		if err := rows.Scan(&g.GoodsNo, &g.GoodsName, &boxQty,
			&g.Today.Orders, &g.Today.Bottles, &g.Today.Boxes, &g.PrevBottles,
			&g.Month.Orders, &g.Month.Bottles, &g.Month.Boxes, &pallet); err != nil {
			log.Printf("[sales-daily-report] queryTopGoods scan 失败: %v", err)
			return nil
		}
		if boxQty.Valid {
			g.BoxQty = boxQty.Float64
		}
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
