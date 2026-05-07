// 业务预决算报表 (v0.59)
// 跑哥要求"前端所有的设计都和财务报表一样" — 1:1 复刻 Report.tsx 的 filter + 主表格
//
// 顶部 filter: 年份范围 + 月份范围 + 渠道 checkbox 多选 + 上传 Excel 按钮（占位）
// 主表格: 科目 (fixed) + 区间合计 + 各 (year, month) 矩阵
// 多 channel 时: 每个 (year, month) 列下展开 channel 子列

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Select, Spin, Table, Empty, Typography, Space, Button, Tooltip, TreeSelect } from 'antd';
import { UploadOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';
import { formatWanHint } from '../../chartTheme';

const { Text } = Typography;

const YEAR_OPTIONS = [2023, 2024, 2025, 2026];

interface ChannelItem { channel: string; subChannel: string; key: string; label: string; subjCount: number; }
interface ChannelGroup { channel: string; items: ChannelItem[]; }

interface BBRCell {
  budget?: number;
  actual?: number;
  achievementRate?: number;
  ratio?: number;
}
interface BBRSeries {
  yearStart: BBRCell;
  rangeTotal: BBRCell;
  cells: Record<string, BBRCell>;
}
interface BBRChannelSeries {
  channel: string;
  series: BBRSeries;
}
interface BBRRow {
  code: string;
  name: string;
  level: number;
  parent: string;
  category: string;
  channel: string;
  subChannel: string;
  total: BBRSeries;
  byChannel?: BBRChannelSeries[];
  children?: BBRRow[];
}
interface BBRData {
  channels: string[];
  yearMonths: string[];
  rows: BBRRow[];
}

const BusinessReport: React.FC = () => {
  const [yearStart, setYearStart] = useState<number>(2026);
  const [yearEnd, setYearEnd] = useState<number>(2026);
  const [monthStart, setMonthStart] = useState<number>(1);
  const [monthEnd, setMonthEnd] = useState<number>(12);
  const [channels, setChannels] = useState<string[]>(['总']);
  const [channelGroups, setChannelGroups] = useState<ChannelGroup[]>([]);
  const [data, setData] = useState<BBRData | null>(null);
  const [loading, setLoading] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    fetch(`${API_BASE}/api/finance/business-report/channels`, { credentials: 'include' })
      .then(r => r.json())
      .then(res => setChannelGroups(res?.data?.groups || []))
      .catch(() => setChannelGroups([]));
  }, []);

  const fetchReport = useCallback(() => {
    if (channels.length === 0) {
      setData(null);
      return;
    }
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    const params = new URLSearchParams({
      yearStart: String(yearStart),
      yearEnd: String(yearEnd),
      monthStart: String(monthStart),
      monthEnd: String(monthEnd),
      channels: channels.join(','),
    });
    fetch(`${API_BASE}/api/finance/business-report?${params.toString()}`, { credentials: 'include', signal: ctrl.signal })
      .then((r) => r.json())
      .then((res) => setData(res.data))
      .catch((e: any) => { if (e?.name !== 'AbortError') setData(null); })
      .finally(() => setLoading(false));
  }, [yearStart, yearEnd, monthStart, monthEnd, channels]);

  useEffect(() => {
    const t = setTimeout(() => fetchReport(), 250);
    return () => clearTimeout(t);
  }, [fetchReport]);

  const reportFilter = (
    <div style={{ marginBottom: 12, padding: '8px 12px', background: '#f8fafc', borderRadius: 6, display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 8, justifyContent: 'space-between' }}>
      <Space wrap size="middle">
        <span>年份：</span>
        <Select value={yearStart} onChange={(v) => { setYearStart(v); if (v > yearEnd) setYearEnd(v); }} style={{ width: 90 }} options={YEAR_OPTIONS.map((y) => ({ label: y, value: y }))} />
        <span>至</span>
        <Select value={yearEnd} onChange={(v) => { setYearEnd(v); if (v < yearStart) setYearStart(v); }} style={{ width: 90 }} options={YEAR_OPTIONS.map((y) => ({ label: y, value: y }))} />
        <span style={{ marginLeft: 12 }}>月份：</span>
        <Select value={monthStart} onChange={(v) => { setMonthStart(v); if (v > monthEnd) setMonthEnd(v); }} style={{ width: 90 }}
          options={Array.from({ length: 12 }, (_, i) => ({ label: `${i + 1}月`, value: i + 1 }))} />
        <span>至</span>
        <Select value={monthEnd} onChange={(v) => { setMonthEnd(v); if (v < monthStart) setMonthStart(v); }} style={{ width: 90 }}
          options={Array.from({ length: 12 }, (_, i) => ({ label: `${i + 1}月`, value: i + 1 }))} />
        <span style={{ marginLeft: 12 }}>渠道：</span>
        <TreeSelect
          multiple
          value={channels}
          onChange={(v) => setChannels(v as string[])}
          treeData={channelGroups.map((g) => {
            // 父节点用一级渠道汇总（电商|''），children 为非空子渠道
            const summary = g.items.find((it) => it.subChannel === '');
            const subs = g.items.filter((it) => it.subChannel !== '');
            return {
              title: g.channel,
              value: summary ? summary.key : `${g.channel}-group`,
              selectable: !!summary,
              children: subs.map((it) => ({ title: it.subChannel, value: it.key })),
            };
          })}
          treeDefaultExpandAll={false}
          showCheckedStrategy="SHOW_ALL"
          treeCheckable={false}
          allowClear
          maxTagCount="responsive"
          placeholder="选择渠道（支持多选 + 子渠道）"
          style={{ minWidth: 360, maxWidth: 600 }}
        />
      </Space>
      <Space>
        <Tooltip title="下个版本支持自助上传，目前请联系数据组导入">
          <Button icon={<UploadOutlined />} disabled>上传 Excel</Button>
        </Tooltip>
      </Space>
    </div>
  );

  return (
    <>
      {reportFilter}
      <BusinessReportTable data={data} loading={loading} />
    </>
  );
};

// ============= 主表格 (1:1 复刻 ReportTable 视觉) =============
const BusinessReportTable: React.FC<{ data: BBRData | null; loading: boolean }> = ({ data, loading }) => {
  const columns = useMemo(() => {
    if (!data) return [] as any[];
    return buildColumns(data);
  }, [data]);
  if (loading) return <div style={{ textAlign: 'center', padding: 60 }}><Spin /></div>;
  if (!data || data.rows.length === 0) return <Empty description="暂无数据" />;
  return (
    <Table
      columns={columns}
      dataSource={data.rows}
      rowKey={(r) => r.code}
      pagination={false}
      size="small"
      bordered
      scroll={{ x: 'max-content', y: 'calc(100vh - 400px)' }}
      rowClassName={(r) => (r.level === 1 ? 'fin-row-group' : '')}
    />
  );
};

const buildColumns = (data: BBRData): any[] => {
  const multi = data.channels.length > 1;
  const findChannel = (row: BBRRow, ch: string) => row.byChannel?.find((x) => x.channel === ch);

  // 单值渲染：预算 / 实际 / 达成率 各自独立列
  const fmtNum = (v?: number) => v == null ? '-' : v.toLocaleString('zh-CN', { maximumFractionDigits: 0 });
  const achColor = (r?: number) => r == null ? '#94a3b8' : r >= 1 ? '#16a34a' : r >= 0.8 ? '#ca8a04' : '#dc2626';
  type CellKind = 'budget' | 'actual' | 'ratio';
  const formatVal = (c?: BBRCell, kind: CellKind = 'actual', level?: number, isChannel?: boolean) => {
    if (!c) return <Text type="secondary">-</Text>;
    const bold = level === 1;
    if (kind === 'budget') {
      if (c.budget == null) return <Text type="secondary">-</Text>;
      return <div style={{ textAlign: 'right', color: isChannel ? '#64748b' : undefined, fontWeight: bold ? 700 : 400 }}>{fmtNum(c.budget)}</div>;
    }
    if (kind === 'actual') {
      if (c.actual == null) return <Text type="secondary">-</Text>;
      return <div style={{ textAlign: 'right', color: isChannel ? '#64748b' : undefined, fontWeight: bold ? 700 : 500 }}>{fmtNum(c.actual)}</div>;
    }
    // ratio = 科目实际 / 营业收入（xlsx 原值"占比销售"列）
    if (c.ratio == null || !isFinite(c.ratio)) return <Text type="secondary">-</Text>;
    return <div style={{ textAlign: 'right', color: '#64748b', fontSize: 12 }}>{(c.ratio * 100).toFixed(2)}%</div>;
  };
  const _suppress = achColor; void _suppress;
  const isGmvRow = (row: BBRRow) => row.category === 'GMV数据';
  const formatRatio = (c: BBRCell | undefined, row: BBRRow) => {
    if (isGmvRow(row)) return null;
    if (!c || c.ratio === undefined || c.ratio === null || !isFinite(c.ratio)) return <Text type="secondary">-</Text>;
    return <div style={{ textAlign: 'right', color: '#64748b', fontSize: 11 }}>{(c.ratio * 100).toFixed(2)}%</div>;
  };
  const divider = {
    onCell: () => ({ style: { borderRight: '2px solid #94a3b8' } }),
    onHeaderCell: () => ({ style: { borderRight: '2px solid #94a3b8' } }),
  };
  // chKey "电商|TOC" → "电商-TOC" 显示；"电商" → "电商"
  const chLabel = (k: string) => k.includes('|') ? k.replace('|', '-') : k;
  // 多 channel 时每个 channel 拆成 [预算 + 实际 + 占比] 子列
  const channelSubColsBA = (getCell: (row: BBRRow, ch: string) => BBRCell | undefined, keyPrefix: string) => {
    const cols: any[] = [];
    data.channels.forEach((ch, i) => {
      const isLast = i === data.channels.length - 1;
      cols.push({
        title: chLabel(ch),
        key: `${keyPrefix}_${ch}`,
        children: [
          { title: '预算', key: `${keyPrefix}_${ch}_b`, width: 110, render: (_: any, row: BBRRow) => formatVal(getCell(row, ch), 'budget', row.level, true) },
          { title: '实际', key: `${keyPrefix}_${ch}_a`, width: 110, render: (_: any, row: BBRRow) => formatVal(getCell(row, ch), 'actual', row.level, true) },
          { title: '占比', key: `${keyPrefix}_${ch}_r`, width: 70, render: (_: any, row: BBRRow) => formatVal(getCell(row, ch), 'ratio', row.level, true), ...(isLast ? divider : {}) },
        ],
      });
    });
    return cols;
  };
  // 多 channel 年度预算只有 budget 一列
  const channelSubColsBudget = (getCell: (row: BBRRow, ch: string) => BBRCell | undefined, keyPrefix: string) => {
    return data.channels.map((ch, i) => ({
      title: chLabel(ch),
      key: `${keyPrefix}_${ch}`,
      width: 120,
      render: (_: any, row: BBRRow) => formatVal(getCell(row, ch), 'budget', row.level, true),
      ...(i === data.channels.length - 1 ? divider : {}),
    }));
  };

  const cols: any[] = [
    {
      title: '科目',
      key: 'name',
      width: 220,
      fixed: 'left' as const,
      render: (_: any, row: BBRRow) => {
        if (row.level === 1) {
          return <div style={{ fontWeight: 700, color: '#0f172a', fontSize: 13 }}>{row.name}</div>;
        }
        const indent = (row.level - 1) * 16;
        const label = row.subChannel ? `· ${row.subChannel}` : row.name;
        return <div style={{ paddingLeft: indent, fontSize: 13 }}>{label}</div>;
      },
    },
    {
      title: '年度预算',
      key: 'yearStart',
      children: multi
        ? [
            { title: '总', key: 'ys_total', width: 130, render: (_: any, row: BBRRow) => formatVal(row.total.yearStart, 'budget', row.level), ...divider },
            ...channelSubColsBudget((row, ch) => findChannel(row, ch)?.series.yearStart, 'ys'),
          ]
        : [{ title: '金额', key: 'ys_total', width: 130, render: (_: any, row: BBRRow) => formatVal(row.total.yearStart, 'budget', row.level), ...divider }],
    },
    {
      title: '区间合计',
      key: 'range',
      children: multi
        ? [
            { title: '总-预算', key: 'r_b', width: 120, render: (_: any, row: BBRRow) => formatVal(row.total.rangeTotal, 'budget', row.level) },
            { title: '总-实际', key: 'r_a', width: 120, render: (_: any, row: BBRRow) => formatVal(row.total.rangeTotal, 'actual', row.level) },
            { title: '总-占比', key: 'r_r', width: 70, render: (_: any, row: BBRRow) => formatVal(row.total.rangeTotal, 'ratio', row.level), ...divider },
            ...channelSubColsBA((row, ch) => findChannel(row, ch)?.series.rangeTotal, 'range'),
          ]
        : [
            { title: '预算', key: 'r_b', width: 120, render: (_: any, row: BBRRow) => formatVal(row.total.rangeTotal, 'budget', row.level) },
            { title: '实际', key: 'r_a', width: 120, render: (_: any, row: BBRRow) => formatVal(row.total.rangeTotal, 'actual', row.level) },
            { title: '占比', key: 'r_r', width: 70, render: (_: any, row: BBRRow) => formatVal(row.total.rangeTotal, 'ratio', row.level), ...divider },
          ],
    },
    ...data.yearMonths.map((ym) => ({
      title: ym,
      key: ym,
      children: multi
        ? [
            { title: '总-预算', key: `${ym}_b`, width: 110, render: (_: any, row: BBRRow) => formatVal(row.total.cells[ym], 'budget', row.level) },
            { title: '总-实际', key: `${ym}_a`, width: 110, render: (_: any, row: BBRRow) => formatVal(row.total.cells[ym], 'actual', row.level) },
            { title: '总-占比', key: `${ym}_r`, width: 70, render: (_: any, row: BBRRow) => formatVal(row.total.cells[ym], 'ratio', row.level), ...divider },
            ...channelSubColsBA((row, ch) => findChannel(row, ch)?.series.cells[ym], ym),
          ]
        : [
            { title: '预算', key: `${ym}_b`, width: 110, render: (_: any, row: BBRRow) => formatVal(row.total.cells[ym], 'budget', row.level) },
            { title: '实际', key: `${ym}_a`, width: 110, render: (_: any, row: BBRRow) => formatVal(row.total.cells[ym], 'actual', row.level) },
            { title: '占比', key: `${ym}_r`, width: 70, render: (_: any, row: BBRRow) => formatVal(row.total.cells[ym], 'ratio', row.level), ...divider },
          ],
    })),
  ];
  return cols;
};

export default BusinessReport;
