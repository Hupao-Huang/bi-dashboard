package business

// parser_internal_test.go — business/parser.go unexported 函数单元测试
// 同 package 可以测私有函数. 已 Read parser.go line 216-360 实现.

import "testing"

// === isOfflineRegion (line 291-299) ===
// 源码: 11 个大区精确匹配 (华南/华东/华北/华中/西南/西北/东北/重客/山东/母婴/新零售)

func TestIsOfflineRegion(t *testing.T) {
	cases := map[string]bool{
		"华南":    true,
		"华东":    true,
		"华北":    true,
		"华中":    true,
		"西南":    true,
		"西北":    true,
		"东北":    true,
		"重客":    true,
		"山东":    true,
		"母婴":    true,
		"新零售":   true,
		" 华北 ":  true, // trim
		"":      false,
		"华北大区": false, // 必须精确, 不模糊
		"华北X":  false,
		"南华":   false,
		"国际零售": false,
	}
	for input, want := range cases {
		if got := isOfflineRegion(input); got != want {
			t.Errorf("isOfflineRegion(%q)=%v want %v", input, got, want)
		}
	}
}

// === normalizeChannel (line 282-289) ===
// 源码: contains "国际零售" → 返 "国际零售"; 否则原值 trim

func TestNormalizeChannel(t *testing.T) {
	cases := map[string]string{
		"国际零售业务": "国际零售", // 跑哥业务: 这种命名变体统一成短名
		"国际零售":   "国际零售",
		" 国际零售业务 ": "国际零售",
		"电商":     "电商",   // 普通渠道原样
		"分销":     "分销",
		" 社媒 ":   "社媒",   // trim
		"":       "",
	}
	for input, want := range cases {
		if got := normalizeChannel(input); got != want {
			t.Errorf("normalizeChannel(%q)=%q want %q", input, got, want)
		}
	}
}

// === stripLevel1Prefix (line 275-280) ===
// 源码: regex "^[0-9一二三四五六七八九十]+[、.]?\\s*(.+)$" 匹配则取捕获组, 否则原值 trim

func TestStripLevel1Prefix(t *testing.T) {
	cases := map[string]string{
		"1、电商":    "电商",
		"2、社媒":    "社媒",
		"3.线下":    "线下",  // 点号分隔
		"4 分销":    "分销",  // 空格分隔
		"5、 线下":   "线下",  // 多余空格 trim
		"一、电商":    "电商",  // 中文数字
		"二、社媒":    "社媒",
		"电商":      "电商",  // 没序号原样
		"":        "",
		" 电商 ":   "电商",  // trim
	}
	for input, want := range cases {
		if got := stripLevel1Prefix(input); got != want {
			t.Errorf("stripLevel1Prefix(%q)=%q want %q", input, got, want)
		}
	}
}

// === parseSheetName (line 216-270) ===
// 源码逻辑:
//   - "" → ("","",false)
//   - "总" → ("总","",true)
//   - "中后台合计" → ("中后台","",true)
//   - "5.X xxx" → ("线下", "xxx", true)
//   - "X、xxx" 一级渠道 → (规范化 channel, "", true)
//   - 含"国际零售" → ("国际零售","",true)
//   - "X-Y" 或 "X—Y" 二级 → (parent, child, true)
//   - 大区名 → ("线下", "X", true)
//   - 都不匹配 → ("","",false)

func TestParseSheetName(t *testing.T) {
	cases := []struct {
		input         string
		wantChannel   string
		wantSubChan   string
		wantOk        bool
	}{
		// 总 / 中后台
		{"总", "总", "", true},
		{"中后台合计", "中后台", "", true},

		// 一级渠道 (含 normalizeChannel "国际零售业务"→"国际零售")
		{"1、电商", "电商", "", true},
		{"2、社媒", "社媒", "", true},
		{"3、线下", "线下", "", true},
		{"4、分销", "分销", "", true},
		{"6、国际零售业务", "国际零售", "", true},

		// 5.X 子项归线下
		{"5.1 直营", "线下", "直营", true},
		{"5.2 经销", "线下", "经销", true},

		// 二级 (parent-child)
		{"电商-TOC", "电商", "TOC", true},
		{"分销-礼品", "分销", "礼品", true},
		{"社媒-小红书", "社媒", "小红书", true},
		{"社媒—视频号", "社媒", "视频号", true}, // em dash

		// 大区
		{"华北", "线下", "华北", true},
		{"华东", "线下", "华东", true},
		{"重客", "线下", "重客", true},

		// 不识别
		{"", "", "", false},
		{"未知 sheet", "", "", false},
	}
	for _, c := range cases {
		gotChan, gotSub, gotOk := parseSheetName(c.input)
		if gotChan != c.wantChannel || gotSub != c.wantSubChan || gotOk != c.wantOk {
			t.Errorf("parseSheetName(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.input, gotChan, gotSub, gotOk, c.wantChannel, c.wantSubChan, c.wantOk)
		}
	}
}
