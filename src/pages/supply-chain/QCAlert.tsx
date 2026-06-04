import React, { useState, useEffect, useCallback } from 'react';
import { Card, Row, Col, Table, Statistic, Tag, Modal, message, Empty, Descriptions, Switch, Space, Button } from 'antd';
import ReactECharts from 'echarts-for-react';
import dayjs from 'dayjs';
import PageLoading from '../../components/PageLoading';
import DateFilter from '../../components/DateFilter';
import { API_BASE } from '../../config';

interface KPI { total: number; bad: number; badRate: number; supplierCnt: number; badArrivals: number; }
interface ArrivalRow {
  arrival: string; purchaseOrder: string; billType: string; supplier: string;
  inspectDate: string; total: number; pass: number; bad: number; badRate: number;
}
interface InsRow {
  id: string; code: string; inspectDate: string; materialCode: string; materialName: string;
  batch: string; result: string; inspectNum: number; badNum: number; qRate: number;
  stockStatus: string; handleType: string;
}
interface TrendRow { month: string; total: number; bad: number; badRate: number; }
interface SupplierRow { supplier: string; total: number; bad: number; badRate: number; }

const handleColor = (h: string): string => {
  if (h.includes('报废') || h.includes('废')) return 'red';
  if (h.includes('退货') || h.includes('拒收')) return 'volcano';
  if (h.includes('让步') || h.includes('挑选') || h.includes('返工')) return 'orange';
  return 'default';
};

const QCAlert: React.FC = () => {
  const [startDate, setStartDate] = useState<string>(dayjs().subtract(90, 'day').format('YYYY-MM-DD'));
  const [endDate, setEndDate] = useState<string>(dayjs().format('YYYY-MM-DD'));
  const [kpi, setKpi] = useState<KPI>({ total: 0, bad: 0, badRate: 0, supplierCnt: 0, badArrivals: 0 });
  const [arrivals, setArrivals] = useState<ArrivalRow[]>([]);
  const [trend, setTrend] = useState<TrendRow[]>([]);
  const [bySupplier, setBySupplier] = useState<SupplierRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [onlyBad, setOnlyBad] = useState(true);
  const [page, setPage] = useState(1);

  // 到货单详情缓存: 到货单号 → 检验单明细
  const [insCache, setInsCache] = useState<Record<string, InsRow[]>>({});
  const [insLoading, setInsLoading] = useState<Record<string, boolean>>({});

  // 到货单详情 modal
  const [arrivalOpen, setArrivalOpen] = useState(false);
  const [arrivalRow, setArrivalRow] = useState<ArrivalRow | null>(null);

  // 单张检验单详情 modal
  const [detailOpen, setDetailOpen] = useState(false);
  const [detailRow, setDetailRow] = useState<InsRow | null>(null);
  const [detail, setDetail] = useState<Record<string, unknown> | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const fetchAll = useCallback(() => {
    setLoading(true);
    setInsCache({});
    setPage(1);
    fetch(`${API_BASE}/api/supply-chain/qc-alert?start=${startDate}&end=${endDate}`, { credentials: 'include' })
      .then(r => r.json())
      .then(j => {
        if (j.code === 200) {
          setKpi(j.data?.kpi || { total: 0, bad: 0, badRate: 0, supplierCnt: 0, badArrivals: 0 });
          setArrivals(j.data?.arrivals || []);
          setTrend(j.data?.trend || []);
          setBySupplier(j.data?.bySupplier || []);
        } else {
          message.error(j.msg || '加载失败');
        }
      })
      .catch(err => message.error(`加载失败: ${err instanceof Error ? err.message : String(err)}`))
      .finally(() => setLoading(false));
  }, [startDate, endDate]);

  useEffect(() => { fetchAll(); }, [fetchAll]);

  const loadArrival = (vs: string) => {
    if (insCache[vs]) return;
    setInsLoading(p => ({ ...p, [vs]: true }));
    fetch(`${API_BASE}/api/supply-chain/qc-alert/arrival?vsourcecode=${encodeURIComponent(vs)}`, { credentials: 'include' })
      .then(r => r.json())
      .then(j => { if (j.code === 200) setInsCache(p => ({ ...p, [vs]: j.data?.list || [] })); })
      .catch(() => { /* 忽略 */ })
      .finally(() => setInsLoading(p => ({ ...p, [vs]: false })));
  };

  const openArrival = (rec: ArrivalRow) => {
    setArrivalRow(rec);
    setArrivalOpen(true);
    loadArrival(rec.arrival);
  };

  const openDetail = (row: InsRow) => {
    setDetailRow(row);
    setDetailOpen(true);
    setDetailLoading(true);
    setDetail(null);
    fetch(`${API_BASE}/api/supply-chain/qc-alert/detail?id=${encodeURIComponent(row.id)}`, { credentials: 'include' })
      .then(r => r.json())
      .then(j => { if (j.code === 200) setDetail(j.data?.detail || null); })
      .catch(() => { /* 忽略 */ })
      .finally(() => setDetailLoading(false));
  };

  if (loading && arrivals.length === 0) return <PageLoading />;

  const sup = bySupplier.slice(0, 12).slice().reverse();
  const supplierOption = {
    grid: { left: 160, right: 50, top: 10, bottom: 20 },
    tooltip: {
      trigger: 'axis',
      formatter: (p: any) => {
        const s = sup[p[0].dataIndex];
        return `${s.supplier}<br/>不合格 ${s.bad} 单 / 已检 ${s.total} 单<br/>不合格率 ${s.badRate.toFixed(1)}%`;
      },
    },
    xAxis: { type: 'value' },
    yAxis: { type: 'category', data: sup.map(s => s.supplier), axisLabel: { width: 150, overflow: 'truncate' } },
    series: [{
      type: 'bar', data: sup.map(s => s.bad),
      label: { show: true, position: 'right', formatter: (p: any) => `${sup[p.dataIndex].bad} (${sup[p.dataIndex].badRate.toFixed(0)}%)` },
      itemStyle: { color: '#cf1322' },
    }],
  };

  const trendOption = {
    grid: { left: 50, right: 50, top: 30, bottom: 30 },
    tooltip: { trigger: 'axis' },
    legend: { data: ['不合格单数', '不合格率%'], top: 0 },
    xAxis: { type: 'category', data: trend.map(t => t.month) },
    yAxis: [
      { type: 'value', name: '单数' },
      { type: 'value', name: '率%', axisLabel: { formatter: '{value}%' } },
    ],
    series: [
      { name: '不合格单数', type: 'bar', data: trend.map(t => t.bad), itemStyle: { color: '#faad14' } },
      { name: '不合格率%', type: 'line', yAxisIndex: 1, data: trend.map(t => Number(t.badRate.toFixed(2))), itemStyle: { color: '#cf1322' }, smooth: true },
    ],
  };

  const shownArrivals = onlyBad ? arrivals.filter(a => a.bad > 0) : arrivals;

  const arrivalColumns = [
    { title: '到货日', dataIndex: 'inspectDate', key: 'inspectDate', width: 110 },
    { title: '采购单号', dataIndex: 'purchaseOrder', key: 'purchaseOrder', width: 170,
      render: (v: string) => v || <span style={{ color: '#bbb' }}>—</span> },
    { title: '到货单号', dataIndex: 'arrival', key: 'arrival', width: 160 },
    { title: '来源', dataIndex: 'billType', key: 'billType', width: 90, render: (v: string) => <Tag>{v}</Tag> },
    { title: '供应商', dataIndex: 'supplier', key: 'supplier', ellipsis: true },
    { title: '检验', dataIndex: 'total', key: 'total', width: 70, align: 'right' as const, render: (v: number) => `${v} 张` },
    { title: '合格', dataIndex: 'pass', key: 'pass', width: 70, align: 'right' as const,
      render: (v: number) => <span style={{ color: '#3f8600' }}>{v}</span> },
    { title: '不合格', dataIndex: 'bad', key: 'bad', width: 80, align: 'right' as const,
      render: (v: number) => v > 0 ? <Tag color="red">{v}</Tag> : <span style={{ color: '#bbb' }}>0</span> },
    { title: '不合格率', dataIndex: 'badRate', key: 'badRate', width: 90, align: 'right' as const,
      render: (v: number) => `${v.toFixed(1)}%` },
    { title: '操作', key: 'op', width: 100, fixed: 'right' as const,
      render: (_: unknown, r: ArrivalRow) => <Button size="small" type="link" onClick={() => openArrival(r)}>查看详情</Button> },
  ];

  // 到货单详情里的检验单明细列
  const insColumns = [
    { title: '检验单号', dataIndex: 'code', key: 'code', width: 160,
      render: (v: string, r: InsRow) => <a onClick={() => openDetail(r)}>{v}</a> },
    { title: '物料', dataIndex: 'materialName', key: 'materialName', ellipsis: true,
      render: (v: string, r: InsRow) => <span>{v} <span style={{ color: '#94a3b8' }}>({r.materialCode})</span></span> },
    { title: '批次', dataIndex: 'batch', key: 'batch', width: 120 },
    { title: '判定', dataIndex: 'result', key: 'result', width: 80,
      render: (v: string) => v === '2' ? <Tag color="red">不合格</Tag> : <Tag color="green">合格</Tag> },
    { title: '检验数', dataIndex: 'inspectNum', key: 'inspectNum', width: 70, align: 'right' as const, render: (v: number) => v ? v.toFixed(0) : '—' },
    { title: '不合格数', dataIndex: 'badNum', key: 'badNum', width: 80, align: 'right' as const, render: (v: number) => v ? v.toFixed(0) : '—' },
    { title: '库存状态', dataIndex: 'stockStatus', key: 'stockStatus', width: 90,
      render: (v: string) => v ? <Tag color={v.includes('合格') && !v.includes('不') ? 'green' : 'red'}>{v}</Tag> : '—' },
    { title: '处理方式', dataIndex: 'handleType', key: 'handleType', width: 90,
      render: (v: string) => v ? <Tag color={handleColor(v)}>{v}</Tag> : '—' },
  ];

  const ds = (k: string): string => {
    const v = detail?.[k];
    if (v === undefined || v === null || v === '') return '—';
    return String(v);
  };

  return (
    <div style={{ padding: 16 }}>
      <DateFilter start={startDate} end={endDate} onChange={(s, e) => { setStartDate(s); setEndDate(e); }} />
      <div style={{ color: '#888', fontSize: 13, marginBottom: 12 }}>
        🔬 来料/入库质检预警：按到货单看每批货的检验判定（来源用友来料检验单，每天 09:40 自动更新）。点「查看详情」看这批货下每张检验单合格/不合格。
      </div>

      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={6}><Card className="bi-stat-card"><Statistic title="不合格检验单" value={kpi.bad} suffix="单" /></Card></Col>
        <Col span={6}><Card className="bi-stat-card"><Statistic title="已检单数" value={kpi.total} suffix="单" /></Card></Col>
        <Col span={6}><Card className="bi-stat-card"><Statistic title="不合格率" value={kpi.badRate} precision={2} suffix="%" /></Card></Col>
        <Col span={6}><Card className="bi-stat-card"><Statistic title="问题到货批次" value={kpi.badArrivals} suffix="批" /></Card></Col>
      </Row>

      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={12}>
          <Card title={`问题供应商排行（${kpi.supplierCnt} 家有不合格）`} size="small">
            {bySupplier.length === 0 ? <Empty description="该时间段无不合格" /> :
              <ReactECharts option={supplierOption} style={{ height: 360 }} notMerge />}
          </Card>
        </Col>
        <Col span={12}>
          <Card title="不合格率月度趋势（近 12 月）" size="small">
            {trend.length === 0 ? <Empty description="暂无数据" /> :
              <ReactECharts option={trendOption} style={{ height: 360 }} notMerge />}
          </Card>
        </Col>
      </Row>

      <Card
        title="到货质检清单（按到货单）"
        extra={
          <Space>
            <span style={{ color: '#888', fontSize: 13 }}>只看有问题的到货</span>
            <Switch checked={onlyBad} onChange={(v) => { setOnlyBad(v); setPage(1); }} />
          </Space>
        }
      >
        {shownArrivals.length === 0 ? <Empty description={onlyBad ? '该时间段无不合格到货 🎉' : '该时间段无到货检验'} /> :
          <Table
            dataSource={shownArrivals}
            columns={arrivalColumns}
            rowKey="arrival"
            size="small"
            pagination={{ current: page, pageSize: 20, total: shownArrivals.length, showSizeChanger: false, onChange: (p) => setPage(p) }}
            scroll={{ x: 1200 }}
          />}
      </Card>

      {/* 到货单详情: 该批货下每张检验单 */}
      <Modal
        title={arrivalRow ? `到货单 ${arrivalRow.arrival} · ${arrivalRow.supplier}` : '到货单详情'}
        open={arrivalOpen}
        onCancel={() => setArrivalOpen(false)}
        footer={null}
        width={1000}
      >
        {arrivalRow && (
          <div style={{ marginBottom: 12, color: '#64748b', fontSize: 13 }}>
            采购单 {arrivalRow.purchaseOrder || '—'} · {arrivalRow.billType} · 共 {arrivalRow.total} 张检验单（合格 {arrivalRow.pass} / 不合格 {arrivalRow.bad}）
          </div>
        )}
        {arrivalRow && (insLoading[arrivalRow.arrival] || !insCache[arrivalRow.arrival])
          ? <div style={{ textAlign: 'center', padding: 40 }}>加载中...</div>
          : <Table dataSource={arrivalRow ? insCache[arrivalRow.arrival] : []} columns={insColumns} rowKey="id" size="small" pagination={false} scroll={{ x: 900 }} />}
      </Modal>

      {/* 单张检验单详情 */}
      <Modal
        title={`检验单详情 · ${detailRow?.code || ''}`}
        open={detailOpen}
        onCancel={() => setDetailOpen(false)}
        footer={null}
        width={760}
      >
        {detailLoading ? <div style={{ textAlign: 'center', padding: 40 }}>加载中...</div> : (
          <Descriptions bordered size="small" column={2}>
            <Descriptions.Item label="检验单号">{detailRow?.code}</Descriptions.Item>
            <Descriptions.Item label="检验日期">{detailRow?.inspectDate}</Descriptions.Item>
            <Descriptions.Item label="判定">{detailRow?.result === '2' ? <Tag color="red">不合格</Tag> : <Tag color="green">合格</Tag>}</Descriptions.Item>
            <Descriptions.Item label="处理方式">{detailRow?.handleType || '—'}</Descriptions.Item>
            <Descriptions.Item label="物料" span={2}>{detailRow?.materialName}（{detailRow?.materialCode}）</Descriptions.Item>
            <Descriptions.Item label="批次">{detailRow?.batch}</Descriptions.Item>
            <Descriptions.Item label="库存状态">{detailRow?.stockStatus || '—'}</Descriptions.Item>
            <Descriptions.Item label="检验员">{ds('pk_inspecter_name')}</Descriptions.Item>
            <Descriptions.Item label="检验部门">{ds('pk_inspectdept_name')}</Descriptions.Item>
            <Descriptions.Item label="检验方案">{ds('pk_inspectionplan_name')}</Descriptions.Item>
            <Descriptions.Item label="检验数">{detailRow?.inspectNum?.toFixed(0)}</Descriptions.Item>
            <Descriptions.Item label="不合格数">{detailRow?.badNum?.toFixed(0)}</Descriptions.Item>
            <Descriptions.Item label="合格率">{detailRow?.qRate?.toFixed(1)}%</Descriptions.Item>
            <Descriptions.Item label="生产日期">{ds('producedate') !== '—' ? ds('producedate') : ds('manufacture_date')}</Descriptions.Item>
            <Descriptions.Item label="有效期至">{ds('validityDate')}</Descriptions.Item>
            <Descriptions.Item label="采购单号">{ds('sourceOrderCode')}</Descriptions.Item>
            <Descriptions.Item label="到货单号">{ds('vsourcecode')}</Descriptions.Item>
          </Descriptions>
        )}
      </Modal>
    </div>
  );
};

export default QCAlert;
