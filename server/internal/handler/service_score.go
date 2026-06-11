package handler

// 客服-服务分管理 查询/导出/修改接口。
// 数据来自 op_service_score_daily (import-service-score 导入, RPA 宽表拆平)。
// 三项分数含义按平台不同 (前端按平台贴标签):
//   京东自营: score1=平均响应时间 score2=应答率(0-1) score3=满意度(0-1)
//   拼多多:   score1=发货分 score1_extra=物流分 score2=商品分 score3=服务分 score3_extra=基础分
//   其他:     score1=物流分 score2=商品分 score3=服务分 (刻度: POP 10分制/抖音 100分制/天猫等 5分制)
// 修改 (跑哥 6/11: 跟评论数据一样要能改): 修正值存独立对照表 service_score_override,
// RPA 每天重导只更新原始表, 人工修正不被冲掉; 展示/导出用 COALESCE(修正, 原始)。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

func ensureServiceScoreOverride(h *DashboardHandler) {
	_, _ = h.DB.Exec(`CREATE TABLE IF NOT EXISTS service_score_override (
		id BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '主键',
		stat_date DATE NOT NULL COMMENT '业务日期',
		platform VARCHAR(32) NOT NULL COMMENT '平台',
		shop_name VARCHAR(128) NOT NULL COMMENT '店铺',
		score1 DECIMAL(10,3) NULL COMMENT '修正后第1项主值',
		score1_extra DECIMAL(10,3) NULL COMMENT '修正后第1项斜杠后值(拼多多)',
		score1_raw VARCHAR(32) NOT NULL DEFAULT '' COMMENT '修正后第1项原文',
		score2 DECIMAL(10,3) NULL COMMENT '修正后第2项主值',
		score2_raw VARCHAR(32) NOT NULL DEFAULT '' COMMENT '修正后第2项原文',
		score3 DECIMAL(10,3) NULL COMMENT '修正后第3项主值',
		score3_extra DECIMAL(10,3) NULL COMMENT '修正后第3项斜杠后值(拼多多)',
		score3_raw VARCHAR(32) NOT NULL DEFAULT '' COMMENT '修正后第3项原文',
		edited_by VARCHAR(64) NOT NULL DEFAULT '' COMMENT '操作人',
		edited_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '操作时间',
		UNIQUE KEY uk_dps (stat_date, platform, shop_name)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='客服服务分人工修正对照(不动RPA原始数据, 重导不丢)'`)
}

// ssParseNum "100"/"9.8" → *float64; ""/"/" → nil (与 import-service-score 同口径)
func ssParseNum(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "/" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

// ssParseSlash "4.4/2.6" → (4.4, 2.6); 普通数字副值 nil
func ssParseSlash(s string) (*float64, *float64) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "/"); i >= 0 {
		return ssParseNum(s[:i]), ssParseNum(s[i+1:])
	}
	return ssParseNum(s), nil
}

const serviceScoreSelect = `SELECT DATE_FORMAT(s.stat_date,'%Y-%m-%d'), s.platform, s.shop_name,
	COALESCE(o.score1, s.score1), COALESCE(o.score1_extra, s.score1_extra),
	COALESCE(o.score2, s.score2), COALESCE(o.score3, s.score3), COALESCE(o.score3_extra, s.score3_extra),
	s.target,
	COALESCE(NULLIF(o.score1_raw,''), s.score1_raw), COALESCE(NULLIF(o.score2_raw,''), s.score2_raw),
	COALESCE(NULLIF(o.score3_raw,''), s.score3_raw), s.target_raw,
	CASE WHEN o.id IS NULL THEN 0 ELSE 1 END
	FROM op_service_score_daily s
	LEFT JOIN service_score_override o
	  ON o.stat_date=s.stat_date AND o.platform=s.platform AND o.shop_name=s.shop_name `

type serviceScoreItem struct {
	Date        string   `json:"date"`
	Platform    string   `json:"platform"`
	ShopName    string   `json:"shopName"`
	Score1      *float64 `json:"score1"`
	Score1Extra *float64 `json:"score1Extra"`
	Score2      *float64 `json:"score2"`
	Score3      *float64 `json:"score3"`
	Score3Extra *float64 `json:"score3Extra"`
	Target      *float64 `json:"target"`
	Score1Raw   string   `json:"score1Raw"`
	Score2Raw   string   `json:"score2Raw"`
	Score3Raw   string   `json:"score3Raw"`
	TargetRaw   string   `json:"targetRaw"`
	Edited      bool     `json:"edited"`
}

func (h *DashboardHandler) queryServiceScores(dateFrom, dateTo string) ([]serviceScoreItem, error) {
	ensureServiceScoreOverride(h)
	var where []string
	var args []interface{}
	if dateFrom != "" {
		where = append(where, "s.stat_date >= ?")
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		where = append(where, "s.stat_date <= ?")
		args = append(args, dateTo)
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}
	rows, err := h.DB.Query(serviceScoreSelect+whereSQL+" ORDER BY s.platform, s.shop_name, s.stat_date", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]serviceScoreItem, 0, 512)
	for rows.Next() {
		var it serviceScoreItem
		var edited int
		if err := rows.Scan(&it.Date, &it.Platform, &it.ShopName,
			&it.Score1, &it.Score1Extra, &it.Score2, &it.Score3, &it.Score3Extra, &it.Target,
			&it.Score1Raw, &it.Score2Raw, &it.Score3Raw, &it.TargetRaw, &edited); err != nil {
			return nil, err
		}
		it.Edited = edited == 1
		list = append(list, it)
	}
	return list, rows.Err()
}

// GetServiceScores GET /api/customer/service-scores?date_from=&date_to=
func (h *DashboardHandler) GetServiceScores(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list, err := h.queryServiceScores(strings.TrimSpace(q.Get("date_from")), strings.TrimSpace(q.Get("date_to")))
	if err != nil {
		writeServerError(w, 500, "查询服务分失败", err)
		return
	}
	var latestDate string
	_ = h.DB.QueryRow(`SELECT IFNULL(DATE_FORMAT(MAX(stat_date),'%Y-%m-%d'),'') FROM op_service_score_daily`).Scan(&latestDate)
	writeJSON(w, map[string]interface{}{"list": list, "latestDate": latestDate})
}

// ServiceScoreExport GET /api/customer/service-scores/export?date_from=&date_to=&platform=
// 导出 xlsx, 数值用"修正后原文"(拼多多 4.4/2.6 等斜杠格式原样, 跟 RPA 源表一致)
func (h *DashboardHandler) ServiceScoreExport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	platform := strings.TrimSpace(q.Get("platform"))
	list, err := h.queryServiceScores(strings.TrimSpace(q.Get("date_from")), strings.TrimSpace(q.Get("date_to")))
	if err != nil {
		writeServerError(w, 500, "查询服务分失败", err)
		return
	}
	f := excelize.NewFile()
	sheet := "服务分"
	f.SetSheetName(f.GetSheetName(0), sheet)
	headers := []string{"日期", "平台", "店铺", "第1项(物流分/响应时间/发货分)", "第2项(商品分/应答率)", "第3项(服务分/满意度)", "服务分目标", "人工修正"}
	for i, hd := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, hd)
	}
	rowN := 2
	for _, it := range list {
		if platform != "" && it.Platform != platform {
			continue
		}
		vals := []interface{}{it.Date, it.Platform, it.ShopName, it.Score1Raw, it.Score2Raw, it.Score3Raw, it.TargetRaw,
			map[bool]string{true: "是", false: ""}[it.Edited]}
		for i, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(i+1, rowN)
			_ = f.SetCellValue(sheet, cell, v)
		}
		rowN++
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="service_scores.xlsx"`)
	_ = f.Write(w)
}

// ServiceScoreEdit POST /api/customer/service-scores/edit
// Body: {date, platform, shop, score1Raw, score2Raw, score3Raw} — 存修正对照表 (原始数据不动)
func (h *DashboardHandler) ServiceScoreEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, 401, "unauthorized")
		return
	}
	var req struct {
		Date      string `json:"date"`
		Platform  string `json:"platform"`
		Shop      string `json:"shop"`
		Score1Raw string `json:"score1Raw"`
		Score2Raw string `json:"score2Raw"`
		Score3Raw string `json:"score3Raw"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Date == "" || req.Platform == "" || req.Shop == "" {
		writeError(w, 400, "参数不完整")
		return
	}
	ensureServiceScoreOverride(h)
	s1, s1b := ssParseSlash(req.Score1Raw)
	s2 := ssParseNum(req.Score2Raw)
	s3, s3b := ssParseSlash(req.Score3Raw)
	_, err := h.DB.Exec(`INSERT INTO service_score_override
		(stat_date, platform, shop_name, score1, score1_extra, score1_raw, score2, score2_raw, score3, score3_extra, score3_raw, edited_by)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		 score1=VALUES(score1), score1_extra=VALUES(score1_extra), score1_raw=VALUES(score1_raw),
		 score2=VALUES(score2), score2_raw=VALUES(score2_raw),
		 score3=VALUES(score3), score3_extra=VALUES(score3_extra), score3_raw=VALUES(score3_raw),
		 edited_by=VALUES(edited_by)`,
		req.Date, req.Platform, req.Shop,
		s1, s1b, strings.TrimSpace(req.Score1Raw), s2, strings.TrimSpace(req.Score2Raw),
		s3, s3b, strings.TrimSpace(req.Score3Raw), payload.User.RealName)
	if err != nil {
		writeServerError(w, 500, "保存修正失败", err)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}

// ServiceScoreRestore POST /api/customer/service-scores/restore — 删修正, 恢复 RPA 原始值 (仅管理员)
func (h *DashboardHandler) ServiceScoreRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	payload, ok := authPayloadFromContext(r)
	if !ok || payload == nil {
		writeError(w, 401, "unauthorized")
		return
	}
	if !hasPermission(payload, "user.manage") {
		writeError(w, 403, "恢复原始值仅管理员可操作")
		return
	}
	var req struct {
		Date     string `json:"date"`
		Platform string `json:"platform"`
		Shop     string `json:"shop"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Date == "" {
		writeError(w, 400, "参数不完整")
		return
	}
	if _, err := h.DB.Exec(`DELETE FROM service_score_override WHERE stat_date=? AND platform=? AND shop_name=?`,
		req.Date, req.Platform, req.Shop); err != nil {
		writeServerError(w, 500, "恢复失败", err)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}

var _ = fmt.Sprintf // 保留 fmt 引用 (后续规则可能用)
