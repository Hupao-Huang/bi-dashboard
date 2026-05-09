// sync-goods-blend: 拉取吉客云组合装 BOM 子件信息
//
// 接口: erp-goods.goods.listgoodspackage
// 用法:
//   全量(默认): ./sync-goods-blend
//   单 SKU 测试: ./sync-goods-blend SYFX2026041109
//
// 翻页: maxSkuId 游标分页, 单页 200, 总 8262 个组合装预计 ~42 页
package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"bi-dashboard/internal/jackyun"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type packageItem struct {
	SkuId     json.Number `json:"skuId"`
	GoodsId   json.Number `json:"goodsId"`
	GoodsNo   string      `json:"goodsNo"`
	GoodsName string      `json:"goodsName"`

	GoodsPackageDetail []struct {
		PackageSkuId      json.Number `json:"packageSkuId"`
		GoodsId           json.Number `json:"goodsId"`
		SkuId             json.Number `json:"skuId"`
		GoodsAmount       json.Number `json:"goodsAmount"`
		SkuBarcode        string      `json:"skuBarcode"`
		GoodsName         string      `json:"goodsName"`
		GoodsNo           string      `json:"goodsNo"`
		SkuProperitesName string      `json:"skuProperitesName"`
		UnitName          string      `json:"unitName"`
		SharePrice        json.Number `json:"sharePrice"`
		ShareAmount       json.Number `json:"shareAmount"`
		ShareRatio        json.Number `json:"shareRatio"`
		IsGiveaway        json.Number `json:"isGiveaway"`
		CateName          string      `json:"cateName"`
	} `json:"goodsPackageDetail"`
}

func main() {
	unlock := importutil.AcquireLock("sync-goods-blend")
	defer unlock()

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	cli := jackyun.NewClient(cfg.JackYun.AppKey, cfg.JackYun.Secret, cfg.JackYun.APIURL)

	specificGoodsNo := ""
	if len(os.Args) >= 2 {
		specificGoodsNo = os.Args[1]
	}

	maxSkuId := int64(0)
	pageSize := 200
	totalParents := 0
	totalChildren := 0
	pageNum := 0

	for {
		pageNum++
		biz := map[string]interface{}{
			"pageIndex": 0,
			"pageSize":  pageSize,
			"maxSkuId":  maxSkuId,
		}
		if specificGoodsNo != "" {
			biz["goodsNo"] = specificGoodsNo
		}

		resp, err := cli.Call("erp-goods.goods.listgoodspackage", biz)
		if err != nil {
			log.Fatalf("[page %d] call err: %v", pageNum, err)
		}
		if resp.Code != 200 {
			log.Fatalf("[page %d] api code=%d msg=%s sub=%s", pageNum, resp.Code, resp.Msg, resp.SubCode)
		}

		// 解包 result.data: 可能是 array (列表) 也可能是 object (单条) 或 string-encoded JSON
		var wrapper struct {
			Data json.RawMessage `json:"data"`
		}
		json.Unmarshal(resp.Result, &wrapper)

		// 尝试当作 string 解 (有些接口返回 escaped json string)
		var dataBytes []byte
		var dataStr string
		if err := json.Unmarshal(wrapper.Data, &dataStr); err == nil {
			dataBytes = []byte(dataStr)
		} else {
			dataBytes = wrapper.Data
		}

		// data 既可能是 array, 也可能是单 object
		var items []packageItem
		if err := json.Unmarshal(dataBytes, &items); err != nil {
			// 尝试单 object
			var single packageItem
			if err2 := json.Unmarshal(dataBytes, &single); err2 == nil && single.GoodsNo != "" {
				items = []packageItem{single}
			} else {
				log.Printf("[page %d] unmarshal data 失败: %v / single: %v, body: %s", pageNum, err, err2, string(dataBytes)[:min(500, len(dataBytes))])
				break
			}
		}

		if len(items) == 0 {
			log.Printf("[page %d] 0 items, 结束", pageNum)
			break
		}

		log.Printf("[page %d] 拉到 %d 个组合装父品", pageNum, len(items))

		var lastSkuId int64
		for _, p := range items {
			if cur, _ := p.SkuId.Int64(); cur > lastSkuId {
				lastSkuId = cur
			}
			if len(p.GoodsPackageDetail) == 0 {
				continue
			}
			totalParents++
			for _, c := range p.GoodsPackageDetail {
				goodsAmt, _ := c.GoodsAmount.Float64()
				if goodsAmt <= 0 {
					goodsAmt = 1
				}
				sharePrice, _ := c.SharePrice.Float64()
				shareAmount, _ := c.ShareAmount.Float64()
				isGiveaway := 0
				if c.IsGiveaway.String() == "1" {
					isGiveaway = 1
				}
				_, err := db.Exec(`
					INSERT INTO goods_blend_detail
						(parent_goods_no, parent_sku_id, parent_goods_name,
						 child_goods_no, child_sku_id, child_goods_name, child_barcode, child_spec_name,
						 goods_amount, unit_name, share_price, share_amount, share_ratio,
						 is_giveaway, cate_name)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
					ON DUPLICATE KEY UPDATE
						parent_sku_id=VALUES(parent_sku_id),
						parent_goods_name=VALUES(parent_goods_name),
						child_goods_name=VALUES(child_goods_name),
						child_barcode=VALUES(child_barcode),
						child_spec_name=VALUES(child_spec_name),
						goods_amount=VALUES(goods_amount),
						unit_name=VALUES(unit_name),
						share_price=VALUES(share_price),
						share_amount=VALUES(share_amount),
						share_ratio=VALUES(share_ratio),
						is_giveaway=VALUES(is_giveaway),
						cate_name=VALUES(cate_name)`,
					p.GoodsNo, p.SkuId.String(), p.GoodsName,
					c.GoodsNo, c.SkuId.String(), c.GoodsName, c.SkuBarcode, c.SkuProperitesName,
					goodsAmt, c.UnitName, sharePrice, shareAmount, c.ShareRatio.String(),
					isGiveaway, c.CateName,
				)
				if err != nil {
					log.Printf("写入失败 parent=%s child=%s: %v", p.GoodsNo, c.GoodsNo, err)
				} else {
					totalChildren++
				}
			}
		}

		// 单 SKU 测试模式只跑一页
		if specificGoodsNo != "" {
			break
		}

		// 分页结束判定: 返回少于 pageSize 或 lastSkuId 没变
		if len(items) < pageSize {
			break
		}
		if lastSkuId == maxSkuId {
			log.Printf("maxSkuId 没增长(%d), 停止", lastSkuId)
			break
		}
		maxSkuId = lastSkuId
		// 友好限流
		time.Sleep(300 * time.Millisecond)
	}

	log.Printf("=== 同步完成: %d 个组合装父品, %d 条子件 (%d 页) ===", totalParents, totalChildren, pageNum)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
