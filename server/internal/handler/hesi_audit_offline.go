package handler

// hesi_audit_offline.go - 线下(世创/世用)专属规则15 及其日期区间辅助
// (2026-06-25 从 hesi_audit_rules.go 拆出, 纯挪位置不改逻辑)

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"time"
)

// 过路费 fee_type (规则 15-3 补贴扣减: 消费日按发票通行日期算)
const tollFeeTypeID = "ID01KhLSijR8FV"

// 专票类型集合 (规则 15-1.2: 非专票才要求付款截图; 樊雪娇口径"非专用发票=专票不要")
var specialInvoiceTypes = map[string]bool{
	"FULL_DIGITAl_SPECIAL": true, "DIGITAL_SPECIAL": true, "PAPER_SPECIAL": true, "SPECIAL": true,
}

// 规则 15-2 豁免类型: 补贴/样品/业务宣传费/私车公用 不要求 付款金额=发票金额
var offlineAmountExempt = map[string]bool{
	"ID01Fk0MQBAAQ7": true, "ID01Fk0MQBAB6D": true, "ID01FkA9pR8zGT": true, // 补贴类
	"ID01Fk0FsIqhDV": true, "ID01KhLrRhAWp9": true, "ID01Fk0B0hMfF5": true, // 样品类
	"ID01MBFnQppBWn": true, // 业务宣传费
	"ID01Fr2mX8KP2T": true, // 私车公用
}

// ruleOfflineExtras 规则 15: 线下(世创/世用)专属 (樊雪娇 2026-06-11 口径)
// 15-1.2 非专票(含电子铁路票/行程单)的明细必须传"付款截图"附件
// 15-2   除豁免类型外, 明细金额必须 = 该明细发票合计 (一分不差)
// 15-3   出差补贴: 集团经理及以上(含集团总监) 120/天(50餐+70交通), 其他 100/天(50餐+50交通);
//
//	当天有私车公用的, 该天只算 50 餐补 (花名册无"大区经理"档, 大区经理归集团经理档)
//
// 15-4   私车公用(整单聚合): 全部私车明细的油费发票合计 ≥ 私车报销总额, 否则驳回
//
//	(樊雪娇口径"超过总私车公用报销金额的油费发票即可"; 6/12 从按明细判改成整单判, 防误驳)
func (h *DashboardHandler) ruleOfflineExtras(raw map[string]interface{}, flowID, submitterID string) ([]string, []string) {
	var rejects, warnings []string
	details, _ := raw["details"].([]interface{})

	type det struct {
		no         int
		feeTypeID  string
		amount     float64
		hasPayShot bool
		dates      []string // feeDate / feeDatePeriod 覆盖的日期 (yyyy-mm-dd)
		days       float64  // u_天数 (补贴明细)
		subsidyAmt float64  // u_出差补贴金额
		driveRecID string   // 私车公用: 行车记录实例ID (消费日按行车记录时间算, 条2)
	}
	byID := map[string]det{}
	tollDetailIDs := map[string]bool{} // 过路费明细 detail_id → 后面查发票通行日期 (条3)
	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		var dt det
		dt.feeTypeID, _ = dm["feeTypeId"].(string)
		if n, ok := form["detailNo"].(float64); ok {
			dt.no = int(n)
		}
		if a, ok := form["amount"].(map[string]interface{}); ok {
			if s, ok := a["standard"].(string); ok {
				dt.amount, _ = strconv.ParseFloat(s, 64)
			}
		}
		if v, ok := form["u_付款截图"].([]interface{}); ok && len(v) > 0 {
			dt.hasPayShot = true
		} else if s, ok := form["u_付款截图"].(string); ok && s != "" {
			dt.hasPayShot = true
		}
		dt.dates = extractFeeDates(form)
		if v, ok := form["u_天数"].(string); ok {
			dt.days, _ = strconv.ParseFloat(v, 64)
		}
		if a, ok := form["u_出差补贴金额"].(map[string]interface{}); ok {
			if s, ok := a["standard"].(string); ok {
				dt.subsidyAmt, _ = strconv.ParseFloat(s, 64)
			}
		}
		// 私车公用: 消费日按行车记录时间算 (条2), 记行车记录实例ID
		if dt.feeTypeID == driveFeeTypeID {
			dt.driveRecID, _ = form["u_行车记录"].(string)
		}
		if did, ok := form["detailId"].(string); ok && did != "" {
			byID[did] = dt
			if dt.feeTypeID == tollFeeTypeID {
				tollDetailIDs[did] = true // 过路费明细, 查发票通行日期 (条3)
			}
		}
	}
	if len(byID) == 0 {
		return nil, nil
	}

	// 拉发票: detail_id → (合计金额, 是否全专票, 非专票存在); 顺带取过路费发票通行日期 (条3)
	invSum := map[string]float64{}
	hasNonSpecial := map[string]bool{}
	tollDayByDetail := map[string][]string{} // 过路费明细 → 发票通行日期 (yyyy-mm-dd)
	if flowID != "" {
		rows, err := h.DB.Query(`SELECT IFNULL(detail_id,''), IFNULL(invoice_type,''),
			IFNULL(total_amount,0), IFNULL(approve_amount,0), IFNULL(toll_pass_start,0) FROM hesi_flow_invoice WHERE flow_id=?`, flowID)
		if err == nil {
			defer rows.Close()
			invoiceReadBroken := false
			for rows.Next() {
				var did, itype string
				var total, approve float64
				var tollStart int64
				if err := rows.Scan(&did, &itype, &total, &approve, &tollStart); err != nil {
					if !invoiceReadBroken { // 只记首条, 防 DB 故障时每行刷一条日志
						log.Printf("[hesi-audit] 规则15 发票行解析失败 flow=%s: %v", flowID, err)
					}
					invoiceReadBroken = true
					continue
				}
				amt := total
				if amt == 0 {
					amt = approve
				}
				invSum[did] += amt
				if itype != "" && !specialInvoiceTypes[itype] {
					hasNonSpecial[did] = true
				}
				if tollStart > 1e12 && tollDetailIDs[did] {
					tollDayByDetail[did] = append(tollDayByDetail[did], time.UnixMilli(tollStart).Format("2006-01-02"))
				}
			}
			if err := rows.Err(); err != nil {
				invoiceReadBroken = true
				log.Printf("[hesi-audit] 规则15 发票读取中断 flow=%s: %v", flowID, err)
			}
			// invSum 不完整时 15-2/15-4 金额对碰必判错 (合计偏低 → 误驳合法单), 整体降级转人工
			if invoiceReadBroken {
				return nil, []string{"发票数据读取不完整, 规则 15 未自动判定, 需人工核"}
			}
		} else {
			log.Printf("[hesi-audit] 规则15 查发票失败 flow=%s: %v", flowID, err)
			return nil, []string{"发票数据读取失败, 规则 15 未自动判定, 需人工核"}
		}
	}

	// 扣交通补的天 = (私车日 ∪ 过路日) ∩ 出差申请单范围:
	//   私车日按行车记录时间(条2), 过路日按发票通行日期(条3), 只扣落在出差申请单期间的(条4)。
	driveTollDaySet := map[string]bool{}
	for _, dt := range byID {
		if dt.feeTypeID != driveFeeTypeID {
			continue
		}
		if dt.driveRecID != "" {
			if rec := h.LookupDriveRecord(dt.driveRecID); rec != nil && rec.StartTime > 1e12 {
				driveTollDaySet[time.UnixMilli(rec.StartTime).Format("2006-01-02")] = true
				continue
			}
		}
		// 行车记录拉不到(缓存冷/无记录)→ 兜底用明细自填日期, 不漏扣 (安全偏严)
		for _, d := range dt.dates {
			driveTollDaySet[d] = true
		}
	}
	for _, days := range tollDayByDetail { // 过路费: 发票通行日期
		for _, d := range days {
			driveTollDaySet[d] = true
		}
	}
	tripSpans := h.tripDateSpans(raw)
	deductDaySet := map[string]bool{}
	for d := range driveTollDaySet {
		// 有出差申请单时只扣落在出差期间的; 没关联/拉不到申请单时保守沿用旧行为(都扣, 偏严不放过)
		if len(tripSpans) == 0 || inAnySpan(d, tripSpans) {
			deductDaySet[d] = true
		}
	}

	// 职级 → 线下补贴日标准
	position := ""
	if submitterID != "" {
		_ = h.DB.QueryRow(`SELECT IFNULL(position,'') FROM hesi_employee_contract_company WHERE hesi_staff_id = ? LIMIT 1`, submitterID).Scan(&position)
	}
	transport := 50.0 // 其他员工 50餐+50交通
	if position == "集团经理" || position == "集团总监" || position == "副总裁" || position == "总裁" {
		transport = 70.0 // 集团经理及以上(大区经理归此档) 50餐+70交通
	}
	posKnown := position != ""
	// 付款截图(15-1.2)在费用明细"付款截图"字段或单据附件二选一即可 (樊雪娇 2026-06-18)。
	// 懒查: 仅当某明细确实要因缺付款截图被驳时才查一次附件 (绝大多数单不触发, 省掉附件查询)。
	docPayShotChecked, docPayShot := false, false
	var docPayShotErr error

	for did, dt := range byID {
		// 15-1.2 非专票必须传付款截图 — 交通及差旅费类型豁免 (财务 6/12: 铁路票/行程单
		// 本来就是差旅票, 只有差旅之外的费用类型才要求付款截图)
		if hasNonSpecial[did] && !dt.hasPayShot && travelExpenseFeeTypes[dt.feeTypeID] == "" {
			if !docPayShotChecked {
				docPayShot, docPayShotErr = h.hasPaymentProofAttachment(flowID)
				docPayShotChecked = true
			}
			// 附件读取失败 → 不自动驳, 循环后统一转人工 (与本函数其它 read-broken 一致)
			if docPayShotErr == nil && !docPayShot {
				rejects = append(rejects, fmt.Sprintf("明细#%d 含非专用发票(普票等), 须在费用明细'付款截图'字段或单据附件上传付款凭证 (线下规则 15-1.2)", dt.no))
			}
		}
		// 15-2 付款金额 = 发票合计 (一分不差, 豁免: 补贴/样品/业务宣传/私车公用)
		if !offlineAmountExempt[dt.feeTypeID] && invSum[did] > 0 && math.Abs(invSum[did]-dt.amount) > 0.005 {
			rejects = append(rejects, fmt.Sprintf("明细#%d 金额 ¥%.2f ≠ 发票合计 ¥%.2f, 线下要求一分不差 (规则 15-2)", dt.no, dt.amount, invSum[did]))
		}
		// 15-3 出差补贴线下口径
		if dt.feeTypeID == "ID01Fk0MQBAAQ7" && dt.subsidyAmt > 0 && dt.days > 0 {
			days := math.Ceil(dt.days)
			// 私车公用日只剩餐补: 补贴明细自带日期时, 只数落在补贴期间内的私车日;
			// 没填日期时退回全单私车日数 (单行程单据的旧行为) — 规则15二审修
			deductDays := float64(len(deductDaySet))
			if len(dt.dates) > 0 {
				n := 0
				for _, d := range dt.dates {
					if deductDaySet[d] {
						n++
					}
				}
				deductDays = float64(n)
			}
			if deductDays > days {
				deductDays = days
			}
			cap := 50*days + transport*(days-deductDays) // 私车/过路且在出差申请单内的天只剩50餐补(扣交通补)
			msg := fmt.Sprintf("明细#%d 出差补贴 ¥%.2f > 线下标准 ¥%.2f (50餐补×%.0f天 + %.0f交通补×%.0f天, 私车/过路 %.0f 天无交通补, 规则 15-3)",
				dt.no, dt.subsidyAmt, cap, days, transport, days-deductDays, deductDays)
			if dt.subsidyAmt > cap+0.005 {
				if posKnown {
					rejects = append(rejects, msg)
				} else {
					warnings = append(warnings, msg+" [花名册未匹配职级, 按其他员工档算, 请人工核]")
				}
			}
		}
	}

	// 付款截图附件查询失败 → 不自动驳 15-1.2, 转人工 (一单一次, 与发票 read-broken 降级一致, 樊雪娇 2026-06-18)
	if docPayShotErr != nil {
		log.Printf("[hesi-audit] 规则15-1.2 查付款截图附件失败 flow=%s: %v", flowID, docPayShotErr)
		warnings = append(warnings, "付款截图附件读取失败, 规则 15-1.2 未自动判定, 需人工核")
	}

	// 15-4 私车公用(整单聚合, 6/12 修): 樊雪娇原话"提供超过**总**私车公用报销金额的油费
	// 发票即可" — 油费发票通常一张大票挂一条明细管整月, 按明细判会大面积误驳 (dry-run 实证)
	driveAmtSum, driveInvSum := 0.0, 0.0
	hasDrive := false
	for did, dt := range byID {
		if dt.feeTypeID == driveFeeTypeID {
			hasDrive = true
			driveAmtSum += dt.amount
			driveInvSum += invSum[did]
		}
	}
	if hasDrive && driveInvSum+0.005 < driveAmtSum {
		rejects = append(rejects, fmt.Sprintf("私车公用合计 ¥%.2f, 油费发票合计仅 ¥%.2f, 须 ≥ 总报销金额 (规则 15-4)", driveAmtSum, driveInvSum))
	}

	return rejects, warnings
}

// tripDateSpans 从 expenseLinks 关联的出差申请单取出差起止日期区间 (yyyy-mm-dd, 可能多张);
// 供规则 15-3 判断私车/过路消费日是否落在出差期间 (与规则 17 ruleSubsidyDateInTrip 同源逻辑)。
func (h *DashboardHandler) tripDateSpans(raw map[string]interface{}) [][2]string {
	links, _ := raw["expenseLinks"].([]interface{})
	var spans [][2]string
	for _, l := range links {
		lid, _ := l.(string)
		if lid == "" || h.DB == nil {
			continue
		}
		var lraw string
		if err := h.DB.QueryRow(`SELECT IFNULL(raw_json,'') FROM hesi_flow WHERE flow_id=? LIMIT 1`, lid).Scan(&lraw); err != nil || lraw == "" {
			continue
		}
		var rm map[string]interface{}
		if json.Unmarshal([]byte(lraw), &rm) != nil {
			continue
		}
		tp, _ := rm["u_出差起止日期"].(map[string]interface{})
		if tp == nil {
			continue
		}
		s, _ := tp["start"].(float64)
		e, _ := tp["end"].(float64)
		if s > 1e12 && e >= s {
			spans = append(spans, [2]string{
				time.UnixMilli(int64(s)).Format("2006-01-02"),
				time.UnixMilli(int64(e)).Format("2006-01-02"),
			})
		}
	}
	return spans
}

// inAnySpan 日期 d (yyyy-mm-dd 定长, 可直接字典序比较) 是否落在任一区间内。
func inAnySpan(d string, spans [][2]string) bool {
	for _, sp := range spans {
		if d >= sp[0] && d <= sp[1] {
			return true
		}
	}
	return false
}

// extractFeeDates 从明细 form 提取 feeDate / feeDatePeriod 覆盖的日期 (yyyy-mm-dd)
// 起止须是合法毫秒时间戳且区间 ≤370 天, 防脏数据把循环跑飞 (规则15二审守卫, 规则17复用)
func extractFeeDates(form map[string]interface{}) []string {
	var dates []string
	if fd, ok := form["feeDate"].(float64); ok && fd > 1e12 {
		dates = append(dates, time.UnixMilli(int64(fd)).Format("2006-01-02"))
	}
	if fp, ok := form["feeDatePeriod"].(map[string]interface{}); ok {
		s, _ := fp["start"].(float64)
		e, _ := fp["end"].(float64)
		if s > 1e12 && e >= s && e-s <= 370*86400000 {
			for t := int64(s); t <= int64(e); t += 86400000 {
				dates = append(dates, time.UnixMilli(t).Format("2006-01-02"))
			}
		}
	}
	return dates
}
