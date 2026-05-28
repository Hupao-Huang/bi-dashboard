// 心智渗透 — 主题页 B: 好感度层 (B1-B12)
import React from 'react';
import { Card, Col, Row, Space, Tag, Timeline, Typography } from 'antd';
import { BannerNotice, MetricCard, StatusBadge, TrendChart } from './components';
import { METRIC_MAP } from './mockData';

const { Title } = Typography;

const Affinity: React.FC = () => {
  const m = (code: string) => METRIC_MAP[code];

  return (
    <div style={{ padding: '0 4px' }}>
      <Title level={4} style={{ marginTop: 0 }}>主题页 B — 好感度层</Title>
      <BannerNotice>
        好感度层 = 让人 <strong>觉得"减钠"是好的、松鲜鲜值这价</strong>。12 个指标分 5 区: 电商口碑 / 舆情健康 / 品牌资产 / 内容共鸣 / 权威背书。
      </BannerNotice>

      {/* 顶部核心 3 指标 */}
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col xs={8}><MetricCard data={m('B1')} emphasize /></Col>
        <Col xs={8}><MetricCard data={m('B2')} emphasize /></Col>
        <Col xs={8}><MetricCard data={m('B8')} emphasize /></Col>
      </Row>

      {/* 区域 1: 电商口碑 */}
      <Card title="⭐ 电商口碑 (B2 / B3 / B4 — 三平台综合好评率)" style={{ marginBottom: 16 }}>
        <Row gutter={16}>
          <Col xs={24} lg={16}>
            <TrendChart
              metrics={[m('B2'), m('B3'), m('B4')]}
              colors={['#fa8c16', '#cf1322', '#000000']}
              title="天猫 / 京东 / 抖音 好评率趋势"
            />
          </Col>
          <Col xs={24} lg={8}>
            <Space direction="vertical" size={8} style={{ width: '100%' }}>
              <MetricCard data={m('B2')} compact />
              <MetricCard data={m('B3')} compact />
              <MetricCard data={m('B4')} compact />
            </Space>
          </Col>
        </Row>
      </Card>

      {/* 区域 2: 舆情健康 */}
      <Card title="💬 舆情健康 (B5 / B6 / B7)" style={{ marginBottom: 16 }}>
        <Row gutter={16}>
          <Col xs={24} lg={8}><MetricCard data={m('B5')} /></Col>
          <Col xs={24} lg={8}><MetricCard data={m('B6')} /></Col>
          <Col xs={24} lg={8}><MetricCard data={m('B7')} /></Col>
        </Row>
        <div style={{ marginTop: 16 }}>
          <TrendChart
            metrics={[m('B5'), m('B6')]}
            colors={['#52c41a', '#ff4d4f']}
            title="正面声量占比 vs 负面舆情数"
            height={240}
          />
        </div>
      </Card>

      {/* 区域 3: 品牌资产 */}
      <Card title="💰 品牌资产 (B8 / B9 — 线上 vs 线下溢价指数)" style={{ marginBottom: 16 }}>
        <Row gutter={16}>
          <Col xs={24} lg={16}>
            <TrendChart
              metrics={[m('B8'), m('B9')]}
              colors={['#1677ff', '#52c41a']}
              title="品牌溢价: 线上 vs 线下"
            />
          </Col>
          <Col xs={24} lg={8}>
            <Space direction="vertical" size={12} style={{ width: '100%' }}>
              <MetricCard data={m('B8')} compact />
              <MetricCard data={m('B9')} compact />
            </Space>
          </Col>
        </Row>
      </Card>

      {/* 区域 4: 内容共鸣 */}
      <Card title="🎬 内容共鸣 (B10 — 种草内容 CPE)" style={{ marginBottom: 16 }}>
        <Row gutter={16}>
          <Col xs={24} lg={8}><MetricCard data={m('B10')} /></Col>
          <Col xs={24} lg={16}>
            <TrendChart
              metrics={[m('B10')]}
              colors={['#722ed1']}
              title="种草 CPE 趋势 (越低越好)"
              height={240}
            />
          </Col>
        </Row>
      </Card>

      {/* 区域 5: 权威背书 */}
      <Card title="🏆 权威背书 (B11 行业奖项里程碑 / B12 媒体覆盖率)">
        <Row gutter={16}>
          <Col xs={24} lg={12}>
            <Card type="inner" title="B11 行业奖项时间轴" size="small">
              <Timeline
                items={[
                  { color: 'green', children: <div><Tag color="success">2026-05</Tag> 中国调味品行业协会 - 减钠创新奖</div> },
                  { color: 'green', children: <div><Tag color="success">2026-04</Tag> 江南大学联合实验室 - 学术认证</div> },
                  { color: 'green', children: <div><Tag color="success">2026-03</Tag> 健康产业大会 - 减钠先锋品牌</div> },
                  { color: 'blue', children: <div><Tag color="processing">2026-06 预计</Tag> 博鳌峰会 - 行业贡献奖</div> },
                ]}
              />
            </Card>
          </Col>
          <Col xs={24} lg={12}>
            <Card type="inner" title="B12 核心媒体覆盖率" size="small">
              <MetricCard data={m('B12')} compact />
              <div style={{ marginTop: 12 }}>
                <TrendChart
                  metrics={[m('B12')]}
                  colors={['#1677ff']}
                  height={180}
                />
              </div>
            </Card>
          </Col>
        </Row>
      </Card>
    </div>
  );
};

export default Affinity;
