package handler

// supply_chain_dashboard_golden_test.go — GetSupplyChainDashboard 真实数据 golden-diff 回归网。
// 给这个并发巨函数 (原 ~900 行 / 20 路并发查询) 提供"行为保持"硬证据 (纯 DB, 无外部 API/字典)。
// 复用 dashboard_department_golden_test.go 的 openRealDBForGolden / firstLineDiff。
//
// 默认 SKIP (CI 无真库)。本地用法 (cwd = server):
//   写基线(重构前):  SC_GOLDEN=1 SC_GOLDEN_WRITE=1 go test ./internal/handler -run TestGoldenSupplyChainDashboard -count=1 -v
//   对拍  (重构后):  SC_GOLDEN=1                    go test ./internal/handler -run TestGoldenSupplyChainDashboard -count=1 -v
// golden 数据写到 SC_GOLDEN_DIR (默认系统临时目录, 含真实库存金额/SKU, 不入 git)。
// 并发函数: 各 goroutine 写各自变量, wg.Wait 后顺序组装; 双拍验确定性。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func goldenSCDir() string {
	if d := os.Getenv("SC_GOLDEN_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.TempDir(), "sc_golden")
}

func TestGoldenSupplyChainDashboard(t *testing.T) {
	if os.Getenv("SC_GOLDEN") == "" {
		t.Skip("跳过真库 golden-diff (设 SC_GOLDEN=1 运行)")
	}
	db := openRealDBForGolden(t)
	defer db.Close()
	h := &DashboardHandler{DB: db}

	// 库存快照 2026-04-20 ~ 06-18; 覆盖整月 + 部分月 + 跨月
	cases := []struct{ name, query string }{
		{"may", "start=2026-05-01&end=2026-05-31"},
		{"june_partial", "start=2026-06-01&end=2026-06-18"},
		{"cross_month", "start=2026-04-21&end=2026-06-18"},
	}

	dir := goldenSCDir()
	write := os.Getenv("SC_GOLDEN_WRITE") != ""
	if write {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		t.Logf("golden 目录: %s", dir)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/supply-chain/dashboard?"+c.query, nil)
			h.GetSupplyChainDashboard(rec, req)
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
				t.Fatalf("读基线失败 %s: %v (先用 SC_GOLDEN_WRITE=1 生成基线)", path, err)
			}
			if string(golden) != string(norm) {
				_ = os.WriteFile(path+".actual", norm, 0o644)
				t.Errorf("golden 不一致 case=%s\n%s\n(actual 已写 %s.actual)", c.name, firstLineDiff(string(golden), string(norm)), path)
			}
		})
	}
}
