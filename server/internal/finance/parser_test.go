package finance

import "testing"

func TestLevel1CodeForName(t *testing.T) {
	cases := []struct {
		name    string
		want    string
		wantOk  bool
	}{
		{"GMV数据", "GMV_GROUP", true},
		{"财务数据", "FIN_GROUP", true},
		{" GMV数据 ", "GMV_GROUP", true}, // 自动 trim
		{"未知", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := Level1CodeForName(c.name)
		if got != c.want || ok != c.wantOk {
			t.Errorf("Level1CodeForName(%q) = (%q, %v), want (%q, %v)", c.name, got, ok, c.want, c.wantOk)
		}
	}
}

func TestLevel2CodeForName(t *testing.T) {
	cases := []struct {
		name    string
		want    string
		wantOk  bool
	}{
		// 精确匹配
		{"GMV合计", "GMV_TOTAL", true},
		{"售退", "RETURN", true},
		{"营业额合计", "REV_TOTAL", true},
		{"营业毛利", "PROFIT_GROSS", true},
		{"运营利润", "PROFIT_OP", true},
		{"利润总额", "PROFIT_TOTAL", true},
		{"营业利润", "PROFIT_TOTAL", true}, // alias
		{"税金及附加", "TAX_SURCHARGE", true},
		{"所得税费用", "TAX_INCOME", true},

		// HasPrefix 匹配
		{"一、营业收入", "REV_MAIN", true},
		{"一、营业收入(扩展)", "REV_MAIN", true},
		{"减：营业成本", "COST_MAIN", true},
		{"减：销售费用合计", "SALES_EXP", true},
		{"减：管理费用占比", "MGMT_EXP", true},
		{"减：研发费用", "RND_EXP", true},
		{"加：营业外收入", "NON_REV", true},
		{"减：营业外支出", "NON_EXP", true},
		{"其中：报废损失", "LOSS_SCRAP", true},
		{"二：净利润", "NET_PROFIT", true},
		{"二、净利润", "NET_PROFIT", true},
		{"补充数据 (增值税)", "VAT_EXTRA", true},

		// trim
		{"  GMV合计  ", "GMV_TOTAL", true},

		// 不匹配
		{"未知科目", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := Level2CodeForName(c.name)
		if got != c.want || ok != c.wantOk {
			t.Errorf("Level2CodeForName(%q) = (%q, %v), want (%q, %v)", c.name, got, ok, c.want, c.wantOk)
		}
	}
}

func TestLevel2Category(t *testing.T) {
	cases := map[string]string{
		"GMV_TOTAL":    "GMV",
		"RETURN":       "GMV",
		"REV_TOTAL":    "GMV",
		"REV_MAIN":     "收入",
		"COST_MAIN":    "成本",
		"PROFIT_GROSS": "毛利",
		"SALES_EXP":    "销售费用",
		"PROFIT_OP":    "运营利润",
		"MGMT_EXP":     "管理费用",
		"RND_EXP":      "研发费用",
		"PROFIT_TOTAL": "利润总额",
		"NON_REV":      "营业外",
		"NON_EXP":      "营业外",
		"LOSS_SCRAP":   "营业外",
		"TAX_SURCHARGE": "税费",
		"TAX_INCOME":   "税费",
		"VAT_EXTRA":    "税费",
		"NET_PROFIT":   "净利润",
		"UNKNOWN_CODE": "其他",
		"":             "其他",
	}
	for code, want := range cases {
		if got := Level2Category(code); got != want {
			t.Errorf("Level2Category(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestParseYearFromFilename(t *testing.T) {
	// 实际行为: 必须含 "YYYY年" 模式才匹配, 否则返回 0
	cases := map[string]int{
		"2026年财务报表.xlsx":              2026,
		"2025年12月财务报表.xlsx":          2025,
		"/path/to/2024年报表.xlsx":         2024,    // 取 basename, 路径里的年份不算
		"财务-2026.xlsx":                  0,        // 没"年"字, 不匹配
		"financial-report-2025-12.xlsx":  0,
		"":                              0,
		"2026年-2027年.xlsx":              2026,    // 多个匹配取第一个
	}
	for filename, want := range cases {
		if got := ParseYearFromFilename(filename); got != want {
			t.Errorf("ParseYearFromFilename(%q) = %d, want %d", filename, got, want)
		}
	}
}
