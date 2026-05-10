import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Row, Col, Card, Statistic, Table, Tooltip } from 'antd';
import {
  ShoppingCartOutlined,
  GlobalOutlined,
  ShopOutlined,
  ShareAltOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import ReactECharts from '../../components/Chart';
import DateFilter from '../../components/DateFilter';
import PageLoading from '../../components/PageLoading';
import GoodsChannelExpand from '../../components/GoodsChannelExpand';
import { formatShopBarTooltip } from './shopBarTooltip';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../../config';
import {
  getBaseOption,
  barItemStyle,
  lineAreaStyle,
  pieStyle,
  formatMoney,
  formatWanHint,
  getNiceAxisInterval,
  DEPT_COLORS,
  GRADE_COLORS,
} from '../../chartTheme';

const deptConfig: Record<string, { label: string; color: string; icon: React.ReactNode }> = {
  ecommerce: { label: '电商部门', color: DEPT_COLORS.ecommerce, icon: <ShoppingCartOutlined /> },
  social: { label: '社媒部门', color: DEPT_COLORS.social, icon: <GlobalOutlined /> },
  offline: { label: '线下部门', color: DEPT_COLORS.offline, icon: <ShopOutlined /> },
  distribution: { label: '分销部门', color: DEPT_COLORS.distribution, icon: <ShareAltOutlined /> },
  instant_retail: { label: '即时零售部', color: DEPT_COLORS.instant_retail, icon: <ShoppingCartOutlined /> },
};

const OverviewPage: React.FC = () => {
  const [data, setData] = useState<any>(null);
  const [trendData, setTrendData] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);
  const [activeDept, setActiveDept] = useState<string>('ecommerce');
  const requestSeqRef = useRef(0);

  const fetchData = useCallback(async (s: string, e: string) => {
    const reqId = ++requestSeqRef.current;
    setLoading(true);

    const diffDays = dayjs(e).diff(dayjs(s), 'day');
    const trendStart = diffDays <= 3 ? dayjs(e).subtract(13, 'day').format('YYYY-MM-DD') : s;
    const trendEnd = e;

    try {
      const response = await fetch(
        `${API_BASE}/api/overview?start=${s}&end=${e}&trendStart=${trendStart}&trendEnd=${trendEnd}`,
      );
      const result = await response.json();
      if (reqId !== requestSeqRef.current) return;

      setData(result.data);
      setTrendData(result.data?.trend || []);
    } catch {
      if (reqId !== requestSeqRef.current) return;
      setData(null);
      setTrendData([]);
    } finally {
      if (reqId === requestSeqRef.current) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    void fetchData(startDate, endDate);
  }, [fetchData, startDate, endDate]);

  const currentDepts = data?.departments || [];
  const visibleDepts = currentDepts.filter((dept: any) => dept.department !== 'other');

  useEffect(() => {
    if (!visibleDepts.some((dept: any) => dept.department === activeDept)) {
      setActiveDept(visibleDepts[0]?.department || 'ecommerce');
    }
  }, [activeDept, visibleDepts]);

  const formatDate = (d: string) => {
    if (!d) return '';
    const m = d.match(/(\d{4}-)?(\d{2}-\d{2})/);
    return m ? m[2] : d.slice(0, 10);
  };

  const trendComputed = useMemo(() => {
    const isShortRange = dayjs(endDate).diff(dayjs(startDate), 'day') <= 3;
    const isSingleDay = startDate === endDate;
    const trendWindowStart = isShortRange ? dayjs(endDate).subtract(13, 'day').format('YYYY-MM-DD') : startDate;
    const trendWindowEnd = endDate;

    const deptTrend = trendData.filter((t: any) => t.department === activeDept);
    const deptTrendMap = new Map(deptTrend.map((t: any) => [String(t.date).slice(0, 10), t]));

    const trendDatesRaw: string[] = [];
    for (
      let cursor = dayjs(trendWindowStart);
      !cursor.isAfter(dayjs(trendWindowEnd), 'day');
      cursor = cursor.add(1, 'day')
    ) {
      trendDatesRaw.push(cursor.format('YYYY-MM-DD'));
    }

    const trendDates = trendDatesRaw.map(formatDate);
    const inSelected = (date: string) => date >= startDate && date <= endDate;

    const trendSalesRaw = trendDatesRaw.map((date) => deptTrendMap.get(date)?.sales || 0);
    const trendSales = trendSalesRaw;
    const trendQty = trendDatesRaw.map((date) => deptTrendMap.get(date)?.qty || 0);
    const trendAvgPrice = trendQty.map((qty, index) => (qty > 0 ? +(trendSales[index] / qty).toFixed(2) : 0));

    return {
      isShortRange,
      isSingleDay,
      inSelected,
      trendDates,
      trendDatesRaw,
      trendSales,
      trendQty,
      trendAvgPrice,
    };
  }, [activeDept, endDate, startDate, trendData]);

  const {
    isShortRange,
    inSelected,
    trendDates,
    trendDatesRaw,
    trendSales,
    trendQty,
    trendAvgPrice,
  } = trendComputed;
  const isLongRange = trendDatesRaw.length >= 60;
  const activeCfg = deptConfig[activeDept] || deptConfig.ecommerce;

  const baseOpt = useMemo(() => getBaseOption(), []);
  const splits = 5;
  const maxSales = Math.max(...trendSales, 1);
  const maxQty = Math.max(...trendQty, 1);
  const salesInterval = getNiceAxisInterval(maxSales, splits);
  const qtyInterval = getNiceAxisInterval(maxQty, splits);

  const trendOption = useMemo(() => ({
    ...baseOpt,
    animation: !isLongRange,
    legend: { ...baseOpt.legend, data: ['销售额', '货品数'], top: 4 },
    grid: { left: 60, right: 60, top: 48, bottom: 32 },
    xAxis: { ...baseOpt.xAxis, type: 'category' as const, data: trendDates, axisTick: { alignWithLabel: true } },
    yAxis: [
      {
        ...baseOpt.yAxis,
        type: 'value' as const,
        name: '销售额',
        min: 0,
        max: salesInterval * splits,
        interval: salesInterval,
        axisLabel: { ...baseOpt.yAxis.axisLabel, formatter: formatMoney },
      },
      {
        ...baseOpt.yAxis,
        type: 'value' as const,
        name: '货品数',
        min: 0,
        max: qtyInterval * splits,
        interval: qtyInterval,
        position: 'right' as const,
      },
    ],
    series: [
      {
        name: '销售额',
        type: 'bar',
        data: isShortRange
          ? trendDatesRaw.map((date: string, i: number) => ({
              value: trendSales[i],
              itemStyle: { color: inSelected(date) ? activeCfg.color : activeCfg.color + '30' },
            }))
          : trendSales,
        ...(isShortRange ? {} : barItemStyle(activeCfg.color)),
        barWidth: 14,
      },
      {
        name: '货品数',
        type: 'line',
        yAxisIndex: 1,
        smooth: true,
        data: trendQty,
        ...lineAreaStyle('#f59e0b'),
        symbol: isLongRange ? 'none' : 'circle',
        symbolSize: 4,
      },
    ],
  }), [
    activeCfg.color,
    baseOpt,
    inSelected,
    isLongRange,
    isShortRange,
    qtyInterval,
    salesInterval,
    trendDates,
    trendDatesRaw,
    trendQty,
    trendSales,
  ]);

  const avgPriceAvg =
    trendAvgPrice.length > 0 ? +(trendAvgPrice.reduce((a: number, b: number) => a + b, 0) / trendAvgPrice.length).toFixed(2) : 0;
  const avgPriceOption = useMemo(() => ({
    ...baseOpt,
    animation: !isLongRange,
    legend: { show: false },
    grid: { left: 56, right: 80, top: 16, bottom: 32 },
    xAxis: { ...baseOpt.xAxis, type: 'category' as const, data: trendDates, boundaryGap: false },
    yAxis: {
      ...baseOpt.yAxis,
      type: 'value' as const,
      min: 0,
      axisLabel: { ...baseOpt.yAxis.axisLabel, formatter: (v: number) => `¥${v}` },
    },
    series: [
      {
        name: '客单价',
        type: 'line',
        smooth: true,
        data: trendAvgPrice,
        ...lineAreaStyle(activeCfg.color),
        symbol: isLongRange ? 'none' : 'circle',
        symbolSize: 4,
        markLine: {
          silent: true,
          data: [
            {
              yAxis: avgPriceAvg,
              label: {
                formatter: `均值: ¥${avgPriceAvg}`,
                position: 'insideEndTop' as const,
                fontSize: 11,
                color: '#94a3b8',
              },
              lineStyle: { type: 'dashed' as const, color: '#e2e8f0' },
            },
          ],
        },
      },
    ],
  }), [
    avgPriceAvg,
    baseOpt,
    isLongRange,
    trendAvgPrice,
    trendDates,
  ]);

  const pieData = currentDepts
    .filter((d: any) => d.department !== 'other')
    .map((d: any) => ({
      value: d.sales,
      name: deptConfig[d.department]?.label || d.department,
      itemStyle: { color: deptConfig[d.department]?.color },
    }));
  const pieOption = useMemo(() => ({
    ...pieStyle,
    animation: !isLongRange,
    series: [
      {
        type: 'pie',
        radius: ['42%', '72%'],
        center: ['50%', '45%'],
        label: { show: true, formatter: '{b}\n{d}%', fontSize: 11, lineHeight: 16, color: '#64748b' },
        labelLine: { length: 18, length2: 12, lineStyle: { color: '#e2e8f0' } },
        itemStyle: { borderColor: '#fff', borderWidth: 2, borderRadius: 4 },
        emphasis: { scaleSize: 6 },
        data: pieData,
      },
    ],
  }), [isLongRange, pieData]);

  const grades = data?.grades || [];
  const gradeDeptSales: any[] = data?.gradeDeptSales || [];
  const totalGradeSales = grades.reduce((s: number, g: any) => s + g.sales, 0);

  // 按 grade 汇聚部门明细
  const gradeDeptMap = new Map<string, { total: number; depts: { dept: string; label: string; sales: number }[] }>();
  gradeDeptSales.forEach((item: any) => {
    const entry = gradeDeptMap.get(item.grade) || { total: 0, depts: [] };
    entry.total += item.sales;
    entry.depts.push({
      dept: item.department,
      label: deptConfig[item.department]?.label || item.department,
      sales: item.sales,
    });
    gradeDeptMap.set(item.grade, entry);
  });
  gradeDeptMap.forEach(entry => entry.depts.sort((a, b) => b.sales - a.sales));

  const gradePieOption = {
    ...pieStyle,
    animation: !isLongRange,
    tooltip: {
      ...pieStyle.tooltip,
      trigger: 'item' as const,
      formatter: (p: any) => {
        const pct = totalGradeSales > 0 ? (p.value / totalGradeSales * 100).toFixed(1) : '0.0';
        const depts: { label: string; sales: number }[] = p.data?._depts || [];
        let html = `<b>${p.name}</b>：¥${p.value?.toLocaleString()}（${pct}%）`;
        if (depts.length > 0) {
          html += '<br/><div style="margin-top:6px;border-top:1px solid #eee;padding-top:6px">';
          depts.forEach(d => {
            const dPct = totalGradeSales > 0 ? (d.sales / totalGradeSales * 100).toFixed(1) : '0.0';
            html += `${d.label}：¥${d.sales.toLocaleString()}（${dPct}%）<br/>`;
          });
          html += '</div>';
        }
        return html;
      },
    },
    legend: {
      orient: 'horizontal' as const,
      left: 'center',
      bottom: 0,
      type: 'scroll' as const,
      itemGap: 14,
      textStyle: { fontSize: 11 },
    },
    series: [{
      type: 'pie',
      radius: ['30%', '60%'],
      center: ['50%', '45%'],
      label: {
        show: true,
        formatter: (p: any) => {
          const pct = totalGradeSales > 0 ? (p.value / totalGradeSales * 100).toFixed(1) : '0.0';
          return `${p.name}\n{value|${pct}%}`;
        },
        rich: { value: { fontSize: 11, color: '#999', lineHeight: 18 } },
        fontSize: 12,
        color: '#333',
      },
      labelLayout: { hideOverlap: true },
      labelLine: { length: 15, length2: 18, lineStyle: { color: '#cbd5e1' } },
      itemStyle: { borderRadius: 4, borderColor: '#fff', borderWidth: 2 },
      data: grades.map((g: any) => ({
        value: g.sales,
        name: g.grade + '品',
        _depts: gradeDeptMap.get(g.grade)?.depts || [],
        itemStyle: { color: GRADE_COLORS[g.grade] || '#94a3b8' },
      })),
    }],
  };

  const goodsChannels = data?.goodsChannels || {};
  const indexedTopGoods = (data?.topGoods || []).map((g: any, i: number) => ({ ...g, _rank: i + 1 }));
  const goodsColumns = [
    {
      title: '#',
      dataIndex: '_rank',
      key: 'rank',
      width: 40,
      render: (v: number) => (
        <span style={{ color: v <= 3 ? '#1e40af' : '#94a3b8', fontWeight: v <= 3 ? 700 : 400 }}>{v}</span>
      ),
    },
    { title: '商品编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 100 },
    { title: '商品名称', dataIndex: 'goodsName', key: 'goodsName', ellipsis: true },
    { title: '产品定位', dataIndex: 'grade', key: 'grade', width: 80,
      render: (v: string) => {
        return v ? <span style={{ color: GRADE_COLORS[v] || '#94a3b8', fontWeight: 600 }}>{v}</span> : <span style={{ color: '#cbd5e1' }}>-</span>;
      },
    },
    { title: '分类', dataIndex: 'category', key: 'category', width: 100, ellipsis: true },
    { title: '品牌', dataIndex: 'brand', key: 'brand', width: 90, ellipsis: true },
    {
      title: '销售额',
      dataIndex: 'sales',
      key: 'sales',
      width: 190,
      render: (v: number) => {
        const hint = formatWanHint(v || 0);
        return (
          <span style={{ fontVariantNumeric: 'tabular-nums', whiteSpace: 'nowrap' }}>
            <span style={{ fontWeight: 600 }}>¥{v?.toLocaleString()}</span>
            {hint && <span style={{ color: '#94a3b8', fontSize: 12, marginLeft: 6, whiteSpace: 'nowrap' }}>{hint.replace('约', '≈')}</span>}
          </span>
        );
      },
    },
    { title: '销量', dataIndex: 'qty', key: 'qty', width: 130, render: (v: number) => {
      const hint = formatWanHint(v || 0);
      return (
        <span style={{ fontVariantNumeric: 'tabular-nums', whiteSpace: 'nowrap' }}>
          {v?.toLocaleString()}
          {hint && <span style={{ color: '#94a3b8', fontSize: 12, marginLeft: 4, whiteSpace: 'nowrap' }}>{hint.replace('约', '≈')}</span>}
        </span>
      );
    }},
    {
      title: '客单价',
      key: 'avgPrice',
      width: 100,
      render: (_: any, r: any) => (r.qty > 0 ? `¥${(r.sales / r.qty).toLocaleString(undefined, { maximumFractionDigits: 2 })}` : '-'),
    },
  ];

  const shopNames = (data?.topShops || []).map((s: any) => s.shopName).reverse();
  const shopSales = (data?.topShops || []).map((s: any) => s.sales).reverse();
  const shopBreakdown = data?.shopBreakdown || {};
  const shopBarOption = useMemo(() => ({
    ...baseOpt,
    animation: !isLongRange,
    grid: { left: 8, right: 40, top: 8, bottom: 8, containLabel: true },
    tooltip: {
      trigger: 'axis' as const,
      axisPointer: { type: 'shadow' as const },
      enterable: true,
      confine: true,
      appendToBody: true,
      extraCssText: 'max-width: 380px; white-space: normal; z-index: 1100;',
      formatter: (params: any) => {
        const p = Array.isArray(params) ? params[0] : params;
        return formatShopBarTooltip(p.name, p.value || 0, shopBreakdown[p.name]);
      },
    },
    xAxis: { ...baseOpt.xAxis, type: 'value' as const, axisLabel: { ...baseOpt.xAxis.axisLabel, formatter: formatMoney } },
    yAxis: {
      ...baseOpt.yAxis,
      type: 'category' as const,
      data: shopNames,
      axisLabel: { color: '#334155', fontSize: 12, width: 220, overflow: 'none' as const },
    },
    series: [{ type: 'bar', data: shopSales, ...barItemStyle('#1e40af'), barWidth: 16 }],
  }), [baseOpt, isLongRange, shopNames, shopSales, shopBreakdown]);

  const totalSales = currentDepts.reduce((s: number, d: any) => s + d.sales, 0);
  const totalQty = currentDepts.reduce((s: number, d: any) => s + d.qty, 0);
  const avgOrderValue = totalQty > 0 ? totalSales / totalQty : 0;

  const statColors = ['#1e40af', '#f59e0b', '#06b6d4'];
  const fmtDept = (v: number) => v >= 10000 ? `¥${(v / 10000).toFixed(1)}万` : `¥${v.toLocaleString()}`;
  const fmtDeptQty = (v: number) => v >= 10000 ? `${(v / 10000).toFixed(1)}万` : v.toLocaleString();
  const summaryCards = [
    { title: '总销售额', value: totalSales, precision: 2, prefix: '¥', color: statColors[0],
      depts: visibleDepts.map((d: any) => ({ label: deptConfig[d.department]?.label || d.department, value: fmtDept(d.sales), color: deptConfig[d.department]?.color })) },
    { title: '总货品数', value: totalQty, precision: 0, prefix: '', color: statColors[1],
      depts: visibleDepts.map((d: any) => ({ label: deptConfig[d.department]?.label || d.department, value: fmtDeptQty(d.qty), color: deptConfig[d.department]?.color })) },
    { title: '综合客单价', value: avgOrderValue, precision: 2, prefix: '¥', color: statColors[2],
      depts: visibleDepts.map((d: any) => ({ label: deptConfig[d.department]?.label || d.department, value: `¥${d.qty > 0 ? (d.sales / d.qty).toFixed(0) : '-'}`, color: deptConfig[d.department]?.color })) },
  ];

  const handleDateChange = (s: string, e: string) => {
    setStartDate(s);
    setEndDate(e);
  };

  if (loading) return <PageLoading />;
  if (!data) return <div className="bi-empty-state">加载失败</div>;

  return (
    <div>
      <DateFilter start={startDate} end={endDate} onChange={handleDateChange} />

      <Row gutter={[16, 16]}>
        {summaryCards.map((card: any, i: number) => {
          const hint = formatWanHint(card.value);
          return (
            <Col xs={24} sm={8} key={i}>
              <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.color }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                  <div>
                    <Statistic
                      title={card.title}
                      value={card.value}
                      precision={card.precision}
                      prefix={card.prefix}
                    />
                    <div style={{ fontSize: 14, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>
                      {hint ? hint.replace('约', '≈ ') : ' '}
                    </div>
                  </div>
                  {card.depts && card.depts.length > 0 && (
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, justifyContent: 'flex-end', maxWidth: '60%' }}>
                      {card.depts.map((d: any) => (
                        <span key={d.label} style={{ fontSize: 11, color: '#64748b', background: `${d.color}10`, border: `1px solid ${d.color}20`, borderRadius: 4, padding: '1px 6px', whiteSpace: 'nowrap' }}>
                          <span style={{ color: d.color, fontWeight: 600 }}>{d.label}</span> {d.value}
                        </span>
                      ))}
                    </div>
                  )}
                </div>
              </Card>
            </Col>
          );
        })}
      </Row>

      <Row gutter={[12, 12]} style={{ marginTop: 16 }} wrap>
        {visibleDepts.map((dept: any) => {
          const cfg = deptConfig[dept.department] || { label: dept.department, color: '#999', icon: null };
          const isActive = activeDept === dept.department;
          return (
            <Col flex="1 1 220px" style={{ minWidth: 200 }} key={dept.department}>
              <Card
                hoverable
                className={`bi-dept-card${isActive ? ' active' : ''}`}
                onClick={() => setActiveDept(dept.department)}
                style={{
                  ['--active-color' as any]: cfg.color,
                  ['--active-glow' as any]: `${cfg.color}15`,
                  ['--active-bg' as any]: `${cfg.color}04`,
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 12 }}>
                  <div
                    style={{
                      width: 34,
                      height: 34,
                      borderRadius: 12,
                      display: 'inline-flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      fontSize: 15,
                      background: `${cfg.color}12`,
                      color: cfg.color,
                    }}
                  >
                    {cfg.icon}
                  </div>
                  <span style={{ color: '#64748b', fontSize: 13, fontWeight: 600 }}>{cfg.label}</span>
                  {dept.department === 'ecommerce' && (
                    <Tooltip title="电商部门 KPI 不含特殊渠道调拨金额（京东自营/天猫超市寄售），按销售单统计">
                      <span style={{ fontSize: 10, color: '#94a3b8', background: '#f1f5f9', borderRadius: 3, padding: '0 4px', lineHeight: '14px', cursor: 'help' }}>
                        不含调拨
                      </span>
                    </Tooltip>
                  )}
                </div>
                <div
                  style={{
                    color: '#1e293b',
                    fontSize: 24,
                    fontWeight: 700,
                    fontVariantNumeric: 'tabular-nums',
                    letterSpacing: '-0.02em',
                  }}
                >
                  ¥{dept.sales?.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                </div>
                {formatWanHint(dept.sales || 0) && (
                  <div style={{ fontSize: 13, color: '#64748b', marginTop: 2, fontVariantNumeric: 'tabular-nums', fontWeight: 400 }}>
                    {formatWanHint(dept.sales || 0).replace('约', '≈ ')}
                  </div>
                )}
                <div
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    gap: 10,
                    marginTop: 10,
                    color: '#94a3b8',
                    fontSize: 12,
                  }}
                >
                  <span>货品: {dept.qty?.toLocaleString()}</span>
                  <span>客单价: ¥{dept.qty > 0 ? (dept.sales / dept.qty).toFixed(2) : '-'}</span>
                </div>
              </Card>
            </Col>
          );
        })}
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={15}>
          <Card
            title={
              <span>
                {activeCfg.label} 每日销售趋势
                {isShortRange && (
                  <span style={{ fontSize: 12, color: '#94a3b8', fontWeight: 400, marginLeft: 8 }}>
                    蓝色区域为选中日期
                  </span>
                )}
              </span>
            }
          >
            <ReactECharts option={trendOption} lazyUpdate={true} style={{ height: 300 }} />
          </Card>
          <Card title={`${activeCfg.label} 客单价趋势`} style={{ marginTop: 16 }}>
            <ReactECharts option={avgPriceOption} lazyUpdate={true} style={{ height: 200 }} />
          </Card>
        </Col>
        <Col xs={24} lg={9}>
          <Card title="各部门销售占比">
            <ReactECharts option={pieOption} lazyUpdate={true} style={{ height: 270 }} />
          </Card>
          {grades.length > 0 && (
            <Card title="产品定位分布" style={{ marginTop: 16 }}>
              <ReactECharts option={gradePieOption} lazyUpdate={true} style={{ height: 230 }} />
            </Card>
          )}
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={10}>
          <Card title="店铺销售额排行 TOP15" style={{ height: '100%' }}>
            <ReactECharts option={shopBarOption} lazyUpdate={true} style={{ height: 540 }} />
          </Card>
        </Col>
        <Col xs={24} lg={14}>
          <Card title="商品销售排行 TOP15（点击展开查看渠道分布）">
            <Table
              dataSource={indexedTopGoods}
              columns={goodsColumns}
              rowKey="goodsNo"
              pagination={false}
              size="small"
              scroll={{ y: 500 }}
              expandable={{
                expandedRowRender: (record: any) => {
                  const channels: any[] = goodsChannels[record.goodsNo] || [];
                  return <GoodsChannelExpand channels={channels} mode="department" />;
                },
                rowExpandable: (record: any) => (goodsChannels[record.goodsNo] || []).length > 0,
              }}
            />
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default OverviewPage;
