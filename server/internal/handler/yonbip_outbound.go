package handler

// 用友 YonBIP 批量出库工具 (系统设置 → 小工具, 单人高频用)。
// 一比一移植自本地 Python 工具 Desktop/project/yonbip_api/app.py:
//   - plan_jky_export      → ybPlanExport (拆单算法, 纯逻辑, 注入库存查询便于单测)
//   - api_jky_export_execute → (*DashboardHandler).ybExecute (三阶段: 批次转换→出库单→审核)
//   - create_batch_conversion / create_other_out → ybBuildConversionBody / ybBuildOutItem+ybBuildOtherOutBody
// 写库存=不可逆, 所有 YS 调用走 h.YS (复用 token/签名/1.1s 限流)。

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"bi-dashboard/internal/yonsuite"
)

// ---------------- 常量 (移植自 Python) ----------------

type ybOrg struct {
	ID   string
	Name string
}

// ybOrgPriority 跨组织扣库存优先级 (按顺序扣, 未列入的组织不参与)。
var ybOrgPriority = []ybOrg{
	{"2451285875823214599", "浙江松鲜鲜自然调味品有限公司"},
	{"2451285927362822152", "杭州润松自然调味品有限公司"},
	{"2451285918772887559", "杭州华鲜高新技术有限公司"},
}

const ybDefaultOrgID = "2451285875823214599"

// ybStatusPriority 库存状态优先级: 合格 → 不合格 → 废品 → 其他。
var ybStatusPriority = map[string]int{
	"2448706971278246078": 0, // 合格
	"2448706971278246081": 1, // 不合格
	"2448706971278246082": 2, // 废品
}

// ybQualifiedStatusDoc 合格状态 doc。出库一律出合格品: 不合格/废品库存出库前先状态转换成合格。
const ybQualifiedStatusDoc = "2448706971278246078"

func ybStatusPri(sd string) int {
	if p, ok := ybStatusPriority[sd]; ok {
		return p
	}
	return 99
}

// ybCategoryNameToCode 收发类别 中文→code (前端可能传中文或 code)。
var ybCategoryNameToCode = map[string]string{
	"其他出库": "29", "销售出库": "22", "留样出库": "23",
	"抽检损耗出库": "24", "组装拆卸出库": "25", "参展出库": "26",
	"调拨出库": "27", "盘亏出库": "28", "生产损耗": "30",
	"生产领料": "20", "委外发料": "21",
}

func ybNormalizeCategory(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if ybIsDigits(s) {
		return s
	}
	return ybCategoryNameToCode[s] // 未知中文 → ""
}

// ybWhStripTokens 仓库名 normalize 时丢弃的组织/类型修饰段。
var ybWhStripTokens = map[string]bool{
	"润松": true, "浙江松鲜鲜": true, "公司仓": true, "外仓": true,
	"委外": true, "公司": true, "分公司": true,
}

func ybNormalizeWhName(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "-")
	keep := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || ybWhStripTokens[p] {
			continue
		}
		keep = append(keep, p)
	}
	if len(keep) == 0 {
		return s
	}
	return strings.Join(keep, "-")
}

// ybWhMatch 仓库名严格匹配: 精确相等, 或 normalize 后精确相等 (不做子串匹配)。
func ybWhMatch(jkyName, stockName string) bool {
	a := strings.TrimSpace(jkyName)
	b := strings.TrimSpace(stockName)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	na, nb := ybNormalizeWhName(a), ybNormalizeWhName(b)
	return na != "" && nb != "" && na == nb
}

// ---------------- 类型 (JSON 与前端 round-trip) ----------------

type ybRow struct {
	ProductCode   string `json:"product_code"`
	Qty           string `json:"qty"`
	TargetBatch   string `json:"target_batch"`
	Bustype       string `json:"bustype"`
	BillNo        string `json:"bill_no"`
	Category      string `json:"category"`
	WarehouseName string `json:"warehouse_name"`
}

type ybConvertSource struct {
	FromBatch       string `json:"from_batch"`
	FromProducedate string `json:"from_producedate"`
	FromInvaliddate string `json:"from_invaliddate"`
	Qty             int    `json:"qty"`
	ProductskuID    string `json:"productsku_id"`
	UnitID          string `json:"unit_id"`
	StockUnitID     string `json:"stockUnitId"`
	StockStatusDoc  string `json:"stockStatusDoc"`
}

type ybShipment struct {
	OrgID           string            `json:"org_id"`
	OrgName         string            `json:"org_name"`
	WarehouseCode   string            `json:"warehouse_code"`
	WarehouseName   string            `json:"warehouse_name"`
	Qty             int               `json:"qty"`
	TargetQtyDirect int               `json:"target_qty_direct"`
	ConvertQty      int               `json:"convert_qty"`
	ConvertSources  []ybConvertSource `json:"convert_sources"`
	ProductCode     string            `json:"product_code"`
	ProductName     string            `json:"product_name"`
	ProductskuID    string            `json:"productsku_id"`
	UnitID          string            `json:"unit_id"`
	StockUnitID     string            `json:"stockUnitId"`
	StockStatusDoc  string            `json:"stockStatusDoc"`
	OutBatch        string            `json:"out_batch"`
	OutProducedate  string            `json:"out_producedate"`
	OutInvaliddate  string            `json:"out_invaliddate"`
}

type ybPlan struct {
	Row          ybRow        `json:"row"`
	ProductName  string       `json:"product_name"`
	NeededQty    int          `json:"needed_qty"`
	FulfilledQty int          `json:"fulfilled_qty"`
	RemainingQty int          `json:"remaining_qty"`
	Shipments    []ybShipment `json:"shipments"`
	BillShort    bool         `json:"bill_short"` // 所在吉客云单号有任一行缺货 → 整单不出(连转换也不做)
}

// ---------------- 拆单算法 (纯逻辑, 注入库存查询) ----------------

// ybPlanExport 跨组织拆单。queryStock(oid, productCode) 返回该组织该货品的现存量行。
// 多行明细共享一个库存池, 避免同仓同批次被重复占用。
func ybPlanExport(queryStock func(orgID, productCode string) []yonsuite.StockRow, rows []ybRow) []ybPlan {
	stockCache := map[string][]yonsuite.StockRow{}
	stockPool := map[string]int{} // oid|wh|product|batch|status → 剩余可用量

	poolKey := func(oid, wh, product, batch, status string) string {
		return oid + "|" + wh + "|" + product + "|" + batch + "|" + status
	}
	getStock := func(oid, productCode string) []yonsuite.StockRow {
		ck := oid + "|" + productCode
		if srs, ok := stockCache[ck]; ok {
			return srs
		}
		srs := queryStock(oid, productCode)
		stockCache[ck] = srs
		for _, sr := range srs {
			stockPool[poolKey(oid, sr.WarehouseCode, productCode, sr.Batchno, sr.StockStatusDoc)] = int(sr.AvailableQty)
		}
		return srs
	}

	plans := make([]ybPlan, 0, len(rows))
	for _, row := range rows {
		productCode := strings.TrimSpace(row.ProductCode)
		targetBatch := strings.TrimSpace(row.TargetBatch)
		whName := strings.TrimSpace(row.WarehouseName)
		needed := 0
		if f, err := strconv.ParseFloat(strings.TrimSpace(row.Qty), 64); err == nil {
			needed = int(f)
		}

		shipments := []ybShipment{}
		remaining := needed
		productName := ""

		for _, org := range ybOrgPriority {
			if remaining <= 0 {
				break
			}
			oid := org.ID
			stockRows := getStock(oid, productCode)
			if len(stockRows) > 0 && productName == "" {
				productName = stockRows[0].ProductName
			}

			inWh := make([]yonsuite.StockRow, 0)
			for _, r := range stockRows {
				if ybWhMatch(whName, r.WarehouseName) {
					inWh = append(inWh, r)
				}
			}
			if len(inWh) == 0 {
				continue
			}

			whGroups := map[string][]yonsuite.StockRow{}
			whOrder := []string{}
			for _, r := range inWh {
				if _, ok := whGroups[r.WarehouseCode]; !ok {
					whOrder = append(whOrder, r.WarehouseCode)
				}
				whGroups[r.WarehouseCode] = append(whGroups[r.WarehouseCode], r)
			}
			sort.Strings(whOrder)

			for _, whCode := range whOrder {
				if remaining <= 0 {
					break
				}
				whRows := whGroups[whCode]
				whRealName := whRows[0].WarehouseName
				poolOf := func(r yonsuite.StockRow) int {
					return stockPool[poolKey(oid, whCode, productCode, r.Batchno, r.StockStatusDoc)]
				}

				// 一律凑成「合格 + 目标批次」。每行按需要的处理分 3 档, 档内不合格优先于废品、池量大优先:
				//   档1 合格且就在目标批次 → 直出(不转)
				//   档2 合格但在别批次     → 批次转换 → 目标批次
				//   档3 不合格/废品(任意批次) → 状态转换成合格(顺带并到目标批次, 一张形态转换单同时改批次+状态)
				// 消费按档序: 合格优先, 坏的最后才动(档3 内不合格先于废品)。
				tierOf := func(r yonsuite.StockRow) int {
					// 未知状态(非 合格/不合格/废品, 如冻结/在途/质押/未来新状态): 业务含义不明, 不碰、不自动转、不消费。
					if ybStatusPri(r.StockStatusDoc) == 99 {
						return 9
					}
					isQ := r.StockStatusDoc == ybQualifiedStatusDoc
					isTarget := targetBatch == "" || r.Batchno == targetBatch
					switch {
					case isQ && isTarget:
						return 1
					case isQ:
						return 2
					default:
						return 3 // 不合格/废品 → 状态转换成合格
					}
				}
				ordered := append([]yonsuite.StockRow{}, whRows...)
				sort.SliceStable(ordered, func(i, j int) bool {
					ti, tj := tierOf(ordered[i]), tierOf(ordered[j])
					if ti != tj {
						return ti < tj
					}
					pi, pj := ybStatusPri(ordered[i].StockStatusDoc), ybStatusPri(ordered[j].StockStatusDoc)
					if pi != pj {
						return pi < pj // 档内: 不合格(1) 先于 废品(2)
					}
					return poolOf(ordered[i]) > poolOf(ordered[j])
				})

				for _, r := range ordered {
					if remaining <= 0 {
						break
					}
					if tierOf(r) >= 9 {
						continue // 未知状态不消费(留库存, 计入缺口让人工处理)
					}
					av := poolOf(r)
					if av <= 0 {
						continue
					}
					use := av
					if remaining < use {
						use = remaining
					}
					stockPool[poolKey(oid, whCode, productCode, r.Batchno, r.StockStatusDoc)] = av - use
					outBatch := targetBatch
					if outBatch == "" {
						outBatch = r.Batchno
					}
					sh := ybShipment{
						OrgID: oid, OrgName: org.Name,
						WarehouseCode: whCode, WarehouseName: whRealName,
						Qty:            use,
						ProductCode:    productCode,
						ProductName:    ybFirstNonEmpty(r.ProductName, productName),
						ProductskuID:   r.ProductskuID, UnitID: r.UnitID, StockUnitID: r.StockUnitID,
						StockStatusDoc: ybQualifiedStatusDoc, // 出库一律合格(转换后即合格)
						OutBatch:       outBatch,
						OutProducedate: r.Producedate, OutInvaliddate: r.Invaliddate,
					}
					if tierOf(r) == 1 {
						// 档1 直出: 现成合格 + 目标批次, 不转
						sh.TargetQtyDirect = use
						sh.ConvertSources = []ybConvertSource{}
					} else {
						// 档2 批次转换 / 档3 状态转换(可能+批次): 都记一个转换源, 转换后= 合格 + 目标批次
						sh.ConvertQty = use
						sh.ConvertSources = []ybConvertSource{{
							FromBatch: r.Batchno, FromProducedate: r.Producedate, FromInvaliddate: r.Invaliddate,
							Qty: use, ProductskuID: r.ProductskuID, UnitID: r.UnitID, StockUnitID: r.StockUnitID,
							StockStatusDoc: r.StockStatusDoc, // 来源状态(合格/不合格/废品), 转换 before 用它
						}}
					}
					shipments = append(shipments, sh)
					remaining -= use
				}
			}
		}

		plans = append(plans, ybPlan{
			Row: row, ProductName: productName,
			NeededQty: needed, FulfilledQty: needed - remaining, RemainingQty: remaining,
			Shipments: shipments,
		})
	}

	// 单据级缺货: 同一吉客云单号下任一行有缺口 → 整号所有行标 BillShort(整单不出, 全有或全无)。
	// 空单号视作各自独立(不会因别的空单号行牵连)。
	shortBill := map[string]bool{}
	for _, p := range plans {
		if p.RemainingQty > 0 && strings.TrimSpace(p.Row.BillNo) != "" {
			shortBill[p.Row.BillNo] = true
		}
	}
	for i := range plans {
		bn := strings.TrimSpace(plans[i].Row.BillNo)
		if (bn != "" && shortBill[bn]) || (bn == "" && plans[i].RemainingQty > 0) {
			plans[i].BillShort = true
		}
	}
	return plans
}

// ---------------- 执行编排 (三阶段, 写真用友) ----------------

type ybConvLog struct {
	FromBatch string `json:"from_batch"`
	Qty       int    `json:"qty"`
	DocCode   string `json:"doc_code"`
	AuditOk   bool   `json:"audit_ok"`
	Skipped   bool   `json:"skipped,omitempty"`   // 防重: 同一笔批次转换10分钟内已提交过, 本次跳过
	Uncertain bool   `json:"uncertain,omitempty"` // 保存传输层失败: 单据可能已建成, 重做前必须核对
	Error     string `json:"error,omitempty"`
}

type ybShipLog struct {
	OrgName       string      `json:"org_name"`
	WarehouseName string      `json:"warehouse_name"`
	Qty           int         `json:"qty"`
	Conversions   []ybConvLog `json:"conversions"`
	OutDocCode    string      `json:"out_doc_code"`
	OutDocID      string      `json:"out_doc_id"`
	AuditOk       bool        `json:"audit_ok"`
	OutSkipped    bool        `json:"out_skipped,omitempty"` // 防重: 同一张出库单10分钟内已提交过, 本次跳过
	Uncertain     bool        `json:"uncertain,omitempty"`   // 出库单保存传输层失败: 单据可能已建成, 重做前必须核对
	BillShort     bool        `json:"bill_short,omitempty"`  // 所在单据缺货, 整单不出(转换也没做)
	Error         string      `json:"error,omitempty"`
	skipOut       bool        // 内部: 批次转换失败 or 单据缺货 → 跳过出库
}

type ybIdx struct{ pi, si int }

// ybExecute 三阶段执行: ①批次转换 save+审核 ②按(组织,仓库,类别)合并出库单 save+审核 ③回填结果。
// ctx: 客户端断开(超时/关页)即停手, 不再往用友写; 防重: 同一笔10分钟内已提交过则跳过, 不重复建单。
func (h *DashboardHandler) ybExecute(ctx context.Context, vouchdate string, plans []ybPlan, groupByBill bool, phase string, onProgress func(done, total int, label string)) []map[string]interface{} {
	h.ybEnsureSubmitLog()
	// phase: "convert"只做批次转换 / "out"只做出库 / "all"或空=两步连做(兼容旧调用)。
	// 拆两步是为了中间人工复查目标批次是否到货(见 handler + 前端三步向导)。
	doConvert := phase != "out"
	doOut := phase != "convert"
	// 进度: convert 阶段按转换笔数计, out 阶段按出库单张数计(Phase2 分组后补进 total)。
	// onProgress 非空才回调(SSE 流式推前端); 每处理一笔 done+1。
	done, total := 0, 0
	if doConvert {
		for pi := range plans {
			if plans[pi].BillShort {
				continue // 缺货单整单不出, 转换不做, 不计进度
			}
			tb := strings.TrimSpace(plans[pi].Row.TargetBatch)
			for si := range plans[pi].Shipments {
				for _, cs := range plans[pi].Shipments[si].ConvertSources {
					total += ybConvDocCount(plans[pi].Shipments[si], cs, tb) // 一条源可能拆 2 张单
				}
			}
		}
	}
	emit := func(label string) {
		done++
		if onProgress != nil {
			onProgress(done, total, label)
		}
	}
	logs := map[ybIdx]*ybShipLog{}
	for pi := range plans {
		for si := range plans[pi].Shipments {
			sh := plans[pi].Shipments[si]
			lg := &ybShipLog{
				OrgName: sh.OrgName, WarehouseName: sh.WarehouseName, Qty: sh.Qty,
				Conversions: []ybConvLog{},
			}
			// 单据缺货: 整单不出, 连转换都不做 → 预置 skipOut + 标记, Phase1/Phase2 都跳过它。
			if plans[pi].BillShort {
				lg.BillShort = true
				lg.skipOut = true
				lg.Error = "所在单据缺货，整单不出（转换也未做）"
			}
			logs[ybIdx{pi, si}] = lg
		}
	}

	if doConvert {
		// ---- 阶段 1: 批次/状态转换 ----
		for pi := range plans {
			if plans[pi].BillShort {
				continue // 缺货单整单不出 → 转换也不做(已确认: 不白改库存)
			}
			targetBatch := strings.TrimSpace(plans[pi].Row.TargetBatch)
			for si := range plans[pi].Shipments {
				sh := plans[pi].Shipments[si]
				lg := logs[ybIdx{pi, si}]
				for _, cs := range sh.ConvertSources {
					convLog := ybConvLog{FromBatch: cs.FromBatch, Qty: cs.Qty}

					// 客户端断了(超时/关页)立即停手: 本条及后续不再写用友。已写的已落流水, 重发会跳过。
					if ctx.Err() != nil {
						convLog.Error = "连接中断，未执行（已执行的请去用友核对，重新提交不会重复建单）"
						lg.Conversions = append(lg.Conversions, convLog)
						lg.Error = convLog.Error
						lg.skipOut = true
						break
					}

					// 一条转换源可能拆 1~2 张形态转换单(批次+状态都变=拆两张:先状态后批次, 见 ybBuildConversionDocs)。
					// 逐张 save+审核; 防重指纹按"每张单"算(带 di), doc1 成功 doc2 失败时重做只补 doc2。
					docs := ybBuildConversionDocs(sh, vouchdate, cs, targetBatch)
					convLog.AuditOk = true // 乐观: 任一张失败置 false
					convLog.Skipped = true // 乐观: 任一张真执行置 false
					for di, body := range docs {
						if ctx.Err() != nil {
							convLog.Error = "连接中断，未执行（已执行的请去用友核对，重新提交不会重复建单）"
							convLog.AuditOk = false
							break
						}
						emit("批次/状态转换 " + cs.FromBatch + "→" + ybConvTargetBatch(cs, targetBatch))

						// 指纹覆盖该张单内容 + di(区分两张), 漏字段会误挡不同业务。
						fp := ybFingerprint("conv_out", ybResolveOrg(sh.OrgID), sh.WarehouseCode,
							sh.ProductCode, ybFirstNonEmpty(cs.ProductskuID, sh.ProductskuID),
							ybFirstNonEmpty(cs.StockStatusDoc, sh.StockStatusDoc),
							cs.FromBatch, targetBatch, cs.FromProducedate, cs.FromInvaliddate,
							strconv.Itoa(cs.Qty), vouchdate, strconv.Itoa(di))
						if dup, prevDoc := h.ybRecentSubmit(fp); dup {
							convLog.DocCode = prevDoc // 这张已提交过, 跳过看下一张
							continue
						}
						convLog.Skipped = false

						wr, err := h.YS.MorphologyConversionSave(body)
						if err != nil {
							convLog.Error = ybErrMsg(wr, err, "转换保存失败")
							convLog.Uncertain = wr == nil // 传输层失败: 转换单可能已建成, 重做前必须核对
							convLog.AuditOk = false
							break
						}
						var sd struct {
							Infos []struct {
								ID   json.Number `json:"id"`
								Code string      `json:"code"`
							} `json:"infos"`
						}
						_ = ybDecodeData(wr, &sd)
						if len(sd.Infos) == 0 {
							convLog.Error = "转换已保存但用友未返回单据信息，无法自动审核，请去用友手动核对"
							convLog.AuditOk = false
							break
						}
						convLog.DocCode = sd.Infos[0].Code
						// 单据已在用友建成(不可逆) → 立即落防重流水。
						h.ybRecordSubmit(fp, "conv_out", convLog.DocCode, vouchdate)
						cvID, _ := sd.Infos[0].ID.Int64()
						if cvID == 0 {
							convLog.Error = "转换已保存但用友未返回单据id，无法自动审核，请去用友手动核对"
							convLog.AuditOk = false
							break
						}
						awr, aerr := h.YS.MorphologyConversionBatchAudit([]int64{cvID})
						if !(aerr == nil && ybAuditOK(awr)) {
							convLog.Error = ybErrMsg(awr, aerr, "转换审核失败")
							convLog.AuditOk = false
							break // 本张(如状态转换)审核失败→库存未真转, 不继续下一张(批次转换)
						}
					}
					lg.Conversions = append(lg.Conversions, convLog)
					if convLog.Error != "" {
						lg.Error = "批次/状态转换失败: " + convLog.Error
						lg.Uncertain = convLog.Uncertain
						lg.skipOut = true
						break
					}
				}
			}
		}

	} // ← if doConvert 结束

	if doOut {
		// ---- 阶段 2: 按 (组织,仓库,bustype,category[,bill]) 合并 → 每组一张其他出库单 ----
		type grpKey struct{ org, wh, bustype, category, bill string }
		groups := map[grpKey][]ybIdx{}
		groupOrder := []grpKey{}
		for pi := range plans {
			row := plans[pi].Row
			bustype := ybFirstNonEmpty(strings.TrimSpace(row.Bustype), "A10001")
			category := ybNormalizeCategory(row.Category)
			for si := range plans[pi].Shipments {
				sh := plans[pi].Shipments[si]
				lg := logs[ybIdx{pi, si}]
				if lg.skipOut || sh.Qty <= 0 {
					continue
				}
				k := grpKey{sh.OrgID, sh.WarehouseCode, bustype, category, ""}
				if groupByBill {
					k.bill = row.BillNo
				}
				if _, ok := groups[k]; !ok {
					groupOrder = append(groupOrder, k)
				}
				groups[k] = append(groups[k], ybIdx{pi, si})
			}
		}

		total += len(groupOrder) // 进度: 出库单张数补进总数
		for _, k := range groupOrder {
			grp := groups[k]

			// 客户端断了: 本组及后续出库单不再写用友, 标注中断。
			if ctx.Err() != nil {
				for _, gi := range grp {
					if logs[gi].Error == "" {
						logs[gi].Error = "连接中断，本出库单未执行（已执行的请去用友核对，重新提交不会重复建单）"
					}
				}
				continue
			}

			emit("出库单 " + plans[grp[0].pi].Shipments[grp[0].si].WarehouseName)

			items := make([]map[string]interface{}, 0, len(grp))
			itemSigs := make([]string, 0, len(grp)) // 出库单内容签名(防重指纹用), 与明细一一对应
			billNos := []string{}
			seenBill := map[string]bool{}
			for _, gi := range grp {
				sh := plans[gi.pi].Shipments[gi.si]
				row := plans[gi.pi].Row
				outBatch := ybFirstNonEmpty(sh.OutBatch, strings.TrimSpace(row.TargetBatch))
				prod, inv := sh.OutProducedate, sh.OutInvaliddate
				if (prod == "" || inv == "") && len(sh.ConvertSources) > 0 {
					cs0 := sh.ConvertSources[0]
					prod = ybFirstNonEmpty(prod, cs0.FromProducedate)
					inv = ybFirstNonEmpty(inv, cs0.FromInvaliddate)
				}
				items = append(items, ybBuildOutItem(sh, outBatch, prod, inv))
				// 签名须覆盖 ybBuildOutItem 写入的全部字段(状态/产日/到期/sku), 漏字段会把不同出库误判成重复。
				itemSigs = append(itemSigs, strings.Join([]string{
					sh.ProductCode, strconv.Itoa(sh.Qty), outBatch,
					sh.StockStatusDoc, prod, inv, sh.ProductskuID,
				}, "#"))
				if row.BillNo != "" && !seenBill[row.BillNo] {
					seenBill[row.BillNo] = true
					billNos = append(billNos, row.BillNo)
				}
			}
			memo := ""
			if len(billNos) > 0 {
				memo = "来自吉客云出库单 " + strings.Join(billNos, ", ")
			}

			// 防重: 同一张出库单(组织+仓库+类型+类别+单号+明细内容)10分钟内已提交过 → 跳过, 不重复建单。
			sort.Strings(itemSigs)
			fp := ybFingerprint("out", k.org, k.wh, k.bustype, k.category, k.bill, vouchdate,
				strings.Join(itemSigs, ","))
			if dup, prevDoc := h.ybRecentSubmit(fp); dup {
				for _, gi := range grp {
					lg := logs[gi]
					// 跳过=上次已建单, 审核状态以用友为准, 不谎报已审核。
					lg.OutSkipped = true
					lg.OutDocCode = prevDoc
					if lg.Error == "" {
						lg.Error = "10分钟内已提交过，已自动跳过防止重复建单（审核状态以用友为准）"
					}
				}
				continue
			}

			body := ybBuildOtherOutBody(k.org, vouchdate, k.bustype, k.wh, memo, k.category, items)
			wr, err := h.YS.OtherOutSave(body)
			if err != nil {
				em := ybErrMsg(wr, err, "其他出库单保存失败")
				uncertain := wr == nil // 传输层失败: 出库单可能已建成, 重做前必须核对
				for _, gi := range grp {
					if logs[gi].Error == "" {
						logs[gi].Error = em
						logs[gi].Uncertain = uncertain
					}
				}
				continue
			}
			var od struct {
				ID   json.Number `json:"id"`
				Code string      `json:"code"`
			}
			_ = ybDecodeData(wr, &od)
			docCode := od.Code
			outID, _ := od.ID.Int64()
			// 出库单已在用友建成(不可逆) → 立即落防重流水, 即便后续审核失败/连接断, 重发也不会再建。
			h.ybRecordSubmit(fp, "out", docCode, vouchdate)
			auditOk := false
			auditErr := ""
			if outID != 0 {
				awr, aerr := h.YS.OtherOutBatchAudit([]int64{outID})
				auditOk = aerr == nil && ybAuditOK(awr)
				if !auditOk {
					auditErr = ybErrMsg(awr, aerr, "其他出库审核失败")
				}
			} else {
				// 保存返回 code 200 但无单据 id (YS 异常响应): 单已建但无法自动审核, 明确报出
				auditErr = "其他出库单已保存但用友未返回单据id，无法自动审核，请去用友手动审核"
			}
			for _, gi := range grp {
				lg := logs[gi]
				lg.OutDocCode = docCode
				if outID != 0 {
					lg.OutDocID = strconv.FormatInt(outID, 10)
				}
				lg.AuditOk = auditOk
				if auditErr != "" && lg.Error == "" {
					lg.Error = auditErr
				}
			}
		}

	} // ← if doOut 结束

	// ---- 阶段 3: 构造返回 ----
	out := make([]map[string]interface{}, 0, len(plans))
	for pi := range plans {
		subs := make([]*ybShipLog, 0, len(plans[pi].Shipments))
		for si := range plans[pi].Shipments {
			subs = append(subs, logs[ybIdx{pi, si}])
		}
		out = append(out, map[string]interface{}{
			"row":           plans[pi].Row,
			"needed_qty":    plans[pi].NeededQty,
			"fulfilled_qty": plans[pi].FulfilledQty,
			"remaining_qty": plans[pi].RemainingQty,
			"shipments":     subs,
		})
	}
	return out
}

// ---------------- YS 报文构造 ----------------

// ybBuildMorphDoc 一张形态转换单报文 (before lineType=1 → after lineType=2)。
// before/after 的批次、状态各自可控; 日期沿用来源 cs。
func ybBuildMorphDoc(sh ybShipment, vouchdate string, cs ybConvertSource, beforeBatch, afterBatch, beforeStatus, afterStatus string) map[string]interface{} {
	vd := ybFmtDate(vouchdate) + " 00:00:00"
	unitID := ybFirstNonEmpty(cs.UnitID, sh.UnitID)
	stockUnit := ybFirstNonEmpty(cs.StockUnitID, cs.UnitID, sh.StockUnitID, sh.UnitID)

	base := func() map[string]interface{} {
		m := map[string]interface{}{
			"warehouse":        sh.WarehouseCode,
			"product":          sh.ProductCode,
			"mainUnitId":       unitID,
			"stockUnitId":      stockUnit,
			"invExchRate":      "1",
			"unitExchangeType": 0,
			"qty":              cs.Qty,
			"subQty":           cs.Qty,
		}
		if sku := ybFirstNonEmpty(cs.ProductskuID, sh.ProductskuID); sku != "" && sku != "0" {
			m["productsku"] = sku
		}
		if cs.FromProducedate != "" {
			m["producedate"] = ybFmtDate(cs.FromProducedate) + " 00:00:00"
		}
		if cs.FromInvaliddate != "" {
			m["invaliddate"] = ybFmtDate(cs.FromInvaliddate) + " 00:00:00"
		}
		return m
	}

	before := base()
	before["groupNumber"] = "1"
	before["lineType"] = "1"
	before["batchno"] = beforeBatch
	before["stockStatusDoc"] = beforeStatus

	after := base()
	after["groupNumber"] = "1"
	after["lineType"] = "2"
	after["batchno"] = afterBatch
	after["stockStatusDoc"] = afterStatus

	return map[string]interface{}{
		"data": map[string]interface{}{
			"org":                        ybResolveOrg(sh.OrgID),
			"businesstypeId":             "A70002",
			"conversionType":             "1",
			"mcType":                     "1",
			"vouchdate":                  vd,
			"beforeWarehouse":            sh.WarehouseCode,
			"afterWarehouse":             sh.WarehouseCode,
			"_status":                    "Insert",
			"morphologyconversiondetail": []map[string]interface{}{before, after},
		},
	}
}

// ybConvTargetBatch 转换后落到哪个批次 (空目标批次=不改批次, 同来源批次)。
func ybConvTargetBatch(cs ybConvertSource, targetBatch string) string {
	return ybFirstNonEmpty(targetBatch, cs.FromBatch)
}

// ybBuildConversionDocs 把一条转换源拆成 1 或 2 张形态转换单。
// 用友限制: 一张形态转换单只能"只改批次"或"只改状态", 不能批次+状态同时改
//   (一张单里货品状态没变化才能做批次转换; 状态变化时不能同时变批次)。
// 所以批次和状态都要变时拆两张: ①先状态转换(同批次, 来源状态→合格) ②再批次转换(同合格, 来源批次→目标批次)。
// 只改一样时一张: 纯批次转换(合格→合格只改批次) 或 纯状态转换(不合格/废品→合格, 批次不变)。
func ybBuildConversionDocs(sh ybShipment, vouchdate string, cs ybConvertSource, targetBatch string) []map[string]interface{} {
	fromStatus := ybFirstNonEmpty(cs.StockStatusDoc, sh.StockStatusDoc)
	fromBatch := cs.FromBatch
	toBatch := ybConvTargetBatch(cs, targetBatch)
	statusChanges := fromStatus != ybQualifiedStatusDoc
	batchChanges := toBatch != fromBatch

	if statusChanges && batchChanges {
		return []map[string]interface{}{
			ybBuildMorphDoc(sh, vouchdate, cs, fromBatch, fromBatch, fromStatus, ybQualifiedStatusDoc),   // ①状态转换(同批次)
			ybBuildMorphDoc(sh, vouchdate, cs, fromBatch, toBatch, ybQualifiedStatusDoc, ybQualifiedStatusDoc), // ②批次转换(同合格)
		}
	}
	// 一张: 只改批次(状态不变) 或 只改状态(批次不变)
	return []map[string]interface{}{
		ybBuildMorphDoc(sh, vouchdate, cs, fromBatch, toBatch, fromStatus, ybQualifiedStatusDoc),
	}
}

// ybConvDocCount 一条转换源会拆成几张形态转换单(1 或 2)。批次+状态都变=2, 否则=1。
func ybConvDocCount(sh ybShipment, cs ybConvertSource, targetBatch string) int {
	fromStatus := ybFirstNonEmpty(cs.StockStatusDoc, sh.StockStatusDoc)
	if fromStatus != ybQualifiedStatusDoc && ybConvTargetBatch(cs, targetBatch) != cs.FromBatch {
		return 2
	}
	return 1
}

// ybBuildOutItem 其他出库单单行明细。
func ybBuildOutItem(sh ybShipment, outBatch, prod, inv string) map[string]interface{} {
	stockUnit := ybFirstNonEmpty(sh.StockUnitID, sh.UnitID)
	line := map[string]interface{}{
		"_status":          "Insert",
		"product":          sh.ProductCode,
		"qty":              sh.Qty,
		"subQty":           sh.Qty,
		"invExchRate":      "1",
		"unitExchangeType": 0,
	}
	if outBatch != "" {
		line["batchno"] = outBatch
	}
	if sh.UnitID != "" {
		line["unit"] = sh.UnitID
	}
	if stockUnit != "" {
		line["stockUnitId"] = stockUnit
	}
	if sh.ProductskuID != "" && sh.ProductskuID != "0" {
		line["productsku"] = sh.ProductskuID
	}
	if prod != "" {
		line["producedate"] = ybFmtDate(prod) + " 00:00:00"
	}
	if inv != "" {
		line["invaliddate"] = ybFmtDate(inv) + " 00:00:00"
	}
	if sh.StockStatusDoc != "" && sh.StockStatusDoc != "0" {
		line["stockStatusDoc"] = sh.StockStatusDoc
	}
	return line
}

// ybBuildOtherOutBody 其他出库单报文 (一组明细一张单)。
func ybBuildOtherOutBody(orgID, vouchdate, bustype, whCode, memo, categoryCode string, items []map[string]interface{}) map[string]interface{} {
	org := ybResolveOrg(orgID)
	data := map[string]interface{}{
		"_status":       "Insert",
		"org":           org,
		"accountOrg":    org,
		"vouchdate":     ybFmtDate(vouchdate),
		"bustype":       ybFirstNonEmpty(bustype, "A10001"),
		"warehouse":     whCode,
		"memo":          memo,
		"othOutRecords": items,
	}
	if categoryCode != "" {
		data["othOutRecordDefineCharacter"] = map[string]interface{}{"SF001": categoryCode}
	}
	return map[string]interface{}{"data": data}
}

// ---------------- HTTP handlers ----------------

// YonbipExportPlan POST /api/yonbip/export-plan — 拆单计划 (只查库存, 不写)。
func (h *DashboardHandler) YonbipExportPlan(w http.ResponseWriter, r *http.Request) {
	if h.YS == nil {
		writeError(w, http.StatusServiceUnavailable, "用友 YS 未配置")
		return
	}
	var req struct {
		Rows []ybRow `json:"rows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求解析失败: "+err.Error())
		return
	}
	if len(req.Rows) == 0 {
		writeError(w, http.StatusBadRequest, "未解析到任何出库行")
		return
	}
	queryStock := func(orgID, productCode string) []yonsuite.StockRow {
		srs, err := h.YS.QueryStockByCondition(orgID, productCode, "", "", "")
		if err != nil {
			return nil
		}
		return srs
	}
	plans := ybPlanExport(queryStock, req.Rows)
	writeJSON(w, map[string]interface{}{"plans": plans})
}

// YonbipExportExecute POST /api/yonbip/export-execute — 真执行 (写用友, 不可逆)。
func (h *DashboardHandler) YonbipExportExecute(w http.ResponseWriter, r *http.Request) {
	if h.YS == nil {
		writeError(w, http.StatusServiceUnavailable, "用友 YS 未配置")
		return
	}
	var req struct {
		Vouchdate   string   `json:"vouchdate"`
		Plans       []ybPlan `json:"plans"`
		GroupByBill bool     `json:"group_by_bill"`
		Phase       string   `json:"phase"` // convert只转换 / out只出库 / all|空=两步连做(兼容)
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求解析失败: "+err.Error())
		return
	}
	if len(req.Plans) == 0 {
		writeError(w, http.StatusBadRequest, "无计划可执行")
		return
	}
	if req.Phase != "" && req.Phase != "convert" && req.Phase != "out" && req.Phase != "all" {
		writeError(w, http.StatusBadRequest, "phase 只能是 convert/out/all")
		return
	}
	// 防呆: phase=out 时计划里若还有未执行的转换(批次/状态), 拒绝 —— 否则会出"还没转出来"的合格库存。
	// 正常流程: 执行①转换 → 点【生成/刷新计划】重查, 转换好的会变成直出(无 ConvertSources)才放行②。
	// 后端硬挡, 不只靠前端按钮 disable(防绕过/重放)。
	if req.Phase == "out" && ybPlanHasUnconverted(req.Plans) {
		writeError(w, http.StatusBadRequest, "计划里还有未执行的转换（批次/状态），请先执行①转换并点【生成/刷新计划】复查，再出库")
		return
	}
	// 大批量出库会跑很久(每笔多次用友调用 × 1.1s 限流), 默认 120s WriteTimeout 会半路掐断连接,
	// 导致后端还在写用友、前端却收到"失败"→用户重发→重复建单。这里单独清掉本请求的写超时。
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	// stream=1: SSE 流式推执行进度(每处理一笔推一条 progress, 最后推一条 result)。
	// 靠 statusRecorder.Unwrap() 让 Flush 能穿透访问日志包装层(见 main.go)。
	if r.URL.Query().Get("stream") == "1" {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		writeSSE := func(event string, payload interface{}) {
			b, _ := json.Marshal(payload)
			_, _ = w.Write([]byte("event: " + event + "\ndata: "))
			_, _ = w.Write(b)
			_, _ = w.Write([]byte("\n\n"))
			_ = rc.Flush()
		}
		onProgress := func(done, total int, label string) {
			writeSSE("progress", map[string]interface{}{"done": done, "total": total, "label": label})
		}
		results := h.ybExecute(r.Context(), ybFmtDate(req.Vouchdate), req.Plans, req.GroupByBill, req.Phase, onProgress)
		writeSSE("result", map[string]interface{}{"results": results})
		return
	}

	results := h.ybExecute(r.Context(), ybFmtDate(req.Vouchdate), req.Plans, req.GroupByBill, req.Phase, nil)
	writeJSON(w, map[string]interface{}{"results": results})
}

// ---------------- 小助手 ----------------

// ybPlanHasUnconverted 计划里是否还有未执行的转换(批次/状态)。
// phase=out 前用它挡: 有 ConvertSources = 还没转, 直接出会拉到不存在的合格库存。
func ybPlanHasUnconverted(plans []ybPlan) bool {
	for _, p := range plans {
		if p.BillShort {
			continue // 缺货单整单不出, 不转换, 不算待转换(否则永远挡住②出库)
		}
		for _, sh := range p.Shipments {
			if len(sh.ConvertSources) > 0 {
				return true
			}
		}
	}
	return false
}

func ybResolveOrg(orgID string) string {
	if strings.TrimSpace(orgID) == "" {
		return ybDefaultOrgID
	}
	return orgID
}

func ybFirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func ybIsDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ybFmtDate "20260531" / "2026-05-31" / "2026/05/31" → "2026-05-31"。
func ybFmtDate(d string) string {
	t := strings.TrimSpace(d)
	s := strings.NewReplacer("-", "", "/", "").Replace(t)
	if len(s) >= 8 && ybIsDigits(s[:8]) {
		return s[:4] + "-" + s[4:6] + "-" + s[6:8]
	}
	return t
}

// ybDecodeData 用 UseNumber 解 WriteResp.Data (防 19 位 id 精度丢失)。
func ybDecodeData(wr *yonsuite.WriteResp, v interface{}) error {
	if wr == nil || len(wr.Data) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(wr.Data))
	dec.UseNumber()
	return dec.Decode(v)
}

// ybAuditOK 审核成功 = code 200 且 failCount 0。
func ybAuditOK(wr *yonsuite.WriteResp) bool {
	if wr == nil || wr.Code != "200" {
		return false
	}
	var d struct {
		FailCount json.Number `json:"failCount"`
	}
	_ = ybDecodeData(wr, &d)
	fc, _ := d.FailCount.Int64()
	return fc == 0
}

// ybErrMsg 优先用 YS message, 再用 err, 最后默认文案。
func ybErrMsg(wr *yonsuite.WriteResp, err error, dflt string) string {
	if wr != nil && strings.TrimSpace(wr.Message) != "" {
		return wr.Message
	}
	if err != nil {
		return err.Error()
	}
	return dflt
}
