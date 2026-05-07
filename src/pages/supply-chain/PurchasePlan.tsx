import React, { useEffect, useState } from 'react';
import { Row, Col, Card, Table, Tag, Input, Select, Empty, Tooltip, Button, message, Tabs, Popover, Spin, Modal, Progress } from 'antd';
import { ExclamationCircleOutlined } from '@ant-design/icons';
import {
  AlertOutlined,
  CarOutlined,
  ToolOutlined,
  DollarOutlined,
  InfoCircleOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import PageLoading from '../../components/PageLoading';
import { API_BASE } from '../../config';

interface KPIs {
  urgentSku: number;
  inTransitOrders: number;
  inTransitSubcontract: number;
  recent30Amount: number;
}

interface MonthRow {
  month: string;
  amount: number;
}

interface VendorRow {
  vendorName: string;
  amount: number;
  orderCount: number;
}

interface SuggestRow {
  type: string;
  jkyCode: string;
  ysCode: string;
  goodsName: string;
  stock: number;
  dailyAvg: number;
  inTransit: number;
  inTransitSubcontract: number;
  suggestedQty: number;
  status: string;
  sellableDays: number;
  nextArriveDate: string;
  nextArriveDays: number;
  ysClassName: string;
  position: string;
  cateName: string;
}

const fmtAmt = (v: number) => v >= 10000 ? `¥${(v / 10000).toFixed(1)} 万` : `¥${v.toLocaleString()}`;
const fmtQty = (v: number) => v >= 10000 ? `${(v / 10000).toFixed(1)} 万` : v.toLocaleString();

const statusColor: Record<string, string> = {
  '断货': 'red',
  '紧急': 'volcano',
  '偏低': 'orange',
  '正常': 'green',
  '积压': 'purple',
};

const PurchasePlan: React.FC = () => {
  const [data, setData] = useState<{
    kpis: KPIs;
    monthlyTrend: MonthRow[];
    topVendors: VendorRow[];
    suggested: SuggestRow[];
    params: { finishedGoodsTargetDays: number; materialTargetDays: number };
  } | null>(null);
  const [loading, setLoading] = useState(true);
  const [syncing, setSyncing] = useState(false);
  const [typeFilter, setTypeFilter] = useState<'成品/半成品' | '原材料/包材' | '其他'>('成品/半成品');
  const isSalesType = typeFilter !== '原材料/包材'; // 成品/半成品 + 其他 都用吉客云销售口径(45天)，原材料/包材用用友消耗口径(90天)
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [keyword, setKeyword] = useState('');

  // v0.67: 在途订单详情按需异步加载 (hover Popover 触发)
  type InTransitOrder = {
    code: string; vendorName: string; orgName: string;
    vouchDate: string; arriveDate: string;
    totalQty: number; incomingQty: number; inTransitQty: number;
    statusText: string;
  };
  type InTransitDetail = {
    loading: boolean;
    purchaseOrders?: InTransitOrder[];
    subcontractOrders?: InTransitOrder[];
  };
  const [inTransitCache, setInTransitCache] = useState<Record<string, InTransitDetail>>({});
  const loadInTransitDetail = (goodsNo: string) => {
    if (!goodsNo) return;
    const cur = inTransitCache[goodsNo];
    if (cur && !cur.loading) return; // 已缓存
    setInTransitCache((c) => ({ ...c, [goodsNo]: { loading: true } }));
    fetch(`${API_BASE}/api/supply-chain/in-transit-detail?goodsNo=${encodeURIComponent(goodsNo)}`, { credentials: 'include' })
      .then((r) => r.json())
      .then((j) => {
        if (j.code === 200 && j.data) {
          setInTransitCache((c) => ({ ...c, [goodsNo]: {
            loading: false,
            purchaseOrders: j.data.purchaseOrders || [],
            subcontractOrders: j.data.subcontractOrders || [],
          }}));
        } else {
          setInTransitCache((c) => ({ ...c, [goodsNo]: { loading: false, purchaseOrders: [], subcontractOrders: [] } }));
        }
      })
      .catch(() => {
        setInTransitCache((c) => ({ ...c, [goodsNo]: { loading: false, purchaseOrders: [], subcontractOrders: [] } }));
      });
  };
  const renderInTransitPopover = (goodsNo: string, kind: 'purchase' | 'subcontract') => {
    const detail = inTransitCache[goodsNo];
    if (!detail || detail.loading) {
      return <div style={{ padding: 12, textAlign: 'center', color: '#94a3b8' }}><Spin size="small" /> 加载中...</div>;
    }
    const orders = kind === 'purchase' ? detail.purchaseOrders : detail.subcontractOrders;
    if (!orders || orders.length === 0) {
      return <div style={{ padding: 12, color: '#94a3b8' }}>暂无在途{kind === 'purchase' ? '采购' : '委外'}订单</div>;
    }
    const totalIn = orders.reduce((s, o) => s + (o.inTransitQty || 0), 0);
    return (
      <div style={{ maxHeight: 420, overflow: 'auto' }}>
        <div style={{ marginBottom: 6, fontSize: 12, color: '#64748b' }}>
          共 <b style={{ color: kind === 'purchase' ? '#1e40af' : '#7c3aed' }}>{orders.length}</b> 单, 在途合计 <b>{fmtQty(totalIn)}</b>
        </div>
        <Table
          size="small"
          pagination={false}
          dataSource={orders}
          rowKey={(r, i) => `${r.code}-${i}`}
          columns={[
            { title: '用友单号', dataIndex: 'code', width: 160,
              render: (v: string) => <span style={{ fontFamily: 'Consolas', fontSize: 12 }}>{v}</span> },
            { title: '供应商', dataIndex: 'vendorName', width: 180, ellipsis: true },
            { title: '开单', dataIndex: 'vouchDate', width: 95 },
            { title: '到货', dataIndex: 'arriveDate', width: 95,
              render: (v: string) => v ? v : <span style={{ color: '#cbd5e1' }}>—</span> },
            { title: '订单量', dataIndex: 'totalQty', width: 80, align: 'right',
              render: (v: number) => fmtQty(v) },
            { title: '已入', dataIndex: 'incomingQty', width: 80, align: 'right',
              render: (v: number) => <span style={{ color: '#16a34a' }}>{fmtQty(v)}</span> },
            { title: '在途', dataIndex: 'inTransitQty', width: 80, align: 'right',
              render: (v: number) => <span style={{ color: kind === 'purchase' ? '#1e40af' : '#7c3aed', fontWeight: 600 }}>{fmtQty(v)}</span> },
            { title: '状态', dataIndex: 'statusText', width: 110 },
          ]}
        />
      </div>
    );
  };

  const fetchData = () => {
    setLoading(true);
    fetch(`${API_BASE}/api/supply-chain/purchase-plan`, { credentials: 'include' })
      .then((r) => r.json())
      .then((j) => {
        if (j.code === 200) setData(j.data);
        setLoading(false);
      })
      .catch(() => setLoading(false));
  };

  useEffect(() => { fetchData(); }, []);

  // v0.55: layout-content padding-top:20 + overflow:auto 让 padding 区跟着滚, sticky 表头跟数据行重叠
  // MainLayout 用 inline style 设 padding, useEffect 覆盖会被 re-render 抹掉
  // 用 body class 配 CSS !important 强制覆盖, React 不动 body class
  useEffect(() => {
    document.body.classList.add('purchase-plan-no-content-padding');
    return () => document.body.classList.remove('purchase-plan-no-content-padding');
  }, []);

  // v0.71: 同步进度状态 (轮询展示)
  type SyncStep = { name: string; ins: number; upd: number; err: number; durationSec: number; failed: boolean; message?: string };
  type SyncProgress = {
    running: boolean; done: boolean;
    totalSteps: number; currentStep: number; currentName: string;
    results: SyncStep[]; elapsedSec: number; startedAt: string;
  };
  const [syncProgress, setSyncProgress] = useState<SyncProgress | null>(null);
  const lastSyncEndRef = React.useRef<number>(0);

  const handleSync = async () => {
    // 防御 1: 当前在同步
    if (syncing) {
      message.warning('已有同步任务在执行, 请稍候');
      return;
    }
    // 防御 2: Modal 还没关闭 (上一轮显示完成态)
    if (syncProgress) {
      message.warning('上一次同步结果还未关闭, 请先关闭进度框');
      return;
    }
    // 防御 3: 60 秒 cooldown (跟后端一致) — 防止异步竞态/误双击
    const now = Date.now();
    if (now - lastSyncEndRef.current < 60000 && lastSyncEndRef.current > 0) {
      const wait = Math.ceil((60000 - (now - lastSyncEndRef.current)) / 1000);
      message.warning(`上次刷新刚完成, ${wait} 秒后再试`);
      return;
    }

    // v0.80: 二次确认弹窗 — 防误点击 (同步耗时 4-6 分钟)
    const confirmed = await new Promise<boolean>((resolve) => {
      Modal.confirm({
        title: '确认刷新看板数据?',
        icon: <ExclamationCircleOutlined style={{ color: '#faad14' }} />,
        content: (
          <div style={{ fontSize: 13, lineHeight: 1.7, marginTop: 8 }}>
            <div>系统会拉取吉客云和用友里所有最新数据:</div>
            <div style={{ marginTop: 4 }}>　• 吉客云库存 (~1 分钟)</div>
            <div>　• 用友库存 / 采购单 / 委外单 / 领料消耗 (~3-5 分钟)</div>
            <div style={{ marginTop: 8, color: '#dc2626' }}>
              ⚠ 总耗时 4-6 分钟, 期间无法再触发新一轮
            </div>
            <div style={{ marginTop: 4, color: '#64748b' }}>
              系统每小时会自动同步一次, 没有特殊情况不用手动点
            </div>
          </div>
        ),
        okText: '确认刷新',
        cancelText: '再想想',
        okButtonProps: { type: 'primary' },
        onOk: () => resolve(true),
        onCancel: () => resolve(false),
      });
    });
    if (!confirmed) return;
    // v0.74 诊断: 记录 fetch 调用来源, 帮排查"为什么浏览器自动发第 2 次请求"
    console.log('🔍 [SYNC] handleSync 被调用, 时间=', new Date().toISOString(), 'stack=', new Error().stack);
    setSyncing(true);
    setSyncProgress({ running: true, done: false, totalSteps: 5, currentStep: 0, currentName: '准备中...', results: [], elapsedSec: 0, startedAt: '' });

    // 启动轮询
    const pollInterval = setInterval(async () => {
      try {
        const pr = await fetch(`${API_BASE}/api/supply-chain/sync-ys-progress`, { credentials: 'include' });
        const pj = await pr.json();
        if (pj.code === 200 && pj.data) {
          // v0.78: 防 polling 把已 done=true 的状态覆盖回 running=true (race condition)
          setSyncProgress((cur) => {
            if (cur && cur.done && !pj.data.done) return cur;
            return pj.data;
          });
        }
      } catch { /* 忽略瞬时错误 */ }
    }, 1500);

    try {
      console.log('🔍 [SYNC] 即将发出 POST /sync-ys-stock, 时间=', new Date().toISOString());
      const r = await fetch(`${API_BASE}/api/supply-chain/sync-ys-stock`, {
        method: 'POST', credentials: 'include',
      });
      const j = await r.json();
      console.log('🔍 [SYNC] POST 返回 code=', j.code, '时间=', new Date().toISOString());
      clearInterval(pollInterval);
      if (j.code === 200) {
        setSyncProgress((p) => p ? { ...p, running: false, done: true, results: j.data.steps || p.results, elapsedSec: j.data.durationSec } : p);
        message.success(`数据已刷新, 耗时 ${j.data.durationSec} 秒`, 5);
        fetchData();
      } else if (j.code === 429) {
        // v0.78: 被后端 cooldown 拒绝 → 必须关闭 Modal, 否则用户看到"僵尸 Modal"以为又在跑
        console.warn('🟡 [SYNC] 被后端 cooldown 拒绝 (上次同步刚完成):', j.msg);
        setSyncProgress(null);
        message.warning(j.msg || '上次刷新刚完成, 请稍后再试', 5);
      } else {
        message.error(`刷新失败: ${j.msg || '未知错误'}`, 5);
        setSyncProgress(null);
      }
    } catch (e: any) {
      clearInterval(pollInterval);
      message.error(`刷新异常: ${e?.message || e}`, 5);
      setSyncProgress(null);
    } finally {
      setSyncing(false);
      lastSyncEndRef.current = Date.now(); // v0.71.1 cooldown 起点
    }
  };

  if (loading) return <PageLoading />;
  if (!data) return <Empty description="暂无数据" />;

  const { kpis, monthlyTrend, topVendors, suggested, params } = data;

  // 筛选
  const filtered = suggested.filter((s) => {
    if (typeFilter && s.type !== typeFilter) return false;
    if (statusFilter && s.status !== statusFilter) return false;
    if (keyword && !(s.jkyCode.includes(keyword) || s.ysCode.includes(keyword) || s.goodsName.includes(keyword))) return false;
    return true;
  });

  // 4 KPI 卡定义
  const kpiCards = [
    {
      title: '紧急 SKU 数', value: kpis.urgentSku, suffix: '个',
      desc: '成品可售天数 < 7 天', icon: <AlertOutlined />, color: '#dc2626',
    },
    {
      title: '在途采购订单', value: kpis.inTransitOrders, suffix: '单',
      desc: '未入库的采购单', icon: <CarOutlined />, color: '#1e40af',
    },
    {
      title: '在途委外订单', value: kpis.inTransitSubcontract, suffix: '单',
      desc: '未关闭的委外', icon: <ToolOutlined />, color: '#7c3aed',
    },
    {
      title: '近30天采购额', value: fmtAmt(kpis.recent30Amount), suffix: '',
      desc: '基于 DB 内最近 30 天', icon: <DollarOutlined />, color: '#16a34a',
    },
  ];

  // 月度趋势 ECharts option
  const trendOption = {
    grid: { left: 60, right: 20, top: 30, bottom: 40 },
    tooltip: { trigger: 'axis', formatter: (p: any) => `${p[0].name}<br/>采购额: ${fmtAmt(p[0].value)}` },
    xAxis: { type: 'category', data: monthlyTrend.map((m) => m.month) },
    yAxis: { type: 'value', axisLabel: { formatter: (v: number) => `${(v / 10000).toFixed(0)}万` } },
    series: [{
      type: 'line', smooth: true, symbol: 'circle', symbolSize: 8,
      itemStyle: { color: '#1e40af' }, areaStyle: { color: 'rgba(30, 64, 175, 0.1)' },
      data: monthlyTrend.map((m) => m.amount),
    }],
  };

  // TOP10 供应商 ECharts option
  const vendorOption = {
    grid: { left: 200, right: 40, top: 10, bottom: 30 },
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' },
      formatter: (p: any) => `${p[0].name}<br/>采购额: ${fmtAmt(p[0].value)}<br/>订单数: ${p[0].data.orderCount}` },
    xAxis: { type: 'value', axisLabel: { formatter: (v: number) => `${(v / 10000).toFixed(0)}万` } },
    yAxis: {
      type: 'category', inverse: true,
      data: topVendors.map((v) => v.vendorName.length > 16 ? v.vendorName.slice(0, 16) + '…' : v.vendorName),
    },
    series: [{
      type: 'bar', itemStyle: { color: '#1e40af', borderRadius: [0, 4, 4, 0] },
      label: { show: true, position: 'right', formatter: (p: any) => fmtAmt(p.value) },
      data: topVendors.map((v) => ({ value: v.amount, orderCount: v.orderCount })),
    }],
  };

  return (
    <div style={{ paddingTop: 20 }}>
      <style>{`
        /* layout-content padding-top:20 + overflow:auto 让 padding 跟内容滚, sticky 表头被数据行穿透 */
        body.purchase-plan-no-content-padding .ant-layout-content { padding-top: 0 !important; }
        /* 打通 ancestor overflow 让 sticky 绑定到 .ant-layout-content */
        .purchase-plan-card { overflow: visible !important; }
        .purchase-plan-card .ant-card-body { overflow: visible !important; }
        .purchase-plan-card .ant-table-wrapper { overflow: visible !important; }
        .purchase-plan-card .ant-table { overflow: visible !important; }
        .purchase-plan-card .ant-table-container { overflow: visible !important; }
        .purchase-plan-card .ant-table-content { overflow: visible !important; }
        .purchase-plan-sticky .ant-table-thead > tr > th {
          position: sticky !important;
          top: 0 !important;
          z-index: 100 !important;
          background: #fafafa !important;
        }
      `}</style>

      {/* 顶部分类 Tab — 成品 / 包材 */}
      <Tabs
        activeKey={typeFilter}
        onChange={(k) => setTypeFilter(k as '成品/半成品' | '原材料/包材' | '其他')}
        items={[
          { key: '成品/半成品', label: <span style={{ fontSize: 15, fontWeight: 600 }}>📦 成品/半成品采购计划</span> },
          { key: '原材料/包材', label: <span style={{ fontSize: 15, fontWeight: 600 }}>🏷 原材料/包材采购计划</span> },
          { key: '其他', label: <span style={{ fontSize: 15, fontWeight: 600 }}>🎁 其他采购计划 (含广宣品)</span> },
        ]}
        size="large"
        style={{ marginBottom: 8 }}
      />

      {/* 状态分布 (公式说明已在列名 Tooltip 体现, 不重复) */}
      <div style={{ background: '#f8fafc', border: '1px solid #e2e8f0', borderRadius: 6,
                    padding: '8px 14px', marginBottom: 12, fontSize: 13 }}>
        <span style={{ color: '#1e293b', fontWeight: 600, marginRight: 6 }}>状态分布:</span>
        {['断货', '紧急', '偏低', '正常', '积压'].map((s) => {
          const cnt = suggested.filter((x) => x.type === typeFilter && x.status === s).length;
          return (
            <Tag key={s} color={statusColor[s]} style={{ marginRight: 6 }}>
              {s} {cnt}
            </Tag>
          );
        })}
        <span style={{ color: '#64748b' }}>当前 {typeFilter} 共 {suggested.filter((x) => x.type === typeFilter).length} 项</span>
      </div>

      {/* 4 KPI 卡片 */}
      <Row gutter={[12, 12]}>
        {kpiCards.map((k) => (
          <Col xs={24} sm={12} lg={6} key={k.title}>
            <Card bodyStyle={{ padding: 14 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <div>
                  <div style={{ fontSize: 12, color: '#64748b', marginBottom: 4 }}>{k.title}</div>
                  <div style={{ fontSize: 22, fontWeight: 700, color: k.color }}>
                    {k.value}<span style={{ fontSize: 12, fontWeight: 400, color: '#94a3b8', marginLeft: 4 }}>{k.suffix}</span>
                  </div>
                  <div style={{ fontSize: 11, color: '#94a3b8', marginTop: 4 }}>{k.desc}</div>
                </div>
                <div style={{ fontSize: 28, color: k.color, opacity: 0.6 }}>{k.icon}</div>
              </div>
            </Card>
          </Col>
        ))}
      </Row>

      {/* 月度趋势 + TOP10 供应商 */}
      <Row gutter={[12, 12]} style={{ marginTop: 12 }}>
        <Col xs={24} lg={12}>
          <Card title="近 6 月采购金额趋势" bodyStyle={{ padding: 12 }}>
            {monthlyTrend.length > 0 ? (
              <ReactECharts option={trendOption} style={{ height: 280 }} />
            ) : <Empty description="暂无数据" />}
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card title="TOP 10 供应商 (按采购金额)" bodyStyle={{ padding: 12 }}>
            {topVendors.length > 0 ? (
              <ReactECharts option={vendorOption} style={{ height: 280 }} />
            ) : <Empty description="暂无数据" />}
          </Card>
        </Col>
      </Row>

      {/* 建议采购清单 */}
      <Card
        className="purchase-plan-card"
        title={`建议采购清单 (共 ${filtered.length} 项, 按建议量倒序)`}
        style={{ marginTop: 12 }}
        extra={
          <div style={{ display: 'flex', gap: 8 }}>
            <Select value={statusFilter || undefined} onChange={(v) => setStatusFilter(v || '')} style={{ width: 110 }}
              placeholder="状态筛选" allowClear
              options={['断货', '紧急', '偏低', '正常', '积压'].map((s) => ({ value: s, label: s }))} />
            <Input.Search placeholder="搜索 吉客云/用友编码 / 名称" value={keyword}
              onChange={(e) => setKeyword(e.target.value)} style={{ width: 200 }} allowClear />
            <Tooltip title={
              <div style={{ fontSize: 12, lineHeight: 1.7 }}>
                <div><b>把吉客云和用友里所有最新数据拉到看板</b></div>
                <div style={{ marginTop: 4 }}>包含: 库存 / 采购单 / 委外单 / 领料消耗</div>
                <div style={{ marginTop: 4 }}>大概 4-6 分钟, 完成后所有标签的数据都是最新的</div>
                <div style={{ marginTop: 6, color: '#f59e0b' }}>没有特殊情况不用经常点, 系统每小时会自动拉一次</div>
              </div>
            }>
              <Button type="primary" icon={<SyncOutlined spin={syncing} />}
                loading={syncing}
                disabled={syncing || !!syncProgress}
                onClick={handleSync}>
                刷新看板数据
              </Button>
            </Tooltip>
          </div>
        }
      >
        <Table
          className="purchase-plan-sticky"
          dataSource={filtered}
          rowKey={(r) => `${r.type}-${r.jkyCode || r.ysCode}`}
          size="small"
          pagination={{ defaultPageSize: 50, pageSizeOptions: ['50', '100', '200'], showSizeChanger: true,
                        showTotal: (t) => `共 ${t} 条` }}
          columns={[
            { title: '分类', dataIndex: 'ysClassName', width: 110, align: 'center',
              filters: Array.from(new Set(filtered.map((r) => r.ysClassName).filter(Boolean)))
                .sort()
                .map((v) => ({ text: v, value: v })),
              onFilter: (val: any, r: SuggestRow) => r.ysClassName === val,
              render: (v: string, r: SuggestRow) => v
                ? <Tag color={r.type === '成品' ? 'geekblue' : 'orange'}>{v}</Tag>
                : <span style={{ color: '#cbd5e1' }}>—</span> },
            { title: '状态', dataIndex: 'status', width: 80, align: 'center',
              filters: ['断货', '紧急', '偏低', '正常', '积压'].map((s) => ({ text: s, value: s })),
              onFilter: (val: any, r: SuggestRow) => r.status === val,
              render: (s: string) => <Tag color={statusColor[s] || 'default'}>{s}</Tag>,
              sorter: (a: SuggestRow, b: SuggestRow) => {
                const order = ['断货', '紧急', '偏低', '正常', '积压'];
                return order.indexOf(a.status) - order.indexOf(b.status);
              } },
            { title: '吉客云编码', dataIndex: 'jkyCode', width: 120,
              render: (v: string, r: SuggestRow) => v
                ? v
                : <Tooltip title={r.type === '原材料/包材' ? '原材料/包材通常只在用友里流转, 没有吉客云编码是正常的' : '吉客云里没有这个商品的档案'}>
                    <span style={{ color: '#cbd5e1' }}>—</span>
                  </Tooltip> },
            { title: '用友编码', dataIndex: 'ysCode', width: 120,
              render: (v: string, r: SuggestRow) => v
                ? v
                : <Tooltip title={r.type === '成品' ? '用友里还没建这个商品的档案 (需要在用友录入对应的吉客云编码)' : '吉客云里还没建这个商品的档案'}>
                    <span style={{ color: '#cbd5e1' }}>—</span>
                  </Tooltip> },
            { title: '物料名称', dataIndex: 'goodsName', ellipsis: true },
            { title: '产品定位', dataIndex: 'position', width: 90, align: 'center',
              filters: Array.from(new Set(filtered.map((r) => r.position).filter(Boolean)))
                .sort()
                .map((v) => ({ text: v, value: v })),
              onFilter: (val: any, r: SuggestRow) => r.position === val,
              render: (v: string) => {
                if (!v) return <span style={{ color: '#cbd5e1' }}>—</span>;
                const colorMap: Record<string, string> = { 'S': 'red', 'A': 'volcano', 'B': 'gold', 'C': 'blue', 'D': 'default' };
                return <Tag color={colorMap[v] || 'default'}>{v}</Tag>;
              } },
            { title: '吉客云分类', dataIndex: 'cateName', width: 130, ellipsis: true,
              filters: Array.from(new Set(filtered.map((r) => r.cateName).filter(Boolean)))
                .sort()
                .map((v) => ({ text: v, value: v })),
              onFilter: (val: any, r: SuggestRow) => r.cateName === val,
              render: (v: string) => v || <span style={{ color: '#cbd5e1' }}>—</span> },
            { title: <Tooltip title={
                isSalesType ? (
                  <div style={{ fontSize: 12, lineHeight: 1.6 }}>
                    <div><b>当前库存</b> = 实物库存 - 已被订单锁定的</div>
                    <div style={{ marginTop: 4 }}>取自 <b>吉客云</b>, 仅 7 个核心仓相加:</div>
                    <div>　南京委外成品 / 天津委外 / 西安成品</div>
                    <div>　松鲜鲜&大地密码云仓 / 长沙委外成品</div>
                    <div>　安徽郎溪成品 / 南京分销虚拟仓</div>
                    <div style={{ marginTop: 4, color: '#dc2626' }}>　❌ 排除: 京东自营/天猫超市/朴朴 等平台外仓</div>
                    <div>　❌ 排除: 采购外仓 / 不合格仓 / 安徽香松</div>
                  </div>
                ) : (
                  <div style={{ fontSize: 12, lineHeight: 1.6 }}>
                    <div><b>当前库存</b> = 实物库存 - 已被订单锁定的</div>
                    <div style={{ marginTop: 4 }}>取自 <b>用友</b>, 所有 用友仓库相加</div>
                    <div style={{ marginTop: 4, color: '#dc2626' }}>　❌ 排除: 安徽香松组织</div>
                    <div>　❌ 排除: 固态/液态/半固态/广宣品/周边品(这些是成品分类)</div>
                  </div>
                )
              }><span>当前库存 <InfoCircleOutlined style={{ color: '#94a3b8' }} /></span></Tooltip>,
              dataIndex: 'stock', width: 110, align: 'right',
              render: (v: number) => fmtQty(v),
              sorter: (a: SuggestRow, b: SuggestRow) => a.stock - b.stock },
            { title: <Tooltip title={
                isSalesType ? (
                  <div style={{ fontSize: 12, lineHeight: 1.6 }}>
                    <div><b>日均销售</b> = 近 30 天销售出库 ÷ 30 天</div>
                    <div style={{ marginTop: 4 }}>取自 <b>吉客云</b> 的销售出库数据</div>
                    <div>仅累计上面 7 个核心仓的销量, 跟"当前库存"口径一致</div>
                  </div>
                ) : (
                  <div style={{ fontSize: 12, lineHeight: 1.6 }}>
                    <div><b>日均消耗</b> = 近 30 天领料消耗 ÷ 30 天</div>
                    <div style={{ marginTop: 4 }}>取自 <b>用友</b> 的材料出库单</div>
                    <div>排除安徽香松组织</div>
                  </div>
                )
              }><span>{isSalesType ? '日均销售' : '日均消耗'} <InfoCircleOutlined style={{ color: '#94a3b8' }} /></span></Tooltip>,
              dataIndex: 'dailyAvg', width: 140, align: 'right',
              render: (v: number) => v > 0 ? v.toLocaleString() : <span style={{ color: '#cbd5e1' }}>-</span> },
            { title: <Tooltip title={
                <div style={{ fontSize: 12, lineHeight: 1.6 }}>
                  <div><b>{isSalesType ? '可售天数' : '可用天数'}</b> = 当前库存 ÷ 日均{isSalesType ? '销售' : '消耗'}</div>
                  <div style={{ marginTop: 4 }}>含义: 不补货的话, 现有库存还能撑多少天</div>
                  <div style={{ marginTop: 4 }}>分档:</div>
                  <div>　🔴 断货: 库存 ≤ 0 但还在{isSalesType ? '卖' : '用'}</div>
                  <div>　🔴 紧急: 不够 7 天</div>
                  <div>　🟠 偏低: 7-14 天</div>
                  <div>　🟢 正常: 14-90 天</div>
                  <div>　🟣 积压: 超过 90 天</div>
                  <div>　— : 没{isSalesType ? '销售' : '消耗'}记录</div>
                </div>
              }><span>{isSalesType ? '可售天数' : '可用天数'} <InfoCircleOutlined style={{ color: '#94a3b8' }} /></span></Tooltip>,
              dataIndex: 'sellableDays', width: 110, align: 'right',
              render: (v: number) => {
                if (v < 0) return <span style={{ color: '#dc2626', fontWeight: 600 }}>断货</span>;
                if (v >= 9999) return <span style={{ color: '#cbd5e1' }}>-</span>;
                const c = v < 7 ? '#dc2626' : v < 14 ? '#f59e0b' : v > 90 ? '#7c3aed' : '#16a34a';
                return <span style={{ color: c, fontWeight: 600 }}>{v} 天</span>;
              },
              sorter: (a: SuggestRow, b: SuggestRow) => a.sellableDays - b.sellableDays },
            { title: <Tooltip title={
                <div style={{ fontSize: 12, lineHeight: 1.6 }}>
                  <div><b>在途采购</b> = 已下采购单但还没全部到货的剩余量</div>
                  <div style={{ marginTop: 4 }}>取自 <b>用友</b> 的采购订单</div>
                  <div style={{ marginTop: 4 }}>过滤规则:</div>
                  <div>　• 单据状态 = 已审核或部分入库</div>
                  <div>　• 预计 90 天内到货 (远期超期单不算)</div>
                  <div>　• 排除安徽香松供应商</div>
                </div>
              }><span>在途采购 <InfoCircleOutlined style={{ color: '#94a3b8' }} /></span></Tooltip>,
              dataIndex: 'inTransit', width: 100, align: 'right',
              render: (v: number, r: SuggestRow) => v > 0 ? (
                <Popover
                  content={renderInTransitPopover(r.jkyCode, 'purchase')}
                  trigger="hover"
                  placement="left"
                  overlayStyle={{ maxWidth: 820 }}
                  onOpenChange={(open) => open && loadInTransitDetail(r.jkyCode)}
                >
                  <span style={{ color: '#1e40af', cursor: 'help', borderBottom: '1px dashed #1e40af' }}>{fmtQty(v)}</span>
                </Popover>
              ) : <span style={{ color: '#cbd5e1' }}>—</span> },
            { title: <Tooltip title={
                <div style={{ fontSize: 12, lineHeight: 1.6 }}>
                  <div><b>在途委外</b> = 委外加工单已下但还没回成品的量</div>
                  <div style={{ marginTop: 4 }}>取自 <b>用友</b> 的委外加工单</div>
                  <div style={{ marginTop: 4 }}>过滤规则:</div>
                  <div>　• 单据未关闭</div>
                  <div>　• 预计 90 天内交货</div>
                  <div>　• 排除安徽香松组织</div>
                </div>
              }><span>在途委外 <InfoCircleOutlined style={{ color: '#94a3b8' }} /></span></Tooltip>,
              dataIndex: 'inTransitSubcontract', width: 100, align: 'right',
              render: (v: number, r: SuggestRow) => v > 0 ? (
                <Popover
                  content={renderInTransitPopover(r.jkyCode, 'subcontract')}
                  trigger="hover"
                  placement="left"
                  overlayStyle={{ maxWidth: 820 }}
                  onOpenChange={(open) => open && loadInTransitDetail(r.jkyCode)}
                >
                  <span style={{ color: '#7c3aed', cursor: 'help', borderBottom: '1px dashed #7c3aed' }}>{fmtQty(v)}</span>
                </Popover>
              ) : <span style={{ color: '#cbd5e1' }}>—</span> },
            { title: <Tooltip title={
                <div style={{ fontSize: 12, lineHeight: 1.6 }}>
                  <div><b>最近到货</b> = 所有在途单中最早到货那一天</div>
                  <div style={{ marginTop: 4 }}>采购+委外两类单一起比, 取最早的</div>
                  <div style={{ marginTop: 4 }}>📌 显示"(估)" = 采购员/委外没填具体到货日</div>
                  <div>　 系统按"开单日 + 30 天"估算</div>
                </div>
              }><span>最近到货 <InfoCircleOutlined style={{ color: '#94a3b8' }} /></span></Tooltip>,
              dataIndex: 'nextArriveDate', width: 140, align: 'center',
              render: (date: string, r: SuggestRow) => {
                if (!date) return <span style={{ color: '#cbd5e1' }}>—</span>;
                const d = r.nextArriveDays;
                if (d === 999) return <Tooltip title="采购员未填到货日期, 用 vouchdate+30天 估算"><span style={{ color: '#94a3b8' }}>{date} (估)</span></Tooltip>;
                let color = '#16a34a', label = `${d} 天后`;
                if (d < 0) { color = '#dc2626'; label = `逾期 ${-d} 天`; }
                else if (d <= 7) color = '#dc2626';
                else if (d <= 30) color = '#f59e0b';
                return <Tooltip title={`预计 ${date} 到货`}><span style={{ color, fontWeight: 600 }}>{label}</span></Tooltip>;
              },
              sorter: (a: SuggestRow, b: SuggestRow) => a.nextArriveDays - b.nextArriveDays },
            { title: <Tooltip title={
                <div style={{ fontSize: 12, lineHeight: 1.6 }}>
                  <div><b>建议采购量</b> = 目标天数 × 日均{isSalesType ? '销售' : '消耗'} - 当前库存 - 在途采购 - 在途委外</div>
                  <div style={{ marginTop: 4 }}>(算出来 ≤ 0 时取 0)</div>
                  <div style={{ marginTop: 4 }}>{isSalesType ? '📦' : '🏷'} {typeFilter} 目标备货: {isSalesType ? params.finishedGoodsTargetDays : params.materialTargetDays} 天</div>
                  <div style={{ marginTop: 4 }}>含义: 把库存补到能撑"目标天数", 减掉已经有的 + 在路上的</div>
                  <div style={{ marginTop: 4 }}>= 0: 库存 + 在途已经够用, 不用再下单</div>
                </div>
              }><span>建议采购量 <InfoCircleOutlined style={{ color: '#94a3b8' }} /></span></Tooltip>,
              dataIndex: 'suggestedQty', width: 130, align: 'right',
              render: (v: number) => <span style={{ fontWeight: 600 }}>{fmtQty(v)}</span>,
              sorter: (a: SuggestRow, b: SuggestRow) => a.suggestedQty - b.suggestedQty,
              defaultSortOrder: 'descend' },
          ]}
        />
      </Card>

      {/* v0.71: 同步进度 Modal */}
      <Modal
        open={!!syncProgress}
        title={syncProgress?.done ? '✅ 数据已刷新' : '🔄 正在刷新看板数据'}
        onCancel={() => syncProgress?.done && setSyncProgress(null)}
        closable={!!syncProgress?.done}
        maskClosable={false}
        keyboard={false}
        footer={syncProgress?.done ? [
          <Button key="close" type="primary" onClick={() => setSyncProgress(null)}>关闭</Button>
        ] : null}
        width={560}
      >
        {syncProgress && (
          <div>
            <Progress
              percent={syncProgress.totalSteps > 0 ? Math.round((syncProgress.results.length / syncProgress.totalSteps) * 100) : 0}
              status={syncProgress.done ? 'success' : 'active'}
              strokeColor={syncProgress.done ? '#52c41a' : '#1677ff'}
            />
            <div style={{ marginTop: 12, fontSize: 13, color: '#475569' }}>
              {!syncProgress.done && (
                <div>
                  ⏱ 已用时 <b>{syncProgress.elapsedSec}s</b> / 第 <b>{syncProgress.currentStep}/{syncProgress.totalSteps}</b> 步
                  <div style={{ marginTop: 4 }}>当前: <b style={{ color: '#1677ff' }}>{syncProgress.currentName || '准备中...'}</b></div>
                </div>
              )}
              {syncProgress.done && (
                <div style={{ marginBottom: 8 }}>
                  ✅ 总耗时 <b>{syncProgress.elapsedSec}s</b>
                </div>
              )}
            </div>
            <div style={{ marginTop: 12, padding: 12, background: '#f8fafc', borderRadius: 6, fontSize: 12 }}>
              <div style={{ fontWeight: 600, marginBottom: 6, color: '#1e293b' }}>步骤明细:</div>
              {syncProgress.results.length === 0 && !syncProgress.done && (
                <div style={{ color: '#94a3b8' }}>等待第一步开始...</div>
              )}
              {syncProgress.results.map((r, idx) => (
                <div key={idx} style={{ marginBottom: 4, color: r.failed ? '#dc2626' : '#16a34a' }}>
                  {r.failed ? '✗' : '✓'} <b>{r.name}</b>
                  <span style={{ marginLeft: 8, color: '#64748b' }}>
                    新增 {r.ins} / 更新 {r.upd} / 失败 {r.err} ({r.durationSec}s)
                  </span>
                  {r.failed && r.message && (
                    <div style={{ color: '#dc2626', marginLeft: 16, fontSize: 11 }}>{r.message}</div>
                  )}
                </div>
              ))}
              {!syncProgress.done && syncProgress.currentStep > syncProgress.results.length && (
                <div style={{ marginTop: 4, color: '#1677ff' }}>
                  ⏳ <b>{syncProgress.currentName}</b> 进行中...
                </div>
              )}
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
};

export default PurchasePlan;
