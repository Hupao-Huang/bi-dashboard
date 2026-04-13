import React, { useEffect, useState, useCallback } from 'react';
import { Row, Col, Card, Table, Statistic } from 'antd';
import ReactECharts from './Chart';
import DateFilter from './DateFilter';
import PageLoading from './PageLoading';
import GoodsChannelExpand from './GoodsChannelExpand';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../config';
import { barItemStyle, CHART_COLORS, getBaseOption, pieStyle } from '../chartTheme';

interface Props {
  dept: string;
}

const deptColorMap: Record<string, string> = {
  ecommerce: '#4f46e5',
  social: '#10b981',
  offline: '#faad14',
  distribution: '#8b5cf6',
};

const ProductDashboard: React.FC<Props> = ({ dept }) => {
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);

  const color = deptColorMap[dept] || '#4f46e5';
  const baseOpt = getBaseOption();

  const fetchData = useCallback((s: string, e: string) => {
    setLoading(true);
    fetch(`${API_BASE}/api/department?dept=${dept}&start=${s}&end=${e}`)
      .then(res => res.json())
      .then(res => { setData(res.data); setLoading(false); })
      .catch(() => setLoading(false));
  }, [dept]);

  useEffect(() => { fetchData(startDate, endDate); }, [fetchData, startDate, endDate]);

  const handleDateChange = (s: string, e: string) => {
    setStartDate(s);
    setEndDate(e);
  };

  if (loading) return <PageLoading />;
  if (!data) return <div>加载失败</div>;

  const goods = data.goods || [];
  const brands = data.brands || [];
  const goodsChannels = data.goodsChannels || {};
  const grades = data.grades || [];

  // 商品TOP15柱状图
  const top15 = goods.slice(0, 15);
  const topGoodsOption = {
    ...baseOpt,
    legend: { ...baseOpt.legend, data: ['销售额'], top: 4 },
    grid: { left: 160, right: 80, top: 48, bottom: 20 },
    xAxis: {
      ...baseOpt.xAxis,
      type: 'value' as const,
      axisLabel: {
        ...baseOpt.xAxis.axisLabel,
        formatter: (v: number) => v >= 10000 ? (v / 10000).toFixed(0) + '万' : String(v),
      },
    },
    yAxis: {
      ...baseOpt.yAxis,
      type: 'category' as const,
      data: top15.map((g: any) => g.goodsName || g.goodsNo).reverse(),
      axisLabel: { ...baseOpt.yAxis.axisLabel, width: 140, overflow: 'truncate' as const, fontSize: 11 },
    },
    series: [
      {
        name: '销售额',
        type: 'bar',
        data: top15.map((g: any) => g.sales).reverse(),
        ...barItemStyle(color),
        barWidth: 14,
      },
    ],
  };

  // 品牌分类占比
  const brandPieOption = {
    ...pieStyle,
    color: CHART_COLORS,
    legend: { ...pieStyle.legend, bottom: 0, type: 'scroll' as const },
    series: [{
      type: 'pie',
      radius: ['35%', '65%'],
      label: { show: true, formatter: '{b}\n{d}%', fontSize: 11, lineHeight: 15, color: '#64748b' },
      labelLine: { length: 15, length2: 10, lineStyle: { color: '#cbd5e1' } },
      itemStyle: { borderColor: '#fff', borderWidth: 2, borderRadius: 4 },
      data: brands.map((b: any) => ({ value: b.sales, name: b.brand || '未知' })),
    }],
  };

  // 产品定位分布
  const gradeColors: Record<string, string> = { S: '#f5222d', A: '#fa8c16', B: '#1890ff', C: '#52c41a', D: '#999', '未设置': '#d9d9d9' };
  const gradePieOption = {
    ...pieStyle,
    legend: { ...pieStyle.legend, bottom: 0 },
    series: [{
      type: 'pie',
      radius: ['35%', '65%'],
      label: { show: true, formatter: '{b}\n{d}%', fontSize: 11, lineHeight: 15, color: '#64748b' },
      labelLine: { length: 15, length2: 10, lineStyle: { color: '#cbd5e1' } },
      itemStyle: { borderColor: '#fff', borderWidth: 2, borderRadius: 4 },
      data: grades.map((g: any) => ({ value: g.sales, name: g.grade, itemStyle: { color: gradeColors[g.grade] || '#8c8c8c' } })),
    }],
  };

  // 商品表格
  const indexedGoods = goods.map((g: any, i: number) => ({ ...g, _rank: i + 1 }));
  const goodsColumns = [
    { title: '排名', dataIndex: '_rank', key: 'rank', width: 50 },
    { title: '编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 110 },
    { title: '商品名称', dataIndex: 'goodsName', key: 'goodsName', width: 280, ellipsis: true },
    { title: '分类', dataIndex: 'category', key: 'category', width: 100, ellipsis: true,
      filters: Array.from(new Set(goods.map((g: any) => g.category || '未分类'))).map((c: any) => ({ text: c, value: c })),
      onFilter: (value: any, record: any) => (record.category || '未分类') === value },
    { title: '品牌', dataIndex: 'brand', key: 'brand', width: 80 },
    { title: '产品定位', dataIndex: 'grade', key: 'grade', width: 80,
      filters: ['S', 'A', 'B', 'C', 'D'].map(g => ({ text: g, value: g })).concat([{ text: '未设置', value: '' }]),
      onFilter: (value: any, record: any) => (record.grade || '') === value,
      render: (v: string) => {
        const colors: Record<string, string> = { S: '#f5222d', A: '#fa8c16', B: '#1890ff', C: '#52c41a', D: '#999' };
        return v ? <span style={{ color: colors[v] || '#333', fontWeight: 600 }}>{v}</span> : <span style={{ color: '#ccc' }}>-</span>;
      }
    },
    { title: '销售额', dataIndex: 'sales', key: 'sales', width: 110, render: (v: number) => `¥${v?.toLocaleString()}` },
    { title: '销量', dataIndex: 'qty', key: 'qty', width: 70, render: (v: number) => v?.toLocaleString() },
    { title: '客单价', key: 'avgPrice', width: 100, render: (_: any, row: any) => row.qty > 0 ? `¥${(row.sales / row.qty).toFixed(2)}` : '-' },
  ];

  const totalSales = goods.reduce((s: number, g: any) => s + (g.sales || 0), 0);
  const totalQty = goods.reduce((s: number, g: any) => s + (g.qty || 0), 0);
  const avgOrderValue = totalQty > 0 ? totalSales / totalQty : 0;
  const statCards = [
    { title: '总销售额', value: totalSales, precision: 2, prefix: '¥', accentColor: color },
    { title: '总货品数', value: totalQty, precision: 0, prefix: '', accentColor: '#10b981' },
    { title: '综合客单价', value: avgOrderValue, precision: 2, prefix: '¥', accentColor: '#8b5cf6' },
    { title: '商品种类(SKU)', value: goods.length, precision: 0, suffix: '种', accentColor: '#f59e0b' },
  ];

  return (
    <div>
      <DateFilter start={startDate} end={endDate} onChange={handleDateChange} />

      {/* 汇总统计 */}
      <Row gutter={[16, 16]}>
        {statCards.map((card) => (
          <Col xs={24} sm={6} key={card.title}>
            <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
              <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
            </Card>
          </Col>
        ))}
      </Row>

      {/* 商品TOP15 + 品牌占比 + 产品定位 */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} xl={12}>
          <Card title="商品销售额 TOP15">
            <ReactECharts option={topGoodsOption} lazyUpdate={true} style={{ height: 400 }} />
          </Card>
        </Col>
        <Col xs={24} sm={12} xl={6}>
          <Card title="品牌销售占比">
            <ReactECharts option={brandPieOption} lazyUpdate={true} style={{ height: 400 }} />
          </Card>
        </Col>
        <Col xs={24} sm={12} xl={6}>
          <Card title="产品定位分布">
            <ReactECharts option={gradePieOption} lazyUpdate={true} style={{ height: 400 }} />
          </Card>
        </Col>
      </Row>

      {/* 商品明细表 */}
      <Card title="货品销售明细（点击展开查看渠道分布）" style={{ marginTop: 16 }}>
        <Table
          dataSource={indexedGoods}
          columns={goodsColumns}
          rowKey="goodsNo"
          pagination={{ pageSize: 20, showSizeChanger: true }}
          size="small"
          expandable={{
            expandedRowRender: (record: any) => {
              const channels: any[] = goodsChannels[record.goodsNo] || [];
              return <GoodsChannelExpand channels={channels} />;
            },
            rowExpandable: (record: any) => (goodsChannels[record.goodsNo] || []).length > 0,
          }}
        />
      </Card>
    </div>
  );
};

export default ProductDashboard;
