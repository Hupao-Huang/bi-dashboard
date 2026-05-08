package importutil

import (
	"strconv"
	"strings"
)

// ParseFloat 解析字符串到 float64, 自动去逗号/百分号/空白, 失败返 0.
// 适用于 RPA Excel 单元格值通用解析.
func ParseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "%", "")
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// ParseInt 解析字符串到 int, 自动去逗号/空白, 失败返 0.
func ParseInt(s string) int {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}
	v, _ := strconv.Atoi(s)
	return v
}

// CellStr 安全获取一行某列的 trim 后字符串值, 越界返 "".
func CellStr(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

// FormatDate YYYYMMDD → YYYY-MM-DD, 其他原样返回.
func FormatDate(dateStr string) string {
	if len(dateStr) == 8 {
		return dateStr[:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]
	}
	return dateStr
}
