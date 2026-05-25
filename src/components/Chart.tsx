import React, { Suspense, lazy, useMemo } from 'react';
import { Spin } from 'antd';
import { getBaseOption } from '../chartTheme';

const LazyReactECharts = lazy(() => import('echarts-for-react'));
const EChartsComponent = LazyReactECharts as React.ComponentType<any>;

type ChartProps = Record<string, unknown> & {
  option?: Record<string, any>;
  style?: React.CSSProperties;
};

const defaultFallbackStyle: React.CSSProperties = {
  minHeight: 240,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
};

const mergeAxisOption = (baseAxis: Record<string, any>, axis: Record<string, any>) => ({
  ...baseAxis,
  ...axis,
  axisLine: { ...baseAxis.axisLine, ...(axis?.axisLine || {}) },
  axisTick: { ...baseAxis.axisTick, ...(axis?.axisTick || {}) },
  axisLabel: { ...baseAxis.axisLabel, ...(axis?.axisLabel || {}) },
  splitLine: { ...baseAxis.splitLine, ...(axis?.splitLine || {}) },
  nameTextStyle: { ...baseAxis.nameTextStyle, ...(axis?.nameTextStyle || {}) },
});

const themedOption = (option?: Record<string, any>) => {
  if (!option) return option;
  const base = getBaseOption() as Record<string, any>;
  const merged: Record<string, any> = {
    ...option,
    color: option.color || base.color,
    tooltip: { ...base.tooltip, ...(option.tooltip || {}) },
    legend: { ...base.legend, ...(option.legend || {}) },
    grid: { ...base.grid, ...(option.grid || {}) },
  };

  if (Array.isArray(option.xAxis)) {
    merged.xAxis = option.xAxis.map((axis: Record<string, any>) => mergeAxisOption(base.xAxis, axis));
  } else if (option.xAxis) {
    merged.xAxis = mergeAxisOption(base.xAxis, option.xAxis as Record<string, any>);
  }

  if (Array.isArray(option.yAxis)) {
    merged.yAxis = option.yAxis.map((axis: Record<string, any>) => mergeAxisOption(base.yAxis, axis));
  } else if (option.yAxis) {
    merged.yAxis = mergeAxisOption(base.yAxis, option.yAxis as Record<string, any>);
  }

  return merged;
};

const Chart: React.FC<ChartProps> = ({ style, option, ...props }) => {
  const mergedOption = useMemo(() => themedOption(option), [option]);

  // v1.74.3-3 跑哥 5/25 修: notMerge=true 让 setOption 全替换不残留旧 series
  // 之前 merge 模式下切部门 tab, ecommerce 的"调拨额"黄柱 series 残留到其它部门 tab
  return (
    <Suspense fallback={<div style={style ? { ...style, display: 'flex', alignItems: 'center', justifyContent: 'center' } : defaultFallbackStyle}><Spin /></div>}>
      <EChartsComponent notMerge {...props} option={mergedOption} style={style} />
    </Suspense>
  );
};

export default Chart;
