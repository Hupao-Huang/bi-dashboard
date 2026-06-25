package ocr

import "strings"

// foreignCurrencyMarkers 外币币种标记: OCR 转写文本命中任一即判该截图为国外交易。
// 二审收紧(2026-06-25): 只留**明确币种名**, 去掉裸符号 $/€/£(国内英文App界面常见 + OCR易把¥认成$)
// 和"汇率"单独(银行/支付宝首页 chrome 常显"今日汇率"与本笔消费无关)——这是自动通过路径, 关键词太松=无票单误放行。
var foreignCurrencyMarkers = []string{
	"美元", "USD", "港币", "HKD", "欧元", "EUR", "日元", "JPY", "英镑", "GBP",
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
