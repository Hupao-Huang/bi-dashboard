import React from 'react';
import { Row, Col, Table } from 'antd';
import ReactECharts from './Chart';
import { CHART_COLORS, pieStyle } from '../chartTheme';

function getPlatform(shopName: string): string {
  if (shopName.startsWith('ds-天猫超市')) return '天猫超市';
  if (shopName.startsWith('ds-天猫-')) return '天猫';
  if (shopName.startsWith('ds-京东-')) return '京东';
  if (shopName.startsWith('ds-拼多多-')) return '拼多多';
  if (shopName.startsWith('ds-唯品会-')) return '唯品会';
  if (shopName.startsWith('js-')) return '即时零售';
  if (shopName.includes('抖音')) return '抖音';
  if (shopName.includes('快手')) return '快手';
  if (shopName.includes('小红书')) return '小红书';
  if (shopName.includes('视频小店') || shopName.includes('视频号')) return '视频号';
  if (shopName.includes('有赞')) return '有赞';
  if (shopName.includes('微店')) return '微店';
  if (shopName.includes('微信销售')) return '微信销售';
  if (shopName.includes('分销')) return '分销';
  if (shopName.includes('线下')) return '线下';
  return '其他';
}

function getDepartment(shopName: string): string {
  // v1.02: js- 前缀拆出即时零售部 (跟数据库 department='instant_retail' 对齐)
  if (shopName.startsWith('js-即时零售')) return '即时零售部';
  if (shopName.startsWith('ds-') || shopName.startsWith('js-')) return '电商部门';
  if (shopName.startsWith('社媒-') || shopName.includes('抖音') || shopName.includes('快手') || shopName.includes('小红书') || shopName.includes('视频号') || shopName.includes('有赞') || shopName.includes('微店') || shopName.includes('飞瓜')) return '社媒部门';
  if (shopName.includes('分销')) return '分销部门';
  if (shopName.includes('线下') || shopName.includes('大区')) return '线下部门';
  return '其他';
}

interface Channel {
  shopName: string;
  sales: number;
  qty: number;
}

// mode: 'platform' 按平台聚合（电商/货品看板），'department' 按部门聚合（综合看板）
const GoodsChannelExpand: React.FC<{ channels: Channel[]; mode?: 'platform' | 'department'; hidePlatSection?: boolean }> = ({ channels, mode = 'platform', hidePlatSection = false }) => {
  if (channels.length === 0) return <span style={{ color: '#64748b' }}>暂无渠道数据</span>;
  const totalSales = channels.reduce((s, c) => s + c.sales, 0);

  // 聚合
  const groupFn = mode === 'department' ? getDepartment : getPlatform;
  const groupLabel = mode === 'department' ? '部门' : '平台';
  const platMap: Record<string, { sales: number; qty: number }> = {};
  channels.forEach(c => {
    const key = groupFn(c.shopName || '');
    if (!platMap[key]) platMap[key] = { sales: 0, qty: 0 };
    platMap[key].sales += c.sales;
    platMap[key].qty += c.qty;
  });
  const platData = Object.entries(platMap)
    .map(([name, d]) => ({ name, sales: d.sales, qty: d.qty }))
    .sort((a, b) => b.sales - a.sales);

  return (
    <>
      {!hidePlatSection && <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={12}>
          <div style={{ fontWeight: 600, marginBottom: 8 }}>{groupLabel}分布</div>
          <Table
            dataSource={platData.map((p, i) => ({ ...p, key: i, pct: totalSales > 0 ? (p.sales / totalSales * 100).toFixed(1) : '0' }))}
            columns={[
              { title: groupLabel, dataIndex: 'name', key: 'name' },
              { title: '销售额', dataIndex: 'sales', key: 'sales', width: 110, render: (v: number) => `¥${v?.toLocaleString()}` },
              { title: '销量', dataIndex: 'qty', key: 'qty', width: 70, render: (v: number) => v?.toLocaleString() },
              { title: '占比', dataIndex: 'pct', key: 'pct', width: 80, render: (v: string) => `${v}%` },
            ]}
            pagination={false}
            size="small"
          />
        </Col>
        <Col span={12}>
          <div style={{ fontWeight: 600, marginBottom: 8 }}>&nbsp;</div>
          <ReactECharts
            option={{
              ...pieStyle,
              legend: { ...pieStyle.legend, type: 'scroll' as const },
              series: [{
                type: 'pie', radius: ['30%', '60%'],
                label: { show: true, formatter: '{b}\n{d}%', fontSize: 11, color: '#475569' },
                data: platData.map((p, i) => ({ value: p.sales, name: p.name, itemStyle: { color: CHART_COLORS[i % CHART_COLORS.length] } })),
              }],
            }}
            style={{ height: Math.max(200, platData.length * 28) }}
          />
        </Col>
      </Row>}
      <Row gutter={16}>
        <Col span={12}>
          <div style={{ fontWeight: 600, marginBottom: 8 }}>渠道分布</div>
          <Table
            dataSource={channels.map((c, i) => ({ ...c, key: i, pct: totalSales > 0 ? (c.sales / totalSales * 100).toFixed(1) : '0' }))}
            columns={[
              { title: '渠道/店铺', dataIndex: 'shopName', key: 'shopName', ellipsis: true },
              { title: '销售额', dataIndex: 'sales', key: 'sales', width: 110, render: (v: number) => `¥${v?.toLocaleString()}` },
              { title: '销量', dataIndex: 'qty', key: 'qty', width: 70, render: (v: number) => v?.toLocaleString() },
              { title: '占比', dataIndex: 'pct', key: 'pct', width: 80, render: (v: string) => `${v}%` },
            ]}
            pagination={false}
            size="small"
          />
        </Col>
        <Col span={12}>
          <div style={{ fontWeight: 600, marginBottom: 8 }}>&nbsp;</div>
          <ReactECharts
            option={{
              ...pieStyle,
              color: CHART_COLORS,
              legend: { ...pieStyle.legend, type: 'scroll' as const },
              series: [{
                type: 'pie', radius: ['30%', '60%'],
                label: { show: true, formatter: '{b}\n{d}%', fontSize: 11, color: '#475569' },
                data: channels.map((c) => ({ value: c.sales, name: c.shopName })),
              }],
            }}
            style={{ height: Math.max(200, channels.length * 28) }}
          />
        </Col>
      </Row>
    </>
  );
};

export default GoodsChannelExpand;
