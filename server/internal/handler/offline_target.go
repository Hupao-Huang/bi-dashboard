package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// GET /api/offline/targets?year=2026
func (h *DashboardHandler) GetOfflineTargets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	yearStr := r.URL.Query().Get("year")
	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 2020 || year > 2100 {
		year = time.Now().Year()
	}

	rows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT month, region, target
		FROM offline_region_target
		WHERE year = ?
		ORDER BY month, region`, year)
	if !ok {
		return
	}
	defer rows.Close()

	// 返回 map[month]map[region]target
	type TargetItem struct {
		Month  int     `json:"month"`
		Region string  `json:"region"`
		Target float64 `json:"target"`
	}
	items := []TargetItem{}
	for rows.Next() {
		var it TargetItem
		if writeDatabaseError(w, rows.Scan(&it.Month, &it.Region, &it.Target)) {
			return
		}
		items = append(items, it)
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}
	writeJSON(w, map[string]interface{}{"year": year, "items": items})
}

// POST /api/offline/targets  body: {"year":2026,"items":[{"month":4,"region":"华北大区","target":6084000},...]}
func (h *DashboardHandler) SaveOfflineTargets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}

	var req struct {
		Year  int `json:"year"`
		Items []struct {
			Month  int     `json:"month"`
			Region string  `json:"region"`
			Target float64 `json:"target"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "参数错误")
		return
	}
	if req.Year < 2020 || req.Year > 2100 {
		writeError(w, 400, "年份不合法")
		return
	}

	tx, err := h.DB.Begin()
	if writeDatabaseError(w, err) {
		return
	}
	defer tx.Rollback()

	for _, it := range req.Items {
		if it.Month < 1 || it.Month > 12 || it.Region == "" {
			continue
		}
		if it.Target < 0 {
			it.Target = 0
		}
		_, err := tx.Exec(`
			INSERT INTO offline_region_target (year, month, region, target)
			VALUES (?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE target = VALUES(target), updated_at = NOW()`,
			req.Year, it.Month, it.Region, it.Target)
		if writeDatabaseError(w, err) {
			return
		}
	}

	if writeDatabaseError(w, tx.Commit()) {
		return
	}
	writeJSON(w, map[string]string{"message": "保存成功"})
}

// GET /api/offline/targets/month?year=2026&month=4  — 给看板接口调用
func (h *DashboardHandler) GetOfflineTargetsByMonth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	month, _ := strconv.Atoi(r.URL.Query().Get("month"))
	if year == 0 {
		year = time.Now().Year()
	}
	if month == 0 {
		month = int(time.Now().Month())
	}

	rows, ok := queryRowsOrWriteError(w, h.DB, `
		SELECT region, target FROM offline_region_target
		WHERE year = ? AND month = ?`, year, month)
	if !ok {
		return
	}
	defer rows.Close()

	result := map[string]float64{}
	for rows.Next() {
		var region string
		var target float64
		if writeDatabaseError(w, rows.Scan(&region, &target)) {
			return
		}
		result[region] = target
	}
	if writeDatabaseError(w, rows.Err()) {
		return
	}
	writeJSON(w, result)
}
