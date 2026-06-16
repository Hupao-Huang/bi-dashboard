package yonsuite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// accbookListPath 账簿(法人主体)清单查询 — 凭证查询页下拉用
const accbookListPath = "/iuap-api-gateway/yonbip/fi/fipub/basedoc/querybd/accbook"

// Accbook 账簿 = 一个法人主体一本账
type Accbook struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// accbookResp 账簿接口返回 (注意: 这个接口 code 是数字 200, data 是直接 array)
type accbookResp struct {
	Code    json.Number `json:"code"`
	Message string      `json:"message"`
	Data    []Accbook   `json:"data"`
}

// QueryAccbookList 拉全部账簿 (code + name)
func (c *Client) QueryAccbookList() ([]Accbook, error) {
	token, err := c.AccessToken()
	if err != nil {
		return nil, err
	}
	c.waitRateLimit()

	body, err := json.Marshal(map[string]interface{}{
		"fields":    []string{"code", "name"},
		"pageIndex": 1,
		"pageSize":  1000,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal accbook req: %w", err)
	}

	q := url.Values{}
	q.Set("access_token", token)
	fullURL := c.BaseURL + accbookListPath + "?" + q.Encode()

	httpReq, err := http.NewRequest("POST", fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("yonsuite accbook list http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var ar accbookResp
	dec := json.NewDecoder(bytes.NewReader(respBody))
	dec.UseNumber()
	if err := dec.Decode(&ar); err != nil {
		return nil, fmt.Errorf("unmarshal accbook list: %w, body=%s", err, truncate(string(respBody), 500))
	}
	if ar.Code.String() != "200" {
		return nil, fmt.Errorf("yonsuite accbook list non-200: code=%s msg=%s", ar.Code.String(), ar.Message)
	}
	return ar.Data, nil
}
