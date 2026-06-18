// 心智渗透 — 总览页 (心智健康度驾驶舱)
import React from 'react';
import { Card, Col, Row, Space, Tag, Timeline, Typography } from 'antd';
import { useNavigate } from 'react-router-dom';
import {
  BannerNotice, HealthLightPanel, LayerSummaryFunnel, QuadrantChart,
  StatusBadge, TeamHeatmap,
} from './components';
import {
  CORE_MEDIA, METRIC_MAP, UPCOMING_EVENTS,
} from './mockData';

const { Title } = Typography;

const MindShareOverview: React.FC = () => {
  const navigate = useNavigate();
  // 三层核心指标取代表性指标
  const aSummary = METRIC_MAP['A6']; // 品类词搜索量-天猫
  const bSummary = METRIC_MAP['B2']; // 综合好评率-天猫
  const cSummary = METRIC_MAP['C5']; // 品牌/品类搜索绑定度

  return (
    <div style={{ padding: '0 4px' }}>
      <Title level={4} style={{ marginTop: 0 }}>心智健康度驾驶舱</Title>
      <BannerNotice>
        当前为 <strong>骨架预览版</strong>, 部分指标数据待 RPA 团队补加 5 个文件 + 业务录入到位后回填。
        指标卡片右上角标签: <StatusBadge status="green" /> 已接入 / <StatusBadge status="yellow" /> 部分接入 (缺业务输入) / <StatusBadge status="blue" /> 系统算 / <StatusBadge status="red" /> 待接入。
      </BannerNotice>

      {/* 顶部: 三层健康灯 */}
      <div style={{ marginBottom: 16 }}>
        <HealthLightPanel />
      </div>

      {/* 中部左: 四象限 + 三层核心 */}
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col xs={24} lg={16}>
          <QuadrantChart />
        </Col>
        <Col xs={24} lg={8}>
          <LayerSummaryFunnel
            aSummary={aSummary}
            bSummary={bSummary}
            cSummary={cSummary}
            onLayerClick={(layer) => {
              const path = layer === 'A' ? 'awareness' : layer === 'B' ? 'affinity' : 'binding';
              navigate(`/brand/${path}`);
            }}
          />
        </Col>
      </Row>

      {/* 底部: 团队归因热力图 + 线上线下对比 */}
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col xs={24} lg={14}>
          <TeamHeatmap />
        </Col>
        <Col xs={24} lg={10}>
          <OnlineOfflineComparison />
        </Col>
      </Row>

      {/* 6 月关键事件预告 */}
      <Row gutter={16}>
        <Col xs={24} lg={14}>
          <Card title="6 月关键事件预告 (心智驱动节点)">
            <Timeline
              items={UPCOMING_EVENTS.map(e => ({
                color: 'blue',
                children: (
                  <div>
                    <Space>
                      <Tag color="blue">{e.date}</Tag>
                      <strong>{e.title}</strong>
                    </Space>
                    <div style={{ color: '#64748b', fontSize: 12, marginTop: 4 }}>{e.desc}</div>
                  </div>
                ),
              }))}
            />
          </Card>
        </Col>
        <Col xs={24} lg={10}>
          <Card title="核心媒体覆盖率 (B12)">
            <div style={{ marginBottom: 12, color: '#475569' }}>
              已合作 <strong style={{ fontSize: 20, color: '#1677ff' }}>{CORE_MEDIA.filter(m => m.covered).length}</strong> / {CORE_MEDIA.length} 家
              <Tag color="success" style={{ marginLeft: 8 }}>+6% MoM</Tag>
            </div>
            <Space size={[6, 6]} wrap>
              {CORE_MEDIA.map(m => (
                <Tag key={m.name} color={m.covered ? 'success' : 'default'} style={{ marginRight: 0 }}>
                  {m.covered ? '✓ ' : '○ '}{m.name}
                </Tag>
              ))}
            </Space>
          </Card>
        </Col>
      </Row>
    </div>
  );
};

// 线上 vs 线下心智对比
const OnlineOfflineComparison: React.FC = () => {
  const onlineMetrics = [
    { code: 'A1', name: '减钠百度指数', value: METRIC_MAP['A1'].current, mom: METRIC_MAP['A1'].mom, status: METRIC_MAP['A1'].meta.status },
    { code: 'C1', name: '抖音品类关联度', value: METRIC_MAP['C1'].current + '%', mom: METRIC_MAP['C1'].mom, status: METRIC_MAP['C1'].meta.status },
    { code: 'C5', name: '搜索绑定度', value: METRIC_MAP['C5'].current + '%', mom: METRIC_MAP['C5'].mom, status: METRIC_MAP['C5'].meta.status },
  ];
  const offlineMetrics = [
    { code: 'C10', name: '减钠 vs 减盐销量', value: METRIC_MAP['C10'].current + '%', mom: METRIC_MAP['C10'].mom, status: METRIC_MAP['C10'].meta.status },
    { code: 'A15', name: '物料门店增速差', value: METRIC_MAP['A15'].current + '%', mom: METRIC_MAP['A15'].mom, status: METRIC_MAP['A15'].meta.status },
    { code: 'C14', name: '新客首购减钠占比', value: METRIC_MAP['C14'].current + '%', mom: METRIC_MAP['C14'].mom, status: METRIC_MAP['C14'].meta.status },
  ];
  return (
    <Card title="线上 vs 线下 心智对比">
      <Row gutter={12}>
        <Col span={12}>
          <div style={{ padding: 12, background: '#f0f7ff', borderRadius: 8, height: '100%' }}>
            <div style={{ fontWeight: 600, marginBottom: 12, color: '#1677ff' }}>📱 线上心智</div>
            {onlineMetrics.map(m => (
              <div key={m.code} style={{ marginBottom: 10, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <div style={{ fontSize: 12, color: '#475569' }}>
                  <strong>{m.code}</strong> {m.name}
                </div>
                <Space size={4}>
                  <span style={{ fontWeight: 600 }}>{m.status === 'red' ? '—' : m.value}</span>
                  <StatusBadge status={m.status} />
                </Space>
              </div>
            ))}
          </div>
        </Col>
        <Col span={12}>
          <div style={{ padding: 12, background: '#f6ffed', borderRadius: 8, height: '100%' }}>
            <div style={{ fontWeight: 600, marginBottom: 12, color: '#52c41a' }}>🏪 线下心智</div>
            {offlineMetrics.map(m => (
              <div key={m.code} style={{ marginBottom: 10, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <div style={{ fontSize: 12, color: '#475569' }}>
                  <strong>{m.code}</strong> {m.name}
                </div>
                <Space size={4}>
                  <span style={{ fontWeight: 600 }}>{m.status === 'red' ? '—' : m.value}</span>
                  <StatusBadge status={m.status} />
                </Space>
              </div>
            ))}
          </div>
        </Col>
      </Row>
    </Card>
  );
};

export default MindShareOverview;
