import React, { useEffect, useState, useCallback, useRef } from 'react';
import dayjs from 'dayjs';
import { DEPT_COLORS } from '../chartTheme';
import { Row, Col, Card, Table, Statistic, Select, Tag } from 'antd';
import ReactECharts from './Chart';
import DateFilter from './DateFilter';
import PageLoading from './PageLoading';
import { API_BASE } from '../config';

// 月度利润统计默认展示近 12 个月（避免进入时只显示 1 个月导致趋势图只有 1 个点）
const DEFAULT_MONTHLY_START = dayjs().subtract(11, 'month').startOf('month').format('YYYY-MM-DD');
const DEFAULT_MONTHLY_END = dayjs().subtract(1, 'day').format('YYYY-MM-DD');

const deptOptions = [
  { value: 'all', label: '全部部门' },
  { value: 'ecommerce', label: '电商部门' },
  { value: 'social', label: '社媒部门' },
  { value: 'offline', label: '线下部门' },
  { value: 'distribution', label: '分销部门' },
  { value: 'instant_retail', label: '即时零售部' },
];


const profitRateColor = (rate: number) => {
  if (rate >= 0.5) return '#10b981';
  if (rate >= 0.3) return '#faad14';
  return '#f5222d';
};

const FinanceMonthlyProfit: React.FC = () => {
  const abortRef = useRef<AbortController | null>(null);
  const [dept, setDept] = useState('all');
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [startDate, setStartDate] = useState(DEFAULT_MONTHLY_START);
  const [endDate, setEndDate] = useState(DEFAULT_MONTHLY_END);

  const color = DEPT_COLORS[dept] || '#1e40af';

  const fetchData = useCallback((d: string, s: string, e: string) => {
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    const url = d === 'all'
      ? `${API_BASE}/api/overview?start=${s}&end=${e}`
      : `${API_BASE}/api/department?dept=${d}&start=${s}&end=${e}`;
    fetch(url)
      .then(res => res.json())
      .then(res => { setData({ ...res.data, _mode: d }); setLoading(false); })
      .catch((e: any) => { if (e?.name !== 'AbortError') setLoading(false); });
  }, []);

  useEffect(() => { fetchData(dept, startDate, endDate); }, [fetchData, dept, startDate, endDate]);

  const handleDateChange = (s: string, e: string) => {
    setStartDate(s);
    setEndDate(e);
  };

  if (loading) return <PageLoading />;
  if (!data) return <div>加载失败</div>;

  // Gather daily rows
  let dailyRows: any[] = [];
  if (data._mode === 'all') {
    // /api/overview returns trend array with {date, department, sales, qty}
    // We need to sum across depts per day for profit; try to use departments-level or trend
    // Since overview API may not return profit in trend, we use departments for totals
    // and build monthly from trend sales only (profit may not be available per-day in overview)
    dailyRows = (data.trend || []).map((t: any) => ({
      date: t.date,
      sales: t.sales || 0,
      profit: t.profit || 0,
      cost: t.cost || 0,
      qty: t.qty || 0,
    }));
  } else {
    dailyRows = (data.daily || []).map((d: any) => ({
      date: d.date,
      sales: d.sales || 0,
      profit: d.profit || 0,
      cost: d.cost || 0,
      qty: d.qty || 0,
    }));
  }

  // Aggregate by month
  const monthMap: Record<string, { month: string; sales: number; profit: number; cost: number; qty: number; days: number }> = {};
  dailyRows.forEach(d => {
    const dateStr = d.date ? String(d.date).slice(0, 10) : '';
    const month = dateStr.slice(0, 7);
    if (!month) return;
    if (!monthMap[month]) {
      monthMap[month] = { month, sales: 0, profit: 0, cost: 0, qty: 0, days: 0 };
    }
    monthMap[month].sales += d.sales;
    monthMap[month].profit += d.profit;
    monthMap[month].cost += d.cost;
    monthMap[month].qty += d.qty;
    monthMap[month].days += 1;
  });

  // If all-mode and no profit in trend, derive from departments totals proportionally
  // (best effort: profit rate overall applied per month)
  const monthlyData = Object.values(monthMap).sort((a, b) => a.month.localeCompare(b.month));

  // If no profit data at all for 'all' mode, derive profit from departments data
  const hasProfitData = monthlyData.some(m => m.profit > 0);
  if (!hasProfitData && data._mode === 'all') {
    const depts = data.departments || [];
    const totalSalesAll = depts.reduce((s: number, d: any) => s + (d.sales || 0), 0);
    const totalProfitAll = depts.reduce((s: number, d: any) => s + (d.profit || 0), 0);
    const overallRate = totalSalesAll > 0 ? totalProfitAll / totalSalesAll : 0;
    monthlyData.forEach(m => {
      m.profit = Math.round(m.sales * overallRate);
      m.cost = Math.round(m.sales * (1 - overallRate));
    });
  }

  const monthlyWithMoM = monthlyData.map((item, i) => {
    const prev = monthlyData[i - 1];
    const profitMoM = prev && prev.profit > 0 ? ((item.profit - prev.profit) / prev.profit * 100) : null;
    const salesMoM = prev && prev.sales > 0 ? ((item.sales - prev.sales) / prev.sales * 100) : null;
    const profitRate = item.sales > 0 ? item.profit / item.sales : 0;
    return {
      ...item,
      profitRate,
      profitMoM,
      salesMoM,
      avgDailyProfit: item.days > 0 ? Math.round(item.profit / item.days) : 0,
    };
  });

  const totalSales = monthlyWithMoM.reduce((s, m) => s + m.sales, 0);
  const totalProfit = monthlyWithMoM.reduce((s, m) => s + m.profit, 0);
  const overallProfitRate = totalSales > 0 ? totalProfit / totalSales : 0;
  const statCards = [
    { title: '期间销售额', value: totalSales, precision: 2, prefix: '¥', accentColor: color },
    { title: '期间毛利', value: totalProfit, precision: 2, prefix: '¥', accentColor: '#10b981' },
    { title: '综合毛利率', value: overallProfitRate * 100, precision: 1, suffix: '%', accentColor: profitRateColor(overallProfitRate) },
  ];

  // Bar chart: monthly sales vs profit
  const barOption = {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['销售额', '毛利'], top: 0 },
    grid: { left: 80, right: 40, top: 50, bottom: 30 },
    xAxis: {
      type: 'category' as const,
      data: monthlyWithMoM.map(m => m.month),
    },
    yAxis: {
      type: 'value' as const,
      axisLabel: { formatter: (v: number) => v >= 10000 ? (v / 10000).toFixed(0) + '万' : String(v) },
    },
    series: [
      {
        name: '销售额',
        type: 'bar',
        data: monthlyWithMoM.map(m => m.sales),
        itemStyle: { color },
        barWidth: 30,
        label: { show: true, position: 'top' as const, formatter: (p: any) => p.value >= 10000 ? (p.value / 10000).toFixed(1) + '万' : p.value },
      },
      {
        name: '毛利',
        type: 'bar',
        data: monthlyWithMoM.map(m => m.profit),
        itemStyle: { color: '#10b981' },
        barWidth: 30,
        label: { show: true, position: 'top' as const, formatter: (p: any) => p.value >= 10000 ? (p.value / 10000).toFixed(1) + '万' : p.value },
      },
    ],
  };

  // Monthly profit rate trend line
  const rateLineOption = {
    tooltip: {
      trigger: 'axis' as const,
      formatter: (params: any) => `${params[0].name}<br/>毛利率: ${params[0].value?.toFixed(1)}%`,
    },
    grid: { left: 60, right: 40, top: 30, bottom: 30 },
    xAxis: { type: 'category' as const, data: monthlyWithMoM.map(m => m.month) },
    yAxis: {
      type: 'value' as const,
      min: 0,
      max: 100,
      axisLabel: { formatter: (v: number) => v + '%' },
    },
    series: [{
      type: 'line',
      smooth: true,
      data: monthlyWithMoM.map(m => parseFloat((m.profitRate * 100).toFixed(2))),
      itemStyle: { color: '#7c3aed' },
      areaStyle: { color: 'rgba(114,46,209,0.1)' },
      label: { show: true, formatter: (p: any) => p.value.toFixed(1) + '%' },
    }],
  };

  const columns = [
    { title: '月份', dataIndex: 'month', key: 'month', width: 90 },
    { title: '销售额', dataIndex: 'sales', key: 'sales', width: 130, render: (v: number) => `¥${v?.toLocaleString()}` },
    { title: '毛利', dataIndex: 'profit', key: 'profit', width: 130, render: (v: number) => `¥${v?.toLocaleString()}` },
    {
      title: '毛利率',
      dataIndex: 'profitRate',
      key: 'profitRate',
      width: 100,
      render: (v: number) => <span style={{ color: profitRateColor(v), fontWeight: 600 }}>{(v * 100).toFixed(1)}%</span>,
    },
    {
      title: '环比',
      dataIndex: 'profitMoM',
      key: 'profitMoM',
      width: 110,
      render: (v: number | null) => {
        if (v === null) return '-';
        return <Tag color={v >= 0 ? 'green' : 'red'}>{v >= 0 ? '+' : ''}{v.toFixed(1)}%</Tag>;
      },
    },
    { title: '货品数', dataIndex: 'qty', key: 'qty', width: 80, render: (v: number) => v?.toLocaleString() },
    { title: '日均毛利', dataIndex: 'avgDailyProfit', key: 'avgDailyProfit', width: 120, render: (v: number) => `¥${v?.toLocaleString()}` },
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
          <Col xs={24} sm={8} key={card.title}>
            <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
              <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
            </Card>
          </Col>
        ))}
      </Row>

      {/* Charts */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={14}>
          <Card title="月度销售额 & 毛利对比">
            <ReactECharts lazyUpdate={true} option={barOption} style={{ height: 320 }} />
          </Card>
        </Col>
        <Col xs={24} lg={10}>
          <Card title="月度毛利率趋势">
            <ReactECharts lazyUpdate={true} option={rateLineOption} style={{ height: 320 }} />
          </Card>
        </Col>
      </Row>

      {/* Table */}
      <Card className="bi-table-card" title="月度利润统计表" style={{ marginTop: 16 }}>
        <Table
          dataSource={monthlyWithMoM}
          columns={columns}
          rowKey="month"
          pagination={false}
          size="small"
          summary={pageData => {
            const sumSales = pageData.reduce((s, row) => s + row.sales, 0);
            const sumProfit = pageData.reduce((s, row) => s + row.profit, 0);
            const sumQty = pageData.reduce((s, row) => s + row.qty, 0);
            const totalDays = pageData.reduce((s, row) => s + row.days, 0);
            const rate = sumSales > 0 ? sumProfit / sumSales : 0;
            return (
              <Table.Summary.Row style={{ fontWeight: 'bold', background: '#fafafa' }}>
                <Table.Summary.Cell index={0}>合计</Table.Summary.Cell>
                <Table.Summary.Cell index={1}>¥{sumSales.toLocaleString()}</Table.Summary.Cell>
                <Table.Summary.Cell index={2}>¥{sumProfit.toLocaleString()}</Table.Summary.Cell>
                <Table.Summary.Cell index={3}><span style={{ color: profitRateColor(rate), fontWeight: 600 }}>{(rate * 100).toFixed(1)}%</span></Table.Summary.Cell>
                <Table.Summary.Cell index={4}>-</Table.Summary.Cell>
                <Table.Summary.Cell index={5}>{sumQty.toLocaleString()}</Table.Summary.Cell>
                <Table.Summary.Cell index={6}>¥{totalDays > 0 ? Math.round(sumProfit / totalDays).toLocaleString() : 0}</Table.Summary.Cell>
              </Table.Summary.Row>
            );
          }}
        />
      </Card>
    </div>
  );
};

export default FinanceMonthlyProfit;
