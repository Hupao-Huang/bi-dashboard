package handler

import "sort"

// ChannelRow 渠道汇总行(平台/渠道维度)
type ChannelRow struct {
	Platform string  `json:"platform"`
	Channel  string  `json:"channel"`
	Orders   int     `json:"orders"`   // 发货单数
	Bottles  float64 `json:"bottles"`  // 单瓶数(sell_count×箱规)
	WeightKg float64 `json:"weightKg"` // 重量(kg)
}

// GoodsRow TOP10 单品行
type GoodsRow struct {
	GoodsNo   string  `json:"goodsNo"`
	GoodsName string  `json:"goodsName"`
	Orders    int     `json:"orders"`
	Bottles   float64 `json:"bottles"`
	Boxes     float64 `json:"boxes"`   // 箱数=Σsell_count
	Pallets   float64 `json:"pallets"` // 托数=箱数÷托规
}

// ComboRow TOP10 货品组合行(订单整篮子)
type ComboRow struct {
	Display  string  `json:"display"`
	Orders   int     `json:"orders"`
	Bottles  float64 `json:"bottles"`
	WeightKg float64 `json:"weightKg"`
}

// ratio 占比(total=0 返 0,防 #DIV/0!)
func ratio(part, total float64) float64 {
	if total == 0 {
		return 0
	}
	return part / total
}

// perOrder 单均(orders=0 返 0)
func perOrder(v float64, orders int) float64 {
	if orders == 0 {
		return 0
	}
	return v / float64(orders)
}

// palletsOf 托数=箱数÷托规(无托规[≤0]返 0)
func palletsOf(boxes, palletBoxQty float64) float64 {
	if palletBoxQty <= 0 {
		return 0
	}
	return boxes / palletBoxQty
}

var platformOrder = map[string]int{"社媒": 0, "电商": 1, "其他": 2}

// rollupPlatforms 渠道明细行 → 每平台前插「X合计」,末尾加「总计」;
// 平台按 社媒/电商/其他 排,平台内按 Bottles 降序
func rollupPlatforms(rows []ChannelRow) []ChannelRow {
	sort.SliceStable(rows, func(i, j int) bool {
		oi, oj := platformOrder[rows[i].Platform], platformOrder[rows[j].Platform]
		if oi != oj {
			return oi < oj
		}
		return rows[i].Bottles > rows[j].Bottles
	})
	var out []ChannelRow
	grand := ChannelRow{Channel: "总计"}
	i := 0
	for i < len(rows) {
		p := rows[i].Platform
		sum := ChannelRow{Platform: p, Channel: p + "合计"}
		j := i
		for j < len(rows) && rows[j].Platform == p {
			sum.Orders += rows[j].Orders
			sum.Bottles += rows[j].Bottles
			sum.WeightKg += rows[j].WeightKg
			j++
		}
		out = append(out, sum)
		out = append(out, rows[i:j]...)
		grand.Orders += sum.Orders
		grand.Bottles += sum.Bottles
		grand.WeightKg += sum.WeightKg
		i = j
	}
	out = append(out, grand)
	return out
}
