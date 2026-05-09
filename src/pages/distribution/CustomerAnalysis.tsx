import React, { useCallback, useEffect, useState } from 'react';
import { Card, Table, Tag, Modal, Statistic, Row, Col, Typography, message, Spin, Space } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { CrownOutlined, ArrowUpOutlined, ArrowDownOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import DateFilter from '../../components/DateFilter';
import Chart from '../../components/Chart';
import { API_BASE } from '../../config';

const { Title } = Typography;

type HVRow = {
  customerCode: string;
  customerName: string;
  grade: string;
  amount: number;
  orders: number;
};

type KPIData = {
  hvCustomers: number;
  hvAmount: number;
  totalAmount: number;
  hvShare: number;
  momChange: number;
};

type MonthlyRow = { month: string; amount: number; orders: number };

const formatWan = (v: number) => `¥${(v / 10000).toFixed(2)}万`;
const gradeColor = (g: string) => (g === 'SA' || g === 'S' ? 'red' : g === 'A' ? 'orange' : 'default');

const CustomerAnalysis: React.FC = () => {
  const today = dayjs();
  const [start, setStart] = useState(today.startOf('month').format('YYYY-MM-DD'));
  const [end, setEnd] = useState(today.format('YYYY-MM-DD'));
  const [kpi, setKpi] = useState<KPIData | null>(null);
  const [list, setList] = useState<HVRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [total, setTotal] = useState(0);
  const [drillTarget, setDrillTarget] = useState<HVRow | null>(null);
  const [drillData, setDrillData] = useState<MonthlyRow[]>([]);
  const [drillLoading, setDrillLoading] = useState(false);
  const [skusTarget, setSkusTarget] = useState<HVRow | null>(null);
  const [skusData, setSkusData] = useState<any[]>([]);
  const [skusLoading, setSkusLoading] = useState(false);

  const fetchAll = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams({ startDate: start, endDate: end });
      const [kpiRes, listRes] = await Promise.all([
        fetch(`${API_BASE}/api/distribution/customer-analysis/kpi?${params}`, { credentials: 'include' }).then(r => r.json()),
        fetch(`${API_BASE}/api/distribution/customer-analysis/list?${params}&page=${page}&pageSize=${pageSize}`, { credentials: 'include' }).then(r => r.json()),
      ]);
      setKpi(kpiRes.data || kpiRes);
      const lp = listRes.data || listRes;
      setList(lp.list || []);
      setTotal(lp.total || 0);
    } catch (e) {
      message.error('加载失败');
    } finally {
      setLoading(false);
    }
  }, [start, end, page, pageSize]);

  useEffect(() => { fetchAll(); /* eslint-disable-next-line */ }, [start, end, page, pageSize]);

  const openDrill = (row: HVRow) => {
    // 销售趋势 Modal: 用 2025-01 ~ 今天 全期看趋势(月度+历年同月对比)
    setDrillTarget(row);
    setDrillLoading(true);
    setDrillData([]);
    const params = new URLSearchParams({
      customerCode: row.customerCode,
      startMonth: '2025-01',
      endMonth: today.format('YYYY-MM'),
    });
    fetch(`${API_BASE}/api/distribution/customer-analysis/monthly?${params}`, { credentials: 'include' })
      .then(r => r.json()).then(j => {
        const p = j.data || j;
        setDrillData(p.months || []);
      }).catch(() => message.error('加载月度数据失败'))
      .finally(() => setDrillLoading(false));
  };

  const openSkus = (row: HVRow) => {
    // 销售明细 Modal: 用主页面 DateFilter 的 startDate/endDate, 跟 KPI 一致
    setSkusTarget(row);
    setSkusLoading(true);
    setSkusData([]);
    const params = new URLSearchParams({
      customerCode: row.customerCode,
      startDate: start,
      endDate: end,
    });
    fetch(`${API_BASE}/api/distribution/customer-analysis/skus?${params}`, { credentials: 'include' })
      .then(r => r.json()).then(j => {
        const p = j.data || j;
        setSkusData(p.list || []);
      }).catch(() => message.error('加载销售明细失败'))
      .finally(() => setSkusLoading(false));
  };

  const monthlyOption = (rows: MonthlyRow[]) => ({
    tooltip: {
      trigger: 'axis',
      formatter: (p: any[]) => {
        const m = p[0]?.axisValue;
        const a = p.find(x => x.seriesName === '销售额')?.value || 0;
        const o = p.find(x => x.seriesName === '订单数')?.value || 0;
        return `${m}<br/>销售额: ¥${a.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}<br/>订单数: ${o}`;
      },
    },
    legend: { data: ['销售额', '订单数'], top: 0 },
    grid: { top: 50, bottom: 30, left: 60, right: 60 },
    xAxis: { type: 'category', data: rows.map(r => r.month) },
    yAxis: [
      { type: 'value', name: '销售额(元)' },
      { type: 'value', name: '订单数', alignTicks: true },
    ],
    series: [
      { name: '销售额', type: 'bar', data: rows.map(r => r.amount), yAxisIndex: 0 },
      { name: '订单数', type: 'line', data: rows.map(r => r.orders), yAxisIndex: 1, smooth: true },
    ],
  });

  // 历年同月对比（按 yyyy 拆分）
  const yearlyCompareOption = (rows: MonthlyRow[]) => {
    const byYear: Record<string, number[]> = {};
    rows.forEach(r => {
      const [y, m] = r.month.split('-');
      if (!byYear[y]) byYear[y] = Array(12).fill(0);
      byYear[y][parseInt(m, 10) - 1] = r.amount;
    });
    const years = Object.keys(byYear).sort();
    return {
      tooltip: { trigger: 'axis', valueFormatter: (v: number) => `¥${(v || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2 })}` },
      legend: { data: years, top: 0 },
      grid: { top: 50, bottom: 30, left: 60, right: 30 },
      xAxis: { type: 'category', data: ['1月', '2月', '3月', '4月', '5月', '6月', '7月', '8月', '9月', '10月', '11月', '12月'] },
      yAxis: { type: 'value', name: '销售额(元)' },
      series: years.map(y => ({ name: y, type: 'line', data: byYear[y], smooth: true })),
    };
  };

  const columns: ColumnsType<HVRow> = [
    {
      title: '排名', width: 60, align: 'center',
      render: (_: any, __: any, idx: number) => (page - 1) * pageSize + idx + 1,
    },
    {
      title: '等级', dataIndex: 'grade', width: 60, align: 'center',
      render: (v: string) => <Tag color={gradeColor(v)}>{v}</Tag>,
    },
    { title: '客户名称', dataIndex: 'customerName', width: 240, ellipsis: true },
    { title: '客户编码', dataIndex: 'customerCode', width: 130 },
    {
      title: '期间销售额', dataIndex: 'amount', width: 120, align: 'right',
      render: (v: number) => `¥${(v || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2 })}`,
    },
    { title: '订单数', dataIndex: 'orders', width: 80, align: 'right' },
    {
      title: '操作', width: 130,
      render: (_: any, r: HVRow) => (
        <Space size="small">
          <a onClick={() => openDrill(r)}>查看趋势</a>
          <a onClick={() => openSkus(r)}>查看明细</a>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <Title level={4} style={{ marginBottom: 16 }}>
        <CrownOutlined /> 分销·客户分析
      </Title>

      <Card className="bi-filter-card" style={{ marginBottom: 16 }}>
        <DateFilter start={start} end={end} onChange={(s, e) => { setStart(s); setEnd(e); setPage(1); }} />
      </Card>

      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col span={6}>
          <Card className="bi-stat-card">
            <Statistic title="高价值客户数" value={kpi?.hvCustomers || 0} suffix="个" />
            <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 4 }}>S+A 级 全量</div>
          </Card>
        </Col>
        <Col span={6}>
          <Card className="bi-stat-card">
            <Statistic title="高价值贡献销售额" value={kpi?.hvAmount || 0} precision={2} prefix="¥" />
            <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 4 }}>≈ {formatWan(kpi?.hvAmount || 0)}</div>
          </Card>
        </Col>
        <Col span={6}>
          <Card className="bi-stat-card">
            <Statistic title="占分销总销售额" value={kpi?.hvShare || 0} precision={2} suffix="%" />
            <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 4 }}>分销总额 {formatWan(kpi?.totalAmount || 0)}</div>
          </Card>
        </Col>
        <Col span={6}>
          <Card className="bi-stat-card">
            <Statistic
              title="高价值销售月环比"
              value={Math.abs(kpi?.momChange || 0)}
              precision={2}
              suffix="%"
              prefix={(kpi?.momChange || 0) >= 0 ? <ArrowUpOutlined /> : <ArrowDownOutlined />}
              valueStyle={{ color: (kpi?.momChange || 0) >= 0 ? '#3f8600' : '#cf1322' }}
            />
            <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 4 }}>对比上一同等长度时段</div>
          </Card>
        </Col>
      </Row>

      <Card title="高价值客户排名 (期间销售额)">
        <Table
          rowKey="customerCode"
          columns={columns}
          dataSource={list}
          loading={loading}
          size="small"
          scroll={{ x: 'max-content' }}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: t => `共 ${t} 个高价值客户`,
            onChange: (p, ps) => { setPage(p); setPageSize(ps); },
          }}
        />
      </Card>

      <Modal
        title={drillTarget ? `${drillTarget.customerName} - 销售趋势` : ''}
        open={!!drillTarget}
        onCancel={() => setDrillTarget(null)}
        footer={null}
        width={960}
      >
        {drillTarget && (
          <div style={{ marginBottom: 12 }}>
            等级 <Tag color={gradeColor(drillTarget.grade)}>{drillTarget.grade}</Tag>
            客户编码 {drillTarget.customerCode}
          </div>
        )}
        <Spin spinning={drillLoading}>
          <Card title="月度销售时序" size="small" style={{ marginBottom: 12 }}>
            <Chart option={monthlyOption(drillData)} style={{ height: 320 }} />
          </Card>
          <Card title="历年同月对比" size="small">
            <Chart option={yearlyCompareOption(drillData)} style={{ height: 320 }} />
          </Card>
        </Spin>
      </Modal>

      <Modal
        title={skusTarget ? `${skusTarget.customerName} - 销售明细 (按 SKU)` : ''}
        open={!!skusTarget}
        onCancel={() => setSkusTarget(null)}
        footer={null}
        width={960}
      >
        {skusTarget && (
          <div style={{ marginBottom: 12 }}>
            等级 <Tag color={gradeColor(skusTarget.grade)}>{skusTarget.grade}</Tag>
            客户编码 {skusTarget.customerCode}　时间 {start} ~ {end}
          </div>
        )}
        <Spin spinning={skusLoading}>
          <Table
            rowKey={(r: any) => r.goodsNo + '|' + r.goodsName}
            dataSource={skusData}
            size="small"
            pagination={{ pageSize: 20, showTotal: t => `共 ${t} 个 SKU` }}
            expandable={{
              rowExpandable: (r: any) => r.isPackage === 1 && (r.packageChildren?.length || 0) > 0,
              expandedRowRender: (r: any) => (
                <Table
                  rowKey={(c: any) => c.childGoodsNo + '|' + c.childSpecName}
                  dataSource={r.packageChildren}
                  size="small"
                  pagination={false}
                  columns={[
                    { title: '子件编码', dataIndex: 'childGoodsNo', width: 130 },
                    { title: '子件名称', dataIndex: 'childGoodsName', ellipsis: true },
                    { title: '规格', dataIndex: 'childSpecName', width: 120 },
                    { title: '数量', dataIndex: 'goodsAmount', width: 80, align: 'right',
                      render: (v: number) => (v || 0).toLocaleString('zh-CN') },
                    { title: '单位', dataIndex: 'unitName', width: 70 },
                    { title: '分摊金额', dataIndex: 'shareAmount', width: 100, align: 'right',
                      render: (v: number) => `¥${(v || 0).toFixed(2)}` },
                  ]}
                />
              ),
            }}
            columns={[
              { title: '排名', width: 60, align: 'center', render: (_: any, __: any, idx: number) => idx + 1 },
              {
                title: '类型', dataIndex: 'isPackage', width: 70, align: 'center',
                render: (v: number) => v === 1 ? <Tag color="blue">组合</Tag> : <Tag>单品</Tag>,
              },
              { title: '商品编码', dataIndex: 'goodsNo', width: 130 },
              { title: '商品名称', dataIndex: 'goodsName', ellipsis: true },
              {
                title: '销售额', dataIndex: 'amount', width: 120, align: 'right',
                render: (v: number) => `¥${(v || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2 })}`,
              },
              {
                title: '件数', dataIndex: 'qty', width: 90, align: 'right',
                render: (v: number) => (v || 0).toLocaleString('zh-CN'),
              },
              { title: '订单数', dataIndex: 'orderCount', width: 80, align: 'right' },
            ]}
          />
        </Spin>
      </Modal>
    </div>
  );
};

export default CustomerAnalysis;
