package main

import "testing"

func TestParseCommentDate(t *testing.T) {
	cases := map[string]string{
		"2026年06月07日 12:39:34": "2026-06-07", // 抖音文本格式
		"2026-06-07 13:40:07":   "2026-06-07", // 标准 datetime 文本
		"2026/6/7 13:40:07":     "2026-06-07", // 斜杠 + 单位数月日
		"2026-06-07":            "2026-06-07", // 纯日期
		"2026年6月7日":             "2026-06-07", // 年月日无时间无前导零
		"6/7/26 13:40":          "2026-06-07", // 拼多多 excelize 渲染的美式 M/D/YY
		"6/7/26 0:47":           "2026-06-07", // M/D/YY 单位数小时
		"初次评价: 2026-06-05 22:53": "2026-06-05", // 天猫带「初次评价: 」前缀
		"初次评价：2026/6/5":          "2026-06-05", // 天猫前缀变体
		"":                      "",
		"   ":                   "",
		"乱七八糟":                  "",
	}
	for in, want := range cases {
		if got := parseCommentDate(in); got != want {
			t.Errorf("parseCommentDate(%q)=%q 期望 %q", in, got, want)
		}
	}
}

func TestExtractOrderNo(t *testing.T) {
	cases := map[string]string{
		"订单编号：260527-277872699724055": "260527-277872699724055", // 拼多多中文冒号前缀
		"6953400731356894751":          "6953400731356894751",   // 抖音纯数字无前缀
		"订单编号:123456":                  "123456",                // 英文冒号
		"  订单编号：  789  ":              "789",                   // 带空格
		"":                             "",
	}
	for in, want := range cases {
		if got := extractOrderNo(in); got != want {
			t.Errorf("extractOrderNo(%q)=%q 期望 %q", in, got, want)
		}
	}
}

func TestParseScore(t *testing.T) {
	if parseScore("1") != 1 {
		t.Error("评分 1 应为 1")
	}
	if parseScore("5") != 5 {
		t.Error("评分 5 应为 5")
	}
	if parseScore("") != -1 {
		t.Error("空评分应为 -1(NULL)")
	}
	if parseScore("x") != -1 {
		t.Error("非数字评分应为 -1")
	}
}

func TestContentHashStable(t *testing.T) {
	a := contentHash("抖音", "店A", "订单编号：123", "酱油", "好评")
	b := contentHash("抖音", "店A", "订单编号：123", "酱油", "好评")
	if a != b {
		t.Error("相同输入 hash 应稳定(幂等去重靠这个)")
	}
	c := contentHash("抖音", "店A", "订单编号：123", "酱油", "差评")
	if a == c {
		t.Error("不同评价内容 hash 应不同")
	}
	if len(a) != 32 {
		t.Errorf("md5 hex 应 32 位, got %d", len(a))
	}
}
