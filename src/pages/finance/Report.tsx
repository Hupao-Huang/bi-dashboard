import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Card, Select, Tabs, Upload, Button, Spin, message, Table, Modal, Empty, Tag, InputNumber, Space, Typography, Checkbox } from 'antd';
import type { UploadProps } from 'antd';
import { UploadOutlined, FileTextOutlined, DownloadOutlined } from '@ant-design/icons';
import Chart from '../../components/Chart';
import { API_BASE } from '../../config';
import { CHART_COLORS, formatMoney, formatWanHint } from '../../chartTheme';
import { useAuth } from '../../auth/AuthContext';

const { Text } = Typography;

type FinCell = { amount: number; ratio?: number };
type FinSeries = { rangeTotal: FinCell; cells: Record<string, FinCell> };
type FinChannelSeries = { channel: string; series: FinSeries };
type FinRow = {
  code: string;
  name: string;
  level: number;
  parent: string;
  category: string;
  subChannel?: string;
  sortOrder: number;
  total: FinSeries;
  byChannel?: FinChannelSeries[];
};

type ReportData = {
  yearStart: number;
  yearEnd: number;
  monthStart: number;
  monthEnd: number;
  channels: string[];
  yearMonths: string[];
  rows: FinRow[];
};

type SubjectDict = {
  code: string;
  name: string;
  category: string;
  level: number;
  parent: string;
  order: number;
};

const ALL_CHANNELS = ['汇总', '电商', '社媒', '线下', '分销', '私域', '国际零售', '即时零售', '糙有力量', '中台'];
const YEAR_OPTIONS = [2022, 2023, 2024, 2025, 2026];
const DEFAULT_YEAR = new Date().getFullYear();
const HIGHLIGHT_CODES = new Set(['GMV_TOTAL', 'REV_MAIN', 'COST_MAIN', 'PROFIT_GROSS', 'PROFIT_OP', 'NET_PROFIT', 'PROFIT_TOTAL']);

const ReportTable: React.FC<{ data: ReportData | null; loading: boolean }> = ({ data, loading }) => {
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
      rowKey={(r) => `${r.code}|${r.subChannel || ''}`}
      pagination={false}
      size="small"
      bordered
      scroll={{ x: 'max-content', y: 'calc(100vh - 400px)' }}
      rowClassName={(r) => {
        if (r.level === 1) return 'fin-row-group';
        if (HIGHLIGHT_CODES.has(r.code) && r.level === 2) return 'fin-row-highlight';
        return '';
      }}
    />
  );
};

const buildColumns = (data: ReportData): any[] => {
  const multi = data.channels.length > 1;
  const findChannel = (row: FinRow, ch: string) => row.byChannel?.find((x) => x.channel === ch);

  const formatCell = (c?: FinCell, level?: number, isChannel?: boolean) => {
    if (level === 1) return null;
    if (!c || c.amount === 0) return <Text type="secondary">-</Text>;
    const hint = formatWanHint(c.amount);
    return (
      <div style={{ textAlign: 'right', color: isChannel ? '#64748b' : undefined }}>
        <div>{c.amount.toLocaleString('zh-CN', { maximumFractionDigits: 2 })}</div>
        {hint && <div style={{ fontSize: 11, color: '#94a3b8' }}>{hint}</div>}
      </div>
    );
  };
  const isGmvRow = (row: FinRow) => row.category === 'GMV';
  const formatRatio = (c: FinCell | undefined, row: FinRow) => {
    if (row.level === 1 || isGmvRow(row)) return null;
    if (!c || c.ratio === undefined || c.ratio === null || !isFinite(c.ratio)) return <Text type="secondary">-</Text>;
    return <div style={{ textAlign: 'right', color: '#64748b', fontSize: 11 }}>{(c.ratio * 100).toFixed(2)}%</div>;
  };

  const divider = {
    onCell: () => ({ style: { borderRight: '2px solid #94a3b8' } }),
    onHeaderCell: () => ({ style: { borderRight: '2px solid #94a3b8' } }),
  };
  const channelSubCols = (getCell: (row: FinRow, ch: string) => FinCell | undefined, keyPrefix: string) => {
    const cols: any[] = [];
    data.channels.forEach((ch, i) => {
      const isLast = i === data.channels.length - 1;
      cols.push({
        title: ch,
        key: `${keyPrefix}_${ch}`,
        width: 120,
        render: (_: any, row: FinRow) => formatCell(getCell(row, ch), row.level, true),
      });
      cols.push({
        title: '占比',
        key: `${keyPrefix}_${ch}_r`,
        width: 60,
        render: (_: any, row: FinRow) => formatRatio(getCell(row, ch), row),
        ...(isLast ? divider : {}),
      });
    });
    return cols;
  };

  const columns: any[] = [
    {
      title: '科目',
      key: 'name',
      width: 220,
      fixed: 'left' as const,
      render: (_: any, row: FinRow) => {
        if (row.level === 1) {
          return <div style={{ fontWeight: 700, color: '#0f172a', fontSize: 13 }}>{row.name}</div>;
        }
        const indent = (row.level - 1) * 16;
        const bold = row.level === 2 && HIGHLIGHT_CODES.has(row.code);
        const color = bold ? '#1e40af' : undefined;
        const label = row.subChannel ? `· ${row.subChannel}` : row.name;
        return <div style={{ paddingLeft: indent, fontWeight: bold ? 600 : 400, color, fontSize: 13 }}>{label}</div>;
      },
    },
    {
      title: '区间合计',
      key: 'range',
      children: [
        { title: multi ? '总' : '金额', key: 'range_total', width: 130, render: (_: any, row: FinRow) => formatCell(row.total.rangeTotal, row.level), ...(multi ? {} : {}) },
        { title: '占比', key: 'range_ratio', width: 70, render: (_: any, row: FinRow) => formatRatio(row.total.rangeTotal, row), ...(multi ? {} : divider) },
        ...(multi ? channelSubCols((row, ch) => findChannel(row, ch)?.series.rangeTotal, 'range') : []),
      ],
    },
    ...data.yearMonths.map((ym) => ({
      title: ym,
      key: ym,
      children: [
        { title: multi ? '总' : '金额', key: `${ym}_total`, width: 120, render: (_: any, row: FinRow) => formatCell(row.total.cells[ym], row.level) },
        { title: '占比', key: `${ym}_ratio`, width: 65, render: (_: any, row: FinRow) => formatRatio(row.total.cells[ym], row), ...(multi ? {} : divider) },
        ...(multi ? channelSubCols((row, ch) => findChannel(row, ch)?.series.cells[ym], ym) : []),
      ],
    })),
  ];
  return columns;
};

const ReportTrend: React.FC<{ year: number; subjectDict: SubjectDict[] }> = ({ year, subjectDict }) => {
  const defaultSubjects = ['REV_MAIN', 'COST_MAIN', 'PROFIT_GROSS', 'SALES_EXP', 'MGMT_EXP', 'NET_PROFIT'];
  const [selectedSubjects, setSelectedSubjects] = useState<string[]>(defaultSubjects);
  const [selectedChannels, setSelectedChannels] = useState<string[]>(['汇总']);
  const [yearStart, setYearStart] = useState<number>(Math.max(2022, year - 2));
  const [yearEnd, setYearEnd] = useState<number>(year);
  const [points, setPoints] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (selectedSubjects.length === 0 || selectedChannels.length === 0) return;
    setLoading(true);
    const url = `${API_BASE}/api/finance/report/trend?subjects=${selectedSubjects.join(',')}&channels=${encodeURIComponent(selectedChannels.join(','))}&yearStart=${yearStart}&yearEnd=${yearEnd}`;
    fetch(url, { credentials: 'include' })
      .then((r) => r.json())
      .then((res) => setPoints(res.data?.points || []))
      .catch(() => setPoints([]))
      .finally(() => setLoading(false));
  }, [selectedSubjects, selectedChannels, yearStart, yearEnd]);

  const option = useMemo(() => {
    const xLabels: string[] = [];
    for (let y = yearStart; y <= yearEnd; y++) {
      for (let m = 1; m <= 12; m++) xLabels.push(`${y}-${String(m).padStart(2, '0')}`);
    }
    const seriesMap = new Map<string, Map<string, number>>();
    points.forEach((p) => {
      const sKey = `${p.department}·${p.subjectName}`;
      if (!seriesMap.has(sKey)) seriesMap.set(sKey, new Map());
      seriesMap.get(sKey)!.set(`${p.year}-${String(p.month).padStart(2, '0')}`, p.amount);
    });
    const series: any[] = [];
    Array.from(seriesMap.keys()).forEach((key, idx) => {
      const d = xLabels.map((x) => seriesMap.get(key)?.get(x) ?? null);
      series.push({ name: key, type: 'line', smooth: true, connectNulls: true, data: d, itemStyle: { color: CHART_COLORS[idx % CHART_COLORS.length] } });
    });
    return {
      xAxis: { type: 'category', data: xLabels, axisLabel: { rotate: 45 } },
      yAxis: { type: 'value', axisLabel: { formatter: (v: number) => formatMoney(v) } },
      legend: { top: 0, type: 'scroll' as const },
      series,
      tooltip: {
        trigger: 'axis',
        formatter: (params: any[]) => {
          const lines = [params[0].axisValue];
          params.forEach((p) => { if (p.value != null) lines.push(`${p.marker} ${p.seriesName}: ${formatMoney(p.value)}`); });
          return lines.join('<br/>');
        },
      },
    };
  }, [points, yearStart, yearEnd]);

  const subjectOptions = subjectDict.filter((s) => s.level === 2).map((s) => ({ label: `${s.name} (${s.category})`, value: s.code }));
  return (
    <>
      <Space wrap style={{ marginBottom: 12 }}>
        <span>科目：</span>
        <Select mode="multiple" value={selectedSubjects} onChange={setSelectedSubjects} options={subjectOptions} style={{ minWidth: 400 }} placeholder="选择科目" maxTagCount="responsive" />
        <span>渠道：</span>
        <Select mode="multiple" value={selectedChannels} onChange={setSelectedChannels} options={ALL_CHANNELS.map((c) => ({ label: c, value: c }))} style={{ minWidth: 260 }} placeholder="选择渠道" maxTagCount="responsive" />
        <span>起始年：</span>
        <InputNumber value={yearStart} onChange={(v) => v && setYearStart(v)} min={2022} max={2026} />
        <span>结束年：</span>
        <InputNumber value={yearEnd} onChange={(v) => v && setYearEnd(v)} min={2022} max={2026} />
      </Space>
      {loading ? <div style={{ textAlign: 'center', padding: 60 }}><Spin /></div> : <Chart option={option} style={{ height: 500 }} />}
    </>
  );
};

const ReportCompare: React.FC<{ year: number }> = ({ year }) => {
  const [month, setMonth] = useState<number>(0);
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    setLoading(true);
    fetch(`${API_BASE}/api/finance/report/compare?year=${year}&month=${month}`, { credentials: 'include' })
      .then((r) => r.json())
      .then((res) => setData(res.data))
      .catch(() => setData(null))
      .finally(() => setLoading(false));
  }, [year, month]);

  const barOption = useMemo(() => {
    if (!data) return {};
    const depts = Object.keys(data.data).filter((d) => d !== '汇总').sort();
    const kpiCodes = ['GMV_TOTAL', 'REV_MAIN', 'PROFIT_GROSS', 'NET_PROFIT'];
    const names = data.subjectNames || {};
    const series = kpiCodes.map((code, idx) => ({
      name: names[code] || code,
      type: 'bar',
      data: depts.map((d) => data.data[d]?.[code] ?? 0),
      itemStyle: { color: CHART_COLORS[idx % CHART_COLORS.length], borderRadius: [3, 3, 0, 0] },
    }));
    return {
      xAxis: { type: 'category', data: depts },
      yAxis: { type: 'value', axisLabel: { formatter: (v: number) => formatMoney(v) } },
      legend: { top: 0 },
      series,
      tooltip: {
        trigger: 'axis',
        formatter: (params: any[]) => {
          const lines = [params[0].axisValue];
          params.forEach((p) => lines.push(`${p.marker} ${p.seriesName}: ${formatMoney(p.value)}`));
          return lines.join('<br/>');
        },
      },
    };
  }, [data]);

  const pieOption = useMemo(() => {
    if (!data) return {};
    const depts = Object.keys(data.data).filter((d) => d !== '汇总').sort();
    const pieData = depts.map((d) => ({ name: d, value: Math.max(0, data.data[d]?.NET_PROFIT ?? 0) }));
    return {
      tooltip: { trigger: 'item', formatter: (p: any) => `${p.name}: ${formatMoney(p.value)} (${p.percent}%)` },
      legend: { bottom: 0 },
      series: [{ name: '净利润占比', type: 'pie', radius: ['40%', '65%'], data: pieData, label: { formatter: '{b}\n{d}%' } }],
      color: CHART_COLORS,
    };
  }, [data]);

  return (
    <>
      <Space style={{ marginBottom: 12 }}>
        <span>月份：</span>
        <Select value={month} onChange={setMonth} style={{ width: 120 }} options={[{ label: '全年合计', value: 0 }, ...Array.from({ length: 12 }, (_, i) => ({ label: `${i + 1}月`, value: i + 1 }))]} />
      </Space>
      {loading ? <div style={{ textAlign: 'center', padding: 60 }}><Spin /></div> : (
        <>
          <Chart option={barOption} style={{ height: 400 }} />
          <Chart option={pieOption} style={{ height: 400, marginTop: 16 }} />
        </>
      )}
    </>
  );
};

const ReportStructure: React.FC<{ year: number; department: string }> = ({ year, department }) => {
  const [month, setMonth] = useState<number>(0);
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    setLoading(true);
    fetch(`${API_BASE}/api/finance/report/structure?year=${year}&department=${encodeURIComponent(department)}&month=${month}`, { credentials: 'include' })
      .then((r) => r.json())
      .then((res) => setData(res.data))
      .catch(() => setData(null))
      .finally(() => setLoading(false));
  }, [year, department, month]);

  const pieOf = (items: any[], title: string) => ({
    tooltip: { trigger: 'item', formatter: (p: any) => `${p.name}: ${formatMoney(p.value)} (${p.percent}%)` },
    legend: { bottom: 0, type: 'scroll' as const },
    title: { text: title, left: 'center', textStyle: { fontSize: 14, color: '#334155' } },
    series: [{ name: title, type: 'pie', radius: ['40%', '65%'], center: ['50%', '45%'], data: items.map((it: any) => ({ name: it.name, value: Math.abs(it.amount) })), label: { formatter: '{b}\n{d}%' } }],
    color: CHART_COLORS,
  });

  const waterfallOption = useMemo(() => {
    if (!data?.waterfall || data.waterfall.length === 0) return {};
    const steps = data.waterfall;
    const xData = steps.map((s: any) => s.name);
    const values = steps.map((s: any) => s.amount);
    return {
      xAxis: { type: 'category', data: xData },
      yAxis: { type: 'value', axisLabel: { formatter: (v: number) => formatMoney(v) } },
      tooltip: { trigger: 'axis', formatter: (params: any[]) => `${params[0].axisValue}: ${formatMoney(params[0].value)}` },
      series: [{ name: '金额', type: 'bar', data: values, itemStyle: { color: '#1e40af', borderRadius: [3, 3, 0, 0] }, label: { show: true, position: 'top', formatter: (p: any) => formatMoney(p.value) } }],
    };
  }, [data]);

  return (
    <>
      <Space style={{ marginBottom: 12 }}>
        <span>月份：</span>
        <Select value={month} onChange={setMonth} style={{ width: 120 }} options={[{ label: '全年合计', value: 0 }, ...Array.from({ length: 12 }, (_, i) => ({ label: `${i + 1}月`, value: i + 1 }))]} />
      </Space>
      {loading ? <div style={{ textAlign: 'center', padding: 60 }}><Spin /></div> : !data ? <Empty /> : (
        <>
          <div style={{ marginBottom: 16 }}>
            <h4 style={{ margin: '16px 0 8px' }}>利润流向（GMV→收入→毛利→运营利润→净利润）</h4>
            <Chart option={waterfallOption} style={{ height: 350 }} />
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(360px, 1fr))', gap: 16 }}>
            {data.cost?.length > 0 && <Chart option={pieOf(data.cost, '成本构成')} style={{ height: 380 }} />}
            {data.salesExp?.length > 0 && <Chart option={pieOf(data.salesExp, '销售费用构成')} style={{ height: 380 }} />}
            {data.mgmtExp?.length > 0 && <Chart option={pieOf(data.mgmtExp, '管理费用构成')} style={{ height: 380 }} />}
          </div>
        </>
      )}
    </>
  );
};

const FinanceReportPage: React.FC = () => {
  const { session } = useAuth();
  const canImport = !!session && (session.isSuperAdmin || session.permissions.includes('finance.report:import'));
  const canExport = !!session && (session.isSuperAdmin || session.permissions.includes('data:export'));

  // 顶部筛选（给 其他 tab 用，单年份+单渠道）
  const [year, setYear] = useState<number>(DEFAULT_YEAR);
  const [department, setDepartment] = useState<string>('汇总');

  // 损益表 tab 专用筛选：年月区间 + 多渠道
  const [yearStart, setYearStart] = useState<number>(DEFAULT_YEAR);
  const [yearEnd, setYearEnd] = useState<number>(DEFAULT_YEAR);
  const [monthStart, setMonthStart] = useState<number>(1);
  const [monthEnd, setMonthEnd] = useState<number>(12);
  const [channels, setChannels] = useState<string[]>(['汇总']);

  const [data, setData] = useState<ReportData | null>(null);
  const [loading, setLoading] = useState(false);
  const [subjectDict, setSubjectDict] = useState<SubjectDict[]>([]);
  const [uploading, setUploading] = useState(false);
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
    fetch(`${API_BASE}/api/finance/report?${params.toString()}`, { credentials: 'include', signal: ctrl.signal })
      .then((r) => r.json())
      .then((res) => setData(res.data))
      .catch((e: any) => { if (e?.name !== 'AbortError') setData(null); })
      .finally(() => setLoading(false));
  }, [yearStart, yearEnd, monthStart, monthEnd, channels]);

  useEffect(() => {
    const t = setTimeout(() => fetchReport(), 250);
    return () => clearTimeout(t);
  }, [fetchReport]);

  useEffect(() => {
    fetch(`${API_BASE}/api/finance/report/subjects`, { credentials: 'include' })
      .then((r) => r.json())
      .then((res) => setSubjectDict(res.data?.subjects || []))
      .catch(() => setSubjectDict([]));
  }, []);

  const doUpload = async (file: Blob) => {
    const form = new FormData();
    form.append('file', file);
    setUploading(true);
    try {
      const res = await fetch(`${API_BASE}/api/finance/report/import`, { method: 'POST', credentials: 'include', body: form });
      const json = await res.json();
      if (json.code !== 200) {
        message.error(json.msg || '导入失败');
        return;
      }
      const { year: y, rowCount, unmapped } = json.data;
      message.success(`导入成功：${y}年 共 ${rowCount} 条记录`);
      if (unmapped?.length > 0) {
        Modal.warning({
          title: `有 ${unmapped.length} 个科目未映射到字典（数据未入库）`,
          width: 640,
          content: (
            <div style={{ maxHeight: 400, overflow: 'auto' }}>
              {unmapped.map((u: any, i: number) => (
                <div key={i} style={{ padding: '4px 0' }}>
                  <Tag color="orange">{u.sheet}</Tag>
                  <Tag>{u.department}</Tag>
                  父：{u.parent || '-'}，科目：<Text strong>{u.subject}</Text>
                </div>
              ))}
              <div style={{ marginTop: 8, color: '#f59e0b', fontSize: 12 }}>这些科目需要联系管理员补充到字典后重新上传</div>
            </div>
          ),
        });
      }
      fetchReport();
    } catch (e: any) {
      message.error('上传失败：' + e.message);
    } finally {
      setUploading(false);
    }
  };

  const uploadProps: UploadProps = {
    name: 'file',
    accept: '.xlsx',
    maxCount: 1,
    showUploadList: false,
    beforeUpload: (file) => {
      // 年份校验：文件名必须能推出年份（YYYY年财务管理报表.xlsx 或 YYYY年 前缀）
      const m = file.name.match(/(\d{4})\s*年/);
      if (!m) {
        message.error(`文件名 "${file.name}" 无法识别年份，请用"YYYY年财务管理报表.xlsx"格式`);
        return Upload.LIST_IGNORE;
      }
      const y = parseInt(m[1], 10);
      if (y < 2000 || y > 2100) {
        message.error(`年份 ${y} 不合理，请检查文件名`);
        return Upload.LIST_IGNORE;
      }
      Modal.confirm({
        title: '确认导入',
        content: (
          <div>
            <div>文件：<Text strong>{file.name}</Text></div>
            <div>年份：<Text strong style={{ color: '#1e40af' }}>{y} 年</Text></div>
            <div style={{ marginTop: 8, color: '#dc2626' }}>此操作会覆盖 {y} 年的全部财务报表数据，确认继续？</div>
          </div>
        ),
        okText: '确认导入',
        cancelText: '取消',
        onOk: () => doUpload(file),
      });
      return Upload.LIST_IGNORE;
    },
  };

  const doExport = () => {
    const params = new URLSearchParams({
      yearStart: String(yearStart),
      yearEnd: String(yearEnd),
      monthStart: String(monthStart),
      monthEnd: String(monthEnd),
      channels: channels.join(','),
    });
    window.open(`${API_BASE}/api/finance/report/export?${params.toString()}`, '_blank');
  };

  const reportFilter = (
    <div style={{ marginBottom: 12, padding: '8px 12px', background: '#f8fafc', borderRadius: 6, display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 8, justifyContent: 'space-between' }}>
      <Space wrap size="middle">
        <span>年份：</span>
        <Select value={yearStart} onChange={(v) => { setYearStart(v); if (v > yearEnd) setYearEnd(v); }} style={{ width: 90 }} options={YEAR_OPTIONS.map((y) => ({ label: y, value: y }))} />
        <span>至</span>
        <Select value={yearEnd} onChange={(v) => { setYearEnd(v); if (v < yearStart) setYearStart(v); }} style={{ width: 90 }} options={YEAR_OPTIONS.map((y) => ({ label: y, value: y }))} />
        <span style={{ marginLeft: 12 }}>月份：</span>
        <InputNumber value={monthStart} onChange={(v) => v && setMonthStart(v)} min={1} max={12} style={{ width: 70 }} />
        <span>至</span>
        <InputNumber value={monthEnd} onChange={(v) => v && setMonthEnd(v)} min={1} max={12} style={{ width: 70 }} />
        <span style={{ marginLeft: 12 }}>渠道：</span>
        <Checkbox.Group
          value={channels}
          onChange={(v) => setChannels(v as string[])}
          options={ALL_CHANNELS.map((c) => ({ label: c, value: c }))}
        />
      </Space>
      <Space>
        {canExport && <Button icon={<DownloadOutlined />} onClick={doExport}>导出 Excel</Button>}
        {canImport && (
          <Upload {...uploadProps}>
            <Button type="primary" icon={<UploadOutlined />} loading={uploading}>上传 Excel</Button>
          </Upload>
        )}
      </Space>
    </div>
  );

  return (
    <div style={{ padding: 16 }}>
      <Card title={<><FileTextOutlined /> 财务报表</>}>
        <Tabs
          defaultActiveKey="table"
          items={[
            { key: 'table', label: '损益表', children: (
              <>
                {reportFilter}
                <ReportTable data={data} loading={loading} />
              </>
            ) },
            { key: 'trend', label: '跨月跨年趋势', children: <ReportTrend year={year} subjectDict={subjectDict} /> },
            { key: 'compare', label: '渠道对比', children: <ReportCompare year={year} /> },
            { key: 'structure', label: '成本/费用结构', children: <ReportStructure year={year} department={department} /> },
          ]}
        />
      </Card>
      <style>{`.fin-row-highlight { background: rgba(30, 64, 175, 0.04); } .fin-row-highlight td { font-weight: 600; } .fin-row-detail td { background: #fafafa; } .fin-row-group td { background: #e2e8f0 !important; border-top: 1px solid #cbd5e1; border-bottom: 1px solid #cbd5e1; }`}</style>
    </div>
  );
};

export default FinanceReportPage;
