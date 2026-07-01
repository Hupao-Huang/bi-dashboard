package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// 销售日报渠道映射(dim_sales_channel_map)的页面维护接口。
// 背景: 渠道汇总按 shop_name join 这张表拆平台/渠道; 原来只能改 RPA映射表.xlsx 再跑 import-report-maps 重导。
//   本接口让供应链角色直接在销售日报页维护「店铺→渠道→平台」映射, 随时调整, 不用走 Excel。
// 权限: 读写都走 supply_chain.sales_daily_report:edit (授超管+供应链角色, 跑哥定"供应链角色都能改")。
// 平台三选一(跑哥定手动选, 不再由 platformOf 自动推)。

var validPlatforms = map[string]bool{"社媒": true, "电商": true, "其他": true}

// validateChannelMapRow 校验一行映射: 店铺/渠道非空, 平台必须三选一
func validateChannelMapRow(shop, channel, platform string) error {
	if strings.TrimSpace(shop) == "" {
		return fmt.Errorf("店铺名不能为空")
	}
	if strings.TrimSpace(channel) == "" {
		return fmt.Errorf("渠道不能为空")
	}
	if !validPlatforms[strings.TrimSpace(platform)] {
		return fmt.Errorf("平台只能是 社媒/电商/其他")
	}
	return nil
}

type channelMapRow struct {
	ShopName string `json:"shopName"`
	Channel  string `json:"channel"`
	Platform string `json:"platform"`
	Mapped   bool   `json:"mapped"`   // 是否已配渠道/平台(前端区分未归类店铺)
	CateName string `json:"cateName"` // 吉客云渠道分类(cate_name), 给业务填渠道/平台时参考
}

// GetChannelMap GET /api/supply-chain/channel-map
// 列出吉客云全部店铺(sales_channel.channel_name)+ 已有映射: 已配的带出渠道/平台, 没配的空着等业务选。
func (h *DashboardHandler) GetChannelMap(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT sc.channel_name,
		        IFNULL(m.channel,''), IFNULL(m.platform,''),
		        IF(m.shop_name IS NULL, 0, 1) AS mapped,
		        IFNULL(sc.cate_name,'') AS cate_name
		 FROM (SELECT channel_name, MAX(cate_name) AS cate_name FROM sales_channel WHERE channel_name<>'' GROUP BY channel_name) sc
		 LEFT JOIN dim_sales_channel_map m ON m.shop_name=sc.channel_name
		 UNION
		 SELECT m2.shop_name, m2.channel, m2.platform, 1, ''
		 FROM dim_sales_channel_map m2
		 WHERE m2.shop_name NOT IN (SELECT channel_name FROM sales_channel WHERE channel_name<>'')`)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	defer rows.Close()
	var list []channelMapRow
	seen := map[string]bool{}
	for rows.Next() {
		var m channelMapRow
		var mapped int
		if err := rows.Scan(&m.ShopName, &m.Channel, &m.Platform, &mapped, &m.CateName); err != nil {
			writeError(w, http.StatusInternalServerError, "读取失败")
			return
		}
		if seen[m.ShopName] {
			continue
		}
		seen[m.ShopName] = true
		m.Mapped = mapped == 1
		list = append(list, m)
	}
	// 排序: 先未配的(等业务处理), 再按 平台→渠道→店铺
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Mapped != list[j].Mapped {
			return !list[i].Mapped // 未配的排前面
		}
		oi, oj := platformOrder[list[i].Platform], platformOrder[list[j].Platform]
		if oi != oj {
			return oi < oj
		}
		if list[i].Channel != list[j].Channel {
			return list[i].Channel < list[j].Channel
		}
		return list[i].ShopName < list[j].ShopName
	})
	writeJSON(w, map[string]interface{}{"list": list})
}

// SaveChannelMap POST /api/supply-chain/channel-map/save — 批量 upsert(店铺为主键)
// body: {rows: [{shopName, channel, platform}, ...]}
func (h *DashboardHandler) SaveChannelMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Rows []channelMapRow `json:"rows"`
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
	// 先全部校验(全有或全无, 防写一半)
	for i, m := range req.Rows {
		if err := validateChannelMapRow(m.ShopName, m.Channel, m.Platform); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("第 %d 行: %v", i+1, err))
			return
		}
	}
	for _, m := range req.Rows {
		if _, err := h.DB.ExecContext(r.Context(),
			`INSERT INTO dim_sales_channel_map(shop_name, channel, platform) VALUES(?,?,?)
			 ON DUPLICATE KEY UPDATE channel=VALUES(channel), platform=VALUES(platform)`,
			strings.TrimSpace(m.ShopName), strings.TrimSpace(m.Channel), strings.TrimSpace(m.Platform),
		); writeDatabaseError(w, err) {
			return
		}
	}
	writeJSON(w, map[string]interface{}{"saved": len(req.Rows)})
}

// DeleteChannelMap POST /api/supply-chain/channel-map/delete — 删一行
// body: {shopName}
func (h *DashboardHandler) DeleteChannelMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		ShopName string `json:"shopName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if strings.TrimSpace(req.ShopName) == "" {
		writeError(w, http.StatusBadRequest, "店铺名不能为空")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM dim_sales_channel_map WHERE shop_name=?`, strings.TrimSpace(req.ShopName),
	); writeDatabaseError(w, err) {
		return
	}
	writeJSON(w, map[string]interface{}{"deleted": req.ShopName})
}
