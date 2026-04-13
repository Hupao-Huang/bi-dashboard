import { formatMoney } from './chartTheme';

test('formats money by scale', () => {
  expect(formatMoney(1234)).toBe('1234');
  expect(formatMoney(12000)).toBe('1万');
  expect(formatMoney(250000000)).toBe('2.5亿');
});
