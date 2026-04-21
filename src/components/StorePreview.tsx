import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Row, Col, Card, Statistic, Table } from 'antd';
import ReactECharts from './Chart';
import DateFilter from './DateFilter';
import PageLoading from './PageLoading';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../config';
import { barItemStyle, CHART_COLORS, GRADE_COLORS, getBaseOption, pieStyle } from '../chartTheme';

interface Props {
  dept: string;
  title: string;
  color: string;
}

const StorePreview: React.FC<Props> = ({ dept, title, color  }) => {
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
    fetch(`${API_BASE}/api/department?dept=${dept}&start=${s}&end=${e}`, { signal: ctrl.signal })
      .then(res => res.json())
      .then(res => { setData(res.data); setLoading(false); })
      .catch((e: any) => { if (e?.name !== 'AbortError') setLoading(false); });
  }, [dept]);

  useEffect(() => { fetchData(startDate, endDate); }, [fetchData, startDate, endDate]);

  if (loading) return <PageLoading />;
  if (!data) return <div>加载失败</div>;

  const shops = data.shops || [];
  const totalSales = shops.reduce((s: number, d: any) => s + d.sales, 0);
  const totalQty = shops.reduce((s: number, d: any) => s + d.qty, 0);
  const avgOrderValue = totalQty > 0 ? totalSales / totalQty : 0;
  const baseOpt = getBaseOption();

  // 店铺排名表
  const indexedShops = shops.map((g: any, i: number) => ({ ...g, _rank: i + 1 }));
  const columns = [
    { title: '排名', dataIndex: '_rank', key: 'rank', width: 60, align: 'center' as const,
      render: (rank: number) => <span style={{ color: rank <= 3 ? color : '#94a3b8', fontWeight: rank <= 3 ? 700 : 400 }}>{rank}</span> },
    { title: '店铺名称', dataIndex: 'shopName', key: 'shopName', ellipsis: true, width: 300 },
    { title: '销售额', dataIndex: 'sales', key: 'sales', width: 130, sorter: (a: any, b: any) => a.sales - b.sales,
      render: (v: number) => `¥${v?.toLocaleString()}` },
    { title: '货品数', dataIndex: 'qty', key: 'qty', width: 90, sorter: (a: any, b: any) => a.qty - b.qty,
      render: (v: number) => v?.toLocaleString() },
    { title: '客单价', key: 'avgPrice', width: 110, sorter: (a: any, b: any) => (a.qty > 0 ? a.sales/a.qty : 0) - (b.qty > 0 ? b.sales/b.qty : 0),
      render: (_: any, r: any) => r.qty > 0 ? `¥${(r.sales / r.qty).toFixed(2)}` : '-' },
    { title: '销售占比', key: 'pct', width: 100,
      render: (_: any, r: any) => totalSales > 0 ? `${(r.sales / totalSales * 100).toFixed(1)}%` : '-' },
  ];

  // 销售额占比 — 店铺过多时显示 TOP10 + 其他，避免标签挤压
  const sortedBySales = [...shops].sort((a: any, b: any) => (b.sales || 0) - (a.sales || 0));
  const pieTopCount = 10;
  const topPieShops = sortedBySales.slice(0, pieTopCount);
  const otherSales = sortedBySales.slice(pieTopCount).reduce((sum: number, s: any) => sum + (s.sales || 0), 0);
  const pieData = [
    ...topPieShops.map((s: any) => ({ value: s.sales, name: s.shopName })),
    ...(otherSales > 0 ? [{ value: otherSales, name: '其他店铺' }] : []),
  ];
  const salesPieOption = {
    ...pieStyle,
    color: CHART_COLORS,
    tooltip: { ...pieStyle.tooltip, trigger: 'item' as const, formatter: (p: any) => `${p.name}<br/>¥${p.value?.toLocaleString()}（${p.percent}%）` },
    legend: {
      ...pieStyle.legend,
      orient: 'horizontal' as const,
      left: 'center',
      bottom: 0,
      top: 'auto',
      type: 'scroll' as const,
      itemGap: 14,
      textStyle: { fontSize: 11 },
    },
    series: [{
      type: 'pie',
      radius: ['30%', '58%'],
      center: ['50%', '42%'],
      label: {
        show: true,
        position: 'outside' as const,
        formatter: (p: any) => `${p.name}\n{value|${p.percent}%}`,
        rich: {
          value: { fontSize: 11, color: '#999', lineHeight: 18 },
        },
        fontSize: 11,
        color: '#333',
      },
      labelLayout: { hideOverlap: true },
      labelLine: { length: 12, length2: 16, lineStyle: { color: '#e2e8f0' } },
      itemStyle: { borderRadius: 4, borderColor: '#fff', borderWidth: 2 },
      data: pieData.map((item: any) => {
        const share = totalSales > 0 ? item.value / totalSales : 0;
        const showLabel = item.name === '其他店铺' || share >= 0.04;
        return {
          ...item,
          label: { show: showLabel },
          labelLine: { show: showLabel },
        };
      }),
    }],
  };

  // 客单价对比 — 横向柱状图（按客单价排序，值标在柱子右边）
  const avgPriceData = shops
    .map((s: any) => ({ name: s.shopName, avgPrice: s.qty > 0 ? +(s.sales / s.qty).toFixed(2) : 0 }))
    .sort((a: any, b: any) => b.avgPrice - a.avgPrice)
    .slice(0, 20);
  const avgPriceOption = {
    ...baseOpt,
    tooltip: { ...baseOpt.tooltip, trigger: 'axis' as const, formatter: (p: any) => `${p[0].name}<br/>客单价: ¥${p[0].value}` },
    grid: { left: '50%', right: 50, top: 10, bottom: 20 },
    xAxis: { ...baseOpt.xAxis, type: 'value' as const, show: false },
    yAxis: { type: 'category' as const,
      data: avgPriceData.map((d: any) => d.name).reverse(),
      axisLabel: { ...baseOpt.yAxis.axisLabel, fontSize: 11, width: 180, overflow: 'truncate' as const },
    },
    series: [{
      type: 'bar', barWidth: 12,
      data: avgPriceData.map((d: any) => d.avgPrice).reverse(),
      ...barItemStyle('#faad14'),
      label: { show: true, position: 'right', fontSize: 11, formatter: '¥{c}' },
    }],
  };

  // 平台销售额分布饼图
  const platformSales = data.platformSales || [];
  const platformPieOption = {
    ...pieStyle,
    color: CHART_COLORS,
    tooltip: { ...pieStyle.tooltip, trigger: 'item' as const, formatter: (p: any) => `${p.name}<br/>¥${p.value?.toLocaleString()}（${p.percent}%）` },
    legend: {
      ...pieStyle.legend,
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
      center: ['50%', '42%'],
      label: {
        show: true,
        formatter: '{b}\n{d}%',
        fontSize: 11,
        color: '#333',
      },
      labelLayout: { hideOverlap: true },
      labelLine: { length: 10, length2: 14, lineStyle: { color: '#e2e8f0' } },
      itemStyle: { borderRadius: 4, borderColor: '#fff', borderWidth: 2 },
      data: platformSales.map((p: any) => ({ value: p.sales, name: p.platform })),
    }],
  };

  // 产品定位分布 — 单圈饼图，hover展示平台明细（仅电商部门）
  const gradePlatSales: any[] = data.gradePlatSales || [];
  const gradeOrder = ['S', 'A', 'B', 'C', 'D', '未设置'];

  const gradePlatMap = new Map<string, { total: number; platforms: { platform: string; sales: number }[] }>();
  gradePlatSales.forEach((item: any) => {
    const entry = gradePlatMap.get(item.grade) || { total: 0, platforms: [] };
    entry.total += item.sales;
    entry.platforms.push({ platform: item.platform, sales: item.sales });
    gradePlatMap.set(item.grade, entry);
  });
  gradePlatMap.forEach(entry => entry.platforms.sort((a, b) => b.sales - a.sales));

  const gradePieData = gradeOrder
    .filter(g => gradePlatMap.has(g))
    .map(g => ({
      value: gradePlatMap.get(g)!.total,
      name: g + '品',
      _grade: g,
      _platforms: gradePlatMap.get(g)!.platforms,
      itemStyle: { color: GRADE_COLORS[g] },
    }));

  const gradeDonutOption = dept === 'ecommerce' && totalSales > 0 && gradePieData.length > 0 ? {
    ...pieStyle,
    tooltip: {
      ...pieStyle.tooltip,
      trigger: 'item' as const,
      formatter: (p: any) => {
        const pct = (p.value / totalSales * 100).toFixed(1);
        const platforms: { platform: string; sales: number }[] = p.data?._platforms || [];
        let html = `<b>${p.name}</b>：¥${p.value?.toLocaleString()}（${pct}%）`;
        if (platforms.length > 0) {
          html += '<br/><div style="margin-top:6px;border-top:1px solid #eee;padding-top:6px">';
          platforms.forEach(pt => {
            const ptPct = (pt.sales / totalSales * 100).toFixed(1);
            html += `${pt.platform}：¥${pt.sales.toLocaleString()}（${ptPct}%）<br/>`;
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
      name: '产品定位',
      type: 'pie',
      radius: ['30%', '60%'],
      center: ['50%', '45%'],
      label: {
        show: true,
        position: 'outside' as const,
        formatter: (p: any) => {
          const pct = (p.value / totalSales * 100).toFixed(1);
          return `${p.name}\n{value|${pct}%}`;
        },
        rich: { value: { fontSize: 11, color: '#999', lineHeight: 18 } },
        fontSize: 12,
        color: '#333',
      },
      labelLayout: { hideOverlap: true },
      labelLine: { length: 15, length2: 18, lineStyle: { color: '#cbd5e1' } },
      itemStyle: { borderRadius: 4, borderColor: '#fff', borderWidth: 2 },
      data: gradePieData,
    }],
  } : null;

  const statCards = [
    { title: '总销售额', value: totalSales, precision: 2, prefix: '¥', accentColor: color },
    { title: '总货品数', value: totalQty, precision: 0, accentColor: '#10b981' },
    { title: '综合客单价', value: avgOrderValue, precision: 2, prefix: '¥', accentColor: '#7c3aed' },
    { title: '店铺数量', value: shops.length, precision: 0, suffix: '家', accentColor: '#f59e0b' },
  ];

  return (
    <div>
      <DateFilter start={startDate} end={endDate} onChange={(s, e) => { setStartDate(s); setEndDate(e); }} />
      <Row gutter={[16, 16]}>
        {statCards.map((card) => (
          <Col xs={24} sm={6} key={card.title}>
            <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
              <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
            </Card>
          </Col>
        ))}
      </Row>
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={gradeDonutOption || platformSales.length > 0 ? 12 : 24}>
          <Card title={shops.length > pieTopCount ? `店铺销售额占比（TOP${pieTopCount}+其他）` : '店铺销售额占比'}>
            <ReactECharts option={salesPieOption} lazyUpdate={true} style={{ height: 380 }} />
          </Card>
        </Col>
        {gradeDonutOption ? (
          <Col xs={24} lg={12}>
            <Card title="产品定位 × 平台分布（hover查看平台明细）">
              <ReactECharts option={gradeDonutOption} lazyUpdate={true} style={{ height: 380 }} />
            </Card>
          </Col>
        ) : platformSales.length > 0 ? (
          <Col xs={24} lg={12}>
            <Card title="平台销售额分布">
              <ReactECharts option={platformPieOption} lazyUpdate={true} style={{ height: 380 }} />
            </Card>
          </Col>
        ) : null}
      </Row>
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={8}>
          <Card title="店铺客单价对比">
            <ReactECharts option={avgPriceOption} lazyUpdate={true} style={{ height: Math.max(400, Math.min(avgPriceData.length, 20) * 28) }} />
          </Card>
        </Col>
        <Col xs={24} lg={16}>
          <Card className="bi-table-card" title={`店铺排名（共${shops.length}家）`}>
            <Table dataSource={indexedShops} columns={columns} rowKey="shopName" pagination={false} size="small" scroll={{ y: Math.max(400, Math.min(avgPriceData.length, 20) * 28 - 8) }} />
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default StorePreview;
