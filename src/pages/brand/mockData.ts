// 心智渗透 BI 看板 — 假数据 (2026-05-27 版, 真实事件感)
// 用真实店铺/SKU/关键词/月份/活动名, 6 个月趋势 (2025-12 ~ 2026-05)
// 跟现有 IT 可获取性分类对齐: green(已接入) / yellow(部分接入) / blue(系统算) / red(待接入)

export type MetricStatus = 'green' | 'yellow' | 'blue' | 'red';

export type MetricMeta = {
  code: string;
  name: string;
  layer: 'A' | 'B' | 'C' | 'T';
  layerName?: string;
  description: string;
  unit: string;
  baseline: string;
  teams: string[];
  status: MetricStatus;
  statusLabel: string;
  sourceNote?: string; // 数据来源备注
};

export type MonthPoint = {
  month: string;
  value: number;
};

export type MetricData = {
  meta: MetricMeta;
  current: number;
  prev: number;
  mom: number; // 环比 %
  health: 'good' | 'warn' | 'bad';
  trend: MonthPoint[];
};

export const MONTHS = ['2025-12', '2026-01', '2026-02', '2026-03', '2026-04', '2026-05'];

export const EVENT_MARKERS: Record<string, string> = {
  '2026-02': '春节',
  '2026-04': '江南大学揭牌',
  '2026-05': '517 高血压日',
};

export const TEAMS = ['公关', '品牌基建', '媒介', '市场营销', '产品营销', '线下营销'];

export const STATUS_LABEL: Record<MetricStatus, string> = {
  green: '已接入',
  yellow: '部分接入',
  blue: '系统算',
  red: '待接入',
};

export const STATUS_COLOR: Record<MetricStatus, string> = {
  green: '#52c41a',
  yellow: '#faad14',
  blue: '#1677ff',
  red: '#bfbfbf',
};

// 生成 6 个月趋势 (起点 base, 月度增长率 momPct, 春节降幅 springDrop, 加噪声)
function genTrend(base: number, momPct: number, springDrop = 0.85, noise = 0.05): MonthPoint[] {
  let v = base;
  return MONTHS.map((m, idx) => {
    if (idx > 0) {
      v = v * (1 + momPct);
      // 春节 2 月降
      if (m === '2026-02') v = v * springDrop;
      // 加 ±noise 的随机扰动
      v = v * (1 + (Math.random() - 0.5) * noise * 2);
    }
    return { month: m, value: Math.round(v * 100) / 100 };
  });
}

function fromTrend(trend: MonthPoint[]): { current: number; prev: number; mom: number } {
  const current = trend[trend.length - 1].value;
  const prev = trend[trend.length - 2].value;
  const mom = prev !== 0 ? ((current - prev) / Math.abs(prev)) * 100 : 0;
  return { current, prev, mom: Math.round(mom * 10) / 10 };
}

function healthOf(mom: number, threshold = 0): MetricData['health'] {
  if (mom > threshold + 3) return 'good';
  if (mom < threshold - 3) return 'bad';
  return 'warn';
}

// 固定种子的"随机", 让每次刷新数据稳定
let seedIdx = 0;
const seedRand = () => {
  seedIdx++;
  const x = Math.sin(seedIdx * 9999) * 10000;
  return x - Math.floor(x);
};
// 覆盖 Math.random 让上面的 noise 稳定
const origRandom = Math.random;
Math.random = seedRand;

function makeMetric(
  code: string,
  name: string,
  layer: 'A' | 'B' | 'C' | 'T',
  layerName: string,
  description: string,
  unit: string,
  baseline: string,
  teams: string[],
  status: MetricStatus,
  baseValue: number,
  momPct: number,
  sourceNote?: string,
): MetricData {
  const trend = genTrend(baseValue, momPct);
  const { current, prev, mom } = fromTrend(trend);
  return {
    meta: {
      code, name, layer, layerName, description, unit, baseline, teams,
      status, statusLabel: STATUS_LABEL[status], sourceNote,
    },
    trend,
    current,
    prev,
    mom,
    health: healthOf(mom),
  };
}

// 综合层 T-1 / T-2
export const COMPOSITE: MetricData[] = [
  makeMetric('T-1', '心智状态四象限判定', 'T', '综合', '横轴=知名度趋势, 纵轴=心智绑定趋势', '象限', '双重增长', ['品牌基建', 'IT'], 'blue', 0, 0, '系统算 (依赖 A/C 环比)'),
  makeMetric('T-2', '心智三层递进健康灯', 'T', '综合', '三盏灯, 任一红即总灯红', '灯', '三层全绿', ['品牌基建', 'IT'], 'blue', 0, 0, '系统算'),
];

// A 知名度层 15 个
export const LAYER_A: MetricData[] = [
  makeMetric('A1', '百度搜索指数-减钠关键词组', 'A', '知名度', '减钠调味+减钠酱油+减钠调味料月均之和', '次', '环比>0', ['IT', '品牌基建'], 'red', 1850, 0.08, '百度指数 API 待定 (跑哥暂搁置)'),
  makeMetric('A2', '百度搜索指数-减盐/健康调味', 'A', '知名度(对照)', '减盐+薄盐+低盐+0添加月均之和', '次', '对照观察', ['品牌基建'], 'red', 12600, 0.015, '百度指数 API 待定'),
  makeMetric('A3', '减钠概念渗透占比', 'A', '知名度', 'A1÷(A1+A2)×100%', '%', '逐月上升', ['IT'], 'blue', 12.8, 0.04, '系统算 (依赖 A1/A2)'),
  makeMetric('A4', '微信指数-减钠调味', 'A', '知名度', '每日微信指数取月平均', '次', '环比>0', ['市场营销'], 'red', 8900, 0.07, '微信指数小程序日抓 (未对接)'),
  makeMetric('A5', '微信指数-松鲜鲜', 'A', '知名度', '微信指数月均, 与 A4 组对照', '次', '环比>0', ['市场营销'], 'red', 24500, 0.06, '同 A4'),
  makeMetric('A6', '品类词电商搜索量-天猫', 'A', '知名度', '天猫减钠调味/酱油/调味料月搜索人次和', '人次', '环比>0', ['市场营销', '产品营销'], 'red', 42800, 0.085, '生意参谋_搜索分析 RPA 待补'),
  makeMetric('A7', '品类词电商搜索量-京东', 'A', '知名度', '京东同口径品类词搜索量', '人次', '环比>0', ['市场营销'], 'red', 18900, 0.07, '京东商智_搜索词 RPA 待补 (现有数据是排名区间)'),
  makeMetric('A8', '品牌词电商搜索量-天猫', 'A', '知名度', '天猫搜索"松鲜鲜"月人次', '人次', '环比>0', ['品牌基建'], 'red', 67200, 0.09, '同 A6'),
  makeMetric('A9', '品牌词电商搜索量-京东', 'A', '知名度', '京东同口径品牌词搜索量', '人次', '环比>0', ['品牌基建'], 'red', 28600, 0.07, '同 A7'),
  makeMetric('A10', '减钠内容新增量-抖音', 'A', '知名度', '当月减钠关键词下抖音新增视频量', '条', '环比>0', ['媒介', '市场营销'], 'red', 1240, 0.12, '巨量算数导出未对接'),
  makeMetric('A11', '减钠内容新增量-小红书', 'A', '知名度', '小红书同口径新增内容量', '篇', '环比>0', ['媒介', '市场营销'], 'red', 2680, 0.15, '千瓜数据导出未对接'),
  makeMetric('A12', '创始人IP内容曝光量', 'A', '知名度', '创始人相关内容曝光/播放量', '万次', '环比上升', ['公关'], 'green', 380, 0.18, '公关团队月度人工录入'),
  makeMetric('A13', '媒体报道数', 'A', '知名度', '主流/行业媒体月报道篇数', '篇', '环比上升', ['公关'], 'green', 28, 0.22, '公关甘萍凉月度录入 (15 分钟/月)'),
  makeMetric('A14', '品牌知名度(辅助提示)', 'A', '知名度', '季度调研, 提示后说出品牌名比例', '%', '逐季提升', ['品牌基建'], 'green', 68, 0.05, '品牌基建季度问卷'),
  makeMetric('A15', '减钠物料门店销售增速对比', 'A', '知名度', '已替换 vs 未替换门店减钠系列月销环比之差', '%', '差值>5%', ['线下营销', 'IT'], 'yellow', 7.2, 0.06, '吉客云 POS 有, 缺 SKU 清单+门店分组台账'),
];

// B 好感度层 12 个
export const LAYER_B: MetricData[] = [
  makeMetric('B1', 'NPS (净推荐值)', 'B', '好感度', '推荐者比例-贬损者比例', '分', '≥30', ['品牌基建'], 'green', 38, 0.02, '品牌基建季度问卷'),
  makeMetric('B2', '电商综合好评率-天猫', 'B', '好感度', '天猫店铺 4-5 星评价占比', '%', '≥95%', ['产品营销'], 'red', 96.2, 0.002, '生意参谋_店铺评价 RPA 待补'),
  makeMetric('B3', '电商综合好评率-京东', 'B', '好感度', '京东店铺 4-5 星评价占比', '%', '≥95%', ['产品营销'], 'red', 95.8, 0.001, '京东_店铺评价 RPA 待补'),
  makeMetric('B4', '电商综合好评率-抖音', 'B', '好感度', '抖音店铺 4-5 星评价占比', '%', '≥95%', ['产品营销'], 'red', 94.5, 0.003, '抖音罗盘_店铺评价 RPA 待补'),
  makeMetric('B5', '正面声量占比', 'B', '好感度', '全网正面提及占比 (NLP)', '%', '≥80%', ['公关'], 'red', 82.5, 0.01, '舆情工具+NLP 未到位'),
  makeMetric('B6', '负面舆情数', 'B', '好感度', '月内负面提及/投诉数量', '条', '逐月下降', ['公关'], 'red', 14, -0.08, '舆情工具未到位'),
  makeMetric('B7', '舆情响应时效', 'B', '好感度', '发现负面到首次响应平均时长', '小时', '≤4 小时', ['公关'], 'red', 3.2, -0.05, '舆情工单系统未建'),
  makeMetric('B8', '品牌溢价指数-线上', 'B', '好感度', '松鲜鲜线上均价÷品类均价', '倍', '≥1.5', ['品牌基建'], 'red', 1.62, 0.015, '生意参谋_行业粒度 RPA 待补'),
  makeMetric('B9', '品牌溢价指数-线下', 'B', '好感度', '松鲜鲜线下均价÷品类均价', '倍', '≥1.5', ['线下营销', '品牌基建'], 'yellow', 1.48, 0.01, '吉客云有自家均价, 缺线下竞品价'),
  makeMetric('B10', '种草内容 CPE', 'B', '好感度', '种草花费÷总互动数', '元', '≤5 元', ['媒介'], 'green', 4.6, -0.04, '媒介团队月度录入'),
  makeMetric('B11', '行业奖项/认证数', 'B', '好感度', '本月获得行业奖项或认证数', '项', '持续积累', ['公关'], 'green', 2, 0.10, '公关按事件录入'),
  makeMetric('B12', '核心媒体覆盖率', 'B', '好感度', '已合作媒体÷目标核心媒体', '%', '逐季上升', ['公关'], 'green', 56, 0.06, '公关团队月度录入'),
];

// C 心智绑定层 14 个
export const LAYER_C: MetricData[] = [
  makeMetric('C1', '品类关联度-抖音', 'C', '心智绑定', '抖音搜"减钠调味"前 100 条含松鲜鲜占比', '%', '≥30%', ['媒介'], 'green', 32, 0.08, '媒介(大可)月度手动抽样 30 分钟'),
  makeMetric('C2', '品类关联度-小红书', 'C', '心智绑定', '小红书同口径品类关联度', '%', '≥30%', ['媒介'], 'green', 28, 0.09, '媒介团队月度手动抽样'),
  makeMetric('C3', 'SOV-减钠子话题声量份额', 'C', '心智绑定', '减钠话题下松鲜鲜提及量占主要竞品总和', '%', '≥50%', ['公关'], 'red', 52, 0.03, '舆情工具+关键词过滤未到位'),
  makeMetric('C4', 'SOV-全话题声量份额', 'C', '心智绑定', '调味品全话题中松鲜鲜声量占比', '%', '与 C3 对比', ['公关'], 'red', 12, 0.02, '舆情工具未到位'),
  makeMetric('C5', '品牌/品类搜索绑定度', 'C', '心智绑定', '搜品类词的人群中同时搜品牌词占比', '%', '逐月上升', ['IT'], 'blue', 142, 0.025, '系统算 =(A8+A9)/(A6+A7) (依赖 A6-A9)'),
  makeMetric('C6', '搜索品牌归因-点击份额', 'C', '心智绑定', '搜品类词后点击松鲜鲜占比', '%', '排名第一', ['产品营销'], 'red', 24, 0.04, '生意参谋/商智搜索分析 RPA 待补'),
  makeMetric('C7', '语义绑定率', 'C', '心智绑定', '用户主动发布减钠内容中自然提及松鲜鲜占比', '%', '逐月上升', ['市场营销'], 'red', 18, 0.06, 'Q3 NLP 到位后启动'),
  makeMetric('C8', '评价"减钠"关键词渗透率', 'C', '心智绑定', '全平台评价中自然出现"减钠"占比', '%', '逐月上升', ['产品营销'], 'red', 9, 0.10, 'Q3 NLP 到位后启动'),
  makeMetric('C9', '品类教育品牌渗透率', 'C', '心智绑定', '减钠内容中提及松鲜鲜量÷减钠内容总量', '%', '逐月上升', ['市场营销'], 'blue', 15, 0.07, '系统算 (依赖 A10/A11)'),
  makeMetric('C10', '同门店减钠 vs 减盐销量对比', 'C', '心智绑定', '同门店松鲜鲜减钠÷减盐竞品销量', '%', '>100%', ['线下营销', 'IT'], 'yellow', 88, 0.08, '吉客云 POS 有, 缺减钠/减盐 SKU 清单'),
  makeMetric('C11', '减钠系列非促销期自然动销占比', 'C', '心智绑定', '非促销时段减钠销量÷全系列销量', '%', '逐月上升', ['线下营销', 'IT'], 'yellow', 42, 0.04, 'POS 有, 缺促销日历'),
  makeMetric('C12', '竞品"减钠"跟进事件数', 'C', '心智绑定', '当月竞品包装/物料/宣传用减钠表述事件数', '件', '监测跟踪', ['市场营销', '品牌基建'], 'green', 4, 0.30, '市场营销/品牌基建录入'),
  makeMetric('C13', '渠道品类拉力事件数', 'C', '心智绑定', 'B 端主动以减钠找松鲜鲜合作事件数', '件', '持续积累', ['线下营销'], 'green', 6, 0.25, '业务团队录入 (许府牛=1 件)'),
  makeMetric('C14', '减钠系列新客首购占比', 'C', '心智绑定', '新客首单含减钠 SKU 占比', '%', '>40%', ['线下营销', 'IT'], 'yellow', 36, 0.08, '吉客云 POS+CRM 有, 缺 SKU 清单+新客规则'),
];

// 还原 Math.random
Math.random = origRandom;

// 索引: 编号 → 数据
export const ALL_METRICS: MetricData[] = [...COMPOSITE, ...LAYER_A, ...LAYER_B, ...LAYER_C];
export const METRIC_MAP = ALL_METRICS.reduce((acc, m) => {
  acc[m.meta.code] = m;
  return acc;
}, {} as Record<string, MetricData>);

// 团队归因矩阵 (6 团队 × 3 层级)
// 值 = 该团队在该层级关联指标的综合环比 %
export type TeamCellHealth = 'good' | 'warn' | 'bad';
export type TeamCell = { team: string; layer: 'A' | 'B' | 'C'; mom: number; health: TeamCellHealth; metrics: string[] };

function teamLayerMom(team: string, layer: 'A' | 'B' | 'C', layerMetrics: MetricData[]): TeamCell {
  const related = layerMetrics.filter(m => m.meta.teams.includes(team));
  const avgMom = related.length > 0 ? related.reduce((s, m) => s + m.mom, 0) / related.length : 0;
  const mom = Math.round(avgMom * 10) / 10;
  let health: TeamCellHealth = 'warn';
  if (mom > 3) health = 'good';
  else if (mom < -3) health = 'bad';
  return { team, layer, mom, health, metrics: related.map(m => m.meta.code) };
}

export const TEAM_MATRIX: TeamCell[] = TEAMS.flatMap(team => [
  teamLayerMom(team, 'A', LAYER_A),
  teamLayerMom(team, 'B', LAYER_B),
  teamLayerMom(team, 'C', LAYER_C),
]);

// 4 象限判定 (T-1)
export type QuadrantData = {
  xAxis: number; // 知名度综合环比
  yAxis: number; // 心智绑定综合环比
  quadrant: '双重增长' | '认知优先' | '绑定优先' | '双弱';
  desc: string;
};

function calcQuadrant(): QuadrantData {
  // X 轴: A1*0.4 + A5*0.3 + A8*0.3 环比
  const x = METRIC_MAP['A1'].mom * 0.4 + METRIC_MAP['A5'].mom * 0.3 + METRIC_MAP['A8'].mom * 0.3;
  // Y 轴: C1*0.4 + C3*0.3 + C5*0.3 环比
  const y = METRIC_MAP['C1'].mom * 0.4 + METRIC_MAP['C3'].mom * 0.3 + METRIC_MAP['C5'].mom * 0.3;
  let quadrant: QuadrantData['quadrant'] = '双弱';
  let desc = '品类认知和品牌绑定都在收缩, 警戒';
  if (x > 0 && y > 0) { quadrant = '双重增长'; desc = '品类认知↑+品牌份额↑, 健康'; }
  else if (x > 0 && y <= 0) { quadrant = '认知优先'; desc = '认知在涨, 绑定未跟上, 需加强品牌锚点'; }
  else if (x <= 0 && y > 0) { quadrant = '绑定优先'; desc = '绑定在涨, 认知未扩, 需扩品类盘'; }
  return {
    xAxis: Math.round(x * 10) / 10,
    yAxis: Math.round(y * 10) / 10,
    quadrant,
    desc,
  };
}

export const QUADRANT: QuadrantData = calcQuadrant();

// 三层健康灯 (T-2)
export type LayerHealth = { layer: 'A' | 'B' | 'C'; name: string; health: 'green' | 'red'; comment: string; momAvg: number };

function calcLayerHealth(layer: 'A' | 'B' | 'C', metrics: MetricData[], comment: string): LayerHealth {
  const momAvg = metrics.reduce((s, m) => s + m.mom, 0) / metrics.length;
  return {
    layer,
    name: layer === 'A' ? '知名度层' : layer === 'B' ? '好感度层' : '心智绑定层',
    health: momAvg > 0 ? 'green' : 'red',
    momAvg: Math.round(momAvg * 10) / 10,
    comment,
  };
}

export const HEALTH_LIGHTS: LayerHealth[] = [
  calcLayerHealth('A', LAYER_A, '品类认知池稳步扩张, 减钠话题度持续提升'),
  calcLayerHealth('B', LAYER_B, '电商好评稳健, 舆情健康度良好'),
  calcLayerHealth('C', LAYER_C, '心智绑定持续走强, C10 线下指数接近 100% 节点'),
];

// 真实事件 (用于趋势曲线标记)
export const TIMELINE_EVENTS = [
  { month: '2026-02', name: '春节', type: 'season' },
  { month: '2026-04', name: '江南大学联合实验室揭牌', type: 'pr' },
  { month: '2026-05', name: '517 全国高血压日活动', type: 'campaign' },
  { month: '2026-05', name: '老爸评测 D-Day 筹备 (6.3)', type: 'campaign' },
];

// 真实店铺清单 (用于电商指标分平台显示)
export const SHOPS = {
  tmall: ['松鲜鲜调味品旗舰店', '松鲜鲜挚先专卖店', '糙能农场旗舰店'],
  jd: ['清心湖调味品旗舰店', '糙能农场旗舰店'],
  douyin: ['松鲜鲜官方旗舰店', '松鲜鲜调味品旗舰店'],
};

// 公关/媒体清单 (B12 用)
export const CORE_MEDIA = [
  { name: '人民日报健康客户端', covered: true },
  { name: '新华社', covered: true },
  { name: '央视财经', covered: true },
  { name: '中国新闻周刊', covered: true },
  { name: '财经', covered: false },
  { name: '南方周末', covered: true },
  { name: '虎嗅', covered: true },
  { name: '36 氪', covered: true },
  { name: '澎湃新闻', covered: false },
  { name: '凤凰网财经', covered: false },
];

// 6 月 D-Day 关键事件预告 (用于事件时间轴)
export const UPCOMING_EVENTS = [
  { date: '2026-06-03', title: '老爸评测 D-Day', desc: 'PR 稿 50 家媒体 + 抖音营销号 50 条 + 头条 150 条 + 小红书 KOC 350 条' },
  { date: '2026-06-中', title: '博鳌峰会 PR 发布', desc: '权威背书 + 行业奖项' },
  { date: '2026-06-下', title: '李佳琦直播', desc: '品牌词搜索预计有显著上扬' },
];
