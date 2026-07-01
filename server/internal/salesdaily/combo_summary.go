package salesdaily

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

var reportWarehouses = []string{
	"南京委外成品仓-公司仓-委外",
	"天津委外仓-公司仓-外仓",
	"松鲜鲜&大地密码云仓",
	"长沙委外成品仓-公司仓-外仓",
}

func EnsureComboSummaryTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS sales_daily_combo_summary (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		stat_date DATE NOT NULL COMMENT '发货日期',
		combo_hash VARCHAR(128) NOT NULL COMMENT '货品组合签名前缀',
		combo_sig MEDIUMTEXT NOT NULL COMMENT '货品组合签名',
		combo_display MEDIUMTEXT NOT NULL COMMENT '货品组合展示',
		orders INT NOT NULL DEFAULT 0 COMMENT '发货单数',
		bottles DECIMAL(18,4) NOT NULL DEFAULT 0 COMMENT '发货件数',
		weight_kg DECIMAL(18,4) NOT NULL DEFAULT 0 COMMENT '重量kg',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
		KEY idx_date_combo (stat_date, combo_hash),
		KEY idx_date_orders (stat_date, orders),
		KEY idx_date_bottles (stat_date, bottles)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='销售日报货品组合日汇总'`)
	if err != nil {
		return err
	}
	if err := dropIndexIfExists(db, "sales_daily_combo_summary", "uk_date_combo"); err != nil {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE sales_daily_combo_summary MODIFY combo_hash VARCHAR(128) NOT NULL COMMENT '货品组合签名前缀'`); err != nil {
		return err
	}
	if err := addIndexIfMissing(db, "sales_daily_combo_summary", "idx_date_combo",
		`ALTER TABLE sales_daily_combo_summary ADD KEY idx_date_combo (stat_date, combo_hash)`); err != nil {
		return err
	}
	return nil
}

func RebuildComboSummaryRange(ctx context.Context, db *sql.DB, startDate, endDate time.Time) error {
	if err := EnsureComboSummaryTable(db); err != nil {
		return err
	}
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		if err := RebuildComboSummaryDay(ctx, db, d); err != nil {
			return err
		}
	}
	return nil
}

func RebuildComboSummaryDay(ctx context.Context, db *sql.DB, d time.Time) error {
	dayStr := d.Format("2006-01-02")
	part := "trade_" + d.Format("200601")
	partG := "trade_goods_" + d.Format("200601")
	nextDay := d.AddDate(0, 0, 1).Format("2006-01-02")
	whPH, whArgs := warehouseArgs()

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("%s get conn: %w", dayStr, err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SET SESSION group_concat_max_len = 100000"); err != nil {
		return fmt.Errorf("%s set group_concat_max_len: %w", dayStr, err)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s begin: %w", dayStr, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM sales_daily_combo_summary WHERE stat_date=?`, dayStr); err != nil {
		return fmt.Errorf("%s delete old combo summary: %w", dayStr, err)
	}

	insertSQL := fmt.Sprintf(`INSERT INTO sales_daily_combo_summary
		(stat_date, combo_hash, combo_sig, combo_display, orders, bottles, weight_kg)
		SELECT ?, LEFT(sig,128), sig, MAX(sig_display), COUNT(*), IFNULL(SUM(ob),0), IFNULL(SUM(ow),0)/1000
		FROM (
		  SELECT t.trade_id, MAX(t.estimate_weight) AS ow,
		    GROUP_CONCAT(tg.goods_no,'#',CAST(tg.sell_count AS SIGNED) ORDER BY tg.goods_no SEPARATOR '|') AS sig,
		    GROUP_CONCAT(tg.goods_name,'(',CAST(tg.sell_count AS SIGNED),')' ORDER BY tg.goods_no SEPARATOR ', ') AS sig_display,
		    SUM(tg.sell_count*COALESCE(p.box_qty,1)) AS ob
		  FROM %s t JOIN %s tg ON tg.trade_id=t.trade_id
		  LEFT JOIN dim_goods_pack_spec p ON p.goods_no=tg.goods_no
		  WHERE t.trade_type NOT IN (8,12) AND t.consign_time>=? AND t.consign_time<? AND t.warehouse_name IN (%s)
		  GROUP BY t.trade_id
		) o GROUP BY sig`, part, partG, whPH)
	args := append([]interface{}{dayStr, dayStr, nextDay}, whArgs...)
	res, err := tx.ExecContext(ctx, insertSQL, args...)
	if err != nil {
		return fmt.Errorf("%s insert combo summary: %w", dayStr, err)
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s commit combo summary: %w", dayStr, err)
	}
	log.Printf("[sales-daily-combo-summary] %s 写入 %d 个组合", dayStr, n)
	return nil
}

func dropIndexIfExists(db *sql.DB, table, index string) error {
	if !indexExists(db, table, index) {
		return nil
	}
	_, err := db.Exec(`ALTER TABLE ` + table + ` DROP INDEX ` + index)
	return err
}

func addIndexIfMissing(db *sql.DB, table, index, ddl string) error {
	if indexExists(db, table, index) {
		return nil
	}
	_, err := db.Exec(ddl)
	return err
}

func indexExists(db *sql.DB, table, index string) bool {
	var n int
	err := db.QueryRow(`SELECT COUNT(*)
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND INDEX_NAME=?`, table, index).Scan(&n)
	return err == nil && n > 0
}

func warehouseArgs() (string, []interface{}) {
	ph := strings.TrimRight(strings.Repeat("?,", len(reportWarehouses)), ",")
	args := make([]interface{}, len(reportWarehouses))
	for i, w := range reportWarehouses {
		args[i] = w
	}
	return ph, args
}
