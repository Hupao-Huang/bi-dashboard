import React, { useEffect, useState, useCallback, useRef } from 'react';
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

const FinanceProductProfit: React.FC = () => {
  const abortRef = useRef<AbortController | null>(null);
  const [dept, setDept] = useState('ecommerce');
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);
  const [sortedInfo, setSortedInfo] = useState<any>({});

  const color = DEPT_COLORS[dept] || '#4f46e5';

  const fetchData = useCallback((d: string, s: string, e: string) => {
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    fetch(`${API_BASE}/api/department?dept=${d}&start=${s}&end=${e}`, { signal: ctrl.signal })
      .then(res => res.json())
      .then(res => { setData(res.data); setLoading(false); })
      .catch((e: any) => { if (e?.name !== 'AbortError') setLoading(false); });
  }, []);

  useEffect(() => { fetchData(dept, startDate, endDate); }, [fetchData, dept, startDate, endDate]);

  const handleDateChange = (s: string, e: string) => {
    setStartDate(s);
    setEndDate(e);
  };

  if (loading) return <PageLoading />;
  if (!data) return <div>加载失败</div>;

  const goods = data.goods || [];

  const totalSales = goods.reduce((s: number, g: any) => s + (g.sales || 0), 0);
  const totalProfit = goods.reduce((s: number, g: any) => s + (g.profit || 0), 0);
  const overallProfitRate = totalSales > 0 ? totalProfit / totalSales : 0;
  const statCards = [
    { title: '总销售额', value: totalSales, precision: 2, prefix: '¥', accentColor: color },
    { title: '总毛利', value: totalProfit, precision: 2, prefix: '¥', accentColor: '#10b981' },
    { title: '综合毛利率', value: overallProfitRate * 100, precision: 1, suffix: '%', accentColor: profitRateColor(overallProfitRate) },
    { title: 'SKU种类数', value: goods.length, suffix: '种', accentColor: '#8b5cf6' },
  ];

  // TOP15 horizontal bar chart by profit
  const top15 = [...goods].sort((a, b) => (b.profit || 0) - (a.profit || 0)).slice(0, 15);
  const top15Names = top15.map(g => g.goodsName || g.goodsNo).reverse();
  const top15Profits = top15.map(g => g.profit || 0).reverse();
  const top15Rates = top15.map(g => g.sales > 0 ? parseFloat(((g.profit || 0) / g.sales * 100).toFixed(2)) : 0).reverse();

  const horizBarOption = {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['毛利', '毛利率(%)'], top: 0 },
    grid: { left: 200, right: 80, top: 40, bottom: 20 },
    xAxis: [
      {
        type: 'value' as const,
        name: '毛利',
        axisLabel: { formatter: (v: number) => v >= 10000 ? (v / 10000).toFixed(0) + '万' : String(v) },
      },
      {
        type: 'value' as const,
        name: '毛利率(%)',
        min: 0,
        max: 100,
        axisLabel: { formatter: (v: number) => v + '%' },
      },
    ],
    yAxis: {
      type: 'category' as const,
      data: top15Names,
    },
    series: [
      {
        name: '毛利',
        type: 'bar',
        data: top15Profits,
        itemStyle: { color },
        barWidth: 14,
        label: { show: true, position: 'right' as const, formatter: (p: any) => p.value >= 10000 ? (p.value / 10000).toFixed(1) + '万' : p.value },
      },
      {
        name: '毛利率(%)',
        type: 'line',
        xAxisIndex: 1,
        data: top15Rates,
        itemStyle: { color: '#8b5cf6' },
      },
    ],
  };

  // Brand profit pie chart
  const brandProfitMap: Record<string, number> = {};
  goods.forEach((g: any) => {
    const b = g.brand || '未知';
    brandProfitMap[b] = (brandProfitMap[b] || 0) + (g.profit || 0);
  });
  const brandPieData = Object.entries(brandProfitMap)
    .sort((a, b) => b[1] - a[1])
    .slice(0, 10)
    .map(([name, value]) => ({ name, value }));

  const brandPieOption = {
    tooltip: { trigger: 'item' as const, formatter: '{b}: ¥{c} ({d}%)' },
    legend: { bottom: 0, type: 'scroll' as const },
    series: [{
      type: 'pie',
      radius: ['35%', '65%'],
      label: {
        show: true,
        formatter: '{b}\n{d}%',
        fontSize: 11,
        lineHeight: 15,
      },
      data: brandPieData,
    }],
  };

  // Table with sorting
  const goodsColumns = [
    { title: '商品编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 110 },
    { title: '商品名称', dataIndex: 'goodsName', key: 'goodsName', ellipsis: true },
    { title: '品牌', dataIndex: 'brand', key: 'brand', width: 90 },
    {
      title: '销售额',
      dataIndex: 'sales',
      key: 'sales',
      width: 130,
      sorter: (a: any, b: any) => a.sales - b.sales,
      sortOrder: sortedInfo.columnKey === 'sales' ? sortedInfo.order : null,
      render: (v: number) => `¥${(v || 0).toLocaleString()}`,
    },
    {
      title: '毛利',
      dataIndex: 'profit',
      key: 'profit',
      width: 130,
      sorter: (a: any, b: any) => (a.profit || 0) - (b.profit || 0),
      sortOrder: sortedInfo.columnKey === 'profit' ? sortedInfo.order : null,
      defaultSortOrder: 'descend' as const,
      render: (v: number) => `¥${(v || 0).toLocaleString()}`,
    },
    {
      title: '毛利率',
      key: 'profit_rate',
      width: 100,
      sorter: (a: any, b: any) => {
        const ra = a.sales > 0 ? (a.profit || 0) / a.sales : 0;
        const rb = b.sales > 0 ? (b.profit || 0) / b.sales : 0;
        return ra - rb;
      },
      sortOrder: sortedInfo.columnKey === 'profit_rate' ? sortedInfo.order : null,
      render: (_: any, row: any) => {
        const rate = row.sales > 0 ? (row.profit || 0) / row.sales : 0;
        return <span style={{ color: profitRateColor(rate), fontWeight: 600 }}>{(rate * 100).toFixed(1)}%</span>;
      },
    },
  ];

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', alignItems: 'center', gap: 12 }}>
        <span style={{ fontWeight: 500 }}>选择部门：</span>
        <Select
          value={dept}
          onChange={val => setDept(val)}
          options={deptOptions}
          style={{ width: 160 }}
        />
      </div>

      <DateFilter start={startDate} end={endDate} onChange={handleDateChange} />

      {/* Summary */}
      <Row gutter={[16, 16]}>
        {statCards.map((card) => (
          <Col xs={24} sm={6} key={card.title}>
            <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
              <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
            </Card>
          </Col>
        ))}
      </Row>

      {/* Charts */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={14}>
          <Card title="TOP15 商品毛利排名">
            <ReactECharts lazyUpdate={true} option={horizBarOption} style={{ height: 420 }} />
          </Card>
        </Col>
        <Col xs={24} lg={10}>
          <Card title="品牌毛利占比 TOP10">
            <ReactECharts lazyUpdate={true} option={brandPieOption} style={{ height: 420 }} />
          </Card>
        </Col>
      </Row>

      {/* Table */}
      <Card className="bi-table-card" title="产品利润统计明细" style={{ marginTop: 16 }}>
        <Table
          dataSource={goods}
          columns={goodsColumns}
          rowKey="goodsNo"
          pagination={{ pageSize: 20, showSizeChanger: true }}
          size="small"
          onChange={(_, __, sorter: any) => setSortedInfo(sorter)}
        />
      </Card>
    </div>
  );
};

export default FinanceProductProfit;
