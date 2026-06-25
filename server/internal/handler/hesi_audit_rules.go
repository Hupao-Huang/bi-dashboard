package handler

// 樊雪娇日常报销单 AI 审批建议规则 (2026-05-27 新建)
// 仅适用于 spec=ID01Fk3qJYYFvp prefix (日常报销单)
// 调用前必须在上游加 spec_id 过滤 + 樊雪娇审批人判定 (见 profile_hesi_pending.go)
//
// 模式: 两阶段
//   - 阶段 1 (当前): AI 建议 dry-run, 仅展示 suggestion 字段给审批人参考
//   - 阶段 2 (后续): 真自动审批, 命中 reject 直接 INSERT into hesi_approval_queue

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
)

// AuditSuggestion 审批建议输出
type AuditSuggestion struct {
	Action  string   `json:"action"` // agree / reject / manual
	Reasons []string `json:"reasons"`
}

// AuditDailyExpense 日常报销单审批规则引擎 (handler 方法, 需访问 LookupLegalEntityName 等)
// 入参:
//   - ownerDeptID: hesi_flow.owner_department = ownerDefaultDepartment 发起人员工默认部门 — 现仅作 submitDeptID 的兜底来源
//     (规则 1/10/13 统一按 raw_json.u_提交人部门 单据填的提交人部门判定, 该字段缺失时才退回此列, 2026-06-23)
//   - departmentID: hesi_flow.department_id (冻结的首次入库值) — 规则 2 改优先读 raw_json.expenseDepartment, 仅在 raw 缺失时退回此列兜底
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

	// 外币(国外)无票判定: 整单一次, 供规则10豁免E + 规则19跳过 (跑哥 2026-06-25)
	isForeign := h.isForeignFlow(flowID)

	var rejectReasons []string
	var warnings []string

	// 发起人部门: 不再审核 (跑哥 2026-06-23 撤销原"规则 1 发起人部门末级")。
	// 但 submitDeptID(单据填的提交人部门 u_提交人部门, 缺则退回员工默认部门列)仍供规则 10/13 判研发/品牌中心链。
	submitDeptID, _ := raw["u_提交人部门"].(string)
	if submitDeptID == "" {
		submitDeptID = ownerDeptID
	}

	// 规则 2: 报销/费用承担部门 不能选"公司"(法人实体) — 必须选公司下面的具体部门 (跑哥 2026-06-23 改, 原"末级"判定撤销)。
	// 读 raw_json.expenseDepartment (借款单兜底 loanDepartment, 都缺再退回 department_id 列)。
	// "公司"=合思部门树顶层节点(父=根corp), 见 companyDeptName。注意公司节点可能 has_child=0(如华鲜高新),
	// 老"末级(has_child=0)"判定会漏放行公司, 故改判"是不是公司"。
	expDeptID, _ := raw["expenseDepartment"].(string)
	if expDeptID == "" {
		expDeptID, _ = raw["loanDepartment"].(string)
	}
	if expDeptID == "" {
		expDeptID = departmentID
	}
	if name := h.companyDeptName(expDeptID); name != "" {
		rejectReasons = append(rejectReasons, "报销/费用承担部门「"+name+"」不能选公司, 请选公司下面的具体部门 (规则 2)")
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
	// 规则10 研发样品无票豁免按 submitDeptID(单据填的提交人部门) 判研发链, 与规则1同口径 (跑哥 2026-06-23)
	if invRej, invWarn := h.ruleInvoiceChecks(raw, submitDeptID, flowID, isForeign); len(invRej) > 0 || len(invWarn) > 0 {
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
	// 13-②/③ 品牌中心/研发中心判定按 submitDeptID(单据填的提交人部门), 与规则1同口径 (跑哥 2026-06-23)
	if rej, warn := h.ruleRequiredFields(raw, submitDeptID); len(rej) > 0 || len(warn) > 0 {
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

	// 规则 19: 付款截图金额 vs 发票总额核对 (口径B, 跑哥 2026-06-25)
	// 付款截图实付总额 > 发票价税合计总额 → 转人工复核 (warnings → Action "manual")。
	// Pending(截图还没OCR完) / 查询出错 → checkFlowPayment 返回 Flag=false, 此处不动判定。
	// 外币(国外)单无发票可比, 跳过规则19 (跑哥 2026-06-25)
	if !isForeign {
		if pc := h.checkFlowPayment(flowID); pc.Flag {
			warnings = append(warnings, pc.Note)
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
