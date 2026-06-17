package yonsuite

// 写接口: 批次转换单 + 其他出库单的 save / 批量审核
// 移植自本地 Python 出库工具 (Desktop/project/yonbip_api/app.py 的
// create_batch_conversion / audit_conversions / create_other_out / audit_other_outs)
// 复用 client.go 的 token / 签名 / 限流 / POST 管道, 与 QueryStockList 同款写法。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

const (
	// morphologyConvSavePath 批次转换单保存 (同物料 A 批次 → B 批次)
	morphologyConvSavePath = "/iuap-api-gateway/yonbip/scm/morphologyconversion/save"
	// morphologyConvAuditPath 批次转换单批量审核
	morphologyConvAuditPath = "/iuap-api-gateway/yonbip/scm/morphologyconversion/batchaudit"
	// otherOutSavePath 其他出库单保存
	otherOutSavePath = "/iuap-api-gateway/yonbip/scm/othoutrecord/single/save"
	// otherOutAuditPath 其他出库单批量审核
	otherOutAuditPath = "/iuap-api-gateway/yonbip/scm/othoutrecord/batchaudit"
)

// WriteResp 写/审核接口通用返回。
// data 结构因接口而异 (save 返回 {id,...}; audit 返回 {sucessCount,failCount,failInfos,...}),
// 用 RawMessage 原样保留交调用方按需解析。
// 注意: 业务级部分失败 (failCount>0) 仍是 code="200", 在 data 里; 调用方必须自行检查。
// code != "200" 才是传输/鉴权级失败 (此时返回 error)。
type WriteResp struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// rawPost 写接口通用 POST (access_token 走 query, body 为 JSON)。
// 用 UseNumber() 解码, 防止响应里 19 位主表 id 被 float64 截断精度。
// code != "200" 时返回 error, 但 WriteResp 仍带回供排查。
func (c *Client) rawPost(path string, body interface{}) (*WriteResp, error) {
	respBody, err := c.postJSON(path, "write "+path, body)
	if err != nil {
		return nil, err
	}

	var wr WriteResp
	dec := json.NewDecoder(bytes.NewReader(respBody))
	dec.UseNumber()
	if err := dec.Decode(&wr); err != nil {
		return nil, fmt.Errorf("unmarshal write resp: %w, body=%s", err, truncate(string(respBody), 500))
	}
	if wr.Code != "200" {
		return &wr, fmt.Errorf("yonsuite write %s non-200: code=%s msg=%s", path, wr.Code, wr.Message)
	}
	return &wr, nil
}

// MorphologyConversionSave 批次转换单保存。
// body 由调用方按 YS 报文构造 ({"data":{org,businesstypeId:"A70002",conversionType:"1",
// mcType:"1",vouchdate,beforeWarehouse,afterWarehouse,_status:"Insert",
// morphologyconversiondetail:[before(lineType"1"), after(lineType"2")]}})。
func (c *Client) MorphologyConversionSave(body interface{}) (*WriteResp, error) {
	return c.rawPost(morphologyConvSavePath, body)
}

// MorphologyConversionBatchAudit 批次转换单批量审核 (ids = 主表 id 列表)。
func (c *Client) MorphologyConversionBatchAudit(ids []int64) (*WriteResp, error) {
	items := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		items = append(items, map[string]interface{}{"id": id, "pubts": ""})
	}
	return c.rawPost(morphologyConvAuditPath, map[string]interface{}{"data": items})
}

// OtherOutSave 其他出库单保存。
// body 由调用方按 YS 报文构造 ({"data":{_status:"Insert",org,accountOrg,vouchdate,
// bustype:"A10001",warehouse,memo,othOutRecords:[...], othOutRecordDefineCharacter:{SF001:收发类别code}}})。
func (c *Client) OtherOutSave(body interface{}) (*WriteResp, error) {
	return c.rawPost(otherOutSavePath, body)
}

// OtherOutBatchAudit 其他出库单批量审核 (ids = 主表 id 列表)。
// 注意: YS 该接口只收 id, 审核日期一律取系统当天, 无法通过 API 回填 (已实测确认)。
func (c *Client) OtherOutBatchAudit(ids []int64) (*WriteResp, error) {
	items := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		items = append(items, map[string]interface{}{"id": id})
	}
	return c.rawPost(otherOutAuditPath, map[string]interface{}{"data": items})
}

// StockRow 现存量单行 (出库工具拆单用)。
// 字段语义对齐本地 Python 工具 query_stock: id 类字段统一成 string (19 位防精度),
// 数量类字段 float64。Producedate/Invaliddate 取日期部分 (前 10 位)。
type StockRow struct {
	WarehouseCode  string  `json:"warehouse_code"`
	WarehouseName  string  `json:"warehouse_name"`
	WarehouseID    string  `json:"warehouse_id"`
	ProductCode    string  `json:"product_code"`
	ProductName    string  `json:"product_name"`
	ProductID      string  `json:"product_id"`
	ProductskuID   string  `json:"productsku_id"`
	Model          string  `json:"model"`
	Batchno        string  `json:"batchno"`
	Producedate    string  `json:"producedate"`
	Invaliddate    string  `json:"invaliddate"`
	CurrentQty     float64 `json:"currentqty"`
	AvailableQty   float64 `json:"availableqty"`
	Unit           string  `json:"unit"`
	UnitID         string  `json:"unit_id"`
	ManageClass    string  `json:"manageClass"`
	Status         string  `json:"status"`
	StockStatusDoc string  `json:"stockStatusDoc"`
	StockUnitID    string  `json:"stockUnitId"`
}

// QueryStockByCondition 按 组织 + (可选)货品编码/仓库编码/批次/库存状态 查现存量。
// 对齐 Python query_stock: body 用点号字段 (productn.code / warehouse.code);
// data 是直接 array; 过滤掉 现存量 & 可用量都为 0 的行。
func (c *Client) QueryStockByCondition(orgID, productCode, warehouseCode, batchno, statusDoc string) ([]StockRow, error) {
	body := map[string]interface{}{"org": orgID}
	if productCode != "" {
		body["productn.code"] = productCode
	}
	if warehouseCode != "" {
		body["warehouse.code"] = warehouseCode
	}
	if batchno != "" {
		body["batchno"] = batchno
	}
	if statusDoc != "" {
		body["stockStatusDoc"] = statusDoc
	}

	respBody, err := c.postJSON(stockListPath, "stock-cond", body)
	if err != nil {
		return nil, err
	}

	var pr StockListResp
	dec := json.NewDecoder(bytes.NewReader(respBody))
	dec.UseNumber()
	if err := dec.Decode(&pr); err != nil {
		return nil, fmt.Errorf("unmarshal stock-cond: %w, body=%s", err, truncate(string(respBody), 500))
	}
	if pr.Code != "200" {
		return nil, fmt.Errorf("yonsuite stock-cond non-200: code=%s msg=%s", pr.Code, pr.Message)
	}

	rows := make([]StockRow, 0, len(pr.Data))
	for _, it := range pr.Data {
		cur := jsonFloat(it["currentqty"])
		avail := jsonFloat(it["availableqty"])
		if cur == 0 && avail == 0 {
			continue // 现存量&可用量都为 0 的不要
		}
		unitID := JSONString(it["unit"])
		stockUnitID := JSONString(it["stockUnitId"])
		if stockUnitID == "" {
			stockUnitID = unitID
		}
		rows = append(rows, StockRow{
			WarehouseCode:  JSONString(it["warehouse_code"]),
			WarehouseName:  JSONString(it["warehouse_name"]),
			WarehouseID:    JSONString(it["warehouse"]),
			ProductCode:    JSONString(it["product_code"]),
			ProductName:    JSONString(it["product_name"]),
			ProductID:      JSONString(it["product"]),
			ProductskuID:   JSONString(it["productsku"]),
			Model:          JSONString(it["product_modelDescription"]),
			Batchno:        JSONString(it["batchno"]),
			Producedate:    first10(JSONString(it["producedate"])),
			Invaliddate:    first10(JSONString(it["invaliddate"])),
			CurrentQty:     cur,
			AvailableQty:   avail,
			Unit:           JSONString(it["product_unitName"]),
			UnitID:         unitID,
			ManageClass:    JSONString(it["manageClass_name"]),
			Status:         JSONString(it["stockStatusDoc_statusName"]),
			StockStatusDoc: JSONString(it["stockStatusDoc"]),
			StockUnitID:    stockUnitID,
		})
	}
	return rows, nil
}

// JSONString 把 YS 返回的 interface{} (json.Number / string / nil / 数值 / 布尔) 统一成 string。
// 用 json.UseNumber() 解码时数字是 json.Number; 未知/嵌套结构 (map / array) 返回空串而不是
// fmt 出 "map[...]"，避免在财务凭证/库存展示里渲染出垃圾值。
// yonsuite 包内 (现存量行) + handler 凭证页 ysMapStr 共用这一处, 单一事实来源。
func JSONString(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		return ""
	}
}

// jsonFloat 把 YS 返回的数值字段统一成 float64。
func jsonFloat(v interface{}) float64 {
	switch x := v.(type) {
	case json.Number:
		f, _ := x.Float64()
		return f
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	default:
		return 0
	}
}

// first10 取字符串前 10 位 (日期部分, 如 "2026-05-31 00:00:00" → "2026-05-31")。
func first10(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}
