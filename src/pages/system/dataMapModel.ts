// 数据关联三维图 — 架构模型 (人工梳理, 高阶三层总览)
// 库里表之间没有外键、架构不在数据库里, 这份是按真实数据流向梳理的节点+连线。
// 加减数据源/看板时改这里即可。三层: source(外部数据源) / domain(数据域) / board(看板模块)

export type DataMapLayer = 'source' | 'domain' | 'board';

export interface DataMapNode {
  id: string;
  name: string;
  layer: DataMapLayer;
  desc: string; // 悬停说明
}

export interface DataMapLink {
  source: string;
  target: string;
}

// 三层配色 (BI 经典青蓝调色盘: 源=青蓝 / 数据域=金黄 / 看板=翡翠)
export const LAYER_META: Record<DataMapLayer, { label: string; color: string }> = {
  source: { label: '外部数据源', color: '#1e40af' },
  domain: { label: '数据域', color: '#f59e0b' },
  board: { label: '看板模块', color: '#10b981' },
};

export const DATA_MAP_NODES: DataMapNode[] = [
  // ① 外部数据源
  { id: 'src_jackyun', name: '吉客云 ERP', layer: 'source', desc: '订单/库存/调拨/商品/汇总账主数据源' },
  { id: 'src_ys', name: '用友 YonBIP', layer: 'source', desc: '采购/委外/材料出库/现存量/来料检验' },
  { id: 'src_hesi', name: '合思费控', layer: 'source', desc: '报销/付款单据、审批流、商旅订单' },
  { id: 'src_dingtalk', name: '钉钉', layer: 'source', desc: '扫码登录、花名册(部门/合同主体)' },
  { id: 'src_rpa', name: 'RPA (Z盘 11平台)', layer: 'source', desc: '天猫/京东/拼多多/抖音等平台 + 客服服务分/评论 Excel' },

  // ② 数据域
  { id: 'dom_trade', name: '销售单/明细/包裹', layer: 'domain', desc: 'trade 按月分表, 已发货订单+商品行+物流包裹' },
  { id: 'dom_summary', name: '销售汇总账', layer: 'domain', desc: '货品销售汇总(日/月聚合), 多看板核心口径' },
  { id: 'dom_stock', name: '库存/快照/批次', layer: 'domain', desc: '吉客云库存 + 每日快照 + 批次库存' },
  { id: 'dom_allocate', name: '调拨单', layer: 'domain', desc: '吉客云调拨(含特殊渠道当销售口径)' },
  { id: 'dom_ops', name: '运营数据(11平台)', layer: 'domain', desc: 'op_*_daily 各平台流量/转化/推广宽表' },
  { id: 'dom_ysdata', name: '采购/委外/现存量', layer: 'domain', desc: 'ys_* 用友采购订单/委外/材料出库/现存量' },
  { id: 'dom_hesi', name: '费控/审批单据', layer: 'domain', desc: 'hesi_* 单据/明细/发票/审批流' },
  { id: 'dom_travel', name: '商旅订单', layer: 'domain', desc: 'hesi_travel_order 机票/火车/酒店/用车' },
  { id: 'dom_customer', name: '客服(服务分/评论)', layer: 'domain', desc: '店铺服务分 + 评论数据 + 客服总览指标' },
  { id: 'dom_finance', name: '财务报表', layer: 'domain', desc: '财报/业务预决算(Excel 解析入库)' },
  { id: 'dom_futures', name: '原料行情', layer: 'domain', desc: '新浪期货日线 + 盘中准实时快照' },

  // ③ 看板模块
  { id: 'brd_overview', name: '综合看板', layer: 'board', desc: '全公司销售/部门/趋势/排行总览' },
  { id: 'brd_ecommerce', name: '电商/店铺/货品', layer: 'board', desc: '电商部主页 + 店铺看板 + 货品看板' },
  { id: 'brd_supply', name: '供应链', layer: 'board', desc: '采购计划/库存预警/快递仓储/月度账单/质检' },
  { id: 'brd_finance', name: '财务看板', layer: 'board', desc: '利润总览/部门·产品利润/财报/SKU动销' },
  { id: 'brd_customer', name: '客服看板', layer: 'board', desc: '客服总览/趋势/平台/店铺/服务分/评论' },
  { id: 'brd_expense', name: '费控管理', layer: 'board', desc: '费控单据列表/详情/AI 审批建议' },
  { id: 'brd_hesibot', name: '合思审批机器人', layer: 'board', desc: '待审批列表/规则判定/批量审批' },
  { id: 'brd_marketing', name: '营销看板', layer: 'board', desc: '各平台推广/SKU/转化分析' },
  { id: 'brd_futures', name: '原料行情总览', layer: 'board', desc: '16 品种期货行情(日线+准实时)' },
  { id: 'brd_forecast', name: '销量预测', layer: 'board', desc: '线下大区销量预测+回测' },
  { id: 'brd_ai', name: 'AI 智能助手', layer: 'board', desc: '跨域自然语言问数(8 模块路由)' },
];

export const DATA_MAP_LINKS: DataMapLink[] = [
  // 源 → 数据域
  { source: 'src_jackyun', target: 'dom_trade' },
  { source: 'src_jackyun', target: 'dom_summary' },
  { source: 'src_jackyun', target: 'dom_stock' },
  { source: 'src_jackyun', target: 'dom_allocate' },
  { source: 'src_ys', target: 'dom_ysdata' },
  { source: 'src_hesi', target: 'dom_hesi' },
  { source: 'src_hesi', target: 'dom_travel' },
  { source: 'src_dingtalk', target: 'dom_hesi' }, // 花名册部门/合同主体喂费控校验
  { source: 'src_rpa', target: 'dom_ops' },
  { source: 'src_rpa', target: 'dom_customer' },

  // 数据域 → 看板
  { source: 'dom_summary', target: 'brd_overview' },
  { source: 'dom_summary', target: 'brd_ecommerce' },
  { source: 'dom_summary', target: 'brd_marketing' },
  { source: 'dom_trade', target: 'brd_supply' }, // 快递仓储用包裹
  { source: 'dom_trade', target: 'brd_forecast' },
  { source: 'dom_stock', target: 'brd_overview' },
  { source: 'dom_stock', target: 'brd_supply' },
  { source: 'dom_allocate', target: 'brd_ecommerce' },
  { source: 'dom_allocate', target: 'brd_overview' },
  { source: 'dom_ops', target: 'brd_marketing' },
  { source: 'dom_ops', target: 'brd_ecommerce' },
  { source: 'dom_ysdata', target: 'brd_supply' },
  { source: 'dom_hesi', target: 'brd_expense' },
  { source: 'dom_hesi', target: 'brd_hesibot' },
  { source: 'dom_travel', target: 'brd_hesibot' }, // 行程一致性校验
  { source: 'dom_customer', target: 'brd_customer' },
  { source: 'dom_finance', target: 'brd_finance' },
  { source: 'dom_summary', target: 'brd_finance' }, // 利润看板也吃汇总
  { source: 'dom_futures', target: 'brd_futures' },

  // AI 助手跨域问数
  { source: 'dom_summary', target: 'brd_ai' },
  { source: 'dom_stock', target: 'brd_ai' },
  { source: 'dom_ops', target: 'brd_ai' },
  { source: 'dom_finance', target: 'brd_ai' },
  { source: 'dom_customer', target: 'brd_ai' },
];
