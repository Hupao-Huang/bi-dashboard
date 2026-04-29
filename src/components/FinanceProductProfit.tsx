import React, { useEffect, useState, useCallback, useRef } from 'react';
import { DEPT_COLORS, CHART_COLORS } from '../chartTheme';
import { Row, Col, Card, Table, Statistic, Select, Radio, Switch } from 'antd';
import ReactECharts from './Chart';
import DateFilter from './DateFilter';
import PageLoading from './PageLoading';
import GoodsChannelExpand from './GoodsChannelExpand';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../config';

const deptOptions = [
  { value: 'ecommerce', label: '电商部门' },
  { value: 'social', label: '社媒部门' },
  { value: 'offline', label: '线下部门' },
  { value: 'distribution', label: '分销部门' },
];


const profitRateColor = (rate: number) => {
  if (rate >= 0.5) return '#10b981';
  if (rate >= 0.3) return '#faad14';
  return '#f5222d';
};

const FinanceProductProfit: React.FC = () => {
  const abortRef = useRef<AbortController | null>(null);
  const [dept, setDept] = useState('ecommerce');
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);
  const [sortedInfo, setSortedInfo] = useState<any>({});
  const [selectedGrade, setSelectedGrade] = useState<string>('S');
  const [pieMetric, setPieMetric] = useState<'sales' | 'profit'>('sales');
  const [pieDim, setPieDim] = useState<'grade' | 'dept' | 'channel'>('grade');
  const [showAllChannels, setShowAllChannels] = useState<boolean>(false);

  const color = DEPT_COLORS[dept] || '#1e40af';

  const fetchData = useCallback((d: string, s: string, e: string) => {
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    fetch(`${API_BASE}/api/department?dept=${d}&start=${s}&end=${e}&crossDept=1`, { signal: ctrl.signal })
      .then(res => res.json())
      .then(res => { setData(res.data); setLoading(false); })
      .catch((e: any) => { if (e?.name !== 'AbortError') setLoading(false); });
  }, []);

  useEffect(() => { fetchData(dept, startDate, endDate); }, [fetchData, dept, startDate, endDate]);

  const handleDateChange = (s: string, e: string) => {
    setStartDate(s);
    setEndDate(e);
  };

  if (loading) return <PageLoading />;
  if (!data) return <div>加载失败</div>;

  const goods = data.goods || [];
  const goodsChannels = data.goodsChannels || {};

  // 排名按 profit 降序固定（不随用户表头排序变化）
  const rankMap: Record<string, number> = {};
  [...goods]
    .sort((a: any, b: any) => (b.profit || 0) - (a.profit || 0))
    .forEach((g: any, i: number) => { rankMap[g.goodsNo] = i + 1; });

  const totalSales = goods.reduce((s: number, g: any) => s + (g.sales || 0), 0);
  const totalProfit = goods.reduce((s: number, g: any) => s + (g.profit || 0), 0);
  const overallProfitRate = totalSales > 0 ? totalProfit / totalSales : 0;
  const statCards = [
    { title: '总销售额', value: totalSales, precision: 2, prefix: '¥', accentColor: color },
    { title: '总毛利', value: totalProfit, precision: 2, prefix: '¥', accentColor: '#10b981' },
    { title: '综合毛利率', value: overallProfitRate * 100, precision: 1, suffix: '%', accentColor: profitRateColor(overallProfitRate) },
    { title: 'SKU种类数', value: goods.length, suffix: '种', accentColor: '#7c3aed' },
  ];

  // 产品定位 × 店铺 明细表（按 grade Tab 切换，含毛利；矩阵表已迁至利润总览）
  const gradeOrder = ['S', 'A', 'B', 'C', 'D', '未设置'];
  const fmtAmount = (v: number) => v >= 100000000
    ? (v / 100000000).toFixed(2) + '亿'
    : v >= 10000 ? (v / 10000).toFixed(1) + '万' : (v || 0).toFixed(0);
  const deptLabel: Record<string, string> = {
    ecommerce: '电商', social: '社媒', offline: '线下', distribution: '分销', '其他': '其他', '': '其他',
  };

  // ★ 单环饼图：产品定位 / 部门 / 渠道 占比 (Radio 维度+度量切换)
  const gssAllPie: any[] = data.gradeShopSalesAll || [];
  const pieMetricKey = pieMetric;

  // 维度键提取
  const pieKeyOf = (it: any): string => {
    if (pieDim === 'grade') return it.grade || '未设置';
    if (pieDim === 'dept') return deptLabel[it.department] ?? (it.department || '其他');
    return it.shopName || '其他'; // channel
  };
  const pieAggMap: Record<string, { sales: number; profit: number }> = {};
  gssAllPie.forEach((it: any) => {
    const key = pieKeyOf(it);
    if (!pieAggMap[key]) pieAggMap[key] = { sales: 0, profit: 0 };
    pieAggMap[key].sales += (it.sales || 0);
    pieAggMap[key].profit += (it.profit || 0);
  });

  // 配色（克制但可区分）
  const gradeColorMap: Record<string, string> = {
    'S': '#f59e0b',  // 顶级品 — 暖橙突出
    'A': '#1e40af',  // 主题深蓝
    'B': '#0ea5e9',  // 青蓝
    'C': '#14b8a6',  // 青绿
    'D': '#7c3aed',  // 紫
    '未设置': '#94a3b8',
  };
  const deptColorMap: Record<string, string> = {
    '电商': DEPT_COLORS.ecommerce || '#1e40af',
    '社媒': DEPT_COLORS.social || '#7c3aed',
    '线下': DEPT_COLORS.offline || '#d97706',
    '分销': DEPT_COLORS.distribution || '#059669',
    '其他': '#94a3b8',
  };

  let pieData: any[] = [];
  if (pieDim === 'grade' || pieDim === 'dept') {
    const order = pieDim === 'grade' ? gradeOrder : ['电商', '社媒', '线下', '分销', '其他'];
    const colorMap = pieDim === 'grade' ? gradeColorMap : deptColorMap;
    pieData = order
      .filter(k => pieAggMap[k] && Math.max(0, pieAggMap[k][pieMetricKey] || 0) > 0)
      .map(k => ({
        name: k,
        value: Math.max(0, pieAggMap[k][pieMetricKey] || 0),
        itemStyle: { color: colorMap[k] || '#94a3b8' },
      }));
  } else {
    // 渠道：按当前度量降序，可切换 Top 10 + 其他 / 全部展开
    const sorted = Object.entries(pieAggMap)
      .map(([name, v]) => ({ name, value: Math.max(0, v[pieMetricKey] || 0) }))
      .filter(x => x.value > 0)
      .sort((a, b) => b.value - a.value);
    const palette = CHART_COLORS && CHART_COLORS.length > 0 ? CHART_COLORS : ['#1e40af', '#0ea5e9', '#14b8a6', '#7c3aed', '#f59e0b', '#dc2626', '#059669', '#d97706', '#6366f1', '#ec4899'];
    if (showAllChannels) {
      pieData = sorted.map((x, i) => ({
        name: x.name,
        value: x.value,
        itemStyle: { color: palette[i % palette.length] },
      }));
    } else {
      const TOP = 10;
      const top = sorted.slice(0, TOP);
      const rest = sorted.slice(TOP);
      pieData = top.map((x, i) => ({
        name: x.name,
        value: x.value,
        itemStyle: { color: palette[i % palette.length] },
      }));
      if (rest.length > 0) {
        const restSum = rest.reduce((s, x) => s + x.value, 0);
        pieData.push({
          name: `其他 (${rest.length} 家)`,
          value: restSum,
          itemStyle: { color: '#cbd5e1' },
        });
      }
    }
  }
  const pieTotal = pieData.reduce((s, x) => s + x.value, 0);
  const pieMetricLabel = pieMetric === 'sales' ? '销售' : '毛利';
  const pieDimLabel = pieDim === 'grade' ? '产品定位' : pieDim === 'dept' ? '部门' : '渠道';

  // 横向条形图：按值降序 (保留 grade 固定顺序时除外)
  // grade 维度按 S→未设置 顺序更易读, 其他按值降序
  const barSorted = pieDim === 'grade'
    ? pieData
    : [...pieData].sort((a, b) => b.value - a.value);
  // 横向条 yAxis 从下往上画第一项, 所以反转一次让最大值在最上
  const barNames = barSorted.map(x => x.name).reverse();
  const barValues = barSorted.map(x => ({
    value: x.value,
    itemStyle: x.itemStyle,
  })).reverse();
  const pieOption = {
    tooltip: {
      trigger: 'axis' as const,
      axisPointer: { type: 'shadow' as const },
      formatter: (params: any) => {
        const p = Array.isArray(params) ? params[0] : params;
        const v = p?.value || 0;
        const pct = pieTotal > 0 ? (v / pieTotal * 100).toFixed(1) : '0.0';
        return `<div style="font-weight:600;margin-bottom:4px">${p?.name || ''}</div>${pieMetricLabel} ¥${fmtAmount(v)}<br/>占比 ${pct}%`;
      },
    },
    title: {
      text: `按${pieDimLabel}·${pieMetricLabel}`,
      subtext: `合计 ¥${fmtAmount(pieTotal)}`,
      left: 'center' as const,
      top: 4,
      textStyle: { fontSize: 14, color: '#475569' },
      subtextStyle: { fontSize: 12, color: '#94a3b8' },
    },
    grid: { left: 110, right: 100, top: 60, bottom: 16 },
    xAxis: {
      type: 'value' as const,
      axisLabel: { formatter: (v: number) => fmtAmount(v), color: '#64748b' },
      splitLine: { lineStyle: { color: '#f1f5f9' } },
    },
    yAxis: {
      type: 'category' as const,
      data: barNames,
      axisTick: { show: false },
      axisLine: { lineStyle: { color: '#cbd5e1' } },
      axisLabel: { color: '#1e293b', fontSize: 12, fontWeight: 600 },
    },
    series: [{
      type: 'bar' as const,
      data: barValues,
      barWidth: pieDim === 'channel' ? 16 : 22,
      label: {
        show: true,
        position: 'right' as const,
        formatter: (p: any) => {
          const v = p.value || 0;
          const pct = pieTotal > 0 ? (v / pieTotal * 100).toFixed(1) : '0.0';
          return `¥${fmtAmount(v)}  ${pct}%`;
        },
        fontSize: 11,
        color: '#475569',
      },
    }],
  };
  const barChartHeight = pieDim === 'channel'
    ? (showAllChannels ? Math.max(420, barNames.length * 26 + 80) : Math.max(360, barNames.length * 32 + 80))
    : Math.max(280, barNames.length * 42 + 80);
  const gssAll: any[] = data.gradeShopSalesAll || [];
  const shopByGrade: Record<string, { shopName: string; sales: number; profit: number }[]> = {};
  gssAll.forEach((it: any) => {
    const g = it.grade || '未设置';
    if (!shopByGrade[g]) shopByGrade[g] = [];
    shopByGrade[g].push({ shopName: it.shopName || '其他', sales: it.sales || 0, profit: it.profit || 0 });
  });
  Object.keys(shopByGrade).forEach(g => {
    shopByGrade[g].sort((a, b) => b.sales - a.sales);
  });
  const gradeTabs = gradeOrder.filter(g => (shopByGrade[g] || []).length > 0);
  const shopGradeData = (selectedGrade && shopByGrade[selectedGrade]) || [];
  const shopGradeTotal = shopGradeData.reduce((s, x) => s + x.sales, 0);
  const shopGradeProfitTotal = shopGradeData.reduce((s, x) => s + x.profit, 0);
  const shopColumns = [
    { title: '排名', key: 'rank', width: 60, align: 'center' as const, render: (_: any, __: any, i: number) => i + 1 },
    { title: '店铺/渠道', dataIndex: 'shopName', key: 'shopName', ellipsis: true },
    {
      title: '销售额', dataIndex: 'sales', key: 'sales', width: 130, align: 'right' as const,
      sorter: (a: any, b: any) => a.sales - b.sales,
      defaultSortOrder: 'descend' as const,
      render: (v: number) => <span style={{ fontVariantNumeric: 'tabular-nums' as const }}>¥{(v || 0).toLocaleString()}</span>,
    },
    {
      title: '占比', dataIndex: 'sales', key: 'pct', width: 150, align: 'left' as const,
      render: (v: number) => {
        const pct = shopGradeTotal > 0 ? (v / shopGradeTotal * 100) : 0;
        return (
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <div style={{ flex: 1, height: 6, background: '#f1f5f9', borderRadius: 3, overflow: 'hidden' }}>
              <div style={{ width: pct + '%', height: '100%', background: '#1e40af' }} />
            </div>
            <span style={{ width: 50, textAlign: 'right' as const, fontSize: 12, color: '#475569', fontVariantNumeric: 'tabular-nums' as const }}>{pct.toFixed(1)}%</span>
          </div>
        );
      },
    },
    {
      title: '毛利', dataIndex: 'profit', key: 'profit', width: 130, align: 'right' as const,
      sorter: (a: any, b: any) => (a.profit || 0) - (b.profit || 0),
      render: (v: number) => <span style={{ fontVariantNumeric: 'tabular-nums' as const, color: '#10b981', fontWeight: 600 }}>¥{(v || 0).toLocaleString()}</span>,
    },
    {
      title: '毛利率', key: 'profitRate', width: 90, align: 'right' as const,
      sorter: (a: any, b: any) => {
        const ra = a.sales > 0 ? a.profit / a.sales : 0;
        const rb = b.sales > 0 ? b.profit / b.sales : 0;
        return ra - rb;
      },
      render: (_: any, row: any) => {
        const rate = row.sales > 0 ? (row.profit || 0) / row.sales : 0;
        return <span style={{ color: profitRateColor(rate), fontWeight: 600, fontVariantNumeric: 'tabular-nums' as const }}>{(rate * 100).toFixed(1)}%</span>;
      },
    },
  ];

  // Table with sorting
  const goodsColumns = [
    {
      title: '排名',
      key: 'rank',
      width: 60,
      align: 'center' as const,
      render: (_: any, row: any) => {
        const v = rankMap[row.goodsNo] || 0;
        const bg = v === 1 ? '#f5222d' : v === 2 ? '#fa8c16' : v === 3 ? '#faad14' : '#94a3b8';
        return (
          <span style={{
            display: 'inline-block', width: 24, height: 24, lineHeight: '24px',
            borderRadius: '50%', background: bg, color: '#fff', fontWeight: 600, fontSize: 12,
          }}>{v}</span>
        );
      },
    },
    { title: '商品编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 110 },
    { title: '商品名称', dataIndex: 'goodsName', key: 'goodsName', ellipsis: true },
    { title: '品牌', dataIndex: 'brand', key: 'brand', width: 90 },
    {
      title: '定位',
      dataIndex: 'grade',
      key: 'grade',
      width: 80,
      align: 'center' as const,
      filters: [
        { text: 'S', value: 'S' },
        { text: 'A', value: 'A' },
        { text: 'B', value: 'B' },
        { text: 'C', value: 'C' },
        { text: 'D', value: 'D' },
        { text: '未设置', value: '' },
      ],
      onFilter: (value: any, record: any) => (record.grade || '') === value,
      render: (v: string) => v
        ? <span style={{ fontWeight: 600, color: '#0f172a' }}>{v}</span>
        : <span style={{ color: '#94a3b8' }}>—</span>,
    },
    {
      title: '销售额',
      dataIndex: 'sales',
      key: 'sales',
      width: 130,
      sorter: (a: any, b: any) => a.sales - b.sales,
      sortOrder: sortedInfo.columnKey === 'sales' ? sortedInfo.order : null,
      render: (v: number) => `¥${(v || 0).toLocaleString()}`,
    },
    {
      title: '毛利',
      dataIndex: 'profit',
      key: 'profit',
      width: 130,
      sorter: (a: any, b: any) => (a.profit || 0) - (b.profit || 0),
      sortOrder: sortedInfo.columnKey === 'profit' ? sortedInfo.order : null,
      defaultSortOrder: 'descend' as const,
      render: (v: number) => `¥${(v || 0).toLocaleString()}`,
    },
    {
      title: '毛利率',
      key: 'profit_rate',
      width: 100,
      sorter: (a: any, b: any) => {
        const ra = a.sales > 0 ? (a.profit || 0) / a.sales : 0;
        const rb = b.sales > 0 ? (b.profit || 0) / b.sales : 0;
        return ra - rb;
      },
      sortOrder: sortedInfo.columnKey === 'profit_rate' ? sortedInfo.order : null,
      render: (_: any, row: any) => {
        const rate = row.sales > 0 ? (row.profit || 0) / row.sales : 0;
        return <span style={{ color: profitRateColor(rate), fontWeight: 600 }}>{(rate * 100).toFixed(1)}%</span>;
      },
    },
  ];

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', alignItems: 'center', gap: 12 }}>
        <span style={{ fontWeight: 500 }}>选择部门：</span>
        <Select
          value={dept}
          onChange={val => setDept(val)}
          options={deptOptions}
          style={{ width: 160 }}
        />
      </div>

      <DateFilter start={startDate} end={endDate} onChange={handleDateChange} />

      {/* Summary */}
      <Row gutter={[16, 16]}>
        {statCards.map((card) => (
          <Col xs={24} sm={6} key={card.title}>
            <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
              <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
            </Card>
          </Col>
        ))}
      </Row>

      {/* 占比饼图（维度 + 度量双切换） */}
      <Card
        title="占比图"
        style={{ marginTop: 16 }}
        extra={
          <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
            <Radio.Group
              value={pieDim}
              onChange={(e) => setPieDim(e.target.value)}
              size="small"
              optionType="button"
              options={[
                { label: '按定位', value: 'grade' },
                { label: '按部门', value: 'dept' },
                { label: '按渠道', value: 'channel' },
              ]}
            />
            {pieDim === 'channel' && (
              <span style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, color: '#475569' }}>
                <Switch
                  size="small"
                  checked={showAllChannels}
                  onChange={setShowAllChannels}
                />
                <span>{showAllChannels ? '全部渠道' : `Top 10`}</span>
              </span>
            )}
            <Radio.Group
              value={pieMetric}
              onChange={(e) => setPieMetric(e.target.value)}
              size="small"
              optionType="button"
              buttonStyle="solid"
              options={[
                { label: '按销售', value: 'sales' },
                { label: '按毛利', value: 'profit' },
              ]}
            />
          </div>
        }
      >
        <ReactECharts lazyUpdate={true} option={pieOption} style={{ height: barChartHeight }} />
      </Card>

      {/* 产品定位 × 店铺 明细 (跨 4 部门全口径) */}
      <Card
        title="产品定位 × 店铺 明细（跨部门全口径）"
        style={{ marginTop: 16 }}
      >
        <div style={{ fontWeight: 600, color: '#475569', margin: '0 0 8px', display: 'flex', alignItems: 'center', gap: 12 }}>
          <span>选择产品定位</span>
          <Select
            value={gradeTabs.includes(selectedGrade) ? selectedGrade : (gradeTabs[0] || 'S')}
            onChange={setSelectedGrade}
            options={gradeTabs.map(g => ({ value: g, label: `${g} 品 (${(shopByGrade[g] || []).length} 店)` }))}
            style={{ width: 200 }}
            size="small"
          />
          <span style={{ marginLeft: 'auto', color: '#64748b', fontWeight: 400, fontSize: 12 }}>
            {selectedGrade} 品合计：销售 ¥{shopGradeTotal.toLocaleString()} · 毛利 <span style={{ color: '#10b981', fontWeight: 600 }}>¥{shopGradeProfitTotal.toLocaleString()}</span>
            {shopGradeTotal > 0 && (
              <span style={{ color: profitRateColor(shopGradeProfitTotal / shopGradeTotal), fontWeight: 600 }}> ({(shopGradeProfitTotal / shopGradeTotal * 100).toFixed(1)}%)</span>
            )} · {shopGradeData.length} 个店铺
          </span>
        </div>
        <Table
          dataSource={shopGradeData.map((d, i) => ({ ...d, key: i }))}
          columns={shopColumns}
          pagination={{ pageSize: 15, showSizeChanger: true }}
          size="small"
        />
      </Card>

      {/* Table */}
      <Card className="bi-table-card" title="产品利润统计明细" style={{ marginTop: 16 }}>
        <Table
          dataSource={goods}
          columns={goodsColumns}
          rowKey="goodsNo"
          pagination={{ pageSize: 20, showSizeChanger: true }}
          size="small"
          onChange={(_, __, sorter: any) => setSortedInfo(sorter)}
          expandable={{
            expandedRowRender: (record: any) => {
              const channels: any[] = goodsChannels[record.goodsNo] || [];
              return <GoodsChannelExpand channels={channels} mode="department" />;
            },
            rowExpandable: (record: any) => (goodsChannels[record.goodsNo] || []).length > 0,
          }}
        />
      </Card>
    </div>
  );
};

export default FinanceProductProfit;
