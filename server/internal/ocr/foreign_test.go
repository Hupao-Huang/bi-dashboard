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
	}
	for _, c := range cases {
		if got := HasForeignCurrencyMarker(c.in); got != c.want {
			t.Errorf("HasForeignCurrencyMarker(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
