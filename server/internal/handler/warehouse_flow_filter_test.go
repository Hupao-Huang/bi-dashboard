package handler

// warehouse_flow_filter_test.go — flowFilter 单元测试
// 已 Read warehouse_flow.go 第 92-152 行 + supply_chain.go planWarehouses 定义.
// 已扫 CLAUDE.md "排除 trade_type IN (8,12)" Gotcha (line 173 体现).

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// === needsGoods (源码 line 115-117) ===

func TestFlowFilterNeedsGoods(t *testing.T) {
	cases := []struct {
		name string
		f    flowFilter
		want bool
	}{
		// 源码: f.skuKw != "" || f.skuNo != ""
		{"both empty", flowFilter{}, false},
		{"skuKw only", flowFilter{skuKw: "松茸"}, true},
		{"skuNo only", flowFilter{skuNo: "03010147"}, true},
		{"both set", flowFilter{skuKw: "X", skuNo: "Y"}, true},
		// 其他字段不影响
		{"shop set, no sku", flowFilter{shop: "天猫"}, false},
		{"province set, no sku", flowFilter{province: "江苏"}, false},
		{"warehouse set, no sku", flowFilter{warehouse: "安徽郎溪"}, false},
	}
	for _, c := range cases {
		if got := c.f.needsGoods(); got != c.want {
			t.Errorf("%s: needsGoods()=%v want %v", c.name, got, c.want)
		}
	}
}

// === buildSummaryWhere (源码 line 134-152) ===
// 源码逻辑: 起手 " AND ym = ?" + ym[:4]+"-"+ym[4:], 然后按 shop/warehouse/province 顺序追加

func TestBuildSummaryWhereYMOnly(t *testing.T) {
	f := flowFilter{}
	where, args := f.buildSummaryWhere("202605")
	if where != " AND ym = ?" {
		t.Errorf("仅 ym 应只有 ym 子句, got %q", where)
	}
	// ym 格式转换: 200605 → 2026-05
	if len(args) != 1 || args[0] != "2026-05" {
		t.Errorf("ym 必须从 YYYYMM 转 YYYY-MM, got args=%v", args)
	}
}

func TestBuildSummaryWhereAllFilters(t *testing.T) {
	f := flowFilter{shop: "天猫旗舰店", warehouse: "南京", province: "江苏"}
	where, args := f.buildSummaryWhere("202605")

	// 验证每个 filter 都加进 WHERE
	for _, frag := range []string{"AND ym = ?", "AND shop_name = ?", "AND warehouse_name = ?", "AND province = ?"} {
		if !contains(where, frag) {
			t.Errorf("WHERE 缺片段 %q, got %q", frag, where)
		}
	}
	// args 顺序: ym, shop, warehouse, province
	wantArgs := []interface{}{"2026-05", "天猫旗舰店", "南京", "江苏"}
	if len(args) != len(wantArgs) {
		t.Fatalf("args 数量 want %d got %d: %v", len(wantArgs), len(args), args)
	}
	for i, w := range wantArgs {
		if args[i] != w {
			t.Errorf("args[%d] want %v got %v", i, w, args[i])
		}
	}
}

func TestBuildSummaryWhereSkipEmptyFilters(t *testing.T) {
	// shop 设置, warehouse + province 空 → 只 shop 子句加
	f := flowFilter{shop: "天猫"}
	where, args := f.buildSummaryWhere("202605")
	if !contains(where, "AND shop_name = ?") {
		t.Errorf("有 shop 应加子句, got %q", where)
	}
	if contains(where, "warehouse_name") {
		t.Errorf("warehouse 空不应加子句, got %q", where)
	}
	if contains(where, "province") {
		t.Errorf("province 空不应加子句, got %q", where)
	}
	if len(args) != 2 || args[1] != "天猫" {
		t.Errorf("args 应只有 ym + shop, got %v", args)
	}
}

// === canUseSummary (源码 line 122-130) ===

func TestCanUseSummaryGoodsFilterAlwaysFalse(t *testing.T) {
	// 源码 line 123-125: 有 SKU 过滤直接返 false, 不查 DB
	db, _, err := sqlmock.New() // 不 expect 任何 query
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	// skuKw 激活 → 直接 false
	f := flowFilter{skuKw: "松茸"}
	if f.canUseSummary(db, "202605") {
		t.Error("有 skuKw 时必须直接返 false (源码 line 124), 不应查 DB")
	}

	// skuNo 激活 → 同样
	f2 := flowFilter{skuNo: "03010147"}
	if f2.canUseSummary(db, "202605") {
		t.Error("有 skuNo 时必须直接返 false")
	}
}

func TestCanUseSummaryNoMaterializedYMFalse(t *testing.T) {
	// 源码: 查 SELECT COUNT(*) FROM warehouse_flow_summary WHERE ym=?, count=0 → false
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM warehouse_flow_summary`).
		WithArgs("2026-05").
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(0))

	f := flowFilter{}
	if f.canUseSummary(db, "202605") {
		t.Error("count=0 时应返 false (未物化)")
	}
}

func TestCanUseSummaryHasMaterializedYMTrue(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM warehouse_flow_summary`).
		WithArgs("2026-05").
		WillReturnRows(sqlmock.NewRows([]string{"cnt"}).AddRow(3624)) // 5月物化 3624 行

	f := flowFilter{}
	if !f.canUseSummary(db, "202605") {
		t.Error("count>0 时应返 true (已物化)")
	}
}

// === buildJoins (源码 line 155-161) ===
// 源码: 永远 LEFT JOIN trade_package, 仅 SKU 过滤激活时 JOIN trade_goods

func TestBuildJoinsAlwaysHasPackageLeftJoin(t *testing.T) {
	f := flowFilter{} // 无 SKU
	joins := f.buildJoins("trade_202605", "trade_goods_202605", "trade_package_202605")
	if !contains(joins, "FROM trade_202605 t LEFT JOIN trade_package_202605 p") {
		t.Errorf("无 SKU 也应永远 LEFT JOIN trade_package, got %q", joins)
	}
	if contains(joins, "trade_goods_202605") {
		t.Errorf("无 SKU 不应 JOIN trade_goods (v0.56.5 性能), got %q", joins)
	}
}

func TestBuildJoinsAddsGoodsJoinWhenSkuFilter(t *testing.T) {
	f := flowFilter{skuNo: "X"}
	joins := f.buildJoins("trade_202605", "trade_goods_202605", "trade_package_202605")
	if !contains(joins, "JOIN trade_goods_202605 g ON g.trade_id = t.trade_id") {
		t.Errorf("有 SKU 应 JOIN trade_goods, got %q", joins)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
