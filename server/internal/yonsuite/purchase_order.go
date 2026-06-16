package yonsuite

// 新增采购订单工具用到的用友接口:
//   - 采购组织字典 (getallorgdeptbaseinfo, orgunit 职能) → 组织名↔编码
//   - 物料档案批量详情 (product/batchdetailnew) → 拿采购/计价/主计量单位编码 + 税目编码(进项税率)
//   - 供应商分页 (vendor/list) → 名称↔编码字典
//   - 采购订单单个保存 (purchaseorder/singleSave_v1) → 建单(开立态, 不审核)
// 接口已实测授权通过(2026-06-16)。写用友=不可逆, 调用方须走两步向导+幂等防重。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// parseTaxRatePct 从货品进项税率名称解析税率% (如 "13%增值税税率" → 13)。
func parseTaxRatePct(name string) float64 {
	i := strings.Index(name, "%")
	if i <= 0 {
		return 0
	}
	f, _ := strconv.ParseFloat(strings.TrimSpace(name[:i]), 64)
	return f
}

const (
	orgListPath         = "/iuap-api-gateway/yonbip/digitalModel/openapi/orgdatasync/getallorgdeptbaseinfo"
	productDetailPath   = "/iuap-api-gateway/yonbip/digitalModel/product/batchdetailnew"
	poSingleSavePath    = "/iuap-api-gateway/yonbip/scm/purchaseorder/singleSave_v1"
)

// OrgInfo 采购组织 (法人主体)
type OrgInfo struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	OrgID string `json:"orgid"`
}

// orgListResp getallorgdeptbaseinfo 返回 (code 是数字 200, 用 json.Number 兜)
type orgListResp struct {
	Code json.Number `json:"code"`
	Data struct {
		RecordCount int       `json:"recordCount"`
		RecordList  []OrgInfo `json:"recordList"`
	} `json:"data"`
	Message string `json:"message"`
}

// QueryPurchaseOrgs 拉全部业务单元组织 (isBizUnit=1), 给"组织名→编码"字典用。
// 香松采购无独立 purchaseorg 职能, 采购组织即业务单元 orgunit。
func (c *Client) QueryPurchaseOrgs() ([]OrgInfo, error) {
	token, err := c.AccessToken()
	if err != nil {
		return nil, err
	}
	c.waitRateLimit()

	reqBody, err := json.Marshal(map[string]interface{}{
		"funcTypeCode": "orgunit",
		"isBizUnit":    "1",
		"pageIndex":    "1",
		"pageSize":     "1000",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal org req: %w", err)
	}

	q := url.Values{}
	q.Set("access_token", token)
	httpReq, err := http.NewRequest("POST", c.BaseURL+orgListPath+"?"+q.Encode(), bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("yonsuite org list http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var r orgListResp
	dec := json.NewDecoder(bytes.NewReader(respBody))
	dec.UseNumber()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("unmarshal org list: %w, body=%s", err, truncate(string(respBody), 500))
	}
	if r.Code.String() != "200" {
		return nil, fmt.Errorf("yonsuite org list non-200: code=%s msg=%s", r.Code.String(), r.Message)
	}
	return r.Data.RecordList, nil
}

// ProductDetail 物料档案详情精简 — 采购单需要的单位三件套 + 税目编码, 都从货品档案直接读。
// 实测(2026-06-16) batchdetailnew 对香松 02030063 返回: 采购/计价/主计量编码=01, incomeTaxRates=10004。
type ProductDetail struct {
	Code         string  // 物料编码
	UnitCode     string  // 主计量单位编码
	PurUOMCode   string  // 采购单位编码
	PriceUOMCode string  // 采购计价单位编码
	TaxitemsCode string  // 税目编码 (= 货品进项税率 incomeTaxRates), 直接喂采购单 taxitems_code
	TaxRatePct   float64 // 进项税率% (从 incomeTaxRatesName "13%增值税税率" 解析, 算价以此为准)
	TaxRateName  string  // 进项税率名称 (展示用, 如 "13%增值税税率")
}

// productDetailResp batchdetailnew 返回 (code 字符串 "200"; data 是数组, 每项含顶层字段 + 嵌套 detail)
type productDetailResp struct {
	Code string `json:"code"`
	Data []struct {
		Code     string `json:"code"`
		UnitCode string `json:"unitCode"`
		Detail   struct {
			PurchaseUnitCode      string `json:"purchaseUnitCode"`
			PurchasePriceUnitCode string `json:"purchasePriceUnitCode"`
			IncomeTaxRates        string `json:"incomeTaxRates"`
			IncomeTaxRatesName    string `json:"incomeTaxRatesName"`
		} `json:"detail"`
	} `json:"data"`
	Message string `json:"message"`
}

// QueryProductDetails 批量查物料详情(批量上限 10), 给"物料编码→单位编码/税目编码"字典用。
// orgCode 必填(物料档案按组织); 返回 map[物料编码]*ProductDetail, 查不到的编码不在 map 里。
func (c *Client) QueryProductDetails(orgCode string, productCodes []string) (map[string]*ProductDetail, error) {
	result := make(map[string]*ProductDetail)
	if orgCode == "" || len(productCodes) == 0 {
		return result, nil
	}

	// 批量上限 10, 分批查
	for start := 0; start < len(productCodes); start += 10 {
		end := start + 10
		if end > len(productCodes) {
			end = len(productCodes)
		}
		batch := productCodes[start:end]

		token, err := c.AccessToken()
		if err != nil {
			return nil, err
		}
		c.waitRateLimit()

		items := make([]map[string]interface{}, 0, len(batch))
		for _, code := range batch {
			items = append(items, map[string]interface{}{"productCode": code, "orgCode": orgCode})
		}
		reqBody, err := json.Marshal(items)
		if err != nil {
			return nil, fmt.Errorf("marshal product detail req: %w", err)
		}

		q := url.Values{}
		q.Set("access_token", token)
		httpReq, err := http.NewRequest("POST", c.BaseURL+productDetailPath+"?"+q.Encode(), bytes.NewReader(reqBody))
		if err != nil {
			return nil, fmt.Errorf("new request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.HTTP.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("yonsuite product detail http: %w", err)
		}
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}

		var r productDetailResp
		dec := json.NewDecoder(bytes.NewReader(respBody))
		dec.UseNumber()
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("unmarshal product detail: %w, body=%s", err, truncate(string(respBody), 500))
		}
		if r.Code != "200" {
			return nil, fmt.Errorf("yonsuite product detail non-200: code=%s msg=%s", r.Code, r.Message)
		}
		for i := range r.Data {
			d := &r.Data[i]
			pd := &ProductDetail{
				Code:         d.Code,
				UnitCode:     d.UnitCode,
				PurUOMCode:   d.Detail.PurchaseUnitCode,
				PriceUOMCode: d.Detail.PurchasePriceUnitCode,
				TaxitemsCode: d.Detail.IncomeTaxRates,
				TaxRatePct:   parseTaxRatePct(d.Detail.IncomeTaxRatesName),
				TaxRateName:  d.Detail.IncomeTaxRatesName,
			}
			if pd.PurUOMCode == "" {
				pd.PurUOMCode = pd.UnitCode // 采购单位缺省=主计量
			}
			if pd.PriceUOMCode == "" {
				pd.PriceUOMCode = pd.PurUOMCode
			}
			result[d.Code] = pd
		}
	}
	return result, nil
}

const vendorListPath = "/iuap-api-gateway/yonbip/digitalModel/vendor/list"

// VendorInfo 供应商精简 (名称→编码字典用)
type VendorInfo struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// vendorListResp vendor/list 返回 (code 字符串 "200")
type vendorListResp struct {
	Code string `json:"code"`
	Data struct {
		PageCount   int          `json:"pageCount"`
		RecordCount int          `json:"recordCount"`
		RecordList  []VendorInfo `json:"recordList"`
	} `json:"data"`
	Message string `json:"message"`
}

// QueryVendorsPage 拉一页供应商 (企业级共享, 无名称过滤, 全量翻页建字典)。
// 返回该页供应商 + 总页数。供应商挂"企业账号级"(org 不填=全量)。
func (c *Client) QueryVendorsPage(pageIndex, pageSize int) ([]VendorInfo, int, error) {
	token, err := c.AccessToken()
	if err != nil {
		return nil, 0, err
	}
	c.waitRateLimit()

	reqBody, err := json.Marshal(map[string]interface{}{
		"pageIndex": pageIndex,
		"pageSize":  pageSize,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("marshal vendor req: %w", err)
	}

	q := url.Values{}
	q.Set("access_token", token)
	httpReq, err := http.NewRequest("POST", c.BaseURL+vendorListPath+"?"+q.Encode(), bytes.NewReader(reqBody))
	if err != nil {
		return nil, 0, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("yonsuite vendor list http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read body: %w", err)
	}

	var r vendorListResp
	dec := json.NewDecoder(bytes.NewReader(respBody))
	dec.UseNumber()
	if err := dec.Decode(&r); err != nil {
		return nil, 0, fmt.Errorf("unmarshal vendor list: %w, body=%s", err, truncate(string(respBody), 500))
	}
	if r.Code != "200" {
		return nil, 0, fmt.Errorf("yonsuite vendor list non-200: code=%s msg=%s", r.Code, r.Message)
	}
	return r.Data.RecordList, r.Data.PageCount, nil
}

// PurchaseOrderSingleSave 采购订单单个保存 (建单到"开立"态, 不审核)。
// body 由调用方按 YS 报文构造 ({"data":{...,"_status":"Insert","purchaseOrders":[...]}}),
// resubmitCheckKey 做幂等防重。复用 rawPost 的 token/签名/限流/UseNumber 管道。
func (c *Client) PurchaseOrderSingleSave(body interface{}) (*WriteResp, error) {
	return c.rawPost(poSingleSavePath, body)
}

// SavePurchaseOrder 建单并"严格判成功": code=="200" 不够(用友业务失败也可能返200, 二审#2),
// 必须从 data 里拿到采购订单主表 id 才算真建成。返回 (订单id, 原始resp, err)。
func (c *Client) SavePurchaseOrder(body interface{}) (string, *WriteResp, error) {
	resp, err := c.PurchaseOrderSingleSave(body)
	if err != nil {
		return "", resp, err
	}
	var d struct {
		ID   json.Number `json:"id"`
		Code string      `json:"code"`
	}
	if resp != nil && len(resp.Data) > 0 {
		dec := json.NewDecoder(bytes.NewReader(resp.Data))
		dec.UseNumber()
		_ = dec.Decode(&d) // data 非预期结构时下面统一按"无id=失败"兜
	}
	if id := d.ID.String(); id != "" && id != "0" {
		return id, resp, nil
	}
	return "", resp, fmt.Errorf("用友未返回采购订单id(疑业务失败), data=%s", func() string {
		if resp != nil {
			return truncate(string(resp.Data), 800)
		}
		return "<nil>"
	}())
}
