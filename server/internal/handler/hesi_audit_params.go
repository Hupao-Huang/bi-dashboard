package handler

// hesi_audit_params.go - 审批规则引擎的标准/配置/各类缓存查询
// (2026-06-25 从 hesi_audit_rules.go 拆出, 纯挪位置不改逻辑)

import (
	"database/sql"
	"encoding/json"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// dailyExpenseSpecPrefix 日常报销单单据模板 spec 前缀 (樊雪娇规则集仅适用此模板; 详情页公司判定也按此 gate)
const dailyExpenseSpecPrefix = "ID01Fk3qJYYFvp"

// 业务招待费 fee_type ID 集合 (合思 feeTypes API 2026-05-27 递归 descendants 实查 4 项)
// 父类 ID01Fk0FsIqgQn = "业务招待费"
var businessTreatFeeTypes = map[string]string{
	"ID01Fk0FsIqgQn": "业务招待费",
	"ID01Fk0IC65Hhd": "招待费",
	"ID01Fk0IC65HxJ": "礼品",
	"ID01KuC6LQ4oPR": "总经办客情",
	// 修正 v1.75.15: ID01MBFnQppBWn 实际是"业务宣传费" (parent=广告宣传费用), 不属于招待费, 移除
}

// 招待费用申请单 specification_id 前缀 (合思 2026-05-27 实查)
const reqTreatmentSpecPrefix = "ID01FAvUKAThbV"

// 固定资产 fee_type ID (合思 2026-05-27 实查, 父类无子类)
var fixedAssetFeeTypes = map[string]string{
	"ID01FFN9mLHtrp": "固定资产",
}

// 固定资产申请单 specification_id 前缀
const reqFixedAssetSpecPrefix = "ID01FFO5f39taD"

// 交通及差旅费 fee_type ID 集合 (合思 feeTypes API 2026-05-27 递归 descendants 实查)
// 父类 ID01Fk0sq1ya5x = "交通及差旅费", 含 15 项 (交通费/住宿费/补贴/各交通工具/过路停车)
var travelExpenseFeeTypes = map[string]string{
	"ID01Fk0sq1ya5x": "交通及差旅费",
	"ID01Fk0STRw38z": "交通费",
	"ID01Fk0STRw3p5": "住宿费",
	"ID01Fk0MQBAAQ7": "补贴",
	"ID01Fk0MQBAB6D": "市内补贴",
	"ID01FkA9pR8zGT": "餐费补贴",
	"ID01Fk0IZFCaJx": "飞机",
	"ID01Fk0IZFCb03": "火车",
	"ID01Fk0IZFCbgz": "客车",
	"ID01Fk0IZFCbx5": "其他交通",
	"ID01Fr2mX8KP2T": "私车公用",
	"ID01KhLSijR88T": "汽车",
	"ID01KhLSijR8FV": "过路费",
	"ID01KhLSijR8Wr": "停车费",
	"ID01KhLSijR8pp": "地铁",
}

// 出差申请单 specification_id 前缀 (合思预置, corp_prefix=ID01FfMgoeP7cz)
const reqTripSpecPrefix = "ID01FfMgoeP7cz:PRESET_REQUISITION_TRIP"

// 住宿标准矩阵 (¥/晚, PDF V7.0 2026-01-23)
// 城市分级 × 职级, 同住按职位高者标准上浮 20%
var accommodationStandard = map[string]map[string]float64{
	"总裁":    {"一线": 1200, "新一线": 1000, "二线": 1000, "其他": 800},
	"副总裁":   {"一线": 1000, "新一线": 800, "二线": 800, "其他": 600},
	"集团总监":  {"一线": 500, "新一线": 400, "二线": 400, "其他": 300},
	"集团经理":  {"一线": 450, "新一线": 350, "二线": 350, "其他": 300},
	"主管和其他": {"一线": 400, "新一线": 300, "二线": 300, "其他": 300},
}

// accommodationStandardOffline 线下(世创/世用)住宿标准矩阵 (¥/晚, 线下 PDF 2.2; 樊雪娇 2026-06-17)
// 线下只有 一线/二线/国内其他 3 档城市 + 其他员工/大区经理及以上/集团总监 3 档职级,
// 跟集团那套 (4 城市档×5 职级) 结构不同; 同住上浮 20% 与集团一致。
var accommodationStandardOffline = map[string]map[string]float64{
	"集团总监":    {"一线": 500, "二线": 400, "国内其他": 300},
	"大区经理及以上": {"一线": 450, "二线": 350, "国内其他": 280},
	"其他员工":    {"一线": 350, "二线": 280, "国内其他": 230},
}

// 出差补贴标准 (¥/天, 规则 11)
// PDF V7.0 + 跑哥口述: 总裁 200 / 副总裁 150 / 集团总监 100 / 集团经理 80 / 主管及以下 60
var subsidyStandard = map[string]float64{
	"总裁":    200,
	"副总裁":   150,
	"集团总监":  100,
	"集团经理":  80,
	"主管和其他": 60,
}

// 私车公用 fee_type (规则 12-1 自驾报销)
const driveFeeTypeID = "ID01Fr2mX8KP2T"

// offlineHotelLevel 把花名册职级映射到线下住宿 3 档 (樊雪娇 2026-06-17, 同线下补贴口径延伸)
// 集团总监/副总裁/总裁 → "集团总监"; 集团经理(大区经理归此档) → "大区经理及以上"; 其余 → "其他员工"
func offlineHotelLevel(position string) string {
	switch position {
	case "集团总监", "副总裁", "总裁":
		return "集团总监"
	case "集团经理":
		return "大区经理及以上"
	default:
		return "其他员工"
	}
}

// offlineCityTier 把集团 4 档城市分级映射到线下 3 档 (新一线→二线, 跑哥 2026-06-17)
// extractTier 只会返回 一线/新一线/二线/其他, 这里收口到线下的 一线/二线/国内其他
func offlineCityTier(tier string) string {
	switch tier {
	case "一线":
		return "一线"
	case "新一线", "二线":
		return "二线"
	default: // "其他" 及兜底
		return "国内其他"
	}
}

// ====== 审批业务参数 DB 配置化 (2026-06-12 第三批) ======
// 住宿矩阵/补贴标准是财务最常调的两组参数, 搬进 hesi_audit_param 表后改口径不用改代码:
//
//	UPDATE hesi_audit_param SET param_json='{"总裁":220,...}' WHERE param_key='subsidy_standard';
//
// 5 分钟内自动生效 (loader TTL); 表里没有/JSON 解析失败时回落到上面的代码默认值, 永不空转
var (
	hesiAuditParamOnce sync.Once
	hesiAuditParamMu   sync.Mutex
	accomStdCache      map[string]map[string]float64
	subsidyStdCache    map[string]float64
	hesiAuditParamAt   time.Time
)

func (h *DashboardHandler) ensureHesiAuditParamTable() {
	if h.DB == nil {
		return
	}
	hesiAuditParamOnce.Do(func() {
		if _, err := h.DB.Exec(`CREATE TABLE IF NOT EXISTS hesi_audit_param (
			param_key VARCHAR(64) PRIMARY KEY COMMENT '参数名',
			param_json TEXT NOT NULL COMMENT '参数值(JSON)',
			remark VARCHAR(255) DEFAULT '' COMMENT '说明',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间'
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='合思审批规则业务参数(财务改口径直接改param_json, 5分钟生效, 不用改代码)'`); err != nil {
			log.Printf("[hesi-audit] 建参数表失败: %v", err)
			return
		}
		seed := func(key string, v interface{}, remark string) {
			b, err := json.Marshal(v)
			if err != nil {
				return
			}
			if _, err := h.DB.Exec(`INSERT IGNORE INTO hesi_audit_param (param_key, param_json, remark) VALUES (?,?,?)`, key, string(b), remark); err != nil {
				log.Printf("[hesi-audit] 参数种子 %s 失败: %v", key, err)
			}
		}
		seed("accommodation_standard", accommodationStandard, "住宿费单晚标准: 职级×城市分级 (规则 7-3)")
		seed("accommodation_standard_offline", accommodationStandardOffline, "线下(世创/世用)住宿费单晚标准: 3档职级×3档城市 (规则 7-3 线下)")
		seed("subsidy_standard", subsidyStandard, "出差补贴标准 ¥/天: 职级 (规则 11)")
	})
}

// loadHesiAuditParams 返回 (住宿矩阵, 补贴标准), DB 配置优先, 失败回落代码默认值
func (h *DashboardHandler) loadHesiAuditParams() (map[string]map[string]float64, map[string]float64) {
	if h.DB == nil { // 单测裸 handler 直接用代码默认值
		return accommodationStandard, subsidyStandard
	}
	h.ensureHesiAuditParamTable()
	hesiAuditParamMu.Lock()
	defer hesiAuditParamMu.Unlock()
	if accomStdCache != nil && time.Since(hesiAuditParamAt) < 5*time.Minute {
		return accomStdCache, subsidyStdCache
	}
	accom, subsidy := accommodationStandard, subsidyStandard
	rows, err := h.DB.Query(`SELECT param_key, param_json FROM hesi_audit_param WHERE param_key IN ('accommodation_standard','subsidy_standard')`)
	if err != nil {
		log.Printf("[hesi-audit] 参数表读取失败, 用代码默认值: %v", err)
		return accom, subsidy
	}
	defer rows.Close()
	for rows.Next() {
		var k, j string
		if rows.Scan(&k, &j) != nil {
			continue
		}
		// 合并语义: DB 配置按职级档覆盖, 没写的档用代码默认值补齐 —
		// 防财务手填 JSON 少一档时, 该档从"有标准"静默变"未配置"(二审抓的风险)
		switch k {
		case "accommodation_standard":
			var m map[string]map[string]float64
			if e := json.Unmarshal([]byte(j), &m); e == nil && len(m) > 0 {
				merged := map[string]map[string]float64{}
				for pos, tiers := range accommodationStandard {
					merged[pos] = tiers
				}
				for pos, tiers := range m {
					merged[pos] = tiers
				}
				accom = merged
			} else {
				log.Printf("[hesi-audit] accommodation_standard JSON 异常, 用代码默认值: %v", e)
			}
		case "subsidy_standard":
			var m map[string]float64
			if e := json.Unmarshal([]byte(j), &m); e == nil && len(m) > 0 {
				merged := map[string]float64{}
				for pos, v := range subsidyStandard {
					merged[pos] = v
				}
				for pos, v := range m {
					merged[pos] = v
				}
				subsidy = merged
			} else {
				log.Printf("[hesi-audit] subsidy_standard JSON 异常, 用代码默认值: %v", e)
			}
		}
	}
	if err := rows.Err(); err != nil {
		// 读取中断的残缺结果不落缓存 (跟部门树缓存同教训), 本次用默认值
		log.Printf("[hesi-audit] 参数表读取中断, 用代码默认值: %v", err)
		return accommodationStandard, subsidyStandard
	}
	accomStdCache, subsidyStdCache = accom, subsidy
	hesiAuditParamAt = time.Now()
	return accom, subsidy
}

// 线下住宿标准缓存 (独立 5min TTL, 不与集团标准共缓存)
var (
	accomOfflineStdCache map[string]map[string]float64
	accomOfflineAt       time.Time
)

// loadOfflineAccomStd 返回线下(世创/世用)住宿矩阵, DB 配置优先(accommodation_standard_offline),
// 缺档用代码默认值补齐, 读不到/解析失败回落代码默认值 — 跟 loadHesiAuditParams 同套路。
func (h *DashboardHandler) loadOfflineAccomStd() map[string]map[string]float64 {
	if h.DB == nil {
		return accommodationStandardOffline
	}
	h.ensureHesiAuditParamTable()
	hesiAuditParamMu.Lock()
	defer hesiAuditParamMu.Unlock()
	if accomOfflineStdCache != nil && time.Since(accomOfflineAt) < 5*time.Minute {
		return accomOfflineStdCache
	}
	accom := accommodationStandardOffline
	var j string
	if err := h.DB.QueryRow(`SELECT param_json FROM hesi_audit_param WHERE param_key='accommodation_standard_offline' LIMIT 1`).Scan(&j); err == nil {
		var m map[string]map[string]float64
		if e := json.Unmarshal([]byte(j), &m); e == nil && len(m) > 0 {
			merged := map[string]map[string]float64{}
			for pos, tiers := range accommodationStandardOffline {
				merged[pos] = tiers
			}
			for pos, tiers := range m {
				merged[pos] = tiers
			}
			accom = merged
		} else {
			log.Printf("[hesi-audit] accommodation_standard_offline JSON 异常, 用代码默认值: %v", e)
		}
	} else if err != sql.ErrNoRows {
		log.Printf("[hesi-audit] 线下住宿参数读取失败, 用代码默认值: %v", err)
	}
	accomOfflineStdCache = accom
	accomOfflineAt = time.Now()
	return accom
}

// hesiDeptNode 部门缓存节点 (进程级, 5min TTL — 表 ~511 行, 每天 04:30 BI-SyncHesiDepartment 同步)
// 修部门树 N+1: 原来每张单据 2×末级判定 + 2×递归祖先链 (每层 1 SELECT), 待审批列表一刷 ~2200 条部门小查询卡 2-3 秒
type hesiDeptNode struct {
	name     string
	parentID string
	hasChild bool
	active   bool
}

var (
	hesiDeptTreeCache   map[string]hesiDeptNode
	hesiDeptTreeCacheAt time.Time
	hesiDeptTreeCacheMu sync.Mutex
)

// loadhesiDeptTreeCache 一次性加载全部门表进内存 (加载失败时退回过期缓存兜底, 都没有则空 map → 各规则按"部门不在表"宽松跳过)
func (h *DashboardHandler) loadhesiDeptTreeCache() map[string]hesiDeptNode {
	hesiDeptTreeCacheMu.Lock()
	defer hesiDeptTreeCacheMu.Unlock()
	if hesiDeptTreeCache != nil && time.Since(hesiDeptTreeCacheAt) < 5*time.Minute {
		return hesiDeptTreeCache
	}
	m := map[string]hesiDeptNode{}
	rows, err := h.DB.Query(`SELECT id, name, IFNULL(parent_id,''), IFNULL(has_child,0), IFNULL(active,0) FROM hesi_department`)
	if err != nil {
		log.Printf("[hesi-audit] 部门表加载失败: %v", err)
		if hesiDeptTreeCache != nil {
			return hesiDeptTreeCache
		}
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, parent string
		var hasChild, active int
		if err := rows.Scan(&id, &name, &parent, &hasChild, &active); err != nil {
			continue
		}
		m[id] = hesiDeptNode{name: name, parentID: parent, hasChild: hasChild == 1, active: active == 1}
	}
	if err := rows.Err(); err != nil {
		log.Printf("[hesi-audit] 部门表读取中断: %v", err)
		if hesiDeptTreeCache != nil {
			return hesiDeptTreeCache
		}
		// 没有旧缓存可兜底: 本次将就用残缺数据, 但不能存成缓存被信任 5 分钟, 下次调用重新加载
		return m
	}
	hesiDeptTreeCache = m
	hesiDeptTreeCacheAt = time.Now()
	return m
}

// deptChainContains 沿 parent_id 链向上找部门名含 keyword 的祖先 (含自身), 最多 10 层 — 跟原递归 SELECT 语义一致
func deptChainContains(m map[string]hesiDeptNode, deptID, keyword string) bool {
	cur := deptID
	for i := 0; i < 10 && cur != ""; i++ {
		node, ok := m[cur]
		if !ok {
			return false
		}
		if strings.Contains(node.name, keyword) {
			return true
		}
		if node.parentID == cur {
			break
		}
		cur = node.parentID
	}
	return false
}

// ruleDeptLeaf 部门必须末级 (hesi_department.has_child = 0)
// label 用于区分调用方 (例: "提交人部门 (规则 1)")
func (h *DashboardHandler) ruleDeptLeaf(deptID string, label string) string {
	if deptID == "" {
		return label + " 为空"
	}
	node, ok := h.loadhesiDeptTreeCache()[deptID]
	if !ok || !node.active {
		// 部门不在表 (新部门 / 未同步 / 已停用) → 跳过此规则 (原 SQL 带 active=1 条件, 语义同)
		return ""
	}
	if node.hasChild {
		return label + "「" + node.name + "」非末级"
	}
	return ""
}

// companyDeptName 若 deptID 是合思部门树顶层"公司"(法人实体)节点, 返回公司名; 否则返回 ""。
// 用于规则 2: 费用承担部门不能选公司, 必须选公司下面的具体部门。
// 判定: 部门树根 corp 节点 parentID 为空; 其直接子节点(parentID=根)即一家公司(世创/世用/华鲜高新/各分公司/集团 等 47 家)。
// 注意公司节点可能 has_child=0(如华鲜高新无下级), 不能用"末级"判定, 必须按"是不是顶层公司节点"判。
func (h *DashboardHandler) companyDeptName(deptID string) string {
	if deptID == "" {
		return ""
	}
	m := h.loadhesiDeptTreeCache()
	node, ok := m[deptID]
	if !ok || !node.active {
		// 不在表 / 已停用 → 跳过 (与 ruleDeptLeaf 同口径; 防顶层有停用的非公司节点如"培训部"被误判为公司)
		return ""
	}
	if node.parentID == "" {
		return node.name // 自身即根 corp
	}
	if parent, ok := m[node.parentID]; ok && parent.parentID == "" {
		return node.name // 父是根 → 自身是顶层公司
	}
	return ""
}

// isResearchDept 递归判定部门是否在研发链 (部门名或祖先含"研发")
// 用于规则 10 豁免 1: 研发部门样品采买无票豁免
// 实查: 应用研发部 / 技术研发部 / 产品研发中心 / 研发一二三组 等
func (h *DashboardHandler) isResearchDept(deptID string) bool {
	if deptID == "" {
		return false
	}
	return deptChainContains(h.loadhesiDeptTreeCache(), deptID, "研发")
}

// isBrandCenterDept 判定部门在品牌中心子树 (递归 parent_id)
// 复用 isResearchDept 模式
func (h *DashboardHandler) isBrandCenterDept(deptID string) bool {
	if deptID == "" {
		return false
	}
	return deptChainContains(h.loadhesiDeptTreeCache(), deptID, "品牌中心")
}

// 城市分级缓存 (5min TTL, hesi_city_tier 表 68 城市)
var (
	cityTierCache   map[string]string
	cityTierCacheAt time.Time
	cityTierCacheMu sync.Mutex
)

// loadCityTierCache 加载 hesi_city_tier 表到内存 cache (5min TTL)
func (h *DashboardHandler) loadCityTierCache() map[string]string {
	cityTierCacheMu.Lock()
	defer cityTierCacheMu.Unlock()
	if cityTierCache != nil && time.Since(cityTierCacheAt) < 5*time.Minute {
		return cityTierCache
	}
	m := map[string]string{}
	rows, err := h.DB.Query(`SELECT city_name, tier FROM hesi_city_tier`)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var name, tier string
		if err := rows.Scan(&name, &tier); err == nil {
			m[name] = tier
		}
	}
	cityTierCache = m
	cityTierCacheAt = time.Now()
	return m
}

// extractTier 从 city raw 字符串 (例 [{"label":"浙江省/杭州/余杭区"}]) 找匹配的城市分级
// 按 city_name 长度 desc 排序匹配, 避免 "石" 抢 "石家庄"
func extractTier(cityRaw string, tierMap map[string]string) string {
	if cityRaw == "" || len(tierMap) == 0 {
		return "其他"
	}
	keys := make([]string, 0, len(tierMap))
	for k := range tierMap {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		if strings.Contains(cityRaw, k) {
			return tierMap[k]
		}
	}
	return "其他"
}
