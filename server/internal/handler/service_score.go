package handler

// 客服-服务分管理 查询接口。
// 数据来自 op_service_score_daily (import-service-score 导入, RPA 宽表拆平)。
// 三项分数含义按平台不同 (前端按平台贴标签):
//   京东自营: score1=平均响应时间 score2=应答率(0-1) score3=满意度(0-1)
//   拼多多:   score1=发货分 score1_extra=物流分 score2=商品分 score3=服务分 score3_extra=基础分
//   其他:     score1=物流分 score2=商品分 score3=服务分 (刻度: POP 10分制/抖音 100分制/天猫等 5分制)
// 目标(target)是"服务分目标", 只对默认三列平台有意义, 跟 score3 比较。

import (
	"net/http"
	"strings"
)

// GetServiceScores GET /api/customer/service-scores?date_from=&date_to=
// 返回区间内全部平台/店铺的每日分数 (前端按平台分 Tab + 店铺×日期透视)
func (h *DashboardHandler) GetServiceScores(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dateFrom := strings.TrimSpace(q.Get("date_from"))
	dateTo := strings.TrimSpace(q.Get("date_to"))

	var where []string
	var args []interface{}
	if dateFrom != "" {
		where = append(where, "stat_date >= ?")
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		where = append(where, "stat_date <= ?")
		args = append(args, dateTo)
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	rows, err := h.DB.Query(`SELECT DATE_FORMAT(stat_date,'%Y-%m-%d'), platform, shop_name,
		score1, score1_extra, score2, score3, score3_extra, target
		FROM op_service_score_daily `+whereSQL+`
		ORDER BY platform, shop_name, stat_date`, args...)
	if err != nil {
		writeServerError(w, 500, "查询服务分失败", err)
		return
	}
	defer rows.Close()

	type item struct {
		Date        string   `json:"date"`
		Platform    string   `json:"platform"`
		ShopName    string   `json:"shopName"`
		Score1      *float64 `json:"score1"`
		Score1Extra *float64 `json:"score1Extra"`
		Score2      *float64 `json:"score2"`
		Score3      *float64 `json:"score3"`
		Score3Extra *float64 `json:"score3Extra"`
		Target      *float64 `json:"target"`
	}
	list := make([]item, 0, 512)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Date, &it.Platform, &it.ShopName,
			&it.Score1, &it.Score1Extra, &it.Score2, &it.Score3, &it.Score3Extra, &it.Target); err != nil {
			writeServerError(w, 500, "读取服务分失败", err)
			return
		}
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		writeServerError(w, 500, "读取服务分失败", err)
		return
	}

	// 库里最新有数日期 (前端默认定位/达标统计用)
	var latestDate string
	_ = h.DB.QueryRow(`SELECT IFNULL(DATE_FORMAT(MAX(stat_date),'%Y-%m-%d'),'') FROM op_service_score_daily`).Scan(&latestDate)

	writeJSON(w, map[string]interface{}{
		"list":       list,
		"latestDate": latestDate,
	})
}
