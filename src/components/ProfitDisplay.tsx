import React, { useEffect, useState, useCallback, useRef } from 'react';
import { DEPT_COLORS } from '../chartTheme';
import { Row, Col, Card, Statistic, Spin } from 'antd';
import dayjs from 'dayjs';
import ReactECharts from './Chart';
import DateFilter from './DateFilter';
import PageLoading from './PageLoading';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../config';

interface Props {
  dept: string;
}


const formatDate = (d: string) => {
  if (!d) return '';
  const m = d.match(/(\d{4}-)?(\d{2}-\d{2})/);
  return m ? m[2] : d.slice(0, 10);
};

const ProfitDisplay: React.FC<Props> = ({ dept  }) => {
  const abortRef = useRef<AbortController | null>(null);
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);

  const color = DEPT_COLORS[dept] || '#1e40af';

  const fetchData = useCallback((s: string, e: string) => {
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    fetch(`${API_BASE}/api/department?dept=${dept}&start=${s}&end=${e}`, { signal: ctrl.signal })
      .then(res => res.json())
      .then(res => { setData(res.data); setLoading(false); })
      .catch((e: any) => { if (e?.name !== 'AbortError') setLoading(false); });
  }, [dept]);

  useEffect(() => { fetchData(startDate, endDate); }, [fetchData, startDate, endDate]);

  const handleDateChange = (s: string, e: string) => {
    setStartDate(s);
    setEndDate(e);
  };

  if (loading) return <PageLoading />;
  if (!data) return <div>加载失败</div>;

  const daily = data.daily || [];
  const shops = data.shops || [];

  const totalSales = daily.reduce((s: number, d: any) => s + d.sales, 0);
  const totalQty = daily.reduce((s: number, d: any) => s + d.qty, 0);
  const avgOrderValue = totalQty > 0 ? totalSales / totalQty : 0;
  const days = daily.length;
  const dailyAvgSales = days > 0 ? totalSales / days : 0;
  const statCards = [
    { title: '总销售额', value: totalSales, precision: 2, prefix: '¥', accentColor: color },
    { title: '总销量', value: totalQty, suffix: '件', accentColor: '#10b981' },
    { title: '客单价', value: avgOrderValue, precision: 2, prefix: '¥', accentColor: '#1e40af' },
    { title: '日均销售额', value: dailyAvgSales, precision: 2, prefix: '¥', accentColor: '#7c3aed' },
  ];

  // 每日销售趋势（销售额柱 + 销量折线）
  const salesTrendOption = {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['销售额', '销量'], top: 0 },
    grid: { left: 80, right: 80, top: 50, bottom: 40 },
    xAxis: {
      type: 'category' as const,
      data: daily.map((d: any) => formatDate(d.date)),
      axisLabel: { fontSize: 11 },
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
      { name: '销售额', type: 'bar', data: daily.map((d: any) => d.sales), itemStyle: { color }, barWidth: 12 },
      {
        name: '销量',
        type: 'line',
        yAxisIndex: 1,
        smooth: true,
        data: daily.map((d: any) => d.qty),
        itemStyle: { color: '#ea580c' },
      },
    ],
  };

  // 各店铺销售额对比（仅销售额）
  const shopSalesOption = {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['销售额', '销量'], top: 0 },
    grid: { left: 160, right: 40, top: 40, bottom: 20 },
    xAxis: {
      type: 'value' as const,
      axisLabel: { formatter: (v: number) => v >= 10000 ? (v / 10000).toFixed(0) + '万' : String(v) },
    },
    yAxis: {
      type: 'category' as const,
      data: shops.slice(0, 10).map((s: any) => s.shopName).reverse(),
    },
    series: [
      {
        name: '销售额',
        type: 'bar',
        data: shops.slice(0, 10).map((s: any) => s.sales).reverse(),
        itemStyle: { color },
        barWidth: 14,
      },
      {
        name: '销量',
        type: 'bar',
        data: shops.slice(0, 10).map((s: any) => s.qty).reverse(),
        itemStyle: { color: '#c7d2fe' },
        barWidth: 14,
      },
    ],
  };

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

      {/* 每日销售趋势 */}
      <Card title="每日销售趋势" style={{ marginTop: 16 }}>
        <ReactECharts lazyUpdate={true} option={salesTrendOption} style={{ height: 350 }} />
      </Card>

      {/* 各店铺销售额对比 */}
      <Card title="各店铺销售额对比 TOP10" style={{ marginTop: 16 }}>
        <ReactECharts lazyUpdate={true} option={shopSalesOption} style={{ height: 380 }} />
      </Card>
    </div>
  );
};

export default ProfitDisplay;
