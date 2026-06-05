package handler

import (
	"strings"
	"testing"
)

// 规则 12-2 消费事由字数: 50 字以内通过, 超 50 字驳回 (跑哥 2026-06-05 改, 去掉 10-50 字人工核档)
// 验证 ruleDriveAndReasons 的字数分支
func TestRuleConsumptionReasonLength(t *testing.T) {
	mkRaw := func(reason string) map[string]interface{} {
		return map[string]interface{}{
			"details": []interface{}{
				map[string]interface{}{
					"feeTypeId": "ID01XXXXX", // 非自驾, 只测字数
					"feeTypeForm": map[string]interface{}{
						"detailNo":           float64(1),
						"consumptionReasons": reason,
					},
				},
			},
		}
	}

	cases := []struct {
		name       string
		runeLen    int
		wantReject bool
		wantWarn   bool
	}{
		{"10字-通过", 10, false, false},
		{"30字-通过(原来人工核,现在通过)", 30, false, false},
		{"50字边界-通过", 50, false, false},
		{"51字-驳回", 51, true, false},
		{"80字-驳回", 80, true, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reason := strings.Repeat("阿", c.runeLen) // 中文 1 rune/字
			reject, warn := ruleDriveAndReasons(mkRaw(reason))
			if c.wantReject != (reject != "") {
				t.Errorf("%d 字: 期望驳回=%v, 实际 reject=%q", c.runeLen, c.wantReject, reject)
			}
			// 字数档不应再产生 warning (自驾 warning 是另一条, 本用例无自驾)
			if c.wantWarn != (warn != "") {
				t.Errorf("%d 字: 期望人工核=%v, 实际 warn=%q", c.runeLen, c.wantWarn, warn)
			}
		})
	}
}
