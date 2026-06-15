import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Row, Col, Card, Statistic, Table, DatePicker, Tag, Progress, Input } from 'antd';
import dayjs, { Dayjs } from 'dayjs';
import ReactECharts from '../../components/Chart';
import PageLoading from '../../components/PageLoading';
import { API_BASE } from '../../config';
import { GRADE_COLORS } from '../../chartTheme';

// 财务-SKU动销率 (2026-06-15)
// 口径: 仅产成品(打了产品定位等级 S/A/B/C/D 的货品); 有销售=当期销量>0 含调拨当销售; 货品级; 等级用当前
interface GradeAgg { grade: string; total: number; sold: number; rate: number; }
interface SkuRow { goodsNo: string; goodsName: string; grade: string; daysSold: number; days: number; rate: number; }
interface PageData {
  month: string;
  daysInPeriod: number;
  isPartial: boolean;
  periodStart?: string;
  periodEnd?: string;
  overall: { totalSku: number; soldSku: number; rate: number };
  byGrade: GradeAgg[];
  skus: SkuRow[];
}

const GRADE_ORDER = ['S', 'A', 'B', 'C', 'D'];

const SkuMovement: React.FC = () => {
  const abortRef = useRef<AbortController | null>(null);
  // 默认上一个完整月
  const [month, setMonth] = useState<Dayjs>(dayjs().subtract(1, 'month'));
  const [data, setData] = useState<PageData | null>(null);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');

  const fetchData = useCallback((m: Dayjs) => {
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    fetch(`${API_BASE}/api/finance/sku-movement?month=${m.format('YYYY-MM')}`, { credentials: 'include', signal: ctrl.signal })
      .then(res => res.json())
      .then(res => { setData(res.data); setLoading(false); })
      .catch((e: any) => { if (e?.name !== 'AbortError') setLoading(false); });
  }, []);

  useEffect(() => { fetchData(month); }, [fetchData, month]);

  if (loading) return <PageLoading />;
  if (!data) return <div>加载失败</div>;

  const { overall, byGrade, skus, daysInPeriod, isPartial } = data;
  const idleCount = overall.totalSku - overall.soldSku; // 滞销(全期零销售)数

  // 单品动销率分布 (滞销 / ≤3成 / 中 / ≥8成)
  const dist = skus.reduce(
    (acc, s) => {
      if (s.rate <= 0) acc.zero++;
      else if (s.rate < 0.3) acc.low++;
      else if (s.rate < 0.8) acc.mid++;
      else acc.high++;
      return acc;
    },
    { zero: 0, low: 0, mid: 0, high: 0 },
  );

  const statCards = [
    { title: '整体动销率', value: overall.rate * 100, precision: 1, suffix: '%', accent: '#1e40af' },
    { title: '产成品总数', value: overall.totalSku, suffix: '个', accent: '#7c3aed' },
    { title: '有销售', value: overall.soldSku, suffix: '个', accent: '#10b981' },
    { title: '滞销(全期0销售)', value: idleCount, suffix: '个', accent: '#ef4444' },
  ];

  // 品类动销率柱状图
  const gradeBarOption = {
    tooltip: {
      trigger: 'axis' as const,
      formatter: (p: any) => {
        const g = byGrade[p[0].dataIndex];
        return `${g.grade}品<br/>动销率: ${(g.rate * 100).toFixed(1)}%<br/>有销售: ${g.sold} / ${g.total}`;
      },
    },
    grid: { left: 48, right: 20, top: 24, bottom: 28 },
    xAxis: {
      type: 'category' as const,
      data: byGrade.map(g => g.grade + '品'),
      axisLabel: { color: '#64748b', fontSize: 12 },
      axisLine: { lineStyle: { color: '#e2e8f0' } },
    },
    yAxis: {
      type: 'value' as const,
      max: 100,
      axisLabel: { color: '#94a3b8', fontSize: 11, formatter: '{value}%' },
      splitLine: { lineStyle: { color: '#f1f5f9' } },
    },
    series: [{
      type: 'bar' as const,
      barWidth: 40,
      data: byGrade.map(g => ({
        value: +(g.rate * 100).toFixed(1),
        itemStyle: { color: GRADE_COLORS[g.grade] || '#1e40af', borderRadius: [4, 4, 0, 0] },
      })),
      label: { show: true, position: 'top' as const, formatter: '{c}%', fontSize: 11, color: '#475569' },
    }],
  };

  // 明细表 — 搜索手动过滤; 等级交给列内置 filter
  const filtered = skus.filter(s => {
    if (search) {
      const q = search.trim().toLowerCase();
      if (!s.goodsNo.toLowerCase().includes(q) && !(s.goodsName || '').toLowerCase().includes(q)) return false;
    }
    return true;
  });

  const columns = [
    { title: '货品编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 150, ellipsis: true },
    { title: '货品名称', dataIndex: 'goodsName', key: 'goodsName', ellipsis: true,
      render: (v: string) => v || <span style={{ color: 'var(--text-tertiary)' }}>-</span> },
    { title: '等级', dataIndex: 'grade', key: 'grade', width: 72, align: 'center' as const,
      filters: GRADE_ORDER.map(g => ({ text: g + '品', value: g })),
      onFilter: (val: any, r: SkuRow) => r.grade === val,
      render: (g: string) => <Tag color={GRADE_COLORS[g] || 'default'} style={{ marginInlineEnd: 0 }}>{g}品</Tag> },
    { title: '有销售天数', dataIndex: 'daysSold', key: 'daysSold', width: 110, align: 'right' as const,
      sorter: (a: SkuRow, b: SkuRow) => a.daysSold - b.daysSold,
      render: (v: number) => `${v} / ${daysInPeriod}` },
    { title: '单品动销率', dataIndex: 'rate', key: 'rate', width: 220,
      defaultSortOrder: 'ascend' as const,
      sorter: (a: SkuRow, b: SkuRow) => a.rate - b.rate,
      render: (v: number) => {
        const pct = +(v * 100).toFixed(1);
        const c = v <= 0 ? '#ef4444' : v < 0.3 ? '#f59e0b' : v < 0.8 ? '#3b82f6' : '#10b981';
        return (
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Progress percent={pct} strokeColor={c} trailColor="#f1f5f9" showInfo={false} style={{ flex: 1, marginBottom: 0 }} size="small" />
            <span style={{ color: c, fontWeight: 600, minWidth: 46, textAlign: 'right' }}>{pct}%</span>
          </div>
        );
      } },
  ];

  return (
    <div>
      {/* 月份选择 */}
      <div style={{ marginBottom: 16, display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
        <span style={{ color: 'var(--text-secondary)' }}>统计月份</span>
        <DatePicker
          picker="month"
          value={month}
          onChange={d => d && setMonth(d)}
          allowClear={false}
          disabledDate={d => d.isAfter(dayjs(), 'month')}
        />
        <span style={{ color: 'var(--text-tertiary)', fontSize: 13 }}>
          当期 {daysInPeriod} 天{isPartial ? '(本月进行中, 按已过天数算)' : ''}
        </span>
      </div>

      {/* 指标1: 整体 KPI */}
      <Row gutter={[16, 16]}>
        {statCards.map(card => (
          <Col xs={12} sm={6} key={card.title}>
            <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accent, height: '100%' }}>
              <Statistic title={card.title} value={card.value} precision={card.precision} suffix={card.suffix} />
            </Card>
          </Col>
        ))}
      </Row>

      {/* 指标2: 品类动销率 */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={12}>
          <Card title="品类动销率(按产品定位等级)">
            <ReactECharts option={gradeBarOption} style={{ height: 320 }} />
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card title="品类动销率明细" styles={{ body: { paddingTop: 8 } }}>
            <Table
              dataSource={byGrade}
              rowKey="grade"
              pagination={false}
              size="small"
              columns={[
                { title: '等级', dataIndex: 'grade', width: 90, render: (g: string) => <Tag color={GRADE_COLORS[g] || 'default'}>{g}品</Tag> },
                { title: '产成品数', dataIndex: 'total', align: 'right' as const },
                { title: '有销售', dataIndex: 'sold', align: 'right' as const },
                { title: '动销率', dataIndex: 'rate', align: 'right' as const, render: (v: number) => `${(v * 100).toFixed(1)}%` },
              ]}
            />
            <div style={{ marginTop: 12, fontSize: 13, color: 'var(--text-secondary)' }}>
              单品动销率分布:
              <Tag color="error" style={{ marginLeft: 8 }}>滞销 {dist.zero}</Tag>
              <Tag color="warning">≤3成 {dist.low}</Tag>
              <Tag color="processing">中 {dist.mid}</Tag>
              <Tag color="success">≥8成 {dist.high}</Tag>
            </div>
          </Card>
        </Col>
      </Row>

      {/* 指标3: 单品动销率明细 */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col span={24}>
          <Card
            className="bi-table-card"
            title={`单品动销率明细(共 ${skus.length} 个产成品)`}
            extra={
              <Input.Search
                placeholder="搜货品编码 / 名称"
                allowClear
                onChange={e => setSearch(e.target.value)}
                style={{ width: 220 }}
              />
            }
          >
            <Table
              dataSource={filtered}
              columns={columns}
              rowKey="goodsNo"
              size="small"
              pagination={{ pageSize: 50, showSizeChanger: true, showTotal: t => `共 ${t} 个` }}
              rowClassName={(r: SkuRow) => (r.rate <= 0 ? 'sku-idle-row' : '')}
              scroll={{ y: 520 }}
            />
          </Card>
        </Col>
      </Row>

      <style>{`
        .sku-idle-row td { background-color: #fef2f2 !important; }
        .sku-idle-row:hover td { background-color: #fee2e2 !important; }
      `}</style>
    </div>
  );
};

export default SkuMovement;
