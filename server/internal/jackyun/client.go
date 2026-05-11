package jackyun

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Client struct {
	AppKey string
	Secret string
	APIURL string
	HTTP   *http.Client
}

func NewClient(appkey, secret, apiURL string) *Client {
	// 显式 Transport 配置 — 历史月份(2025-01 起)API 响应慢, 默认 transport TLS握手10s + Client.Timeout 120s 经常撞.
	// 实测撞点: TLSHandshakeTimeout 10s(默认)/ awaiting headers (Client.Timeout 120s)/ read body (同 Client.Timeout)
	transport := &http.Transport{
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 180 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}
	return &Client{
		AppKey: appkey,
		Secret: secret,
		APIURL: apiURL,
		HTTP:   &http.Client{Timeout: 300 * time.Second, Transport: transport},
	}
}

// APIResponse 吉客云通用返回结构
type APIResponse struct {
	Code    int             `json:"code"`
	Msg     string          `json:"msg"`
	SubCode string          `json:"subCode"`
	Result  json.RawMessage `json:"result"`
}

// Call 调用吉客云接口
func (c *Client) Call(method string, bizContent interface{}) (*APIResponse, error) {
	bizBytes, err := json.Marshal(bizContent)
	if err != nil {
		return nil, fmt.Errorf("marshal bizcontent: %w", err)
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")

	params := map[string]string{
		"method":      method,
		"appkey":      c.AppKey,
		"version":     "v1.0",
		"contenttype": "json",
		"timestamp":   timestamp,
		"bizcontent":  string(bizBytes),
	}

	// 生成签名
	params["sign"] = c.sign(params)

	// 构建 form 请求
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	resp, err := c.HTTP.PostForm(c.APIURL, form)
	if err != nil {
		return nil, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w, body: %s", err, string(body[:min(len(body), 500)]))
	}

	return &apiResp, nil
}

// sign 吉客云签名算法：
// 1. 除sign和contextid外的所有参数按key排序拼接(key+value无分隔符)
// 2. 前后加AppSecret
// 3. 整个字符串转小写
// 4. MD5加密
func (c *Client) sign(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "sign" || k == "contextid" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf strings.Builder
	buf.WriteString(c.Secret)
	for _, k := range keys {
		buf.WriteString(k)
		buf.WriteString(params[k])
	}
	buf.WriteString(c.Secret)

	// 转小写后再MD5
	lower := strings.ToLower(buf.String())
	hash := md5.Sum([]byte(lower))
	return hex.EncodeToString(hash[:])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
