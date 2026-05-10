package handler

// dashboard_sql_guard_test.go — 防御 2026-05-10 跑哥发现的关键 bug 重现
//
// 这一组 test 是 source-level 守护: 直接读 dashboard_*.go 源码, 断言关键 SQL 片段存在.
// 跟 sqlmock 单元测试互补, 防御"被无意改坏"的回归. (memory feedback_test_and_verify)

import (
	"os"
	"strings"
	"testing"
)

// readSource 读取 handler 包内某 .go 文件源码 (相对 internal/handler/)
func readSource(t *testing.T, name string) string {
	t.Helper()
	src, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(src)
}

// Bug 1: 跑哥 2026-05-10 在综合看板 hover 时看到 "other ¥0 (0.0%)" 块
// 修复: dashboard_overview.go 全 7+ 处 SQL 加 IFNULL(department,'') NOT IN ('excluded','other','')
func TestDashboardOverviewExcludesOtherAndExcludedAcross7Queries(t *testing.T) {
	src := readSource(t, "dashboard_overview.go")
	frag1 := "IFNULL(department,'') NOT IN ('excluded','other','')"
	frag2 := "IFNULL(s.department,'') NOT IN ('excluded','other','')"
	count := strings.Count(src, frag1) + strings.Count(src, frag2)
	if count < 7 {
		t.Fatalf("dashboard_overview.go 应至少 7 处 dept 排除规则 (dept汇总/趋势/商品TOP15/商品渠道/店铺TOP15/grade/gradeDept), 当前 %d", count)
	}
}

// Bug 2: dashboard_department.go gradeDeptSalesAll 和 gradeShopSalesAll
// 也是 crossDept=1 时返回的全口径聚合, 必须排除 other/excluded
func TestDashboardDepartmentCrossDeptExcludesOtherAndExcluded(t *testing.T) {
	src := readSource(t, "dashboard_department.go")
	frag := "IFNULL(s.department,'') NOT IN ('excluded','other','')"
	if strings.Count(src, frag) < 2 {
		t.Fatalf("dashboard_department.go gradeDeptSalesAll + gradeShopSalesAll 应都加排除, 当前 %d 处", strings.Count(src, frag))
	}
}

// Bug 3: 跑哥 2026-05-10 报"店铺榜全部 20 家但累加 > 20"
// root cause: SQL 写死 LIMIT 20 截断, 前端 .length 当总数误读
// 修复 v1.46.0: 删 LIMIT 20 + 新增 shopTotalCount 字段 (COUNT DISTINCT)
func TestDashboardDepartmentShopListNoLimit20(t *testing.T) {
	src := readSource(t, "dashboard_department.go")
	// 不能再有 "ORDER BY sales DESC LIMIT 20" 这种 hardcode 截断
	if strings.Contains(src, "ORDER BY sales DESC LIMIT 20") {
		t.Fatalf("dashboard_department.go 不应再有 ORDER BY sales DESC LIMIT 20 截断, 这是 v1.46.0 修过的店铺数 bug")
	}
	// 必须有 shopTotalCount 字段
	if !strings.Contains(src, "shopTotalCount") {
		t.Fatal("dashboard_department.go 必须返回 shopTotalCount 字段, 区分排行截断 vs 真实总数")
	}
	// COUNT(DISTINCT shop_name) 计算真实店铺数
	if !strings.Contains(src, "COUNT(DISTINCT shop_name)") {
		t.Fatal("dashboard_department.go 必须用 COUNT(DISTINCT shop_name) 算真实店铺数")
	}
}

// Bug 4: shopBreakdown 功能 (Hover tooltip 显示 Top 5 货品 + Top 5 分类)
// 必须用 ROW_NUMBER() OVER (PARTITION BY) 取每店 Top 5, 不是简单 LIMIT
func TestShopBreakdownUsesRowNumberPartitionPerShop(t *testing.T) {
	src := readSource(t, "dashboard_overview.go")
	// CTE + ROW_NUMBER 必须在
	if !strings.Contains(src, "ROW_NUMBER() OVER (PARTITION BY s.shop_name") {
		t.Fatal("shopBreakdown 必须用 ROW_NUMBER() OVER (PARTITION BY shop_name) 取每店 Top N")
	}
	// rn <= 5 限制每店 Top 5
	if !strings.Contains(src, "rn <= 5") {
		t.Fatal("shopBreakdown 必须用 rn <= 5 限制每店 Top 5 (货品 + 分类各取 5)")
	}
	// 必须返回 shopBreakdown 字段
	if !strings.Contains(src, `"shopBreakdown":`) {
		t.Fatal("response 必须含 shopBreakdown 字段供前端 tooltip 用")
	}
	// 跨表 SELECT 含 grade 字段 (产品定位)
	if !strings.Contains(src, "g.goods_field7") {
		t.Fatal("shopBreakdown 货品 SQL 必须 LEFT JOIN goods 取 goods_field7 作为 grade")
	}
}

// Bug 5: dashboard_test.go SQL regex 跟实际 SQL 不一致导致 test FAIL
// (我刚改 SQL 加 NOT IN 后忘了同步 mock regex, 跑哥发现"测试覆盖率近为零")
// 修复: regex 用 SQL fragment "FROM sales_goods_summary" 通用匹配, 不用完整 SELECT prefix
func TestExistingDashboardTestUsesGenericSQLFragment(t *testing.T) {
	src := readSource(t, "dashboard_test.go")
	// 不应再有 SELECT department, 这种过窄的 prefix
	if strings.Contains(src, `mock.ExpectQuery("SELECT department,").WillReturnError`) {
		t.Fatal("dashboard_test.go 不应再用 'SELECT department,' 这种 prefix, 改 SQL 时容易忘了同步")
	}
	// 应该用通用 fragment
	if !strings.Contains(src, `"FROM sales_goods_summary"`) {
		t.Fatal("dashboard_test.go 推荐用 'FROM sales_goods_summary' 通用 fragment 作为 mock regex")
	}
}

// Bug 6: 综合看板 GoodsChannelExpand 平台映射不全 (视频小店掉到 'other')
// 守护: 后端 platLabelMap 包含视频号映射 (前端 getPlatform 同步在 src/components/GoodsChannelExpand.tsx)
func TestDashboardDepartmentPlatformMapHasVideoChannel(t *testing.T) {
	src := readSource(t, "dashboard_department.go")
	// "微信视频号小店" → "视频号" 的映射必须在
	if !strings.Contains(src, `"微信视频号小店": "视频号"`) {
		t.Fatal("platLabelMap 必须有 \"微信视频号小店\": \"视频号\" 映射, 否则视频号渠道掉到 '其他'")
	}
}
