package handler

// hesi_flow_detail_golden_test.go — GetHesiFlowDetail 真实数据 golden-diff 回归网。
// 给 GetHesiFlowDetail(457行) 的 extract-method 重构提供"行为保持"证据。
// 复用 dashboard_department_golden_test.go 的 openRealDBForGolden / firstLineDiff。
//
// 注意: GetHesiFlowDetail 里有合思字典反查(LookupStaffName/DeptName/FeeTypeName/SpecName/LegalEntityName),
// 冷缓存会调合思 API。本测试给 handler 配上合思 creds(从 config 读)让字典正常加载+缓存(返真名, 确定),
// h.YS 留 nil(用友凭证那段返 nil, 守卫安全)。**跑前清代理 env** 让合思 API 直连。
//
// 默认 SKIP (CI 无真库+无合思网络)。本地用法 (cwd = server, 必须清代理):
//   写基线(重构前):  HESI_GOLDEN=1 HESI_GOLDEN_WRITE=1 go test ./internal/handler -run TestGoldenHesiFlowDetail -count=1 -v
//   对拍  (重构后):  HESI_GOLDEN=1                       go test ./internal/handler -run TestGoldenHesiFlowDetail -count=1 -v
// golden 数据写到 HESI_GOLDEN_DIR (默认系统临时目录, 含真实单据/金额/人名, 不入 git)。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"bi-dashboard/internal/config"
)

func goldenHesiDir() string {
	if d := os.Getenv("HESI_GOLDEN_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.TempDir(), "hesi_golden")
}

func TestGoldenHesiFlowDetail(t *testing.T) {
	if os.Getenv("HESI_GOLDEN") == "" {
		t.Skip("跳过真库 golden-diff (设 HESI_GOLDEN=1 运行, 需清代理让合思API直连)")
	}
	db := openRealDBForGolden(t)
	defer db.Close()
	cfg, err := config.Load("../../config.json")
	if err != nil {
		t.Skipf("跳过: 读不到 config (%v)", err)
	}
	// 配合思 creds 让字典反查正常(返真名, 确定); YS 留 nil(凭证段返 nil, 守卫安全)
	h := &DashboardHandler{DB: db, HesiAppKey: cfg.Hesi.AppKey, HesiSecret: cfg.Hesi.Secret}

	// 覆盖 4 个 form_type + expenseLinks(关联申请单)分支
	cases := []struct{ name, flowID string }{
		{"requisition", "ID01FC9liwVPGf"},
		{"expense", "ID01FCaQszsnuf"},
		{"loan", "ID01KCElaW5umX"},
		{"custom", "ID01KCzko11FJd"},
		{"expense_links1", "ID01TpjWlwUO07"},
		{"expense_links2", "ID01Tod5qjhHgr"},
	}

	dir := goldenHesiDir()
	write := os.Getenv("HESI_GOLDEN_WRITE") != ""
	if write {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		t.Logf("golden 目录: %s", dir)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/hesi/flow-detail?flowId="+c.flowID, nil)
			h.GetHesiFlowDetail(rec, req)
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
				t.Fatalf("读基线失败 %s: %v (先用 HESI_GOLDEN_WRITE=1 生成基线)", path, err)
			}
			if string(golden) != string(norm) {
				_ = os.WriteFile(path+".actual", norm, 0o644)
				t.Errorf("golden 不一致 case=%s\n%s\n(actual 已写 %s.actual)", c.name, firstLineDiff(string(golden), string(norm)), path)
			}
		})
	}
}
