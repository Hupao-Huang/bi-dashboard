package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"
)

// 期货行情相关接口
//
// 数据来源: 新浪财经期货日线 (sync-futures.exe 每日 17:30 拉取)
// 数据范围: 16 个品种主连合约, 2018-01-01 起的日线 OHLCV

// FuturesSymbol 期货品种字典
type FuturesSymbol struct {
	Code        string `json:"code"`
	NameCN      string `json:"nameCn"`
	Exchange    string `json:"exchange"`
	Category    string `json:"category"`
	Unit        string `json:"unit"`
	BusinessTag string `json:"businessTag"`
	SortOrder   int    `json:"sortOrder"`
}

// FuturesQuote 一个品种的最新行情快照（行情总览页用）
type FuturesQuote struct {
	FuturesSymbol
	TradeDate    string  `json:"tradeDate"`
	Close        float64 `json:"close"`
	PrevClose    float64 `json:"prevClose"`
	Change       float64 `json:"change"`       // 收盘 - 昨收
	ChangePct    float64 `json:"changePct"`    // 百分比
	High         float64 `json:"high"`
	Low          float64 `json:"low"`
	Open         float64 `json:"open"`
	Volume       int64   `json:"volume"`
	OpenInterest int64   `json:"openInterest"`
	// MiniTrend 最近 30 天收盘价（首页迷你折线图用，节省一次请求）
	MiniTrend []float64 `json:"miniTrend"`
}

// FuturesBar 一根 K 线 / 一个日线点
type FuturesBar struct {
	Date         string  `json:"date"`
	Open         float64 `json:"open"`
	High         float64 `json:"high"`
	Low          float64 `json:"low"`
	Close        float64 `json:"close"`
	Volume       int64   `json:"volume"`
	OpenInterest int64   `json:"openInterest"`
}

// GetFuturesSymbols GET /api/futures/symbols
// 返回所有启用的品种字典（前端筛选下拉用）
func (h *DashboardHandler) GetFuturesSymbols(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := h.DB.QueryContext(ctx, `
		SELECT symbol_code, name_cn, exchange, category, COALESCE(unit,''), COALESCE(business_tag,''), sort_order
		FROM futures_symbol
		WHERE is_enabled=1
		ORDER BY sort_order, symbol_code`)
	if err != nil {
		writeServerError(w, 500, "查询期货品种失败", err)
		return
	}
	defer rows.Close()

	out := []FuturesSymbol{}
	for rows.Next() {
		var s FuturesSymbol
		if err := rows.Scan(&s.Code, &s.NameCN, &s.Exchange, &s.Category, &s.Unit, &s.BusinessTag, &s.SortOrder); err != nil {
			writeServerError(w, 500, "读期货品种失败", err)
			return
		}
		out = append(out, s)
	}
	writeJSON(w, out)
}

// GetFuturesQuotes GET /api/futures/quotes
// 返回所有品种的最新收盘行情 + 涨跌 + 最近 30 天迷你折线，首页一次请求搞定
func (h *DashboardHandler) GetFuturesQuotes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	// 1. 拿所有启用品种
	syms, err := loadSymbolMap(ctx, h.DB)
	if err != nil {
		writeServerError(w, 500, "查询期货品种失败", err)
		return
	}

	// 2. 一次查每个品种最近 30 天日线（一个 SQL 用窗口函数）
	//    没用窗口函数（兼容 MySQL 5.7+）：用子查询 ROW_NUMBER 替代
	rows, err := h.DB.QueryContext(ctx, `
		SELECT t.symbol_code, t.trade_date, t.open_price, t.high_price, t.low_price, t.close_price, t.volume, t.open_interest
		FROM (
			SELECT
				p.symbol_code, p.trade_date, p.open_price, p.high_price, p.low_price, p.close_price, p.volume, p.open_interest,
				(@rk := IF(@cur = p.symbol_code, @rk + 1, 1)) AS rk,
				(@cur := p.symbol_code)
			FROM futures_price_daily p
			CROSS JOIN (SELECT @cur := '', @rk := 0) v
			ORDER BY p.symbol_code, p.trade_date DESC
		) t
		WHERE t.rk <= 30
		ORDER BY t.symbol_code, t.trade_date ASC`)
	if err != nil {
		writeServerError(w, 500, "查询期货日线失败", err)
		return
	}
	defer rows.Close()

	// 收集每个品种的最近 30 天
	bucket := map[string][]FuturesBar{}
	for rows.Next() {
		var code string
		var b FuturesBar
		if err := rows.Scan(&code, &b.Date, &b.Open, &b.High, &b.Low, &b.Close, &b.Volume, &b.OpenInterest); err != nil {
			writeServerError(w, 500, "读期货日线失败", err)
			return
		}
		bucket[code] = append(bucket[code], b)
	}

	// 3. 拼装 Quote 列表
	out := []FuturesQuote{}
	for _, s := range syms {
		bars := bucket[s.Code]
		if len(bars) == 0 {
			continue // 该品种暂无数据
		}
		last := bars[len(bars)-1]
		var prevClose float64
		if len(bars) >= 2 {
			prevClose = bars[len(bars)-2].Close
		} else {
			prevClose = last.Close
		}
		change := last.Close - prevClose
		var changePct float64
		if prevClose > 0 {
			changePct = change / prevClose * 100
		}
		mini := make([]float64, 0, len(bars))
		for _, b := range bars {
			mini = append(mini, b.Close)
		}
		out = append(out, FuturesQuote{
			FuturesSymbol: s,
			TradeDate:     last.Date,
			Close:         last.Close,
			PrevClose:     prevClose,
			Change:        change,
			ChangePct:     changePct,
			High:          last.High,
			Low:           last.Low,
			Open:          last.Open,
			Volume:        last.Volume,
			OpenInterest:  last.OpenInterest,
			MiniTrend:     mini,
		})
	}
	writeJSON(w, out)
}

// GetFuturesDaily GET /api/futures/daily?code=M0&start=2024-01-01&end=2026-05-13
// 单品种历史日线（走势图/详情页用）
// 不带 start/end 默认返回最近 250 个交易日（约一年）
func (h *DashboardHandler) GetFuturesDaily(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		writeError(w, 400, "code 参数必填")
		return
	}
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var rows *sql.Rows
	var err error
	if start != "" && end != "" {
		rows, err = h.DB.QueryContext(ctx, `
			SELECT trade_date, open_price, high_price, low_price, close_price, volume, open_interest
			FROM futures_price_daily
			WHERE symbol_code=? AND trade_date BETWEEN ? AND ?
			ORDER BY trade_date ASC`, code, start, end)
	} else {
		// 默认最近 250 个交易日（约 1 年）
		rows, err = h.DB.QueryContext(ctx, `
			SELECT trade_date, open_price, high_price, low_price, close_price, volume, open_interest
			FROM (
				SELECT trade_date, open_price, high_price, low_price, close_price, volume, open_interest
				FROM futures_price_daily
				WHERE symbol_code=?
				ORDER BY trade_date DESC LIMIT 250
			) t
			ORDER BY trade_date ASC`, code)
	}
	if err != nil {
		writeServerError(w, 500, "查询期货日线失败", err)
		return
	}
	defer rows.Close()

	bars := []FuturesBar{}
	for rows.Next() {
		var b FuturesBar
		if err := rows.Scan(&b.Date, &b.Open, &b.High, &b.Low, &b.Close, &b.Volume, &b.OpenInterest); err != nil {
			writeServerError(w, 500, "读期货日线失败", err)
			return
		}
		bars = append(bars, b)
	}

	// 拿品种元信息一起返回，前端少一次请求
	syms, _ := loadSymbolMap(ctx, h.DB)
	meta, ok := syms[code]
	if !ok {
		writeError(w, 404, "品种不存在")
		return
	}

	writeJSON(w, map[string]interface{}{
		"symbol": meta,
		"bars":   bars,
	})
}

// loadSymbolMap 把品种字典读成 map 便于查找
func loadSymbolMap(ctx context.Context, db *sql.DB) (map[string]FuturesSymbol, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT symbol_code, name_cn, exchange, category, COALESCE(unit,''), COALESCE(business_tag,''), sort_order
		FROM futures_symbol
		WHERE is_enabled=1
		ORDER BY sort_order, symbol_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]FuturesSymbol{}
	for rows.Next() {
		var s FuturesSymbol
		if err := rows.Scan(&s.Code, &s.NameCN, &s.Exchange, &s.Category, &s.Unit, &s.BusinessTag, &s.SortOrder); err != nil {
			return nil, err
		}
		out[s.Code] = s
	}
	return out, nil
}
