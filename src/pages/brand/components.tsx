// 心智渗透看板 — 公用组件 (健康灯/四象限/指标卡/趋势/热力图)
import React from 'react';
import { Card, Col, Empty, Row, Space, Tag, Tooltip } from 'antd';
import { InfoCircleOutlined, RiseOutlined, FallOutlined, MinusOutlined } from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import {
  HEALTH_LIGHTS, MONTHS, QUADRANT, STATUS_COLOR, STATUS_LABEL, TEAM_MATRIX, TEAMS,
  type LayerHealth, type MetricData, type MetricStatus,
} from './mockData';

// ---------- 1. 三层健康灯 ----------
export const HealthLightPanel: React.FC<{ lights?: LayerHealth[] }> = ({ lights = HEALTH_LIGHTS }) => {
  const allGreen = lights.every(l => l.health === 'green');
  return (
    <Card
      bordered={false}
      style={{
        background: allGreen
          ? 'linear-gradient(135deg, #f6ffed 0%, #fff 100%)'
          : 'linear-gradient(135deg, #fffbe6 0%, #fff 100%)',
      }}
    >
      <Row gutter={16} align="middle">
        <Col flex="0 0 auto">
          <div style={{ fontSize: 16, fontWeight: 600 }}>
            心智三层健康灯 <Tooltip title="任一层红灯, 总灯变红"><InfoCircleOutlined style={{ color: '#94a3b8', fontSize: 13 }} /></Tooltip>
          </div>
          <div style={{ fontSize: 12, color: '#64748b', marginTop: 4 }}>
            总状态: {allGreen ? <Tag color="success">三层全绿</Tag> : <Tag color="warning">需关注</Tag>}
          </div>
        </Col>
        <Col flex="1 1 auto">
          <Row gutter={16}>
            {lights.map(l => (
              <Col span={8} key={l.layer}>
                <div style={{
                  border: `2px solid ${l.health === 'green' ? '#52c41a' : '#ff4d4f'}`,
                  borderRadius: 8,
                  padding: 16,
                  background: l.health === 'green' ? '#f6ffed' : '#fff1f0',
                }}>
                  <Row align="middle" gutter={12}>
                    <Col>
                      <div style={{
                        width: 28, height: 28, borderRadius: '50%',
                        background: l.health === 'green' ? '#52c41a' : '#ff4d4f',
                        boxShadow: `0 0 12px ${l.health === 'green' ? '#52c41a' : '#ff4d4f'}80`,
                      }} />
                    </Col>
                    <Col flex="1">
                      <div style={{ fontWeight: 600, fontSize: 15 }}>{l.name}</div>
                      <div style={{ fontSize: 12, color: '#64748b', marginTop: 2 }}>
                        综合环比 {l.momAvg > 0 ? '+' : ''}{l.momAvg}%
                      </div>
                    </Col>
                  </Row>
                  <div style={{ fontSize: 12, color: '#475569', marginTop: 8 }}>{l.comment}</div>
                </div>
              </Col>
            ))}
          </Row>
        </Col>
      </Row>
    </Card>
  );
};

// ---------- 2. 四象限判定图 ----------
export const QuadrantChart: React.FC = () => {
  const q = QUADRANT;
  // 用 ECharts scatter 实现 4 象限
  const option = {
    grid: { top: 30, bottom: 30, left: 30, right: 30 },
    xAxis: {
      type: 'value',
      name: '知名度综合环比 →',
      nameGap: 8,
      min: -10, max: 10,
      axisLine: { onZero: false, lineStyle: { color: '#94a3b8' } },
      splitLine: { lineStyle: { type: 'dashed', color: '#e2e8f0' } },
    },
    yAxis: {
      type: 'value',
      name: '心智绑定综合环比 →',
      nameGap: 8,
      min: -10, max: 10,
      axisLine: { onZero: false, lineStyle: { color: '#94a3b8' } },
      splitLine: { lineStyle: { type: 'dashed', color: '#e2e8f0' } },
    },
    series: [
      {
        type: 'scatter',
        symbolSize: 30,
        data: [[q.xAxis, q.yAxis]],
        itemStyle: {
          color: q.quadrant === '双重增长' ? '#52c41a' :
                 q.quadrant === '双弱' ? '#ff4d4f' : '#faad14',
          shadowBlur: 16,
          shadowColor: q.quadrant === '双重增长' ? '#52c41a80' :
                       q.quadrant === '双弱' ? '#ff4d4f80' : '#faad1480',
        },
        label: { show: true, formatter: '当前', position: 'top', fontWeight: 600 },
      },
    ],
    graphic: [
      { type: 'text', left: '72%', top: '12%', style: { text: '✅ 双重增长', fontSize: 12, fill: '#52c41a' } },
      { type: 'text', left: '8%', top: '12%', style: { text: '⚠️ 绑定优先', fontSize: 12, fill: '#faad14' } },
      { type: 'text', left: '72%', top: '85%', style: { text: '⚠️ 认知优先', fontSize: 12, fill: '#faad14' } },
      { type: 'text', left: '8%', top: '85%', style: { text: '🔴 双弱', fontSize: 12, fill: '#ff4d4f' } },
    ],
  };
  return (
    <Card title={<span>心智状态四象限 <Tooltip title="横轴=知名度环比 (A1×0.4+A5×0.3+A8×0.3), 纵轴=心智绑定环比 (C1×0.4+C3×0.3+C5×0.3)"><InfoCircleOutlined style={{ color: '#94a3b8', fontSize: 13 }} /></Tooltip></span>}>
      <Row gutter={24} align="middle">
        <Col span={16}>
          <ReactECharts option={option} style={{ height: 320 }} />
        </Col>
        <Col span={8}>
          <div style={{
            padding: 20,
            background: q.quadrant === '双重增长' ? '#f6ffed' :
                        q.quadrant === '双弱' ? '#fff1f0' : '#fffbe6',
            borderRadius: 8,
          }}>
            <div style={{ fontSize: 14, color: '#64748b' }}>当前象限</div>
            <div style={{ fontSize: 28, fontWeight: 600, marginTop: 8 }}>{q.quadrant}</div>
            <div style={{ fontSize: 13, color: '#475569', marginTop: 12 }}>{q.desc}</div>
            <div style={{ marginTop: 16, fontSize: 12, color: '#64748b' }}>
              <div>横轴 (知名度): <strong>{q.xAxis > 0 ? '+' : ''}{q.xAxis}%</strong></div>
              <div>纵轴 (心智绑定): <strong>{q.yAxis > 0 ? '+' : ''}{q.yAxis}%</strong></div>
            </div>
          </div>
        </Col>
      </Row>
    </Card>
  );
};

// ---------- 3. 指标卡 ----------
export const StatusBadge: React.FC<{ status: MetricStatus; size?: 'small' | 'default' }> = ({ status, size = 'small' }) => (
  <Tag
    color={status === 'green' ? 'success' :
           status === 'yellow' ? 'warning' :
           status === 'blue' ? 'processing' : 'default'}
    style={{
      fontSize: size === 'small' ? 11 : 12,
      lineHeight: '18px',
      padding: '0 6px',
      margin: 0,
      opacity: status === 'red' ? 0.7 : 1,
    }}
  >
    {STATUS_LABEL[status]}
  </Tag>
);

export const MomTag: React.FC<{ mom: number }> = ({ mom }) => {
  if (mom > 0.1) return <Tag color="success" icon={<RiseOutlined />} style={{ margin: 0 }}>+{mom}%</Tag>;
  if (mom < -0.1) return <Tag color="error" icon={<FallOutlined />} style={{ margin: 0 }}>{mom}%</Tag>;
  return <Tag color="default" icon={<MinusOutlined />} style={{ margin: 0 }}>0%</Tag>;
};

export const MetricCard: React.FC<{
  data: MetricData;
  compact?: boolean;
  emphasize?: boolean;
}> = ({ data, compact, emphasize }) => {
  const { meta, current, mom } = data;
  const isRed = meta.status === 'red';
  return (
    <Card
      size={compact ? 'small' : 'default'}
      style={{
        height: '100%',
        borderTop: emphasize ? `3px solid ${STATUS_COLOR[meta.status]}` : undefined,
        opacity: isRed ? 0.78 : 1,
      }}
      bodyStyle={{ padding: compact ? 12 : 16 }}
    >
      <Row align="middle" justify="space-between" wrap={false}>
        <Col flex="1 1 auto" style={{ minWidth: 0 }}>
          <Space size={4} style={{ fontSize: 12, color: '#64748b' }}>
            <strong style={{ color: '#475569' }}>{meta.code}</strong>
            <span style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{meta.name}</span>
          </Space>
        </Col>
        <Col flex="0 0 auto">
          <Tooltip title={meta.sourceNote || meta.description}>
            <StatusBadge status={meta.status} />
          </Tooltip>
        </Col>
      </Row>
      <Row align="bottom" justify="space-between" style={{ marginTop: compact ? 6 : 10 }}>
        <Col>
          <Space size={6} align="baseline">
            <span style={{ fontSize: compact ? 22 : 28, fontWeight: 600, color: isRed ? '#94a3b8' : '#0f172a' }}>
              {isRed ? '—' : (typeof current === 'number' ? current.toLocaleString(undefined, { maximumFractionDigits: 2 }) : current)}
            </span>
            {!isRed && <span style={{ fontSize: 12, color: '#94a3b8' }}>{meta.unit}</span>}
          </Space>
        </Col>
        <Col>{!isRed && <MomTag mom={mom} />}</Col>
      </Row>
      {!compact && (
        <div style={{ fontSize: 11, color: '#94a3b8', marginTop: 6 }}>
          基准: {meta.baseline}
        </div>
      )}
    </Card>
  );
};

// ---------- 4. 趋势曲线 ----------
export const TrendChart: React.FC<{
  metrics: MetricData[];
  title?: string;
  height?: number;
  colors?: string[];
}> = ({ metrics, title, height = 280, colors }) => {
  if (metrics.length === 0) return <Empty />;
  const allRed = metrics.every(m => m.meta.status === 'red');
  const option = {
    title: title ? { text: title, textStyle: { fontSize: 14, fontWeight: 500 } } : undefined,
    tooltip: { trigger: 'axis' },
    legend: { bottom: 0, type: 'scroll' },
    grid: { top: title ? 40 : 20, bottom: 50, left: 50, right: 30 },
    xAxis: {
      type: 'category',
      data: MONTHS,
      axisLine: { lineStyle: { color: '#94a3b8' } },
    },
    yAxis: {
      type: 'value',
      axisLine: { lineStyle: { color: '#94a3b8' } },
      splitLine: { lineStyle: { type: 'dashed', color: '#e2e8f0' } },
    },
    color: colors,
    series: metrics.map(m => ({
      name: `${m.meta.code} ${m.meta.name}`,
      type: 'line',
      smooth: true,
      data: m.trend.map(p => p.value),
      symbol: 'circle',
      symbolSize: 6,
      lineStyle: { width: 2 },
      itemStyle: { opacity: m.meta.status === 'red' ? 0.5 : 1 },
    })),
  };
  return (
    <div style={{ position: 'relative' }}>
      {allRed && (
        <div style={{
          position: 'absolute', top: 8, right: 8, zIndex: 10,
          background: '#fafafa', padding: '2px 8px', borderRadius: 4,
          fontSize: 11, color: '#94a3b8', border: '1px solid #e2e8f0',
        }}>
          ⏳ 数据待接入, 显示为示意趋势
        </div>
      )}
      <ReactECharts option={option} style={{ height }} />
    </div>
  );
};

// ---------- 5. 团队归因矩阵 (热力图) ----------
export const TeamHeatmap: React.FC = () => {
  // 数据格式: [x团队序号, y层级序号, mom值]
  const layers = ['A', 'B', 'C'];
  const layerNames = ['知名度', '好感度', '心智绑定'];
  const data = TEAM_MATRIX.map(cell => [
    TEAMS.indexOf(cell.team),
    layers.indexOf(cell.layer),
    cell.mom,
  ]);
  const option = {
    tooltip: {
      position: 'top',
      formatter: (params: any) => {
        const cell = TEAM_MATRIX.find(c => c.team === TEAMS[params.data[0]] && c.layer === layers[params.data[1]]);
        if (!cell) return '';
        return `<b>${cell.team} × ${layerNames[params.data[1]]}</b><br/>
        综合环比: <strong>${cell.mom > 0 ? '+' : ''}${cell.mom}%</strong><br/>
        关联指标: ${cell.metrics.join(', ') || '无'}`;
      },
    },
    grid: { left: 80, right: 30, top: 30, bottom: 60 },
    xAxis: {
      type: 'category', data: TEAMS, splitArea: { show: true },
      axisLabel: { fontSize: 12 },
    },
    yAxis: {
      type: 'category', data: layerNames, splitArea: { show: true },
      axisLabel: { fontSize: 12 },
    },
    visualMap: {
      min: -10, max: 10,
      orient: 'horizontal', left: 'center', bottom: 5,
      inRange: { color: ['#ff4d4f', '#fafafa', '#52c41a'] },
      text: ['+10%', '-10%'],
    },
    series: [{
      name: '综合环比',
      type: 'heatmap',
      data,
      label: { show: true, formatter: (p: any) => `${p.data[2] > 0 ? '+' : ''}${p.data[2]}%`, fontSize: 12, fontWeight: 600 },
      emphasis: { itemStyle: { shadowBlur: 10, shadowColor: 'rgba(0,0,0,0.5)' } },
    }],
  };
  return (
    <Card title={<span>团队归因矩阵 <Tooltip title="6 团队 × 3 心智层级, 颜色映射综合环比"><InfoCircleOutlined style={{ color: '#94a3b8', fontSize: 13 }} /></Tooltip></span>}>
      <ReactECharts option={option} style={{ height: 280 }} />
    </Card>
  );
};

// ---------- 6. 三层漏斗/金字塔 (核心指标缩略) ----------
export const LayerSummaryFunnel: React.FC<{
  aSummary: MetricData;
  bSummary: MetricData;
  cSummary: MetricData;
  onLayerClick?: (layer: 'A' | 'B' | 'C') => void;
}> = ({ aSummary, bSummary, cSummary, onLayerClick }) => (
  <Card title="三层核心指标">
    <Row gutter={[16, 16]}>
      {[
        { layer: 'A' as const, label: '知名度层', m: aSummary, color: '#1677ff' },
        { layer: 'B' as const, label: '好感度层', m: bSummary, color: '#722ed1' },
        { layer: 'C' as const, label: '心智绑定层', m: cSummary, color: '#13c2c2' },
      ].map(({ layer, label, m, color }) => (
        <Col span={24} key={layer}>
          <div
            onClick={() => onLayerClick?.(layer)}
            style={{
              cursor: onLayerClick ? 'pointer' : 'default',
              padding: 16, background: '#fafafa', borderRadius: 8, borderLeft: `4px solid ${color}`,
            }}
          >
            <Row align="middle" justify="space-between">
              <Col>
                <Space size={8}>
                  <strong style={{ fontSize: 15, color }}>{layer} {label}</strong>
                  <span style={{ color: '#64748b', fontSize: 12 }}>核心: {m.meta.name}</span>
                </Space>
              </Col>
              <Col>
                <Space size={12}>
                  <span style={{ fontSize: 22, fontWeight: 600 }}>{m.meta.status === 'red' ? '—' : m.current.toLocaleString()} <span style={{ fontSize: 12, color: '#94a3b8' }}>{m.meta.unit}</span></span>
                  <MomTag mom={m.mom} />
                  <StatusBadge status={m.meta.status} />
                </Space>
              </Col>
            </Row>
          </div>
        </Col>
      ))}
    </Row>
  </Card>
);

// ---------- 7. 页面通用提示横幅 ----------
export const BannerNotice: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <Card
    size="small"
    style={{ background: '#e6f4ff', borderColor: '#91caff', marginBottom: 16 }}
    bodyStyle={{ padding: '8px 12px' }}
  >
    <Space size={8}>
      <InfoCircleOutlined style={{ color: '#1677ff' }} />
      <span style={{ fontSize: 13, color: '#0f172a' }}>{children}</span>
    </Space>
  </Card>
);
