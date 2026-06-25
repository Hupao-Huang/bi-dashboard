package handler

// hesi_payment_ocr.go — 付款截图OCR结果缓存表存取层
// 表 hesi_payment_ocr：以 file_id 为主键，缓存每张截图的OCR识别结果。
// 懒建表模式（仿 ensureCfPresetTable）：首次调用任一接口时建表。
// 上层任务(Task 5/6)调用 ensurePaymentOcrTable 时机自行决定，此处只定义。

import (
	"database/sql"
	"fmt"
	"math"
)

// ensurePaymentOcrTable 懒建表：建 hesi_payment_ocr（幂等，CREATE TABLE IF NOT EXISTS）。
// 调用方(Task 5/6)负责在适当时机调用一次。
func ensurePaymentOcrTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS hesi_payment_ocr (
  file_id    VARCHAR(128) NOT NULL COMMENT '合思附件file_id',
  flow_id    VARCHAR(64)  NOT NULL COMMENT '单据ID',
  file_name  VARCHAR(500)          COMMENT '文件名',
  amount     DECIMAL(14,4)         COMMENT 'OCR识别的实付金额(保留符号)',
  status     VARCHAR(16)  NOT NULL COMMENT 'ok/fail/skip',
  raw_text   VARCHAR(500)          COMMENT 'OCR原始返回',
  updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (file_id),
  KEY idx_flow (flow_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='合思付款截图OCR结果缓存'`)
	if err != nil {
		return fmt.Errorf("ensurePaymentOcrTable: %w", err)
	}
	return nil
}

// paymentOcrRow 是从 hesi_payment_ocr 查出的一行摘要（供上层对账用）。
// amount 保留符号（支出为负），调用方对账时自行 abs()。
type paymentOcrRow struct {
	FileID string
	Amount float64
	Status string
}

// upsertPaymentOcr 插入或覆盖一条OCR结果。
// 主键 file_id 冲突时覆盖 amount/status/raw_text/file_name（重新OCR场景）。
func upsertPaymentOcr(db *sql.DB, fileID, flowID, fileName string, amount float64, status, raw string) error {
	_, err := db.Exec(
		`INSERT INTO hesi_payment_ocr (file_id, flow_id, file_name, amount, status, raw_text)`+
			` VALUES (?, ?, ?, ?, ?, ?)`+
			` ON DUPLICATE KEY UPDATE amount=VALUES(amount), status=VALUES(status), raw_text=VALUES(raw_text), file_name=VALUES(file_name)`,
		fileID, flowID, fileName, amount, status, raw,
	)
	if err != nil {
		return fmt.Errorf("upsertPaymentOcr: %w", err)
	}
	return nil
}

// reconcilePayment 对账纯函数（口径B）：
// payTotal = Σ|payAmounts|（取绝对值，兼容负号支出）
// invTotal = ΣinvoiceTotals
// flag = payTotal > invTotal + tolerance（付款超发票才需人工复核）
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

// PaymentCheck 汇总付款截图与发票的对账结果，供审批建议(Task 6)消费。
type PaymentCheck struct {
	Flag     bool    // true = 付款>发票, 建议人工复核
	PayTotal float64 // 付款截图实付总额(绝对值)
	InvTotal float64 // 发票价税合计总额
	Pending  bool    // 有付款截图还没OCR成功, 暂不判定
	Note     string  // 给审批建议看的说明
}

// paymentOverToleranceYuan 付款比发票多出的容差(元)：多出 ≤2 元(手续费/凑整)算正常、自动通过；
// 超过 2 元才转人工复核。付款少于发票永远自动通过(差旅通行费等有票无截图不受影响)。(跑哥 2026-06-25 口径)
const paymentOverToleranceYuan = 2.0

// checkFlowPayment 查询某单的付款截图OCR结果和发票金额，输出对账结论。
// 仅统计 status=ok 的截图金额；有 status!=ok 的截图时 Pending=true，Flag 强制 false。
func (h *DashboardHandler) checkFlowPayment(flowID string) PaymentCheck {
	// 1. 读付款截图OCR结果
	ocrRows, err := getPaymentOcrByFlow(h.DB, flowID)
	if err != nil {
		return PaymentCheck{Note: "查询付款OCR缓存失败"}
	}

	// 2. 分类：ok 的金额 + 是否有 pending
	var okAmounts []float64
	pending := false
	for _, r := range ocrRows {
		if r.Status == "ok" {
			okAmounts = append(okAmounts, r.Amount)
		} else {
			pending = true
		}
	}

	// 3. 查发票金额 (复用 sumInvoiceTotal，IFNULL 处理 NULL total_amount)
	invSum := h.sumInvoiceTotal(flowID)

	// 4. 对账
	flag, payTotal, invTotal := reconcilePayment(okAmounts, []float64{invSum}, paymentOverToleranceYuan)

	// 5. 有未完成OCR时不下判定
	if pending {
		flag = false
	}

	// 6. 组装说明
	var note string
	if flag {
		note = fmt.Sprintf("付款截图实付 ¥%.2f 比发票总额 ¥%.2f 多出超过2元, 建议人工复核", payTotal, invTotal)
	} else if pending {
		note = "部分付款截图待识别"
	}

	return PaymentCheck{Flag: flag, PayTotal: payTotal, InvTotal: invTotal, Pending: pending, Note: note}
}

// EnsurePaymentOcrTable 导出包装: 供 cmd/ocr-hesi-payment 等外部包调用。
func EnsurePaymentOcrTable(db *sql.DB) error {
	return ensurePaymentOcrTable(db)
}

// UpsertPaymentOcr 导出包装: 供 cmd/ocr-hesi-payment 等外部包调用。
func UpsertPaymentOcr(db *sql.DB, fileID, flowID, fileName string, amount float64, status, raw string) error {
	return upsertPaymentOcr(db, fileID, flowID, fileName, amount, status, raw)
}

// getPaymentOcrByFlow 按单据ID查该单所有截图的OCR摘要。
func getPaymentOcrByFlow(db *sql.DB, flowID string) ([]paymentOcrRow, error) {
	rows, err := db.Query(
		`SELECT file_id, amount, status FROM hesi_payment_ocr WHERE flow_id=?`,
		flowID,
	)
	if err != nil {
		return nil, fmt.Errorf("getPaymentOcrByFlow: %w", err)
	}
	defer rows.Close()
	var result []paymentOcrRow
	for rows.Next() {
		var r paymentOcrRow
		if err := rows.Scan(&r.FileID, &r.Amount, &r.Status); err != nil {
			return nil, fmt.Errorf("getPaymentOcrByFlow scan: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("getPaymentOcrByFlow rows: %w", err)
	}
	return result, nil
}
