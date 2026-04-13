// sync-batch-stock: 从吉客云同步批次库存数据
// 接口: erp.batchstockquantity.get (warehouseCode必填，需遍历仓库)
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
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	unlock := importutil.AcquireLock("sync-batch-stock")
	defer unlock()

	log.SetFlags(log.LstdFlags)
	log.Println("=== 批次库存同步开始 ===")

	cfgPath := "config.json"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)

	client := jackyun.NewClient(cfg.JackYun.AppKey, cfg.JackYun.Secret, cfg.JackYun.APIURL)

	// 查出所有有库存的仓库编码
	rows, err := db.Query(`SELECT DISTINCT warehouse_code, warehouse_name FROM stock_quantity WHERE goods_attr=1 AND current_qty > 0 AND warehouse_code != '' ORDER BY warehouse_code`)
	if err != nil {
		log.Fatalf("查询仓库失败: %v", err)
	}
	type Warehouse struct {
		Code string
		Name string
	}
	warehouses := []Warehouse{}
	for rows.Next() {
		var wh Warehouse
		rows.Scan(&wh.Code, &wh.Name)
		warehouses = append(warehouses, wh)
	}
	rows.Close()

	log.Printf("共 %d 个仓库需要同步", len(warehouses))

	totalInserted := 0
	startTime := time.Now()

	for _, wh := range warehouses {
		log.Printf("--- 仓库: %s (%s) ---", wh.Name, wh.Code)
		whInserted := 0

		for page := 0; ; page++ {
			biz := map[string]interface{}{
				"warehouseCode": wh.Code,
				"pageIndex":     page,
				"pageSize":      50,
			}

			resp, err := client.Call("erp.batchstockquantity.get", biz)
			if err != nil {
				log.Printf("  API调用失败(page=%d): %v", page, err)
				break
			}
			if resp.Code != 200 {
				log.Printf("  API错误: code=%d msg=%s", resp.Code, resp.Msg)
				break
			}

			var wrapper struct {
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(resp.Result, &wrapper); err != nil {
				log.Printf("  解析result失败: %v", err)
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
				log.Printf("  解析data失败: %v", err)
				break
			}

			items := result.GoodsStockQuantity
			if len(items) == 0 {
				break
			}

			for _, item := range items {
				err := upsertBatch(db, item)
				if err != nil {
					log.Printf("  写入失败: %v", err)
					continue
				}
				whInserted++
			}

			if len(items) < 50 {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}

		totalInserted += whInserted
		log.Printf("  %s: %d 条批次数据", wh.Name, whInserted)
	}

	elapsed := time.Since(startTime)
	log.Printf("=== 批次库存同步完成: 共 %d 条, 耗时 %s ===", totalInserted, elapsed.Round(time.Second))
}

func upsertBatch(db *sql.DB, m map[string]interface{}) error {
	// 解析生产日期（可能是毫秒时间戳或日期字符串）
	prodDate := parseDateField(m, "productionDate")
	expDate := parseDateField(m, "expirationDate")

	_, err := db.Exec(`INSERT INTO stock_batch
		(quantity_id, goods_id, goods_no, goods_name, sku_id, sku_name, sku_barcode, unit_name,
		 warehouse_id, warehouse_name, warehouse_code,
		 batch_no, batch_number, production_date, expiration_date,
		 shelf_life, shelf_life_unit,
		 current_qty, locked_qty, use_qty, defective_qty,
		 memo, out_sku_code, production_depart, registration_number,
		 synced_at)
		VALUES (?,?,?,?,?,?,?,?, ?,?,?, ?,?,?,?, ?,?, ?,?,?,?, ?,?,?,?, NOW())
		ON DUPLICATE KEY UPDATE
			goods_no=VALUES(goods_no), goods_name=VALUES(goods_name),
			sku_name=VALUES(sku_name), sku_barcode=VALUES(sku_barcode),
			warehouse_name=VALUES(warehouse_name),
			batch_number=VALUES(batch_number),
			production_date=VALUES(production_date), expiration_date=VALUES(expiration_date),
			shelf_life=VALUES(shelf_life), shelf_life_unit=VALUES(shelf_life_unit),
			current_qty=VALUES(current_qty), locked_qty=VALUES(locked_qty),
			use_qty=VALUES(use_qty), defective_qty=VALUES(defective_qty),
			memo=VALUES(memo), out_sku_code=VALUES(out_sku_code),
			production_depart=VALUES(production_depart), registration_number=VALUES(registration_number),
			synced_at=NOW()`,
		gf(m, "quantityId"), gf(m, "goodsId"), gs(m, "goodsNo"), gs(m, "goodsName"),
		gf(m, "skuId"), gs(m, "skuName"), gs(m, "skuBarcode"), gs(m, "unitName"),
		gf(m, "warehouseId"), gs(m, "warehouseName"), gs(m, "warehouseCode"),
		gs(m, "batchNo"), gs(m, "batchNumber"),
		prodDate, expDate,
		gi(m, "shelfLife"), gs(m, "shelfLiftUnit"),
		gd(m, "currentQuantity"), gd(m, "lockedQuantity"),
		gd(m, "useQuantity"), gd(m, "defectiveQuanity"),
		gs(m, "memo"), gs(m, "outSkuCode"),
		gs(m, "productionDepart"), gs(m, "registrationNumber"),
	)
	return err
}

// parseDateField 解析日期字段（支持float64毫秒时间戳、字符串时间戳、日期字符串）
func parseDateField(m map[string]interface{}, key string) interface{} {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}

	// 直接是float64（JSON数字，可能是毫秒时间戳）
	if f, ok := v.(float64); ok && f > 1000000000000 {
		t := time.Unix(int64(f)/1000, 0)
		return t.Format("2006-01-02")
	}
	if f, ok := v.(float64); ok && f > 1000000000 {
		t := time.Unix(int64(f), 0)
		return t.Format("2006-01-02")
	}

	s := fmt.Sprintf("%v", v)
	if s == "" || s == "0" || s == "null" {
		return nil
	}
	s = strings.TrimSpace(s)

	// 尝试解析为数字时间戳
	if ts, err := strconv.ParseFloat(s, 64); err == nil {
		if ts > 1000000000000 {
			t := time.Unix(int64(ts)/1000, 0)
			return t.Format("2006-01-02")
		}
		if ts > 1000000000 {
			t := time.Unix(int64(ts), 0)
			return t.Format("2006-01-02")
		}
	}

	// 已是日期字符串：必须是 YYYY-MM-DD 格式
	if len(s) >= 10 {
		candidate := s[:10]
		if _, err := time.Parse("2006-01-02", candidate); err == nil {
			return candidate
		}
	}
	return nil
}

func gs(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

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

func gd(m map[string]interface{}, key string) float64 { return gf(m, key) }
func gi(m map[string]interface{}, key string) int      { return int(gf(m, key)) }
