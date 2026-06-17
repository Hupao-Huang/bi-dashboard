package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
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

// asMs 要兼容合思明细行的数字/json.Number/字符串三种形态 (普票通行日期字段在不同链路类型不一)
func TestAsMs(t *testing.T) {
	const want = int64(1775059200000)
	cases := []struct {
		name string
		in   interface{}
		want int64
	}{
		{"float64", float64(1775059200000), want},
		{"jsonNumber", json.Number("1775059200000"), want},
		{"string整数", "1775059200000", want},
		{"string科学计数", "1.7750592e12", want},
		{"nil全电无字段", nil, 0},
		{"空串", "", 0},
		{"垃圾串", "abc", 0},
		{"map非法", map[string]interface{}{"x": 1}, 0},
	}
	for _, c := range cases {
		if got := asMs(c.in); got != c.want {
			t.Errorf("%s: asMs(%v)=%d, 期望 %d", c.name, c.in, got, c.want)
		}
	}
}

// pruneFlowInvoices 单据级删残留 (跑哥 2026-06-17: 驳回换发票后旧发票/整条明细被删的发票残留致误报)
func TestPruneFlowInvoices(t *testing.T) {
	// ① keys 空 → 保守不删, 不碰 db (nil 也不崩)
	pruneFlowInvoices(nil, "F1", nil)
	// ② flowId 空 → 不删
	pruneFlowInvoices(nil, "", [][2]string{{"D1", "inv1"}})

	// ③ keys 非空 → 删 (detail_id,invoice_id) 整对 NOT IN; 整条明细被删的发票也能清
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectExec(`DELETE FROM hesi_flow_invoice WHERE flow_id=\? AND \(detail_id, invoice_id\) NOT IN`).
		WithArgs("F1", "D1", "inv1", "D1", "inv2", "D2", "inv3").
		WillReturnResult(sqlmock.NewResult(0, 1))
	pruneFlowInvoices(db, "F1", [][2]string{{"D1", "inv1"}, {"D1", "inv2"}, {"D2", "inv3"}})
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("③非空keys应删整对NOT IN: %v", err)
	}
}

// retireGhostFlows: 合思删单/撤回到草稿后 getApplyList 不再返回, 靠 approveStates 报的
// "已删除/尚未提交"退场 active=0 (跑哥 2026-06-17)。锁死 denylist 只含这两个标记, 防误退真实待审节点。
func TestRetireGhostFlows(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	// 必须是 UPDATE active=0, 且 denylist 严格等于 已删除/尚未提交 ([\s\S]* 跨换行)
	mock.ExpectExec(`UPDATE hesi_flow SET active=0[\s\S]*current_stage_name IN \('已删除','尚未提交'\)`).
		WillReturnResult(sqlmock.NewResult(0, 34))
	n, err := retireGhostFlows(db)
	if err != nil {
		t.Fatalf("retireGhostFlows: %v", err)
	}
	if n != 34 {
		t.Errorf("应退场 34 单, got %d", n)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("退场幽灵单应执行 UPDATE active=0 且只退 已删除/尚未提交: %v", err)
	}
}

// mergeTollPass: 普票明细行取毫秒; 全电(无通行日期字段, nil)跳过; 同一发票多明细行取最早起
func TestMergeTollPass(t *testing.T) {
	out := map[string][2]int64{}
	mergeTollPass(out, []invDetailLine{
		{MasterID: "PU", PassStart: float64(1775059200000), PassEnd: float64(1776182400000)}, // 普票
		{MasterID: "QD", PassStart: nil, PassEnd: nil},                                        // 全电: 明细行无通行日期 → 跳过
		{MasterID: "", PassStart: float64(1775059200000)},                                     // 空 masterId → 跳过
		{MasterID: "MULTI", PassStart: float64(3000000000000), PassEnd: float64(3000000000001)},
		{MasterID: "MULTI", PassStart: float64(2000000000000), PassEnd: float64(2000000000001)}, // 更早, 应胜出
		{MasterID: "MULTI", PassStart: float64(5000000000000), PassEnd: float64(5000000000001)},
	})
	if p, ok := out["PU"]; !ok || p[0] != 1775059200000 || p[1] != 1776182400000 {
		t.Errorf("普票应解析出通行日期, got %v ok=%v", out["PU"], ok)
	}
	if _, ok := out["QD"]; ok {
		t.Errorf("全电票明细行无通行日期, 不应写入 (它走备注)")
	}
	if _, ok := out[""]; ok {
		t.Errorf("空 masterId 不应写入")
	}
	if p := out["MULTI"]; p[0] != 2000000000000 {
		t.Errorf("同一发票多明细行应取最早起, got %d 期望 2000000000000", p[0])
	}
}
