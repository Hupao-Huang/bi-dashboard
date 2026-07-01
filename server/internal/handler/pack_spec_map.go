package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// 销售日报箱规映射(dim_goods_pack_spec)的页面维护接口。
// 背景: TOP10 单品「发货件数=sell_count×箱规」「发货箱数/托数」都靠这张表; 箱规=1 表示该货品已是最小单位。
//   原来只能改 RPA映射表.xlsx / 箱规拖规.xlsx 再跑 import-report-maps 重导, 本接口让供应链角色直接页面维护。
// 权限: 读写复用 supply_chain.sales_daily_report:edit(与渠道映射同, 授超管+供应链角色)。

// validatePackSpecRow 校验一行: 货品编码非空, 箱规必填且>0, 托规可空(0/空=不填)但有值必>0
func validatePackSpecRow(goodsNo string, boxQty, palletBoxQty float64) error {
	if strings.TrimSpace(goodsNo) == "" {
		return fmt.Errorf("货品编码不能为空")
	}
	if boxQty <= 0 {
		return fmt.Errorf("箱规必须大于 0(最小单位填 1)")
	}
	if palletBoxQty < 0 {
		return fmt.Errorf("托规不能为负")
	}
	return nil
}

type packSpecRow struct {
	GoodsNo      string  `json:"goodsNo"`
	BoxQty       float64 `json:"boxQty"`
	PalletBoxQty float64 `json:"palletBoxQty"` // 0 = 未填(前端显—)
}

// GetPackSpec GET /api/supply-chain/pack-spec — 列出全部箱规映射(按货品编码排)
func (h *DashboardHandler) GetPackSpec(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT goods_no, box_qty, IFNULL(pallet_box_qty,0) FROM dim_goods_pack_spec`)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	defer rows.Close()
	var list []packSpecRow
	for rows.Next() {
		var m packSpecRow
		if err := rows.Scan(&m.GoodsNo, &m.BoxQty, &m.PalletBoxQty); err != nil {
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
		if err := validatePackSpecRow(m.GoodsNo, m.BoxQty, m.PalletBoxQty); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("第 %d 行: %v", i+1, err))
			return
		}
	}
	for _, m := range req.Rows {
		var pallet interface{}
		if m.PalletBoxQty > 0 {
			pallet = m.PalletBoxQty
		} // 0 → NULL(未填托规)
		if _, err := h.DB.ExecContext(r.Context(),
			`INSERT INTO dim_goods_pack_spec(goods_no, box_qty, pallet_box_qty) VALUES(?,?,?)
			 ON DUPLICATE KEY UPDATE box_qty=VALUES(box_qty), pallet_box_qty=VALUES(pallet_box_qty)`,
			strings.TrimSpace(m.GoodsNo), m.BoxQty, pallet,
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
