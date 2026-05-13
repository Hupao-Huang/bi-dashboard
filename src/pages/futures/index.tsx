// 原料行情·总览页
// 顶部 3 类卡片网格（主要原料/包材/大宗），每张卡片含品种名 + 当前价 + 涨跌 + 迷你折线图
// 下方涨跌幅排行榜
import React, { useEffect, useMemo, useState } from 'react';
import { Card, Col, Row, Spin, Tabs, Tag, Tooltip, Typography, Empty } from 'antd';
import { CaretDownOutlined, CaretUpOutlined, ReloadOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { Link } from 'react-router-dom';
import ReactECharts from '../../components/Chart';
import { API_BASE } from '../../config';
import { type FuturesQuote, type FuturesSymbol, categoryLabel, categoryColor, exchangeLabel, upColor, downColor, trendColor } from './types';

const { Title, Text } = Typography;

interface QuoteCardProps {
  q: FuturesQuote;
}

const QuoteCard: React.FC<QuoteCardProps> = ({ q }) => {
  const color = trendColor(q.change);
  const sign = q.change > 0 ? '+' : '';

  const miniOption = useMemo(() => ({
    grid: { left: 0, right: 0, top: 4, bottom: 4 },
    xAxis: { type: 'category', show: false, data: q.miniTrend.map((_, i) => i) },
    yAxis: { type: 'value', show: false, scale: true },
    tooltip: { show: false },
    series: [{
      type: 'line',
      data: q.miniTrend,
      smooth: true,
      showSymbol: false,
      lineStyle: { width: 1.5, color },
      areaStyle: { color, opacity: 0.12 },
    }],
  }), [q.miniTrend, color]);

  return (
    <Link to={`/futures/detail?code=${q.code}`} style={{ textDecoration: 'none' }}>
      <Card
        hoverable
        styles={{ body: { padding: 16 } }}
        style={{ height: '100%', borderRadius: 12, borderTop: `3px solid ${categoryColor[q.category]}` }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
          <div>
            <div style={{ fontSize: 16, fontWeight: 600, color: '#0f172a' }}>{q.nameCn}</div>
            <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 2 }}>
              {exchangeLabel[q.exchange] || q.exchange} · {q.code}
            </div>
          </div>
          <Tag style={{ borderRadius: 999, fontSize: 11, marginInlineEnd: 0 }}>{q.businessTag || categoryLabel[q.category]}</Tag>
        </div>
        <div style={{ marginTop: 14, display: 'flex', alignItems: 'baseline', gap: 8 }}>
          <span style={{ fontSize: 26, fontWeight: 600, color, fontVariantNumeric: 'tabular-nums' }}>{q.close.toLocaleString()}</span>
          <span style={{ fontSize: 12, color: '#94a3b8' }}>{q.unit}</span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 4, color, fontSize: 13, fontWeight: 500 }}>
          {q.change > 0 ? <CaretUpOutlined /> : q.change < 0 ? <CaretDownOutlined /> : null}
          <span>{sign}{q.change.toFixed(2)}</span>
          <span>({sign}{q.changePct.toFixed(2)}%)</span>
        </div>
        <div style={{ height: 50, marginTop: 8 }}>
          <ReactECharts option={miniOption} style={{ height: '100%' }} notMerge />
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: '#94a3b8', marginTop: 6 }}>
          <span>开 {q.open.toLocaleString()}</span>
          <span>高 <span style={{ color: upColor }}>{q.high.toLocaleString()}</span></span>
          <span>低 <span style={{ color: downColor }}>{q.low.toLocaleString()}</span></span>
        </div>
      </Card>
    </Link>
  );
};

const FuturesOverview: React.FC = () => {
  const [quotes, setQuotes] = useState<FuturesQuote[]>([]);
  const [loading, setLoading] = useState(true);

  const fetchData = async () => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/futures/quotes`, { credentials: 'include' });
      const j = await res.json();
      setQuotes(j.data || []);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchData(); }, []);

  // 按分类拆
  const grouped = useMemo(() => {
    const out: Record<FuturesSymbol['category'], FuturesQuote[]> = { material: [], package: [], macro: [] };
    quotes.forEach(q => out[q.category]?.push(q));
    return out;
  }, [quotes]);

  // 涨跌排行
  const { topGainers, topLosers } = useMemo(() => {
    const sorted = [...quotes].sort((a, b) => b.changePct - a.changePct);
    return {
      topGainers: sorted.filter(q => q.changePct > 0).slice(0, 5),
      topLosers: sorted.filter(q => q.changePct < 0).slice(-5).reverse(),
    };
  }, [quotes]);

  // 数据截止时间（取最新的 tradeDate）
  const lastTradeDate = useMemo(() => {
    if (quotes.length === 0) return '';
    const dates = quotes.map(q => dayjs(q.tradeDate).format('YYYY-MM-DD')).sort();
    return dates[dates.length - 1];
  }, [quotes]);

  if (loading && quotes.length === 0) {
    return <div style={{ textAlign: 'center', padding: 80 }}><Spin size="large" /></div>;
  }

  if (quotes.length === 0) {
    return <Empty description="暂无行情数据。请等待今晚 17:30 自动同步或联系管理员。" style={{ marginBlock: 80 }} />;
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16, flexWrap: 'wrap', gap: 8 }}>
        <div>
          <Title level={3} style={{ margin: 0 }}>原料行情总览</Title>
          <Text type="secondary" style={{ fontSize: 13 }}>
            数据来源：新浪财经期货 · 数据截止 {lastTradeDate}
          </Text>
        </div>
        <Tooltip title="重新拉取最新行情">
          <a onClick={fetchData} style={{ color: '#1e40af', fontSize: 13, cursor: 'pointer' }}>
            <ReloadOutlined /> 刷新
          </a>
        </Tooltip>
      </div>

      {/* 三类品种网格 */}
      {(['material', 'package', 'macro'] as const).map(cat => {
        const list = grouped[cat];
        if (list.length === 0) return null;
        return (
          <div key={cat} style={{ marginBottom: 24 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 12 }}>
              <div style={{ width: 4, height: 18, background: categoryColor[cat], borderRadius: 2 }} />
              <Title level={5} style={{ margin: 0 }}>{categoryLabel[cat]}</Title>
              <Tag style={{ borderRadius: 999 }}>{list.length} 个品种</Tag>
            </div>
            <Row gutter={[12, 12]}>
              {list.map(q => (
                <Col xs={24} sm={12} md={8} lg={6} xl={6} key={q.code}>
                  <QuoteCard q={q} />
                </Col>
              ))}
            </Row>
          </div>
        );
      })}

      {/* 排行榜 */}
      <Row gutter={[16, 16]} style={{ marginTop: 8 }}>
        <Col xs={24} md={12}>
          <Card title={<span style={{ color: upColor }}><CaretUpOutlined /> 涨幅 TOP5</span>} styles={{ body: { padding: 0 } }}>
            <RankList list={topGainers} />
          </Card>
        </Col>
        <Col xs={24} md={12}>
          <Card title={<span style={{ color: downColor }}><CaretDownOutlined /> 跌幅 TOP5</span>} styles={{ body: { padding: 0 } }}>
            <RankList list={topLosers} />
          </Card>
        </Col>
      </Row>
    </div>
  );
};

const RankList: React.FC<{ list: FuturesQuote[] }> = ({ list }) => {
  if (list.length === 0) {
    return <div style={{ padding: 20, color: '#94a3b8', textAlign: 'center' }}>暂无</div>;
  }
  return (
    <div>
      {list.map((q, idx) => (
        <Link
          key={q.code}
          to={`/futures/detail?code=${q.code}`}
          style={{
            display: 'flex', alignItems: 'center', padding: '10px 16px',
            borderTop: idx === 0 ? 'none' : '1px solid #f1f5f9',
            textDecoration: 'none', color: '#0f172a',
          }}
        >
          <span style={{
            width: 22, height: 22, borderRadius: 6, background: '#f1f5f9',
            display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
            fontSize: 11, color: '#64748b', marginInlineEnd: 10, flex: 'none', fontWeight: 600,
          }}>{idx + 1}</span>
          <span style={{ flex: 1, fontWeight: 500 }}>{q.nameCn}</span>
          <span style={{ marginInlineEnd: 12, color: '#475569', fontVariantNumeric: 'tabular-nums' }}>{q.close.toLocaleString()}</span>
          <span style={{ color: trendColor(q.change), fontWeight: 500, minWidth: 70, textAlign: 'right' }}>
            {q.changePct > 0 ? '+' : ''}{q.changePct.toFixed(2)}%
          </span>
        </Link>
      ))}
    </div>
  );
};

export default FuturesOverview;
