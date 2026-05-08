package handler

import (
	"database/sql"
	"math"
	"net/http"
	"sort"
	"strings"
)
type customerMetricRecord struct {
	Platform         string
	Date             string
	ShopName         string
	ConsultUsers     float64
	InquiryUsers     float64
	PayUsers         float64
	SalesAmount      float64
	FirstRespSeconds float64
	ResponseSeconds  float64
	SatisfactionRate float64
	ConvRate         float64
}

type customerMetricAgg struct {
	RecordCount       int
	ConsultUsers      float64
	InquiryUsers      float64
	PayUsers          float64
	SalesAmount       float64
	FirstRespSeconds  float64
	FirstRespCount    int
	ResponseSeconds   float64
	ResponseCount     int
	SatisfactionRate  float64
	SatisfactionCount int
	ConvRate          float64
	ConvCount         int
}

type customerPlatformStat struct {
	Platform            string  `json:"platform"`
	RecordCount         int     `json:"recordCount"`
	ShopCount           int     `json:"shopCount"`
	ConsultUsers        float64 `json:"consultUsers"`
	InquiryUsers        float64 `json:"inquiryUsers"`
	PayUsers            float64 `json:"payUsers"`
	SalesAmount         float64 `json:"salesAmount"`
	AvgFirstRespSeconds float64 `json:"avgFirstRespSeconds"`
	AvgResponseSeconds  float64 `json:"avgResponseSeconds"`
	AvgSatisfactionRate float64 `json:"avgSatisfactionRate"`
	AvgConvRate         float64 `json:"avgConvRate"`
}

type customerTrendPoint struct {
	Date                string  `json:"date"`
	ConsultUsers        float64 `json:"consultUsers"`
	InquiryUsers        float64 `json:"inquiryUsers"`
	PayUsers            float64 `json:"payUsers"`
	SalesAmount         float64 `json:"salesAmount"`
	AvgFirstRespSeconds float64 `json:"avgFirstRespSeconds"`
	AvgResponseSeconds  float64 `json:"avgResponseSeconds"`
	AvgSatisfactionRate float64 `json:"avgSatisfactionRate"`
	AvgConvRate         float64 `json:"avgConvRate"`
}

type customerShopStat struct {
	Platform            string  `json:"platform"`
	ShopName            string  `json:"shopName"`
	RecordCount         int     `json:"recordCount"`
	ConsultUsers        float64 `json:"consultUsers"`
	InquiryUsers        float64 `json:"inquiryUsers"`
	PayUsers            float64 `json:"payUsers"`
	SalesAmount         float64 `json:"salesAmount"`
	AvgFirstRespSeconds float64 `json:"avgFirstRespSeconds"`
	AvgResponseSeconds  float64 `json:"avgResponseSeconds"`
	AvgSatisfactionRate float64 `json:"avgSatisfactionRate"`
	AvgConvRate         float64 `json:"avgConvRate"`
}

func nullFloat(v sql.NullFloat64) float64 {
	if v.Valid {
		return v.Float64
	}
	return 0
}

func normalizeRate(v float64) float64 {
	if v <= 0 {
		return 0
	}
	if v <= 1 {
		return v * 100
	}
	return v
}

func roundFloat(v float64, digits int) float64 {
	if digits < 0 {
		return v
	}
	pow := math.Pow(10, float64(digits))
	return math.Round(v*pow) / pow
}

func (a *customerMetricAgg) add(rec customerMetricRecord) {
	a.RecordCount++
	a.ConsultUsers += rec.ConsultUsers
	a.InquiryUsers += rec.InquiryUsers
	a.PayUsers += rec.PayUsers
	a.SalesAmount += rec.SalesAmount
	if rec.FirstRespSeconds > 0 {
		a.FirstRespSeconds += rec.FirstRespSeconds
		a.FirstRespCount++
	}
	if rec.ResponseSeconds > 0 {
		a.ResponseSeconds += rec.ResponseSeconds
		a.ResponseCount++
	}
	if rec.SatisfactionRate > 0 {
		a.SatisfactionRate += normalizeRate(rec.SatisfactionRate)
		a.SatisfactionCount++
	}
	if rec.ConvRate > 0 {
		a.ConvRate += normalizeRate(rec.ConvRate)
		a.ConvCount++
	}
}

func (a *customerMetricAgg) avgFirstRespSeconds() float64 {
	if a.FirstRespCount == 0 {
		return 0
	}
	return a.FirstRespSeconds / float64(a.FirstRespCount)
}

func (a *customerMetricAgg) avgResponseSeconds() float64 {
	if a.ResponseCount == 0 {
		return 0
	}
	return a.ResponseSeconds / float64(a.ResponseCount)
}

func (a *customerMetricAgg) avgSatisfactionRate() float64 {
	if a.SatisfactionCount == 0 {
		return 0
	}
	return a.SatisfactionRate / float64(a.SatisfactionCount)
}

func (a *customerMetricAgg) avgConvRate() float64 {
	if a.ConvCount == 0 {
		return 0
	}
	return a.ConvRate / float64(a.ConvCount)
}

// GetCustomerOverview 客服总览（跨平台统一指标）
func (h *DashboardHandler) GetCustomerOverview(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDomainAccess(r, "ops")) {
		return
	}

	start, end := getDateRange(r, h.DB)
	trendStart, trendEnd := getTrendDateRange(start, end)
	platformFilter := strings.TrimSpace(r.URL.Query().Get("platform"))
	args := []interface{}{
		trendStart, trendEnd, // tmall
		trendStart, trendEnd, // pdd
		trendStart, trendEnd, // jd
		trendStart, trendEnd, // douyin
		trendStart, trendEnd, // kuaishou
		trendStart, trendEnd, // xhs
		platformFilter,
		platformFilter,
	}

	rows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT platform, stat_date, shop_name, consult_users, inquiry_users, pay_users, sales_amount, first_response_seconds, response_seconds, satisfaction_rate, conv_rate
		FROM (
			SELECT
				'天猫' AS platform,
				DATE_FORMAT(tc.stat_date, '%Y-%m-%d') AS stat_date,
				tc.shop_name,
				IFNULL(tc.consult_users, 0) AS consult_users,
				IFNULL(ti.inquiry_users, 0) AS inquiry_users,
				IFNULL(ti.final_pay_users, 0) AS pay_users,
				IFNULL(ta.sales_amount, 0) AS sales_amount,
				IFNULL(tc.first_resp_sec, 0) AS first_response_seconds,
				IFNULL(tc.avg_response_sec, 0) AS response_seconds,
				IFNULL(te.total_satisfaction_rate, 0) AS satisfaction_rate,
				CASE
					WHEN IFNULL(ti.final_conv_rate, 0) <= 1 THEN IFNULL(ti.final_conv_rate, 0) * 100
					ELSE IFNULL(ti.final_conv_rate, 0)
				END AS conv_rate
			FROM op_tmall_service_consult tc
			LEFT JOIN op_tmall_service_inquiry ti ON ti.stat_date = tc.stat_date AND ti.shop_name = tc.shop_name
			LEFT JOIN op_tmall_service_avgprice ta ON ta.stat_date = tc.stat_date AND ta.shop_name = tc.shop_name
			LEFT JOIN op_tmall_service_evaluation te ON te.stat_date = tc.stat_date AND te.shop_name = tc.shop_name
			WHERE tc.stat_date BETWEEN ? AND ?

			UNION ALL

			SELECT
				'拼多多' AS platform,
				DATE_FORMAT(pbase.stat_date, '%Y-%m-%d') AS stat_date,
				pbase.shop_name,
				IFNULL(ps.inquiry_users, 0) AS consult_users,
				IFNULL(ps.inquiry_users, 0) AS inquiry_users,
				IFNULL(ps.final_group_users, 0) AS pay_users,
				IFNULL(ps.cs_sales_amount, 0) AS sales_amount,
				0 AS first_response_seconds,
				0 AS response_seconds,
				CASE
					WHEN IFNULL(px.three_min_reply_rate_823, 0) <= 1 THEN IFNULL(px.three_min_reply_rate_823, 0) * 100
					ELSE IFNULL(px.three_min_reply_rate_823, 0)
				END AS satisfaction_rate,
				IFNULL(ps.inquiry_conv_rate, 0) AS conv_rate
			FROM (
				SELECT stat_date, shop_name FROM op_pdd_cs_service_daily
				UNION
				SELECT stat_date, shop_name FROM op_pdd_cs_sales_daily
			) pbase
			LEFT JOIN op_pdd_cs_service_daily px ON px.stat_date = pbase.stat_date AND px.shop_name = pbase.shop_name
			LEFT JOIN op_pdd_cs_sales_daily ps ON ps.stat_date = pbase.stat_date AND ps.shop_name = pbase.shop_name
			WHERE pbase.stat_date BETWEEN ? AND ?

			UNION ALL

			SELECT
				'京东' AS platform,
				DATE_FORMAT(jbase.stat_date, '%Y-%m-%d') AS stat_date,
				jbase.shop_name,
				IFNULL(js.presale_receive_users, IFNULL(jw.message_consult_count, IFNULL(jw.consult_count, 0))) AS consult_users,
				IFNULL(js.presale_receive_users, IFNULL(jw.message_consult_count, IFNULL(jw.consult_count, 0))) AS inquiry_users,
				IFNULL(js.order_users, 0) AS pay_users,
				IFNULL(js.order_goods_amount, 0) AS sales_amount,
				IFNULL(jw.first_avg_resp_seconds, 0) AS first_response_seconds,
				IFNULL(jw.new_avg_resp_seconds, 0) AS response_seconds,
				IFNULL(jw.satisfaction_rate, 0) AS satisfaction_rate,
				IFNULL(js.consult_to_order_rate, 0) AS conv_rate
			FROM (
				SELECT stat_date, shop_name FROM op_jd_cs_workload_daily
				UNION
				SELECT stat_date, shop_name FROM op_jd_cs_sales_perf_daily
			) jbase
			LEFT JOIN op_jd_cs_workload_daily jw ON jw.stat_date = jbase.stat_date AND jw.shop_name = jbase.shop_name
			LEFT JOIN op_jd_cs_sales_perf_daily js ON js.stat_date = jbase.stat_date AND js.shop_name = jbase.shop_name
			WHERE jbase.stat_date BETWEEN ? AND ?

			UNION ALL

			SELECT
				'抖音' AS platform,
				DATE_FORMAT(stat_date, '%Y-%m-%d') AS stat_date,
				shop_name,
				IFNULL(inquiry_users, IFNULL(received_users, 0)) AS consult_users,
				IFNULL(inquiry_users, IFNULL(received_users, 0)) AS inquiry_users,
				IFNULL(pay_users, 0) AS pay_users,
				IFNULL(inquiry_pay_amount, 0) AS sales_amount,
				IFNULL(all_day_first_reply_seconds, 0) AS first_response_seconds,
				IFNULL(all_day_avg_reply_seconds, 0) AS response_seconds,
				IFNULL(all_day_satisfaction_rate, 0) AS satisfaction_rate,
				IFNULL(inquiry_conv_rate, 0) AS conv_rate
			FROM op_douyin_cs_feige_daily
			WHERE stat_date BETWEEN ? AND ?

			UNION ALL

			SELECT
				'快手' AS platform,
				DATE_FORMAT(stat_date, '%Y-%m-%d') AS stat_date,
				shop_name,
				IFNULL(consult_users, 0) AS consult_users,
				IFNULL(consult_users, 0) AS inquiry_users,
				IFNULL(pay_users, 0) AS pay_users,
				IFNULL(cs_sales_amount, 0) AS sales_amount,
				0 AS first_response_seconds,
				CASE
					WHEN IFNULL(reply_3min_rate_person, 0) <= 1 THEN IFNULL(reply_3min_rate_person, 0) * 100
					ELSE IFNULL(reply_3min_rate_person, 0)
				END AS response_seconds,
				CASE
					WHEN IFNULL(good_rate_person, 0) <= 1 THEN IFNULL(good_rate_person, 0) * 100
					ELSE IFNULL(good_rate_person, 0)
				END AS satisfaction_rate,
				CASE
					WHEN IFNULL(inquiry_conv_rate, 0) <= 1 THEN IFNULL(inquiry_conv_rate, 0) * 100
					ELSE IFNULL(inquiry_conv_rate, 0)
				END AS conv_rate
			FROM op_kuaishou_cs_assessment_daily
			WHERE stat_date BETWEEN ? AND ?

			UNION ALL

			SELECT
				'小红书' AS platform,
				DATE_FORMAT(stat_date, '%Y-%m-%d') AS stat_date,
				shop_name,
				IFNULL(case_count, 0) AS consult_users,
				IFNULL(case_count, 0) AS inquiry_users,
				IFNULL(inquiry_pay_pkg_count, 0) AS pay_users,
				IFNULL(inquiry_pay_gmv, 0) AS sales_amount,
				0 AS first_response_seconds,
				CASE
					WHEN IFNULL(reply_in_3min_case_ratio, 0) <= 1 THEN IFNULL(reply_in_3min_case_ratio, 0) * 100
					ELSE IFNULL(reply_in_3min_case_ratio, 0)
				END AS response_seconds,
				CASE
					WHEN IFNULL(positive_case_ratio, 0) <= 1 THEN IFNULL(positive_case_ratio, 0) * 100
					ELSE IFNULL(positive_case_ratio, 0)
				END AS satisfaction_rate,
				CASE
					WHEN IFNULL(inquiry_pay_case_ratio, 0) <= 1 THEN IFNULL(inquiry_pay_case_ratio, 0) * 100
					ELSE IFNULL(inquiry_pay_case_ratio, 0)
				END AS conv_rate
			FROM op_xhs_cs_analysis_daily
			WHERE stat_date BETWEEN ? AND ?
		) metrics
		WHERE (? = '' OR platform = ?)
		ORDER BY stat_date, platform, shop_name
	`, args...)
	if !ok {
		return
	}
	defer rows.Close()

	records := make([]customerMetricRecord, 0)
	for rows.Next() {
		var rec customerMetricRecord
		var consultUsers sql.NullFloat64
		var inquiryUsers sql.NullFloat64
		var payUsers sql.NullFloat64
		var salesAmount sql.NullFloat64
		var firstRespSeconds sql.NullFloat64
		var responseSeconds sql.NullFloat64
		var satisfactionRate sql.NullFloat64
		var convRate sql.NullFloat64

		if writeDatabaseError(w, rows.Scan(
			&rec.Platform,
			&rec.Date,
			&rec.ShopName,
			&consultUsers,
			&inquiryUsers,
			&payUsers,
			&salesAmount,
			&firstRespSeconds,
			&responseSeconds,
			&satisfactionRate,
			&convRate,
		)) {
			return
		}

		rec.ConsultUsers = nullFloat(consultUsers)
		rec.InquiryUsers = nullFloat(inquiryUsers)
		rec.PayUsers = nullFloat(payUsers)
		rec.SalesAmount = nullFloat(salesAmount)
		rec.FirstRespSeconds = nullFloat(firstRespSeconds)
		rec.ResponseSeconds = nullFloat(responseSeconds)
		rec.SatisfactionRate = nullFloat(satisfactionRate)
		rec.ConvRate = nullFloat(convRate)

		if platformFilter != "" && rec.Platform != platformFilter {
			continue
		}

		if rec.Platform == "天猫" &&
			rec.ConsultUsers <= 0 &&
			rec.SalesAmount <= 0 &&
			rec.FirstRespSeconds <= 0 &&
			rec.ResponseSeconds <= 0 &&
			rec.SatisfactionRate <= 0 &&
			rec.ConvRate <= 0 {
			continue
		}

		records = append(records, rec)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}

	totalAgg := customerMetricAgg{}
	platformAgg := map[string]*customerMetricAgg{}
	platformShopSets := map[string]map[string]struct{}{}
	trendAgg := map[string]*customerMetricAgg{}
	shopAgg := map[string]*customerMetricAgg{}
	shopMeta := map[string]customerShopStat{}
	allShops := map[string]struct{}{}
	latestDate := ""

	for _, rec := range records {
		dAgg, ok := trendAgg[rec.Date]
		if !ok {
			dAgg = &customerMetricAgg{}
			trendAgg[rec.Date] = dAgg
		}
		dAgg.add(rec)

		if rec.Date < start || rec.Date > end {
			continue
		}
		totalAgg.add(rec)
		if rec.Date > latestDate {
			latestDate = rec.Date
		}
		allShops[rec.Platform+"|"+rec.ShopName] = struct{}{}

		pAgg, ok := platformAgg[rec.Platform]
		if !ok {
			pAgg = &customerMetricAgg{}
			platformAgg[rec.Platform] = pAgg
		}
		pAgg.add(rec)
		if _, ok := platformShopSets[rec.Platform]; !ok {
			platformShopSets[rec.Platform] = map[string]struct{}{}
		}
		platformShopSets[rec.Platform][rec.ShopName] = struct{}{}

		shopKey := rec.Platform + "|" + rec.ShopName
		sAgg, ok := shopAgg[shopKey]
		if !ok {
			sAgg = &customerMetricAgg{}
			shopAgg[shopKey] = sAgg
			shopMeta[shopKey] = customerShopStat{
				Platform: rec.Platform,
				ShopName: rec.ShopName,
			}
		}
		sAgg.add(rec)
	}

	platformStats := make([]customerPlatformStat, 0, len(platformAgg))
	for platform, agg := range platformAgg {
		platformStats = append(platformStats, customerPlatformStat{
			Platform:            platform,
			RecordCount:         agg.RecordCount,
			ShopCount:           len(platformShopSets[platform]),
			ConsultUsers:        roundFloat(agg.ConsultUsers, 0),
			InquiryUsers:        roundFloat(agg.InquiryUsers, 0),
			PayUsers:            roundFloat(agg.PayUsers, 0),
			SalesAmount:         roundFloat(agg.SalesAmount, 2),
			AvgFirstRespSeconds: roundFloat(agg.avgFirstRespSeconds(), 1),
			AvgResponseSeconds:  roundFloat(agg.avgResponseSeconds(), 1),
			AvgSatisfactionRate: roundFloat(agg.avgSatisfactionRate(), 2),
			AvgConvRate:         roundFloat(agg.avgConvRate(), 2),
		})
	}
	sort.Slice(platformStats, func(i, j int) bool {
		return platformStats[i].SalesAmount > platformStats[j].SalesAmount
	})

	trendDates := make([]string, 0, len(trendAgg))
	for date := range trendAgg {
		trendDates = append(trendDates, date)
	}
	sort.Strings(trendDates)
	trend := make([]customerTrendPoint, 0, len(trendDates))
	for _, date := range trendDates {
		agg := trendAgg[date]
		trend = append(trend, customerTrendPoint{
			Date:                date,
			ConsultUsers:        roundFloat(agg.ConsultUsers, 0),
			InquiryUsers:        roundFloat(agg.InquiryUsers, 0),
			PayUsers:            roundFloat(agg.PayUsers, 0),
			SalesAmount:         roundFloat(agg.SalesAmount, 2),
			AvgFirstRespSeconds: roundFloat(agg.avgFirstRespSeconds(), 1),
			AvgResponseSeconds:  roundFloat(agg.avgResponseSeconds(), 1),
			AvgSatisfactionRate: roundFloat(agg.avgSatisfactionRate(), 2),
			AvgConvRate:         roundFloat(agg.avgConvRate(), 2),
		})
	}

	shopRanking := make([]customerShopStat, 0, len(shopAgg))
	for key, agg := range shopAgg {
		meta := shopMeta[key]
		shopRanking = append(shopRanking, customerShopStat{
			Platform:            meta.Platform,
			ShopName:            meta.ShopName,
			RecordCount:         agg.RecordCount,
			ConsultUsers:        roundFloat(agg.ConsultUsers, 0),
			InquiryUsers:        roundFloat(agg.InquiryUsers, 0),
			PayUsers:            roundFloat(agg.PayUsers, 0),
			SalesAmount:         roundFloat(agg.SalesAmount, 2),
			AvgFirstRespSeconds: roundFloat(agg.avgFirstRespSeconds(), 1),
			AvgResponseSeconds:  roundFloat(agg.avgResponseSeconds(), 1),
			AvgSatisfactionRate: roundFloat(agg.avgSatisfactionRate(), 2),
			AvgConvRate:         roundFloat(agg.avgConvRate(), 2),
		})
	}
	sort.Slice(shopRanking, func(i, j int) bool {
		return shopRanking[i].SalesAmount > shopRanking[j].SalesAmount
	})
	if len(shopRanking) > 30 {
		shopRanking = shopRanking[:30]
	}

	payUserConsultRatio := 0.0
	salesPerConsultUser := 0.0
	salesPerPayUser := 0.0
	if totalAgg.ConsultUsers > 0 {
		payUserConsultRatio = totalAgg.PayUsers / totalAgg.ConsultUsers * 100
		salesPerConsultUser = totalAgg.SalesAmount / totalAgg.ConsultUsers
	}
	if totalAgg.PayUsers > 0 {
		salesPerPayUser = totalAgg.SalesAmount / totalAgg.PayUsers
	}

	writeJSON(w, map[string]interface{}{
		"summary": map[string]interface{}{
			"platformCount":       len(platformStats),
			"shopCount":           len(allShops),
			"recordCount":         totalAgg.RecordCount,
			"consultUsers":        roundFloat(totalAgg.ConsultUsers, 0),
			"payUsers":            roundFloat(totalAgg.PayUsers, 0),
			"salesAmount":         roundFloat(totalAgg.SalesAmount, 2),
			"avgFirstRespSeconds": roundFloat(totalAgg.avgFirstRespSeconds(), 1),
			"avgResponseSeconds":  roundFloat(totalAgg.avgResponseSeconds(), 1),
			"avgSatisfactionRate": roundFloat(totalAgg.avgSatisfactionRate(), 2),
			"avgConversionRate":   roundFloat(totalAgg.avgConvRate(), 2),
			"payUserConsultRatio": roundFloat(payUserConsultRatio, 2),
			"salesPerConsultUser": roundFloat(salesPerConsultUser, 2),
			"salesPerPayUser":     roundFloat(salesPerPayUser, 2),
			"latestDate":          latestDate,
		},
		"platformStats": platformStats,
		"trend":         trend,
		"shopRanking":   shopRanking,
		"dateRange": map[string]string{
			"start": start,
			"end":   end,
		},
		"trendRange": map[string]string{
			"start": trendStart,
			"end":   trendEnd,
		},
	})
}


