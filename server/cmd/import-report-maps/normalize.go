package main

import "strings"

// isAllDigits 是否全为数字(空串返 false)
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// normalizeGoodsNo 纯数字且不足 8 位则左补 0(Excel 吃掉前导 0),其余 trim 后原样
func normalizeGoodsNo(raw string) string {
	s := strings.TrimSpace(raw)
	if isAllDigits(s) && len(s) < 8 {
		return strings.Repeat("0", 8-len(s)) + s
	}
	return s
}
