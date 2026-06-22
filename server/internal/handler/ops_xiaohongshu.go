package handler

// 小红书看板（社媒部门）三只读接口：filters / note / goods。
// 数据源 op_xhs_note_daily / op_xhs_goods_daily（每日全量快照）。
// 口径铁律：禁止跨天 SUM —— KPI/明细固定单日(默认最新)，趋势按 stat_date 分组。
// 商品默认 business_type='全部' AND carrier='全部'（每商品一行总口径，避免切片重复）。

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// xhsCond 据 shops(逗号分隔) + 额外等值条件 拼 WHERE 片段和参数
func xhsCond(r *http.Request, extraCol, extraVal string) (string, []interface{}) {
	cond := ""
	var args []interface{}
	shops := strings.TrimSpace(r.URL.Query().Get("shops"))
	if shops != "" {
		ph := make([]string, 0)
		for _, p := range strings.Split(shops, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			ph = append(ph, "?")
			args = append(args, p)
		}
		if len(ph) > 0 {
			cond += " AND shop_name IN (" + strings.Join(ph, ",") + ")"
		}
	}
	if extraCol != "" && extraVal != "" {
		cond += " AND " + extraCol + "=?"
		args = append(args, extraVal)
	}
	return cond, args
}

// resolveXhsDate 取 date 参数, 空则查该表最新 stat_date
func (h *DashboardHandler) resolveXhsDate(ctx context.Context, table, date string) string {
	date = strings.TrimSpace(date)
	if date != "" {
		return date
	}
	var latest string
	h.DB.QueryRowContext(ctx, "SELECT IFNULL(DATE_FORMAT(MAX(stat_date),'%Y-%m-%d'),'') FROM "+table).Scan(&latest)
	return latest
}

// GetXhsFilters GET /api/xiaohongshu/filters
func (h *DashboardHandler) GetXhsFilters(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDeptAccess(r, "social")) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	var latest string
	if err := h.DB.QueryRowContext(ctx, `SELECT IFNULL(DATE_FORMAT(MAX(stat_date),'%Y-%m-%d'),'') FROM op_xhs_note_daily`).Scan(&latest); err != nil {
		writeDatabaseError(w, err)
		return
	}
	// 容错读取单列列表：出错则该列返回空数组（不二次写响应）
	readCol := func(q string) []string {
		out := []string{}
		rows, err := h.DB.QueryContext(ctx, q)
		if err != nil {
			return out
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			if rows.Scan(&s) != nil {
				continue
			}
			if strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	}
	writeJSON(w, map[string]interface{}{
		"latestDate": latest,
		"shops":      readCol(`SELECT DISTINCT shop_name FROM op_xhs_note_daily ORDER BY shop_name`),
		"noteTypes":  readCol(`SELECT DISTINCT note_type FROM op_xhs_note_daily WHERE note_type<>'' ORDER BY note_type`),
		"categories": readCol(`SELECT DISTINCT category_l1 FROM op_xhs_goods_daily WHERE category_l1<>'' ORDER BY category_l1`),
	})
}

// GetXhsNote GET /api/xiaohongshu/note —— 笔记效果
func (h *DashboardHandler) GetXhsNote(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDeptAccess(r, "social")) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// 口径：本"笔记效果"页按【笔记发布日期 note_create_time】筛选与组织。
	// 指标(阅读/互动/GMV…)是每日增量快照，一条笔记的总表现 = 跨所有快照日加总(不能只取单日)。
	// start/end = 顶部时间框选的"发布日期"范围；note_create_time 是 'YYYY-MM-DD HH:MM:SS' 字符串，字典序可比。
	noteType := strings.TrimSpace(r.URL.Query().Get("note_type"))
	cond, condArgs := xhsCond(r, "note_type", noteType)
	start := strings.TrimSpace(r.URL.Query().Get("start"))
	end := strings.TrimSpace(r.URL.Query().Get("end"))
	pubCond := ""
	var pubArgs []interface{}
	if start != "" {
		pubCond += " AND note_create_time >= ?"
		pubArgs = append(pubArgs, start)
	}
	if end != "" {
		pubCond += " AND note_create_time <= ?"
		pubArgs = append(pubArgs, end+" 23:59:59")
	}
	whereSQL := cond + pubCond
	whereArgs := append(append([]interface{}{}, condArgs...), pubArgs...)

	// KPI：发布于该期间的笔记，跨所有快照日加总
	type noteKPI struct {
		Notes    int     `json:"notes"`
		Reads    int     `json:"reads"`
		Interact int     `json:"interact"`
		GMV      float64 `json:"gmv"`
		Orders   int     `json:"orders"`
		ConvRate float64 `json:"convRate"`
	}
	var k noteKPI
	var payUV, clickUV float64
	if err := h.DB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT note_id), IFNULL(SUM(read_count),0),
		IFNULL(SUM(like_count+collect_count+comment_count+share_count),0),
		IFNULL(SUM(pay_amount),0), IFNULL(SUM(pay_order_count),0),
		IFNULL(SUM(pay_uv),0), IFNULL(SUM(product_click_uv),0)
		FROM op_xhs_note_daily WHERE 1=1`+whereSQL, whereArgs...).
		Scan(&k.Notes, &k.Reads, &k.Interact, &k.GMV, &k.Orders, &payUV, &clickUV); err != nil {
		writeDatabaseError(w, err)
		return
	}
	if clickUV > 0 {
		k.ConvRate = payUV / clickUV
	}

	// 趋势：按笔记发布日聚合（每根柱=该日发布的笔记累计带来的阅读/GMV）
	type tPoint struct {
		Date  string  `json:"date"`
		Reads int     `json:"reads"`
		GMV   float64 `json:"gmv"`
	}
	trend := []tPoint{}
	// GROUP BY/ORDER BY 必须与 SELECT 表达式完全一致, 否则 only_full_group_by 报错
	tRows, ok := queryRowsOrWriteError(w, r, h.DB, `SELECT DATE_FORMAT(note_create_time,'%Y-%m-%d'),
		IFNULL(SUM(read_count),0), IFNULL(SUM(pay_amount),0)
		FROM op_xhs_note_daily WHERE 1=1`+whereSQL+` GROUP BY DATE_FORMAT(note_create_time,'%Y-%m-%d') ORDER BY DATE_FORMAT(note_create_time,'%Y-%m-%d')`, whereArgs...)
	if !ok {
		return
	}
	defer tRows.Close()
	for tRows.Next() {
		var p tPoint
		if writeDatabaseError(w, tRows.Scan(&p.Date, &p.Reads, &p.GMV)) {
			return
		}
		trend = append(trend, p)
	}
	if writeDatabaseError(w, tRows.Err()) {
		return
	}

	// 明细 TOP50：按笔记聚合（跨快照日加总），note_url 仅 http 才输出
	type noteRow struct {
		Title   string  `json:"title"`
		Type    string  `json:"type"`
		Author  string  `json:"author"`
		PubDate string  `json:"pubDate"`
		Read    int     `json:"read"`
		Like    int     `json:"like"`
		Collect int     `json:"collect"`
		Comment int     `json:"comment"`
		Share   int     `json:"share"`
		GMV     float64 `json:"gmv"`
		Product string  `json:"product"`
		URL     string  `json:"url"`
	}
	detail := []noteRow{}
	dRows, ok := queryRowsOrWriteError(w, r, h.DB, `SELECT ANY_VALUE(note_title), ANY_VALUE(note_type), ANY_VALUE(author_name),
		DATE_FORMAT(MIN(note_create_time),'%Y-%m-%d'),
		IFNULL(SUM(read_count),0), IFNULL(SUM(like_count),0), IFNULL(SUM(collect_count),0),
		IFNULL(SUM(comment_count),0), IFNULL(SUM(share_count),0), IFNULL(SUM(pay_amount),0),
		ANY_VALUE(related_product_name),
		ANY_VALUE(CASE WHEN note_url LIKE 'http%' THEN note_url ELSE '' END)
		FROM op_xhs_note_daily WHERE 1=1`+whereSQL+` GROUP BY note_id ORDER BY SUM(pay_amount) DESC, SUM(read_count) DESC LIMIT 50`, whereArgs...)
	if !ok {
		return
	}
	defer dRows.Close()
	for dRows.Next() {
		var d noteRow
		if writeDatabaseError(w, dRows.Scan(&d.Title, &d.Type, &d.Author, &d.PubDate, &d.Read, &d.Like, &d.Collect, &d.Comment, &d.Share, &d.GMV, &d.Product, &d.URL)) {
			return
		}
		detail = append(detail, d)
	}
	if writeDatabaseError(w, dRows.Err()) {
		return
	}

	writeJSON(w, map[string]interface{}{
		"kpi": k, "trend": trend, "detail": detail,
		"dateRange": map[string]string{"start": start, "end": end},
	})
}

// GetXhsGoods GET /api/xiaohongshu/goods —— 商品销售（默认 全部×全部）
func (h *DashboardHandler) GetXhsGoods(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDeptAccess(r, "social")) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	date := h.resolveXhsDate(ctx, "op_xhs_goods_daily", r.URL.Query().Get("date"))
	bizType := strings.TrimSpace(r.URL.Query().Get("business_type"))
	if bizType == "" {
		bizType = "全部"
	}
	carrier := strings.TrimSpace(r.URL.Query().Get("carrier"))
	if carrier == "" {
		carrier = "全部"
	}
	cat := strings.TrimSpace(r.URL.Query().Get("category_l1"))

	cond, condArgs := xhsCond(r, "business_type", bizType)
	cond += " AND carrier=?"
	condArgs = append(condArgs, carrier)
	if cat != "" {
		cond += " AND category_l1=?"
		condArgs = append(condArgs, cat)
	}

	type goodsKPI struct {
		Goods    int     `json:"goods"`
		Visitors int     `json:"visitors"`
		GMV      float64 `json:"gmv"`
		Orders   int     `json:"orders"`
		Qty      int     `json:"qty"`
		Refund   float64 `json:"refund"`
	}
	var k goodsKPI
	kArgs := append([]interface{}{date}, condArgs...)
	if err := h.DB.QueryRowContext(ctx, `SELECT COUNT(*), IFNULL(SUM(visitor_count),0),
		IFNULL(SUM(pay_amount),0), IFNULL(SUM(pay_order_count),0),
		IFNULL(SUM(pay_qty),0), IFNULL(SUM(refund_amount_by_pay),0)
		FROM op_xhs_goods_daily WHERE stat_date=?`+cond, kArgs...).
		Scan(&k.Goods, &k.Visitors, &k.GMV, &k.Orders, &k.Qty, &k.Refund); err != nil {
		writeDatabaseError(w, err)
		return
	}

	start := strings.TrimSpace(r.URL.Query().Get("start"))
	end := strings.TrimSpace(r.URL.Query().Get("end"))
	type tPoint struct {
		Date     string  `json:"date"`
		GMV      float64 `json:"gmv"`
		Visitors int     `json:"visitors"`
	}
	trend := []tPoint{}
	tWhere := "1=1"
	tArgs := append([]interface{}{}, condArgs...)
	if start != "" && end != "" {
		tWhere = "stat_date BETWEEN ? AND ?"
		tArgs = append([]interface{}{start, end}, condArgs...)
	}
	tRows, ok := queryRowsOrWriteError(w, r, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
		IFNULL(SUM(pay_amount),0), IFNULL(SUM(visitor_count),0)
		FROM op_xhs_goods_daily WHERE `+tWhere+cond+` GROUP BY stat_date ORDER BY stat_date`, tArgs...)
	if !ok {
		return
	}
	defer tRows.Close()
	for tRows.Next() {
		var p tPoint
		if writeDatabaseError(w, tRows.Scan(&p.Date, &p.GMV, &p.Visitors)) {
			return
		}
		trend = append(trend, p)
	}
	if writeDatabaseError(w, tRows.Err()) {
		return
	}

	type goodsRow struct {
		Name     string  `json:"name"`
		Cat1     string  `json:"cat1"`
		Cat2     string  `json:"cat2"`
		Visitors int     `json:"visitors"`
		Views    int     `json:"views"`
		Cart     int     `json:"cart"`
		GMV      float64 `json:"gmv"`
		Orders   int     `json:"orders"`
		Qty      int     `json:"qty"`
		ConvRate float64 `json:"convRate"`
		AOV      float64 `json:"aov"`
		Refund   float64 `json:"refund"`
	}
	detail := []goodsRow{}
	dRows, ok := queryRowsOrWriteError(w, r, h.DB, `SELECT product_name, category_l1, category_l2,
		visitor_count, view_count, add_cart_qty, pay_amount, pay_order_count, pay_qty,
		pay_conv_rate, avg_order_amount, refund_amount_by_pay
		FROM op_xhs_goods_daily WHERE stat_date=?`+cond+` ORDER BY pay_amount DESC LIMIT 50`, kArgs...)
	if !ok {
		return
	}
	defer dRows.Close()
	for dRows.Next() {
		var d goodsRow
		if writeDatabaseError(w, dRows.Scan(&d.Name, &d.Cat1, &d.Cat2, &d.Visitors, &d.Views, &d.Cart, &d.GMV, &d.Orders, &d.Qty, &d.ConvRate, &d.AOV, &d.Refund)) {
			return
		}
		detail = append(detail, d)
	}
	if writeDatabaseError(w, dRows.Err()) {
		return
	}

	writeJSON(w, map[string]interface{}{
		"kpi": k, "trend": trend, "detail": detail, "date": date,
		"dateRange": map[string]string{"start": start, "end": end},
	})
}
