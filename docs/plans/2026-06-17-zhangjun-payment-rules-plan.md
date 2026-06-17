# 张俊「对外付款单」AI 审批建议 实施计划 (A+B 档)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给合思审批人张俊的「付款单/预付款单」加 AI 审批建议 (dry-run, 不真审批), 实现设计文档 §三/§四 的 22 条规则。

**Architecture:** 新建独立引擎 `AuditPayment` (新文件 `hesi_audit_payment_rules.go`), 与樊雪娇 `AuditDailyExpense` 并排, 复用现成 helper (部门树/收款方/发票/法人实体)。在 `profile_hesi_pending.go` 加触发分支 (张俊 + 付款单/预付款单 spec)。引擎内按模板类型走不同规则子集。返回 `*AuditSuggestion{Action, Reasons}`, 优先级 reject > manual > agree。

**Tech Stack:** Go (net/http 标准库), MySQL, 现有 handler 包模式; 测试用标准 `testing` + 现有 sqlmock 模式。

**设计文档:** `docs/specs/2026-06-17-zhangjun-payment-rules-design.md` (本计划实现它)

## Global Constraints

- **dry-run 建议态**: 只填 `row.Suggestion`, **绝不调** `hesi_approval_queue` / worker。
- **安全底线**: 数据缺失 / 解析失败 / 字典查不到 → **转人工 (manual) 或跳过该规则**, **绝不硬驳 (reject)**。
- **动作三档**: `reject` (确定违规) / `manual` (拿不准) / `agree` (全过)。reject 优先于 manual 优先于 agree。
- **业务红线**: 本模块=审批规则改造, 上线前必须 `/code-review` 二审 (本机用 /code-review 不用 /codex)。
- **数据库注释中文**; 长 SQL 加 timeout (用现有 `queryRowContext` / `QueryContext` + 5s ctx 模式)。
- **改 handler / profile_hesi_pending.go 后**: `cd server && go build -o bi-server.exe ./cmd/server` → 拷到 server 根 → kill 8080 PID 重启; 重启前清代理 env (`Remove-Item env:*PROXY*`)。
- **金额比对精确到分**, 用 `float64` 容差 `0.01` (沿用 `AuditDailyExpense` 现有口径)。
- **现有在制改动**: `hesi_audit_rules.go` / `profile_hesi_pending.go` 有未提交的"发票超期规则 (applyStableSubmitDate)", 本计划新增代码尽量进**新文件** `hesi_audit_payment_rules.go`, 只在 `profile_hesi_pending.go` 加最小触发分支, 提交时与发票超期改动分开。

---

## raw_json 字段图 (2026-06-17 实查, 真实字段)

**付款单 (B 开头, spec `ID01KgaO6dcZtR`)** 顶层键:
| 业务含义 | raw_json 键 | 备注 |
|---|---|---|
| 所属公司 (法人实体) | `法人实体` | 值是 ID, 用 `h.LookupLegalEntityName(id)` 反查名 |
| 申请/费用部门 | `expenseDepartment` | 用 `h.ruleDeptLeaf(id, label)` 判末级 |
| 收款方 | `payeeId` | 用 `h.payeeName(id)` 取户名; `rulePayeeBank(h.DB,id)` 判银行 |
| 付款事由 (单据级) | `description` | 付款单可空 (要的是明细消费事由) |
| 客户多选 | `u_客户多选` | JSON 字符串数组如 `["ID01GU1fnDLSmb"]` |
| 支付金额 | `payMoney` | |
| 已核销金额 | `writtenOffMoney` | |
| 税额份数总计 | `u_WmLv_税额份数总计` | B3 用 |
| 费用明细 | `details[]` | 每项 `{feeTypeId, feeTypeForm, specificationId}` |
| 附件 | `attachments[]` | 每项 `{key, fileId, fileName}` (无"类型"字段, 只有文件名) |
| 提交人 | `submitterId` | |
| 提交时间 | `submitDate` | (毫秒戳) |

**预付款单 (J 开头, spec `ID01FhdI9II9A3`)** 顶层键差异:
| 业务含义 | raw_json 键 |
|---|---|
| 付款事由 (单据级, **必填**) | `description` |
| 借款部门 | `loanDepartment` (亦有 `expenseDepartment`) |
| 借款/预付金额 | `loanMoney` |
| 付款类别 | `u_付款类别` |
| 平台类型 | `u_平台类型` |
| **无** `details` / `u_客户多选` / `u_WmLv_税额份数总计` | — |

**发票表 `hesi_flow_invoice`** (按 `flow_id` 关联): `buyer_name` (购买方) / `seller_name` (开票方) / `tax_amount` (税额) / `invoice_amount` / `total_amount` / `is_verified`。

> 待执行时确认的深层结构 (各任务首步 discovery): `details[].feeTypeForm` 里"消费事由"字段名; `expenseLinks` (核销关联单) 结构; `payee` 记录里银行字段。每个相关任务第 1 步给出具体 discovery 命令。

---

## File Structure

- **Create** `server/internal/handler/hesi_audit_payment_rules.go` — 付款单引擎 `AuditPayment` + 所有付款专属 rule helper。主体新代码都在这。
- **Create** `server/internal/handler/hesi_audit_payment_rules_test.go` — 全部单测。
- **Modify** `server/internal/handler/profile_hesi_pending.go` — `GetMyHesiPending` 内加张俊触发分支 (最小改动, ~6 行)。
- **复用 (不改)**: `ruleDeptLeaf` / `rulePayeeBank` / `payeeName` / `LookupLegalEntityName` / `sumInvoiceTotal` (在 `hesi_audit_rules.go` / `hesi.go`, 同包直接调)。

---

## Task 1: 引擎骨架 + 触发挂载 (dry-run 空规则跑通链路)

**Files:**
- Create: `server/internal/handler/hesi_audit_payment_rules.go`
- Create: `server/internal/handler/hesi_audit_payment_rules_test.go`
- Modify: `server/internal/handler/profile_hesi_pending.go` (GetMyHesiPending 触发块)

**Interfaces:**
- Consumes: `AuditSuggestion` 类型 (已存在于 `hesi_audit_rules.go`: `{Action string; Reasons []string}`); `*DashboardHandler`。
- Produces: `func (h *DashboardHandler) AuditPayment(flowID, specID, rawJSON string, firstSubmitDate int64) *AuditSuggestion`; 常量 `paymentSpecPrefix="ID01KgaO6dcZtR"`, `prepaySpecPrefix="ID01FhdI9II9A3"`。

- [ ] **Step 1: 写失败测试** (骨架: 空规则返回 agree, 能识别两种模板)

在 `hesi_audit_payment_rules_test.go`:
```go
package handler

import "testing"

func TestAuditPayment_EmptyRulesAgree(t *testing.T) {
	h := &DashboardHandler{}
	raw := `{"法人实体":"X","payMoney":100}`
	got := h.AuditPayment("F1", "ID01KgaO6dcZtR:abc", raw, 0)
	if got == nil || got.Action != "agree" {
		t.Fatalf("空规则应 agree, got %+v", got)
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
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd server && go test ./internal/handler/ -run 'TestAuditPayment_EmptyRulesAgree|TestPaymentTemplateType' -v`
Expected: FAIL (`undefined: AuditPayment` / `paymentTemplate`)

- [ ] **Step 3: 写骨架实现**

在 `hesi_audit_payment_rules.go`:
```go
package handler

import "encoding/json"

const (
	paymentSpecPrefix = "ID01KgaO6dcZtR" // 付款单 (票到付款/票到核销), B 开头
	prepaySpecPrefix  = "ID01FhdI9II9A3" // 预付款单 (先款后票), J 开头
)

// paymentTemplate 按 spec 前缀判模板类型: "payment"=付款单 / "prepay"=预付款单 / ""=不适用
func paymentTemplate(specID string) string {
	switch {
	case len(specID) >= len(paymentSpecPrefix) && specID[:len(paymentSpecPrefix)] == paymentSpecPrefix:
		return "payment"
	case len(specID) >= len(prepaySpecPrefix) && specID[:len(prepaySpecPrefix)] == prepaySpecPrefix:
		return "prepay"
	default:
		return ""
	}
}

// AuditPayment 张俊对外付款单审批规则引擎 (dry-run 建议态)。
// firstSubmitDate: 稳定首次提交时间 (同 AuditDailyExpense, 供发票时效用), 0=缺失。
func (h *DashboardHandler) AuditPayment(flowID, specID, rawJSON string, firstSubmitDate int64) *AuditSuggestion {
	tmpl := paymentTemplate(specID)
	if tmpl == "" {
		return &AuditSuggestion{Action: "agree", Reasons: []string{"非付款单模板, 不评估"}}
	}

	var raw map[string]interface{}
	if rawJSON != "" {
		_ = json.Unmarshal([]byte(rawJSON), &raw)
	}
	applyStableSubmitDate(firstSubmitDate, raw) // 复用 hesi_audit_rules.go 的稳定提交时间

	var rejectReasons []string
	var warnings []string

	// 后续任务在此追加 rule helper 调用 (按 tmpl 分支)
	_ = raw
	_ = flowID
	_ = tmpl

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
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./internal/handler/ -run 'TestAuditPayment_EmptyRulesAgree|TestPaymentTemplateType' -v`
Expected: PASS

- [ ] **Step 5: 挂触发分支** (profile_hesi_pending.go)

先 discovery 樊雪娇触发块: `cd server && grep -n "樊雪娇\|AuditDailyExpense\|isFan" internal/handler/profile_hesi_pending.go`
在 `GetMyHesiPending` 里樊雪娇分支**旁边**加 (张俊判定仿 `isFanXuejiao` 的 displayName/queryName/hesiRealName 含名字模式):
```go
// 张俊: 付款单/预付款单 AI 审批建议 (dry-run, 详见 hesi_audit_payment_rules.go)
isZhangJun := strings.Contains(displayName, "张俊") || strings.Contains(queryName, "张俊") || strings.Contains(hesiRealName, "张俊")
if isZhangJun && paymentTemplate(row.SpecificationID) != "" {
	firstSubmit := int64(0)
	if row.SubmitDate != nil {
		firstSubmit = *row.SubmitDate
	}
	row.Suggestion = h.AuditPayment(row.FlowID, row.SpecificationID, rawJSON, firstSubmit)
}
```
> 注: 用与樊雪娇分支相同的 `rawJSON` / `row.SubmitDate` 局部变量 (Task 1 step5 discovery 确认它们在作用域内; 若樊雪娇分支在循环内取的是 `row.RawJSON`, 照搬同名)。确保**张俊分支与樊雪娇分支互斥** (一个单不会同时是俩人的待审, 但加 `else if` 更稳)。

- [ ] **Step 6: 跑全包测试 + build**

Run: `cd server && go test ./internal/handler/ -run 'TestAuditPayment|TestPaymentTemplate' -v && go build -o bi-server.exe ./cmd/server`
Expected: PASS + build 成功

- [ ] **Step 7: Commit**

```bash
git add server/internal/handler/hesi_audit_payment_rules.go server/internal/handler/hesi_audit_payment_rules_test.go server/internal/handler/profile_hesi_pending.go
git commit -m "feat(hesi): 张俊付款单审批引擎骨架 + 触发挂载 (dry-run 空规则)"
```

---

## Task 2: A1 所属公司=支付主体 (规则模板范例)

> 本任务是"加一条规则"的完整 TDD 范例, 后续规则任务照此模式。

**Files:** Modify `hesi_audit_payment_rules.go` + `..._test.go`

**Interfaces:**
- Produces: `func rulePaymentOwnerCompany(raw map[string]interface{}) string` (返回驳回理由, "" = 通过)。

- [ ] **Step 1: 写失败测试**
```go
func TestRulePaymentOwnerCompany(t *testing.T) {
	// 法人实体为空 → 驳回
	if rulePaymentOwnerCompany(map[string]interface{}{}) == "" {
		t.Error("法人实体为空应驳回")
	}
	// 有法人实体 → 通过 (与支付主体一致性在 B2 用发票购买方校验, 此处仅查非空)
	if r := rulePaymentOwnerCompany(map[string]interface{}{"法人实体": "ID01X"}); r != "" {
		t.Errorf("有法人实体应通过, got %q", r)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd server && go test ./internal/handler/ -run TestRulePaymentOwnerCompany -v`
Expected: FAIL (`undefined: rulePaymentOwnerCompany`)

- [ ] **Step 3: 写实现 + 接入引擎**
```go
// A1 所属公司 (法人实体) 必须非空。与支付主体的实质一致性由 B2 (发票购买方=所属公司) 兜底。
func rulePaymentOwnerCompany(raw map[string]interface{}) string {
	if id, _ := raw["法人实体"].(string); id == "" {
		return "所属公司 (法人实体) 为空 (A1)"
	}
	return ""
}
```
在 `AuditPayment` 的"后续任务追加"处 (两种模板都做):
```go
	if r := rulePaymentOwnerCompany(raw); r != "" {
		rejectReasons = append(rejectReasons, r)
	}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./internal/handler/ -run TestRulePaymentOwnerCompany -v`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add server/internal/handler/hesi_audit_payment_rules.go server/internal/handler/hesi_audit_payment_rules_test.go
git commit -m "feat(hesi): 张俊付款单规则 A1 所属公司非空"
```

---

## Task 3: A2 申请部门末级 (转人工提醒, 复用 ruleDeptLeaf)

**Interfaces:** Produces: 引擎内调 `h.ruleDeptLeaf(deptID, label)` (已存在, 返回非末级理由)。

- [ ] **Step 1: 写失败测试** (需 sqlmock 部门树; 仿 `hesi_audit_rule15_test.go` 的 DB mock 模式)

先 discovery: `cd server && grep -n "ruleDeptLeaf" internal/handler/hesi_audit_rules.go` 看签名与查询。
```go
func TestAuditPayment_DeptLeafManual(t *testing.T) {
	// 部门非末级时, ruleDeptLeaf 返回理由 → 但 A2 裁定为"转人工"不硬驳:
	// 引擎应把它放进 warnings 而非 rejectReasons。本测试验证 A2 走 manual 通道。
	// (用 paymentDeptLeafWarn 包装 ruleDeptLeaf 的输出)
	msg := paymentDeptLeafWarn("非末级部门 (A2 申请部门)")
	if msg == "" {
		t.Error("非末级应产出转人工提醒")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd server && go test ./internal/handler/ -run TestAuditPayment_DeptLeafManual -v`
Expected: FAIL (`undefined: paymentDeptLeafWarn`)

- [ ] **Step 3: 写实现 + 接入** (A2 = 转人工, 进 warnings)
```go
// A2 申请部门末级: 裁定为"转人工提醒", 不硬驳 (跑哥 2026-06-17, 新文档矛盾但我们有部门树)。
func paymentDeptLeafWarn(deptLeafReason string) string {
	if deptLeafReason == "" {
		return ""
	}
	return deptLeafReason // 非空即提醒文案, 由引擎塞入 warnings
}
```
在 `AuditPayment` (两种模板都做; 付款单用 `expenseDepartment`, 预付款单优先 `loanDepartment` 兜底 `expenseDepartment`):
```go
	deptID, _ := raw["expenseDepartment"].(string)
	if tmpl == "prepay" {
		if ld, _ := raw["loanDepartment"].(string); ld != "" {
			deptID = ld
		}
	}
	if w := paymentDeptLeafWarn(h.ruleDeptLeaf(deptID, "申请部门 (A2)")); w != "" {
		warnings = append(warnings, w)
	}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./internal/handler/ -run TestAuditPayment_DeptLeafManual -v`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add server/internal/handler/hesi_audit_payment_rules.go server/internal/handler/hesi_audit_payment_rules_test.go
git commit -m "feat(hesi): 张俊付款单规则 A2 申请部门末级 (转人工提醒)"
```

---

## Task 4: A5 事由必填 + 不含合计/小计

**Interfaces:** Produces: `func rulePaymentReason(raw map[string]interface{}, tmpl string) string`。

- [ ] **Step 1: discovery 明细消费事由字段**

Run: `cd server && go run ./cmd/probe-hesi-payment-reason` **若不存在则**直接查库:
```sql
SELECT JSON_KEYS(JSON_EXTRACT(raw_json,'$.details[0].feeTypeForm'))
FROM hesi_flow WHERE specification_id LIKE 'ID01KgaO6dcZtR%' AND active=1
  AND JSON_LENGTH(JSON_EXTRACT(raw_json,'$.details'))>0 LIMIT 3;
```
记下 feeTypeForm 里"消费事由"的键名 (合思常见 `feeDetailReason` / `note` / `u_消费事由`; 以实查为准, 记作 `<REASON_KEY>`)。

- [ ] **Step 2: 写失败测试**
```go
func TestRulePaymentReason(t *testing.T) {
	// 预付款单: description (付款事由) 必填
	if rulePaymentReason(map[string]interface{}{"description": ""}, "prepay") == "" {
		t.Error("预付款单 付款事由为空应驳回")
	}
	// 含"合计" → 驳回
	if rulePaymentReason(map[string]interface{}{"description": "本月合计"}, "prepay") == "" {
		t.Error("含合计应驳回")
	}
	// 正常 → 通过
	if r := rulePaymentReason(map[string]interface{}{"description": "支付6月推广服务费"}, "prepay"); r != "" {
		t.Errorf("正常事由应通过, got %q", r)
	}
}
```

- [ ] **Step 3: 写实现 + 接入** (付款单查明细消费事由用 step1 的 `<REASON_KEY>`; 预付款单查 description)
```go
var vagueReasonWords = []string{"合计", "小计"}

// A5 事由: 预付款单查单据级 description; 付款单查每行明细的消费事由。必填 + 不含"合计/小计"。
func rulePaymentReason(raw map[string]interface{}, tmpl string) string {
	check := func(s, where string) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return where + "为空 (A5)"
		}
		for _, w := range vagueReasonWords {
			if strings.Contains(s, w) {
				return where + "含模糊词\"" + w + "\" (A5)"
			}
		}
		return ""
	}
	if tmpl == "prepay" {
		s, _ := raw["description"].(string)
		return check(s, "付款事由")
	}
	// 付款单: 遍历 details[].feeTypeForm.<REASON_KEY>
	details, _ := raw["details"].([]interface{})
	if len(details) == 0 {
		return "" // 无明细由 A8 处理, 此处不重复驳
	}
	for i, d := range details {
		dm, _ := d.(map[string]interface{})
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		s, _ := form["<REASON_KEY>"].(string) // ← 用 step1 实查键名替换
		if r := check(s, fmt.Sprintf("第%d行消费事由", i+1)); r != "" {
			return r
		}
	}
	return ""
}
```
接入 `AuditPayment` (两模板都做):
```go
	if r := rulePaymentReason(raw, tmpl); r != "" {
		rejectReasons = append(rejectReasons, r)
	}
```
> 顶部 import 补 `"fmt"` `"strings"` (若骨架未引)。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./internal/handler/ -run TestRulePaymentReason -v`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add server/internal/handler/hesi_audit_payment_rules.go server/internal/handler/hesi_audit_payment_rules_test.go
git commit -m "feat(hesi): 张俊付款单规则 A5 事由必填+不含合计小计"
```

---

## Task 5: A6 收款信息合规 + A7 收款方≠所属公司 (防自付)

**Interfaces:** Produces: `func (h *DashboardHandler) rulePaymentPayee(raw map[string]interface{}) (reject string)`。复用 `rulePayeeBank(h.DB, id)` + `h.payeeName(id)` + `h.LookupLegalEntityName(id)`。

- [ ] **Step 1: 写失败测试** (payeeName / 法人实体名 同名 → 防自付驳回)
```go
func TestPaymentSelfPayDetect(t *testing.T) {
	// 纯函数部分: 收款方户名 == 所属公司名 → 驳回理由非空
	if r := selfPayReason("杭州松鲜鲜食品", "杭州松鲜鲜食品"); r == "" {
		t.Error("收款方=所属公司应驳回 (防自付)")
	}
	if r := selfPayReason("供应商A", "杭州松鲜鲜食品"); r != "" {
		t.Errorf("不同应通过, got %q", r)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd server && go test ./internal/handler/ -run TestPaymentSelfPayDetect -v`
Expected: FAIL

- [ ] **Step 3: 写实现 + 接入**
```go
// A7 防自付: 收款方户名 = 所属公司名 → 驳回。
func selfPayReason(payeeName, ownerCompany string) string {
	if payeeName != "" && ownerCompany != "" && payeeName == ownerCompany {
		return "收款方与所属公司同名, 疑似自我付款 (A7)"
	}
	return ""
}

// A6+A7 收款信息: 银行账户合规 (复用 rulePayeeBank) + 防自付。
func (h *DashboardHandler) rulePaymentPayee(raw map[string]interface{}) string {
	payeeID, _ := raw["payeeId"].(string)
	if payeeID == "" {
		return "收款信息为空 (A6)"
	}
	if r := rulePayeeBank(h.DB, payeeID); r != "" {
		return r // A6: 非银行账户等
	}
	leID, _ := raw["法人实体"].(string)
	return selfPayReason(h.payeeName(payeeID), h.LookupLegalEntityName(leID))
}
```
接入 (两模板都做): `if r := h.rulePaymentPayee(raw); r != "" { rejectReasons = append(rejectReasons, r) }`

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./internal/handler/ -run TestPaymentSelfPayDetect -v`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add -A server/internal/handler/hesi_audit_payment_rules*.go
git commit -m "feat(hesi): 张俊付款单规则 A6 收款信息合规 + A7 防自付"
```

---

## Task 6: A8 明细行数 / 无票判定

**Interfaces:** Produces: `func rulePaymentItemRows(raw map[string]interface{}, tmpl string, code string) (reject, warn string)`。

- [ ] **Step 1: discovery 无票费用标记**

查 details[].feeTypeForm 是否有"是否无票/无发票"标记字段:
```sql
SELECT code, JSON_EXTRACT(raw_json,'$.details[0].feeTypeForm')
FROM hesi_flow WHERE specification_id LIKE 'ID01KgaO6dcZtR%' AND active=1
  AND JSON_LENGTH(JSON_EXTRACT(raw_json,'$.details'))>0 LIMIT 5;
```
判断"无票费用"识别口径 (设计 §8.2): 多半是某 feeTypeId 属"无票费用"类, 或 feeTypeForm 有 `invoiceCount=0` 之类。记作 `<NOINVOICE_RULE>`。

- [ ] **Step 2: 写失败测试**
```go
func TestRulePaymentItemRows(t *testing.T) {
	// 预付款单 (prepay): 可无明细 → 通过
	if rej, _ := rulePaymentItemRows(map[string]interface{}{}, "prepay", "J26000001"); rej != "" {
		t.Errorf("预付款单可无明细, got %q", rej)
	}
	// 付款单无明细 → 转人工 (warn 非空)
	_, warn := rulePaymentItemRows(map[string]interface{}{}, "payment", "B26000001")
	if warn == "" {
		t.Error("付款单无明细应转人工")
	}
}
```

- [ ] **Step 3: 写实现 + 接入** (J 开头/prepay 可无明细; payment 无明细→转人工)
```go
// A8 明细行数: 预付款单(J)可无明细; 付款单(B)无明细→转人工; 有明细则各行无票判定走 <NOINVOICE_RULE>。
func rulePaymentItemRows(raw map[string]interface{}, tmpl, code string) (reject, warn string) {
	details, _ := raw["details"].([]interface{})
	if len(details) == 0 {
		if tmpl == "prepay" {
			return "", "" // 先款后票, 可无明细
		}
		return "", "付款单无费用明细, 请人工确认 (A8)"
	}
	// 有明细: 无票费用可无发票, 其余必须有发票 (发票存在性在 A9 校验, 此处仅放行无票费用)
	return "", ""
}
```
接入 (两模板都做):
```go
	if rej, w := rulePaymentItemRows(raw, tmpl, codeOf(raw)); rej != "" || w != "" {
		if rej != "" { rejectReasons = append(rejectReasons, rej) }
		if w != "" { warnings = append(warnings, w) }
	}
```
> 补 helper `func codeOf(raw map[string]interface{}) string { s,_ := raw["code"].(string); return s }` (后续多处用)。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./internal/handler/ -run TestRulePaymentItemRows -v`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add -A server/internal/handler/hesi_audit_payment_rules*.go
git commit -m "feat(hesi): 张俊付款单规则 A8 明细行数/无票 (预付款单可无明细)"
```

---

## Task 7: A3 客户多选 (非线下选虚拟客户)

**Interfaces:** Produces: `func rulePaymentCustomer(raw map[string]interface{}) (warn string)`。

- [ ] **Step 1: discovery 虚拟客户/经销商 ID**

`u_客户多选` 是 ID 数组 (如 `["ID01GU1fnDLSmb"]`)。查"虚拟客户"对应 ID:
```sql
SELECT DISTINCT JSON_EXTRACT(raw_json,'$."u_客户多选"') v, COUNT(*) c
FROM hesi_flow WHERE specification_id LIKE 'ID01KgaO6dcZtR%' AND active=1
GROUP BY v ORDER BY c DESC LIMIT 20;
```
找出现率最高的那个 ID = 大概率"虚拟客户"; 与张俊核实虚拟客户 ID (设计 §8.2)。记作 `<VIRTUAL_CUSTOMER_ID>`。**A3 设为转人工提醒** (口径未完全定, 不硬驳)。

- [ ] **Step 2: 写失败测试**
```go
func TestRulePaymentCustomer(t *testing.T) {
	// 客户多选为空 → 转人工提醒
	if w := rulePaymentCustomer(map[string]interface{}{}); w == "" {
		t.Error("客户多选为空应提醒")
	}
}
```

- [ ] **Step 3: 写实现 + 接入** (A3 转人工, 进 warnings; 仅付款单做, 预付款单无此字段)
```go
// A3 客户多选: 非线下应选虚拟客户。口径未完全定 → 转人工提醒, 不硬驳 (设计 §8.2)。
func rulePaymentCustomer(raw map[string]interface{}) string {
	v, _ := raw["u_客户多选"].(string) // 存的是 JSON 字符串
	if strings.TrimSpace(v) == "" || v == "[]" {
		return "客户多选为空, 请确认是否应选虚拟客户 (A3)"
	}
	return ""
}
```
接入 (仅 payment): `if tmpl == "payment" { if w := rulePaymentCustomer(raw); w != "" { warnings = append(warnings, w) } }`

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./internal/handler/ -run TestRulePaymentCustomer -v`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add -A server/internal/handler/hesi_audit_payment_rules*.go
git commit -m "feat(hesi): 张俊付款单规则 A3 客户多选 (转人工提醒)"
```

---

## Task 8: A9 发票影像上传 + A12 检测费附件 + A17 大额合同(提醒)

**Interfaces:** Produces: `func (h *DashboardHandler) rulePaymentAttachments(raw map[string]interface{}, tmpl string) (reject, warn string)`。

- [ ] **Step 1: discovery 费用类型集合 (检测费 feeTypeId)**

`cd server && grep -rn "LookupFeeTypeName\|feeType" internal/handler/hesi_audit_rules.go | head -30`; 并查检测费 feeTypeId:
```sql
SELECT DISTINCT d.feeTypeId FROM hesi_flow f,
  JSON_TABLE(f.raw_json,'$.details[*]' COLUMNS(feeTypeId VARCHAR(64) PATH '$.feeTypeId')) d
WHERE f.specification_id LIKE 'ID01KgaO6dcZtR%' AND f.active=1 LIMIT 50;
```
配合费用类型字典反查名含"检测"的 ID, 记作 `detectionFeeTypes` set。

- [ ] **Step 2: 写失败测试**
```go
func TestPaymentAttachmentRules(t *testing.T) {
	// >2万无附件 → 提醒(warn), 不驳回(reject 必须空)
	rej, warn := attachmentChecks(20001, true /*hasContractKeyword*/ , false /*isDetection*/, false /*hasAttachment*/)
	if rej != "" { t.Errorf(">2万缺合同只提醒不驳, got reject %q", rej) }
	if warn == "" { t.Error(">2万缺合同应提醒") }
	// 检测费无附件 → 驳回
	rej2, _ := attachmentChecks(500, false, true, false)
	if rej2 == "" { t.Error("检测费无附件应驳回 (A12)") }
}
```

- [ ] **Step 3: 写实现 + 接入**
```go
// A9/A12/A17 附件类。attachmentChecks 为纯逻辑 (便于测试)。
func attachmentChecks(payMoney float64, hasContractKeyword, isDetection, hasAttachment bool) (reject, warn string) {
	if isDetection && !hasAttachment {
		return "检测费缺附件 (须合同/委托协议/报价单) (A12)", "" // 硬驳
	}
	if payMoney > 20000 && !hasAttachment {
		return "", "对外付款>2万元未见盖章合同, 请人工确认 (A17)" // 提醒 (跑哥 2026-06-17)
	}
	return "", ""
}

func (h *DashboardHandler) rulePaymentAttachments(raw map[string]interface{}, tmpl string) (reject, warn string) {
	atts, _ := raw["attachments"].([]interface{})
	hasAttachment := len(atts) > 0
	payMoney := toFloat(raw["payMoney"])
	isDetection := detailsHaveFeeType(raw, detectionFeeTypes) // step1 集合
	return attachmentChecks(payMoney, false, isDetection, hasAttachment)
}
```
补 helper:
```go
func toFloat(v interface{}) float64 { f, _ := v.(float64); return f }
func detailsHaveFeeType(raw map[string]interface{}, set map[string]bool) bool {
	details, _ := raw["details"].([]interface{})
	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if id, _ := dm["feeTypeId"].(string); set[id] { return true }
	}
	return false
}
```
接入 (付款单做全部; 预付款单无发票 → 只做 A17 大额合同 + A12 检测费, A9 跳过):
```go
	if rej, w := h.rulePaymentAttachments(raw, tmpl); rej != "" || w != "" {
		if rej != "" { rejectReasons = append(rejectReasons, rej) }
		if w != "" { warnings = append(warnings, w) }
	}
```
> A9「有票项必须传影像」: 付款单专属, 在 Task 6/A8 的有明细分支里, 若行非无票费用且 attachments 为空 → 驳回。本任务先做 A12/A17; A9 影像存在性合并进 attachmentChecks 的付款单分支 (执行时若 details 有票行且 !hasAttachment → reject "有票费用未上传发票影像 (A9)")。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./internal/handler/ -run TestPaymentAttachmentRules -v`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add -A server/internal/handler/hesi_audit_payment_rules*.go
git commit -m "feat(hesi): 张俊付款单规则 A9/A12/A17 附件 (大额合同仅提醒)"
```

---

## Task 9: A10 品牌中心必选 + A11 研发中心 RD 必填

**Interfaces:** Produces: `func (h *DashboardHandler) rulePaymentBrandRD(raw map[string]interface{}) (reject []string)`。

- [ ] **Step 1: discovery 必选字段 + 审批链 + 费用类型集合**

(1) 推广/广告/宣传 feeTypeId 集合 (`marketingFeeTypes`) 与研发费 feeTypeId 集合 (`rdFeeTypes`): 用费用类型字典名含"推广/广告/宣传"、"研发"反查。
(2) "市场中心营销费用"必选值字段 + "RD 研发项目"字段在 raw_json 的键: 查
```sql
SELECT JSON_KEYS(raw_json) FROM hesi_flow WHERE specification_id LIKE 'ID01KgaO6dcZtR%'
  AND active=1 AND title LIKE '%广告%' LIMIT 5;
```
找形如 `u_市场中心营销费用` / `u_研发项目` 的键, 记作 `<BRAND_FIELD>` / `<RD_FIELD>`。
(3) 审批链人员: 查审批流 (设计 §8.2, A10 审批链读取待落地)。**本任务先做"必选值"判定; 审批链是否含洪黎敏/王嘉 作为 A10 第二段, 读不到审批流时转人工**。

- [ ] **Step 2: 写失败测试**
```go
func TestBrandRDRequired(t *testing.T) {
	// 推广费但未选"市场中心营销费用" → 驳回
	rej := brandFieldCheck(true /*isMarketing*/, "" /*brandFieldVal*/)
	if rej == "" { t.Error("推广费未选市场中心营销费用应驳回 (A10)") }
	// 研发费但无 RD 项目 → 驳回
	if rdFieldCheck(true, "") == "" { t.Error("研发费缺RD项目应驳回 (A11)") }
}
```

- [ ] **Step 3: 写实现 + 接入**
```go
// A10 品牌中心: 推广/广告/宣传费 必选"市场中心营销费用"。
func brandFieldCheck(isMarketing bool, brandFieldVal string) string {
	if isMarketing && strings.TrimSpace(brandFieldVal) == "" {
		return "推广/广告类费用未选「市场中心营销费用」(A10)"
	}
	return ""
}
// A11 研发中心: 涉研发费 必填 RD 开头项目。
func rdFieldCheck(isRD bool, rdFieldVal string) string {
	if isRD && strings.TrimSpace(rdFieldVal) == "" {
		return "研发相关费用未填 RD 研发项目 (A11)"
	}
	return ""
}
func (h *DashboardHandler) rulePaymentBrandRD(raw map[string]interface{}) (reject []string) {
	isMkt := detailsHaveFeeType(raw, marketingFeeTypes)
	brandVal, _ := raw["<BRAND_FIELD>"].(string)
	if r := brandFieldCheck(isMkt, brandVal); r != "" { reject = append(reject, r) }
	isRD := detailsHaveFeeType(raw, rdFeeTypes)
	rdVal, _ := raw["<RD_FIELD>"].(string)
	if r := rdFieldCheck(isRD, rdVal); r != "" { reject = append(reject, r) }
	return reject
}
```
接入 (两模板都做): `rejectReasons = append(rejectReasons, h.rulePaymentBrandRD(raw)...)`
> A10 审批链含洪黎敏/王嘉 的第二段: 单独 helper, 读审批流字段 (step1 确认); 读不到 → warnings 转人工。康宁/陈焕焕 是否纳入 = 与张俊核实后再加 (设计 §九)。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./internal/handler/ -run TestBrandRDRequired -v`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add -A server/internal/handler/hesi_audit_payment_rules*.go
git commit -m "feat(hesi): 张俊付款单规则 A10 品牌中心 + A11 研发必选"
```

---

## Task 10: A13 阳光天际→悦伍 + A14 装修费→集团

**Interfaces:** Produces: `func (h *DashboardHandler) rulePaymentSpecialSubject(raw map[string]interface{}) (reject []string)`。

- [ ] **Step 1: 写失败测试**
```go
func TestSpecialSubject(t *testing.T) {
	// 事由含阳光天际 但所属公司名不含悦伍 → 驳回
	if r := sunshineRule("阳光天际项目款", "杭州松鲜鲜自然调味品"); r == "" {
		t.Error("阳光天际应要求悦伍公司 (A13)")
	}
	if r := sunshineRule("阳光天际项目款", "杭州松鲜鲜悦伍食品科技有限公司"); r != "" {
		t.Errorf("悦伍公司应通过, got %q", r)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd server && go test ./internal/handler/ -run TestSpecialSubject -v`
Expected: FAIL

- [ ] **Step 3: 写实现 + 接入**
```go
// A13 阳光天际→悦伍。
func sunshineRule(reason, ownerCompanyName string) string {
	if strings.Contains(reason, "阳光天际") && !strings.Contains(ownerCompanyName, "悦伍") {
		return "付款事由含「阳光天际」, 所属公司应为杭州松鲜鲜悦伍食品科技有限公司 (A13)"
	}
	return ""
}
func (h *DashboardHandler) rulePaymentSpecialSubject(raw map[string]interface{}) (reject []string) {
	reason, _ := raw["description"].(string)
	leName := h.LookupLegalEntityName(strOf(raw["法人实体"]))
	if r := sunshineRule(reason, leName); r != "" { reject = append(reject, r) }
	// A14 装修费→集团: 若明细含装修费 feeTypeId, 核维度=集团 (维度字段 step 探, 读不到则转人工; 见执行注)
	return reject
}
func strOf(v interface{}) string { s, _ := v.(string); return s }
```
接入 (两模板都做): `rejectReasons = append(rejectReasons, h.rulePaymentSpecialSubject(raw)...)`

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./internal/handler/ -run TestSpecialSubject -v`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add -A server/internal/handler/hesi_audit_payment_rules*.go
git commit -m "feat(hesi): 张俊付款单规则 A13 阳光天际→悦伍 + A14 装修费→集团"
```

---

## Task 11: A4/A15/A16 核销类 (仅付款单·票到核销)

**Interfaces:** Produces: `func (h *DashboardHandler) rulePaymentWriteoff(raw map[string]interface{}) (reject, warn []string)`。

- [ ] **Step 1: discovery 核销关联字段**

票到核销单的关联在 `expenseLinks` 或专门字段。查:
```sql
SELECT code, JSON_EXTRACT(raw_json,'$.writtenOffMoney'),
  JSON_KEYS(raw_json) FROM hesi_flow
WHERE specification_id LIKE 'ID01KgaO6dcZtR%' AND active=1
  AND CAST(JSON_EXTRACT(raw_json,'$.writtenOffMoney') AS DECIMAL(14,2))>0 LIMIT 5;
```
确认: 哪个字段=核销类型标记, 哪个=关联原预付款单 ID, 原单未核销余额从何来。记作 `<WRITEOFF_LINK>`。**读不到关联 → 转人工 (不硬驳)**。

- [ ] **Step 2: 写失败测试**
```go
func TestWriteoffCrossCompany(t *testing.T) {
	// 跨公司核销 → 驳回
	if r := crossCompanyWriteoff("公司A", "公司B"); r == "" {
		t.Error("跨公司核销应驳回 (A15)")
	}
	if r := crossCompanyWriteoff("公司A", "公司A"); r != "" {
		t.Errorf("同公司核销应通过, got %q", r)
	}
}
```

- [ ] **Step 3: 写实现 + 接入** (仅 tmpl=payment 且有核销)
```go
// A15 跨公司核销禁止。
func crossCompanyWriteoff(origCompany, thisCompany string) string {
	if origCompany != "" && thisCompany != "" && origCompany != thisCompany {
		return "核销原预付款单与本单所属公司不一致, 禁止跨公司核销 (A15)"
	}
	return ""
}
func (h *DashboardHandler) rulePaymentWriteoff(raw map[string]interface{}) (reject, warn []string) {
	writtenOff := toFloat(raw["writtenOffMoney"])
	if writtenOff <= 0 {
		return nil, nil // 非核销单
	}
	// A4: 票到核销须关联原预付款单 (读 <WRITEOFF_LINK>); 缺失→驳回。
	// A15: 原单所属公司=本单 (读原单); A16: 核销额≤原单未核销余额→否则转人工。
	// 关联/原单读不到 → 转人工:
	warn = append(warn, "核销单据, 请人工核对原预付款单关联/余额 (A4/A15/A16)")
	return reject, warn
}
```
接入 (仅 payment):
```go
	if tmpl == "payment" {
		if rej, w := h.rulePaymentWriteoff(raw); len(rej) > 0 || len(w) > 0 {
			rejectReasons = append(rejectReasons, rej...)
			warnings = append(warnings, w...)
		}
	}
```
> 执行时按 step1 的 `<WRITEOFF_LINK>` 把"转人工兜底"升级为真实的 A4/A15/A16 判定 (能读到关联就硬判, 读不到才转人工)。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./internal/handler/ -run TestWriteoffCrossCompany -v`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add -A server/internal/handler/hesi_audit_payment_rules*.go
git commit -m "feat(hesi): 张俊付款单规则 A4/A15/A16 核销类 (跨公司禁止+余额转人工)"
```

---

## Task 12: A18 支付金额上限

**Interfaces:** Produces: `func rulePaymentAmountCap(raw map[string]interface{}, invoiceTotal float64) (warn string)`。

- [ ] **Step 1: 写失败测试**
```go
func TestPaymentAmountCap(t *testing.T) {
	// 支付额 > 发票合计 → 转人工
	raw := map[string]interface{}{"payMoney": float64(1000)}
	if w := rulePaymentAmountCap(raw, 800); w == "" {
		t.Error("支付额超发票合计应转人工 (A18)")
	}
	if w := rulePaymentAmountCap(raw, 1000); w != "" {
		t.Errorf("支付额≤发票应通过, got %q", w)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd server && go test ./internal/handler/ -run TestPaymentAmountCap -v`
Expected: FAIL

- [ ] **Step 3: 写实现 + 接入** (仅付款单; 预付款单无发票跳过)
```go
// A18 支付金额 ≤ 发票合计 + 无票 + 退回备用金 (此处先用发票合计兜底, 无票/退回备用金待 discovery 字段后加)。超→转人工。
func rulePaymentAmountCap(raw map[string]interface{}, invoiceTotal float64) string {
	pay := toFloat(raw["payMoney"])
	if invoiceTotal > 0 && pay > invoiceTotal+0.01 {
		return fmt.Sprintf("支付额 ¥%.2f 超过发票合计 ¥%.2f, 请人工核对 (A18)", pay, invoiceTotal)
	}
	return ""
}
```
接入 (仅 payment): `if tmpl == "payment" { if w := rulePaymentAmountCap(raw, h.sumInvoiceTotal(flowID)); w != "" { warnings = append(warnings, w) } }`

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./internal/handler/ -run TestPaymentAmountCap -v`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add -A server/internal/handler/hesi_audit_payment_rules*.go
git commit -m "feat(hesi): 张俊付款单规则 A18 支付金额上限 (转人工)"
```

---

## Task 13: B1 收款方=开票方 + B2 购买方=所属公司 (发票核验)

**Interfaces:**
- Consumes: `hesi_flow_invoice` (buyer_name/seller_name); `h.payeeName(id)`; `h.LookupLegalEntityName(id)`。
- Produces: `func (h *DashboardHandler) rulePaymentInvoiceParties(flowID string, raw map[string]interface{}) (reject, warn []string)`。

- [ ] **Step 1: 写失败测试** (纯比对逻辑)
```go
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
	// 购买方 != 所属公司 → 驳回
	if r := buyerCompanyMismatch([]string{"公司X"}, "公司Y"); r == "" {
		t.Error("购买方≠所属公司应驳回 (B2)")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd server && go test ./internal/handler/ -run TestInvoicePartyMatch -v`
Expected: FAIL

- [ ] **Step 3: 写实现 + 接入**
```go
// B1 收款方=开票方 (任一发票匹配即可) → 否则转人工。
func payeeSellerMismatch(payee string, sellers []string) string {
	if payee == "" || len(sellers) == 0 {
		return "" // 缺数据由 B 档兜底转人工逻辑统一处理
	}
	for _, s := range sellers {
		if s != "" && s == payee {
			return ""
		}
	}
	return "收款方与发票开票方不一致, 请人工核对 (B1)"
}
// B2 购买方=所属公司 (全部发票均须一致) → 否则驳回。
func buyerCompanyMismatch(buyers []string, ownerCompany string) string {
	if ownerCompany == "" || len(buyers) == 0 {
		return ""
	}
	for _, b := range buyers {
		if b != "" && b != ownerCompany {
			return "发票购买方与所属公司不一致 (B2)"
		}
	}
	return ""
}
func (h *DashboardHandler) rulePaymentInvoiceParties(flowID string, raw map[string]interface{}) (reject, warn []string) {
	buyers, sellers := h.invoiceParties(flowID) // Task13 step3b helper
	if len(buyers) == 0 && len(sellers) == 0 {
		warn = append(warn, "未识别到发票购买方/开票方, 请人工核对 (B1/B2)")
		return reject, warn
	}
	owner := h.LookupLegalEntityName(strOf(raw["法人实体"]))
	if r := buyerCompanyMismatch(buyers, owner); r != "" {
		reject = append(reject, r)
	}
	if w := payeeSellerMismatch(h.payeeName(strOf(raw["payeeId"])), sellers); w != "" {
		warn = append(warn, w)
	}
	return reject, warn
}
```
Step 3b — 加发票方查询 helper (用 5s ctx):
```go
func (h *DashboardHandler) invoiceParties(flowID string) (buyers, sellers []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := h.DB.QueryContext(ctx,
		`SELECT IFNULL(buyer_name,''), IFNULL(seller_name,'') FROM hesi_flow_invoice WHERE flow_id=?`, flowID)
	if err != nil { return nil, nil }
	defer rows.Close()
	for rows.Next() {
		var b, s string
		if rows.Scan(&b, &s) == nil {
			if b != "" { buyers = append(buyers, b) }
			if s != "" { sellers = append(sellers, s) }
		}
	}
	_ = rows.Err()
	return buyers, sellers
}
```
接入 (仅 payment, 预付款单无发票跳过):
```go
	if tmpl == "payment" {
		if rej, w := h.rulePaymentInvoiceParties(flowID, raw); len(rej) > 0 || len(w) > 0 {
			rejectReasons = append(rejectReasons, rej...)
			warnings = append(warnings, w...)
		}
	}
```
> 顶部 import 补 `"context"` `"time"`。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./internal/handler/ -run TestInvoicePartyMatch -v`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add -A server/internal/handler/hesi_audit_payment_rules*.go
git commit -m "feat(hesi): 张俊付款单规则 B1 收款方=开票方 + B2 购买方=所属公司"
```

---

## Task 14: B3 税额份数对账

**Interfaces:** Produces: `func (h *DashboardHandler) rulePaymentTaxCount(flowID string, raw map[string]interface{}) (warn string)`。

- [ ] **Step 1: discovery 单据填写的税额份数格式**

`u_WmLv_税额份数总计` 的值格式 (是"N张/¥X"文本还是结构): 
```sql
SELECT DISTINCT LEFT(JSON_UNQUOTE(JSON_EXTRACT(raw_json,'$."u_WmLv_税额份数总计"')),40)
FROM hesi_flow WHERE specification_id LIKE 'ID01KgaO6dcZtR%' AND active=1
  AND JSON_EXTRACT(raw_json,'$."u_WmLv_税额份数总计"') IS NOT NULL LIMIT 10;
```
确定解析方式 (记作解析为 `declaredCount`, `declaredTax`)。

- [ ] **Step 2: 写失败测试** (0税正常放过)
```go
func TestTaxCountReconcile(t *testing.T) {
	// 发票税额合计与申报不符 → 转人工
	if w := taxCountMismatch(2 /*invCount*/, 90.0 /*invTax*/, 2, 50.0); w == "" {
		t.Error("税额不符应转人工 (B3)")
	}
	// 一致 → 通过
	if w := taxCountMismatch(2, 90.0, 2, 90.0); w != "" {
		t.Errorf("一致应通过, got %q", w)
	}
	// 0 税且份数一致 → 通过 (0税正常)
	if w := taxCountMismatch(1, 0.0, 1, 0.0); w != "" {
		t.Errorf("0税正常应通过, got %q", w)
	}
}
```

- [ ] **Step 3: 写实现 + 接入** (仅付款单)
```go
// B3 税额份数对账: 发票实际(张数+税额合计) vs 单据申报。不符→转人工。0税正常 (不据此误判)。
func taxCountMismatch(invCount int, invTax float64, declCount int, declTax float64) string {
	if invCount != declCount {
		return fmt.Sprintf("发票张数(%d)与申报(%d)不符, 请人工核对 (B3)", invCount, declCount)
	}
	if math.Abs(invTax-declTax) > 0.01 {
		return fmt.Sprintf("发票税额合计 ¥%.2f 与申报 ¥%.2f 不符, 请人工核对 (B3)", invTax, declTax)
	}
	return ""
}
```
接入: 查发票张数 + SUM(tax_amount) (用 invoiceParties 同款 5s ctx 查询, 或扩 `h.sumInvoiceTotal`); 解析 step1 的申报值; 仅 payment。
> 顶部 import 补 `"math"`。申报值解析不出来 → 转人工 (安全底线)。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./internal/handler/ -run TestTaxCountReconcile -v`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add -A server/internal/handler/hesi_audit_payment_rules*.go
git commit -m "feat(hesi): 张俊付款单规则 B3 税额份数对账 (0税放过/不符转人工)"
```

---

## Task 15: B4 防重复付款 (付款单 + 预付款单都做)

**Interfaces:** Produces: `func (h *DashboardHandler) rulePaymentDuplicate(flowID string, raw map[string]interface{}) (warn string)`。

- [ ] **Step 1: 写失败测试** (sqlmock: 历史存在同收款方+同额单 → 提示)
```go
func TestDuplicatePayment(t *testing.T) {
	// 纯逻辑: 找到历史单号 → 提示文案含原单号
	if w := dupWarnText([]string{"B26001234"}); w == "" || !strings.Contains(w, "B26001234") {
		t.Errorf("应提示原单号, got %q", w)
	}
	if w := dupWarnText(nil); w != "" {
		t.Errorf("无重复应空, got %q", w)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd server && go test ./internal/handler/ -run TestDuplicatePayment -v`
Expected: FAIL

- [ ] **Step 3: 写实现 + 接入**
```go
// B4 防重复付款: 同收款方 + 金额完全一致的历史单 → 转人工 + 提示原单号 (付款单+预付款单都做)。
func dupWarnText(dupeCodes []string) string {
	if len(dupeCodes) == 0 {
		return ""
	}
	return "疑似重复付款 (同收款方+同金额), 历史单号: " + strings.Join(dupeCodes, ", ") + " (B4)"
}
func (h *DashboardHandler) rulePaymentDuplicate(flowID string, raw map[string]interface{}) string {
	payeeID, _ := raw["payeeId"].(string)
	amount := toFloat(raw["payMoney"])
	if payeeID == "" || amount <= 0 {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := h.DB.QueryContext(ctx,
		`SELECT code FROM hesi_flow
		 WHERE flow_id<>? AND active=1
		   AND JSON_UNQUOTE(JSON_EXTRACT(raw_json,'$.payeeId'))=?
		   AND ABS(IFNULL(pay_money,0)-?)<0.01
		   AND (specification_id LIKE 'ID01KgaO6dcZtR%' OR specification_id LIKE 'ID01FhdI9II9A3%')
		 LIMIT 5`, flowID, payeeID, amount)
	if err != nil { return "" }
	defer rows.Close()
	var codes []string
	for rows.Next() {
		var c string
		if rows.Scan(&c) == nil { codes = append(codes, c) }
	}
	_ = rows.Err()
	return dupWarnText(codes)
}
```
接入 (两模板都做): `if w := h.rulePaymentDuplicate(flowID, raw); w != "" { warnings = append(warnings, w) }`

- [ ] **Step 4: 跑测试确认通过**

Run: `cd server && go test ./internal/handler/ -run TestDuplicatePayment -v`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add -A server/internal/handler/hesi_audit_payment_rules*.go
git commit -m "feat(hesi): 张俊付款单规则 B4 防重复付款 (付款单+预付款单)"
```

---

## Task 16: dry-run 真实单据回测 + 误判率 + 前端展示验证

**Files:** Create `server/cmd/dry-run-payment-audit/main.go` (一次性回测探针, 仿 `cmd/dry-run-fan-audit`)

- [ ] **Step 1: discovery 现有 dry-run 探针模式**

Run: `cd server && grep -rln "AuditDailyExpense" cmd/` → 看 `cmd/dry-run-fan-audit/main.go` 怎么遍历单据调引擎。

- [ ] **Step 2: 写回测探针**

仿 dry-run-fan-audit, 改为: 拉张俊待审 (current_approver_name='张俊' + 两 spec 前缀) 全部单据, 逐单调 `h.AuditPayment`, 打印 code/模板/Action/Reasons 汇总 + 各动作计数。

- [ ] **Step 3: 跑回测**

Run: `cd server && go run ./cmd/dry-run-payment-audit`
Expected: 输出 张俊 70 付款单 + 4 预付款单 的建议分布 (reject/manual/agree 计数 + 明细)。

- [ ] **Step 4: 人工抽查误判率**

抽 reject 单 10~20 张人工核对; **误判率 < 10% 才算过** (同樊雪娇标准)。误判高的规则回到对应 Task 调参。

- [ ] **Step 5: 前端实测**

build + 重启 bi-server; 用张俊账号 (或管理员切张俊视角) 看待审列表 + 详情弹窗的 AI 建议正常显示。截图留证。

- [ ] **Step 6: Commit**
```bash
git add server/cmd/dry-run-payment-audit/main.go
git commit -m "test(hesi): 张俊付款单审批 dry-run 回测探针"
```

---

## Task 17: /code-review 二审 + 上线

- [ ] **Step 1:** `cd server && go build -o bi-server.exe ./cmd/server && go vet ./internal/handler/ && go test ./internal/handler/...` 全绿。
- [ ] **Step 2:** invoke `/code-review` (业务红线必须二审)。按反馈修。
- [ ] **Step 3:** 错峰重启 bi-server (清代理 env + 只 kill 8080 PID); 前端无改动 (复用樊雪娇展示) 则不必 build。
- [ ] **Step 4:** 钉钉知会张俊: AI 建议已上线, dry-run 仅供参考。
- [ ] **Step 5:** 发版 (跑哥定版号) — CHANGELOG + tag + notice (业务文案: 新增张俊付款单 AI 审批建议)。

---

## Self-Review (计划对照设计)

**Spec 覆盖**: 设计 §三 A1-A18 → Task 2~12; §四 B1-B4 → Task 13~15; 模板分支 §2.4 → Task 1 `paymentTemplate` + 各任务的 tmpl 分支; 触发 §2.2 → Task 1 step5; 回测/误判率 §七 → Task 16; 二审/上线 §十 → Task 17。**无遗漏规则**。

**占位符说明**: `<REASON_KEY>` / `<VIRTUAL_CUSTOMER_ID>` / `<BRAND_FIELD>` / `<RD_FIELD>` / `<WRITEOFF_LINK>` / `detectionFeeTypes` 等是设计 §8.2 明确"实施时拉真实样本定"的口径, **每个都配了具体 discovery 命令**作为该任务 step1, 不是空泛 TODO。读不到一律转人工 (安全底线), 不会硬驳。

**类型一致**: `AuditSuggestion{Action, Reasons}` 全程一致; helper 命名 `rulePaymentXxx`; `paymentTemplate` 返回 "payment"/"prepay"/"" 全程一致; `toFloat`/`strOf`/`detailsHaveFeeType`/`invoiceParties`/`codeOf` 在首次出现处定义, 后续复用。

**风险**: A4/A15/A16 核销、A10 审批链、B3 申报值解析在数据结构未完全确认前以"转人工"兜底, 执行时 discovery 后升级为硬判 — 符合"宁可转人工不误驳"底线。
