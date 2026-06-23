package handler

// 小红书乘风看板（社媒部门）只读接口：chengfeng/filters · chengfeng/list · chengfeng/note-trend。
// 数据源 op_xhs_chengfeng_daily（每日每店每笔记信息流投流，145 字段）。
// 明细按 note_id 聚合区间（跨天）：量类 SUM；率类用 总量÷总量 加权重算（禁简单平均），口径见 cfMetrics 各列 expr。
// 列配置 cfMetrics 由 scratchpad/cf_cols_contract.tsv 生成（跑哥 2026-06-23 指定的 107 指标列顺序）。
// 注：少数率列分母口径存疑（行动按钮点击率/搜索组件转化率/平均搜后阅读篇数/7日支付转化率/店铺新客ROI/停留时长），
//     按设计文档 4.3 假设实现，发版前待业务核对，核对后改对应 expr 分母即可。

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// cfMetric 乘风明细表一列的聚合定义：Key=英文列名(也是JSON key)/Label=中文表头/Expr=聚合SQL/Fmt=前端格式
type cfMetric struct {
	Key, Label, Expr, Fmt string
}

var cfMetrics = []cfMetric{
	{"cost", "消费", "IFNULL(SUM(cost),0)", "money"},
	{"impression", "展现量", "IFNULL(SUM(impression),0)", "int"},
	{"click_count", "点击量", "IFNULL(SUM(click_count),0)", "int"},
	{"click_rate", "点击率(%)", "IFNULL(SUM(click_count)/NULLIF(SUM(impression),0)*100,0)", "rate"},
	{"avg_click_cost", "平均点击成本", "IFNULL(SUM(cost)/NULLIF(SUM(click_count),0),0)", "cost"},
	{"avg_cpm", "平均千次展示费用", "IFNULL(SUM(cost)/NULLIF(SUM(impression),0)*1000,0)", "cost"},
	{"newcust_cost", "新客消耗", "IFNULL(SUM(newcust_cost),0)", "money"},
	{"newcust_impression", "新客曝光", "IFNULL(SUM(newcust_impression),0)", "int"},
	{"newcust_click", "新客点击", "IFNULL(SUM(newcust_click),0)", "int"},
	{"newcust_click_rate", "新客点击率(%)", "IFNULL(SUM(newcust_click)/NULLIF(SUM(newcust_impression),0)*100,0)", "rate"},
	{"newcust_avg_click_cost", "新客平均点击成本", "IFNULL(SUM(newcust_cost)/NULLIF(SUM(newcust_click),0),0)", "cost"},
	{"newcust_avg_cpm", "新客平均千次展示费用", "IFNULL(SUM(newcust_cost)/NULLIF(SUM(newcust_impression),0)*1000,0)", "cost"},
	{"platform_subsidy_amount", "平台额外补贴金额", "IFNULL(SUM(platform_subsidy_amount),0)", "money"},
	{"subsidy_driven_gmv", "补贴撬动全店交易额", "IFNULL(SUM(subsidy_driven_gmv),0)", "money"},
	{"like_count", "点赞", "IFNULL(SUM(like_count),0)", "int"},
	{"comment_count", "评论", "IFNULL(SUM(comment_count),0)", "int"},
	{"collect_count", "收藏", "IFNULL(SUM(collect_count),0)", "int"},
	{"follow_count", "关注", "IFNULL(SUM(follow_count),0)", "int"},
	{"share_count", "分享", "IFNULL(SUM(share_count),0)", "int"},
	{"interaction_count", "互动量", "IFNULL(SUM(interaction_count),0)", "int"},
	{"avg_interaction_cost", "平均互动成本", "IFNULL(SUM(cost)/NULLIF(SUM(interaction_count),0),0)", "cost"},
	{"action_btn_click_count", "行动按钮点击量", "IFNULL(SUM(action_btn_click_count),0)", "int"},
	{"action_btn_click_rate", "行动按钮点击率(%)", "IFNULL(SUM(action_btn_click_count)/NULLIF(SUM(impression),0)*100,0)", "rate"},
	{"screenshot_count", "截图", "IFNULL(SUM(screenshot_count),0)", "int"},
	{"save_image_count", "保存图片", "IFNULL(SUM(save_image_count),0)", "int"},
	{"search_widget_click_count", "搜索组件点击量", "IFNULL(SUM(search_widget_click_count),0)", "int"},
	{"search_widget_click_conv_rate", "搜索组件点击转化率(%)", "IFNULL(SUM(search_widget_click_count)/NULLIF(SUM(impression),0)*100,0)", "rate"},
	{"avg_post_search_read_notes", "平均搜索后阅读笔记篇数", "IFNULL(SUM(post_search_read_count)/NULLIF(SUM(search_widget_click_count),0),0)", "num2"},
	{"post_search_read_count", "搜后阅读量", "IFNULL(SUM(post_search_read_count),0)", "int"},
	{"reservation_count", "预约人次", "IFNULL(SUM(reservation_count),0)", "int"},
	{"live_reservation_count", "直播预约人次", "IFNULL(SUM(live_reservation_count),0)", "int"},
	{"live_reservation_cost", "直播预约成本", "IFNULL(SUM(cost)/NULLIF(SUM(live_reservation_count),0),0)", "cost"},
	{"reserve_reach_live_impression", "预约触达人群开播曝光人次", "IFNULL(SUM(reserve_reach_live_impression),0)", "int"},
	{"reserve_reach_live_view", "预约触达人群开播观看人次", "IFNULL(SUM(reserve_reach_live_view),0)", "int"},
	{"reserve_reach_live_order", "预约触达人群直播间下单人次", "IFNULL(SUM(reserve_reach_live_order),0)", "int"},
	{"live_view_count", "直播间观看次数", "IFNULL(SUM(live_view_count),0)", "int"},
	{"live_view_cost", "直播间观看成本", "IFNULL(SUM(cost)/NULLIF(SUM(live_view_count),0),0)", "cost"},
	{"live_avg_stay_duration", "直播间平均停留时长", "IFNULL(SUM(live_avg_stay_duration*live_view_count)/NULLIF(SUM(live_view_count),0),0)", "num2"},
	{"live_new_fans", "直播间新增粉丝数量", "IFNULL(SUM(live_new_fans),0)", "int"},
	{"live_5s_view_count", "直播间5s观看次数", "IFNULL(SUM(live_5s_view_count),0)", "int"},
	{"live_5s_view_cost", "直播间5s观看成本", "IFNULL(SUM(cost)/NULLIF(SUM(live_5s_view_count),0),0)", "cost"},
	{"live_comment_count", "直播间评论次数", "IFNULL(SUM(live_comment_count),0)", "int"},
	{"live_30s_view_count", "直播间30s观看次数", "IFNULL(SUM(live_30s_view_count),0)", "int"},
	{"live_30s_view_cost", "直播间30s观看成本", "IFNULL(SUM(cost)/NULLIF(SUM(live_30s_view_count),0),0)", "cost"},
	{"live_direct_goods_visitor", "直播间直接商品访客量", "IFNULL(SUM(live_direct_goods_visitor),0)", "int"},
	{"live_direct_goods_visitor_cost", "直播间直接商品访客量成本", "IFNULL(SUM(cost)/NULLIF(SUM(live_direct_goods_visitor),0),0)", "cost"},
	{"live_direct_goods_addcart", "直播间直接商品加购量", "IFNULL(SUM(live_direct_goods_addcart),0)", "int"},
	{"live_direct_goods_addcart_cost", "直播间直接商品加购量成本", "IFNULL(SUM(cost)/NULLIF(SUM(live_direct_goods_addcart),0),0)", "cost"},
	{"goods_visitor_7d", "7日总商品访客量", "IFNULL(SUM(goods_visitor_7d),0)", "int"},
	{"goods_visitor_7d_cost", "7日总商品访客成本", "IFNULL(SUM(cost)/NULLIF(SUM(goods_visitor_7d),0),0)", "cost"},
	{"goods_addcart_7d", "7日总商品加购量", "IFNULL(SUM(goods_addcart_7d),0)", "int"},
	{"goods_addcart_7d_cost", "7日总商品加购成本", "IFNULL(SUM(cost)/NULLIF(SUM(goods_addcart_7d),0),0)", "cost"},
	{"video_play_count", "视频播放量", "IFNULL(SUM(video_play_count),0)", "int"},
	{"video_5s_play_count", "视频5s播放量", "IFNULL(SUM(video_5s_play_count),0)", "int"},
	{"video_5s_finish_rate", "视频5s完播率(%)", "IFNULL(SUM(video_5s_play_count)/NULLIF(SUM(video_play_count),0)*100,0)", "rate"},
	{"order_7d_count", "7日总下单订单量", "IFNULL(SUM(order_7d_count),0)", "int"},
	{"order_7d_cost", "7日总下单订单成本", "IFNULL(SUM(cost)/NULLIF(SUM(order_7d_count),0),0)", "cost"},
	{"order_7d_amount", "7日总下单金额", "IFNULL(SUM(order_7d_amount),0)", "money"},
	{"order_7d_roi", "7日总下单ROI", "IFNULL(SUM(order_7d_amount)/NULLIF(SUM(cost),0),0)", "roi"},
	{"pay_7d_order_count", "7日总支付订单量", "IFNULL(SUM(pay_7d_order_count),0)", "int"},
	{"pay_7d_order_cost", "7日总支付订单成本", "IFNULL(SUM(cost)/NULLIF(SUM(pay_7d_order_count),0),0)", "cost"},
	{"pay_7d_amount", "7日总支付金额", "IFNULL(SUM(pay_7d_amount),0)", "money"},
	{"pay_7d_roi", "7日总支付ROI", "IFNULL(SUM(pay_7d_amount)/NULLIF(SUM(cost),0),0)", "roi"},
	{"pay_7d_conv_rate", "7日支付转化率(%)", "IFNULL(SUM(pay_7d_order_count)/NULLIF(SUM(click_count),0)*100,0)", "rate"},
	{"order_7d_count_conv", "7日总下单订单量(转化时间)", "IFNULL(SUM(order_7d_count_conv),0)", "int"},
	{"order_7d_cost_conv", "7日总下单订单成本(转化时间)", "IFNULL(SUM(cost)/NULLIF(SUM(order_7d_count_conv),0),0)", "cost"},
	{"order_7d_amount_conv", "7日总下单金额(转化时间)", "IFNULL(SUM(order_7d_amount_conv),0)", "money"},
	{"order_7d_roi_conv", "7日总下单ROI(转化时间)", "IFNULL(SUM(order_7d_amount_conv)/NULLIF(SUM(cost),0),0)", "roi"},
	{"pay_7d_order_count_conv", "7日总支付订单量(转化时间)", "IFNULL(SUM(pay_7d_order_count_conv),0)", "int"},
	{"pay_7d_order_cost_conv", "7日总支付订单成本(转化时间)", "IFNULL(SUM(cost)/NULLIF(SUM(pay_7d_order_count_conv),0),0)", "cost"},
	{"pay_7d_amount_conv", "7日总支付金额(转化时间)", "IFNULL(SUM(pay_7d_amount_conv),0)", "money"},
	{"pay_7d_roi_conv", "7日总支付ROI(转化时间)", "IFNULL(SUM(pay_7d_amount_conv)/NULLIF(SUM(cost),0),0)", "roi"},
	{"direct_pay_order_count", "直接支付订单量", "IFNULL(SUM(direct_pay_order_count),0)", "int"},
	{"direct_pay_order_cost", "直接支付订单成本", "IFNULL(SUM(cost)/NULLIF(SUM(direct_pay_order_count),0),0)", "cost"},
	{"direct_pay_gmv", "直接支付订单gmv", "IFNULL(SUM(direct_pay_gmv),0)", "money"},
	{"direct_pay_roi", "直接支付ROI", "IFNULL(SUM(direct_pay_gmv)/NULLIF(SUM(cost),0),0)", "roi"},
	{"goods_direct_order_count", "商品直接下单订单量", "IFNULL(SUM(goods_direct_order_count),0)", "int"},
	{"goods_direct_order_cost", "商品直接下单订单成本", "IFNULL(SUM(cost)/NULLIF(SUM(goods_direct_order_count),0),0)", "cost"},
	{"goods_direct_order_amount", "商品直接下单金额", "IFNULL(SUM(goods_direct_order_amount),0)", "money"},
	{"goods_direct_order_roi", "商品直接下单ROI", "IFNULL(SUM(goods_direct_order_amount)/NULLIF(SUM(cost),0),0)", "roi"},
	{"goods_1d_pay_order_count", "商品1日支付订单量", "IFNULL(SUM(goods_1d_pay_order_count),0)", "int"},
	{"goods_1d_pay_order_cost", "商品1日支付订单成本", "IFNULL(SUM(cost)/NULLIF(SUM(goods_1d_pay_order_count),0),0)", "cost"},
	{"goods_1d_pay_amount", "商品1日支付金额", "IFNULL(SUM(goods_1d_pay_amount),0)", "money"},
	{"goods_1d_pay_roi", "商品1日支付ROI", "IFNULL(SUM(goods_1d_pay_amount)/NULLIF(SUM(cost),0),0)", "roi"},
	{"live_direct_order_count", "直播间直接下单订单量", "IFNULL(SUM(live_direct_order_count),0)", "int"},
	{"live_direct_order_cost", "直播间直接下单订单成本", "IFNULL(SUM(cost)/NULLIF(SUM(live_direct_order_count),0),0)", "cost"},
	{"live_direct_order_amount", "直播间直接下单金额", "IFNULL(SUM(live_direct_order_amount),0)", "money"},
	{"live_direct_order_roi", "直播间直接下单ROI", "IFNULL(SUM(live_direct_order_amount)/NULLIF(SUM(cost),0),0)", "roi"},
	{"live_direct_pay_order_count", "直播间直接支付订单量", "IFNULL(SUM(live_direct_pay_order_count),0)", "int"},
	{"live_direct_pay_order_cost", "直播间直接支付订单成本", "IFNULL(SUM(cost)/NULLIF(SUM(live_direct_pay_order_count),0),0)", "cost"},
	{"live_direct_pay_amount", "直播间直接支付金额", "IFNULL(SUM(live_direct_pay_amount),0)", "money"},
	{"live_direct_pay_roi", "直播间直接支付ROI", "IFNULL(SUM(live_direct_pay_amount)/NULLIF(SUM(cost),0),0)", "roi"},
	{"reserve_reach_live_pay_amount", "预约触达人群直播间直接支付金额", "IFNULL(SUM(reserve_reach_live_pay_amount),0)", "money"},
	{"reserve_reach_live_pay_order_count", "预约触达人群直播间直接支付订单量", "IFNULL(SUM(reserve_reach_live_pay_order_count),0)", "int"},
	{"reserve_reach_pay_7d_amount", "预约触达人群7日总支付金额", "IFNULL(SUM(reserve_reach_pay_7d_amount),0)", "money"},
	{"reserve_reach_pay_7d_order_count", "预约触达人群7日支付订单量", "IFNULL(SUM(reserve_reach_pay_7d_order_count),0)", "int"},
	{"reserve_reach_pay_15d_amount", "预约触达人群15日总支付金额", "IFNULL(SUM(reserve_reach_pay_15d_amount),0)", "money"},
	{"reserve_reach_pay_15d_order_count", "预约触达人群15日总支付订单量", "IFNULL(SUM(reserve_reach_pay_15d_order_count),0)", "int"},
	{"shop_newcust_goods_visit", "店铺新客商品访问量", "IFNULL(SUM(shop_newcust_goods_visit),0)", "int"},
	{"shop_newcust_pay_order_count", "店铺新客支付订单量", "IFNULL(SUM(shop_newcust_pay_order_count),0)", "int"},
	{"shop_newcust_pay_amount", "店铺新客支付金额", "IFNULL(SUM(shop_newcust_pay_amount),0)", "money"},
	{"shop_newcust_pay_roi", "店铺新客支付ROI", "IFNULL(SUM(shop_newcust_pay_amount)/NULLIF(SUM(cost),0),0)", "roi"},
	{"shop_newcust_pay_people", "店铺新客支付人数", "IFNULL(SUM(shop_newcust_pay_people),0)", "int"},
	{"shop_newcust_pay_7d_order_count", "店铺新客7日支付订单量", "IFNULL(SUM(shop_newcust_pay_7d_order_count),0)", "int"},
	{"shop_newcust_pay_7d_amount", "店铺新客7日支付金额", "IFNULL(SUM(shop_newcust_pay_7d_amount),0)", "money"},
	{"shop_newcust_order_roi", "店铺新客下单ROI", "IFNULL(SUM(shop_newcust_pay_7d_amount)/NULLIF(SUM(cost),0),0)", "roi"},
	{"shop_newcust_order_people", "店铺新客下单人数", "IFNULL(SUM(shop_newcust_order_people),0)", "int"},
}

// cfGroups 指标分组（仿小红书千帆后台「自定义指标」左侧分组）。每组列出其下指标 Key，
// 顺序即组内展示顺序；GetCfFilters 据此给每列附 group。未归类的 Key 落「其他指标」兜底。
var cfGroups = []struct {
	Name string
	Keys []string
}{
	{"基础·通用", []string{"cost", "impression", "click_count", "click_rate", "avg_click_cost", "avg_cpm"}},
	{"基础·新客", []string{"newcust_cost", "newcust_impression", "newcust_click", "newcust_click_rate", "newcust_avg_click_cost", "newcust_avg_cpm"}},
	{"优惠券与补贴", []string{"platform_subsidy_amount", "subsidy_driven_gmv"}},
	{"互动效果", []string{"like_count", "comment_count", "collect_count", "follow_count", "share_count", "interaction_count", "avg_interaction_cost", "action_btn_click_count", "action_btn_click_rate", "screenshot_count", "save_image_count"}},
	{"搜索组件", []string{"search_widget_click_count", "search_widget_click_conv_rate", "avg_post_search_read_notes", "post_search_read_count"}},
	{"预约与直播观看", []string{"reservation_count", "live_reservation_count", "live_reservation_cost", "reserve_reach_live_impression", "reserve_reach_live_view", "reserve_reach_live_order", "live_view_count", "live_view_cost", "live_avg_stay_duration", "live_new_fans", "live_5s_view_count", "live_5s_view_cost", "live_comment_count", "live_30s_view_count", "live_30s_view_cost", "live_direct_goods_visitor", "live_direct_goods_visitor_cost", "live_direct_goods_addcart", "live_direct_goods_addcart_cost"}},
	{"视频播放", []string{"video_play_count", "video_5s_play_count", "video_5s_finish_rate"}},
	{"商品转化(下单/支付)", []string{"goods_visitor_7d", "goods_visitor_7d_cost", "goods_addcart_7d", "goods_addcart_7d_cost", "order_7d_count", "order_7d_cost", "order_7d_amount", "order_7d_roi", "pay_7d_order_count", "pay_7d_order_cost", "pay_7d_amount", "pay_7d_roi", "pay_7d_conv_rate", "order_7d_count_conv", "order_7d_cost_conv", "order_7d_amount_conv", "order_7d_roi_conv", "pay_7d_order_count_conv", "pay_7d_order_cost_conv", "pay_7d_amount_conv", "pay_7d_roi_conv", "direct_pay_order_count", "direct_pay_order_cost", "direct_pay_gmv", "direct_pay_roi", "goods_direct_order_count", "goods_direct_order_cost", "goods_direct_order_amount", "goods_direct_order_roi", "goods_1d_pay_order_count", "goods_1d_pay_order_cost", "goods_1d_pay_amount", "goods_1d_pay_roi"}},
	{"直播间转化", []string{"live_direct_order_count", "live_direct_order_cost", "live_direct_order_amount", "live_direct_order_roi", "live_direct_pay_order_count", "live_direct_pay_order_cost", "live_direct_pay_amount", "live_direct_pay_roi"}},
	{"预约触达转化", []string{"reserve_reach_live_pay_amount", "reserve_reach_live_pay_order_count", "reserve_reach_pay_7d_amount", "reserve_reach_pay_7d_order_count", "reserve_reach_pay_15d_amount", "reserve_reach_pay_15d_order_count"}},
	{"店铺新客", []string{"shop_newcust_goods_visit", "shop_newcust_pay_order_count", "shop_newcust_pay_amount", "shop_newcust_pay_roi", "shop_newcust_pay_people", "shop_newcust_pay_7d_order_count", "shop_newcust_pay_7d_amount", "shop_newcust_order_roi", "shop_newcust_order_people"}},
}

// cfGroupOf 反查某指标 Key 属于哪个分组，未归类返回「其他指标」
func cfGroupOf(key string) string {
	for _, g := range cfGroups {
		for _, k := range g.Keys {
			if k == key {
				return g.Name
			}
		}
	}
	return "其他指标"
}

// cfWhere 拼 stat_date 范围 + shops + note_id 模糊 条件
func cfWhere(r *http.Request) (string, []interface{}) {
	where := ""
	var args []interface{}
	start := strings.TrimSpace(r.URL.Query().Get("start"))
	end := strings.TrimSpace(r.URL.Query().Get("end"))
	if start != "" && end != "" {
		where += " AND stat_date BETWEEN ? AND ?"
		args = append(args, start, end)
	}
	shops := strings.TrimSpace(r.URL.Query().Get("shops"))
	if shops != "" {
		var ph []string
		for _, p := range strings.Split(shops, ",") {
			if p = strings.TrimSpace(p); p != "" {
				ph = append(ph, "?")
				args = append(args, p)
			}
		}
		if len(ph) > 0 {
			where += " AND shop_name IN (" + strings.Join(ph, ",") + ")"
		}
	}
	if idLike := strings.TrimSpace(r.URL.Query().Get("note_id_like")); idLike != "" {
		where += " AND note_id LIKE ?"
		args = append(args, "%"+idLike+"%")
	}
	return where, args
}

// GetCfFilters GET /api/xiaohongshu/chengfeng/filters
func (h *DashboardHandler) GetCfFilters(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDeptAccess(r, "social")) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	var latest string
	if err := h.DB.QueryRowContext(ctx, `SELECT IFNULL(DATE_FORMAT(MAX(stat_date),'%Y-%m-%d'),'') FROM op_xhs_chengfeng_daily`).Scan(&latest); err != nil {
		writeDatabaseError(w, err)
		return
	}
	shops := []string{}
	rows, err := h.DB.QueryContext(ctx, `SELECT DISTINCT shop_name FROM op_xhs_chengfeng_daily ORDER BY shop_name`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var s string
			if rows.Scan(&s) == nil && strings.TrimSpace(s) != "" {
				shops = append(shops, s)
			}
		}
	}
	// columns: 给前端的列元信息(顺序+标签+格式+分组), 前端按此渲染明细表表头 & 自定义指标弹窗分组
	cols := make([]map[string]string, 0, len(cfMetrics))
	for _, m := range cfMetrics {
		cols = append(cols, map[string]string{"key": m.Key, "label": m.Label, "fmt": m.Fmt, "group": cfGroupOf(m.Key)})
	}
	writeJSON(w, map[string]interface{}{"latestDate": latest, "shops": shops, "columns": cols})
}

// GetCfList GET /api/xiaohongshu/chengfeng/list?start&end&shops&note_id_like
func (h *DashboardHandler) GetCfList(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDeptAccess(r, "social")) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	where, args := cfWhere(r)

	// KPI: 总消费/总展现/总点击/7日支付GMV(全口径 SUM); 综合ROI=Σ7日支付金额/Σ消费(加权)
	type cfKPI struct {
		Cost   float64 `json:"cost"`
		Imp    float64 `json:"impression"`
		Click  float64 `json:"click"`
		PayGMV float64 `json:"payGmv"`
		ROI    float64 `json:"roi"`
	}
	var k cfKPI
	if err := h.DB.QueryRowContext(ctx, `SELECT IFNULL(SUM(cost),0), IFNULL(SUM(impression),0),
		IFNULL(SUM(click_count),0), IFNULL(SUM(pay_7d_amount),0)
		FROM op_xhs_chengfeng_daily WHERE 1=1`+where, args...).
		Scan(&k.Cost, &k.Imp, &k.Click, &k.PayGMV); err != nil {
		writeDatabaseError(w, err)
		return
	}
	if k.Cost > 0 {
		k.ROI = k.PayGMV / k.Cost
	}

	// 明细: 按 note_id 聚合区间, 107 指标列由 cfMetrics 拼 SELECT, ORDER BY Σ消费 倒序 TOP 200
	exprs := make([]string, len(cfMetrics))
	for i, m := range cfMetrics {
		exprs[i] = m.Expr
	}
	sql := `SELECT note_id, ANY_VALUE(note_title),
		ANY_VALUE(CASE WHEN note_url LIKE 'http%' THEN note_url ELSE '' END), ` +
		strings.Join(exprs, ", ") +
		` FROM op_xhs_chengfeng_daily WHERE 1=1` + where +
		` GROUP BY note_id ORDER BY SUM(cost) DESC, SUM(impression) DESC LIMIT 50`

	rows, ok := queryRowsOrWriteError(w, r, h.DB, sql, args...)
	if !ok {
		return
	}
	defer rows.Close()
	detail := []map[string]interface{}{}
	for rows.Next() {
		var noteID, title, url string
		vals := make([]float64, len(cfMetrics))
		scanArgs := make([]interface{}, 0, 3+len(cfMetrics))
		scanArgs = append(scanArgs, &noteID, &title, &url)
		for i := range vals {
			scanArgs = append(scanArgs, &vals[i])
		}
		if writeDatabaseError(w, rows.Scan(scanArgs...)) {
			return
		}
		row := map[string]interface{}{"noteId": noteID, "title": title, "url": url}
		for i, m := range cfMetrics {
			row[m.Key] = vals[i]
		}
		detail = append(detail, row)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}

	writeJSON(w, map[string]interface{}{
		"kpi": k, "detail": detail,
		"dateRange": map[string]string{
			"start": strings.TrimSpace(r.URL.Query().Get("start")),
			"end":   strings.TrimSpace(r.URL.Query().Get("end")),
		},
	})
}

// GetCfNoteTrend GET /api/xiaohongshu/chengfeng/note-trend?note_id&start&end
// 单条笔记按 stat_date 每天走势(明细行下钻): 消费 vs 7日支付金额
func (h *DashboardHandler) GetCfNoteTrend(w http.ResponseWriter, r *http.Request) {
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
		Date string  `json:"date"`
		Cost float64 `json:"cost"`
		GMV  float64 `json:"gmv"`
	}
	trend := []tPoint{}
	rows, ok := queryRowsOrWriteError(w, r, h.DB, `SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'),
		IFNULL(SUM(cost),0), IFNULL(SUM(pay_7d_amount),0)
		FROM op_xhs_chengfeng_daily WHERE note_id=?`+cond+` GROUP BY stat_date ORDER BY stat_date`, args...)
	if !ok {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var p tPoint
		if writeDatabaseError(w, rows.Scan(&p.Date, &p.Cost, &p.GMV)) {
			return
		}
		trend = append(trend, p)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}
	writeJSON(w, map[string]interface{}{"trend": trend})
}
