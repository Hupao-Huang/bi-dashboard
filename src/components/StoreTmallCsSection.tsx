import React from 'react';
import { Row, Col, Card, Statistic, Table } from 'antd';
import ReactECharts from './Chart';

const formatDate = (d: string) => {
  if (!d) return '';
  const parts = d.split('-');
  return parts.length >= 3 ? `${parts[1]}-${parts[2].slice(0, 2)}` : d;
};
const fmtMoney = (v: number) => v >= 10000 ? (v / 10000).toFixed(1) + '万' : v.toLocaleString();

interface StoreTmallCsSectionProps {
  tmallcsOps: any;
  inSelectedRange: (d: string) => boolean;
  isExpanded: boolean;
}

const StoreTmallCsSection: React.FC<StoreTmallCsSectionProps> = ({ tmallcsOps, inSelectedRange, isExpanded }) => {
  const business = tmallcsOps?.business || [];
  const tmcsCampaigns = tmallcsOps?.campaigns || [];
  const keywords = tmallcsOps?.keywords || [];
  const ranks = tmallcsOps?.ranks || [];
  if (business.length === 0 && tmcsCampaigns.length === 0 && keywords.length === 0 && ranks.length === 0) return null;

  const bInRange = business.filter((d: any) => inSelectedRange(d.date));
  const bSum = {
    payAmount: bInRange.reduce((s: number, d: any) => s + d.payAmount, 0),
    paySubOrders: bInRange.reduce((s: number, d: any) => s + d.paySubOrders, 0),
    payQty: bInRange.reduce((s: number, d: any) => s + d.payQty, 0),
    payUsers: bInRange.reduce((s: number, d: any) => s + d.payUsers, 0),
    ipvUv: bInRange.reduce((s: number, d: any) => s + d.ipvUv, 0),
    avgPrice: bInRange.length > 0 ? bInRange.reduce((s: number, d: any) => s + d.avgPrice, 0) / bInRange.length : 0,
    avgConvRate: bInRange.length > 0 ? bInRange.reduce((s: number, d: any) => s + d.convRate, 0) / bInRange.length : 0,
  };

  const businessOption = business.length > 0 ? {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['支付金额', '支付用户数', '转化率'], top: 0 },
    grid: { left: 80, right: 80, top: 40, bottom: 40 },
    xAxis: {
      type: 'category' as const, data: business.map((d: any) => formatDate(d.date)),
      axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: business.length > 15 ? 45 : 0 },
    },
    yAxis: [
      { type: 'value' as const, name: '金额', min: 0, axisLabel: { formatter: (v: number) => fmtMoney(v) } },
      { type: 'value' as const, name: '转化率%', min: 0, position: 'right' as const, axisLabel: { formatter: '{value}%' } },
    ],
    series: [
      { name: '支付金额', type: 'bar', barWidth: 8,
        data: business.map((d: any) => ({ value: d.payAmount, itemStyle: { color: isExpanded && !inSelectedRange(d.date) ? 'rgba(19,194,194,0.25)' : '#13c2c2' } })) },
      { name: '支付用户数', type: 'line', data: business.map((d: any) => d.payUsers), itemStyle: { color: '#1e40af' } },
      { name: '转化率', type: 'line', yAxisIndex: 1, smooth: true,
        data: business.map((d: any) => (d.convRate * 100).toFixed(2)),
        itemStyle: { color: '#faad14' }, lineStyle: { type: 'dashed' as const } },
    ],
  } : null;

  const categoryGroups: Record<string, any[]> = {};
  ranks.forEach((r: any) => {
    if (!categoryGroups[r.category]) categoryGroups[r.category] = [];
    if (categoryGroups[r.category].length < 10) categoryGroups[r.category].push(r);
  });

  return (
    <>
      {business.length > 0 && (
        <Card title="经营概况（天猫超市）" style={{ marginBottom: 16 }}
          styles={{ header: { background: 'linear-gradient(90deg, #e6fffb 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
          <Row gutter={16} style={{ marginBottom: 16 }}>
            {[
              { title: '支付金额', value: bSum.payAmount, precision: 2, prefix: '¥', accentColor: '#14b8a6' },
              { title: '支付用户数', value: bSum.payUsers, accentColor: '#1e40af' },
              { title: '支付子订单', value: bSum.paySubOrders, accentColor: '#06b6d4' },
              { title: '支付件数', value: bSum.payQty, accentColor: '#10b981' },
              { title: '客单价', value: bSum.avgPrice, precision: 2, prefix: '¥', accentColor: '#7c3aed' },
              { title: '平均转化率', value: (bSum.avgConvRate * 100).toFixed(2), suffix: '%', accentColor: '#f59e0b' },
            ].map((card: any) => (
              <Col span={4} key={card.title}>
                <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                  <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
                  <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>
                    {card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}
                  </div>
                </Card>
              </Col>
            ))}
          </Row>
          {businessOption && <ReactECharts lazyUpdate={true} option={businessOption} style={{ height: 320 }} />}
        </Card>
      )}

      {keywords.length > 0 && (
        <Card className="bi-table-card" title="行业搜索热词TOP30（天猫超市）" style={{ marginBottom: 16 }}
          styles={{ header: { background: 'linear-gradient(90deg, #e6fffb 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
          <Table dataSource={keywords} pagination={false} size="small" rowKey="keyword"
            columns={[
              { title: '排名', render: (_, __, i) => i + 1, width: 60 },
              { title: '搜索词', dataIndex: 'keyword' },
              { title: '搜索曝光热度', dataIndex: 'searchImpression', render: (v: number) => v.toFixed(2), align: 'right' as const },
              { title: '引导成交热度', dataIndex: 'tradeHeat', render: (v: number) => v.toFixed(2), align: 'right' as const },
              { title: '引导成交规模', dataIndex: 'tradeScale', render: (v: number) => v.toFixed(2), align: 'right' as const },
              { title: '引导转化指数', dataIndex: 'convIndex', render: (v: number) => v.toFixed(2), align: 'right' as const },
              { title: '引导访问热度', dataIndex: 'visitHeat', render: (v: number) => v.toFixed(2), align: 'right' as const },
            ]}
          />
        </Card>
      )}

      {Object.keys(categoryGroups).length > 0 && (
        <Card className="bi-table-card" title="市场品牌排名（天猫超市）" style={{ marginBottom: 16 }}
          styles={{ header: { background: 'linear-gradient(90deg, #e6fffb 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
          {Object.entries(categoryGroups).map(([category, list]) => (
            <div key={category} style={{ marginBottom: 16 }}>
              <div style={{ fontWeight: 600, marginBottom: 8, color: '#13c2c2' }}>{category}</div>
              <Table dataSource={list} pagination={false} size="small" rowKey="brandName"
                columns={[
                  { title: '排名', render: (_, __, i) => i + 1, width: 60 },
                  { title: '品牌', dataIndex: 'brandName' },
                  { title: '成交热度', dataIndex: 'tradeHeat', render: (v: number) => v.toFixed(2), align: 'right' as const },
                  { title: '成交人气', dataIndex: 'tradePopularity', render: (v: number) => v.toFixed(2), align: 'right' as const },
                  { title: '访问热度', dataIndex: 'visitHeat', render: (v: number) => v.toFixed(2), align: 'right' as const },
                  { title: '转化指数', dataIndex: 'convIndex', render: (v: number) => v.toFixed(2), align: 'right' as const },
                  { title: '交易指数', dataIndex: 'tradeIndex', render: (v: number) => v.toFixed(2), align: 'right' as const },
                ]}
              />
            </div>
          ))}
        </Card>
      )}
    </>
  );
};

export default StoreTmallCsSection;
