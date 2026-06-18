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
// u_客户多选 经 json.Unmarshal 后是 []interface{}, 不是 string。
// 测试用例必须用 []interface{}{...} 构造, 模拟真实反序列化后的类型。
func TestRulePaymentCustomer(t *testing.T) {
	// 字段缺失 → 转人工提醒
	if w := rulePaymentCustomer(map[string]interface{}{}); w == "" {
		t.Error("字段缺失应提醒 (A3)")
	}
	// 空数组 ([]interface{}{}) → 提醒
	if w := rulePaymentCustomer(map[string]interface{}{"u_客户多选": []interface{}{}}); w == "" {
		t.Error("空数组应提醒 (A3)")
	}
	// 非空数组 (真实生产格式) → 通过
	if w := rulePaymentCustomer(map[string]interface{}{"u_客户多选": []interface{}{"ID01GU1fnDLSmb"}}); w != "" {
		t.Errorf("非空数组应通过, got %q", w)
	}
	// 多个元素 → 通过
	if w := rulePaymentCustomer(map[string]interface{}{"u_客户多选": []interface{}{"ID01GU1fnDLSmb", "ID01ABC"}}); w != "" {
		t.Errorf("多个客户ID应通过, got %q", w)
	}
	// 旧代码中 .(string) 断言对真实数组必失败 → 修复后不再误报
	// (此用例等价于"字段类型正确时不触发 A3")
	realArr := []interface{}{"ID01GU1fnDLSmb"}
	raw := map[string]interface{}{"u_客户多选": realArr}
	if w := rulePaymentCustomer(raw); w != "" {
		t.Errorf("修复后真实数组格式不应误报, got %q (原 .(string) 断言必失败导致全部误报)", w)
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
	// 预付款单: 预付事由(title) 为空 → 驳回
	if rulePaymentReason(map[string]interface{}{"title": ""}, "prepay") == "" {
		t.Error("预付款单 预付事由为空应驳回 (A5)")
	}
	// 预付款单: title 含"合计" → 驳回
	if rulePaymentReason(map[string]interface{}{"title": "本月合计"}, "prepay") == "" {
		t.Error("含合计应驳回 (A5)")
	}
	// 预付款单: title 含"小计" → 驳回
	if rulePaymentReason(map[string]interface{}{"title": "各项小计"}, "prepay") == "" {
		t.Error("含小计应驳回 (A5)")
	}
	// 预付款单: 正常预付事由(title) → 通过
	if r := rulePaymentReason(map[string]interface{}{"title": "支付6月推广服务费"}, "prepay"); r != "" {
		t.Errorf("正常预付事由应通过, got %q", r)
	}
	// 预付款单回归 (J26000749): title 有预付事由 + description 空 → 不再误报"事由为空"
	if r := rulePaymentReason(map[string]interface{}{"title": "申请有机甄鲜松茸松露调味料有机码", "description": ""}, "prepay"); r != "" {
		t.Errorf("J26000749: title 有预付事由 description 空不应误驳, got %q", r)
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

// Bug 1: A13 付款单 consumptionReasons 含「阳光天际」→ 应触发驳回
// 付款单的事由在 details[].feeTypeForm.consumptionReasons，不在 description
func TestA13SunshinePaymentVoucher(t *testing.T) {
	// 付款单: consumptionReasons 含「阳光天际」, 所属公司非悦伍 → extractPaymentReasonText 应包含关键词
	rawPaymentSunshine := map[string]interface{}{
		"details": []interface{}{
			map[string]interface{}{
				"feeTypeForm": map[string]interface{}{
					"consumptionReasons": "阳光天际项目推广款",
				},
			},
		},
	}
	reasonText := extractPaymentReasonText(rawPaymentSunshine, "payment")
	if !strings.Contains(reasonText, "阳光天际") {
		t.Errorf("付款单 details consumptionReasons 含阳光天际, extractPaymentReasonText 应包含该词, got %q", reasonText)
	}
	// 再验证 sunshineRule 对提取出的 reason 能正确触发
	if r := sunshineRule(reasonText, "杭州松鲜鲜自然调味品"); r == "" {
		t.Error("A13: 付款单事由含阳光天际且非悦伍公司应驳回")
	}
	// 付款单 description 也含「阳光天际」→ belt-and-suspenders 也能被提取
	rawPaymentDescSunshine := map[string]interface{}{
		"description": "阳光天际补充款",
	}
	reasonText2 := extractPaymentReasonText(rawPaymentDescSunshine, "payment")
	if !strings.Contains(reasonText2, "阳光天际") {
		t.Errorf("付款单 description 含阳光天际, extractPaymentReasonText 应包含, got %q", reasonText2)
	}
	// 预付款单: 预付事由在 title, 含「阳光天际」→ 能提取 (修复前 prepay 只读 description 会漏判)
	if reasonT := extractPaymentReasonText(map[string]interface{}{"title": "阳光天际项目预付款"}, "prepay"); !strings.Contains(reasonT, "阳光天际") {
		t.Errorf("预付款单 title 含阳光天际, extractPaymentReasonText 应包含, got %q", reasonT)
	}
	// 预付款单: 补充说明 description 含「阳光天际」→ 也能提取 (belt-and-suspenders, title+description 合并)
	rawPrepaySunshine := map[string]interface{}{
		"description": "阳光天际项目预付款",
	}
	reasonText3 := extractPaymentReasonText(rawPrepaySunshine, "prepay")
	if !strings.Contains(reasonText3, "阳光天际") {
		t.Errorf("预付款单 description 含阳光天际, extractPaymentReasonText 应包含, got %q", reasonText3)
	}
	// 付款单事由正常 (无阳光天际关键词) → sunshineRule 不触发
	rawNormal := map[string]interface{}{
		"details": []interface{}{
			map[string]interface{}{
				"feeTypeForm": map[string]interface{}{
					"consumptionReasons": "支付供应商货款",
				},
			},
		},
	}
	reasonNormal := extractPaymentReasonText(rawNormal, "payment")
	if r := sunshineRule(reasonNormal, "杭州松鲜鲜自然调味品"); r != "" {
		t.Errorf("无阳光天际关键词应通过, got %q", r)
	}
}

// Bug 2: B4 预付款单用 loanMoney 做金额提取, amount>0 才不早返
func TestB4PrepayLoanMoneyAmount(t *testing.T) {
	// payMoney=0 (预付款单无此字段), loanMoney=5000 → amount 应取到 5000
	amount := extractDupCheckAmount(map[string]interface{}{
		"payMoney":  float64(0),
		"loanMoney": float64(5000),
	})
	if amount != 5000 {
		t.Errorf("预付款单 loanMoney=5000, payMoney=0 → amount 应为 5000, got %v", amount)
	}
	// payMoney>0 时优先用 payMoney
	amount2 := extractDupCheckAmount(map[string]interface{}{
		"payMoney":  float64(3000),
		"loanMoney": float64(5000),
	})
	if amount2 != 3000 {
		t.Errorf("payMoney=3000 优先, got %v", amount2)
	}
	// 两者均缺失 → 0
	amount3 := extractDupCheckAmount(map[string]interface{}{})
	if amount3 != 0 {
		t.Errorf("两者均缺失应为 0, got %v", amount3)
	}
}

// Bug 3: B2 仅 buyers 为空 (sellers 存在) → 应产生 B2 人工提醒
func TestB2BuyerEmptyManualWarning(t *testing.T) {
	// buyers 为空, sellers 存在 → 应有 B2 提醒
	_, warn := rulePaymentInvoicePartiesLogic(
		nil,              // buyers 为空
		[]string{"卖方A"}, // sellers 存在
		"卖方A",            // payeeName
		"某公司",            // owner
	)
	hasB2Warn := false
	for _, w := range warn {
		if strings.Contains(w, "B2") {
			hasB2Warn = true
		}
	}
	if !hasB2Warn {
		t.Errorf("buyers 为空但 sellers 存在时, 应产生 B2 人工提醒, got warn=%v", warn)
	}

	// sellers 为空, buyers 存在 → 应有 B1 提醒
	_, warn2 := rulePaymentInvoicePartiesLogic(
		[]string{"买方A"}, // buyers 存在
		nil,              // sellers 为空
		"买方A",            // payeeName (即便匹配也无 sellers 可比)
		"买方A",            // owner = buyers[0] → buyerMismatch 通过
	)
	hasB1Warn := false
	for _, w := range warn2 {
		if strings.Contains(w, "B1") {
			hasB1Warn = true
		}
	}
	if !hasB1Warn {
		t.Errorf("sellers 为空时应产生 B1 提醒, got warn2=%v", warn2)
	}

	// buyers 和 sellers 均为空 → 原有 B1/B2 合并兜底提醒仍触发
	_, warn3 := rulePaymentInvoicePartiesLogic(nil, nil, "", "某公司")
	hasBothWarn := false
	for _, w := range warn3 {
		if strings.Contains(w, "B1") && strings.Contains(w, "B2") {
			hasBothWarn = true
		}
	}
	if !hasBothWarn {
		t.Errorf("buyers 和 sellers 均为空, 应产生 B1/B2 合并提醒, got warn3=%v", warn3)
	}

	// buyers 和 sellers 均存在且一致 → 无提醒
	rej4, warn4 := rulePaymentInvoicePartiesLogic(
		[]string{"买方X"},
		[]string{"卖方Y"},
		"卖方Y", // 收款方匹配卖方
		"买方X", // 所属公司匹配买方
	)
	if len(rej4) > 0 || len(warn4) > 0 {
		t.Errorf("数据完整且一致时应全通过, got rej=%v warn=%v", rej4, warn4)
	}
}
