package main

import "testing"

func TestNormalizeGoodsNo(t *testing.T) {
	cases := map[string]string{
		"3010082":     "03010082", // 数字被 Excel 吃了前导 0 → 补回 8 位
		"03010082":    "03010082",
		"S3-02-0003":  "S3-02-0003", // 非纯数字原样
		"sxxtwl-480g": "sxxtwl-480g",
		" 3010137 ":   "03010137", // trim + 补 0
		"123456789":   "123456789", // 已 ≥8 位不动
	}
	for in, want := range cases {
		if got := normalizeGoodsNo(in); got != want {
			t.Errorf("normalizeGoodsNo(%q)=%q want %q", in, got, want)
		}
	}
}
