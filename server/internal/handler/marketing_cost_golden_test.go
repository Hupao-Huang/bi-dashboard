package handler

// marketing_cost_golden_test.go — GetMarketingCost 真实数据 golden-diff 回归网。
// 给 GetMarketingCost 的 extract-method 重构提供"行为 100% 保持"硬证据 (覆盖 all + 4 个单平台分支)。
// 复用 dashboard_department_golden_test.go 的 openRealDBForGolden / firstLineDiff。
//
// 默认 SKIP (CI 无真库)。本地用法 (cwd = server):
//   写基线(重构前):  MKTG_GOLDEN=1 MKTG_GOLDEN_WRITE=1 go test ./internal/handler -run TestGoldenMarketingCost -count=1 -v
//   对拍  (重构后):  MKTG_GOLDEN=1                       go test ./internal/handler -run TestGoldenMarketingCost -count=1 -v
// golden 数据写到 MKTG_GOLDEN_DIR (默认系统临时目录, 含真实费用金额/店名, 不入 git)。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func goldenMktgDir() string {
	if d := os.Getenv("MKTG_GOLDEN_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.TempDir(), "mktg_golden")
}

func TestGoldenMarketingCost(t *testing.T) {
	if os.Getenv("MKTG_GOLDEN") == "" {
		t.Skip("跳过真库 golden-diff (设 MKTG_GOLDEN=1 运行)")
	}
	db := openRealDBForGolden(t)
	defer db.Close()
	h := &DashboardHandler{DB: db}

	// 覆盖 all(跨平台合并) + 4 个单平台分支(各自的场景明细/SKU TOP/视频/淘客)
	cases := []struct{ name, query string }{
		{"all", "platform=all&start=2026-05-01&end=2026-05-31"},
		{"tmall", "platform=tmall&start=2026-05-01&end=2026-05-31"},
		{"jd", "platform=jd&start=2026-05-01&end=2026-05-31"},
		{"pdd", "platform=pdd&start=2026-05-01&end=2026-05-31"},
		{"tmall_cs", "platform=tmall_cs&start=2026-05-01&end=2026-05-31"},
	}

	dir := goldenMktgDir()
	write := os.Getenv("MKTG_GOLDEN_WRITE") != ""
	if write {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		t.Logf("golden 目录: %s", dir)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/marketing-cost?"+c.query, nil)
			h.GetMarketingCost(rec, req)
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
				t.Fatalf("读基线失败 %s: %v (先用 MKTG_GOLDEN_WRITE=1 生成基线)", path, err)
			}
			if string(golden) != string(norm) {
				_ = os.WriteFile(path+".actual", norm, 0o644)
				t.Errorf("golden 不一致 case=%s\n%s\n(actual 已写 %s.actual)", c.name, firstLineDiff(string(golden), string(norm)), path)
			}
		})
	}
}
