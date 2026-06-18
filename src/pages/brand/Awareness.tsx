// 心智渗透 — 主题页 A: 知名度层 (A1-A15)
import React from 'react';
import { Card, Col, Row, Space, Typography } from 'antd';
import { BannerNotice, MetricCard, TrendChart } from './components';
import { METRIC_MAP } from './mockData';

const { Title } = Typography;

const Awareness: React.FC = () => {
  // 按区域分组
  const m = (code: string) => METRIC_MAP[code];

  return (
    <div style={{ padding: '0 4px' }}>
      <Title level={4} style={{ marginTop: 0 }}>主题页 A — 知名度层</Title>
      <BannerNotice>
        知名度层 = 让人 <strong>知道有"减钠"这事</strong>。15 个指标分 5 区: 品类认知池 / 电商搜索 / 内容生态 / 公关 IP / 线下验证。
      </BannerNotice>

      {/* 顶部核心 4 指标 */}
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col xs={12} sm={6}><MetricCard data={m('A1')} emphasize /></Col>
        <Col xs={12} sm={6}><MetricCard data={m('A5')} emphasize /></Col>
        <Col xs={12} sm={6}><MetricCard data={m('A8')} emphasize /></Col>
        <Col xs={12} sm={6}><MetricCard data={m('A9')} emphasize /></Col>
      </Row>

      {/* 区域 1: 品类认知池趋势 */}
      <Card title="🌊 品类认知池趋势 (A1 / A2 / A3 对照)" style={{ marginBottom: 16 }}>
        <Row gutter={16}>
          <Col xs={24} lg={16}>
            <TrendChart
              metrics={[m('A1'), m('A2')]}
              title="百度指数: 减钠关键词组 vs 减盐基线"
              colors={['#1677ff', '#94a3b8']}
            />
          </Col>
          <Col xs={24} lg={8}>
            <Space direction="vertical" size={12} style={{ width: '100%' }}>
              <MetricCard data={m('A1')} compact />
              <MetricCard data={m('A2')} compact />
              <MetricCard data={m('A3')} compact />
            </Space>
          </Col>
        </Row>
      </Card>

      {/* 区域 2: 电商搜索趋势 */}
      <Card title="🛒 电商搜索趋势 (A6 / A7 / A8 / A9 — 品类词 vs 品牌词)" style={{ marginBottom: 16 }}>
        <Row gutter={16}>
          <Col xs={24} lg={16}>
            <TrendChart
              metrics={[m('A6'), m('A7'), m('A8'), m('A9')]}
              colors={['#1677ff', '#52c41a', '#fa8c16', '#722ed1']}
              title="天猫/京东 × 品类/品牌 搜索量"
            />
          </Col>
          <Col xs={24} lg={8}>
            <Space direction="vertical" size={8} style={{ width: '100%' }}>
              <MetricCard data={m('A6')} compact />
              <MetricCard data={m('A7')} compact />
              <MetricCard data={m('A8')} compact />
              <MetricCard data={m('A9')} compact />
            </Space>
          </Col>
        </Row>
      </Card>

      {/* 区域 3: 内容生态 */}
      <Card title="📱 内容生态 (A4 / A5 微信指数 + A10 / A11 内容增量)" style={{ marginBottom: 16 }}>
        <Row gutter={16}>
          <Col xs={24} lg={12}>
            <TrendChart
              metrics={[m('A4'), m('A5')]}
              title="微信指数: 减钠调味 vs 松鲜鲜"
              colors={['#1677ff', '#13c2c2']}
              height={240}
            />
          </Col>
          <Col xs={24} lg={12}>
            <TrendChart
              metrics={[m('A10'), m('A11')]}
              title="减钠内容新增量: 抖音 vs 小红书"
              colors={['#fa541c', '#eb2f96']}
              height={240}
            />
          </Col>
        </Row>
      </Card>

      {/* 区域 4: 公关 / IP */}
      <Card title="📢 公关 / 创始人 IP (A12 / A13 / A14)" style={{ marginBottom: 16 }}>
        <Row gutter={16}>
          <Col xs={24} lg={8}><MetricCard data={m('A12')} /></Col>
          <Col xs={24} lg={8}><MetricCard data={m('A13')} /></Col>
          <Col xs={24} lg={8}><MetricCard data={m('A14')} /></Col>
        </Row>
        <div style={{ marginTop: 16 }}>
          <TrendChart
            metrics={[m('A12'), m('A13')]}
            colors={['#722ed1', '#fa8c16']}
            title="创始人 IP 曝光 / 媒体报道数趋势"
            height={240}
          />
        </div>
      </Card>

      {/* 区域 5: 线下验证 */}
      <Card title="🏪 线下验证 (A15 — 物料门店销售增速对比)" style={{ marginBottom: 16 }}>
        <Row gutter={16}>
          <Col xs={24} lg={8}><MetricCard data={m('A15')} /></Col>
          <Col xs={24} lg={16}>
            <TrendChart
              metrics={[m('A15')]}
              colors={['#52c41a']}
              title="已替换门店 vs 未替换门店减钠系列销售增速差"
              height={240}
            />
          </Col>
        </Row>
        <div style={{ marginTop: 12, fontSize: 12, color: '#94a3b8' }}>
          算法: 已替换门店增速 - 未替换门店增速 (差值 &gt; 5% 视为物料替换驱动销售)
        </div>
      </Card>
    </div>
  );
};

export default Awareness;
