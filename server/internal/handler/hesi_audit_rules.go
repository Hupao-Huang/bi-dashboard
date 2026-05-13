package handler

// ⚠️ 注意: 当前这套规则**仅适用于张俊**作为审批人时使用 (规则源自张俊提供的 Excel 私有定义)
// 调用前必须在上游判断 displayName/queryName 含"张俊", 别人不跑这套规则
// 见 profile_hesi_pending.go 的 enableAuditSuggestion 开关

import (
	"encoding/json"
	"strings"
)

// AuditSuggestion 审批建议输出 (仅张俊场景使用)
type AuditSuggestion struct {
	Action  string   `json:"action"` // agree / reject / manual
	Reasons []string `json:"reasons"`
}

// 法人实体白名单 (杭州松鲜鲜自然调味品有限公司 + 香松工厂主体)
// 数据来源: hesi_flow.法人实体 字段 top 2 出现频次
// TODO: 跑哥确认两个 ID 对应中文名后, 改成完整 map 含中文
var corpWhitelist = map[string]string{
	"ID01Fk0t6PxS5F": "杭州松鲜鲜自然调味品有限公司",
	"ID01Fk0sq1uiBx": "杭州松鲜鲜香松食品有限公司",
}

// 消费事由黑词
var consumptionBlackWords = []string{"合计", "小计"}

// AuditExpenseFlow 报销单审批建议规则引擎 (v1.63 第一批 MVP, 仅字段判定)
// 只跑 form_type=expense 的单据, 其他返回 nil
func AuditExpenseFlow(rawJSON string) *AuditSuggestion {
	if rawJSON == "" {
		return nil
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return nil
	}

	var rejectReasons []string
	var manualReasons []string

	// 提取 details 数组
	detailsRaw, _ := raw["details"].([]interface{})
	consumptionReasons := []string{}
	for _, d := range detailsRaw {
		dm, ok := d.(map[string]interface{})
		if !ok {
			continue
		}
		ftf, _ := dm["feeTypeForm"].(map[string]interface{})
		if ftf == nil {
			continue
		}
		if cr, ok := ftf["consumptionReasons"].(string); ok {
			consumptionReasons = append(consumptionReasons, cr)
		}
	}

	// 规则 1: 消费事由为空 → 驳回
	allEmpty := true
	for _, cr := range consumptionReasons {
		if strings.TrimSpace(cr) != "" {
			allEmpty = false
			break
		}
	}
	if len(consumptionReasons) == 0 || allEmpty {
		rejectReasons = append(rejectReasons, "消费事由全部为空 (基本信息②)")
	}

	// 规则 2: 消费事由含 "合计" / "小计" → 驳回
	for _, cr := range consumptionReasons {
		for _, w := range consumptionBlackWords {
			if strings.Contains(cr, w) {
				rejectReasons = append(rejectReasons, "消费事由含禁词「"+w+"」: "+truncate(cr, 30))
				break
			}
		}
	}

	// 规则 3: 消费事由 > 50 字 → 转人工
	for _, cr := range consumptionReasons {
		if runeCount(cr) > 50 {
			manualReasons = append(manualReasons, "消费事由过长 ("+itoa(runeCount(cr))+" 字, 标准 ≤ 50)")
		}
	}

	// 规则 4: 法人实体不在白名单 → 转人工
	corpID, _ := raw["法人实体"].(string)
	if corpID != "" {
		if _, ok := corpWhitelist[corpID]; !ok {
			manualReasons = append(manualReasons, "法人实体不在白名单 ("+corpID+")")
		}
	}

	// 综合判定: 任意驳回 → 驳回; 否则任意转人工 → 转人工; 否则同意
	if len(rejectReasons) > 0 {
		return &AuditSuggestion{Action: "reject", Reasons: rejectReasons}
	}
	if len(manualReasons) > 0 {
		return &AuditSuggestion{Action: "manual", Reasons: manualReasons}
	}
	return &AuditSuggestion{Action: "agree", Reasons: []string{"基本信息+消费事由规则全部通过"}}
}

func runeCount(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func truncate(s string, max int) string {
	if runeCount(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max]) + "..."
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
