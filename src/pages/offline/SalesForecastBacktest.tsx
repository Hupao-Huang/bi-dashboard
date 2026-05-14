// 销量预测算法回测 — 历史月份 × 算法 × 大区 预测 vs 实际对比
// 数据源: offline_sales_forecast_backtest 表 (Python 脚本写入)
import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Card, Empty, Radio, Spin, Statistic, Table, Tag, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { API_BASE } from '../../config';

interface BacktestItem {
  ym: string;
  algo: string;
  region: string;
  forecastQty: number;
  actualQty: number;
  errPct: number;
  absErrPct: number;
  trainEndDate: string;
  runAt: string;
}

interface SummaryItem {
  ym: string;
  algo: string;
  forecastQty: number;
  actualQty: number;
  totalErrPct: number;
  mape: number;
  regionCount: number;
}

// 算法中文展示名 (Tag 用) + 英文学名 + 业务能看懂的说明 (Tooltip)
// v1.65 删除 prophet/yoy/last_month/lightgbm 因为回测平均误差 > 50%
const algoMeta: Record<string, { label: string; en: string; desc: string }> = {
  lightgbm_sku:   { label: '梯度提升·SKU级', en: 'LightGBM (SKU-grain)',  desc: '微软梯度提升树, M5 比赛冠军方案, SKU × 大区颗粒度训练' },
  statsforecast:  { label: '统计集成',       en: 'StatsForecast',         desc: 'Nixtla 三模型集成 — AutoARIMA + AutoETS + AutoTheta 的均值' },
  builtin:        { label: '内置公式',       en: 'Built-in',               desc: '近 3 月均 ÷ 近 3 月季节系数均 × 预测月系数 × 大区同比 × 大区环比' },
  auto:           { label: '智能路由',       en: 'Auto-router',            desc: '看历史回测 MAPE 自动选当月最准的算法' },
  avg3m:          { label: '近 3 月均',      en: 'Average of last 3 mo.',  desc: '直接取前 3 个月销量平均值' },
  wma3:           { label: '加权 3 月均',    en: 'Weighted MA-3',          desc: '50% 上月 + 30% 前月 + 20% 大前月' },
  yoy_v2:         { label: '去年同期',       en: 'Year-on-year (v2)',     desc: '拿去年同月销量当本月预测 (春节月推荐, 业务手算同比验证最稳)' },
};

const algoLabel = (a: string) => algoMeta[a]?.label || a;
const algoTooltip = (a: string) => {
  const m = algoMeta[a];
  if (!m) return a;
  return <div><div><b>{m.en}</b></div><div style={{ marginTop: 4, fontSize: 12, color: '#cbd5e1' }}>{m.desc}</div></div>;
};

const errColor = (abs: number) =>
  abs <= 10 ? 'success' : abs <= 30 ? 'warning' : 'error';

const SalesForecastBacktest: React.FC = () => {
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState<BacktestItem[]>([]);
  const [summary, setSummary] = useState<SummaryItem[]>([]);
  const [regions, setRegions] = useState<string[]>([]);
  const [algoFilter, setAlgoFilter] = useState<string>('all');

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/offline/sales-forecast/backtest`, { credentials: 'include' });
      const json = await res.json();
      if (json.code !== 200) return;
      setItems(json.data.items || []);
      setSummary(json.data.summary || []);
      setRegions(json.data.regions || []);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchData(); }, [fetchData]);

  const algoOptions = useMemo(() => {
    const set = new Set(items.map(i => i.algo));
    return Array.from(set).sort();
  }, [items]);

  const filteredItems = useMemo(() => (
    algoFilter === 'all' ? items : items.filter(i => i.algo === algoFilter)
  ), [items, algoFilter]);
  const filteredSummary = useMemo(() => (
    algoFilter === 'all' ? summary : summary.filter(s => s.algo === algoFilter)
  ), [summary, algoFilter]);

  // 月份和算法的双轴矩阵 — 行=月份, 列=算法, 单元格=MAPE
  const mapeMatrix = useMemo(() => {
    const months = Array.from(new Set(summary.map(s => s.ym))).sort();
    const algos = Array.from(new Set(summary.map(s => s.algo))).sort();
    const m: Record<string, Record<string, SummaryItem | undefined>> = {};
    months.forEach(ym => {
      m[ym] = {};
      algos.forEach(a => {
        m[ym][a] = summary.find(s => s.ym === ym && s.algo === a);
      });
    });
    return { months, algos, m };
  }, [summary]);

  // 按月平均 MAPE — 跨算法平均 (展示该月整体精度)
  const monthlyMape = useMemo(() => {
    const map = new Map<string, { sum: number; cnt: number }>();
    filteredSummary.forEach(s => {
      const cur = map.get(s.ym) || { sum: 0, cnt: 0 };
      cur.sum += s.mape;
      cur.cnt += 1;
      map.set(s.ym, cur);
    });
    return Array.from(map.entries())
      .map(([ym, v]) => ({ ym, mape: v.cnt > 0 ? Math.round(v.sum / v.cnt * 10) / 10 : 0 }))
      .sort((a, b) => a.ym.localeCompare(b.ym));
  }, [filteredSummary]);

  // 全局 MAPE
  const overallMape = useMemo(() => {
    if (filteredSummary.length === 0) return 0;
    const sum = filteredSummary.reduce((s, x) => s + x.mape, 0);
    return Math.round(sum / filteredSummary.length * 10) / 10;
  }, [filteredSummary]);

  const detailColumns: ColumnsType<BacktestItem> = [
    { title: '月份', dataIndex: 'ym', width: 90, fixed: 'left' },
    {
      title: '算法', dataIndex: 'algo', width: 130,
      render: (a: string) => {
        const colorMap: Record<string, string> = {
          lightgbm_sku: 'green',
          statsforecast: 'blue',
          yoy_v2: 'purple',
          builtin: 'gold',
          avg3m: 'default', wma3: 'default',
        };
        return (
          <Tooltip title={algoTooltip(a)}>
            <Tag color={colorMap[a] || 'default'} style={{ cursor: 'help' }}>{algoLabel(a)}</Tag>
          </Tooltip>
        );
      },
    },
    { title: '大区', dataIndex: 'region', width: 100 },
    {
      title: '预测', dataIndex: 'forecastQty', width: 100, align: 'right',
      render: (v: number) => v.toLocaleString(),
    },
    {
      title: '实际', dataIndex: 'actualQty', width: 100, align: 'right',
      render: (v: number) => v.toLocaleString(),
    },
    {
      title: '差值', width: 100, align: 'right',
      render: (_: any, r: BacktestItem) => {
        const diff = r.forecastQty - r.actualQty;
        return <span style={{ color: diff > 0 ? '#16a34a' : '#dc2626' }}>{diff > 0 ? '+' : ''}{diff.toLocaleString()}</span>;
      },
    },
    {
      title: '相对误差%', dataIndex: 'errPct', width: 110, align: 'right',
      render: (v: number) => <Tag color={errColor(Math.abs(v))}>{v > 0 ? '+' : ''}{v.toFixed(1)}%</Tag>,
      sorter: (a, b) => Math.abs(a.errPct) - Math.abs(b.errPct),
    },
    {
      title: '训练截至', dataIndex: 'trainEndDate', width: 110,
      render: (v: string) => v || '-',
    },
    { title: '回测时间', dataIndex: 'runAt', width: 160 },
  ];

  return (
    <div>
      <Alert
        type="info"
        showIcon
        message="什么是算法回测?"
        description={
          <div style={{ lineHeight: 1.8 }}>
            <div>用<b>截至上月底</b>的历史数据训练算法 → 预测当月 → 对比当月实际销量, 得出每个算法的精度.</div>
            <div style={{ color: '#64748b' }}>
              MAPE = 平均绝对误差%, 越小越准. 一般 10% 内算优秀 · 10-30% 可接受 · 30%+ 算法需要调整.
              数据由后台 Python 脚本回写, 当前覆盖 2026-01 ~ 2026-04 月 × Prophet / StatsForecast 两个算法.
            </div>
          </div>
        }
        style={{ marginBottom: 12 }}
      />

      <Spin spinning={loading}>
        {summary.length === 0 && !loading ? (
          <Empty description="暂无回测数据 (待后台 Python 脚本跑完)" />
        ) : (
          <>
            {/* 顶部 KPI */}
            <div style={{ display: 'flex', gap: 24, marginBottom: 16, flexWrap: 'wrap' }}>
              <Card size="small" style={{ minWidth: 160 }}>
                <Statistic title="覆盖月份" value={mapeMatrix.months.length} suffix="个月" />
              </Card>
              <Card size="small" style={{ minWidth: 160 }}>
                <Statistic title="覆盖算法" value={mapeMatrix.algos.length} suffix="个" />
              </Card>
              <Card size="small" style={{ minWidth: 200 }}>
                <Statistic
                  title="总体平均 MAPE"
                  value={overallMape}
                  suffix="%"
                  valueStyle={{ color: errColor(overallMape) === 'success' ? '#16a34a' : errColor(overallMape) === 'warning' ? '#d97706' : '#dc2626' }}
                />
              </Card>
            </div>

            {/* 算法切换筛选 */}
            <Card size="small" style={{ marginBottom: 12 }}>
              <Radio.Group value={algoFilter} onChange={e => setAlgoFilter(e.target.value)}>
                <Radio.Button value="all">全部算法</Radio.Button>
                {algoOptions.map(a => (
                  <Tooltip key={a} title={algoTooltip(a)}>
                    <Radio.Button value={a}>{algoLabel(a)}</Radio.Button>
                  </Tooltip>
                ))}
              </Radio.Group>
            </Card>

            {/* MAPE 矩阵 (月份 × 算法) */}
            <Card title="按月 × 算法 MAPE 矩阵 (越绿越准, 越红越偏)" size="small" style={{ marginBottom: 12 }}>
              <Table
                size="small"
                pagination={false}
                rowKey="ym"
                dataSource={mapeMatrix.months.map(ym => {
                  const row: any = { ym };
                  mapeMatrix.algos.forEach(a => { row[a] = mapeMatrix.m[ym][a]; });
                  return row;
                })}
                columns={[
                  { title: '月份', dataIndex: 'ym', width: 100, fixed: 'left' },
                  ...mapeMatrix.algos.map(a => ({
                    title: <Tooltip title={algoTooltip(a)}><span style={{ cursor: 'help', borderBottom: '1px dashed #94a3b8' }}>{algoLabel(a)}</span></Tooltip>,
                    dataIndex: a,
                    width: 180,
                    render: (s: SummaryItem | undefined) => {
                      if (!s) return <span style={{ color: '#cbd5e1' }}>-</span>;
                      return (
                        <Tooltip title={<div>
                          <div>预测合计: {s.forecastQty.toLocaleString()}</div>
                          <div>实际合计: {s.actualQty.toLocaleString()}</div>
                          <div>大区合计误差: {s.totalErrPct > 0 ? '+' : ''}{s.totalErrPct}%</div>
                          <div>覆盖大区: {s.regionCount}</div>
                        </div>}>
                          <Tag color={errColor(s.mape)} style={{ cursor: 'help' }}>
                            MAPE {s.mape}%
                          </Tag>
                        </Tooltip>
                      );
                    },
                  })),
                ]}
              />
            </Card>

            {/* 月度 MAPE 趋势 */}
            <Card title="月度 MAPE 趋势 (跨算法平均)" size="small" style={{ marginBottom: 12 }}>
              <Table
                size="small"
                pagination={false}
                rowKey="ym"
                dataSource={monthlyMape}
                columns={[
                  { title: '月份', dataIndex: 'ym', width: 100 },
                  {
                    title: '平均 MAPE', dataIndex: 'mape', width: 200,
                    render: (v: number) => (
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                        <div style={{ width: 200, height: 12, background: '#f1f5f9', borderRadius: 4, overflow: 'hidden' }}>
                          <div style={{
                            width: `${Math.min(v, 100)}%`, height: '100%',
                            background: errColor(v) === 'success' ? '#16a34a' : errColor(v) === 'warning' ? '#d97706' : '#dc2626',
                          }} />
                        </div>
                        <span>{v}%</span>
                      </div>
                    ),
                  },
                ]}
              />
            </Card>

            {/* 明细 (按 月 × 算法 × 大区) */}
            <Card title="回测明细 (月 × 算法 × 大区)" size="small">
              <Table
                size="small"
                rowKey={(r) => `${r.ym}-${r.algo}-${r.region}`}
                columns={detailColumns}
                dataSource={filteredItems}
                pagination={{ pageSize: 50, showSizeChanger: true }}
                scroll={{ x: 1100 }}
              />
            </Card>
          </>
        )}
      </Spin>
    </div>
  );
};

export default SalesForecastBacktest;
