import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Row, Col, Card, Statistic, Table, Progress } from 'antd';
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
  const regionTargets: Record<string, number> = data.regionTargets || {};
  const totalSales = shops.reduce((s: number, d: any) => s + d.sales, 0);
  const totalQty = shops.reduce((s: number, d: any) => s + d.qty, 0);
  const avgOrderValue = totalQty > 0 ? totalSales / totalQty : 0;
  const baseOpt = getBaseOption();

  const indexedShops = shops.map((g: any, i: number) => ({ ...g, _rank: i + 1 }));
  const columns = [
    { title: '排名', dataIndex: '_rank', key: 'rank', width: 50, align: 'center' as const,
      render: (rank: number) => <span style={{ color: rank <= 3 ? color : '#94a3b8', fontWeight: rank <= 3 ? 700 : 400 }}>{rank}</span> },
    { title: dept === 'offline' ? '大区' : '店铺', dataIndex: 'shopName', key: 'shopName', ellipsis: true, width: dept === 'offline' ? 100 : 300 },
    { title: '销售额', dataIndex: 'sales', key: 'sales', width: dept === 'offline' ? 200 : 130, sorter: (a: any, b: any) => a.sales - b.sales,
      render: (v: number, record: any) => {
        const target = regionTargets[record.shopName];
        if (dept === 'offline' && target > 0) {
          const pct = Math.min(v / target * 100, 100);
          return (
            <div>
              <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, marginBottom: 2 }}>
                <span style={{ fontWeight: 600 }}>¥{v?.toLocaleString()}</span>
                <span style={{ color: pct >= 100 ? '#16a34a' : '#94a3b8' }}>{pct.toFixed(1)}%</span>
              </div>
              <Progress percent={pct} showInfo={false} strokeColor={color} trailColor="#e2e8f0" size={['100%', 6]} />
              <div style={{ fontSize: 11, color: '#94a3b8', marginTop: 1 }}>目标¥{target.toLocaleString()}</div>
            </div>
          );
        }
        return `¥${v?.toLocaleString()}`;
      } },
    { title: '货品数', dataIndex: 'qty', key: 'qty', width: 80, sorter: (a: any, b: any) => a.qty - b.qty,
      render: (v: number) => v?.toLocaleString() },
    { title: '客单价', key: 'avgPrice', width: 90, sorter: (a: any, b: any) => (a.qty > 0 ? a.sales/a.qty : 0) - (b.qty > 0 ? b.sales/b.qty : 0),
      render: (_: any, r: any) => r.qty > 0 ? `¥${(r.sales / r.qty).toFixed(0)}` : '-' },
    { title: '占比', key: 'pct', width: 70,
      render: (_: any, r: any) => totalSales > 0 ? `${(r.sales / totalSales * 100).toFixed(1)}%` : '-' },
  ];

  const sortedBySales = [...shops].sort((a: any, b: any) => (b.sales || 0) - (a.sales || 0));
  const pieTopCount = 10;
  const topPieShops = sortedBySales.slice(0, pieTopCount);
  const otherSales = sortedBySales.slice(pieTopCount).reduce((sum: number, s: any) => sum + (s.sales || 0), 0);
  const pieData = [
    ...topPieShops.map((s: any) => ({ value: s.sales, name: s.shopName })),
    ...(otherSales > 0 ? [{ value: otherSales, name: '其他' }] : []),
  ];
  const salesPieOption = {
    ...pieStyle,
    color: CHART_COLORS,
    tooltip: { ...pieStyle.tooltip, trigger: 'item' as const, formatter: (p: any) => `${p.name}<br/>¥${p.value?.toLocaleString()}（${p.percent}%）` },
    legend: { orient: 'horizontal' as const, left: 'center', bottom: 0, top: 'auto', type: 'scroll' as const, itemGap: 10, textStyle: { fontSize: 10 } },
    series: [{
      type: 'pie', radius: ['28%', '55%'], center: ['50%', '42%'],
      label: { show: true, position: 'outside' as const, formatter: (p: any) => `${p.name}\n{v|${p.percent}%}`, rich: { v: { fontSize: 10, color: '#999', lineHeight: 16 } }, fontSize: 10, color: '#333' },
      labelLayout: { hideOverlap: true },
      labelLine: { length: 10, length2: 12, lineStyle: { color: '#e2e8f0' } },
      itemStyle: { borderRadius: 4, borderColor: '#fff', borderWidth: 2 },
      data: pieData.map((item: any) => {
        const share = totalSales > 0 ? item.value / totalSales : 0;
        const showLabel = item.name === '其他' || share >= 0.04;
        return { ...item, label: { show: showLabel }, labelLine: { show: showLabel } };
      }),
    }],
  };

  const avgPriceData = shops
    .map((s: any) => ({ name: s.shopName, avgPrice: s.qty > 0 ? +(s.sales / s.qty).toFixed(2) : 0 }))
    .sort((a: any, b: any) => b.avgPrice - a.avgPrice)
    .slice(0, 20);
  const avgPriceOption = {
    ...baseOpt,
    tooltip: { ...baseOpt.tooltip, trigger: 'axis' as const, formatter: (p: any) => `${p[0].name}<br/>客单价: ¥${p[0].value}` },
    grid: { left: '50%', right: 55, top: 5, bottom: 10 },
    xAxis: { ...baseOpt.xAxis, type: 'value' as const, show: false },
    yAxis: { type: 'category' as const, data: avgPriceData.map((d: any) => d.name).reverse(), axisLabel: { ...baseOpt.yAxis.axisLabel, fontSize: 11, width: 160, overflow: 'truncate' as const } },
    series: [{ type: 'bar', barWidth: 10, data: avgPriceData.map((d: any) => d.avgPrice).reverse(), ...barItemStyle('#faad14'), label: { show: true, position: 'right', fontSize: 10, formatter: '¥{c}' } }],
  };

  const platformSales = data.platformSales || [];
  const platformPieOption = {
    ...pieStyle, color: CHART_COLORS,
    tooltip: { ...pieStyle.tooltip, trigger: 'item' as const, formatter: (p: any) => `${p.name}<br/>¥${p.value?.toLocaleString()}（${p.percent}%）` },
    legend: { orient: 'horizontal' as const, left: 'center', bottom: 0, type: 'scroll' as const, itemGap: 10, textStyle: { fontSize: 10 } },
    series: [{ type: 'pie', radius: ['28%', '55%'], center: ['50%', '42%'], label: { show: true, formatter: '{b}\n{d}%', fontSize: 10, color: '#333' }, labelLayout: { hideOverlap: true }, labelLine: { length: 8, length2: 12, lineStyle: { color: '#e2e8f0' } }, itemStyle: { borderRadius: 4, borderColor: '#fff', borderWidth: 2 }, data: platformSales.map((p: any) => ({ value: p.sales, name: p.platform })) }],
  };

  const gradePlatSales: any[] = data.gradePlatSales || [];
  const gradeOrder = ['S', 'A', 'B', 'C', 'D', '未设置'];
  const dimensionLabel = (dept === 'offline' || dept === 'distribution') ? '渠道' : '平台';
  const gradePlatMap = new Map<string, { total: number; platforms: { platform: string; sales: number }[] }>();
  gradePlatSales.forEach((item: any) => {
    const entry = gradePlatMap.get(item.grade) || { total: 0, platforms: [] };
    entry.total += item.sales;
    entry.platforms.push({ platform: item.platform, sales: item.sales });
    gradePlatMap.set(item.grade, entry);
  });
  gradePlatMap.forEach(entry => entry.platforms.sort((a, b) => b.sales - a.sales));
  const gradePieData = gradeOrder.filter(g => gradePlatMap.has(g)).map(g => ({
    value: gradePlatMap.get(g)!.total, name: g + '品', _grade: g, _platforms: gradePlatMap.get(g)!.platforms, itemStyle: { color: GRADE_COLORS[g] },
  }));
  const gradeDonutOption = (dept === 'ecommerce' || dept === 'social' || dept === 'offline' || dept === 'distribution') && totalSales > 0 && gradePieData.length > 0 ? {
    ...pieStyle,
    tooltip: { ...pieStyle.tooltip, trigger: 'item' as const, formatter: (p: any) => {
      const pct = (p.value / totalSales * 100).toFixed(1);
      const platforms: { platform: string; sales: number }[] = p.data?._platforms || [];
      let html = `<b>${p.name}</b>：¥${p.value?.toLocaleString()}（${pct}%）`;
      if (platforms.length > 0) { html += '<br/><div style="margin-top:6px;border-top:1px solid #eee;padding-top:6px">'; platforms.forEach(pt => { html += `${pt.platform}：¥${pt.sales.toLocaleString()}（${(pt.sales/totalSales*100).toFixed(1)}%）<br/>`; }); html += '</div>'; }
      return html;
    }},
    legend: { orient: 'horizontal' as const, left: 'center', bottom: 0, type: 'scroll' as const, itemGap: 10, textStyle: { fontSize: 10 } },
    series: [{ name: '产品定位', type: 'pie', radius: ['28%', '55%'], center: ['50%', '42%'],
      label: { show: true, position: 'outside' as const, formatter: (p: any) => `${p.name}\n{v|${(p.value/totalSales*100).toFixed(1)}%}`, rich: { v: { fontSize: 10, color: '#999', lineHeight: 16 } }, fontSize: 10, color: '#333' },
      labelLayout: { hideOverlap: true }, labelLine: { length: 12, length2: 14, lineStyle: { color: '#cbd5e1' } }, itemStyle: { borderRadius: 4, borderColor: '#fff', borderWidth: 2 }, data: gradePieData }],
  } : null;

  const totalTarget = Object.values(regionTargets).reduce((s, v) => s + v, 0);
  const totalPct = totalTarget > 0 ? Math.min(totalSales / totalTarget * 100, 100) : 0;

  const statCards = [
    { title: '总销售额', value: totalSales, precision: 2, prefix: '¥', accentColor: color },
    { title: '总货品数', value: totalQty, precision: 0, accentColor: '#10b981' },
    { title: '综合客单价', value: avgOrderValue, precision: 2, prefix: '¥', accentColor: '#7c3aed' },
    { title: dept === 'offline' ? '大区数量' : '店铺数量', value: shops.length, precision: 0, suffix: dept === 'offline' ? '个' : '家', accentColor: '#f59e0b' },
  ];

  // 左侧图表高度（offline 紧凑）
  const isOffline = dept === 'offline';
  const pieH = isOffline ? 200 : 280;
  const avgPriceH = isOffline ? Math.max(160, avgPriceData.length * 22) : Math.max(220, avgPriceData.length * 22);

  return (
    <div>
      <DateFilter start={startDate} end={endDate} onChange={(s, e) => { setStartDate(s); setEndDate(e); }} />

      {/* ── KPI 行 ── */}
      {isOffline ? (
        <Row gutter={[12, 12]} align="stretch">
          <Col xs={24} lg={7}>
            <Card className="bi-stat-card" style={{ ['--accent-color' as any]: color, height: '100%' }}
              styles={{ body: { height: '100%', display: 'flex', flexDirection: 'column' } }}>
              <div style={{ fontSize: 12, color: '#64748b', marginBottom: 4 }}>总销售额</div>
              <div style={{ fontSize: 26, fontWeight: 700, color, fontVariantNumeric: 'tabular-nums', lineHeight: 1.2 }}>
                ¥{totalSales.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
              </div>
              <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 3 }}>
                {totalSales >= 10000 ? `≈ ${(totalSales / 10000).toFixed(1)}万` : ''}
              </div>
              {totalTarget > 0 && (
                <div style={{ marginTop: 'auto', paddingTop: 8 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, marginBottom: 3 }}>
                    <span style={{ color: '#64748b' }}>目标完成</span>
                    <span style={{ fontWeight: 600, color: totalPct >= 100 ? '#16a34a' : color }}>{totalPct.toFixed(1)}%</span>
                  </div>
                  <Progress percent={totalPct} showInfo={false} strokeColor={color} trailColor="#e2e8f0" size={['100%', 7]} />
                  <div style={{ fontSize: 11, color: '#94a3b8', marginTop: 3 }}>目标 ¥{totalTarget.toLocaleString()}</div>
                </div>
              )}
            </Card>
          </Col>
          <Col xs={24} lg={17}>
            <Row gutter={[12, 12]} style={{ height: '100%' }}>
              {[
                { title: '总货品数', value: totalQty, suffix: '', prefix: '', precision: 0, color: '#10b981' },
                { title: '综合客单价', value: avgOrderValue, suffix: '', prefix: '¥', precision: 2, color: '#7c3aed' },
                { title: '大区数量', value: shops.length, suffix: '个', prefix: '', precision: 0, color: '#f59e0b' },
              ].map(s => (
                <Col span={8} key={s.title}>
                  <Card className="bi-stat-card" size="small" style={{ ['--accent-color' as any]: s.color, height: '100%' }}
                    styles={{ body: { display: 'flex', flexDirection: 'column', justifyContent: 'center', height: '100%' } }}>
                    <div style={{ fontSize: 12, color: '#64748b', marginBottom: 4 }}>{s.title}</div>
                    <div style={{ fontSize: 22, fontWeight: 700, color: s.color, fontVariantNumeric: 'tabular-nums' }}>
                      {s.prefix}{s.value.toLocaleString(undefined, { minimumFractionDigits: s.precision, maximumFractionDigits: s.precision })}{s.suffix}
                    </div>
                    {s.value >= 10000 && s.prefix !== '¥' && (
                      <div style={{ fontSize: 11, color: '#94a3b8', marginTop: 2 }}>≈ {(s.value / 10000).toFixed(1)}万</div>
                    )}
                  </Card>
                </Col>
              ))}
            </Row>
          </Col>
        </Row>
      ) : (
        <Row gutter={[16, 16]} align="stretch">
          {statCards.map((card) => {
            const hint = card.value >= 10000 ? (card.value >= 100000000 ? `≈ ${(card.value / 100000000).toFixed(2)}亿` : `≈ ${(card.value / 10000).toFixed(1)}万`) : '';
            return (
              <Col xs={24} sm={6} key={card.title}>
                <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                  <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
                  <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, minHeight: '1.4em' }}>{hint || ' '}</div>
                </Card>
              </Col>
            );
          })}
        </Row>
      )}

      {/* ── 主体：左侧图表 + 右侧排名表 ── */}
      <Row gutter={[12, 12]} style={{ marginTop: 12 }}>
        <Col xs={24} lg={isOffline ? 8 : 8}>
          <Card title="销售额占比" size="small" style={{ marginBottom: 12 }}>
            <ReactECharts option={salesPieOption} lazyUpdate={true} style={{ height: pieH }} />
          </Card>
          {gradeDonutOption ? (
            <Card title={`产品定位 × ${dimensionLabel}分布`} size="small" style={{ marginBottom: 12 }}>
              <ReactECharts option={gradeDonutOption} lazyUpdate={true} style={{ height: pieH }} />
            </Card>
          ) : platformSales.length > 0 ? (
            <Card title="平台销售额分布" size="small" style={{ marginBottom: 12 }}>
              <ReactECharts option={platformPieOption} lazyUpdate={true} style={{ height: pieH }} />
            </Card>
          ) : null}
          <Card title="客单价对比" size="small">
            <ReactECharts option={avgPriceOption} lazyUpdate={true} style={{ height: avgPriceH }} />
          </Card>
        </Col>

        <Col xs={24} lg={16}>
          <Card className="bi-table-card" size="small"
            title={`${isOffline ? '大区' : '店铺'}排名（共${shops.length}${isOffline ? '个' : '家'}）`}>
            <Table
              dataSource={indexedShops}
              columns={columns}
              rowKey="shopName"
              pagination={false}
              size="small"
              scroll={isOffline ? undefined : { y: 500 }}
            />
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default StorePreview;
