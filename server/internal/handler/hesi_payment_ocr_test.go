package handler

// hesi_payment_ocr_test.go — 付款截图OCR结果缓存表存取层单元测试
// sqlmock default matcher = regexp

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

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
}
