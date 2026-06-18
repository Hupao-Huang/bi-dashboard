// 心智渗透 — 主题页 C: 心智绑定层 (C1-C14)
import React from 'react';
import { Card, Col, Progress, Row, Space, Tag, Typography } from 'antd';
import { BannerNotice, MetricCard, TrendChart } from './components';
import { METRIC_MAP } from './mockData';

const { Title } = Typography;

const Binding: React.FC = () => {
  const m = (code: string) => METRIC_MAP[code];

  return (
    <div style={{ padding: '0 4px' }}>
      <Title level={4} style={{ marginTop: 0 }}>主题页 C — 心智绑定层</Title>
      <BannerNotice>
        心智绑定层 = 让人 <strong>想到"减钠"就想到松鲜鲜</strong>。14 个指标分 5 区: 线上绑定 / 声量份额 / 语义口碑 / 品类教育 / 线下实证。
      </BannerNotice>

      {/* 顶部核心 3 指标 */}
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col xs={8}><MetricCard data={m('C3')} emphasize /></Col>
        <Col xs={8}><MetricCard data={m('C5')} emphasize /></Col>
        <Col xs={8}><MetricCard data={m('C10')} emphasize /></Col>
      </Row>

      {/* 区域 1: 线上绑定 */}
      <Card title="📱 线上绑定 (C1 / C2 品类关联度 + C5 搜索绑定度 + C6 点击份额)" style={{ marginBottom: 16 }}>
        <Row gutter={16}>
          <Col xs={24} lg={16}>
            <TrendChart
              metrics={[m('C1'), m('C2'), m('C5'), m('C6')]}
              colors={['#fa541c', '#eb2f96', '#1677ff', '#52c41a']}
              title="抖音/小红书品类关联度 + 搜索绑定度 + 点击份额"
            />
          </Col>
          <Col xs={24} lg={8}>
            <Space direction="vertical" size={8} style={{ width: '100%' }}>
              <MetricCard data={m('C1')} compact />
              <MetricCard data={m('C2')} compact />
              <MetricCard data={m('C5')} compact />
              <MetricCard data={m('C6')} compact />
            </Space>
          </Col>
        </Row>
      </Card>

      {/* 区域 2: 声量份额 */}
      <Card title="📣 声量份额 (C3 减钠 SOV vs C4 全话题 SOV)" style={{ marginBottom: 16 }}>
        <Row gutter={16}>
          <Col xs={24} lg={16}>
            <TrendChart
              metrics={[m('C3'), m('C4')]}
              colors={['#1677ff', '#94a3b8']}
              title="松鲜鲜 SOV: 减钠子话题 vs 全调味品话题"
            />
          </Col>
          <Col xs={24} lg={8}>
            <Space direction="vertical" size={12} style={{ width: '100%' }}>
              <MetricCard data={m('C3')} compact />
              <MetricCard data={m('C4')} compact />
              <Card size="small" style={{ background: '#fafafa' }}>
                <div style={{ fontSize: 12, color: '#475569' }}>
                  C3/C4 差值反映 <strong>品类聚焦度</strong> — 减钠话题占有率显著高于全话题, 说明品牌已成减钠代名词
                </div>
              </Card>
            </Space>
          </Col>
        </Row>
      </Card>

      {/* 区域 3: 语义与口碑绑定 */}
      <Card title="📝 语义与口碑绑定 (C7 / C8 — NLP 类指标)" style={{ marginBottom: 16 }}>
        <Row gutter={16}>
          <Col xs={24} lg={12}><MetricCard data={m('C7')} /></Col>
          <Col xs={24} lg={12}><MetricCard data={m('C8')} /></Col>
        </Row>
        <div style={{ marginTop: 16 }}>
          <TrendChart
            metrics={[m('C7'), m('C8')]}
            colors={['#722ed1', '#13c2c2']}
            height={240}
          />
        </div>
        <div style={{ marginTop: 12, fontSize: 12, color: '#fa8c16' }}>
          ⚠️ C7/C8 依赖 NLP 工具, Q3 到位后启用; 目前显示为示意趋势
        </div>
      </Card>

      {/* 区域 4: 品类教育 */}
      <Card title="🎓 品类教育 (C9 — 品类教育品牌渗透率)" style={{ marginBottom: 16 }}>
        <Row gutter={16}>
          <Col xs={24} lg={8}><MetricCard data={m('C9')} /></Col>
          <Col xs={24} lg={16}>
            <TrendChart
              metrics={[m('C9')]}
              colors={['#13c2c2']}
              title="减钠内容中提及松鲜鲜占比 = (A10含松+A11含松)/(A10+A11)"
              height={240}
            />
          </Col>
        </Row>
      </Card>

      {/* 区域 5: 线下实证 */}
      <Card title="🏪 线下实证 (C10 门店对比 / C11 非促销动销 / C14 新客首购)">
        <Row gutter={16}>
          <Col xs={24} lg={8}><MetricCard data={m('C10')} /></Col>
          <Col xs={24} lg={8}><MetricCard data={m('C11')} /></Col>
          <Col xs={24} lg={8}><MetricCard data={m('C14')} /></Col>
        </Row>

        <div style={{ marginTop: 16 }}>
          <Card type="inner" size="small" title="C10 同门店减钠 vs 减盐销量对比指数 — 6 大示例门店">
            <Row gutter={[8, 12]}>
              {[
                { shop: '上海陆家嘴 Ole 旗舰店', value: 112, color: '#52c41a' },
                { shop: '北京 SKP 麦德龙', value: 105, color: '#52c41a' },
                { shop: '杭州大悦城超市', value: 98, color: '#faad14' },
                { shop: '深圳南山华润万家', value: 92, color: '#faad14' },
                { shop: '成都春熙路盒马', value: 76, color: '#ff4d4f' },
                { shop: '南京新街口苏果', value: 68, color: '#ff4d4f' },
              ].map(s => (
                <Col xs={24} sm={12} lg={8} key={s.shop}>
                  <div style={{ padding: 10, background: '#fafafa', borderRadius: 6 }}>
                    <div style={{ fontSize: 12, color: '#64748b', marginBottom: 6 }}>{s.shop}</div>
                    <Row align="middle" justify="space-between">
                      <Col><strong style={{ fontSize: 18, color: s.color }}>{s.value}%</strong></Col>
                      <Col>
                        <Tag color={s.value > 100 ? 'success' : s.value > 80 ? 'warning' : 'error'}>
                          {s.value > 100 ? '减钠 > 减盐' : s.value > 80 ? '接近' : '减盐占优'}
                        </Tag>
                      </Col>
                    </Row>
                    <Progress percent={Math.min(s.value, 130)} showInfo={false} strokeColor={s.color} size="small" style={{ marginTop: 6 }} />
                  </div>
                </Col>
              ))}
            </Row>
            <div style={{ marginTop: 12, fontSize: 12, color: '#94a3b8' }}>
              目标 &gt; 100%: 减钠心智已超越减盐 (示意数据, 真实数据待减钠/减盐 SKU 清单到位)
            </div>
          </Card>
        </div>

        <Row gutter={16} style={{ marginTop: 16 }}>
          <Col xs={24} lg={8}><MetricCard data={m('C12')} /></Col>
          <Col xs={24} lg={8}><MetricCard data={m('C13')} /></Col>
        </Row>
      </Card>
    </div>
  );
};

export default Binding;
