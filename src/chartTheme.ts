// ECharts 全局主题配置 - BI Dashboard
// 统一图表配色、字体、网格线、tooltip 等样式

export const CHART_COLORS = [
  '#4f46e5', // indigo (primary)
  '#10b981', // green
  '#f59e0b', // amber
  '#8b5cf6', // purple
  '#ec4899', // pink
  '#06b6d4', // cyan
  '#ef4444', // red
  '#6366f1', // indigo-light
  '#14b8a6', // teal
  '#f97316', // orange
];

export const DEPT_COLORS: Record<string, string> = {
  all: '#1e293b',
  ecommerce: '#4f46e5',
  social: '#10b981',
  offline: '#faad14',
  distribution: '#8b5cf6',
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
  axisLine: { lineStyle: { color: '#e2e8f0' } },
  axisTick: { lineStyle: { color: '#e2e8f0' }, length: 3 },
  axisLabel: {
    color: '#94a3b8',
    fontSize: 11,
    fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
  },
  splitLine: {
    lineStyle: { color: '#f1f5f9', type: 'dashed' as const },
  },
  nameTextStyle: {
    color: '#94a3b8',
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
    textStyle: { color: '#64748b', fontSize: 12 },
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
    textStyle: { color: '#64748b', fontSize: 12 },
    itemWidth: 10,
    itemHeight: 10,
    itemGap: 12,
    icon: 'circle',
  },
};
