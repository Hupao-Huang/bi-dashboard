package handler

// 规则 14 健康证及体检: 金额 ≤100 且 发票开票 ≤6 个月通过, 否则驳回 (跑哥 2026-06-11)
// 连带验证: 体检类发票不走规则 8-4 的 3 个月通用线

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const r14Month = int64(30 * 24 * 3600 * 1000)

// mkHealthRaw 构造含一条健康证及体检明细的 raw_json map
func mkHealthRaw(amount float64, submitDate int64) map[string]interface{} {
	return map[string]interface{}{
		"submitDate": float64(submitDate),
		"details": []interface{}{
			map[string]interface{}{
				"feeTypeId": healthExamFeeTypeID,
				"feeTypeForm": map[string]interface{}{
					"detailId": "D-health-1",
					"detailNo": float64(1),
					"amount":   map[string]interface{}{"standard": fmt.Sprintf("%.2f", amount)},
				},
			},
		},
	}
}

func TestRuleHealthExamAmount(t *testing.T) {
	submit := int64(1781222400000) // 2026-06-09 前后, 具体值不影响金额分支
	cases := []struct {
		name       string
		amount     float64
		invoiceAge int64 // 开票距提交的毫秒数
		wantReject bool
		wantHint   string
	}{
		{"100整-发票新鲜-通过", 100.00, r14Month, false, ""},
		{"金额0(解析失败)-驳回偏严", 0, r14Month, true, "金额无法识别"},
		{"99.5-发票新鲜-通过", 99.5, 2 * r14Month, false, ""},
		{"100.01-驳回", 100.01, r14Month, true, "> 标准 ¥100"},
		{"150-驳回", 150, r14Month, true, "> 标准 ¥100"},
		{"80-发票5个月-通过(8-4的3个月线不适用)", 80, 5 * r14Month, false, ""},
		{"80-发票7个月-驳回", 80, 7 * r14Month, true, "> 6 个月"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()
			mock.ExpectQuery(`SELECT IFNULL\(detail_id,''\), IFNULL\(invoice_date,0\) FROM hesi_flow_invoice`).
				WillReturnRows(sqlmock.NewRows([]string{"detail_id", "invoice_date"}).
					AddRow("D-health-1", submit-c.invoiceAge))

			h := &DashboardHandler{DB: db}
			rejects := h.ruleHealthExam(mkHealthRaw(c.amount, submit), "F-health")
			got := strings.Join(rejects, "; ")
			if c.wantReject != (len(rejects) > 0) {
				t.Errorf("期望驳回=%v, 实际=%q", c.wantReject, got)
			}
			if c.wantHint != "" && !strings.Contains(got, c.wantHint) {
				t.Errorf("驳回理由应含 %q, 实际=%q", c.wantHint, got)
			}
		})
	}
}

func TestRuleHealthExamNotTriggered(t *testing.T) {
	// 无体检明细 → 不触发, 不查库
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := &DashboardHandler{DB: db}
	raw := map[string]interface{}{
		"submitDate": float64(1781222400000),
		"details": []interface{}{
			map[string]interface{}{
				"feeTypeId": "ID01OTHER",
				"feeTypeForm": map[string]interface{}{
					"detailId": "D1", "detailNo": float64(1),
					"amount": map[string]interface{}{"standard": "999.00"},
				},
			},
		},
	}
	if rejects := h.ruleHealthExam(raw, "F1"); len(rejects) != 0 {
		t.Errorf("非体检明细不应触发规则 14, got %v", rejects)
	}
}
