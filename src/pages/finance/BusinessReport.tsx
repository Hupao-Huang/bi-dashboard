// 业务预决算报表 (v0.59)
// 跑哥要求"前端所有的设计都和财务报表一样" — 1:1 复刻 Report.tsx 的 filter + 主表格
//
// 顶部 filter: 年份范围 + 月份范围 + 渠道 checkbox 多选 + 上传 Excel 按钮（占位）
// 主表格: 科目 (fixed) + 区间合计 + 各 (year, month) 矩阵
// 多 channel 时: 每个 (year, month) 列下展开 channel 子列

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Select, Spin, Table, Empty, Typography, Space, Button, TreeSelect, Modal, Upload, Radio, message } from 'antd';
import type { UploadProps } from 'antd';
import { UploadOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';
import { useAuth } from '../../auth/AuthContext';

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
  const { session } = useAuth();
  const canImport = !!session && (session.isSuperAdmin || session.permissions.includes('finance.business_report:import'));

  const [importModal, setImportModal] = useState<{
    open: boolean; step: 1 | 2; mode: 'full' | 'incremental';
    file: File | null; snapshotMonth: number | null; preview: any | null; loading: boolean;
  }>({ open: false, step: 1, mode: 'full', file: null, snapshotMonth: null, preview: null, loading: false });

  const closeImportModal = () => setImportModal({ open: false, step: 1, mode: 'full', file: null, snapshotMonth: null, preview: null, loading: false });
  const openImportModal = () => setImportModal({ open: true, step: 1, mode: 'full', file: null, snapshotMonth: null, preview: null, loading: false });

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

  const uploadProps: UploadProps = {
    name: 'file', accept: '.xlsx', maxCount: 1, showUploadList: false,
    beforeUpload: (file) => { setImportModal((s) => ({ ...s, file })); return Upload.LIST_IGNORE; },
  };

  const doPreview = async () => {
    if (!importModal.file) { message.error('请选择文件'); return; }
    setImportModal((s) => ({ ...s, loading: true }));
    const form = new FormData();
    form.append('file', importModal.file);
    form.append('mode', importModal.mode);
    if (importModal.snapshotMonth) form.append('snapshotMonth', String(importModal.snapshotMonth));
    try {
      const res = await fetch(`${API_BASE}/api/finance/business-report/import/preview`, { method: 'POST', credentials: 'include', body: form });
      const json = await res.json();
      if (json.code !== 200) { message.error(json.msg || '预览失败'); setImportModal((s) => ({ ...s, loading: false })); return; }
      setImportModal((s) => ({ ...s, step: 2, preview: json.data, loading: false }));
    } catch (e: any) { message.error('预览失败：' + e.message); setImportModal((s) => ({ ...s, loading: false })); }
  };

  const doConfirm = async () => {
    if (!importModal.preview?.token) return;
    setImportModal((s) => ({ ...s, loading: true }));
    try {
      const res = await fetch(`${API_BASE}/api/finance/business-report/import/confirm`, {
        method: 'POST', credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token: importModal.preview.token }),
      });
      const json = await res.json();
      if (json.code !== 200) { message.error(json.msg || '导入失败'); setImportModal((s) => ({ ...s, loading: false })); return; }
      message.success(`导入成功：${json.data.snapshotYear}年${json.data.snapshotMonth}月 共 ${json.data.rowCount} 条（${json.data.mode === 'incremental' ? '增量' : '全量覆盖'}）`);
      closeImportModal();
      fetchReport();
    } catch (e: any) { message.error('导入失败：' + e.message); setImportModal((s) => ({ ...s, loading: false })); }
  };

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
        {canImport && (
          <Button type="primary" icon={<UploadOutlined />} onClick={openImportModal}>上传 Excel</Button>
        )}
      </Space>
    </div>
  );

  return (
    <>
      {reportFilter}
      <BusinessReportTable data={data} loading={loading} />
      <Modal
        title={`业务报表导入 · 第 ${importModal.step} / 2 步`}
        open={importModal.open}
        width={importModal.step === 2 ? 960 : 560}
        onCancel={importModal.loading ? undefined : closeImportModal}
        footer={importModal.step === 1
          ? [<Button key="c" onClick={closeImportModal} disabled={importModal.loading}>取消</Button>,
             <Button key="p" type="primary" loading={importModal.loading} disabled={!importModal.file} onClick={doPreview}>下一步：预览变更</Button>]
          : [<Button key="b" onClick={() => setImportModal((s) => ({ ...s, step: 1, preview: null }))} disabled={importModal.loading}>← 返回上一步</Button>,
             <Button key="c" onClick={closeImportModal} disabled={importModal.loading}>取消</Button>,
             <Button key="ok" type="primary" danger loading={importModal.loading} onClick={doConfirm}>确认导入（不可撤销）</Button>]}
      >
        {importModal.step === 1 ? (
          <div>
            <div style={{ marginBottom: 8, fontWeight: 600 }}>① 选择导入模式：</div>
            <Radio.Group value={importModal.mode} onChange={(e) => setImportModal((s) => ({ ...s, mode: e.target.value }))} style={{ width: '100%' }}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <Radio value="full" style={{ alignItems: 'flex-start', padding: 12, border: '1px solid #e2e8f0', borderRadius: 6, margin: 0, background: importModal.mode === 'full' ? '#eff6ff' : '#fff' }}>
                  <div style={{ marginLeft: 4 }}>
                    <div style={{ fontWeight: 600, marginBottom: 4 }}>📊 全量（整版覆盖）</div>
                    <div style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>导入会清空该快照（年月）当前所有渠道数据，再写入整张表。文件里没有的渠道 / 子渠道会被删除。</div>
                  </div>
                </Radio>
                <Radio value="incremental" style={{ alignItems: 'flex-start', padding: 12, border: '1px solid #e2e8f0', borderRadius: 6, margin: 0, background: importModal.mode === 'incremental' ? '#eff6ff' : '#fff' }}>
                  <div style={{ marginLeft: 4 }}>
                    <div style={{ fontWeight: 600, marginBottom: 4 }}>📅 增量（按子渠道精确替换）</div>
                    <div style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>只删除并重写 Excel 里出现的（渠道 + 子渠道），文件里没有的子渠道保留旧值。</div>
                  </div>
                </Radio>
              </div>
            </Radio.Group>

            <div style={{ marginTop: 16, marginBottom: 8, fontWeight: 600 }}>② 快照月份：</div>
            <Select allowClear placeholder="文件名带「YYYY年MM月」自动识别，否则手选" style={{ width: '100%' }}
              value={importModal.snapshotMonth ?? undefined}
              onChange={(v) => setImportModal((s) => ({ ...s, snapshotMonth: v ?? null }))}
              options={Array.from({ length: 12 }, (_, i) => ({ label: `${i + 1} 月`, value: i + 1 }))} />

            <div style={{ marginTop: 16, marginBottom: 8, fontWeight: 600 }}>③ 选择 Excel 文件：</div>
            <Upload.Dragger {...uploadProps} style={{ padding: '12px 0' }}>
              <p style={{ margin: 0 }}><UploadOutlined style={{ fontSize: 28, color: '#1e40af' }} /></p>
              <p style={{ margin: '8px 0 4px', fontWeight: 600 }}>点击或拖拽 Excel 到此区域</p>
              <p style={{ margin: 0, fontSize: 12, color: 'var(--text-tertiary)' }}>仅支持 .xlsx 格式，文件名建议含「YYYY年MM月」（如 2026年04月业务预决算报表.xlsx）</p>
            </Upload.Dragger>

            {importModal.file && (
              <div style={{ marginTop: 12, padding: '10px 14px', background: '#f0f9ff', borderRadius: 6, border: '1px solid #bae6fd' }}>
                已选文件：<Typography.Text strong>{importModal.file.name}</Typography.Text>
              </div>
            )}

            <div style={{ marginTop: 12, padding: '8px 12px', background: '#fffbeb', borderRadius: 4, fontSize: 12, color: '#92400e' }}>
              ⚠️ 选错模式可能导致数据丢失（增量当作全量，会清空其他渠道）。下一步可以预览变更，确认后再写库。
            </div>
          </div>
        ) : (
          <Space direction="vertical" style={{ width: '100%' }}>
            <div>
              {importModal.preview?.diff?.isNewSnapshot ? '🆕 新增快照' : '⚠️ 覆盖已有快照'}
              {' '}{importModal.preview?.snapshotYear}年{importModal.preview?.snapshotMonth}月，
              共 {importModal.preview?.rowCount} 行；
              新增 {importModal.preview?.diff?.totalNew}、修改 {importModal.preview?.diff?.totalChanged}、删除 {importModal.preview?.diff?.totalDeleted}
            </div>
            <Table
              size="small"
              rowKey={(g: any) => `${g.channel}|${g.subChannel}`}
              dataSource={importModal.preview?.diff?.groups || []}
              pagination={false}
              columns={[
                { title: '渠道', dataIndex: 'channel' },
                { title: '子渠道', dataIndex: 'subChannel', render: (v: string) => v || '—' },
                { title: '动作', dataIndex: 'action' },
                { title: '旧行', dataIndex: 'oldRows' },
                { title: '新行', dataIndex: 'newRows' },
                { title: '变更格', dataIndex: 'changedCells' },
              ]}
              expandable={{
                expandedRowRender: (g: any) => (
                  <div style={{ maxHeight: 200, overflow: 'auto' }}>
                    {(g.cells || []).map((c: any, i: number) => (
                      <div key={i}>{c.parentSubject}/{c.subject} {c.periodMonth}月 {c.field}: {c.old ?? '无'} → {c.new ?? '无'}</div>
                    ))}
                    {g.truncated && <div>…(明细已截断,仅显示前 50 条)</div>}
                  </div>
                ),
              }}
            />
          </Space>
        )}
      </Modal>
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
      return <div style={{ textAlign: 'right', color: isChannel ? 'var(--text-tertiary)' : undefined, fontWeight: bold ? 700 : 400 }}>{fmtNum(c.budget)}</div>;
    }
    if (kind === 'actual') {
      if (c.actual == null) return <Text type="secondary">-</Text>;
      return <div style={{ textAlign: 'right', color: isChannel ? 'var(--text-tertiary)' : undefined, fontWeight: bold ? 700 : 500 }}>{fmtNum(c.actual)}</div>;
    }
    // ratio = 科目实际 / 营业收入（xlsx 原值"占比销售"列）
    if (c.ratio == null || !isFinite(c.ratio)) return <Text type="secondary">-</Text>;
    return <div style={{ textAlign: 'right', color: 'var(--text-tertiary)', fontSize: 12 }}>{(c.ratio * 100).toFixed(2)}%</div>;
  };
  const _suppress = achColor; void _suppress;
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
          return <div style={{ fontWeight: 700, color: 'var(--text-primary)', fontSize: 13 }}>{row.name}</div>;
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
