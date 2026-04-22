import React, { useEffect, useState, useCallback, useMemo, useRef } from 'react';
import { Row, Col, Card, Table, Statistic, Select, Empty, Tabs } from 'antd';
import ReactECharts from '../../components/Chart';
import DateFilter from '../../components/DateFilter';
import PageLoading from '../../components/PageLoading';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../../config';
import { getNiceAxisInterval } from '../../chartTheme';

const formatDate = (d: string) => { const m = d?.match(/\d{4}-(\d{2}-\d{2})/); return m ? m[1] : d; };
const fmtMoney = (v: number) => v >= 10000 ? (v/10000).toFixed(1)+'万' : v?.toLocaleString();

const PLATFORM_TABS = [
  { key: 'all', label: '全部平台' },
  { key: 'tmall', label: '天猫' },
  { key: 'jd', label: '京东' },
  { key: 'pdd', label: '拼多多' },
  { key: 'tmall_cs', label: '天猫超市' },
];

const PLATFORM_SOURCES: Record<string, string> = {
  all: '天猫万象台 + 淘宝联盟 + 京准通 + 京东联盟 + 拼多多推广 + 天猫超市',
  tmall: '天猫万象台（CPC）+ 淘宝联盟（CPS）',
  jd: '京准通（CPC）+ 京东联盟（CPS）',
  pdd: '拼多多商品推广 / 明星店铺 / 直播推广（CPC）',
  tmall_cs: '天猫超市智多星 / 无界场景（CPC）',
};

const MarketingCostPage: React.FC = () => {
  const abortRef = useRef<AbortController | null>(null);
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [platform, setPlatform] = useState('all');
  const [selectedShop, setSelectedShop] = useState('all');
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);

  const fetchData = useCallback((s: string, e: string, plat: string, shop: string) => {
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    const shopParam = shop !== 'all' ? `&shop=${encodeURIComponent(shop)}` : '';
    fetch(`${API_BASE}/api/marketing-cost?start=${s}&end=${e}&platform=${plat}${shopParam}`, { signal: ctrl.signal })
      .then(res => res.json())
      .then(res => { setData(res.data); setLoading(false); })
      .catch((e: any) => { if (e?.name !== 'AbortError') setLoading(false); });
  }, []);

  // 统一 effect: 任何筛选项（平台/店铺/日期）变化都触发 fetch，
  // 不再因为"shop === all"跳过，避免从具体店铺切回全部时不刷新
  useEffect(() => {
    fetchData(startDate, endDate, platform, selectedShop);
  }, [fetchData, startDate, endDate, platform, selectedShop]);

  const cpcDaily = data?.cpcDaily || [];
  const cpsDaily = data?.cpsDaily || [];
  const shopCosts = data?.shopCosts || [];
  const details = data?.details || [];
  const shops = data?.shops || [];
  const hasCps = data?.hasCps || false;
  const dateRange = data?.dateRange || { start: startDate, end: endDate };

  // 判断趋势是否被扩展了（选中范围短时后端自动扩展）
  const isExpanded = data?.trendRange?.start !== dateRange.start;

  // 过滤出原始选中范围内的数据用于汇总指标
  const inRange = (d: string) => d >= dateRange.start && d <= dateRange.end;
  const cpcInRange = cpcDaily.filter((c: any) => inRange(c.date));
  const cpsInRange = cpsDaily.filter((c: any) => inRange(c.date));

  // 汇总（只统计选中日期范围）
  const summary = useMemo(() => {
    const cpcCost = cpcInRange.reduce((s: number, c: any) => s + c.cost, 0);
    const cpcPay = cpcInRange.reduce((s: number, c: any) => s + c.payAmount, 0);
    const cpsCommission = cpsInRange.reduce((s: number, c: any) => s + c.payCommission, 0);
    const cpsPay = cpsInRange.reduce((s: number, c: any) => s + c.payAmount, 0);
    const cpcClicks = cpcInRange.reduce((s: number, c: any) => s + c.clicks, 0);
    return {
      cpcCost, cpcPay,
      cpcROI: cpcCost > 0 ? cpcPay / cpcCost : 0,
      cpsCommission, cpsPay,
      totalCost: cpcCost + cpsCommission,
      cpcClicks,
      avgCpc: cpcClicks > 0 ? cpcCost / cpcClicks : 0,
    };
  }, [cpcInRange, cpsInRange]);
  const statCards = [
    { title: '推广总花费', value: summary.totalCost, precision: 2, prefix: '¥', accentColor: '#ef4444' },
    { title: 'CPC花费', value: summary.cpcCost, precision: 2, prefix: '¥', accentColor: '#f59e0b' },
    { title: 'CPC成交额', value: summary.cpcPay, precision: 2, prefix: '¥', accentColor: '#10b981' },
    { title: 'CPC ROI', value: summary.cpcROI, precision: 2, accentColor: '#1e40af' },
    { title: 'CPC点击数', value: summary.cpcClicks, precision: 0, accentColor: '#7c3aed' },
    { title: '平均CPC', value: summary.avgCpc, precision: 2, prefix: '¥', accentColor: '#06b6d4' },
    ...(hasCps
      ? [
          { title: 'CPS佣金', value: summary.cpsCommission, precision: 2, prefix: '¥', accentColor: '#faad14' },
          { title: 'CPS成交额', value: summary.cpsPay, precision: 2, prefix: '¥', accentColor: '#22c55e' },
        ]
      : []),
  ];

  // CPC趋势（选中范围内高亮，范围外淡色）
  const cpcCostColors = cpcDaily.map((c: any) => inRange(c.date) ? '#ef4444' : 'rgba(255,77,79,0.25)');
  const cpcPayColors = cpcDaily.map((c: any) => inRange(c.date) ? '#10b981' : 'rgba(82,196,26,0.25)');
  const cpcSplits = 5;
  const cpcAmountMax = Math.max(...cpcDaily.flatMap((c: any) => [c.cost || 0, c.payAmount || 0]), 1);
  const cpcRoiMax = Math.max(...cpcDaily.map((c: any) => c.roi || 0), 1);
  const cpcAmountInterval = getNiceAxisInterval(cpcAmountMax, cpcSplits);
  const cpcRoiInterval = getNiceAxisInterval(cpcRoiMax, cpcSplits);
  const cpcTrendOption = {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['CPC花费', 'CPC成交额', 'ROI'], top: 0 },
    grid: { left: 60, right: 40, top: 40, bottom: 70, containLabel: true },
    xAxis: { type: 'category' as const, data: cpcDaily.map((c: any) => formatDate(c.date)),
      axisTick: { alignWithLabel: true }, axisLabel: { rotate: 45, fontSize: 11 } },
    yAxis: [
      { type: 'value' as const, name: '金额', min: 0, max: cpcAmountInterval * cpcSplits, interval: cpcAmountInterval, axisLabel: { formatter: (v: number) => fmtMoney(v) } },
      { type: 'value' as const, name: 'ROI', min: 0, max: cpcRoiInterval * cpcSplits, interval: cpcRoiInterval, position: 'right' as const },
    ],
    series: [
      { name: 'CPC花费', type: 'bar', data: cpcDaily.map((c: any, i: number) => ({ value: c.cost, itemStyle: { color: cpcCostColors[i] } })), barMaxWidth: 8 },
      { name: 'CPC成交额', type: 'bar', data: cpcDaily.map((c: any, i: number) => ({ value: c.payAmount, itemStyle: { color: cpcPayColors[i] } })), barMaxWidth: 8 },
      { name: 'ROI', type: 'line', yAxisIndex: 1, smooth: true, data: cpcDaily.map((c: any) => c.roi),
        itemStyle: { color: '#1e40af' }, lineStyle: { width: 2 } },
    ],
  };

  // CPS趋势
  const cpsBarColors = cpsDaily.map((c: any) => inRange(c.date) ? '#faad14' : 'rgba(250,173,20,0.25)');
  const cpsSplits = 5;
  const cpsAmountMax = Math.max(...cpsDaily.flatMap((c: any) => [c.payAmount || 0, c.payCommission || 0]), 1);
  const cpsAmountInterval = getNiceAxisInterval(cpsAmountMax, cpsSplits);
  const cpsTrendOption = {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['CPS成交额', 'CPS佣金'], top: 0 },
    grid: { left: 80, right: 80, top: 40, bottom: 40 },
    xAxis: { type: 'category' as const, data: cpsDaily.map((c: any) => formatDate(c.date)),
      axisTick: { alignWithLabel: true }, axisLabel: { rotate: 45, fontSize: 11, interval: 0 } },
    yAxis: [{ type: 'value' as const, name: '金额', min: 0, max: cpsAmountInterval * cpsSplits, interval: cpsAmountInterval, axisLabel: { formatter: (v: number) => fmtMoney(v) } }],
    series: [
      { name: 'CPS成交额', type: 'bar', data: cpsDaily.map((c: any, i: number) => ({ value: c.payAmount, itemStyle: { color: cpsBarColors[i] } })), barWidth: 10 },
      { name: 'CPS佣金', type: 'line', smooth: true, data: cpsDaily.map((c: any) => c.payCommission),
        itemStyle: { color: '#ef4444' }, areaStyle: { color: 'rgba(255,77,79,0.1)' } },
    ],
  };

  // 店铺花费对比
  const shopSplits = 5;
  const shopAmountMax = Math.max(...shopCosts.flatMap((s: any) => [s.cost || 0, s.payAmount || 0]), 1);
  const shopAmountInterval = getNiceAxisInterval(shopAmountMax, shopSplits);
  const shopCostOption = {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['花费', '成交额'], top: 0 },
    grid: { left: 140, right: 40, top: 40, bottom: 20 },
    yAxis: { type: 'category' as const, data: shopCosts.map((s: any) => s.shopName), inverse: true },
    xAxis: { type: 'value' as const, min: 0, max: shopAmountInterval * shopSplits, interval: shopAmountInterval, axisLabel: { formatter: (v: number) => fmtMoney(v) } },
    series: [
      { name: '花费', type: 'bar', data: shopCosts.map((s: any) => s.cost), itemStyle: { color: '#ef4444' } },
      { name: '成交额', type: 'bar', data: shopCosts.map((s: any) => s.payAmount), itemStyle: { color: '#10b981' } },
    ],
  };

  // 明细占比饼图
  const detailPieOption = {
    tooltip: { trigger: 'item' as const, formatter: '{b}: ¥{c} ({d}%)' },
    legend: { bottom: 0, type: 'scroll' as const },
    series: [{
      type: 'pie', radius: ['35%', '65%'],
      label: { show: true, formatter: '{b}\n{d}%', fontSize: 11, lineHeight: 15 },
      data: details.filter((d: any) => d.cost > 0).map((d: any) => ({ value: d.cost, name: d.name || '未知' })),
    }],
  };

  if (loading) return <PageLoading />;

  return (
    <div>
      <DateFilter start={startDate} end={endDate} onChange={(s, e) => { setStartDate(s); setEndDate(e); }} />

      {/* 平台Tab + 店铺筛选 */}
      <Card className="bi-filter-card" style={{ marginBottom: 16 }}>
        <Tabs
          activeKey={platform}
          onChange={p => { setPlatform(p); setSelectedShop('all'); }}
          items={PLATFORM_TABS.map(t => ({ key: t.key, label: t.label }))}
          style={{ marginBottom: 12 }}
        />
        <Row align="middle" gutter={16}>
          {platform !== 'all' && (
            <Col>
              <span style={{ fontWeight: 500, marginRight: 8 }}>店铺筛选：</span>
              <Select
                value={selectedShop}
                onChange={setSelectedShop}
                style={{ width: 350 }}
                options={[
                  { value: 'all', label: '全部店铺' },
                  ...shops.map((s: string) => ({ value: s, label: s })),
                ]}
              />
            </Col>
          )}
          <Col style={{ color: '#999', fontSize: 13 }}>
            数据来源：{PLATFORM_SOURCES[platform] || ''}
          </Col>
        </Row>
      </Card>

      {(cpcDaily.length === 0 && cpsDaily.length === 0) ? (
        <Card><Empty description="暂无营销费用数据" /></Card>
      ) : (
        <>
          {/* 汇总指标 */}
          <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
            {statCards.map((card) => {
              const hint = (card.prefix === '¥' && card.value >= 10000) ? `≈ ${(card.value / 10000).toFixed(1)}万` : '';
              return (
                <Col xs={12} sm={6} md={3} key={card.title}>
                  <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                    <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} />
                    <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>{hint || ' '}</div>
                  </Card>
                </Col>
              );
            })}
          </Row>

          {/* CPC趋势 + CPS趋势 */}
          {isExpanded && <div style={{ color: '#999', fontSize: 12, marginBottom: 8 }}>深色柱为选中日期，浅色柱为趋势参考</div>}
          <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
            <Col xs={24} lg={hasCps ? 12 : 24}>
              <Card title="CPC每日趋势">
                <ReactECharts option={cpcTrendOption} lazyUpdate={true} style={{ height: 320 }} />
              </Card>
            </Col>
            {hasCps && cpsDaily.length > 0 && (
              <Col xs={24} lg={12}>
                <Card title="CPS每日趋势">
                  <ReactECharts option={cpsTrendOption} lazyUpdate={true} style={{ height: 320 }} />
                </Card>
              </Col>
            )}
          </Row>

          {/* 店铺对比 + 明细占比 */}
          <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
            <Col xs={24} lg={14}>
              <Card title="各店铺推广投入对比">
                {shopCosts.length > 0 ? (
                  <ReactECharts option={shopCostOption} lazyUpdate={true} style={{ height: Math.max(200, shopCosts.length * 60) }} />
                ) : <Empty description="暂无数据" />}
              </Card>
            </Col>
            <Col xs={24} lg={10}>
              <Card title="推广类型花费占比">
                {details.length > 0 ? (
                  <ReactECharts option={detailPieOption} lazyUpdate={true} style={{ height: 320 }} />
                ) : <Empty description="暂无数据" />}
              </Card>
            </Col>
          </Row>

          {/* 明细表格 */}
          <Row gutter={[16, 16]}>
            <Col xs={24} lg={hasCps ? 12 : 24}>
              <Card className="bi-table-card" title="CPC推广明细">
                <Table
                  dataSource={details}
                  rowKey={(r: any) => `${r.platform}-${r.name}`}
                  size="small"
                  pagination={false}
                  columns={[
                    { title: '平台', dataIndex: 'platform', key: 'platform', width: 80,
                      filters: Array.from(new Set(details.map((d: any) => d.platform))).map((p: any) => ({ text: p, value: p })),
                      onFilter: (value: any, record: any) => record.platform === value },
                    { title: '推广类型/场景', dataIndex: 'name', key: 'name' },
                    { title: '花费', dataIndex: 'cost', key: 'cost', sorter: (a: any, b: any) => a.cost - b.cost,
                      render: (v: number) => `¥${v?.toLocaleString()}` },
                    { title: '成交额', dataIndex: 'payAmount', key: 'payAmount',
                      render: (v: number) => `¥${v?.toLocaleString()}` },
                    { title: 'ROI', dataIndex: 'roi', key: 'roi', render: (v: number) => v?.toFixed(2) },
                    { title: '点击量', dataIndex: 'clicks', key: 'clicks', render: (v: number) => v ? v.toLocaleString() : '-' },
                    { title: '平均CPC', dataIndex: 'avgCpc', key: 'avgCpc', render: (v: number) => v ? `¥${v.toFixed(2)}` : '-' },
                  ]}
                />
              </Card>
            </Col>
            {hasCps && cpsDaily.length > 0 && (
              <Col xs={24} lg={12}>
                <Card className="bi-table-card" title="CPS佣金汇总">
                  <Table
                    dataSource={(() => {
                      const totalAmt = cpsDaily.reduce((s: number, c: any) => s + c.payAmount, 0);
                      const totalComm = cpsDaily.reduce((s: number, c: any) => s + c.payCommission, 0);
                      const totalUsers = cpsDaily.reduce((s: number, c: any) => s + c.payUsers, 0);
                      return [{ key: 'total', name: 'CPS合计', payAmount: totalAmt, payCommission: totalComm, payUsers: totalUsers }];
                    })()}
                    rowKey="key"
                    size="small"
                    pagination={false}
                    columns={[
                      { title: '类型', dataIndex: 'name', key: 'name' },
                      { title: '成交额', dataIndex: 'payAmount', key: 'payAmount', render: (v: number) => `¥${v?.toLocaleString()}` },
                      { title: '佣金支出', dataIndex: 'payCommission', key: 'payCommission', render: (v: number) => `¥${v?.toLocaleString()}` },
                      { title: '人数', dataIndex: 'payUsers', key: 'payUsers' },
                      { title: '佣金率', key: 'rate', render: (_: any, r: any) => r.payAmount > 0 ? `${(r.payCommission/r.payAmount*100).toFixed(2)}%` : '-' },
                    ]}
                  />
                </Card>
              </Col>
            )}
          </Row>
        </>
      )}
    </div>
  );
};

export default MarketingCostPage;
