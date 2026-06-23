import React, { useEffect, useState, useCallback, useRef, useMemo } from 'react';
import { Row, Col, Card, Table, Statistic, Select, Empty, Input, Typography, Button } from 'antd';
import { SettingOutlined } from '@ant-design/icons';
import ReactECharts from '../../components/Chart';
import DateFilter from '../../components/DateFilter';
import PageLoading from '../../components/PageLoading';
import { API_BASE } from '../../config';
import { CHART_COLORS } from '../../chartTheme';
import CfMetricPicker, { CfPreset } from './CfMetricPicker';

const { Text } = Typography;

type ColMeta = { key: string; label: string; fmt: string; group?: string };

const CF_COLS_LS_KEY = 'cf_visible_cols_v1'; // 列选择记到浏览器本地，刷新后保留

// 数值格式化：money/cost=¥ / int=千分位 / rate=x.xx% / roi/num2=x.xx
const fmtVal = (v: number, fmt: string): string => {
  const n = Number(v) || 0;
  switch (fmt) {
    case 'money':
    case 'cost':
      return '¥' + n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    case 'int':
      return Math.round(n).toLocaleString('zh-CN');
    case 'rate':
      return n.toFixed(2) + '%';
    case 'roi':
    case 'num2':
      return n.toFixed(2);
    default:
      return String(n);
  }
};

// 选了单天时，把趋势区间往前扩到 14 天，避免只画 1 个点（feedback_trend_expand）
const expandTrendStart = (start: string, end: string): string => {
  if (!start || !end || start !== end) return start;
  const d = new Date(end + 'T00:00:00');
  if (isNaN(d.getTime())) return start;
  d.setDate(d.getDate() - 13);
  return d.toISOString().slice(0, 10);
};

// 单条笔记按数据更新日的每天走势：消费(柱) vs 7日支付金额(线)
const CfNoteTrend: React.FC<{ noteId: string; start: string; end: string }> = ({ noteId, start, end }) => {
  const [rows, setRows] = useState<any[] | null>(null);
  useEffect(() => {
    const p = new URLSearchParams({ note_id: noteId });
    const s = expandTrendStart(start, end);
    if (s) p.set('start', s);
    if (end) p.set('end', end);
    let alive = true;
    fetch(`${API_BASE}/api/xiaohongshu/chengfeng/note-trend?${p.toString()}`)
      .then((r) => r.json())
      .then((res) => { if (alive) setRows(res.data?.trend || []); })
      .catch(() => { if (alive) setRows([]); });
    return () => { alive = false; };
  }, [noteId, start, end]);

  if (rows === null) return <div style={{ padding: 16 }}>加载中…</div>;
  if (!rows.length) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="这段时间该笔记没有数据" />;
  return (
    <ReactECharts
      lazyUpdate
      style={{ height: 280 }}
      option={{
        tooltip: { trigger: 'axis' },
        legend: { data: ['消费', '7日支付金额'], top: 0 },
        grid: { left: 55, right: 55, top: 32, bottom: 24, containLabel: true },
        xAxis: { type: 'category', data: rows.map((p) => p.date) },
        yAxis: [{ type: 'value', name: '消费' }, { type: 'value', name: 'GMV' }],
        series: [
          { name: '消费', type: 'bar', yAxisIndex: 0, data: rows.map((p) => p.cost), itemStyle: { color: CHART_COLORS[0] } },
          { name: '7日支付金额', type: 'line', yAxisIndex: 1, smooth: true, data: rows.map((p) => p.gmv), itemStyle: { color: CHART_COLORS[1] } },
        ],
      }}
    />
  );
};

const ChengfengDashboard: React.FC = () => {
  const [shopsOpt, setShopsOpt] = useState<string[]>([]);
  const [columns, setColumns] = useState<ColMeta[]>([]);
  const [shops, setShops] = useState<string[]>([]);
  const [noteIdQuery, setNoteIdQuery] = useState('');
  const [start, setStart] = useState('');
  const [end, setEnd] = useState('');
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const abortRef = useRef<AbortController | null>(null);

  // 选列：从本地存储恢复；没存过则 null，待 columns 加载后默认全选（暂时全部展示字段）
  const [visibleKeys, setVisibleKeys] = useState<string[] | null>(() => {
    try {
      const saved = localStorage.getItem(CF_COLS_LS_KEY);
      if (saved != null) { const arr = JSON.parse(saved); if (Array.isArray(arr)) return arr; }
    } catch { /* ignore */ }
    return null;
  });
  const setCols = useCallback((keys: string[]) => {
    setVisibleKeys(keys);
    try { localStorage.setItem(CF_COLS_LS_KEY, JSON.stringify(keys)); } catch { /* ignore */ }
  }, []);

  // 自定义指标弹窗 + 常用方案
  const [pickerOpen, setPickerOpen] = useState(false);
  const [presets, setPresets] = useState<CfPreset[]>([]);
  const loadPresets = useCallback(() => {
    fetch(`${API_BASE}/api/xiaohongshu/chengfeng/presets`)
      .then((r) => r.json())
      .then((res) => setPresets(res.data?.presets || []))
      .catch(() => {});
  }, []);
  useEffect(() => { loadPresets(); }, [loadPresets]);
  const savePreset = useCallback((name: string, keys: string[]) => {
    fetch(`${API_BASE}/api/xiaohongshu/chengfeng/presets/save`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name, keys }),
    }).then(() => loadPresets()).catch(() => {});
  }, [loadPresets]);
  const deletePreset = useCallback((id: number) => {
    fetch(`${API_BASE}/api/xiaohongshu/chengfeng/presets/delete`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ id }),
    }).then(() => loadPresets()).catch(() => {});
  }, [loadPresets]);

  // 初次拉 filters：默认数据更新时间 = 最新一天
  useEffect(() => {
    fetch(`${API_BASE}/api/xiaohongshu/chengfeng/filters`)
      .then((r) => r.json())
      .then((res) => {
        const f = res.data || {};
        setShopsOpt(f.shops || []);
        const cols: ColMeta[] = f.columns || [];
        setColumns(cols);
        // 没在本地存过列选择 → 默认全部展示（不写本地，用户手动选过才记）
        setVisibleKeys((prev) => (prev == null ? cols.map((c) => c.key) : prev));
        if (f.latestDate) { setStart(f.latestDate); setEnd(f.latestDate); }
      })
      .catch(() => {});
  }, []);

  const fetchData = useCallback((s: string, e: string, shopArr: string[], idq: string) => {
    if (!e) return;
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    const p = new URLSearchParams();
    if (s) p.set('start', s);
    if (e) p.set('end', e);
    if (shopArr.length) p.set('shops', shopArr.join(','));
    if (idq) p.set('note_id_like', idq);
    fetch(`${API_BASE}/api/xiaohongshu/chengfeng/list?${p.toString()}`, { signal: ctrl.signal })
      .then((r) => r.json())
      .then((res) => { setData(res.data || null); setLoading(false); })
      .catch((err) => { if (err.name !== 'AbortError') setLoading(false); });
  }, []);

  useEffect(() => {
    if (end) fetchData(start, end, shops, noteIdQuery);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [start, end, shops, noteIdQuery]);

  // KPI 卡（仿千帆 bi-stat-card + accent 色）
  const cfCards = (k: any) => [
    { title: '总消费', value: k.cost, precision: 2, prefix: '¥', accent: '#ef4444' },
    { title: '总展现量', value: k.impression, accent: '#3b82f6' },
    { title: '总点击量', value: k.click, accent: '#8b5cf6' },
    { title: '7日支付GMV', value: k.payGmv, precision: 2, prefix: '¥', accent: '#10b981' },
    { title: '综合ROI', value: k.roi, precision: 2, accent: '#f59e0b' },
  ];

  // 明细表列：标题 + ID 固定左侧，其余按"已选指标的顺序"渲染（顺序来自自定义指标弹窗的拖拽结果）
  const tableColumns: any[] = useMemo(() => {
    const colMap: Record<string, ColMeta> = {};
    columns.forEach((c) => { colMap[c.key] = c; });
    const visible = (visibleKeys ?? []).map((k) => colMap[k]).filter(Boolean);
    return [
      {
        title: '笔记标题', dataIndex: 'title', key: 'title', fixed: 'left', width: 220, ellipsis: true,
        render: (v: string, row: any) =>
          row.url ? <a href={row.url} target="_blank" rel="noreferrer">{v || '(无标题)'}</a> : (v || '(无标题)'),
      },
      {
        title: '笔记/素材ID', dataIndex: 'noteId', key: 'noteId', fixed: 'left', width: 230, ellipsis: true,
        render: (v: string) => (v ? <Text copyable={{ text: v }} style={{ whiteSpace: 'nowrap' }}>{v}</Text> : '-'),
      },
      ...visible.map((c) => ({
        title: c.label,
        dataIndex: c.key,
        key: c.key,
        align: 'right' as const,
        width: c.label.length > 8 ? 150 : 120,
        render: (v: number) => fmtVal(v, c.fmt),
      })),
    ].map((c: any) => ({ ...c, onHeaderCell: () => ({ style: { whiteSpace: 'nowrap' } }) }));
  }, [columns, visibleKeys]);

  return (
    <div>
      <DateFilter label="数据更新时间" start={start} end={end} onChange={(s, e) => { setStart(s); setEnd(e); }} />
      <Card className="bi-filter-card" style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center' }}>
          <Select
            mode="multiple" allowClear placeholder="店铺(全部)" style={{ minWidth: 240 }}
            value={shops} onChange={setShops}
            options={shopsOpt.map((s) => ({ label: s, value: s }))}
          />
          <Input.Search
            placeholder="笔记/素材ID搜索(回车)"
            allowClear
            onSearch={(v) => setNoteIdQuery(v.trim())}
            onChange={(e) => { if (!e.target.value) setNoteIdQuery(''); }}
            style={{ width: 240 }}
          />
        </div>
      </Card>

      {loading ? (
        <PageLoading />
      ) : !data ? (
        <Empty description="暂无数据" />
      ) : (
        <>
          <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
            {cfCards(data.kpi || {}).map((c: any) => (
              <Col xs={12} sm={4} key={c.title}>
                <Card className="bi-stat-card" style={{ ['--accent-color' as any]: c.accent }}>
                  <Statistic title={c.title} value={c.value} precision={c.precision} prefix={c.prefix} />
                </Card>
              </Col>
            ))}
          </Row>

          <Card
            className="bi-table-card"
            title="明细 TOP50（点开每行 ▸ 看这条笔记每天走势）"
            extra={
              <Button icon={<SettingOutlined />} onClick={() => setPickerOpen(true)}>自定义指标</Button>
            }
          >
            <Table
              dataSource={data.detail || []}
              columns={tableColumns}
              rowKey={(r: any, i) => (r.noteId ? r.noteId : String(i))}
              size="small"
              pagination={false}
              scroll={{ x: 'max-content', y: 480 }}
              expandable={{
                expandedRowRender: (record: any) => <CfNoteTrend noteId={record.noteId} start={start} end={end} />,
                rowExpandable: (record: any) => !!record.noteId,
              }}
            />
          </Card>
        </>
      )}

      <CfMetricPicker
        open={pickerOpen}
        columns={columns}
        value={visibleKeys ?? []}
        defaultKeys={columns.map((c) => c.key)}
        presets={presets}
        onOk={(keys) => { setCols(keys); setPickerOpen(false); }}
        onCancel={() => setPickerOpen(false)}
        onSavePreset={savePreset}
        onDeletePreset={deletePreset}
      />
    </div>
  );
};

export default ChengfengDashboard;
