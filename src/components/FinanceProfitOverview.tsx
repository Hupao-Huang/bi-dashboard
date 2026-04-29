import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Row, Col, Card, Table, Statistic } from 'antd';
import ReactECharts from './Chart';
import DateFilter from './DateFilter';
import PageLoading from './PageLoading';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../config';
import { CHART_COLORS, DEPT_COLORS, barItemStyle, formatMoney, getBaseOption, pieStyle } from '../chartTheme';

const deptConfig: Record<string, { label: string; color: string }> = {
  ecommerce: { label: '电商部门', color: DEPT_COLORS.ecommerce },
  social: { label: '社媒部门', color: DEPT_COLORS.social },
  offline: { label: '线下部门', color: DEPT_COLORS.offline },
  distribution: { label: '分销部门', color: DEPT_COLORS.distribution },
};

const profitRateColor = (rate: number) => {
  if (rate >= 0.5) return '#059669';
  if (rate >= 0.3) return '#f59e0b';
  return '#dc2626';
};

const OverviewTabContent: React.FC = () => {
  const abortRef = useRef<AbortController | null>(null);
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);

  const fetchData = useCallback((s: string, e: string) => {
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    fetch(`${API_BASE}/api/overview?start=${s}&end=${e}`, { signal: ctrl.signal })
      .then(res => res.json())
      .then(res => { setData(res.data); setLoading(false); })
      .catch((e: any) => { if (e?.name !== 'AbortError') setLoading(false); });
  }, []);

  useEffect(() => { fetchData(startDate, endDate); }, [fetchData, startDate, endDate]);

  const handleDateChange = (s: string, e: string) => {
    setStartDate(s);
    setEndDate(e);
  };

  if (loading) return <PageLoading />;
  if (!data) return <div>加载失败</div>;

  const depts = data.departments || [];

  // 产品定位 × 部门 矩阵 (从 /api/overview 的 gradeDeptSales)
  const gradeOrder = ['S', 'A', 'B', 'C', 'D', '未设置'];
  const deptLabel: Record<string, string> = {
    ecommerce: '电商', social: '社媒', offline: '线下', distribution: '分销', '其他': '其他',
  };
  const fmtAmount = (v: number) => v >= 100000000
    ? (v / 100000000).toFixed(2) + '亿'
    : v >= 10000 ? (v / 10000).toFixed(1) + '万' : (v || 0).toFixed(0);
  type Cell = { sales: number; profit: number };
  const gdsAll: any[] = data.gradeDeptSales || [];
  const gdMatrix: Record<string, Record<string, Cell>> = {};
  gdsAll.forEach((it: any) => {
    const g = it.grade || '未设置';
    const d = deptLabel[it.department] ?? (it.department || '其他');
    if (!gdMatrix[g]) gdMatrix[g] = {};
    if (!gdMatrix[g][d]) gdMatrix[g][d] = { sales: 0, profit: 0 };
    gdMatrix[g][d].sales += (it.sales || 0);
    gdMatrix[g][d].profit += (it.profit || 0);
  });
  const matrixDeptOrder: string[] = ['电商', '社媒', '线下', '分销', '其他']
    .filter(d => gradeOrder.some(g => (gdMatrix[g]?.[d]?.sales || 0) !== 0 || (gdMatrix[g]?.[d]?.profit || 0) !== 0));
  const matrixColTotals: Record<string, Cell> = { __total__: { sales: 0, profit: 0 } };
  matrixDeptOrder.forEach(d => { matrixColTotals[d] = { sales: 0, profit: 0 }; });
  gradeOrder.forEach(g => {
    matrixDeptOrder.forEach(d => {
      const c = gdMatrix[g]?.[d];
      if (c) {
        matrixColTotals[d].sales += c.sales;
        matrixColTotals[d].profit += c.profit;
        matrixColTotals.__total__.sales += c.sales;
        matrixColTotals.__total__.profit += c.profit;
      }
    });
  });
  const matrixMax = Math.max(0, ...gradeOrder.flatMap(g => matrixDeptOrder.map(d => gdMatrix[g]?.[d]?.sales || 0)));
  const heatBg = (v: number) => {
    if (matrixMax <= 0 || !v) return 'transparent';
    const ratio = Math.min(1, Math.max(0, v / matrixMax));
    const alpha = (0.05 + ratio * 0.45).toFixed(2);
    return `rgba(30,64,175,${alpha})`;
  };
  const matrixRows = gradeOrder
    .filter(g => matrixDeptOrder.some(d => (gdMatrix[g]?.[d]?.sales || 0) !== 0 || (gdMatrix[g]?.[d]?.profit || 0) !== 0))
    .map(g => {
      const row: any = { key: g, grade: g };
      let rs = 0, rp = 0;
      matrixDeptOrder.forEach(d => {
        const c = gdMatrix[g]?.[d] || { sales: 0, profit: 0 };
        row[d] = c;
        rs += c.sales;
        rp += c.profit;
      });
      row.__total__ = { sales: rs, profit: rp };
      return row;
    });
  const matrixTotalRow: any = { key: '__total__', grade: '合计' };
  matrixDeptOrder.forEach(d => { matrixTotalRow[d] = matrixColTotals[d]; });
  matrixTotalRow.__total__ = matrixColTotals.__total__;
  const cellRender = (c: Cell) => {
    if (!c || (!c.sales && !c.profit)) return <span style={{ color: '#cbd5e1' }}>—</span>;
    const rate = c.sales > 0 ? (c.profit / c.sales) : 0;
    return (
      <div style={{ textAlign: 'right' as const, fontVariantNumeric: 'tabular-nums' as const, lineHeight: 1.4 }}>
        <div style={{ fontWeight: 600 }}>¥{fmtAmount(c.sales)}</div>
        <div style={{ fontSize: 11, color: '#64748b' }}>毛利 ¥{fmtAmount(c.profit)} <span style={{ color: profitRateColor(rate), fontWeight: 600 }}>({(rate * 100).toFixed(1)}%)</span></div>
      </div>
    );
  };
  const totalCellRender = (c: Cell) => {
    const rate = c.sales > 0 ? (c.profit / c.sales) : 0;
    return (
      <div style={{ textAlign: 'right' as const, fontVariantNumeric: 'tabular-nums' as const, lineHeight: 1.4 }}>
        <div style={{ fontWeight: 700 }}>¥{fmtAmount(c.sales)}</div>
        <div style={{ fontSize: 11, color: '#475569' }}>毛利 ¥{fmtAmount(c.profit)} <span style={{ color: profitRateColor(rate), fontWeight: 600 }}>({(rate * 100).toFixed(1)}%)</span></div>
      </div>
    );
  };
  const matrixColumns = [
    {
      title: '产品定位', dataIndex: 'grade', key: 'grade', width: 100, fixed: 'left' as const,
      align: 'center' as const,
      render: (v: string) => v === '合计'
        ? <span style={{ fontWeight: 700, color: '#0f172a' }}>{v}</span>
        : <span style={{ fontWeight: 600, color: '#0f172a' }}>{v}</span>,
    },
    ...matrixDeptOrder.map(d => ({
      title: d, dataIndex: d, key: d, align: 'right' as const,
      onCell: (row: any) => row.key !== '__total__' ? { style: { background: heatBg(row[d]?.sales || 0) } } : { style: { background: '#f8fafc' } },
      render: (c: Cell, row: any) => row.key === '__total__' ? totalCellRender(c) : cellRender(c),
    })),
    {
      title: '合计', dataIndex: '__total__', key: '__total__', align: 'right' as const,
      onCell: () => ({ style: { background: '#f1f5f9' } }),
      render: (c: Cell) => totalCellRender(c),
    },
  ];

  const totalSales = depts.reduce((s: number, d: any) => s + (d.sales || 0), 0);
  const totalCost = depts.reduce((s: number, d: any) => s + (d.cost || 0), 0);
  const totalProfit = depts.reduce((s: number, d: any) => s + (d.profit || 0), 0);
  const overallProfitRate = totalSales > 0 ? totalProfit / totalSales : 0;
  const statCards = [
    { title: '总销售额', value: totalSales, precision: 2, prefix: '¥', accentColor: CHART_COLORS[0] },
    { title: '总成本', value: totalCost, precision: 2, prefix: '¥', accentColor: CHART_COLORS[1] },
    { title: '总毛利', value: totalProfit, precision: 2, prefix: '¥', accentColor: '#059669' },
    { title: '综合毛利率', value: overallProfitRate * 100, precision: 1, suffix: '%', accentColor: profitRateColor(overallProfitRate) },
  ];

  // Bar chart: sales vs profit vs cost per dept
  const deptNames = depts.map((d: any) => deptConfig[d.department]?.label || d.department);
  const base = getBaseOption();
  const moneyFmt = (v: number) => formatMoney(v);
  const barOption = {
    ...base,
    legend: { ...base.legend, data: ['销售额', '毛利', '成本'], top: 0 },
    grid: { ...base.grid, left: 64, right: 32, top: 48, bottom: 32 },
    xAxis: { ...base.xAxis, type: 'category' as const, data: deptNames },
    yAxis: { ...base.yAxis, type: 'value' as const, axisLabel: { ...base.yAxis.axisLabel, formatter: moneyFmt } },
    series: [
      {
        name: '销售额',
        type: 'bar',
        data: depts.map((d: any) => d.sales || 0),
        itemStyle: barItemStyle(CHART_COLORS[0]),
        barWidth: 20,
        label: { show: true, position: 'top' as const, formatter: (p: any) => moneyFmt(p.value), color: '#475569', fontSize: 11 },
      },
      {
        name: '毛利',
        type: 'bar',
        data: depts.map((d: any) => d.profit || 0),
        itemStyle: barItemStyle('#059669'),
        barWidth: 20,
        label: { show: true, position: 'top' as const, formatter: (p: any) => moneyFmt(p.value), color: '#475569', fontSize: 11 },
      },
      {
        name: '成本',
        type: 'bar',
        data: depts.map((d: any) => d.cost || 0),
        itemStyle: barItemStyle(CHART_COLORS[1]),
        barWidth: 20,
        label: { show: true, position: 'top' as const, formatter: (p: any) => moneyFmt(p.value), color: '#475569', fontSize: 11 },
      },
    ],
  };

  // Pie chart: profit share per dept
  const pieData = depts.map((d: any) => ({
    value: d.profit || 0,
    name: deptConfig[d.department]?.label || d.department,
    itemStyle: { color: deptConfig[d.department]?.color },
  }));
  const pieOption = {
    ...pieStyle,
    color: CHART_COLORS,
    legend: { ...pieStyle.legend, type: 'scroll' as const },
    series: [{
      type: 'pie',
      radius: ['40%', '65%'],
      center: ['50%', '45%'],
      avoidLabelOverlap: true,
      minShowLabelAngle: 8,
      label: {
        show: true,
        formatter: '{b}\n{d}%',
        fontSize: 12,
        lineHeight: 16,
        color: '#475569',
        overflow: 'truncate' as const,
        width: 80,
      },
      labelLine: { length: 12, length2: 10 },
      data: pieData,
    }],
  };

  // Table columns
  const columns = [
    { title: '部门', dataIndex: 'department', key: 'department', width: 120, render: (v: string) => deptConfig[v]?.label || v },
    { title: '销售额', dataIndex: 'sales', key: 'sales', width: 130, render: (v: number) => `¥${v?.toLocaleString()}` },
    { title: '成本', dataIndex: 'cost', key: 'cost', width: 130, render: (v: number) => `¥${(v || 0).toLocaleString()}` },
    { title: '毛利', dataIndex: 'profit', key: 'profit', width: 130, render: (v: number) => `¥${(v || 0).toLocaleString()}` },
    {
      title: '毛利率',
      key: 'profit_rate',
      width: 100,
      render: (_: any, row: any) => {
        const rate = row.sales > 0 ? (row.profit || 0) / row.sales : 0;
        return <span style={{ color: profitRateColor(rate), fontWeight: 600 }}>{(rate * 100).toFixed(1)}%</span>;
      },
    },
    { title: '货品数', dataIndex: 'qty', key: 'qty', width: 90, render: (v: number) => v?.toLocaleString() },
  ];

  return (
    <div>
      <DateFilter start={startDate} end={endDate} onChange={handleDateChange} />

      {/* Summary cards */}
      <Row gutter={[16, 16]}>
        {statCards.map((card) => (
          <Col xs={24} sm={6} key={card.title}>
            <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
              <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
            </Card>
          </Col>
        ))}
      </Row>

      {/* Charts */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={14}>
          <Card title="各部门销售额 vs 毛利 vs 成本">
            <ReactECharts lazyUpdate={true} option={barOption} style={{ height: 360 }} />
          </Card>
        </Col>
        <Col xs={24} lg={10}>
          <Card title="各部门毛利占比">
            <ReactECharts lazyUpdate={true} option={pieOption} style={{ height: 360 }} />
          </Card>
        </Col>
      </Row>

      {/* 产品定位 × 部门 矩阵 */}
      <Card className="bi-table-card" title="产品定位 × 部门 矩阵（含毛利）" style={{ marginTop: 16 }}>
        <Table
          dataSource={[...matrixRows, matrixTotalRow]}
          columns={matrixColumns}
          pagination={false}
          size="small"
          bordered
        />
      </Card>

      {/* Table */}
      <Card className="bi-table-card" title="部门利润汇总表" style={{ marginTop: 16 }}>
        <Table
          dataSource={depts}
          columns={columns}
          rowKey="department"
          pagination={false}
          size="small"
          summary={pageData => {
            const sumSales = pageData.reduce((s: number, row: any) => s + (row.sales || 0), 0);
            const sumCost = pageData.reduce((s: number, row: any) => s + (row.cost || 0), 0);
            const sumProfit = pageData.reduce((s: number, row: any) => s + (row.profit || 0), 0);
            const sumQty = pageData.reduce((s: number, row: any) => s + (row.qty || 0), 0);
            const rate = sumSales > 0 ? sumProfit / sumSales : 0;
            return (
              <Table.Summary.Row style={{ fontWeight: 'bold', background: '#fafafa' }}>
                <Table.Summary.Cell index={0}>合计</Table.Summary.Cell>
                <Table.Summary.Cell index={1}>¥{sumSales.toLocaleString()}</Table.Summary.Cell>
                <Table.Summary.Cell index={2}>¥{sumCost.toLocaleString()}</Table.Summary.Cell>
                <Table.Summary.Cell index={3}>¥{sumProfit.toLocaleString()}</Table.Summary.Cell>
                <Table.Summary.Cell index={4}><span style={{ color: profitRateColor(rate), fontWeight: 600 }}>{(rate * 100).toFixed(1)}%</span></Table.Summary.Cell>
                <Table.Summary.Cell index={5}>{sumQty.toLocaleString()}</Table.Summary.Cell>
              </Table.Summary.Row>
            );
          }}
        />
      </Card>
    </div>
  );
};

const FinanceProfitOverview: React.FC = () => <OverviewTabContent />;

export default FinanceProfitOverview;
