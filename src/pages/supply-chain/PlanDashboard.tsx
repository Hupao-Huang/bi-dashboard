import React, { useEffect, useState, useCallback } from 'react';
import { Row, Col, Card, Table } from 'antd';
import {
  DollarOutlined,
  DatabaseOutlined,
  SyncOutlined,
  WarningOutlined,
  StopOutlined,
} from '@ant-design/icons';
import ReactECharts from '../../components/Chart';
import DateFilter from '../../components/DateFilter';
import AnimatedNumber from '../../components/AnimatedNumber';
import PageLoading from '../../components/PageLoading';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../../config';
import { getBaseOption, barItemStyle, formatMoney, CHART_COLORS } from '../../chartTheme';

const PlanDashboard: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<any>(null);
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);
  const [warehouse] = useState('');

  const fetchData = useCallback((s: string, e: string, wh: string) => {
    setLoading(true);
    const params = new URLSearchParams({ start: s, end: e });
    if (wh) params.set('warehouse', wh);
    fetch(`${API_BASE}/api/supply-chain/dashboard?${params}`)
      .then(res => res.json())
      .then(res => { if (res.code === 200) setData(res.data); setLoading(false); })
      .catch(() => setLoading(false));
  }, []);

  useEffect(() => { fetchData(startDate, endDate, warehouse); }, [fetchData, startDate, endDate, warehouse]);

  if (loading && !data) return <PageLoading />;
  if (!data) return <div>加载失败</div>;

  const kpi = data.kpi || {};

  // ========== KPI 卡片 ==========
  const fmtYuan = (v: number) => `¥${v.toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
  const fmtWan = (v: number) => `¥${(v / 10000).toFixed(1)}万`;
  const fmtDay = (v: number) => `${v.toFixed(1)}天`;
  const fmtPct = (v: number) => `${v.toFixed(1)}%`;

  const kpiCards = [
    { title: '销售GMV', num: kpi.salesGMV || 0, fmt: fmtYuan, color: '#4f46e5', icon: <DollarOutlined />, desc: '销售出库销售额', animated: true },
    { title: '库存成本', num: kpi.stockCost || 0, fmt: fmtYuan, color: '#06b6d4', icon: <DatabaseOutlined />, desc: '当前库存金额', tag: '实时' },
    { title: '库存周转', num: kpi.turnoverDays || 0, fmt: fmtDay, color: '#f59e0b', icon: <SyncOutlined />, desc: '库存成本÷日均销售额', animated: true },
    { title: '高库存占比', num: kpi.highStockRate || 0, fmt: fmtPct, color: '#8b5cf6', icon: <WarningOutlined />, desc: '周转>50天的库存占比', tag: '实时' },
    { title: '缺货率', num: kpi.stockoutRate || 0, fmt: fmtPct, color: '#ef4444', icon: <StopOutlined />, desc: `${kpi.stockoutSKU || 0}/${kpi.salesSKU || 0} SKU`, tag: '实时' },
    { title: '库龄>90天', num: kpi.agedStockValue || 0, fmt: fmtWan, color: '#f97316', icon: <WarningOutlined />, desc: '生产日期超90天的库存金额', tag: '实时' },
  ];

  // ========== 月度销售趋势 ==========
  const baseOpt = getBaseOption();
  const monthly = data.monthlySales || [];
  const salesTrendOption = {
    ...baseOpt,
    grid: { left: 60, right: 20, top: 24, bottom: 24 },
    xAxis: { ...baseOpt.xAxis, type: 'category' as const, data: monthly.map((d: any) => d.month) },
    yAxis: { ...baseOpt.yAxis, type: 'value' as const, axisLabel: { ...baseOpt.yAxis.axisLabel, formatter: formatMoney } },
    series: [{
      type: 'bar', data: monthly.map((d: any) => d.value), ...barItemStyle('#4f46e5'), barWidth: 20,
      label: { show: true, position: 'top', fontSize: 10, color: '#64748b', formatter: (p: any) => formatMoney(p.value) },
    }],
  };

  // ========== 各渠道销售额 ==========
  const deptNames: Record<string, string> = { ecommerce: '电商', social: '社媒', offline: '线下', distribution: '分销', other: '其他' };
  const rateRender = (v: number) => {
    if (!v) return '-';
    const color = v > 0 ? '#10b981' : '#ef4444';
    return <span style={{ color, fontWeight: 500 }}>{v > 0 ? '+' : ''}{v.toFixed(1)}%</span>;
  };
  const channelCols = [
    { title: '渠道', dataIndex: 'channel', key: 'channel', width: 70, render: (v: string) => deptNames[v] || v },
    { title: '日均销售额', dataIndex: 'dailyAvg', key: 'dailyAvg', width: 110, align: 'right' as const, render: (v: number) => `¥${v?.toLocaleString()}` },
    { title: '累计销售额', dataIndex: 'total', key: 'total', width: 120, align: 'right' as const, render: (v: number) => <span style={{ fontWeight: 600 }}>¥{v?.toLocaleString()}</span> },
    { title: '上月同期', dataIndex: 'lastMonth', key: 'lastMonth', width: 110, align: 'right' as const, render: (v: number) => v > 0 ? `¥${v?.toLocaleString()}` : '-' },
    { title: '环比', dataIndex: 'momRate', key: 'momRate', width: 70, align: 'right' as const, render: rateRender },
    { title: '同比', dataIndex: 'yoyRate', key: 'yoyRate', width: 70, align: 'right' as const, render: rateRender },
  ];

  // ========== 品类库存健康度 ==========
  const cateCols = [
    { title: '产品分类', dataIndex: 'category', key: 'category', width: 100 },
    { title: '库存金额', dataIndex: 'stockValue', key: 'stockValue', width: 120, align: 'right' as const, render: (v: number) => `¥${v?.toLocaleString(undefined, { maximumFractionDigits: 0 })}` },
    { title: '日均销售成本', dataIndex: 'dailySalesCost', key: 'dailySalesCost', width: 120, align: 'right' as const, render: (v: number) => `¥${v?.toLocaleString(undefined, { maximumFractionDigits: 0 })}` },
    {
      title: '库存周转(天)', dataIndex: 'turnover', key: 'turnover', width: 110, align: 'right' as const,
      sorter: (a: any, b: any) => a.turnover - b.turnover,
      render: (v: number) => {
        const color = v > 50 ? '#ef4444' : v > 30 ? '#f59e0b' : '#10b981';
        return <span style={{ fontWeight: 600, color }}>{v?.toFixed(1)}</span>;
      },
    },
    {
      title: '高库存占比', dataIndex: 'highStockRate', key: 'highStockRate', width: 100, align: 'right' as const,
      render: (v: number) => {
        const color = v > 30 ? '#ef4444' : v > 15 ? '#f59e0b' : '#10b981';
        return <span style={{ color }}>{v?.toFixed(1)}%</span>;
      },
    },
    {
      title: '缺货率', dataIndex: 'stockoutRate', key: 'stockoutRate', width: 80, align: 'right' as const,
      render: (v: number) => {
        const color = v > 10 ? '#ef4444' : v > 5 ? '#f59e0b' : '#10b981';
        return <span style={{ color }}>{v?.toFixed(1)}%</span>;
      },
    },
  ];

  // ========== 高库存产品明细 ==========
  const highStockCols = [
    { title: '商品编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 110 },
    { title: '商品名称', dataIndex: 'goodsName', key: 'goodsName', width: 200, ellipsis: true },
    { title: '仓库', dataIndex: 'warehouse', key: 'warehouse', width: 160, ellipsis: true },
    { title: '可用库存', dataIndex: 'usableQty', key: 'usableQty', width: 90, align: 'right' as const, render: (v: number) => v?.toLocaleString() },
    { title: '日均销量', dataIndex: 'dailySales', key: 'dailySales', width: 80, align: 'right' as const },
    {
      title: '周转(天)', dataIndex: 'turnover', key: 'turnover', width: 90, align: 'right' as const,
      sorter: (a: any, b: any) => a.turnover - b.turnover,
      render: (v: number) => <span style={{ color: '#ef4444', fontWeight: 600 }}>{v}</span>,
    },
    { title: '库存金额', dataIndex: 'stockValue', key: 'stockValue', width: 110, align: 'right' as const, render: (v: number) => `¥${v?.toLocaleString()}` },
  ];

  // ========== 缺货产品明细 ==========
  const stockoutCols = [
    { title: '商品编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 110 },
    { title: '商品名称', dataIndex: 'goodsName', key: 'goodsName', width: 200, ellipsis: true },
    { title: '仓库', dataIndex: 'warehouse', key: 'warehouse', width: 160, ellipsis: true },
    { title: '日均销量', dataIndex: 'dailySales', key: 'dailySales', width: 80, align: 'right' as const },
    { title: '日均损失', dataIndex: 'dailyValue', key: 'dailyValue', width: 100, align: 'right' as const, render: (v: number) => <span style={{ color: '#ef4444' }}>¥{v?.toLocaleString()}</span> },
  ];

  // ========== 销售TOP20 ==========
  const topCols = [
    { title: '#', key: 'rank', width: 40, render: (_: any, __: any, i: number) => <span style={{ color: i < 3 ? '#4f46e5' : '#94a3b8', fontWeight: i < 3 ? 700 : 400 }}>{i + 1}</span> },
    { title: '商品编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 110 },
    { title: '商品名称', dataIndex: 'goodsName', key: 'goodsName', width: 200, ellipsis: true },
    { title: '分类', dataIndex: 'category', key: 'category', width: 80, ellipsis: true, render: (v: string) => v ? v.replace(/^成品\//, '') : '-' },
    { title: '定位', dataIndex: 'grade', key: 'grade', width: 70, render: (v: string) => {
      if (!v) return '-';
      const colors: Record<string, string> = { S: '#ef4444', A: '#f59e0b', B: '#4f46e5', C: '#10b981', D: '#94a3b8' };
      return <span style={{ color: colors[v] || '#64748b', fontWeight: 600, fontSize: 12 }}>{v}</span>;
    }},
    { title: '销售额', dataIndex: 'sales', key: 'sales', width: 120, align: 'right' as const, render: (v: number) => <span style={{ fontWeight: 600 }}>¥{v?.toLocaleString()}</span> },
    { title: '数量', dataIndex: 'qty', key: 'qty', width: 80, align: 'right' as const, render: (v: number) => v?.toLocaleString() },
  ];

  // ========== 饼图通用配置 ==========
  const makePieOption = (pieData: { value: number; name: string }[]) => {
    return {
      tooltip: { trigger: 'item' as const, formatter: '{b}: ¥{c} ({d}%)' },
      color: CHART_COLORS,
      legend: { show: false },
      series: [{
        type: 'pie', radius: ['35%', '65%'], center: ['50%', '50%'],
        label: {
          show: true,
          formatter: (p: any) => {
            const val = p.value >= 10000 ? `¥${(p.value / 10000).toFixed(1)}万` : `¥${p.value.toLocaleString()}`;
            return `{name|${p.name}}\n{val|${val}  ${p.percent}%}`;
          },
          rich: {
            name: { fontSize: 12, color: '#1e293b', fontWeight: 500 as any, lineHeight: 18 },
            val: { fontSize: 11, color: '#94a3b8', lineHeight: 16 },
          },
        },
        labelLine: {
          show: true, length: 12, length2: 16,
          lineStyle: { color: '#cbd5e1', width: 1 },
        },
        itemStyle: { borderColor: '#fff', borderWidth: 2, borderRadius: 4 },
        data: pieData,
      }],
    };
  };

  // ========== 销售占比饼图 ==========
  const channelPieData = (data.channels || []).map((c: any) => ({ value: c.total, name: deptNames[c.channel] || c.channel }));
  const channelPieOption = makePieOption(channelPieData);

  return (
    <div style={{ position: 'relative' }}>
      {/* 加载遮罩（暂时关闭） */}

      <DateFilter start={startDate} end={endDate} onChange={(s, e) => { setStartDate(s); setEndDate(e); }} />

      {/* 第1行：KPI 卡片 */}
      <Row gutter={[16, 16]}>
        {kpiCards.map((card, i) => (
          <Col xs={12} sm={8} lg={4} key={i}>
            <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.color }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div style={{ minWidth: 0 }}>
                  <div style={{ fontSize: 12, color: '#64748b', marginBottom: 4, whiteSpace: 'nowrap', display: 'flex', alignItems: 'center', gap: 4 }}>
                    {card.title}
                    {(card as any).tag && <span style={{ fontSize: 10, color: '#94a3b8', background: '#f1f5f9', borderRadius: 3, padding: '0 4px', lineHeight: '16px' }}>{(card as any).tag}</span>}
                  </div>
                  <div style={{ fontSize: 20, fontWeight: 700, color: '#1e293b', fontVariantNumeric: 'tabular-nums', whiteSpace: 'nowrap' }}>
                    {(card as any).animated ? <AnimatedNumber value={card.num} formatter={card.fmt} /> : card.fmt(card.num)}
                  </div>
                  <div style={{ fontSize: 11, color: '#b0b8c4', marginTop: 4, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{card.desc}</div>
                </div>
                <div style={{ fontSize: 20, color: card.color, opacity: 0.15, flexShrink: 0, marginLeft: 4 }}>{card.icon}</div>
              </div>
            </Card>
          </Col>
        ))}
      </Row>

      {/* 第2行：月度销售趋势（全宽扁长） */}
      <Card title="月度销售趋势" style={{ marginTop: 16 }}>
        <ReactECharts option={salesTrendOption} style={{ height: 180 }} />
      </Card>

      {/* 第3行：品类库存健康度 + 渠道销售占比 */}
      <div style={{ display: 'flex', gap: 16, marginTop: 16, alignItems: 'flex-start' }}>
        <div style={{ flex: 55, minWidth: 0 }}>
          <Card title="品类库存健康度">
            <Table
              dataSource={data.categories || []}
              columns={cateCols}
              rowKey="category"
              pagination={false}
              size="small"
              scroll={{ x: 630 }}
              summary={(pageData) => {
                const totals = pageData.reduce((acc, row: any) => ({
                  stockValue: acc.stockValue + (row.stockValue || 0),
                  dailySalesCost: acc.dailySalesCost + (row.dailySalesCost || 0),
                }), { stockValue: 0, dailySalesCost: 0 });
                const avgTurnover = totals.dailySalesCost > 0 ? totals.stockValue / totals.dailySalesCost : 0;
                return (
                  <Table.Summary fixed>
                    <Table.Summary.Row style={{ background: '#fafbfc', fontWeight: 600 }}>
                      <Table.Summary.Cell index={0}>汇总</Table.Summary.Cell>
                      <Table.Summary.Cell index={1} align="right">¥{totals.stockValue.toLocaleString(undefined, { maximumFractionDigits: 0 })}</Table.Summary.Cell>
                      <Table.Summary.Cell index={2} align="right">¥{totals.dailySalesCost.toLocaleString(undefined, { maximumFractionDigits: 0 })}</Table.Summary.Cell>
                      <Table.Summary.Cell index={3} align="right">{avgTurnover.toFixed(1)}</Table.Summary.Cell>
                      <Table.Summary.Cell index={4} align="right">-</Table.Summary.Cell>
                      <Table.Summary.Cell index={5} align="right">-</Table.Summary.Cell>
                    </Table.Summary.Row>
                  </Table.Summary>
                );
              }}
            />
          </Card>
        </div>
        <div style={{ flex: 45, minWidth: 0 }}>
          <Card title="各渠道销售额环比">
            <Table dataSource={data.channels || []} columns={channelCols} rowKey="channel" pagination={false} size="small" scroll={{ x: 550 }} />
          </Card>
          <Card title="渠道销售占比" style={{ marginTop: 16 }}>
            <ReactECharts option={channelPieOption} style={{ height: 240 }} />
          </Card>
        </div>
      </div>

      {/* 第4行：品类销售占比 + 品类毛利占比 */}
      <div style={{ display: 'flex', gap: 16, marginTop: 16 }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <Card title="品类销售占比" style={{ height: '100%' }}>
            <ReactECharts option={makePieOption(
              (data.cateSales || []).slice(0, 10).map((c: any) => ({ value: c.sales, name: c.category }))
            )} style={{ height: 240 }} />
          </Card>
        </div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <Card title="品类毛利占比" style={{ height: '100%' }}>
            <ReactECharts option={makePieOption(
              (data.cateSales || []).filter((c: any) => c.profit > 0).slice(0, 10).map((c: any) => ({ value: c.profit, name: c.category }))
            )} style={{ height: 240 }} />
          </Card>
        </div>
      </div>

      {/* 第5行：高库存明细(55%) + 缺货明细(45%) */}
      <div style={{ display: 'flex', gap: 16, marginTop: 16, alignItems: 'flex-start' }}>
        <div style={{ flex: 55, minWidth: 0 }}>
          <Card title={`高库存产品明细（周转>50天）· ${(data.highStockItems || []).length}个`}>
            <Table dataSource={data.highStockItems || []} columns={highStockCols} rowKey={(r: any) => r.goodsNo + r.warehouse} pagination={{ pageSize: 10, size: 'small' }} size="small" scroll={{ x: 840 }} />
          </Card>
        </div>
        <div style={{ flex: 45, minWidth: 0 }}>
          <Card title={`缺货产品明细 · ${(data.stockoutItems || []).length}个`}>
            <Table dataSource={data.stockoutItems || []} columns={stockoutCols} rowKey={(r: any) => r.goodsNo + r.warehouse} pagination={{ pageSize: 10, size: 'small' }} size="small" scroll={{ x: 650 }} />
          </Card>
        </div>
      </div>

      {/* 第6行：库龄>90天产品明细（全宽，8列需要） */}
      {(data.agedItems || []).length > 0 && (
        <Card title={`库龄>90天产品明细 · ${(data.agedItems || []).length}个`} style={{ marginTop: 16 }}>
          <Table dataSource={data.agedItems || []} columns={[
            { title: '商品编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 110 },
            { title: '商品名称', dataIndex: 'goodsName', key: 'goodsName', width: 200, ellipsis: true },
            { title: '仓库', dataIndex: 'warehouse', key: 'warehouse', width: 180, ellipsis: true },
            { title: '库存数量', dataIndex: 'qty', key: 'qty', width: 90, align: 'right' as const, render: (v: number) => v?.toLocaleString() },
            { title: '库存金额', dataIndex: 'stockValue', key: 'stockValue', width: 110, align: 'right' as const, render: (v: number) => `¥${v?.toLocaleString()}` },
            { title: '批次', dataIndex: 'batchNo', key: 'batchNo', width: 120 },
            { title: '生产日期', dataIndex: 'productionDate', key: 'productionDate', width: 100, render: (v: string) => v ? v.slice(0, 10) : '-' },
            { title: '库龄(天)', dataIndex: 'ageDays', key: 'ageDays', width: 90, align: 'right' as const, sorter: (a: any, b: any) => a.ageDays - b.ageDays, render: (v: number) => <span style={{ color: v > 180 ? '#ef4444' : '#f97316', fontWeight: 600 }}>{v}</span> },
          ]} rowKey={(r: any) => r.goodsNo + r.warehouse + r.batchNo} pagination={{ pageSize: 10, size: 'small' }} size="small" scroll={{ x: 1000 }} />
        </Card>
      )}

      {/* 第7行：销售额TOP20(50%) + 销售数量TOP20(50%) */}
      <div style={{ display: 'flex', gap: 16, marginTop: 16, alignItems: 'flex-start' }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <Card title="销售额 TOP20">
            <Table dataSource={data.topProducts || []} columns={topCols} rowKey="goodsNo" pagination={false} size="small" scroll={{ x: 700 }} />
          </Card>
        </div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <Card title="销售数量 TOP20">
            <Table dataSource={data.topQtyProducts || []} columns={[
              { title: '#', key: 'rank', width: 40, render: (_: any, __: any, i: number) => <span style={{ color: i < 3 ? '#4f46e5' : '#94a3b8', fontWeight: i < 3 ? 700 : 400 }}>{i + 1}</span> },
              { title: '商品编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 110 },
              { title: '商品名称', dataIndex: 'goodsName', key: 'goodsName', width: 200, ellipsis: true },
              { title: '分类', dataIndex: 'category', key: 'category', width: 80, ellipsis: true, render: (v: string) => v ? v.replace(/^成品\//, '') : '-' },
              { title: '定位', dataIndex: 'grade', key: 'grade', width: 70, render: (v: string) => {
                if (!v) return '-';
                const colors: Record<string, string> = { S: '#ef4444', A: '#f59e0b', B: '#4f46e5', C: '#10b981', D: '#94a3b8' };
                return <span style={{ color: colors[v] || '#64748b', fontWeight: 600, fontSize: 12 }}>{v}</span>;
              }},
              { title: '数量', dataIndex: 'qty', key: 'qty', width: 80, align: 'right' as const, render: (v: number) => <span style={{ fontWeight: 600 }}>{v?.toLocaleString()}</span> },
              { title: '销售额', dataIndex: 'sales', key: 'sales', width: 110, align: 'right' as const, render: (v: number) => `¥${v?.toLocaleString()}` },
            ]} rowKey="goodsNo" pagination={false} size="small" scroll={{ x: 690 }} />
          </Card>
        </div>
      </div>
    </div>
  );
};

export default PlanDashboard;
