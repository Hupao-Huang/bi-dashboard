import { pct, kg1, int0, num0, perOrderStr, isSummaryChannel } from './salesDailyReportColumns';

test('pct 占比一位小数', () => {
  expect(pct(0.25)).toBe('25.0%');
  expect(pct(0)).toBe('0.0%');
  expect(pct(1)).toBe('100.0%');
});

test('kg1 重量一位小数', () => {
  expect(kg1(1.341)).toBe('1.3');
  expect(kg1(0)).toBe('0.0');
});

test('int0 四舍五入整数', () => {
  expect(int0(49.52)).toBe('50');
  expect(int0(2.99)).toBe('3');
});

test('num0 千分位整数', () => {
  expect(num0(1660200)).toBe('1,660,200');
  expect(num0(0)).toBe('0');
});

test('perOrderStr 单件比(单瓶÷单数)', () => {
  expect(perOrderStr(300, 100)).toBe('3.00'); // 每单3瓶
  expect(perOrderStr(5, 0)).toBe('0.00'); // 单数0不炸
  expect(perOrderStr(6900.5, 7196, 1)).toBe('1.0'); // 单均重量一位小数
});

test('isSummaryChannel 合计行判定', () => {
  expect(isSummaryChannel('总计')).toBe(true);
  expect(isSummaryChannel('社媒合计')).toBe(true);
  expect(isSummaryChannel('抖音')).toBe(false);
});
