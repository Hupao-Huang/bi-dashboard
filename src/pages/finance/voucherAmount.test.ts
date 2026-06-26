import { amountToChinese } from './voucherAmount';

describe('amountToChinese', () => {
  it('零', () => expect(amountToChinese(0)).toBe('零元整'));
  it('用友例：肆万贰仟元整', () => expect(amountToChinese(42000)).toBe('肆万贰仟元整'));
  it('带角分', () => expect(amountToChinese(1596.78)).toBe('壹仟伍佰玖拾陆元柒角捌分'));
  it('用友例：陆拾贰万...陆角玖分', () =>
    expect(amountToChinese(623790.69)).toBe('陆拾贰万叁仟柒佰玖拾元陆角玖分'));
  it('整百', () => expect(amountToChinese(100)).toBe('壹佰元整'));
  it('整万', () => expect(amountToChinese(10000)).toBe('壹万元整'));
  it('一亿', () => expect(amountToChinese(100000000)).toBe('壹亿元整'));
  it('有分无角', () => expect(amountToChinese(100.05)).toBe('壹佰元零伍分'));
  it('有角无分', () => expect(amountToChinese(100.5)).toBe('壹佰元伍角'));
  it('中间零', () => expect(amountToChinese(10005)).toBe('壹万零伍元整'));
  it('负数', () => expect(amountToChinese(-42000)).toBe('负肆万贰仟元整'));
  it('浮点防误差（0.1+0.2）', () => expect(amountToChinese(0.1 + 0.2)).toBe('零元叁角'));
  it('非法值返回空串', () => expect(amountToChinese(NaN)).toBe(''));
});
