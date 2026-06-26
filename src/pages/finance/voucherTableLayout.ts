// 凭证表格列布局（列宽 + 列顺序）纯逻辑层 —— 与视图分离便于单测
// 持久化到 localStorage；列定义增减时跟存储对账，防止旧存储错位/留废列
// 注：本文件刻意不 import antd，jest 直接可跑（antd v6 barrel 在 CRA-jest deep-import 解析不了）

export interface TableLayout {
  order: string[]; // 列 key 顺序
  widths: Record<string, number>; // 列 key -> 宽度(px)；缺省的列用默认宽
}

export const MIN_COL_WIDTH = 48; // 列最小宽度，防止拖到看不见

// 把列宽钳制到最小值以上，并取整；非法值退回最小值
export function clampWidth(w: number, min: number = MIN_COL_WIDTH): number {
  if (!Number.isFinite(w)) return min;
  return Math.max(min, Math.round(w));
}

// 把数组元素从 fromIdx 移到 toIdx（列序拖拽用），返回新数组，不改原数组
export function reorder<T>(arr: T[], fromIdx: number, toIdx: number): T[] {
  const next = arr.slice();
  if (
    fromIdx === toIdx ||
    fromIdx < 0 || toIdx < 0 ||
    fromIdx >= arr.length || toIdx >= arr.length
  ) {
    return next;
  }
  const [moved] = next.splice(fromIdx, 1);
  next.splice(toIdx, 0, moved);
  return next;
}

// 跟当前列 keys 对账：保留存储里仍存在的 key（原顺序），新增 key 追加末尾，废弃 key 剔除
export function reconcileOrder(savedOrder: string[], currentKeys: string[]): string[] {
  const cur = new Set(currentKeys);
  const kept = savedOrder.filter((k) => cur.has(k));
  const keptSet = new Set(kept);
  const added = currentKeys.filter((k) => !keptSet.has(k));
  return [...kept, ...added];
}

// 从 localStorage 读布局并跟当前列对账；空/坏数据 → 默认（order=currentKeys，widths=defaults）
// storage 参数便于单测注入；生产传 undefined 用全局 localStorage
export function loadLayout(
  storageKey: string,
  currentKeys: string[],
  defaultWidths: Record<string, number>,
  storage?: Storage,
): TableLayout {
  const store = storage || (typeof localStorage !== 'undefined' ? localStorage : undefined);
  const fallback: TableLayout = { order: currentKeys.slice(), widths: { ...defaultWidths } };
  if (!store) return fallback;

  let raw: string | null = null;
  try {
    raw = store.getItem(storageKey);
  } catch {
    return fallback;
  }
  if (!raw) return fallback;

  let parsed: any;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return fallback;
  }
  if (!parsed || typeof parsed !== 'object') return fallback;

  const savedOrder: string[] = Array.isArray(parsed.order)
    ? parsed.order.filter((k: any) => typeof k === 'string')
    : [];
  const order = reconcileOrder(savedOrder, currentKeys);

  const widths: Record<string, number> = { ...defaultWidths };
  if (parsed.widths && typeof parsed.widths === 'object') {
    for (const k of currentKeys) {
      const w = parsed.widths[k];
      if (typeof w === 'number' && Number.isFinite(w)) widths[k] = clampWidth(w);
    }
  }
  return { order, widths };
}

// 存布局到 localStorage（容错：配额满/隐私模式不抛）
export function saveLayout(storageKey: string, layout: TableLayout, storage?: Storage): void {
  const store = storage || (typeof localStorage !== 'undefined' ? localStorage : undefined);
  if (!store) return;
  try {
    store.setItem(storageKey, JSON.stringify(layout));
  } catch {
    /* 忽略写入失败 */
  }
}
