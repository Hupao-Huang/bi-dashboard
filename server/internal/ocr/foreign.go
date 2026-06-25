package ocr

import "strings"

// foreignCurrencyMarkers 外币/汇率标记 (跑哥 2026-06-25 确认): OCR 转写文本命中任一即判该截图为国外交易。
var foreignCurrencyMarkers = []string{
	"汇率",
	"美元", "USD", "港币", "HKD", "欧元", "EUR", "日元", "JPY", "英镑", "GBP",
	"$", "€", "£",
}

// HasForeignCurrencyMarker 判定 OCR 转写文本里是否出现外币或汇率标记。
// 用于规则10"外币(国外)无票"豁免: 国外报销无中国发票, 凭境外付款截图。
func HasForeignCurrencyMarker(text string) bool {
	for _, m := range foreignCurrencyMarkers {
		if strings.Contains(text, m) {
			return true
		}
	}
	return false
}
