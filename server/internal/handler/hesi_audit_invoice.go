package handler

// hesi_audit_invoice.go - 发票域规则: 规则8·10发票审核与无票豁免/规则14健康证/规则18广告费
// (2026-06-25 从 hesi_audit_rules.go 拆出, 纯挪位置不改逻辑)

import (
	"fmt"
	"log"
	"strconv"
	"strings"
)

// 样品 fee_type 集合 (规则 10 豁免 1: 研发部门 + 样品 = 无票豁免)
var sampleFeeTypes = map[string]string{
	"ID01Fk0FsIqhDV": "赠品及样品",
	"ID01KhLrRhAWp9": "样品费用",
	"ID01Fk0B0hMfF5": "小样推广赠品、样品",
}

// 出差/餐补/市内补贴 fee_type 集合 (规则 10 豁免 2: 补贴类无票豁免)
var subsidyFeeTypes = map[string]string{
	"ID01Fk0MQBAAQ7": "补贴",
	"ID01Fk0MQBAB6D": "市内补贴",
	"ID01FkA9pR8zGT": "餐费补贴",
}

// 健康证及体检 fee_type (规则 14, 合思 feeTypes API 2026-06-11 实查, 无子类)
const healthExamFeeTypeID = "ID01KTruvX23pl"

// 广告费 fee_type (规则 18; 旧版已停用 ID 兜底历史单)
var adFeeTypes = map[string]bool{
	"ID01MBFo8YEVQ3": true, // 广告费 (启用中, 父类=广告宣传费用)
	"ID01KhLrRhAVl5": true, // 广告费 (旧, 已停用)
}

// ruleInvoiceChecks 规则 8: 发票审核 4 项 + 规则 10: 无票判定 + 豁免
//
// 8-1: 发票抬头 (buyer_name) 必须 = 所属公司 (raw_json.法人实体 → invoice_title)
// 8-2: 发票税号 (buyer_tax_no) 必须 = 所属公司税号 (开票资料 PDF 字典)
// 8-3: 每个明细发票合计 (sum total_amount) ≥ 明细金额 (有票才校验)
// 8-4: 开票时间 (invoice_date) 距单据提交 ≤1月 OK / 1-3月 manual / >3月 reject
// 10:  每个明细必须有票, 除非属于豁免:
//   - 豁免 A: 研发部门 (递归含"研发") + 样品 fee_type → 无票 OK
//   - 豁免 B: 补贴 fee_type (出差补贴/市内补贴/餐费补贴) → 无票 OK
//   - 豁免 C: u_无票原因截图说明 (实查 0 单, 当前不实现, 等合思后台加字段)
//
// 返回 (rejectReasons, warnings)
func (h *DashboardHandler) ruleInvoiceChecks(raw map[string]interface{}, ownerDeptID, flowID string, isForeign bool) ([]string, []string) {
	if flowID == "" {
		return nil, nil
	}

	// 拉单据全部发票 + LEFT JOIN detail 拿 detail_no 行号
	type inv struct {
		detailID  string
		detailNo  int
		buyerName string
		taxNo     string
		date      int64
		total     float64
		approve   float64
	}
	rows, err := h.DB.Query(`SELECT
		IFNULL(i.detail_id,''), IFNULL(d.detail_no, 0),
		IFNULL(i.buyer_name,''), IFNULL(i.buyer_tax_no,''),
		IFNULL(i.invoice_date,0), IFNULL(i.total_amount,0), IFNULL(i.approve_amount,0)
		FROM hesi_flow_invoice i
		LEFT JOIN hesi_flow_detail d ON i.detail_id = d.detail_id AND d.flow_id = i.flow_id
		WHERE i.flow_id = ?`, flowID)
	if err != nil {
		log.Printf("[hesi-audit] 规则8/10 查发票失败 flow=%s: %v", flowID, err)
		return nil, []string{"发票数据读取失败, 规则 8/10 未自动判定, 需人工核发票"}
	}
	defer rows.Close()
	var invoices []inv
	invoiceReadBroken := false
	for rows.Next() {
		var i inv
		if err := rows.Scan(&i.detailID, &i.detailNo, &i.buyerName, &i.taxNo, &i.date, &i.total, &i.approve); err == nil {
			invoices = append(invoices, i)
		} else {
			if !invoiceReadBroken { // 只记首条, 防 DB 故障时每行刷一条日志
				log.Printf("[hesi-audit] 规则8/10 发票行解析失败 flow=%s: %v", flowID, err)
			}
			invoiceReadBroken = true
		}
	}
	if err := rows.Err(); err != nil {
		invoiceReadBroken = true
		log.Printf("[hesi-audit] 规则8/10 发票读取中断 flow=%s: %v", flowID, err)
	}
	// 发票数据不完整时规则 8/10 两个方向都会判错 (漏驳/误驳), 整体降级转人工
	if invoiceReadBroken {
		return nil, []string{"发票数据读取不完整, 规则 8/10 未自动判定, 需人工核发票"}
	}
	// 拉 details (从 raw_json) → 行号/金额/fee_type (用于规则 8-3 + 规则 10)
	details, _ := raw["details"].([]interface{})
	detailAmtMap := map[string]float64{}
	detailNoMap := map[string]int{}
	detailFeeTypeMap := map[string]string{}
	allDetailIDs := []string{}
	for _, iv := range invoices {
		if iv.detailNo > 0 {
			detailNoMap[iv.detailID] = iv.detailNo
		}
	}
	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		// detailId/detailNo/amount 都嵌在 feeTypeForm 子对象里 (合思 raw_json 结构实查)
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		did, _ := form["detailId"].(string)
		if did == "" {
			continue
		}
		allDetailIDs = append(allDetailIDs, did)
		if no, ok := form["detailNo"].(float64); ok && int(no) > 0 {
			detailNoMap[did] = int(no)
		}
		// feeTypeId 在 detail 顶层 (跟其他规则一致)
		if ft, ok := dm["feeTypeId"].(string); ok {
			detailFeeTypeMap[did] = ft
		}
		if a, ok := form["amount"].(map[string]interface{}); ok {
			if s, ok := a["standard"].(string); ok {
				if v, e := strconv.ParseFloat(s, 64); e == nil {
					detailAmtMap[did] = v
				}
			}
		}
	}

	// 规则 10: 无票判定 — 每个明细必须有票, 除非属于豁免
	// 跑哥规则: 研发部门 + 样品 / 出差补贴类 / 特殊无票截图说明 (字段未存, 走 manual)
	detailHasInvoice := map[string]bool{}
	for _, iv := range invoices {
		detailHasInvoice[iv.detailID] = true
	}
	isResearch := h.isResearchDept(ownerDeptID)
	var noInvoiceReject, missingFeeType []int
	for _, did := range allDetailIDs {
		if detailHasInvoice[did] {
			continue // 有票, 不进规则 10
		}
		// 豁免 E: 外币(国外)无票 — 整单检测到外币汇率 → 无票OK, 自动通过 (跑哥 2026-06-25)
		if isForeign {
			continue
		}
		feeTypeID := detailFeeTypeMap[did]
		no := detailNoMap[did]
		// raw_json 缺 feeTypeId 时豁免判定全失配, 不能当 "不豁免" 误驳, 转人工
		if feeTypeID == "" {
			missingFeeType = append(missingFeeType, no)
			continue
		}
		// 豁免 B: 补贴类 (出差/餐补/市内)
		if _, ok := subsidyFeeTypes[feeTypeID]; ok {
			continue
		}
		// 豁免 A: 研发部门 + 样品 fee_type
		if _, ok := sampleFeeTypes[feeTypeID]; ok && isResearch {
			continue
		}
		// 豁免 D: 私车公用 (按里程核账走规则 12-1, 本就无发票; 跑哥 2026-06-17)
		if feeTypeID == driveFeeTypeID {
			continue
		}
		// 都不豁免 → reject
		noInvoiceReject = append(noInvoiceReject, no)
	}
	var missingFeeTypeWarn []string
	if len(missingFeeType) > 0 {
		missingFeeTypeWarn = append(missingFeeTypeWarn, fmt.Sprintf("明细 %v 无发票且费用类型缺失, 豁免无法自动判定, 需人工核 (规则 10)", uniqueInts(missingFeeType)))
	}
	if len(invoices) == 0 {
		// 无发票时, 只跑规则 10 — 其他规则 8-1/8-2/8-3/8-4 都不适用
		if len(noInvoiceReject) > 0 {
			return []string{fmt.Sprintf("明细 %v 无发票, 不属于豁免 (规则 10: 仅研发样品/出差补贴/私车公用/特殊截图说明 豁免)", uniqueInts(noInvoiceReject))}, missingFeeTypeWarn
		}
		return nil, missingFeeTypeWarn
	}

	var rejects, warnings []string
	warnings = append(warnings, missingFeeTypeWarn...)

	// 拉所属公司 (法人实体) 的开票字典
	legalEntityID, _ := raw["法人实体"].(string)
	var expectedTitle, expectedTaxNo string
	if legalEntityID != "" {
		_ = h.DB.QueryRow(`SELECT IFNULL(invoice_title,''), IFNULL(tax_no,'') FROM hesi_legal_entity_invoice_info WHERE legal_entity_id = ? AND active=1 LIMIT 1`,
			legalEntityID).Scan(&expectedTitle, &expectedTaxNo)
	}

	// 规则 8-1/8-2: 抬头+税号 vs 法人实体 (法人实体未在字典 → 转人工)
	if expectedTitle == "" {
		warnings = append(warnings, "所属公司未在开票资料字典内, 抬头/税号无法判定 (规则 8-1/8-2)")
	} else {
		var wrongTitle, wrongTax []int
		for _, iv := range invoices {
			no := detailNoMap[iv.detailID]
			if iv.buyerName != "" && iv.buyerName != expectedTitle {
				wrongTitle = append(wrongTitle, no)
			}
			if iv.taxNo != "" && iv.taxNo != expectedTaxNo {
				wrongTax = append(wrongTax, no)
			}
		}
		if len(wrongTitle) > 0 {
			rejects = append(rejects, fmt.Sprintf("明细 %v 发票抬头 ≠「%s」(规则 8-1)", uniqueInts(wrongTitle), expectedTitle))
		}
		if len(wrongTax) > 0 {
			rejects = append(rejects, fmt.Sprintf("明细 %v 发票税号 ≠「%s」(规则 8-2)", uniqueInts(wrongTax), expectedTaxNo))
		}
	}

	// 规则 8-3: 每个明细发票合计 ≥ 明细金额
	detailInvoiceSum := map[string]float64{}
	for _, iv := range invoices {
		amt := iv.total
		if amt == 0 {
			amt = iv.approve
		}
		detailInvoiceSum[iv.detailID] += amt
	}
	var undetAmt []int // 有票但价税合计/核定额都识别不出(定额发票等) → 报销≤票面无法自动核, 转人工 (樊雪娇 2026-06-18)
	for did, detailAmt := range detailAmtMap {
		sumInv := detailInvoiceSum[did]
		switch {
		case sumInv > 0:
			// float64 精度容差 0.01 (一分钱), 业务一分钱差异不计较
			if sumInv+0.01 < detailAmt {
				no := detailNoMap[did]
				rejects = append(rejects, fmt.Sprintf("明细#%d 报销 ¥%.2f > 发票合计 ¥%.2f (规则 8-3)", no, detailAmt, sumInv))
			}
		case detailHasInvoice[did] && detailAmt > 0:
			// 有发票但金额=0(系统识别不出, 如定额发票): 报销≤票面没法自动核对 → 人工 (樊雪娇 2026-06-18)
			undetAmt = append(undetAmt, detailNoMap[did])
		}
	}
	if len(undetAmt) > 0 {
		warnings = append(warnings, fmt.Sprintf("明细 %v 发票金额无法识别(如定额发票), 报销额无法自动核对票面, 需人工核 (规则 8-3)", uniqueInts(undetAmt)))
	}

	// 规则 8-4: 开票时间 (距单据提交) ≤1月 OK / 1-3月 manual / >3月 reject
	// raw_json.submitDate 是 毫秒级时间戳
	submitDate := int64(0)
	if v, ok := raw["submitDate"].(float64); ok {
		submitDate = int64(v)
	}
	if submitDate > 0 {
		const month = int64(30 * 24 * 3600 * 1000)
		var stale1m, stale3m []int
		for _, iv := range invoices {
			if iv.date == 0 {
				continue
			}
			// 健康证及体检明细不走 3 个月通用线 (规则 14 单独给 6 个月)
			if detailFeeTypeMap[iv.detailID] == healthExamFeeTypeID {
				continue
			}
			diff := submitDate - iv.date
			no := detailNoMap[iv.detailID]
			if diff > 3*month {
				stale3m = append(stale3m, no)
			} else if diff > month {
				stale1m = append(stale1m, no)
			}
		}
		if len(stale3m) > 0 {
			rejects = append(rejects, fmt.Sprintf("明细 %v 发票开票时间 > 3 个月 (规则 8-4)", uniqueInts(stale3m)))
		}
		if len(stale1m) > 0 {
			warnings = append(warnings, fmt.Sprintf("明细 %v 发票开票时间 1-3 个月, 需酌情核 (规则 8-4)", uniqueInts(stale1m)))
		}
	}

	// 规则 10: 部分明细无票且不豁免 → reject (有票明细的规则 8 已上面跑过)
	if len(noInvoiceReject) > 0 {
		rejects = append(rejects, fmt.Sprintf("明细 %v 无发票, 不属于豁免 (规则 10: 仅研发样品/出差补贴/私车公用/特殊截图说明 豁免)", uniqueInts(noInvoiceReject)))
	}

	return rejects, warnings
}

// ruleHealthExam 规则 14: 健康证及体检 (跑哥 2026-06-11)
// 触发: detail.feeTypeId = 健康证及体检
// 判定: ① 明细金额 ≤ ¥100  ② 发票开票时间距单据提交 ≤ 6 个月 (本类型不走规则 8-4 的 3 个月通用线)
// 两条都满足通过, 否则建议驳回。无票场景由规则 10 兜底 (体检不在豁免清单)。
func (h *DashboardHandler) ruleHealthExam(raw map[string]interface{}, flowID string) []string {
	details, _ := raw["details"].([]interface{})
	type hit struct {
		id     string
		no     int
		amount float64
	}
	var hits []hit
	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		if ft, _ := dm["feeTypeId"].(string); ft != healthExamFeeTypeID {
			continue
		}
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		did, _ := form["detailId"].(string)
		no := 0
		if n, ok := form["detailNo"].(float64); ok {
			no = int(n)
		}
		var amt float64
		if a, ok := form["amount"].(map[string]interface{}); ok {
			if s, ok := a["standard"].(string); ok {
				amt, _ = strconv.ParseFloat(s, 64)
			}
		}
		hits = append(hits, hit{did, no, amt})
	}
	if len(hits) == 0 {
		return nil
	}

	var rejects []string
	for _, ht := range hits {
		// 金额解析失败不静默放行 (沿用规则 5 二审定的"偏严"先例)
		if ht.amount <= 0 {
			rejects = append(rejects, fmt.Sprintf("明细#%d 健康证及体检金额无法识别 (规则 14)", ht.no))
			continue
		}
		// 跑哥规则: ≤100 通过, 100.01 起驳回 (不留容差)
		if ht.amount > 100 {
			rejects = append(rejects, fmt.Sprintf("明细#%d 健康证及体检 ¥%.2f > 标准 ¥100 (规则 14)", ht.no, ht.amount))
		}
	}

	// 发票开票时间 ≤ 6 个月 (距单据提交)
	submitDate := int64(0)
	if v, ok := raw["submitDate"].(float64); ok {
		submitDate = int64(v)
	}
	if submitDate > 0 && flowID != "" {
		idSet := map[string]int{}
		for _, ht := range hits {
			if ht.id != "" {
				idSet[ht.id] = ht.no
			}
		}
		if len(idSet) > 0 {
			rows, err := h.DB.Query(`SELECT IFNULL(detail_id,''), IFNULL(invoice_date,0) FROM hesi_flow_invoice WHERE flow_id=?`, flowID)
			if err != nil {
				// 查库失败 → 6 个月检查跳过, 但留日志别静默 (今天的主题: 静默失败要吵出来)
				log.Printf("[hesi-audit] 规则14 查发票失败 flow=%s: %v (开票时限检查跳过)", flowID, err)
			}
			if err == nil {
				defer rows.Close()
				const month = int64(30 * 24 * 3600 * 1000)
				var stale []int
				for rows.Next() {
					var did string
					var date int64
					if rows.Scan(&did, &date) != nil {
						continue
					}
					no, ok := idSet[did]
					if !ok || date == 0 {
						continue
					}
					if submitDate-date > 6*month {
						stale = append(stale, no)
					}
				}
				if len(stale) > 0 {
					rejects = append(rejects, fmt.Sprintf("明细 %v 健康证及体检发票开票时间 > 6 个月 (规则 14)", uniqueInts(stale)))
				}
			}
		}
	}
	return rejects
}

// adInvRef 广告费明细关联的一张发票 (规则 18 判定输入)
type adInvRef struct {
	no        int    // 明细号
	invoiceID string // 合思发票 ID
	number    string // 发票号码 (提示里显示尾号)
}

// checkAdInvoiceItems 规则 18 纯判定: 发票货物明细行名称全部不含"广告/推广" → 建议驳回
// names: invoiceID → 该发票所有明细行项目名称; 接口没回明细的发票不判 (不冤枉)
func checkAdInvoiceItems(invs []adInvRef, names map[string][]string) []string {
	var rejects []string
	for _, iv := range invs {
		items := names[iv.invoiceID]
		if len(items) == 0 {
			continue
		}
		hit := false
		for _, n := range items {
			if strings.Contains(n, "广告") || strings.Contains(n, "推广") {
				hit = true
				break
			}
		}
		if !hit {
			show := strings.Join(items, "、")
			if r := []rune(show); len(r) > 40 {
				show = string(r[:40]) + "…"
			}
			tail := iv.number
			if len(tail) > 8 {
				tail = tail[len(tail)-8:]
			}
			rejects = append(rejects, fmt.Sprintf("明细#%d 广告费发票(尾号%s)项目名称「%s」不含'广告/推广' (规则 18)", iv.no, tail, show))
		}
	}
	return rejects
}

// checkNonAdInvoiceItems 规则 18 反向 (樊雪娇 2026-06-17, 跑哥严格口径):
// 非广告费明细的发票货物行名称含"广告/推广" → 费用类型与发票不符, 应报广告费, 建议驳回。
// 跑哥拍板严格: 业务宣传费等同家族类型也驳 (广告/推广发票只能报广告费)。
func checkNonAdInvoiceItems(invs []adInvRef, names map[string][]string) []string {
	var rejects []string
	for _, iv := range invs {
		items := names[iv.invoiceID]
		if len(items) == 0 {
			continue
		}
		hitName := ""
		for _, n := range items {
			if strings.Contains(n, "广告") || strings.Contains(n, "推广") {
				hitName = n
				break
			}
		}
		if hitName == "" {
			continue
		}
		show := hitName
		if r := []rune(show); len(r) > 40 {
			show = string(r[:40]) + "…"
		}
		tail := iv.number
		if len(tail) > 8 {
			tail = tail[len(tail)-8:]
		}
		rejects = append(rejects, fmt.Sprintf("明细#%d 非广告费发票(尾号%s)项目名称含'广告/推广'(「%s」), 费用类型应为广告费 (规则 18 反向)", iv.no, tail, show))
	}
	return rejects
}

// ruleAdInvoiceItemName 规则 18: 广告费 ⟺ 发票项目名含"广告/推广" 双向校验 (财务 2026-06-12; 樊雪娇 2026-06-17 加反向)
// 正向: 广告费明细配"*印刷品*KT板立牌"发票 → 项目名与费用类型不符, 建议驳回。
// 反向(跑哥严格口径): 非广告费明细的发票项目名含"广告/推广" → 应报广告费, 建议驳回(业务宣传费也驳)。
// 发票货物明细行名称本地没存, 现场调合思接口 (5min 缓存); 拉取失败时正向转人工核, 反向不冤枉(跳过)
func (h *DashboardHandler) ruleAdInvoiceItemName(raw map[string]interface{}, flowID string) ([]string, []string) {
	details, _ := raw["details"].([]interface{})
	// detailId → (明细号, 是否广告费); 广告费走正向, 其余走反向
	type adMeta struct {
		no   int
		isAd bool
	}
	detailMap := map[string]adMeta{}
	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		ft, _ := dm["feeTypeId"].(string)
		isAd := adFeeTypes[ft]
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		did, _ := form["detailId"].(string)
		no := 0
		if n, ok := form["detailNo"].(float64); ok {
			no = int(n)
		}
		if did != "" {
			detailMap[did] = adMeta{no, isAd}
		}
	}
	if len(detailMap) == 0 || flowID == "" {
		return nil, nil
	}

	rows, err := h.DB.Query(`SELECT IFNULL(detail_id,''), IFNULL(invoice_id,''), IFNULL(invoice_number,'')
		FROM hesi_flow_invoice WHERE flow_id=?`, flowID)
	if err != nil {
		log.Printf("[hesi-audit] 规则18 查发票失败 flow=%s: %v", flowID, err)
		return nil, nil
	}
	defer rows.Close()
	var adInvs, nonAdInvs []adInvRef // 广告费明细发票(正向) / 非广告费明细发票(反向)
	for rows.Next() {
		var did, iid, num string
		if rows.Scan(&did, &iid, &num) != nil {
			continue
		}
		meta, ok := detailMap[did]
		if !ok || iid == "" {
			continue
		}
		if meta.isAd {
			adInvs = append(adInvs, adInvRef{meta.no, iid, num})
		} else {
			nonAdInvs = append(nonAdInvs, adInvRef{meta.no, iid, num})
		}
	}
	if len(adInvs) == 0 && len(nonAdInvs) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(adInvs)+len(nonAdInvs))
	for _, iv := range adInvs {
		ids = append(ids, iv.invoiceID)
	}
	for _, iv := range nonAdInvs {
		ids = append(ids, iv.invoiceID)
	}
	names, err := h.fetchInvoiceItemNames(ids)
	if err != nil {
		log.Printf("[hesi-audit] 规则18 拉发票明细失败 flow=%s: %v", flowID, err)
		// 拉不到: 有广告费发票才提示正向人工核; 反向(非广告费)拉不到不冤枉(跳过)
		if len(adInvs) > 0 {
			return nil, []string{"广告费发票项目名称暂时拉不到, 请人工核是否含'广告/推广' (规则 18)"}
		}
		return nil, nil
	}
	rejects := checkAdInvoiceItems(adInvs, names)                          // 正向: 广告费发票须含广告/推广
	rejects = append(rejects, checkNonAdInvoiceItems(nonAdInvs, names)...) // 反向: 非广告费发票不得含广告/推广
	return rejects, nil
}
