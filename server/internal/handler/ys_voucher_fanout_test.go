package handler

import (
	"testing"
)

func TestFlattenVoucherRecordsTagsAccbook(t *testing.T) {
	recs := []map[string]interface{}{
		{
			"header": map[string]interface{}{
				"id":              "1234567890123456789",
				"period":          "2026-06",
				"displayname":     "转-1",
				"vouchertype":     map[string]interface{}{"name": "转账凭证"},
				"description":     "计提工资",
				"totaldebit_org":  float64(1000),
				"totalcredit_org": float64(1000),
				"srcsystem":       "总账",
				"maker":           map[string]interface{}{"name": "张三"},
				"voucherstatus":   "04",
				"maketime":        "2026-06-30",
				"attachedbill":    "2",
			},
			"body": []interface{}{
				map[string]interface{}{
					"recordnumber":   "1",
					"description":    "计提工资",
					"accsubject":     map[string]interface{}{"code": "6601", "name": "管理费用"},
					"auxiliaryShow":  "行政部",
					"debit_original": float64(1000),
				},
			},
		},
	}
	rows := flattenVoucherRecords(recs, "ZJ001", "浙江松鲜鲜")
	if len(rows) != 1 {
		t.Fatalf("应抽平 1 行, got %d", len(rows))
	}
	r := rows[0]
	if r.AccbookCode != "ZJ001" || r.AccbookName != "浙江松鲜鲜" {
		t.Errorf("账簿标记错: code=%q name=%q", r.AccbookCode, r.AccbookName)
	}
	if r.ID != "1234567890123456789" {
		t.Errorf("19 位 id 应原样 string, got %q", r.ID)
	}
	if r.VoucherNo != "转-1" || r.VoucherType != "转账凭证" {
		t.Errorf("字号/类型错: %q %q", r.VoucherNo, r.VoucherType)
	}
	if r.Status != "已记账" {
		t.Errorf("状态 04 应→已记账, got %q", r.Status)
	}
	if r.TotalDebit != 1000 || r.TotalCredit != 1000 {
		t.Errorf("借贷合计错: %v %v", r.TotalDebit, r.TotalCredit)
	}
	if len(r.Lines) != 1 || r.Lines[0].SubjectName != "管理费用" {
		t.Errorf("分录错: %+v", r.Lines)
	}
}
