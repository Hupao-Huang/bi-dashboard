import {
  clampWidth,
  reorder,
  reconcileOrder,
  loadLayout,
  saveLayout,
  MIN_COL_WIDTH,
  TableLayout,
} from './voucherTableLayout';

describe('clampWidth', () => {
  it('小于最小值返回最小值', () => {
    expect(clampWidth(10)).toBe(MIN_COL_WIDTH);
  });
  it('正常值取整返回', () => {
    expect(clampWidth(123.6)).toBe(124);
  });
  it('NaN 返回最小值', () => {
    expect(clampWidth(NaN)).toBe(MIN_COL_WIDTH);
  });
});

describe('reorder', () => {
  it('从前移到后', () => {
    expect(reorder(['a', 'b', 'c', 'd'], 0, 2)).toEqual(['b', 'c', 'a', 'd']);
  });
  it('从后移到前', () => {
    expect(reorder(['a', 'b', 'c', 'd'], 3, 1)).toEqual(['a', 'd', 'b', 'c']);
  });
  it('同位置不变', () => {
    expect(reorder(['a', 'b', 'c'], 1, 1)).toEqual(['a', 'b', 'c']);
  });
  it('越界返回副本不崩', () => {
    expect(reorder(['a', 'b'], 0, 5)).toEqual(['a', 'b']);
  });
  it('不改原数组', () => {
    const o = ['a', 'b', 'c'];
    reorder(o, 0, 2);
    expect(o).toEqual(['a', 'b', 'c']);
  });
});

describe('reconcileOrder', () => {
  it('保留存在的 key 原顺序', () => {
    expect(reconcileOrder(['b', 'a', 'c'], ['a', 'b', 'c'])).toEqual(['b', 'a', 'c']);
  });
  it('新增列追加到末尾', () => {
    expect(reconcileOrder(['b', 'a'], ['a', 'b', 'c'])).toEqual(['b', 'a', 'c']);
  });
  it('废弃列剔除', () => {
    expect(reconcileOrder(['b', 'x', 'a'], ['a', 'b'])).toEqual(['b', 'a']);
  });
});

describe('loadLayout / saveLayout', () => {
  const KEYS = ['账簿', '账期', '摘要'];
  const DEF = { 账簿: 160, 账期: 90, 摘要: 300 };

  const mkStore = (init?: Record<string, string>): Storage => {
    const m: Record<string, string> = { ...(init || {}) };
    return {
      getItem: (k: string) => (k in m ? m[k] : null),
      setItem: (k: string, v: string) => {
        m[k] = v;
      },
      removeItem: (k: string) => {
        delete m[k];
      },
      clear: () => {
        Object.keys(m).forEach((k) => delete m[k]);
      },
      key: (i: number) => Object.keys(m)[i] ?? null,
      get length() {
        return Object.keys(m).length;
      },
    } as Storage;
  };

  it('空存储返回默认', () => {
    const s = mkStore();
    expect(loadLayout('k', KEYS, DEF, s)).toEqual({ order: KEYS, widths: DEF });
  });

  it('round-trip 存取一致', () => {
    const s = mkStore();
    const layout: TableLayout = {
      order: ['摘要', '账簿', '账期'],
      widths: { 账簿: 200, 账期: 90, 摘要: 400 },
    };
    saveLayout('k', layout, s);
    expect(loadLayout('k', KEYS, DEF, s)).toEqual(layout);
  });

  it('坏 JSON 回默认', () => {
    const s = mkStore({ k: '{bad json' });
    expect(loadLayout('k', KEYS, DEF, s)).toEqual({ order: KEYS, widths: DEF });
  });

  it('列增减时对账：废列剔除 + 新列追加 + 存的宽生效', () => {
    const s = mkStore({
      k: JSON.stringify({ order: ['摘要', '废列', '账簿'], widths: { 摘要: 400, 废列: 99 } }),
    });
    const out = loadLayout('k', KEYS, DEF, s);
    expect(out.order).toEqual(['摘要', '账簿', '账期']); // 废列剔除，账期(新)追加末尾
    expect(out.widths).toEqual({ 账簿: 160, 账期: 90, 摘要: 400 }); // 废列宽忽略，摘要存的生效
  });

  it('存储里宽度小于最小值被钳制', () => {
    const s = mkStore({ k: JSON.stringify({ order: KEYS, widths: { 账簿: 10 } }) });
    const out = loadLayout('k', KEYS, DEF, s);
    expect(out.widths.账簿).toBe(MIN_COL_WIDTH);
  });
});
