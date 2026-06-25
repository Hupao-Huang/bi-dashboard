package handler

// hesi_payment_ocr.go — 付款截图OCR结果缓存表存取层
// 表 hesi_payment_ocr：以 file_id 为主键，缓存每张截图的OCR识别结果。
// 懒建表模式（仿 ensureCfPresetTable）：首次调用任一接口时建表。
// 上层任务(Task 5/6)调用 ensurePaymentOcrTable 时机自行决定，此处只定义。

import (
	"database/sql"
	"fmt"
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
  updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
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
