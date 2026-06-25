import { filterBooks, toggleCode, mergeAll, triggerLabelOf, AccbookOption } from './accbookPickerLogic';

const BOOKS: AccbookOption[] = [
  { code: '1000', name: '杭州松鲜鲜自然调味品账簿' },
  { code: '100001', name: '苏州松鲜鲜食品科技账簿' },
  { code: '100005', name: '浙江松鲜鲜自然调味品有限公司' },
];

describe('filterBooks', () => {
  test('空关键词返回全部', () => {
    expect(filterBooks(BOOKS, '')).toHaveLength(3);
    expect(filterBooks(BOOKS, '   ')).toHaveLength(3);
  });
  test('按编码过滤', () => {
    const r = filterBooks(BOOKS, '100005');
    expect(r).toHaveLength(1);
    expect(r[0].code).toBe('100005');
  });
  test('按名称过滤(子串)', () => {
    const r = filterBooks(BOOKS, '苏州');
    expect(r).toHaveLength(1);
    expect(r[0].code).toBe('100001');
  });
  test('大小写不敏感 + 多命中', () => {
    // "10000" 同时是 100001 / 100005 的前缀
    expect(filterBooks(BOOKS, '10000')).toHaveLength(2);
  });
  test('无命中返回空', () => {
    expect(filterBooks(BOOKS, '深圳')).toHaveLength(0);
  });
});

describe('toggleCode', () => {
  test('不存在则加入', () => {
    expect(toggleCode(['1000'], '100001')).toEqual(['1000', '100001']);
  });
  test('已存在则移除', () => {
    expect(toggleCode(['1000', '100001'], '1000')).toEqual(['100001']);
  });
});

describe('mergeAll', () => {
  test('并入全部并去重', () => {
    expect(mergeAll(['1000'], BOOKS).sort()).toEqual(['100001', '100005', '1000'].sort());
  });
  test('空已选并入过滤结果', () => {
    const filtered = filterBooks(BOOKS, '10000'); // 100001 + 100005
    expect(mergeAll([], filtered).sort()).toEqual(['100001', '100005'].sort());
  });
});

describe('triggerLabelOf', () => {
  const nameOf = new Map(BOOKS.map((b) => [b.code, b.name]));
  test('空显示 placeholder', () => {
    expect(triggerLabelOf([], nameOf, '选择账簿')).toBe('选择账簿');
  });
  test('1-2个显示名称(顿号分隔)', () => {
    expect(triggerLabelOf(['1000'], nameOf, 'x')).toBe('杭州松鲜鲜自然调味品账簿');
    expect(triggerLabelOf(['1000', '100001'], nameOf, 'x')).toBe(
      '杭州松鲜鲜自然调味品账簿、苏州松鲜鲜食品科技账簿'
    );
  });
  test('3个及以上显示计数', () => {
    expect(triggerLabelOf(['1000', '100001', '100005'], nameOf, 'x')).toBe('已选 3 个账簿');
  });
  test('找不到名称回退编码', () => {
    expect(triggerLabelOf(['999'], nameOf, 'x')).toBe('999');
  });
});
