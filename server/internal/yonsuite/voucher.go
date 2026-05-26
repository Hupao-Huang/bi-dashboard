package yonsuite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// voucherListPath 凭证列表查询
const voucherListPath = "/iuap-api-gateway/yonbip/fi/ficloud/openapi/voucher/queryVouchers"

// VoucherListReq 凭证列表查询请求
// accbookCode 必填, 其他都是过滤条件
type VoucherListReq struct {
	Pager struct {
		PageIndex int `json:"pageIndex"`
		PageSize  int `json:"pageSize"`
	} `json:"pager"`
	AccbookCode         string   `json:"accbookCode"`                   // 必填: 账簿 code
	PeriodStart         string   `json:"periodStart,omitempty"`         // yyyy-MM
	PeriodEnd           string   `json:"periodEnd,omitempty"`           // yyyy-MM
	BillcodeMin         int      `json:"billcodeMin,omitempty"`         // 凭证号区间
	BillcodeMax         int      `json:"billcodeMax,omitempty"`         // 凭证号区间
	VoucherTypeCodeList []string `json:"voucherTypeCodeList,omitempty"` // ["1","2","3","4"]
	VoucherStatusList   []string `json:"voucherStatusList,omitempty"`   // ["01","04"] 01 保存 04 已记账
}

// VoucherListResp 凭证列表返回
// header + body 嵌套, 用 map 接收保留所有原始字段
type VoucherListResp struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		PageIndex   int                      `json:"pageIndex"`
		PageSize    int                      `json:"pageSize"`
		RecordCount int                      `json:"recordCount"`
		RecordList  []map[string]interface{} `json:"recordList"`
	} `json:"data"`
}

// QueryVoucherList 调用友凭证列表查询接口
// 返回 raw recordList (含 header / body), 上层自己 parse
func (c *Client) QueryVoucherList(req *VoucherListReq) (*VoucherListResp, error) {
	if req.AccbookCode == "" {
		return nil, fmt.Errorf("accbookCode 必填")
	}
	if req.Pager.PageSize == 0 {
		req.Pager.PageIndex = 1
		req.Pager.PageSize = 20
	}

	token, err := c.AccessToken()
	if err != nil {
		return nil, err
	}
	c.waitRateLimit()

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal voucher list req: %w", err)
	}

	q := url.Values{}
	q.Set("access_token", token)
	fullURL := c.BaseURL + voucherListPath + "?" + q.Encode()

	httpReq, err := http.NewRequest("POST", fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("yonsuite voucher list http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	// UseNumber 防止 19 位 id 精度丢失
	var vr VoucherListResp
	dec := json.NewDecoder(bytes.NewReader(respBody))
	dec.UseNumber()
	if err := dec.Decode(&vr); err != nil {
		return nil, fmt.Errorf("unmarshal voucher list: %w, body=%s", err, truncate(string(respBody), 500))
	}
	if vr.Code != "200" {
		return nil, fmt.Errorf("yonsuite voucher list non-200: code=%s msg=%s", vr.Code, vr.Message)
	}
	return &vr, nil
}
