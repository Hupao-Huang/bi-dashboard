package handler

import (
	"encoding/json"
	"net/http"
	"strings"
)

// 特殊渠道价格表(channel_special_price)的页面维护接口。
// 背景: 京东/猫超/朴朴 调拨当销售, 单价来自这张表; 表里没配的商品销售额按 0 算(对账页弹"价格表缺失")。
//   原来只能改桌面 价格体系.xlsx 再跑 import-channel-price.exe 重导。本接口让有权限的人直接在对账页填价,
//   填完当场把这商品已发的调拨单金额重算 + 清看板缓存 + 记审计, 不用再走 Excel。
// 权限: 保存走 ecommerce.special_channel_allot:edit (默认仅超管, 跑哥可在角色权限页放给指定角色);
//   列表走 ecommerce/instant_retail 任一 view (谁能看对账页就能看价格)。
// Excel 仍可用: import-channel-price.exe 已改成"有则更新无则新增"(不再整渠道清空), 两边并存不冲突。

var validChannelKeys = map[string]bool{"京东": true, "猫超": true, "朴朴": true}

// dept → 渠道清单 (跟 special_channel.go channelMapByDept 一致)
var priceChannelsByDept = map[string][]string{
	"ecommerce":      {"京东", "猫超"},
	"instant_retail": {"朴朴"},
}

// SaveChannelPrice POST /api/special-channel-allot/save-price
// body: {channelKey, goodsNo, barcode, goodsName, price}
// 存价 → 重算该商品全部调拨明细金额 → 清缓存 → 审计
func (h *DashboardHandler) SaveChannelPrice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		ChannelKey string  `json:"channelKey"`
		GoodsNo    string  `json:"goodsNo"`
		Barcode    string  `json:"barcode"`
		GoodsName  string  `json:"goodsName"`
		Price      float64 `json:"price"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	req.ChannelKey = strings.TrimSpace(req.ChannelKey)
	req.GoodsNo = strings.TrimSpace(req.GoodsNo)
	req.Barcode = strings.TrimSpace(req.Barcode)
	req.GoodsName = strings.TrimSpace(req.GoodsName)

	if !validChannelKeys[req.ChannelKey] {
		writeError(w, http.StatusBadRequest, "渠道不对(只支持 京东/猫超/朴朴)")
		return
	}
	if req.GoodsNo == "" {
		writeError(w, http.StatusBadRequest, "商品编码不能为空")
		return
	}
	// 价格护栏: 必须大于 0, 不超过 10 万(防手滑填负数/天文数字)
	if req.Price <= 0 || req.Price > 100000 {
		writeError(w, http.StatusBadRequest, "单价要大于 0 且不超过 10 万")
		return
	}

	// 1) 存价格表 (UK: channel_key+goods_no)
	if _, err := h.DB.Exec(`INSERT INTO channel_special_price
		(channel_key, goods_no, barcode, goods_name, price, source_xlsx)
		VALUES (?, ?, ?, ?, ?, '页面维护')
		ON DUPLICATE KEY UPDATE
		barcode=VALUES(barcode), goods_name=VALUES(goods_name),
		price=VALUES(price), source_xlsx=VALUES(source_xlsx)`,
		req.ChannelKey, req.GoodsNo, req.Barcode, req.GoodsName, req.Price); writeDatabaseError(w, err) {
		return
	}

	// 2) 重算这商品已发的全部调拨明细金额 (含原来"缺失"的, 也含已配价改价的)
	res, err := h.DB.Exec(`UPDATE allocate_details
		SET excel_price=?, excel_amount=ROUND(?*sku_count,4), price_source='excel'
		WHERE channel_key=? AND goods_no=?`,
		req.Price, req.Price, req.ChannelKey, req.GoodsNo)
	if writeDatabaseError(w, err) {
		return
	}
	updated, _ := res.RowsAffected()

	// 3) 清看板缓存 (综合/部门/计划看板的调拨 GMV 用了缓存, 不清挂到过期)
	ClearOverviewCache()

	// 4) 审计: 谁把哪个商品改成多少
	h.logAudit(r, "edit_channel_price", "special_channel_allot", map[string]interface{}{
		"channelKey": req.ChannelKey,
		"goodsNo":    req.GoodsNo,
		"goodsName":  req.GoodsName,
		"price":      req.Price,
		"rows":       updated,
	})

	writeJSON(w, map[string]interface{}{
		"goodsNo":      req.GoodsNo,
		"price":        req.Price,
		"updatedRows":  updated,
	})
}

// GetChannelPrices GET /api/special-channel-allot/prices?dept=ecommerce|instant_retail
// 返回该部门对应渠道的全部已配价格 (给"全部价格"表用)
func (h *DashboardHandler) GetChannelPrices(w http.ResponseWriter, r *http.Request) {
	dept := r.URL.Query().Get("dept")
	channels, ok := priceChannelsByDept[dept]
	if !ok {
		// 空 dept = 全部 (向后兼容)
		channels = []string{"京东", "猫超", "朴朴"}
	}
	placeholders := make([]string, len(channels))
	args := make([]interface{}, len(channels))
	for i, c := range channels {
		placeholders[i] = "?"
		args[i] = c
	}

	rows, err := h.DB.Query(`SELECT channel_key, goods_no, barcode, goods_name, price,
		source_xlsx, DATE_FORMAT(imported_at, '%Y-%m-%d %H:%i')
		FROM channel_special_price
		WHERE channel_key IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY goods_no, channel_key`, args...)
	if writeDatabaseError(w, err) {
		return
	}
	defer rows.Close()

	type PriceRow struct {
		ChannelKey string  `json:"channelKey"`
		GoodsNo    string  `json:"goodsNo"`
		Barcode    string  `json:"barcode"`
		GoodsName  string  `json:"goodsName"`
		Price      float64 `json:"price"`
		Source     string  `json:"source"`
		UpdatedAt  string  `json:"updatedAt"`
	}
	list := []PriceRow{}
	for rows.Next() {
		var p PriceRow
		if writeDatabaseError(w, rows.Scan(&p.ChannelKey, &p.GoodsNo, &p.Barcode, &p.GoodsName, &p.Price, &p.Source, &p.UpdatedAt)) {
			return
		}
		list = append(list, p)
	}
	writeJSON(w, map[string]interface{}{"list": list})
}
