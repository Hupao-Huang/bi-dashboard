// ECharts 全局主题配置 - BI Dashboard
// 统一图表配色、字体、网格线、tooltip 等样式

// BI 经典调色盘 - 深青蓝主色 + 高对比度多色
// 参考 ECharts/Tableau/Power BI 业界标准，优先保证系列在图表中清晰可辨
export const CHART_COLORS = [
  '#1e40af', // 深青蓝（主色）
  '#f59e0b', // 金黄
  '#059669', // 翡翠绿
  '#dc2626', // 辣红
  '#06b6d4', // 青瓷蓝
  '#7c3aed', // 紫
  '#ea580c', // 橙红
  '#65a30d', // 松柏绿
  '#be123c', // 玫红
  '#64748b', // 石板灰
];

export const DEPT_COLORS: Record<string, string> = {
  all: '#1e293b',
  ecommerce: '#1e40af',    // 电商 → 深青蓝（主）
  social: '#f59e0b',       // 社媒 → 金黄
  offline: '#059669',      // 线下 → 翡翠绿
  distribution: '#7c3aed', // 分销 → 紫
};

// 产品定位等级色（S→D BI 经典热力渐变：最红→最灰）
// S=辣红（顶级强调）/ A=金黄（主打）/ B=青瓷（中档）/ C=翡翠（稳定）/ D=冷灰（淡化）
export const GRADE_COLORS: Record<string, string> = {
  S: '#dc2626',
  A: '#f59e0b',
  B: '#06b6d4',
  C: '#059669',
  D: '#94a3b8',
  '未设置': '#cbd5e1',
};

// 格式化金额
export const formatMoney = (v: number): string => {
  if (Math.abs(v) >= 100000000) return (v / 100000000).toFixed(1) + '亿';
  if (Math.abs(v) >= 10000) return (v / 10000).toFixed(0) + '万';
  return String(v);
};

export const getNiceAxisInterval = (maxValue: number, splits: number): number => {
  const safeMax = Math.max(maxValue, 1);
  const rawInterval = safeMax / splits;
  const magnitude = 10 ** Math.floor(Math.log10(rawInterval));
  const normalized = rawInterval / magnitude;

  if (normalized <= 1) return magnitude;
  if (normalized <= 2) return 2 * magnitude;
  if (normalized <= 5) return 5 * magnitude;
  return 10 * magnitude;
};

// 通用 tooltip 样式
const tooltipStyle = {
  backgroundColor: 'rgba(15, 23, 42, 0.92)',
  borderColor: 'rgba(255,255,255,0.08)',
  borderWidth: 1,
  borderRadius: 8,
  padding: [8, 12],
  textStyle: {
    color: '#e2e8f0',
    fontSize: 12,
    fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Microsoft YaHei", sans-serif',
  },
  extraCssText: 'box-shadow: 0 8px 24px rgba(0,0,0,0.2);',
};

// 通用 grid 间距
const gridStyle = {
  left: 56,
  right: 24,
  top: 48,
  bottom: 32,
  containLabel: false,
};

// 通用坐标轴样式
const axisCommon = {
  axisLine: { lineStyle: { color: '#cbd5e1' } },
  axisTick: { lineStyle: { color: '#cbd5e1' }, length: 3 },
  axisLabel: {
    color: '#64748b',
    fontSize: 11,
    fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
  },
  splitLine: {
    lineStyle: { color: '#e2e8f0', type: 'dashed' as const },
  },
  nameTextStyle: {
    color: '#64748b',
    fontSize: 11,
    padding: [0, 0, 0, 0],
  },
};

// 构造完整的基础 option，用于 merge
export const getBaseOption = () => ({
  tooltip: { ...tooltipStyle, trigger: 'axis' as const },
  grid: { ...gridStyle },
  color: CHART_COLORS,
  xAxis: { ...axisCommon },
  yAxis: { ...axisCommon },
  legend: {
    textStyle: { color: '#475569', fontSize: 12 },
    itemWidth: 12,
    itemHeight: 8,
    itemGap: 16,
    icon: 'roundRect',
  },
});

// 柱状图 item 样式增强
export const barItemStyle = (color: string) => ({
  color: {
    type: 'linear' as const,
    x: 0, y: 0, x2: 0, y2: 1,
    colorStops: [
      { offset: 0, color },
      { offset: 1, color: color + 'cc' },
    ],
  },
  borderRadius: [3, 3, 0, 0],
});

// 折线图 area 样式增强
export const lineAreaStyle = (color: string) => ({
  color,
  areaStyle: {
    color: {
      type: 'linear' as const,
      x: 0, y: 0, x2: 0, y2: 1,
      colorStops: [
        { offset: 0, color: color + '25' },
        { offset: 1, color: color + '02' },
      ],
    },
  },
});

// 饼图通用样式
export const pieStyle = {
  tooltip: { ...tooltipStyle, trigger: 'item' as const, formatter: '{b}: ¥{c} ({d}%)' },
  legend: {
    bottom: 0,
    textStyle: { color: '#475569', fontSize: 12 },
    itemWidth: 10,
    itemHeight: 10,
    itemGap: 12,
    icon: 'circle',
  },
};
