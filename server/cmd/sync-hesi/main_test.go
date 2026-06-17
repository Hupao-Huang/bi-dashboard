package main

import (
	"testing"
	"time"
)

func TestParseTollPassDates(t *testing.T) {
	// 真实过路费(ETC全电)发票备注格式
	remark := "车牌号：鲁V7BC76\n车辆类型：客车\n通行日期起/止：2026-05-24 11:25:11/2026-05-24 11:48:07\n入/出口站：山东潍坊西站-山东青州西站（不可用于增值税进项抵扣）"
	start, end := parseTollPassDates(remark)
	if start == 0 || end == 0 {
		t.Fatalf("应解析出通行起止, got start=%d end=%d", start, end)
	}
	if got := time.UnixMilli(start).Format("2006-01-02 15:04:05"); got != "2026-05-24 11:25:11" {
		t.Errorf("通行起始解析错: %s", got)
	}
	if got := time.UnixMilli(end).Format("2006-01-02 15:04:05"); got != "2026-05-24 11:48:07" {
		t.Errorf("通行结束解析错: %s", got)
	}
}

func TestParseTollPassDatesNonToll(t *testing.T) {
	// 非过路费 / 无通行字段 / 空 → 0,0 (不误填)
	for _, s := range []string{"普通发票备注 无通行信息", "", "车牌号：鲁A12345"} {
		if a, b := parseTollPassDates(s); a != 0 || b != 0 {
			t.Errorf("无通行日期应返回 0,0, 输入 %q got %d/%d", s, a, b)
		}
	}
}
