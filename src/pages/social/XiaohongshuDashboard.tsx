import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Row, Col, Card, Table, Statistic, Tabs, Select, Empty } from 'antd';
import ReactECharts from '../../components/Chart';
import DateFilter from '../../components/DateFilter';
import PageLoading from '../../components/PageLoading';
import { API_BASE } from '../../config';
import { CHART_COLORS } from '../../chartTheme';

const XiaohongshuDashboard: React.FC = () => {
  const [tab, setTab] = useState<'note' | 'goods'>('note');
  const [filters, setFilters] = useState<any>({ shops: [], noteTypes: [], categories: [], latestDate: '' });
  const [shops, setShops] = useState<string[]>([]);
  const [noteType, setNoteType] = useState('');
  const [cat, setCat] = useState('');
  const [start, setStart] = useState('');
  const [end, setEnd] = useState('');
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const abortRef = useRef<AbortController | null>(null);

  // 初次拉 filters，默认时间范围 = 本月
  useEffect(() => {
    fetch(`${API_BASE}/api/xiaohongshu/filters`)
      .then((r) => r.json())
      .then((res) => {
        const f = res.data || {};
        setFilters({ shops: f.shops || [], noteTypes: f.noteTypes || [], categories: f.categories || [], latestDate: f.latestDate || '' });
        if (f.latestDate) {
          // 默认展示本月: 最新数据日所在月 1 号 ~ 最新数据日
          setStart(f.latestDate.slice(0, 7) + '-01');
          setEnd(f.latestDate);
        }
      })
      .catch(() => {});
  }, []);

  const fetchData = useCallback((t: string, s: string, e: string, shopArr: string[], nt: string, c: string) => {
    if (!e) return;
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    // 笔记tab: start/end = 笔记发布日期范围(后端按 note_create_time 筛+跨快照日聚合)
    // 商品tab: date = 最新数据日(单日快照), start/end = 趋势的数据日期范围
    const p = new URLSearchParams({ date: e, start: s, end: e });
    if (shopArr.length) p.set('shops', shopArr.join(','));
    if (t === 'note' && nt) p.set('note_type', nt);
    if (t === 'goods' && c) p.set('category_l1', c);
    fetch(`${API_BASE}/api/xiaohongshu/${t}?${p.toString()}`, { signal: ctrl.signal })
      .then((r) => r.json())
      .then((res) => {
        setData(res.data);
        setLoading(false);
      })
      .catch((err: any) => {
        if (err?.name !== 'AbortError') setLoading(false);
      });
  }, []);

  useEffect(() => {
    fetchData(tab, start, end, shops, noteType, cat);
  }, [fetchData, tab, start, end, shops, noteType, cat]);

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

  const trendOption = (trend: any[], leftKey: string, rightKey: string, leftName: string, rightName: string) => ({
    tooltip: { trigger: 'axis' },
    legend: { data: [leftName, rightName], top: 0 },
    grid: { left: 60, right: 60, top: 52, bottom: 28, containLabel: true },
    xAxis: { type: 'category', data: (trend || []).map((p) => p.date) },
    yAxis: [
      { type: 'value', name: leftName },
      { type: 'value', name: rightName },
    ],
    series: [
      { name: leftName, type: 'bar', yAxisIndex: 0, data: (trend || []).map((p) => p[leftKey]), itemStyle: { color: CHART_COLORS[0] } },
      { name: rightName, type: 'line', yAxisIndex: 1, smooth: true, data: (trend || []).map((p) => p[rightKey]), itemStyle: { color: CHART_COLORS[1] } },
    ],
  });

  const yuan = (v: number) => `¥${(v || 0).toFixed(2)}`;
  const noteColumns = [
    { title: '笔记标题', dataIndex: 'title', ellipsis: true, render: (t: string, r: any) => (r.url ? <a href={r.url} target="_blank" rel="noreferrer">{t}</a> : t) },
    { title: '类型', dataIndex: 'type', width: 70 },
    { title: '作者', dataIndex: 'author', width: 110, ellipsis: true },
    { title: '发布日期', dataIndex: 'pubDate', width: 108 },
    { title: '阅读', dataIndex: 'read', width: 80, sorter: (a: any, b: any) => a.read - b.read },
    { title: '点赞', dataIndex: 'like', width: 70 },
    { title: '收藏', dataIndex: 'collect', width: 70 },
    { title: '评论', dataIndex: 'comment', width: 70 },
    { title: '带货GMV', dataIndex: 'gmv', width: 100, render: yuan, sorter: (a: any, b: any) => a.gmv - b.gmv },
    { title: '关联商品', dataIndex: 'product', ellipsis: true },
  ];
  const goodsColumns = [
    { title: '商品名', dataIndex: 'name', ellipsis: true },
    { title: '一级品类', dataIndex: 'cat1', width: 140, ellipsis: true },
    { title: '访客', dataIndex: 'visitors', width: 80, sorter: (a: any, b: any) => a.visitors - b.visitors },
    { title: '加购', dataIndex: 'cart', width: 70 },
    { title: '支付金额', dataIndex: 'gmv', width: 100, render: yuan, sorter: (a: any, b: any) => a.gmv - b.gmv },
    { title: '订单', dataIndex: 'orders', width: 70 },
    { title: '件数', dataIndex: 'qty', width: 70 },
    { title: '客单价', dataIndex: 'aov', width: 90, render: yuan },
    { title: '退款', dataIndex: 'refund', width: 90, render: yuan },
  ];

  return (
    <div>
      <DateFilter start={start} end={end} onChange={(s, e) => { setStart(s); setEnd(e); }} />
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
            <Select
              allowClear placeholder="笔记类型(全部)" style={{ minWidth: 150 }}
              value={noteType || undefined} onChange={(v) => setNoteType(v || '')}
              options={(filters.noteTypes || []).map((s: string) => ({ label: s, value: s }))}
            />
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

          <Card title={tab === 'note' ? '趋势（按笔记发布日）' : `趋势（数据日期：${data.date || '-'}）`} style={{ marginBottom: 16 }}>
            {(data.trend || []).length > 0 ? (
              <ReactECharts
                lazyUpdate
                style={{ height: 350 }}
                option={
                  tab === 'note'
                    ? trendOption(data.trend, 'reads', 'gmv', '阅读量', '带货GMV')
                    : trendOption(data.trend, 'visitors', 'gmv', '访客', '支付金额')
                }
              />
            ) : (
              <Empty description="暂无数据" />
            )}
          </Card>

          <Card className="bi-table-card" title={tab === 'note' ? '明细 TOP50（按发布日期筛选）' : `明细 TOP50（数据日期：${data.date || '-'}）`}>
            <Table
              dataSource={data.detail || []}
              columns={tab === 'note' ? noteColumns : goodsColumns}
              rowKey={(_, i) => String(i)}
              size="small"
              pagination={false}
              scroll={{ x: 'max-content', y: 480 }}
            />
          </Card>
        </>
      )}
    </div>
  );
};

export default XiaohongshuDashboard;
