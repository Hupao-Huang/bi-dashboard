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
// 合思后台未存舱位字段, 合思订票时源头按职级卡座位; 凭票报销时无字段判定, 给审批人 manual 提示
var seatReviewFeeTypes = map[string]string{
	"ID01Fk0IZFCaJx": "飞机 (经济舱)",
	"ID01Fk0IZFCb03": "火车 (二等座及以下)",
	"ID01Fk0IZFCbgz": "客车",
	"ID01Fk0IZFCbx5": "其他交通",
	"ID01KhLSijR88T": "汽车 (二等座及以下)",
}

// 住宿费 fee_type (规则 7-3 触发 + 单晚价提取)
const hotelFeeTypeID = "ID01Fk0STRw3p5"

// 住宿标准矩阵 (¥/晚, PDF V7.0 2026-01-23)
// 城市分级 × 职级, 同住按职位高者标准上浮 20%
var accommodationStandard = map[string]map[string]float64{
	"总裁":      {"一线": 1200, "新一线": 1000, "二线": 1000, "其他": 800},
	"副总裁":     {"一线": 1000, "新一线": 800, "二线": 800, "其他": 600},
	"集团总监":    {"一线": 500, "新一线": 400, "二线": 400, "其他": 300},
	"集团经理":    {"一线": 450, "新一线": 350, "二线": 350, "其他": 300},
	"主管和其他": {"一线": 400, "新一线": 300, "二线": 300, "其他": 300},
}

// 城市分级缓存 (5min TTL, hesi_city_tier 表 68 城市)
var (
	cityTierCache   map[string]string
	cityTierCacheAt time.Time
	cityTierCacheMu sync.Mutex
)

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
func (h *DashboardHandler) AuditDailyExpense(ownerDeptID, departmentID, submitterID string, expenseMoney float64, rawJSON string) *AuditSuggestion {
	var raw map[string]interface{}
	if rawJSON != "" {
		_ = json.Unmarshal([]byte(rawJSON), &raw)
	}

	var rejectReasons []string
	var warnings []string

	// 规则 1: 发起人部门 末级 (员工提交人部门, raw_json.u_提交人部门 = hesi_flow.owner_department)
	if r := ruleDeptLeaf(h.DB, ownerDeptID, "发起人部门 (规则 1)"); r != "" {
		rejectReasons = append(rejectReasons, r)
	}

	// 规则 2: 报销/借款部门 末级 (= 费用承担部门, raw_json.expenseDepartment = hesi_flow.department_id)
	if r := ruleDeptLeaf(h.DB, departmentID, "报销/借款部门 (规则 2)"); r != "" {
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

	// 规则 7-1: 交通及差旅费 关联出差申请单 + 金额不超
	if r := h.ruleRequisitionLink(raw, expenseMoney, travelExpenseFeeTypes, reqTripSpecPrefix, "交通及差旅费", "出差申请单", "规则 7-1"); r != "" {
		rejectReasons = append(rejectReasons, r)
	}

	// 规则 7-2: 飞机/火车/汽车明细需人工核座位等级 (合思未存舱位字段, manual 提示)
	if w := ruleSeatManualReview(raw); w != "" {
		warnings = append(warnings, w)
	}

	// 规则 7-3: 住宿费单晚价 ≤ 城市×职级标准 (同住上浮 20%)
	if rejectMsg, warnMsg := h.ruleAccommodationStandard(raw, submitterID); rejectMsg != "" || warnMsg != "" {
		if rejectMsg != "" {
			rejectReasons = append(rejectReasons, rejectMsg)
		}
		if warnMsg != "" {
			warnings = append(warnings, warnMsg)
		}
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

	if legalEntityName != contractCompany.String {
		return "所属公司「" + legalEntityName + "」≠ 合同公司「" + contractCompany.String + "」(规则 4)"
	}
	return ""
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

// ruleDeptLeaf 部门必须末级 (hesi_department.has_child = 0)
// label 用于区分调用方 (例: "提交人部门 (规则 1)")
func ruleDeptLeaf(db *sql.DB, deptID string, label string) string {
	if deptID == "" {
		return label + " 为空"
	}
	var hasChild int
	var name string
	err := db.QueryRow(`SELECT name, has_child FROM hesi_department WHERE id = ? AND active = 1 LIMIT 1`, deptID).Scan(&name, &hasChild)
	if err != nil {
		// 部门不在表 (新部门 / 未同步 / 已停用) → 跳过此规则
		return ""
	}
	if hasChild == 1 {
		return label + "「" + name + "」非末级"
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

// ruleAccommodationStandard 规则 7-3: 住宿费明细单晚价 ≤ 城市×职级标准
// 跑哥规则: 一线/新一线/二线/其他城市 × 总裁/副总裁/集团总监/集团经理/主管和其他 5 档 (PDF V7.0)
// 同住上浮: u_是否两人同住 非空 → 上浮 20% (按职位高者标准, 简化按当前人)
// 单晚价算法: amount / 出差天数 (feeDatePeriod.end - start)/86400000 + 1
// 返回: (reject 原因, warn 原因), 任一非空即触发
func (h *DashboardHandler) ruleAccommodationStandard(raw map[string]interface{}, submitterID string) (string, string) {
	details, _ := raw["details"].([]interface{})

	type hotelLine struct {
		idx     int
		amount  float64
		cityRaw string
		days    int
		cohabit bool
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
		if fp, ok := form["feeDatePeriod"].(map[string]interface{}); ok {
			start, _ := fp["start"].(float64)
			end, _ := fp["end"].(float64)
			if end > start {
				days = int((end-start)/86400000) + 1
				if days < 1 {
					days = 1
				}
			}
		}
		lines = append(lines, hotelLine{i + 1, amt, cityRaw, days, cohabit})
	}
	if len(lines) == 0 {
		return "", ""
	}

	// 查提交人岗位职级 (花名册 SSC 表)
	position := ""
	if submitterID != "" {
		_ = h.DB.QueryRow(`SELECT IFNULL(position,'') FROM hesi_employee_contract_company WHERE hesi_staff_id = ? LIMIT 1`, submitterID).Scan(&position)
	}
	if position == "" {
		return "", "提交人无岗位职级数据 (花名册未匹配), 住宿标准无法判定 (规则 7-3)"
	}
	standards, ok := accommodationStandard[position]
	if !ok {
		return "", "岗位职级「" + position + "」非标准 5 档, 住宿标准未配置 (规则 7-3)"
	}

	tierMap := h.loadCityTierCache()
	var rejectMsgs []string
	var warnMsgs []string
	for _, line := range lines {
		tier := extractTier(line.cityRaw, tierMap)
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
			rejectMsgs = append(rejectMsgs, fmt.Sprintf(
				"住宿#%d ¥%.2f > 标准 ¥%.0f×%d晚%s=¥%.2f (%s/%s, 规则 7-3)",
				line.idx, line.amount, std, line.days, cohabitTag, cap, position, tier,
			))
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
