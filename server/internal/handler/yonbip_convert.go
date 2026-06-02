package handler

// 用友 YS 批次/状态转换工具 (系统设置工具箱 → YS工具 → 批次转换, 单人用)。
// 移植自本地 Python 工具 Desktop/project/yonbip_api/app.py:
//   - query_stock              → YonbipConvertStock   (查现存量, 复用 h.YS.QueryStockByCondition)
//   - create_batch_conversion  → ybBuildMorphConvBody(type=batch)   同物料 A批次→B批次
//   - create_status_conversion → ybBuildMorphConvBody(type=status)  同批次 A状态→B状态
//   - audit_conversions        → h.YS.MorphologyConversionBatchAudit
// 批次转换/状态转换在 YS 是同一接口 morphologyconversion/save, 只差 before/after 行。
// 写库存=不可逆。复用 yonbip_outbound.go 的 helper(ybFmtDate/ybResolveOrg/ybFirstNonEmpty/
// ybErrMsg/ybAuditOK/ybDecodeData) 与常量。

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// ybStatusDocName 库存状态 doc → 中文名 (与 yonbip_outbound.go ybStatusPriority 对齐)。
var ybStatusDocName = map[string]string{
	"2448706971278246078": "合格",
	"2448706971278246081": "不合格",
	"2448706971278246082": "废品",
}

func ybStatusName(doc string) string {
	if n, ok := ybStatusDocName[strings.TrimSpace(doc)]; ok {
		return n
	}
	return doc
}

func ybConvQtyNum(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

// ybConvItem 一条待执行的转换 (前端清单的一行)。
type ybConvItem struct {
	Type           string `json:"type"`           // "batch"(批次转换) | "status"(状态转换)
	OrgID          string `json:"org_id"`
	WarehouseCode  string `json:"warehouse_code"`
	WarehouseName  string `json:"warehouse_name"`
	ProductCode    string `json:"product_code"`
	ProductName    string `json:"product_name"`
	ProductskuID   string `json:"productsku_id"`
	UnitID         string `json:"unit_id"`
	StockUnitID    string `json:"stockUnitId"`
	Qty            string `json:"qty"`
	Batchno        string `json:"batchno"`        // 当前批次
	Producedate    string `json:"producedate"`
	Invaliddate    string `json:"invaliddate"`
	StockStatusDoc string `json:"stockStatusDoc"` // 当前库存状态 doc
	ToBatch        string `json:"to_batch"`       // 批次转换: 目标批次
	ToStatusDoc    string `json:"to_status_doc"`  // 状态转换: 目标状态 doc
}

// ybBuildMorphConvBody 形态转换单报文 (批次转换 or 状态转换)。
// 批次转换: before/after 仅 batchno 不同(A→B), 库存状态相同。
// 状态转换: before/after 仅 stockStatusDoc 不同(A→B), 批次相同。
func ybBuildMorphConvBody(it ybConvItem, vouchdate string) map[string]interface{} {
	vd := ybFmtDate(vouchdate) + " 00:00:00"
	unitID := it.UnitID
	stockUnit := ybFirstNonEmpty(it.StockUnitID, it.UnitID)
	// 数量取整, 对齐原 Python create_batch_conversion 的 int() 与出库工具 (YS 已验证接受 int)。
	qty := int(ybConvQtyNum(it.Qty))

	base := func() map[string]interface{} {
		m := map[string]interface{}{
			"warehouse":        it.WarehouseCode,
			"product":          it.ProductCode,
			"mainUnitId":       unitID,
			"stockUnitId":      stockUnit,
			"invExchRate":      "1",
			"unitExchangeType": 0,
			"qty":              qty,
			"subQty":           qty,
		}
		if it.ProductskuID != "" && it.ProductskuID != "0" {
			m["productsku"] = it.ProductskuID
		}
		if it.Producedate != "" {
			m["producedate"] = ybFmtDate(it.Producedate) + " 00:00:00"
		}
		if it.Invaliddate != "" {
			m["invaliddate"] = ybFmtDate(it.Invaliddate) + " 00:00:00"
		}
		return m
	}

	before := base()
	before["groupNumber"] = "1"
	before["lineType"] = "1"
	after := base()
	after["groupNumber"] = "1"
	after["lineType"] = "2"

	if it.Type == "status" {
		// 状态转换: 同物料同批次, 状态 A→B
		if it.Batchno != "" {
			before["batchno"] = it.Batchno
			after["batchno"] = it.Batchno
		}
		before["stockStatusDoc"] = it.StockStatusDoc
		after["stockStatusDoc"] = it.ToStatusDoc
	} else {
		// 批次转换: 同物料同状态, 批次 A→B。
		// stockStatusDoc 无条件带 (对齐原 Python: 形态转换必填, 漏带会让 before/after 状态行不一致建错单)。
		before["batchno"] = it.Batchno
		after["batchno"] = it.ToBatch
		before["stockStatusDoc"] = it.StockStatusDoc
		after["stockStatusDoc"] = it.StockStatusDoc
	}

	return map[string]interface{}{
		"data": map[string]interface{}{
			"org":                        ybResolveOrg(it.OrgID),
			"businesstypeId":             "A70002",
			"conversionType":             "1",
			"mcType":                     "1",
			"vouchdate":                  vd,
			"beforeWarehouse":            it.WarehouseCode,
			"afterWarehouse":             it.WarehouseCode,
			"_status":                    "Insert",
			"morphologyconversiondetail": []map[string]interface{}{before, after},
		},
	}
}

// ybConvResult 一条转换的执行结果。
type ybConvResult struct {
	Type    string `json:"type"`
	Product string `json:"product_code"`
	From    string `json:"from"` // 批次转换=源批次; 状态转换=源状态名
	To      string `json:"to"`   // 批次转换=目标批次; 状态转换=目标状态名
	Qty     string `json:"qty"`
	DocCode string `json:"doc_code"`
	AuditOk bool   `json:"audit_ok"`
	Error   string `json:"error,omitempty"`
}

// ybExecuteConvert 逐条建转换单 + 自动审核 (写真用友, 不可逆)。
func (h *DashboardHandler) ybExecuteConvert(vouchdate string, items []ybConvItem) []ybConvResult {
	results := make([]ybConvResult, 0, len(items))
	for _, it := range items {
		res := ybConvResult{Type: it.Type, Product: it.ProductCode, Qty: it.Qty}
		if it.Type == "status" {
			res.From = ybStatusName(it.StockStatusDoc)
			res.To = ybStatusName(it.ToStatusDoc)
		} else {
			res.From = it.Batchno
			res.To = it.ToBatch
		}

		body := ybBuildMorphConvBody(it, vouchdate)
		wr, err := h.YS.MorphologyConversionSave(body)
		if err != nil {
			res.Error = ybErrMsg(wr, err, "转换单保存失败")
			results = append(results, res)
			continue
		}
		var sd struct {
			Infos []struct {
				ID   json.Number `json:"id"`
				Code string      `json:"code"`
			} `json:"infos"`
		}
		_ = ybDecodeData(wr, &sd)
		if len(sd.Infos) == 0 {
			res.Error = "转换单已保存但用友未返回单据信息，无法自动审核，请去用友手动核对"
			results = append(results, res)
			continue
		}
		res.DocCode = sd.Infos[0].Code
		cvID, _ := sd.Infos[0].ID.Int64()
		if cvID == 0 {
			res.Error = "转换单已保存但用友未返回单据id，无法自动审核，请去用友手动核对"
			results = append(results, res)
			continue
		}
		awr, aerr := h.YS.MorphologyConversionBatchAudit([]int64{cvID})
		res.AuditOk = aerr == nil && ybAuditOK(awr)
		if !res.AuditOk {
			res.Error = ybErrMsg(awr, aerr, "转换单审核失败")
		}
		results = append(results, res)
	}
	return results
}

// ---------------- HTTP handlers ----------------

// YonbipConvertStock POST /api/yonbip/convert-stock — 查现存量 (只读, 给转换工具选批次)。
func (h *DashboardHandler) YonbipConvertStock(w http.ResponseWriter, r *http.Request) {
	if h.YS == nil {
		writeError(w, http.StatusServiceUnavailable, "用友 YS 未配置")
		return
	}
	var req struct {
		OrgID         string `json:"org_id"`
		ProductCode   string `json:"product_code"`
		WarehouseCode string `json:"warehouse_code"`
		Batchno       string `json:"batchno"`
		StatusDoc     string `json:"status_doc"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求解析失败: "+err.Error())
		return
	}
	if strings.TrimSpace(req.ProductCode) == "" && strings.TrimSpace(req.WarehouseCode) == "" {
		writeError(w, http.StatusBadRequest, "请至少填货品编码或仓库编码再查，避免全量拉取")
		return
	}
	rows, err := h.YS.QueryStockByCondition(ybResolveOrg(req.OrgID), req.ProductCode, req.WarehouseCode, req.Batchno, req.StatusDoc)
	if err != nil {
		writeError(w, http.StatusBadGateway, "查用友库存失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"rows": rows})
}

// YonbipConvertExecute POST /api/yonbip/convert-execute — 真执行批次/状态转换 (写用友, 不可逆)。
func (h *DashboardHandler) YonbipConvertExecute(w http.ResponseWriter, r *http.Request) {
	if h.YS == nil {
		writeError(w, http.StatusServiceUnavailable, "用友 YS 未配置")
		return
	}
	var req struct {
		Vouchdate string       `json:"vouchdate"`
		Items     []ybConvItem `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求解析失败: "+err.Error())
		return
	}
	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "没有要转换的项")
		return
	}
	// 防御: 每条必须有数量且 > 0, 目标必须填
	for i, it := range req.Items {
		if ybConvQtyNum(it.Qty) <= 0 {
			writeError(w, http.StatusBadRequest, "第"+strconv.Itoa(i+1)+"条数量无效")
			return
		}
		if it.Type == "status" {
			if strings.TrimSpace(it.ToStatusDoc) == "" {
				writeError(w, http.StatusBadRequest, "第"+strconv.Itoa(i+1)+"条缺目标状态")
				return
			}
		} else {
			if strings.TrimSpace(it.ToBatch) == "" {
				writeError(w, http.StatusBadRequest, "第"+strconv.Itoa(i+1)+"条缺目标批次")
				return
			}
		}
	}
	results := h.ybExecuteConvert(ybFmtDate(req.Vouchdate), req.Items)
	writeJSON(w, map[string]interface{}{"results": results})
}
