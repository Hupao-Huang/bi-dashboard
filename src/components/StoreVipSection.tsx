import React from 'react';
import { Row, Col, Card, Statistic } from 'antd';
import ReactECharts from './Chart';

const formatDate = (d: string) => {
  if (!d) return '';
  const parts = d.split('-');
  return parts.length >= 3 ? `${parts[1]}-${parts[2].slice(0, 2)}` : d;
};

interface StoreVipSectionProps {
  vipOps: any;
  inSelectedRange: (d: string) => boolean;
  isExpanded: boolean;
}

const StoreVipSection: React.FC<StoreVipSectionProps> = ({ vipOps, inSelectedRange, isExpanded }) => {
  const vipDaily = vipOps?.daily || [];
  if (!vipDaily.length) return null;

  const vipSummary = {
    impressions: vipDaily.filter((d: any) => inSelectedRange(d.date)).reduce((s: number, d: any) => s + d.impressions, 0),
    detailUv: vipDaily.filter((d: any) => inSelectedRange(d.date)).reduce((s: number, d: any) => s + d.detailUv, 0),
    cartBuyers: vipDaily.filter((d: any) => inSelectedRange(d.date)).reduce((s: number, d: any) => s + d.cartBuyers, 0),
    payAmount: vipDaily.filter((d: any) => inSelectedRange(d.date)).reduce((s: number, d: any) => s + d.payAmount, 0),
    payCount: vipDaily.filter((d: any) => inSelectedRange(d.date)).reduce((s: number, d: any) => s + d.payCount, 0),
    visitors: vipDaily.filter((d: any) => inSelectedRange(d.date)).reduce((s: number, d: any) => s + d.visitors, 0),
  };
  const avgUvValue = vipSummary.detailUv > 0 ? vipSummary.payAmount / vipSummary.detailUv : 0;

  const vipTrafficOption = {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['曝光流量', '商详UV', '加购人数'], top: 0 },
    grid: { left: 80, right: 80, top: 40, bottom: 40 },
    xAxis: {
      type: 'category' as const, data: vipDaily.map((d: any) => formatDate(d.date)),
      axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: vipDaily.length > 15 ? 45 : 0 },
    },
    yAxis: [{ type: 'value' as const, name: '人数', min: 0 }],
    series: [
      {
        name: '曝光流量', type: 'bar', barWidth: 8,
        data: vipDaily.map((d: any) => ({ value: d.impressions, itemStyle: { color: isExpanded && !inSelectedRange(d.date) ? 'rgba(114,46,209,0.25)' : '#7c3aed' } })),
      },
      { name: '商详UV', type: 'line', data: vipDaily.map((d: any) => d.detailUv), itemStyle: { color: '#1e40af' } },
      { name: '加购人数', type: 'line', data: vipDaily.map((d: any) => d.cartBuyers), itemStyle: { color: '#10b981' } },
    ],
  };

  return (
    <Card title="流量转化（唯品会）" style={{ marginBottom: 16 }}
      styles={{ header: { background: 'linear-gradient(90deg, #f9f0ff 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        {[
          { title: '曝光流量', value: vipSummary.impressions, accentColor: '#7c3aed' },
          { title: '商详UV', value: vipSummary.detailUv, accentColor: '#1e40af' },
          { title: '加购人数', value: vipSummary.cartBuyers, accentColor: '#10b981' },
          { title: '销售额', value: vipSummary.payAmount, precision: 2, prefix: '¥', accentColor: '#ef4444' },
          { title: '客户数', value: vipSummary.visitors, accentColor: '#06b6d4' },
          { title: 'UV价值', value: avgUvValue, precision: 2, prefix: '¥', accentColor: '#f59e0b' },
        ].map((card) => (
          <Col xs={12} sm={4} key={card.title}>
            <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
              <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} />
              <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>
                {card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}
              </div>
            </Card>
          </Col>
        ))}
      </Row>
      <ReactECharts lazyUpdate={true} option={vipTrafficOption} style={{ height: 300 }} />
    </Card>
  );
};

export default StoreVipSection;
