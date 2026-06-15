package handler

import (
	"strings"
)

// v0.52: 跑哥要求所有 YS 数据屏蔽"安徽香松自然调味品有限公司" 组织
// 改这一处即可同步影响所有 YS 表 (ys_stock / ys_material_out / ys_purchase_orders / ys_subcontract_orders) 查询
const excludeAnhuiOrgWHERE = " AND org_name != '安徽香松自然调味品有限公司'"
const excludeAnhuiOrgYsWHERE = " AND ys.org_name != '安徽香松自然调味品有限公司'"

// planWarehouses 计划/采购看板 + 库存预警共用的 8 仓白名单 (2026-06-03 加南京自营仓 7→8)
// 改这一处即可同步影响：计划看板、库存预警等所有"按仓库白名单"过滤的查询
// 注意: 快递仓储分析(warehouse_flow.go)+物化表(cmd/build-warehouse-flow-summary)也共用此名单; 加/减仓须同步改 CLI 并重建 warehouse_flow_summary
var planWarehouses = []string{
	"南京委外成品仓-公司仓-委外",
	"南京仓库成品-公司仓-自营", // 退役自营仓(2025-08 切到委外仓), 加回让南京销售趋势连续; 库存=0 不影响库存预警
	"天津委外仓-公司仓-外仓",
	"西安仓库成品-公司仓-外仓",
	"松鲜鲜&大地密码云仓",
	"长沙委外成品仓-公司仓-外仓",
	"安徽郎溪成品-公司仓-自营",
	"南京分销虚拟仓-公司仓-外仓",
}

// planExcludeGoods 计划/采购看板排除 SKU 名单 (虚拟商品/邮费/差价补拍 等)
// 通用 goods_no 黑名单, 影响 prodSQL + matSQL + otherSQL
// v0.64: 删除 05010493 (广宣品已分流到"其他"Tab, 不再需要全黑名单)
var planExcludeGoods = []string{
	"yflj", // 运费说明及差价补拍链接 邮费
}

// planCategories 计划看板品类白名单 (10 个核心调味品类)
// v1.02: 跟"品类库存健康度"表口径统一, 6 KPI 也按这 10 品类过滤
// 排除: 包装材料/广宣礼盒/广宣品/清心湖自营/组套产品/成品礼盒/快递包材/私域代发/半保产品 等
var planCategories = []string{"调味料", "酱油", "调味汁", "干制面", "素蚝油", "酱类", "醋", "汤底", "番茄沙司", "糖"}

// planStockoutExcludeFlags 缺货统计排除的产品标签 (goods.flag_data 字段)
// v1.02: 跑哥指示, 缺货产品明细 + 缺货占比 KPI 不计入这 5 类标签的 SKU
var planStockoutExcludeFlags = []string{"非卖品", "已下架", "下架中", "接单产", "新品-接单产"}

// planStockoutExcludeFlagsCond 返回 "AND <alias>.goods_no NOT IN (排除标签 SKU 子查询)" 子句和参数
func planStockoutExcludeFlagsCond(alias string) (string, []interface{}) {
	args := make([]interface{}, len(planStockoutExcludeFlags))
	placeholders := make([]string, len(planStockoutExcludeFlags))
	for i, f := range planStockoutExcludeFlags {
		args[i] = f
		placeholders[i] = "?"
	}
	col := "goods_no"
	if alias != "" {
		col = alias + ".goods_no"
	}
	cond := " AND " + col + ` NOT IN (
		SELECT goods_no FROM goods WHERE is_delete=0 AND flag_data IN (` + strings.Join(placeholders, ",") + `))`
	return cond, args
}

// planCategoryGoodsCond 返回 "AND <alias>.goods_no IN (品类白名单对应 goods_no 子查询)" 子句和参数
// alias 不带点, "" 表示直接用字段名 (无表别名场景)
// 三个表共用: stock_quantity_daily / stock_batch_daily / sales_goods_summary 都有 goods_no 字段
func planCategoryGoodsCond(alias string) (string, []interface{}) {
	args := make([]interface{}, len(planCategories))
	placeholders := make([]string, len(planCategories))
	for i, c := range planCategories {
		args[i] = c
		placeholders[i] = "?"
	}
	col := "goods_no"
	if alias != "" {
		col = alias + ".goods_no"
	}
	cond := " AND " + col + ` IN (
		SELECT goods_no FROM goods WHERE is_delete=0 AND (
			CASE
				WHEN cate_full_name LIKE '成品/%' THEN SUBSTRING_INDEX(SUBSTRING_INDEX(cate_full_name,'/',2),'/',-1)
				WHEN cate_full_name IS NOT NULL AND cate_full_name != '' THEN cate_full_name
				ELSE '未分类'
			END
		) IN (` + strings.Join(placeholders, ",") + `))`
	return cond, args
}

// planSpecialAllotQtySubSQL: 近30天"特殊渠道(京东/猫超/朴朴)调拨当销售"按 goods_no 汇总数量的派生表。
// 背景: 吉客云现存量自带的 month_qty(月销量)只算销售出库, 不含调拨; 京东/猫超/朴朴这几个渠道靠调拨单
//   发货当销售(口径见 special_channel.go + allocate_orders/allocate_details 表)。不并进周转分母, 这些靠
//   调拨走量的畅销货会被"高库存周转>50天"误判成滞销(实测松茸鲜素蚝油360g周转被算成4624天, 并进后49天)。
// 用法: LEFT JOIN (`+planSpecialAllotQtySubSQL+`) sca ON sca.goods_no = <库存表>.goods_no
//   周转分母改用 SUM(month_qty)+IFNULL(MAX(sca.allot_qty),0); 两个 ? 都传库存快照日 stockSnapDate。
// 注意: 渠道清单(京东/猫超/朴朴)唯一真源在 special_channel.go, 新增特殊渠道时这里要同步。
//   allocate_details 不含仓库维度 → allot_qty 是全公司合计, 与库存侧按仓过滤(warehouseCond)口径不对称。
//   当前安全: 计划看板前端无仓库选择器(warehouse 恒空)且无任何 warehouse 数据范围角色, warehouseCond 恒空。
//   ⚠️ 若日后加"按仓筛选"或 warehouse 数据范围角色, 单仓视角下分母会被全公司调拨量灌水(周转低估、高库存漏列),
//   届时需把调拨并入限制为仅全仓口径(warehouseCond 为空时才 fold)。
//   stat_date 由 sync-allocate 仅对已审核单(status 2/3/20)写值, 草稿/待审为 NULL, 被 stat_date 范围比较自动排除;
//   但已审核后整单作废且从吉客云 API 消失的残留单无法自动清零(见 memory project_special_channel_allot 残留风险),
//   30 天窗口内会虚增一点分母, 影响极小, 上线知会即可。
const planSpecialAllotQtySubSQL = `(SELECT goods_no, SUM(sku_count) AS allot_qty
		FROM allocate_details
		WHERE channel_key IN ('京东','猫超','朴朴')
		  AND stat_date > DATE_SUB(?, INTERVAL 30 DAY) AND stat_date <= ?
		GROUP BY goods_no)`

// planSpecialAllotQtyLiveSubSQL: 同 planSpecialAllotQtySubSQL, 但日期锚用 CURDATE() (0 个 ?), 供查 LIVE 表
//   stock_quantity(无 snapshot_date 列)的采购计划页/库存监控页用。计划看板查 stock_quantity_daily 有快照日,
//   用上面那个绑 stockSnapDate 的版本; 这两页查实时表没有快照日变量, 故近30天锚到今天。
// 用法: LEFT JOIN (`+planSpecialAllotQtyLiveSubSQL+`) sca ON sca.goods_no = sq.goods_no
//   周转/日均分母 (SUM(month_qty)+IFNULL(MAX(sca.allot_qty),0)); 渠道/作废残留口径同 planSpecialAllotQtySubSQL 注释。
const planSpecialAllotQtyLiveSubSQL = `(SELECT goods_no, SUM(sku_count) AS allot_qty
		FROM allocate_details
		WHERE channel_key IN ('京东','猫超','朴朴')
		  AND stat_date > DATE_SUB(CURDATE(), INTERVAL 30 DAY) AND stat_date <= CURDATE()
		GROUP BY goods_no)`

// planExcludeAllotShopsCond: 计划看板销售查询排除"京东自营/猫超寄售"这 2 个特殊渠道店铺的销售单。
//   这俩渠道只按调拨当销售算(见 loadPlanAllot), 销售单一律不进计划看板销售, 防与调拨重复计算。
//   当前它们销售单挂自己平台仓(京东自营仓/天猫超市仓)本就不在计划 8 仓, 这道把规则写死: 哪天它们改从计划仓发货也不会双算。
//   朴朴(js-…朴朴)纯调拨、在 sales_goods_summary 无销售单, 无需排除。shop_id 真源同 loadEcommerceAllotAdjustment/special_channel.go。
//   无占位符, 直接拼到销售查询 WHERE 末尾(列名 shop_id 在 sales_goods_summary / _monthly 均无歧义)。
const planExcludeAllotShopsCond = ` AND shop_id NOT IN ('1819610592561398400','1819610591915475584')`

// planCategoryGoodsSubquery 返回品类白名单对应 goods_no 的"派生表子查询"(不含 AND goods_no IN) + 参数
// 用法: FROM 大表 JOIN (`+sub+`) gc ON gc.goods_no = 大表.goods_no
// 为什么不直接用 planCategoryGoodsCond: 月度物化表(sales_goods_summary_monthly)全历史趋势上,
// goods_no IN(子查询) 优化器选了坏计划要 9s, 改成 JOIN 派生表(品类 goods_no 只物化一次)只要 1.3s (口径完全一致, 实测同值)
func planCategoryGoodsSubquery() (string, []interface{}) {
	args := make([]interface{}, len(planCategories))
	placeholders := make([]string, len(planCategories))
	for i, c := range planCategories {
		args[i] = c
		placeholders[i] = "?"
	}
	sub := `SELECT goods_no FROM goods WHERE is_delete=0 AND (
		CASE
			WHEN cate_full_name LIKE '成品/%' THEN SUBSTRING_INDEX(SUBSTRING_INDEX(cate_full_name,'/',2),'/',-1)
			WHEN cate_full_name IS NOT NULL AND cate_full_name != '' THEN cate_full_name
			ELSE '未分类'
		END
	) IN (` + strings.Join(placeholders, ",") + `)`
	return sub, args
}

// buildExcludeGoodsFilter 返回 " AND <column> NOT IN (?,?,...)" 子句和参数
func buildExcludeGoodsFilter(column string) (string, []interface{}) {
	if len(planExcludeGoods) == 0 {
		return "", nil
	}
	args := make([]interface{}, len(planExcludeGoods))
	placeholders := ""
	for i, g := range planExcludeGoods {
		args[i] = g
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
	}
	return " AND " + column + " NOT IN (" + placeholders + ")", args
}

// buildPlanWarehouseFilter 返回 " AND <column> IN (?,?,...)" 子句和对应参数
func buildPlanWarehouseFilter(column string) (string, []interface{}) {
	args := make([]interface{}, len(planWarehouses))
	placeholders := ""
	for i, w := range planWarehouses {
		args[i] = w
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
	}
	return " AND " + column + " IN (" + placeholders + ")", args
}
