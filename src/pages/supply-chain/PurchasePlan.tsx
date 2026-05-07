import React, { useEffect, useState } from 'react';
import { Row, Col, Card, Table, Tag, Input, Select, Empty, Tooltip, Button, message, Tabs } from 'antd';
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
  const [typeFilter, setTypeFilter] = useState<'成品' | '包材/原料'>('成品');
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [keyword, setKeyword] = useState('');

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
        onChange={(k) => setTypeFilter(k as '成品' | '包材/原料')}
        items={[
          { key: '成品', label: <span style={{ fontSize: 15, fontWeight: 600 }}>📦 成品采购计划</span> },
          { key: '包材/原料', label: <span style={{ fontSize: 15, fontWeight: 600 }}>🏷 包材原料采购计划</span> },
        ]}
        size="large"
        style={{ marginBottom: 8 }}
      />

      {/* 公式 + 数据来源说明 */}
      <div style={{ background: '#f8fafc', border: '1px solid #e2e8f0', borderRadius: 6,
                    padding: '10px 14px', marginBottom: 12, fontSize: 12, color: '#64748b' }}>
        <InfoCircleOutlined style={{ marginRight: 6, color: '#1e40af' }} />
        {typeFilter === '成品' ? (
          <>
            <span style={{ color: '#1e293b', fontWeight: 600 }}>成品建议采购量：</span>
            max(0, {params.finishedGoodsTargetDays} 天 × 日均销售 - 当前库存 - 在途采购 - 在途委外)；
            <span style={{ color: '#1e293b', fontWeight: 600, marginLeft: 8 }}>当前库存：</span>
            取自吉客云, 仅 7 个核心仓 (已排除京东/天猫超市/朴朴 等平台外仓)；
            <span style={{ color: '#1e293b', fontWeight: 600, marginLeft: 8 }}>日均销售：</span>
            吉客云近 30 天销售出库 ÷ 30
          </>
        ) : (
          <>
            <span style={{ color: '#1e293b', fontWeight: 600 }}>包材原料建议采购量：</span>
            max(0, {params.materialTargetDays} 天 × 日均消耗 - 当前库存 - 在途采购 - 在途委外)；
            <span style={{ color: '#1e293b', fontWeight: 600, marginLeft: 8 }}>当前库存：</span>
            取自用友 BIP, 全部 YS 仓库相加 (排安徽香松)；
            <span style={{ color: '#1e293b', fontWeight: 600, marginLeft: 8 }}>日均消耗：</span>
            用友 BIP 近 30 天领料消耗 ÷ 30
          </>
        )}
        <div style={{ marginTop: 4, color: '#94a3b8' }}>
          {typeFilter === '成品'
            ? '说明: 成品 YS 编码列空表示用友 BIP 端尚未建立货品档案'
            : '说明: 包材原料常规仅在用友 BIP 流转, 吉客云编码列空属正常'}
        </div>
        <div style={{ marginTop: 6 }}>
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
            { title: <Tooltip title={
                typeFilter === '成品' ? (
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
                    <div style={{ marginTop: 4 }}>取自 <b>用友 BIP</b>, 所有 YS 仓库相加</div>
                    <div style={{ marginTop: 4, color: '#dc2626' }}>　❌ 排除: 安徽香松组织</div>
                    <div>　❌ 排除: 固态/液态/半固态/广宣品/周边品(这些是成品分类)</div>
                  </div>
                )
              }><span>当前库存 <InfoCircleOutlined style={{ color: '#94a3b8' }} /></span></Tooltip>,
              dataIndex: 'stock', width: 110, align: 'right',
              render: (v: number) => fmtQty(v),
              sorter: (a: SuggestRow, b: SuggestRow) => a.stock - b.stock },
            { title: <Tooltip title={
                typeFilter === '成品' ? (
                  <div style={{ fontSize: 12, lineHeight: 1.6 }}>
                    <div><b>日均销售</b> = 近 30 天销售出库 ÷ 30 天</div>
                    <div style={{ marginTop: 4 }}>取自 <b>吉客云</b> 的销售出库数据</div>
                    <div>仅累计上面 7 个核心仓的销量, 跟"当前库存"口径一致</div>
                  </div>
                ) : (
                  <div style={{ fontSize: 12, lineHeight: 1.6 }}>
                    <div><b>日均消耗</b> = 近 30 天领料消耗 ÷ 30 天</div>
                    <div style={{ marginTop: 4 }}>取自 <b>用友 BIP</b> 的材料出库单</div>
                    <div>排除安徽香松组织</div>
                  </div>
                )
              }><span>{typeFilter === '成品' ? '日均销售' : '日均消耗'} <InfoCircleOutlined style={{ color: '#94a3b8' }} /></span></Tooltip>,
              dataIndex: 'dailyAvg', width: 140, align: 'right',
              render: (v: number) => v > 0 ? v.toLocaleString() : <span style={{ color: '#cbd5e1' }}>-</span> },
            { title: <Tooltip title={
                <div style={{ fontSize: 12, lineHeight: 1.6 }}>
                  <div><b>{typeFilter === '成品' ? '可售天数' : '可用天数'}</b> = 当前库存 ÷ 日均{typeFilter === '成品' ? '销售' : '消耗'}</div>
                  <div style={{ marginTop: 4 }}>含义: 不补货的话, 现有库存还能撑多少天</div>
                  <div style={{ marginTop: 4 }}>分档:</div>
                  <div>　🔴 断货: 库存 ≤ 0 但还在{typeFilter === '成品' ? '卖' : '用'}</div>
                  <div>　🔴 紧急: 不够 7 天</div>
                  <div>　🟠 偏低: 7-14 天</div>
                  <div>　🟢 正常: 14-90 天</div>
                  <div>　🟣 积压: 超过 90 天</div>
                  <div>　— : 没{typeFilter === '成品' ? '销售' : '消耗'}记录</div>
                </div>
              }><span>{typeFilter === '成品' ? '可售天数' : '可用天数'} <InfoCircleOutlined style={{ color: '#94a3b8' }} /></span></Tooltip>,
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
                  <div style={{ marginTop: 4 }}>取自 <b>用友 BIP</b> 的采购订单</div>
                  <div style={{ marginTop: 4 }}>过滤规则:</div>
                  <div>　• 单据状态 = 已审核或部分入库</div>
                  <div>　• 预计 90 天内到货 (远期超期单不算)</div>
                  <div>　• 排除安徽香松供应商</div>
                </div>
              }><span>在途采购 <InfoCircleOutlined style={{ color: '#94a3b8' }} /></span></Tooltip>,
              dataIndex: 'inTransit', width: 100, align: 'right',
              render: (v: number) => v > 0 ? <span style={{ color: '#1e40af' }}>{fmtQty(v)}</span> : <span style={{ color: '#cbd5e1' }}>—</span> },
            { title: <Tooltip title={
                <div style={{ fontSize: 12, lineHeight: 1.6 }}>
                  <div><b>在途委外</b> = 委外加工单已下但还没回成品的量</div>
                  <div style={{ marginTop: 4 }}>取自 <b>用友 BIP</b> 的委外加工单</div>
                  <div style={{ marginTop: 4 }}>过滤规则:</div>
                  <div>　• 单据未关闭</div>
                  <div>　• 预计 90 天内交货</div>
                  <div>　• 排除安徽香松组织</div>
                </div>
              }><span>在途委外 <InfoCircleOutlined style={{ color: '#94a3b8' }} /></span></Tooltip>,
              dataIndex: 'inTransitSubcontract', width: 100, align: 'right',
              render: (v: number) => v > 0 ? <span style={{ color: '#7c3aed' }}>{fmtQty(v)}</span> : <span style={{ color: '#cbd5e1' }}>—</span> },
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
                  <div><b>建议采购量</b> = 目标天数 × 日均{typeFilter === '成品' ? '销售' : '消耗'} - 当前库存 - 在途采购 - 在途委外</div>
                  <div style={{ marginTop: 4 }}>(算出来 ≤ 0 时取 0)</div>
                  <div style={{ marginTop: 4 }}>{typeFilter === '成品' ? '📦 成品目标备货: 45 天' : '🏷 包材原料目标备货: 90 天'}</div>
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
    </div>
  );
};

export default PurchasePlan;
