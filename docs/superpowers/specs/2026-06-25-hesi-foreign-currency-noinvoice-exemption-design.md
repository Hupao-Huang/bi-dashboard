# 合思报销 规则10 加"外币(国外)无票"豁免 — 设计

日期: 2026-06-25　作者: 跑哥 + Claude　状态: 待跑哥审

## 背景 / 问题

合思报销机器人 **规则 10(无票判定)**: 费用明细无发票时, 除非属于 4 种豁免(研发样品 / 出差补贴 / 私车公用 / 截图说明)否则**驳回**。

国外报销(如 claude 接口订阅、Google Play 充值)**本就没有中国发票**, 凭证是境外付款截图。这类单子现被规则10误驳。

实例 **B26003947**(claude接口订阅费用, ¥884.65 = 130.00 美元): 付款截图 "Pockyt Shop -884.65 / 超优汇率 1美元=6.805人民币", 当前 AI 建议=**驳回**。

**关键约束(查实)**:
- 合思 **没有结构化币种字段** —— B26003947 raw_json 2496 字节, 无 美元/USD/汇率/currency/币种 任何标记。外币信息**只在付款截图图片里**, 必须靠 OCR。
- 该单付款截图文件名 `9ab065c3e9f60c15a37b6b7a59a5d312.jpg` 是**哈希名, 不含"付款/支付/账单"关键词** → 现有付款截图 OCR 流程**根本不会选中它**(`hesi_payment_ocr` 里这单为空)。所以外币检测**不能挂在付款关键词过滤上**。

## 目标

付款截图里识别到**外币/汇率** → 判定"国外无票" → **规则10 不驳 + 规则19 跳过**, 其它规则照常 → 都过则**自动通过**(跑哥 2026-06-25 拍板口径)。

## 方案

### ① 外币检测(OCR 转写 + Go 关键词)
- OCR **转写付款截图全文**(qwen-vl-ocr, 强项是把图上字读出来), 不用"有没有汇率回 yes/no"(OCR 模型推理不如转写稳)。
- Go 纯函数 `hasForeignCurrencyMarker(text)` 判定(确定 + 可单测): 命中以下任一即 true:
  - `汇率`
  - 货币名: `美元` `USD` `港币` `HKD` `欧元` `EUR` `日元` `JPY` `英镑` `GBP`
  - 符号: `$` `€` `£`
- (跑哥确认关键词集足够; 暂无"有汇率却是国内"的反例。)

### ② 检测范围(扫哪些图)
- **无票报销单**(form_type=expense, state∈approving/pending, **该单无 hesi_flow_invoice 行**)上的**所有非发票图片**(is_invoice=0, jpg/jpeg/png, 含哈希命名)。
- 规模实测: **33 单 / 51 张图**, 一遍 OCR 很便宜。
- 故意**不挂付款关键词** —— 否则哈希名截图(如 B26003947)看不到。

### ③ 存储(不新建表, 跑哥定)
- `hesi_payment_ocr` 加列 **`is_foreign TINYINT NULL`**: `NULL`=未扫 / `0`=扫了无外币 / `1`=扫了有外币。
- 外币扫描对哈希名图(付款金额 OCR 没处理过的)**新插行**: `status='fx'`, amount=0, is_foreign=0/1, raw_text=转写文本(截断)。对已有行(付款关键词图)只 **UPDATE is_foreign**。
- **隔离规则19**: `getPaymentOcrByFlow`(规则19 读金额用)加 `AND status<>'fx'`, 让 fx 扫描行**完全不进规则19**, 不放大上次二审那批金额相关老问题。

### ④ 接进审批(hesi_audit_rules.go)
- 新 helper `isForeignFlow(flowID) bool`: `EXISTS(SELECT 1 FROM hesi_payment_ocr WHERE flow_id=? AND is_foreign=1)`。
- **规则10**(`ruleInvoiceChecks` 加 `isForeign bool` 入参): 无票明细判定时, `isForeign=true` → 算豁免 E(外币国外无票), 不驳。
- **规则19**(AuditDailyExpense line 525): `isForeignFlow` → 跳过付款vs发票核对(国外无发票可比, 否则会卡人工)。
- 其它规则不变。都过 → 自动通过。

### ⑤ 跑批 + 定时
- 外币扫描作为 `cmd/ocr-hesi-payment` 的**新增一段**(金额扫描之后), 复用现有 合思token / 拉附件URL / 下图 / ShrinkJPEG。
- 不新建 schtask, 跟现有 **BI-OcrHesiPayment**(每30分钟)一起跑。增量: 只扫 is_foreign IS NULL(未扫过)的图。

## 测试

- **`hasForeignCurrencyMarker` 纯函数 TDD**: 命中(超优汇率 1美元=6.805 / Pockyt $130 / 港币…)+ 不命中(纯人民币付款截图 / 普通报销)。
- **`ruleInvoiceChecks` isForeign 参数单测**: 无票 + isForeign=true → 不驳; isForeign=false → 维持原驳回。
- **`ocr.TranscribeText`**: 解析/错误传播单测(仿 RecognizePaymentAmount)。
- **端到端**: 跑批扫 B26003947 → is_foreign=1; 审批建议从"驳回"变"通过"(SQL + 实跑核实)。

## 上线

财务红线: 改完 **/code-review 二审** → 跑哥确认 → build `cmd/ocr-hesi-payment` + `bi-server`(规则改动) → 错峰重启 → 实测。**未经跑哥明说不部署。**

## 非目标(YAGNI)

- 不做币种汇率换算 / 不校验外币金额(只判"有没有外币" → 豁免无票)。
- 不碰规则19 的金额比对逻辑(只加"外币则跳过"开关)。
- 上次二审剩 9 条 findings 不在本次范围(择期另办)。
