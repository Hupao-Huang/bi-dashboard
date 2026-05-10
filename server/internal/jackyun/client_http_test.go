package jackyun

// client_http_test.go — jackyun SDK 4 个 fetch 接口 + Call 路径 httptest.Server mock
// 已 Read client.go (Call line 41), channel.go (FetchChannels line 105),
//          sales_summary.go (FetchSalesSummary line 96), stock_io.go (FetchStockIO line 54),
//          trade.go (FetchTrades line 161).

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 通用 mock server: 收到 form, method 字段决定返哪个 body
func newMockServer(handler func(method string) string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		method := r.FormValue("method")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(handler(method)))
	}))
}

// ---------- NewClient ----------

func TestNewClientHTTPTimeoutSet(t *testing.T) {
	c := NewClient("k", "s", "http://example.com")
	if c.AppKey != "k" || c.Secret != "s" || c.APIURL != "http://example.com" {
		t.Errorf("NewClient 字段错: %+v", c)
	}
	if c.HTTP.Timeout != 120*time.Second {
		t.Errorf("HTTP.Timeout 应 120s, got %v", c.HTTP.Timeout)
	}
}

// ---------- Call ----------

func TestCallHappyPath(t *testing.T) {
	srv := newMockServer(func(method string) string {
		return `{"code":200,"msg":"ok","subCode":"","result":{"data":[]}}`
	})
	defer srv.Close()

	c := NewClient("k", "s", srv.URL)
	resp, err := c.Call("test.method", map[string]string{"x": "y"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Code != 200 {
		t.Errorf("code=%d want 200", resp.Code)
	}
}

func TestCallInvalidJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := NewClient("k", "s", srv.URL)
	_, err := c.Call("x.y", nil)
	if err == nil {
		t.Error("无效 JSON 应返 err")
	}
}

func TestCallSignsParams(t *testing.T) {
	gotMethod := ""
	gotSign := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotMethod = r.FormValue("method")
		gotSign = r.FormValue("sign")
		_, _ = w.Write([]byte(`{"code":200,"msg":"ok"}`))
	}))
	defer srv.Close()

	c := NewClient("appkey-test", "secret-test", srv.URL)
	_, err := c.Call("erp.test", map[string]string{"a": "1"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if gotMethod != "erp.test" {
		t.Errorf("method 应 erp.test, got %q", gotMethod)
	}
	if gotSign == "" {
		t.Error("sign 应非空 (Call 必须签名)")
	}
	if len(gotSign) != 32 {
		t.Errorf("sign 应 MD5 32 hex, got len=%d", len(gotSign))
	}
}

// ---------- FetchChannels ----------

func TestFetchChannelsSinglePage(t *testing.T) {
	// 单页返 2 条 < 50 → break
	srv := newMockServer(func(method string) string {
		return `{"code":200,"msg":"ok","result":{"data":{"salesChannelInfo":[
			{"channelId":1,"channelCode":"CH001","channelName":"天猫旗舰店","channelType":1,"onlinePlatTypeName":"天猫商城"},
			{"channelId":2,"channelCode":"CH002","channelName":"京东自营","channelType":1,"onlinePlatTypeName":"京东"}
		]}}}`
	})
	defer srv.Close()

	c := NewClient("k", "s", srv.URL)
	var collected []Channel
	err := c.FetchChannels(func(items []Channel) error {
		collected = append(collected, items...)
		return nil
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(collected) != 2 {
		t.Errorf("应收到 2 个 channel, got %d", len(collected))
	}
	if collected[0].ChannelCode != "CH001" {
		t.Errorf("channelCode 错: %q", collected[0].ChannelCode)
	}
}

func TestFetchChannelsAPIErrorCode(t *testing.T) {
	srv := newMockServer(func(method string) string {
		return `{"code":500,"msg":"server error"}`
	})
	defer srv.Close()

	c := NewClient("k", "s", srv.URL)
	err := c.FetchChannels(func(items []Channel) error { return nil })
	if err == nil {
		t.Error("非 200 code 应返 err")
	}
}

func TestFetchChannelsCallbackError(t *testing.T) {
	srv := newMockServer(func(method string) string {
		return `{"code":200,"msg":"ok","result":{"data":{"salesChannelInfo":[{"channelId":1,"channelCode":"X"}]}}}`
	})
	defer srv.Close()

	c := NewClient("k", "s", srv.URL)
	err := c.FetchChannels(func(items []Channel) error {
		return assertErr("callback failed")
	})
	if err == nil {
		t.Error("callback 返 err 应传出")
	}
}

// ---------- FetchSalesSummary ----------

func TestFetchSalesSummarySinglePage(t *testing.T) {
	srv := newMockServer(func(method string) string {
		return `{"code":200,"msg":"ok","result":{"data":[
			{"shopId":"SH001","shopName":"天猫店"},
			{"shopId":"SH002","shopName":"京东店"}
		]}}`
	})
	defer srv.Close()

	c := NewClient("k", "s", srv.URL)
	var collected []SalesSummaryItem
	err := c.FetchSalesSummary(SalesSummaryQuery{TimeType: 3, StartTime: "2026-04-01"}, func(items []SalesSummaryItem) error {
		collected = append(collected, items...)
		return nil
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(collected) != 2 {
		t.Errorf("应收到 2 条, got %d", len(collected))
	}
}

func TestFetchSalesSummaryEmptyDataBreaks(t *testing.T) {
	srv := newMockServer(func(method string) string {
		return `{"code":200,"msg":"ok","result":{"data":[]}}`
	})
	defer srv.Close()

	c := NewClient("k", "s", srv.URL)
	called := 0
	err := c.FetchSalesSummary(SalesSummaryQuery{TimeType: 3}, func(items []SalesSummaryItem) error {
		called++
		return nil
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if called != 0 {
		t.Errorf("空 data 不应回调 callback (line 125 break), called=%d", called)
	}
}

// ---------- FetchStockIO ----------

func TestFetchStockIOSinglePage(t *testing.T) {
	srv := newMockServer(func(method string) string {
		return `{"code":200,"msg":"ok","result":{"data":[
			{"goodsdocNo":"DOC001","goodsName":"商品A","quantity":100},
			{"goodsdocNo":"DOC002","goodsName":"商品B","quantity":50}
		]}}`
	})
	defer srv.Close()

	c := NewClient("k", "s", srv.URL)
	var collected []StockIOItem
	err := c.FetchStockIO("erp.in.get", StockIOQuery{}, func(items []StockIOItem) error {
		collected = append(collected, items...)
		return nil
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(collected) != 2 {
		t.Errorf("应收到 2 个 stock io, got %d", len(collected))
	}
}

func TestFetchStockIOAPIError(t *testing.T) {
	srv := newMockServer(func(method string) string {
		return `{"code":403,"msg":"无权限"}`
	})
	defer srv.Close()

	c := NewClient("k", "s", srv.URL)
	err := c.FetchStockIO("erp.in.get", StockIOQuery{}, func(items []StockIOItem) error { return nil })
	if err == nil {
		t.Error("403 应返 err")
	}
}

// ---------- FetchTrades ----------

func TestFetchTradesSinglePageWithTotal(t *testing.T) {
	// TotalResults=2, 单页返 2 条 → fetched>=total break
	srv := newMockServer(func(method string) string {
		return `{"code":200,"msg":"ok","result":{"data":{
			"totalResults":2,
			"trades":[
				{"tradeNo":"T001","shopName":"天猫店"},
				{"tradeNo":"T002","shopName":"京东店"}
			],
			"scrollId":""
		}}}`
	})
	defer srv.Close()

	c := NewClient("k", "s", srv.URL)
	var collected []Trade
	start := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 30, 23, 59, 59, 0, time.UTC)
	err := c.FetchTrades(start, end, func(items []Trade) error {
		collected = append(collected, items...)
		return nil
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(collected) != 2 {
		t.Errorf("应收到 2 条 trade, got %d", len(collected))
	}
}

func TestFetchTradesEmptyData(t *testing.T) {
	srv := newMockServer(func(method string) string {
		return `{"code":200,"msg":"ok","result":{"data":{"totalResults":0,"trades":[]}}}`
	})
	defer srv.Close()

	c := NewClient("k", "s", srv.URL)
	called := 0
	err := c.FetchTrades(time.Now(), time.Now(), func(items []Trade) error {
		called++
		return nil
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if called != 0 {
		t.Errorf("空 trades 不应回调 callback")
	}
}

func TestFetchTradesProgressFn(t *testing.T) {
	srv := newMockServer(func(method string) string {
		return `{"code":200,"msg":"ok","result":{"data":{
			"totalResults":3,
			"trades":[{"tradeNo":"T01"},{"tradeNo":"T02"},{"tradeNo":"T03"}]
		}}}`
	})
	defer srv.Close()

	c := NewClient("k", "s", srv.URL)
	progressCalled := false
	var lastFetched, lastTotal int
	err := c.FetchTrades(time.Now(), time.Now(),
		func([]Trade) error { return nil },
		func(fetched, total int) {
			progressCalled = true
			lastFetched = fetched
			lastTotal = total
		})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !progressCalled {
		t.Error("progressFn 应被调用")
	}
	if lastFetched != 3 || lastTotal != 3 {
		t.Errorf("fetched/total: %d/%d, want 3/3", lastFetched, lastTotal)
	}
}

// helper
type assertErr string

func (e assertErr) Error() string { return string(e) }

// 验证 sign 有 sign Excludes contextid (line 94)
func TestSignExcludesContextid(t *testing.T) {
	c := NewClient("k", "secret", "http://x")
	withCtx := c.sign(map[string]string{"a": "1", "contextid": "ignored"})
	withoutCtx := c.sign(map[string]string{"a": "1"})
	if withCtx != withoutCtx {
		t.Errorf("contextid 应被排除签名, got with=%s without=%s", withCtx, withoutCtx)
	}
}

func TestMinHelperFn(t *testing.T) {
	// min 是 client.go line 115 的 helper
	cases := []struct{ a, b, want int }{
		{1, 2, 1},
		{5, 3, 3},
		{0, 0, 0},
		{-1, 1, -1},
	}
	for _, c := range cases {
		if got := min(c.a, c.b); got != c.want {
			t.Errorf("min(%d,%d)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}
