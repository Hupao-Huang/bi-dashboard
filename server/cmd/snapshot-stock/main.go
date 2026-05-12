package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// 库存每日快照（把 stock_quantity、stock_batch 当前状态复制到 _daily 快照表）
// 用于计划看板切换历史日期时查询当时的库存KPI
//
// 用法：
//   snapshot-stock.exe                              # 快照今天
//   SNAPSHOT_DATE=2026-04-21 snapshot-stock.exe     # 指定日期
//
// 定时任务建议：每天 23:00 运行（库存同步完成后）
func main() {
	unlock := importutil.AcquireLock("snapshot-stock")
	defer unlock()

	logFile, err := os.OpenFile(`C:\Users\Administrator\bi-dashboard\server\snapshot-stock.log`, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		// 同时写固定文件 + stdout, 让 bi-server 触发的 manual-*.log 也能捕获 (v1.56.1)
		log.SetOutput(io.MultiWriter(logFile, os.Stdout))
		defer logFile.Close()
	}
	log.Println("========== 开始库存快照 ==========")

	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()

	dateStr := os.Getenv("SNAPSHOT_DATE")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		log.Fatalf("日期格式错误(需yyyy-MM-dd): %v", err)
	}

	log.Printf("快照日期: %s", dateStr)

	// 1. 快照 stock_quantity
	startTime := time.Now()
	delRes, err := db.Exec(`DELETE FROM stock_quantity_daily WHERE snapshot_date = ?`, dateStr)
	if err != nil {
		log.Fatalf("删除旧库存快照失败: %v", err)
	}
	delCount, _ := delRes.RowsAffected()

	insRes, err := db.Exec(`
		INSERT INTO stock_quantity_daily
		(snapshot_date, quantity_id, goods_id, goods_no, goods_name, sku_id, sku_name, sku_barcode, unit_name,
		 warehouse_id, warehouse_name, warehouse_code,
		 current_qty, locked_qty, defective_qty, cost_price,
		 yesterday_qty, week_qty, month_qty,
		 allocate_quantity, ordering_quantity, purchasing_quantity, producting_quantity,
		 stock_in_quantity, stock_out_quantity, sales_return_quantity,
		 reserve_quantity, residual_quantity, use_quantity, outer_quantity, defective_use_quantity,
		 stock_index, owner_name, batch_no, production_date, expiration_date,
		 shelf_life, shelf_life_unit, goods_attr, is_batch_management,
		 last_purch_no_tax_price, last_purch_price, locking_quantity, owner_id, owner_type)
		SELECT ?, quantity_id, goods_id, goods_no, goods_name, sku_id, sku_name, sku_barcode, unit_name,
		 warehouse_id, warehouse_name, warehouse_code,
		 current_qty, locked_qty, defective_qty, cost_price,
		 yesterday_qty, week_qty, month_qty,
		 allocate_quantity, ordering_quantity, purchasing_quantity, producting_quantity,
		 stock_in_quantity, stock_out_quantity, sales_return_quantity,
		 reserve_quantity, residual_quantity, use_quantity, outer_quantity, defective_use_quantity,
		 stock_index, owner_name, batch_no, production_date, expiration_date,
		 shelf_life, shelf_life_unit, goods_attr, is_batch_management,
		 last_purch_no_tax_price, last_purch_price, locking_quantity, owner_id, owner_type
		FROM stock_quantity`, dateStr)
	if err != nil {
		log.Fatalf("插入库存快照失败: %v", err)
	}
	insCount, _ := insRes.RowsAffected()
	log.Printf("stock_quantity_daily: 删除 %d 条，新写入 %d 条，耗时 %v", delCount, insCount, time.Since(startTime))

	// 2. 快照 stock_batch
	startTime = time.Now()
	delRes2, err := db.Exec(`DELETE FROM stock_batch_daily WHERE snapshot_date = ?`, dateStr)
	if err != nil {
		log.Fatalf("删除旧批次快照失败: %v", err)
	}
	delCount2, _ := delRes2.RowsAffected()

	insRes2, err := db.Exec(`
		INSERT INTO stock_batch_daily
		(snapshot_date, quantity_id, goods_id, goods_no, goods_name, sku_id, sku_name, sku_barcode, unit_name,
		 warehouse_id, warehouse_name, warehouse_code,
		 batch_no, batch_number, production_date, expiration_date, shelf_life, shelf_life_unit,
		 current_qty, locked_qty, use_qty, defective_qty,
		 memo, out_sku_code, production_depart, registration_number)
		SELECT ?, quantity_id, goods_id, goods_no, goods_name, sku_id, sku_name, sku_barcode, unit_name,
		 warehouse_id, warehouse_name, warehouse_code,
		 batch_no, batch_number, production_date, expiration_date, shelf_life, shelf_life_unit,
		 current_qty, locked_qty, use_qty, defective_qty,
		 memo, out_sku_code, production_depart, registration_number
		FROM stock_batch`, dateStr)
	if err != nil {
		log.Fatalf("插入批次快照失败: %v", err)
	}
	insCount2, _ := insRes2.RowsAffected()
	log.Printf("stock_batch_daily: 删除 %d 条，新写入 %d 条，耗时 %v", delCount2, insCount2, time.Since(startTime))

	fmt.Printf("快照完成 [%s]: 库存 %d 条, 批次 %d 条\n", dateStr, insCount, insCount2)
	log.Println("========== 库存快照结束 ==========")
}
