import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Row, Col, Card, Table, Statistic } from 'antd';

import ReactECharts from './Chart';
import DateFilter from './DateFilter';
import PageLoading from './PageLoading';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../config';
import {
  getBaseOption,
  barItemStyle,
  lineAreaStyle,
  pieStyle,
  formatMoney,
  CHART_COLORS,
} from '../chartTheme';

interface Props {
  dept: string;
  title: string;
  color: string;
}

const DepartmentPage: React.FC<Props> = ({ dept, title, color  }) => {
  const abortRef = useRef<AbortController | null>(null);
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);

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
  const totalSales = daily.reduce((s: number, d: any) => s + d.sales, 0);
  const totalQty = daily.reduce((s: number, d: any) => s + d.qty, 0);
  const avgOrderValue = totalQty > 0 ? totalSales / totalQty : 0;
  const days = daily.length;
  const dailyAvgSales = days > 0 ? totalSales / days : 0;

  const formatDate = (d: string) => {
    if (!d) return '';
    const m = d.match(/(\d{4}-)?(\d{2}-\d{2})/);
    return m ? m[2] : d.slice(0, 10);
  };

  const baseOpt = getBaseOption();

  // 统计卡片配置 - 白底+左侧色条
  const statCards = [
    { title: '总销售额', value: totalSales, precision: 2, prefix: '¥', accentColor: '#1e40af' },
    { title: '总货品数', value: totalQty, precision: 0, prefix: '', accentColor: '#10b981' },
    { title: '综合客单价', value: avgOrderValue, precision: 2, prefix: '¥', accentColor: '#7c3aed' },
    { title: '日均销售额', value: dailyAvgSales, precision: 2, prefix: '¥', accentColor: '#f59e0b' },
  ];

  // 每日销售趋势
  const trendOption = {
    ...baseOpt,
    legend: { ...baseOpt.legend, data: ['销售额', '货品数'], top: 4 },
    grid: { left: 60, right: 60, top: 48, bottom: 32 },
    xAxis: { ...baseOpt.xAxis, type: 'category' as const, data: daily.map((d: any) => formatDate(d.date)) },
    yAxis: [
      { ...baseOpt.yAxis, type: 'value' as const, name: '金额', min: 0, axisLabel: { ...baseOpt.yAxis.axisLabel, formatter: formatMoney } },
      { ...baseOpt.yAxis, type: 'value' as const, name: '货品数', min: 0, position: 'right' as const },
    ],
    series: [
      { name: '销售额', type: 'bar', data: daily.map((d: any) => d.sales), ...barItemStyle(color), barWidth: 14 },
      { name: '货品数', type: 'line', yAxisIndex: 1, smooth: true, data: daily.map((d: any) => d.qty), ...lineAreaStyle('#f59e0b'), symbol: 'circle', symbolSize: 4 },
    ],
  };

  // 店铺排行
  const shops = data.shops || [];
  const shopOption = {
    ...baseOpt,
    grid: { left: 8, right: 40, top: 8, bottom: 8, containLabel: true },
    xAxis: { ...baseOpt.xAxis, type: 'value' as const, axisLabel: { ...baseOpt.xAxis.axisLabel, formatter: formatMoney } },
    yAxis: { ...baseOpt.yAxis, type: 'category' as const, data: shops.slice(0, 10).map((s: any) => s.shopName).reverse(), axisLabel: { ...baseOpt.yAxis.axisLabel, width: 160, overflow: 'truncate' as const } },
    series: [{ type: 'bar', data: shops.slice(0, 10).map((s: any) => s.sales).reverse(), ...barItemStyle(color), barWidth: 16 }],
  };

  // 品牌占比
  const brands = data.brands || [];
  const brandOption = {
    ...pieStyle,
    color: CHART_COLORS,
    series: [{
      type: 'pie',
      radius: ['38%', '68%'],
      center: ['50%', '45%'],
      label: { show: true, formatter: '{b}: {d}%', fontSize: 11, color: '#64748b' },
      labelLine: { length: 15, length2: 10, lineStyle: { color: '#cbd5e1' } },
      itemStyle: { borderColor: '#fff', borderWidth: 2, borderRadius: 4 },
      emphasis: { scaleSize: 6 },
      data: brands.map((b: any) => ({ value: b.sales, name: b.brand || '未知' })),
    }],
  };

  // 商品排行表
  const indexedGoods = (data.goods || []).map((g: any, i: number) => ({ ...g, _rank: i + 1 }));
  const goodsColumns = [
    { title: '#', dataIndex: '_rank', key: 'rank', width: 40, render: (v: number) => <span style={{ color: v <= 3 ? color : '#94a3b8', fontWeight: v <= 3 ? 700 : 400 }}>{v}</span> },
    { title: '编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 110 },
    { title: '商品名称', dataIndex: 'goodsName', key: 'goodsName', ellipsis: true },
    { title: '品牌', dataIndex: 'brand', key: 'brand', width: 80 },
    { title: '销售额', dataIndex: 'sales', key: 'sales', width: 110, render: (v: number) => <span style={{ fontWeight: 600, fontVariantNumeric: 'tabular-nums' }}>¥{v?.toLocaleString()}</span> },
    { title: '销量', dataIndex: 'qty', key: 'qty', width: 70, render: (v: number) => v?.toLocaleString() },
    { title: '客单价', key: 'avgPrice', width: 100, render: (_: any, r: any) => r.qty > 0 ? `¥${(r.sales / r.qty).toFixed(2)}` : '-' },
  ];

  return (
    <div>
      <DateFilter start={startDate} end={endDate} onChange={handleDateChange} />

      {/* 统计卡片 - 白底+左侧色条 */}
      <Row gutter={[16, 16]}>
        {statCards.map((card, i) => (
          <Col xs={24} sm={12} lg={6} key={i}>
            <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
              <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} />
            </Card>
          </Col>
        ))}
      </Row>

      {/* 每日趋势 */}
      <Card className="bi-card" title={`${title} 每日销售趋势`} style={{ marginTop: 16 }}>
        <ReactECharts lazyUpdate={true} option={trendOption} style={{ height: 340 }} />
      </Card>

      {/* 店铺排行 + 品牌占比 */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={14}>
          <Card className="bi-card" title="店铺销售排行">
            <ReactECharts lazyUpdate={true} option={shopOption} style={{ height: 380 }} />
          </Card>
        </Col>
        <Col xs={24} lg={10}>
          <Card className="bi-card" title="品牌销售占比">
            <ReactECharts lazyUpdate={true} option={brandOption} style={{ height: 380 }} />
          </Card>
        </Col>
      </Row>

      {/* 商品排行 */}
      <Card className="bi-card" title="商品销售排行 TOP15" style={{ marginTop: 16 }}>
        <Table dataSource={indexedGoods} columns={goodsColumns} rowKey="goodsNo" pagination={false} size="small" />
      </Card>
    </div>
  );
};

export default DepartmentPage;

