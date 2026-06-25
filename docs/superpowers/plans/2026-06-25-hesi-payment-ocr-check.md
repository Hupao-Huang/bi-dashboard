# 合思报销单「付款截图金额 vs 发票金额」核对 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 报销单审批机器人(AuditDailyExpense)自动核对「付款截图实付总额」与「发票价税合计总额」,付款 > 发票时给出"人工复核"建议。

**Architecture:** 三层解耦,OCR 绝不在审批页同步跑(避免冷缓存卡死,见 [[feedback_hesi_cold_cache_request_path]]):
1. **OCR 客户端**(`internal/ocr`):缩图(长边≤1280)→ 阿里云百炼 qwen-vl-ocr(OpenAI 兼容)→ 解析金额。纯函数 + HTTP,可单测。
2. **后台 OCR 跑批**(`cmd/ocr-hesi-payment`,schtask 每小时):查"待审报销单里还没 OCR 的付款截图"→ 合思拿签名下载链 → 下图 → OCR → 存进 `hesi_payment_ocr`(按 file_id 缓存,只 OCR 一次)。
3. **对账规则**(`hesi_audit_rules.go` 新函数):读缓存的付款金额 + `hesi_flow_invoice.total_amount`,按口径 B 判定,挂进 AuditDailyExpense 的建议里。

**Tech Stack:** Go(net/http 标准库)+ MySQL + 阿里云百炼 DashScope(qwen-vl-ocr,OpenAI 兼容端点)+ `golang.org/x/image/draw`(缩图)。

## Global Constraints

- **口径 B(跑哥 2026-06-25 拍板,逐字)**:范围=报销单(expense,商品采购+差旅都纳入);核对=付款截图实付总额(**取绝对值**,支付截图支出显负数)vs 发票价税合计总额(`hesi_flow_invoice.total_amount` 求和);判定=**付款 ≤ 发票 → 正常放行;付款 > 发票(超过容差)→ 标记"人工复核"**。
- **OCR 不进同步关键路径、不进审批页同步路径**:只在独立 schtask 跑批,审批页只读缓存。OCR 失败/未跑完 = 该单标"付款截图待识别",**不误判、不阻塞**。
- **缩图必做**:qwen-vl-ocr 直接吃高清原图(1170×2532)会稳定读错返"100",长边缩到 ≤1280(JPEG q88)才准。
- **财务红线**:本功能改动审批机器人 = 业务红线。上线前必过 `/code-review` 二审 + 跑哥明确说"上线"。
- **金额精度**:用 `decimal(14,4)` 存 OCR 金额;Go 内比较用 `float64` + 容差(默认 0.01,可配)。
- **密钥不硬编码**:DashScope key 进 `config.json`,经 config.Load 读入。key 当前值 `sk-ws-...`(跑哥 2026-06-25 提供,实测可用)。
- **数据库注释中文**;新表/字段必须有中文注释。
- **OCR 调用前清代理 env**(阿里云直连,shell 自带 xray 代理会劫持;Go 进程用 `config` 不读 env proxy 或显式 `Transport{Proxy:nil}`)。

---

### Task 1: OCR 客户端(缩图 + qwen-vl-ocr + 解析金额)

**Files:**
- Create: `server/internal/ocr/client.go`
- Test: `server/internal/ocr/client_test.go`

**Interfaces:**
- Produces:
  - `func ParseAmount(raw string) (float64, error)` — 从模型返回的文本里抠出金额(兼容 `175.91` / `-529.00` / `¥175.91` / `175.91元` / ```{"实付款":"175.91"}```),保留符号。
  - `func ShrinkJPEG(img []byte, longest int) ([]byte, error)` — 解码任意 png/jpg,长边 > longest 时等比缩放,重编码 JPEG(q88)。
  - `func RecognizePaymentAmount(ctx context.Context, apiKey string, img []byte) (amount float64, raw string, err error)` — 缩图→调 qwen-vl-ocr→ParseAmount。amount 保留符号(调用方取绝对值)。
  - `const dashScopeEndpoint = "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"`
  - `const ocrPrompt = "这是一张付款/支付截图。只返回本次实际支付的总金额数字，例如175.91。不要手续费、余额、原价、优惠、红包、运费、数量等其它数字。只输出一个数字。"`

- [ ] **Step 1: Write failing test for ParseAmount**

```go
package ocr

import (
	"math"
	"testing"
)

func TestParseAmount(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"175.91", 175.91},
		{"-529.00", -529.00},          // 支付截图支出显负数, 保留符号
		{"¥118.00", 118.00},
		{"175.91元", 175.91},
		{"实付款: 1,520.14", 1520.14},   // 含千分位
		{"```json\n{\"实付款\": \"175.91\"}\n```", 175.91},
	}
	for _, c := range cases {
		got, err := ParseAmount(c.in)
		if err != nil {
			t.Fatalf("ParseAmount(%q) err: %v", c.in, err)
		}
		if math.Abs(got-c.want) > 0.001 {
			t.Errorf("ParseAmount(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if _, err := ParseAmount("交易成功"); err == nil {
		t.Error("ParseAmount(无数字) 应报错")
	}
}
```

- [ ] **Step 2: Run test, verify FAIL**

Run: `cd server && go test ./internal/ocr/ -run TestParseAmount -v`
Expected: FAIL（`undefined: ParseAmount`）

- [ ] **Step 3: Implement ParseAmount**

```go
package ocr

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var amountRe = regexp.MustCompile(`-?\d[\d,]*\.?\d*`)

// ParseAmount 从模型返回文本抠出第一个金额数字, 保留正负号(支付截图支出常为负).
func ParseAmount(raw string) (float64, error) {
	m := amountRe.FindString(raw)
	if m == "" {
		return 0, fmt.Errorf("未找到金额数字: %q", raw)
	}
	m = strings.ReplaceAll(m, ",", "")
	v, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return 0, fmt.Errorf("金额解析失败 %q: %w", m, err)
	}
	return v, nil
}
```

- [ ] **Step 4: Run test, verify PASS**

Run: `cd server && go test ./internal/ocr/ -run TestParseAmount -v`
Expected: PASS

- [ ] **Step 5: Write failing test for ShrinkJPEG**

```go
package ocr

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestShrinkJPEG(t *testing.T) {
	// 造一张 2000x4000 的 png
	src := image.NewRGBA(image.Rect(0, 0, 2000, 4000))
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	out, err := ShrinkJPEG(buf.Bytes(), 1280)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Height != 1280 || cfg.Width != 640 {
		t.Errorf("缩后尺寸 = %dx%d, want 640x1280", cfg.Width, cfg.Height)
	}
}
```

- [ ] **Step 6: Run test, verify FAIL**

Run: `cd server && go test ./internal/ocr/ -run TestShrinkJPEG -v`
Expected: FAIL（`undefined: ShrinkJPEG`）

- [ ] **Step 7: Add x/image dep + implement ShrinkJPEG**

Run: `cd server && go get golang.org/x/image/draw`

```go
import (
	"bytes"
	"image"
	"image/jpeg"
	_ "image/png"

	xdraw "golang.org/x/image/draw"
)

// ShrinkJPEG 解码 png/jpg, 长边 > longest 时等比缩小, 重编码为 JPEG(q88).
func ShrinkJPEG(img []byte, longest int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(img))
	if err != nil {
		return nil, fmt.Errorf("解码图片失败: %w", err)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if max := w; h > max {
		max = h
	}
	dw, dh := w, h
	if mx := w; h > mx {
		mx = h
	}
	if m := w; h > m {
		m = h
	}
	// 等比: 长边缩到 longest
	long := w
	if h > w {
		long = h
	}
	if long > longest {
		scale := float64(longest) / float64(long)
		dw = int(float64(w) * scale)
		dh = int(float64(h) * scale)
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 88}); err != nil {
		return nil, fmt.Errorf("编码 JPEG 失败: %w", err)
	}
	return out.Bytes(), nil
}
```

> NOTE during impl: 上面的 dw/dh 计算清理掉冗余的 max 变量,只保留 long/scale 那段。这里写全是为了让测试先过,执行时精简成单一 long 计算。

- [ ] **Step 8: Run test, verify PASS**

Run: `cd server && go test ./internal/ocr/ -run TestShrinkJPEG -v`
Expected: PASS

- [ ] **Step 9: Implement RecognizePaymentAmount (HTTP, 无独立单测,集成时验)**

```go
import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"
)

func RecognizePaymentAmount(ctx context.Context, apiKey string, img []byte) (float64, string, error) {
	small, err := ShrinkJPEG(img, 1280)
	if err != nil {
		return 0, "", err
	}
	dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(small)
	body := map[string]any{
		"model": "qwen-vl-ocr",
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "image_url", "image_url": map[string]string{"url": dataURL}},
				map[string]any{"type": "text", "text": ocrPrompt},
			},
		}},
	}
	bs, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", dashScopeEndpoint, bytes.NewReader(bs))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	// 阿里云直连, 不走 env 代理
	client := &http.Client{Timeout: 60 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	var out struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, "", err
	}
	if resp.StatusCode != 200 || len(out.Choices) == 0 {
		return 0, "", fmt.Errorf("ocr http %d", resp.StatusCode)
	}
	raw := out.Choices[0].Message.Content
	amt, err := ParseAmount(raw)
	return amt, raw, err
}
```

- [ ] **Step 10: Build + commit**

```bash
cd server && go build ./internal/ocr/ && go test ./internal/ocr/ -v
git add server/internal/ocr/ server/go.mod server/go.sum
git commit -m "feat(ocr): 加 qwen-vl-ocr 付款截图金额识别(缩图+解析+取绝对值)"
```

---

### Task 2: config 加 DashScope key

**Files:**
- Modify: `server/internal/config/config.go`
- Modify: `server/config.json`

**Interfaces:**
- Produces: `config.Config.DashScopeKey string`(读自 config.json `dashscope.api_key`)

- [ ] **Step 1: 看现有 config 结构**

Run: `cd server && grep -n "Hesi\|json:\"hesi\"\|type Config" internal/config/config.go`
Expected: 看到 Config struct 和 hesi 段的写法,照抄风格。

- [ ] **Step 2: 加字段 + 解析**

在 `config.go` 的 Config struct(或对应嵌套结构)加:

```go
DashScope struct {
	APIKey string `json:"api_key"`
} `json:"dashscope"`
```

- [ ] **Step 3: config.json 加段**

```json
"dashscope": {
    "api_key": "sk-ws-H.RYHXEYL.Kapb.MEUCIDtk8twM51GcUBiejK69xPrv9xf5VOL1i_ElYUCap_wFAiEA3lt3vIKU6hJuiiKbuVMlRyt0cI1VmGdNSBQFkbmQrng"
},
```

- [ ] **Step 4: Build + commit**

```bash
cd server && go build ./...
git add server/internal/config/config.go
git commit -m "feat(config): 加阿里云百炼 DashScope key 配置"
# 注意: config.json 在 .gitignore, 不 commit, 但要确认本地已加该段
```

---

### Task 3: `hesi_payment_ocr` 缓存表 + 存取层

**Files:**
- Create: `server/internal/handler/hesi_payment_ocr.go`
- Test: `server/internal/handler/hesi_payment_ocr_test.go`

**Interfaces:**
- Produces:
  - `func ensurePaymentOcrTable(db *sql.DB) error` — 懒建表(仿 ensureCfPresetTable)。
  - `func upsertPaymentOcr(db *sql.DB, fileID, flowID, fileName string, amount float64, status, raw string) error`
  - `type paymentOcrRow struct { FileID string; Amount float64; Status string }`
  - `func getPaymentOcrByFlow(db *sql.DB, flowID string) ([]paymentOcrRow, error)`
- 表结构:
```sql
CREATE TABLE IF NOT EXISTS hesi_payment_ocr (
  file_id     VARCHAR(128) NOT NULL COMMENT '合思附件file_id',
  flow_id     VARCHAR(64)  NOT NULL COMMENT '单据ID',
  file_name   VARCHAR(500)          COMMENT '文件名',
  amount      DECIMAL(14,4)         COMMENT 'OCR识别的实付金额(保留符号)',
  status      VARCHAR(16)  NOT NULL COMMENT 'ok/fail/skip',
  raw_text    VARCHAR(500)          COMMENT 'OCR原始返回',
  updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (file_id),
  KEY idx_flow (flow_id)
) COMMENT '合思付款截图OCR结果缓存';
```

- [ ] **Step 1: Write failing test (sqlmock upsert + get)**

```go
func TestUpsertAndGetPaymentOcr(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectExec("INSERT INTO hesi_payment_ocr").
		WithArgs("fileA", "flow1", "付款截图1.jpg", 175.91, "ok", "175.91").
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := upsertPaymentOcr(db, "fileA", "flow1", "付款截图1.jpg", 175.91, "ok", "175.91"); err != nil {
		t.Fatal(err)
	}
	rows := sqlmock.NewRows([]string{"file_id", "amount", "status"}).
		AddRow("fileA", 175.91, "ok").AddRow("fileB", -118.0, "ok")
	mock.ExpectQuery("SELECT file_id, amount, status FROM hesi_payment_ocr WHERE flow_id=").
		WithArgs("flow1").WillReturnRows(rows)
	got, err := getPaymentOcrByFlow(db, "flow1")
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v err %v", got, err)
	}
}
```

- [ ] **Step 2: Run, verify FAIL** — `go test ./internal/handler/ -run TestUpsertAndGetPaymentOcr -v` → FAIL undefined.

- [ ] **Step 3: Implement** (ensurePaymentOcrTable + upsert `INSERT ... ON DUPLICATE KEY UPDATE` + get) — 见 Interfaces 表结构,upsert 用 `INSERT INTO hesi_payment_ocr (...) VALUES (...) ON DUPLICATE KEY UPDATE amount=VALUES(amount), status=VALUES(status), raw_text=VALUES(raw_text)`。

- [ ] **Step 4: Run, verify PASS**

- [ ] **Step 5: Commit**
```bash
git add server/internal/handler/hesi_payment_ocr.go server/internal/handler/hesi_payment_ocr_test.go
git commit -m "feat(hesi): 加付款截图OCR结果缓存表+存取层"
```

---

### Task 4: 对账规则(口径 B 核心逻辑)

**Files:**
- Modify: `server/internal/handler/hesi_payment_ocr.go`(加纯函数 + DB 组装)
- Test: `server/internal/handler/hesi_payment_ocr_test.go`

**Interfaces:**
- Produces:
  - `func reconcilePayment(payAmounts, invoiceTotals []float64, tolerance float64) (flag bool, payTotal, invTotal float64)` — 纯函数。payTotal=Σ|payAmounts|,invTotal=ΣinvoiceTotals,flag = payTotal > invTotal+tolerance。
  - `type PaymentCheck struct { Flag bool; PayTotal, InvTotal float64; Pending bool; Note string }`
  - `func (h *DashboardHandler) checkFlowPayment(flowID string) PaymentCheck` — 读 `getPaymentOcrByFlow` + 查 `hesi_flow_invoice.total_amount`,组装。付款截图存在但有 status!=ok 的(没 OCR 成功)→ Pending=true,Flag=false(不误判)。

- [ ] **Step 1: Write failing test for reconcilePayment**

```go
func TestReconcilePayment(t *testing.T) {
	// B26003890: 付款 175.91+118 = 293.91, 发票 293.91 → 不flag
	f, p, i := reconcilePayment([]float64{175.91, 118}, []float64{175.91, 118}, 0.01)
	if f || math.Abs(p-293.91) > 0.001 || math.Abs(i-293.91) > 0.001 {
		t.Errorf("890: flag=%v pay=%v inv=%v", f, p, i)
	}
	// B26003807 差旅: 付款 |−529|+|−564| = 1093, 发票 1241.20 → 付款<发票 → 不flag
	f2, _, _ := reconcilePayment([]float64{-529, -564}, []float64{529, 564, 35.72, 11.09, 23.34, 15.95, 26, 26.09, 10.01}, 0.01)
	if f2 {
		t.Error("差旅 付款<发票 不该flag")
	}
	// 付款 > 发票 → flag
	f3, _, _ := reconcilePayment([]float64{200}, []float64{100}, 0.01)
	if !f3 {
		t.Error("付款200>发票100 应flag")
	}
}
```

- [ ] **Step 2: Run, verify FAIL**

- [ ] **Step 3: Implement reconcilePayment + checkFlowPayment**

```go
import "math"

func reconcilePayment(payAmounts, invoiceTotals []float64, tolerance float64) (bool, float64, float64) {
	var pay, inv float64
	for _, a := range payAmounts {
		pay += math.Abs(a)
	}
	for _, t := range invoiceTotals {
		inv += t
	}
	return pay > inv+tolerance, pay, inv
}
```

`checkFlowPayment`:读 getPaymentOcrByFlow(只算 status=ok 的金额),若有付款截图记录但全部/部分 status!=ok → `Pending=true`;查 `SELECT total_amount FROM hesi_flow_invoice WHERE flow_id=?`;调 reconcilePayment(容差 0.01);组装 Note(flag 时:`付款截图实付¥X 超过发票总额¥Y,建议人工复核`)。

- [ ] **Step 4: Run, verify PASS**

- [ ] **Step 5: Commit**
```bash
git commit -am "feat(hesi): 付款vs发票对账规则(口径B 付款>发票才flag, 取绝对值)"
```

---

### Task 5: 后台 OCR 跑批工具

**Files:**
- Create: `server/cmd/ocr-hesi-payment/main.go`

**逻辑(无独立单测,真实环境验):**
1. config.Load → DB + 合思 appkey/secret + DashScopeKey。
2. ensurePaymentOcrTable。
3. 查待办:报销单(form_type='expense', state='approving')的付款截图附件(`hesi_flow_attachment` is_invoice=0 且 file_name 像图片:`.jpg/.jpeg/.png` 结尾,大小写不敏感)中,`file_id` 不在 `hesi_payment_ocr`(或 status='fail' 且超过 N 小时)的。按 flow 分组。
4. getAccessToken(合思)→ 分批 `POST /api/openapi/v1/flowDetails/attachment {flowIds}` 拿 attachmentUrls 的签名 `url`(有时效,现拿现用)。
5. 对每张:GET url 下图 → `ocr.RecognizePaymentAmount(ctx, key, img)` → `upsertPaymentOcr(status="ok", amount, raw)`;失败存 status="fail"。
6. 限速:每张之间 sleep ~300ms,避免 OCR QPS 超限;**OCR 失败不中断整批**,log + 继续。

**Interfaces consumed:** `ocr.RecognizePaymentAmount`(Task1)、`upsertPaymentOcr`/`ensurePaymentOcrTable`(Task3)、合思 getAccessToken/flowDetails/attachment(抄 `cmd/sync-hesi/main.go:41,154`)。

- [ ] **Step 1: 写 main.go**(抄 sync-hesi 的 getAccessToken + getAttachments;图片过滤;循环 OCR+存)
- [ ] **Step 2: Build**: `cd server && go build -o ocr-hesi-payment.exe ./cmd/ocr-hesi-payment`
- [ ] **Step 3: 真跑 B26003807/B26003890**: `./ocr-hesi-payment.exe` → 查库 `SELECT * FROM hesi_payment_ocr WHERE flow_id IN ('ID01TZuIJFTM1F','ID01U1POv19A0n')` 应得 175.91/118/-529/-564。
- [ ] **Step 4: Commit**
```bash
git add server/cmd/ocr-hesi-payment/
git commit -m "feat(hesi): 后台OCR跑批 — 下载付款截图识别金额存缓存"
```

---

### Task 6: 挂进报销单审批机器人 AuditDailyExpense

**Files:**
- Modify: `server/internal/handler/hesi_audit_rules.go`(AuditDailyExpense 加一条规则)
- Test: `server/internal/handler/hesi_audit_rules_test.go`(或同目录新测)

**Interfaces:**
- Consumes: `checkFlowPayment`(Task4)、现有 `AuditSuggestion` 结构 + AuditDailyExpense 签名。
- 行为:AuditDailyExpense 末尾调 `h.checkFlowPayment(flowID)`;若 `Flag` → 往建议里加一条"⚠️ 付款截图金额 ¥X 超过发票总额 ¥Y,建议人工复核"并把总体建议拉到"转人工";若 `Pending` → 加提示"付款截图待识别(稍后自动)",不改判定。

- [ ] **Step 1: 看 AuditSuggestion 结构 + AuditDailyExpense 尾部**
Run: `cd server && grep -n "type AuditSuggestion\|func.*AuditDailyExpense" internal/handler/hesi_audit_rules.go`

- [ ] **Step 2: 写 failing 测试**(构造一个 flag 场景:mock checkFlowPayment 的 DB 输入 → 断言 suggestion 含"超过发票总额"且判定=转人工)。按现有该文件测试风格写(sqlmock 或纯函数注入)。

- [ ] **Step 3: Run, verify FAIL**

- [ ] **Step 4: Implement** — 在 AuditDailyExpense 适当位置插入 checkFlowPayment 调用 + 建议拼接。注意:**不阻塞**——checkFlowPayment 只读 DB 缓存,不发起 OCR。

- [ ] **Step 5: Run, verify PASS** + 整包回归 `cd server && go test ./internal/handler/...`

- [ ] **Step 6: Commit**
```bash
git commit -am "feat(hesi): 报销单机器人加付款截图vs发票核对建议(口径B)"
```

---

### Task 7: 部署 + schtask + 上线

- [ ] **Step 1:** `cd server && go build -o bi-server.exe ./cmd/server`(含新规则)+ `go build -o ocr-hesi-payment.exe ./cmd/ocr-hesi-payment`
- [ ] **Step 2:** 注册 schtask `BI-OcrHesiPayment`(SYSTEM,每小时,在 BI-SyncHesi 之后约 15 分,.bat 包装,`/RL HIGHEST`,exe 带 `.\` 前缀,CRLF+ASCII)。
- [ ] **Step 3:** 拷 exe 到 server 根,错峰 kill 8080 PID 重启 bi-server(清代理 env + 绝对路径 + 留 bi-server.old.exe)。
- [ ] **Step 4:** 真单验收:跑哥用 樊雪娇/金海侠 账号看待审报销单,确认 B26003807 显示"付款<发票放行"、构造一个付款>发票的看是否提示人工复核;截图 + SQL 证据。
- [ ] **Step 5:** 发版(版本号跑哥定)+ notice(业务大白话,不写技术细节)。

---

## 风险 / 待跑哥确认

- **容差**默认 0.01(几分钱不算超)。若实际有 1-2 元手续费场景需放宽,改 reconcilePayment 入参(可做成 hesi_audit_param 配置)。
- **付款截图识别范围**:目前按 `is_invoice=0 且文件名以图片后缀结尾` 过滤,排除 PDF(通行费汇总单等)。若有人把付款截图存成 PDF,会漏(可后续加 PDF→图);先按图片做。
- **OCR 成本**:每张 ~3000 token ≈ 1 分钱;只 OCR 待审报销单的新截图,量可控。
- **多次重提**:报销单退回重提 file_id 不变 → 缓存复用,不重复 OCR。
