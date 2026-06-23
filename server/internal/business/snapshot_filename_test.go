package business

import "testing"

func TestParseSnapshotFromFilename(t *testing.T) {
	cases := []struct {
		name             string
		wantY, wantM int
	}{
		{"2026年04月业务预决算报表.xlsx", 2026, 4},
		{"2026年4月业务预决算报表.xlsx", 2026, 4},
		{"2025年12月业务预决算报表.xlsx", 2025, 12},
		{"/tmp/upload-123-2026年04月业务预决算报表.xlsx", 2026, 4},
		{"2026年业务预决算报表.xlsx", 0, 0}, // 无月份 → 0,0
		{"乱七八糟.xlsx", 0, 0},
	}
	for _, c := range cases {
		y, m := ParseSnapshotFromFilename(c.name)
		if y != c.wantY || m != c.wantM {
			t.Errorf("%s: got (%d,%d) want (%d,%d)", c.name, y, m, c.wantY, c.wantM)
		}
	}
}
