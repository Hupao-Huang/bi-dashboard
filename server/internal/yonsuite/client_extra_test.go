package yonsuite

// client_extra_test.go — 重构 postJSON 公用管道后, 补齐之前没测到的几个消费方:
//   - 写路径 SavePurchaseOrder (建用友采购单, 不可逆, 二审#2 的"严格判成功"逻辑)
//   - 采购订单工具的字典查询 (组织/供应商/物料详情, code 是 json.Number 的分支)
//   - 财务凭证页消费的 QueryVoucherList / QueryAccbookList
//   - JSONString 统一字符串转换 (yonsuite 现存量 + handler 凭证页 ysMapStr 共用)
// 复用 client_http_test.go 的 mockServer / tokenJSON / newTestClient。
// 19 位 long id 的精度 (UseNumber) 是这些接口的命根子, 重点验。

import (
	"encoding/json"
	"testing"
)

// ---------- 写路径: SavePurchaseOrder (rawPost → postJSON) ----------

// 真建成: data 带回 19 位主表 id, 且不丢精度
func TestSavePurchaseOrderHappyPath(t *testing.T) {
	srv := mockServer(t, map[string]string{
		"/getAccessToken":          tokenJSON("tok", 7200),
		"purchaseorder/singleSave": `{"code":"200","message":"ok","data":{"id":1234567890123456789}}`,
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	id, _, err := c.SavePurchaseOrder(map[string]interface{}{"data": map[string]interface{}{"_status": "Insert"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if id != "1234567890123456789" {
		t.Errorf("采购订单 19 位 id 精度丢失: got %q want 1234567890123456789", id)
	}
}

// 业务失败(二审#2): code=200 但 data 里没 id → 必须判失败, 不能当建成
func TestSavePurchaseOrderBusinessFailNoID(t *testing.T) {
	srv := mockServer(t, map[string]string{
		"/getAccessToken":          tokenJSON("tok", 7200),
		"purchaseorder/singleSave": `{"code":"200","message":"ok","data":{"code":"BIZ_FAIL","message":"供应商停用"}}`,
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	id, _, err := c.SavePurchaseOrder(map[string]interface{}{"data": map[string]interface{}{}})
	if err == nil {
		t.Error("data 无 id 应判业务失败, 不能返成功")
	}
	if id != "" {
		t.Errorf("失败时 id 应为空, got %q", id)
	}
}

// 传输/鉴权级失败: code != 200 → rawPost 返 error
func TestSavePurchaseOrderNon200(t *testing.T) {
	srv := mockServer(t, map[string]string{
		"/getAccessToken":          tokenJSON("tok", 7200),
		"purchaseorder/singleSave": `{"code":"500","message":"系统异常"}`,
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, _, err := c.SavePurchaseOrder(map[string]interface{}{"data": map[string]interface{}{}})
	if err == nil {
		t.Error("非 200 code 应返 err")
	}
}

// ---------- 采购订单字典查询 ----------

// 组织字典: code 是数字 200 (json.Number 分支)
func TestQueryPurchaseOrgsHappyPath(t *testing.T) {
	srv := mockServer(t, map[string]string{
		"/getAccessToken":       tokenJSON("tok", 7200),
		"getallorgdeptbaseinfo": `{"code":200,"message":"ok","data":{"recordCount":2,"recordList":[{"code":"02030063","name":"香松"},{"code":"01","name":"集团"}]}}`,
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	orgs, err := c.QueryPurchaseOrgs()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(orgs) != 2 {
		t.Fatalf("应 2 个组织, got %d", len(orgs))
	}
	if orgs[0].Code != "02030063" || orgs[0].Name != "香松" {
		t.Errorf("组织解析错: %+v", orgs[0])
	}
}

// 数字 code 非 200 应返 err (json.Number 路径的错误分支)
func TestQueryPurchaseOrgsNon200(t *testing.T) {
	srv := mockServer(t, map[string]string{
		"/getAccessToken":       tokenJSON("tok", 7200),
		"getallorgdeptbaseinfo": `{"code":500,"message":"权限不足"}`,
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.QueryPurchaseOrgs()
	if err == nil {
		t.Error("数字 code 非 200 应返 err")
	}
}

// 供应商分页: 返回该页 + 总页数
func TestQueryVendorsPageHappyPath(t *testing.T) {
	srv := mockServer(t, map[string]string{
		"/getAccessToken": tokenJSON("tok", 7200),
		"vendor/list":     `{"code":"200","message":"ok","data":{"pageCount":3,"recordCount":250,"recordList":[{"code":"V001","name":"供应商甲"},{"code":"V002","name":"供应商乙"}]}}`,
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	vendors, pageCount, err := c.QueryVendorsPage(1, 100)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pageCount != 3 {
		t.Errorf("pageCount 应 3, got %d", pageCount)
	}
	if len(vendors) != 2 || vendors[1].Code != "V002" {
		t.Errorf("供应商解析错: %+v", vendors)
	}
}

// 物料详情: 单位三件套 + 税率从货品档案解析 (算价以此为准)
func TestQueryProductDetailsHappyPath(t *testing.T) {
	srv := mockServer(t, map[string]string{
		"/getAccessToken": tokenJSON("tok", 7200),
		"batchdetailnew":  `{"code":"200","message":"ok","data":[{"code":"02030063","unitCode":"01","detail":{"purchaseUnitCode":"01","purchasePriceUnitCode":"01","incomeTaxRates":"10004","incomeTaxRatesName":"13%增值税税率"}}]}`,
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	m, err := c.QueryProductDetails("ORG01", []string{"02030063"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	pd := m["02030063"]
	if pd == nil {
		t.Fatal("应查到物料 02030063")
	}
	if pd.TaxitemsCode != "10004" {
		t.Errorf("税目编码错: got %q want 10004", pd.TaxitemsCode)
	}
	if pd.TaxRatePct != 13 {
		t.Errorf("税率%%解析错: got %v want 13", pd.TaxRatePct)
	}
	if pd.PurUOMCode != "01" || pd.PriceUOMCode != "01" {
		t.Errorf("单位编码错: %+v", pd)
	}
}

// 空入参不打接口, 直接返空 map
func TestQueryProductDetailsEmptyInput(t *testing.T) {
	c := newTestClient(t, "http://127.0.0.1:0") // 不会被调用
	m, err := c.QueryProductDetails("", nil)
	if err != nil {
		t.Fatalf("空入参不应报错: %v", err)
	}
	if len(m) != 0 {
		t.Errorf("空入参应返空 map, got %d", len(m))
	}
}

// ---------- 财务凭证页消费方 ----------

// 凭证列表: header/body 嵌套, 19 位凭证 id 不丢精度
func TestQueryVoucherListHappyPath(t *testing.T) {
	srv := mockServer(t, map[string]string{
		"/getAccessToken": tokenJSON("tok", 7200),
		"queryVouchers": `{"code":"200","message":"ok","data":{"pageIndex":1,"pageSize":20,"recordCount":1,
			"recordList":[{"header":{"id":1987654321098765432,"period":"2026-05","displayname":"转-1"},
				"body":[{"recordnumber":"1","debit_original":100.5,"credit_original":0}]}]}}`,
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	req := &VoucherListReq{AccbookCode: "001"}
	req.Pager.PageIndex = 1
	req.Pager.PageSize = 20
	resp, err := c.QueryVoucherList(req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(resp.Data.RecordList) != 1 {
		t.Fatalf("应 1 条凭证, got %d", len(resp.Data.RecordList))
	}
	header, ok := resp.Data.RecordList[0]["header"].(map[string]interface{})
	if !ok {
		t.Fatal("header 应为 map")
	}
	// 19 位凭证 id 必须是 json.Number 且不丢精度
	if num, ok := header["id"].(json.Number); !ok || num.String() != "1987654321098765432" {
		t.Errorf("19 位凭证 id 精度丢失: %v (%T)", header["id"], header["id"])
	}
}

// accbookCode 必填守卫
func TestQueryVoucherListRequiresAccbook(t *testing.T) {
	c := newTestClient(t, "http://127.0.0.1:0")
	_, err := c.QueryVoucherList(&VoucherListReq{})
	if err == nil {
		t.Error("accbookCode 为空应返 err, 不打接口")
	}
}

// 账簿清单: code 数字 200, data 直接 array
func TestQueryAccbookListHappyPath(t *testing.T) {
	srv := mockServer(t, map[string]string{
		"/getAccessToken": tokenJSON("tok", 7200),
		"querybd/accbook": `{"code":200,"message":"ok","data":[{"code":"001","name":"香松账簿"},{"code":"002","name":"集团账簿"}]}`,
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	books, err := c.QueryAccbookList()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(books) != 2 || books[0].Code != "001" || books[1].Name != "集团账簿" {
		t.Errorf("账簿解析错: %+v", books)
	}
}

// ---------- JSONString 统一字符串转换 ----------

// 锁住语义: 标量照常转, 未知/嵌套结构返空串 (不渲染 "map[...]" 垃圾, 财务/库存展示安全)
func TestJSONString(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want string
	}{
		{"nil", nil, ""},
		{"string", "香松", "香松"},
		{"json.Number", json.Number("123"), "123"},
		{"json.Number 19位", json.Number("1234567890123456789"), "1234567890123456789"},
		{"float64", float64(1.5), "1.5"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"嵌套 map 返空", map[string]interface{}{"a": 1}, ""},
		{"嵌套 array 返空", []interface{}{1, 2}, ""},
	}
	for _, tc := range cases {
		if got := JSONString(tc.in); got != tc.want {
			t.Errorf("JSONString(%s)=%q want %q", tc.name, got, tc.want)
		}
	}
}
