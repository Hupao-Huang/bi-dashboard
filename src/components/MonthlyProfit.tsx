import React, { useEffect, useState, useCallback } from 'react';
import { DEPT_COLORS } from '../chartTheme';
import { Row, Col, Card, Table, Statistic, Spin, Tag } from 'antd';
import dayjs from 'dayjs';
import ReactECharts from './Chart';
import DateFilter from './DateFilter';
import PageLoading from './PageLoading';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../config';

interface Props {
  dept: string;
}


const MonthlyProfit: React.FC<Props> = ({ dept }) => {
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);

  const color = DEPT_COLORS[dept] || '#4f46e5';

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

  const daily = data.daily || [];

  // 按月汇总
  const monthMap: Record<string, { month: string; sales: number; qty: number; days: number }> = {};
  daily.forEach((d: any) => {
    const dateStr = d.date ? String(d.date).slice(0, 10) : '';
    const month = dateStr.slice(0, 7);
    if (!month) return;
    if (!monthMap[month]) {
      monthMap[month] = { month, sales: 0, qty: 0, days: 0 };
    }
    monthMap[month].sales += d.sales || 0;
    monthMap[month].qty += d.qty || 0;
    monthMap[month].days += 1;
  });

  const monthlyData = Object.values(monthMap).sort((a, b) => a.month.localeCompare(b.month));

  // 环比计算
  const monthlyWithMoM = monthlyData.map((item, i) => {
    const prev = monthlyData[i - 1];
    const salesMoM = prev && prev.sales > 0 ? ((item.sales - prev.sales) / prev.sales * 100) : null;
    return {
      ...item,
      avgSales: item.days > 0 ? Math.round(item.sales / item.days) : 0,
      salesMoM,
    };
  });

  // 月度销售对比图
  const monthBarOption = {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['销售额', '销量'], top: 0 },
    grid: { left: 80, right: 80, top: 50, bottom: 30 },
    xAxis: {
      type: 'category' as const,
      data: monthlyWithMoM.map(m => m.month),
    },
    yAxis: [
      {
        type: 'value' as const,
        name: '销售额',
        min: 0,
        axisLabel: { formatter: (v: number) => v >= 10000 ? (v / 10000).toFixed(0) + '万' : String(v) },
      },
      {
        type: 'value' as const,
        name: '销量',
        min: 0,
        position: 'right' as const,
      },
    ],
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
        name: '销量',
        type: 'line',
        yAxisIndex: 1,
        smooth: true,
        data: monthlyWithMoM.map(m => m.qty),
        itemStyle: { color: '#f97316' },
      },
    ],
  };

  // 日均销售趋势
  const avgLineOption = {
    tooltip: { trigger: 'axis' as const, formatter: (params: any) => `${params[0].name}<br/>日均销售: ¥${params[0].value?.toLocaleString()}` },
    grid: { left: 80, right: 40, top: 30, bottom: 30 },
    xAxis: { type: 'category' as const, data: monthlyWithMoM.map(m => m.month) },
    yAxis: {
      type: 'value' as const,
      min: 0,
      axisLabel: { formatter: (v: number) => v >= 10000 ? (v / 10000).toFixed(0) + '万' : String(v) },
    },
    series: [{
      type: 'line',
      smooth: true,
      data: monthlyWithMoM.map(m => m.avgSales),
      itemStyle: { color: '#4f46e5' },
      areaStyle: { color: 'rgba(24,144,255,0.1)' },
      label: { show: true, formatter: (p: any) => p.value >= 10000 ? (p.value / 10000).toFixed(1) + '万' : p.value },
    }],
  };

  // 表格列
  const columns = [
    { title: '月份', dataIndex: 'month', key: 'month', width: 90 },
    { title: '销售额', dataIndex: 'sales', key: 'sales', width: 130, render: (v: number) => `¥${v?.toLocaleString()}` },
    {
      title: '销售额环比',
      dataIndex: 'salesMoM',
      key: 'salesMoM',
      width: 110,
      render: (v: number | null) => {
        if (v === null) return '-';
        return <Tag color={v >= 0 ? 'green' : 'red'}>{v >= 0 ? '+' : ''}{v.toFixed(1)}%</Tag>;
      },
    },
    { title: '销量', dataIndex: 'qty', key: 'qty', width: 90, render: (v: number) => v?.toLocaleString() },
    { title: '天数', dataIndex: 'days', key: 'days', width: 60 },
    {
      title: '日均销售',
      dataIndex: 'avgSales',
      key: 'avgSales',
      width: 120,
      render: (v: number) => `¥${v?.toLocaleString()}`,
    },
  ];

  const totalSales = monthlyWithMoM.reduce((s, m) => s + m.sales, 0);
  const totalQty = monthlyWithMoM.reduce((s, m) => s + m.qty, 0);
  const totalDays = monthlyWithMoM.reduce((s, m) => s + m.days, 0);
  const statCards = [
    { title: '期间销售额', value: totalSales, precision: 2, prefix: '¥', accentColor: color },
    { title: '期间销量', value: totalQty, suffix: '件', accentColor: '#10b981' },
    { title: '日均销售额', value: totalDays > 0 ? Math.round(totalSales / totalDays) : 0, precision: 0, prefix: '¥', accentColor: '#f59e0b' },
  ];

  return (
    <div>
      <DateFilter start={startDate} end={endDate} onChange={handleDateChange} />

      {/* 汇总 */}
      <Row gutter={[16, 16]}>
        {statCards.map((card) => (
          <Col xs={24} sm={8} key={card.title}>
            <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
              <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
            </Card>
          </Col>
        ))}
      </Row>

      {/* 月度对比图 */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={14}>
          <Card title="月度销售额 & 销量对比">
            <ReactECharts lazyUpdate={true} option={monthBarOption} style={{ height: 320 }} />
          </Card>
        </Col>
        <Col xs={24} lg={10}>
          <Card title="月度日均销售趋势">
            <ReactECharts lazyUpdate={true} option={avgLineOption} style={{ height: 320 }} />
          </Card>
        </Col>
      </Row>

      {/* 月度统计表 */}
      <Card className="bi-table-card" title="月度销售统计表" style={{ marginTop: 16 }}>
        <Table
          dataSource={monthlyWithMoM}
          columns={columns}
          rowKey="month"
          pagination={false}
          size="small"
          summary={pageData => {
            const sumSales = pageData.reduce((s, row) => s + row.sales, 0);
            const sumQty = pageData.reduce((s, row) => s + row.qty, 0);
            const sumDays = pageData.reduce((s, row) => s + row.days, 0);
            return (
              <Table.Summary.Row style={{ fontWeight: 'bold', background: '#fafafa' }}>
                <Table.Summary.Cell index={0}>合计</Table.Summary.Cell>
                <Table.Summary.Cell index={1}>¥{sumSales.toLocaleString()}</Table.Summary.Cell>
                <Table.Summary.Cell index={2}>-</Table.Summary.Cell>
                <Table.Summary.Cell index={3}>{sumQty.toLocaleString()}</Table.Summary.Cell>
                <Table.Summary.Cell index={4}>{sumDays}</Table.Summary.Cell>
                <Table.Summary.Cell index={5}>¥{sumDays > 0 ? Math.round(sumSales / sumDays).toLocaleString() : 0}</Table.Summary.Cell>
              </Table.Summary.Row>
            );
          }}
        />
      </Card>
    </div>
  );
};

export default MonthlyProfit;
