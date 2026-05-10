package handler

// handler_pure_test.go — handler 包大量 pure 工具函数测试
// 已 Read 各源码 (line 引用见每段)

import (
	"database/sql"
	"testing"
)

// === business_report.go ===

// formatSnapshotLabel (line 287-293) — sy != y 时加"(覆盖 X 年)"
func TestFormatSnapshotLabel(t *testing.T) {
	cases := []struct {
		sy, sm, y int
		want      string
	}{
		{2026, 5, 2026, "2026-05"},
		{2026, 12, 2026, "2026-12"},
		{2026, 1, 2026, "2026-01"},
		{2026, 5, 2025, "2026-05 (覆盖 2025 年)"}, // sy != y
		{2025, 12, 2024, "2025-12 (覆盖 2024 年)"},
	}
	for _, c := range cases {
		if got := formatSnapshotLabel(c.sy, c.sm, c.y); got != c.want {
			t.Errorf("formatSnapshotLabel(%d,%d,%d)=%q want %q", c.sy, c.sm, c.y, got, c.want)
		}
	}
}

// zeroPad (line 295-301) — 单位补 0, 多位原样
func TestZeroPad(t *testing.T) {
	cases := map[int]string{
		0:   "00",
		1:   "01",
		9:   "09",
		10:  "10",
		99:  "99",
		100: "100", // > 2 位原样, 不会变 "0100"
	}
	for n, want := range cases {
		if got := zeroPad(n); got != want {
			t.Errorf("zeroPad(%d)=%q want %q", n, got, want)
		}
	}
}

// nullToPtr (line 303-309)
func TestNullToPtr(t *testing.T) {
	// Valid → 取 Float64
	v := nullToPtr(sql.NullFloat64{Float64: 12.5, Valid: true})
	if v == nil || *v != 12.5 {
		t.Errorf("Valid 应返指针, got %v", v)
	}
	// !Valid → nil
	if got := nullToPtr(sql.NullFloat64{Float64: 99.9, Valid: false}); got != nil {
		t.Error("!Valid 应返 nil")
	}
}

// splitCsv (line 762-772)
func TestSplitCsv(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b , c ", []string{"a", "b", "c"}}, // trim
		{"a,,b", []string{"a", "b"}},               // 跳空
		{"", []string{}},                            // 空 string → ""分割是 [""], trim 后跳过
		{",,,", []string{}},
		{"x", []string{"x"}},
	}
	for _, c := range cases {
		got := splitCsv(c.input)
		if len(got) != len(c.want) {
			t.Errorf("splitCsv(%q)=%v want %v", c.input, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitCsv(%q)[%d]=%q want %q", c.input, i, got[i], c.want[i])
			}
		}
	}
}

// === admin.go ===

// adminDeptLabel (line 114-119) — 字典查不到 fallback 原值
func TestAdminDeptLabel(t *testing.T) {
	cases := map[string]string{
		"ecommerce":      "电商部门", // 命中 adminDeptLabelMap
		"social":         "社媒部门",
		"distribution":   "分销部门",
		"instant_retail": "即时零售部",
		"finance":        "财务部门",
		"other":          "其他",       // v1.47.0 加
		"excluded":       "不计算销售",   // v1.47.0 加
		"unknown_xyz":    "unknown_xyz", // 未命中 fallback
		"":              "",
	}
	for input, want := range cases {
		if got := adminDeptLabel(input); got != want {
			t.Errorf("adminDeptLabel(%q)=%q want %q", input, got, want)
		}
	}
}

// parseAdminUserPath (line 501-519) — /api/admin/users/<id> 或 /api/admin/users/<id>/<sub>
func TestParseAdminUserPath(t *testing.T) {
	cases := []struct {
		path string
		wantID int64
		wantSub string
		wantOk bool
	}{
		// 命中, 单段
		{"/api/admin/users/123", 123, "", true},
		{"/api/admin/users/456/", 456, "", true}, // trailing slash trim
		// 命中, 双段
		{"/api/admin/users/789/permissions", 789, "permissions", true},
		// 不命中
		{"/api/admin/users", 0, "", false},      // 缺 id
		{"/api/admin/users/", 0, "", false},     // 空 id
		{"/api/admin/users/abc", 0, "", false},  // id 非数字
		{"/api/admin/roles/123", 0, "", false}, // 非 users prefix
		{"/wrong/prefix", 0, "", false},
		// 3 段以上 (源码 if len(parts) > 2 fail)
		{"/api/admin/users/123/x/y", 0, "", false},
	}
	for _, c := range cases {
		gotID, gotSub, gotOk := parseAdminUserPath(c.path)
		if gotID != c.wantID || gotSub != c.wantSub || gotOk != c.wantOk {
			t.Errorf("parseAdminUserPath(%q)=(%d,%q,%v) want (%d,%q,%v)",
				c.path, gotID, gotSub, gotOk, c.wantID, c.wantSub, c.wantOk)
		}
	}
}

// parseAdminRolePath (line 521-...) — /api/admin/roles/<id>
func TestParseAdminRolePath(t *testing.T) {
	cases := []struct {
		path string
		wantID int64
		wantOk bool
	}{
		{"/api/admin/roles/123", 123, true},
		{"/api/admin/roles/0", 0, true}, // 0 也是合法 int64
		{"/api/admin/roles", 0, false},
		{"/api/admin/roles/abc", 0, false},
		{"/wrong", 0, false},
	}
	for _, c := range cases {
		gotID, gotOk := parseAdminRolePath(c.path)
		if gotID != c.wantID || gotOk != c.wantOk {
			t.Errorf("parseAdminRolePath(%q)=(%d,%v) want (%d,%v)",
				c.path, gotID, gotOk, c.wantID, c.wantOk)
		}
	}
}
