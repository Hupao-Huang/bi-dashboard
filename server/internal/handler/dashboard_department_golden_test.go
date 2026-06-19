package handler

// dashboard_department_golden_test.go — 真实数据 golden-diff 回归网。
// 目的: 给 GetDepartmentDetail 的 extract-method 重构提供"行为 100% 保持"的硬证据
// (现有 sqlmock 单测只盖 ecommerce/distribution/crossDept, 没盖 offline/social/instant_retail;
//  这里直接连真库, 对所有部门×参数组合逐字节对拍重构前后的 JSON)。
//
// 默认 SKIP (CI 无真库, DEPT_GOLDEN 未设即跳过, 不影响 CI / 普通 go test)。
// 本地用法 (cwd = server):
//   写基线(重构前):  DEPT_GOLDEN=1 DEPT_GOLDEN_WRITE=1 go test ./internal/handler -run TestGoldenDepartmentDetail -count=1 -v
//   对拍  (重构后):  DEPT_GOLDEN=1                      go test ./internal/handler -run TestGoldenDepartmentDetail -count=1 -v
// golden 数据写到 DEPT_GOLDEN_DIR (默认系统临时目录, 含真实业务金额/店名/SKU, 不入 git)。

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bi-dashboard/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

func goldenDeptDir() string {
	if d := os.Getenv("DEPT_GOLDEN_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.TempDir(), "dept_golden")
}

func openRealDBForGolden(t *testing.T) *sql.DB {
	t.Helper()
	// 基础设施缺失(没 config.json / 连不上真库)一律 t.Skip 而非 Fatalf:
	// 这是真库回归网, 在没有生产库的机器上(CI/同事本地)即使误设 DEPT_GOLDEN=1 也应优雅跳过,
	// 不能把整包测试搞红。Fatalf 只留给"库连上了但查询真出错"的情况。
	cfg, err := config.Load("../../config.json")
	if err != nil {
		t.Skipf("跳过: 读不到 ../../config.json (%v)", err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		t.Skipf("跳过: 打不开 DB 连接 (%v)", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("跳过: 真库不可达 (%v)", err)
	}
	return db
}

// firstLineDiff 返回首个不同的行号(1-based)+上下文, 便于定位 golden 漂移
func firstLineDiff(golden, actual string) string {
	gl := strings.Split(golden, "\n")
	al := strings.Split(actual, "\n")
	n := len(gl)
	if len(al) < n {
		n = len(al)
	}
	for i := 0; i < n; i++ {
		if gl[i] != al[i] {
			return fmt.Sprintf("首差异行 %d:\n  golden: %s\n  actual: %s", i+1, gl[i], al[i])
		}
	}
	if len(gl) != len(al) {
		return fmt.Sprintf("行数不同: golden=%d actual=%d", len(gl), len(al))
	}
	return "(无行级差异, 可能是结尾换行)"
}

func TestGoldenDepartmentDetail(t *testing.T) {
	if os.Getenv("DEPT_GOLDEN") == "" {
		t.Skip("跳过真库 golden-diff (设 DEPT_GOLDEN=1 运行)")
	}
	db := openRealDBForGolden(t)
	defer db.Close()
	h := &DashboardHandler{DB: db}

	// 覆盖所有部门分支 + 平台/店铺过滤 + crossDept + 调拨专区
	cases := []struct{ name, query string }{
		{"ecommerce", "dept=ecommerce&start=2026-04-01&end=2026-04-30"},
		{"ecommerce_allot", "dept=ecommerce&start=2026-04-01&end=2026-04-30&platform=allot"},
		{"ecommerce_jd", "dept=ecommerce&start=2026-04-01&end=2026-04-30&platform=jd"},
		{"ecommerce_cross", "dept=ecommerce&start=2026-04-01&end=2026-04-30&crossDept=1"},
		{"social", "dept=social&start=2026-04-01&end=2026-04-30"},
		{"offline", "dept=offline&start=2026-04-01&end=2026-04-30"},
		{"offline_region", "dept=offline&start=2026-04-01&end=2026-04-30&shop=华东大区"},
		{"distribution", "dept=distribution&start=2026-04-01&end=2026-04-30"},
		{"instant_retail", "dept=instant_retail&start=2026-04-01&end=2026-04-30"},
	}

	dir := goldenDeptDir()
	write := os.Getenv("DEPT_GOLDEN_WRITE") != ""
	if write {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		t.Logf("golden 目录: %s", dir)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/department?"+c.query, nil)
			h.GetDepartmentDetail(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
			}
			// 归一化: unmarshal + 缩进重 marshal (Go json 对 map key 自动排序 => 确定输出)
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
				t.Fatalf("读基线失败 %s: %v (先用 DEPT_GOLDEN_WRITE=1 生成基线)", path, err)
			}
			if string(golden) != string(norm) {
				_ = os.WriteFile(path+".actual", norm, 0o644)
				t.Errorf("golden 不一致 case=%s\n%s\n(actual 已写 %s.actual)", c.name, firstLineDiff(string(golden), string(norm)), path)
			}
		})
	}
}
