package handler

// finance_report_golden_test.go — GetFinanceReport 真实数据 golden-diff 回归网。
// 给 GetFinanceReport (273 行) 的 extract-method 重构提供"行为保持"硬证据 (纯 DB 只读, 无外部 API)。
// 复用 dashboard_department_golden_test.go 的 openRealDBForGolden / firstLineDiff。
//
// 默认 SKIP (CI 无真库)。本地用法 (cwd = server):
//   写基线(重构前):  FIN_GOLDEN=1 FIN_GOLDEN_WRITE=1 go test ./internal/handler -run TestGoldenFinanceReport -count=1 -v
//   对拍  (重构后):  FIN_GOLDEN=1                     go test ./internal/handler -run TestGoldenFinanceReport -count=1 -v
// golden 数据写到 FIN_GOLDEN_DIR (默认系统临时目录, 含真实财务金额, 不入 git)。
// 覆盖: 单渠道(Total 路径)/多渠道(ByChannel 路径)/跨年区间/单月。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func goldenFinDir() string {
	if d := os.Getenv("FIN_GOLDEN_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.TempDir(), "fin_golden")
}

func TestGoldenFinanceReport(t *testing.T) {
	if os.Getenv("FIN_GOLDEN") == "" {
		t.Skip("跳过真库 golden-diff (设 FIN_GOLDEN=1 运行)")
	}
	db := openRealDBForGolden(t)
	defer db.Close()
	h := &DashboardHandler{DB: db}

	cases := []struct{ name, query string }{
		{"summary_2026", "yearStart=2026&monthStart=1&monthEnd=12&channels=汇总"},
		{"multi_2026_h1", "yearStart=2026&monthStart=1&monthEnd=6&channels=电商,社媒,私域"},
		{"cross_year", "yearStart=2024&yearEnd=2026&monthStart=1&monthEnd=12&channels=汇总"},
		{"single_month", "yearStart=2026&monthStart=3&monthEnd=3&channels=电商"},
	}

	dir := goldenFinDir()
	write := os.Getenv("FIN_GOLDEN_WRITE") != ""
	if write {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		t.Logf("golden 目录: %s", dir)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/finance/report?"+c.query, nil)
			h.GetFinanceReport(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
			}
			var v interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
				t.Fatalf("unmarshal resp: %v", err)
			}
			norm, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				t.Fatalf("marshal norm: %v", err)
			}
			path := filepath.Join(dir, c.name+".json")

			if write {
				if err := os.WriteFile(path, norm, 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				t.Logf("已写基线 %s (%d 字节)", path, len(norm))
				return
			}

			golden, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("读基线失败 %s: %v (先用 FIN_GOLDEN_WRITE=1 生成基线)", path, err)
			}
			if string(golden) != string(norm) {
				_ = os.WriteFile(path+".actual", norm, 0o644)
				t.Errorf("golden 不一致 case=%s\n%s\n(actual 已写 %s.actual)", c.name, firstLineDiff(string(golden), string(norm)), path)
			}
		})
	}
}
