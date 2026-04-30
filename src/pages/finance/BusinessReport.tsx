// 业务预决算报表 (v0.58)
// 替换 Report.tsx tab "业务报表" 的 Empty 占位
// 4 子 tab:
//   - 渠道明细：snapshot + channel + sub_channel → 完整 4×12 月数据表
//   - 渠道总览：所有 channel 的 GMV合计 达成率横向对比
//   - 月度趋势：单 subject 跨 12 月 budget vs actual 折线图
//   - 经营 KPI：经营指标 sheet 的 22 项核心指标

import React, { useEffect, useMemo, useState } from 'react';
import { Card, Select, Tabs, Table, Spin, Empty, Tag, Row, Col, Typography, Space, Statistic } from 'antd';
import ReactECharts from 'echarts-for-react';
import { API_BASE } from '../../config';

const { Text } = Typography;

// 后端 writeJSON 包 {code, data}，统一用 .data 解包
const fetchJson = async (url: string): Promise<any> => {
  const r = await fetch(url, { credentials: 'include' });
  if (!r.ok) throw new Error(`HTTP ${r.status}`);
  const j = await r.json();
  return j?.data ?? j;
};

interface Snapshot {
  snapshotYear: number;
  snapshotMonth: number;
  year: number;
  label: string;
  rowCount: number;
  channelCount: number;
}

interface BudgetMonth {
  month: number;
  budget?: number;
  ratioBudget?: number;
  actual?: number;
  ratioActual?: number;
}

interface BudgetCell {
  subject: string;
  subjectLevel: number;
  subjectCategory: string;
  parentSubject: string;
  sortOrder: number;
  budgetYearStart?: number;
  ratioYearStart?: number;
  budgetTotal?: number;
  ratioBudget?: number;
  actualTotal?: number;
  ratioActual?: number;
  achievementRate?: number;
  months: BudgetMonth[];
}

interface DetailResp {
  snapshotYear: number;
  snapshotMonth: number;
  channel: string;
  subChannel: string;
  subChannels: string[];
  cells: BudgetCell[];
}

interface ChannelOverviewItem {
  channel: string;
  subChannel: string;
  subject: string;
  budgetTotal?: number;
  actualTotal?: number;
  achievementRate?: number;
}

const fmt = (v: number | null | undefined, opts?: { pct?: boolean; wan?: boolean }) => {
  if (v == null || v === 0) return v === 0 ? '0' : '-';
  if (opts?.pct) return `${(v * 100).toFixed(2)}%`;
  if (opts?.wan && Math.abs(v) >= 10000) return `${(v / 10000).toFixed(2)}万`;
  return v.toLocaleString('zh-CN', { maximumFractionDigits: 2 });
};

const achievementColor = (r?: number) => {
  if (r == null) return undefined;
  if (r >= 1) return 'green';
  if (r >= 0.8) return 'gold';
  return 'red';
};

const BusinessReport: React.FC = () => {
  const [snapshots, setSnapshots] = useState<Snapshot[]>([]);
  const [snap, setSnap] = useState<string>(''); // "YYYY-MM"
  const [loadingSnap, setLoadingSnap] = useState(false);

  useEffect(() => {
    setLoadingSnap(true);
    fetchJson(`${API_BASE}/api/finance/business-report/snapshots`)
      .then(d => {
        const list: Snapshot[] = d?.snapshots || [];
        setSnapshots(list);
        if (list.length > 0) {
          setSnap(`${list[0].snapshotYear}-${String(list[0].snapshotMonth).padStart(2, '0')}`);
        }
      })
      .finally(() => setLoadingSnap(false));
  }, []);

  const snapHeader = (
    <Space size={16} wrap>
      <Space>
        <Text strong>快照：</Text>
        <Select
          loading={loadingSnap}
          value={snap || undefined}
          style={{ minWidth: 220 }}
          options={snapshots.map(s => ({
            value: `${s.snapshotYear}-${String(s.snapshotMonth).padStart(2, '0')}`,
            label: `${s.label}  ·  ${s.rowCount.toLocaleString()} 行 / ${s.channelCount} 渠道`,
          }))}
          onChange={setSnap}
          placeholder="选择快照"
        />
      </Space>
    </Space>
  );

  if (!snap) {
    return <Card>{snapHeader}<Empty description="无业务报表数据，请先用 import-business-report.exe 导入" style={{ marginTop: 32 }} /></Card>;
  }

  return (
    <Card>
      <div style={{ marginBottom: 12 }}>{snapHeader}</div>
      <Tabs
        defaultActiveKey="detail"
        items={[
          { key: 'detail', label: '渠道明细', children: <DetailTab snap={snap} /> },
          { key: 'overview', label: '渠道总览', children: <OverviewTab snap={snap} /> },
          { key: 'trend', label: '月度趋势', children: <TrendTab snap={snap} /> },
          { key: 'kpi', label: '经营 KPI', children: <KPITab snap={snap} /> },
        ]}
      />
    </Card>
  );
};

// ----------------- DetailTab -----------------
const CHANNEL_OPTIONS = ['总', '电商', '私域', '分销', '社媒', '线下', '国际零售', '即时零售', '糙能', '中后台'];

const DetailTab: React.FC<{ snap: string }> = ({ snap }) => {
  const [channel, setChannel] = useState<string>('总');
  const [subChannel, setSubChannel] = useState<string>('');
  const [data, setData] = useState<DetailResp | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => { setSubChannel(''); }, [channel, snap]);

  useEffect(() => {
    if (!snap || !channel) return;
    setLoading(true);
    fetchJson(`${API_BASE}/api/finance/business-report/detail?snapshot=${snap}&channel=${encodeURIComponent(channel)}&sub_channel=${encodeURIComponent(subChannel)}`)
      .then(setData)
      .finally(() => setLoading(false));
  }, [snap, channel, subChannel]);

  const cells = data?.cells || [];
  const subOptions = (data?.subChannels || []).filter(s => s !== '');

  // 列：subject + 年初 + 合计预算 + 合计实际 + 达成率 + 12 月（每月 实际 / 预算两行）
  const monthCols = Array.from({ length: 12 }, (_, i) => i + 1).map(m => ({
    title: `${m}月`,
    children: [
      {
        title: '预算',
        key: `b${m}`,
        align: 'right' as const,
        width: 90,
        render: (_: unknown, row: BudgetCell) => fmt(row.months[m - 1]?.budget, { wan: true }),
      },
      {
        title: '实际',
        key: `a${m}`,
        align: 'right' as const,
        width: 90,
        render: (_: unknown, row: BudgetCell) => fmt(row.months[m - 1]?.actual, { wan: true }),
      },
    ],
  }));

  const columns = [
    {
      title: '科目',
      dataIndex: 'subject',
      fixed: 'left' as const,
      width: 200,
      render: (text: string, row: BudgetCell) => (
        <span style={{ paddingLeft: (row.subjectLevel - 1) * 12, fontWeight: row.subjectLevel === 1 ? 600 : 400 }}>
          {text}
          {row.subjectCategory && row.subjectLevel === 2 && <Tag style={{ marginLeft: 6 }} color="blue">{row.subjectCategory}</Tag>}
        </span>
      ),
    },
    { title: '年初预算', key: 'bys', fixed: 'left' as const, width: 110, align: 'right' as const, render: (_: unknown, r: BudgetCell) => fmt(r.budgetYearStart, { wan: true }) },
    { title: '合计预算', key: 'bt', fixed: 'left' as const, width: 110, align: 'right' as const, render: (_: unknown, r: BudgetCell) => fmt(r.budgetTotal, { wan: true }) },
    { title: '合计实际', key: 'at', fixed: 'left' as const, width: 110, align: 'right' as const, render: (_: unknown, r: BudgetCell) => fmt(r.actualTotal, { wan: true }) },
    {
      title: '达成率',
      key: 'ar',
      fixed: 'left' as const,
      width: 90,
      align: 'right' as const,
      render: (_: unknown, r: BudgetCell) => r.achievementRate == null ? '-' : <Tag color={achievementColor(r.achievementRate)}>{fmt(r.achievementRate, { pct: true })}</Tag>,
    },
    ...monthCols,
  ];

  return (
    <Spin spinning={loading}>
      <Space style={{ marginBottom: 12 }} wrap>
        <Text>渠道：</Text>
        <Select value={channel} onChange={setChannel} options={CHANNEL_OPTIONS.map(c => ({ value: c, label: c }))} style={{ minWidth: 120 }} />
        {subOptions.length > 0 && (
          <>
            <Text>子渠道：</Text>
            <Select
              value={subChannel || ''}
              onChange={setSubChannel}
              options={[{ value: '', label: '【汇总】' }, ...subOptions.map(s => ({ value: s, label: s }))]}
              style={{ minWidth: 160 }}
            />
          </>
        )}
      </Space>
      <Table
        rowKey="sortOrder"
        size="small"
        bordered
        scroll={{ x: 'max-content', y: 600 }}
        columns={columns as any}
        dataSource={cells}
        pagination={false}
      />
    </Spin>
  );
};

// ----------------- OverviewTab -----------------
const OverviewTab: React.FC<{ snap: string }> = ({ snap }) => {
  const [subject, setSubject] = useState<string>('GMV合计');
  const [data, setData] = useState<ChannelOverviewItem[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!snap) return;
    setLoading(true);
    fetchJson(`${API_BASE}/api/finance/business-report/overview?snapshot=${snap}&subject=${encodeURIComponent(subject)}`)
      .then(d => setData(d?.channels || []))
      .finally(() => setLoading(false));
  }, [snap, subject]);

  // 只展示 sub_channel='' 的（即一级渠道汇总）
  const top = data.filter(d => d.subChannel === '');
  const totalBudget = top.reduce((s, d) => s + (d.budgetTotal || 0), 0);
  const totalActual = top.reduce((s, d) => s + (d.actualTotal || 0), 0);

  const columns = [
    { title: '渠道', dataIndex: 'channel', width: 140 },
    { title: '子渠道', dataIndex: 'subChannel', width: 120, render: (v: string) => v || <Text type="secondary">汇总</Text> },
    { title: '合计预算', dataIndex: 'budgetTotal', align: 'right' as const, render: (v: number) => fmt(v, { wan: true }) },
    { title: '合计实际', dataIndex: 'actualTotal', align: 'right' as const, render: (v: number) => fmt(v, { wan: true }) },
    {
      title: '达成率',
      dataIndex: 'achievementRate',
      align: 'right' as const,
      render: (v?: number) => v == null ? '-' : <Tag color={achievementColor(v)}>{fmt(v, { pct: true })}</Tag>,
    },
  ];

  return (
    <Spin spinning={loading}>
      <Space style={{ marginBottom: 12 }}>
        <Text>科目：</Text>
        <Select
          value={subject}
          onChange={setSubject}
          style={{ minWidth: 200 }}
          options={['GMV合计', '退款金额', '一、营业收入', '减：营业成本', '营业毛利', '减：销售费用', '运营利润', '减：管理费用（不可控成本）', '利润总额', '二：净利润'].map(s => ({ value: s, label: s }))}
        />
      </Space>
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={6}><Card><Statistic title="一级渠道数" value={top.length} /></Card></Col>
        <Col span={6}><Card><Statistic title={`${subject} 合计预算`} value={fmt(totalBudget, { wan: true })} /></Card></Col>
        <Col span={6}><Card><Statistic title={`${subject} 合计实际`} value={fmt(totalActual, { wan: true })} /></Card></Col>
        <Col span={6}><Card><Statistic title="整体达成率" value={totalBudget ? fmt(totalActual / totalBudget, { pct: true }) : '-'} /></Card></Col>
      </Row>
      <Table rowKey={(r) => `${r.channel}_${r.subChannel}`} size="small" bordered columns={columns} dataSource={data} pagination={false} />
    </Spin>
  );
};

// ----------------- TrendTab -----------------
const TrendTab: React.FC<{ snap: string }> = ({ snap }) => {
  const [channel, setChannel] = useState<string>('总');
  const [subChannel, setSubChannel] = useState<string>('');
  const [subject, setSubject] = useState<string>('GMV合计');
  const [points, setPoints] = useState<{ month: number; budget?: number; actual?: number }[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!snap || !channel || !subject) return;
    setLoading(true);
    fetchJson(`${API_BASE}/api/finance/business-report/trend?snapshot=${snap}&channel=${encodeURIComponent(channel)}&sub_channel=${encodeURIComponent(subChannel)}&subject=${encodeURIComponent(subject)}`)
      .then(d => setPoints(d?.points || []))
      .finally(() => setLoading(false));
  }, [snap, channel, subChannel, subject]);

  const option = useMemo(() => {
    const months = Array.from({ length: 12 }, (_, i) => i + 1);
    const ptMap = new Map(points.map(p => [p.month, p]));
    const budgets = months.map(m => ptMap.get(m)?.budget ?? null);
    const actuals = months.map(m => ptMap.get(m)?.actual ?? null);
    return {
      tooltip: { trigger: 'axis', valueFormatter: (v: any) => v == null ? '-' : fmt(v, { wan: true }) },
      legend: { data: ['预算', '实际'] },
      xAxis: { type: 'category', data: months.map(m => `${m}月`) },
      yAxis: { type: 'value', name: '金额', axisLabel: { formatter: (v: number) => Math.abs(v) >= 10000 ? `${(v / 10000).toFixed(0)}万` : `${v}` } },
      series: [
        { name: '预算', type: 'bar', data: budgets, itemStyle: { color: '#94a3b8' } },
        { name: '实际', type: 'line', data: actuals, smooth: true, itemStyle: { color: '#1e40af' }, lineStyle: { width: 3 } },
      ],
    };
  }, [points]);

  return (
    <Spin spinning={loading}>
      <Space style={{ marginBottom: 12 }} wrap>
        <Text>渠道：</Text>
        <Select value={channel} onChange={(v) => { setChannel(v); setSubChannel(''); }} options={CHANNEL_OPTIONS.map(c => ({ value: c, label: c }))} style={{ minWidth: 120 }} />
        <Text>子渠道：</Text>
        <Select value={subChannel || ''} onChange={setSubChannel} options={[{ value: '', label: '【汇总】' }]} style={{ minWidth: 120 }} />
        <Text>科目：</Text>
        <Select value={subject} onChange={setSubject} style={{ minWidth: 200 }}
          options={['GMV合计', '退款金额', '一、营业收入', '营业毛利', '运营利润', '利润总额', '二：净利润'].map(s => ({ value: s, label: s }))}
        />
      </Space>
      <ReactECharts option={option} style={{ height: 400 }} />
    </Spin>
  );
};

// ----------------- KPITab -----------------
const KPITab: React.FC<{ snap: string }> = ({ snap }) => {
  const [data, setData] = useState<DetailResp | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!snap) return;
    setLoading(true);
    fetchJson(`${API_BASE}/api/finance/business-report/detail?snapshot=${snap}&channel=${encodeURIComponent('经营指标')}&sub_channel=`)
      .then(setData)
      .finally(() => setLoading(false));
  }, [snap]);

  const cells = data?.cells || [];
  if (!loading && cells.length === 0) {
    return <Empty description="此快照无经营指标数据（仅 2026-04 快照含经营指标 sheet）" />;
  }

  const columns = [
    { title: '指标', dataIndex: 'subject', width: 200 },
    { title: '年度预算', key: 'bt', align: 'right' as const, render: (_: unknown, r: BudgetCell) => fmt(r.budgetTotal, { wan: true }) },
    { title: '上年值', key: 'at', align: 'right' as const, render: (_: unknown, r: BudgetCell) => fmt(r.actualTotal, { wan: true }) },
    {
      title: '增长率',
      key: 'ar',
      align: 'right' as const,
      render: (_: unknown, r: BudgetCell) => r.achievementRate == null ? '-' : <Tag color={(r.achievementRate || 0) > 0 ? 'green' : 'red'}>{fmt(r.achievementRate, { pct: true })}</Tag>,
    },
    ...Array.from({ length: 12 }, (_, i) => i + 1).map(m => ({
      title: `${m}月`,
      key: `m${m}`,
      align: 'right' as const,
      width: 90,
      render: (_: unknown, r: BudgetCell) => fmt(r.months[m - 1]?.actual, { wan: true }),
    })),
  ];

  return (
    <Spin spinning={loading}>
      <Table
        rowKey="sortOrder"
        size="small"
        bordered
        scroll={{ x: 'max-content', y: 600 }}
        columns={columns as any}
        dataSource={cells}
        pagination={false}
      />
    </Spin>
  );
};

export default BusinessReport;
