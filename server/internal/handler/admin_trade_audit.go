package handler

// 销售单核对页 — 按月按日聚合 trade/trade_goods/trade_package 三表行数, 用于跑哥手工对吉客云后台.
// 口径与 dashboard 一致: 按 consign_time(发货时间) 分日, trade_YYYYMM 表按月分区.

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type tradeAuditDayRow struct {
	Date         string `json:"date"`
	TradeCount   int64  `json:"tradeCount"`
	GoodsCount   int64  `json:"goodsCount"`
	PackageCount int64  `json:"packageCount"`
}

type tradeAuditResponse struct {
	Month            string             `json:"month"`
	Rows             []tradeAuditDayRow `json:"rows"`
	TotalTradeCount  int64              `json:"totalTradeCount"`
	TotalGoodsCount  int64              `json:"totalGoodsCount"`
	TotalPackageCnt  int64              `json:"totalPackageCount"`
	TableExists      bool               `json:"tableExists"`
}

// AdminTradeAudit GET /api/admin/trade-audit?month=2026-01
// 返回该月每天的销售单/明细/包裹数, 供跑哥与吉客云后台核对.
func (h *DashboardHandler) AdminTradeAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	month := strings.TrimSpace(r.URL.Query().Get("month"))
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	t, err := time.Parse("2006-01", month)
	if err != nil {
		writeError(w, http.StatusBadRequest, "month 格式必须是 YYYY-MM")
		return
	}
	ym := t.Format("200601") // 202601

	tradeTbl := "trade_" + ym
	goodsTbl := "trade_goods_" + ym
	pkgTbl := "trade_package_" + ym

	resp := tradeAuditResponse{
		Month: month,
		Rows:  []tradeAuditDayRow{},
	}

	// 检查主表存在
	var existCnt int
	err = h.DB.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?`, tradeTbl).Scan(&existCnt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "查询表存在失败: "+err.Error())
		return
	}
	if existCnt == 0 {
		writeJSON(w, resp) // 表不存在直接返回空, TableExists=false
		return
	}
	resp.TableExists = true

	// 主单数: 按 DATE(consign_time) GROUP BY
	tradeMap := map[string]int64{}
	rows, err := h.DB.Query(fmt.Sprintf(
		`SELECT DATE_FORMAT(consign_time,'%%Y-%%m-%%d') AS d, COUNT(*) FROM %s WHERE consign_time IS NOT NULL GROUP BY d`,
		tradeTbl,
	))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "查询销售单数失败: "+err.Error())
		return
	}
	for rows.Next() {
		var d string
		var c int64
		if err := rows.Scan(&d, &c); err == nil {
			tradeMap[d] = c
			resp.TotalTradeCount += c
		}
	}
	rows.Close()

	// 明细行数: trade_goods JOIN trade
	goodsMap := map[string]int64{}
	rows, err = h.DB.Query(fmt.Sprintf(
		`SELECT DATE_FORMAT(t.consign_time,'%%Y-%%m-%%d') AS d, COUNT(*)
		 FROM %s g JOIN %s t ON g.trade_no=t.trade_no
		 WHERE t.consign_time IS NOT NULL GROUP BY d`,
		goodsTbl, tradeTbl,
	))
	if err == nil {
		for rows.Next() {
			var d string
			var c int64
			if err := rows.Scan(&d, &c); err == nil {
				goodsMap[d] = c
				resp.TotalGoodsCount += c
			}
		}
		rows.Close()
	}

	// 包裹数: trade_package JOIN trade
	pkgMap := map[string]int64{}
	rows, err = h.DB.Query(fmt.Sprintf(
		`SELECT DATE_FORMAT(t.consign_time,'%%Y-%%m-%%d') AS d, COUNT(*)
		 FROM %s p JOIN %s t ON p.trade_id=t.trade_id
		 WHERE t.consign_time IS NOT NULL GROUP BY d`,
		pkgTbl, tradeTbl,
	))
	if err == nil {
		for rows.Next() {
			var d string
			var c int64
			if err := rows.Scan(&d, &c); err == nil {
				pkgMap[d] = c
				resp.TotalPackageCnt += c
			}
		}
		rows.Close()
	}

	// 合并按日列表, 当月每一天都出一行
	daysInMonth := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	for day := 1; day <= daysInMonth; day++ {
		d := fmt.Sprintf("%s-%02d", month, day)
		resp.Rows = append(resp.Rows, tradeAuditDayRow{
			Date:         d,
			TradeCount:   tradeMap[d],
			GoodsCount:   goodsMap[d],
			PackageCount: pkgMap[d],
		})
	}

	writeJSON(w, resp)
}
