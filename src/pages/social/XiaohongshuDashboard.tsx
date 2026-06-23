import React, { useEffect, useState, useCallback, useRef, useMemo } from 'react';
import { Row, Col, Card, Table, Statistic, Tabs, Select, Empty, DatePicker, Input, Typography, Button, message } from 'antd';
import { SettingOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import ReactECharts from '../../components/Chart';
import DateFilter from '../../components/DateFilter';
import PageLoading from '../../components/PageLoading';
import { API_BASE } from '../../config';
import { CHART_COLORS } from '../../chartTheme';
import CfMetricPicker, { CfColMeta, CfPreset, cfColSorter } from './CfMetricPicker';

const { RangePicker } = DatePicker;
const { Text } = Typography;

// 数值格式化(同乘风)：text=原值 / money=¥ / int=千分位 / rate=x.xx% / num2=x.xx
const fmtVal = (v: any, fmt?: string): React.ReactNode => {
  if (fmt === 'text') return (v === undefined || v === null || v === '') ? '-' : String(v);
  const n = Number(v) || 0;
  switch (fmt) {
    case 'money': return '¥' + n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    case 'int': return Math.round(n).toLocaleString('zh-CN');
    case 'rate': return n.toFixed(2) + '%';
    case 'num2': return n.toFixed(2);
    default: return String(n);
  }
};

// v2: 列 key 从 camelCase 改为后端 snake_case(数据驱动全字段), 旧 v1 值不兼容直接弃用
const NOTE_LS = 'xhs_note_cols_v2';
const GOODS_LS = 'xhs_goods_cols_v2';
const loadCols = (lsKey: string): string[] | null => {
  try {
    const saved = localStorage.getItem(lsKey);
    if (saved != null) { const arr = JSON.parse(saved); if (Array.isArray(arr)) return arr; }
  } catch { /* ignore */ }
  return null;
};

// 单条笔记按【数据更新日】的每天走势（明细行展开时拉取）
const NoteTrend: React.FC<{ noteId: string; start: string; end: string }> = ({ noteId, start, end }) => {
  const [rows, setRows] = useState<any[] | null>(null);
  useEffect(() => {
    const p = new URLSearchParams({ note_id: noteId });
    if (start) p.set('start', start);
    if (end) p.set('end', end);
    let alive = true;
    fetch(`${API_BASE}/api/xiaohongshu/note-trend?${p.toString()}`)
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
      style={{ height: 260 }}
      option={{
        tooltip: { trigger: 'axis' },
        legend: { data: ['阅读', '带货GMV'], top: 0 },
        grid: { left: 55, right: 55, top: 32, bottom: 24, containLabel: true },
        xAxis: { type: 'category', data: rows.map((p) => p.date) },
        yAxis: [
          { type: 'value', name: '阅读' },
          { type: 'value', name: 'GMV' },
        ],
        series: [
          { name: '阅读', type: 'bar', yAxisIndex: 0, data: rows.map((p) => p.reads), itemStyle: { color: CHART_COLORS[0] } },
          { name: '带货GMV', type: 'line', yAxisIndex: 1, smooth: true, data: rows.map((p) => p.gmv), itemStyle: { color: CHART_COLORS[1] } },
        ],
      }}
    />
  );
};

const XiaohongshuDashboard: React.FC = () => {
  const [tab, setTab] = useState<'note' | 'goods'>('note');
  const [filters, setFilters] = useState<any>({ shops: [], noteTypes: [], categories: [], latestDate: '', noteColumns: [], goodsColumns: [], noteDefaultKeys: [], goodsDefaultKeys: [] });
  const [shops, setShops] = useState<string[]>([]);
  const [noteType, setNoteType] = useState('');
  const [cat, setCat] = useState('');
  const [pubStart, setPubStart] = useState('');
  const [pubEnd, setPubEnd] = useState('');
  const [noteIdQuery, setNoteIdQuery] = useState('');
  const [start, setStart] = useState('');
  const [end, setEnd] = useState('');
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const abortRef = useRef<AbortController | null>(null);

  // 自定义指标：笔记/商品各一套(localStorage 分开存), null = 用后端默认列
  const [noteVisible, setNoteVisible] = useState<string[] | null>(() => loadCols(NOTE_LS));
  const [goodsVisible, setGoodsVisible] = useState<string[] | null>(() => loadCols(GOODS_LS));
  const [pickerOpen, setPickerOpen] = useState(false);
  const [presets, setPresets] = useState<CfPreset[]>([]);

  const scope = tab === 'note' ? 'xhs_note' : 'xhs_goods';
  const colMeta: CfColMeta[] = useMemo(() => (tab === 'note' ? (filters.noteColumns || []) : (filters.goodsColumns || [])), [tab, filters]);
  const defaultKeys: string[] = useMemo(() => (tab === 'note' ? (filters.noteDefaultKeys || []) : (filters.goodsDefaultKeys || [])), [tab, filters]);

  const loadPresets = useCallback((sc: string) => {
    fetch(`${API_BASE}/api/xiaohongshu/chengfeng/presets?scope=${sc}`)
      .then((r) => r.json())
      .then((res) => setPresets(res.data?.presets || []))
      .catch(() => {});
  }, []);
  useEffect(() => { loadPresets(scope); }, [scope, loadPresets]);

  const savePreset = useCallback((sc: string, name: string, keys: string[]) => {
    fetch(`${API_BASE}/api/xiaohongshu/chengfeng/presets/save`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ scope: sc, name, keys }),
    }).then((r) => r.json()).then((j) => {
      if (j.code && j.code !== 0) { message.error(j.msg || '保存常用方案失败'); return; }
      message.success('已保存常用方案'); loadPresets(sc);
    }).catch(() => message.error('保存常用方案失败'));
  }, [loadPresets]);
  const deletePreset = useCallback((sc: string, id: number) => {
    fetch(`${API_BASE}/api/xiaohongshu/chengfeng/presets/delete`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ id }),
    }).then((r) => r.json()).then((j) => {
      if (j.code && j.code !== 0) { message.error(j.msg || '删除失败'); return; }
      loadPresets(sc);
    }).catch(() => message.error('删除常用方案失败'));
  }, [loadPresets]);

  const setCols = useCallback((t: 'note' | 'goods', keys: string[]) => {
    if (t === 'note') { setNoteVisible(keys); try { localStorage.setItem(NOTE_LS, JSON.stringify(keys)); } catch { /* ignore */ } }
    else { setGoodsVisible(keys); try { localStorage.setItem(GOODS_LS, JSON.stringify(keys)); } catch { /* ignore */ } }
  }, []);

  // 初次拉 filters，默认数据更新时间 = 本月
  useEffect(() => {
    fetch(`${API_BASE}/api/xiaohongshu/filters`)
      .then((r) => r.json())
      .then((res) => {
        const f = res.data || {};
        setFilters({
          shops: f.shops || [], noteTypes: f.noteTypes || [], categories: f.categories || [], latestDate: f.latestDate || '',
          noteColumns: f.noteColumns || [], goodsColumns: f.goodsColumns || [],
          noteDefaultKeys: f.noteDefaultKeys || [], goodsDefaultKeys: f.goodsDefaultKeys || [],
        });
        if (f.latestDate) {
          setStart(f.latestDate.slice(0, 7) + '-01');
          setEnd(f.latestDate);
        }
      })
      .catch(() => {});
  }, []);

  // start/end = 数据更新时间(stat_date)；pubStart/pubEnd = 笔记发布时间(note_create_time)
  const fetchData = useCallback((t: string, s: string, e: string, shopArr: string[], nt: string, c: string, ps: string, pe: string, idq: string) => {
    if (!e) return;
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    const p = new URLSearchParams();
    if (shopArr.length) p.set('shops', shopArr.join(','));
    if (t === 'note') {
      if (s) p.set('start', s);
      if (e) p.set('end', e);
      if (nt) p.set('note_type', nt);
      if (ps) p.set('pub_start', ps);
      if (pe) p.set('pub_end', pe);
      if (idq) p.set('note_id_like', idq);
    } else {
      if (s) p.set('start', s);
      if (e) p.set('end', e);
      if (c) p.set('category_l1', c);
    }
    fetch(`${API_BASE}/api/xiaohongshu/${t}?${p.toString()}`, { signal: ctrl.signal })
      .then((r) => r.json())
      .then((res) => { setData(res.data); setLoading(false); })
      .catch((err: any) => { if (err?.name !== 'AbortError') setLoading(false); });
  }, []);

  useEffect(() => {
    fetchData(tab, start, end, shops, noteType, cat, pubStart, pubEnd, noteIdQuery);
  }, [fetchData, tab, start, end, shops, noteType, cat, pubStart, pubEnd, noteIdQuery]);

  const noteCards = (k: any) => [
    { title: '笔记数', value: k.notes, accent: '#ef4444' },
    { title: '总阅读', value: k.reads, accent: '#3b82f6' },
    { title: '总互动', value: k.interact, accent: '#8b5cf6' },
    { title: '带货GMV', value: k.gmv, precision: 2, prefix: '¥', accent: '#10b981' },
    { title: '带货订单', value: k.orders, accent: '#f59e0b' },
    { title: '转化率', value: (k.convRate || 0) * 100, precision: 2, suffix: '%', accent: '#ec4899' },
  ];
  const goodsCards = (k: any) => [
    { title: '商品数', value: k.goods, accent: '#ef4444' },
    { title: '总访客', value: k.visitors, accent: '#3b82f6' },
    { title: '支付金额', value: k.gmv, precision: 2, prefix: '¥', accent: '#10b981' },
    { title: '支付订单', value: k.orders, accent: '#f59e0b' },
    { title: '支付件数', value: k.qty, accent: '#8b5cf6' },
    { title: '退款金额', value: k.refund, precision: 2, prefix: '¥', accent: '#6b7280' },
  ];

  // 商品 tab 的整月趋势（按数据更新日）
  const goodsTrendOption = (trend: any[]) => ({
    tooltip: { trigger: 'axis' },
    legend: { data: ['访客', '支付金额'], top: 0 },
    grid: { left: 60, right: 60, top: 52, bottom: 28, containLabel: true },
    xAxis: { type: 'category', data: (trend || []).map((p) => p.date) },
    yAxis: [
      { type: 'value', name: '访客' },
      { type: 'value', name: '支付金额' },
    ],
    series: [
      { name: '访客', type: 'bar', yAxisIndex: 0, data: (trend || []).map((p) => p.visitors), itemStyle: { color: CHART_COLORS[0] } },
      { name: '支付金额', type: 'line', yAxisIndex: 1, smooth: true, data: (trend || []).map((p) => p.gmv), itemStyle: { color: CHART_COLORS[1] } },
    ],
  });

  // 明细表列：固定列 + 按已选指标顺序数据驱动渲染(顺序来自弹窗拖拽)
  const tableColumns = useMemo(() => {
    const metaMap: Record<string, CfColMeta> = {};
    colMeta.forEach((m) => { metaMap[m.key] = m; });
    const resolved = (tab === 'note' ? noteVisible : goodsVisible) ?? defaultKeys;
    const dynCols = resolved.map((k) => {
      const m = metaMap[k];
      if (!m) return null;
      const sorter = cfColSorter(k, m.fmt);
      return {
        title: m.label, dataIndex: k, key: k,
        align: m.fmt === 'text' ? ('left' as const) : ('right' as const),
        width: m.label.length > 8 ? 175 : 120, ellipsis: m.fmt === 'text',
        render: (v: any) => fmtVal(v, m.fmt),
        ...(sorter ? { sorter, sortDirections: ['descend', 'ascend'] as const } : {}),
      };
    }).filter(Boolean) as any[];

    let fixed: any[];
    if (tab === 'note') {
      fixed = [
        { title: '笔记标题', dataIndex: 'title', key: 'title', fixed: 'left', width: 220, ellipsis: true, render: (t: string, r: any) => (r.url ? <a href={r.url} target="_blank" rel="noreferrer">{t}</a> : t) },
        { title: '笔记ID', dataIndex: 'noteId', key: 'noteId', fixed: 'left', width: 230, ellipsis: true, render: (v: string) => (v ? <Text copyable={{ text: v }} style={{ whiteSpace: 'nowrap' }}>{v}</Text> : '-') },
      ];
    } else {
      fixed = [
        { title: '商品名', dataIndex: 'name', key: 'name', fixed: 'left', width: 240, ellipsis: true },
      ];
    }
    return [...fixed, ...dynCols].map((c: any) => ({ ...c, onHeaderCell: () => ({ style: { whiteSpace: 'nowrap' } }) }));
  }, [tab, noteVisible, goodsVisible, colMeta, defaultKeys]);

  const pickerValue = ((tab === 'note' ? noteVisible : goodsVisible) ?? defaultKeys);

  return (
    <div>
      <DateFilter label="数据更新时间" start={start} end={end} onChange={(s, e) => { setStart(s); setEnd(e); }} />
      <Card className="bi-filter-card" style={{ marginBottom: 16 }}>
        <Tabs
          activeKey={tab}
          onChange={(k) => { setTab(k as 'note' | 'goods'); setData(null); }}
          items={[{ key: 'note', label: '笔记效果' }, { key: 'goods', label: '商品销售' }]}
        />
        <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', marginTop: 8 }}>
          <Select
            mode="multiple" allowClear placeholder="店铺(全部)" style={{ minWidth: 240 }}
            value={shops} onChange={setShops}
            options={(filters.shops || []).map((s: string) => ({ label: s, value: s }))}
          />
          {tab === 'note' ? (
            <>
              <Select
                allowClear placeholder="笔记类型(全部)" style={{ minWidth: 150 }}
                value={noteType || undefined} onChange={(v) => setNoteType(v || '')}
                options={(filters.noteTypes || []).map((s: string) => ({ label: s, value: s }))}
              />
              <RangePicker
                placeholder={['笔记发布-起', '笔记发布-止']}
                disabledDate={(current: any) => current && current > dayjs().endOf('day')}
                value={pubStart && pubEnd ? [dayjs(pubStart), dayjs(pubEnd)] : null}
                onChange={(d: any) => { setPubStart(d?.[0]?.format('YYYY-MM-DD') || ''); setPubEnd(d?.[1]?.format('YYYY-MM-DD') || ''); }}
              />
              <Input.Search
                placeholder="笔记ID搜索(回车)"
                allowClear
                onSearch={(v) => setNoteIdQuery(v.trim())}
                onChange={(e) => { if (!e.target.value) setNoteIdQuery(''); }}
                style={{ width: 240 }}
              />
            </>
          ) : (
            <Select
              allowClear placeholder="一级品类(全部)" style={{ minWidth: 220 }}
              value={cat || undefined} onChange={(v) => setCat(v || '')}
              options={(filters.categories || []).map((s: string) => ({ label: s, value: s }))}
            />
          )}
        </div>
      </Card>

      {loading ? (
        <PageLoading />
      ) : !data ? (
        <Empty description="暂无数据" />
      ) : (
        <>
          <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
            {(tab === 'note' ? noteCards(data.kpi || {}) : goodsCards(data.kpi || {})).map((c: any) => (
              <Col xs={12} sm={4} key={c.title}>
                <Card className="bi-stat-card" style={{ ['--accent-color' as any]: c.accent }}>
                  <Statistic title={c.title} value={c.value} precision={c.precision} prefix={c.prefix} suffix={c.suffix} />
                </Card>
              </Col>
            ))}
          </Row>

          {tab === 'goods' && (
            <Card title="趋势（按数据更新日）" style={{ marginBottom: 16 }}>
              {(data.trend || []).length > 0 ? (
                <ReactECharts lazyUpdate style={{ height: 350 }} option={goodsTrendOption(data.trend)} />
              ) : (
                <Empty description="暂无数据" />
              )}
            </Card>
          )}

          <Card
            className="bi-table-card"
            title={tab === 'note' ? '明细（全部 · 点开每行 ▸ 看这条笔记每天走势）' : '明细（全部 · 数据更新时间累计）'}
            extra={<Button icon={<SettingOutlined />} onClick={() => setPickerOpen(true)}>自定义指标</Button>}
          >
            <Table
              dataSource={data.detail || []}
              columns={tableColumns}
              rowKey={(r: any, i) => (tab === 'note' && r.noteId ? r.noteId : String(i))}
              size="small"
              pagination={{ pageSize: 50, showSizeChanger: false, showTotal: (t) => `共 ${t} 条` }}
              scroll={{ x: 'max-content', y: 480 }}
              expandable={
                tab === 'note'
                  ? {
                      expandedRowRender: (record: any) => <NoteTrend noteId={record.noteId} start={start} end={end} />,
                      rowExpandable: (record: any) => !!record.noteId,
                    }
                  : undefined
              }
            />
          </Card>
        </>
      )}

      <CfMetricPicker
        open={pickerOpen}
        columns={colMeta}
        value={pickerValue}
        defaultKeys={defaultKeys}
        presets={presets}
        onOk={(keys) => { setCols(tab, keys); setPickerOpen(false); }}
        onCancel={() => setPickerOpen(false)}
        onSavePreset={(name, keys) => savePreset(scope, name, keys)}
        onDeletePreset={(id) => deletePreset(scope, id)}
      />
    </div>
  );
};

export default XiaohongshuDashboard;
