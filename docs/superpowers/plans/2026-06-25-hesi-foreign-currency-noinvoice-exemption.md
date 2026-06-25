# 合思 规则10 "外币(国外)无票"豁免 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development 或 executing-plans 逐任务实现。Steps 用 `- [ ]` 勾选跟踪。

**Goal:** 付款截图识别到外币/汇率 → 判"国外无票" → 规则10不驳 + 规则19跳过 → 自动通过。

**Architecture:** OCR 转写无票报销单的非发票图 → Go 纯函数查外币关键词 → `hesi_payment_ocr` 新列 `is_foreign` 标记(fx 行 status='fx' 与规则19金额逻辑隔离) → 审批规则读 `isForeignFlow` 做豁免/跳过。

**Tech Stack:** Go (net/http 标准库), MySQL, 阿里云百炼 qwen-vl-ocr。

## Global Constraints
- go build CWD 必须 `server/`(`cd server && go build ...`)。
- 数据库表/字段注释必须中文。`ON DUPLICATE KEY UPDATE` 必须配 UNIQUE KEY(file_id 是 PK ✓)。
- 财务红线: 全部完成后 **/code-review 二审 → 跑哥明说才部署**。本计划只到"本地验证完成"。
- 不碰已验证的 `RecognizePaymentAmount`(金额路径无 HTTP 测试兜底)。
- 改动文件: `internal/ocr/{foreign.go,foreign_test.go,client.go}`, `internal/handler/{hesi_payment_ocr.go,hesi_payment_ocr_test.go,hesi_audit_rules.go}`, `cmd/ocr-hesi-payment/main.go`。

---

### Task 1: 外币标记纯函数 `HasForeignCurrencyMarker`

**Files:**
- Create: `server/internal/ocr/foreign.go`
- Test: `server/internal/ocr/foreign_test.go`

**Interfaces:**
- Produces: `ocr.HasForeignCurrencyMarker(text string) bool`

- [ ] **Step 1: 写失败测试** `server/internal/ocr/foreign_test.go`

```go
package ocr

import "testing"

func TestHasForeignCurrencyMarker(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"超优汇率 1美元 = 6.805人民币(黄金会员汇率)", true}, // B26003947 实例
		{"Pockyt Shop 订单金额 884.65元(130.00美元)", true},
		{"HK$ 980.00 港币 支付成功", true},
		{"Google Play USD 19.99", true},
		{"€ 49.00 EUR", true},
		{"微信支付 -529.00 交易成功", false}, // 纯人民币付款截图
		{"实付款 ¥175.91 交易成功", false},   // 人民币符号不算外币
		{"报销说明: 办公用品采购", false},
	}
	for _, c := range cases {
		if got := HasForeignCurrencyMarker(c.in); got != c.want {
			t.Errorf("HasForeignCurrencyMarker(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: 跑测试确认失败** — `cd server && go test ./internal/ocr/ -run TestHasForeignCurrencyMarker` → FAIL `undefined: HasForeignCurrencyMarker`

- [ ] **Step 3: 写实现** `server/internal/ocr/foreign.go`

```go
package ocr

import "strings"

// foreignCurrencyMarkers 外币/汇率标记 (跑哥 2026-06-25 确认): OCR 转写文本命中任一即判该截图为国外交易。
var foreignCurrencyMarkers = []string{
	"汇率",
	"美元", "USD", "港币", "HKD", "欧元", "EUR", "日元", "JPY", "英镑", "GBP",
	"$", "€", "£",
}

// HasForeignCurrencyMarker 判定 OCR 转写文本里是否出现外币或汇率标记。
// 用于规则10"外币(国外)无票"豁免: 国外报销无中国发票, 凭境外付款截图。
func HasForeignCurrencyMarker(text string) bool {
	for _, m := range foreignCurrencyMarkers {
		if strings.Contains(text, m) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: 跑测试确认通过** — `cd server && go test ./internal/ocr/ -run TestHasForeignCurrencyMarker` → PASS

- [ ] **Step 5: Commit** — `git add server/internal/ocr/foreign.go server/internal/ocr/foreign_test.go && git commit -m "feat(ocr): 加外币/汇率标记检测纯函数 HasForeignCurrencyMarker"`

---

### Task 2: OCR 全文转写 `TranscribeText`

**Files:**
- Modify: `server/internal/ocr/client.go`(在 `RecognizePaymentAmount` 之后追加, 不改它)

**Interfaces:**
- Consumes: `ShrinkJPEG`, `dashScopeEndpoint`(同包已有)
- Produces: `ocr.TranscribeText(ctx context.Context, apiKey string, img []byte) (string, error)`

说明: 结构与 `RecognizePaymentAmount` 同(缩图→qwen-vl-ocr), 仅 prompt 改"转写全文"、返回 raw 文本不解析金额。**故意不抽公共 helper / 不动 RecognizePaymentAmount**(金额路径无 HTTP 测试兜底, 零风险优先; DRY 留作上次二审 findings 的 reuse 项)。HTTP 调用无单测(同 RecognizePaymentAmount), 靠 Task 5 端到端验证。

- [ ] **Step 1: 追加实现** `server/internal/ocr/client.go` 末尾

```go
const transcribePrompt = "请识别并完整输出这张图片中的所有文字内容, 原样输出, 不要总结、不要解释。"

// TranscribeText 转写图片全文 (用于外币检测 HasForeignCurrencyMarker)。缩图→qwen-vl-ocr→返回原始文本。
// 注: 与 RecognizePaymentAmount 共用 DashScope 端点, 故意不抽公共 helper 以免动已验证的金额路径。
func TranscribeText(ctx context.Context, apiKey string, img []byte) (string, error) {
	small, err := ShrinkJPEG(img, 1280)
	if err != nil {
		return "", err
	}
	dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(small)
	body := map[string]any{
		"model": "qwen-vl-ocr",
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "image_url", "image_url": map[string]string{"url": dataURL}},
				map[string]any{"type": "text", "text": transcribePrompt},
			},
		}},
	}
	bs, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("序列化转写请求体失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", dashScopeEndpoint, bytes.NewReader(bs))
	if err != nil {
		return "", fmt.Errorf("构造转写请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if resp.StatusCode != 200 || len(out.Choices) == 0 {
		return "", fmt.Errorf("转写调用失败 http %d: %s %s", resp.StatusCode, out.Error.Code, out.Error.Message)
	}
	return out.Choices[0].Message.Content, nil
}
```

- [ ] **Step 2: 编译 + 现有 ocr 测试全过** — `cd server && go build ./internal/ocr/ && go test ./internal/ocr/` → ok(ParseAmount/ShrinkJPEG/HasForeignCurrencyMarker 都过)

- [ ] **Step 3: Commit** — `git add server/internal/ocr/client.go && git commit -m "feat(ocr): 加 TranscribeText 图片全文转写(外币检测用)"`

---

### Task 3: 存取层 — is_foreign 列 / 隔离规则19 / isForeignFlow / UpsertForeignScan

**Files:**
- Modify: `server/internal/handler/hesi_payment_ocr.go`
- Test: `server/internal/handler/hesi_payment_ocr_test.go`(追加 sqlmock 测试)

**Interfaces:**
- Produces: `(h *DashboardHandler) isForeignFlow(flowID string) bool`; `UpsertForeignScan(db *sql.DB, fileID, flowID, fileName string, isForeign bool, raw string) error`
- 改: `ensurePaymentOcrTable`(CREATE 加列 + 老表 ALTER 迁移); `getPaymentOcrByFlow`(加 `AND status<>'fx'`)

- [ ] **Step 1: 写失败测试**(追加到 `hesi_payment_ocr_test.go`)

```go
func TestIsForeignFlowTrue(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &DashboardHandler{DB: db}
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("FX1").
		WillReturnRows(sqlmock.NewRows([]string{"x"}).AddRow(1))
	if !h.isForeignFlow("FX1") {
		t.Error("有 is_foreign=1 行应返回 true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestUpsertForeignScan(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// 绑定参数只有 5 个: fileID, flowID, fileName, raw, isForeign (status='fx'/amount=0 是 SQL 字面量)
	mock.ExpectExec("INSERT INTO hesi_payment_ocr").
		WithArgs("F1", "FLOW1", "x.jpg", "汇率…", true).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := UpsertForeignScan(db, "F1", "FLOW1", "x.jpg", true, "汇率…"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestGetPaymentOcrByFlowExcludesFx(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("status<>'fx'").
		WithArgs("FLOW1").
		WillReturnRows(sqlmock.NewRows([]string{"file_id", "amount", "status"}).AddRow("F1", 100.0, "ok"))
	rows, err := getPaymentOcrByFlow(db, "FLOW1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Status != "ok" {
		t.Errorf("应只返回非fx行, 得 %+v", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
```

- [ ] **Step 2: 跑测试确认失败** — `cd server && go test ./internal/handler/ -run "TestIsForeignFlow|TestUpsertForeignScan|TestGetPaymentOcrByFlowExcludesFx"` → FAIL(undefined / SQL 不匹配)

- [ ] **Step 3: 实现** 改 `hesi_payment_ocr.go`:

(a) `ensurePaymentOcrTable` 的 CREATE TABLE 里, `raw_text` 行之后加一列:
```go
  is_foreign TINYINT     DEFAULT NULL COMMENT '外币检测: NULL=未扫 0=无外币 1=有外币(国外无票)',
```
并在 CREATE 之后追加幂等 ALTER 迁移老表(仿 ensureCfPresetTable scope 迁移):
```go
	// 幂等迁移: 老表补 is_foreign 列 (外币国外无票, 跑哥 2026-06-25)
	var col string
	if err := db.QueryRow(`SELECT COLUMN_NAME FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='hesi_payment_ocr' AND COLUMN_NAME='is_foreign'`).Scan(&col); err == sql.ErrNoRows {
		if _, e := db.Exec(`ALTER TABLE hesi_payment_ocr ADD COLUMN is_foreign TINYINT DEFAULT NULL COMMENT '外币检测: NULL=未扫 0=无外币 1=有外币(国外无票)'`); e != nil {
			return fmt.Errorf("ensurePaymentOcrTable 迁移 is_foreign: %w", e)
		}
	}
```

(b) `getPaymentOcrByFlow` 的 SQL 加 `AND status<>'fx'`:
```go
		`SELECT file_id, amount, status FROM hesi_payment_ocr WHERE flow_id=? AND status<>'fx'`,
```

(c) 新增 `isForeignFlow` + `UpsertForeignScan`:
```go
// isForeignFlow 该单是否检测到外币(国外无票)。供规则10豁免E + 规则19跳过用。
func (h *DashboardHandler) isForeignFlow(flowID string) bool {
	if h.DB == nil || flowID == "" {
		return false
	}
	var ok bool
	if err := h.DB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM hesi_payment_ocr WHERE flow_id=? AND is_foreign=1)`, flowID,
	).Scan(&ok); err != nil {
		return false // 查不到/出错 → 不豁免(安全: 维持原判定)
	}
	return ok
}

// UpsertForeignScan 写外币检测结果。新图(付款金额OCR没处理过)插 status='fx' 行;
// 已有行(付款关键词图)只更 is_foreign/raw_text, 不动 amount/status (PK=file_id)。
func UpsertForeignScan(db *sql.DB, fileID, flowID, fileName string, isForeign bool, raw string) error {
	_, err := db.Exec(
		`INSERT INTO hesi_payment_ocr (file_id, flow_id, file_name, amount, status, raw_text, is_foreign)`+
			` VALUES (?, ?, ?, 0, 'fx', ?, ?)`+
			` ON DUPLICATE KEY UPDATE is_foreign=VALUES(is_foreign), raw_text=VALUES(raw_text)`,
		fileID, flowID, fileName, raw, isForeign,
	)
	if err != nil {
		return fmt.Errorf("UpsertForeignScan: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: 跑测试确认通过** — `cd server && go test ./internal/handler/ -run "TestIsForeignFlow|TestUpsertForeignScan|TestGetPaymentOcrByFlowExcludesFx"` → PASS。再跑全包 `go test ./internal/handler/` 确认没碰坏现有 OCR 测试。

- [ ] **Step 5: Commit** — `git add server/internal/handler/hesi_payment_ocr.go server/internal/handler/hesi_payment_ocr_test.go && git commit -m "feat(hesi): hesi_payment_ocr加is_foreign列+isForeignFlow/UpsertForeignScan, 隔离规则19"`

---

### Task 4: 接进审批 — 规则10 豁免E + 规则19 跳过

**Files:**
- Modify: `server/internal/handler/hesi_audit_rules.go`

**Interfaces:**
- Consumes: `h.isForeignFlow(flowID)`(Task 3)
- 改: `ruleInvoiceChecks` 签名加 `isForeign bool`; `AuditDailyExpense` 计算一次 isForeign 并传入 + gate 规则19

- [ ] **Step 1: 改 `ruleInvoiceChecks` 签名 + 加豁免E**

签名(line 1157)改为:
```go
func (h *DashboardHandler) ruleInvoiceChecks(raw map[string]interface{}, ownerDeptID, flowID string, isForeign bool) ([]string, []string) {
```
在无票循环(line 1255 `for _, did := range allDetailIDs {`)里, 紧跟 `if detailHasInvoice[did] { continue }` 之后加:
```go
		// 豁免 E: 外币(国外)无票 — 整单检测到外币汇率 → 无票OK, 自动通过 (跑哥 2026-06-25)
		if isForeign {
			continue
		}
```

- [ ] **Step 2: 改调用点 + gate 规则19**(`AuditDailyExpense`)

在 `var rejectReasons []string`(line 372)之前加:
```go
	// 外币(国外)无票判定: 整单一次, 供规则10豁免E + 规则19跳过 (跑哥 2026-06-25)
	isForeign := h.isForeignFlow(flowID)
```
规则8+10 调用(line 453)改为传 isForeign:
```go
	if invRej, invWarn := h.ruleInvoiceChecks(raw, submitDeptID, flowID, isForeign); len(invRej) > 0 || len(invWarn) > 0 {
```
规则19(line 525)改为外币则跳过:
```go
	// 规则 19: 付款截图金额 vs 发票总额核对 (口径B)。外币(国外)单无发票可比, 跳过 (跑哥 2026-06-25)。
	if !isForeign {
		if pc := h.checkFlowPayment(flowID); pc.Flag {
			warnings = append(warnings, pc.Note)
		}
	}
```

- [ ] **Step 3: 编译 + 全包测试** — `cd server && go build ./... && go test ./internal/handler/` → 全过(确认签名改动没漏改其它调用点)

- [ ] **Step 4: Commit** — `git add server/internal/handler/hesi_audit_rules.go && git commit -m "feat(hesi): 规则10加外币(国外)无票豁免E + 规则19外币单跳过"`

---

### Task 5: 跑批工具加外币扫描 pass

**Files:**
- Modify: `server/cmd/ocr-hesi-payment/main.go`

**Interfaces:**
- Consumes: `ocr.TranscribeText`, `ocr.HasForeignCurrencyMarker`, `handler.UpsertForeignScan`, 现有 `getAttachmentURLs`/`downloadImage`/`truncate`/`pendingRow`

- [ ] **Step 1: 加 `runForeignScan` 函数**(放在 `main` 之后)

```go
// runForeignScan 外币(国外)无票检测: 扫无票报销单的非发票图, OCR转写→查外币标记→标 is_foreign。
// 与金额扫描分开: 范围是"无发票单的所有非发票图"(含哈希命名), 不挂付款关键词。
func runForeignScan(db *sql.DB, token, apiKey, flowCode string) {
	baseSQL := `SELECT a.flow_id, a.file_id, a.file_name
FROM hesi_flow_attachment a
JOIN hesi_flow f ON f.flow_id = a.flow_id
LEFT JOIN hesi_flow_invoice i ON i.flow_id = f.flow_id
WHERE f.form_type='expense' AND f.state IN ('approving','pending')
  AND a.is_invoice=0
  AND (LOWER(a.file_name) LIKE '%.jpg' OR LOWER(a.file_name) LIKE '%.jpeg' OR LOWER(a.file_name) LIKE '%.png')
  AND i.flow_id IS NULL
  AND a.file_id NOT IN (SELECT file_id FROM hesi_payment_ocr WHERE is_foreign IS NOT NULL)`
	var rows *sql.Rows
	var err error
	if flowCode != "" {
		rows, err = db.Query(baseSQL+" AND f.code = ? GROUP BY a.flow_id, a.file_id, a.file_name", flowCode)
	} else {
		rows, err = db.Query(baseSQL + " GROUP BY a.flow_id, a.file_id, a.file_name")
	}
	if err != nil {
		log.Printf("[fx] 查询待检测外币截图失败: %v", err)
		return
	}
	var pending []pendingRow
	flowIDSet := make(map[string]bool)
	for rows.Next() {
		var r pendingRow
		if err := rows.Scan(&r.FlowID, &r.FileID, &r.FileName); err != nil {
			log.Printf("[fx] Scan失败: %v", err)
			continue
		}
		pending = append(pending, r)
		flowIDSet[r.FlowID] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		log.Printf("[fx] 查询行错误: %v", err)
		return
	}
	if len(pending) == 0 {
		log.Println("[fx] 无待检测外币截图")
		return
	}
	log.Printf("[fx] 待检测外币截图: %d 张, 涉及 %d 个无票单", len(pending), len(flowIDSet))

	flowIDs := make([]string, 0, len(flowIDSet))
	for fid := range flowIDSet {
		flowIDs = append(flowIDs, fid)
	}
	urlMap, _ := getAttachmentURLs(token, flowIDs)

	ctx := context.Background()
	nFx := 0
	for i, row := range pending {
		att, ok := urlMap[row.FileID]
		if !ok {
			log.Printf("[fx][%d/%d] file_id=%s 无签名URL, 跳过(留NULL下轮重试)", i+1, len(pending), row.FileID)
			continue
		}
		img, err := downloadImage(att.URL)
		if err != nil {
			log.Printf("[fx][%d/%d] 下载失败: %v", i+1, len(pending), err)
			continue
		}
		text, err := ocr.TranscribeText(ctx, apiKey, img)
		if err != nil {
			log.Printf("[fx][%d/%d] 转写失败: %v", i+1, len(pending), err)
			continue
		}
		isForeign := ocr.HasForeignCurrencyMarker(text)
		if err := handler.UpsertForeignScan(db, row.FileID, row.FlowID, row.FileName, isForeign, truncate(text, 500)); err != nil {
			log.Printf("[fx][%d/%d] upsert失败: %v", i+1, len(pending), err)
		}
		if isForeign {
			nFx++
			log.Printf("[fx][%d/%d] 国外无票: flow=%s file=%s", i+1, len(pending), row.FlowID, row.FileName)
		}
		time.Sleep(300 * time.Millisecond)
	}
	log.Printf("[fx] 外币检测完成: 共 %d 张, 标记 %d 张为国外无票", len(pending), nFx)
}
```

- [ ] **Step 2: 在 `main()` 末尾(金额跑批 log 之后)调用**

在 `log.Printf("========== 跑批完成...` 之前加:
```go
	// 外币(国外)无票检测 pass (跑哥 2026-06-25)
	log.Println("=== 外币(国外)无票检测 ===")
	runForeignScan(db, token, cfg.DashScope.APIKey, *flowCode)
```

- [ ] **Step 3: 编译 + vet** — `cd server && go vet ./cmd/ocr-hesi-payment/ && go build -o ocr-hesi-payment.new.exe ./cmd/ocr-hesi-payment` → OK

- [ ] **Step 4: Commit** — `git add server/cmd/ocr-hesi-payment/main.go && git commit -m "feat(hesi): 跑批工具加外币(国外)无票检测pass"`

---

### Task 6: 端到端验证(B26003947)

- [ ] **Step 1: 测试模式跑** — `cd server && ./ocr-hesi-payment.new.exe -flow B26003947`(配置/合思token/dashscope key 走 config.json), 看日志 `[fx] 国外无票: flow=ID01U4XiZ1T93V`
- [ ] **Step 2: 查库确认** — `SELECT file_id,file_name,status,is_foreign FROM hesi_payment_ocr WHERE flow_id='ID01U4XiZ1T93V'` → 有 is_foreign=1 行
- [ ] **Step 3: 审批建议验证** — 重启 bi-server(新 exe 含规则改动)后, 该单 AI 建议从"驳回"变"通过"(/api/.../audit 或挂起页实测; 留 SQL/截图)
- [ ] **Step 4: 不误伤回归** — 抽一个纯人民币无票单确认仍按原规则(研发样品/补贴豁免不变, 不属豁免的仍驳回)

---

## 上线(本计划之外, 待跑哥)
1. `cd server && go build -o bi-server.exe ./cmd/server`(规则改动) + 部署 ocr-hesi-payment.exe(留 .old.exe)
2. **/code-review 二审**(财务红线铁律)
3. 跑哥明说 → 错峰 kill 8080 重启 bi-server + 跑一次外币扫描回填存量无票单
4. 发版/notice 随积压一起(版号跑哥定)
