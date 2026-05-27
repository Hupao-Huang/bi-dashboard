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
	"strconv"
	"strings"
)

// 业务招待费 fee_type ID 集合 (合思后台 2026-05-27 实查)
// 父类 ID01Fk0FsIqgQn = "业务招待费", 子类: 招待费 / 礼品 / 总经办客情 / 等
var businessTreatFeeTypes = map[string]string{
	"ID01Fk0FsIqgQn": "业务招待费",
	"ID01Fk0IC65Hhd": "招待费",
	"ID01Fk0IC65HxJ": "礼品",
	"ID01KuC6LQ4oPR": "总经办客情",
	"ID01MBFnQppBWn": "业务招待子类", // 樊雪娇数据出现过, 待补名
}

// 招待费用申请单 specification_id 前缀 (合思后台 2026-05-27 实查)
const reqTreatmentSpecPrefix = "ID01FAvUKAThbV"

// 固定资产 fee_type ID (合思后台 2026-05-27 实查, 父类无子类)
var fixedAssetFeeTypes = map[string]string{
	"ID01FFN9mLHtrp": "固定资产",
}

// 固定资产申请单 specification_id 前缀
const reqFixedAssetSpecPrefix = "ID01FFO5f39taD"

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
func (h *DashboardHandler) AuditDailyExpense(ownerDeptID, departmentID string, expenseMoney float64, rawJSON string) *AuditSuggestion {
	var raw map[string]interface{}
	if rawJSON != "" {
		_ = json.Unmarshal([]byte(rawJSON), &raw)
	}

	var rejectReasons []string

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

	if len(rejectReasons) > 0 {
		return &AuditSuggestion{Action: "reject", Reasons: rejectReasons}
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
	var totalReqMoney float64
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

	if expenseMoney > totalReqMoney && totalReqMoney > 0 {
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
