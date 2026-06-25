package handler

// hesi_audit_travel.go - 差旅域规则: 规则7-2座位/7-3住宿/11补贴/12私车公用/16企业支付防重/17补贴日期
// (2026-06-25 从 hesi_audit_rules.go 拆出, 纯挪位置不改逻辑)

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"
)

// 需人工核座位等级的 fee_type (规则 7-2)
// 跑哥规则: 汽车/火车/高铁/动车二等座及以下, 飞机经济舱
// 火车票: 合思发票主体已 OCR 出"座位类型", 改为自动判 (见 ruleTrainSeatClass), 不在此 manual 名单。
// 飞机/客车/汽车/其他交通: 合思未存结构化舱位, 仍给审批人 manual 提示。
var seatReviewFeeTypes = map[string]string{
	"ID01Fk0IZFCaJx": "飞机 (经济舱)",
	"ID01Fk0IZFCbgz": "客车",
	"ID01Fk0IZFCbx5": "其他交通",
	"ID01KhLSijR88T": "汽车 (二等座及以下)",
}

// 火车座位类型分类 (规则 7-2, 跑哥 2026-06-05 拍板: 二等座及以下合规)
// 数据驱动: 全库火车票实查 12 种座位类型, 按此分三档。其余(二等座/硬座/硬卧/软座/二等卧/
// 卧代二等座/硬卧代硬座/无座等)视为"二等座及以下"自动通过。
var trainSeatOverStandard = map[string]bool{ // 明显高于二等座 → 驳回
	"一等座": true, "商务座": true, "特等座": true, "优选一等座": true, "一等卧": true,
}
var trainSeatNeedReview = map[string]bool{ // 卧铺类 → 人工核(可能合理也可能超标)
	"软卧": true, "高级软卧": true, "动卧": true,
}

// 住宿费 fee_type (规则 7-3 触发 + 单晚价提取)
const hotelFeeTypeID = "ID01Fk0STRw3p5"

// ruleDriveRecordCheck 规则 12-1 自动化 (跑哥 2026-06-12: 行车记录已可拉取, 不再一律人工核)
// 私车公用明细对账: 报销金额 vs 行车记录"系统算出补助"(里程 × 标准, 油¥0.7/电¥0.6 标准在合思配置里):
//   - 金额 ≤ 系统算出 → 自动通过 (少报不拦)
//   - 金额 > 系统算出 → 建议驳回 (多报了)
//   - 行车记录缺失/拉取失败 → 保留原人工核提示 (降级兜底)
func (h *DashboardHandler) ruleDriveRecordCheck(raw map[string]interface{}) ([]string, []string) {
	details, _ := raw["details"].([]interface{})
	var rejects []string
	var manual, zeroAmt []int
	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		if ft, _ := dm["feeTypeId"].(string); ft != driveFeeTypeID {
			continue
		}
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
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
		// 金额 0/缺失/解析失败时不能走 "0 ≤ 补助 → 通过" 静默放行 (真实金额可能解析失败被置 0), 转人工
		if amt <= 0 {
			zeroAmt = append(zeroAmt, no)
			continue
		}
		recID, _ := form["u_行车记录"].(string)
		var rec *DriveRecord
		if recID != "" {
			rec = h.LookupDriveRecord(recID)
		}
		sub := 0.0
		if rec != nil && rec.Subsidy != "" {
			sub, _ = strconv.ParseFloat(rec.Subsidy, 64)
		}
		if sub <= 0 {
			manual = append(manual, no)
			continue
		}
		if amt > sub+0.005 {
			rejects = append(rejects, fmt.Sprintf("明细#%d 私车公用 ¥%.2f > 行车记录算出补助 ¥%.2f (%s km × ¥%s/km, 规则 12-1)",
				no, amt, sub, rec.Mileage, rec.Standard))
		}
	}
	var warnings []string
	if len(manual) > 0 {
		warnings = append(warnings, fmt.Sprintf("明细 %v 自驾, 行车记录缺失或拉取失败, 需人工核 KM × 车型单价 (油 ¥0.7/KM, 电 ¥0.6/KM, 规则 12-1)", uniqueInts(manual)))
	}
	if len(zeroAmt) > 0 {
		warnings = append(warnings, fmt.Sprintf("明细 %v 自驾, 报销金额为 0 或解析失败, 需人工核金额 (规则 12-1)", uniqueInts(zeroAmt)))
	}
	return rejects, warnings
}

// ruleDriveAndReasons 规则 12-2: 消费事由长度审核
// (12-1 自驾对账已升级为 ruleDriveRecordCheck 自动判, 2026-06-12 从本函数摘除)
// 12-2: consumptionReasons 长度 ≤50字 agree / >50字 reject (跑哥 2026-06-05 改: 50字以内都通过, 去掉10-50字人工核档)
//
//	字数按 rune 计 (中文 1 字)
func ruleDriveAndReasons(raw map[string]interface{}) (string, string) {
	details, _ := raw["details"].([]interface{})
	var driveDetails, longReasons []int
	_ = driveDetails

	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		no := 0
		if n, ok := form["detailNo"].(float64); ok {
			no = int(n)
		}

		// 12-2: 消费事由长度 (rune 计数, 中文 1 字) — 50 字以内通过, 超过 50 字驳回
		reason, _ := form["consumptionReasons"].(string)
		if len([]rune(reason)) > 50 {
			longReasons = append(longReasons, no)
		}
	}

	var rejects, warnings []string
	if len(longReasons) > 0 {
		rejects = append(rejects, fmt.Sprintf("明细 %v 消费事由 > 50 字 (规则 12-2)", uniqueInts(longReasons)))
	}
	return strings.Join(rejects, "; "), strings.Join(warnings, "; ")
}

// ruleSubsidyStandard 规则 11: 出差补贴 ≤ u_天数 × 职级标准
// 跑哥规则: 总裁 200 / 副总裁 150 / 集团总监 100 / 集团经理 80 / 主管和其他 60 (¥/天)
// 半天按 1 天计算 (math.Ceil)
// 触发: detail.feeTypeId = ID01Fk0MQBAAQ7 (补贴顶类), 取 feeTypeForm.u_出差补贴金额 + u_天数
// 注: 同明细的 u_市内补贴金额 / u_餐费补贴金额 不在本规则 (跑哥规则文写"出差补贴")
// 返回 (rejectMsg, warnMsg)
func (h *DashboardHandler) ruleSubsidyStandard(raw map[string]interface{}, submitterID string) (string, string) {
	details, _ := raw["details"].([]interface{})
	type hit struct {
		no     int
		amount float64
		days   float64
	}
	var hits []hit
	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		feeTypeID, _ := dm["feeTypeId"].(string)
		if feeTypeID != "ID01Fk0MQBAAQ7" {
			continue
		}
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		no := 0
		if n, ok := form["detailNo"].(float64); ok {
			no = int(n)
		}
		var amt, days float64
		if a, ok := form["u_出差补贴金额"].(map[string]interface{}); ok {
			if s, ok := a["standard"].(string); ok {
				amt, _ = strconv.ParseFloat(s, 64)
			}
		}
		if v, ok := form["u_天数"].(string); ok {
			days, _ = strconv.ParseFloat(v, 64)
		}
		if amt > 0 && days > 0 {
			hits = append(hits, hit{no, amt, days})
		}
	}
	if len(hits) == 0 {
		return "", ""
	}

	// 查提交人岗位职级 (复用规则 7-3 的字段)
	// fallback: SSC 表只 241 人 (审批职级), 普通员工 ~640 人不在.
	// codex 二审 v1.75.17 修: 未匹配时按"主管和其他" cap 算, 但超额转 manual (不冤枉潜在未维护的高管)
	position := ""
	if submitterID != "" {
		_ = h.DB.QueryRow(`SELECT IFNULL(position,'') FROM hesi_employee_contract_company WHERE hesi_staff_id = ? LIMIT 1`, submitterID).Scan(&position)
	}
	positionForDisplay := position
	isFallback := false
	if position == "" {
		position = "主管和其他"
		positionForDisplay = "主管和其他 (花名册未匹配)"
		isFallback = true
	}
	_, subsidyStd := h.loadHesiAuditParams() // DB 配置优先, 财务改口径 5 分钟生效
	std, ok := subsidyStd[position]
	if !ok {
		return "", "岗位职级「" + position + "」非标准 5 档, 出差补贴标准未配置 (规则 11)"
	}

	var rejects, warnings []string
	for _, h := range hits {
		// 半天按 1 天 → ceil
		ceilDays := math.Ceil(h.days)
		cap := ceilDays * std
		if h.amount > cap+0.01 {
			msg := fmt.Sprintf("明细#%d 出差补贴 ¥%.2f > 标准 ¥%.0f×%.0f天=¥%.2f (%s, 规则 11)",
				h.no, h.amount, std, ceilDays, cap, positionForDisplay)
			if isFallback {
				warnings = append(warnings, msg+" [SSC 未匹配, 请人工核职级]")
			} else {
				rejects = append(rejects, msg)
			}
		}
	}
	return strings.Join(rejects, "; "), strings.Join(warnings, "; ")
}

// ruleAccommodationStandard 规则 7-3: 住宿费明细单晚价 ≤ 城市×职级标准
// 跑哥规则: 一线/新一线/二线/其他城市 × 总裁/副总裁/集团总监/集团经理/主管和其他 5 档 (PDF V7.0)
// 线下(世创/世用 isOffline=true): 改用线下住宿表(3档城市×3档职级), 新一线→二线 (樊雪娇 2026-06-17)
// 同住上浮: u_是否两人同住 非空 → 上浮 20% (按职位高者标准, 简化按当前人)
// 单晚价算法: amount / 出差天数 (feeDatePeriod.end - start)/86400000 + 1
// 返回: (reject 原因, warn 原因), 任一非空即触发
func (h *DashboardHandler) ruleAccommodationStandard(raw map[string]interface{}, submitterID string, isOffline bool) (string, string) {
	details, _ := raw["details"].([]interface{})

	type hotelLine struct {
		idx       int
		amount    float64
		cityRaw   string
		days      int
		cohabit   bool
		badPeriod bool // 日期区间缺失/异常 (如 start=0), 间夜数对不准, 转人工
	}
	var lines []hotelLine
	for i, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		feeTypeID, _ := dm["feeTypeId"].(string)
		if feeTypeID != hotelFeeTypeID {
			continue
		}
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		var amt float64
		if a, ok := form["amount"].(map[string]interface{}); ok {
			if s, ok := a["standard"].(string); ok {
				amt, _ = strconv.ParseFloat(s, 64)
			}
		}
		cityRaw, _ := form["city"].(string)
		cohabit := false
		if v, ok := form["u_是否两人同住"].(string); ok && v != "" {
			cohabit = true
		}
		days := 1
		badPeriod := false
		if fp, ok := form["feeDatePeriod"].(map[string]interface{}); ok {
			start, _ := fp["start"].(float64)
			end, _ := fp["end"].(float64)
			// start 必须是合法毫秒时间戳(>1e12) + 区间封顶 370 天, 跟 extractFeeDates 同口径
			// 否则 (如缺起始日期 start=0, end 正常) 会按 end-0 算出 2 万+天, 标准上限被撑爆 → 超标住宿永不触发
			if start > 1e12 && end >= start && end-start <= 370*86400000 {
				days = int((end-start)/86400000) + 1
			} else {
				badPeriod = true
			}
		}
		lines = append(lines, hotelLine{i + 1, amt, cityRaw, days, cohabit, badPeriod})
	}
	if len(lines) == 0 {
		return "", ""
	}

	// 查提交人岗位职级 (花名册 SSC 表)
	// codex 二审 v1.75.17 修: SSC 未匹配时按"主管和其他" cap 算, 但超额转 manual
	position := ""
	if submitterID != "" {
		_ = h.DB.QueryRow(`SELECT IFNULL(position,'') FROM hesi_employee_contract_company WHERE hesi_staff_id = ? LIMIT 1`, submitterID).Scan(&position)
	}
	positionForDisplay := position
	isFallback := false
	if position == "" {
		position = "主管和其他"
		positionForDisplay = "主管和其他 (花名册未匹配)"
		isFallback = true
	}
	var standards map[string]float64
	if isOffline {
		// 线下(世创/世用)用线下住宿表: 职级先收口到 3 档 (其他员工/大区经理及以上/集团总监)
		olevel := offlineHotelLevel(position)
		s, ok := h.loadOfflineAccomStd()[olevel]
		if !ok {
			return "", "线下住宿职级「" + olevel + "」未配置 (规则 7-3 线下)"
		}
		standards = s
		if isFallback {
			positionForDisplay = olevel + " (线下, 花名册未匹配)"
		} else {
			positionForDisplay = olevel + " (线下)"
		}
	} else {
		accomStd, _ := h.loadHesiAuditParams() // DB 配置优先, 财务改口径 5 分钟生效
		s, ok := accomStd[position]
		if !ok {
			return "", "岗位职级「" + position + "」非标准 5 档, 住宿标准未配置 (规则 7-3)"
		}
		standards = s
	}

	tierMap := h.loadCityTierCache()
	var rejectMsgs []string
	var warnMsgs []string
	for _, line := range lines {
		if line.badPeriod {
			warnMsgs = append(warnMsgs, fmt.Sprintf("住宿明细#%d 日期区间缺失/异常, 间夜数无法核算, 需人工核 (规则 7-3)", line.idx))
			continue
		}
		tier := extractTier(line.cityRaw, tierMap)
		if isOffline {
			tier = offlineCityTier(tier) // 集团 4 档 → 线下 3 档 (新一线→二线)
		}
		std := standards[tier]
		if std == 0 {
			warnMsgs = append(warnMsgs, fmt.Sprintf("住宿明细#%d 城市分级未识别 (规则 7-3)", line.idx))
			continue
		}
		cap := std * float64(line.days)
		cohabitTag := ""
		if line.cohabit {
			cap *= 1.2
			cohabitTag = " ×1.2(同住)"
		}
		if line.amount > cap {
			msg := fmt.Sprintf(
				"住宿#%d ¥%.2f > 标准 ¥%.0f×%d晚%s=¥%.2f (%s/%s, 规则 7-3)",
				line.idx, line.amount, std, line.days, cohabitTag, cap, positionForDisplay, tier,
			)
			if isFallback {
				warnMsgs = append(warnMsgs, msg+" [SSC 未匹配, 请人工核职级]")
			} else {
				rejectMsgs = append(rejectMsgs, msg)
			}
		}
	}
	return strings.Join(rejectMsgs, "; "), strings.Join(warnMsgs, "; ")
}

// ruleSeatManualReview 规则 7-2: 飞机/火车/汽车/客车/其他交通明细 → 需人工核座位等级
// 跑哥规则: 汽车/火车/高铁/动车二等座及以下, 飞机经济舱
// 合思 raw_json 未存舱位/座位字段, 凭票报销时只能给 manual 提示
// 数据驱动: 实查 286 单日常报销单含飞机/火车明细, 283 单 (99%) 有 u_付款截图 = 凭票报销
func ruleSeatManualReview(raw map[string]interface{}) string {
	details, _ := raw["details"].([]interface{})
	var hits []string
	seen := map[string]bool{}
	for _, d := range details {
		dm, ok := d.(map[string]interface{})
		if !ok {
			continue
		}
		feeTypeID, _ := dm["feeTypeId"].(string)
		if label, ok := seatReviewFeeTypes[feeTypeID]; ok && !seen[label] {
			hits = append(hits, label)
			seen[label] = true
		}
	}
	if len(hits) == 0 {
		return ""
	}
	return "含" + strings.Join(hits, "/") + ", 需人工核座位等级 (规则 7-2)"
}

// classifyTrainSeat 火车座位等级分类: "over"超标驳回 / "review"人工核(卧铺或未识别) / "ok"通过(二等座及以下)
func classifyTrainSeat(seat string) string {
	switch {
	case seat == "":
		return "review"
	case trainSeatOverStandard[seat]:
		return "over"
	case trainSeatNeedReview[seat]:
		return "review"
	default:
		return "ok"
	}
}

// ruleTrainSeatClass 规则 7-2 火车票座位等级自动判 (跑哥 2026-06-05: 二等座及以下合规)
// 数据来源: 合思发票主体 OCR 出的"座位类型"(回填进 hesi_flow_invoice.seat_type)。
//   - 一等座/商务座/特等座/一等卧 → 超标驳回
//   - 软卧/动卧等卧铺 → 人工核
//   - 座位类型识别不到(空) → 人工核
//   - 其余(二等座/硬座/硬卧/软座/二等卧/卧代二等座…)= 二等座及以下 → 通过(不记)
//
// 返回 (rejectMsg, warnMsg)
func (h *DashboardHandler) ruleTrainSeatClass(flowID string) (string, string) {
	if flowID == "" {
		return "", ""
	}
	rows, err := h.DB.Query(`SELECT IFNULL(seat_type,''), IFNULL(train_no,''), IFNULL(from_station,''), IFNULL(to_station,'')
		FROM hesi_flow_invoice WHERE flow_id = ? AND invoice_type LIKE '%TRAIN%'`, flowID)
	if err != nil {
		return "", ""
	}
	defer rows.Close()

	var over, review []string
	for rows.Next() {
		var seat, train, from, to string
		if rows.Scan(&seat, &train, &from, &to) != nil {
			continue
		}
		label := train
		if from != "" || to != "" {
			label = strings.TrimSpace(train + " " + from + "→" + to)
		}
		if label == "" {
			label = "火车票"
		}
		switch classifyTrainSeat(seat) {
		case "over":
			over = append(over, label+" "+seat)
		case "review":
			if seat == "" {
				review = append(review, label+" 座位类型未识别")
			} else {
				review = append(review, label+" "+seat)
			}
		}
	}

	var rej, warn string
	if len(over) > 0 {
		rej = "火车票超「二等座及以下」标准: " + strings.Join(over, "; ") + " (规则 7-2)"
	}
	if len(review) > 0 {
		warn = "火车票卧铺/座位未识别需人工核: " + strings.Join(review, "; ") + " (规则 7-2)"
	}
	return rej, warn
}

// ruleSubsidyDateInTrip 规则 17: 出差补贴日期须落在关联出差申请单的起止日期内 (财务 2026-06-12)
// 补贴明细 (出差补贴 ID01Fk0MQBAAQ7) 的日期 vs 关联申请单 u_出差起止日期 [start,end]:
//   - feeDate 单日: 必须被任一申请单覆盖, 否则建议驳回 (出差那天没申请单)
//   - feeDatePeriod 区间: 业务员按月报销惯例填"整月"(dry-run 实查 5/1~5/31 大量存在),
//     逐天判会大面积误杀 → 只要求区间与任一申请单有交集, 完全不沾边才驳回
//
// 明细没填日期 / 关联申请单都没有起止日期 → 不判 (关联缺失本身由规则 7-1 管)
func (h *DashboardHandler) ruleSubsidyDateInTrip(raw map[string]interface{}) []string {
	details, _ := raw["details"].([]interface{})
	type sub struct {
		no           int
		singleDates  []string // feeDate 单日
		pStart, pEnd string   // feeDatePeriod 区间 (空=没填)
	}
	var subs []sub
	for _, d := range details {
		dm, _ := d.(map[string]interface{})
		if dm == nil {
			continue
		}
		if ft, _ := dm["feeTypeId"].(string); ft != "ID01Fk0MQBAAQ7" {
			continue
		}
		form, _ := dm["feeTypeForm"].(map[string]interface{})
		if form == nil {
			continue
		}
		var sb sub
		if n, ok := form["detailNo"].(float64); ok {
			sb.no = int(n)
		}
		if fd, ok := form["feeDate"].(float64); ok && fd > 1e12 {
			sb.singleDates = append(sb.singleDates, time.UnixMilli(int64(fd)).Format("2006-01-02"))
		}
		if fp, ok := form["feeDatePeriod"].(map[string]interface{}); ok {
			s, _ := fp["start"].(float64)
			e, _ := fp["end"].(float64)
			if s > 1e12 && e >= s {
				sb.pStart = time.UnixMilli(int64(s)).Format("2006-01-02")
				sb.pEnd = time.UnixMilli(int64(e)).Format("2006-01-02")
			}
		}
		if len(sb.singleDates) > 0 || sb.pStart != "" {
			subs = append(subs, sb)
		}
	}
	if len(subs) == 0 {
		return nil
	}

	// 关联出差申请单的起止区间 (天粒度字符串, 可能多张申请单取并集)
	links, _ := raw["expenseLinks"].([]interface{})
	type span struct{ code, start, end string }
	var spans []span
	for _, l := range links {
		lid, _ := l.(string)
		if lid == "" {
			continue
		}
		var code, lraw string
		if err := h.DB.QueryRow(`SELECT code, IFNULL(raw_json,'') FROM hesi_flow WHERE flow_id=? LIMIT 1`, lid).Scan(&code, &lraw); err != nil || lraw == "" {
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
			spans = append(spans, span{code,
				time.UnixMilli(int64(s)).Format("2006-01-02"),
				time.UnixMilli(int64(e)).Format("2006-01-02")})
		}
	}
	if len(spans) == 0 {
		return nil
	}

	spanDesc := func() string {
		var rngs []string
		for _, sp := range spans {
			rngs = append(rngs, fmt.Sprintf("%s(%s~%s)", sp.code, sp.start, sp.end))
		}
		return strings.Join(rngs, "、")
	}

	var rejects []string
	for _, sb := range subs {
		// 单日: 必须被某申请单覆盖
		var outside []string
		for _, d := range sb.singleDates {
			covered := false
			for _, sp := range spans {
				if d >= sp.start && d <= sp.end {
					covered = true
					break
				}
			}
			if !covered {
				outside = append(outside, d)
			}
		}
		if len(outside) > 0 {
			rejects = append(rejects, fmt.Sprintf("明细#%d 补贴日期 %s 不在出差申请单 %s 起止日期内 (规则 17)",
				sb.no, strings.Join(outside, "/"), spanDesc()))
		}
		// 区间: 与所有申请单都无交集才算超范围
		if sb.pStart != "" {
			overlap := false
			for _, sp := range spans {
				if sb.pStart <= sp.end && sb.pEnd >= sp.start {
					overlap = true
					break
				}
			}
			if !overlap {
				rejects = append(rejects, fmt.Sprintf("明细#%d 补贴期间 %s~%s 与出差申请单 %s 起止日期完全不重叠 (规则 17)",
					sb.no, sb.pStart, sb.pEnd, spanDesc()))
			}
		}
	}
	return rejects
}

// ruleCorpPaidDuplicate 规则 16: 企业支付行程防重复报销 (跑哥 2026-06-11; 樊雪娇 2026-06-17 收窄)
// 报销单关联的出差申请单下, 公司已"企业支付"买好的火车/飞机票 (合思商旅订单),
// 若同车次+同乘车人又出现在本单报销发票里:
//
//	票价也一致 (订单"票面价"=发票金额) → 公司已付的票疑似二次报销, 建议驳回
//	票价不一致 → 不是同一张票, 不算重复报销, 直接通过 (樊雪娇 2026-06-17: 这条只防重复, 不转人工)
//
// 数据校准 (2026-06-11 实查): 企业支付金额 = 票面价 + ~3元服务费, 对碰必须用票面价;
// 商旅订单没有可用出发日期列, 靠"只看本单关联申请单"收口, 不跨行程乱碰
func (h *DashboardHandler) ruleCorpPaidDuplicate(raw map[string]interface{}, flowID string) ([]string, []string) {
	links, _ := raw["expenseLinks"].([]interface{})
	if len(links) == 0 || flowID == "" {
		return nil, nil
	}
	// 关联申请单号 (expenseLinks 存的是申请单 flow_id, 换 code)
	var codes []interface{}
	for _, l := range links {
		lid, _ := l.(string)
		if lid == "" {
			continue
		}
		var code string
		if err := h.DB.QueryRow(`SELECT code FROM hesi_flow WHERE flow_id=? LIMIT 1`, lid).Scan(&code); err == nil && code != "" {
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		return nil, nil
	}

	// 申请单下的企业支付车票订单 (火车/飞机才有车次; 退票/退订=公司没掏成钱, 不算)
	// 状态实查 (2026-06-11): 确认订单/出票/已完成/改签/退订/退票 六种
	type corpOrder struct {
		tripNo, traveler, reqCode string
		fare                      float64 // 票面价 (= 发票金额口径); 拿不到时退回企业支付金额
		used                      bool
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(codes)), ",")
	rows, err := h.DB.Query(`SELECT IFNULL(trip_no,''), IFNULL(traveler,''), IFNULL(req_code,''),
		IFNULL(corp_pay,0), IFNULL(raw_json,'') FROM hesi_travel_order
		WHERE req_code IN (`+ph+`) AND pay_method='企业支付' AND trip_no<>'' AND order_state NOT IN ('退票','退订')`, codes...)
	if err != nil {
		log.Printf("[hesi-audit] 规则16 查商旅订单失败 flow=%s: %v", flowID, err)
		return nil, nil
	}
	var orders []*corpOrder
	for rows.Next() {
		var o corpOrder
		var corpPay float64
		var rawJSON string
		if rows.Scan(&o.tripNo, &o.traveler, &o.reqCode, &corpPay, &rawJSON) != nil {
			continue
		}
		o.fare = corpPay
		if rawJSON != "" {
			var om map[string]interface{}
			if json.Unmarshal([]byte(rawJSON), &om) == nil {
				if v, ok := getStandardAmount(om["票面价"]); ok && v > 0 {
					o.fare = v
				}
			}
		}
		if o.tripNo != "" {
			orders = append(orders, &o)
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("[hesi-audit] 规则16 读商旅订单中断 flow=%s: %v", flowID, err)
	}
	rows.Close()
	if len(orders) == 0 {
		return nil, nil
	}

	// detailId → detailNo (提示里标明细号)
	detailNo := map[string]int{}
	if details, ok := raw["details"].([]interface{}); ok {
		for _, d := range details {
			dm, _ := d.(map[string]interface{})
			if dm == nil {
				continue
			}
			form, _ := dm["feeTypeForm"].(map[string]interface{})
			if form == nil {
				continue
			}
			did, _ := form["detailId"].(string)
			if n, ok := form["detailNo"].(float64); ok && did != "" {
				detailNo[did] = int(n)
			}
		}
	}

	// 本单带车次的发票 (火车票 OCR 有车次/乘车人)
	ivRows, err := h.DB.Query(`SELECT IFNULL(detail_id,''), IFNULL(train_no,''), IFNULL(passenger,''),
		IFNULL(total_amount,0) FROM hesi_flow_invoice WHERE flow_id=? AND IFNULL(train_no,'')<>''`, flowID)
	if err != nil {
		log.Printf("[hesi-audit] 规则16 查发票失败 flow=%s: %v", flowID, err)
		return nil, nil
	}
	defer ivRows.Close()

	var rejects, warnings []string
	ticketReadBroken := false
	for ivRows.Next() {
		var did, trainNo, passenger string
		var amt float64
		if err := ivRows.Scan(&did, &trainNo, &passenger, &amt); err != nil {
			if !ticketReadBroken { // 只记首条, 防 DB 故障时每行刷一条日志
				log.Printf("[hesi-audit] 规则16 发票行解析失败 flow=%s: %v", flowID, err)
			}
			ticketReadBroken = true
			continue
		}
		if trainNo == "" || passenger == "" {
			continue
		}
		// OCR 的乘车人可能带空格 ("郑 华坤"), 去空格再比
		passenger = strings.ReplaceAll(passenger, " ", "")
		// 樊雪娇 2026-06-17: 这条只防重复报销, 只认 同车次+同乘车人+"同票价"(票面价=发票金额);
		// 票价不同 = 不是同一张票 = 不算重复, 直接通过 (不再转人工核)。
		// 关键: 只在"同票价"命中时才标记订单 used —— 票价不同的不消耗订单, 否则会"用掉"订单,
		// 害得后面真正同票价的重复发票匹配不到而漏判 (削弱防重复本身)。
		var hit *corpOrder
		for _, o := range orders {
			if o.used || o.tripNo != trainNo || strings.ReplaceAll(o.traveler, " ", "") != passenger {
				continue
			}
			if math.Abs(o.fare-amt) <= 0.005 { // 只认同票价
				hit = o
				break
			}
		}
		if hit == nil {
			continue // 没有同车次+同人+同票价的企业支付票 = 不是重复报销, 通过 (票价不同走这里)
		}
		hit.used = true
		no := detailNo[did]
		rejects = append(rejects, fmt.Sprintf("明细#%d 车次 %s (乘车人%s ¥%.2f) 在出差申请单 %s 里公司已企业支付同车次同票价的票, 疑似已付票二次报销 (规则 16)",
			no, trainNo, passenger, amt, hit.reqCode))
	}
	if err := ivRows.Err(); err != nil {
		ticketReadBroken = true
		log.Printf("[hesi-audit] 规则16 发票读取中断 flow=%s: %v", flowID, err)
	}
	// 车票数据不完整只会漏判 (不会误驳), 保留已出结果, 补一条人工核提示
	if ticketReadBroken {
		warnings = append(warnings, "车票发票数据读取不完整, 规则 16 防重复报销可能漏判, 需人工核 (规则 16)")
	}
	return rejects, warnings
}
