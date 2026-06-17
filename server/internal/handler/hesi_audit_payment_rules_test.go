package handler

import (
	"strings"
	"testing"
)

func TestAuditPayment_EmptyRulesAgree(t *testing.T) {
	// 骨架测试: 非付款单模板 → agree (不评估)
	h := &DashboardHandler{}
	got := h.AuditPayment("F1", "ID01Other:abc", `{}`, 0)
	if got == nil || got.Action != "agree" {
		t.Fatalf("非付款单模板应 agree, got %+v", got)
	}
}

func TestPaymentTemplateType(t *testing.T) {
	if paymentTemplate("ID01KgaO6dcZtR:abc") != "payment" {
		t.Errorf("B开头 spec 应为 payment")
	}
	if paymentTemplate("ID01FhdI9II9A3:xyz") != "prepay" {
		t.Errorf("J开头 spec 应为 prepay")
	}
	if paymentTemplate("ID01Other") != "" {
		t.Errorf("其它 spec 应为空")
	}
}

// A1: 所属公司非空
func TestRulePaymentOwnerCompany(t *testing.T) {
	// 法人实体为空 → 驳回
	if rulePaymentOwnerCompany(map[string]interface{}{}) == "" {
		t.Error("法人实体为空应驳回")
	}
	// 有法人实体 → 通过
	if r := rulePaymentOwnerCompany(map[string]interface{}{"法人实体": "ID01X"}); r != "" {
		t.Errorf("有法人实体应通过, got %q", r)
	}
}

// A2: 申请部门末级 (转人工)
func TestAuditPayment_DeptLeafManual(t *testing.T) {
	// paymentDeptLeafWarn 把 ruleDeptLeaf 的非空结果原样返回 (转人工)
	msg := paymentDeptLeafWarn("非末级部门 (A2 申请部门)")
	if msg == "" {
		t.Error("非末级应产出转人工提醒")
	}
	// 空结果 (末级或不在树) → 无提醒
	if paymentDeptLeafWarn("") != "" {
		t.Error("末级应无提醒")
	}
}

// A6+A7: 防自付
func TestPaymentSelfPayDetect(t *testing.T) {
	// 收款方户名 == 所属公司名 → 驳回
	if r := selfPayReason("杭州松鲜鲜食品", "杭州松鲜鲜食品"); r == "" {
		t.Error("收款方=所属公司应驳回 (防自付)")
	}
	// 不同名 → 通过
	if r := selfPayReason("供应商A", "杭州松鲜鲜食品"); r != "" {
		t.Errorf("不同应通过, got %q", r)
	}
	// 任一为空 → 不误判
	if r := selfPayReason("", "杭州松鲜鲜食品"); r != "" {
		t.Errorf("收款方空时不应驳回, got %q", r)
	}
	if r := selfPayReason("供应商A", ""); r != "" {
		t.Errorf("公司名空时不应驳回, got %q", r)
	}
}

// A13: 阳光天际 → 悦伍公司
func TestSpecialSubject(t *testing.T) {
	// 事由含阳光天际 但所属公司名不含悦伍 → 驳回
	if r := sunshineRule("阳光天际项目款", "杭州松鲜鲜自然调味品"); r == "" {
		t.Error("阳光天际应要求悦伍公司 (A13)")
	}
	// 所属公司含悦伍 → 通过
	if r := sunshineRule("阳光天际项目款", "杭州松鲜鲜悦伍食品科技有限公司"); r != "" {
		t.Errorf("悦伍公司应通过, got %q", r)
	}
	// 事由不含阳光天际 → 通过
	if r := sunshineRule("普通供应商款", "杭州松鲜鲜自然调味品"); r != "" {
		t.Errorf("无阳光天际关键词应通过, got %q", r)
	}
}

// A18: 支付金额上限 (仅付款单)
func TestPaymentAmountCap(t *testing.T) {
	// 支付额 > 发票合计 → 转人工
	raw := map[string]interface{}{"payMoney": float64(1000)}
	if w := rulePaymentAmountCap(raw, 800); w == "" {
		t.Error("支付额超发票合计应转人工 (A18)")
	}
	// 支付额 == 发票合计 → 通过
	if w := rulePaymentAmountCap(raw, 1000); w != "" {
		t.Errorf("支付额=发票应通过, got %q", w)
	}
	// 支付额 < 发票合计 → 通过
	if w := rulePaymentAmountCap(raw, 1200); w != "" {
		t.Errorf("支付额<发票应通过, got %q", w)
	}
	// invoiceTotal = 0 (无发票) → 不误判
	if w := rulePaymentAmountCap(raw, 0); w != "" {
		t.Errorf("无发票时不应转人工, got %q", w)
	}
}

// A1 集成: AuditPayment 对空法人实体返回 reject
func TestAuditPayment_A1Reject(t *testing.T) {
	h := &DashboardHandler{}
	// 付款单, 法人实体为空
	got := h.AuditPayment("F2", "ID01KgaO6dcZtR:abc", `{"payMoney":500}`, 0)
	if got == nil || got.Action != "reject" {
		t.Fatalf("A1 应触发 reject, got %+v", got)
	}
}

// A1 集成: 预付款单也检查法人实体
func TestAuditPayment_A1PrepayReject(t *testing.T) {
	h := &DashboardHandler{}
	got := h.AuditPayment("F3", "ID01FhdI9II9A3:xyz", `{"loanMoney":200}`, 0)
	if got == nil || got.Action != "reject" {
		t.Fatalf("预付款单 A1 应触发 reject, got %+v", got)
	}
}

// A8: 明细行数/无票
func TestRulePaymentItemRows(t *testing.T) {
	// 预付款单 (prepay): 可无明细 → 通过
	if rej, _ := rulePaymentItemRows(map[string]interface{}{}, "prepay", "J26000001"); rej != "" {
		t.Errorf("预付款单可无明细, got rej=%q", rej)
	}
	// 付款单无明细 → 转人工 (warn 非空, reject 为空)
	rej, warn := rulePaymentItemRows(map[string]interface{}{}, "payment", "B26000001")
	if rej != "" {
		t.Errorf("付款单无明细应是 warn 不是 reject, got rej=%q", rej)
	}
	if warn == "" {
		t.Error("付款单无明细应转人工 (warn 非空)")
	}
	// 付款单有明细 → 通过
	raw := map[string]interface{}{
		"details": []interface{}{map[string]interface{}{"feeTypeId": "X"}},
	}
	if rej2, warn2 := rulePaymentItemRows(raw, "payment", "B26000002"); rej2 != "" || warn2 != "" {
		t.Errorf("有明细应通过, got rej=%q warn=%q", rej2, warn2)
	}
}

// A3: 客户多选 (仅付款单)
func TestRulePaymentCustomer(t *testing.T) {
	// 客户多选为空 → 转人工提醒
	if w := rulePaymentCustomer(map[string]interface{}{}); w == "" {
		t.Error("客户多选为空应提醒")
	}
	// 空 JSON 数组 → 提醒
	if w := rulePaymentCustomer(map[string]interface{}{"u_客户多选": "[]"}); w == "" {
		t.Error("u_客户多选=[] 应提醒")
	}
	// 有值 → 通过
	if w := rulePaymentCustomer(map[string]interface{}{"u_客户多选": `["ID01GU1fnDLSmb"]`}); w != "" {
		t.Errorf("有客户ID应通过, got %q", w)
	}
}

// B1+B2: 发票方核验 (纯逻辑)
func TestInvoicePartyMatch(t *testing.T) {
	sellers := []string{"唐山市艺诚广告有限公司"}
	// 收款方匹配任一开票方 → 通过
	if w := payeeSellerMismatch("唐山市艺诚广告有限公司", sellers); w != "" {
		t.Errorf("收款方=开票方应通过, got %q", w)
	}
	// 不匹配 → 转人工
	if w := payeeSellerMismatch("别的公司", sellers); w == "" {
		t.Error("收款方≠开票方应转人工 (B1)")
	}
	// 缺数据 → 不误判
	if w := payeeSellerMismatch("", sellers); w != "" {
		t.Errorf("收款方为空不误判, got %q", w)
	}
	if w := payeeSellerMismatch("公司X", nil); w != "" {
		t.Errorf("卖家列表空不误判, got %q", w)
	}

	// 购买方 != 所属公司 → 驳回
	if r := buyerCompanyMismatch([]string{"公司X"}, "公司Y"); r == "" {
		t.Error("购买方≠所属公司应驳回 (B2)")
	}
	// 购买方 = 所属公司 → 通过
	if r := buyerCompanyMismatch([]string{"公司X"}, "公司X"); r != "" {
		t.Errorf("购买方=所属公司应通过, got %q", r)
	}
	// 缺数据 → 不误判
	if r := buyerCompanyMismatch(nil, "公司X"); r != "" {
		t.Errorf("买家列表空不误判, got %q", r)
	}
	if r := buyerCompanyMismatch([]string{"公司X"}, ""); r != "" {
		t.Errorf("所属公司空不误判, got %q", r)
	}
}

// B4: 防重复付款 (纯逻辑)
func TestDuplicatePayment(t *testing.T) {
	// 找到历史单号 → 提示文案含原单号
	if w := dupWarnText([]string{"B26001234"}); w == "" || !strings.Contains(w, "B26001234") {
		t.Errorf("应提示原单号, got %q", w)
	}
	// 多个单号 → 都出现
	if w := dupWarnText([]string{"B26001111", "B26002222"}); !strings.Contains(w, "B26001111") || !strings.Contains(w, "B26002222") {
		t.Errorf("多个单号应全部出现, got %q", w)
	}
	// 无重复 → 空
	if w := dupWarnText(nil); w != "" {
		t.Errorf("无重复应空, got %q", w)
	}
	if w := dupWarnText([]string{}); w != "" {
		t.Errorf("空切片应空, got %q", w)
	}
}

// A5: 事由必填 + 不含合计/小计
func TestRulePaymentReason(t *testing.T) {
	// 预付款单: description 为空 → 驳回
	if rulePaymentReason(map[string]interface{}{"description": ""}, "prepay") == "" {
		t.Error("预付款单 付款事由为空应驳回 (A5)")
	}
	// 预付款单: description 含"合计" → 驳回
	if rulePaymentReason(map[string]interface{}{"description": "本月合计"}, "prepay") == "" {
		t.Error("含合计应驳回 (A5)")
	}
	// 预付款单: description 含"小计" → 驳回
	if rulePaymentReason(map[string]interface{}{"description": "各项小计"}, "prepay") == "" {
		t.Error("含小计应驳回 (A5)")
	}
	// 预付款单: 正常事由 → 通过
	if r := rulePaymentReason(map[string]interface{}{"description": "支付6月推广服务费"}, "prepay"); r != "" {
		t.Errorf("正常事由应通过, got %q", r)
	}
	// 付款单: 无明细 → 通过 (A8 兜底, A5 不重复驳)
	if r := rulePaymentReason(map[string]interface{}{}, "payment"); r != "" {
		t.Errorf("付款单无明细 A5 应放行, got %q", r)
	}
	// 付款单: 明细消费事由为空 → 驳回
	rawPayment := map[string]interface{}{
		"details": []interface{}{
			map[string]interface{}{
				"feeTypeForm": map[string]interface{}{"consumptionReasons": ""},
			},
		},
	}
	if rulePaymentReason(rawPayment, "payment") == "" {
		t.Error("付款单明细消费事由为空应驳回 (A5)")
	}
	// 付款单: 消费事由含"合计" → 驳回
	rawPayment2 := map[string]interface{}{
		"details": []interface{}{
			map[string]interface{}{
				"feeTypeForm": map[string]interface{}{"consumptionReasons": "第3行合计"},
			},
		},
	}
	if rulePaymentReason(rawPayment2, "payment") == "" {
		t.Error("付款单消费事由含合计应驳回 (A5)")
	}
	// 付款单: 消费事由正常 → 通过
	rawPayment3 := map[string]interface{}{
		"details": []interface{}{
			map[string]interface{}{
				"feeTypeForm": map[string]interface{}{"consumptionReasons": "支付供应商货款"},
			},
		},
	}
	if r := rulePaymentReason(rawPayment3, "payment"); r != "" {
		t.Errorf("付款单正常消费事由应通过, got %q", r)
	}
}

// A17: 大额付款缺附件 → 转人工提醒
func TestRulePaymentLargeContract(t *testing.T) {
	// 付款单 > 2万, 无附件 → 提醒
	rawBig := map[string]interface{}{
		"payMoney":    float64(25000),
		"attachments": []interface{}{},
	}
	if w := rulePaymentLargeContract(rawBig, "payment"); w == "" {
		t.Error("付款单>2万无附件应提醒 (A17)")
	}
	// 付款单 > 2万, 有附件 → 通过
	rawBigAtt := map[string]interface{}{
		"payMoney": float64(25000),
		"attachments": []interface{}{
			map[string]interface{}{"fileId": "X"},
		},
	}
	if w := rulePaymentLargeContract(rawBigAtt, "payment"); w != "" {
		t.Errorf("付款单>2万有附件应通过, got %q", w)
	}
	// 付款单 ≤ 2万 → 通过
	rawSmall := map[string]interface{}{
		"payMoney":    float64(20000),
		"attachments": []interface{}{},
	}
	if w := rulePaymentLargeContract(rawSmall, "payment"); w != "" {
		t.Errorf("付款单≤2万不提醒, got %q", w)
	}
	// 预付款单 > 2万, 无附件, 取 loanMoney → 提醒
	rawPrepay := map[string]interface{}{
		"loanMoney":   float64(30000),
		"attachments": []interface{}{},
	}
	if w := rulePaymentLargeContract(rawPrepay, "prepay"); w == "" {
		t.Error("预付款单>2万无附件应提醒 (A17)")
	}
	// 合思金额对象格式 (生产真实格式) → 能正确解析
	rawMoneyObj := map[string]interface{}{
		"payMoney": map[string]interface{}{
			"standard": "25844.00",
		},
		"attachments": []interface{}{},
	}
	if w := rulePaymentLargeContract(rawMoneyObj, "payment"); w == "" {
		t.Error("合思金额对象格式>2万无附件应提醒 (A17)")
	}
}

// B3: 税额份数对账 (纯逻辑)
func TestTaxCountReconcile(t *testing.T) {
	// 发票张数 != 申报张数 → 转人工
	if w := taxCountMismatch(2, 3); w == "" {
		t.Error("张数不符应转人工 (B3)")
	}
	// 发票张数 == 申报张数 → 通过
	if w := taxCountMismatch(2, 2); w != "" {
		t.Errorf("张数一致应通过, got %q", w)
	}
	// 申报 0 份 → 不判 (安全底线)
	if w := taxCountMismatch(5, 0); w != "" {
		t.Errorf("申报0份应跳过不判, got %q", w)
	}
	// 申报 1 张, 实际 1 张 → 通过
	if w := taxCountMismatch(1, 1); w != "" {
		t.Errorf("1张一致应通过, got %q", w)
	}
	// 提示文案含张数信息
	msg := taxCountMismatch(3, 5)
	if !strings.Contains(msg, "3") || !strings.Contains(msg, "5") {
		t.Errorf("提示应含实际张数和申报张数, got %q", msg)
	}
}

// toFloatPayment: 合思金额对象格式测试
func TestToFloatPayment(t *testing.T) {
	// 普通 float64 (测试用)
	if toFloatPayment(float64(1234.5)) != 1234.5 {
		t.Error("普通 float64 应正确返回")
	}
	// 合思对象格式
	obj := map[string]interface{}{"standard": "25844.00"}
	if got := toFloatPayment(obj); got != 25844.0 {
		t.Errorf("合思金额对象应解析为 25844.0, got %v", got)
	}
	// nil/空 → 0
	if toFloatPayment(nil) != 0 {
		t.Error("nil 应返回 0")
	}
}
