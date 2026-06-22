package handler

// 小红书看板（社媒部门）只读接口：filters / note / note-trend / goods。
// 数据源 op_xhs_note_daily / op_xhs_goods_daily（每日增量快照：每天文件=当天数据，非累计）。
// 笔记效果口径：两个时间筛选——数据更新时间(stat_date) + 笔记发布时间(note_create_time)。
//   看一个月要正确聚合：量类跨天 SUM；笔记数 COUNT(DISTINCT note_id)；率类用 总量÷总量 重算(禁简单平均)。
//   note-trend 提供单条笔记按数据更新日的每天走势(明细行下钻)。
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

	// 口径：笔记效果按两个时间筛选——
	//   数据更新时间 start/end → stat_date 范围(看哪几天的数据)
	//   笔记发布时间 pub_start/pub_end → note_create_time 范围(筛哪些笔记)
	// 每日增量快照：看一个月要跨天 SUM；笔记数按 note_id 去重；率类用 总量÷总量 重算。
	noteType := strings.TrimSpace(r.URL.Query().Get("note_type"))
	cond, condArgs := xhsCond(r, "note_type", noteType)
	start := strings.TrimSpace(r.URL.Query().Get("start"))
	end := strings.TrimSpace(r.URL.Query().Get("end"))
	updCond := ""
	var updArgs []interface{}
	if start != "" && end != "" {
		updCond = " AND stat_date BETWEEN ? AND ?"
		updArgs = append(updArgs, start, end)
	}
	pubStart := strings.TrimSpace(r.URL.Query().Get("pub_start"))
	pubEnd := strings.TrimSpace(r.URL.Query().Get("pub_end"))
	pubCond := ""
	var pubArgs []interface{}
	if pubStart != "" {
		pubCond += " AND note_create_time >= ?"
		pubArgs = append(pubArgs, pubStart)
	}
	if pubEnd != "" {
		pubCond += " AND note_create_time <= ?"
		pubArgs = append(pubArgs, pubEnd+" 23:59:59")
	}
	// 笔记ID 搜索(含模糊匹配，可贴完整或部分 ID)
	noteIDLike := strings.TrimSpace(r.URL.Query().Get("note_id_like"))
	idCond := ""
	var idArgs []interface{}
	if noteIDLike != "" {
		idCond = " AND note_id LIKE ?"
		idArgs = append(idArgs, "%"+noteIDLike+"%")
	}
	whereSQL := cond + updCond + pubCond + idCond
	whereArgs := append(append(append(append([]interface{}{}, condArgs...), updArgs...), pubArgs...), idArgs...)

	// KPI：量类跨天 SUM，笔记数去重，转化率=总支付人数÷总点击人数(加权重算)
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

	// 明细 TOP50：按笔记聚合(跨天加总)，带 note_id 供下钻看单条每天趋势；note_url 仅 http 才输出
	type noteRow struct {
		NoteID  string  `json:"noteId"`
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
	dRows, ok := queryRowsOrWriteError(w, r, h.DB, `SELECT note_id, ANY_VALUE(note_title), ANY_VALUE(note_type), ANY_VALUE(author_name),
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
		if writeDatabaseError(w, dRows.Scan(&d.NoteID, &d.Title, &d.Type, &d.Author, &d.PubDate, &d.Read, &d.Like, &d.Collect, &d.Comment, &d.Share, &d.GMV, &d.Product, &d.URL)) {
			return
		}
		detail = append(detail, d)
	}
	if writeDatabaseError(w, dRows.Err()) {
		return
	}

	writeJSON(w, map[string]interface{}{
		"kpi": k, "detail": detail,
		"dateRange":    map[string]string{"start": start, "end": end},
		"publishRange": map[string]string{"start": pubStart, "end": pubEnd},
	})
}

// GetXhsNoteTrend GET /api/xiaohongshu/note-trend —— 单条笔记按数据更新日的每天走势(明细行下钻)
// note_id 必填；start/end = 数据更新时间(stat_date)范围。每天一条(同 note_id 跨店保险起见 SUM)。
func (h *DashboardHandler) GetXhsNoteTrend(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDeptAccess(r, "social")) {
		return
	}
	noteID := strings.TrimSpace(r.URL.Query().Get("note_id"))
	if noteID == "" {
		writeJSON(w, map[string]interface{}{"trend": []interface{}{}})
		return
	}
	args := []interface{}{noteID}
	cond := ""
	start := strings.TrimSpace(r.URL.Query().Get("start"))
	end := strings.TrimSpace(r.URL.Query().Get("end"))
	if start != "" && end != "" {
		cond = " AND stat_date BETWEEN ? AND ?"
		args = append(args, start, end)
	}
	type tPoint struct {
		Date   string  `json:"date"`
		Reads  int     `json:"reads"`
		GMV    float64 `json:"gmv"`
		Orders int     `json:"orders"`
	}
	trend := []tPoint{}
	rows, ok := queryRowsOrWriteError(w, r, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
		IFNULL(SUM(read_count),0), IFNULL(SUM(pay_amount),0), IFNULL(SUM(pay_order_count),0)
		FROM op_xhs_note_daily WHERE note_id=?`+cond+` GROUP BY stat_date ORDER BY stat_date`, args...)
	if !ok {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var p tPoint
		if writeDatabaseError(w, rows.Scan(&p.Date, &p.Reads, &p.GMV, &p.Orders)) {
			return
		}
		trend = append(trend, p)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}
	writeJSON(w, map[string]interface{}{"trend": trend})
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
