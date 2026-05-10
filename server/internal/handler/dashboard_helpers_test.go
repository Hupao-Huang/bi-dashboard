package handler

// dashboard_helpers_test.go — pure function 单元测试
// 已 Read dashboard_helpers.go 全文 (167 行), 按每个 if/return 分支写 case.

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// === getOverviewTrendRange (4 个分支) ===

func TestGetOverviewTrendRange_FallbackWhenEmpty(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/overview", nil)
	s, e := getOverviewTrendRange(req, "2026-05-01", "2026-05-09")
	if s != "2026-05-01" || e != "2026-05-09" {
		t.Errorf("空 query trendStart/trendEnd 应 fallback start/end, got %s~%s", s, e)
	}
}

func TestGetOverviewTrendRange_FallbackWhenInvalid(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/overview?trendStart=not-a-date&trendEnd=2026-05-10", nil)
	s, e := getOverviewTrendRange(req, "fallback-s", "fallback-e")
	if s != "fallback-s" || e != "fallback-e" {
		t.Errorf("invalid date 应 fallback, got %s~%s", s, e)
	}
}

func TestGetOverviewTrendRange_FallbackWhenStartGTEnd(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/overview?trendStart=2026-05-10&trendEnd=2026-05-01", nil)
	s, e := getOverviewTrendRange(req, "fallback-s", "fallback-e")
	if s != "fallback-s" || e != "fallback-e" {
		t.Errorf("trendStart > trendEnd 应 fallback, got %s~%s", s, e)
	}
}

func TestGetOverviewTrendRange_UseValidQuery(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/overview?trendStart=2026-04-01&trendEnd=2026-05-01", nil)
	s, e := getOverviewTrendRange(req, "fallback-s", "fallback-e")
	if s != "2026-04-01" || e != "2026-05-01" {
		t.Errorf("有效 trend 范围应使用, got %s~%s", s, e)
	}
}

// === isPlatformAllowedForDept (whitelist 语义) ===

func TestIsPlatformAllowedForDept(t *testing.T) {
	cases := []struct {
		dept, platform string
		want           bool
	}{
		// social 部门白名单
		{"social", "douyin", true},
		{"social", "kuaishou", true},
		{"social", "xiaohongshu", true},
		{"social", "shipinhao", true},
		{"social", "youzan", true},
		{"social", "weidian", true},

		// social 部门白名单外
		{"social", "tmall", false},
		{"social", "jd", false},
		{"social", "pdd", false},

		// 其他部门没有白名单 (无限制)
		{"ecommerce", "tmall", true},
		{"ecommerce", "jd", true},
		{"distribution", "anything", true},
		{"offline", "whatever", true},
	}
	for _, c := range cases {
		if got := isPlatformAllowedForDept(c.dept, c.platform); got != c.want {
			t.Errorf("isPlatformAllowedForDept(%q,%q)=%v, want %v", c.dept, c.platform, got, c.want)
		}
	}
}

// === buildPlatformCond (复杂业务逻辑, 多分支) ===

func TestBuildPlatformCond_AllOrEmptyReturnsEmpty(t *testing.T) {
	for _, p := range []string{"", "all"} {
		cond, args := buildPlatformCond("ecommerce", p)
		if cond != "" || args != nil {
			t.Errorf("platform=%q 应返空, got cond=%q args=%v", p, cond, args)
		}
	}
}

func TestBuildPlatformCond_BlockedDeptReturns1eq0(t *testing.T) {
	// social 部门不允许 tmall, 应返 " AND 1=0"
	cond, args := buildPlatformCond("social", "tmall")
	if cond != " AND 1=0" {
		t.Errorf("social 不允许 tmall 应返 ' AND 1=0', got %q", cond)
	}
	if args != nil {
		t.Errorf("拒绝时 args 应 nil, got %v", args)
	}
}

func TestBuildPlatformCond_InstantUsesShopNameLike(t *testing.T) {
	cond, _ := buildPlatformCond("ecommerce", "instant")
	if !strings.Contains(cond, "shop_name LIKE '%即时零售%'") {
		t.Errorf("instant 应 LIKE '%%即时零售%%', got %q", cond)
	}
}

func TestBuildPlatformCond_TmallReturnsSubquery(t *testing.T) {
	cond, args := buildPlatformCond("ecommerce", "tmall")
	if !strings.Contains(cond, "shop_name IN (SELECT channel_name FROM sales_channel") {
		t.Errorf("tmall 应返子查询, got %q", cond)
	}
	if !strings.Contains(cond, "online_plat_name IN (?") {
		t.Errorf("tmall 应有 plat IN (?), got %q", cond)
	}
	// args 头是 dept, 后跟 plat 名列表
	if len(args) < 2 {
		t.Fatalf("tmall args 应 ≥ 2, got %v", args)
	}
	if args[0] != "ecommerce" {
		t.Errorf("args[0] 应 dept='ecommerce', got %v", args[0])
	}
	if args[1] != "天猫商城" {
		t.Errorf("args[1] 应 '天猫商城', got %v", args[1])
	}
}

func TestBuildPlatformCond_VipMultipleNames(t *testing.T) {
	// vip 映射 2 个名字: 唯品会MP / 唯品会JIT
	cond, args := buildPlatformCond("ecommerce", "vip")
	if !strings.Contains(cond, "?,?") {
		t.Errorf("vip 应 2 个 placeholder, got %q", cond)
	}
	// args = [dept, "唯品会MP", "唯品会JIT"]
	if len(args) != 3 {
		t.Fatalf("vip args 应 3 个 (dept + 2 plats), got %d %v", len(args), args)
	}
	plats := []string{args[1].(string), args[2].(string)}
	hasMP, hasJIT := false, false
	for _, p := range plats {
		if p == "唯品会MP" {
			hasMP = true
		}
		if p == "唯品会JIT" {
			hasJIT = true
		}
	}
	if !hasMP || !hasJIT {
		t.Errorf("vip 应同时映射 唯品会MP + 唯品会JIT, got %v", plats)
	}
}

func TestBuildPlatformCond_UnknownPlatformReturnsEmpty(t *testing.T) {
	cond, args := buildPlatformCond("ecommerce", "unknown_platform_xyz")
	if cond != "" || args != nil {
		t.Errorf("未知 platform 应返空, got cond=%q args=%v", cond, args)
	}
}

// === joinStrings ===

func TestJoinStrings(t *testing.T) {
	cases := []struct {
		ss   []string
		sep  string
		want string
	}{
		{[]string{}, ",", ""},
		{[]string{"a"}, ",", "a"},
		{[]string{"a", "b", "c"}, ",", "a,b,c"},
		{[]string{"x", "y"}, " AND ", "x AND y"},
	}
	for _, c := range cases {
		if got := joinStrings(c.ss, c.sep); got != c.want {
			t.Errorf("joinStrings(%v,%q)=%q, want %q", c.ss, c.sep, got, c.want)
		}
	}
}

// === platformToPlats 映射数据完整性 (memory 视频号坑防御) ===

func TestPlatformToPlatsHasShipinhao(t *testing.T) {
	plats, ok := platformToPlats["shipinhao"]
	if !ok {
		t.Fatal("platformToPlats 必须有 'shipinhao' (视频号) 映射")
	}
	if len(plats) != 1 || plats[0] != "微信视频号小店" {
		t.Errorf("shipinhao 应映射 ['微信视频号小店'], got %v", plats)
	}
}

func TestPlatformToPlatsCoreChannels(t *testing.T) {
	expected := map[string][]string{
		"tmall":    {"天猫商城"},
		"tmall_cs": {"天猫超市"},
		"jd":       {"京东"},
		"pdd":      {"拼多多"},
		"vip":      {"唯品会MP", "唯品会JIT"},
		"taobao":   {"淘宝"},
	}
	for plat, want := range expected {
		got, ok := platformToPlats[plat]
		if !ok {
			t.Errorf("缺失 platform=%s", plat)
			continue
		}
		if len(got) != len(want) {
			t.Errorf("%s 映射数量错: got %d want %d", plat, len(got), len(want))
		}
	}
}
