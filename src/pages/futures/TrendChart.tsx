// 期货走势图页：多品种叠加对比 + 折线/K线切换 + 时间范围
import React, { useEffect, useMemo, useState, useCallback } from 'react';
import { Card, Checkbox, Radio, Spin, Typography, DatePicker, Tag, Empty } from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import ReactECharts from '../../components/Chart';
import { API_BASE } from '../../config';
import { type FuturesSymbol, type FuturesBar, categoryLabel, categoryColor } from './types';
import KlineChart, { type Indicator } from './KlineChart';

const { Title, Text } = Typography;
const { RangePicker } = DatePicker;

type ChartType = 'line' | 'kline';
type RangeKey = '1m' | '3m' | '6m' | '1y' | 'all' | 'custom';

const rangeOptions: Array<{ key: RangeKey; label: string }> = [
  { key: '1m', label: '近1月' },
  { key: '3m', label: '近3月' },
  { key: '6m', label: '近半年' },
  { key: '1y', label: '近1年' },
  { key: 'all', label: '全部' },
];

// 多品种叠加用一组对比色（避开红绿，跟单品种 K 线区分）
const seriesColors = ['#1e40af', '#f59e0b', '#10b981', '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16', '#ef4444'];

const FuturesTrend: React.FC = () => {
  const [symbols, setSymbols] = useState<FuturesSymbol[]>([]);
  const [selected, setSelected] = useState<string[]>(['M0']); // 默认选豆粕
  const [chartType, setChartType] = useState<ChartType>('line');
  const [klineIndicator, setKlineIndicator] = useState<Indicator>('MACD');
  const [rangeKey, setRangeKey] = useState<RangeKey>('3m');
  const [customRange, setCustomRange] = useState<[Dayjs, Dayjs] | null>(null);
  const [barsByCode, setBarsByCode] = useState<Record<string, FuturesBar[]>>({});
  const [loading, setLoading] = useState(false);

  // 拉品种字典
  useEffect(() => {
    fetch(`${API_BASE}/api/futures/symbols`, { credentials: 'include' })
      .then(r => r.json())
      .then(j => setSymbols(j.data || []));
  }, []);

  // 计算时间范围
  const dateRange = useMemo<{ start: string; end: string } | null>(() => {
    const end = dayjs();
    if (rangeKey === 'custom' && customRange) {
      return { start: customRange[0].format('YYYY-MM-DD'), end: customRange[1].format('YYYY-MM-DD') };
    }
    if (rangeKey === 'all') return { start: '2018-01-01', end: end.format('YYYY-MM-DD') };
    if (rangeKey === 'custom') return null;
    const months: Record<Exclude<RangeKey, 'custom' | 'all'>, number> = { '1m': 1, '3m': 3, '6m': 6, '1y': 12 };
    return { start: end.subtract(months[rangeKey], 'month').format('YYYY-MM-DD'), end: end.format('YYYY-MM-DD') };
  }, [rangeKey, customRange]);

  // 拉选中品种的日线
  const fetchBars = useCallback(async () => {
    if (selected.length === 0) {
      setBarsByCode({});
      return;
    }
    setLoading(true);
    try {
      const tasks = selected.map(code => {
        const params = dateRange ? `&start=${dateRange.start}&end=${dateRange.end}` : '';
        return fetch(`${API_BASE}/api/futures/daily?code=${code}${params}`, { credentials: 'include' })
          .then(r => r.json())
          .then(j => ({ code, bars: (j.data?.bars || []) as FuturesBar[] }));
      });
      const results = await Promise.all(tasks);
      const map: Record<string, FuturesBar[]> = {};
      results.forEach(r => { map[r.code] = r.bars; });
      setBarsByCode(map);
    } finally {
      setLoading(false);
    }
  }, [selected, dateRange]);

  useEffect(() => { fetchBars(); }, [fetchBars]);

  // ECharts 配置（折线模式 + 多品种叠加）
  // K 线模式不在这里渲染，下方直接用 KlineChart 组件
  const option = useMemo(() => {
    // 折线模式 + 多品种叠加（按"涨跌百分比 vs 起点"归一化，避免不同单位混淆）
    const seriesList = selected.map((code, idx) => {
      const bars = barsByCode[code] || [];
      const sym = symbols.find(s => s.code === code);
      const base = bars[0]?.close || 1;
      const data = bars.map(b => [dayjs(b.date).format('YYYY-MM-DD'), selected.length > 1 ? ((b.close - base) / base * 100) : b.close]);
      return {
        name: sym?.nameCn || code,
        type: 'line',
        showSymbol: false,
        smooth: true,
        data,
        lineStyle: { width: 2, color: seriesColors[idx % seriesColors.length] },
        itemStyle: { color: seriesColors[idx % seriesColors.length] },
      };
    });

    const multiMode = selected.length > 1;
    return {
      tooltip: {
        trigger: 'axis',
        valueFormatter: (v: number) => multiMode ? `${v?.toFixed(2)}%` : v?.toLocaleString(),
      },
      legend: { data: seriesList.map(s => s.name), top: 0 },
      grid: { left: 70, right: 30, top: 40, bottom: 60 },
      xAxis: { type: 'category', boundaryGap: false },
      yAxis: {
        scale: true,
        name: multiMode ? '相对起点涨跌%' : (symbols.find(s => s.code === selected[0])?.unit || ''),
        axisLabel: { formatter: multiMode ? (v: number) => v.toFixed(1) + '%' : undefined },
      },
      dataZoom: [{ type: 'inside' }, { type: 'slider', height: 18 }],
      series: seriesList,
    };
  }, [chartType, selected, barsByCode, symbols]);

  // 按分类分组的品种 checkbox
  const grouped = useMemo(() => {
    const out: Record<string, FuturesSymbol[]> = { material: [], package: [], macro: [] };
    symbols.forEach(s => out[s.category]?.push(s));
    return out;
  }, [symbols]);

  return (
    <div>
      <Title level={3} style={{ margin: 0, marginBottom: 4 }}>走势图</Title>
      <Text type="secondary" style={{ fontSize: 13 }}>
        单选品种看 K 线；多选品种自动切换为"涨跌百分比"对比模式
      </Text>

      <Card style={{ marginTop: 16 }} styles={{ body: { padding: 16 } }}>
        {/* 控制行 */}
        <div style={{ display: 'flex', gap: 24, alignItems: 'center', flexWrap: 'wrap', marginBottom: 12 }}>
          <div>
            <Text strong style={{ marginInlineEnd: 8 }}>时间范围</Text>
            <Radio.Group
              size="small"
              value={rangeKey}
              onChange={e => setRangeKey(e.target.value)}
              optionType="button"
              buttonStyle="solid"
              options={rangeOptions.map(o => ({ label: o.label, value: o.key }))}
            />
            <RangePicker
              size="small"
              style={{ marginInlineStart: 8 }}
              value={customRange as any}
              onChange={(v) => { if (v?.[0] && v?.[1]) { setCustomRange([v[0], v[1]]); setRangeKey('custom'); } }}
            />
          </div>
          <div>
            <Text strong style={{ marginInlineEnd: 8 }}>图表类型</Text>
            <Radio.Group
              size="small"
              value={chartType}
              onChange={e => setChartType(e.target.value)}
              optionType="button"
              disabled={selected.length > 1}
            >
              <Radio.Button value="line">折线</Radio.Button>
              <Radio.Button value="kline">K线（仅单品种）</Radio.Button>
            </Radio.Group>
          </div>
          {chartType === 'kline' && selected.length === 1 && (
            <div>
              <Text strong style={{ marginInlineEnd: 8 }}>指标</Text>
              <Radio.Group
                size="small"
                value={klineIndicator}
                onChange={e => setKlineIndicator(e.target.value)}
                optionType="button"
              >
                <Radio.Button value="MACD">MACD</Radio.Button>
                <Radio.Button value="KDJ">KDJ</Radio.Button>
                <Radio.Button value="RSI">RSI</Radio.Button>
                <Radio.Button value="BOLL">BOLL</Radio.Button>
              </Radio.Group>
            </div>
          )}
        </div>

        {/* 品种多选 */}
        <div style={{ borderTop: '1px solid #f1f5f9', paddingTop: 12 }}>
          {(['material', 'package', 'macro'] as const).map(cat => (
            <div key={cat} style={{ marginBottom: 8 }}>
              <div style={{ display: 'inline-flex', alignItems: 'center', gap: 6, marginInlineEnd: 12, minWidth: 100 }}>
                <div style={{ width: 3, height: 14, background: categoryColor[cat], borderRadius: 2 }} />
                <Text strong style={{ fontSize: 13 }}>{categoryLabel[cat]}</Text>
              </div>
              <Checkbox.Group
                value={selected}
                onChange={(vals) => {
                  setSelected(vals as string[]);
                  // 多选时强制切到折线
                  if ((vals as string[]).length > 1) setChartType('line');
                }}
                options={(grouped[cat] || []).map(s => ({ label: s.nameCn, value: s.code }))}
              />
            </div>
          ))}
        </div>
      </Card>

      <Card style={{ marginTop: 12 }} styles={{ body: { padding: 12 } }}>
        {loading ? (
          <div style={{ textAlign: 'center', padding: 80 }}><Spin size="large" /></div>
        ) : selected.length === 0 ? (
          <Empty description="左侧勾选至少一个品种" style={{ marginBlock: 80 }} />
        ) : chartType === 'kline' && selected.length === 1 ? (
          <KlineChart
            bars={barsByCode[selected[0]] || []}
            indicator={klineIndicator}
            height={580}
            unit={symbols.find(s => s.code === selected[0])?.unit || ''}
            title={symbols.find(s => s.code === selected[0])?.nameCn || ''}
          />
        ) : (
          <ReactECharts option={option} style={{ height: 540 }} notMerge />
        )}
        {selected.length > 1 && (
          <div style={{ marginTop: 8, textAlign: 'center' }}>
            <Tag color="blue" style={{ borderRadius: 999 }}>
              多品种对比模式：所有曲线以第一天为基准计算涨跌百分比
            </Tag>
          </div>
        )}
      </Card>
    </div>
  );
};

export default FuturesTrend;
