// 销量预测·算法回测卡片 (v1.66.2)
//
// 销量预测主页底部嵌一个折叠卡片, 展示当前算法对最近 12 月做实时回测的结果
// 业务一眼看到"算法到底准不准"
import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Card, Col, Empty, Row, Spin, Statistic, Table, Tag, Tooltip } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import Chart from '../../components/Chart';
import { API_BASE } from '../../config';

interface BacktestRow {
  ym: string;
  predicted: number;
  actual: number;
  diff: number;
  errPct: number;
  absErrPct: number;
  holidayContext: string;
  formulaText: string;
}

interface BacktestSummary {
  months: number;
  avgAbsErrPct: number;
  medianAbsErrPct: number;
  bestMonth: string;
  bestErrPct: number;
  worstMonth: string;
  worstErrPct: number;
}

const errColor = (abs: number) => {
  if (abs <= 5) return '#16a34a';   // 绿
  if (abs <= 15) return '#d97706';  // 橙
  return '#dc2626';                  // 红
};

const errTagColor = (abs: number) => {
  if (abs <= 5) return 'success';
  if (abs <= 15) return 'warning';
  return 'error';
};

const ForecastBacktestCard: React.FC = () => {
  const [items, setItems] = useState<BacktestRow[]>([]);
  const [summary, setSummary] = useState<BacktestSummary | null>(null);
  const [loading, setLoading] = useState(false);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/offline/sales-forecast/backtest-recent?months=12`, {
        credentials: 'include',
      });
      const json = await res.json();
      if (json.code === 200) {
        setItems(json.data?.items || []);
        setSummary(json.data?.summary || null);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchData(); }, [fetchData]);

  // ECharts: 双柱 (预测 vs 实际) + 折线 (绝对误差%)
  const chartOption = useMemo(() => {
    if (items.length === 0) return null;
    return {
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      legend: { data: ['算法预测', '实际销量', '绝对误差%'], top: 0 },
      grid: { left: 60, right: 60, top: 36, bottom: 24 },
      xAxis: { type: 'category', data: items.map(b => b.ym) },
      yAxis: [
        { type: 'value', name: '销量(件)', axisLabel: { formatter: (v: number) => (v / 10000).toFixed(0) + 'w' } },
        { type: 'value', name: '误差%', position: 'right', axisLabel: { formatter: '{value}%' }, max: 50 },
      ],
      series: [
        { name: '算法预测', type: 'bar', data: items.map(b => b.predicted), itemStyle: { color: '#3b82f6' } },
        { name: '实际销量', type: 'bar', data: items.map(b => b.actual), itemStyle: { color: '#10b981' } },
        {
          name: '绝对误差%', type: 'line', yAxisIndex: 1,
          data: items.map(b => ({ value: b.absErrPct, itemStyle: { color: errColor(b.absErrPct) } })),
          symbol: 'circle', symbolSize: 8,
          lineStyle: { color: '#94a3b8', width: 2, type: 'dashed' },
          label: { show: true, formatter: '{c}%', fontSize: 11 },
        },
      ],
    };
  }, [items]);

  const columns = [
    { title: '月份', dataIndex: 'ym', width: 90 },
    {
      title: '场景', dataIndex: 'holidayContext', width: 140,
      render: (v: string) => v ? <Tag style={{ borderRadius: 999 }}>{v}</Tag> : '-',
    },
    {
      title: '算法预测', dataIndex: 'predicted', width: 110, align: 'right' as const,
      render: (v: number) => v.toLocaleString(),
    },
    {
      title: '实际销量', dataIndex: 'actual', width: 110, align: 'right' as const,
      render: (v: number) => v.toLocaleString(),
    },
    {
      title: '差值', dataIndex: 'diff', width: 110, align: 'right' as const,
      render: (v: number) => (
        <span style={{ color: v > 0 ? '#dc2626' : v < 0 ? '#16a34a' : '#64748b' }}>
          {v > 0 ? '+' : ''}{v.toLocaleString()}
        </span>
      ),
    },
    {
      title: '误差%', dataIndex: 'errPct', width: 110, align: 'right' as const,
      render: (v: number, r: BacktestRow) => (
        <Tag color={errTagColor(r.absErrPct)}>{v > 0 ? '+' : ''}{v.toFixed(1)}%</Tag>
      ),
      sorter: (a: BacktestRow, b: BacktestRow) => a.absErrPct - b.absErrPct,
    },
    {
      title: '当月公式', dataIndex: 'formulaText',
      render: (v: string) => (
        <Tooltip title={v}>
          <span style={{ fontSize: 11, color: '#64748b', cursor: 'help' }}>
            {v.length > 50 ? v.slice(0, 50) + '...' : v}
          </span>
        </Tooltip>
      ),
    },
  ];

  return (
    <Card
      size="small"
      title={
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span>算法回测</span>
          <Tag color="blue" style={{ borderRadius: 999, fontSize: 11 }}>近 12 月真实数据</Tag>
          <Tag color="purple" style={{ borderRadius: 999, fontSize: 11 }}>大区合计口径</Tag>
        </div>
      }
      extra={
        <Tooltip title="重新拉取最新回测">
          <a onClick={fetchData} style={{ fontSize: 13 }}>
            <ReloadOutlined /> 刷新
          </a>
        </Tooltip>
      }
      style={{ marginTop: 12 }}
    >
      <Spin spinning={loading}>
        {items.length === 0 && !loading ? (
          <Empty description="暂无回测数据" />
        ) : (
          <>
            {/* 顶部 KPI */}
            {summary && (
              <Row gutter={16} style={{ marginBottom: 12 }}>
                <Col xs={12} md={6}>
                  <Statistic
                    title="平均绝对误差"
                    value={summary.avgAbsErrPct}
                    suffix="%"
                    valueStyle={{ color: errColor(summary.avgAbsErrPct), fontSize: 22 }}
                  />
                </Col>
                <Col xs={12} md={6}>
                  <Statistic
                    title="中位数误差"
                    value={summary.medianAbsErrPct}
                    suffix="%"
                    valueStyle={{ fontSize: 22 }}
                  />
                </Col>
                <Col xs={12} md={6}>
                  <Statistic
                    title={`最准 (${summary.bestMonth})`}
                    value={summary.bestErrPct}
                    suffix="%"
                    valueStyle={{ color: '#16a34a', fontSize: 22 }}
                    formatter={(v) => `${(v as number) > 0 ? '+' : ''}${(v as number).toFixed(1)}`}
                  />
                </Col>
                <Col xs={12} md={6}>
                  <Statistic
                    title={`最大偏差 (${summary.worstMonth})`}
                    value={summary.worstErrPct}
                    suffix="%"
                    valueStyle={{ color: '#dc2626', fontSize: 22 }}
                    formatter={(v) => `${(v as number) > 0 ? '+' : ''}${(v as number).toFixed(1)}`}
                  />
                </Col>
              </Row>
            )}

            {/* 双柱+折线对比图 */}
            {chartOption && <Chart option={chartOption} style={{ height: 320 }} />}

            {/* 详细表格 */}
            <Table
              size="small"
              rowKey="ym"
              dataSource={items}
              columns={columns}
              pagination={false}
              style={{ marginTop: 12 }}
            />

            <div style={{ marginTop: 8, fontSize: 12, color: '#94a3b8', lineHeight: 1.7 }}>
              💡 回测原理: 对每个历史月份, 用截至上月底的销量历史数据 + 当前算法 (含节假日因子和增长系数) 算预测值, 跟实际销量对比.
              误差颜色: <Tag color="success" style={{ marginInlineEnd: 4 }}>绿 ≤ 5%</Tag>
              <Tag color="warning" style={{ marginInlineEnd: 4 }}>橙 ≤ 15%</Tag>
              <Tag color="error">红 &gt; 15%</Tag>
            </div>
          </>
        )}
      </Spin>
    </Card>
  );
};

export default ForecastBacktestCard;
