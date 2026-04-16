import React, { useEffect, useState, useCallback } from 'react';
import { DEPT_COLORS } from '../chartTheme';
import { Row, Col, Card, Table, Statistic, Select } from 'antd';
import ReactECharts from './Chart';
import DateFilter from './DateFilter';
import PageLoading from './PageLoading';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../config';

const deptOptions = [
  { value: 'ecommerce', label: '电商部门' },
  { value: 'social', label: '社媒部门' },
  { value: 'offline', label: '线下部门' },
  { value: 'distribution', label: '分销部门' },
];


const profitRateColor = (rate: number) => {
  if (rate >= 0.5) return '#10b981';
  if (rate >= 0.3) return '#faad14';
  return '#f5222d';
};

const FinanceDepartmentProfit: React.FC = () => {
  const [dept, setDept] = useState('ecommerce');
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);

  const color = DEPT_COLORS[dept] || '#4f46e5';

  const fetchData = useCallback((d: string, s: string, e: string) => {
    setLoading(true);
    fetch(`${API_BASE}/api/department?dept=${d}&start=${s}&end=${e}`)
      .then(res => res.json())
      .then(res => { setData(res.data); setLoading(false); })
      .catch(() => setLoading(false));
  }, []);

  useEffect(() => { fetchData(dept, startDate, endDate); }, [fetchData, dept, startDate, endDate]);

  const handleDateChange = (s: string, e: string) => {
    setStartDate(s);
    setEndDate(e);
  };

  const handleDeptChange = (val: string) => {
    setDept(val);
  };

  if (loading) return <PageLoading />;
  if (!data) return <div>加载失败</div>;

  const daily = data.daily || [];
  const shops = data.shops || [];
  const goods = data.goods || [];

  const totalSales = daily.reduce((s: number, d: any) => s + (d.sales || 0), 0);
  const totalCost = daily.reduce((s: number, d: any) => s + (d.cost || 0), 0);
  const totalProfit = daily.reduce((s: number, d: any) => s + (d.profit || 0), 0);
  const profitRate = totalSales > 0 ? totalProfit / totalSales : 0;
  const statCards = [
    { title: '销售额', value: totalSales, precision: 2, prefix: '¥', accentColor: color },
    { title: '成本', value: totalCost, precision: 2, prefix: '¥', accentColor: '#f97316' },
    { title: '毛利', value: totalProfit, precision: 2, prefix: '¥', accentColor: '#10b981' },
    { title: '毛利率', value: profitRate * 100, precision: 1, suffix: '%', accentColor: profitRateColor(profitRate) },
  ];

  // Daily profit trend: dual Y-axis line chart
  const formatDate = (d: string) => {
    if (!d) return '';
    const m = d.match(/(\d{4}-)?(\d{2}-\d{2})/);
    return m ? m[2] : d.slice(0, 10);
  };

  const trendDates = daily.map((d: any) => formatDate(String(d.date || '').slice(0, 10)));
  const trendOption = {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['销售额', '毛利', '毛利率(%)'], top: 0 },
    grid: { left: 80, right: 70, top: 50, bottom: 40 },
    xAxis: {
      type: 'category' as const,
      data: trendDates,
      axisLabel: { fontSize: 11 },
    },
    yAxis: [
      {
        type: 'value' as const,
        name: '金额',
        axisLabel: { formatter: (v: number) => v >= 10000 ? (v / 10000).toFixed(0) + '万' : String(v) },
      },
      {
        type: 'value' as const,
        name: '毛利率(%)',
        position: 'right' as const,
        min: 0,
        max: 100,
        axisLabel: { formatter: (v: number) => v + '%' },
      },
    ],
    series: [
      {
        name: '销售额',
        type: 'line',
        smooth: true,
        data: daily.map((d: any) => d.sales || 0),
        itemStyle: { color },
      },
      {
        name: '毛利',
        type: 'line',
        smooth: true,
        data: daily.map((d: any) => d.profit || 0),
        itemStyle: { color: '#10b981' },
      },
      {
        name: '毛利率(%)',
        type: 'line',
        smooth: true,
        yAxisIndex: 1,
        data: daily.map((d: any) => {
          const s = d.sales || 0;
          const p = d.profit || 0;
          return s > 0 ? parseFloat((p / s * 100).toFixed(2)) : 0;
        }),
        itemStyle: { color: '#8b5cf6' },
        lineStyle: { type: 'dashed' as const },
      },
    ],
  };

  // Shop profit ranking table
  const shopsSorted = [...shops].sort((a, b) => (b.profit || 0) - (a.profit || 0));
  const indexedShopsSorted = shopsSorted.map((g: any, i: number) => ({ ...g, _rank: i + 1 }));
  const shopColumns = [
    { title: '排名', dataIndex: '_rank', key: 'rank', width: 60 },
    { title: '店铺名称', dataIndex: 'shopName', key: 'shopName', ellipsis: true },
    { title: '销售额', dataIndex: 'sales', key: 'sales', width: 130, render: (v: number) => `¥${(v || 0).toLocaleString()}` },
    { title: '毛利', dataIndex: 'profit', key: 'profit', width: 130, render: (v: number) => `¥${(v || 0).toLocaleString()}` },
    {
      title: '毛利率',
      key: 'profit_rate',
      width: 100,
      render: (_: any, row: any) => {
        const rate = row.sales > 0 ? (row.profit || 0) / row.sales : 0;
        return <span style={{ color: profitRateColor(rate), fontWeight: 600 }}>{(rate * 100).toFixed(1)}%</span>;
      },
    },
  ];

  // Top products by profit
  const goodsSorted = [...goods].sort((a, b) => (b.profit || 0) - (a.profit || 0));
  const indexedGoodsSorted = goodsSorted.map((g: any, i: number) => ({ ...g, _rank: i + 1 }));
  const goodsColumns = [
    { title: '排名', dataIndex: '_rank', key: 'rank', width: 60 },
    { title: '商品编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 110 },
    { title: '商品名称', dataIndex: 'goodsName', key: 'goodsName', ellipsis: true },
    { title: '品牌', dataIndex: 'brand', key: 'brand', width: 90 },
    { title: '销售额', dataIndex: 'sales', key: 'sales', width: 120, render: (v: number) => `¥${(v || 0).toLocaleString()}` },
    { title: '毛利', dataIndex: 'profit', key: 'profit', width: 120, render: (v: number) => `¥${(v || 0).toLocaleString()}` },
    {
      title: '毛利率',
      key: 'profit_rate',
      width: 100,
      render: (_: any, row: any) => {
        const rate = row.sales > 0 ? (row.profit || 0) / row.sales : 0;
        return <span style={{ color: profitRateColor(rate), fontWeight: 600 }}>{(rate * 100).toFixed(1)}%</span>;
      },
    },
  ];

  return (
    <div>
      {/* Department selector */}
      <div style={{ marginBottom: 16, display: 'flex', alignItems: 'center', gap: 12 }}>
        <span style={{ fontWeight: 500 }}>选择部门：</span>
        <Select
          value={dept}
          onChange={handleDeptChange}
          options={deptOptions}
          style={{ width: 160 }}
        />
      </div>

      <DateFilter start={startDate} end={endDate} onChange={handleDateChange} />

      {/* Summary cards */}
      <Row gutter={[16, 16]}>
        {statCards.map((card) => (
          <Col xs={24} sm={6} key={card.title}>
            <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
              <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
            </Card>
          </Col>
        ))}
      </Row>

      {/* Daily trend chart */}
      <Card title="每日利润趋势" style={{ marginTop: 16 }}>
        <ReactECharts lazyUpdate={true} option={trendOption} style={{ height: 380 }} />
      </Card>

      {/* Shop & goods tables */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={12}>
          <Card className="bi-table-card" title="店铺利润排名">
            <Table
              dataSource={indexedShopsSorted}
              columns={shopColumns}
              rowKey="shopName"
              pagination={{ pageSize: 10, showSizeChanger: false }}
              size="small"
            />
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card className="bi-table-card" title="商品利润排名 TOP20">
            <Table
              dataSource={indexedGoodsSorted.slice(0, 20)}
              columns={goodsColumns}
              rowKey="goodsNo"
              pagination={false}
              size="small"
              scroll={{ y: 360 }}
            />
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default FinanceDepartmentProfit;
