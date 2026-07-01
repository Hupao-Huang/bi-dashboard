package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// 销售日报箱规映射(dim_goods_pack_spec)的页面维护接口。
// 背景: TOP10 单品「件数=sell_count×销售规格」「箱数=件数÷装箱规格」「托数=箱数÷托规」都靠这张表。
//   销售规格(box_qty)=吉客云下单1件几瓶; 装箱规格(carton_pieces)=仓库1箱几瓶; 二者独立列(整箱货相等, 单品散卖货不等)。
//   原来只能改 RPA映射表.xlsx / 箱规拖规.xlsx 再跑 import-report-maps 重导, 本接口让供应链角色直接页面维护。
// 权限: 读写复用 supply_chain.sales_daily_report:edit(与渠道映射同, 授超管+供应链角色)。

// validatePackSpecRow 校验一行: 货品编码非空; 销售规格必填且>0; 装箱规格可空(0=未填,默认同销售规格)但有值必>0; 托规同理≥0
func validatePackSpecRow(goodsNo string, boxQty, cartonPieces, palletBoxQty float64) error {
	if strings.TrimSpace(goodsNo) == "" {
		return fmt.Errorf("货品编码不能为空")
	}
	if boxQty <= 0 {
		return fmt.Errorf("销售规格必须大于 0(单品散卖填 1)")
	}
	if cartonPieces < 0 {
		return fmt.Errorf("装箱规格不能为负")
	}
	if palletBoxQty < 0 {
		return fmt.Errorf("托规不能为负")
	}
	return nil
}

type packSpecRow struct {
	GoodsNo      string  `json:"goodsNo"`
	GoodsName    string  `json:"goodsName"`    // join goods 拿名称(只看编码认不出货)
	BoxQty       float64 `json:"boxQty"`       // 销售规格=每件瓶数(下单1件折几瓶)
	CartonPieces float64 `json:"cartonPieces"` // 装箱规格=每箱瓶数(1箱几瓶), 0=未填(默认同销售规格)
	PalletBoxQty float64 `json:"palletBoxQty"` // 托规=每托箱数, 0 = 未填(前端显—)
}

// GetPackSpec GET /api/supply-chain/pack-spec — 列出全部箱规映射(按货品编码排), 带货品名称
func (h *DashboardHandler) GetPackSpec(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT p.goods_no, IFNULL(MAX(g.goods_name),''), IFNULL(p.box_qty,0), IFNULL(p.carton_pieces,0), IFNULL(p.pallet_box_qty,0)
		 FROM dim_goods_pack_spec p LEFT JOIN goods g ON g.goods_no=p.goods_no
		 GROUP BY p.goods_no, p.box_qty, p.carton_pieces, p.pallet_box_qty`)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	defer rows.Close()
	var list []packSpecRow
	for rows.Next() {
		var m packSpecRow
		if err := rows.Scan(&m.GoodsNo, &m.GoodsName, &m.BoxQty, &m.CartonPieces, &m.PalletBoxQty); err != nil {
			writeError(w, http.StatusInternalServerError, "读取失败")
			return
		}
		list = append(list, m)
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].GoodsNo < list[j].GoodsNo })
	writeJSON(w, map[string]interface{}{"list": list})
}

// SavePackSpec POST /api/supply-chain/pack-spec/save — 批量 upsert(货品编码为主键)
// body: {rows: [{goodsNo, boxQty, palletBoxQty}, ...]}
func (h *DashboardHandler) SavePackSpec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Rows []packSpecRow `json:"rows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if len(req.Rows) == 0 {
		writeError(w, http.StatusBadRequest, "没有要保存的行")
		return
	}
	if len(req.Rows) > 500 {
		writeError(w, http.StatusBadRequest, "单次最多保存 500 行")
		return
	}
	for i, m := range req.Rows {
		if err := validatePackSpecRow(m.GoodsNo, m.BoxQty, m.CartonPieces, m.PalletBoxQty); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("第 %d 行: %v", i+1, err))
			return
		}
	}
	for _, m := range req.Rows {
		var carton, pallet interface{}
		if m.CartonPieces > 0 {
			carton = m.CartonPieces
		} // 0 → NULL(未填,箱数默认按销售规格算)
		if m.PalletBoxQty > 0 {
			pallet = m.PalletBoxQty
		} // 0 → NULL(未填托规)
		if _, err := h.DB.ExecContext(r.Context(),
			`INSERT INTO dim_goods_pack_spec(goods_no, box_qty, carton_pieces, pallet_box_qty) VALUES(?,?,?,?)
			 ON DUPLICATE KEY UPDATE box_qty=VALUES(box_qty), carton_pieces=VALUES(carton_pieces), pallet_box_qty=VALUES(pallet_box_qty)`,
			strings.TrimSpace(m.GoodsNo), m.BoxQty, carton, pallet,
		); writeDatabaseError(w, err) {
			return
		}
	}
	writeJSON(w, map[string]interface{}{"saved": len(req.Rows)})
}

// DeletePackSpec POST /api/supply-chain/pack-spec/delete — 删一行 body: {goodsNo}
func (h *DashboardHandler) DeletePackSpec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		GoodsNo string `json:"goodsNo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if strings.TrimSpace(req.GoodsNo) == "" {
		writeError(w, http.StatusBadRequest, "货品编码不能为空")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM dim_goods_pack_spec WHERE goods_no=?`, strings.TrimSpace(req.GoodsNo),
	); writeDatabaseError(w, err) {
		return
	}
	writeJSON(w, map[string]interface{}{"deleted": req.GoodsNo})
}
