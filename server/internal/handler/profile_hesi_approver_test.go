package handler

import "testing"

// AI 审批建议审批人名单匹配 (跑哥 2026-06-18 扩: 日常报销+3人 / 付款单+苏安妮)
func TestMatchApproverName(t *testing.T) {
	// 日常报销名单: 樊雪娇(不回归) + 新增 金海侠/周翻翻/张勇
	for _, n := range []string{"樊雪娇", "金海侠", "周翻翻", "张勇"} {
		if !matchApproverName(dailyExpenseApproverNames, n, "", "") {
			t.Errorf("%s 应命中日常报销审批人名单", n)
		}
	}
	// 付款单名单: 张俊(不回归) + 新增 苏安妮
	for _, n := range []string{"张俊", "苏安妮"} {
		if !matchApproverName(paymentApproverNames, n, "", "") {
			t.Errorf("%s 应命中付款单审批人名单", n)
		}
	}
	// 无关名字都不命中
	if matchApproverName(dailyExpenseApproverNames, "李四", "王五", "赵六") {
		t.Error("无关名字不应命中日常报销")
	}
	if matchApproverName(paymentApproverNames, "李四", "王五", "赵六") {
		t.Error("无关名字不应命中付款单")
	}
	// 多字段兜底: 仅 hesiRealName(第3参) 命中也算
	if !matchApproverName(dailyExpenseApproverNames, "", "", "周翻翻") {
		t.Error("hesiRealName 命中也应算 (沿用原 displayName/queryName/hesiRealName 三选一)")
	}
	// 跨集合不串: 苏安妮只在付款单, 不在日常报销
	if matchApproverName(dailyExpenseApproverNames, "苏安妮", "", "") {
		t.Error("苏安妮只审付款单, 不应命中日常报销名单")
	}
	// 子串陷阱: 张勇 命中日常报销, 但不能误命中付款单的"张俊" (Contains 反向也不命中)
	if !matchApproverName(dailyExpenseApproverNames, "张勇", "", "") {
		t.Error("张勇应命中日常报销")
	}
	if matchApproverName(paymentApproverNames, "张勇", "", "") {
		t.Error("张勇(日常报销)不应误命中付款单(张俊) — 张勇≠张俊")
	}
	// 空名单/空名字不 panic 且不命中
	if matchApproverName(nil, "樊雪娇") {
		t.Error("空名单不应命中")
	}
	if matchApproverName(dailyExpenseApproverNames) {
		t.Error("无 names 入参不应命中")
	}
}
