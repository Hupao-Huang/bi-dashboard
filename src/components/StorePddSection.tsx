import React from 'react';
import { Row, Col, Card, Statistic } from 'antd';
import ReactECharts from './Chart';

const formatDate = (d: string) => {
  if (!d) return '';
  const parts = d.split('-');
  return parts.length >= 3 ? `${parts[1]}-${parts[2].slice(0, 2)}` : d;
};
const fmtMoney = (v: number) => v >= 10000 ? (v / 10000).toFixed(1) + '万' : v.toLocaleString();

interface StorePddSectionProps {
  pddOps: any;
  inSelectedRange: (d: string) => boolean;
  isExpanded: boolean;
}

const StorePddSection: React.FC<StorePddSectionProps> = ({ pddOps, inSelectedRange, isExpanded }) => {
  const pddShop = pddOps?.shop || [];
  const pddGoods = pddOps?.goods || [];
  const pddVideo = pddOps?.video || [];
  const hasData = pddShop.length > 0 || pddGoods.length > 0 || pddVideo.length > 0;
  if (!hasData) return null;

  const shopInRange = pddShop.filter((d: any) => inSelectedRange(d.date));
  const shopSum = {
    payAmount: shopInRange.reduce((s: number, d: any) => s + d.payAmount, 0),
    payCount: shopInRange.reduce((s: number, d: any) => s + d.payCount, 0),
    payOrders: shopInRange.reduce((s: number, d: any) => s + d.payOrders, 0),
    avgConvRate: shopInRange.length > 0 ? shopInRange.reduce((s: number, d: any) => s + d.convRate, 0) / shopInRange.length : 0,
    avgUnitPrice: shopInRange.length > 0 ? shopInRange.reduce((s: number, d: any) => s + d.unitPrice, 0) / shopInRange.length : 0,
  };
  const goodsInRange = pddGoods.filter((d: any) => inSelectedRange(d.date));
  const goodsSum = {
    visitors: goodsInRange.reduce((s: number, d: any) => s + d.goodsVisitors, 0),
    views: goodsInRange.reduce((s: number, d: any) => s + d.goodsViews, 0),
    collect: goodsInRange.reduce((s: number, d: any) => s + d.goodsCollect, 0),
    saleGoods: goodsInRange.length > 0 ? Math.round(goodsInRange.reduce((s: number, d: any) => s + d.saleGoodsCount, 0) / goodsInRange.length) : 0,
  };
  const videoInRange = pddVideo.filter((d: any) => inSelectedRange(d.date));
  const videoSum = {
    gmv: videoInRange.reduce((s: number, d: any) => s + d.totalGmv, 0),
    orders: videoInRange.reduce((s: number, d: any) => s + d.orderCount, 0),
    feedCount: videoInRange.reduce((s: number, d: any) => s + d.feedCount, 0),
    videoViews: videoInRange.reduce((s: number, d: any) => s + d.videoViewCnt, 0),
    goodsClicks: videoInRange.reduce((s: number, d: any) => s + d.goodsClickCnt, 0),
  };

  const pddShopOption = pddShop.length > 0 ? {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['成交金额', '成交订单数', '转化率'], top: 0 },
    grid: { left: 80, right: 80, top: 40, bottom: 40 },
    xAxis: {
      type: 'category' as const, data: pddShop.map((d: any) => formatDate(d.date)),
      axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: pddShop.length > 15 ? 45 : 0 },
    },
    yAxis: [
      { type: 'value' as const, name: '金额', min: 0, axisLabel: { formatter: (v: number) => fmtMoney(v) } },
      { type: 'value' as const, name: '转化率%', min: 0, position: 'right' as const, axisLabel: { formatter: '{value}%' } },
    ],
    series: [
      { name: '成交金额', type: 'bar', barWidth: 8,
        data: pddShop.map((d: any) => ({ value: d.payAmount, itemStyle: { color: isExpanded && !inSelectedRange(d.date) ? 'rgba(245,34,45,0.25)' : '#f5222d' } })) },
      { name: '成交订单数', type: 'line', data: pddShop.map((d: any) => d.payOrders), itemStyle: { color: '#1e40af' } },
      { name: '转化率', type: 'line', yAxisIndex: 1, smooth: true, data: pddShop.map((d: any) => d.convRate.toFixed(2)),
        itemStyle: { color: '#faad14' }, lineStyle: { type: 'dashed' as const } },
    ],
  } : null;

  const pddGoodsOption = pddGoods.length > 0 ? {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['商品访客', '商品浏览', '收藏'], top: 0 },
    grid: { left: 80, right: 80, top: 40, bottom: 40 },
    xAxis: {
      type: 'category' as const, data: pddGoods.map((d: any) => formatDate(d.date)),
      axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: pddGoods.length > 15 ? 45 : 0 },
    },
    yAxis: [{ type: 'value' as const, name: '人数/次', min: 0 }],
    series: [
      { name: '商品访客', type: 'bar', barWidth: 8,
        data: pddGoods.map((d: any) => ({ value: d.goodsVisitors, itemStyle: { color: isExpanded && !inSelectedRange(d.date) ? 'rgba(245,34,45,0.25)' : '#f5222d' } })) },
      { name: '商品浏览', type: 'line', data: pddGoods.map((d: any) => d.goodsViews), itemStyle: { color: '#1e40af' } },
      { name: '收藏', type: 'line', data: pddGoods.map((d: any) => d.goodsCollect), itemStyle: { color: '#10b981' } },
    ],
  } : null;

  const pddVideoOption = pddVideo.length > 0 ? {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['GMV', '播放量', '商品点击'], top: 0 },
    grid: { left: 80, right: 80, top: 40, bottom: 40 },
    xAxis: {
      type: 'category' as const, data: pddVideo.map((d: any) => formatDate(d.date)),
      axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: pddVideo.length > 15 ? 45 : 0 },
    },
    yAxis: [
      { type: 'value' as const, name: '金额', min: 0, axisLabel: { formatter: (v: number) => fmtMoney(v) } },
      { type: 'value' as const, name: '次数', min: 0, position: 'right' as const },
    ],
    series: [
      { name: 'GMV', type: 'bar', barWidth: 8,
        data: pddVideo.map((d: any) => ({ value: d.totalGmv, itemStyle: { color: isExpanded && !inSelectedRange(d.date) ? 'rgba(245,34,45,0.25)' : '#f5222d' } })) },
      { name: '播放量', type: 'line', yAxisIndex: 1, data: pddVideo.map((d: any) => d.videoViewCnt), itemStyle: { color: '#7c3aed' } },
      { name: '商品点击', type: 'line', yAxisIndex: 1, data: pddVideo.map((d: any) => d.goodsClickCnt), itemStyle: { color: '#10b981' } },
    ],
  } : null;

  return (
    <>
      {pddShop.length > 0 && (
        <Card title="店铺经营（拼多多）" style={{ marginBottom: 16 }}
          styles={{ header: { background: 'linear-gradient(90deg, #fff1f0 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
          <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
            {[
              { title: '成交金额', value: shopSum.payAmount, precision: 2, prefix: '¥', accentColor: '#ef4444' },
              { title: '成交件数', value: shopSum.payCount, accentColor: '#10b981' },
              { title: '成交订单数', value: shopSum.payOrders, accentColor: '#06b6d4' },
              { title: '平均转化率', value: shopSum.avgConvRate, precision: 2, suffix: '%', accentColor: '#f59e0b' },
              { title: '平均客单价', value: shopSum.avgUnitPrice, precision: 2, prefix: '¥', accentColor: '#1e40af' },
            ].map((card: any) => (
              <Col xs={12} sm={4} key={card.title}>
                <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                  <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
                  <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>
                    {card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}
                  </div>
                </Card>
              </Col>
            ))}
          </Row>
          {pddShopOption && <ReactECharts lazyUpdate={true} option={pddShopOption} style={{ height: 300 }} />}
        </Card>
      )}
      {pddGoods.length > 0 && (
        <Card title="商品数据（拼多多）" style={{ marginBottom: 16 }}
          styles={{ header: { background: 'linear-gradient(90deg, #fff7e6 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
          <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
            {[
              { title: '商品访客', value: goodsSum.visitors, accentColor: '#ef4444' },
              { title: '商品浏览量', value: goodsSum.views, accentColor: '#1e40af' },
              { title: '收藏用户', value: goodsSum.collect, accentColor: '#10b981' },
              { title: '日均动销商品', value: goodsSum.saleGoods, accentColor: '#7c3aed' },
            ].map((card) => (
              <Col xs={12} sm={4} key={card.title}>
                <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                  <Statistic title={card.title} value={card.value} />
                  <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>
                    {card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}
                  </div>
                </Card>
              </Col>
            ))}
          </Row>
          {pddGoodsOption && <ReactECharts lazyUpdate={true} option={pddGoodsOption} style={{ height: 300 }} />}
        </Card>
      )}
      {pddVideo.length > 0 && (
        <Card title="短视频数据（拼多多）" style={{ marginBottom: 16 }}
          styles={{ header: { background: 'linear-gradient(90deg, #f9f0ff 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
          <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
            {[
              { title: '视频GMV', value: videoSum.gmv, precision: 2, prefix: '¥', accentColor: '#ef4444' },
              { title: '订单数', value: videoSum.orders, accentColor: '#1e40af' },
              { title: '发布作品', value: videoSum.feedCount, accentColor: '#06b6d4' },
              { title: '播放量', value: videoSum.videoViews, accentColor: '#7c3aed' },
              { title: '商品点击', value: videoSum.goodsClicks, accentColor: '#10b981' },
            ].map((card: any) => (
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
          {pddVideoOption && <ReactECharts lazyUpdate={true} option={pddVideoOption} style={{ height: 300 }} />}
        </Card>
      )}
    </>
  );
};

export default StorePddSection;
