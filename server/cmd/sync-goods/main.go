package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"bi-dashboard/internal/jackyun"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	unlock := importutil.AcquireLock("sync-goods")
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
	db.SetMaxOpenConns(20)

	client := jackyun.NewClient(cfg.JackYun.AppKey, cfg.JackYun.Secret, cfg.JackYun.APIURL)

	fmt.Println("开始同步货品档案...")

	total := 0
	maxSkuId := "0"

	for {
		biz := map[string]interface{}{
			"pageIndex": 0,
			"pageSize":  50,
			"maxSkuId":  maxSkuId,
		}

		resp, err := client.Call("erp.storage.goodslist", biz)
		if err != nil {
			log.Fatalf("调用失败: %v", err)
		}
		if resp.Code != 200 {
			log.Fatalf("接口报错: code=%d msg=%s", resp.Code, resp.Msg)
		}

		var wrapper struct {
			Data json.RawMessage `json:"data"`
		}
		json.Unmarshal(resp.Result, &wrapper)

		var dataWrapper struct {
			Goods []map[string]interface{} `json:"goods"`
		}
		if err := json.Unmarshal(wrapper.Data, &dataWrapper); err != nil {
			log.Fatalf("解析失败: %v", err)
		}

		items := dataWrapper.Goods
		if len(items) == 0 {
			break
		}

		for _, g := range items {
			_, err := db.Exec(`
				INSERT INTO goods
					(goods_id, goods_no, goods_name, goods_name_en, goods_alias, goods_desc, goods_memo, goods_attr,
					 sku_id, sku_no, sku_name, sku_barcode, main_barcode, sku_code, unit_name,
					 cate_id, cate_name, cate_full_name, brand_id, brand_name,
					 color_code, color_name, size_code, size_name,
					 sku_weight, sku_length, sku_width, sku_height, volume, retail_price,
					 abc_cate, warehouse_id, warehouse_name, default_vend_id, default_vend_name,
					 owner_type, owner_name, sku_img_url,
					 is_package_good, is_delete, sku_is_blockup, is_serial_mgmt, is_batch_mgmt,
					 flag_data, sell_info,
					 gmt_create, gmt_modified, sku_gmt_create, sku_gmt_modified,
					 goods_field1, goods_field2, goods_field3, goods_field4, goods_field5,
					 goods_field6, goods_field7, goods_field8, goods_field9, goods_field10,
					 sku_field1, sku_field2, sku_field3, sku_field4, sku_field5)
				Values (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
				ON DUPLICATE KEY UPDATE
					goods_no=VALUES(goods_no), goods_name=VALUES(goods_name), sku_name=VALUES(sku_name),
					sku_barcode=VALUES(sku_barcode), cate_name=VALUES(cate_name), brand_name=VALUES(brand_name),
					gmt_modified=VALUES(gmt_modified), sku_gmt_modified=VALUES(sku_gmt_modified),
					is_delete=VALUES(is_delete), sku_is_blockup=VALUES(sku_is_blockup),
					goods_field6=VALUES(goods_field6), goods_field7=VALUES(goods_field7), goods_field8=VALUES(goods_field8), goods_field9=VALUES(goods_field9), goods_field10=VALUES(goods_field10),
					flag_data=VALUES(flag_data)`,
				gs(g, "goodsId"), gn(g, "goodsNo"), gn(g, "goodsName"), gn(g, "goodsNameEn"), gn(g, "goodsAlias"),
				gn(g, "goodsDesc"), gn(g, "goodsMemo"), gn(g, "goodsAttr"),
				gs(g, "skuId"), gn(g, "skuNo"), gn(g, "skuName"), gn(g, "skuBarcode"), gn(g, "mainBarcode"), gn(g, "skuCode"), gn(g, "unitName"),
				gs(g, "cateId"), gn(g, "cateName"), gn(g, "cateFullName"), gs(g, "brandId"), gn(g, "brandName"),
				gn(g, "colorCode"), gn(g, "colorName"), gn(g, "sizeCode"), gn(g, "sizeName"),
				gn(g, "skuWeight"), gn(g, "skuLength"), gn(g, "skuWidth"), gn(g, "skuHeight"), gn(g, "volume"), gn(g, "retailPrice"),
				gn(g, "abcCate"), gs(g, "warehouseId"), gn(g, "warehouseName"), gs(g, "defaultVendId"), gn(g, "defaultVendName"),
				gn(g, "ownerType"), gn(g, "ownerName"), gn(g, "skuImgUrl"),
				gn(g, "isPackageGood"), gn(g, "isDelete"), gn(g, "skuIsBlockup"), gn(g, "isSerialManagement"), gn(g, "isBatchMgmt"),
				gn(g, "flagData"), gn(g, "sellInfo"),
				gt(g, "gmtCreate"), gt(g, "goodsGmtModified"), gt(g, "skuGmtCreate"), gt(g, "skuGmtModified"),
				gn(g, "goodsField1"), gn(g, "goodsField2"), gn(g, "goodsField3"), gn(g, "goodsField4"), gn(g, "goodsField5"),
				gn(g, "goodsField6"), gn(g, "goodsField7"), gn(g, "goodsField8"), gn(g, "goodsField9"), gn(g, "goodsField10"),
				gn(g, "skuField1"), gn(g, "skuField2"), gn(g, "skuField3"), gn(g, "skuField4"), gn(g, "skuField5"),
			)
			if err != nil {
				log.Printf("写入 %v/%v 失败: %v", g["goodsNo"], g["skuName"], err)
			}

			// 更新 maxSkuId
			sid := gs(g, "skuId")
			if sid > maxSkuId {
				maxSkuId = sid
			}
		}

		total += len(items)
		fmt.Printf("  已同步 %d 条 (maxSkuId=%s)\n", total, maxSkuId)

		if len(items) < 50 {
			break
		}
	}

	fmt.Printf("\n同步完成！共 %d 条货品记录\n", total)
}

// gs: get string (never nil, for IDs)
func gs(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return "0"
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == math.Trunc(val) {
			return fmt.Sprintf("%.0f", val)
		}
		return fmt.Sprintf("%v", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// gn: get nullable value
func gn(m map[string]interface{}, key string) interface{} {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		if val == "" {
			return nil
		}
		return val
	case float64:
		return val
	default:
		return fmt.Sprintf("%v", val)
	}
}

// gt: get timestamp -> datetime
func gt(m map[string]interface{}, key string) interface{} {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch val := v.(type) {
	case float64:
		if val == 0 {
			return nil
		}
		ms := int64(val)
		if ms > 1000000000000 {
			ms = ms / 1000
		}
		return time.Unix(ms, 0).Format("2006-01-02 15:04:05")
	case string:
		if val == "" || val == "0" {
			return nil
		}
		return val
	default:
		return nil
	}
}
