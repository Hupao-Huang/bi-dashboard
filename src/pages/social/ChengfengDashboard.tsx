import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Row, Col, Card, Table, Statistic, Select, Empty, Input, Typography, Tooltip, message } from 'antd';
import { CopyOutlined } from '@ant-design/icons';
import ReactECharts from '../../components/Chart';
import DateFilter from '../../components/DateFilter';
import PageLoading from '../../components/PageLoading';
import { API_BASE } from '../../config';
import { CHART_COLORS } from '../../chartTheme';

const { Text } = Typography;

type ColMeta = { key: string; label: string; fmt: string };

// 数值格式化：money=¥千分位2位 / int=千分位 / rate=x.xx% / roi=x.xx / cost=¥x.xx / num2=x.xx
const fmtVal = (v: number, fmt: string): string => {
  const n = Number(v) || 0;
  switch (fmt) {
    case 'money':
      return '¥' + n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    case 'cost':
      return '¥' + n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    case 'int':
      return Math.round(n).toLocaleString('zh-CN');
    case 'rate':
      return n.toFixed(2) + '%';
    case 'roi':
      return n.toFixed(2);
    case 'num2':
      return n.toFixed(2);
    default:
      return String(n);
  }
};

const wan = (v: number) => (Math.abs(v) >= 10000 ? `≈${(v / 10000).toFixed(1)}万` : '');

// 单条笔记按数据更新日的每天走势：消费(柱) vs 7日支付金额(线)
const CfNoteTrend: React.FC<{ noteId: string; start: string; end: string }> = ({ noteId, start, end }) => {
  const [rows, setRows] = useState<any[] | null>(null);
  useEffect(() => {
    const p = new URLSearchParams({ note_id: noteId });
    if (start) p.set('start', start);
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
      style={{ height: 260 }}
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

  // 初次拉 filters：默认数据更新时间 = 最新一天
  useEffect(() => {
    fetch(`${API_BASE}/api/xiaohongshu/chengfeng/filters`)
      .then((r) => r.json())
      .then((res) => {
        const f = res.data || {};
        setShopsOpt(f.shops || []);
        setColumns(f.columns || []);
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

  // 明细表列：笔记ID/标题固定左侧，其余指标列横向滚动，按 fmt 格式化右对齐
  const tableColumns: any[] = [
    {
      title: '笔记/素材ID', dataIndex: 'noteId', key: 'noteId', fixed: 'left', width: 200,
      render: (v: string) => (
        <span style={{ whiteSpace: 'nowrap' }}>
          <Text style={{ fontSize: 12 }}>{v}</Text>
          <Tooltip title="复制ID">
            <CopyOutlined
              style={{ marginLeft: 6, cursor: 'pointer', color: '#999' }}
              onClick={() => { navigator.clipboard?.writeText(v); message.success('已复制'); }}
            />
          </Tooltip>
        </span>
      ),
    },
    {
      title: '笔记标题', dataIndex: 'title', key: 'title', fixed: 'left', width: 220,
      render: (v: string, row: any) =>
        row.url ? <a href={row.url} target="_blank" rel="noreferrer">{v || '(无标题)'}</a> : (v || '(无标题)'),
    },
    ...columns.map((c) => ({
      title: c.label,
      dataIndex: c.key,
      key: c.key,
      align: 'right' as const,
      width: c.label.length > 8 ? 150 : 120,
      render: (v: number) => fmtVal(v, c.fmt),
    })),
  ];

  const kpi = data?.kpi || {};

  return (
    <div>
      <DateFilter label="数据更新时间" start={start} end={end} onChange={(s, e) => { setStart(s); setEnd(e); }} />
      <Card className="bi-filter-card" size="small" style={{ marginBottom: 12 }}>
        <Row gutter={[12, 12]} align="middle">
          <Col flex="auto">
            <Select
              mode="multiple" allowClear placeholder="全部店铺" style={{ minWidth: 260 }}
              value={shops} onChange={setShops}
              options={shopsOpt.map((s) => ({ label: s, value: s }))}
            />
          </Col>
          <Col>
            <Input.Search
              allowClear placeholder="笔记/素材ID 搜索" style={{ width: 220 }}
              onSearch={(v) => setNoteIdQuery(v.trim())}
            />
          </Col>
        </Row>
      </Card>

      <Row gutter={[12, 12]} style={{ marginBottom: 12 }}>
        {[
          { t: '总消费', v: kpi.cost, money: true },
          { t: '总展现量', v: kpi.impression },
          { t: '总点击量', v: kpi.click },
          { t: '7日支付GMV', v: kpi.payGmv, money: true },
          { t: '综合ROI', v: kpi.roi, roi: true },
        ].map((c) => (
          <Col xs={12} sm={8} md={Math.floor(24 / 5)} key={c.t}>
            <Card size="small">
              <Statistic
                title={c.t}
                value={c.v || 0}
                precision={c.roi ? 2 : c.money ? 2 : 0}
                prefix={c.money ? '¥' : ''}
              />
              {c.money && wan(c.v || 0) && <Text type="secondary" style={{ fontSize: 12 }}>{wan(c.v || 0)}</Text>}
            </Card>
          </Col>
        ))}
      </Row>

      {loading ? (
        <PageLoading />
      ) : (
        <Card size="small">
          <Table
            rowKey="noteId"
            size="small"
            columns={tableColumns}
            dataSource={data?.detail || []}
            scroll={{ x: 'max-content' }}
            pagination={{ pageSize: 50, showSizeChanger: true, showTotal: (t) => `共 ${t} 条笔记` }}
            expandable={{
              expandedRowRender: (row: any) => <CfNoteTrend noteId={row.noteId} start={start} end={end} />,
            }}
          />
        </Card>
      )}
    </div>
  );
};

export default ChengfengDashboard;
