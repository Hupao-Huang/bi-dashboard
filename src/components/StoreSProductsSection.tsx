import React from 'react';
import { Row, Col, Card, Statistic, Table } from 'antd';
import ReactECharts from './Chart';
import GoodsChannelExpand from './GoodsChannelExpand';

const formatDate = (d: string) => {
  if (!d) return '';
  const parts = d.split('-');
  return parts.length >= 3 ? `${parts[1]}-${parts[2].slice(0, 2)}` : d;
};
const fmtMoney = (v: number) => v >= 10000 ? (v / 10000).toFixed(1) + '万' : v.toLocaleString();

interface StoreSProductsSectionProps {
  sProducts: any;
  dept: string;
  platform: string;
  isAllShops: boolean;
  inSelectedRange: (d: string) => boolean;
  isExpanded: boolean;
}

const StoreSProductsSection: React.FC<StoreSProductsSectionProps> = ({ sProducts, dept, platform, isAllShops, inSelectedRange, isExpanded }) => {
  if (dept !== 'ecommerce' || !sProducts) return null;
  const shopRank = sProducts?.shopRank || [];
  const goodsRank = sProducts?.goodsRank || [];
  const sTrend = sProducts?.trend || [];
  const details = sProducts?.details || [];
  if (shopRank.length === 0 && goodsRank.length === 0) return null;

  const totalSales = goodsRank.reduce((s: number, g: any) => s + g.sales, 0);
  const totalQty = goodsRank.reduce((s: number, g: any) => s + g.qty, 0);
  const skuCount = goodsRank.length;

  const shopChartData = shopRank.slice(0, 15);
  const pieColors = ['#f59e0b', '#7c3aed', '#10b981', '#f43f5e', '#3b82f6', '#06b6d4', '#be123c', '#84cc16'];
  const shopBarOption = shopChartData.length > 0 ? {
    tooltip: { trigger: 'item' as const, formatter: '{b}: ¥{c} ({d}%)' },
    legend: { bottom: 0, type: 'scroll' as const },
    series: [{
      type: 'pie', radius: ['30%', '60%'], center: ['50%', '45%'],
      label: { show: true, formatter: '{b}\n{d}%', fontSize: 11 },
      data: shopChartData.map((s: any, i: number) => ({
        name: s.shopName, value: s.sales,
        itemStyle: { color: pieColors[i % pieColors.length] },
      })),
    }],
  } : null;

  const sTrendOption = sTrend.length > 0 ? {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['销售额', '销量'], top: 0 },
    grid: { left: 80, right: 80, top: 40, bottom: 40 },
    xAxis: {
      type: 'category' as const, data: sTrend.map((t: any) => formatDate(t.date)),
      axisTick: { alignWithLabel: true },
      axisLabel: { fontSize: 11, interval: 0, rotate: sTrend.length > 15 ? 45 : 0 },
    },
    yAxis: (() => {
      const maxSales = Math.max(...sTrend.map((t: any) => t.sales), 1);
      const maxQty = Math.max(...sTrend.map((t: any) => t.qty), 1);
      const salesInterval = Math.ceil(maxSales / 5 / 1000) * 1000;
      const qtyInterval = Math.ceil(maxQty / 5 / 100) * 100;
      return [
        { type: 'value' as const, name: '金额', min: 0, max: salesInterval * 5, interval: salesInterval, axisLabel: { formatter: (v: number) => fmtMoney(v) } },
        { type: 'value' as const, name: '销量', min: 0, max: qtyInterval * 5, interval: qtyInterval, position: 'right' as const },
      ];
    })(),
    series: [
      { name: '销售额', type: 'bar', barWidth: 8,
        data: sTrend.map((t: any) => ({ value: t.sales, itemStyle: { color: isExpanded && !inSelectedRange(t.date) ? 'rgba(245,158,11,0.25)' : '#f59e0b' } })) },
      { name: '销量', type: 'line', yAxisIndex: 1, data: sTrend.map((t: any) => t.qty), itemStyle: { color: '#7c3aed' } },
    ],
  } : null;

  const goodsDetailMap: Record<string, any[]> = {};
  details.forEach((d: any) => {
    if (!goodsDetailMap[d.goodsName]) goodsDetailMap[d.goodsName] = [];
    goodsDetailMap[d.goodsName].push(d);
  });

  return (
    <>
      <Card title="S品渠道销售分析" style={{ marginBottom: 16 }}
        styles={{ header: { background: 'linear-gradient(90deg, #fffbeb 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
        <Row gutter={16} style={{ marginBottom: 16 }}>
          {[
            { title: 'S品总销售额', value: totalSales, precision: 2, prefix: '¥', accentColor: '#1e40af' },
            { title: 'S品总销量', value: totalQty, accentColor: '#10b981' },
            { title: 'S品SKU数', value: skuCount, accentColor: '#7c3aed' },
          ].map((card: any) => (
            <Col span={8} key={card.title}>
              <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} />
                <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>
                  {card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}
                </div>
              </Card>
            </Col>
          ))}
        </Row>
        {sTrendOption && <ReactECharts lazyUpdate={true} option={sTrendOption} style={{ height: 300 }} />}
      </Card>

      <Row gutter={16}>
        {isAllShops && shopBarOption && (
          <Col span={10}>
            <Card title={platform === 'all' || !platform ? 'S品平台排名' : 'S品渠道排行'} style={{ marginBottom: 16 }}
              styles={{ header: { background: 'linear-gradient(90deg, #fffbeb 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
              <ReactECharts lazyUpdate={true} option={shopBarOption} style={{ height: 320 }} />
            </Card>
          </Col>
        )}
        {goodsRank.length > 0 && (
          <Col span={isAllShops && shopBarOption ? 14 : 24}>
            <Card className="bi-table-card" title={`S品单品排行（点击展开查看${platform === 'all' || !platform ? '平台' : '渠道'}分布）`} style={{ marginBottom: 16 }}
              styles={{ header: { background: 'linear-gradient(90deg, #fffbeb 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
              <Table dataSource={goodsRank} pagination={false} size="small" rowKey="goodsNo"
                expandable={{
                  expandedRowRender: (record: any) => {
                    const shopList = goodsDetailMap[record.goodsName] || [];
                    return <GoodsChannelExpand channels={shopList} />;
                  },
                  rowExpandable: (record: any) => (goodsDetailMap[record.goodsName] || []).length > 0,
                }}
                columns={[
                  { title: '商品名称', dataIndex: 'goodsName', ellipsis: true },
                  { title: '销售额', dataIndex: 'sales', width: 140, render: (v: number) => '¥' + v.toLocaleString(), align: 'right' as const, sorter: (a: any, b: any) => a.sales - b.sales, defaultSortOrder: 'descend' as const },
                  { title: '销量', dataIndex: 'qty', width: 100, render: (v: number) => v.toLocaleString(), align: 'right' as const },
                  { title: platform === 'all' || !platform ? '覆盖平台数' : '覆盖渠道数', dataIndex: 'shopCount', width: 110, align: 'right' as const },
                ]}
              />
            </Card>
          </Col>
        )}
      </Row>
    </>
  );
};

export default StoreSProductsSection;
