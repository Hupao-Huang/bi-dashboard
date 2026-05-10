package handler

// scope_test.go — 数据范围相关 pure 函数
// 已 Read scope.go (line 11-224) + auth.go (1598-1622): uniqueSortedStrings/containsString

import (
	"strings"
	"testing"
)

// === uniqueSortedStrings (auth.go line 1598-1613) ===
// 跳过空 + 去重 + sort

func TestUniqueSortedStrings(t *testing.T) {
	cases := []struct {
		input []string
		want  []string
	}{
		{[]string{"b", "a", "c"}, []string{"a", "b", "c"}},
		{[]string{"a", "a", "b"}, []string{"a", "b"}},     // 去重
		{[]string{"", "a", "", "b", ""}, []string{"a", "b"}}, // 跳空
		{[]string{}, []string{}},
		{[]string{""}, []string{}},
	}
	for _, c := range cases {
		got := uniqueSortedStrings(c.input)
		if len(got) != len(c.want) {
			t.Errorf("uniqueSortedStrings(%v): len=%d want %d", c.input, len(got), len(c.want))
			continue
		}
		for i, v := range got {
			if v != c.want[i] {
				t.Errorf("uniqueSortedStrings(%v)[%d]=%q want %q", c.input, i, v, c.want[i])
			}
		}
	}
}

// === containsString (auth.go line 1615-1622) ===

func TestContainsString(t *testing.T) {
	if !containsString([]string{"a", "b", "c"}, "b") {
		t.Error("'b' 应在 [a,b,c] 中")
	}
	if containsString([]string{"a", "b"}, "x") {
		t.Error("'x' 不在 [a,b]")
	}
	if containsString([]string{}, "any") {
		t.Error("空 slice 一定不含")
	}
	if containsString(nil, "any") {
		t.Error("nil slice 一定不含")
	}
	// 大小写敏感
	if containsString([]string{"abc"}, "ABC") {
		t.Error("应区分大小写")
	}
}

// === withAlias (scope.go line 50-57) ===
// alias='' 不变; 否则把 'shop_name' / 'department' 加前缀

func TestWithAlias(t *testing.T) {
	cases := []struct {
		scopeCond, alias, want string
	}{
		// alias='' 不变 (源码 line 51)
		{"", "", ""},
		{" AND shop_name = ?", "", " AND shop_name = ?"},
		// 替换 shop_name
		{" AND shop_name = ?", "s", " AND s.shop_name = ?"},
		// 替换 department
		{" AND department IN (?,?)", "s", " AND s.department IN (?,?)"},
		// 同时含两者
		{" AND shop_name = ? AND department = ?", "t", " AND t.shop_name = ? AND t.department = ?"},
		// 空 scopeCond 不变 (源码 line 51 短路)
		{"", "s", ""},
	}
	for _, c := range cases {
		if got := withAlias(c.scopeCond, c.alias); got != c.want {
			t.Errorf("withAlias(%q,%q)=%q want %q", c.scopeCond, c.alias, got, c.want)
		}
	}
}

// === toInterfaceSlice (line 195-201) ===

func TestToInterfaceSlice(t *testing.T) {
	got := toInterfaceSlice([]string{"a", "b", "c"})
	if len(got) != 3 {
		t.Fatalf("len 应 3, got %d", len(got))
	}
	for i, v := range []string{"a", "b", "c"} {
		if got[i].(string) != v {
			t.Errorf("got[%d]=%v want %s", i, got[i], v)
		}
	}
	// 空
	if got := toInterfaceSlice(nil); len(got) != 0 {
		t.Errorf("nil 应空, got %v", got)
	}
}

// === buildPlatformCondForKeys (line 11-44) ===
// 多平台拼 OR 子句, "instant" 单独 LIKE

func TestBuildPlatformCondForKeys_Empty(t *testing.T) {
	cond, args := buildPlatformCondForKeys(nil, "shop_name")
	if cond != "" || args != nil {
		t.Errorf("空 keys 应返空, got %q %v", cond, args)
	}
}

func TestBuildPlatformCondForKeys_SinglePlatform(t *testing.T) {
	cond, args := buildPlatformCondForKeys([]string{"tmall"}, "shop_name")
	if !strings.Contains(cond, "shop_name IN (SELECT channel_name FROM sales_channel") {
		t.Errorf("应有子查询, got %q", cond)
	}
	if !strings.Contains(cond, "online_plat_name IN") {
		t.Errorf("应有 online_plat_name IN, got %q", cond)
	}
	if len(args) < 1 {
		t.Errorf("args 应非空, got %v", args)
	}
}

func TestBuildPlatformCondForKeys_InstantStandalone(t *testing.T) {
	cond, _ := buildPlatformCondForKeys([]string{"instant"}, "shop_name")
	if !strings.Contains(cond, "shop_name LIKE '%即时零售%'") {
		t.Errorf("instant 应单独 LIKE, got %q", cond)
	}
}

func TestBuildPlatformCondForKeys_MixedInstantAndOthers(t *testing.T) {
	cond, args := buildPlatformCondForKeys([]string{"tmall", "instant"}, "shop_name")
	// 必含 OR 拼接
	if !strings.Contains(cond, " OR ") {
		t.Errorf("mixed 应用 OR 拼接, got %q", cond)
	}
	// 含子查询 (tmall) + LIKE (instant)
	if !strings.Contains(cond, "channel_name") {
		t.Errorf("tmall 应走子查询, got %q", cond)
	}
	if !strings.Contains(cond, "LIKE '%即时零售%'") {
		t.Errorf("instant 应走 LIKE, got %q", cond)
	}
	// args 仅含 tmall 的, instant 不带 args
	if len(args) != 1 {
		t.Errorf("args 应只含 tmall '天猫商城', got %v", args)
	}
}
