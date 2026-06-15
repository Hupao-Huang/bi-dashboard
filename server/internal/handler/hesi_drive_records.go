package handler

// 合思私车公用"行车记录"业务对象 (跑哥 2026-06-12: 详情里要体现行车记录)
// 明细 feeTypeForm.u_行车记录 存的是实例 ID, 本体在 用车补贴平台(SCGY:corpId) 的
// 行车记录 entity (d2136e97ce9be84087c0)。SCGY 扩展域不开放按 ID 查询,
// 只能走通用 v2.1 datalink 按 entity 全量分页拉 (实查 634 条) → 进程内缓存 15min。

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"time"
)

const hesiDriveRecordEntityID = "d2136e97ce9be84087c0"

// DriveRecord 行车记录 (给详情接口输出)
type DriveRecord struct {
	Departure   string `json:"departure"`   // 出发地地址
	Destination string `json:"destination"` // 目的地地址
	Waypoints   int    `json:"waypoints"`   // 途经地个数
	Mileage     string `json:"mileage"`     // 实际里程 km
	Standard    string `json:"standard"`    // 补助标准 元/km
	Subsidy     string `json:"subsidy"`     // 补助金额 元
	StartTime   int64  `json:"startTime"`   // 起始时间 ms
	EndTime     int64  `json:"endTime"`     // 结束时间 ms
}

var (
	hesiDriveRecCache      map[string]*DriveRecord
	hesiDriveRecCacheAt    time.Time
	hesiDriveRecRefreshing bool // 是否有后台重拉在途 (防并发重复全量拉)
	hesiDriveRecMu         sync.Mutex
)

// LookupDriveRecord 行车记录实例 ID → 解析后的记录。
// 非阻塞 (跑哥 2026-06-15 修): 缓存冷/过期只触发【后台异步】重拉, 当场返回已有缓存(可能略旧)
// 或 nil, 绝不在请求路径里同步全量拉合思 (634 条 / 7 页 API, 每页超时 30s)。
// 旧实现把全量拉放在锁内 + 请求路径, 重启后冷缓存撞上 = 合思审批列表加载卡几分钟 (樊雪娇 35 单案例)。
// 返回 nil 时规则 12-1 降级"人工核"(安全兜底), 后台刷完下次自动恢复自动对账。
func (h *DashboardHandler) LookupDriveRecord(id string) *DriveRecord {
	if id == "" {
		return nil
	}
	hesiDriveRecMu.Lock()
	cache := hesiDriveRecCache
	stale := cache == nil || time.Since(hesiDriveRecCacheAt) >= 15*time.Minute
	hesiDriveRecMu.Unlock()
	if stale {
		h.triggerDriveRecRefresh()
	}
	if cache == nil {
		return nil
	}
	return cache[id]
}

// triggerDriveRecRefresh 若无在途重拉, 起一个后台 goroutine 全量重拉行车记录 (非阻塞)。
func (h *DashboardHandler) triggerDriveRecRefresh() {
	hesiDriveRecMu.Lock()
	if hesiDriveRecRefreshing {
		hesiDriveRecMu.Unlock()
		return
	}
	hesiDriveRecRefreshing = true
	hesiDriveRecMu.Unlock()
	go h.refreshDriveRecordsAsync()
}

// refreshDriveRecordsAsync 后台全量重拉行车记录并换缓存 (不持锁拉, 只在换缓存瞬间持锁)。
func (h *DashboardHandler) refreshDriveRecordsAsync() {
	m, err := h.fetchAllDriveRecords()
	hesiDriveRecMu.Lock()
	defer hesiDriveRecMu.Unlock()
	hesiDriveRecRefreshing = false
	if err != nil {
		log.Printf("[hesi-drive] 后台拉行车记录失败: %v", err)
		return
	}
	hesiDriveRecCache = m
	hesiDriveRecCacheAt = time.Now()
}

// StartDriveRecordPrewarm 启动即预热 + 每 10 分钟后台刷新行车记录缓存,
// 让合思审批列表/详情永远命中暖缓存 (15min TTL 内), 把"重启后冷缓存现拉"窗口降到最小。
func (h *DashboardHandler) StartDriveRecordPrewarm() {
	h.triggerDriveRecRefresh()
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for range t.C {
		h.triggerDriveRecRefresh()
	}
}

// fetchAllDriveRecords 分页拉行车记录全量 (v2.1 datalink, 每页上限 100)
func (h *DashboardHandler) fetchAllDriveRecords() (map[string]*DriveRecord, error) {
	token, err := h.getHesiToken()
	if err != nil {
		return nil, err
	}
	out := map[string]*DriveRecord{}
	for start := 0; ; start += 100 {
		// 不带 active 参数 = 返回全部 (含已被单据引用的"停用"记录 — 被报销单挂上的行车记录
		// 恰恰是 active=false, 带 active=true 会全拉不到, 6/12 实测踩坑)
		url := fmt.Sprintf("%s/api/openapi/v2.1/datalink?accessToken=%s&entityId=%s&start=%d&count=100",
			hesiAPIBase, token, hesiDriveRecordEntityID, start)
		resp, err := hesiHTTP.Get(url)
		if err != nil {
			return nil, err
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			snip := string(data)
			if len(snip) > 200 {
				snip = snip[:200]
			}
			return nil, fmt.Errorf("合思返回 HTTP %d: %s", resp.StatusCode, snip)
		}
		var parsed struct {
			Count int `json:"count"`
			Items []struct {
				ID   string                 `json:"id"`
				Form map[string]interface{} `json:"form"`
			} `json:"items"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("解析行车记录失败: %w", err)
		}
		for _, it := range parsed.Items {
			if it.ID == "" || it.Form == nil {
				continue
			}
			out[it.ID] = parseDriveRecord(it.Form)
		}
		if len(parsed.Items) < 100 || start+100 >= parsed.Count {
			break
		}
	}
	log.Printf("[hesi-drive] 行车记录缓存刷新: %d 条", len(out))
	return out, nil
}

// parseDriveRecord 从 datalink form (E_<entity>_xxx 扁平字段) 解析展示字段
func parseDriveRecord(form map[string]interface{}) *DriveRecord {
	pre := "E_" + hesiDriveRecordEntityID + "_"
	rec := &DriveRecord{}
	if loc, ok := form[pre+"出发地"].(map[string]interface{}); ok {
		rec.Departure, _ = loc["address"].(string)
	}
	if loc, ok := form[pre+"目的地"].(map[string]interface{}); ok {
		rec.Destination, _ = loc["address"].(string)
	}
	if wps, ok := form[pre+"途经地"].([]interface{}); ok {
		rec.Waypoints = len(wps)
	}
	rec.Mileage, _ = form[pre+"实际里程"].(string)
	if rec.Mileage == "" {
		rec.Mileage, _ = form[pre+"行驶总里程"].(string)
	}
	if v, ok := getStandardAmount(form[pre+"补助标准"]); ok {
		rec.Standard = fmt.Sprintf("%.2f", v)
	}
	if v, ok := getStandardAmount(form[pre+"补助金额"]); ok {
		rec.Subsidy = fmt.Sprintf("%.2f", v)
	}
	if t, ok := form[pre+"起始时间"].(float64); ok {
		rec.StartTime = int64(t)
	}
	if t, ok := form[pre+"结束时间"].(float64); ok {
		rec.EndTime = int64(t)
	}
	return rec
}
