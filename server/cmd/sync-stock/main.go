// sync-stock: 从吉客云同步库存数据到 stock_quantity 表
// 接口: erp.stockquantity.get (库存分页查询)
// 用法: sync-stock.exe [configPath]
package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"bi-dashboard/internal/jackyun"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	unlock := importutil.AcquireLock("sync-stock")
	defer unlock()

	log.SetFlags(log.LstdFlags)
	log.Println("=== 库存同步开始 ===")

	// 加载配置
	cfgPath := "config.json"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 连接数据库
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)

	// 创建吉客云客户端（用旧AppKey，标准接口）
	client := jackyun.NewClient(cfg.JackYun.AppKey, cfg.JackYun.Secret, cfg.JackYun.APIURL)

	// 游标方式拉取全部库存（避免超1万条限制）
	pageSize := 50
	totalInserted := 0
	startTime := time.Now()
	maxQuantityId := "0"

	for round := 1; ; round++ {
		biz := map[string]interface{}{
			"pageIndex":            0,
			"pageSize":             pageSize,
			"isBlockup":            2,
			"isNotQueryBatchStock": "1",
			"maxQuantityId":        maxQuantityId,
		}

		log.Printf("拉取第 %d 批 (maxQuantityId=%s)...", round, maxQuantityId)
		resp, err := client.Call("erp.stockquantity.get", biz)
		if err != nil {
			log.Printf("API调用失败: %v", err)
			break
		}
		if resp.Code != 200 {
			log.Printf("API返回错误: code=%d msg=%s subCode=%s", resp.Code, resp.Msg, resp.SubCode)
			break
		}

		var wrapper struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(resp.Result, &wrapper); err != nil {
			log.Printf("解析result失败: %v", err)
			break
		}

		var dataBytes []byte
		var dataStr string
		if err := json.Unmarshal(wrapper.Data, &dataStr); err == nil {
			dataBytes = []byte(dataStr)
		} else {
			dataBytes = wrapper.Data
		}

		var result struct {
			GoodsStockQuantity []map[string]interface{} `json:"goodsStockQuantity"`
		}
		if err := json.Unmarshal(dataBytes, &result); err != nil {
			log.Printf("解析data失败: %v", err)
			break
		}

		items := result.GoodsStockQuantity
		if len(items) == 0 {
			log.Printf("无更多数据，同步完成")
			break
		}

		// 写入数据库，并记录最后一条的quantityId作为下次游标
		inserted := 0
		lastQid := ""
		for _, item := range items {
			err := upsertStock(db, item)
			if err != nil {
				log.Printf("写入失败: goods_no=%s sku_id=%s err=%v", gs(item, "goodsNo"), gs(item, "skuId"), err)
				continue
			}
			inserted++
			if qid := gs(item, "quantityId"); qid != "" && qid != "0" {
				lastQid = qid
			}
		}
		totalInserted += inserted
		log.Printf("  第 %d 批: %d/%d 条写入成功", round, inserted, len(items))

		// 不足一页说明已到最后
		if len(items) < pageSize {
			break
		}

		// 更新游标
		if lastQid == "" {
			break
		}
		maxQuantityId = lastQid

		// 限速，避免频率过高
		time.Sleep(200 * time.Millisecond)
	}

	elapsed := time.Since(startTime)
	log.Printf("=== 库存同步完成: 共 %d 条, 耗时 %s ===", totalInserted, elapsed.Round(time.Second))

	// 存历史明细快照
	// 已停: stock_snapshot_YYYYMM 没人查, 占空间. BI 看板用的是 stock_quantity_daily (snapshot-stock.exe 每天 23:30 写)
	// 未来要恢复: 取消下面这行注释 + 重 build sync-stock.exe
	// saveDetailSnapshot(db)

	// 存当日汇总快照
	saveSnapshot(db)
}

func saveDetailSnapshot(db *sql.DB) {
	now := time.Now()
	snapTime := now.Format("2006-01-02 15:04:05")
	tableMonth := now.Format("200601")
	tableName := "stock_snapshot_" + tableMonth

	// 确保月表存在
	var count int
	db.QueryRow("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", tableName).Scan(&count)
	if count == 0 {
		db.Exec(fmt.Sprintf("CREATE TABLE %s LIKE stock_snapshot_template", tableName))
		log.Printf("自动创建快照表: %s", tableName)
	}

	result, err := db.Exec(fmt.Sprintf(`INSERT INTO %s
		(snap_time, goods_id, goods_no, goods_name, sku_id, sku_name,
		 warehouse_id, warehouse_name, current_qty, locked_qty, locking_quantity,
		 defective_qty, cost_price, month_qty, goods_attr)
		SELECT ?, goods_id, goods_no, goods_name, sku_id, sku_name,
		 warehouse_id, warehouse_name, current_qty, locked_qty, locking_quantity,
		 defective_qty, cost_price, month_qty, goods_attr
		FROM stock_quantity
		WHERE goods_attr = 1 AND warehouse_name != ''`, tableName), snapTime)
	if err != nil {
		log.Printf("保存明细快照失败: %v", err)
		return
	}
	rows, _ := result.RowsAffected()
	log.Printf("保存明细快照成功: %s %d 条", tableName, rows)
}

func saveSnapshot(db *sql.DB) {
	log.Println("保存当日库存快照...")
	_, err := db.Exec(`INSERT INTO stock_daily_snapshot
		(snap_date, total_stock_value, turnover_days, high_stock_rate,
		 stockout_rate, stockout_sku, sales_sku, aged_stock_value)
		SELECT
			CURDATE(),
			IFNULL(SUM(current_qty * cost_price), 0),
			CASE WHEN SUM(month_qty * cost_price / 30) > 0
				THEN ROUND(SUM(current_qty * cost_price) / SUM(month_qty * cost_price / 30), 1)
				ELSE 0 END,
			CASE WHEN SUM(current_qty * cost_price) > 0
				THEN ROUND(SUM(CASE WHEN month_qty > 0 AND (current_qty-locked_qty)/(month_qty/30) > 50
					THEN current_qty * cost_price ELSE 0 END) / SUM(current_qty * cost_price) * 100, 1)
				ELSE 0 END,
			CASE WHEN SUM(CASE WHEN month_qty > 0 THEN 1 ELSE 0 END) > 0
				THEN ROUND(SUM(CASE WHEN current_qty-locked_qty <= 0 AND month_qty > 0 THEN 1 ELSE 0 END)
					/ SUM(CASE WHEN month_qty > 0 THEN 1 ELSE 0 END) * 100, 1)
				ELSE 0 END,
			IFNULL(SUM(CASE WHEN current_qty-locked_qty <= 0 AND month_qty > 0 THEN 1 ELSE 0 END), 0),
			IFNULL(SUM(CASE WHEN month_qty > 0 THEN 1 ELSE 0 END), 0),
			IFNULL((SELECT SUM(b.current_qty * IFNULL(s2.cost_price,0))
				FROM stock_batch b
				LEFT JOIN stock_quantity s2 ON b.sku_id = s2.sku_id AND b.warehouse_id = s2.warehouse_id
				WHERE b.production_date IS NOT NULL AND b.current_qty > 0
				AND DATEDIFF(CURDATE(), b.production_date) > 90), 0)
		FROM stock_quantity
		WHERE goods_attr = 1 AND warehouse_name != ''
		ON DUPLICATE KEY UPDATE
			total_stock_value = VALUES(total_stock_value),
			turnover_days = VALUES(turnover_days),
			high_stock_rate = VALUES(high_stock_rate),
			stockout_rate = VALUES(stockout_rate),
			stockout_sku = VALUES(stockout_sku),
			sales_sku = VALUES(sales_sku),
			aged_stock_value = VALUES(aged_stock_value)`)
	if err != nil {
		log.Printf("保存快照失败: %v", err)
	} else {
		log.Println("快照保存成功")
	}
}

func upsertStock(db *sql.DB, m map[string]interface{}) error {
	_, err := db.Exec(`INSERT INTO stock_quantity
		(goods_id, goods_no, goods_name, sku_id, sku_name, sku_barcode, unit_name,
		 warehouse_id, warehouse_name, warehouse_code,
		 current_qty, locked_qty, defective_qty, cost_price,
		 yesterday_qty, week_qty, month_qty, goods_attr,
		 quantity_id, allocate_quantity, ordering_quantity, purchasing_quantity,
		 producting_quantity, stock_in_quantity, stock_out_quantity, sales_return_quantity,
		 reserve_quantity, residual_quantity, use_quantity, outer_quantity,
		 defective_use_quantity, stock_index, owner_name, batch_no,
		 production_date, expiration_date, shelf_life, shelf_life_unit,
		 is_batch_management, last_purch_no_tax_price, last_purch_price,
		 locking_quantity, owner_id, owner_type,
		 synced_at)
		VALUES (?,?,?,?,?,?,?, ?,?,?, ?,?,?,?, ?,?,?,?,
		        ?,?,?,?, ?,?,?,?, ?,?,?,?, ?,?,?,?, ?,?,?,?, ?,?,?,?,?,?, NOW())
		ON DUPLICATE KEY UPDATE
			goods_no=VALUES(goods_no), goods_name=VALUES(goods_name),
			sku_name=VALUES(sku_name), sku_barcode=VALUES(sku_barcode), unit_name=VALUES(unit_name),
			warehouse_name=VALUES(warehouse_name), warehouse_code=VALUES(warehouse_code),
			current_qty=VALUES(current_qty), locked_qty=VALUES(locked_qty),
			defective_qty=VALUES(defective_qty), cost_price=VALUES(cost_price),
			yesterday_qty=VALUES(yesterday_qty), week_qty=VALUES(week_qty),
			month_qty=VALUES(month_qty), goods_attr=VALUES(goods_attr),
			quantity_id=VALUES(quantity_id),
			allocate_quantity=VALUES(allocate_quantity), ordering_quantity=VALUES(ordering_quantity),
			purchasing_quantity=VALUES(purchasing_quantity), producting_quantity=VALUES(producting_quantity),
			stock_in_quantity=VALUES(stock_in_quantity), stock_out_quantity=VALUES(stock_out_quantity),
			sales_return_quantity=VALUES(sales_return_quantity), reserve_quantity=VALUES(reserve_quantity),
			residual_quantity=VALUES(residual_quantity), use_quantity=VALUES(use_quantity),
			outer_quantity=VALUES(outer_quantity), defective_use_quantity=VALUES(defective_use_quantity),
			stock_index=VALUES(stock_index), owner_name=VALUES(owner_name),
			batch_no=VALUES(batch_no), production_date=VALUES(production_date),
			expiration_date=VALUES(expiration_date), shelf_life=VALUES(shelf_life),
			shelf_life_unit=VALUES(shelf_life_unit),
			is_batch_management=VALUES(is_batch_management),
			last_purch_no_tax_price=VALUES(last_purch_no_tax_price),
			last_purch_price=VALUES(last_purch_price),
			locking_quantity=VALUES(locking_quantity),
			owner_id=VALUES(owner_id), owner_type=VALUES(owner_type),
			synced_at=NOW()`,
		gf(m, "goodsId"), gs(m, "goodsNo"), gs(m, "goodsName"),
		gf(m, "skuId"), gs(m, "skuName"), gs(m, "skuBarcode"), gs(m, "unitName"),
		gf(m, "warehouseId"), gs(m, "warehouseName"), gs(m, "warehouseCode"),
		gd(m, "currentQuantity"), gd(m, "lockedQuantity"),
		gd(m, "defectiveQuanity"), gd(m, "costPrice"),
		gd(m, "yesterdayQuantity"), gd(m, "weekQuantity"),
		gd(m, "threedayQuantity"), gi(m, "goodsAttr"),
		gs(m, "quantityId"),
		gd(m, "allocateQuantity"), gd(m, "orderingQuantity"),
		gd(m, "purchasingQuantity"), gd(m, "productingQuantity"),
		gd(m, "stockInQuantity"), gd(m, "stockOutuantity"),
		gd(m, "salesReturnQuantity"), gd(m, "reserveQuantity"),
		gd(m, "residualQuantity"), gd(m, "useQuantity"),
		gd(m, "outerQuantity"), gd(m, "defectiveUseQuantity"),
		gi(m, "stockIndex"), gs(m, "ownerName"),
		gs(m, "batchNo"), gns(m, "productionDate"),
		gns(m, "expirationDate"), gi(m, "shelfLife"),
		gs(m, "shelfLiftUnit"),
		gi(m, "isbatchmanagement"), gd(m, "lastPurchNoTaxPrice"),
		gd(m, "lastPurchPrice"), gd(m, "lockingQuantity"),
		gs(m, "ownerId"), gi(m, "ownerType"),
	)
	return err
}

// gs 获取字符串值
func gs(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// gf 获取数值（用于ID字段）
func gf(m map[string]interface{}, key string) float64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	case json.Number:
		f, _ := val.Float64()
		return f
	}
	return 0
}

// gd 获取 decimal 值
func gd(m map[string]interface{}, key string) float64 {
	return gf(m, key)
}

// gi 获取整数值
func gi(m map[string]interface{}, key string) int {
	return int(gf(m, key))
}

// gns 获取可空字符串值（用于日期等可为NULL的字段）
func gns(m map[string]interface{}, key string) interface{} {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	s := fmt.Sprintf("%v", v)
	if s == "" || s == "0001-01-01" {
		return nil
	}
	return s
}
