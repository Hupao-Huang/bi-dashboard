package handler

// hesi_payment_ocr_test.go — 付款截图OCR结果缓存表存取层单元测试
// sqlmock default matcher = regexp

import (
	"math"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestReconcilePayment(t *testing.T) {
	// B26003890: 付款 175.91+118 = 293.91, 发票 293.91 → 不flag
	f, p, i := reconcilePayment([]float64{175.91, 118}, []float64{175.91, 118}, 0.01)
	if f || math.Abs(p-293.91) > 0.001 || math.Abs(i-293.91) > 0.001 {
		t.Errorf("890: flag=%v pay=%v inv=%v", f, p, i)
	}
	// B26003807 差旅: 付款 |−529|+|−564| = 1093, 发票 1241.20 → 付款<发票 → 不flag
	f2, _, _ := reconcilePayment([]float64{-529, -564}, []float64{529, 564, 35.72, 11.09, 23.34, 15.95, 26, 26.09, 10.01}, 0.01)
	if f2 {
		t.Error("差旅 付款<发票 不该flag")
	}
	// 付款 > 发票 → flag
	f3, _, _ := reconcilePayment([]float64{200}, []float64{100}, 0.01)
	if !f3 {
		t.Error("付款200>发票100 应flag")
	}
	// 2元容差(跑哥6-25口径): 付款比发票多出 ≤2元 自动通过, >2元才flag
	if f4, _, _ := reconcilePayment([]float64{102}, []float64{100}, paymentOverToleranceYuan); f4 {
		t.Error("付款102 仅多2元(=容差) 不该flag")
	}
	if f5, _, _ := reconcilePayment([]float64{103}, []float64{100}, paymentOverToleranceYuan); !f5 {
		t.Error("付款103 多3元(>容差) 应flag")
	}
}

func TestCheckFlowPayment(t *testing.T) {
	t.Run("付款=发票 不flag", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()

		// getPaymentOcrByFlow SELECT
		ocrRows := sqlmock.NewRows([]string{"file_id", "amount", "status"}).
			AddRow("fileA", 175.91, "ok").
			AddRow("fileB", -118.0, "ok")
		mock.ExpectQuery("SELECT file_id, amount, status FROM hesi_payment_ocr WHERE flow_id=").
			WithArgs("flowX").WillReturnRows(ocrRows)

		// sumInvoiceTotal: SELECT SUM(IFNULL(total_amount,0)) FROM hesi_flow_invoice WHERE flow_id=?
		invRows := sqlmock.NewRows([]string{"SUM(IFNULL(total_amount,0))"}).
			AddRow(293.91)
		mock.ExpectQuery(`SELECT SUM\(IFNULL\(total_amount,0\)\) FROM hesi_flow_invoice WHERE flow_id=`).
			WithArgs("flowX").WillReturnRows(invRows)

		h := &DashboardHandler{DB: db}
		got := h.checkFlowPayment("flowX")

		if got.Flag {
			t.Errorf("付款=发票 不应flag, got=%+v", got)
		}
		if math.Abs(got.PayTotal-293.91) > 0.001 {
			t.Errorf("PayTotal=%.4f, want 293.91", got.PayTotal)
		}
		if math.Abs(got.InvTotal-293.91) > 0.001 {
			t.Errorf("InvTotal=%.4f, want 293.91", got.InvTotal)
		}
		if got.Pending {
			t.Error("Pending 应为 false")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("未满足的 mock 期望: %v", err)
		}
	})

	t.Run("付款>发票 flag=true", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()

		ocrRows := sqlmock.NewRows([]string{"file_id", "amount", "status"}).
			AddRow("fileC", 200.0, "ok")
		mock.ExpectQuery("SELECT file_id, amount, status FROM hesi_payment_ocr WHERE flow_id=").
			WithArgs("flowY").WillReturnRows(ocrRows)

		// sumInvoiceTotal: SUM返回100, 付款200 > 发票100 → flag
		invRows := sqlmock.NewRows([]string{"SUM(IFNULL(total_amount,0))"}).
			AddRow(100.0)
		mock.ExpectQuery(`SELECT SUM\(IFNULL\(total_amount,0\)\) FROM hesi_flow_invoice WHERE flow_id=`).
			WithArgs("flowY").WillReturnRows(invRows)

		h := &DashboardHandler{DB: db}
		got := h.checkFlowPayment("flowY")

		if !got.Flag {
			t.Errorf("付款200>发票100 应flag, got=%+v", got)
		}
		if got.Note == "" {
			t.Error("Flag=true 时 Note 不应为空")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("未满足的 mock 期望: %v", err)
		}
	})

	t.Run("有pending截图 flag强制false", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()

		ocrRows := sqlmock.NewRows([]string{"file_id", "amount", "status"}).
			AddRow("fileD", 200.0, "ok").
			AddRow("fileE", 0.0, "fail")
		mock.ExpectQuery("SELECT file_id, amount, status FROM hesi_payment_ocr WHERE flow_id=").
			WithArgs("flowZ").WillReturnRows(ocrRows)

		// sumInvoiceTotal SUM
		invRows := sqlmock.NewRows([]string{"SUM(IFNULL(total_amount,0))"}).
			AddRow(100.0)
		mock.ExpectQuery(`SELECT SUM\(IFNULL\(total_amount,0\)\) FROM hesi_flow_invoice WHERE flow_id=`).
			WithArgs("flowZ").WillReturnRows(invRows)

		h := &DashboardHandler{DB: db}
		got := h.checkFlowPayment("flowZ")

		if got.Flag {
			t.Errorf("有pending时 flag 应强制false, got=%+v", got)
		}
		if !got.Pending {
			t.Error("Pending 应为 true")
		}
		if got.Note != "部分付款截图待识别" {
			t.Errorf("Note 不对: %q", got.Note)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("未满足的 mock 期望: %v", err)
		}
	})
}

func TestUpsertAndGetPaymentOcr(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectExec("INSERT INTO hesi_payment_ocr").
		WithArgs("fileA", "flow1", "付款截图1.jpg", 175.91, "ok", "175.91").
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := upsertPaymentOcr(db, "fileA", "flow1", "付款截图1.jpg", 175.91, "ok", "175.91"); err != nil {
		t.Fatal(err)
	}
	rows := sqlmock.NewRows([]string{"file_id", "amount", "status"}).
		AddRow("fileA", 175.91, "ok").AddRow("fileB", -118.0, "ok")
	mock.ExpectQuery("SELECT file_id, amount, status FROM hesi_payment_ocr WHERE flow_id=").
		WithArgs("flow1").WillReturnRows(rows)
	got, err := getPaymentOcrByFlow(db, "flow1")
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v err %v", got, err)
	}
	if got[0].FileID != "fileA" || got[0].Amount != 175.91 || got[0].Status != "ok" {
		t.Errorf("row 0: got %+v", got[0])
	}
	if got[1].FileID != "fileB" || got[1].Amount != -118.0 || got[1].Status != "ok" {
		t.Errorf("row 1 (负数金额): got %+v", got[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("未满足的 mock 期望: %v", err)
	}
}
