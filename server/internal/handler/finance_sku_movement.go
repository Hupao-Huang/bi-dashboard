package handler

// 财务-SKU动销率看板 (跑哥/财务 2026-06-15 立项, 只读)
// 口径拍板:
//   - 仅统计打了产品定位等级的产成品 (goods.goods_field7 IN S/A/B/C/D), 其余 SKU(含分类=成品但未打等级) 全不计
//   - 有销售 = 当期销量>0, 含"调拨当销售"(京东/猫超/朴朴/小象/叮咚 5 渠道, 复用 specialchannel 注册表)
//   - 货品级(goods_no); 等级用当前主数据
//   - 指标1 整体动销率 = 有销售产成品数 / 产成品总数
//   - 指标2 品类动销率 = 各等级(S/A/B/C/D) 有销售数 / 该等级总数
//   - 指标3 单品动销率 = 该SKU当期有销售天数 / 当期天数 (完整月=当月总天数; 进行中本月=已过天数, 避免月初虚高滞销)

import (
	"bi-dashboard/internal/specialchannel"
	"net/http"
	"sort"
	"strings"
	"time"
)

type skuMovementRow struct {
	GoodsNo   string  `json:"goodsNo"`
	GoodsName string  `json:"goodsName"`
	Grade     string  `json:"grade"`
	DaysSold  int     `json:"daysSold"` // 当期有销售天数 (含调拨)
	Days      int     `json:"days"`     // 当期总天数 (动销率分母)
	Rate      float64 `json:"rate"`     // 单品动销率 0~1
}

type skuGradeAgg struct {
	Grade string  `json:"grade"`
	Total int     `json:"total"`
	Sold  int     `json:"sold"`
	Rate  float64 `json:"rate"`
}

// GetSKUMovement GET /api/finance/sku-movement?month=YYYY-MM  (默认上一个完整月)
func (h *DashboardHandler) GetSKUMovement(w http.ResponseWriter, r *http.Request) {
	// 1. 解析月份 (默认上一个完整月)
	monthStr := strings.TrimSpace(r.URL.Query().Get("month"))
	var monthStart time.Time
	if monthStr == "" {
		now := time.Now()
		monthStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -1, 0)
	} else {
		m, err := time.ParseInLocation("2006-01", monthStr, time.Local)
		if err != nil {
			writeError(w, 400, "month 格式应为 YYYY-MM")
			return
		}
		monthStart = m
	}
	monthEnd := monthStart.AddDate(0, 1, -1) // 当月最后一天
	now := time.Now()
	yesterday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -1)
	effEnd := monthEnd
	if yesterday.Before(effEnd) {
		effEnd = yesterday // 进行中本月 → 截到昨天
	}
	if effEnd.Before(monthStart) {
		// 未来月 / 本月还没过完一天 → 空
		writeJSON(w, map[string]interface{}{
			"month": monthStart.Format("2006-01"), "daysInPeriod": 0, "isPartial": true,
			"overall": map[string]interface{}{"totalSku": 0, "soldSku": 0, "rate": 0.0},
			"byGrade": []skuGradeAgg{}, "skus": []skuMovementRow{},
		})
		return
	}
	startStr := monthStart.Format("2006-01-02")
	endStr := effEnd.Format("2006-01-02")
	daysInPeriod := int(effEnd.Sub(monthStart).Hours()/24) + 1

	// 2. 产成品主数据 (打了等级的 = 产成品), 货品级去重取当前等级/名称
	gradedRows, ok := queryRowsOrWriteError(w, r, h.DB, `
		SELECT goods_no, IFNULL(MAX(goods_name),''), MAX(goods_field7)
		FROM goods
		WHERE goods_field7 IN ('S','A','B','C','D') AND goods_no IS NOT NULL AND goods_no <> ''
		GROUP BY goods_no`)
	if !ok {
		return
	}
	defer gradedRows.Close()
	skus := []skuMovementRow{}
	for gradedRows.Next() {
		var s skuMovementRow
		if writeDatabaseError(w, gradedRows.Scan(&s.GoodsNo, &s.GoodsName, &s.Grade)) {
			return
		}
		s.Days = daysInPeriod
		skus = append(skus, s)
	}
	if writeDatabaseError(w, gradedRows.Err()) {
		return
	}

	// 3. 当期各 goods_no 有销售天数 (常规销售 ∪ 调拨当销售 5 渠道)
	channelsIn := allotChannelsInClause(specialchannel.KeysByDept(""))
	daysRows, ok := queryRowsOrWriteError(w, r, h.DB, `
		SELECT goods_no, COUNT(DISTINCT stat_date) AS days_sold FROM (
			SELECT goods_no, stat_date FROM sales_goods_summary
				WHERE stat_date BETWEEN ? AND ? AND goods_qty > 0 AND goods_no IS NOT NULL
			UNION
			SELECT d.goods_no, o.stat_date
				FROM allocate_details d JOIN allocate_orders o ON o.allocate_no = d.allocate_no
				WHERE o.stat_date BETWEEN ? AND ? AND o.channel_key IN (`+channelsIn+`)
				  AND d.sku_count > 0 AND d.goods_no IS NOT NULL
		) u GROUP BY goods_no`,
		startStr, endStr, startStr, endStr)
	if !ok {
		return
	}
	defer daysRows.Close()
	daysSoldOf := map[string]int{}
	for daysRows.Next() {
		var gn string
		var d int
		if writeDatabaseError(w, daysRows.Scan(&gn, &d)) {
			return
		}
		daysSoldOf[gn] = d
	}
	if writeDatabaseError(w, daysRows.Err()) {
		return
	}

	// 4. 组装: 单品 + 整体 + 品类 (指标 1/2/3)
	gradeOrder := []string{"S", "A", "B", "C", "D"}
	gradeMap := map[string]*skuGradeAgg{}
	for _, g := range gradeOrder {
		gradeMap[g] = &skuGradeAgg{Grade: g}
	}
	totalSold := 0
	for i := range skus {
		d := daysSoldOf[skus[i].GoodsNo]
		skus[i].DaysSold = d
		if daysInPeriod > 0 {
			skus[i].Rate = float64(d) / float64(daysInPeriod)
		}
		if ga := gradeMap[skus[i].Grade]; ga != nil {
			ga.Total++
			if d > 0 {
				ga.Sold++
			}
		}
		if d > 0 {
			totalSold++
		}
	}
	for _, ga := range gradeMap {
		if ga.Total > 0 {
			ga.Rate = float64(ga.Sold) / float64(ga.Total)
		}
	}
	byGrade := make([]skuGradeAgg, 0, len(gradeOrder))
	for _, g := range gradeOrder {
		byGrade = append(byGrade, *gradeMap[g])
	}

	// 单品按动销率升序 (滞销在前, 财务最该看的)
	sort.SliceStable(skus, func(i, j int) bool {
		if skus[i].Rate != skus[j].Rate {
			return skus[i].Rate < skus[j].Rate
		}
		return skus[i].GoodsNo < skus[j].GoodsNo
	})

	totalSku := len(skus)
	overallRate := 0.0
	if totalSku > 0 {
		overallRate = float64(totalSold) / float64(totalSku)
	}
	writeJSON(w, map[string]interface{}{
		"month":        monthStart.Format("2006-01"),
		"daysInPeriod": daysInPeriod,
		"isPartial":    effEnd.Before(monthEnd), // true=进行中本月(分母用已过天数)
		"periodStart":  startStr,
		"periodEnd":    endStr,
		"overall":      map[string]interface{}{"totalSku": totalSku, "soldSku": totalSold, "rate": overallRate},
		"byGrade":      byGrade,
		"skus":         skus,
	})
}
