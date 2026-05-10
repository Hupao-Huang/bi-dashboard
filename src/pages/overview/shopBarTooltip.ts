// 综合看板"店铺销售额排行 TOP15" hover tooltip 渲染
// 抽出来作为 pure function 方便单元测试 (memory feedback_test_and_verify 铁律)

export type ShopBreakdownGoodsItem = {
  goodsNo: string;
  goodsName: string;
  grade: string;
  sales: number;
};

export type ShopBreakdownCateItem = {
  cateName: string;
  sales: number;
};

export type ShopBreakdownEntry = {
  topGoods: ShopBreakdownGoodsItem[];
  topCates: ShopBreakdownCateItem[];
  totalSales: number;
};

export const GRADE_COLOR_TOOLTIP: Record<string, string> = {
  S: '#dc2626',
  A: '#f59e0b',
  B: '#06b6d4',
  C: '#059669',
  D: '#94a3b8',
};

export function formatShopBarTooltip(
  shopName: string,
  total: number,
  bd?: ShopBreakdownEntry,
): string {
  const head = `<div style="font-weight:600;color:#fff;margin-bottom:4px;">${shopName}</div>` +
    `<div style="margin-bottom:6px;color:#e2e8f0;">销售额 <b style="color:#fff;">¥${total.toLocaleString()}</b></div>`;

  if (!bd) return head;

  const goodsHtml = (bd.topGoods || []).map((g) => {
    const pct = total > 0 ? ((g.sales / total) * 100).toFixed(1) : '0';
    const tag = g.grade
      ? `<span style="display:inline-block;min-width:14px;text-align:center;color:${GRADE_COLOR_TOOLTIP[g.grade] || '#94a3b8'};font-weight:600;margin-right:4px;">${g.grade}</span>`
      : '<span style="display:inline-block;min-width:18px;"></span>';
    return `<div style="font-size:12px;line-height:18px;margin-top:2px;"><div style="color:#e2e8f0;">${tag}${g.goodsName}</div><div style="text-align:right;color:#94a3b8;font-size:11px;">¥${g.sales.toLocaleString()} <span style="color:#cbd5e1;">${pct}%</span></div></div>`;
  }).join('');

  const cateHtml = (bd.topCates || []).map((c) => {
    const pct = total > 0 ? ((c.sales / total) * 100).toFixed(1) : '0';
    return `<div style="display:flex;justify-content:space-between;font-size:12px;line-height:18px;color:#e2e8f0;"><span>${c.cateName}</span><span style="margin-left:8px;color:#cbd5e1;">${pct}%</span></div>`;
  }).join('');

  return head +
    (goodsHtml ? `<div style="font-weight:600;font-size:12px;margin-top:8px;color:#fff;border-top:1px solid rgba(255,255,255,0.12);padding-top:6px;">货品 Top 5</div>${goodsHtml}` : '') +
    (cateHtml ? `<div style="font-weight:600;font-size:12px;margin-top:8px;color:#fff;border-top:1px solid rgba(255,255,255,0.12);padding-top:6px;">分类 Top 5</div>${cateHtml}` : '');
}
