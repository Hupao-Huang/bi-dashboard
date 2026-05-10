package yonsuite

// client_http_test.go — yonsuite SDK 4 个业务接口 + AccessToken 路径 httptest.Server mock
// 已 Read client.go 全文:
//   - authPath line 34 (token 路径)
//   - 4 业务接口 line 35-42 (purchase/subcontract/materialOut/stock)
//   - AccessToken (106) + refreshTokenLocked (117) cache + safety margin
//   - QueryStockList (386) data 是直接 array (不是 data.recordList)
// 测试时必须 SetMinInterval(0) 关 1.1s 节流, 否则 test 卡死.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 通用 mock server: 路径 → 返回的 JSON
func mockServer(t *testing.T, handlers map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for path, body := range handlers {
			if strings.Contains(r.URL.Path, path) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
				return
			}
		}
		http.Error(w, "no mock for "+r.URL.Path, 404)
	}))
}

func tokenJSON(token string, expireSec int) string {
	b, _ := json.Marshal(map[string]interface{}{
		"code":    "00000",
		"message": "ok",
		"data":    map[string]interface{}{"access_token": token, "expire": expireSec},
	})
	return string(b)
}

func newTestClient(t *testing.T, baseURL string) *Client {
	c := NewClient("test-app-key", "test-app-secret", baseURL)
	c.SetMinInterval(0) // 关节流, 否则 1.1s 卡 test
	return c
}

// ---------- AccessToken ----------

func TestAccessTokenHappyPath(t *testing.T) {
	srv := mockServer(t, map[string]string{
		"/getAccessToken": tokenJSON("token-abc-123", 7200),
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	tok, err := c.AccessToken()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if tok != "token-abc-123" {
		t.Errorf("token=%q want token-abc-123", tok)
	}
	// expire = 7200s, safety margin 5min, 应有效到 ~6900s 后
	if !c.tokenExpire.After(time.Now().Add(time.Hour)) {
		t.Errorf("tokenExpire 应在 1h 后, got %v", c.tokenExpire)
	}
}

func TestAccessTokenCacheHit(t *testing.T) {
	// 第一次拉, 第二次应直接 cache 命中, 不再请求
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(tokenJSON("cached-token", 7200)))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	t1, _ := c.AccessToken()
	t2, _ := c.AccessToken()
	if t1 != t2 {
		t.Errorf("两次 AccessToken 应一致, got %q vs %q", t1, t2)
	}
	if hits != 1 {
		t.Errorf("应只发 1 次 token 请求 (cache hit), 实际 %d", hits)
	}
}

func TestAccessTokenErrorCode(t *testing.T) {
	// code != "00000" 应返 err
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"INVALID_APP_KEY","message":"appkey 错误"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.AccessToken()
	if err == nil {
		t.Error("非 00000 code 应返 err")
	}
	if !strings.Contains(err.Error(), "INVALID_APP_KEY") {
		t.Errorf("err 应含 code, got %v", err)
	}
}

func TestAccessTokenEmptyAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"00000","message":"ok","data":{"access_token":"","expire":7200}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.AccessToken()
	if err == nil {
		t.Error("空 access_token 应返 err")
	}
}

func TestAccessTokenInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.AccessToken()
	if err == nil {
		t.Error("无效 JSON 应返 err")
	}
}

// ---------- QueryPurchaseList ----------

func TestQueryPurchaseListHappyPath(t *testing.T) {
	srv := mockServer(t, map[string]string{
		"/getAccessToken": tokenJSON("tok", 7200),
		"/purchaseorder/list": `{
			"code":"200","message":"ok",
			"data":{"pageIndex":1,"pageSize":100,"recordCount":2,"pageCount":1,
				"recordList":[
					{"id":1234567890123456789,"code":"PO-001","amount":99.99},
					{"id":9876543210987654321,"code":"PO-002","amount":50.00}
				]
			}
		}`,
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	resp, err := c.QueryPurchaseList(&PurchaseListReq{PageIndex: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Code != "200" {
		t.Errorf("code=%q want 200", resp.Code)
	}
	if len(resp.Data.RecordList) != 2 {
		t.Errorf("recordList 应 2 条, got %d", len(resp.Data.RecordList))
	}
	// 验证 19 位 long id 用 UseNumber 接住, 不丢精度 (memory feedback_go_long_id_precision)
	id := resp.Data.RecordList[0]["id"]
	if num, ok := id.(json.Number); ok {
		if num.String() != "1234567890123456789" {
			t.Errorf("19 位 id 精度丢失: %s != 1234567890123456789", num.String())
		}
	} else {
		t.Errorf("id 应为 json.Number, 实际 %T", id)
	}
}

func TestQueryPurchaseListNon200(t *testing.T) {
	srv := mockServer(t, map[string]string{
		"/getAccessToken":     tokenJSON("tok", 7200),
		"/purchaseorder/list": `{"code":"500","message":"server error"}`,
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.QueryPurchaseList(&PurchaseListReq{})
	if err == nil {
		t.Error("非 200 code 应返 err")
	}
}

func TestQueryPurchaseListTokenFails(t *testing.T) {
	// token 路径返 error code, purchase 不应被调
	srv := mockServer(t, map[string]string{
		"/getAccessToken": `{"code":"BAD","message":"invalid"}`,
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.QueryPurchaseList(&PurchaseListReq{})
	if err == nil {
		t.Error("token 失败应连带 PurchaseList 失败")
	}
}

// ---------- QuerySubcontractList ----------

func TestQuerySubcontractListHappyPath(t *testing.T) {
	srv := mockServer(t, map[string]string{
		"/getAccessToken":      tokenJSON("tok", 7200),
		"/subcontractorder/list": `{
			"code":"200","message":"ok",
			"data":{"pageIndex":1,"pageSize":100,"recordCount":1,"pageCount":1,
				"recordList":[{"id":"1","code":"SC-001"}]
			}
		}`,
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	resp, err := c.QuerySubcontractList(&PurchaseListReq{PageIndex: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(resp.Data.RecordList) != 1 {
		t.Errorf("recordList 应 1 条, got %d", len(resp.Data.RecordList))
	}
}

// ---------- QueryMaterialOutList ----------

func TestQueryMaterialOutListHappyPath(t *testing.T) {
	srv := mockServer(t, map[string]string{
		"/getAccessToken":  tokenJSON("tok", 7200),
		"/materialout/list": `{
			"code":"200","message":"ok",
			"data":{"pageIndex":1,"pageSize":100,"recordCount":3,"pageCount":1,
				"recordList":[{"id":"1"},{"id":"2"},{"id":"3"}]
			}
		}`,
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	resp, err := c.QueryMaterialOutList(&PurchaseListReq{PageIndex: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(resp.Data.RecordList) != 3 {
		t.Errorf("recordList 应 3 条, got %d", len(resp.Data.RecordList))
	}
}

// ---------- QueryStockList (data 是直接 array, 不是 data.recordList) ----------

func TestQueryStockListHappyPath(t *testing.T) {
	// memory feedback_ys_stock_data_array: 现存量 data 是直接 array
	srv := mockServer(t, map[string]string{
		"/getAccessToken":                       tokenJSON("tok", 7200),
		"/QueryCurrentStocksByCondition": `{
			"code":"200","message":"ok",
			"data":[
				{"productCode":"03030236","stock":1000},
				{"productCode":"03030237","stock":2000}
			]
		}`,
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	resp, err := c.QueryStockList(&StockListReq{PageIndex: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Errorf("Data 应 2 条 (直接 array), got %d", len(resp.Data))
	}
	if resp.Data[0]["productCode"] != "03030236" {
		t.Errorf("productCode 解析错: %v", resp.Data[0]["productCode"])
	}
}

func TestQueryStockListEmptyResponse(t *testing.T) {
	srv := mockServer(t, map[string]string{
		"/getAccessToken":                       tokenJSON("tok", 7200),
		"/QueryCurrentStocksByCondition": `{"code":"200","message":"ok","data":[]}`,
	})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	resp, err := c.QueryStockList(&StockListReq{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("空 array 应 len=0, got %d", len(resp.Data))
	}
}

// ---------- AccessToken http 5xx error ----------

func TestAccessToken5xxRetainsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, `{"code":"INTERNAL","message":"fail"}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.AccessToken()
	if err == nil {
		t.Error("5xx 应连同 code 一起返 err (源码会先解析 body, code != 00000)")
	}
}
