package handler

import (
	"strings"
)

// excludeAnhuiOrg v0.52: 跑哥要求所有 YS 数据屏蔽"安徽香松自然调味品有限公司" 组织
// 改这一处即可同步影响所有 YS 表 (ys_stock / ys_material_out / ys_purchase_orders / ys_subcontract_orders) 查询
const excludeAnhuiOrg = "安徽香松自然调味品有限公司"
const excludeAnhuiOrgWHERE = " AND org_name != '安徽香松自然调味品有限公司'"
const excludeAnhuiOrgYsWHERE = " AND ys.org_name != '安徽香松自然调味品有限公司'"

// planWarehouses 计划/采购看板 + 库存预警共用的 7 仓白名单
// 改这一处即可同步影响：计划看板、库存预警等所有"按仓库白名单"过滤的查询
var planWarehouses = []string{
	"南京委外成品仓-公司仓-委外",
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
