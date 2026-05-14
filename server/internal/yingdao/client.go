// Package yingdao 影刀 RPA 开放 API 客户端
//
// 文档: https://www.yingdao.com/yddoc/rpa/zh-CN
//
// 接入路径:
//   1. GET /oapi/token/v2/token/create (api.yingdao.com) → access_token (2h 有效期)
//   2. 业务接口 (POST application/json): Authorization: Bearer <accessToken>
//      - 启动应用: /oapi/dispatch/v2/job/start (api.yingdao.com)
//      - 查询应用结果: /oapi/dispatch/v2/job/query (api.yingdao.com)
//      - 通知日志: /oapi/dispatch/v2/job/log/notify (api.yingdao.com)
//      - 轮询日志: /oapi/dispatch/v2/job/log/query?requestId=xxx (api.yingdao.com, GET)
//      - 任务列表: /oapi/dispatch/v2/schedule/list (api.winrobot360.com)
//      - 任务详情: /oapi/dispatch/v2/schedule/detail (api.winrobot360.com)
//
// Token cache: 内存 cache, 提前 5 分钟过期主动刷新.
package yingdao

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	tokenSafetyMargin = 5 * time.Minute
	defaultHTTPTimeout = 30 * time.Second
)

// Client 影刀客户端
type Client struct {
	AccessKeyID     string
	AccessKeySecret string
	AuthURL         string // https://api.yingdao.com
	BizURL          string // https://api.winrobot360.com
	DefaultAccount  string // 默认机器人账号 lhx@sxx
	HTTP            *http.Client

	tokenMu     sync.Mutex
	accessToken string
	tokenExpire time.Time
}

// NewClient 构造影刀客户端
func NewClient(accessKeyID, accessKeySecret, authURL, bizURL, defaultAccount string) *Client {
	if authURL == "" {
		authURL = "https://api.yingdao.com"
	}
	if bizURL == "" {
		bizURL = "https://api.winrobot360.com"
	}
	return &Client{
		AccessKeyID:     accessKeyID,
		AccessKeySecret: accessKeySecret,
		AuthURL:         strings.TrimRight(authURL, "/"),
		BizURL:          strings.TrimRight(bizURL, "/"),
		DefaultAccount:  defaultAccount,
		HTTP:            &http.Client{Timeout: defaultHTTPTimeout},
	}
}

// Configured 是否配置可用 (空配置时拒绝调用)
func (c *Client) Configured() bool {
	return c != nil && c.AccessKeyID != "" && c.AccessKeySecret != ""
}

// AccessToken 获取 access_token (cache 命中直接返回, 过期主动刷新)
func (c *Client) AccessToken(ctx context.Context) (string, error) {
	if !c.Configured() {
		return "", fmt.Errorf("yingdao client not configured")
	}
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.tokenExpire) {
		return c.accessToken, nil
	}
	return c.refreshTokenLocked(ctx)
}

// refreshTokenLocked 实际拉 token (必须在 tokenMu 锁内调用)
func (c *Client) refreshTokenLocked(ctx context.Context) (string, error) {
	q := url.Values{}
	q.Set("accessKeyId", c.AccessKeyID)
	q.Set("accessKeySecret", c.AccessKeySecret)
	fullURL := c.AuthURL + "/oapi/token/v2/token/create?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("yingdao token http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token body: %w", err)
	}

	var tr struct {
		Code    int    `json:"code"`
		Success bool   `json:"success"`
		Msg     string `json:"msg"`
		Data    struct {
			AccessToken string `json:"accessToken"`
			ExpiresIn   int    `json:"expiresIn"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("unmarshal token: %w body=%s", err, truncate(string(body), 500))
	}
	if tr.Code != 200 || tr.Data.AccessToken == "" {
		return "", fmt.Errorf("yingdao token failed: code=%d msg=%s body=%s", tr.Code, tr.Msg, truncate(string(body), 500))
	}

	c.accessToken = tr.Data.AccessToken
	expiresIn := tr.Data.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 7200
	}
	c.tokenExpire = time.Now().Add(time.Duration(expiresIn)*time.Second - tokenSafetyMargin)
	return c.accessToken, nil
}

// doJSON 通用请求: POST application/json + Bearer token, 自动反序列化响应
// baseURL: c.AuthURL 或 c.BizURL
// out: 解析目标 (传指针), 设 nil 时跳过反序列化
func (c *Client) doJSON(ctx context.Context, method, baseURL, path string, body, out interface{}) error {
	token, err := c.AccessToken(ctx)
	if err != nil {
		return err
	}

	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal req: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("http %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// 影刀响应体里 code 字段可能在不同接口结构略有差异, 这里取通用结构判断
	var envelope struct {
		Code    int             `json:"code"`
		Success bool            `json:"success"`
		Msg     string          `json:"msg"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return fmt.Errorf("yingdao %s decode: %w body=%s", path, err, truncate(string(respBody), 500))
	}
	// log/query 接口处理中会返回 80204002, 调用方可识别这个 code 决定继续轮询
	if envelope.Code != 200 && envelope.Code != 80204002 {
		return fmt.Errorf("yingdao %s failed: code=%d msg=%s", path, envelope.Code, envelope.Msg)
	}

	if out == nil {
		return nil
	}
	// 把整个 envelope 还原成 out 期望的结构
	wrapped := struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}{Code: envelope.Code, Data: envelope.Data}
	wrappedBytes, _ := json.Marshal(wrapped)
	if err := json.Unmarshal(wrappedBytes, out); err != nil {
		return fmt.Errorf("yingdao %s parse data: %w", path, err)
	}
	return nil
}

// truncate 字符串截断, 防 log 爆量
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
