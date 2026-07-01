// 销售日报格式化纯函数(无 JSX/依赖, 便于单测)。列定义放在页面 .tsx 里。

// pct 占比(0.25 → "25.0%")
export const pct = (v: number): string => `${((v || 0) * 100).toFixed(1)}%`;
// kg1 重量一位小数
export const kg1 = (v: number): string => (v || 0).toFixed(1);
// int0 四舍五入整数(箱数)
export const int0 = (v: number): string => Math.round(v || 0).toString();
// num0 千分位整数(单瓶数/单数)
export const num0 = (v: number): string => Math.round(v || 0).toLocaleString('en-US');

// perOrderStr 单件比/单均(orders=0 返 "0.00")
export const perOrderStr = (v: number, orders: number, digits = 2): string =>
  orders ? (v / orders).toFixed(digits) : (0).toFixed(digits);

// 当日/当月两组并排(对齐 Excel), 每行含 today + month
export interface ChannelStat { orders: number; bottles: number; weightKg: number; }
export interface ChannelRow { platform: string; channel: string; today: ChannelStat; month: ChannelStat; }

export interface GoodsStat { orders: number; bottles: number; boxes: number; pallets: number; }
export interface GoodsRow { goodsNo: string; goodsName: string; boxQty: number; today: GoodsStat; month: GoodsStat; }

export interface ComboStat { orders: number; bottles: number; weightKg: number; }
export interface ComboRow { display: string; today: ComboStat; month: ComboStat; }

// isSummaryChannel 合计/总计行判定(渲染时加粗)
export const isSummaryChannel = (c: string): boolean => c === '总计' || c.endsWith('合计');
