package ocr

import "testing"

func TestHasForeignCurrencyMarker(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"超优汇率 1美元 = 6.805人民币(黄金会员汇率)", true}, // B26003947 实例
		{"Pockyt Shop 订单金额 884.65元(130.00美元)", true},
		{"HK$ 980.00 港币 支付成功", true},
		{"Google Play USD 19.99", true},
		{"€ 49.00 EUR", true},
		{"微信支付 -529.00 交易成功", false}, // 纯人民币付款截图
		{"实付款 ¥175.91 交易成功", false},   // 人民币符号不算外币
		{"报销说明: 办公用品采购", false},
		{"App Store 余额 $0.00 可用", false},  // 收紧(二审): 裸$不再算(国内英文App界面/¥被OCR认成$ 常见)
		{"今日汇率 6.8051 招商银行", false},     // 收紧(二审): 汇率单独不再算(银行App首页chrome, 无币种名)
		{"账户充值 € 到账成功", false},         // 收紧(二审): 裸€/£符号不再算
	}
	for _, c := range cases {
		if got := HasForeignCurrencyMarker(c.in); got != c.want {
			t.Errorf("HasForeignCurrencyMarker(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
