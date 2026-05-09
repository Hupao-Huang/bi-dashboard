import React, { useCallback, useEffect, useState } from 'react';
import { Card, Table, Tag, Modal, Statistic, Row, Col, Typography, message, Spin } from 'antd';
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

  const openDrill = async (row: HVRow) => {
    setDrillTarget(row);
    setDrillLoading(true);
    setDrillData([]);
    try {
      const params = new URLSearchParams({
        customerCode: row.customerCode,
        startMonth: '2025-01',
        endMonth: today.format('YYYY-MM'),
      });
      const res = await fetch(`${API_BASE}/api/distribution/customer-analysis/monthly?${params}`, { credentials: 'include' });
      const json = await res.json();
      const payload = json.data || json;
      setDrillData(payload.months || []);
    } catch {
      message.error('加载月度数据失败');
    } finally {
      setDrillLoading(false);
    }
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
    legend: { data: ['销售额', '订单数'] },
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
      legend: { data: years },
      xAxis: { type: 'category', data: ['1月', '2月', '3月', '4月', '5月', '6月', '7月', '8月', '9月', '10月', '11月', '12月'] },
      yAxis: { type: 'value', name: '销售额(元)' },
      series: years.map(y => ({ name: y, type: 'line', data: byYear[y], smooth: true })),
    };
  };

  const columns: ColumnsType<HVRow> = [
    {
      title: '排名', width: 70, align: 'center',
      render: (_: any, __: any, idx: number) => <span style={{ fontWeight: 600 }}>{(page - 1) * pageSize + idx + 1}</span>,
    },
    {
      title: '等级', dataIndex: 'grade', width: 80, align: 'center',
      render: (v: string) => <Tag color={gradeColor(v)} style={{ fontWeight: 600 }}>{v}</Tag>,
    },
    { title: '客户名称', dataIndex: 'customerName', ellipsis: true },
    { title: '客户编码', dataIndex: 'customerCode', width: 160, render: v => <span style={{ fontFamily: 'monospace', fontSize: 12, color: '#94a3b8' }}>{v}</span> },
    {
      title: '期间销售额', dataIndex: 'amount', width: 140, align: 'right',
      render: (v: number) => <span style={{ fontWeight: 600 }}>¥{(v || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</span>,
    },
    { title: '订单数', dataIndex: 'orders', width: 100, align: 'right' },
    {
      title: '操作', width: 100, fixed: 'right' as const,
      render: (_: any, r: HVRow) => <a onClick={() => openDrill(r)}>查看趋势</a>,
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
          scroll={{ x: 900 }}
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
          <div style={{ marginBottom: 12, color: '#64748b', fontSize: 13 }}>
            等级 <Tag color={gradeColor(drillTarget.grade)}>{drillTarget.grade}</Tag>
            客户编码 <span style={{ fontFamily: 'monospace' }}>{drillTarget.customerCode}</span>
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
    </div>
  );
};

export default CustomerAnalysis;
