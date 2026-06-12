package importutil

import (
	"log"
	"strconv"
	"strings"
)

// ParseFloat 解析字符串到 float64, 自动去 逗号/百分号/人民币符号(¥￥)/"元"/空白, 失败返 0.
// 适用于 RPA Excel 单元格值通用解析. 注意: 百分号只剥不除100 (各平台口径以剥号后数值入库).
// v1.70.6: 真格式异常(非空非"-"但解析失败) 加日志, 区分"业务真 0" vs "数据污染"
// 2026-06-12 第三批: 收编各 import-* 工具的本地实现 — 原来 ¥ 只有抖音处理/"元"只有客服处理,
// 同一格式跨平台解析结果不同(其他平台静默返 0), 统一成超集后以前能解析的结果不变, 以前漏的能解出来
// junkReplacer 一次扫描剥掉全部非数字噪音字符 (比 5 连 ReplaceAll 少扫 4 遍, 导入是百万单元格级热路径)
var junkReplacer = strings.NewReplacer(",", "", "%", "", "¥", "", "￥", "", "元", "")

func ParseFloat(s string) float64 {
	raw := s
	s = junkReplacer.Replace(strings.TrimSpace(s))
	s = strings.TrimSpace(s)
	if s == "" || s == "-" || s == "--" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Printf("[importutil.ParseFloat] 格式异常返0: %q (清洗后 %q) err=%v", raw, s, err)
		return 0
	}
	return v
}

// ParseInt 解析字符串到 int, 清洗规则同 ParseFloat, 失败返 0.
// 2026-06-12: 改走 ParseFloat 再截断 — 原 Atoi 遇 "3.0" 这类小数字符串会失败返 0 (import-jd 的本地实现就是截断语义)
func ParseInt(s string) int {
	return int(ParseFloat(s))
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
