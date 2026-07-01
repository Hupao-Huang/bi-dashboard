package handler

import (
	"context"
	"database/sql"
	"fmt"
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

// latestConsignDate 最近有发货数据的日期(4仓/销售), 默认取当月分区; 空则退当天
func (h *DashboardHandler) latestConsignDate() string {
	now := time.Now()
	part := "trade_" + now.Format("200601")
	whPH, whArgs := whereWarehouseArgs()
	q := fmt.Sprintf(`SELECT IFNULL(DATE_FORMAT(MAX(consign_time),'%%Y-%%m-%%d'),'')
		FROM %s WHERE trade_type=1 AND warehouse_name IN (%s)`, part, whPH)
	var s string
	if err := h.DB.QueryRow(q, whArgs...).Scan(&s); err != nil || s == "" {
		return now.AddDate(0, 0, -1).Format("2006-01-02")
	}
	return s
}

// queryChannel 渠道块: 订单+重量按订单粒度, 单瓶按明细粒度, 按 channel 合并后 rollup
func (h *DashboardHandler) queryChannel(ctx context.Context, part, partG, start, end string) []ChannelRow {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	whPH, whArgs := whereWarehouseArgs()

	// 订单数 + 重量(克→kg), 按 平台/渠道
	qOrders := fmt.Sprintf(`SELECT m.platform, m.channel, COUNT(*) AS orders,
		IFNULL(SUM(t.estimate_weight),0)/1000 AS weight_kg
		FROM %s t JOIN dim_sales_channel_map m ON m.shop_name=t.shop_name
		WHERE t.trade_type=1 AND t.consign_time>=? AND t.consign_time<? AND t.warehouse_name IN (%s)
		GROUP BY m.platform, m.channel`, part, whPH)
	// 单瓶数(sell_count×箱规, 缺箱规×1), 按 渠道
	qBottles := fmt.Sprintf(`SELECT m.channel, IFNULL(SUM(tg.sell_count*COALESCE(p.box_qty,1)),0) AS bottles
		FROM %s t JOIN %s tg ON tg.trade_id=t.trade_id
		JOIN dim_sales_channel_map m ON m.shop_name=t.shop_name
		LEFT JOIN dim_goods_pack_spec p ON p.goods_no=tg.goods_no
		WHERE t.trade_type=1 AND t.consign_time>=? AND t.consign_time<? AND t.warehouse_name IN (%s)
		GROUP BY m.channel`, part, partG, whPH)

	args := append([]interface{}{start, end}, whArgs...)
	byCh := map[string]*ChannelRow{}
	rows, err := h.DB.QueryContext(ctx, qOrders, args...)
	if err != nil {
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
		return nil
	}
	defer rows.Close()
	var out []GoodsRow
	for rows.Next() {
		var g GoodsRow
		var pallet sql.NullFloat64
		if err := rows.Scan(&g.GoodsNo, &g.GoodsName, &g.Orders, &g.Bottles, &g.Boxes, &pallet); err != nil {
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
	// 防长篮子被 group_concat 截断
	if _, err := h.DB.ExecContext(ctx, "SET SESSION group_concat_max_len = 100000"); err != nil {
		return nil
	}
	whPH, whArgs := whereWarehouseArgs()
	q := fmt.Sprintf(`SELECT sig_display, COUNT(*) AS orders,
		IFNULL(SUM(ob),0) AS bottles, IFNULL(SUM(ow),0)/1000 AS weight_kg
		FROM (
		  SELECT t.trade_id, MAX(t.estimate_weight) AS ow,
		    GROUP_CONCAT(tg.goods_no,'#',CAST(tg.sell_count AS UNSIGNED) ORDER BY tg.goods_no SEPARATOR '|') AS sig,
		    GROUP_CONCAT(tg.goods_name,'(',CAST(tg.sell_count AS UNSIGNED),')' ORDER BY tg.goods_no SEPARATOR ', ') AS sig_display,
		    SUM(tg.sell_count*COALESCE(p.box_qty,1)) AS ob
		  FROM %s t JOIN %s tg ON tg.trade_id=t.trade_id
		  LEFT JOIN dim_goods_pack_spec p ON p.goods_no=tg.goods_no
		  WHERE t.trade_type=1 AND t.consign_time>=? AND t.consign_time<? AND t.warehouse_name IN (%s)
		  GROUP BY t.trade_id
		) o GROUP BY sig, sig_display ORDER BY orders DESC LIMIT 10`, part, partG, whPH)
	args := append([]interface{}{start, end}, whArgs...)
	rows, err := h.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []ComboRow
	for rows.Next() {
		var c ComboRow
		if err := rows.Scan(&c.Display, &c.Orders, &c.Bottles, &c.WeightKg); err != nil {
			return nil
		}
		out = append(out, c)
	}
	return out
}
