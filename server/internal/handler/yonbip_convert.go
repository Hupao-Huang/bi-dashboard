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
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bi-dashboard/internal/yonsuite"
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
	DocCode   string `json:"doc_code"`
	AuditOk   bool   `json:"audit_ok"`
	Skipped   bool   `json:"skipped,omitempty"`   // 防重: 同一笔10分钟内已提交过, 本次跳过未重复建单
	Uncertain bool   `json:"uncertain,omitempty"` // 保存传输层失败: 单据可能已在用友建成, 重做前必须核对(防丢响应重复)
	Error     string `json:"error,omitempty"`
}

// ybConvFingerprint 一条转换的内容指纹 (防重复提交用)。
// 必须覆盖所有"决定建出哪张单"的字段(与 ybBuildMorphConvBody 写入的字段对齐),
// 漏字段会把不同业务误判成重复而跳过(漏建单)。
func ybConvFingerprint(it ybConvItem, vouchdate string) string {
	return ybFingerprint(
		"conv", it.Type, ybResolveOrg(it.OrgID), it.WarehouseCode, it.ProductCode,
		it.ProductskuID, it.Batchno, it.StockStatusDoc, it.ToBatch, it.ToStatusDoc,
		it.Producedate, it.Invaliddate,
		strconv.Itoa(int(ybConvQtyNum(it.Qty))), vouchdate,
	)
}

// ybExecuteConvert 逐条建转换单 + 自动审核 (写真用友, 不可逆)。
// ctx: 客户端断开(超时/关页)即停手, 不再往用友写; 防重: 同一笔10分钟内已提交过则跳过。
// force: 强制重发(前端勾选确认), 跳过防重检查仍正常落流水。
func (h *DashboardHandler) ybExecuteConvert(ctx context.Context, vouchdate string, items []ybConvItem, force bool) []ybConvResult {
	results := make([]ybConvResult, 0, len(items))
	h.ybEnsureSubmitLog()
	for _, it := range items {
		res := ybConvResult{Type: it.Type, Product: it.ProductCode, Qty: it.Qty}
		if it.Type == "status" {
			res.From = ybStatusName(it.StockStatusDoc)
			res.To = ybStatusName(it.ToStatusDoc)
		} else {
			res.From = it.Batchno
			res.To = it.ToBatch
		}

		// 客户端断了(超时/关页/切走)立即停手: 本条及后续不再写用友。已写的上面已落流水, 重发会被跳过。
		if ctx.Err() != nil {
			res.Error = "连接中断，本条及后续未执行（已执行的请去用友核对，重新提交不会重复建单）"
			results = append(results, res)
			continue
		}

		// 防重: 同一笔转换在防重窗口内已提交过 → 跳过, 不重复建单。force=强制重发时不查直接建。
		fp := ybConvFingerprint(it, vouchdate)
		if !force {
			if dup, prevDoc := h.ybRecentSubmit(fp); dup {
				res.Skipped = true
				res.DocCode = prevDoc
				res.Error = "10分钟内已提交过，已自动跳过防止重复建单"
				results = append(results, res)
				continue
			}
		}

		body := ybBuildMorphConvBody(it, vouchdate)
		wr, err := h.YS.MorphologyConversionSave(body)
		if err != nil {
			res.Error = ybErrMsg(wr, err, "转换单保存失败")
			// wr==nil = 传输层失败(没拿到用友应答): 单据可能已建成。标记不确定, 让前端提示去核对,
			// 别盲目重做(否则就是原 bug 的"丢响应→重发→重复"). wr!=nil = 用友明确拒单, 没建, 可放心重做。
			res.Uncertain = wr == nil
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
		// 单据已在用友建成(不可逆) → 立即落防重流水, 即便后面审核失败/连接断, 重发也不会再建。
		h.ybRecordSubmit(fp, "conv", res.DocCode, vouchdate)
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
		OrgID          string   `json:"org_id"`
		ProductCode    string   `json:"product_code"`
		WarehouseCode  string   `json:"warehouse_code"`  // 兼容旧单仓库
		WarehouseCodes []string `json:"warehouse_codes"` // 多选仓库
		Batchno        string   `json:"batchno"`
		StatusDoc      string   `json:"status_doc"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求解析失败: "+err.Error())
		return
	}
	// 汇总仓库(多选优先, 兼容单个 warehouse_code), 去空去重
	whSeen := map[string]bool{}
	warehouses := make([]string, 0, len(req.WarehouseCodes)+1)
	for _, wh := range append(req.WarehouseCodes, req.WarehouseCode) {
		wh = strings.TrimSpace(wh)
		if wh != "" && !whSeen[wh] {
			whSeen[wh] = true
			warehouses = append(warehouses, wh)
		}
	}
	if strings.TrimSpace(req.ProductCode) == "" && len(warehouses) == 0 {
		writeError(w, http.StatusBadRequest, "请至少填货品或仓库再查，避免全量拉取")
		return
	}
	org := ybResolveOrg(req.OrgID)
	// 仓库多选: 逐仓库精确查再合并(用友只回该仓库行, 量小; 转换按现货行自己的 warehouse_code 走,
	// 多仓库混合不会串单)。没选仓库则不限仓库查一次(靠货品过滤)。
	rows := make([]yonsuite.StockRow, 0)
	if len(warehouses) == 0 {
		r0, err := h.YS.QueryStockByCondition(org, req.ProductCode, "", req.Batchno, req.StatusDoc)
		if err != nil {
			writeError(w, http.StatusBadGateway, "查用友库存失败: "+err.Error())
			return
		}
		rows = r0
	} else {
		for _, wh := range warehouses {
			r0, err := h.YS.QueryStockByCondition(org, req.ProductCode, wh, req.Batchno, req.StatusDoc)
			if err != nil {
				writeError(w, http.StatusBadGateway, "查仓库 "+wh+" 库存失败: "+err.Error())
				return
			}
			rows = append(rows, r0...)
		}
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
		Force     bool         `json:"force"` // 强制重发: 跳过10分钟防重(前端勾选确认才会带 true)
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
	// 大批量转换会跑很久(每笔2次用友调用 × 1.1s 限流), 默认 120s WriteTimeout 会半路掐断连接,
	// 导致后端还在写用友、前端却收到"失败"→用户重发→重复建单。这里单独清掉本请求的写超时。
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})

	results := h.ybExecuteConvert(r.Context(), ybFmtDate(req.Vouchdate), req.Items, req.Force)
	writeJSON(w, map[string]interface{}{"results": results})
}

// ybOrgInWhitelist 校验组织是否在批次/状态转换允许的白名单内 (守 ybOrgPriority 3 家,
// 与前端 YS_ORGS 对齐; 防止前端传任意 org 把本地 ys_stock 全量拉出来)。
func ybOrgInWhitelist(orgID string) bool {
	for _, o := range ybOrgPriority {
		if o.ID == orgID {
			return true
		}
	}
	return false
}

// ybOption 下拉选项 (编码 + 名字)。
type ybOption struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// ybQueryStockOptions 从本地 ys_stock 按组织取某列(仓库/货品)的去重选项。
// codeCol/nameCol 是固定的内部列名(非用户输入), 用 fmt 拼; org 走占位符防注入。
// 同一编码可能对应多条名字, 用 MAX 收敛成一行, 保证前端下拉 value 唯一不撞 key。
func ybQueryStockOptions(ctx context.Context, db *sql.DB, orgID, codeCol, nameCol string) ([]ybOption, error) {
	q := fmt.Sprintf(
		"SELECT %s AS code, MAX(%s) AS name FROM ys_stock WHERE org=? AND %s<>'' GROUP BY %s ORDER BY %s",
		codeCol, nameCol, codeCol, codeCol, codeCol)
	rows, err := db.QueryContext(ctx, q, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	opts := make([]ybOption, 0, 128)
	for rows.Next() {
		var o ybOption
		if err := rows.Scan(&o.Code, &o.Name); err != nil {
			return nil, err
		}
		opts = append(opts, o)
	}
	return opts, rows.Err()
}

// YonbipConvertOptions POST /api/yonbip/convert-options — 给批次/状态转换工具的货品、仓库下拉提供选项。
// 数据取本地 ys_stock (用友每日同步, T+1, 编码与用友一致), 不调用友实时接口、不占限流。
// 只服务 ybOrgPriority 白名单内的 3 个组织。
func (h *DashboardHandler) YonbipConvertOptions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrgID string `json:"org_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求解析失败: "+err.Error())
		return
	}
	orgID := ybResolveOrg(req.OrgID)
	if !ybOrgInWhitelist(orgID) {
		writeError(w, http.StatusBadRequest, "组织不在允许范围内")
		return
	}
	warehouses, err := ybQueryStockOptions(r.Context(), h.DB, orgID, "warehouse_code", "warehouse_name")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "查仓库清单失败: "+err.Error())
		return
	}
	products, err := ybQueryStockOptions(r.Context(), h.DB, orgID, "product_code", "product_name")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "查货品清单失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"warehouses": warehouses, "products": products})
}
