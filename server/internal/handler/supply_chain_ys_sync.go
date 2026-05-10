package handler

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"
)

var syncYSStockMu sync.Mutex
var syncYSStockRunning bool
var syncYSLastEndTime time.Time // 见上方 7 重防御文档

// syncYSProgress v0.71: 同步进度状态 (前端轮询用)
type syncStepProgress struct {
	Name        string `json:"name"`
	Ins         int    `json:"ins"`
	Upd         int    `json:"upd"`
	Err         int    `json:"err"`
	DurationSec int    `json:"durationSec"`
	Failed      bool   `json:"failed"`
	Message     string `json:"message,omitempty"`
}
type syncProgressState struct {
	Running     bool               `json:"running"`
	Done        bool               `json:"done"`
	StartTime   time.Time          `json:"-"`
	StartedAt   string             `json:"startedAt"`
	TotalSteps  int                `json:"totalSteps"`
	CurrentStep int                `json:"currentStep"` // 1-indexed, 0=未开始
	CurrentName string             `json:"currentName"`
	Results     []syncStepProgress `json:"results"`
	ElapsedSec  int                `json:"elapsedSec"`
	Err         string             `json:"err,omitempty"`
}

var syncYSProgress syncProgressState

// SyncYSStock v0.68: 一键全量同步 YS 4 类数据 (现存量+采购订单+委外订单+材料出库)
// POST /api/supply-chain/sync-ys-stock (路由名保留以保持兼容, 内部行为已升级)
// 串行执行避免 YS API 限流, 总耗时约 60-120s
func (h *DashboardHandler) SyncYSStock(w http.ResponseWriter, r *http.Request) {
	// v0.73 诊断日志: 记录每次 sync 请求的来源, 排查"自动重新同步"问题
	log.Printf("[sync-ys-stock] 收到请求 method=%s remote=%s referer=%q ua=%q origin=%q",
		r.Method, r.RemoteAddr, r.Header.Get("Referer"), r.Header.Get("User-Agent"), r.Header.Get("Origin"))

	if r.Method != "POST" {
		log.Printf("[sync-ys-stock] 拒绝: 非 POST")
		writeError(w, 405, "method not allowed")
		return
	}

	syncYSStockMu.Lock()
	if syncYSStockRunning {
		syncYSStockMu.Unlock()
		log.Printf("[sync-ys-stock] 拒绝: 已有同步在执行")
		writeError(w, 429, "已有同步任务正在执行, 请稍后再试")
		return
	}
	// v0.74: 全局后端 cooldown 60s — 防止任何来源(浏览器 bug/扩展/双 tab/手抖)在上次同步完成后立即触发新一轮
	if !syncYSLastEndTime.IsZero() {
		since := time.Since(syncYSLastEndTime)
		if since < 60*time.Second {
			wait := int((60*time.Second - since).Seconds())
			syncYSStockMu.Unlock()
			log.Printf("[sync-ys-stock] 拒绝: 上次同步 %.0fs 前结束, cooldown 还需 %ds (来源 %s referer=%s)",
				since.Seconds(), wait, r.RemoteAddr, r.Header.Get("Referer"))
			writeError(w, 429, fmt.Sprintf("上次同步刚完成, 请 %d 秒后再试", wait))
			return
		}
	}
	syncYSStockRunning = true
	log.Printf("[sync-ys-stock] ★ 启动新一轮同步, 锁已获取")
	// v0.71: 重置进度状态
	syncYSProgress = syncProgressState{
		Running:   true,
		StartTime: time.Now(),
		StartedAt: time.Now().Format("2006-01-02 15:04:05"),
		Results:   []syncStepProgress{},
	}
	syncYSStockMu.Unlock()
	defer func() {
		syncYSStockMu.Lock()
		syncYSStockRunning = false
		syncYSLastEndTime = time.Now() // v0.74: 记录完成时间, 触发 60s 全局 cooldown
		syncYSProgress.Running = false
		syncYSProgress.Done = true
		syncYSProgress.ElapsedSec = int(time.Since(syncYSProgress.StartTime).Seconds())
		syncYSStockMu.Unlock()
	}()

	start := time.Now()
	exeDir, _ := filepath.Abs(filepath.Dir(os.Args[0]))

	type stepResult = syncStepProgress

	// v0.70: 立即同步按钮动态算范围 — 按本地"未关闭"单的最早 vouchdate 算起点
	// 业务规则: 已关闭单不会再变化, 只要覆盖未关闭单的 vouchdate 范围即可
	// 兜底 30 天 (防止本地暂时无未结单, 仍保留近期窗口)
	rangeEnd := time.Now().Format("2006-01-02")
	defaultStart := time.Now().AddDate(0, 0, -30).Format("2006-01-02")

	// 采购未结单 = status IN (2,3,4) AND qty > total_in_qty
	purchaseStart := defaultStart
	var minPurchase sql.NullString
	if err := h.DB.QueryRow(`SELECT DATE_FORMAT(MIN(vouchdate), '%Y-%m-%d') FROM ys_purchase_orders
		WHERE purchase_orders_in_wh_status IN (2,3,4) AND qty > IFNULL(total_in_qty, 0)`).Scan(&minPurchase); err == nil {
		if minPurchase.Valid && minPurchase.String != "" && minPurchase.String < purchaseStart {
			purchaseStart = minPurchase.String
		}
	}

	// 委外未结单 = status NOT IN (2) AND quantity > incoming
	subcontractStart := defaultStart
	var minSubcontract sql.NullString
	if err := h.DB.QueryRow(`SELECT DATE_FORMAT(MIN(vouchdate), '%Y-%m-%d') FROM ys_subcontract_orders
		WHERE status NOT IN (2)
		  AND order_product_subcontract_quantity_mu > IFNULL(order_product_incoming_quantity, 0)`).Scan(&minSubcontract); err == nil {
		if minSubcontract.Valid && minSubcontract.String != "" && minSubcontract.String < subcontractStart {
			subcontractStart = minSubcontract.String
		}
	}

	purchaseLabel := "采购订单 (" + purchaseStart + " ~ " + rangeEnd + ")"
	subcontractLabel := "委外订单 (" + subcontractStart + " ~ " + rangeEnd + ")"

	exes := []struct {
		name, exe string
		args      []string
	}{
		{"吉客云库存", "sync-stock.exe", nil}, // v0.76: 成品 Tab 数据源
		{"YS 现存量", "sync-yonsuite-stock.exe", nil},
		{purchaseLabel, "sync-yonsuite-purchase.exe", []string{purchaseStart, rangeEnd}},
		{subcontractLabel, "sync-yonsuite-subcontract.exe", []string{subcontractStart, rangeEnd}},
		{"YS 材料出库", "sync-yonsuite-materialout.exe", nil},
	}

	re := regexp.MustCompile(`新增 (\d+) / 更新 (\d+) / 失败 (\d+)`)
	results := make([]stepResult, 0, len(exes))
	var totalIns, totalUpd, totalErr int

	// v0.71: 进度推送 — 每步开始时更新 currentStep + currentName
	syncYSStockMu.Lock()
	syncYSProgress.TotalSteps = len(exes)
	syncYSStockMu.Unlock()

	for i, item := range exes {
		// 步骤开始
		syncYSStockMu.Lock()
		syncYSProgress.CurrentStep = i + 1
		syncYSProgress.CurrentName = item.name
		syncYSProgress.ElapsedSec = int(time.Since(syncYSProgress.StartTime).Seconds())
		syncYSStockMu.Unlock()

		stepStart := time.Now()

		// v0.79: 吉客云库存走公共入口, 跟 InventoryWarning 页面共享 syncStockMu + state
		var output string
		var err error
		if item.exe == "sync-stock.exe" {
			elap, out, locked, runErr := RunSyncStockOnce()
			if locked {
				r := stepResult{Name: item.name, Failed: true,
					Message: "另一处吉客云库存同步在跑, 已跳过这一步"}
				results = append(results, r)
				syncYSStockMu.Lock()
				syncYSProgress.Results = append(syncYSProgress.Results, r)
				syncYSStockMu.Unlock()
				continue
			}
			_ = elap
			output = out
			err = runErr
		} else {
			exePath := filepath.Join(exeDir, item.exe)
			if _, statErr := os.Stat(exePath); statErr != nil {
				r := stepResult{Name: item.name, Failed: true,
					Message: "exe 文件缺失: " + item.exe}
				results = append(results, r)
				syncYSStockMu.Lock()
				syncYSProgress.Results = append(syncYSProgress.Results, r)
				syncYSStockMu.Unlock()
				continue
			}
			cmd := exec.Command(exePath, item.args...)
			cmd.Dir = exeDir
			out, runErr := cmd.CombinedOutput()
			output = string(out)
			err = runErr
		}
		var ins, upd, errN int
		if m := re.FindStringSubmatch(output); len(m) == 4 {
			ins, _ = strconv.Atoi(m[1])
			upd, _ = strconv.Atoi(m[2])
			errN, _ = strconv.Atoi(m[3])
		}
		failed := err != nil
		msg := ""
		if failed {
			msg = err.Error()
			log.Printf("[sync-ys-all] %s 失败: err=%v output=%s", item.name, err, output)
		}
		r := stepResult{
			Name: item.name, Ins: ins, Upd: upd, Err: errN,
			DurationSec: int(time.Since(stepStart).Seconds()),
			Failed:      failed, Message: msg,
		}
		results = append(results, r)
		// 步骤结束 — 推送到 progress
		syncYSStockMu.Lock()
		syncYSProgress.Results = append(syncYSProgress.Results, r)
		syncYSProgress.ElapsedSec = int(time.Since(syncYSProgress.StartTime).Seconds())
		syncYSStockMu.Unlock()
		totalIns += ins
		totalUpd += upd
		totalErr += errN
	}

	// 清缓存 — 计划采购 + 库存 + 供应链整段
	cleared := ClearCacheByPrefix("api|/api/supply-chain")
	cleared += ClearCacheByPrefix("api|/api/stock/")

	log.Printf("[sync-ys-all] 完成 总 ins=%d upd=%d err=%d cache=%d 耗时=%.1fs",
		totalIns, totalUpd, totalErr, cleared, time.Since(start).Seconds())

	writeJSON(w, map[string]interface{}{
		"ok":           true,
		"steps":        results,
		"ins":          totalIns,
		"upd":          totalUpd,
		"err":          totalErr,
		"cacheCleared": cleared,
		"durationSec":  int(time.Since(start).Seconds()),
	})
}

// GetSyncYSProgress v0.71: 同步进度查询 (前端轮询)
// GET /api/supply-chain/sync-ys-progress
// 返回当前同步状态: running/done/totalSteps/currentStep/currentName/results/elapsedSec
func (h *DashboardHandler) GetSyncYSProgress(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDomainAccess(r, "supply_chain")) {
		return
	}
	syncYSStockMu.Lock()
	defer syncYSStockMu.Unlock()
	// 复制一份当前 progress (避免共享指针在 JSON 序列化时被并发改写)
	snapshot := syncProgressState{
		Running:     syncYSProgress.Running,
		Done:        syncYSProgress.Done,
		StartedAt:   syncYSProgress.StartedAt,
		TotalSteps:  syncYSProgress.TotalSteps,
		CurrentStep: syncYSProgress.CurrentStep,
		CurrentName: syncYSProgress.CurrentName,
		Results:     append([]syncStepProgress{}, syncYSProgress.Results...),
		Err:         syncYSProgress.Err,
	}
	if syncYSProgress.Running {
		snapshot.ElapsedSec = int(time.Since(syncYSProgress.StartTime).Seconds())
	} else {
		snapshot.ElapsedSec = syncYSProgress.ElapsedSec
	}
	writeJSON(w, snapshot)
}

// GetInTransitDetail v0.67: 在途采购/委外订单明细 (按 SKU 下钻)
// 参数: goodsNo (吉客云 goods_no, 必填)
// 返回: { purchaseOrders: [...], subcontractOrders: [...] }
//
// 桥接策略 (双路径, v1.01 文档化真实场景):
//  1. 主路径: goods.sku_code → YS product_c_code 桥接 (99% 命中)
//  2. 兜底:   ys_*.product_c_code = goodsNo 直接相等
//     场景: 用友 ERP 已建档(委外/采购下单)但吉客云物料档案尚未建立
//     真实案例 2026-05-08: 03030236 减钠松茸薄盐生抽 — 委外 WWDD20260430000003 已下,
//                          吉客云物料档案次日 sync-goods 才补齐
//     依赖: BI-SyncGoods schtasks 每天 04:00 全量同步, 兜底窗口 ≤24h
