package importutil

import (
	"log"
	"strconv"
	"strings"
)

// ParseFloat 解析字符串到 float64, 自动去逗号/百分号/空白, 失败返 0.
// 适用于 RPA Excel 单元格值通用解析.
// v1.70.6: 真格式异常(非空非"-"但解析失败) 加日志, 区分"业务真 0" vs "数据污染"
func ParseFloat(s string) float64 {
	raw := s
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "%", "")
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Printf("[importutil.ParseFloat] 格式异常返0: %q (清洗后 %q) err=%v", raw, s, err)
		return 0
	}
	return v
}

// ParseInt 解析字符串到 int, 自动去逗号/空白, 失败返 0.
// v1.70.6: 同 ParseFloat, 格式异常加日志
func ParseInt(s string) int {
	raw := s
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		log.Printf("[importutil.ParseInt] 格式异常返0: %q (清洗后 %q) err=%v", raw, s, err)
		return 0
	}
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
