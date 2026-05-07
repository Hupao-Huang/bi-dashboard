import React, { useEffect, useState, useCallback } from 'react';
import { Row, Col, Card, Select, Segmented, Button, Input, Statistic, Empty, Spin, Progress } from 'antd';
import {
  CarOutlined, EnvironmentOutlined, AppstoreOutlined, GlobalOutlined,
  ShopOutlined, SearchOutlined, CalendarOutlined, ClearOutlined, InfoCircleOutlined,
} from '@ant-design/icons';
import * as echarts from 'echarts/core';
import { MapChart, BarChart, LinesChart, EffectScatterChart } from 'echarts/charts';
import {
  TitleComponent, TooltipComponent, GridComponent,
  VisualMapComponent, GeoComponent, DataZoomComponent, LegendComponent,
} from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';
import ReactECharts from 'echarts-for-react/lib/core';
import PageLoading from '../../components/PageLoading';
import { API_BASE } from '../../config';

echarts.use([
  MapChart, BarChart, LinesChart, EffectScatterChart,
  TitleComponent, TooltipComponent, GridComponent,
  VisualMapComponent, GeoComponent, DataZoomComponent, LegendComponent,
  CanvasRenderer,
]);

// 7 仓真实经纬度（与 planWarehouses 一致）
const warehouseCenter: Record<string, [number, number]> = {
  '南京委外成品仓-公司仓-委外':  [118.7969, 32.0603],   // 南京
  '天津委外仓-公司仓-外仓':       [117.1907, 39.1255],   // 天津
  '西安仓库成品-公司仓-外仓':     [108.9402, 34.3416],   // 西安
  '松鲜鲜&大地密码云仓':          [113.6645, 34.7570],   // 郑州（河南，跑哥确认）
  '长沙委外成品仓-公司仓-外仓':   [112.9388, 28.2282],   // 长沙
  '安徽郎溪成品-公司仓-自营':     [119.1804, 31.1265],   // 郎溪（宣城下辖县）
  '南京分销虚拟仓-公司仓-外仓':   [118.7969, 32.0603],   // 南京（同前）
};

// 31 省+港澳台的省会/中心点经纬度（用于飞线终点）
const provinceCenter: Record<string, [number, number]> = {
  '北京市': [116.4074, 39.9042], '上海市': [121.4737, 31.2304], '天津市': [117.1907, 39.1255], '重庆市': [106.5516, 29.5630],
  '广东省': [113.2806, 23.1252], '广西壮族自治区': [108.3669, 22.8170], '宁夏回族自治区': [106.2587, 38.4729],
  '内蒙古自治区': [111.7517, 40.8419], '新疆维吾尔自治区': [87.6177, 43.7928], '西藏自治区': [91.1322, 29.6604],
  '香港特别行政区': [114.1694, 22.3193], '澳门特别行政区': [113.5491, 22.1987], '台湾省': [121.5645, 25.0330],
  '江苏省': [118.7969, 32.0603], '浙江省': [120.1551, 30.2741], '山东省': [117.0009, 36.6758], '福建省': [119.2965, 26.0745],
  '湖南省': [112.9388, 28.2282], '湖北省': [114.3055, 30.5928], '河南省': [113.6645, 34.7570], '河北省': [114.5149, 38.0428],
  '山西省': [112.5491, 37.8577], '陕西省': [108.9402, 34.3416], '四川省': [104.0668, 30.5728], '安徽省': [117.2830, 31.8612],
  '江西省': [115.8580, 28.6829], '辽宁省': [123.4291, 41.8057], '吉林省': [125.3245, 43.8868], '黑龙江省': [126.6424, 45.7563],
  '云南省': [102.7124, 25.0406], '贵州省': [106.7135, 26.5783], '甘肃省': [103.8235, 36.0581], '青海省': [101.7782, 36.6173],
  '海南省': [110.3312, 20.0311],
};

// 仓库简称（去掉 -公司仓-X 后缀，飞线图标签用）
const shortenWarehouse = (n: string) => n.replace(/-公司仓-(委外|外仓|自营)$/, '').replace('松鲜鲜&大地密码云仓', '河南云仓');

type Metric = 'orders' | 'packages';
const metricLabel: Record<Metric, string> = { orders: '订单数', packages: '包裹数' };
const metricUnit: Record<Metric, string> = { orders: '单', packages: '包' };

interface MM { name: string; orders: number; packages: number; }
type Pair = { orders: number; packages: number };
const pick = (r: Pair, m: Metric): number => (m === 'packages' ? r.packages : r.orders);

interface Overview {
  ym: string;
  kpi: { orders: number; packages: number; provinceCnt: number; warehouseCnt: number };
  provinces: MM[];
  warehouses: MM[];
  shops: string[];
  ymList: string[];
}
interface MatrixCell { warehouse: string; province: string; orders: number; packages: number; }
interface MatrixData { ym: string; cells: MatrixCell[]; }

const fmt = (v: number) => {
  if (v >= 10000) return `${(v / 10000).toFixed(1)} 万`;
  return Math.round(v).toLocaleString();
};

// fmtFull 给 tooltip 用: 精确千分位 + 简洁万 (例: "171,644 单（17.2 万）")
const fmtFull = (v: number, unit: string) => {
  const exact = Math.round(v).toLocaleString();
  if (v >= 10000) return `${exact} ${unit}（${(v / 10000).toFixed(1)} 万）`;
  return `${exact} ${unit}`;
};

let mapRegistered = false;

const LogisticsAnalysis: React.FC = () => {
  // === 筛选 ===
  const [ym, setYm] = useState<string>('');
  const [metric, setMetric] = useState<Metric>('orders');
  const [shop, setShop] = useState<string>('');
  const [warehouse, setWarehouse] = useState<string>('');
  const [province, setProvince] = useState<string>('');
  const [skuKw, setSkuKw] = useState<string>('');

  // 飞线图侧栏: 仓库聚焦 (空 = 全部)
  const [focusedWh, setFocusedWh] = useState<string>('');

  // === 数据 ===
  const [overview, setOverview] = useState<Overview | null>(null);
  const [matrix, setMatrix] = useState<MatrixData | null>(null);
  const [loading, setLoading] = useState(true);
  const [mapReady, setMapReady] = useState(mapRegistered);

  // 注册中国地图（一次性）
  useEffect(() => {
    if (mapRegistered) { setMapReady(true); return; }
    fetch('/china.json').then(r => r.json()).then((geo) => {
      // datav GeoJSON 用 properties.center, ECharts label 默认读 properties.cp
      // 不注入 cp 则 map series 的 label 完全不渲染 (跑哥看不到省名的根因)
      geo.features?.forEach((f: any) => {
        if (f.properties?.center && !f.properties.cp) {
          f.properties.cp = f.properties.center;
        }
      });
      echarts.registerMap('china', geo);
      mapRegistered = true;
      setMapReady(true);
    }).catch(() => setMapReady(false));
  }, []);

  // 拉总览 + 矩阵 (一次返回 3 指标全量, metric 切换不重查)
  const fetchAll = useCallback(() => {
    setLoading(true);
    const params = new URLSearchParams({
      ...(ym ? { ym } : {}),
      ...(shop ? { shop } : {}),
      ...(warehouse ? { warehouse } : {}),
      ...(province ? { province } : {}),
      ...(skuKw ? { sku_keyword: skuKw } : {}),
    }).toString();

    Promise.all([
      fetch(`${API_BASE}/api/warehouse-flow/overview?${params}`, { credentials: 'include' }).then(r => r.json()),
      fetch(`${API_BASE}/api/warehouse-flow/matrix?${params}`, { credentials: 'include' }).then(r => r.json()),
    ]).then(([ov, mx]) => {
      setOverview(ov.data || ov);
      setMatrix(mx.data || mx);
      setLoading(false);
    }).catch(() => setLoading(false));
  }, [ym, shop, warehouse, province, skuKw]);

  useEffect(() => { fetchAll(); }, [fetchAll]);

  if (loading) return <PageLoading />;
  if (!overview) return <Empty description="暂无数据" />;

  const k = overview.kpi;

  // 省份按当前 metric 排序 (本地)
  const provincesSorted = [...overview.provinces].sort((a, b) => pick(b, metric) - pick(a, metric));

  // 飞线数据 (来自 matrix.cells, 每条 = 仓 → 省, 用 metric 加权线宽)
  // focusedWh 非空时仅显示该仓飞线
  const allCells = (matrix?.cells || []).filter(c =>
    warehouseCenter[c.warehouse] && provinceCenter[c.province] && pick(c, metric) > 0
  );
  const cellsForMap = focusedWh ? allCells.filter(c => c.warehouse === focusedWh) : allCells;
  const flowLines = cellsForMap.map(c => ({
    coords: [warehouseCenter[c.warehouse], provinceCenter[c.province]],
    value: pick(c, metric),
    warehouse: shortenWarehouse(c.warehouse),
    province: c.province,
  }));
  const maxFlow = Math.max(1, ...flowLines.map(l => l.value));

  // 选中仓的省份占比榜 (focusedWh 空时 = 7 仓汇总)
  const focusedTotal = cellsForMap.reduce((s, c) => s + pick(c, metric), 0);
  const focusedProvBreakdown = (() => {
    const m = new Map<string, number>();
    for (const c of cellsForMap) m.set(c.province, (m.get(c.province) || 0) + pick(c, metric));
    return Array.from(m.entries())
      .map(([name, value]) => ({ name, value, ratio: focusedTotal > 0 ? value / focusedTotal : 0 }))
      .sort((a, b) => b.value - a.value);
  })();

  // 地图 option (省份热力 + 仓库点 + 仓→省飞线)
  const mapOption = {
    tooltip: {
      trigger: 'item',
      formatter: (p: any) => {
        const u = metricUnit[metric];
        if (p.seriesType === 'lines') {
          return `<b>${p.data.warehouse} → ${p.data.province}</b><br/>${metricLabel[metric]}: ${fmtFull(p.data.value, u)}`;
        }
        if (p.seriesType === 'effectScatter') {
          return `<b>${p.data.name}</b><br/>${metricLabel[metric]}: ${fmtFull(p.value[2], u)}`;
        }
        return p.value ? `<b>${p.name}</b><br/>${metricLabel[metric]}: ${fmtFull(p.value, u)}` : `${p.name}<br/>(无)`;
      },
    },
    visualMap: {
      min: 0,
      max: Math.max(...provincesSorted.map(p => pick(p, metric)), 1),
      left: 16, bottom: 24,
      text: ['高', '低'],
      calculable: true,
      inRange: { color: ['#eff6ff', '#bfdbfe', '#3b82f6'] },
      textStyle: { color: '#475569' },
      seriesIndex: 0,
    },
    geo: {
      map: 'china',
      roam: false,
      zoom: 1.2,
      center: [105, 36],
      layoutSize: '95%',
      top: 20,
      bottom: 30,
      itemStyle: { areaColor: '#f8fafc', borderColor: '#cbd5e1', borderWidth: 0.5 },
      label: {
        show: true,
        fontSize: 10,
        color: '#475569',
        fontWeight: 400,
        textBorderColor: '#fff',
        textBorderWidth: 2,
      },
      emphasis: {
        itemStyle: { areaColor: '#bfdbfe' },
        label: { show: true, fontSize: 11, color: '#0f172a', fontWeight: 600 },
      },
    },
    series: [
      // 1. 省份热力 (作为底图, 与 geo 同步 zoom/center)
      // label 写在 geo 上 (用 geoIndex 时 series 的 label 不生效, 这是 ECharts 坑)
      {
        type: 'map',
        map: 'china',
        geoIndex: 0,
        zoom: 1.2,
        center: [105, 36],
        layoutSize: '95%',
        top: 20,
        bottom: 30,
        data: provincesSorted.map(p => ({ name: p.name, value: pick(p, metric) })),
      },
      // 2. 飞线: 仓 → 省 (按 metric 加权线宽 + 飞动效果)
      {
        type: 'lines',
        coordinateSystem: 'geo',
        zlevel: 2,
        effect: {
          show: true,
          period: 5,
          trailLength: 0.5,
          color: '#fff',
          symbolSize: 3,
        },
        lineStyle: {
          color: '#0ea5e9',
          opacity: 0.4,
          curveness: 0.25,
        },
        data: flowLines.map(l => ({
          coords: l.coords,
          value: l.value,
          warehouse: l.warehouse,
          province: l.province,
          lineStyle: {
            width: Math.max(0.6, (l.value / maxFlow) * 5),
            opacity: Math.max(0.25, Math.min(0.85, 0.3 + (l.value / maxFlow) * 0.55)),
          },
        })),
      },
      // 3. 仓库点 (橙色涟漪 + 文字标签, focusedWh 高亮选中仓)
      {
        type: 'effectScatter',
        coordinateSystem: 'geo',
        zlevel: 3,
        rippleEffect: { period: 4, scale: 4, brushType: 'stroke' },
        showEffectOn: 'render',
        data: (overview.warehouses || [])
          .filter(w => warehouseCenter[w.name])
          .map(w => {
            const isFocused = !focusedWh || w.name === focusedWh;
            return {
              name: shortenWarehouse(w.name),
              value: [...warehouseCenter[w.name], pick(w, metric)],
              symbolSize: isFocused ? 16 : 8,
              itemStyle: {
                color: isFocused ? '#f97316' : '#94a3b8',
                shadowBlur: isFocused ? 12 : 0,
                shadowColor: '#fb923c',
                opacity: isFocused ? 1 : 0.35,
              },
              label: {
                show: true,
                position: 'right',
                formatter: '{b}',
                color: isFocused ? '#c2410c' : '#94a3b8',
                fontWeight: isFocused ? 600 : 400,
                fontSize: 11,
                textBorderColor: '#fff',
                textBorderWidth: 2,
                opacity: isFocused ? 1 : 0.5,
              },
            };
          }),
      },
    ],
  };

  // 仓库 Top 横条 (按当前 metric 排)
  const whTop = [...overview.warehouses]
    .sort((a, b) => pick(b, metric) - pick(a, metric))
    .slice(0, 12).reverse();
  const whBarOption = {
    tooltip: { trigger: 'axis', formatter: (p: any) => `<b>${p[0].name}</b><br/>${metricLabel[metric]}: ${fmtFull(p[0].value, metricUnit[metric])}` },
    grid: { left: 8, right: 110, top: 16, bottom: 16, containLabel: true },
    xAxis: {
      type: 'value',
      max: (v: { max: number }) => Math.ceil(v.max * 1.1),
      axisLabel: { formatter: (v: number) => fmt(v) },
    },
    yAxis: { type: 'category', data: whTop.map(w => w.name), axisLabel: { width: 220, overflow: 'truncate' } },
    series: [{
      type: 'bar', data: whTop.map(w => pick(w, metric)),
      itemStyle: { color: '#0ea5e9', borderRadius: [0, 4, 4, 0] },
      label: { show: true, position: 'right', formatter: (p: any) => fmt(p.value), color: '#475569' },
    }],
  };

  // === 矩阵: 客户端按 metric 算 Top 10 + 其它 ===
  const matrixView = (() => {
    if (!matrix || !matrix.cells.length) return null;
    const TOP_N = 10;
    const cells = matrix.cells;
    // 仓总
    const whTotalMap = new Map<string, MM>();
    const provTotalMap = new Map<string, MM>();
    for (const c of cells) {
      const w = whTotalMap.get(c.warehouse) || { name: c.warehouse, orders: 0, packages: 0 };
      w.orders += c.orders; w.packages += c.packages;
      whTotalMap.set(c.warehouse, w);
      const p = provTotalMap.get(c.province) || { name: c.province, orders: 0, packages: 0 };
      p.orders += c.orders; p.packages += c.packages;
      provTotalMap.set(c.province, p);
    }
    const whTotals = Array.from(whTotalMap.values()).sort((a, b) => pick(b, metric) - pick(a, metric));
    const provTotals = Array.from(provTotalMap.values()).sort((a, b) => pick(b, metric) - pick(a, metric));
    const topProvs = provTotals.slice(0, TOP_N);
    const hasOther = provTotals.length > TOP_N;
    const topProvNameSet = new Set(topProvs.map(p => p.name));
    const provNames = topProvs.map(p => p.name).concat(hasOther ? ['其它'] : []);
    // cellMap[wh][prov] = value
    const cellMap = new Map<string, Map<string, number>>();
    for (const c of cells) {
      let inner = cellMap.get(c.warehouse);
      if (!inner) { inner = new Map(); cellMap.set(c.warehouse, inner); }
      inner.set(c.province, pick(c, metric));
    }
    const values: number[][] = whTotals.map(w => {
      const inner = cellMap.get(w.name);
      const row = topProvs.map(p => inner?.get(p.name) || 0);
      if (hasOther) {
        const whTotal = pick(w, metric);
        const topSum = row.reduce((s, x) => s + x, 0);
        row.push(Math.max(0, whTotal - topSum));
      }
      return row;
    });
    const rowTotals = whTotals.map(w => pick(w, metric));
    const colTotals = provNames.map((_, j) => values.reduce((s, r) => s + r[j], 0));
    const grand = rowTotals.reduce((s, x) => s + x, 0);
    return {
      warehouses: whTotals.map(w => w.name),
      provinces: provNames,
      values,
      rowTotals,
      colTotals,
      grand,
    };
  })();

  // 矩阵微条最大值
  const matrixMax = matrixView ? Math.max(1, ...matrixView.values.flat()) : 1;

  return (
    <div style={{ padding: 0 }}>
      {/* 数据来源 + 指标说明（对齐库存预警样式） */}
      <div style={{ background: '#f8fafc', border: '1px solid #e2e8f0', borderRadius: 6, padding: '10px 14px', marginBottom: 12, fontSize: 12, color: '#64748b', lineHeight: '20px' }}>
        <div>
          <InfoCircleOutlined style={{ marginRight: 6, color: '#1e40af' }} />
          <span style={{ color: '#1e293b', fontWeight: 600 }}>数据来源：</span>
          南京委外成品仓、天津委外仓、西安仓库成品、松鲜鲜&大地密码云仓、长沙委外成品仓、安徽郎溪成品、南京分销虚拟仓（共 7 个仓库）
          &nbsp;·&nbsp;
          <span style={{ color: '#1e293b', fontWeight: 600 }}>统计口径：</span>
          按<span style={{ color: '#dc2626', fontWeight: 600 }}>发货时间 consign_time</span> 分月，主表均为已发货订单
        </div>
        <div style={{ marginTop: 4, marginLeft: 20 }}>
          <span style={{ color: '#1e293b', fontWeight: 600 }}>核心指标：</span>
          <span style={{ color: '#1e40af', fontWeight: 600 }}>发货订单数</span>
          ＝ 不重复 trade_id 数（已按发货时间过滤）
          &nbsp;&nbsp;|&nbsp;&nbsp;
          <span style={{ color: '#0ea5e9', fontWeight: 600 }}>物流包裹数</span>
          ＝ trade_id × 物流单号去重（一单可拆多包裹分发）
        </div>
        <div style={{ marginTop: 4, marginLeft: 20 }}>
          <span style={{ color: '#1e293b', fontWeight: 600 }}>过滤规则：</span>
          排除取消单 · 排除收货省为空 · 排除 trade_type 8/12（补差/对账特殊单，不产生物流包裹）· 仓库白名单与"采购计划看板/库存预警"完全一致
        </div>
      </div>

      {/* 筛选条 */}
      <Card className="bi-filter-card" style={{ marginBottom: 16 }}>
        <Row align="middle" gutter={[16, 12]} wrap>
          <Col>
            <span style={{ fontWeight: 500, marginRight: 8 }}>月份：</span>
            <Select
              value={ym || (overview?.ym || '')}
              style={{ width: 130 }}
              onChange={setYm}
              options={(overview?.ymList || []).map(m => ({ value: m, label: m }))}
              suffixIcon={<CalendarOutlined style={{ color: '#94a3b8' }} />}
            />
          </Col>

          <Col>
            <span style={{ fontWeight: 500, marginRight: 8 }}>指标：</span>
            <Segmented
              value={metric}
              onChange={(v) => setMetric(v as Metric)}
              options={[
                { label: '订单数', value: 'orders' },
                { label: '包裹数', value: 'packages' },
              ]}
            />
          </Col>

          <Col>
            <span style={{ fontWeight: 500, marginRight: 8 }}>渠道：</span>
            <Select
              allowClear placeholder="全部渠道" style={{ width: 200 }}
              value={shop || undefined} onChange={(v) => setShop(v || '')}
              options={(overview?.shops || []).map(s => ({ value: s, label: s }))}
              showSearch optionFilterProp="label"
              suffixIcon={<ShopOutlined style={{ color: '#94a3b8' }} />}
            />
          </Col>

          <Col>
            <span style={{ fontWeight: 500, marginRight: 8 }}>仓库：</span>
            <Select
              allowClear placeholder="全部仓库" style={{ width: 240 }}
              value={warehouse || undefined} onChange={(v) => setWarehouse(v || '')}
              options={(overview?.warehouses || []).map(w => ({ value: w.name, label: w.name }))}
              showSearch optionFilterProp="label"
              suffixIcon={<CarOutlined style={{ color: '#94a3b8' }} />}
            />
          </Col>

          <Col>
            <span style={{ fontWeight: 500, marginRight: 8 }}>省份：</span>
            <Select
              allowClear placeholder="全部省份" style={{ width: 160 }}
              value={province || undefined} onChange={(v) => setProvince(v || '')}
              options={(overview?.provinces || []).map(p => ({ value: p.name, label: p.name }))}
              showSearch optionFilterProp="label"
              suffixIcon={<EnvironmentOutlined style={{ color: '#94a3b8' }} />}
            />
          </Col>

          <Col>
            <span style={{ fontWeight: 500, marginRight: 8 }}>SKU：</span>
            <Input
              allowClear placeholder="搜 SKU 名称/编码"
              prefix={<SearchOutlined style={{ color: '#94a3b8' }} />}
              style={{ width: 220 }}
              value={skuKw} onChange={(e) => setSkuKw(e.target.value)}
              onPressEnter={fetchAll}
            />
          </Col>

          {/* 清空筛选 - 仅在有筛选时浮现 */}
          {(shop || warehouse || province || skuKw) && (
            <Col>
              <Button
                type="text"
                icon={<ClearOutlined />}
                onClick={() => { setShop(''); setWarehouse(''); setProvince(''); setSkuKw(''); }}
                style={{ color: '#ef4444' }}
              >
                清空筛选
              </Button>
            </Col>
          )}
        </Row>
      </Card>

      {/* KPI */}
      <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
        <Col xs={12} sm={6}>
          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: '#1e40af' }}>
            <Statistic title="发货订单数" value={k.orders} suffix="单" formatter={(v: any) => fmt(Number(v))}
              prefix={<AppstoreOutlined style={{ color: '#1e40af' }} />} />
            <div style={{ fontSize: 12, color: '#64748b', marginTop: 4 }}>按发货时间 consign_time 统计</div>
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: '#0ea5e9' }}>
            <Statistic title="物流包裹数" value={k.packages} suffix="包" formatter={(v: any) => fmt(Number(v))}
              prefix={<CarOutlined style={{ color: '#0ea5e9' }} />} />
            <div style={{ fontSize: 12, color: '#64748b', marginTop: 4 }}>
              {k.orders > 0 ? `平均 ${(k.packages / k.orders).toFixed(2)} 包/单` : '—'}
            </div>
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: '#f59e0b' }}>
            <Statistic title="销往省级行政区" value={k.provinceCnt} suffix="个"
              prefix={<EnvironmentOutlined style={{ color: '#f59e0b' }} />} />
            <div style={{ fontSize: 12, color: '#64748b', marginTop: 4 }}>覆盖区域</div>
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: '#7c3aed' }}>
            <Statistic title="出货仓库" value={k.warehouseCnt} suffix="个"
              prefix={<ShopOutlined style={{ color: '#7c3aed' }} />} />
            <div style={{ fontSize: 12, color: '#64748b', marginTop: 4 }}>白名单 7 仓内</div>
          </Card>
        </Col>
      </Row>

      {/* 飞线流向图 + 仓库聚焦侧栏 */}
      <Row gutter={12} style={{ marginBottom: 12 }}>
        <Col span={17}>
          <Card
            size="small"
            title={<>
              <GlobalOutlined /> 仓库 → 省份 出货流向图（{metricLabel[metric]}）
              {focusedWh && (
                <span style={{ marginLeft: 12, fontSize: 12, fontWeight: 'normal', color: '#f97316' }}>
                  · 聚焦：{shortenWarehouse(focusedWh)}
                </span>
              )}
            </>}
            styles={{ body: { padding: 8 } }}
          >
            {mapReady ? (
              <ReactECharts echarts={echarts} option={mapOption} style={{ height: 880 }} notMerge />
            ) : (
              <div style={{ height: 880, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#94a3b8' }}>
                <Spin tip="加载地图..." />
              </div>
            )}
          </Card>
        </Col>

        <Col span={7}>
          <Card
            size="small"
            title={<>仓库聚焦 · 省份占比</>}
            styles={{ body: { padding: '8px 12px' } }}
            style={{ height: '100%' }}
          >
            {/* 仓库切换按钮组 */}
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginBottom: 10 }}>
              <Button
                size="small"
                type={!focusedWh ? 'primary' : 'default'}
                onClick={() => setFocusedWh('')}
                style={{ fontSize: 12 }}
              >
                全部 7 仓
              </Button>
              {(overview.warehouses || []).filter(w => warehouseCenter[w.name]).map(w => (
                <Button
                  key={w.name}
                  size="small"
                  type={focusedWh === w.name ? 'primary' : 'default'}
                  onClick={() => setFocusedWh(focusedWh === w.name ? '' : w.name)}
                  style={{
                    fontSize: 12,
                    color: focusedWh === w.name ? '#fff' : '#475569',
                    background: focusedWh === w.name ? '#f97316' : undefined,
                    borderColor: focusedWh === w.name ? '#f97316' : undefined,
                  }}
                >
                  {shortenWarehouse(w.name)}
                </Button>
              ))}
            </div>

            {/* 省份占比榜 */}
            <div style={{ borderTop: '1px solid #e2e8f0', paddingTop: 8, marginBottom: 6 }}>
              <div style={{ fontSize: 11, color: '#64748b', marginBottom: 6, display: 'flex', justifyContent: 'space-between' }}>
                <span>{focusedWh ? `${shortenWarehouse(focusedWh)} → 各省 ${metricLabel[metric]}占比` : `7 仓汇总 → 各省 ${metricLabel[metric]}占比`}</span>
                <span style={{ color: '#0f172a', fontWeight: 600 }}>{fmt(focusedTotal)} {metricUnit[metric]}</span>
              </div>
            </div>

            <div style={{ maxHeight: 760, overflowY: 'auto', paddingRight: 4 }}>
              {focusedProvBreakdown.length === 0 ? <Empty description="暂无数据" /> :
                focusedProvBreakdown.map((p, i) => (
                  <div key={p.name} style={{ marginBottom: 8, fontSize: 12 }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 2 }}>
                      <span style={{ color: '#1e293b', fontWeight: i < 3 ? 600 : 400 }}>
                        <span style={{
                          display: 'inline-block', width: 18, color: i < 3 ? '#f97316' : '#94a3b8', fontWeight: 600,
                        }}>{i + 1}</span>
                        {p.name}
                      </span>
                      <span style={{ color: '#475569' }}>
                        <b style={{ color: '#0f172a' }}>{(p.ratio * 100).toFixed(1)}%</b>
                        <span style={{ color: '#94a3b8', fontSize: 11, marginLeft: 6 }}>{fmt(p.value)}</span>
                      </span>
                    </div>
                    <Progress
                      percent={p.ratio * 100}
                      showInfo={false}
                      size="small"
                      strokeColor={i === 0 ? '#1e40af' : i < 3 ? '#3b82f6' : '#93c5fd'}
                    />
                  </div>
                ))
              }
            </div>
          </Card>
        </Col>
      </Row>

      {/* 仓库 Top - 独占一行 */}
      <Card
        size="small"
        title={<><CarOutlined /> 仓库出货 Top（{metricLabel[metric]}）</>}
        style={{ marginBottom: 12 }}
      >
        {whTop.length ? (
          <ReactECharts echarts={echarts} option={whBarOption} style={{ height: 320 }} />
        ) : <Empty />}
      </Card>

      {/* 仓 × 省 矩阵 */}
      <Card size="small" title={<>仓 × 省 流向矩阵 <span style={{ color: '#94a3b8', fontSize: 12, fontWeight: 'normal', marginLeft: 8 }}>行=仓 列=省 (Top 10 + 其它) · 单位 {metricUnit[metric]}</span></>} style={{ marginBottom: 12 }}>
        {matrixView && matrixView.warehouses.length ? (
          <div style={{ overflowX: 'auto' }}>
            <table style={{ borderCollapse: 'collapse', minWidth: '100%', fontSize: 12 }}>
              <thead>
                <tr style={{ background: '#f8fafc' }}>
                  <th style={{
                    padding: 0,
                    position: 'sticky',
                    left: 0,
                    background: 'linear-gradient(to top right, #f8fafc 49.7%, #cbd5e1 49.7%, #cbd5e1 50.3%, #f8fafc 50.3%)',
                    borderBottom: '1px solid #e2e8f0',
                    minWidth: 220,
                    height: 56,
                  }}>
                    <div style={{ position: 'relative', width: '100%', height: '100%' }}>
                      <span style={{ position: 'absolute', bottom: 8, left: 14, fontSize: 12, color: '#475569', fontWeight: 600 }}>仓库</span>
                      <span style={{ position: 'absolute', top: 8, right: 14, fontSize: 12, color: '#475569', fontWeight: 600 }}>省份</span>
                    </div>
                  </th>
                  {matrixView.provinces.map((p, i) => (
                    <th key={p} style={{ padding: '8px 12px', textAlign: 'right', borderBottom: '1px solid #e2e8f0', minWidth: 100 }}>
                      {p}
                      <div style={{ color: '#94a3b8', fontWeight: 'normal', fontSize: 11 }}>{fmt(matrixView.colTotals[i])}</div>
                    </th>
                  ))}
                  <th style={{ padding: '8px 12px', textAlign: 'right', borderBottom: '1px solid #e2e8f0', background: '#f1f5f9', minWidth: 100 }}>合计</th>
                </tr>
              </thead>
              <tbody>
                {matrixView.warehouses.map((wh, i) => (
                  <tr key={wh} style={{ borderBottom: '1px solid #f1f5f9' }}>
                    <td style={{ padding: '6px 12px', position: 'sticky', left: 0, background: '#fff', borderRight: '1px solid #e2e8f0', fontWeight: 500 }}>{wh}</td>
                    {matrixView.values[i].map((v, j) => {
                      const ratio = v / matrixMax;
                      return (
                        <td key={j} style={{ padding: '6px 12px', textAlign: 'right', position: 'relative' }}>
                          <div style={{ position: 'absolute', left: 0, right: 0, top: 0, bottom: 0, padding: '6px 12px', zIndex: 0 }}>
                            <div style={{
                              position: 'absolute', right: 12, top: '50%', transform: 'translateY(-50%)',
                              height: 16, width: `calc(${Math.min(ratio * 100, 100)}% - 24px)`,
                              minWidth: ratio > 0 ? 2 : 0,
                              background: ratio > 0.5 ? '#1e40af' : ratio > 0.2 ? '#3b82f6' : '#93c5fd',
                              opacity: 0.18, borderRadius: 2,
                            }} />
                          </div>
                          <span style={{ position: 'relative', zIndex: 1, color: v > 0 ? '#0f172a' : '#cbd5e1' }}>
                            {v > 0 ? fmt(v) : '—'}
                          </span>
                        </td>
                      );
                    })}
                    <td style={{ padding: '6px 12px', textAlign: 'right', background: '#f8fafc', fontWeight: 600 }}>{fmt(matrixView.rowTotals[i])}</td>
                  </tr>
                ))}
                <tr style={{ background: '#f1f5f9', fontWeight: 600 }}>
                  <td style={{ padding: '8px 12px', position: 'sticky', left: 0, background: '#f1f5f9' }}>列合计</td>
                  {matrixView.colTotals.map((c, i) => (
                    <td key={i} style={{ padding: '8px 12px', textAlign: 'right' }}>{fmt(c)}</td>
                  ))}
                  <td style={{ padding: '8px 12px', textAlign: 'right', background: '#e0e7ff' }}>{fmt(matrixView.grand)}</td>
                </tr>
              </tbody>
            </table>
          </div>
        ) : <Empty />}
      </Card>

    </div>
  );
};

export default LogisticsAnalysis;
