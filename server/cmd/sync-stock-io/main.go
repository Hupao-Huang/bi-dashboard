package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"bi-dashboard/internal/jackyun"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// 按日期范围同步出入库流水(幂等：先DELETE再INSERT)
// 用法：SYNC_START_DATE=2026-04-01 SYNC_END_DATE=2026-04-20 sync-stock-io.exe
func main() {
	unlock := importutil.AcquireLock("sync-stock-io")
	defer unlock()

	cfg, err := config.Load("config.json")
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

	startStr := os.Getenv("SYNC_START_DATE")
	endStr := os.Getenv("SYNC_END_DATE")
	if startStr == "" || endStr == "" {
		log.Fatalf("必须指定 SYNC_START_DATE 和 SYNC_END_DATE (yyyy-MM-dd)")
	}

	startDate, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		log.Fatalf("开始日期错误: %v", err)
	}
	endDate, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		log.Fatalf("结束日期错误: %v", err)
	}

	fmt.Printf("同步出入库流水: %s ~ %s\n", startStr, endStr)

	// 按天循环：先删后拉
	totalIn, totalOut := 0, 0
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		dayStr := d.Format("2006-01-02")
		dayStart := dayStr + " 00:00:00"
		dayEnd := dayStr + " 23:59:59"

		// 先删除当天旧数据
		if _, err := db.Exec(`DELETE FROM stock_in_log WHERE in_out_date = ?`, dayStr); err != nil {
			log.Fatalf("删除入库 %s 失败: %v", dayStr, err)
		}
		if _, err := db.Exec(`DELETE FROM stock_out_log WHERE in_out_date = ?`, dayStr); err != nil {
			log.Fatalf("删除出库 %s 失败: %v", dayStr, err)
		}

		// 拉入库
		fmt.Printf("[%s] 拉入库...\n", dayStr)
		inQuery := jackyun.StockIOQuery{
			InOutDateStart: dayStart,
			InOutDateEnd:   dayEnd,
			PageIndex:      0,
			PageSize:       50,
		}
		inCount := 0
		err := client.FetchStockIO("erp-busiorder.goodsdocin.search", inQuery, func(items []jackyun.StockIOItem) error {
			for _, it := range items {
				if _, err := db.Exec(`
					INSERT INTO stock_in_log
					(in_out_date, goodsdoc_no, inouttype_name, warehouse_code, warehouse_name,
					 bill_no, vend_code, vend_name, goods_no, goods_name, sku_name, sku_barcode, unit_name,
					 batch_no, quantity, cost_price, cost_amount, no_tax_price, with_tax_price,
					 is_certified, create_user_name, remark, detail_remark)
					VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
					dayStr, it.GoodsdocNo.String(), it.InouttypeName.String(),
					it.WarehouseCode.String(), it.WarehouseName.String(),
					it.BillNo.String(), it.VendCode.String(), it.VendCustomerName.String(),
					it.GoodsNo.String(), it.GoodsName.String(), it.SkuName.String(), it.SkuBarcode.String(),
					it.UnitName.String(), it.BatchNo.String(),
					it.Quantity.Float64(), it.BaceCurrencyCostPrice.Float64(), it.BaceCurrencyCostAmount.Float64(),
					it.BaceCurrencyNoTaxPrice.Float64(), it.BaceCurrencyWithTaxPrice.Float64(),
					int(it.IsCertified), it.CreateUserName.String(),
					it.GoodsdocRemark.String(), it.GoodsDetailRemark.String(),
				); err != nil {
					return fmt.Errorf("入库写入失败 %s/%s: %w", it.GoodsdocNo.String(), it.GoodsNo.String(), err)
				}
				inCount++
			}
			return nil
		})
		if err != nil {
			fmt.Printf("  入库失败: %v\n", err)
		}
		fmt.Printf("  入库完成 (%d 条)\n", inCount)
		totalIn += inCount

		// 拉出库
		fmt.Printf("[%s] 拉出库...\n", dayStr)
		outQuery := jackyun.StockIOQuery{
			InOutDateStart: dayStart,
			InOutDateEnd:   dayEnd,
			PageIndex:      0,
			PageSize:       50,
		}
		outCount := 0
		err = client.FetchStockIO("erp-busiorder.goodsdocout.search", outQuery, func(items []jackyun.StockIOItem) error {
			for _, it := range items {
				var recId int64 = 0
				if it.RecId.String() != "" {
					fmt.Sscanf(it.RecId.String(), "%d", &recId)
				}
				if _, err := db.Exec(`
					INSERT INTO stock_out_log
					(in_out_date, rec_id, goodsdoc_no, inouttype_name, warehouse_code, warehouse_name,
					 bill_no, vend_code, vend_name, channel_code, goods_no, goods_name, sku_name, sku_barcode, unit_name,
					 batch_no, quantity, cost_price, cost_amount, no_tax_price, with_tax_price,
					 is_certified, create_user_name, remark, detail_remark)
					VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
					dayStr, recId, it.GoodsdocNo.String(), it.InouttypeName.String(),
					it.WarehouseCode.String(), it.WarehouseName.String(),
					it.BillNo.String(), it.VendCode.String(), it.VendCustomerName.String(),
					it.ChannelCode.String(),
					it.GoodsNo.String(), it.GoodsName.String(), it.SkuName.String(), it.SkuBarcode.String(),
					it.UnitName.String(), it.BatchNo.String(),
					it.Quantity.Float64(), it.BaceCurrencyCostPrice.Float64(), it.BaceCurrencyCostAmount.Float64(),
					it.BaceCurrencyNoTaxPrice.Float64(), it.BaceCurrencyWithTaxPrice.Float64(),
					int(it.IsCertified), it.CreateUserName.String(),
					it.GoodsdocRemark.String(), it.GoodsDetailRemark.String(),
				); err != nil {
					return fmt.Errorf("出库写入失败 %s/%s: %w", it.GoodsdocNo.String(), it.GoodsNo.String(), err)
				}
				outCount++
			}
			return nil
		})
		if err != nil {
			fmt.Printf("  出库失败: %v\n", err)
		}
		fmt.Printf("  出库完成 (%d 条)\n", outCount)
		totalOut += outCount

		time.Sleep(200 * time.Millisecond)
	}

	fmt.Printf("\n同步完成！入库 %d 条，出库 %d 条\n", totalIn, totalOut)
}
