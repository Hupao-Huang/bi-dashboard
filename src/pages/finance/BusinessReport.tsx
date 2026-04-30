// 业务预决算报表 (v0.59)
// 跑哥要求"前端所有的设计都和财务报表一样" — 1:1 复刻 Report.tsx 的 filter + 主表格
//
// 顶部 filter: 年份范围 + 月份范围 + 渠道 checkbox 多选 + 上传 Excel 按钮（占位）
// 主表格: 科目 (fixed) + 区间合计 + 各 (year, month) 矩阵
// 多 channel 时: 每个 (year, month) 列下展开 channel 子列

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Select, Spin, Table, Empty, Typography, Space, Checkbox, Button, Tooltip } from 'antd';
import { UploadOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';
import { formatWanHint } from '../../chartTheme';

const { Text } = Typography;

const ALL_CHANNELS = ['总', '电商', '私域', '分销', '社媒', '线下', '国际零售', '即时零售', '糙能', '中后台', '经营指标'];
const YEAR_OPTIONS = [2023, 2024, 2025, 2026];

interface BBRCell {
  budget?: number;
  actual?: number;
  achievementRate?: number;
  ratio?: number;
}
interface BBRSeries {
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
  const [data, setData] = useState<BBRData | null>(null);
  const [loading, setLoading] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

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
        <Checkbox.Group
          value={channels}
          onChange={(v) => setChannels(v as string[])}
          options={ALL_CHANNELS.map((c) => ({ label: c, value: c }))}
        />
      </Space>
      <Space>
        <Tooltip title="财务自助上传业务预决算 xlsx — 下个版本上线（当前由数据团队 CLI 导入）">
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
      rowKey={(r) => `${r.channel}|${r.code}|${r.subChannel || ''}`}
      pagination={false}
      size="small"
      bordered
      scroll={{ x: 'max-content', y: 'calc(100vh - 400px)' }}
      rowClassName={(r) => (r.level === 1 ? 'fin-row-group' : '')}
      expandable={{ indentSize: 16 }}
    />
  );
};

const buildColumns = (data: BBRData): any[] => {
  const multi = data.channels.length > 1;
  const findChannel = (row: BBRRow, ch: string) => row.byChannel?.find((x) => x.channel === ch);

  // 单元格三段：预算 / 实际 / 达成率（达成率用颜色 tag 标识）
  const fmtNum = (v?: number) => v == null ? '-' : v.toLocaleString('zh-CN', { maximumFractionDigits: 0 });
  const achColor = (r?: number) => r == null ? '#94a3b8' : r >= 1 ? '#16a34a' : r >= 0.8 ? '#ca8a04' : '#dc2626';
  const formatCell = (c?: BBRCell, level?: number, isChannel?: boolean) => {
    if (level === 1) return null;
    if (!c || (c.budget == null && c.actual == null)) return <Text type="secondary">-</Text>;
    return (
      <div style={{ textAlign: 'right', color: isChannel ? '#64748b' : undefined, lineHeight: 1.3 }}>
        <div style={{ fontSize: 11, color: '#94a3b8' }}>预 {fmtNum(c.budget)}</div>
        <div style={{ fontWeight: 500 }}>{fmtNum(c.actual)}</div>
        {c.achievementRate != null && isFinite(c.achievementRate) && (
          <div style={{ fontSize: 11, color: achColor(c.achievementRate), fontWeight: 500 }}>
            {(c.achievementRate * 100).toFixed(1)}%
          </div>
        )}
      </div>
    );
  };
  const isGmvRow = (row: BBRRow) => row.category === 'GMV数据';
  const formatRatio = (c: BBRCell | undefined, row: BBRRow) => {
    if (row.level === 1 || isGmvRow(row)) return null;
    if (!c || c.ratio === undefined || c.ratio === null || !isFinite(c.ratio)) return <Text type="secondary">-</Text>;
    return <div style={{ textAlign: 'right', color: '#64748b', fontSize: 11 }}>{(c.ratio * 100).toFixed(2)}%</div>;
  };
  const divider = {
    onCell: () => ({ style: { borderRight: '2px solid #94a3b8' } }),
    onHeaderCell: () => ({ style: { borderRight: '2px solid #94a3b8' } }),
  };
  const channelSubCols = (getCell: (row: BBRRow, ch: string) => BBRCell | undefined, keyPrefix: string) => {
    const cols: any[] = [];
    data.channels.forEach((ch, i) => {
      const isLast = i === data.channels.length - 1;
      cols.push({
        title: ch,
        key: `${keyPrefix}_${ch}`,
        width: 120,
        render: (_: any, row: BBRRow) => formatCell(getCell(row, ch), row.level, true),
      });
      cols.push({
        title: '占比',
        key: `${keyPrefix}_${ch}_r`,
        width: 60,
        render: (_: any, row: BBRRow) => formatRatio(getCell(row, ch), row),
        ...(isLast ? divider : {}),
      });
    });
    return cols;
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
      title: '区间合计',
      key: 'range',
      children: [
        { title: multi ? '总' : '金额', key: 'range_total', width: 130, render: (_: any, row: BBRRow) => formatCell(row.total.rangeTotal, row.level) },
        { title: '占比', key: 'range_ratio', width: 70, render: (_: any, row: BBRRow) => formatRatio(row.total.rangeTotal, row), ...(multi ? {} : divider) },
        ...(multi ? channelSubCols((row, ch) => findChannel(row, ch)?.series.rangeTotal, 'range') : []),
      ],
    },
    ...data.yearMonths.map((ym) => ({
      title: ym,
      key: ym,
      children: [
        { title: multi ? '总' : '金额', key: `${ym}_total`, width: 120, render: (_: any, row: BBRRow) => formatCell(row.total.cells[ym], row.level) },
        { title: '占比', key: `${ym}_ratio`, width: 65, render: (_: any, row: BBRRow) => formatRatio(row.total.cells[ym], row), ...(multi ? {} : divider) },
        ...(multi ? channelSubCols((row, ch) => findChannel(row, ch)?.series.cells[ym], ym) : []),
      ],
    })),
  ];
  return cols;
};

export default BusinessReport;
