package handler

// hesi_audit_common.go - 跨规则的通用判定与查询: 规则3收款/规则4主体/规则5·6·7-1关联申请单/规则13必填
// (2026-06-25 从 hesi_audit_rules.go 拆出, 纯挪位置不改逻辑)

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

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

// hasPaymentProofAttachment 单据附件里是否有"付款截图"类凭证 (非发票附件 + 文件名带付款/支付/转账/汇款/回单/流水/截图)。
// 樊雪娇 2026-06-18: 付款截图既可填在费用明细的"付款截图"字段, 也可作为单据附件上传, 两处认一处即可。
// 返回 err≠nil 时调用方应转人工(不自动驳), 与本模块其它 read-broken 降级一致。
func (h *DashboardHandler) hasPaymentProofAttachment(flowID string) (bool, error) {
	if h.DB == nil || flowID == "" {
		return false, nil
	}
	var n int
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM hesi_flow_attachment
		WHERE flow_id=? AND is_invoice=0 AND (
			file_name LIKE '%付款%' OR file_name LIKE '%支付%' OR file_name LIKE '%转账%'
			OR file_name LIKE '%汇款%' OR file_name LIKE '%回单%' OR file_name LIKE '%流水%'
			OR file_name LIKE '%截图%')`, flowID).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
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
