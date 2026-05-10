package finance

// loaddict_test.go — LoadSubjectDict sqlmock 测试
// 已 Read parser.go line 184-202: SELECT subject_code, subject_name, subject_category,
// subject_level, parent_code, aliases FROM finance_subject_dict

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLoadSubjectDictHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"subject_code", "subject_name", "subject_category", "subject_level", "parent_code", "aliases"}).
		AddRow("REV_TOTAL", "营业额合计", "GMV", 2, "", `["营收合计"]`).
		AddRow("COST_MAIN.物流费用", "物流费用", "成本", 3, "COST_MAIN", "")

	mock.ExpectQuery(`SELECT subject_code, subject_name, subject_category, subject_level, parent_code, aliases FROM finance_subject_dict`).
		WillReturnRows(rows)

	dict, err := LoadSubjectDict(db)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(dict) != 2 {
		t.Fatalf("应有 2 条, got %d", len(dict))
	}
	rev := dict["REV_TOTAL"]
	if rev == nil {
		t.Fatal("缺 REV_TOTAL")
	}
	if rev.Name != "营业额合计" {
		t.Errorf("name 错: %s", rev.Name)
	}
	if rev.Level != 2 {
		t.Errorf("level 错: %d", rev.Level)
	}
	if len(rev.Aliases) != 1 || rev.Aliases[0] != "营收合计" {
		t.Errorf("aliases JSON 解析错: %v", rev.Aliases)
	}

	// 第 2 条: aliases 是空 string, 应保持 nil/empty
	cost := dict["COST_MAIN.物流费用"]
	if cost.Parent != "COST_MAIN" {
		t.Errorf("parent 错: %s", cost.Parent)
	}
	if len(cost.Aliases) != 0 {
		t.Errorf("空 aliases 应为空 slice, got %v", cost.Aliases)
	}
}

func TestLoadSubjectDictDBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT subject_code`).WillReturnError(errors.New("connection lost"))

	_, err = LoadSubjectDict(db)
	if err == nil {
		t.Error("DB error 应传出")
	}
}

func TestLoadSubjectDictNullAliases(t *testing.T) {
	// aliases 字段是 NULL, 应不 panic, Aliases=nil
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"subject_code", "subject_name", "subject_category", "subject_level", "parent_code", "aliases"}).
		AddRow("CODE_X", "Name X", "Cat", 2, "", nil) // aliases NULL

	mock.ExpectQuery(`SELECT subject_code`).WillReturnRows(rows)
	dict, err := LoadSubjectDict(db)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if dict["CODE_X"].Aliases != nil && len(dict["CODE_X"].Aliases) != 0 {
		t.Errorf("NULL aliases 应为 nil/empty, got %v", dict["CODE_X"].Aliases)
	}
}

func TestLoadSubjectDictEmptyResult(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT subject_code`).
		WillReturnRows(sqlmock.NewRows([]string{"subject_code", "subject_name", "subject_category", "subject_level", "parent_code", "aliases"}))

	dict, err := LoadSubjectDict(db)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(dict) != 0 {
		t.Errorf("空 rows 应返空 dict, got %d", len(dict))
	}
}
