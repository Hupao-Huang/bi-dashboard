package salesdaily

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

func EnsureShopOrderSummaryTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS sales_daily_shop_order_summary (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		stat_date DATE NOT NULL COMMENT '发货日期',
		shop_name VARCHAR(255) NOT NULL COMMENT '店铺名称',
		orders INT NOT NULL DEFAULT 0 COMMENT '发货单数',
		weight_kg DECIMAL(18,4) NOT NULL DEFAULT 0 COMMENT '重量kg',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
		UNIQUE KEY uk_date_shop (stat_date, shop_name),
		KEY idx_date (stat_date)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='销售日报店铺订单日汇总'`)
	return err
}

func RebuildReportSummaryRange(ctx context.Context, db *sql.DB, startDate, endDate time.Time) error {
	if err := EnsureShopOrderSummaryTable(db); err != nil {
		return err
	}
	if err := EnsureComboSummaryTable(db); err != nil {
		return err
	}
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		if err := RebuildShopOrderSummaryDay(ctx, db, d); err != nil {
			return err
		}
		if err := RebuildComboSummaryDay(ctx, db, d); err != nil {
			return err
		}
	}
	return nil
}

func RebuildShopOrderSummaryRange(ctx context.Context, db *sql.DB, startDate, endDate time.Time) error {
	if err := EnsureShopOrderSummaryTable(db); err != nil {
		return err
	}
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		if err := RebuildShopOrderSummaryDay(ctx, db, d); err != nil {
			return err
		}
	}
	return nil
}

func RebuildShopOrderSummaryDay(ctx context.Context, db *sql.DB, d time.Time) error {
	dayStr := d.Format("2006-01-02")
	part := "trade_" + d.Format("200601")
	nextDay := d.AddDate(0, 0, 1).Format("2006-01-02")
	whPH, whArgs := warehouseArgs()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s begin shop order summary: %w", dayStr, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM sales_daily_shop_order_summary WHERE stat_date=?`, dayStr); err != nil {
		return fmt.Errorf("%s delete old shop order summary: %w", dayStr, err)
	}
	insertSQL := fmt.Sprintf(`INSERT INTO sales_daily_shop_order_summary
		(stat_date, shop_name, orders, weight_kg)
		SELECT ?, COALESCE(t.shop_name,''), COUNT(DISTINCT t.trade_id), IFNULL(SUM(t.estimate_weight),0)/1000
		FROM %s t
		WHERE t.trade_type NOT IN (8,12) AND t.consign_time>=? AND t.consign_time<? AND t.warehouse_name IN (%s)
		GROUP BY COALESCE(t.shop_name,'')`, part, whPH)
	args := append([]interface{}{dayStr, dayStr, nextDay}, whArgs...)
	res, err := tx.ExecContext(ctx, insertSQL, args...)
	if err != nil {
		return fmt.Errorf("%s insert shop order summary: %w", dayStr, err)
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s commit shop order summary: %w", dayStr, err)
	}
	log.Printf("[sales-daily-shop-order-summary] %s 写入 %d 个店铺", dayStr, n)
	return nil
}
