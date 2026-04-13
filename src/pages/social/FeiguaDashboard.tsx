import React, { useEffect, useState, useCallback } from 'react';
import { Row, Col, Card, Table, Statistic, Tabs, Empty } from 'antd';
import ReactECharts from '../../components/Chart';
import DateFilter from '../../components/DateFilter';
import PageLoading from '../../components/PageLoading';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../../config';
import { CHART_COLORS, barItemStyle, getBaseOption, pieStyle } from '../../chartTheme';

const formatDate = (d: string) => { const m = d?.match(/\d{4}-(\d{2}-\d{2})/); return m ? m[1] : d; };
const fmtMoney = (v: number) => v >= 10000 ? (v/10000).toFixed(1)+'万' : v?.toLocaleString();

const PLATFORM_TABS = [
  { key: 'all', label: '全部平台' },
  { key: '抖音', label: '抖音' },
  { key: '快手', label: '快手' },
  { key: '小红书', label: '小红书' },
];

const PLATFORM_COLORS: Record<string, string> = {
  '抖音': '#ef4444',
  '快手': '#faad14',
  '小红书': '#ff6b81',
};

const FeiguaDashboard: React.FC = () => {
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [platform, setPlatform] = useState('all');
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);

  const fetchData = useCallback((s: string, e: string, plat: string) => {
    setLoading(true);
    const platParam = plat !== 'all' ? `&platform=${encodeURIComponent(plat)}` : '';
    fetch(`${API_BASE}/api/feigua?start=${s}&end=${e}${platParam}`)
      .then(res => res.json())
      .then(res => { setData(res.data); setLoading(false); })
      .catch(() => setLoading(false));
  }, []);

  useEffect(() => { fetchData(startDate, endDate, platform); }, [fetchData, startDate, endDate, platform]);

  if (loading) return <PageLoading />;
  if (!data) return <div>加载失败</div>;

  const { summary, dailyGmv, creators, followers, platforms: platShares, roster, dateRange } = data;
  const isExpanded = data.dateRange && dateRange.start !== startDate;
  const inRange = (d: string) => d >= startDate && d <= endDate;
  const baseOpt = getBaseOption();

  // 达人资源总数
  const totalRoster = (roster || []).reduce((s: number, r: any) => s + r.total, 0);
  const totalConnected = (roster || []).reduce((s: number, r: any) => s + r.connected, 0);

  // GMV趋势图 — 按平台堆叠
  const allDates: string[] = (Array.from(new Set((dailyGmv || []).map((d: any) => d.date))) as string[]).sort();
  const platList = platform === 'all' ? ['抖音', '快手', '小红书'] : [platform];

  const gmvTrendOption = {
    ...baseOpt,
    legend: { ...baseOpt.legend, data: platList, top: 4 },
    grid: { left: 80, right: 40, top: 48, bottom: 32 },
    xAxis: {
      ...baseOpt.xAxis,
      type: 'category' as const,
      data: allDates.map(formatDate),
      axisTick: { alignWithLabel: true },
      axisLabel: { ...baseOpt.xAxis.axisLabel, interval: 0, rotate: allDates.length > 15 ? 45 : 0 },
    },
    yAxis: {
      ...baseOpt.yAxis,
      type: 'value' as const,
      name: 'GMV',
      axisLabel: { ...baseOpt.yAxis.axisLabel, formatter: (v: number) => fmtMoney(v) },
    },
    series: platList.map((p) => ({
      name: p,
      type: 'bar',
      stack: 'gmv',
      barMaxWidth: 20,
      data: allDates.map((date) => {
        const item = (dailyGmv || []).find((d: any) => d.date === date && d.platform === p);
        const val = item ? item.gmv : 0;
        return { value: val, itemStyle: { color: isExpanded && !inRange(date) ? (PLATFORM_COLORS[p] || '#999') + '40' : PLATFORM_COLORS[p] || '#999' } };
      }),
    })),
  };

  // 平台占比饼图
  const platPieOption = {
    ...pieStyle,
    color: CHART_COLORS,
    legend: { ...pieStyle.legend, bottom: 0 },
    series: [{
      type: 'pie',
      radius: ['35%', '65%'],
      label: { show: true, formatter: '{b}\n{d}%', fontSize: 12, lineHeight: 16, color: '#64748b' },
      labelLine: { length: 15, length2: 10, lineStyle: { color: '#cbd5e1' } },
      itemStyle: { borderColor: '#fff', borderWidth: 2, borderRadius: 4 },
      data: (platShares || []).map((p: any) => ({
        value: p.gmv,
        name: p.platform,
        itemStyle: { color: PLATFORM_COLORS[p.platform] },
      })),
    }],
  };

  // 跟进人业绩柱状图
  const followerOption = {
    ...baseOpt,
    grid: { left: 80, right: 60, top: 10, bottom: 20 },
    xAxis: { ...baseOpt.xAxis, type: 'value' as const, show: false },
    yAxis: {
      ...baseOpt.yAxis,
      type: 'category' as const,
      data: (followers || []).map((f: any) => f.follower).reverse(),
      axisLabel: { ...baseOpt.yAxis.axisLabel, fontSize: 11 },
    },
    series: [{
      type: 'bar',
      barWidth: 14,
      data: (followers || []).map((f: any) => f.gmv).reverse(),
      ...barItemStyle('#4f46e5'),
      label: {
        show: true,
        position: 'right',
        fontSize: 10,
        formatter: (p: any) => p.value >= 10000 ? `${(p.value / 10000).toFixed(1)}万` : `¥${p.value}`,
      },
    }],
  };

  // 达人排行表
  const creatorColumns = [
    { title: '#', key: 'rank', width: 40, render: (_: any, __: any, i: number) => {
      const rank = i + 1;
      return <span style={{ color: rank <= 3 ? '#4f46e5' : '#94a3b8', fontWeight: rank <= 3 ? 700 : 400 }}>{rank}</span>;
    } },
    { title: '达人昵称', dataIndex: 'creatorName', key: 'creatorName', ellipsis: true, width: 150 },
    { title: '平台', dataIndex: 'platform', key: 'platform', width: 70,
      filters: [{ text: '抖音', value: '抖音' }, { text: '快手', value: '快手' }, { text: '小红书', value: '小红书' }],
      onFilter: (value: any, record: any) => record.platform === value,
      render: (v: string) => <span style={{ color: PLATFORM_COLORS[v] || '#999' }}>{v}</span> },
    { title: 'GMV', dataIndex: 'gmv', key: 'gmv', width: 110, sorter: (a: any, b: any) => a.gmv - b.gmv,
      render: (v: number) => `¥${v?.toLocaleString()}` },
    { title: '订单数', dataIndex: 'orders', key: 'orders', width: 70 },
    { title: '佣金', dataIndex: 'commission', key: 'commission', width: 100,
      render: (v: number) => `¥${v?.toLocaleString()}` },
    { title: '出单商品', dataIndex: 'products', key: 'products', width: 80 },
    { title: '跟进人', dataIndex: 'follower', key: 'follower', width: 80 },
  ];

  // 跟进人业绩表
  const followerColumns = [
    { title: '#', key: 'rank', width: 40, render: (_: any, __: any, i: number) => {
      const rank = i + 1;
      return <span style={{ color: rank <= 3 ? '#4f46e5' : '#94a3b8', fontWeight: rank <= 3 ? 700 : 400 }}>{rank}</span>;
    } },
    { title: '跟进人', dataIndex: 'follower', key: 'follower', width: 100 },
    { title: 'GMV', dataIndex: 'gmv', key: 'gmv', width: 110, sorter: (a: any, b: any) => a.gmv - b.gmv,
      render: (v: number) => `¥${v?.toLocaleString()}` },
    { title: '订单数', dataIndex: 'orders', key: 'orders', width: 70 },
    { title: '达人数', dataIndex: 'creatorCount', key: 'creatorCount', width: 70 },
    { title: '佣金', dataIndex: 'commission', key: 'commission', width: 100,
      render: (v: number) => `¥${v?.toLocaleString()}` },
  ];

  const statCards = [
    { title: '总GMV', value: summary?.totalGmv || 0, precision: 2, prefix: '¥', accentColor: '#ef4444' },
    { title: '成交订单数', value: summary?.totalOrders || 0, accentColor: '#4f46e5' },
    { title: '出单达人数', value: summary?.totalCreators || 0, accentColor: '#10b981' },
    { title: '佣金支出', value: summary?.commission || 0, precision: 2, prefix: '¥', accentColor: '#f59e0b' },
    { title: '达人资源总数', value: totalRoster, accentColor: '#8b5cf6' },
    { title: '已建联达人', value: totalConnected, accentColor: '#06b6d4' },
  ];

  return (
    <div>
      <DateFilter start={startDate} end={endDate} onChange={(s, e) => { setStartDate(s); setEndDate(e); }} />

      <Card className="bi-filter-card" style={{ marginBottom: 16 }}>
        <Tabs
          activeKey={platform}
          onChange={setPlatform}
          items={PLATFORM_TABS.map(t => ({ key: t.key, label: t.label }))}
        />
      </Card>

      {/* 汇总指标 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        {statCards.map((card) => (
          <Col xs={12} sm={4} key={card.title}>
            <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
              <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} />
            </Card>
          </Col>
        ))}
      </Row>

      {/* GMV趋势 + 平台占比 */}
      {isExpanded && <div style={{ color: '#999', fontSize: 12, marginBottom: 8 }}>深色柱为选中日期，浅色柱为趋势参考</div>}
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={24} lg={16}>
          <Card title="GMV每日趋势">
            {allDates.length > 0 ? <ReactECharts option={gmvTrendOption} lazyUpdate={true} style={{ height: 350 }} /> : <Empty description="暂无数据" />}
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card title="平台GMV占比">
            {(platShares || []).length > 0 ? <ReactECharts option={platPieOption} lazyUpdate={true} style={{ height: 350 }} /> : <Empty description="暂无数据" />}
          </Card>
        </Col>
      </Row>

      {/* 跟进人业绩 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={24} lg={12}>
          <Card title="跟进人业绩排行（GMV）">
            {(followers || []).length > 0 ?
              <ReactECharts option={followerOption} lazyUpdate={true} style={{ height: Math.max(250, (followers || []).length * 28) }} /> :
              <Empty description="暂无数据" />}
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card className="bi-table-card" title="跟进人业绩明细">
            <Table dataSource={followers || []} columns={followerColumns} rowKey="follower" size="small" pagination={false} />
          </Card>
        </Col>
      </Row>

      {/* 达人出单排行 */}
      <Card className="bi-table-card" title="达人出单排行 TOP20">
        <Table dataSource={creators || []} columns={creatorColumns} rowKey={(r: any) => `${r.platform}-${r.creatorName}`} size="small" pagination={false} />
      </Card>
    </div>
  );
};

export default FeiguaDashboard;
