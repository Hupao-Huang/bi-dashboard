import React, { useEffect, useState } from 'react';
import { Row, Col, Card, Table, Tag, Input, Select, Empty, Tooltip, Button, message } from 'antd';
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
}

interface DetailRow {
  warehouse: string;
  org: string;
  stock: number;
  dailyAvg: number;
  inTransit: number;
  inTransitSubcontract: number;
  suggested: number;
  nextArriveDate: string;
  nextArriveDays: number;
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
  const [typeFilter, setTypeFilter] = useState<'全部' | '成品' | '包材'>('全部');
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [keyword, setKeyword] = useState('');
  const [detailMap, setDetailMap] = useState<Record<string, DetailRow[] | 'loading' | 'error'>>({});

  const fetchDetail = async (key: string, r: SuggestRow) => {
    setDetailMap((m) => ({ ...m, [key]: 'loading' }));
    try {
      const params = new URLSearchParams({
        ysCode: r.ysCode || '', jkyCode: r.jkyCode || '', type: r.type,
      }).toString();
      const resp = await fetch(`${API_BASE}/api/supply-chain/purchase-plan/detail?${params}`,
        { credentials: 'include' });
      const j = await resp.json();
      if (j.code === 200) {
        setDetailMap((m) => ({ ...m, [key]: j.data.rows || [] }));
      } else {
        setDetailMap((m) => ({ ...m, [key]: 'error' }));
      }
    } catch {
      setDetailMap((m) => ({ ...m, [key]: 'error' }));
    }
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

  const handleSync = async () => {
    setSyncing(true);
    const hide = message.loading('正在拉取用友 BIP 现存量, 通常 30~60 秒...', 0);
    try {
      const r = await fetch(`${API_BASE}/api/supply-chain/sync-ys-stock`, {
        method: 'POST', credentials: 'include',
      });
      const j = await r.json();
      hide();
      if (j.code === 200) {
        message.success(
          `同步完成: 新增 ${j.data.ins} / 更新 ${j.data.upd} / 失败 ${j.data.err} (耗时 ${j.data.durationSec}s)`,
          5,
        );
        fetchData();
      } else {
        message.error(`同步失败: ${j.msg || '未知错误'}`, 5);
      }
    } catch (e: any) {
      hide();
      message.error(`同步异常: ${e?.message || e}`, 5);
    } finally {
      setSyncing(false);
    }
  };

  if (loading) return <PageLoading />;
  if (!data) return <Empty description="暂无数据" />;

  const { kpis, monthlyTrend, topVendors, suggested, params } = data;

  // 筛选
  const filtered = suggested.filter((s) => {
    if (typeFilter !== '全部' && s.type !== typeFilter) return false;
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
      {/* 公式 + 数据来源说明 */}
      <div style={{ background: '#f8fafc', border: '1px solid #e2e8f0', borderRadius: 6,
                    padding: '10px 14px', marginBottom: 12, fontSize: 12, color: '#64748b' }}>
        <InfoCircleOutlined style={{ marginRight: 6, color: '#1e40af' }} />
        <span style={{ color: '#1e293b', fontWeight: 600 }}>建议采购量公式：</span>
        max(0, 目标天数 × 日均消耗 - 当前库存 - 在途采购 - 在途委外)；
        <span style={{ color: '#1e293b', fontWeight: 600, marginLeft: 8 }}>目标天数：</span>
        成品 {params.finishedGoodsTargetDays} 天 / 包材 {params.materialTargetDays} 天；
        <span style={{ color: '#1e293b', fontWeight: 600, marginLeft: 8 }}>日均：</span>
        成品=吉客云销量 / 包材=YS 材料出库单近30天 (真实消耗)
        <div style={{ marginTop: 4, color: '#94a3b8' }}>
          说明：包材/原料常规仅在用友 BIP 流转，吉客云编码列空属正常；成品 YS 编码列空表示 YS 端尚未建立货品档案。
        </div>
        <div style={{ marginTop: 6 }}>
          <span style={{ color: '#1e293b', fontWeight: 600, marginRight: 6 }}>状态分布:</span>
          {['断货', '紧急', '偏低', '正常', '积压'].map((s) => {
            const cnt = suggested.filter((x) => x.status === s).length;
            return (
              <Tag key={s} color={statusColor[s]} style={{ marginRight: 6 }}>
                {s} {cnt}
              </Tag>
            );
          })}
          <span style={{ color: '#64748b' }}>共 {suggested.length} 项</span>
        </div>
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
            <Select value={typeFilter} onChange={setTypeFilter} style={{ width: 100 }}
              options={[{ value: '全部', label: '全部' }, { value: '成品', label: '成品' }, { value: '包材', label: '包材' }]} />
            <Select value={statusFilter} onChange={setStatusFilter} style={{ width: 100 }}
              placeholder="状态" allowClear
              options={['断货', '紧急', '偏低', '正常', '积压'].map((s) => ({ value: s, label: s }))} />
            <Input.Search placeholder="搜索 吉客云/YS 编码 / 名称" value={keyword}
              onChange={(e) => setKeyword(e.target.value)} style={{ width: 200 }} allowClear />
            <Tooltip title="拉取用友 BIP 最新现存量并刷新看板, 自动定时: 09:30 / 14:00 / 18:00">
              <Button type="primary" icon={<SyncOutlined spin={syncing} />}
                loading={syncing} onClick={handleSync}>
                立即同步
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
          expandable={{
            expandedRowRender: (r) => {
              const key = `${r.type}-${r.jkyCode || r.ysCode}`;
              const d = detailMap[key];
              if (d === 'loading') return <div style={{ padding: 12, color: '#64748b' }}>加载中...</div>;
              if (d === 'error') return <div style={{ padding: 12, color: '#dc2626' }}>加载失败, 请重试</div>;
              if (!d) return null;
              if (d.length === 0) return <div style={{ padding: 12, color: '#64748b' }}>无明细数据</div>;
              return (
                <Table
                  size="small"
                  dataSource={d}
                  rowKey={(x) => `${x.warehouse}-${x.org}`}
                  pagination={false}
                  style={{ background: '#f8fafc', margin: '8px 0' }}
                  columns={[
                    { title: '仓库', dataIndex: 'warehouse', width: 160 },
                    { title: '组织', dataIndex: 'org', ellipsis: true },
                    { title: '库存', dataIndex: 'stock', width: 100, align: 'right',
                      render: (v: number) => fmtQty(v) },
                    { title: '日均消耗', dataIndex: 'dailyAvg', width: 100, align: 'right',
                      render: (v: number) => v > 0 ? v.toLocaleString() : <span style={{ color: '#cbd5e1' }}>—</span> },
                    { title: '在途采购', dataIndex: 'inTransit', width: 100, align: 'right',
                      render: (v: number) => v > 0 ? <span style={{ color: '#1e40af' }}>{fmtQty(v)}</span> : <span style={{ color: '#cbd5e1' }}>—</span> },
                    { title: '在途委外', dataIndex: 'inTransitSubcontract', width: 100, align: 'right',
                      render: (v: number) => v > 0 ? <span style={{ color: '#7c3aed' }}>{fmtQty(v)}</span> : <span style={{ color: '#cbd5e1' }}>—</span> },
                    { title: '最近到货', dataIndex: 'nextArriveDate', width: 130, align: 'center',
                      render: (date: string, x: DetailRow) => {
                        if (!date) return <span style={{ color: '#cbd5e1' }}>—</span>;
                        const dd = x.nextArriveDays;
                        if (dd === 999) return <span style={{ color: '#94a3b8' }}>{date} (估)</span>;
                        let color = '#16a34a', label = `${dd} 天后`;
                        if (dd < 0) { color = '#dc2626'; label = `逾期 ${-dd} 天`; }
                        else if (dd <= 7) color = '#dc2626';
                        else if (dd <= 30) color = '#f59e0b';
                        return <span style={{ color, fontWeight: 600 }}>{label}</span>;
                      } },
                    { title: '建议采购量', dataIndex: 'suggested', width: 110, align: 'right',
                      render: (v: number) => v > 0
                        ? <span style={{ fontWeight: 700, color: '#dc2626' }}>{fmtQty(v)}</span>
                        : <span style={{ color: '#cbd5e1' }}>—</span> },
                  ]}
                />
              );
            },
            onExpand: (expanded, r) => {
              const key = `${r.type}-${r.jkyCode || r.ysCode}`;
              if (expanded && !detailMap[key]) fetchDetail(key, r);
            },
          }}
          columns={[
            { title: '类型', dataIndex: 'type', width: 80, align: 'center',
              render: (t: string) => <Tag color={t === '成品' ? 'blue' : 'cyan'}>{t}</Tag> },
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
                : <Tooltip title={r.type === '包材/原料' ? '包材/原料常规仅在用友 BIP 流转，无吉客云编码属正常' : '吉客云端未维护此货品档案'}>
                    <span style={{ color: '#cbd5e1' }}>—</span>
                  </Tooltip> },
            { title: 'YS 编码', dataIndex: 'ysCode', width: 120,
              render: (v: string, r: SuggestRow) => v
                ? v
                : <Tooltip title={r.type === '成品' ? 'YS 端未建立此货品档案 (需要在用友 BIP 录入外部编码 = 吉客云 goods_no)' : '此包材尚未在 goods 表建立外部编码映射'}>
                    <span style={{ color: '#cbd5e1' }}>—</span>
                  </Tooltip> },
            { title: '物料名称', dataIndex: 'goodsName', ellipsis: true },
            { title: '当前库存', dataIndex: 'stock', width: 100, align: 'right',
              render: (v: number) => fmtQty(v),
              sorter: (a: SuggestRow, b: SuggestRow) => a.stock - b.stock },
            { title: '日均(销售/消耗)', dataIndex: 'dailyAvg', width: 130, align: 'right',
              render: (v: number) => v > 0 ? v.toLocaleString() : <span style={{ color: '#cbd5e1' }}>-</span> },
            { title: '可售天数', dataIndex: 'sellableDays', width: 100, align: 'right',
              render: (v: number) => {
                if (v < 0) return <span style={{ color: '#dc2626', fontWeight: 600 }}>断货</span>;
                if (v >= 9999) return <span style={{ color: '#cbd5e1' }}>-</span>;
                const c = v < 7 ? '#dc2626' : v < 14 ? '#f59e0b' : v > 90 ? '#7c3aed' : '#16a34a';
                return <span style={{ color: c, fontWeight: 600 }}>{v} 天</span>;
              },
              sorter: (a: SuggestRow, b: SuggestRow) => a.sellableDays - b.sellableDays },
            { title: '在途采购', dataIndex: 'inTransit', width: 90, align: 'right',
              render: (v: number) => v > 0 ? <span style={{ color: '#1e40af' }}>{fmtQty(v)}</span> : <span style={{ color: '#cbd5e1' }}>—</span> },
            { title: '在途委外', dataIndex: 'inTransitSubcontract', width: 90, align: 'right',
              render: (v: number) => v > 0 ? <span style={{ color: '#7c3aed' }}>{fmtQty(v)}</span> : <span style={{ color: '#cbd5e1' }}>—</span> },
            { title: '最近到货', dataIndex: 'nextArriveDate', width: 130, align: 'center',
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
            { title: '建议采购量', dataIndex: 'suggestedQty', width: 120, align: 'right',
              render: (v: number) => <span style={{ fontWeight: 600 }}>{fmtQty(v)}</span>,
              sorter: (a: SuggestRow, b: SuggestRow) => a.suggestedQty - b.suggestedQty,
              defaultSortOrder: 'descend' },
          ]}
        />
      </Card>
    </div>
  );
};

export default PurchasePlan;
