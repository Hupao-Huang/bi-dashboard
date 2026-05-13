// 同花顺/通达信风格的专业 K 线图组件
//
// 布局（3 个 grid 上下叠）:
//   主图: 价格 K 线 + MA5/10/20/60 均线
//   副图1: 成交量柱图（红涨绿跌）
//   副图2: 技术指标（MACD/KDJ/RSI/BOLL 切换）
//
// 顶部价格信息栏（同花顺风格）: 开/高/低/收/涨跌/涨跌幅/振幅/成交量 + MA 数值
//
// 周期支持: 日K / 周K / 月K（前端把日线聚合）
//
// 直接用 echarts-for-react 不走 Chart.tsx 包装，因为后者的 themedOption 合并多 yAxis 数组会触发
// ECharts "Cannot read properties of undefined (reading 'coordinateSystem')" 报错
import React, { useMemo, lazy, Suspense } from 'react';
import { Spin } from 'antd';
import dayjs from 'dayjs';
import isoWeek from 'dayjs/plugin/isoWeek';
import type { FuturesBar } from './types';

dayjs.extend(isoWeek);

const ReactECharts = lazy(() => import('echarts-for-react'));

// ==================== 同花顺配色 ====================
const COLOR = {
  up: '#ef232a',           // 涨：深红
  down: '#14b143',         // 跌：深绿
  ma5: '#f8b500',          // MA5：黄
  ma10: '#00c8ff',         // MA10：青
  ma20: '#ff37dc',         // MA20：粉
  ma60: '#a900cb',         // MA60：紫
  bg: '#ffffff',           // 背景白
  grid: '#f0f0f0',         // 网格灰
  text: '#1f2937',         // 文字
  textDim: '#9ca3af',      // 次要文字
  macdDif: '#f8b500',      // MACD-DIF 黄
  macdDea: '#00c8ff',      // MACD-DEA 青
  kdjK: '#f8b500',
  kdjD: '#00c8ff',
  kdjJ: '#ff37dc',
  rsi6: '#f8b500',
  rsi12: '#00c8ff',
  rsi24: '#ff37dc',
  bollMid: '#f8b500',
  bollUp: '#ff37dc',
  bollLow: '#00c8ff',
};

export type Period = 'day' | 'week' | 'month';
export type Indicator = 'MACD' | 'KDJ' | 'RSI' | 'BOLL';

interface KlineChartProps {
  bars: FuturesBar[];
  period?: Period;
  indicator?: Indicator;
  height?: number;
  unit?: string;
  title?: string;
}

// ==================== 周期聚合：日线 → 周/月线 ====================
function aggregateBars(bars: FuturesBar[], period: Period): FuturesBar[] {
  if (period === 'day') return bars;
  const groups = new Map<string, FuturesBar[]>();
  for (const b of bars) {
    const d = dayjs(b.date);
    const key = period === 'week'
      ? `${d.isoWeekYear()}-W${String(d.isoWeek()).padStart(2, '0')}`
      : d.format('YYYY-MM');
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key)!.push(b);
  }
  const out: FuturesBar[] = [];
  groups.forEach((arr) => {
    if (arr.length === 0) return;
    arr.sort((a, b) => a.date.localeCompare(b.date));
    out.push({
      date: arr[arr.length - 1].date,        // 周/月末日期
      open: arr[0].open,
      close: arr[arr.length - 1].close,
      high: Math.max(...arr.map(b => b.high)),
      low: Math.min(...arr.map(b => b.low)),
      volume: arr.reduce((s, b) => s + b.volume, 0),
      openInterest: arr[arr.length - 1].openInterest,
    });
  });
  out.sort((a, b) => a.date.localeCompare(b.date));
  return out;
}

// ==================== 技术指标计算 ====================
function calcMA(closes: number[], n: number): (number | null)[] {
  const out: (number | null)[] = [];
  let sum = 0;
  for (let i = 0; i < closes.length; i++) {
    sum += closes[i];
    if (i >= n) sum -= closes[i - n];
    out.push(i >= n - 1 ? +(sum / n).toFixed(2) : null);
  }
  return out;
}

function calcEMA(closes: number[], n: number): number[] {
  const out: number[] = [];
  const k = 2 / (n + 1);
  let prev = closes[0] ?? 0;
  for (let i = 0; i < closes.length; i++) {
    const v = i === 0 ? closes[0] : closes[i] * k + prev * (1 - k);
    out.push(v);
    prev = v;
  }
  return out;
}

function calcMACD(closes: number[]): { dif: number[]; dea: number[]; bar: number[] } {
  const ema12 = calcEMA(closes, 12);
  const ema26 = calcEMA(closes, 26);
  const dif = ema12.map((v, i) => +(v - ema26[i]).toFixed(3));
  const dea = calcEMA(dif, 9).map(v => +v.toFixed(3));
  const bar = dif.map((v, i) => +((v - dea[i]) * 2).toFixed(3));
  return { dif, dea, bar };
}

function calcKDJ(bars: FuturesBar[]): { k: number[]; d: number[]; j: number[] } {
  const n = 9;
  const rsv: number[] = [];
  for (let i = 0; i < bars.length; i++) {
    if (i < n - 1) {
      rsv.push(50);
      continue;
    }
    let hh = -Infinity, ll = Infinity;
    for (let j = i - n + 1; j <= i; j++) {
      if (bars[j].high > hh) hh = bars[j].high;
      if (bars[j].low < ll) ll = bars[j].low;
    }
    const v = hh === ll ? 50 : ((bars[i].close - ll) / (hh - ll)) * 100;
    rsv.push(v);
  }
  const k: number[] = [], d: number[] = [], j: number[] = [];
  let kPrev = 50, dPrev = 50;
  for (let i = 0; i < bars.length; i++) {
    const kv = (2 / 3) * kPrev + (1 / 3) * rsv[i];
    const dv = (2 / 3) * dPrev + (1 / 3) * kv;
    const jv = 3 * kv - 2 * dv;
    k.push(+kv.toFixed(2));
    d.push(+dv.toFixed(2));
    j.push(+jv.toFixed(2));
    kPrev = kv; dPrev = dv;
  }
  return { k, d, j };
}

function calcRSI(closes: number[], n: number): number[] {
  const out: number[] = [];
  let gainSum = 0, lossSum = 0;
  for (let i = 0; i < closes.length; i++) {
    if (i === 0) { out.push(50); continue; }
    const diff = closes[i] - closes[i - 1];
    const gain = Math.max(diff, 0);
    const loss = Math.max(-diff, 0);
    if (i < n) {
      gainSum += gain;
      lossSum += loss;
      out.push(50);
    } else if (i === n) {
      gainSum += gain;
      lossSum += loss;
      const avgGain = gainSum / n;
      const avgLoss = lossSum / n;
      out.push(avgLoss === 0 ? 100 : +(100 - 100 / (1 + avgGain / avgLoss)).toFixed(2));
    } else {
      gainSum = (gainSum * (n - 1) + gain) / n;
      lossSum = (lossSum * (n - 1) + loss) / n;
      out.push(lossSum === 0 ? 100 : +(100 - 100 / (1 + gainSum / lossSum)).toFixed(2));
    }
  }
  return out;
}

function calcBOLL(closes: number[], n: number = 20): { mid: (number | null)[]; up: (number | null)[]; low: (number | null)[] } {
  const mid = calcMA(closes, n);
  const up: (number | null)[] = [];
  const low: (number | null)[] = [];
  for (let i = 0; i < closes.length; i++) {
    if (mid[i] === null) { up.push(null); low.push(null); continue; }
    const m = mid[i] as number;
    let sumSq = 0;
    for (let j = i - n + 1; j <= i; j++) sumSq += (closes[j] - m) ** 2;
    const sd = Math.sqrt(sumSq / n);
    up.push(+(m + 2 * sd).toFixed(2));
    low.push(+(m - 2 * sd).toFixed(2));
  }
  return { mid, up, low };
}

// ==================== 主组件 ====================
const KlineChart: React.FC<KlineChartProps> = ({
  bars,
  period = 'day',
  indicator = 'MACD',
  height = 600,
  unit = '',
  title = '',
}) => {
  const option = useMemo(() => {
    if (bars.length === 0) return null;
    const aggBars = aggregateBars(bars, period);
    const dates = aggBars.map(b => dayjs(b.date).format('YYYY-MM-DD'));
    const closes = aggBars.map(b => b.close);

    // K 线数据 [O, C, L, H]
    const klineData = aggBars.map(b => [b.open, b.close, b.low, b.high]);

    // 成交量（按涨跌染色）
    const volumeData = aggBars.map(b => ({
      value: b.volume,
      itemStyle: { color: b.close >= b.open ? COLOR.up : COLOR.down },
    }));

    // 4 条均线
    const ma5 = calcMA(closes, 5);
    const ma10 = calcMA(closes, 10);
    const ma20 = calcMA(closes, 20);
    const ma60 = calcMA(closes, 60);

    // 副图指标系列
    let indicatorSeries: any[] = [];
    let indicatorYAxisName = '';
    if (indicator === 'MACD') {
      const { dif, dea, bar } = calcMACD(closes);
      indicatorYAxisName = 'MACD';
      indicatorSeries = [
        {
          name: 'DIF', type: 'line', xAxisIndex: 2, yAxisIndex: 2, data: dif,
          showSymbol: false, lineStyle: { width: 1, color: COLOR.macdDif },
        },
        {
          name: 'DEA', type: 'line', xAxisIndex: 2, yAxisIndex: 2, data: dea,
          showSymbol: false, lineStyle: { width: 1, color: COLOR.macdDea },
        },
        {
          name: 'MACD', type: 'bar', xAxisIndex: 2, yAxisIndex: 2,
          data: bar.map(v => ({ value: v, itemStyle: { color: v >= 0 ? COLOR.up : COLOR.down } })),
        },
      ];
    } else if (indicator === 'KDJ') {
      const { k, d, j } = calcKDJ(aggBars);
      indicatorYAxisName = 'KDJ';
      indicatorSeries = [
        { name: 'K', type: 'line', xAxisIndex: 2, yAxisIndex: 2, data: k, showSymbol: false, lineStyle: { width: 1, color: COLOR.kdjK } },
        { name: 'D', type: 'line', xAxisIndex: 2, yAxisIndex: 2, data: d, showSymbol: false, lineStyle: { width: 1, color: COLOR.kdjD } },
        { name: 'J', type: 'line', xAxisIndex: 2, yAxisIndex: 2, data: j, showSymbol: false, lineStyle: { width: 1, color: COLOR.kdjJ } },
      ];
    } else if (indicator === 'RSI') {
      const rsi6 = calcRSI(closes, 6);
      const rsi12 = calcRSI(closes, 12);
      const rsi24 = calcRSI(closes, 24);
      indicatorYAxisName = 'RSI';
      indicatorSeries = [
        { name: 'RSI6', type: 'line', xAxisIndex: 2, yAxisIndex: 2, data: rsi6, showSymbol: false, lineStyle: { width: 1, color: COLOR.rsi6 } },
        { name: 'RSI12', type: 'line', xAxisIndex: 2, yAxisIndex: 2, data: rsi12, showSymbol: false, lineStyle: { width: 1, color: COLOR.rsi12 } },
        { name: 'RSI24', type: 'line', xAxisIndex: 2, yAxisIndex: 2, data: rsi24, showSymbol: false, lineStyle: { width: 1, color: COLOR.rsi24 } },
      ];
    } else if (indicator === 'BOLL') {
      // BOLL 是主图叠加指标，副图区显示带宽
      const { mid, up, low } = calcBOLL(closes, 20);
      const bandwidth = mid.map((m, i) => {
        if (m === null || up[i] === null || low[i] === null) return null;
        return +(((up[i] as number) - (low[i] as number)) / (m as number) * 100).toFixed(2);
      });
      indicatorYAxisName = '带宽%';
      indicatorSeries = [
        { name: 'BOLL带宽', type: 'line', xAxisIndex: 2, yAxisIndex: 2, data: bandwidth, showSymbol: false, areaStyle: { opacity: 0.2 }, lineStyle: { width: 1, color: COLOR.bollMid } },
      ];
    }

    return {
      backgroundColor: COLOR.bg,
      animation: false,
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'cross', link: [{ xAxisIndex: 'all' }], lineStyle: { color: COLOR.textDim, type: 'dashed' } },
        backgroundColor: 'rgba(255, 255, 255, 0.95)',
        borderColor: COLOR.grid,
        textStyle: { color: COLOR.text, fontSize: 12 },
        formatter: (params: any[]) => {
          if (!params || params.length === 0) return '';
          const idx = params[0].dataIndex;
          const b = aggBars[idx];
          if (!b) return '';
          const change = b.close - b.open;
          const changePct = b.open === 0 ? 0 : (change / b.open) * 100;
          const color = change >= 0 ? COLOR.up : COLOR.down;
          const sign = change >= 0 ? '+' : '';
          const lines = [
            `<b>${dates[idx]}</b>`,
            `开盘 <b>${b.open}</b>　收盘 <b style="color:${color}">${b.close}</b>`,
            `最高 <b style="color:${COLOR.up}">${b.high}</b>　最低 <b style="color:${COLOR.down}">${b.low}</b>`,
            `涨跌 <b style="color:${color}">${sign}${change.toFixed(2)} (${sign}${changePct.toFixed(2)}%)</b>`,
            `成交量 <b>${b.volume.toLocaleString()}</b>　持仓 <b>${b.openInterest.toLocaleString()}</b>`,
          ];
          const maLines: string[] = [];
          if (ma5[idx] !== null) maLines.push(`<span style="color:${COLOR.ma5}">MA5: ${ma5[idx]}</span>`);
          if (ma10[idx] !== null) maLines.push(`<span style="color:${COLOR.ma10}">MA10: ${ma10[idx]}</span>`);
          if (ma20[idx] !== null) maLines.push(`<span style="color:${COLOR.ma20}">MA20: ${ma20[idx]}</span>`);
          if (ma60[idx] !== null) maLines.push(`<span style="color:${COLOR.ma60}">MA60: ${ma60[idx]}</span>`);
          if (maLines.length > 0) lines.push(maLines.join('　'));
          return lines.join('<br/>');
        },
      },
      legend: {
        data: [
          { name: 'MA5', icon: 'line', itemStyle: { color: COLOR.ma5 } },
          { name: 'MA10', icon: 'line', itemStyle: { color: COLOR.ma10 } },
          { name: 'MA20', icon: 'line', itemStyle: { color: COLOR.ma20 } },
          { name: 'MA60', icon: 'line', itemStyle: { color: COLOR.ma60 } },
        ],
        top: 4, left: 60, textStyle: { color: COLOR.text, fontSize: 12 },
      },
      axisPointer: { link: [{ xAxisIndex: 'all' }] },
      grid: [
        { left: 60, right: 30, top: 36, height: '56%' },     // 主图
        { left: 60, right: 30, top: '64%', height: '14%' },   // 成交量
        { left: 60, right: 30, top: '80%', height: '14%' },   // 指标
      ],
      xAxis: [
        // 主图
        {
          type: 'category', data: dates, scale: true, boundaryGap: false,
          axisLine: { lineStyle: { color: COLOR.grid } },
          axisLabel: { show: false }, axisTick: { show: false },
          splitLine: { show: false }, axisPointer: { z: 100 },
        },
        // 成交量
        {
          type: 'category', gridIndex: 1, data: dates, scale: true, boundaryGap: false,
          axisLine: { lineStyle: { color: COLOR.grid } },
          axisLabel: { show: false }, axisTick: { show: false },
          splitLine: { show: false },
        },
        // 指标
        {
          type: 'category', gridIndex: 2, data: dates, scale: true, boundaryGap: false,
          axisLine: { lineStyle: { color: COLOR.grid } },
          axisLabel: { color: COLOR.textDim, fontSize: 11 }, axisTick: { show: false },
          splitLine: { show: false },
        },
      ],
      yAxis: [
        // 主图 Y 轴：价格
        {
          scale: true, position: 'right',
          axisLine: { show: false }, axisTick: { show: false },
          axisLabel: { color: COLOR.textDim, fontSize: 11, inside: false },
          splitLine: { lineStyle: { color: COLOR.grid, type: 'dashed' } },
          name: unit, nameTextStyle: { color: COLOR.textDim, fontSize: 11, padding: [0, 0, 0, 30] },
        },
        // 成交量 Y 轴
        {
          gridIndex: 1, scale: true, position: 'right', splitNumber: 2,
          axisLine: { show: false }, axisTick: { show: false },
          axisLabel: {
            color: COLOR.textDim, fontSize: 11,
            formatter: (v: number) => v >= 10000 ? (v / 10000).toFixed(0) + 'w' : v.toString(),
          },
          splitLine: { show: false },
        },
        // 指标 Y 轴
        {
          gridIndex: 2, scale: true, position: 'right', splitNumber: 3,
          axisLine: { show: false }, axisTick: { show: false },
          axisLabel: { color: COLOR.textDim, fontSize: 11 },
          splitLine: { show: false },
          name: indicatorYAxisName, nameTextStyle: { color: COLOR.textDim, fontSize: 10, padding: [0, 0, 0, 28] },
        },
      ],
      dataZoom: [
        { type: 'inside', xAxisIndex: [0, 1, 2], start: Math.max(0, 100 - (200 / aggBars.length) * 100), end: 100 },
        { show: true, xAxisIndex: [0, 1, 2], type: 'slider', bottom: 4, height: 18, borderColor: COLOR.grid },
      ],
      series: [
        // 主图：蜡烛
        {
          name: title || 'K线',
          type: 'candlestick',
          data: klineData,
          itemStyle: {
            color: COLOR.up, color0: COLOR.down,
            borderColor: COLOR.up, borderColor0: COLOR.down,
          },
        },
        // MA 均线
        { name: 'MA5', type: 'line', data: ma5, showSymbol: false, smooth: false, lineStyle: { width: 1, color: COLOR.ma5 } },
        { name: 'MA10', type: 'line', data: ma10, showSymbol: false, smooth: false, lineStyle: { width: 1, color: COLOR.ma10 } },
        { name: 'MA20', type: 'line', data: ma20, showSymbol: false, smooth: false, lineStyle: { width: 1, color: COLOR.ma20 } },
        { name: 'MA60', type: 'line', data: ma60, showSymbol: false, smooth: false, lineStyle: { width: 1, color: COLOR.ma60 } },
        // 成交量
        { name: '成交量', type: 'bar', xAxisIndex: 1, yAxisIndex: 1, data: volumeData },
        // 副图指标
        ...indicatorSeries,
      ],
    };
  }, [bars, period, indicator, unit, title]);

  if (!option) {
    return <div style={{ height, display: 'flex', alignItems: 'center', justifyContent: 'center', color: COLOR.textDim }}>暂无数据</div>;
  }

  return (
    <Suspense fallback={<div style={{ height, display: 'flex', alignItems: 'center', justifyContent: 'center' }}><Spin /></div>}>
      <ReactECharts option={option} style={{ height, width: '100%' }} notMerge lazyUpdate />
    </Suspense>
  );
};

export default KlineChart;
