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

// ChannelRow 渠道汇总行(平台/渠道维度)
type ChannelRow struct {
	Platform string  `json:"platform"`
	Channel  string  `json:"channel"`
	Orders   int     `json:"orders"`   // 发货单数
	Bottles  float64 `json:"bottles"`  // 单瓶数(sell_count×箱规)
	WeightKg float64 `json:"weightKg"` // 重量(kg)
}

// GoodsRow TOP10 单品行
type GoodsRow struct {
	GoodsNo   string  `json:"goodsNo"`
	GoodsName string  `json:"goodsName"`
	Orders    int     `json:"orders"`
	Bottles   float64 `json:"bottles"`
	Boxes     float64 `json:"boxes"`   // 箱数=Σsell_count
	Pallets   float64 `json:"pallets"` // 托数=箱数÷托规
}

// ComboRow TOP10 货品组合行(订单整篮子)
type ComboRow struct {
	Display  string  `json:"display"`
	Orders   int     `json:"orders"`
	Bottles  float64 `json:"bottles"`
	WeightKg float64 `json:"weightKg"`
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
// 平台按 社媒/电商/其他 排,平台内按 Bottles 降序
func rollupPlatforms(rows []ChannelRow) []ChannelRow {
	sort.SliceStable(rows, func(i, j int) bool {
		oi, oj := platformOrder[rows[i].Platform], platformOrder[rows[j].Platform]
		if oi != oj {
			return oi < oj
		}
		return rows[i].Bottles > rows[j].Bottles
	})
	var out []ChannelRow
	grand := ChannelRow{Channel: "总计"}
	i := 0
	for i < len(rows) {
		p := rows[i].Platform
		sum := ChannelRow{Platform: p, Channel: p + "合计"}
		j := i
		for j < len(rows) && rows[j].Platform == p {
			sum.Orders += rows[j].Orders
			sum.Bottles += rows[j].Bottles
			sum.WeightKg += rows[j].WeightKg
			j++
		}
		out = append(out, sum)
		out = append(out, rows[i:j]...)
		grand.Orders += sum.Orders
		grand.Bottles += sum.Bottles
		grand.WeightKg += sum.WeightKg
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
	monStart := time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")

	ctx := r.Context()
	resp := map[string]interface{}{
		"date":         date,
		"channelToday": h.queryChannel(ctx, part, partG, dayStart, dayEnd),
		"channelMonth": h.queryChannel(ctx, part, partG, monStart, dayEnd),
		"goodsToday":   h.queryTopGoods(ctx, part, partG, dayStart, dayEnd),
		"goodsMonth":   h.queryTopGoods(ctx, part, partG, monStart, dayEnd),
		"comboToday":   h.queryTopCombos(ctx, part, partG, dayStart, dayEnd),
		"comboMonth":   h.queryTopCombos(ctx, part, partG, monStart, dayEnd),
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
			FROM %s WHERE trade_type=1 AND warehouse_name IN (%s)`, part, whPH)
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

// queryChannel 渠道块: 订单+重量按订单粒度, 单瓶按明细粒度, 按 channel 合并后 rollup
func (h *DashboardHandler) queryChannel(ctx context.Context, part, partG, start, end string) []ChannelRow {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	whPH, whArgs := whereWarehouseArgs()

	// 订单数 + 重量(克→kg), 按 平台/渠道。LEFT JOIN + 兜底: 没渠道映射的店铺归「其他/未分类」,
	// 不能丢单(否则渠道总计 < 4仓真实销售单, 且与不 join 映射的 TOP10 单品/组合对不上)。
	qOrders := fmt.Sprintf(`SELECT COALESCE(m.platform,'其他') AS platform,
		COALESCE(m.channel,'未分类') AS channel, COUNT(*) AS orders,
		IFNULL(SUM(t.estimate_weight),0)/1000 AS weight_kg
		FROM %s t LEFT JOIN dim_sales_channel_map m ON m.shop_name=t.shop_name
		WHERE t.trade_type=1 AND t.consign_time>=? AND t.consign_time<? AND t.warehouse_name IN (%s)
		GROUP BY COALESCE(m.platform,'其他'), COALESCE(m.channel,'未分类')`, part, whPH)
	// 单瓶数(sell_count×箱规, 缺箱规×1), 按 渠道(同样兜底未分类)
	qBottles := fmt.Sprintf(`SELECT COALESCE(m.channel,'未分类') AS channel,
		IFNULL(SUM(tg.sell_count*COALESCE(p.box_qty,1)),0) AS bottles
		FROM %s t JOIN %s tg ON tg.trade_id=t.trade_id
		LEFT JOIN dim_sales_channel_map m ON m.shop_name=t.shop_name
		LEFT JOIN dim_goods_pack_spec p ON p.goods_no=tg.goods_no
		WHERE t.trade_type=1 AND t.consign_time>=? AND t.consign_time<? AND t.warehouse_name IN (%s)
		GROUP BY COALESCE(m.channel,'未分类')`, part, partG, whPH)

	args := append([]interface{}{start, end}, whArgs...)
	byCh := map[string]*ChannelRow{}
	rows, err := h.DB.QueryContext(ctx, qOrders, args...)
	if err != nil {
		log.Printf("[sales-daily-report] queryChannel qOrders 失败 (%s %s~%s): %v", part, start, end, err)
		return nil
	}
	for rows.Next() {
		var c ChannelRow
		if err := rows.Scan(&c.Platform, &c.Channel, &c.Orders, &c.WeightKg); err != nil {
			rows.Close()
			return nil
		}
		cp := c
		byCh[c.Channel] = &cp
	}
	rows.Close()

	brows, err := h.DB.QueryContext(ctx, qBottles, args...)
	if err != nil {
		log.Printf("[sales-daily-report] queryChannel qBottles 失败 (%s %s~%s): %v", part, start, end, err)
		return nil
	}
	for brows.Next() {
		var ch string
		var bottles float64
		if err := brows.Scan(&ch, &bottles); err != nil {
			brows.Close()
			return nil
		}
		if c, ok := byCh[ch]; ok {
			c.Bottles = bottles
		}
	}
	brows.Close()

	out := make([]ChannelRow, 0, len(byCh))
	for _, c := range byCh {
		out = append(out, *c)
	}
	return rollupPlatforms(out)
}

// queryTopGoods TOP10 单品(按单瓶降序)
func (h *DashboardHandler) queryTopGoods(ctx context.Context, part, partG, start, end string) []GoodsRow {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	whPH, whArgs := whereWarehouseArgs()
	q := fmt.Sprintf(`SELECT tg.goods_no, MAX(tg.goods_name) AS nm,
		COUNT(DISTINCT t.trade_id) AS orders,
		IFNULL(SUM(tg.sell_count*COALESCE(p.box_qty,1)),0) AS bottles,
		IFNULL(SUM(tg.sell_count),0) AS boxes,
		MAX(p.pallet_box_qty) AS pallet_box_qty
		FROM %s t JOIN %s tg ON tg.trade_id=t.trade_id
		LEFT JOIN dim_goods_pack_spec p ON p.goods_no=tg.goods_no
		WHERE t.trade_type=1 AND t.consign_time>=? AND t.consign_time<? AND t.warehouse_name IN (%s)
		GROUP BY tg.goods_no ORDER BY bottles DESC LIMIT 10`, part, partG, whPH)
	args := append([]interface{}{start, end}, whArgs...)
	rows, err := h.DB.QueryContext(ctx, q, args...)
	if err != nil {
		log.Printf("[sales-daily-report] queryTopGoods 失败 (%s %s~%s): %v", part, start, end, err)
		return nil
	}
	defer rows.Close()
	var out []GoodsRow
	for rows.Next() {
		var g GoodsRow
		var pallet sql.NullFloat64
		if err := rows.Scan(&g.GoodsNo, &g.GoodsName, &g.Orders, &g.Bottles, &g.Boxes, &pallet); err != nil {
			log.Printf("[sales-daily-report] queryTopGoods scan 失败: %v", err)
			return nil
		}
		if pallet.Valid {
			g.Pallets = palletsOf(g.Boxes, pallet.Float64)
		}
		out = append(out, g)
	}
	return out
}

// queryTopCombos TOP10 货品组合(订单整篮子频次)
func (h *DashboardHandler) queryTopCombos(ctx context.Context, part, partG, start, end string) []ComboRow {
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
	// sig 用 SIGNED (sell_count 理论上销售单为正, 但退货/改数场景可能出现负/小数, UNSIGNED 会环绕成天文数字污染签名)
	q := fmt.Sprintf(`SELECT MAX(sig_display) AS sig_display, COUNT(*) AS orders,
		IFNULL(SUM(ob),0) AS bottles, IFNULL(SUM(ow),0)/1000 AS weight_kg
		FROM (
		  SELECT t.trade_id, MAX(t.estimate_weight) AS ow,
		    GROUP_CONCAT(tg.goods_no,'#',CAST(tg.sell_count AS SIGNED) ORDER BY tg.goods_no SEPARATOR '|') AS sig,
		    GROUP_CONCAT(tg.goods_name,'(',CAST(tg.sell_count AS SIGNED),')' ORDER BY tg.goods_no SEPARATOR ', ') AS sig_display,
		    SUM(tg.sell_count*COALESCE(p.box_qty,1)) AS ob
		  FROM %s t JOIN %s tg ON tg.trade_id=t.trade_id
		  LEFT JOIN dim_goods_pack_spec p ON p.goods_no=tg.goods_no
		  WHERE t.trade_type=1 AND t.consign_time>=? AND t.consign_time<? AND t.warehouse_name IN (%s)
		  GROUP BY t.trade_id
		) o GROUP BY sig ORDER BY orders DESC LIMIT 10`, part, partG, whPH)
	args := append([]interface{}{start, end}, whArgs...)
	rows, err := conn.QueryContext(ctx, q, args...)
	if err != nil {
		log.Printf("[sales-daily-report] queryTopCombos 查询失败 (%s %s~%s): %v", part, start, end, err)
		return nil
	}
	defer rows.Close()
	var out []ComboRow
	for rows.Next() {
		var c ComboRow
		if err := rows.Scan(&c.Display, &c.Orders, &c.Bottles, &c.WeightKg); err != nil {
			log.Printf("[sales-daily-report] queryTopCombos scan 失败: %v", err)
			return nil
		}
		out = append(out, c)
	}
	return out
}
