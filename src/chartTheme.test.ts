// chartTheme.test.ts — 全局 chart 配色 + 工具函数单元测试
// 已 Read chartTheme.ts (line 40-63 工具函数 + line 6-37 颜色常量)

import {
  CHART_COLORS,
  DEPT_COLORS,
  GRADE_COLORS,
  formatMoney,
  formatWanHint,
  getNiceAxisInterval,
} from './chartTheme';

// === formatMoney (source line 40-44) ===
// 源码: |v| >= 1亿 → '亿', |v| >= 1万 → '万' (toFixed(0)), else String(v)

describe('formatMoney', () => {
  it('returns 万 for >= 1万', () => {
    expect(formatMoney(10000)).toBe('1万');
    expect(formatMoney(123456)).toBe('12万'); // toFixed(0) 四舍五入
    expect(formatMoney(99999999)).toBe('10000万');
  });
  it('returns 亿 for >= 1亿', () => {
    expect(formatMoney(100000000)).toBe('1.0亿');
    expect(formatMoney(1234567890)).toBe('12.3亿');
  });
  it('returns raw String for < 1万', () => {
    expect(formatMoney(0)).toBe('0');
    expect(formatMoney(9999)).toBe('9999');
    expect(formatMoney(1.5)).toBe('1.5');
  });
  it('handles negative numbers via Math.abs', () => {
    expect(formatMoney(-15000)).toBe('-2万'); // -15000/10000=-1.5, toFixed(0)='-2'
    expect(formatMoney(-200000000)).toBe('-2.0亿');
  });
});

// === formatWanHint (source line 47-51) ===
// 源码: |v| < 1万 → '', >= 1亿 → '约X.XX亿', else '约X.X万'

describe('formatWanHint', () => {
  it('returns empty for < 1万', () => {
    expect(formatWanHint(0)).toBe('');
    expect(formatWanHint(9999)).toBe('');
    expect(formatWanHint(-9999)).toBe(''); // 绝对值小于 1万
  });
  it('returns 约X.X万 for 1万 <= v < 1亿', () => {
    expect(formatWanHint(10000)).toBe('约1.0万');
    expect(formatWanHint(123456)).toBe('约12.3万');
    expect(formatWanHint(99999999)).toBe('约10000.0万');
  });
  it('returns 约X.XX亿 for >= 1亿 (注意: 源码用 toFixed(2) 不是 toFixed(1))', () => {
    expect(formatWanHint(100000000)).toBe('约1.00亿');
    expect(formatWanHint(1234567890)).toBe('约12.35亿'); // toFixed(2)
  });
});

// === getNiceAxisInterval (source line 53-63) ===
// 源码: rawInterval = max/splits → magnitude = 10^floor(log10) → normalized 分段返 1/2/5/10×magnitude

describe('getNiceAxisInterval', () => {
  it('handles small max (clamped to >= 1)', () => {
    // 源码逐步: safeMax=Math.max(0,1)=1, rawInterval=1/5=0.2, magnitude=10^floor(log10(0.2))=10^-1=0.1
    // normalized=0.2/0.1=2, normalized<=2 → return 2*0.1=0.2
    expect(getNiceAxisInterval(0, 5)).toBeCloseTo(0.2, 5);
  });
  it('returns nice round numbers (1/2/5/10 × magnitude)', () => {
    // max=1000, splits=5, raw=200, mag=100, norm=2 → 200
    expect(getNiceAxisInterval(1000, 5)).toBe(200);
    // max=4500, splits=5, raw=900, mag=100, norm=9 → 1000 (10×mag)
    expect(getNiceAxisInterval(4500, 5)).toBe(1000);
    // max=300, splits=3, raw=100, mag=100, norm=1 → 100
    expect(getNiceAxisInterval(300, 3)).toBe(100);
    // max=12, splits=4, raw=3, mag=1, norm=3 → 5
    expect(getNiceAxisInterval(12, 4)).toBe(5);
  });
  it('always returns positive interval', () => {
    expect(getNiceAxisInterval(100, 5)).toBeGreaterThan(0);
    expect(getNiceAxisInterval(0.5, 5)).toBeGreaterThan(0);
  });
});

// === GRADE_COLORS 完整性 (source line 30-37, memory feedback_kpi_card_no_decoration 守护) ===

describe('GRADE_COLORS', () => {
  it('has S/A/B/C/D and 未设置, all in hex format', () => {
    expect(GRADE_COLORS.S).toBe('#dc2626'); // 跑哥业务: 辣红
    expect(GRADE_COLORS.A).toBe('#f59e0b'); // 金黄
    expect(GRADE_COLORS.B).toBe('#06b6d4'); // 青瓷
    expect(GRADE_COLORS.C).toBe('#059669'); // 翡翠
    expect(GRADE_COLORS.D).toBe('#94a3b8'); // 冷灰
    expect(GRADE_COLORS['未设置']).toBe('#cbd5e1');
  });
  it('all values are 7-char hex (防止改成命名色或破坏统一)', () => {
    Object.values(GRADE_COLORS).forEach((v) => {
      expect(v).toMatch(/^#[0-9a-f]{6}$/i);
    });
  });
});

// === DEPT_COLORS 完整性 (source line 19-26) ===

describe('DEPT_COLORS', () => {
  it('has 5 main departments + instant_retail (v1.02 加)', () => {
    const required = ['ecommerce', 'social', 'offline', 'distribution', 'instant_retail'];
    required.forEach((dept) => {
      expect(DEPT_COLORS[dept]).toBeDefined();
      expect(DEPT_COLORS[dept]).toMatch(/^#[0-9a-f]{6}$/i);
    });
  });
});

// === CHART_COLORS 数组完整 (source line 6-17) ===

describe('CHART_COLORS', () => {
  it('is non-empty array of hex colors (BI 经典 10 色调色盘)', () => {
    expect(Array.isArray(CHART_COLORS)).toBe(true);
    expect(CHART_COLORS.length).toBeGreaterThanOrEqual(10);
    CHART_COLORS.forEach((c) => {
      expect(c).toMatch(/^#[0-9a-f]{6}$/i);
    });
  });
});
