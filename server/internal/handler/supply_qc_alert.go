package handler

import (
	"encoding/json"
	"net/http"
	"time"
)

// 质检预警: 来料/入库质检, 按"到货单"视角看每批到货的检验判定(合格/不合格)。
// 数据源 ys_inspection (YS 来料检验单, 每天同步)。不合格口径 inspect_result='2'; 已检 = inspect_result IN('1','2')。
// 一个到货单(vsourcecode) → 多张检验单(按物料行/批次拆)。来源类型: pu_arrivalorder采购到货 / po_osm_arrive_order委外到货。

func qcDateRange(r *http.Request) (string, string) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" {
		start = time.Now().AddDate(0, 0, -90).Format("2006-01-02")
	}
	if end == "" {
		end = time.Now().Format("2006-01-02")
	}
	return start + " 00:00:00", end + " 23:59:59"
}

func srcTypeName(t string) string {
	switch t {
	case "pu_arrivalorder":
		return "采购到货"
	case "po_osm_arrive_order":
		return "委外到货"
	default:
		return t
	}
}

// GetQCAlert GET /api/supply-chain/qc-alert?start=&end=
// 返回 KPI + 到货单清单(按到货单分组, 有不合格排前) + 月度趋势(近12月) + 供应商对比
func (h *DashboardHandler) GetQCAlert(w http.ResponseWriter, r *http.Request) {
	startTS, endTS := qcDateRange(r)

	// 1) KPI
	var total, bad, supplierCnt, badArrivals int
	_ = h.DB.QueryRow(`SELECT
			IFNULL(SUM(inspect_result IN('1','2')),0),
			IFNULL(SUM(inspect_result='2'),0),
			COUNT(DISTINCT CASE WHEN inspect_result='2' THEN pk_outsupplier_name END),
			COUNT(DISTINCT CASE WHEN inspect_result='2' THEN vsourcecode END)
		FROM ys_inspection WHERE inspect_date >= ? AND inspect_date <= ?`, startTS, endTS).
		Scan(&total, &bad, &supplierCnt, &badArrivals)
	badRate := 0.0
	if total > 0 {
		badRate = float64(bad) / float64(total) * 100
	}

	// 2) 到货单清单 (按到货单分组, 有不合格的排前)
	type ArrivalRow struct {
		Arrival       string  `json:"arrival"`       // 到货单号 vsourcecode
		PurchaseOrder string  `json:"purchaseOrder"` // 采购单号
		BillType      string  `json:"billType"`      // 来源类型(中文)
		Supplier      string  `json:"supplier"`
		InspectDate   string  `json:"inspectDate"`
		Total         int     `json:"total"`
		Pass          int     `json:"pass"`
		Bad           int     `json:"bad"`
		BadRate       float64 `json:"badRate"`
	}
	aRows, err := h.DB.Query(`SELECT IFNULL(vsourcecode,''), IFNULL(MAX(source_order_code),''),
			IFNULL(MAX(source_bill_type),''), IFNULL(MAX(pk_outsupplier_name),''),
			IFNULL(MAX(DATE_FORMAT(inspect_date,'%Y-%m-%d')),''),
			COUNT(*), SUM(inspect_result='1'), SUM(inspect_result='2')
		FROM ys_inspection
		WHERE inspect_result IN('1','2') AND inspect_date >= ? AND inspect_date <= ?
		  AND vsourcecode IS NOT NULL AND vsourcecode <> ''
		GROUP BY vsourcecode
		ORDER BY SUM(inspect_result='2') DESC, MAX(inspect_date) DESC
		LIMIT 1000`, startTS, endTS)
	if writeDatabaseError(w, err) {
		return
	}
	defer aRows.Close()
	arrivals := []ArrivalRow{}
	for aRows.Next() {
		var a ArrivalRow
		var bt string
		if writeDatabaseError(w, aRows.Scan(&a.Arrival, &a.PurchaseOrder, &bt, &a.Supplier,
			&a.InspectDate, &a.Total, &a.Pass, &a.Bad)) {
			return
		}
		a.BillType = srcTypeName(bt)
		if a.Total > 0 {
			a.BadRate = float64(a.Bad) / float64(a.Total) * 100
		}
		arrivals = append(arrivals, a)
	}

	// 3) 月度趋势 (近12月)
	type TrendRow struct {
		Month   string  `json:"month"`
		Total   int     `json:"total"`
		Bad     int     `json:"bad"`
		BadRate float64 `json:"badRate"`
	}
	trendStart := time.Now().AddDate(0, -11, 0).Format("2006-01") + "-01 00:00:00"
	tRows, err := h.DB.Query(`SELECT DATE_FORMAT(inspect_date,'%Y-%m'),
			SUM(inspect_result IN('1','2')), SUM(inspect_result='2')
		FROM ys_inspection WHERE inspect_result IN('1','2') AND inspect_date >= ?
		GROUP BY 1 ORDER BY 1`, trendStart)
	if writeDatabaseError(w, err) {
		return
	}
	defer tRows.Close()
	trend := []TrendRow{}
	for tRows.Next() {
		var t TrendRow
		if writeDatabaseError(w, tRows.Scan(&t.Month, &t.Total, &t.Bad)) {
			return
		}
		if t.Total > 0 {
			t.BadRate = float64(t.Bad) / float64(t.Total) * 100
		}
		trend = append(trend, t)
	}

	// 4) 供应商对比 (区间内, 有不合格的, 倒序)
	type SupplierRow struct {
		Supplier string  `json:"supplier"`
		Total    int     `json:"total"`
		Bad      int     `json:"bad"`
		BadRate  float64 `json:"badRate"`
	}
	sRows, err := h.DB.Query(`SELECT IFNULL(pk_outsupplier_name,'(未知)'),
			SUM(inspect_result IN('1','2')), SUM(inspect_result='2')
		FROM ys_inspection WHERE inspect_result IN('1','2') AND inspect_date >= ? AND inspect_date <= ?
		GROUP BY 1 HAVING SUM(inspect_result='2') > 0
		ORDER BY 2 DESC LIMIT 20`, startTS, endTS)
	if writeDatabaseError(w, err) {
		return
	}
	defer sRows.Close()
	bySupplier := []SupplierRow{}
	for sRows.Next() {
		var s SupplierRow
		if writeDatabaseError(w, sRows.Scan(&s.Supplier, &s.Total, &s.Bad)) {
			return
		}
		if s.Total > 0 {
			s.BadRate = float64(s.Bad) / float64(s.Total) * 100
		}
		bySupplier = append(bySupplier, s)
	}

	writeJSON(w, map[string]interface{}{
		"kpi": map[string]interface{}{
			"total":       total,
			"bad":         bad,
			"badRate":     badRate,
			"supplierCnt": supplierCnt,
			"badArrivals": badArrivals,
		},
		"arrivals":   arrivals,
		"trend":      trend,
		"bySupplier": bySupplier,
		"start":      r.URL.Query().Get("start"),
		"end":        r.URL.Query().Get("end"),
	})
}

// GetQCArrivalDetail GET /api/supply-chain/qc-alert/arrival?vsourcecode=XXX
// 返回某到货单下的全部检验单(不合格在前), 看这批货哪些判定有问题
func (h *DashboardHandler) GetQCArrivalDetail(w http.ResponseWriter, r *http.Request) {
	vs := r.URL.Query().Get("vsourcecode")
	if vs == "" {
		writeError(w, 400, "vsourcecode required")
		return
	}
	type InsRow struct {
		ID           string  `json:"id"`
		Code         string  `json:"code"`
		InspectDate  string  `json:"inspectDate"`
		MaterialCode string  `json:"materialCode"`
		MaterialName string  `json:"materialName"`
		Batch        string  `json:"batch"`
		Result       string  `json:"result"` // 1合格/2不合格
		InspectNum   float64 `json:"inspectNum"`
		BadNum       float64 `json:"badNum"`
		QRate        float64 `json:"qRate"`
		StockStatus  string  `json:"stockStatus"`
		HandleType   string  `json:"handleType"`
	}
	rows, err := h.DB.Query(`SELECT id, IFNULL(code,''), IFNULL(DATE_FORMAT(inspect_date,'%Y-%m-%d'),''),
			IFNULL(pk_material_code,''), IFNULL(pk_material_name,''), IFNULL(pk_batchcode,''),
			IFNULL(inspect_result,''), IFNULL(inspectnum,0), IFNULL(nqnum,0), IFNULL(qrate,0),
			IFNULL(pk_stockstatus_statusname,''), IFNULL(handle_type_name,'')
		FROM ys_inspection
		WHERE vsourcecode = ? AND inspect_result IN('1','2')
		ORDER BY inspect_result DESC, code`, vs)
	if writeDatabaseError(w, err) {
		return
	}
	defer rows.Close()
	list := []InsRow{}
	for rows.Next() {
		var x InsRow
		if writeDatabaseError(w, rows.Scan(&x.ID, &x.Code, &x.InspectDate, &x.MaterialCode, &x.MaterialName,
			&x.Batch, &x.Result, &x.InspectNum, &x.BadNum, &x.QRate, &x.StockStatus, &x.HandleType)) {
			return
		}
		list = append(list, x)
	}
	writeJSON(w, map[string]interface{}{"list": list})
}

// GetQCAlertDetail GET /api/supply-chain/qc-alert/detail?id=XXX
// 单张检验单全字段(raw_json), 给最深一层下钻
func (h *DashboardHandler) GetQCAlertDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, 400, "id required")
		return
	}
	var raw []byte
	if err := h.DB.QueryRow(`SELECT raw_json FROM ys_inspection WHERE id=?`, id).Scan(&raw); err != nil {
		writeDatabaseError(w, err)
		return
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		writeError(w, 500, "解析失败")
		return
	}
	writeJSON(w, map[string]interface{}{"detail": obj})
}
