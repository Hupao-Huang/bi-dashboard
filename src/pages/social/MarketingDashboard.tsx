import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Row, Col, Card, Statistic, Select, Table, Tabs, Empty } from 'antd';
import {
  VideoCameraOutlined,
  TeamOutlined,
  EyeOutlined,
} from '@ant-design/icons';
import ReactECharts from '../../components/Chart';
import DateFilter from '../../components/DateFilter';
import PageLoading from '../../components/PageLoading';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../../config';
import { CHART_COLORS, getBaseOption, getNiceAxisInterval, lineAreaStyle, pieStyle } from '../../chartTheme';

const fmtMoney = (v: number) => v >= 10000 ? `¥${(v / 10000).toFixed(1)}万` : `¥${v?.toLocaleString()}`;

const MarketingDashboard: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [douyinData, setDouyinData] = useState<any>(null);
  const [distData, setDistData] = useState<any>(null);
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);
  const [tab, setTab] = useState('douyin');
  const [selectedPlan, setSelectedPlan] = useState('all');
  const baseOpt = useMemo(() => getBaseOption(), []);

  const fetchDist = useCallback((s: string, e: string, account: string) => {
    const acctParam = account && account !== 'all' ? `&account=${encodeURIComponent(account)}` : '';
    fetch(`${API_BASE}/api/douyin-dist/ops?start=${s}&end=${e}${acctParam}`, { credentials: 'include' })
      .then(r => r.json())
      .then(res => setDistData(res.data || res))
      .catch(() => {});
  }, []);

  useEffect(() => {
    setLoading(true);
    Promise.all([
      fetch(`${API_BASE}/api/douyin/ops?start=${startDate}&end=${endDate}`, { credentials: 'include' }).then(r => r.json()),
      fetch(`${API_BASE}/api/douyin-dist/ops?start=${startDate}&end=${endDate}`, { credentials: 'include' }).then(r => r.json()),
    ]).then(([dy, dist]) => {
      setDouyinData(dy.data || dy);
      setDistData(dist.data || dist);
      setLoading(false);
    }).catch(() => setLoading(false));
  }, [startDate, endDate]);

  useEffect(() => {
    if (selectedPlan !== 'all') fetchDist(startDate, endDate, selectedPlan);
  }, [fetchDist, selectedPlan, startDate, endDate]);

  if (loading && !douyinData) return <PageLoading />;

  // ========== 抖音自营数据 ==========
  const liveTrend = douyinData?.liveTrend || [];
  const goodsTop = douyinData?.goodsTop || [];
  const anchors = douyinData?.anchors || [];
  const channels = douyinData?.channels || [];
  const funnel = douyinData?.funnel || [];
  const adTrend = douyinData?.adTrend || [];
  const adDetails = douyinData?.adDetails || [];
  const dateRange = douyinData?.dateRange || { start: startDate, end: endDate };
  const inRange = (d: string) => d >= dateRange.start && d <= dateRange.end;
  const liveInRange = liveTrend.filter((d: any) => inRange(d.date));

  const liveSummary = {
    sessions: liveInRange.reduce((s: number, d: any) => s + d.sessions, 0),
    watchUV: liveInRange.reduce((s: number, d: any) => s + d.watchUV, 0),
    payAmount: liveInRange.reduce((s: number, d: any) => s + d.payAmount, 0),
    avgOnline: liveInRange.length > 0 ? Math.round(liveInRange.reduce((s: number, d: any) => s + d.avgOnline, 0) / liveInRange.length) : 0,
    refundRate: liveInRange.length > 0 ? +(liveInRange.reduce((s: number, d: any) => s + d.refundRate, 0) / liveInRange.length).toFixed(2) : 0,
  };

  const liveSplits = 5;
  const livePayMax = Math.max(...liveTrend.map((d: any) => d.payAmount || 0), 1);
  const livePayInterval = getNiceAxisInterval(livePayMax, liveSplits);
  const liveWatchMax = Math.max(...liveTrend.map((d: any) => d.watchUV || 0), 1);
  const liveWatchInterval = getNiceAxisInterval(liveWatchMax, liveSplits);

  const liveTrendOption = {
    ...baseOpt,
    legend: { ...baseOpt.legend, data: ['成交金额', '观看人数'], top: 4 },
    grid: { left: 60, right: 60, top: 48, bottom: 40 },
    xAxis: {
      ...baseOpt.xAxis,
      type: 'category' as const,
      data: liveTrend.map((d: any) => d.date?.slice(5)),
      axisLabel: { ...baseOpt.xAxis.axisLabel, rotate: 45, interval: 0 },
    },
    yAxis: [
      {
        ...baseOpt.yAxis,
        type: 'value' as const,
        name: '金额',
        min: 0,
        max: livePayInterval * liveSplits,
        interval: livePayInterval,
        axisLabel: { ...baseOpt.yAxis.axisLabel, formatter: (v: number) => fmtMoney(v) },
      },
      {
        ...baseOpt.yAxis,
        type: 'value' as const,
        name: '人数',
        min: 0,
        max: liveWatchInterval * liveSplits,
        interval: liveWatchInterval,
        position: 'right' as const,
      },
    ],
    series: [
      {
        name: '成交金额',
        type: 'bar',
        data: liveTrend.map((d: any) => ({ value: d.payAmount, itemStyle: { color: inRange(d.date) ? '#ef4444' : 'rgba(239,68,68,0.25)' } })),
        barMaxWidth: 12,
      },
      {
        name: '观看人数',
        type: 'line',
        yAxisIndex: 1,
        smooth: true,
        data: liveTrend.map((d: any) => d.watchUV),
        ...lineAreaStyle('#4f46e5'),
        symbol: 'circle',
        symbolSize: 4,
      },
    ],
  };

  // ========== 抖音分销数据 ==========
  const distPlans = distData?.plans || [];
  const distTrend = distData?.distTrend || [];
  const accountRank = distData?.accountRank || [];
  const productRank = distData?.productRank || [];
  const distRange = distData?.dateRange || { start: startDate, end: endDate };
  const inDistRange = (d: string) => d >= distRange.start && d <= distRange.end;
  const distInRange = distTrend.filter((d: any) => inDistRange(d.date));

  // 从店铺名中提取简称（取第一个"-"前面的部分）
  const shortPlanName = (name: string) => name?.split('-')[0] || name;

  const distSummary = {
    cost: distInRange.reduce((s: number, d: any) => s + d.cost, 0),
    payAmount: distInRange.reduce((s: number, d: any) => s + d.payAmount, 0),
    roi: 0 as number,
    talents: distPlans.reduce((s: number, p: any) => s + (p.talents || 0), 0),
  };
  distSummary.roi = distSummary.cost > 0 ? +(distSummary.payAmount / distSummary.cost).toFixed(2) : 0;

  const distSplits = 5;
  const distCostMax = Math.max(...distTrend.map((d: any) => Math.max(d.cost || 0, d.payAmount || 0)), 1);
  const distCostInterval = getNiceAxisInterval(distCostMax, distSplits);
  const distRoiMax = Math.max(...distTrend.map((d: any) => d.roi || 0), 1);
  const distRoiInterval = getNiceAxisInterval(distRoiMax, distSplits);

  const distTrendOption = {
    ...baseOpt,
    legend: { ...baseOpt.legend, data: ['消耗', '成交金额', 'ROI'], top: 4 },
    grid: { left: 60, right: 60, top: 48, bottom: 40 },
    xAxis: {
      ...baseOpt.xAxis,
      type: 'category' as const,
      data: distTrend.map((d: any) => d.date?.slice(5)),
      axisLabel: { ...baseOpt.xAxis.axisLabel, rotate: 45, interval: 0 },
    },
    yAxis: [
      {
        ...baseOpt.yAxis,
        type: 'value' as const,
        name: '金额',
        min: 0,
        max: distCostInterval * distSplits,
        interval: distCostInterval,
        axisLabel: { ...baseOpt.yAxis.axisLabel, formatter: (v: number) => fmtMoney(v) },
      },
      {
        ...baseOpt.yAxis,
        type: 'value' as const,
        name: 'ROI',
        min: 0,
        max: distRoiInterval * distSplits,
        interval: distRoiInterval,
        position: 'right' as const,
      },
    ],
    series: [
      {
        name: '消耗',
        type: 'bar',
        data: distTrend.map((d: any) => ({ value: d.cost, itemStyle: { color: inDistRange(d.date) ? '#f59e0b' : 'rgba(245,158,11,0.25)' } })),
        barMaxWidth: 10,
      },
      {
        name: '成交金额',
        type: 'bar',
        data: distTrend.map((d: any) => ({ value: d.payAmount, itemStyle: { color: inDistRange(d.date) ? '#10b981' : 'rgba(16,185,129,0.25)' } })),
        barMaxWidth: 10,
      },
      {
        name: 'ROI',
        type: 'line',
        yAxisIndex: 1,
        smooth: true,
        data: distTrend.map((d: any) => d.roi),
        ...lineAreaStyle('#4f46e5'),
        symbol: 'circle',
        symbolSize: 4,
      },
    ],
  };

  const liveStatCards = [
    { title: '直播场次', value: liveSummary.sessions, prefix: <VideoCameraOutlined />, accentColor: '#ef4444' },
    { title: '观看人数', value: liveSummary.watchUV, prefix: <EyeOutlined />, accentColor: '#4f46e5' },
    { title: '成交金额', value: liveSummary.payAmount, precision: 0, prefix: '¥', accentColor: '#10b981' },
    { title: '平均在线', value: liveSummary.avgOnline, prefix: <TeamOutlined />, accentColor: '#8b5cf6' },
    { title: '退款率', value: liveSummary.refundRate, suffix: '%', accentColor: liveSummary.refundRate > 10 ? '#ef4444' : '#06b6d4' },
  ];

  const distStatCards = [
    { title: '总消耗', value: distSummary.cost, precision: 2, prefix: '¥', accentColor: '#f59e0b' },
    { title: '总成交额', value: distSummary.payAmount, precision: 2, prefix: '¥', accentColor: '#10b981' },
    { title: '整体ROI', value: distSummary.roi, precision: 2, accentColor: '#4f46e5' },
    { title: '达人总数', value: distSummary.talents, prefix: <TeamOutlined />, accentColor: '#8b5cf6' },
  ];

  return (
    <div>
      <DateFilter start={startDate} end={endDate} onChange={(s, e) => { setStartDate(s); setEndDate(e); }} />

      <Card className="bi-filter-card" style={{ marginBottom: 16 }}>
        <Tabs activeKey={tab} onChange={setTab} items={[
          { key: 'douyin', label: '抖音自营' },
          { key: 'dist', label: '抖音分销' },
        ]} />
      </Card>

      {tab === 'douyin' && (
        <>
          {liveTrend.length === 0 && goodsTop.length === 0 ? (
            <Card><Empty description="暂无抖音自营数据" /></Card>
          ) : (
            <>
              {/* KPI卡片 + 推广汇总 */}
              <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
                {liveStatCards.map((card) => (
                  <Col xs={12} sm={6} lg={4} key={card.title}>
                    <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                      <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
                    </Card>
                  </Col>
                ))}
              </Row>

              {/* 直播趋势 + 转化漏斗 */}
              <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
                {liveTrend.length > 0 && (
                  <Col xs={24} lg={funnel.length > 0 ? 14 : 24}>
                    <Card title="每日直播趋势">
                      <ReactECharts option={liveTrendOption} lazyUpdate={true} style={{ height: 320 }} />
                    </Card>
                  </Col>
                )}
                {funnel.length > 0 && (
                  <Col xs={24} lg={10}>
                    <Card title="转化漏斗（最新一天）">
                      <ReactECharts lazyUpdate={true} style={{ height: 320 }} option={{
                        tooltip: { trigger: 'item', formatter: '{b}: {c}' },
                        series: [{
                          type: 'funnel',
                          left: '5%', width: '90%', top: 10, bottom: 10,
                          min: 0,
                          max: Math.max(...funnel.map((f: any) => f.stepValue), 1),
                          sort: 'descending',
                          gap: 2,
                          label: { show: true, position: 'inside', formatter: (p: any) => `${p.name}\n${p.value?.toLocaleString()}`, fontSize: 11, lineHeight: 15 },
                          data: funnel.map((f: any) => ({ value: f.stepValue, name: f.stepName })),
                          itemStyle: { borderWidth: 0 },
                        }],
                        color: CHART_COLORS,
                      }} />
                    </Card>
                  </Col>
                )}
              </Row>

              {/* 商品TOP10 + 主播排行 */}
              <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
                {goodsTop.length > 0 && (
                  <Col xs={24} lg={14}>
                    <Card className="bi-table-card" title="商品销售TOP10">
                      <Table dataSource={goodsTop} rowKey="productName" size="small" pagination={false}
                        columns={[
                          { title: '商品', dataIndex: 'productName', key: 'name', ellipsis: true, width: 200 },
                          { title: '支付金额', dataIndex: 'payAmount', key: 'amt', render: (v: number) => `¥${v?.toLocaleString()}`, sorter: (a: any, b: any) => a.payAmount - b.payAmount, defaultSortOrder: 'descend' as const },
                          { title: '支付件数', dataIndex: 'payQty', key: 'qty' },
                          { title: '转化率', dataIndex: 'convRate', key: 'conv', render: (v: number) => `${v}%` },
                        ]}
                      />
                    </Card>
                  </Col>
                )}
                {anchors.length > 0 && (
                  <Col xs={24} lg={10}>
                    <Card className="bi-table-card" title="主播排行">
                      <Table dataSource={anchors} rowKey="anchorName" size="small" pagination={false} scroll={{ y: 300 }}
                        columns={[
                          { title: '主播', dataIndex: 'anchorName', key: 'name', ellipsis: true },
                          { title: '场次', dataIndex: 'sessions', key: 'sessions', width: 55 },
                          { title: '成交金额', dataIndex: 'payAmount', key: 'amt', render: (v: number) => `¥${v?.toLocaleString()}`, sorter: (a: any, b: any) => a.payAmount - b.payAmount, defaultSortOrder: 'descend' as const },
                          { title: '最高在线', dataIndex: 'maxOnline', key: 'max', width: 75 },
                        ]}
                      />
                    </Card>
                  </Col>
                )}
              </Row>

              {/* 渠道分布 */}
              {channels.length > 0 && (
                <Card title="流量渠道分布" style={{ marginBottom: 16 }}>
                  <ReactECharts lazyUpdate={true} style={{ height: 300 }} option={{
                    ...pieStyle,
                    color: CHART_COLORS,
                    tooltip: { ...pieStyle.tooltip, trigger: 'item' as const, formatter: (p: any) => `${p.name}: ¥${p.value?.toLocaleString()} (${p.percent}%)` },
                    legend: { ...pieStyle.legend, right: 10, top: 'middle', orient: 'vertical', type: 'scroll' as const },
                    series: [{
                      type: 'pie', radius: ['30%', '60%'], center: ['35%', '50%'],
                      label: { show: true, formatter: '{b}\n{d}%', fontSize: 11, lineHeight: 14, color: '#64748b' },
                      labelLine: { length: 14, length2: 10, lineStyle: { color: '#cbd5e1' } },
                      itemStyle: { borderColor: '#fff', borderWidth: 2, borderRadius: 4 },
                      data: channels.filter((c: any) => c.payAmt > 0).map((c: any) => ({ value: c.payAmt, name: c.channelName })),
                    }],
                  }} />
                </Card>
              )}

              {/* 推广直播间投放（独立区域） */}
              {(adTrend.length > 0 || adDetails.length > 0) && (() => {
                const adInRange = adTrend.filter((d: any) => inRange(d.date));
                const totalCost = adInRange.reduce((s: number, d: any) => s + d.cost, 0);
                const totalPay = adInRange.reduce((s: number, d: any) => s + d.payAmount, 0);
                const totalNet = adInRange.reduce((s: number, d: any) => s + d.netAmount, 0);
                const overallROI = totalCost > 0 ? +(totalPay / totalCost).toFixed(2) : 0;
                const netROI = totalCost > 0 ? +(totalNet / totalCost).toFixed(2) : 0;

                const adSplits = 5;
                const adMax = Math.max(...adTrend.map((d: any) => Math.max(d.cost || 0, d.payAmount || 0)), 1);
                const adInterval = getNiceAxisInterval(adMax, adSplits);
                const roiMax = Math.max(...adTrend.map((d: any) => Math.max(d.roi || 0, d.netROI || 0)), 1);
                const roiInterval = getNiceAxisInterval(roiMax, adSplits);

                return (
                  <>
                    <Card title="推广直播间投放" style={{ marginBottom: 16 }}
                      headStyle={{ background: 'linear-gradient(90deg, #fef3c7 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
                      <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
                        {[
                          { title: '推广消耗', value: totalCost, precision: 2, prefix: '¥', accentColor: '#f59e0b' },
                          { title: '成交金额', value: totalPay, precision: 2, prefix: '¥', accentColor: '#10b981' },
                          { title: 'ROI', value: overallROI, precision: 2, accentColor: '#4f46e5' },
                          { title: '净成交', value: totalNet, precision: 2, prefix: '¥', accentColor: '#06b6d4' },
                          { title: '净ROI', value: netROI, precision: 2, accentColor: '#8b5cf6' },
                        ].map((card) => (
                          <Col xs={12} sm={4} key={card.title}>
                            <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                              <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} />
                            </Card>
                          </Col>
                        ))}
                      </Row>
                      <ReactECharts lazyUpdate={true} style={{ height: 300 }} option={{
                        ...baseOpt,
                        legend: { ...baseOpt.legend, data: ['消耗', '成交金额', 'ROI', '净ROI'], top: 4 },
                        grid: { left: 60, right: 60, top: 48, bottom: 40 },
                        xAxis: {
                          ...baseOpt.xAxis,
                          type: 'category' as const,
                          data: adTrend.map((d: any) => d.date?.slice(5)),
                          axisLabel: { ...baseOpt.xAxis.axisLabel, rotate: 45, interval: 0 },
                        },
                        yAxis: [
                          {
                            ...baseOpt.yAxis,
                            type: 'value' as const,
                            name: '金额',
                            min: 0,
                            max: adInterval * adSplits,
                            interval: adInterval,
                            axisLabel: { ...baseOpt.yAxis.axisLabel, formatter: (v: number) => fmtMoney(v) },
                          },
                          {
                            ...baseOpt.yAxis,
                            type: 'value' as const,
                            name: 'ROI',
                            min: 0,
                            max: roiInterval * adSplits,
                            interval: roiInterval,
                            position: 'right' as const,
                          },
                        ],
                        series: [
                          { name: '消耗', type: 'bar', data: adTrend.map((d: any) => ({ value: d.cost, itemStyle: { color: inRange(d.date) ? '#f59e0b' : 'rgba(245,158,11,0.25)' } })), barMaxWidth: 8 },
                          { name: '成交金额', type: 'bar', data: adTrend.map((d: any) => ({ value: d.payAmount, itemStyle: { color: inRange(d.date) ? '#10b981' : 'rgba(16,185,129,0.25)' } })), barMaxWidth: 8 },
                          { name: 'ROI', type: 'line', yAxisIndex: 1, smooth: true, data: adTrend.map((d: any) => d.roi), ...lineAreaStyle('#4f46e5'), symbol: 'circle', symbolSize: 4 },
                          { name: '净ROI', type: 'line', yAxisIndex: 1, smooth: true, data: adTrend.map((d: any) => d.netROI), ...lineAreaStyle('#8b5cf6'), symbol: 'circle', symbolSize: 4, lineStyle: { type: 'dashed' as const } },
                        ],
                      }} />
                    </Card>
                    {adDetails.length > 0 && (
                      <Card className="bi-table-card" title="各直播间投放明细" style={{ marginBottom: 16 }}>
                        <Table dataSource={adDetails} rowKey="douyinName" size="small" pagination={false}
                          columns={[
                            { title: '直播间', dataIndex: 'douyinName', key: 'name', ellipsis: true, width: 180 },
                            { title: '消耗', dataIndex: 'cost', key: 'cost', render: (v: number) => `¥${v?.toLocaleString()}`, sorter: (a: any, b: any) => a.cost - b.cost, defaultSortOrder: 'descend' as const },
                            { title: '成交金额', dataIndex: 'payAmount', key: 'pay', render: (v: number) => `¥${v?.toLocaleString()}` },
                            { title: 'ROI', dataIndex: 'roi', key: 'roi', render: (v: number) => v?.toFixed(2) },
                            { title: '净成交', dataIndex: 'netAmount', key: 'net', render: (v: number) => `¥${v?.toLocaleString()}` },
                            { title: '净ROI', dataIndex: 'netROI', key: 'nroi', render: (v: number) => v?.toFixed(2) },
                            { title: '展示', dataIndex: 'impressions', key: 'imp' },
                            { title: '点击', dataIndex: 'clicks', key: 'click' },
                            { title: '1h退款率', dataIndex: 'refund1hRate', key: 'ref', render: (v: number) => v > 0 ? `${v}%` : '-' },
                          ]}
                        />
                      </Card>
                    )}
                  </>
                );
              })()}
            </>
          )}
        </>
      )}

      {tab === 'dist' && (
        <>
          {distTrend.length === 0 && accountRank.length === 0 && distPlans.length === 0 ? (
            <Card><Empty description="暂无抖音分销数据" /></Card>
          ) : (
            <>
              {/* 投放计划筛选 */}
              <Card className="bi-filter-card" style={{ marginBottom: 16 }}>
                <Row align="middle" gutter={16}>
                  <Col>
                    <span style={{ fontWeight: 500, marginRight: 8 }}>投放计划：</span>
                    <Select
                      value={selectedPlan}
                      onChange={(v) => { setSelectedPlan(v); if (v === 'all') fetchDist(startDate, endDate, 'all'); }}
                      style={{ width: 400 }}
                      options={[
                        { value: 'all', label: `全部计划（${distPlans.length}个）` },
                        ...distPlans.map((p: any) => ({
                          value: p.accountName,
                          label: `${p.accountName}（${p.talents}个达人，¥${p.payAmount?.toLocaleString()}）`,
                        })),
                      ]}
                    />
                  </Col>
                </Row>
              </Card>

              <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
                {distStatCards.map((card) => (
                  <Col xs={12} sm={6} key={card.title}>
                    <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                      <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} />
                    </Card>
                  </Col>
                ))}
              </Row>

              {distTrend.length > 0 && (
                <Card title="每日投放趋势" style={{ marginBottom: 16 }}>
                  <ReactECharts option={distTrendOption} lazyUpdate={true} style={{ height: 350 }} />
                </Card>
              )}

              <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
                {accountRank.length > 0 && (
                  <Col xs={24} lg={14}>
                    <Card className="bi-table-card" title="达人排行TOP20">
                      <Table dataSource={accountRank} rowKey={(r: any) => `${r.douyinName}-${r.accountName}`} size="small" pagination={false} scroll={{ y: 500 }}
                        columns={[
                          { title: '达人', dataIndex: 'douyinName', key: 'name', ellipsis: true, width: 150 },
                          ...(selectedPlan === 'all' ? [{ title: '投放计划', dataIndex: 'accountName', key: 'plan', ellipsis: true, width: 100, render: (v: string) => shortPlanName(v) }] : []),
                          { title: '消耗', dataIndex: 'cost', key: 'cost', render: (v: number) => `¥${v?.toLocaleString()}` },
                          { title: '成交金额', dataIndex: 'payAmount', key: 'amt', render: (v: number) => `¥${v?.toLocaleString()}`, sorter: (a: any, b: any) => a.payAmount - b.payAmount, defaultSortOrder: 'descend' as const },
                          { title: 'ROI', dataIndex: 'roi', key: 'roi', render: (v: number) => v?.toFixed(2) },
                          { title: '净成交', dataIndex: 'netAmount', key: 'net', render: (v: number) => `¥${v?.toLocaleString()}` },
                        ]}
                      />
                    </Card>
                  </Col>
                )}
                {productRank.length > 0 && (
                  <Col xs={24} lg={10}>
                    <Card className="bi-table-card" title="商品排行TOP10">
                      <Table dataSource={productRank} rowKey={(r: any) => `${r.productName}-${r.accountName}`} size="small" pagination={false}
                        columns={[
                          { title: '商品', dataIndex: 'productName', key: 'name', ellipsis: true, width: 180 },
                          ...(selectedPlan === 'all' ? [{ title: '投放计划', dataIndex: 'accountName', key: 'plan', ellipsis: true, width: 100, render: (v: string) => shortPlanName(v) }] : []),
                          { title: '成交金额', dataIndex: 'payAmount', key: 'amt', render: (v: number) => `¥${v?.toLocaleString()}`, sorter: (a: any, b: any) => a.payAmount - b.payAmount, defaultSortOrder: 'descend' as const },
                          { title: 'ROI', dataIndex: 'roi', key: 'roi', render: (v: number) => v?.toFixed(2) },
                          { title: '点击', dataIndex: 'clicks', key: 'clicks' },
                        ]}
                      />
                    </Card>
                  </Col>
                )}
              </Row>
            </>
          )}
        </>
      )}
    </div>
  );
};

export default MarketingDashboard;
