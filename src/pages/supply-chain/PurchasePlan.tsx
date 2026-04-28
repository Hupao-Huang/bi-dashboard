import React, { useEffect, useState } from 'react';
import { Row, Col, Card, Table, Tag, Input, Select, Empty } from 'antd';
import {
  AlertOutlined,
  CarOutlined,
  ToolOutlined,
  DollarOutlined,
  InfoCircleOutlined,
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
  goodsNo: string;
  goodsName: string;
  stock: number;
  dailyAvg: number;
  inTransit: number;
  suggestedQty: number;
  status: string;
  sellableDays: number;
}

const fmtAmt = (v: number) => v >= 10000 ? `¥${(v / 10000).toFixed(1)} 万` : `¥${v.toLocaleString()}`;
const fmtQty = (v: number) => v >= 10000 ? `${(v / 10000).toFixed(1)} 万` : v.toLocaleString();

const statusColor: Record<string, string> = {
  '断货': '#dc2626',
  '紧急': '#ea580c',
  '偏低': '#f59e0b',
  '正常': '#16a34a',
  '积压': '#7c3aed',
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
  const [typeFilter, setTypeFilter] = useState<'全部' | '成品' | '包材'>('全部');
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [keyword, setKeyword] = useState('');

  useEffect(() => {
    fetch(`${API_BASE}/api/supply-chain/purchase-plan`, { credentials: 'include' })
      .then((r) => r.json())
      .then((j) => {
        if (j.code === 200) setData(j.data);
        setLoading(false);
      })
      .catch(() => setLoading(false));
  }, []);

  if (loading) return <PageLoading />;
  if (!data) return <Empty description="暂无数据" />;

  const { kpis, monthlyTrend, topVendors, suggested, params } = data;

  // 筛选
  const filtered = suggested.filter((s) => {
    if (typeFilter !== '全部' && s.type !== typeFilter) return false;
    if (statusFilter && s.status !== statusFilter) return false;
    if (keyword && !(s.goodsNo.includes(keyword) || s.goodsName.includes(keyword))) return false;
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
    <div>
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
        title={`📋 建议采购清单 (共 ${filtered.length} 项, 按建议量倒序)`}
        style={{ marginTop: 12 }}
        extra={
          <div style={{ display: 'flex', gap: 8 }}>
            <Select value={typeFilter} onChange={setTypeFilter} style={{ width: 100 }}
              options={[{ value: '全部', label: '全部' }, { value: '成品', label: '成品' }, { value: '包材', label: '包材' }]} />
            <Select value={statusFilter} onChange={setStatusFilter} style={{ width: 100 }}
              placeholder="状态" allowClear
              options={['断货', '紧急', '偏低', '正常', '积压'].map((s) => ({ value: s, label: s }))} />
            <Input.Search placeholder="搜索物料编码/名称" value={keyword}
              onChange={(e) => setKeyword(e.target.value)} style={{ width: 200 }} allowClear />
          </div>
        }
      >
        <Table
          dataSource={filtered}
          rowKey={(r) => `${r.type}-${r.goodsNo}`}
          size="small"
          pagination={{ defaultPageSize: 50, pageSizeOptions: ['50', '100', '200'], showSizeChanger: true,
                        showTotal: (t) => `共 ${t} 条` }}
          rowClassName={(r) => r.status === '紧急' || r.status === '断货' ? 'bi-row-urgent' : ''}
          columns={[
            { title: '类型', dataIndex: 'type', width: 70, align: 'center',
              render: (t: string) => <Tag color={t === '成品' ? 'blue' : 'orange'}>{t}</Tag> },
            { title: '状态', dataIndex: 'status', width: 80, align: 'center',
              render: (s: string) => <Tag color={statusColor[s] || '#94a3b8'} style={{ color: '#fff', border: 'none' }}>{s}</Tag>,
              sorter: (a: SuggestRow, b: SuggestRow) => {
                const order = ['断货', '紧急', '偏低', '正常', '积压'];
                return order.indexOf(a.status) - order.indexOf(b.status);
              } },
            { title: '物料编码', dataIndex: 'goodsNo', width: 120 },
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
            { title: '在途量', dataIndex: 'inTransit', width: 110, align: 'right',
              render: (v: number) => v > 0 ? <span style={{ color: '#1e40af' }}>{fmtQty(v)}</span> : <span style={{ color: '#cbd5e1' }}>-</span> },
            { title: '🎯 建议采购量', dataIndex: 'suggestedQty', width: 130, align: 'right',
              render: (v: number) => <span style={{ fontWeight: 700, color: '#dc2626' }}>{fmtQty(v)}</span>,
              sorter: (a: SuggestRow, b: SuggestRow) => a.suggestedQty - b.suggestedQty,
              defaultSortOrder: 'descend' },
          ]}
        />
      </Card>

      <style>{`.bi-row-urgent { background: rgba(220, 38, 38, 0.04) !important; }
               .bi-row-urgent:hover td { background: rgba(220, 38, 38, 0.08) !important; }`}</style>
    </div>
  );
};

export default PurchasePlan;
