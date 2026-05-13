package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// 期货日线数据同步工具（新浪财经源）
//
// 用法：
//
//	sync-futures.exe                          # 增量：拉最近 7 天
//	SYNC_DAYS=30 sync-futures.exe             # 拉最近 30 天
//	SYNC_FULL=1 sync-futures.exe              # 全量：从 2018-01-01 起
//	SYNC_SYMBOLS=M0,Y0 sync-futures.exe       # 指定品种
//
// 定时任务建议：每天 17:30 跑（A 股期货收盘 15:00、夜盘 23:00；17:30 收集日盘完整数据）
//
// 数据源：新浪财经 stock2.finance.sina.com.cn/futures/api/jsonp.php/.../getDailyKLine
// 接口非官方但稳定多年，量化业界广泛使用。
//
// 备用：万一新浪挂了，可后续接东方财富 push2his.eastmoney.com（要研究 secid 编码）
func main() {
	unlock := importutil.AcquireLock("sync-futures")
	defer unlock()

	logFile, err := os.OpenFile(`C:\Users\Administrator\bi-dashboard\server\sync-futures.log`, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(io.MultiWriter(logFile, os.Stdout))
		defer logFile.Close()
	}
	log.Println("========== 开始期货日线同步 ==========")

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

	// 1. 启动时自动建表 + 灌品种字典
	ensureSchema(db)

	// 2. 读启用的品种列表
	symbols, err := loadEnabledSymbols(db)
	if err != nil {
		log.Fatalf("读品种字典失败: %v", err)
	}

	// 3. 命令行品种过滤
	if envSym := os.Getenv("SYNC_SYMBOLS"); envSym != "" {
		wanted := map[string]bool{}
		for _, s := range strings.Split(envSym, ",") {
			wanted[strings.TrimSpace(s)] = true
		}
		filtered := []string{}
		for _, s := range symbols {
			if wanted[s] {
				filtered = append(filtered, s)
			}
		}
		symbols = filtered
	}
	log.Printf("待同步品种 %d 个: %s", len(symbols), strings.Join(symbols, ", "))

	// 4. 决定时间范围（全量 vs 增量）
	isFull := os.Getenv("SYNC_FULL") == "1"
	fromDate := time.Now().AddDate(0, 0, -7)
	if days, _ := strconv.Atoi(os.Getenv("SYNC_DAYS")); days > 0 {
		fromDate = time.Now().AddDate(0, 0, -days)
	}
	if isFull {
		fromDate = time.Date(2018, 1, 1, 0, 0, 0, 0, time.Local)
		log.Println("⚠️  全量模式：拉 2018-01-01 起所有历史数据")
	} else {
		log.Printf("增量模式：拉 %s 起的数据", fromDate.Format("2006-01-02"))
	}

	// 5. 逐品种拉取入库
	totalInserted, totalUpdated := 0, 0
	for _, code := range symbols {
		t0 := time.Now()
		bars, err := fetchSinaDaily(code)
		if err != nil {
			log.Printf("[%s] 拉取失败: %v（跳过）", code, err)
			continue
		}
		ins, upd := upsertBars(db, code, bars, fromDate)
		totalInserted += ins
		totalUpdated += upd
		log.Printf("[%s] 拉到 %d 条，新增 %d 更新 %d，耗时 %v", code, len(bars), ins, upd, time.Since(t0))
		time.Sleep(300 * time.Millisecond) // 礼貌限速，避免新浪封 IP
	}

	log.Printf("========== 期货日线同步完成：合计新增 %d 更新 %d ==========", totalInserted, totalUpdated)
	fmt.Printf("期货同步完成 [%d 品种]: 新增 %d / 更新 %d\n", len(symbols), totalInserted, totalUpdated)
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

// 新浪日线 K 线返回的单条记录
type sinaBar struct {
	D string `json:"d"` // 日期 yyyy-MM-dd
	O string `json:"o"` // 开
	H string `json:"h"` // 高
	L string `json:"l"` // 低
	C string `json:"c"` // 收
	V string `json:"v"` // 成交量
	P string `json:"p"` // 持仓量
}

func fetchSinaDaily(symbol string) ([]sinaBar, error) {
	// JSONP 接口，伪装浏览器 + Referer 避免被拒
	url := fmt.Sprintf("https://stock2.finance.sina.com.cn/futures/api/jsonp.php/var=_%s_/InnerFuturesNewService.getDailyKLine?symbol=%s", symbol, symbol)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://finance.sina.com.cn/")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// JSONP 去壳：var=_XX_([{...}]);
	s := string(body)
	lp := strings.Index(s, "(")
	rp := strings.LastIndex(s, ")")
	if lp < 0 || rp <= lp {
		return nil, fmt.Errorf("JSONP 格式异常: %.100s", s)
	}
	jsonStr := s[lp+1 : rp]

	var bars []sinaBar
	if err := json.Unmarshal([]byte(jsonStr), &bars); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}
	return bars, nil
}

// 把 K 线 UPSERT 进数据库（fromDate 之前的跳过，节省 IO）
func upsertBars(db *sql.DB, code string, bars []sinaBar, fromDate time.Time) (inserted, updated int) {
	stmt, err := db.Prepare(`
		INSERT INTO futures_price_daily
			(symbol_code, trade_date, open_price, high_price, low_price, close_price, volume, open_interest)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			open_price=VALUES(open_price), high_price=VALUES(high_price),
			low_price=VALUES(low_price), close_price=VALUES(close_price),
			volume=VALUES(volume), open_interest=VALUES(open_interest)
	`)
	if err != nil {
		log.Printf("[%s] 准备 UPSERT 语句失败: %v", code, err)
		return 0, 0
	}
	defer stmt.Close()

	for _, b := range bars {
		// 跳过过早的数据
		d, err := time.Parse("2006-01-02", b.D)
		if err != nil || d.Before(fromDate) {
			continue
		}

		o := parseFloat(b.O)
		h := parseFloat(b.H)
		l := parseFloat(b.L)
		c := parseFloat(b.C)
		v := parseInt(b.V)
		p := parseInt(b.P)

		// 全 0 的脏数据跳过
		if o == 0 && h == 0 && l == 0 && c == 0 {
			continue
		}

		res, err := stmt.Exec(code, b.D, o, h, l, c, v, p)
		if err != nil {
			log.Printf("[%s] %s UPSERT 失败: %v", code, b.D, err)
			continue
		}
		// MySQL ON DUPLICATE KEY UPDATE 行为：新增返回 1，更新返回 2，无变化返回 0
		n, _ := res.RowsAffected()
		switch n {
		case 1:
			inserted++
		case 2:
			updated++
		}
	}
	return inserted, updated
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func parseInt(s string) int64 {
	// 持仓量/成交量可能是小数（如 "1234.0"），先转 float 再取整
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return int64(f)
}
