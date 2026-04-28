// Package yonsuite 用友 YonBIP 开放平台 client
// 文档: https://c3.yonyoucloud.com (用友 iuap 开放平台)
//
// 认证流程:
//  1. selfAppAuth/base/v1/getAccessToken (HmacSHA256 签名) → access_token (2h 有效期)
//  2. 业务接口 (POST application/json) → query 带 access_token
//
// 签名算法 (getAccessToken 用):
//   sign = Base64(HmacSHA256(parameterMap, AppSecret))
//   parameterMap 按 key 排序后 key+value 直接拼接 (除 signature 外)
//
// Token cache: 内存 cache，过期前 5min 主动刷新。
package yonsuite

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// authPath 自建应用获取 access_token (新版 base/v1)
	authPath = "/iuap-api-auth/open-auth/selfAppAuth/base/v1/getAccessToken"
	// purchaseListPath 采购订单列表查询
	purchaseListPath = "/iuap-api-gateway/yonbip/scm/purchaseorder/list"
	// subcontractListPath 委外订单列表查询
	subcontractListPath = "/iuap-api-gateway/yonbip/mfg/subcontractorder/list"

	// tokenSafetyMargin token 提前 5min 过期，避免边界条件
	tokenSafetyMargin = 5 * time.Minute

	// defaultMinInterval YS 免费标准配置限流 60 次/分钟 = 1 次/秒
	// 默认间隔 1.1s 留 10% 余量，避免边界踩限流
	// 付费版可通过 SetMinInterval(0) 关闭节流
	defaultMinInterval = 1100 * time.Millisecond
)

// Client YS 开放平台客户端
type Client struct {
	AppKey    string
	AppSecret string
	BaseURL   string
	HTTP      *http.Client

	tokenMu     sync.Mutex
	accessToken string
	tokenExpire time.Time

	// 限流: 所有 HTTP 调用 (getToken + 业务接口) 共用一个 rate limiter
	rateMu       sync.Mutex
	lastCallTime time.Time
	minInterval  time.Duration
}

// NewClient 构造 client (默认节流 1.1s/次, 即 < 60 次/分钟)
func NewClient(appkey, appsecret, baseURL string) *Client {
	return &Client{
		AppKey:      appkey,
		AppSecret:   appsecret,
		BaseURL:     strings.TrimRight(baseURL, "/"),
		HTTP:        &http.Client{Timeout: 60 * time.Second},
		minInterval: defaultMinInterval,
	}
}

// SetMinInterval 设置 HTTP 调用最小间隔 (限流保护)
// d=0 关闭节流 (付费版无限流时使用)
func (c *Client) SetMinInterval(d time.Duration) {
	c.rateMu.Lock()
	defer c.rateMu.Unlock()
	c.minInterval = d
}

// waitRateLimit 阻塞直到距上次调用 ≥ minInterval
// 所有 HTTP 调用 (getToken + 业务接口) 入口都调用此方法
func (c *Client) waitRateLimit() {
	c.rateMu.Lock()
	defer c.rateMu.Unlock()
	if c.minInterval <= 0 {
		c.lastCallTime = time.Now()
		return
	}
	elapsed := time.Since(c.lastCallTime)
	if elapsed < c.minInterval {
		time.Sleep(c.minInterval - elapsed)
	}
	c.lastCallTime = time.Now()
}

// AccessToken 获取 access_token (cache 命中直接返回，过期主动刷新)
func (c *Client) AccessToken() (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.tokenExpire) {
		return c.accessToken, nil
	}
	return c.refreshTokenLocked()
}

// refreshTokenLocked 实际拉 token (必须在 tokenMu 锁内调用)
func (c *Client) refreshTokenLocked() (string, error) {
	c.waitRateLimit()

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	params := map[string]string{
		"appKey":    c.AppKey,
		"timestamp": timestamp,
	}
	signature := c.sign(params)

	q := url.Values{}
	q.Set("appKey", c.AppKey)
	q.Set("timestamp", timestamp)
	q.Set("signature", signature)

	fullURL := c.BaseURL + authPath + "?" + q.Encode()

	resp, err := c.HTTP.Get(fullURL)
	if err != nil {
		return "", fmt.Errorf("yonsuite getAccessToken http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token body: %w", err)
	}

	var tr struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Data    struct {
			AccessToken string `json:"access_token"`
			Expire      int    `json:"expire"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("unmarshal token: %w, body=%s", err, truncate(string(body), 500))
	}
	if tr.Code != "00000" {
		return "", fmt.Errorf("yonsuite token failed: code=%s msg=%s", tr.Code, tr.Message)
	}
	if tr.Data.AccessToken == "" {
		return "", fmt.Errorf("yonsuite empty access_token: body=%s", truncate(string(body), 500))
	}

	c.accessToken = tr.Data.AccessToken
	c.tokenExpire = time.Now().Add(time.Duration(tr.Data.Expire)*time.Second - tokenSafetyMargin)
	return c.accessToken, nil
}

// sign HmacSHA256(parameterMap, AppSecret) → Base64
// 参数按 key 排序后 key+value 直接拼接 (除 signature 外)
func (c *Client) sign(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "signature" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf strings.Builder
	for _, k := range keys {
		buf.WriteString(k)
		buf.WriteString(params[k])
	}

	h := hmac.New(sha256.New, []byte(c.AppSecret))
	h.Write([]byte(buf.String()))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// SimpleVO 查询条件 (op: eq/neq/lt/gt/between/in/nin/like/leftlike/rightlike/is_null/is_not_null/and/or)
type SimpleVO struct {
	Field  string `json:"field"`
	Op     string `json:"op"`
	Value1 string `json:"value1"`
	Value2 string `json:"value2,omitempty"` // for between
}

// QueryOrder 排序字段
type QueryOrder struct {
	Field string `json:"field"`
	Order string `json:"order"` // asc / desc
}

// PurchaseListReq 采购订单列表请求
type PurchaseListReq struct {
	PageIndex   int          `json:"pageIndex"`
	PageSize    int          `json:"pageSize"`
	IsSum       bool         `json:"isSum"`
	SimpleVOs   []SimpleVO   `json:"simpleVOs"`
	QueryOrders []QueryOrder `json:"queryOrders"`
}

// PurchaseListResp 采购订单列表返回
// recordList 用 map[string]interface{} 接收，保留所有原始字段供入库时序列化为 raw_json
type PurchaseListResp struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		PageIndex   int                      `json:"pageIndex"`
		PageSize    int                      `json:"pageSize"`
		RecordCount int                      `json:"recordCount"`
		PageCount   int                      `json:"pageCount"`
		RecordList  []map[string]interface{} `json:"recordList"`
	} `json:"data"`
}

// QueryPurchaseList 调采购订单列表接口 (POST + access_token query)
func (c *Client) QueryPurchaseList(req *PurchaseListReq) (*PurchaseListResp, error) {
	token, err := c.AccessToken()
	if err != nil {
		return nil, err
	}

	c.waitRateLimit()

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal purchase list req: %w", err)
	}

	q := url.Values{}
	q.Set("access_token", token)
	fullURL := c.BaseURL + purchaseListPath + "?" + q.Encode()

	httpReq, err := http.NewRequest("POST", fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("yonsuite purchase list http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	// 关键: 用 UseNumber() 防止 19 位 id (long) 被默认 float64 截断精度
	// (YS 主表 id / 行 id 都是 19 位, float64 只能精确表示 ~16 位整数, 不用 Number 会导致 UK 撞车)
	var pr PurchaseListResp
	dec := json.NewDecoder(bytes.NewReader(respBody))
	dec.UseNumber()
	if err := dec.Decode(&pr); err != nil {
		return nil, fmt.Errorf("unmarshal purchase list: %w, body=%s", err, truncate(string(respBody), 500))
	}
	if pr.Code != "200" {
		return nil, fmt.Errorf("yonsuite purchase list non-200: code=%s msg=%s", pr.Code, pr.Message)
	}
	return &pr, nil
}

// QuerySubcontractList 调委外订单列表接口 (POST + access_token query)
// 实测: vouchdate top-level filter 不工作, 用 simpleVOs vouchdate between 模式
// 复用 PurchaseListReq/Resp 结构 (字段一致)
func (c *Client) QuerySubcontractList(req *PurchaseListReq) (*PurchaseListResp, error) {
	token, err := c.AccessToken()
	if err != nil {
		return nil, err
	}
	c.waitRateLimit()

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal subcontract req: %w", err)
	}

	q := url.Values{}
	q.Set("access_token", token)
	fullURL := c.BaseURL + subcontractListPath + "?" + q.Encode()

	httpReq, err := http.NewRequest("POST", fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("yonsuite subcontract list http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var pr PurchaseListResp
	dec := json.NewDecoder(bytes.NewReader(respBody))
	dec.UseNumber()
	if err := dec.Decode(&pr); err != nil {
		return nil, fmt.Errorf("unmarshal subcontract list: %w, body=%s", err, truncate(string(respBody), 500))
	}
	if pr.Code != "200" {
		return nil, fmt.Errorf("yonsuite subcontract non-200: code=%s msg=%s", pr.Code, pr.Message)
	}
	return &pr, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
