package handler

// distribution_customer_test.go — distribution_customer.go handler 边界 + happy path
// 已 Read distribution_customer.go (line 34-227+).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ============ ListDistributionCustomers ============

func TestListDistributionCustomersHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM distribution_high_value_customers WHERE 1=1`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(120))

	mock.ExpectQuery(`SELECT id, customer_code, customer_name.*FROM distribution_high_value_customers WHERE 1=1`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "grade", "remark", "first", "last", "amt", "ord", "by", "ct", "ut"}).
			AddRow(1, "C001", "客户A", "S", "", "2026-01-01", "2026-05-01", 1000000.0, 50, "admin", "2026-01-01 10:00:00", "2026-05-01 10:00:00").
			AddRow(2, "C002", "客户B", "A", "", "2026-02-01", "2026-04-30", 500000.0, 30, "admin", "2026-02-01 10:00:00", "2026-04-30 10:00:00"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/distribution/customers?page=1&pageSize=50", nil)
	(&DashboardHandler{DB: db}).ListDistributionCustomers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if resp["total"] != float64(120) {
		t.Errorf("total 应 120, got %v", resp["total"])
	}
	list, _ := resp["list"].([]interface{})
	if len(list) != 2 {
		t.Errorf("list 应 2, got %d", len(list))
	}
}

func TestListDistributionCustomersGradeFilter(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// grade=S → conds += "grade = ?"
	mock.ExpectQuery(`COUNT\(\*\) FROM distribution_high_value_customers WHERE 1=1 AND grade = \?`).
		WithArgs("S").
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(10))

	mock.ExpectQuery(`FROM distribution_high_value_customers WHERE 1=1 AND grade = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "grade", "remark", "first", "last", "amt", "ord", "by", "ct", "ut"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/distribution/customers?grade=S", nil)
	(&DashboardHandler{DB: db}).ListDistributionCustomers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestListDistributionCustomersGradeNone(t *testing.T) {
	// grade=none → 显式查无等级的客户
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`COUNT\(\*\) FROM distribution_high_value_customers WHERE 1=1 AND \(grade IS NULL OR grade=''\)`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(50))
	mock.ExpectQuery(`FROM distribution_high_value_customers`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "grade", "remark", "first", "last", "amt", "ord", "by", "ct", "ut"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/distribution/customers?grade=none", nil)
	(&DashboardHandler{DB: db}).ListDistributionCustomers(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d", rec.Code)
	}
}

// ============ SetDistributionCustomerGrade ============

func TestSetDistributionCustomerGradeHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`UPDATE distribution_high_value_customers SET grade=\?, remark=\? WHERE customer_code=\?`).
		WithArgs("S", "VIP 客户", "C001").
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := []byte(`{"customerCode":"C001","grade":"S","remark":"VIP 客户"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/distribution/customers/grade", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).SetDistributionCustomerGrade(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSetDistributionCustomerGradeMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/distribution/customers/grade", nil)
	(&DashboardHandler{DB: db}).SetDistributionCustomerGrade(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

func TestSetDistributionCustomerGradeBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/distribution/customers/grade", bytes.NewReader([]byte(`not json`)))
	(&DashboardHandler{DB: db}).SetDistributionCustomerGrade(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json 应 400, got %d", rec.Code)
	}
}

func TestSetDistributionCustomerGradeMissingCode(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"customerCode":"","grade":"S"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/distribution/customers/grade", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).SetDistributionCustomerGrade(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("空 customerCode 应 400, got %d", rec.Code)
	}
}

func TestSetDistributionCustomerGradeInvalidGrade(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"customerCode":"C001","grade":"Z"}`) // Z 不在 S/A/SA 白名单
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/distribution/customers/grade", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).SetDistributionCustomerGrade(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid grade 应 400, got %d", rec.Code)
	}
}

func TestSetDistributionCustomerGradeNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`UPDATE distribution_high_value_customers`).
		WillReturnResult(sqlmock.NewResult(0, 0)) // affected=0

	body := []byte(`{"customerCode":"NONEXIST","grade":"A"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/distribution/customers/grade", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).SetDistributionCustomerGrade(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("affected=0 应 404, got %d", rec.Code)
	}
}

func TestSetDistributionCustomerGradeClearWithEmptyString(t *testing.T) {
	// grade="" → 设置为 NULL (line 138-143)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`UPDATE distribution_high_value_customers SET grade=\?`).
		WithArgs(nil, "", "C001"). // grade 是 nil
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := []byte(`{"customerCode":"C001","grade":""}`) // 清除等级
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/distribution/customers/grade", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).SetDistributionCustomerGrade(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("清除 grade 应 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============ BatchSetDistributionCustomerGrade ============

func TestBatchSetDistributionCustomerGradeMethodNotAllowed(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/distribution/customers/grade-batch", nil)
	(&DashboardHandler{DB: db}).BatchSetDistributionCustomerGrade(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应 405, got %d", rec.Code)
	}
}

func TestBatchSetDistributionCustomerGradeBadJSON(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/distribution/customers/grade-batch", bytes.NewReader([]byte(`bad`)))
	(&DashboardHandler{DB: db}).BatchSetDistributionCustomerGrade(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json 应 400, got %d", rec.Code)
	}
}

func TestBatchSetDistributionCustomerGradeEmptyItems(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	body := []byte(`{"items":[]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/distribution/customers/grade-batch", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).BatchSetDistributionCustomerGrade(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("空 items 应 400, got %d", rec.Code)
	}
}

func TestBatchSetDistributionCustomerGradeHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	// 2 items → 2 UPDATE
	mock.ExpectExec(`UPDATE distribution_high_value_customers SET grade=\?`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE distribution_high_value_customers SET grade=\?`).
		WillReturnResult(sqlmock.NewResult(0, 0)) // notFound
	mock.ExpectCommit()

	body := []byte(`{"items":[{"customerCode":"C001","grade":"S"},{"customerCode":"NX","grade":"A"}]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/distribution/customers/grade-batch", bytes.NewReader(body))
	(&DashboardHandler{DB: db}).BatchSetDistributionCustomerGrade(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &env)
	resp, _ := env["data"].(map[string]interface{})
	if resp["updated"] != float64(1) {
		t.Errorf("updated 应 1 (1 个 affected), got %v", resp["updated"])
	}
	notFound, _ := resp["notFound"].([]interface{})
	if len(notFound) != 1 {
		t.Errorf("notFound 应 1, got %d", len(notFound))
	}
}

// ============ DistributionCustomerSkus ============

func TestDistributionCustomerSkusMissingCustomerCode(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/distribution/customer-analysis/skus", nil)
	(&DashboardHandler{DB: db}).DistributionCustomerSkus(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺 customerCode 应 400, got %d", rec.Code)
	}
}
