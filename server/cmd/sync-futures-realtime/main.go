package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// 期货盘中实时快照同步工具（新浪财经实时报价源）
//
// 与 sync-futures(日线, 每天 17:30 一次)互补:
//   - sync-futures        = 每天收盘后拉"结算后的日线"入历史表 futures_price_daily
//   - sync-futures-realtime = 盘中每 5 分钟拉"最新价"更新快照表 futures_quote_realtime
//
// 用法:
//
//	sync-futures-realtime.exe          # 拉所有启用品种的最新报价, 更新快照表
//
// 数据源: 新浪实时报价 https://hq.sinajs.cn/list=nf_M0,nf_Y0,...
//   一次请求批量拿全部品种, 返回 GBK 文本, 每行 var hq_str_nf_<code>="字段,字段,...";
//   字段下标(已用库内真实日线交叉验证):
//     [0]名称 [1]时间HHMMSS [2]今开 [3]最高 [4]最低 [8]最新价(现价)
//     [10]昨结算(涨跌基准) [13]持仓量 [14]成交量 [17]交易日YYYY-MM-DD
//
// 定时任务: 每 5 分钟跑一次(BI-SyncFuturesRealtime), 工具内部自带交易时段闸门——
//   仅周一至周五 08:00~23:59 真正拉取, 其余时段直接退出, 不打扰新浪也不写脏数据。
//   "盘中实时 / 休市"的判定由前端按新浪报价时间算, 与本任务跑不跑无关。
func main() {
	unlock := importutil.AcquireLock("sync-futures-realtime")
	defer unlock()

	logFile, err := os.OpenFile(`C:\Users\Administrator\bi-dashboard\server\sync-futures-realtime.log`, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(io.MultiWriter(logFile, os.Stdout))
		defer logFile.Close()
	}

	// 交易时段闸门: 周末 + 深夜(03:00~07:59)直接退出, 省得空跑
	// FORCE=1 跳过闸门(手动测试用)
	if os.Getenv("FORCE") != "1" && !inTradingWindow(time.Now()) {
		log.Println("当前非交易时段(周末或深夜), 跳过实时同步")
		return
	}

	log.Println("---------- 期货实时快照同步 ----------")

	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()
	db.SetConnMaxLifetime(5 * time.Minute)

	ensureRealtimeSchema(db)

	symbols, err := loadEnabledSymbols(db)
	if err != nil {
		log.Fatalf("读品种字典失败: %v", err)
	}
	if len(symbols) == 0 {
		log.Println("无启用品种, 退出")
		return
	}

	quotes, err := fetchSinaRealtime(symbols)
	if err != nil {
		log.Fatalf("拉取新浪实时报价失败: %v", err)
	}

	updated := upsertQuotes(db, quotes)
	log.Printf("实时快照更新 %d / %d 个品种", updated, len(symbols))

	// 只清"行情总览"这一块缓存, 不碰综合看板等其它缓存
	if updated > 0 {
		clearFuturesCache(cfg.Webhook.Secret)
	}
	fmt.Printf("期货实时同步完成: 更新 %d 个品种\n", updated)
}

// inTradingWindow 粗判是否在期货交易时段附近(只为省空跑, 不求精确)
// 工作日 且 小时 ∈ [8,24)∪[0,3) 视为可能在交易:
//   覆盖 日盘 9~15 + 夜盘 21~23(农产品/包材) + 有色/能源/黄金夜盘到次日 2:30
// 只挡掉 03:00~07:59 这段真正没交易的深夜, 以及整个周末。
func inTradingWindow(now time.Time) bool {
	wd := now.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}
	h := now.Hour()
	return h >= 8 || h < 3
}

func loadEnabledSymbols(db *sql.DB) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, `SELECT symbol_code FROM futures_symbol WHERE is_enabled=1 ORDER BY sort_order, symbol_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// realtimeQuote 一个品种的盘中快照
type realtimeQuote struct {
	Code         string
	QuoteDate    string // YYYY-MM-DD
	QuoteTime    string // HH:MM:SS
	Last         float64
	Open         float64
	High         float64
	Low          float64
	PrevSettle   float64
	Volume       int64
	OpenInterest int64
}

// fetchSinaRealtime 一次请求批量拉所有品种最新报价
func fetchSinaRealtime(symbols []string) (map[string]realtimeQuote, error) {
	parts := make([]string, 0, len(symbols))
	for _, c := range symbols {
		parts = append(parts, "nf_"+c)
	}
	reqURL := "https://hq.sinajs.cn/list=" + strings.Join(parts, ",")

	req, _ := http.NewRequest("GET", reqURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://finance.sina.com.cn/") // 必带, 否则新浪拒绝

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	out := map[string]realtimeQuote{}
	for _, line := range strings.Split(string(body), "\n") {
		q, ok := parseSinaLine(line)
		if ok {
			out[q.Code] = q
		}
	}
	return out, nil
}

// parseSinaLine 解析一行 var hq_str_nf_M0="名称,时间,开,高,低,...";
func parseSinaLine(line string) (realtimeQuote, bool) {
	const marker = "hq_str_nf_"
	i := strings.Index(line, marker)
	if i < 0 {
		return realtimeQuote{}, false
	}
	rest := line[i+len(marker):]
	eq := strings.Index(rest, "=")
	if eq < 0 {
		return realtimeQuote{}, false
	}
	code := strings.TrimSpace(rest[:eq])
	lq := strings.Index(rest, "\"")
	rq := strings.LastIndex(rest, "\"")
	if lq < 0 || rq <= lq {
		return realtimeQuote{}, false
	}
	fields := strings.Split(rest[lq+1:rq], ",")
	if len(fields) < 18 {
		return realtimeQuote{}, false // 停盘/无数据行(空字符串)
	}

	// 字段[17]必须是 YYYY-MM-DD 交易日 —— 这是商品期货布局的特征。
	// 中金所股指期货(如 IF0)用的是另一套字段排列(开高低现价在最前/日期在末尾),
	// 其[17]不是日期, 会被这里挡掉, 优雅回退到日线收盘价(股指只有日盘, 收盘价日线已覆盖)。
	if strings.Count(strings.TrimSpace(fields[17]), "-") != 2 {
		return realtimeQuote{}, false
	}

	last := parseFloat(fields[8])
	if last <= 0 {
		return realtimeQuote{}, false // 最新价为 0 = 当前无有效报价, 不覆盖
	}

	// 时间 HHMMSS -> HH:MM:SS
	t := strings.TrimSpace(fields[1])
	qtime := t
	if len(t) == 6 {
		qtime = t[0:2] + ":" + t[2:4] + ":" + t[4:6]
	}

	return realtimeQuote{
		Code:         code,
		QuoteDate:    strings.TrimSpace(fields[17]),
		QuoteTime:    qtime,
		Last:         last,
		Open:         parseFloat(fields[2]),
		High:         parseFloat(fields[3]),
		Low:          parseFloat(fields[4]),
		PrevSettle:   parseFloat(fields[10]),
		Volume:       parseInt(fields[14]),
		OpenInterest: parseInt(fields[13]),
	}, true
}

func upsertQuotes(db *sql.DB, quotes map[string]realtimeQuote) int {
	stmt, err := db.Prepare(`
		INSERT INTO futures_quote_realtime
			(symbol_code, quote_date, quote_time, last_price, open_price, high_price, low_price, prev_settle, volume, open_interest)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			quote_date=VALUES(quote_date), quote_time=VALUES(quote_time), last_price=VALUES(last_price),
			open_price=VALUES(open_price), high_price=VALUES(high_price), low_price=VALUES(low_price),
			prev_settle=VALUES(prev_settle), volume=VALUES(volume), open_interest=VALUES(open_interest)
	`)
	if err != nil {
		log.Printf("准备 UPSERT 语句失败: %v", err)
		return 0
	}
	defer stmt.Close()

	n := 0
	for _, q := range quotes {
		date := q.QuoteDate
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}
		if _, err := stmt.Exec(q.Code, date, q.QuoteTime, q.Last, q.Open, q.High, q.Low, q.PrevSettle, q.Volume, q.OpenInterest); err != nil {
			log.Printf("[%s] UPSERT 失败: %v", q.Code, err)
			continue
		}
		n++
	}
	return n
}

// clearFuturesCache 只清"原料行情总览"接口缓存(prefix 精准匹配), 不影响别的看板
func clearFuturesCache(secret string) {
	prefix := "api|/api/futures/quotes"
	reqURL := "http://127.0.0.1:8080/api/webhook/clear-cache?prefix=" + url.QueryEscape(prefix)
	req, err := http.NewRequest("POST", reqURL, strings.NewReader("{}"))
	if err != nil {
		log.Printf("[clear-cache] 构造请求失败: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Secret", secret)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[clear-cache] 调用失败(bi-server 可能没起): %v", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	log.Printf("[clear-cache] status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func parseInt(s string) int64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return int64(f)
}
