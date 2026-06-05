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

// 规则 4 总分公司特例: 合同公司是所属公司的分公司 → 报销可用主公司主体 (跑哥 2026-06-05)
func TestIsBranchOfLegalEntity(t *testing.T) {
	const main = "浙江松鲜鲜世创食品科技有限公司"
	cases := []struct {
		contract string
		legal    string
		want     bool
		desc     string
	}{
		{"浙江松鲜鲜世创食品科技有限公司杭州分公司", main, true, "杭州分公司→主公司 放行"},
		{"浙江松鲜鲜世创食品科技有限公司北京分公司", main, true, "北京分公司→主公司 放行"},
		{"浙江松鲜鲜世创食品科技有限公司西北分公司", main, true, "西北分公司→主公司 放行"},
		{main, main, false, "主公司自身不算分公司(由上游==精确匹配处理)"},
		{"浙江松鲜鲜食品有限公司", main, false, "另一家公司(非世创前缀) 不放行"},
		{"杭州某某分公司", main, false, "无关分公司(前缀对不上) 不放行"},
		{"浙江松鲜鲜世创食品科技有限公司杭州分公司", "", false, "所属公司为空 不放行"},
	}
	for _, c := range cases {
		if got := isBranchOfLegalEntity(c.contract, c.legal); got != c.want {
			t.Errorf("%s: isBranchOfLegalEntity(%q,%q)=%v 期望 %v", c.desc, c.contract, c.legal, got, c.want)
		}
	}
}
