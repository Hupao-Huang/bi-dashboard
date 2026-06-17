package handler

// 樊雪娇日常报销单 AI 审批建议规则 (2026-05-27 新建)
// 仅适用于 spec=ID01Fk3qJYYFvp prefix (日常报销单)
// 调用前必须在上游加 spec_id 过滤 + 樊雪娇审批人判定 (见 profile_hesi_pending.go)
//
// 模式: 两阶段
//   - 阶段 1 (当前): AI 建议 dry-run, 仅展示 suggestion 字段给审批人参考
//   - 阶段 2 (后续): 真自动审批, 命中 reject 直接 INSERT into hesi_approval_queue

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

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

// 需人工核座位等级的 fee_type (规则 7-2)
// 跑哥规则: 汽车/火车/高铁/动车二等座及以下, 飞机经济舱
// 火车票: 合思发票主体已 OCR 出"座位类型", 改为自动判 (见 ruleTrainSeatClass), 不在此 manual 名单。
// 飞机/客车/汽车/其他交通: 合思未存结构化舱位, 仍给审批人 manual 提示。
var seatReviewFeeTypes = map[string]string{
	"ID01Fk0IZFCaJx": "飞机 (经济舱)",
	"ID01Fk0IZFCbgz": "客车",
	"ID01Fk0IZFCbx5": "其他交通",
	"ID01KhLSijR88T": "汽车 (二等座及以下)",
}

// 火车座位类型分类 (规则 7-2, 跑哥 2026-06-05 拍板: 二等座及以下合规)
// 数据驱动: 全库火车票实查 12 种座位类型, 按此分三档。其余(二等座/硬座/硬卧/软座/二等卧/
// 卧代二等座/硬卧代硬座/无座等)视为"二等座及以下"自动通过。
var trainSeatOverStandard = map[string]bool{ // 明显高于二等座 → 驳回
	"一等座": true, "商务座": true, "特等座": true, "优选一等座": true, "一等卧": true,
}
var trainSeatNeedReview = map[string]bool{ // 卧铺类 → 人工核(可能合理也可能超标)
	"软卧": true, "高级软卧": true, "动卧": true,
}

// 住宿费 fee_type (规则 7-3 触发 + 单晚价提取)
const hotelFeeTypeID = "ID01Fk0STRw3p5"

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

// 城市分级缓存 (5min TTL, hesi_city_tier 表 68 城市)
var (
	cityTierCache   map[string]string
	cityTierCacheAt time.Time
	cityTierCacheMu sync.Mutex
)

// 出差补贴标准 (¥/天, 规则 11)
// PDF V7.0 + 跑哥口述: 总裁 200 / 副总裁 150 / 集团总监 100 / 集团经理 80 / 主管及以下 60
var subsidyStandard = map[string]float64{
	"总裁":    200,
	"副总裁":   150,
	"集团总监":  100,
	"集团经理":  80,
	"主管和其他": 60,
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

// AuditSuggestion 审批建议输出
type AuditSuggestion struct {
	Action  string   `json:"action"` // agree / reject / manual
	Reasons []string `json:"reasons"`
}

// AuditDailyExpense 日常报销单审批规则引擎 (handler 方法, 需访问 LookupLegalEntityName 等)
// 入参:
//   - ownerDeptID: hesi_flow.owner_department 发起人部门 ID (= raw_json.u_提交人部门, 规则 1)
//   - departmentID: hesi_flow.department_id 报销/借款部门 ID (= raw_json.expenseDepartment, 规则 2)
//   - expenseMoney: hesi_flow.expense_money 报销金额 (规则 5 招待费金额对比)
//   - rawJSON: 合思单据 raw_json (含 payeeId / submitterId / 法人实体 / details / expenseLinks 等)
//
// applyStableSubmitDate 用调用方传入的"首次提交时间"(表 hesi_flow.submit_date)覆盖 raw["submitDate"],
// 供发票时效规则(8-4 发票>1月 / 14 健康证>6月)使用。
// 为什么必须用表值而非 raw_json.submitDate: 后者会被"退回后重新提交"刷新成最新提交时间, 把发票开票距提交
// 人为拖长、把没超期的发票误判超期 (跑哥 2026-06-17, B26003023: 发票5/10 首次提交5/29=19天合规, 退回后6/11重提
// 用6/11算成32天误判超期)。表 submit_date 在 sync-hesi / hesi_pending_sync 两个同步入口的 ON DUPLICATE KEY UPDATE
// 都不更新, 是冻结的首次入库提交时间。调用方(profile_hesi_pending)本就已查出 submit_date, 传入即可, 不再回查库。
// 守卫: 只在表值"更早"时才覆盖 —— 发票时效取更早的提交基准(宁严勿松); 若表值因脏数据异常地比 raw 更晚,
// 保留 raw 原值, 避免把发票距提交算短而漏判真超期(二审守卫)。firstSubmitDate<=0(老单没同步到)或 raw 为 nil 时不动。
func applyStableSubmitDate(firstSubmitDate int64, raw map[string]interface{}) {
	if raw == nil || firstSubmitDate <= 0 {
		return
	}
	if cur, ok := raw["submitDate"].(float64); ok && cur > 0 && float64(firstSubmitDate) >= cur {
		return // raw 已是更早(或相等)的提交时间, 不覆盖
	}
	raw["submitDate"] = float64(firstSubmitDate)
}

// AuditDailyExpense firstSubmitDate = hesi_flow.submit_date(稳定首次提交毫秒戳, 0 表缺失), 见 applyStableSubmitDate。
func (h *DashboardHandler) AuditDailyExpense(ownerDeptID, departmentID, submitterID, flowID string, expenseMoney float64, rawJSON string, firstSubmitDate int64) *AuditSuggestion {
	var raw map[string]interface{}
	if rawJSON != "" {
		_ = json.Unmarshal([]byte(rawJSON), &raw)
	}

	// 发票时效(规则 8-4 发票>1月 / 规则 14 健康证发票>6月)按"首次提交时间"算:
	// 用表里稳定的 submit_date 覆盖 raw.submitDate, 避免退回重提把发票拖成超期误判 (详见 applyStableSubmitDate)。
	applyStableSubmitDate(firstSubmitDate, raw)

	// 线下判定: 法人实体名含 世创/世用 → 走线下专属规则集 (樊雪娇 2026-06-11, 与集团口径不同)
	isOfflineFlow := false
	if leID, _ := raw["法人实体"].(string); leID != "" {
		leName := h.LookupLegalEntityName(leID)
		if leName == "" {
			// 字典临时拉不到 → 本单只能按集团口径审, 留痕别静默 (规则15二审)
			log.Printf("[hesi-audit] 法人实体 %s 名称查不到 (字典故障或ID缺), flow=%s 按集团规则审", leID, flowID)
		}
		isOfflineFlow = strings.Contains(leName, "世创") || strings.Contains(leName, "世用")
	}

	// 特批: 集团(非线下) + 收款人户名"黄涛" + 实际报销额 ≤ 该单发票总额 → 直接通过, 跳过所有其它规则 (跑哥 2026-06-17)。
	// 户名匹配(跑哥拍板, 认可同名风险); 发票总额=该单所有发票价税合计; 无发票(合计0)不豁免, 走常规。
	if !isOfflineFlow {
		if payeeID, _ := raw["payeeId"].(string); payeeID != "" && expenseMoney > 0 && h.payeeName(payeeID) == "黄涛" {
			if invoiceTotal := h.sumInvoiceTotal(flowID); invoiceTotal > 0 && expenseMoney <= invoiceTotal+0.01 {
				return &AuditSuggestion{Action: "agree", Reasons: []string{fmt.Sprintf("集团收款人黄涛特批: 报销额 ¥%.2f ≤ 发票合计 ¥%.2f, 免其它规则", expenseMoney, invoiceTotal)}}
			}
		}
	}

	var rejectReasons []string
	var warnings []string

	// 规则 1: 发起人部门 末级 (员工提交人部门, raw_json.u_提交人部门 = hesi_flow.owner_department)
	if r := h.ruleDeptLeaf(ownerDeptID, "发起人部门 (规则 1)"); r != "" {
		rejectReasons = append(rejectReasons, r)
	}

	// 规则 2: 报销/借款部门 末级 (= 费用承担部门, raw_json.expenseDepartment = hesi_flow.department_id)
	if r := h.ruleDeptLeaf(departmentID, "报销/借款部门 (规则 2)"); r != "" {
		rejectReasons = append(rejectReasons, r)
	}

	// 规则 3: 收款信息必须为银行账户
	payeeID, _ := raw["payeeId"].(string)
	if r := rulePayeeBank(h.DB, payeeID); r != "" {
		rejectReasons = append(rejectReasons, r)
	}

	// 规则 4: 所属公司 = 提交人合同公司 (钉钉花名册)
	if r := h.ruleCorpMatch(raw); r != "" {
		rejectReasons = append(rejectReasons, r)
	}

	// 规则 5: 业务招待费 关联招待费用申请单 + 金额不超
	if r := h.ruleRequisitionLink(raw, expenseMoney, businessTreatFeeTypes, reqTreatmentSpecPrefix, "业务招待费", "招待费用申请单", "规则 5"); r != "" {
		rejectReasons = append(rejectReasons, r)
	}

	// 规则 6: 固定资产 关联固定资产申请单 + 金额不超
	if r := h.ruleRequisitionLink(raw, expenseMoney, fixedAssetFeeTypes, reqFixedAssetSpecPrefix, "固定资产", "固定资产申请单", "规则 6"); r != "" {
		rejectReasons = append(rejectReasons, r)
	}

	// 规则 7-1: 交通及差旅费(含私车公用/过路费) 关联出差申请单 + 金额不超
	// 仅集团: 线下(世创/世用)交通差旅费全部无需关联; 集团则全部(含私车/过路)都要关联 (跑哥 2026-06-17)
	if !isOfflineFlow {
		if r := h.ruleRequisitionLink(raw, expenseMoney, travelExpenseFeeTypes, reqTripSpecPrefix, "交通及差旅费", "出差申请单", "规则 7-1"); r != "" {
			rejectReasons = append(rejectReasons, r)
		}
	}

	// 规则 7-2: 飞机/客车/汽车明细需人工核座位等级 (合思未存舱位字段, manual 提示)
	if w := ruleSeatManualReview(raw); w != "" {
		warnings = append(warnings, w)
	}

	// 规则 7-2 (火车): 用发票主体 OCR 的座位类型自动判 (二等座及以下过 / 一等座·商务座超标驳回 / 卧铺人工核)
	if rejMsg, warnMsg := h.ruleTrainSeatClass(flowID); rejMsg != "" || warnMsg != "" {
		if rejMsg != "" {
			rejectReasons = append(rejectReasons, rejMsg)
		}
		if warnMsg != "" {
			warnings = append(warnings, warnMsg)
		}
	}

	// 规则 7-3: 住宿费单晚价 ≤ 城市×职级标准 (同住上浮 20%); 线下(世创/世用)用线下住宿表
	if rejectMsg, warnMsg := h.ruleAccommodationStandard(raw, submitterID, isOfflineFlow); rejectMsg != "" || warnMsg != "" {
		if rejectMsg != "" {
			rejectReasons = append(rejectReasons, rejectMsg)
		}
		if warnMsg != "" {
			warnings = append(warnings, warnMsg)
		}
	}

	// 规则 8 + 10: 发票审核 (抬头/税号/金额/开票时间) + 无票判定 + 3 种豁免
	if invRej, invWarn := h.ruleInvoiceChecks(raw, ownerDeptID, flowID); len(invRej) > 0 || len(invWarn) > 0 {
		rejectReasons = append(rejectReasons, invRej...)
		warnings = append(warnings, invWarn...)
	}

	// 规则 11: 出差补贴 ≤ u_天数 × 职级标准 (半天按 1 天 ceil)
	// 线下单不走集团补贴标准 (规则 15-3 用线下口径替代)
	if rejMsg, warnMsg := h.ruleSubsidyStandard(raw, submitterID); !isOfflineFlow && (rejMsg != "" || warnMsg != "") {
		if rejMsg != "" {
			rejectReasons = append(rejectReasons, rejMsg)
		}
		if warnMsg != "" {
			warnings = append(warnings, warnMsg)
		}
	}

	// 规则 12-2: 消费事由长度审核
	if rejMsg, warnMsg := ruleDriveAndReasons(raw); rejMsg != "" || warnMsg != "" {
		if rejMsg != "" {
			rejectReasons = append(rejectReasons, rejMsg)
		}
		if warnMsg != "" {
			warnings = append(warnings, warnMsg)
		}
	}

	// 规则 12-1: 私车公用按行车记录自动对账 (报销 ≤ 系统算出补助 通过, 超了驳回, 拉不到记录转人工)
	if r12Rej, r12Warn := h.ruleDriveRecordCheck(raw); len(r12Rej) > 0 || len(r12Warn) > 0 {
		rejectReasons = append(rejectReasons, r12Rej...)
		warnings = append(warnings, r12Warn...)
	}

	// 规则 13: 必填字段校验 (品牌中心/研发中心必选 + 附件 + 报销=支付金额)
	if rej, warn := h.ruleRequiredFields(raw, ownerDeptID); len(rej) > 0 || len(warn) > 0 {
		rejectReasons = append(rejectReasons, rej...)
		warnings = append(warnings, warn...)
	}

	// 规则 14: 健康证及体检 — 金额 ≤100 且 发票开票 ≤6 个月才通过 (跑哥 2026-06-11)
	if rej := h.ruleHealthExam(raw, flowID); len(rej) > 0 {
		rejectReasons = append(rejectReasons, rej...)
	}

	// ===== 规则 15: 线下专属规则集 (樊雪娇 2026-06-11) =====
	// 触发: 法人实体名含"世创"或"世用" (线下公司)。线下出差补贴口径替代规则 11 (上面已 gate)
	if isOfflineFlow {
		offRej, offWarn := h.ruleOfflineExtras(raw, flowID, submitterID)
		rejectReasons = append(rejectReasons, offRej...)
		warnings = append(warnings, offWarn...)
	}

	// 规则 16: 企业支付行程防重复报销 (跑哥 2026-06-11; 樊雪娇 2026-06-17 收窄: 票价不同→通过)
	if r16Rej, r16Warn := h.ruleCorpPaidDuplicate(raw, flowID); len(r16Rej) > 0 || len(r16Warn) > 0 {
		rejectReasons = append(rejectReasons, r16Rej...)
		warnings = append(warnings, r16Warn...)
	}

	// 规则 17: 补贴日期须在关联出差申请单起止日期内 (财务 2026-06-12)
	if r17 := h.ruleSubsidyDateInTrip(raw); len(r17) > 0 {
		rejectReasons = append(rejectReasons, r17...)
	}

	// 规则 18: 广告费发票项目名称须含"广告/推广" (财务 2026-06-12)
	if r18Rej, r18Warn := h.ruleAdInvoiceItemName(raw, flowID); len(r18Rej) > 0 || len(r18Warn) > 0 {
		rejectReasons = append(rejectReasons, r18Rej...)
		warnings = append(warnings, r18Warn...)
	}

	// 优先级: reject > manual > agree
	if len(rejectReasons) > 0 {
		all := rejectReasons
		for _, w := range warnings {
			all = append(all, "[需人工核] "+w)
		}
		return &AuditSuggestion{Action: "reject", Reasons: all}
	}
	if len(warnings) > 0 {
		return &AuditSuggestion{Action: "manual", Reasons: warnings}
	}
	return &AuditSuggestion{Action: "agree", Reasons: []string{"所有规则通过"}}
}

// ruleCorpMatch 所属公司 (raw_json 法人实体) 必须 = 提交人合同公司 (hesi_employee_contract_company)
// 复用 v1.75.0 GetHesiFlowDetail 的主体校验逻辑
func (h *DashboardHandler) ruleCorpMatch(raw map[string]interface{}) string {
	submitterID, _ := raw["submitterId"].(string)
	legalEntityID, _ := raw["法人实体"].(string)
	if submitterID == "" {
		return "" // 提交人缺失, 不本规则拦截
	}
	if legalEntityID == "" {
		return "所属公司 (法人实体) 为空 (规则 4)"
	}

	var contractCompany sql.NullString
	err := h.DB.QueryRow(
		`SELECT contract_company_name FROM hesi_employee_contract_company WHERE hesi_staff_id = ? LIMIT 1`,
		submitterID,
	).Scan(&contractCompany)
	if err != nil || !contractCompany.Valid || contractCompany.String == "" {
		// 钉钉花名册查不到 / 合同公司空 → 跳过本规则 (v1.75.0 单据详情已有提示)
		return ""
	}

	legalEntityName := h.LookupLegalEntityName(legalEntityID)
	if legalEntityName == "" {
		return "" // 法人实体反查失败 (字典未同步), 跳过
	}

	// 完全一致, 或"合同公司是所属公司的分公司"(分公司非独立法人, 报销可用主公司主体) → 通过
	if legalEntityName == contractCompany.String || isBranchOfLegalEntity(contractCompany.String, legalEntityName) {
		return ""
	}
	return "所属公司「" + legalEntityName + "」≠ 合同公司「" + contractCompany.String + "」(规则 4)"
}

// isBranchOfLegalEntity 合同公司是否为所属公司的分公司。
// 分公司不是独立法人, 报销主体可用主公司; 分公司全名 = 主公司全名 + "XX分公司"。
// 触发场景(跑哥 2026-06-05): 浙江松鲜鲜世创食品科技有限公司 + 7 家分公司(杭州/北京/重庆/南京/东北/山东/西北),
// 分公司员工合同公司是分公司, 报销所属公司填主公司, 应放行。
// 通用判定(不硬编码公司名): 合同公司以所属公司全名开头 且 以"分公司"结尾。
func isBranchOfLegalEntity(contractCompany, legalEntityName string) bool {
	if legalEntityName == "" || contractCompany == "" {
		return false
	}
	return strings.HasPrefix(contractCompany, legalEntityName) && strings.HasSuffix(contractCompany, "分公司")
}

// ruleRequisitionLink 通用规则: 费用明细含某类 fee_type → 关联指定申请单 + 金额不超
// 入参:
//   - feeTypeSet: 触发的 fee_type id 集合 (例: 业务招待费 set, 固定资产 set)
//   - specPrefix: 关联申请单的 spec_id 前缀
//   - feeLabel/reqLabel: 文案标签 (例: "业务招待费"/"招待费用申请单")
//   - ruleLabel: 规则编号 (例: "规则 5")
//
// 逻辑:
//  1. details 含触发 fee_type → 进规则
//  2. expenseLinks 必须非空, 否则驳回 "未关联 reqLabel"
//  3. 关联的每个 flowId 必须 spec=specPrefix, 否则驳回 "关联单类型错"
//  4. 报销金额 ≤ 关联申请单 requisitionMoney 合计, 否则驳回 "超额"
func (h *DashboardHandler) ruleRequisitionLink(raw map[string]interface{}, expenseMoney float64,
	feeTypeSet map[string]string, specPrefix, feeLabel, reqLabel, ruleLabel string) string {

	// 1. 检测是否含触发 fee_type 明细
	details, _ := raw["details"].([]interface{})
	hit := false
	for _, d := range details {
		dm, ok := d.(map[string]interface{})
		if !ok {
			continue
		}
		feeTypeID, _ := dm["feeTypeId"].(string)
		if _, isHit := feeTypeSet[feeTypeID]; isHit {
			hit = true
			break
		}
	}
	if !hit {
		return ""
	}

	// 2. expenseLinks 必须非空
	linksRaw, _ := raw["expenseLinks"].([]interface{})
	if len(linksRaw) == 0 {
		return "费用明细含" + feeLabel + ", 但未关联" + reqLabel + " (" + ruleLabel + ")"
	}

	// 3+4. 累计关联申请单金额 + 类型验证
	// codex 二审修复 v1.75.15: 关联单查不到 / requisitionMoney 解析失败 → 不能静默放行,
	// 改为 reject 提示"关联申请单无法识别金额" (业务安全偏严)
	var totalReqMoney float64
	matched := 0
	for _, l := range linksRaw {
		linkID, _ := l.(string)
		if linkID == "" {
			continue
		}
		var specID sql.NullString
		var reqRawJSON sql.NullString
		err := h.DB.QueryRow(`SELECT IFNULL(specification_id,''), IFNULL(raw_json,'') FROM hesi_flow WHERE flow_id = ? LIMIT 1`, linkID).Scan(&specID, &reqRawJSON)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(specID.String, specPrefix) {
			return "关联单不是「" + reqLabel + "」(" + ruleLabel + ")"
		}
		matched++
		if reqRawJSON.Valid && reqRawJSON.String != "" {
			var reqMap map[string]interface{}
			if json.Unmarshal([]byte(reqRawJSON.String), &reqMap) == nil {
				if rm, ok := reqMap["requisitionMoney"].(map[string]interface{}); ok {
					if std, ok := rm["standard"].(string); ok {
						if v, e := strconv.ParseFloat(std, 64); e == nil {
							totalReqMoney += v
						}
					}
				}
			}
		}
	}

	// 关联单都查不到 (link id 全无效) → reject
	if matched == 0 {
		return "关联" + reqLabel + " 查不到有效单据 (" + ruleLabel + ")"
	}
	// 关联单存在但金额解析失败 → reject (codex 二审防漏拦)
	if totalReqMoney == 0 {
		return "关联" + reqLabel + " 金额无法识别, 无法对比报销额 (" + ruleLabel + ")"
	}
	if expenseMoney > totalReqMoney {
		return fmt.Sprintf("报销金额 ¥%.2f > 关联%s合计 ¥%.2f (%s)", expenseMoney, reqLabel, totalReqMoney, ruleLabel)
	}
	return ""
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

// rulePayeeBank 收款方必须为银行账户 (hesi_payee_info.sort ∈ {BANK, OVERSEABANK})
// 不通过的: ALIPAY 支付宝 / WALLET 微信钉钉钱包 / CHECK 支票 / ACCEPTANCEBILL 承兑汇票 / OTHER 其他
func rulePayeeBank(db *sql.DB, payeeID string) string {
	if payeeID == "" {
		return "收款信息为空 (规则 3)"
	}
	var sort, name string
	err := db.QueryRow(`SELECT sort, name FROM hesi_payee_info WHERE id = ? AND active = 1 LIMIT 1`, payeeID).Scan(&sort, &name)
	if err != nil {
		// payee 不在表 (新增收款方未同步, 等下次 BI-SyncHesiPayee 04:40 后再判) → 跳过
		return ""
	}
	if sort == "BANK" || sort == "OVERSEABANK" {
		return ""
	}
	sortLabel := map[string]string{
		"ALIPAY":         "支付宝",
		"WALLET":         "微信/钉钉钱包",
		"CHECK":          "支票",
		"ACCEPTANCEBILL": "承兑汇票",
		"OTHER":          "其他",
	}[sort]
	if sortLabel == "" {
		sortLabel = sort
	}
	return "收款信息必须为银行账户, 当前为「" + sortLabel + "」(规则 3)"
}

// payeeName 查收款人户名 (hesi_payee_info.name); 查不到返回空。用于集团黄涛特批判定。
func (h *DashboardHandler) payeeName(payeeID string) string {
	if h.DB == nil || payeeID == "" {
		return ""
	}
	var name string
	if err := h.DB.QueryRow(`SELECT name FROM hesi_payee_info WHERE id=? AND active=1 LIMIT 1`, payeeID).Scan(&name); err != nil {
		return ""
	}
	return name
}

// sumInvoiceTotal 该单所有发票价税合计 (hesi_flow_invoice.total_amount 求和); 无发票/查不到返回 0。
func (h *DashboardHandler) sumInvoiceTotal(flowID string) float64 {
	if h.DB == nil || flowID == "" {
		return 0
	}
	var sum sql.NullFloat64
	if err := h.DB.QueryRow(`SELECT SUM(IFNULL(total_amount,0)) FROM hesi_flow_invoice WHERE flow_id=?`, flowID).Scan(&sum); err != nil {
		return 0
	}
	return sum.Float64
}

// 样品 fee_type 集合 (规则 10 豁免 1: 研发部门 + 样品 = 无票豁免)
var sampleFeeTypes = map[string]string{
	"ID01Fk0FsIqhDV": "赠品及样品",
	"ID01KhLrRhAWp9": "样品费用",
	"ID01Fk0B0hMfF5": "小样推广赠品、样品",
}

// 出差/餐补/市内补贴 fee_type 集合 (规则 10 豁免 2: 补贴类无票豁免)
var subsidyFeeTypes = map[string]string{
	"ID01Fk0MQBAAQ7": "补贴",
	"ID01Fk0MQBAB6D": "市内补贴",
	"ID01FkA9pR8zGT": "餐费补贴",
}

// 私车公用 fee_type (规则 12-1 自驾报销)
const driveFeeTypeID = "ID01Fr2mX8KP2T"

// 过路费 fee_type (规则 15-3 补贴扣减: 消费日按发票通行日期算)
const tollFeeTypeID = "ID01KhLSijR8FV"

// 健康证及体检 fee_type (规则 14, 合思 feeTypes API 2026-06-11 实查, 无子类)
const healthExamFeeTypeID = "ID01KTruvX23pl"

// ruleRequiredFields 规则 13: 必填字段校验
// 13-① 备注 (description) — 跑哥说"选填", 不审
// 13-② 品牌中心部门 → u_品牌中心必选 非空
// 13-③ 研发中心部门 → u_研发中心必选 非空
// 13-④ 附件 — 跑哥拍板"只是字段说明, 不强制必填", 不审
// 13-⑤ expenseMoney ≈ payMoney (容差 0.01)
//
//	codex 二审 v1.75.17 修: payMoney < expMoney 视为备用金/借款冲抵 (DB 实查 28+4 单),
//	不再 reject, 转 manual 让审批人核. payMoney > expMoney 才算诡异 reject.
func (h *DashboardHandler) ruleRequiredFields(raw map[string]interface{}, ownerDeptID string) ([]string, []string) {
	var rejects, warnings []string

	// 13-② 品牌中心
	if h.isBrandCenterDept(ownerDeptID) {
		v, _ := raw["u_品牌中心必选"].(string)
		if v == "" {
			rejects = append(rejects, "部门为品牌中心, 但单据未填'品牌中心必选'字段 (规则 13-②)")
		}
	}
	// 13-③ 研发中心 (复用 isResearchDept)
	if h.isResearchDept(ownerDeptID) {
		v, _ := raw["u_研发中心必选"].(string)
		if v == "" {
			rejects = append(rejects, "部门为研发中心, 但单据未填'研发中心必选'字段 (规则 13-③)")
		}
	}
	// 13-⑤ 报销=支付金额 (备用金冲抵场景转 manual, 不冤枉)
	expMoney, ok1 := getStandardAmount(raw["expenseMoney"])
	payMoney, ok2 := getStandardAmount(raw["payMoney"])
	if ok1 && ok2 && expMoney > 0 && payMoney > 0 && math.Abs(expMoney-payMoney) > 0.01 {
		if payMoney < expMoney {
			warnings = append(warnings, fmt.Sprintf("报销金额 ¥%.2f > 支付金额 ¥%.2f (¥%.2f 差额), 疑似备用金/借款冲抵, 需人工核 (规则 13-⑤)",
				expMoney, payMoney, expMoney-payMoney))
		} else {
			rejects = append(rejects, fmt.Sprintf("支付金额 ¥%.2f > 报销金额 ¥%.2f, 数据异常 (规则 13-⑤)", payMoney, expMoney))
		}
	}
	return rejects, warnings
}

// getStandardAmount 从 raw_json 金额结构 {"standard":"123.45",...} 取金额
func getStandardAmount(v interface{}) (float64, bool) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return 0, false
	}
	s, ok := m["standard"].(string)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// isBrandCenterDept 判定部门在品牌中心子树 (递归 parent_id)
// 复用 isResearchDept 模式
func (h *DashboardHandler) isBrandCenterDept(deptID string) bool {
	if deptID == "" {
		return false
	}
	return deptChainContains(h.loadhesiDeptTreeCache(), deptID, "品牌中心")
}

// ruleDriveRecordCheck 规则 12-1 自动化 (跑哥 2026-06-12: 行车记录已可拉取, 不再一律人工核)
// 私车公用明细对账: 报销金额 vs 行车记录"系统算出补助"(里程 × 标准, 油¥0.7/电¥0.6 标准在合思配置里):
//   - 金额 ≤ 系统算出 → 自动通过 (少报不拦)
//   - 金额 > 系统算出 → 建议驳回 (多报了)
//   - 行车记录缺失/拉取失败 → 保留原人工核提示 (降级兜底)
func (h *DashboardHandler) ruleDriveRecordCheck(raw map[string]interface{}) ([]string, []string) {
	details, _ := raw["details"].([]interface{})
	var rejects []string
	var manual, zeroAmt []int
	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		if ft, _ := dm["feeTypeId"].(string); ft != driveFeeTypeID {
			continue
		}
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		no := 0
		if n, ok := form["detailNo"].(float64); ok {
			no = int(n)
		}
		var amt float64
		if a, ok := form["amount"].(map[string]interface{}); ok {
			if s, ok := a["standard"].(string); ok {
				amt, _ = strconv.ParseFloat(s, 64)
			}
		}
		// 金额 0/缺失/解析失败时不能走 "0 ≤ 补助 → 通过" 静默放行 (真实金额可能解析失败被置 0), 转人工
		if amt <= 0 {
			zeroAmt = append(zeroAmt, no)
			continue
		}
		recID, _ := form["u_行车记录"].(string)
		var rec *DriveRecord
		if recID != "" {
			rec = h.LookupDriveRecord(recID)
		}
		sub := 0.0
		if rec != nil && rec.Subsidy != "" {
			sub, _ = strconv.ParseFloat(rec.Subsidy, 64)
		}
		if sub <= 0 {
			manual = append(manual, no)
			continue
		}
		if amt > sub+0.005 {
			rejects = append(rejects, fmt.Sprintf("明细#%d 私车公用 ¥%.2f > 行车记录算出补助 ¥%.2f (%s km × ¥%s/km, 规则 12-1)",
				no, amt, sub, rec.Mileage, rec.Standard))
		}
	}
	var warnings []string
	if len(manual) > 0 {
		warnings = append(warnings, fmt.Sprintf("明细 %v 自驾, 行车记录缺失或拉取失败, 需人工核 KM × 车型单价 (油 ¥0.7/KM, 电 ¥0.6/KM, 规则 12-1)", uniqueInts(manual)))
	}
	if len(zeroAmt) > 0 {
		warnings = append(warnings, fmt.Sprintf("明细 %v 自驾, 报销金额为 0 或解析失败, 需人工核金额 (规则 12-1)", uniqueInts(zeroAmt)))
	}
	return rejects, warnings
}

// ruleDriveAndReasons 规则 12-2: 消费事由长度审核
// (12-1 自驾对账已升级为 ruleDriveRecordCheck 自动判, 2026-06-12 从本函数摘除)
// 12-2: consumptionReasons 长度 ≤50字 agree / >50字 reject (跑哥 2026-06-05 改: 50字以内都通过, 去掉10-50字人工核档)
//
//	字数按 rune 计 (中文 1 字)
func ruleDriveAndReasons(raw map[string]interface{}) (string, string) {
	details, _ := raw["details"].([]interface{})
	var driveDetails, longReasons []int
	_ = driveDetails

	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		no := 0
		if n, ok := form["detailNo"].(float64); ok {
			no = int(n)
		}

		// 12-2: 消费事由长度 (rune 计数, 中文 1 字) — 50 字以内通过, 超过 50 字驳回
		reason, _ := form["consumptionReasons"].(string)
		if len([]rune(reason)) > 50 {
			longReasons = append(longReasons, no)
		}
	}

	var rejects, warnings []string
	if len(longReasons) > 0 {
		rejects = append(rejects, fmt.Sprintf("明细 %v 消费事由 > 50 字 (规则 12-2)", uniqueInts(longReasons)))
	}
	return strings.Join(rejects, "; "), strings.Join(warnings, "; ")
}

// ruleSubsidyStandard 规则 11: 出差补贴 ≤ u_天数 × 职级标准
// 跑哥规则: 总裁 200 / 副总裁 150 / 集团总监 100 / 集团经理 80 / 主管和其他 60 (¥/天)
// 半天按 1 天计算 (math.Ceil)
// 触发: detail.feeTypeId = ID01Fk0MQBAAQ7 (补贴顶类), 取 feeTypeForm.u_出差补贴金额 + u_天数
// 注: 同明细的 u_市内补贴金额 / u_餐费补贴金额 不在本规则 (跑哥规则文写"出差补贴")
// 返回 (rejectMsg, warnMsg)
func (h *DashboardHandler) ruleSubsidyStandard(raw map[string]interface{}, submitterID string) (string, string) {
	details, _ := raw["details"].([]interface{})
	type hit struct {
		no     int
		amount float64
		days   float64
	}
	var hits []hit
	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		feeTypeID, _ := dm["feeTypeId"].(string)
		if feeTypeID != "ID01Fk0MQBAAQ7" {
			continue
		}
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		no := 0
		if n, ok := form["detailNo"].(float64); ok {
			no = int(n)
		}
		var amt, days float64
		if a, ok := form["u_出差补贴金额"].(map[string]interface{}); ok {
			if s, ok := a["standard"].(string); ok {
				amt, _ = strconv.ParseFloat(s, 64)
			}
		}
		if v, ok := form["u_天数"].(string); ok {
			days, _ = strconv.ParseFloat(v, 64)
		}
		if amt > 0 && days > 0 {
			hits = append(hits, hit{no, amt, days})
		}
	}
	if len(hits) == 0 {
		return "", ""
	}

	// 查提交人岗位职级 (复用规则 7-3 的字段)
	// fallback: SSC 表只 241 人 (审批职级), 普通员工 ~640 人不在.
	// codex 二审 v1.75.17 修: 未匹配时按"主管和其他" cap 算, 但超额转 manual (不冤枉潜在未维护的高管)
	position := ""
	if submitterID != "" {
		_ = h.DB.QueryRow(`SELECT IFNULL(position,'') FROM hesi_employee_contract_company WHERE hesi_staff_id = ? LIMIT 1`, submitterID).Scan(&position)
	}
	positionForDisplay := position
	isFallback := false
	if position == "" {
		position = "主管和其他"
		positionForDisplay = "主管和其他 (花名册未匹配)"
		isFallback = true
	}
	_, subsidyStd := h.loadHesiAuditParams() // DB 配置优先, 财务改口径 5 分钟生效
	std, ok := subsidyStd[position]
	if !ok {
		return "", "岗位职级「" + position + "」非标准 5 档, 出差补贴标准未配置 (规则 11)"
	}

	var rejects, warnings []string
	for _, h := range hits {
		// 半天按 1 天 → ceil
		ceilDays := math.Ceil(h.days)
		cap := ceilDays * std
		if h.amount > cap+0.01 {
			msg := fmt.Sprintf("明细#%d 出差补贴 ¥%.2f > 标准 ¥%.0f×%.0f天=¥%.2f (%s, 规则 11)",
				h.no, h.amount, std, ceilDays, cap, positionForDisplay)
			if isFallback {
				warnings = append(warnings, msg+" [SSC 未匹配, 请人工核职级]")
			} else {
				rejects = append(rejects, msg)
			}
		}
	}
	return strings.Join(rejects, "; "), strings.Join(warnings, "; ")
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

// ruleInvoiceChecks 规则 8: 发票审核 4 项 + 规则 10: 无票判定 + 豁免
//
// 8-1: 发票抬头 (buyer_name) 必须 = 所属公司 (raw_json.法人实体 → invoice_title)
// 8-2: 发票税号 (buyer_tax_no) 必须 = 所属公司税号 (开票资料 PDF 字典)
// 8-3: 每个明细发票合计 (sum total_amount) ≥ 明细金额 (有票才校验)
// 8-4: 开票时间 (invoice_date) 距单据提交 ≤1月 OK / 1-3月 manual / >3月 reject
// 10:  每个明细必须有票, 除非属于豁免:
//   - 豁免 A: 研发部门 (递归含"研发") + 样品 fee_type → 无票 OK
//   - 豁免 B: 补贴 fee_type (出差补贴/市内补贴/餐费补贴) → 无票 OK
//   - 豁免 C: u_无票原因截图说明 (实查 0 单, 当前不实现, 等合思后台加字段)
//
// 返回 (rejectReasons, warnings)
func (h *DashboardHandler) ruleInvoiceChecks(raw map[string]interface{}, ownerDeptID, flowID string) ([]string, []string) {
	if flowID == "" {
		return nil, nil
	}

	// 拉单据全部发票 + LEFT JOIN detail 拿 detail_no 行号
	type inv struct {
		detailID  string
		detailNo  int
		buyerName string
		taxNo     string
		date      int64
		total     float64
		approve   float64
	}
	rows, err := h.DB.Query(`SELECT
		IFNULL(i.detail_id,''), IFNULL(d.detail_no, 0),
		IFNULL(i.buyer_name,''), IFNULL(i.buyer_tax_no,''),
		IFNULL(i.invoice_date,0), IFNULL(i.total_amount,0), IFNULL(i.approve_amount,0)
		FROM hesi_flow_invoice i
		LEFT JOIN hesi_flow_detail d ON i.detail_id = d.detail_id AND d.flow_id = i.flow_id
		WHERE i.flow_id = ?`, flowID)
	if err != nil {
		log.Printf("[hesi-audit] 规则8/10 查发票失败 flow=%s: %v", flowID, err)
		return nil, []string{"发票数据读取失败, 规则 8/10 未自动判定, 需人工核发票"}
	}
	defer rows.Close()
	var invoices []inv
	invoiceReadBroken := false
	for rows.Next() {
		var i inv
		if err := rows.Scan(&i.detailID, &i.detailNo, &i.buyerName, &i.taxNo, &i.date, &i.total, &i.approve); err == nil {
			invoices = append(invoices, i)
		} else {
			if !invoiceReadBroken { // 只记首条, 防 DB 故障时每行刷一条日志
				log.Printf("[hesi-audit] 规则8/10 发票行解析失败 flow=%s: %v", flowID, err)
			}
			invoiceReadBroken = true
		}
	}
	if err := rows.Err(); err != nil {
		invoiceReadBroken = true
		log.Printf("[hesi-audit] 规则8/10 发票读取中断 flow=%s: %v", flowID, err)
	}
	// 发票数据不完整时规则 8/10 两个方向都会判错 (漏驳/误驳), 整体降级转人工
	if invoiceReadBroken {
		return nil, []string{"发票数据读取不完整, 规则 8/10 未自动判定, 需人工核发票"}
	}
	// 拉 details (从 raw_json) → 行号/金额/fee_type (用于规则 8-3 + 规则 10)
	details, _ := raw["details"].([]interface{})
	detailAmtMap := map[string]float64{}
	detailNoMap := map[string]int{}
	detailFeeTypeMap := map[string]string{}
	allDetailIDs := []string{}
	for _, iv := range invoices {
		if iv.detailNo > 0 {
			detailNoMap[iv.detailID] = iv.detailNo
		}
	}
	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		// detailId/detailNo/amount 都嵌在 feeTypeForm 子对象里 (合思 raw_json 结构实查)
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		did, _ := form["detailId"].(string)
		if did == "" {
			continue
		}
		allDetailIDs = append(allDetailIDs, did)
		if no, ok := form["detailNo"].(float64); ok && int(no) > 0 {
			detailNoMap[did] = int(no)
		}
		// feeTypeId 在 detail 顶层 (跟其他规则一致)
		if ft, ok := dm["feeTypeId"].(string); ok {
			detailFeeTypeMap[did] = ft
		}
		if a, ok := form["amount"].(map[string]interface{}); ok {
			if s, ok := a["standard"].(string); ok {
				if v, e := strconv.ParseFloat(s, 64); e == nil {
					detailAmtMap[did] = v
				}
			}
		}
	}

	// 规则 10: 无票判定 — 每个明细必须有票, 除非属于豁免
	// 跑哥规则: 研发部门 + 样品 / 出差补贴类 / 特殊无票截图说明 (字段未存, 走 manual)
	detailHasInvoice := map[string]bool{}
	for _, iv := range invoices {
		detailHasInvoice[iv.detailID] = true
	}
	isResearch := h.isResearchDept(ownerDeptID)
	var noInvoiceReject, missingFeeType []int
	for _, did := range allDetailIDs {
		if detailHasInvoice[did] {
			continue // 有票, 不进规则 10
		}
		feeTypeID := detailFeeTypeMap[did]
		no := detailNoMap[did]
		// raw_json 缺 feeTypeId 时豁免判定全失配, 不能当 "不豁免" 误驳, 转人工
		if feeTypeID == "" {
			missingFeeType = append(missingFeeType, no)
			continue
		}
		// 豁免 B: 补贴类 (出差/餐补/市内)
		if _, ok := subsidyFeeTypes[feeTypeID]; ok {
			continue
		}
		// 豁免 A: 研发部门 + 样品 fee_type
		if _, ok := sampleFeeTypes[feeTypeID]; ok && isResearch {
			continue
		}
		// 豁免 D: 私车公用 (按里程核账走规则 12-1, 本就无发票; 跑哥 2026-06-17)
		if feeTypeID == driveFeeTypeID {
			continue
		}
		// 都不豁免 → reject
		noInvoiceReject = append(noInvoiceReject, no)
	}
	var missingFeeTypeWarn []string
	if len(missingFeeType) > 0 {
		missingFeeTypeWarn = append(missingFeeTypeWarn, fmt.Sprintf("明细 %v 无发票且费用类型缺失, 豁免无法自动判定, 需人工核 (规则 10)", uniqueInts(missingFeeType)))
	}
	if len(invoices) == 0 {
		// 无发票时, 只跑规则 10 — 其他规则 8-1/8-2/8-3/8-4 都不适用
		if len(noInvoiceReject) > 0 {
			return []string{fmt.Sprintf("明细 %v 无发票, 不属于豁免 (规则 10: 仅研发样品/出差补贴/私车公用/特殊截图说明 豁免)", uniqueInts(noInvoiceReject))}, missingFeeTypeWarn
		}
		return nil, missingFeeTypeWarn
	}

	var rejects, warnings []string
	warnings = append(warnings, missingFeeTypeWarn...)

	// 拉所属公司 (法人实体) 的开票字典
	legalEntityID, _ := raw["法人实体"].(string)
	var expectedTitle, expectedTaxNo string
	if legalEntityID != "" {
		_ = h.DB.QueryRow(`SELECT IFNULL(invoice_title,''), IFNULL(tax_no,'') FROM hesi_legal_entity_invoice_info WHERE legal_entity_id = ? AND active=1 LIMIT 1`,
			legalEntityID).Scan(&expectedTitle, &expectedTaxNo)
	}

	// 规则 8-1/8-2: 抬头+税号 vs 法人实体 (法人实体未在字典 → 转人工)
	if expectedTitle == "" {
		warnings = append(warnings, "所属公司未在开票资料字典内, 抬头/税号无法判定 (规则 8-1/8-2)")
	} else {
		var wrongTitle, wrongTax []int
		for _, iv := range invoices {
			no := detailNoMap[iv.detailID]
			if iv.buyerName != "" && iv.buyerName != expectedTitle {
				wrongTitle = append(wrongTitle, no)
			}
			if iv.taxNo != "" && iv.taxNo != expectedTaxNo {
				wrongTax = append(wrongTax, no)
			}
		}
		if len(wrongTitle) > 0 {
			rejects = append(rejects, fmt.Sprintf("明细 %v 发票抬头 ≠「%s」(规则 8-1)", uniqueInts(wrongTitle), expectedTitle))
		}
		if len(wrongTax) > 0 {
			rejects = append(rejects, fmt.Sprintf("明细 %v 发票税号 ≠「%s」(规则 8-2)", uniqueInts(wrongTax), expectedTaxNo))
		}
	}

	// 规则 8-3: 每个明细发票合计 ≥ 明细金额
	detailInvoiceSum := map[string]float64{}
	for _, iv := range invoices {
		amt := iv.total
		if amt == 0 {
			amt = iv.approve
		}
		detailInvoiceSum[iv.detailID] += amt
	}
	for did, detailAmt := range detailAmtMap {
		sumInv := detailInvoiceSum[did]
		// float64 精度容差 0.01 (一分钱), 业务一分钱差异不计较
		if sumInv > 0 && sumInv+0.01 < detailAmt {
			no := detailNoMap[did]
			rejects = append(rejects, fmt.Sprintf("明细#%d 报销 ¥%.2f > 发票合计 ¥%.2f (规则 8-3)", no, detailAmt, sumInv))
		}
	}

	// 规则 8-4: 开票时间 (距单据提交) ≤1月 OK / 1-3月 manual / >3月 reject
	// raw_json.submitDate 是 毫秒级时间戳
	submitDate := int64(0)
	if v, ok := raw["submitDate"].(float64); ok {
		submitDate = int64(v)
	}
	if submitDate > 0 {
		const month = int64(30 * 24 * 3600 * 1000)
		var stale1m, stale3m []int
		for _, iv := range invoices {
			if iv.date == 0 {
				continue
			}
			// 健康证及体检明细不走 3 个月通用线 (规则 14 单独给 6 个月)
			if detailFeeTypeMap[iv.detailID] == healthExamFeeTypeID {
				continue
			}
			diff := submitDate - iv.date
			no := detailNoMap[iv.detailID]
			if diff > 3*month {
				stale3m = append(stale3m, no)
			} else if diff > month {
				stale1m = append(stale1m, no)
			}
		}
		if len(stale3m) > 0 {
			rejects = append(rejects, fmt.Sprintf("明细 %v 发票开票时间 > 3 个月 (规则 8-4)", uniqueInts(stale3m)))
		}
		if len(stale1m) > 0 {
			warnings = append(warnings, fmt.Sprintf("明细 %v 发票开票时间 1-3 个月, 需酌情核 (规则 8-4)", uniqueInts(stale1m)))
		}
	}

	// 规则 10: 部分明细无票且不豁免 → reject (有票明细的规则 8 已上面跑过)
	if len(noInvoiceReject) > 0 {
		rejects = append(rejects, fmt.Sprintf("明细 %v 无发票, 不属于豁免 (规则 10: 仅研发样品/出差补贴/私车公用/特殊截图说明 豁免)", uniqueInts(noInvoiceReject)))
	}

	return rejects, warnings
}

// ruleHealthExam 规则 14: 健康证及体检 (跑哥 2026-06-11)
// 触发: detail.feeTypeId = 健康证及体检
// 判定: ① 明细金额 ≤ ¥100  ② 发票开票时间距单据提交 ≤ 6 个月 (本类型不走规则 8-4 的 3 个月通用线)
// 两条都满足通过, 否则建议驳回。无票场景由规则 10 兜底 (体检不在豁免清单)。
func (h *DashboardHandler) ruleHealthExam(raw map[string]interface{}, flowID string) []string {
	details, _ := raw["details"].([]interface{})
	type hit struct {
		id     string
		no     int
		amount float64
	}
	var hits []hit
	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		if ft, _ := dm["feeTypeId"].(string); ft != healthExamFeeTypeID {
			continue
		}
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		did, _ := form["detailId"].(string)
		no := 0
		if n, ok := form["detailNo"].(float64); ok {
			no = int(n)
		}
		var amt float64
		if a, ok := form["amount"].(map[string]interface{}); ok {
			if s, ok := a["standard"].(string); ok {
				amt, _ = strconv.ParseFloat(s, 64)
			}
		}
		hits = append(hits, hit{did, no, amt})
	}
	if len(hits) == 0 {
		return nil
	}

	var rejects []string
	for _, ht := range hits {
		// 金额解析失败不静默放行 (沿用规则 5 二审定的"偏严"先例)
		if ht.amount <= 0 {
			rejects = append(rejects, fmt.Sprintf("明细#%d 健康证及体检金额无法识别 (规则 14)", ht.no))
			continue
		}
		// 跑哥规则: ≤100 通过, 100.01 起驳回 (不留容差)
		if ht.amount > 100 {
			rejects = append(rejects, fmt.Sprintf("明细#%d 健康证及体检 ¥%.2f > 标准 ¥100 (规则 14)", ht.no, ht.amount))
		}
	}

	// 发票开票时间 ≤ 6 个月 (距单据提交)
	submitDate := int64(0)
	if v, ok := raw["submitDate"].(float64); ok {
		submitDate = int64(v)
	}
	if submitDate > 0 && flowID != "" {
		idSet := map[string]int{}
		for _, ht := range hits {
			if ht.id != "" {
				idSet[ht.id] = ht.no
			}
		}
		if len(idSet) > 0 {
			rows, err := h.DB.Query(`SELECT IFNULL(detail_id,''), IFNULL(invoice_date,0) FROM hesi_flow_invoice WHERE flow_id=?`, flowID)
			if err != nil {
				// 查库失败 → 6 个月检查跳过, 但留日志别静默 (今天的主题: 静默失败要吵出来)
				log.Printf("[hesi-audit] 规则14 查发票失败 flow=%s: %v (开票时限检查跳过)", flowID, err)
			}
			if err == nil {
				defer rows.Close()
				const month = int64(30 * 24 * 3600 * 1000)
				var stale []int
				for rows.Next() {
					var did string
					var date int64
					if rows.Scan(&did, &date) != nil {
						continue
					}
					no, ok := idSet[did]
					if !ok || date == 0 {
						continue
					}
					if submitDate-date > 6*month {
						stale = append(stale, no)
					}
				}
				if len(stale) > 0 {
					rejects = append(rejects, fmt.Sprintf("明细 %v 健康证及体检发票开票时间 > 6 个月 (规则 14)", uniqueInts(stale)))
				}
			}
		}
	}
	return rejects
}

// 专票类型集合 (规则 15-1.2: 非专票才要求付款截图; 樊雪娇口径"非专用发票=专票不要")
var specialInvoiceTypes = map[string]bool{
	"FULL_DIGITAl_SPECIAL": true, "DIGITAL_SPECIAL": true, "PAPER_SPECIAL": true, "SPECIAL": true,
}

// 规则 15-2 豁免类型: 补贴/样品/业务宣传费/私车公用 不要求 付款金额=发票金额
var offlineAmountExempt = map[string]bool{
	"ID01Fk0MQBAAQ7": true, "ID01Fk0MQBAB6D": true, "ID01FkA9pR8zGT": true, // 补贴类
	"ID01Fk0FsIqhDV": true, "ID01KhLrRhAWp9": true, "ID01Fk0B0hMfF5": true, // 样品类
	"ID01MBFnQppBWn": true, // 业务宣传费
	"ID01Fr2mX8KP2T": true, // 私车公用
}

// ruleOfflineExtras 规则 15: 线下(世创/世用)专属 (樊雪娇 2026-06-11 口径)
// 15-1.2 非专票(含电子铁路票/行程单)的明细必须传"付款截图"附件
// 15-2   除豁免类型外, 明细金额必须 = 该明细发票合计 (一分不差)
// 15-3   出差补贴: 集团经理及以上(含集团总监) 120/天(50餐+70交通), 其他 100/天(50餐+50交通);
//
//	当天有私车公用的, 该天只算 50 餐补 (花名册无"大区经理"档, 大区经理归集团经理档)
//
// 15-4   私车公用(整单聚合): 全部私车明细的油费发票合计 ≥ 私车报销总额, 否则驳回
//
//	(樊雪娇口径"超过总私车公用报销金额的油费发票即可"; 6/12 从按明细判改成整单判, 防误驳)
func (h *DashboardHandler) ruleOfflineExtras(raw map[string]interface{}, flowID, submitterID string) ([]string, []string) {
	var rejects, warnings []string
	details, _ := raw["details"].([]interface{})

	type det struct {
		no         int
		feeTypeID  string
		amount     float64
		hasPayShot bool
		dates      []string // feeDate / feeDatePeriod 覆盖的日期 (yyyy-mm-dd)
		days       float64  // u_天数 (补贴明细)
		subsidyAmt float64  // u_出差补贴金额
		driveRecID string   // 私车公用: 行车记录实例ID (消费日按行车记录时间算, 条2)
	}
	byID := map[string]det{}
	tollDetailIDs := map[string]bool{} // 过路费明细 detail_id → 后面查发票通行日期 (条3)
	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		var dt det
		dt.feeTypeID, _ = dm["feeTypeId"].(string)
		if n, ok := form["detailNo"].(float64); ok {
			dt.no = int(n)
		}
		if a, ok := form["amount"].(map[string]interface{}); ok {
			if s, ok := a["standard"].(string); ok {
				dt.amount, _ = strconv.ParseFloat(s, 64)
			}
		}
		if v, ok := form["u_付款截图"].([]interface{}); ok && len(v) > 0 {
			dt.hasPayShot = true
		} else if s, ok := form["u_付款截图"].(string); ok && s != "" {
			dt.hasPayShot = true
		}
		dt.dates = extractFeeDates(form)
		if v, ok := form["u_天数"].(string); ok {
			dt.days, _ = strconv.ParseFloat(v, 64)
		}
		if a, ok := form["u_出差补贴金额"].(map[string]interface{}); ok {
			if s, ok := a["standard"].(string); ok {
				dt.subsidyAmt, _ = strconv.ParseFloat(s, 64)
			}
		}
		// 私车公用: 消费日按行车记录时间算 (条2), 记行车记录实例ID
		if dt.feeTypeID == driveFeeTypeID {
			dt.driveRecID, _ = form["u_行车记录"].(string)
		}
		if did, ok := form["detailId"].(string); ok && did != "" {
			byID[did] = dt
			if dt.feeTypeID == tollFeeTypeID {
				tollDetailIDs[did] = true // 过路费明细, 查发票通行日期 (条3)
			}
		}
	}
	if len(byID) == 0 {
		return nil, nil
	}

	// 拉发票: detail_id → (合计金额, 是否全专票, 非专票存在); 顺带取过路费发票通行日期 (条3)
	invSum := map[string]float64{}
	hasNonSpecial := map[string]bool{}
	tollDayByDetail := map[string][]string{} // 过路费明细 → 发票通行日期 (yyyy-mm-dd)
	if flowID != "" {
		rows, err := h.DB.Query(`SELECT IFNULL(detail_id,''), IFNULL(invoice_type,''),
			IFNULL(total_amount,0), IFNULL(approve_amount,0), IFNULL(toll_pass_start,0) FROM hesi_flow_invoice WHERE flow_id=?`, flowID)
		if err == nil {
			defer rows.Close()
			invoiceReadBroken := false
			for rows.Next() {
				var did, itype string
				var total, approve float64
				var tollStart int64
				if err := rows.Scan(&did, &itype, &total, &approve, &tollStart); err != nil {
					if !invoiceReadBroken { // 只记首条, 防 DB 故障时每行刷一条日志
						log.Printf("[hesi-audit] 规则15 发票行解析失败 flow=%s: %v", flowID, err)
					}
					invoiceReadBroken = true
					continue
				}
				amt := total
				if amt == 0 {
					amt = approve
				}
				invSum[did] += amt
				if itype != "" && !specialInvoiceTypes[itype] {
					hasNonSpecial[did] = true
				}
				if tollStart > 1e12 && tollDetailIDs[did] {
					tollDayByDetail[did] = append(tollDayByDetail[did], time.UnixMilli(tollStart).Format("2006-01-02"))
				}
			}
			if err := rows.Err(); err != nil {
				invoiceReadBroken = true
				log.Printf("[hesi-audit] 规则15 发票读取中断 flow=%s: %v", flowID, err)
			}
			// invSum 不完整时 15-2/15-4 金额对碰必判错 (合计偏低 → 误驳合法单), 整体降级转人工
			if invoiceReadBroken {
				return nil, []string{"发票数据读取不完整, 规则 15 未自动判定, 需人工核"}
			}
		} else {
			log.Printf("[hesi-audit] 规则15 查发票失败 flow=%s: %v", flowID, err)
			return nil, []string{"发票数据读取失败, 规则 15 未自动判定, 需人工核"}
		}
	}

	// 扣交通补的天 = (私车日 ∪ 过路日) ∩ 出差申请单范围:
	//   私车日按行车记录时间(条2), 过路日按发票通行日期(条3), 只扣落在出差申请单期间的(条4)。
	driveTollDaySet := map[string]bool{}
	for _, dt := range byID {
		if dt.feeTypeID != driveFeeTypeID {
			continue
		}
		if dt.driveRecID != "" {
			if rec := h.LookupDriveRecord(dt.driveRecID); rec != nil && rec.StartTime > 1e12 {
				driveTollDaySet[time.UnixMilli(rec.StartTime).Format("2006-01-02")] = true
				continue
			}
		}
		// 行车记录拉不到(缓存冷/无记录)→ 兜底用明细自填日期, 不漏扣 (安全偏严)
		for _, d := range dt.dates {
			driveTollDaySet[d] = true
		}
	}
	for _, days := range tollDayByDetail { // 过路费: 发票通行日期
		for _, d := range days {
			driveTollDaySet[d] = true
		}
	}
	tripSpans := h.tripDateSpans(raw)
	deductDaySet := map[string]bool{}
	for d := range driveTollDaySet {
		// 有出差申请单时只扣落在出差期间的; 没关联/拉不到申请单时保守沿用旧行为(都扣, 偏严不放过)
		if len(tripSpans) == 0 || inAnySpan(d, tripSpans) {
			deductDaySet[d] = true
		}
	}

	// 职级 → 线下补贴日标准
	position := ""
	if submitterID != "" {
		_ = h.DB.QueryRow(`SELECT IFNULL(position,'') FROM hesi_employee_contract_company WHERE hesi_staff_id = ? LIMIT 1`, submitterID).Scan(&position)
	}
	transport := 50.0 // 其他员工 50餐+50交通
	if position == "集团经理" || position == "集团总监" || position == "副总裁" || position == "总裁" {
		transport = 70.0 // 集团经理及以上(大区经理归此档) 50餐+70交通
	}
	posKnown := position != ""

	for did, dt := range byID {
		// 15-1.2 非专票必须传付款截图 — 交通及差旅费类型豁免 (财务 6/12: 铁路票/行程单
		// 本来就是差旅票, 只有差旅之外的费用类型才要求付款截图)
		if hasNonSpecial[did] && !dt.hasPayShot && travelExpenseFeeTypes[dt.feeTypeID] == "" {
			rejects = append(rejects, fmt.Sprintf("明细#%d 含非专用发票(普票等), 须在'付款截图'上传付款凭证等附件 (线下规则 15-1.2)", dt.no))
		}
		// 15-2 付款金额 = 发票合计 (一分不差, 豁免: 补贴/样品/业务宣传/私车公用)
		if !offlineAmountExempt[dt.feeTypeID] && invSum[did] > 0 && math.Abs(invSum[did]-dt.amount) > 0.005 {
			rejects = append(rejects, fmt.Sprintf("明细#%d 金额 ¥%.2f ≠ 发票合计 ¥%.2f, 线下要求一分不差 (规则 15-2)", dt.no, dt.amount, invSum[did]))
		}
		// 15-3 出差补贴线下口径
		if dt.feeTypeID == "ID01Fk0MQBAAQ7" && dt.subsidyAmt > 0 && dt.days > 0 {
			days := math.Ceil(dt.days)
			// 私车公用日只剩餐补: 补贴明细自带日期时, 只数落在补贴期间内的私车日;
			// 没填日期时退回全单私车日数 (单行程单据的旧行为) — 规则15二审修
			deductDays := float64(len(deductDaySet))
			if len(dt.dates) > 0 {
				n := 0
				for _, d := range dt.dates {
					if deductDaySet[d] {
						n++
					}
				}
				deductDays = float64(n)
			}
			if deductDays > days {
				deductDays = days
			}
			cap := 50*days + transport*(days-deductDays) // 私车/过路且在出差申请单内的天只剩50餐补(扣交通补)
			msg := fmt.Sprintf("明细#%d 出差补贴 ¥%.2f > 线下标准 ¥%.2f (50餐补×%.0f天 + %.0f交通补×%.0f天, 私车/过路 %.0f 天无交通补, 规则 15-3)",
				dt.no, dt.subsidyAmt, cap, days, transport, days-deductDays, deductDays)
			if dt.subsidyAmt > cap+0.005 {
				if posKnown {
					rejects = append(rejects, msg)
				} else {
					warnings = append(warnings, msg+" [花名册未匹配职级, 按其他员工档算, 请人工核]")
				}
			}
		}
	}

	// 15-4 私车公用(整单聚合, 6/12 修): 樊雪娇原话"提供超过**总**私车公用报销金额的油费
	// 发票即可" — 油费发票通常一张大票挂一条明细管整月, 按明细判会大面积误驳 (dry-run 实证)
	driveAmtSum, driveInvSum := 0.0, 0.0
	hasDrive := false
	for did, dt := range byID {
		if dt.feeTypeID == driveFeeTypeID {
			hasDrive = true
			driveAmtSum += dt.amount
			driveInvSum += invSum[did]
		}
	}
	if hasDrive && driveInvSum+0.005 < driveAmtSum {
		rejects = append(rejects, fmt.Sprintf("私车公用合计 ¥%.2f, 油费发票合计仅 ¥%.2f, 须 ≥ 总报销金额 (规则 15-4)", driveAmtSum, driveInvSum))
	}

	return rejects, warnings
}

// tripDateSpans 从 expenseLinks 关联的出差申请单取出差起止日期区间 (yyyy-mm-dd, 可能多张);
// 供规则 15-3 判断私车/过路消费日是否落在出差期间 (与规则 17 ruleSubsidyDateInTrip 同源逻辑)。
func (h *DashboardHandler) tripDateSpans(raw map[string]interface{}) [][2]string {
	links, _ := raw["expenseLinks"].([]interface{})
	var spans [][2]string
	for _, l := range links {
		lid, _ := l.(string)
		if lid == "" || h.DB == nil {
			continue
		}
		var lraw string
		if err := h.DB.QueryRow(`SELECT IFNULL(raw_json,'') FROM hesi_flow WHERE flow_id=? LIMIT 1`, lid).Scan(&lraw); err != nil || lraw == "" {
			continue
		}
		var rm map[string]interface{}
		if json.Unmarshal([]byte(lraw), &rm) != nil {
			continue
		}
		tp, _ := rm["u_出差起止日期"].(map[string]interface{})
		if tp == nil {
			continue
		}
		s, _ := tp["start"].(float64)
		e, _ := tp["end"].(float64)
		if s > 1e12 && e >= s {
			spans = append(spans, [2]string{
				time.UnixMilli(int64(s)).Format("2006-01-02"),
				time.UnixMilli(int64(e)).Format("2006-01-02"),
			})
		}
	}
	return spans
}

// inAnySpan 日期 d (yyyy-mm-dd 定长, 可直接字典序比较) 是否落在任一区间内。
func inAnySpan(d string, spans [][2]string) bool {
	for _, sp := range spans {
		if d >= sp[0] && d <= sp[1] {
			return true
		}
	}
	return false
}

// extractFeeDates 从明细 form 提取 feeDate / feeDatePeriod 覆盖的日期 (yyyy-mm-dd)
// 起止须是合法毫秒时间戳且区间 ≤370 天, 防脏数据把循环跑飞 (规则15二审守卫, 规则17复用)
func extractFeeDates(form map[string]interface{}) []string {
	var dates []string
	if fd, ok := form["feeDate"].(float64); ok && fd > 1e12 {
		dates = append(dates, time.UnixMilli(int64(fd)).Format("2006-01-02"))
	}
	if fp, ok := form["feeDatePeriod"].(map[string]interface{}); ok {
		s, _ := fp["start"].(float64)
		e, _ := fp["end"].(float64)
		if s > 1e12 && e >= s && e-s <= 370*86400000 {
			for t := int64(s); t <= int64(e); t += 86400000 {
				dates = append(dates, time.UnixMilli(t).Format("2006-01-02"))
			}
		}
	}
	return dates
}

// ruleSubsidyDateInTrip 规则 17: 出差补贴日期须落在关联出差申请单的起止日期内 (财务 2026-06-12)
// 补贴明细 (出差补贴 ID01Fk0MQBAAQ7) 的日期 vs 关联申请单 u_出差起止日期 [start,end]:
//   - feeDate 单日: 必须被任一申请单覆盖, 否则建议驳回 (出差那天没申请单)
//   - feeDatePeriod 区间: 业务员按月报销惯例填"整月"(dry-run 实查 5/1~5/31 大量存在),
//     逐天判会大面积误杀 → 只要求区间与任一申请单有交集, 完全不沾边才驳回
//
// 明细没填日期 / 关联申请单都没有起止日期 → 不判 (关联缺失本身由规则 7-1 管)
func (h *DashboardHandler) ruleSubsidyDateInTrip(raw map[string]interface{}) []string {
	details, _ := raw["details"].([]interface{})
	type sub struct {
		no           int
		singleDates  []string // feeDate 单日
		pStart, pEnd string   // feeDatePeriod 区间 (空=没填)
	}
	var subs []sub
	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		if ft, _ := dm["feeTypeId"].(string); ft != "ID01Fk0MQBAAQ7" {
			continue
		}
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		var sb sub
		if n, ok := form["detailNo"].(float64); ok {
			sb.no = int(n)
		}
		if fd, ok := form["feeDate"].(float64); ok && fd > 1e12 {
			sb.singleDates = append(sb.singleDates, time.UnixMilli(int64(fd)).Format("2006-01-02"))
		}
		if fp, ok := form["feeDatePeriod"].(map[string]interface{}); ok {
			s, _ := fp["start"].(float64)
			e, _ := fp["end"].(float64)
			if s > 1e12 && e >= s {
				sb.pStart = time.UnixMilli(int64(s)).Format("2006-01-02")
				sb.pEnd = time.UnixMilli(int64(e)).Format("2006-01-02")
			}
		}
		if len(sb.singleDates) > 0 || sb.pStart != "" {
			subs = append(subs, sb)
		}
	}
	if len(subs) == 0 {
		return nil
	}

	// 关联出差申请单的起止区间 (天粒度字符串, 可能多张申请单取并集)
	links, _ := raw["expenseLinks"].([]interface{})
	type span struct{ code, start, end string }
	var spans []span
	for _, l := range links {
		lid, _ := l.(string)
		if lid == "" {
			continue
		}
		var code, lraw string
		if err := h.DB.QueryRow(`SELECT code, IFNULL(raw_json,'') FROM hesi_flow WHERE flow_id=? LIMIT 1`, lid).Scan(&code, &lraw); err != nil || lraw == "" {
			continue
		}
		var rm map[string]interface{}
		if json.Unmarshal([]byte(lraw), &rm) != nil {
			continue
		}
		tp, _ := rm["u_出差起止日期"].(map[string]interface{})
		if tp == nil {
			continue
		}
		s, _ := tp["start"].(float64)
		e, _ := tp["end"].(float64)
		if s > 1e12 && e >= s {
			spans = append(spans, span{code,
				time.UnixMilli(int64(s)).Format("2006-01-02"),
				time.UnixMilli(int64(e)).Format("2006-01-02")})
		}
	}
	if len(spans) == 0 {
		return nil
	}

	spanDesc := func() string {
		var rngs []string
		for _, sp := range spans {
			rngs = append(rngs, fmt.Sprintf("%s(%s~%s)", sp.code, sp.start, sp.end))
		}
		return strings.Join(rngs, "、")
	}

	var rejects []string
	for _, sb := range subs {
		// 单日: 必须被某申请单覆盖
		var outside []string
		for _, d := range sb.singleDates {
			covered := false
			for _, sp := range spans {
				if d >= sp.start && d <= sp.end {
					covered = true
					break
				}
			}
			if !covered {
				outside = append(outside, d)
			}
		}
		if len(outside) > 0 {
			rejects = append(rejects, fmt.Sprintf("明细#%d 补贴日期 %s 不在出差申请单 %s 起止日期内 (规则 17)",
				sb.no, strings.Join(outside, "/"), spanDesc()))
		}
		// 区间: 与所有申请单都无交集才算超范围
		if sb.pStart != "" {
			overlap := false
			for _, sp := range spans {
				if sb.pStart <= sp.end && sb.pEnd >= sp.start {
					overlap = true
					break
				}
			}
			if !overlap {
				rejects = append(rejects, fmt.Sprintf("明细#%d 补贴期间 %s~%s 与出差申请单 %s 起止日期完全不重叠 (规则 17)",
					sb.no, sb.pStart, sb.pEnd, spanDesc()))
			}
		}
	}
	return rejects
}

// 广告费 fee_type (规则 18; 旧版已停用 ID 兜底历史单)
var adFeeTypes = map[string]bool{
	"ID01MBFo8YEVQ3": true, // 广告费 (启用中, 父类=广告宣传费用)
	"ID01KhLrRhAVl5": true, // 广告费 (旧, 已停用)
}

// adInvRef 广告费明细关联的一张发票 (规则 18 判定输入)
type adInvRef struct {
	no        int    // 明细号
	invoiceID string // 合思发票 ID
	number    string // 发票号码 (提示里显示尾号)
}

// checkAdInvoiceItems 规则 18 纯判定: 发票货物明细行名称全部不含"广告/推广" → 建议驳回
// names: invoiceID → 该发票所有明细行项目名称; 接口没回明细的发票不判 (不冤枉)
func checkAdInvoiceItems(invs []adInvRef, names map[string][]string) []string {
	var rejects []string
	for _, iv := range invs {
		items := names[iv.invoiceID]
		if len(items) == 0 {
			continue
		}
		hit := false
		for _, n := range items {
			if strings.Contains(n, "广告") || strings.Contains(n, "推广") {
				hit = true
				break
			}
		}
		if !hit {
			show := strings.Join(items, "、")
			if r := []rune(show); len(r) > 40 {
				show = string(r[:40]) + "…"
			}
			tail := iv.number
			if len(tail) > 8 {
				tail = tail[len(tail)-8:]
			}
			rejects = append(rejects, fmt.Sprintf("明细#%d 广告费发票(尾号%s)项目名称「%s」不含'广告/推广' (规则 18)", iv.no, tail, show))
		}
	}
	return rejects
}

// checkNonAdInvoiceItems 规则 18 反向 (樊雪娇 2026-06-17, 跑哥严格口径):
// 非广告费明细的发票货物行名称含"广告/推广" → 费用类型与发票不符, 应报广告费, 建议驳回。
// 跑哥拍板严格: 业务宣传费等同家族类型也驳 (广告/推广发票只能报广告费)。
func checkNonAdInvoiceItems(invs []adInvRef, names map[string][]string) []string {
	var rejects []string
	for _, iv := range invs {
		items := names[iv.invoiceID]
		if len(items) == 0 {
			continue
		}
		hitName := ""
		for _, n := range items {
			if strings.Contains(n, "广告") || strings.Contains(n, "推广") {
				hitName = n
				break
			}
		}
		if hitName == "" {
			continue
		}
		show := hitName
		if r := []rune(show); len(r) > 40 {
			show = string(r[:40]) + "…"
		}
		tail := iv.number
		if len(tail) > 8 {
			tail = tail[len(tail)-8:]
		}
		rejects = append(rejects, fmt.Sprintf("明细#%d 非广告费发票(尾号%s)项目名称含'广告/推广'(「%s」), 费用类型应为广告费 (规则 18 反向)", iv.no, tail, show))
	}
	return rejects
}

// ruleAdInvoiceItemName 规则 18: 广告费 ⟺ 发票项目名含"广告/推广" 双向校验 (财务 2026-06-12; 樊雪娇 2026-06-17 加反向)
// 正向: 广告费明细配"*印刷品*KT板立牌"发票 → 项目名与费用类型不符, 建议驳回。
// 反向(跑哥严格口径): 非广告费明细的发票项目名含"广告/推广" → 应报广告费, 建议驳回(业务宣传费也驳)。
// 发票货物明细行名称本地没存, 现场调合思接口 (5min 缓存); 拉取失败时正向转人工核, 反向不冤枉(跳过)
func (h *DashboardHandler) ruleAdInvoiceItemName(raw map[string]interface{}, flowID string) ([]string, []string) {
	details, _ := raw["details"].([]interface{})
	// detailId → (明细号, 是否广告费); 广告费走正向, 其余走反向
	type adMeta struct {
		no   int
		isAd bool
	}
	detailMap := map[string]adMeta{}
	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		ft, _ := dm["feeTypeId"].(string)
		isAd := adFeeTypes[ft]
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		did, _ := form["detailId"].(string)
		no := 0
		if n, ok := form["detailNo"].(float64); ok {
			no = int(n)
		}
		if did != "" {
			detailMap[did] = adMeta{no, isAd}
		}
	}
	if len(detailMap) == 0 || flowID == "" {
		return nil, nil
	}

	rows, err := h.DB.Query(`SELECT IFNULL(detail_id,''), IFNULL(invoice_id,''), IFNULL(invoice_number,'')
		FROM hesi_flow_invoice WHERE flow_id=?`, flowID)
	if err != nil {
		log.Printf("[hesi-audit] 规则18 查发票失败 flow=%s: %v", flowID, err)
		return nil, nil
	}
	defer rows.Close()
	var adInvs, nonAdInvs []adInvRef // 广告费明细发票(正向) / 非广告费明细发票(反向)
	for rows.Next() {
		var did, iid, num string
		if rows.Scan(&did, &iid, &num) != nil {
			continue
		}
		meta, ok := detailMap[did]
		if !ok || iid == "" {
			continue
		}
		if meta.isAd {
			adInvs = append(adInvs, adInvRef{meta.no, iid, num})
		} else {
			nonAdInvs = append(nonAdInvs, adInvRef{meta.no, iid, num})
		}
	}
	if len(adInvs) == 0 && len(nonAdInvs) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(adInvs)+len(nonAdInvs))
	for _, iv := range adInvs {
		ids = append(ids, iv.invoiceID)
	}
	for _, iv := range nonAdInvs {
		ids = append(ids, iv.invoiceID)
	}
	names, err := h.fetchInvoiceItemNames(ids)
	if err != nil {
		log.Printf("[hesi-audit] 规则18 拉发票明细失败 flow=%s: %v", flowID, err)
		// 拉不到: 有广告费发票才提示正向人工核; 反向(非广告费)拉不到不冤枉(跳过)
		if len(adInvs) > 0 {
			return nil, []string{"广告费发票项目名称暂时拉不到, 请人工核是否含'广告/推广' (规则 18)"}
		}
		return nil, nil
	}
	rejects := checkAdInvoiceItems(adInvs, names)                          // 正向: 广告费发票须含广告/推广
	rejects = append(rejects, checkNonAdInvoiceItems(nonAdInvs, names)...) // 反向: 非广告费发票不得含广告/推广
	return rejects, nil
}

// ruleCorpPaidDuplicate 规则 16: 企业支付行程防重复报销 (跑哥 2026-06-11; 樊雪娇 2026-06-17 收窄)
// 报销单关联的出差申请单下, 公司已"企业支付"买好的火车/飞机票 (合思商旅订单),
// 若同车次+同乘车人又出现在本单报销发票里:
//
//	票价也一致 (订单"票面价"=发票金额) → 公司已付的票疑似二次报销, 建议驳回
//	票价不一致 → 不是同一张票, 不算重复报销, 直接通过 (樊雪娇 2026-06-17: 这条只防重复, 不转人工)
//
// 数据校准 (2026-06-11 实查): 企业支付金额 = 票面价 + ~3元服务费, 对碰必须用票面价;
// 商旅订单没有可用出发日期列, 靠"只看本单关联申请单"收口, 不跨行程乱碰
func (h *DashboardHandler) ruleCorpPaidDuplicate(raw map[string]interface{}, flowID string) ([]string, []string) {
	links, _ := raw["expenseLinks"].([]interface{})
	if len(links) == 0 || flowID == "" {
		return nil, nil
	}
	// 关联申请单号 (expenseLinks 存的是申请单 flow_id, 换 code)
	var codes []interface{}
	for _, l := range links {
		lid, _ := l.(string)
		if lid == "" {
			continue
		}
		var code string
		if err := h.DB.QueryRow(`SELECT code FROM hesi_flow WHERE flow_id=? LIMIT 1`, lid).Scan(&code); err == nil && code != "" {
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		return nil, nil
	}

	// 申请单下的企业支付车票订单 (火车/飞机才有车次; 退票/退订=公司没掏成钱, 不算)
	// 状态实查 (2026-06-11): 确认订单/出票/已完成/改签/退订/退票 六种
	type corpOrder struct {
		tripNo, traveler, reqCode string
		fare                      float64 // 票面价 (= 发票金额口径); 拿不到时退回企业支付金额
		used                      bool
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(codes)), ",")
	rows, err := h.DB.Query(`SELECT IFNULL(trip_no,''), IFNULL(traveler,''), IFNULL(req_code,''),
		IFNULL(corp_pay,0), IFNULL(raw_json,'') FROM hesi_travel_order
		WHERE req_code IN (`+ph+`) AND pay_method='企业支付' AND trip_no<>'' AND order_state NOT IN ('退票','退订')`, codes...)
	if err != nil {
		log.Printf("[hesi-audit] 规则16 查商旅订单失败 flow=%s: %v", flowID, err)
		return nil, nil
	}
	var orders []*corpOrder
	for rows.Next() {
		var o corpOrder
		var corpPay float64
		var rawJSON string
		if rows.Scan(&o.tripNo, &o.traveler, &o.reqCode, &corpPay, &rawJSON) != nil {
			continue
		}
		o.fare = corpPay
		if rawJSON != "" {
			var om map[string]interface{}
			if json.Unmarshal([]byte(rawJSON), &om) == nil {
				if v, ok := getStandardAmount(om["票面价"]); ok && v > 0 {
					o.fare = v
				}
			}
		}
		if o.tripNo != "" {
			orders = append(orders, &o)
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("[hesi-audit] 规则16 读商旅订单中断 flow=%s: %v", flowID, err)
	}
	rows.Close()
	if len(orders) == 0 {
		return nil, nil
	}

	// detailId → detailNo (提示里标明细号)
	detailNo := map[string]int{}
	if details, ok := raw["details"].([]interface{}); ok {
		for _, d := range details {
			dm, _ := d.(map[string]interface{})
			if dm == nil {
				continue
			}
			form, _ := dm["feeTypeForm"].(map[string]interface{})
			if form == nil {
				continue
			}
			did, _ := form["detailId"].(string)
			if n, ok := form["detailNo"].(float64); ok && did != "" {
				detailNo[did] = int(n)
			}
		}
	}

	// 本单带车次的发票 (火车票 OCR 有车次/乘车人)
	ivRows, err := h.DB.Query(`SELECT IFNULL(detail_id,''), IFNULL(train_no,''), IFNULL(passenger,''),
		IFNULL(total_amount,0) FROM hesi_flow_invoice WHERE flow_id=? AND IFNULL(train_no,'')<>''`, flowID)
	if err != nil {
		log.Printf("[hesi-audit] 规则16 查发票失败 flow=%s: %v", flowID, err)
		return nil, nil
	}
	defer ivRows.Close()

	var rejects, warnings []string
	ticketReadBroken := false
	for ivRows.Next() {
		var did, trainNo, passenger string
		var amt float64
		if err := ivRows.Scan(&did, &trainNo, &passenger, &amt); err != nil {
			if !ticketReadBroken { // 只记首条, 防 DB 故障时每行刷一条日志
				log.Printf("[hesi-audit] 规则16 发票行解析失败 flow=%s: %v", flowID, err)
			}
			ticketReadBroken = true
			continue
		}
		if trainNo == "" || passenger == "" {
			continue
		}
		// OCR 的乘车人可能带空格 ("郑 华坤"), 去空格再比
		passenger = strings.ReplaceAll(passenger, " ", "")
		// 樊雪娇 2026-06-17: 这条只防重复报销, 只认 同车次+同乘车人+"同票价"(票面价=发票金额);
		// 票价不同 = 不是同一张票 = 不算重复, 直接通过 (不再转人工核)。
		// 关键: 只在"同票价"命中时才标记订单 used —— 票价不同的不消耗订单, 否则会"用掉"订单,
		// 害得后面真正同票价的重复发票匹配不到而漏判 (削弱防重复本身)。
		var hit *corpOrder
		for _, o := range orders {
			if o.used || o.tripNo != trainNo || strings.ReplaceAll(o.traveler, " ", "") != passenger {
				continue
			}
			if math.Abs(o.fare-amt) <= 0.005 { // 只认同票价
				hit = o
				break
			}
		}
		if hit == nil {
			continue // 没有同车次+同人+同票价的企业支付票 = 不是重复报销, 通过 (票价不同走这里)
		}
		hit.used = true
		no := detailNo[did]
		rejects = append(rejects, fmt.Sprintf("明细#%d 车次 %s (乘车人%s ¥%.2f) 在出差申请单 %s 里公司已企业支付同车次同票价的票, 疑似已付票二次报销 (规则 16)",
			no, trainNo, passenger, amt, hit.reqCode))
	}
	if err := ivRows.Err(); err != nil {
		ticketReadBroken = true
		log.Printf("[hesi-audit] 规则16 发票读取中断 flow=%s: %v", flowID, err)
	}
	// 车票数据不完整只会漏判 (不会误驳), 保留已出结果, 补一条人工核提示
	if ticketReadBroken {
		warnings = append(warnings, "车票发票数据读取不完整, 规则 16 防重复报销可能漏判, 需人工核 (规则 16)")
	}
	return rejects, warnings
}

// uniqueInts 去重 + 升序排, 用于规则 8 提示里的"明细 [1 2 5]"
func uniqueInts(s []int) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Ints(out)
	return out
}

// ruleAccommodationStandard 规则 7-3: 住宿费明细单晚价 ≤ 城市×职级标准
// 跑哥规则: 一线/新一线/二线/其他城市 × 总裁/副总裁/集团总监/集团经理/主管和其他 5 档 (PDF V7.0)
// 线下(世创/世用 isOffline=true): 改用线下住宿表(3档城市×3档职级), 新一线→二线 (樊雪娇 2026-06-17)
// 同住上浮: u_是否两人同住 非空 → 上浮 20% (按职位高者标准, 简化按当前人)
// 单晚价算法: amount / 出差天数 (feeDatePeriod.end - start)/86400000 + 1
// 返回: (reject 原因, warn 原因), 任一非空即触发
func (h *DashboardHandler) ruleAccommodationStandard(raw map[string]interface{}, submitterID string, isOffline bool) (string, string) {
	details, _ := raw["details"].([]interface{})

	type hotelLine struct {
		idx       int
		amount    float64
		cityRaw   string
		days      int
		cohabit   bool
		badPeriod bool // 日期区间缺失/异常 (如 start=0), 间夜数对不准, 转人工
	}
	var lines []hotelLine
	for i, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		feeTypeID, _ := dm["feeTypeId"].(string)
		if feeTypeID != hotelFeeTypeID {
			continue
		}
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		var amt float64
		if a, ok := form["amount"].(map[string]interface{}); ok {
			if s, ok := a["standard"].(string); ok {
				amt, _ = strconv.ParseFloat(s, 64)
			}
		}
		cityRaw, _ := form["city"].(string)
		cohabit := false
		if v, ok := form["u_是否两人同住"].(string); ok && v != "" {
			cohabit = true
		}
		days := 1
		badPeriod := false
		if fp, ok := form["feeDatePeriod"].(map[string]interface{}); ok {
			start, _ := fp["start"].(float64)
			end, _ := fp["end"].(float64)
			// start 必须是合法毫秒时间戳(>1e12) + 区间封顶 370 天, 跟 extractFeeDates 同口径
			// 否则 (如缺起始日期 start=0, end 正常) 会按 end-0 算出 2 万+天, 标准上限被撑爆 → 超标住宿永不触发
			if start > 1e12 && end >= start && end-start <= 370*86400000 {
				days = int((end-start)/86400000) + 1
			} else {
				badPeriod = true
			}
		}
		lines = append(lines, hotelLine{i + 1, amt, cityRaw, days, cohabit, badPeriod})
	}
	if len(lines) == 0 {
		return "", ""
	}

	// 查提交人岗位职级 (花名册 SSC 表)
	// codex 二审 v1.75.17 修: SSC 未匹配时按"主管和其他" cap 算, 但超额转 manual
	position := ""
	if submitterID != "" {
		_ = h.DB.QueryRow(`SELECT IFNULL(position,'') FROM hesi_employee_contract_company WHERE hesi_staff_id = ? LIMIT 1`, submitterID).Scan(&position)
	}
	positionForDisplay := position
	isFallback := false
	if position == "" {
		position = "主管和其他"
		positionForDisplay = "主管和其他 (花名册未匹配)"
		isFallback = true
	}
	var standards map[string]float64
	if isOffline {
		// 线下(世创/世用)用线下住宿表: 职级先收口到 3 档 (其他员工/大区经理及以上/集团总监)
		olevel := offlineHotelLevel(position)
		s, ok := h.loadOfflineAccomStd()[olevel]
		if !ok {
			return "", "线下住宿职级「" + olevel + "」未配置 (规则 7-3 线下)"
		}
		standards = s
		if isFallback {
			positionForDisplay = olevel + " (线下, 花名册未匹配)"
		} else {
			positionForDisplay = olevel + " (线下)"
		}
	} else {
		accomStd, _ := h.loadHesiAuditParams() // DB 配置优先, 财务改口径 5 分钟生效
		s, ok := accomStd[position]
		if !ok {
			return "", "岗位职级「" + position + "」非标准 5 档, 住宿标准未配置 (规则 7-3)"
		}
		standards = s
	}

	tierMap := h.loadCityTierCache()
	var rejectMsgs []string
	var warnMsgs []string
	for _, line := range lines {
		if line.badPeriod {
			warnMsgs = append(warnMsgs, fmt.Sprintf("住宿明细#%d 日期区间缺失/异常, 间夜数无法核算, 需人工核 (规则 7-3)", line.idx))
			continue
		}
		tier := extractTier(line.cityRaw, tierMap)
		if isOffline {
			tier = offlineCityTier(tier) // 集团 4 档 → 线下 3 档 (新一线→二线)
		}
		std := standards[tier]
		if std == 0 {
			warnMsgs = append(warnMsgs, fmt.Sprintf("住宿明细#%d 城市分级未识别 (规则 7-3)", line.idx))
			continue
		}
		cap := std * float64(line.days)
		cohabitTag := ""
		if line.cohabit {
			cap *= 1.2
			cohabitTag = " ×1.2(同住)"
		}
		if line.amount > cap {
			msg := fmt.Sprintf(
				"住宿#%d ¥%.2f > 标准 ¥%.0f×%d晚%s=¥%.2f (%s/%s, 规则 7-3)",
				line.idx, line.amount, std, line.days, cohabitTag, cap, positionForDisplay, tier,
			)
			if isFallback {
				warnMsgs = append(warnMsgs, msg+" [SSC 未匹配, 请人工核职级]")
			} else {
				rejectMsgs = append(rejectMsgs, msg)
			}
		}
	}
	return strings.Join(rejectMsgs, "; "), strings.Join(warnMsgs, "; ")
}

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

// ruleSeatManualReview 规则 7-2: 飞机/火车/汽车/客车/其他交通明细 → 需人工核座位等级
// 跑哥规则: 汽车/火车/高铁/动车二等座及以下, 飞机经济舱
// 合思 raw_json 未存舱位/座位字段, 凭票报销时只能给 manual 提示
// 数据驱动: 实查 286 单日常报销单含飞机/火车明细, 283 单 (99%) 有 u_付款截图 = 凭票报销
func ruleSeatManualReview(raw map[string]interface{}) string {
	details, _ := raw["details"].([]interface{})
	var hits []string
	seen := map[string]bool{}
	for _, d := range details {
		dm, ok := d.(map[string]interface{})
		if !ok {
			continue
		}
		feeTypeID, _ := dm["feeTypeId"].(string)
		if label, ok := seatReviewFeeTypes[feeTypeID]; ok && !seen[label] {
			hits = append(hits, label)
			seen[label] = true
		}
	}
	if len(hits) == 0 {
		return ""
	}
	return "含" + strings.Join(hits, "/") + ", 需人工核座位等级 (规则 7-2)"
}

// classifyTrainSeat 火车座位等级分类: "over"超标驳回 / "review"人工核(卧铺或未识别) / "ok"通过(二等座及以下)
func classifyTrainSeat(seat string) string {
	switch {
	case seat == "":
		return "review"
	case trainSeatOverStandard[seat]:
		return "over"
	case trainSeatNeedReview[seat]:
		return "review"
	default:
		return "ok"
	}
}

// ruleTrainSeatClass 规则 7-2 火车票座位等级自动判 (跑哥 2026-06-05: 二等座及以下合规)
// 数据来源: 合思发票主体 OCR 出的"座位类型"(回填进 hesi_flow_invoice.seat_type)。
//   - 一等座/商务座/特等座/一等卧 → 超标驳回
//   - 软卧/动卧等卧铺 → 人工核
//   - 座位类型识别不到(空) → 人工核
//   - 其余(二等座/硬座/硬卧/软座/二等卧/卧代二等座…)= 二等座及以下 → 通过(不记)
//
// 返回 (rejectMsg, warnMsg)
func (h *DashboardHandler) ruleTrainSeatClass(flowID string) (string, string) {
	if flowID == "" {
		return "", ""
	}
	rows, err := h.DB.Query(`SELECT IFNULL(seat_type,''), IFNULL(train_no,''), IFNULL(from_station,''), IFNULL(to_station,'')
		FROM hesi_flow_invoice WHERE flow_id = ? AND invoice_type LIKE '%TRAIN%'`, flowID)
	if err != nil {
		return "", ""
	}
	defer rows.Close()

	var over, review []string
	for rows.Next() {
		var seat, train, from, to string
		if rows.Scan(&seat, &train, &from, &to) != nil {
			continue
		}
		label := train
		if from != "" || to != "" {
			label = strings.TrimSpace(train + " " + from + "→" + to)
		}
		if label == "" {
			label = "火车票"
		}
		switch classifyTrainSeat(seat) {
		case "over":
			over = append(over, label+" "+seat)
		case "review":
			if seat == "" {
				review = append(review, label+" 座位类型未识别")
			} else {
				review = append(review, label+" "+seat)
			}
		}
	}

	var rej, warn string
	if len(over) > 0 {
		rej = "火车票超「二等座及以下」标准: " + strings.Join(over, "; ") + " (规则 7-2)"
	}
	if len(review) > 0 {
		warn = "火车票卧铺/座位未识别需人工核: " + strings.Join(review, "; ") + " (规则 7-2)"
	}
	return rej, warn
}
