import React from 'react';
import { Row, Col, Card, Statistic, Table } from 'antd';
import ReactECharts from './Chart';

const formatDate = (d: string) => {
  if (!d) return '';
  const parts = d.split('-');
  return parts.length >= 3 ? `${parts[1]}-${parts[2].slice(0, 2)}` : d;
};
const fmtMoney = (v: number) => v >= 10000 ? (v / 10000).toFixed(1) + '万' : v.toLocaleString();

interface StoreJdSectionProps {
  jdOps: any;
  inSelectedRange: (d: string) => boolean;
  isExpanded: boolean;
}

const StoreJdSection: React.FC<StoreJdSectionProps> = ({ jdOps, inSelectedRange, isExpanded }) => {
  const jdShop = jdOps?.shop || [];
  const jdCustomer = jdOps?.customer || [];
  const customerTypes = jdOps?.customerTypes || [];
  const keywords = jdOps?.keywords || [];
  const promos = jdOps?.promos || [];
  const promoSkus = jdOps?.promoSkus || [];

  const hasShopData = jdShop.length > 0 || jdCustomer.length > 0;
  const hasOtherData = customerTypes.length > 0 || keywords.length > 0 || promos.length > 0;
  if (!hasShopData && !hasOtherData) return null;

  // 店铺经营汇总
  const shopInRange = jdShop.filter((d: any) => inSelectedRange(d.date));
  const shopSum = {
    visitors: shopInRange.reduce((s: number, d: any) => s + d.visitors, 0),
    pageViews: shopInRange.reduce((s: number, d: any) => s + d.pageViews, 0),
    payCustomers: shopInRange.reduce((s: number, d: any) => s + d.payCustomers, 0),
    payAmount: shopInRange.reduce((s: number, d: any) => s + d.payAmount, 0),
    payCount: shopInRange.reduce((s: number, d: any) => s + d.payCount, 0),
    avgConvRate: shopInRange.length > 0 ? shopInRange.reduce((s: number, d: any) => s + d.convRate, 0) / shopInRange.length : 0,
    avgUnitPrice: shopInRange.length > 0 ? shopInRange.reduce((s: number, d: any) => s + d.unitPrice, 0) / shopInRange.length : 0,
    avgUvValue: shopInRange.length > 0 ? shopInRange.reduce((s: number, d: any) => s + d.uvValue, 0) / shopInRange.length : 0,
  };

  // 客户分析汇总
  const custInRange = jdCustomer.filter((d: any) => inSelectedRange(d.date));
  const custSum = {
    browse: custInRange.reduce((s: number, d: any) => s + d.browseCustomers, 0),
    cart: custInRange.reduce((s: number, d: any) => s + d.cartCustomers, 0),
    order: custInRange.reduce((s: number, d: any) => s + d.orderCustomers, 0),
    pay: custInRange.reduce((s: number, d: any) => s + d.payCustomers, 0),
    repurchase: custInRange.reduce((s: number, d: any) => s + d.repurchaseCustomers, 0),
    lost: custInRange.reduce((s: number, d: any) => s + d.lostCustomers, 0),
  };

  // 店铺经营趋势图
  const jdShopOption = jdShop.length > 0 ? {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['成交金额', '访客数', '转化率'], top: 0 },
    grid: { left: 80, right: 80, top: 40, bottom: 40 },
    xAxis: { type: 'category' as const, data: jdShop.map((d: any) => formatDate(d.date)),
      axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: jdShop.length > 15 ? 45 : 0 } },
    yAxis: [
      { type: 'value' as const, name: '金额/人数', min: 0, axisLabel: { formatter: (v: number) => fmtMoney(v) } },
      { type: 'value' as const, name: '转化率%', min: 0, position: 'right' as const, axisLabel: { formatter: '{value}%' } },
    ],
    series: [
      { name: '成交金额', type: 'bar', barWidth: 8,
        data: jdShop.map((d: any) => ({ value: d.payAmount, itemStyle: { color: isExpanded && !inSelectedRange(d.date) ? 'rgba(245,34,45,0.25)' : '#f5222d' } })) },
      { name: '访客数', type: 'line', data: jdShop.map((d: any) => d.visitors), itemStyle: { color: '#1e40af' } },
      { name: '转化率', type: 'line', yAxisIndex: 1, smooth: true, data: jdShop.map((d: any) => d.convRate.toFixed(2)),
        itemStyle: { color: '#faad14' }, lineStyle: { type: 'dashed' as const } },
    ],
  } : null;

  // 客户分析趋势图
  const jdCustOption = jdCustomer.length > 0 ? {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['浏览客户', '加购客户', '成交客户', '复购客户'], top: 0 },
    grid: { left: 80, right: 80, top: 40, bottom: 40 },
    xAxis: { type: 'category' as const, data: jdCustomer.map((d: any) => formatDate(d.date)),
      axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: jdCustomer.length > 15 ? 45 : 0 } },
    yAxis: [{ type: 'value' as const, name: '人数', min: 0 }],
    series: [
      { name: '浏览客户', type: 'bar', barWidth: 8,
        data: jdCustomer.map((d: any) => ({ value: d.browseCustomers, itemStyle: { color: isExpanded && !inSelectedRange(d.date) ? 'rgba(24,144,255,0.25)' : '#1e40af' } })) },
      { name: '加购客户', type: 'line', data: jdCustomer.map((d: any) => d.cartCustomers), itemStyle: { color: '#faad14' } },
      { name: '成交客户', type: 'line', data: jdCustomer.map((d: any) => d.payCustomers), itemStyle: { color: '#f5222d' } },
      { name: '复购客户', type: 'line', data: jdCustomer.map((d: any) => d.repurchaseCustomers), itemStyle: { color: '#10b981' } },
    ],
  } : null;

  return (
    <>
      {hasShopData && (
        <>
          {jdShop.length > 0 && (
            <Card title="店铺经营（京东）" style={{ marginBottom: 16 }}
              styles={{ header: { background: 'linear-gradient(90deg, #fff1f0 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
              <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
                {[
                  { title: '访客数', value: shopSum.visitors, accentColor: '#1e40af' },
                  { title: '浏览量', value: shopSum.pageViews, accentColor: '#06b6d4' },
                  { title: '成交客户', value: shopSum.payCustomers, accentColor: '#ef4444' },
                  { title: '成交金额', value: shopSum.payAmount, precision: 2, prefix: '¥', accentColor: '#dc2626' },
                  { title: '成交单量', value: shopSum.payCount, accentColor: '#10b981' },
                  { title: '转化率', value: shopSum.avgConvRate, precision: 2, suffix: '%', accentColor: '#f59e0b' },
                  { title: '客单价', value: shopSum.avgUnitPrice, precision: 2, prefix: '¥', accentColor: '#7c3aed' },
                  { title: 'UV价值', value: shopSum.avgUvValue, precision: 2, prefix: '¥', accentColor: '#14b8a6' },
                ].map((card: any) => (
                  <Col xs={12} sm={3} key={card.title}>
                    <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                      <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
                      <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>{card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}</div>
                    </Card>
                  </Col>
                ))}
              </Row>
              {jdShopOption && <ReactECharts lazyUpdate={true} option={jdShopOption} style={{ height: 300 }} />}
            </Card>
          )}
          {jdCustomer.length > 0 && (
            <Card title="客户分析（京东）" style={{ marginBottom: 16 }}
              styles={{ header: { background: 'linear-gradient(90deg, #e6f7ff 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
              <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
                {[
                  { title: '浏览客户', value: custSum.browse, accentColor: '#1e40af' },
                  { title: '加购客户', value: custSum.cart, accentColor: '#f59e0b' },
                  { title: '成交客户', value: custSum.pay, accentColor: '#ef4444' },
                  { title: '复购客户', value: custSum.repurchase, accentColor: '#10b981' },
                  { title: '流失客户', value: custSum.lost, accentColor: '#94a3b8' },
                ].map((card: any) => (
                  <Col xs={12} sm={4} key={card.title}>
                    <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                      <Statistic title={card.title} value={card.value} />
                      <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>{card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}</div>
                    </Card>
                  </Col>
                ))}
              </Row>
              {jdCustOption && <ReactECharts lazyUpdate={true} option={jdCustOption} style={{ height: 300 }} />}
            </Card>
          )}
        </>
      )}

      {hasOtherData && (
        <>
          {customerTypes.length > 0 && (
            <Card className="bi-table-card" title="新老客分析" style={{ marginBottom: 16 }}
              styles={{ header: { background: 'linear-gradient(90deg, #fef3c7 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
              <Table dataSource={(() => {
                const grouped: Record<string, any> = {};
                customerTypes.forEach((c: any) => {
                  if (!grouped[c.customerType]) grouped[c.customerType] = { type: c.customerType, total: 0, pct: 0, conv: 0, price: 0, cnt: 0 };
                  grouped[c.customerType].total += c.payCustomers;
                  grouped[c.customerType].pct = c.payPct;
                  grouped[c.customerType].conv = c.convRate;
                  grouped[c.customerType].price += c.unitPrice;
                  grouped[c.customerType].cnt += 1;
                });
                return Object.values(grouped).map((g: any) => ({ ...g, price: g.cnt > 0 ? +(g.price / g.cnt).toFixed(2) : 0 }));
              })()} rowKey="type" size="small" pagination={false}
                columns={[
                  { title: '客户类型', dataIndex: 'type', key: 'type' },
                  { title: '支付人数', dataIndex: 'total', key: 'total', sorter: (a: any, b: any) => a.total - b.total },
                  { title: '占比', dataIndex: 'pct', key: 'pct', render: (v: number) => `${v}%` },
                  { title: '转化率', dataIndex: 'conv', key: 'conv', render: (v: number) => `${v}%` },
                  { title: '客单价', dataIndex: 'price', key: 'price', render: (v: number) => `¥${v}` },
                ]}
              />
            </Card>
          )}
          {keywords.length > 0 && (
            <Card className="bi-table-card" title="行业热词TOP20" style={{ marginBottom: 16 }}
              styles={{ header: { background: 'linear-gradient(90deg, #ecfdf5 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
              <Table dataSource={keywords} rowKey="keyword" size="small" pagination={false} scroll={{ x: 600 }}
                columns={[
                  { title: '关键词', dataIndex: 'keyword', key: 'kw', ellipsis: true, width: 150 },
                  { title: '搜索排名', dataIndex: 'searchRank', key: 'sr' },
                  { title: '竞争排名', dataIndex: 'competeRank', key: 'cr' },
                  { title: '点击排名', dataIndex: 'clickRank', key: 'clk' },
                  { title: '成交金额区间', dataIndex: 'payAmountRange', key: 'par', ellipsis: true },
                  { title: 'TOP品牌', dataIndex: 'topBrand', key: 'tb', ellipsis: true },
                ]}
              />
            </Card>
          )}
          <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
            {promos.length > 0 && (
              <Col xs={24} lg={12}>
                <Card className="bi-table-card" title="促销活动汇总" styles={{ header: { fontWeight: 600, fontSize: 15 } }}>
                  <Table dataSource={promos} rowKey="promoType" size="small" pagination={false}
                    columns={[
                      { title: '活动类型', dataIndex: 'promoType', key: 'type' },
                      { title: '支付金额', dataIndex: 'payAmount', key: 'amt', render: (v: number) => `¥${v?.toLocaleString()}`, sorter: (a: any, b: any) => a.payAmount - b.payAmount },
                      { title: '支付人数', dataIndex: 'payUsers', key: 'users' },
                      { title: '转化率', dataIndex: 'convRate', key: 'conv', render: (v: number) => `${v}%` },
                      { title: 'UV', dataIndex: 'uv', key: 'uv' },
                    ]}
                  />
                </Card>
              </Col>
            )}
            {promoSkus.length > 0 && (
              <Col xs={24} lg={12}>
                <Card className="bi-table-card" title="促销商品TOP10" styles={{ header: { fontWeight: 600, fontSize: 15 } }}>
                  <Table dataSource={promoSkus} rowKey={(r: any) => `${r.goodsName}-${r.promoType}`} size="small" pagination={false}
                    columns={[
                      { title: '商品', dataIndex: 'goodsName', key: 'name', ellipsis: true, width: 150 },
                      { title: '活动', dataIndex: 'promoType', key: 'type' },
                      { title: '支付金额', dataIndex: 'payAmount', key: 'amt', render: (v: number) => `¥${v?.toLocaleString()}`, sorter: (a: any, b: any) => a.payAmount - b.payAmount },
                      { title: '支付人数', dataIndex: 'payUsers', key: 'users' },
                      { title: 'UV', dataIndex: 'uv', key: 'uv' },
                    ]}
                  />
                </Card>
              </Col>
            )}
          </Row>
        </>
      )}
    </>
  );
};

export default StoreJdSection;
