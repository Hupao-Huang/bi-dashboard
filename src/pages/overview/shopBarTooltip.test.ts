import { formatShopBarTooltip, ShopBreakdownEntry } from './shopBarTooltip';

describe('formatShopBarTooltip', () => {
  it('shows only head when breakdown data missing', () => {
    const html = formatShopBarTooltip('测试店铺A', 1234567);
    expect(html).toContain('测试店铺A');
    expect(html).toContain('¥1,234,567');
    expect(html).not.toContain('货品 Top 5');
    expect(html).not.toContain('分类 Top 5');
  });

  it('renders both goods and cate sections with correct percentages and grade colors', () => {
    const bd: ShopBreakdownEntry = {
      totalSales: 1000,
      topGoods: [
        { goodsNo: 'SKU1', goodsName: '商品甲', grade: 'S', sales: 600 },
        { goodsNo: 'SKU2', goodsName: '商品乙', grade: 'A', sales: 300 },
      ],
      topCates: [
        { cateName: '调味料', sales: 700 },
        { cateName: '酱油', sales: 300 },
      ],
    };
    const html = formatShopBarTooltip('店铺B', 1000, bd);

    // 标题段都要有
    expect(html).toContain('店铺B');
    expect(html).toContain('货品 Top 5');
    expect(html).toContain('分类 Top 5');

    // 货品名 + 占比正确
    expect(html).toContain('商品甲');
    expect(html).toContain('60.0%');
    expect(html).toContain('商品乙');
    expect(html).toContain('30.0%');

    // 分类正确
    expect(html).toContain('调味料');
    expect(html).toContain('70.0%');
    expect(html).toContain('酱油');

    // grade 染色 (S=红 #dc2626, A=金黄 #f59e0b)
    expect(html).toContain('#dc2626');
    expect(html).toContain('#f59e0b');

    // 关键: 标题颜色不能是 #0f172a (历史 bug, 跟 tooltip 黑底同色看不见)
    expect(html).not.toContain('color:#0f172a;">货品');
    expect(html).not.toContain('color:#0f172a;">分类');

    // 标题色必须 #fff (白色, 在黑底上可见)
    expect(html).toMatch(/color:#fff;[^"]*">货品 Top 5/);
    expect(html).toMatch(/color:#fff;[^"]*">分类 Top 5/);
  });

  it('handles zero totalSales without divide by zero', () => {
    const bd: ShopBreakdownEntry = {
      totalSales: 0,
      topGoods: [{ goodsNo: 'X', goodsName: '空店货品', grade: 'B', sales: 0 }],
      topCates: [{ cateName: '空分类', sales: 0 }],
    };
    const html = formatShopBarTooltip('零销售店', 0, bd);
    expect(html).toContain('¥0');
    expect(html).toContain('0%'); // 占比 fallback 到 '0' 不是 NaN 不是 Infinity
    expect(html).not.toContain('NaN');
    expect(html).not.toContain('Infinity');
  });

  it('handles missing grade by using fallback gray color', () => {
    const bd: ShopBreakdownEntry = {
      totalSales: 100,
      topGoods: [
        { goodsNo: 'X', goodsName: '无定位货品', grade: '', sales: 100 },
      ],
      topCates: [],
    };
    const html = formatShopBarTooltip('店铺C', 100, bd);
    expect(html).toContain('无定位货品');
    expect(html).toContain('100.0%');
    // grade='' 时不渲染 grade tag, 用占位 span
    expect(html).not.toContain('color:undefined');
  });
});
