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
	type platformResult struct {
		idx  int
		info rpaPlatformInfo
	}

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
		storeInfos := make([]rpaStoreInfo, 0, len(storeNames))

		for _, store := range storeNames {
			items := storeItems[store]
			storeDir := filepath.Join(yearDir, date, store)
			dateDir := filepath.Join(yearDir, date)

			// 读取该 store 目录下的文件名列表
			// 同时检查日期根目录（飞瓜等平台文件可能直接放在日期目录下）
			var fileNames []string
			if dirEntries, err := os.ReadDir(storeDir); err == nil {
				for _, fe := range dirEntries {
					if !fe.IsDir() {
						fileNames = append(fileNames, fe.Name())
					}
				}
			}
			if dirEntries, err := os.ReadDir(dateDir); err == nil {
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

// -------- HTTP 处理器 --------

// ScanRPAFiles GET /api/admin/rpa-scan
// 返回 RPA 文件完整性扫描结果（5分钟缓存）
// 平台 → 用于检查导入状态的代表性表名
var platformDBTable = map[string]string{
	"天猫":   "op_tmall_shop_daily",
	"天猫超市": "op_tmall_cs_shop_daily",
	"京东":   "op_jd_shop_daily",
	"京东自营": "op_jd_cs_workload_daily",
	"拼多多":  "op_pdd_shop_daily",
	"唯品会":  "op_vip_shop_daily",
	"抖音":   "op_douyin_goods_daily",
	"抖音分销": "op_douyin_dist_product_daily",
	"快手":   "op_kuaishou_cs_assessment_daily",
	"小红书":  "op_xhs_cs_analysis_daily",
	"飞瓜":   "fg_creator_daily",
}

func (h *DashboardHandler) enrichDBStatus(result *rpaScanResult) {
	for i := range result.Platforms {
		p := &result.Platforms[i]
		tableName, ok := platformDBTable[p.Name]
		if !ok {
			continue
		}

		// 一次查出该表所有有数据的日期
		dateCol := "stat_date"
		rows, err := h.DB.Query("SELECT DISTINCT DATE_FORMAT("+dateCol+",'%Y-%m-%d') FROM "+tableName+" WHERE "+dateCol+" >= '2026-01-01'")
		if err != nil {
			continue
		}
		importedDates := map[string]bool{}
		for rows.Next() {
			var d string
			rows.Scan(&d)
			importedDates[d] = true
		}
		rows.Close()

		for j := range p.Dates {
			// 转换日期格式 20260416 → 2026-04-16
			formatted := p.Dates[j].FormattedDate
			p.Dates[j].DBImported = importedDates[formatted]
		}
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
