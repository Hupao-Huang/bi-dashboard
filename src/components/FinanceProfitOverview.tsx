import React, { useEffect, useState, useCallback } from 'react';
import { Row, Col, Card, Table, Statistic } from 'antd';
import ReactECharts from './Chart';
import DateFilter from './DateFilter';
import PageLoading from './PageLoading';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../config';

const deptConfig: Record<string, { label: string; color: string }> = {
  ecommerce: { label: '电商部门', color: '#4f46e5' },
  social: { label: '社媒部门', color: '#10b981' },
  offline: { label: '线下部门', color: '#faad14' },
  distribution: { label: '分销部门', color: '#8b5cf6' },
};

const profitRateColor = (rate: number) => {
  if (rate >= 0.5) return '#10b981';
  if (rate >= 0.3) return '#faad14';
  return '#f5222d';
};

const FinanceProfitOverview: React.FC = () => {
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);

  const fetchData = useCallback((s: string, e: string) => {
    setLoading(true);
    fetch(`${API_BASE}/api/overview?start=${s}&end=${e}`)
      .then(res => res.json())
      .then(res => { setData(res.data); setLoading(false); })
      .catch(() => setLoading(false));
  }, []);

  useEffect(() => { fetchData(startDate, endDate); }, [fetchData, startDate, endDate]);

  const handleDateChange = (s: string, e: string) => {
    setStartDate(s);
    setEndDate(e);
  };

  if (loading) return <PageLoading />;
  if (!data) return <div>加载失败</div>;

  const depts = data.departments || [];

  const totalSales = depts.reduce((s: number, d: any) => s + (d.sales || 0), 0);
  const totalCost = depts.reduce((s: number, d: any) => s + (d.cost || 0), 0);
  const totalProfit = depts.reduce((s: number, d: any) => s + (d.profit || 0), 0);
  const overallProfitRate = totalSales > 0 ? totalProfit / totalSales : 0;
  const statCards = [
    { title: '总销售额', value: totalSales, precision: 2, prefix: '¥', accentColor: '#4f46e5' },
    { title: '总成本', value: totalCost, precision: 2, prefix: '¥', accentColor: '#f97316' },
    { title: '总毛利', value: totalProfit, precision: 2, prefix: '¥', accentColor: '#10b981' },
    { title: '综合毛利率', value: overallProfitRate * 100, precision: 1, suffix: '%', accentColor: profitRateColor(overallProfitRate) },
  ];

  // Bar chart: sales vs profit vs cost per dept
  const deptNames = depts.map((d: any) => deptConfig[d.department]?.label || d.department);
  const barOption = {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['销售额', '毛利', '成本'], top: 0 },
    grid: { left: 80, right: 40, top: 50, bottom: 30 },
    xAxis: {
      type: 'category' as const,
      data: deptNames,
    },
    yAxis: {
      type: 'value' as const,
      axisLabel: { formatter: (v: number) => v >= 10000 ? (v / 10000).toFixed(0) + '万' : String(v) },
    },
    series: [
      {
        name: '销售额',
        type: 'bar',
        data: depts.map((d: any) => d.sales || 0),
        itemStyle: { color: '#4f46e5' },
        barWidth: 20,
        label: { show: true, position: 'top' as const, formatter: (p: any) => p.value >= 10000 ? (p.value / 10000).toFixed(1) + '万' : p.value },
      },
      {
        name: '毛利',
        type: 'bar',
        data: depts.map((d: any) => d.profit || 0),
        itemStyle: { color: '#10b981' },
        barWidth: 20,
        label: { show: true, position: 'top' as const, formatter: (p: any) => p.value >= 10000 ? (p.value / 10000).toFixed(1) + '万' : p.value },
      },
      {
        name: '成本',
        type: 'bar',
        data: depts.map((d: any) => d.cost || 0),
        itemStyle: { color: '#f97316' },
        barWidth: 20,
        label: { show: true, position: 'top' as const, formatter: (p: any) => p.value >= 10000 ? (p.value / 10000).toFixed(1) + '万' : p.value },
      },
    ],
  };

  // Pie chart: profit share per dept
  const pieData = depts.map((d: any) => ({
    value: d.profit || 0,
    name: deptConfig[d.department]?.label || d.department,
    itemStyle: { color: deptConfig[d.department]?.color },
  }));
  const pieOption = {
    tooltip: { trigger: 'item' as const, formatter: '{b}: ¥{c} ({d}%)' },
    legend: { bottom: 0 },
    series: [{
      type: 'pie',
      radius: ['40%', '70%'],
      label: {
        show: true,
        formatter: '{b}\n{d}%',
        fontSize: 12,
        lineHeight: 16,
      },
      labelLine: { length: 20, length2: 15 },
      data: pieData,
    }],
  };

  // Table columns
  const columns = [
    { title: '部门', dataIndex: 'department', key: 'department', width: 120, render: (v: string) => deptConfig[v]?.label || v },
    { title: '销售额', dataIndex: 'sales', key: 'sales', width: 130, render: (v: number) => `¥${v?.toLocaleString()}` },
    { title: '成本', dataIndex: 'cost', key: 'cost', width: 130, render: (v: number) => `¥${(v || 0).toLocaleString()}` },
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
    { title: '货品数', dataIndex: 'qty', key: 'qty', width: 90, render: (v: number) => v?.toLocaleString() },
  ];

  return (
    <div>
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

      {/* Charts */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={14}>
          <Card title="各部门销售额 vs 毛利 vs 成本">
            <ReactECharts lazyUpdate={true} option={barOption} style={{ height: 360 }} />
          </Card>
        </Col>
        <Col xs={24} lg={10}>
          <Card title="各部门毛利占比">
            <ReactECharts lazyUpdate={true} option={pieOption} style={{ height: 360 }} />
          </Card>
        </Col>
      </Row>

      {/* Table */}
      <Card className="bi-table-card" title="部门利润汇总表" style={{ marginTop: 16 }}>
        <Table
          dataSource={depts}
          columns={columns}
          rowKey="department"
          pagination={false}
          size="small"
          summary={pageData => {
            const sumSales = pageData.reduce((s: number, row: any) => s + (row.sales || 0), 0);
            const sumCost = pageData.reduce((s: number, row: any) => s + (row.cost || 0), 0);
            const sumProfit = pageData.reduce((s: number, row: any) => s + (row.profit || 0), 0);
            const sumQty = pageData.reduce((s: number, row: any) => s + (row.qty || 0), 0);
            const rate = sumSales > 0 ? sumProfit / sumSales : 0;
            return (
              <Table.Summary.Row style={{ fontWeight: 'bold', background: '#fafafa' }}>
                <Table.Summary.Cell index={0}>合计</Table.Summary.Cell>
                <Table.Summary.Cell index={1}>¥{sumSales.toLocaleString()}</Table.Summary.Cell>
                <Table.Summary.Cell index={2}>¥{sumCost.toLocaleString()}</Table.Summary.Cell>
                <Table.Summary.Cell index={3}>¥{sumProfit.toLocaleString()}</Table.Summary.Cell>
                <Table.Summary.Cell index={4}><span style={{ color: profitRateColor(rate), fontWeight: 600 }}>{(rate * 100).toFixed(1)}%</span></Table.Summary.Cell>
                <Table.Summary.Cell index={5}>{sumQty.toLocaleString()}</Table.Summary.Cell>
              </Table.Summary.Row>
            );
          }}
        />
      </Card>
    </div>
  );
};

export default FinanceProfitOverview;
