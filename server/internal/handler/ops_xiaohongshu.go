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

// xhsRowScanner 兼容 *sql.Rows 和 queryRowsOrWriteError 返回的 *rowsWithCancel
type xhsRowScanner interface {
	Next() bool
	Scan(dest ...interface{}) error
	Err() error
}

// xhsCol 千帆明细一列的数据驱动定义(笔记/商品共用):
//   Key=JSON键(也是前端 dataIndex) / Label=表头 / Sel=SELECT 表达式 / Fmt=格式 / Group=分组
//   维度列 Fmt="text"(Sel 用 ANY_VALUE/MIN, 扫描成字符串); 量列 SUM; 率列用 总量÷总量*100 加权重算(禁简单平均)。
//   ⚠ 部分率列分母口径按经验假设(见各列注释 [假设]), 发版前待业务核对, 改对应 Sel 即可。
type xhsCol struct{ Key, Label, Sel, Fmt, Group string }

// 千帆-笔记效果 全部可选指标(标题/ID 在前端固定常显, 不在此列)
var xhsNoteCols = []xhsCol{
	// 笔记属性
	{"author", "作者昵称", "ANY_VALUE(author_name)", "text", "笔记属性"},
	{"createTime", "笔记创建时间", "MIN(note_create_time)", "text", "笔记属性"},
	{"type", "笔记类型", "ANY_VALUE(note_type)", "text", "笔记属性"},
	{"product", "关联商品名称", "ANY_VALUE(related_product_name)", "text", "笔记属性"},
	{"video_duration_sec", "视频时长(秒)", "IFNULL(MAX(video_duration_sec),0)", "num2", "笔记属性"},
	// 流量互动
	{"read_count", "笔记阅读数", "IFNULL(SUM(read_count),0)", "int", "流量互动"},
	{"like_count", "点赞次数", "IFNULL(SUM(like_count),0)", "int", "流量互动"},
	{"collect_count", "收藏次数", "IFNULL(SUM(collect_count),0)", "int", "流量互动"},
	{"comment_count", "评论次数", "IFNULL(SUM(comment_count),0)", "int", "流量互动"},
	{"share_count", "分享次数", "IFNULL(SUM(share_count),0)", "int", "流量互动"},
	{"follow_count", "点击关注次数", "IFNULL(SUM(follow_count),0)", "int", "流量互动"},
	{"danmu_count", "弹幕次数", "IFNULL(SUM(danmu_count),0)", "int", "流量互动"},
	{"avg_read_duration", "平均阅读时长", "IFNULL(SUM(avg_read_duration*read_count)/NULLIF(SUM(read_count),0),0)", "num2", "流量互动"},
	{"finish_rate_pv", "完播率(PV)", "IFNULL(SUM(finish_rate_pv*read_count)/NULLIF(SUM(read_count),0)*100,0)", "rate", "流量互动"},
	// 商品点击
	{"product_click_pv", "笔记商品点击次数", "IFNULL(SUM(product_click_pv),0)", "int", "商品点击"},
	{"product_click_uv", "笔记商品点击人数", "IFNULL(SUM(product_click_uv),0)", "int", "商品点击"},
	{"product_click_rate_pv", "笔记商品点击率(PV)", "IFNULL(SUM(product_click_pv)/NULLIF(SUM(read_count),0)*100,0)", "rate", "商品点击"},
	{"add_cart_qty", "笔记加购件数", "IFNULL(SUM(add_cart_qty),0)", "int", "商品点击"},
	// 支付转化
	{"pay_amount", "笔记支付金额", "IFNULL(SUM(pay_amount),0)", "money", "支付转化"},
	{"pay_order_count", "笔记支付订单数", "IFNULL(SUM(pay_order_count),0)", "int", "支付转化"},
	{"pay_uv", "笔记支付人数", "IFNULL(SUM(pay_uv),0)", "int", "支付转化"},
	{"pay_conv_rate_pv", "笔记支付转化率(PV)", "IFNULL(SUM(pay_order_count)/NULLIF(SUM(product_click_pv),0)*100,0)", "rate", "支付转化"},
	{"pay_conv_rate_uv", "笔记支付转化率(UV)", "IFNULL(SUM(pay_uv)/NULLIF(SUM(product_click_uv),0)*100,0)", "rate", "支付转化"}, // [假设]人数口径
	// 退款
	{"refund_amount_by_refund", "笔记退款金额(退款时间)", "IFNULL(SUM(refund_amount_by_refund),0)", "money", "退款"},
	{"refund_order_by_refund", "笔记退款订单数(退款时间)", "IFNULL(SUM(refund_order_by_refund),0)", "int", "退款"},
	{"refund_uv_by_refund", "笔记退款人数(退款时间)", "IFNULL(SUM(refund_uv_by_refund),0)", "int", "退款"},
	{"refund_amount_by_pay", "笔记退款金额(支付时间)", "IFNULL(SUM(refund_amount_by_pay),0)", "money", "退款"},
	{"refund_order_by_pay", "笔记退款订单数(支付时间)", "IFNULL(SUM(refund_order_by_pay),0)", "int", "退款"},
	{"refund_rate_by_pay", "笔记退款率(支付时间)", "IFNULL(SUM(refund_amount_by_pay)/NULLIF(SUM(pay_amount),0)*100,0)", "rate", "退款"}, // [假设]金额口径
	// 引流
	{"to_shop_home_pv", "引流店铺主页次数", "IFNULL(SUM(to_shop_home_pv),0)", "int", "引流"},
	{"to_shop_home_pay_amount", "引流店铺主页支付金额", "IFNULL(SUM(to_shop_home_pay_amount),0)", "money", "引流"},
	{"to_live_pv", "引流直播间次数", "IFNULL(SUM(to_live_pv),0)", "int", "引流"},
	{"to_live_pay_amount", "引流直播间支付金额", "IFNULL(SUM(to_live_pay_amount),0)", "money", "引流"},
}

// 默认显示的笔记列(localStorage 没存过时用; 与改造前展示列一致, 其余在弹窗按需勾)
var xhsNoteDefaultKeys = []string{"author", "createTime", "type", "product", "pay_amount", "product_click_pv", "product_click_rate_pv", "pay_conv_rate_pv", "refund_amount_by_refund", "add_cart_qty", "to_shop_home_pay_amount", "finish_rate_pv"}

// 千帆-商品销售 全部可选指标(商品名在前端固定常显, 不在此列)
var xhsGoodsCols = []xhsCol{
	// 商品属性
	{"category_l1", "一级品类", "ANY_VALUE(category_l1)", "text", "商品属性"},
	{"category_l2", "二级品类", "ANY_VALUE(category_l2)", "text", "商品属性"},
	{"brand", "品牌", "ANY_VALUE(brand)", "text", "商品属性"},
	// 流量
	{"visitor_count", "商品访客数", "IFNULL(SUM(visitor_count),0)", "int", "流量"},
	{"view_count", "商品浏览量", "IFNULL(SUM(view_count),0)", "int", "流量"},
	{"add_cart_uv", "新增加购人数", "IFNULL(SUM(add_cart_uv),0)", "int", "流量"},
	{"add_cart_qty", "新增加购件数", "IFNULL(SUM(add_cart_qty),0)", "int", "流量"},
	{"add_wishlist_uv", "加入心愿单人数", "IFNULL(SUM(add_wishlist_uv),0)", "int", "流量"},
	// 支付
	{"pay_amount", "支付金额", "IFNULL(SUM(pay_amount),0)", "money", "支付"},
	{"pay_buyer_count", "支付买家数", "IFNULL(SUM(pay_buyer_count),0)", "int", "支付"},
	{"pay_order_count", "支付订单数", "IFNULL(SUM(pay_order_count),0)", "int", "支付"},
	{"pay_qty", "支付件数", "IFNULL(SUM(pay_qty),0)", "int", "支付"},
	{"pay_conv_rate", "支付转化率", "IFNULL(SUM(pay_buyer_count)/NULLIF(SUM(visitor_count),0)*100,0)", "rate", "支付"}, // [假设]买家数/访客数
	{"pay_conv_rate_pv", "支付转化率(PV)", "IFNULL(SUM(pay_order_count)/NULLIF(SUM(view_count),0)*100,0)", "rate", "支付"}, // [假设]订单数/浏览量
	{"avg_order_amount", "客单价", "IFNULL(SUM(pay_amount)/NULLIF(SUM(pay_order_count),0),0)", "money", "支付"},
	// 退款
	{"refund_amount_by_refund", "退款金额(退款时间)", "IFNULL(SUM(refund_amount_by_refund),0)", "money", "退款"},
	{"refund_buyer_by_refund", "退款买家数(退款时间)", "IFNULL(SUM(refund_buyer_by_refund),0)", "int", "退款"},
	{"refund_order_by_refund", "退款订单数(退款时间)", "IFNULL(SUM(refund_order_by_refund),0)", "int", "退款"},
	{"refund_amount_by_pay", "退款金额(支付时间)", "IFNULL(SUM(refund_amount_by_pay),0)", "money", "退款"},
	{"refund_order_by_pay", "退款订单数(支付时间)", "IFNULL(SUM(refund_order_by_pay),0)", "int", "退款"},
	{"refund_rate_by_pay", "退款率(支付时间)", "IFNULL(SUM(refund_amount_by_pay)/NULLIF(SUM(pay_amount),0)*100,0)", "rate", "退款"}, // [假设]金额口径
	{"pre_ship_refund_rate", "发货前退款率(支付时间)", "IFNULL(SUM(pre_ship_refund_rate*pay_amount)/NULLIF(SUM(pay_amount),0)*100,0)", "rate", "退款"}, // [假设]按支付额加权
	{"post_ship_refund_rate", "发货后退款率(支付时间)", "IFNULL(SUM(post_ship_refund_rate*pay_amount)/NULLIF(SUM(pay_amount),0)*100,0)", "rate", "退款"}, // [假设]按支付额加权
	{"net_pay_amount", "退款后支付金额(支付时间)", "IFNULL(SUM(net_pay_amount),0)", "money", "退款"},
}

var xhsGoodsDefaultKeys = []string{"category_l1", "visitor_count", "add_cart_qty", "pay_amount", "pay_order_count", "pay_qty", "avg_order_amount", "refund_amount_by_pay"}

// xhsColMeta 把 xhsCol 列表转成给前端的列元信息(key/label/fmt/group)
func xhsColMeta(cols []xhsCol) []map[string]string {
	out := make([]map[string]string, 0, len(cols))
	for _, c := range cols {
		out = append(out, map[string]string{"key": c.Key, "label": c.Label, "fmt": c.Fmt, "group": c.Group})
	}
	return out
}

// scanXhsDetail 动态扫描明细行: fixedKeys 为前置固定字符串列(标题/ID/url 或 商品名), cols 按 Fmt 区分字符串/数值
func scanXhsDetail(rows xhsRowScanner, fixedKeys []string, cols []xhsCol) ([]map[string]interface{}, error) {
	out := []map[string]interface{}{}
	for rows.Next() {
		fixedVals := make([]string, len(fixedKeys))
		colStr := make([]string, len(cols))
		colNum := make([]float64, len(cols))
		scanArgs := make([]interface{}, 0, len(fixedKeys)+len(cols))
		for i := range fixedKeys {
			scanArgs = append(scanArgs, &fixedVals[i])
		}
		for i, c := range cols {
			if c.Fmt == "text" {
				scanArgs = append(scanArgs, &colStr[i])
			} else {
				scanArgs = append(scanArgs, &colNum[i])
			}
		}
		if err := rows.Scan(scanArgs...); err != nil {
			return nil, err
		}
		row := map[string]interface{}{}
		for i, k := range fixedKeys {
			row[k] = fixedVals[i]
		}
		for i, c := range cols {
			if c.Fmt == "text" {
				row[c.Key] = colStr[i]
			} else {
				row[c.Key] = colNum[i]
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

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
		"latestDate":   latest,
		"shops":        readCol(`SELECT DISTINCT shop_name FROM op_xhs_note_daily ORDER BY shop_name`),
		"noteTypes":    readCol(`SELECT DISTINCT note_type FROM op_xhs_note_daily WHERE note_type<>'' ORDER BY note_type`),
		"categories":   readCol(`SELECT DISTINCT category_l1 FROM op_xhs_goods_daily WHERE category_l1<>'' ORDER BY category_l1`),
		"noteColumns":     xhsColMeta(xhsNoteCols),
		"goodsColumns":    xhsColMeta(xhsGoodsCols),
		"noteDefaultKeys": xhsNoteDefaultKeys,
		"goodsDefaultKeys": xhsGoodsDefaultKeys,
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

	// 明细 TOP50：按笔记聚合(跨天加总)，带 note_id 供下钻看单条每天趋势。数据驱动全字段(见 xhsNoteCols)。
	// 固定前置: note_id / 标题 / url(仅 http 输出, 源是 HYPERLINK 公式 import 已提真链接)。
	// 率类用 总量÷总量 加权重算(禁简单平均), 已验证: 点击率(PV)=Σ商品点击次数/Σ阅读数; 支付转化率(PV)=Σ支付订单/Σ商品点击。
	noteSel := []string{"note_id", "ANY_VALUE(note_title)", "ANY_VALUE(CASE WHEN note_url LIKE 'http%' THEN note_url ELSE '' END)"}
	for _, c := range xhsNoteCols {
		noteSel = append(noteSel, c.Sel)
	}
	dRows, ok := queryRowsOrWriteError(w, r, h.DB, `SELECT `+strings.Join(noteSel, ", ")+
		` FROM op_xhs_note_daily WHERE 1=1`+whereSQL+` GROUP BY note_id ORDER BY SUM(pay_amount) DESC, SUM(read_count) DESC LIMIT 50`, whereArgs...)
	if !ok {
		return
	}
	defer dRows.Close()
	detail, derr := scanXhsDetail(dRows, []string{"noteId", "title", "url"}, xhsNoteCols)
	if writeDatabaseError(w, derr) {
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

	// 商品销售同笔记：每日增量快照。数据更新时间 start/end → stat_date 范围，看一个月跨天聚合。
	// 默认 business_type='全部' AND carrier='全部'（每商品一行总口径，避免切片重复 SUM）。
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
	start := strings.TrimSpace(r.URL.Query().Get("start"))
	end := strings.TrimSpace(r.URL.Query().Get("end"))
	updCond := ""
	var updArgs []interface{}
	if start != "" && end != "" {
		updCond = " AND stat_date BETWEEN ? AND ?"
		updArgs = append(updArgs, start, end)
	}
	whereSQL := cond + updCond
	whereArgs := append(append([]interface{}{}, condArgs...), updArgs...)

	// KPI：量类跨天 SUM，商品数 COUNT(DISTINCT product_id)
	type goodsKPI struct {
		Goods    int     `json:"goods"`
		Visitors int     `json:"visitors"`
		GMV      float64 `json:"gmv"`
		Orders   int     `json:"orders"`
		Qty      int     `json:"qty"`
		Refund   float64 `json:"refund"`
	}
	var k goodsKPI
	if err := h.DB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT product_id), IFNULL(SUM(visitor_count),0),
		IFNULL(SUM(pay_amount),0), IFNULL(SUM(pay_order_count),0),
		IFNULL(SUM(pay_qty),0), IFNULL(SUM(refund_amount_by_pay),0)
		FROM op_xhs_goods_daily WHERE 1=1`+whereSQL, whereArgs...).
		Scan(&k.Goods, &k.Visitors, &k.GMV, &k.Orders, &k.Qty, &k.Refund); err != nil {
		writeDatabaseError(w, err)
		return
	}

	// 趋势：按数据更新日
	type tPoint struct {
		Date     string  `json:"date"`
		GMV      float64 `json:"gmv"`
		Visitors int     `json:"visitors"`
	}
	trend := []tPoint{}
	tRows, ok := queryRowsOrWriteError(w, r, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
		IFNULL(SUM(pay_amount),0), IFNULL(SUM(visitor_count),0)
		FROM op_xhs_goods_daily WHERE 1=1`+whereSQL+` GROUP BY stat_date ORDER BY stat_date`, whereArgs...)
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

	// 明细 TOP50：按商品聚合(跨天加总)，数据驱动全字段(见 xhsGoodsCols)。客单价/转化率用 总量÷总量 重算(禁简单平均)。
	// 固定前置: 商品名(name)。
	goodsSel := []string{"ANY_VALUE(product_name)"}
	for _, c := range xhsGoodsCols {
		goodsSel = append(goodsSel, c.Sel)
	}
	dRows, ok := queryRowsOrWriteError(w, r, h.DB, `SELECT `+strings.Join(goodsSel, ", ")+
		` FROM op_xhs_goods_daily WHERE 1=1`+whereSQL+` GROUP BY product_id ORDER BY SUM(pay_amount) DESC LIMIT 50`, whereArgs...)
	if !ok {
		return
	}
	defer dRows.Close()
	detail, derr := scanXhsDetail(dRows, []string{"name"}, xhsGoodsCols)
	if writeDatabaseError(w, derr) {
		return
	}

	writeJSON(w, map[string]interface{}{
		"kpi": k, "trend": trend, "detail": detail,
		"dateRange": map[string]string{"start": start, "end": end},
	})
}
