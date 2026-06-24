package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// RPA 文件根目录
const rpaBaseDir = `Z:\信息部\RPA_集团数据看板`

// items_map.json 结构：platform -> dataItem -> { stores: [...] }
type rpaItemsMap map[string]map[string]struct {
	Stores []string `json:"stores"`
}

// 扫描结果缓存（5分钟）
var (
	rpaScanMu        sync.RWMutex
	rpaScanCache     *rpaScanResult
	rpaScanCachedAt  time.Time
	rpaScanCacheTTL  = 5 * time.Minute
)

// -------- 响应结构体 --------

type rpaScanResult struct {
	ScannedAt string            `json:"scanned_at"`
	Platforms []rpaPlatformInfo `json:"platforms"`
}

type rpaPlatformInfo struct {
	Name         string        `json:"name"`
	Completeness float64       `json:"completeness"`
	Status       string        `json:"status"`
	Dates        []rpaDateInfo `json:"dates"`
}

type rpaDateInfo struct {
	Date          string         `json:"date"`
	FormattedDate string         `json:"formatted_date"`
	Completeness  float64        `json:"completeness"`
	Status        string         `json:"status"`
	DBImported    bool           `json:"db_imported"`
	Stores        []rpaStoreInfo `json:"stores"`
}

type rpaStoreInfo struct {
	Name           string   `json:"name"`
	Completeness   float64  `json:"completeness"`
	Status         string   `json:"status"`
	CompletedItems []string `json:"completed_items"`
	MissingItems   []string `json:"missing_items"`
}

// -------- 状态辅助 --------

func rpaStatus(completeness float64) string {
	if completeness >= 1.0 {
		return "complete"
	}
	if completeness > 0 {
		return "partial"
	}
	return "missing"
}

// yyyymmdd 格式校验
var yyyymmddRe = regexp.MustCompile(`^\d{8}$`)

// -------- 核心扫描逻辑 --------

func doRPAScan() *rpaScanResult {
	// 读取 items_map.json
	mapPath := filepath.Join(rpaBaseDir, "items_map.json")
	raw, err := os.ReadFile(mapPath)
	if err != nil {
		log.Printf("[rpa_scan] 读取 items_map.json 失败: %v", err)
		return &rpaScanResult{
			ScannedAt: time.Now().Format("2006-01-02T15:04:05"),
			Platforms: []rpaPlatformInfo{},
		}
	}

	var itemsMap rpaItemsMap
	if err := json.Unmarshal(raw, &itemsMap); err != nil {
		log.Printf("[rpa_scan] 解析 items_map.json 失败: %v", err)
		return &rpaScanResult{
			ScannedAt: time.Now().Format("2006-01-02T15:04:05"),
			Platforms: []rpaPlatformInfo{},
		}
	}

	// 并行扫描每个平台
	// 保持平台顺序：先收集 key 列表
	platforms := make([]string, 0, len(itemsMap))
	for p := range itemsMap {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)

	results := make([]rpaPlatformInfo, len(platforms))
	var wg sync.WaitGroup

	for idx, platform := range platforms {
		wg.Add(1)
		go func(idx int, platform string, dataItems map[string]struct {
			Stores []string `json:"stores"`
		}) {
			defer wg.Done()
			results[idx] = scanPlatform(platform, dataItems)
		}(idx, platform, itemsMap[platform])
	}

	wg.Wait()

	// 计算平台整体完成度
	for i := range results {
		total := 0
		done := 0
		for _, d := range results[i].Dates {
			for _, s := range d.Stores {
				total += len(s.CompletedItems) + len(s.MissingItems)
				done += len(s.CompletedItems)
			}
		}
		if total > 0 {
			results[i].Completeness = float64(done) / float64(total)
		} else {
			results[i].Completeness = 0
		}
		results[i].Status = rpaStatus(results[i].Completeness)
	}

	return &rpaScanResult{
		ScannedAt: time.Now().Format("2006-01-02T15:04:05"),
		Platforms: results,
	}
}

func scanPlatform(platform string, dataItems map[string]struct {
	Stores []string `json:"stores"`
}) rpaPlatformInfo {

	yearDir := filepath.Join(rpaBaseDir, platform, "2026")

	// 列出 2026/ 下所有子目录，筛选出 YYYYMMDD 格式
	entries, err := os.ReadDir(yearDir)
	if err != nil {
		return rpaPlatformInfo{
			Name:   platform,
			Status: "missing",
			Dates:  []rpaDateInfo{},
		}
	}

	var dateList []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if yyyymmddRe.MatchString(name) {
			dateList = append(dateList, name)
		}
	}

	// 按日期降序
	sort.Sort(sort.Reverse(sort.StringSlice(dateList)))

	// 补全缺失日期: 让 2026-01-01 起到 T-1 之间, Z 盘没目录的日期都显示出来 (status=no_dir),
	// 用户能看到完整缺失情况 + 点同步按钮触发影刀重跑.
	// 终点取昨天 T-1: 今天 RPA 还没跑完, 不要把今日标"无目录"造成误报.
	existing := map[string]bool{}
	for _, d := range dateList {
		existing[d] = true
	}
	startDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	endDate := time.Now().AddDate(0, 0, -1)
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		key := d.Format("20060102")
		if !existing[key] {
			dateList = append(dateList, key)
			existing[key] = true
		}
	}
	// 重新降序
	sort.Sort(sort.Reverse(sort.StringSlice(dateList)))

	// 收集所有 store->items 映射（各 dataItem 下的 store 列表合并）
	// storeItems: store -> []dataItem
	storeItems := map[string][]string{}
	for dataItem, v := range dataItems {
		for _, store := range v.Stores {
			storeItems[store] = append(storeItems[store], dataItem)
		}
	}

	// 按 store 名排序，保持输出稳定
	storeNames := make([]string, 0, len(storeItems))
	for s := range storeItems {
		storeNames = append(storeNames, s)
	}
	sort.Strings(storeNames)

	// 对每个数据项排序，保持稳定输出
	for s := range storeItems {
		sort.Strings(storeItems[s])
	}

	dateInfos := make([]rpaDateInfo, 0, len(dateList))
	for _, date := range dateList {
		formattedDate := date[:4] + "-" + date[4:6] + "-" + date[6:]
		dateDir := filepath.Join(yearDir, date)

		// Z 盘根本没这个日期目录 → 直接构造 no_dir dateInfo (所有 store 全 missing)
		if _, err := os.Stat(dateDir); os.IsNotExist(err) {
			storeInfos := make([]rpaStoreInfo, 0, len(storeNames))
			for _, store := range storeNames {
				items := storeItems[store]
				missing := append([]string(nil), items...)
				if missing == nil {
					missing = []string{}
				}
				storeInfos = append(storeInfos, rpaStoreInfo{
					Name:           store,
					Completeness:   0,
					Status:         "no_dir",
					CompletedItems: []string{},
					MissingItems:   missing,
				})
			}
			dateInfos = append(dateInfos, rpaDateInfo{
				Date:          date,
				FormattedDate: formattedDate,
				Completeness:  0,
				Status:        "no_dir",
				Stores:        storeInfos,
			})
			continue
		}

		storeInfos := make([]rpaStoreInfo, 0, len(storeNames))

		// dateDir 下的文件 (飞瓜等平台文件直接放日期根目录) — 提到 store 循环外只读 1 次
		// (Z 盘是 SMB 网盘, IO 慢, 每个 store 重复读会 N+1)
		var dateRootFiles []string
		if dirEntries, err := os.ReadDir(dateDir); err == nil {
			for _, fe := range dirEntries {
				if !fe.IsDir() {
					dateRootFiles = append(dateRootFiles, fe.Name())
				}
			}
		}

		for _, store := range storeNames {
			items := storeItems[store]
			storeDir := filepath.Join(yearDir, date, store)

			// 读取该 store 目录下的文件名列表 + 复用 dateRootFiles
			fileNames := append([]string(nil), dateRootFiles...)
			if dirEntries, err := os.ReadDir(storeDir); err == nil {
				for _, fe := range dirEntries {
					if !fe.IsDir() {
						fileNames = append(fileNames, fe.Name())
					}
				}
			}

			var completed, missing []string
			for _, item := range items {
				prefix := platform + "_" + date + "_" + store + "_" + item
				found := false
				for _, fn := range fileNames {
					if strings.HasPrefix(fn, prefix) &&
						(strings.HasSuffix(fn, ".xlsx") || strings.HasSuffix(fn, ".json")) {
						found = true
						break
					}
				}
				if found {
					completed = append(completed, item)
				} else {
					missing = append(missing, item)
				}
			}
			if completed == nil {
				completed = []string{}
			}
			if missing == nil {
				missing = []string{}
			}

			total := len(items)
			var completeness float64
			if total > 0 {
				completeness = float64(len(completed)) / float64(total)
			}

			storeInfos = append(storeInfos, rpaStoreInfo{
				Name:           store,
				Completeness:   completeness,
				Status:         rpaStatus(completeness),
				CompletedItems: completed,
				MissingItems:   missing,
			})
		}

		// 计算当天完成度
		dayTotal := 0
		dayDone := 0
		for _, s := range storeInfos {
			dayTotal += len(s.CompletedItems) + len(s.MissingItems)
			dayDone += len(s.CompletedItems)
		}
		var dayCompleteness float64
		if dayTotal > 0 {
			dayCompleteness = float64(dayDone) / float64(dayTotal)
		}

		dateInfos = append(dateInfos, rpaDateInfo{
			Date:          date,
			FormattedDate: formattedDate,
			Completeness:  dayCompleteness,
			Status:        rpaStatus(dayCompleteness),
			Stores:        storeInfos,
		})
	}

	return rpaPlatformInfo{
		Name:  platform,
		Dates: dateInfos,
	}
}

// -------- 缓存管理 --------

func getRPAScanCached() *rpaScanResult {
	rpaScanMu.RLock()
	if rpaScanCache != nil && time.Since(rpaScanCachedAt) < rpaScanCacheTTL {
		result := rpaScanCache
		rpaScanMu.RUnlock()
		return result
	}
	rpaScanMu.RUnlock()

	// 需要刷新
	rpaScanMu.Lock()
	defer rpaScanMu.Unlock()
	// 双重检查
	if rpaScanCache != nil && time.Since(rpaScanCachedAt) < rpaScanCacheTTL {
		return rpaScanCache
	}
	rpaScanCache = doRPAScan()
	rpaScanCachedAt = time.Now()
	return rpaScanCache
}

func clearRPAScanCache() {
	rpaScanMu.Lock()
	defer rpaScanMu.Unlock()
	rpaScanCache = nil
	rpaScanCachedAt = time.Time{}
}

// StartRPAScanTicker 后台每 5 分钟主动刷一次 RPA 文件扫描缓存
// 让前端 RPAMonitor 页面打开瞬开, 不再卡在缓存过期触发的全盘重扫
func StartRPAScanTicker() {
	log.Println("[rpa_scan] 后台扫描 ticker 启动 (每 5 分钟主动刷缓存)")
	refreshRPAScanBackground()
	ticker := time.NewTicker(rpaScanCacheTTL)
	defer ticker.Stop()
	for range ticker.C {
		refreshRPAScanBackground()
	}
}

// refreshRPAScanBackground 后台跑一次文件扫描并更新缓存, panic 被 recover 防止整进程崩
func refreshRPAScanBackground() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[rpa_scan] 后台扫描 panic recovered: %v", r)
		}
	}()
	start := time.Now()
	result := doRPAScan()
	rpaScanMu.Lock()
	rpaScanCache = result
	rpaScanCachedAt = time.Now()
	rpaScanMu.Unlock()
	log.Printf("[rpa_scan] 后台扫描完成 耗时=%v 平台数=%d", time.Since(start), len(result.Platforms))
}

// -------- HTTP 处理器 --------

// ScanRPAFiles GET /api/admin/rpa-scan
// 返回 RPA 文件完整性扫描结果（5分钟缓存）
// 平台 → 用于检查导入状态的代表性表 (任一表此日期有数据 = "已导入")
// 天猫超市早期 RPA 没"店铺"sheet 但有广告/无界等 sheet, 只看 shop 会误判"未导入"
//
// 约定: 第一张表是"主表"(shop 类), 用它的 MIN(stat_date) 算业务起点 (earliestBiz),
// 早于此的 RPA 文件夹日期不展示. 其他表只用于 OR 判断"已导入".
// 不用 MIN-of-all 因为 campaign 等表常有 fallback 入库的"假"早期数据 (RPA 文件夹日期 fallback)
var platformDBTables = map[string][]string{
	"天猫": {"op_tmall_shop_daily"},
	"天猫超市": {
		"op_tmall_cs_shop_daily",
		"op_tmall_cs_campaign_daily",
		"op_tmall_cs_goods_daily",
		"op_tmall_cs_wujie_scene_daily",
		"op_tmall_cs_wujie_detail_daily",
		"op_tmall_cs_smart_plan_daily",
		"op_tmall_cs_taoke_daily",
	},
	"京东":   {"op_jd_shop_daily"},
	"京东自营": {"op_jd_cs_workload_daily"},
	"拼多多":  {"op_pdd_shop_daily"},
	"唯品会":  {"op_vip_shop_daily"},
	"抖音":   {"op_douyin_goods_daily"},
	"抖音分销": {"op_douyin_dist_product_daily"},
	"快手":   {"op_kuaishou_cs_assessment_daily"},
	"小红书":  {"op_xhs_cs_analysis_daily", "op_xhs_note_daily", "op_xhs_goods_daily"},
	"小红书乘风": {"op_xhs_chengfeng_daily"},
	"巨量云图": {"op_juliang_talent_daily", "op_juliang_keyword_daily"},
	"小红书灵犀": {"op_lingxi_search_trend", "op_lingxi_search_updown", "op_lingxi_search_rank"},
	"飞瓜":   {"fg_creator_daily"},
}

// rpaSealCutoff 文件封存截止日: platform -> yyyy-MM-dd, 此日"之前"的日期标"封存(历史)"
// 用途: 某平台老数据平台侧已拿不到、原始文件没了, 但数据本身早已全部入库(已核实) →
//   监控别再按"文件在不在"把这段报成异常, 标灰封存, 不算异常 / 不进完整率 / "只看异常"不显示。
// 加封存只对"数据已确认入库齐全"的历史段用; 新增平台直接加一行 platform: "日期" 即可。
// 案例: 拼多多 1/1~2/24 销售/商品文件平台拿不到了, 但库里 56 天数据 100% 齐, 故封存到 2026-02-25。
var rpaSealCutoff = map[string]string{
	"拼多多": "2026-02-25",
}

// rpaSealRanges 文件封存区间: platform -> [[start,end]] (闭区间, yyyy-MM-dd)。
// 用途同 rpaSealCutoff, 但针对"中间段"缺失(前后都有正常数据), 单个截止日表达不了。
// 案例: 抖音商品数据春节 2/14~2/19 这6天店铺没产出(正常历史现象), 前后数据正常 → 只封这段, 不误伤年初。
var rpaSealRanges = map[string][][2]string{
	"抖音": {{"2026-02-14", "2026-02-19"}},
}

// isRPASealed 某平台某日期是否封存(历史): 落在截止日之前, 或落在某封存区间(闭区间)内。
func isRPASealed(platform, date string) bool {
	if cutoff, ok := rpaSealCutoff[platform]; ok && date < cutoff {
		return true
	}
	for _, r := range rpaSealRanges[platform] {
		if date >= r[0] && date <= r[1] {
			return true
		}
	}
	return false
}

func (h *DashboardHandler) enrichDBStatus(result *rpaScanResult) {
	for i := range result.Platforms {
		p := &result.Platforms[i]
		tables, ok := platformDBTables[p.Name]
		if !ok {
			continue
		}
		// 任一表此日期有数据 = 已导入 (OR 取并集), 同时记录 earliest 业务日期 (取所有表 MIN 的最早)
		importedDates := map[string]bool{}
		var earliestBiz string
		for _, tableName := range tables {
			rows, err := h.DB.Query("SELECT DISTINCT DATE_FORMAT(stat_date,'%Y-%m-%d') FROM " + tableName + " WHERE stat_date >= '2026-01-01'")
			if err != nil {
				continue
			}
			for rows.Next() {
				var d string
				rows.Scan(&d)
				importedDates[d] = true
				if earliestBiz == "" || d < earliestBiz {
					earliestBiz = d
				}
			}
			rows.Close()
		}
		// 过滤掉早于业务起点的日期 (RPA 第一天前的数据拉不到, 这些 RPA 文件夹的"日期"是
		// 抓取日期, 不是业务日期, 展示出来误导跑哥. 见 feedback_multi_day_excel)
		// 例外: 把"紧挨业务起点、连续的空目录"(status=missing)也纳入展示 —— 这是 RPA 刚建了
		// 文件夹却还没传数据的真实异常 (如乘风 6-21 空目录紧挨 6-22 起点), 藏掉跑哥就发现不了缺数据。
		// 往起点前逐天回溯, 一旦遇到"没目录(no_dir)"或"有文件的旧目录(complete/partial, 抓取日噪音)"
		// 就停, 避免把小红书/抖音分销 1月起的旧目录全炸出来。
		if earliestBiz != "" {
			statusByDate := make(map[string]string, len(p.Dates))
			for _, dt := range p.Dates {
				statusByDate[dt.FormattedDate] = dt.Status
			}
			floor := earliestBiz
			if t, err := time.Parse("2006-01-02", floor); err == nil {
				for {
					t = t.AddDate(0, 0, -1)
					prev := t.Format("2006-01-02")
					if statusByDate[prev] == "missing" {
						floor = prev
					} else {
						break
					}
				}
			}
			filtered := make([]rpaDateInfo, 0, len(p.Dates))
			for _, dt := range p.Dates {
				if dt.FormattedDate >= floor {
					filtered = append(filtered, dt)
				}
			}
			p.Dates = filtered
		}
		// 二元补充: rpa_import_history 里有 success = 该文件夹被 import 处理过 = 已导入
		// 修 T+1 业务日 lag 误报"未导入": RPA 文件夹日期 ≠ Excel 业务日, 老 stat_date 严格匹配
		// 在 T+1 平台 (京东等) 永远查不到. 见 feedback_rpa_monitor_t1_lag.
		histRows, herr := h.DB.Query(`SELECT DISTINCT DATE_FORMAT(folder_date, '%Y-%m-%d') FROM rpa_import_history WHERE platform = ? AND status = 'success' AND folder_date >= '2026-01-01'`, p.Name)
		if herr == nil {
			for histRows.Next() {
				var d string
				histRows.Scan(&d)
				importedDates[d] = true
			}
			histRows.Close()
		}
		for j := range p.Dates {
			p.Dates[j].DBImported = importedDates[p.Dates[j].FormattedDate]
		}
		// 文件封存: 截止日之前 或 落在封存区间内的日期标 sealed (平台老文件拿不到/历史不产出但已确认), 不算异常 / 不进完整率
		for j := range p.Dates {
			if isRPASealed(p.Name, p.Dates[j].FormattedDate) {
				p.Dates[j].Status = "sealed"
				for k := range p.Dates[j].Stores {
					p.Dates[j].Stores[k].Status = "sealed"
				}
			}
		}
		// 重算平台完整率: 按 (店,天) 单元里 status='complete' 的比例 (封存日期排除在分母外)
		// 跟前端店铺级 badge 算法一致 (RPAMonitor.tsx storeCompleteness),
		// 单店时平台率 == 店铺率, 不会出现 26% vs 95% 错位
		totalCells, completeCells := 0, 0
		for _, d := range p.Dates {
			if d.Status == "sealed" {
				continue
			}
			for _, s := range d.Stores {
				totalCells++
				if s.Status == "complete" {
					completeCells++
				}
			}
		}
		if totalCells > 0 {
			p.Completeness = float64(completeCells) / float64(totalCells)
		} else {
			p.Completeness = 0
		}
		p.Status = rpaStatus(p.Completeness)
	}
}

func (h *DashboardHandler) ScanRPAFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	result := getRPAScanCached()
	h.enrichDBStatus(result)
	writeJSON(w, result)
}

// RefreshRPAScan POST /api/admin/rpa-scan/refresh
func (h *DashboardHandler) RefreshRPAScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	clearRPAScanCache()
	result := getRPAScanCached()
	h.enrichDBStatus(result)
	writeJSON(w, result)
}
