package importutil

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseExcelDate 严格解析 Excel 日期列, 兼容多种格式.
// 支持: YYYY-MM-DD / YYYY/MM/DD / YYYY.MM.DD / YYYY年MM月DD日 / YYYYMMDD / YYYY-M-D
// 同时兼容拼多多美式短年份 MM-DD-YY (例: "12-29-25" = 2025-12-29)
// 格式不合规返回 "" (调用方 fallback 到文件名日期).
//
// YY 补全规则: 00-69 → 20xx, 70-99 → 19xx (拼多多场景)
func ParseExcelDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if idx := strings.Index(s, " "); idx > 0 {
		s = s[:idx]
	}
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, "年", "-")
	s = strings.ReplaceAll(s, "月", "-")
	s = strings.ReplaceAll(s, "日", "")

	// YYYYMMDD (无分隔符)
	if len(s) == 8 && !strings.Contains(s, "-") {
		return s[:4] + "-" + s[4:6] + "-" + s[6:8]
	}
	parts := strings.Split(s, "-")
	if len(parts) != 3 {
		return ""
	}
	// 情况 A: YYYY-MM-DD (4 位年份, 标准 ISO)
	if len(parts[0]) == 4 {
		y, m, d := parts[0], parts[1], parts[2]
		if len(m) == 1 {
			m = "0" + m
		}
		if len(d) == 1 {
			d = "0" + d
		}
		if len(m) != 2 || len(d) != 2 {
			return ""
		}
		return y + "-" + m + "-" + d
	}
	// 情况 B: MM-DD-YY (拼多多美式短年份)
	// 识别条件: 三段都是 1-2 位, parts[0] ≤ 12, parts[2] 是 YY
	if len(parts[0]) <= 2 && len(parts[1]) <= 2 && len(parts[2]) == 2 {
		mNum, e1 := strconv.Atoi(parts[0])
		dNum, e2 := strconv.Atoi(parts[1])
		yyNum, e3 := strconv.Atoi(parts[2])
		if e1 != nil || e2 != nil || e3 != nil {
			return ""
		}
		if mNum < 1 || mNum > 12 || dNum < 1 || dNum > 31 {
			return ""
		}
		year := 2000 + yyNum
		if yyNum >= 70 {
			year = 1900 + yyNum
		}
		return fmt.Sprintf("%04d-%02d-%02d", year, mNum, dNum)
	}
	return ""
}
