// 期货行情数据结构（跟后端 internal/handler/futures.go 对齐）

export interface FuturesSymbol {
  code: string;
  nameCn: string;
  exchange: string;        // DCE/CZCE/SHFE/INE/CFFEX
  category: 'material' | 'package' | 'macro';
  unit: string;
  businessTag: string;
  sortOrder: number;
}

export interface FuturesBar {
  date: string;            // ISO date (后端用 time.Time)
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
  openInterest: number;
}

export interface FuturesQuote extends FuturesSymbol {
  tradeDate: string;
  close: number;
  prevClose: number;
  change: number;
  changePct: number;
  high: number;
  low: number;
  open: number;
  volume: number;
  openInterest: number;
  miniTrend: number[];     // 最近 30 天收盘价
}

export const categoryLabel: Record<FuturesSymbol['category'], string> = {
  material: '主要原料',
  package: '包材原料',
  macro: '大宗商品',
};

export const categoryColor: Record<FuturesSymbol['category'], string> = {
  material: '#10b981',
  package: '#6366f1',
  macro: '#f59e0b',
};

export const exchangeLabel: Record<string, string> = {
  DCE: '大商所',
  CZCE: '郑商所',
  SHFE: '上期所',
  INE: '上期能源',
  CFFEX: '中金所',
};

// 涨跌色：跟 A 股习惯一致，红涨绿跌（跟欧美相反）
export const upColor = '#dc2626';
export const downColor = '#059669';
export const flatColor = '#94a3b8';

export const trendColor = (change: number): string => {
  if (change > 0) return upColor;
  if (change < 0) return downColor;
  return flatColor;
};
