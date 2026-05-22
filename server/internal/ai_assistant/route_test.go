package ai_assistant

// W3a 路由单元测试: mock Intent 喂给 route(), 验证各 module SQL 跑通 + 返回结构合法
// Z.AI 欠费状态下无法测 LLM, 此测试只覆盖路由层
// 跑法: cd server && go test ./internal/ai_assistant/... -run TestRoute
// 依赖 bi_dashboard 真 DB, 连不上时自动 t.Skip

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// openTestDB 读 server/config.json 的 DSN; 连不上则 skip
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	configPath := os.Getenv("BI_CONFIG")
	if configPath == "" {
		configPath = "../../config.json"
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Skipf("config.json 读不到, skip integration test: %v", err)
		return nil
	}
	var cfg struct {
		Database struct {
			Host     string `json:"host"`
			Port     int    `json:"port"`
			User     string `json:"user"`
			Password string `json:"password"`
			DBName   string `json:"dbname"`
		} `json:"database"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Skipf("config.json 解析失败: %v", err)
		return nil
	}
	dsn := cfg.Database.User + ":" + cfg.Database.Password + "@tcp(" + cfg.Database.Host + ":" + itoa(cfg.Database.Port) + ")/" + cfg.Database.DBName + "?parseTime=true&charset=utf8mb4"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("DB open 失败: %v", err)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("DB ping 失败 (可能是 DB 未启): %v", err)
		return nil
	}
	return db
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestRoute_Department_OK(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	svc := &Service{DB: db}
	intent := &Intent{
		Type:   "see",
		Module: "department",
		Params: map[string]string{"dept": "ecommerce", "start": "2026-05-01", "end": "2026-05-21"},
	}
	data, api, err := svc.route(context.Background(), intent)
	if err != nil {
		t.Fatalf("route failed: %v", err)
	}
	if api == "" {
		t.Fatal("api path 为空")
	}
	m, ok := data.(map[string]interface{})
	if !ok {
		t.Fatalf("data 不是 map: %T", data)
	}
	if _, ok := m["totalAmount"]; !ok {
		t.Fatal("缺 totalAmount 字段")
	}
}

func TestRoute_Department_InvalidDept(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	svc := &Service{DB: db}
	intent := &Intent{
		Type:   "see",
		Module: "department",
		Params: map[string]string{"dept": "xxx_hack"},
	}
	_, _, err := svc.route(context.Background(), intent)
	if err == nil {
		t.Fatal("非法 dept 应当报错, 实际 nil")
	}
}

func TestRoute_Overview(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	svc := &Service{DB: db}
	intent := &Intent{Type: "see", Module: "overview", Params: map[string]string{}}
	data, _, err := svc.route(context.Background(), intent)
	if err != nil {
		t.Fatalf("route failed: %v", err)
	}
	m := data.(map[string]interface{})
	if _, ok := m["byDept"]; !ok {
		t.Fatal("缺 byDept 字段")
	}
}

func TestRoute_ShopRank(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	svc := &Service{DB: db}
	intent := &Intent{
		Type:   "rank",
		Module: "shop_rank",
		Params: map[string]string{"start": "2026-05-01", "end": "2026-05-21", "limit": "5", "order": "DESC"},
	}
	data, _, err := svc.route(context.Background(), intent)
	if err != nil {
		t.Fatalf("route failed: %v", err)
	}
	m := data.(map[string]interface{})
	if _, ok := m["rank"]; !ok {
		t.Fatal("缺 rank 字段")
	}
}

func TestRoute_ShopRank_WithDept(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	svc := &Service{DB: db}
	intent := &Intent{
		Type:   "rank",
		Module: "shop_rank",
		Params: map[string]string{"dept": "ecommerce", "limit": "3", "order": "ASC"},
	}
	_, _, err := svc.route(context.Background(), intent)
	if err != nil {
		t.Fatalf("route failed: %v", err)
	}
}

func TestRoute_ProductRank_Dims(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	svc := &Service{DB: db}
	for _, dim := range []string{"goods", "cate", "sku", "brand", ""} {
		t.Run("dim="+dim, func(t *testing.T) {
			intent := &Intent{
				Type:   "rank",
				Module: "product_rank",
				Params: map[string]string{"dim": dim, "limit": "5"},
			}
			_, _, err := svc.route(context.Background(), intent)
			if err != nil {
				t.Fatalf("dim=%s route failed: %v", dim, err)
			}
		})
	}
}

func TestRoute_ProductRank_InvalidDim(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	svc := &Service{DB: db}
	intent := &Intent{
		Type:   "rank",
		Module: "product_rank",
		Params: map[string]string{"dim": "drop_table"},
	}
	_, _, err := svc.route(context.Background(), intent)
	if err == nil {
		t.Fatal("非法 dim 应当报错")
	}
}

func TestRoute_Trend(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	svc := &Service{DB: db}
	intent := &Intent{
		Type:   "trend",
		Module: "trend",
		Params: map[string]string{
			"this_start": "2026-05-15", "this_end": "2026-05-21",
			"prev_start": "2026-05-08", "prev_end": "2026-05-14",
		},
	}
	data, _, err := svc.route(context.Background(), intent)
	if err != nil {
		t.Fatalf("route failed: %v", err)
	}
	m := data.(map[string]interface{})
	for _, k := range []string{"thisAmount", "prevAmount", "deltaAmountPct"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("缺字段 %s", k)
		}
	}
}

func TestRoute_Trend_MissingParams(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	svc := &Service{DB: db}
	intent := &Intent{
		Type:   "trend",
		Module: "trend",
		Params: map[string]string{"this_start": "2026-05-15"},
	}
	_, _, err := svc.route(context.Background(), intent)
	if err == nil {
		t.Fatal("缺 prev_start/prev_end 应当报错")
	}
}

func TestRoute_StockWarning(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	svc := &Service{DB: db}
	intent := &Intent{
		Type:   "see",
		Module: "stock_warning",
		Params: map[string]string{"limit": "5"},
	}
	data, _, err := svc.route(context.Background(), intent)
	if err != nil {
		t.Fatalf("route failed: %v", err)
	}
	m := data.(map[string]interface{})
	if _, ok := m["stockoutCount"]; !ok {
		t.Fatal("缺 stockoutCount 字段")
	}
}

func TestRoute_WarehouseFlow(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	svc := &Service{DB: db}
	intent := &Intent{
		Type:   "see",
		Module: "warehouse_flow",
		Params: map[string]string{"ym": "2026-05"},
	}
	data, _, err := svc.route(context.Background(), intent)
	if err != nil {
		t.Fatalf("route failed: %v", err)
	}
	m := data.(map[string]interface{})
	if _, ok := m["orders"]; !ok {
		t.Fatal("缺 orders 字段")
	}
}

func TestRoute_WarehouseFlow_BadYm(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	svc := &Service{DB: db}
	intent := &Intent{
		Type:   "see",
		Module: "warehouse_flow",
		Params: map[string]string{"ym": "2026-13-99"},
	}
	_, _, err := svc.route(context.Background(), intent)
	if err == nil {
		t.Fatal("非法 ym 应当报错")
	}
}

func TestRoute_RPAStatus_AllPlatforms(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	svc := &Service{DB: db}
	intent := &Intent{
		Type:   "see",
		Module: "rpa_status",
		Params: map[string]string{},
	}
	data, _, err := svc.route(context.Background(), intent)
	if err != nil {
		t.Fatalf("route failed: %v", err)
	}
	m := data.(map[string]interface{})
	items, ok := m["items"].([]item_rpaStatus)
	_ = items
	_ = ok
	// 不强校验类型 (Go 反射后 []struct → interface), 只要 items 字段存在即可
	if _, ok := m["items"]; !ok {
		t.Fatal("缺 items 字段")
	}
}

// 占位类型 (因为 routeRPAStatus 内部 type item struct 是局部)
type item_rpaStatus struct {
	Platform string `json:"platform"`
	Table    string `json:"table"`
	LastDate string `json:"lastDate"`
}

func TestRoute_RPAStatus_OnePlatform(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	svc := &Service{DB: db}
	intent := &Intent{
		Type:   "see",
		Module: "rpa_status",
		Params: map[string]string{"platform": "tmall"},
	}
	_, _, err := svc.route(context.Background(), intent)
	if err != nil {
		t.Fatalf("route failed: %v", err)
	}
}

func TestRoute_RPAStatus_InvalidPlatform(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	svc := &Service{DB: db}
	intent := &Intent{
		Type:   "see",
		Module: "rpa_status",
		Params: map[string]string{"platform": "../../etc/passwd"},
	}
	_, _, err := svc.route(context.Background(), intent)
	if err == nil {
		t.Fatal("非法 platform 应当报错")
	}
}

func TestRoute_UnknownModule(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	svc := &Service{DB: db}
	intent := &Intent{Type: "see", Module: "i_dont_exist", Params: map[string]string{}}
	_, _, err := svc.route(context.Background(), intent)
	if err == nil {
		t.Fatal("未知 module 应当报错")
	}
}

func TestClampLimit(t *testing.T) {
	cases := []struct {
		raw  string
		def  int
		max  int
		want int
	}{
		{"", 10, 50, 10},
		{"5", 10, 50, 5},
		{"100", 10, 50, 50},
		{"-3", 10, 50, 1},
		{"abc", 10, 50, 10},
	}
	for _, c := range cases {
		got := clampLimit(c.raw, c.def, c.max)
		if got != c.want {
			t.Errorf("clampLimit(%q,%d,%d) = %d, want %d", c.raw, c.def, c.max, got, c.want)
		}
	}
}
