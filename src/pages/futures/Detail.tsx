// 期货品种详情页 - 同花顺风格专业版
// URL ?code=M0
// 顶部价格信息栏 + 周期/指标切换 + K 线主图 + 成交量副图 + 技术指标副图
import React, { useEffect, useMemo, useState } from 'react';
import { Breadcrumb, Card, Empty, Radio, Spin, Tag, Typography } from 'antd';
import { Link, useSearchParams } from 'react-router-dom';
import { CaretDownOutlined, CaretUpOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { API_BASE } from '../../config';
import { type FuturesBar, type FuturesSymbol, exchangeLabel, upColor, downColor, trendColor } from './types';
import KlineChart, { type Period, type Indicator } from './KlineChart';

const { Title, Text } = Typography;

const periodOptions: Array<{ value: Period; label: string }> = [
  { value: 'day', label: '日K' },
  { value: 'week', label: '周K' },
  { value: 'month', label: '月K' },
];

const indicatorOptions: Array<{ value: Indicator; label: string }> = [
  { value: 'MACD', label: 'MACD' },
  { value: 'KDJ', label: 'KDJ' },
  { value: 'RSI', label: 'RSI' },
  { value: 'BOLL', label: 'BOLL' },
];

const rangeOptions = [
  { value: 3, label: '近3月' },
  { value: 6, label: '近半年' },
  { value: 12, label: '近1年' },
  { value: 24, label: '近2年' },
  { value: 36, label: '近3年' },
  { value: 0, label: '全部' },
];

const FuturesDetail: React.FC = () => {
  const [params] = useSearchParams();
  const code = params.get('code') || 'M0';
  const [symbol, setSymbol] = useState<FuturesSymbol | null>(null);
  const [bars, setBars] = useState<FuturesBar[]>([]);
  const [loading, setLoading] = useState(true);
  const [rangeMonths, setRangeMonths] = useState(12);
  const [period, setPeriod] = useState<Period>('day');
  const [indicator, setIndicator] = useState<Indicator>('MACD');

  useEffect(() => {
    setLoading(true);
    const end = dayjs();
    let url = `${API_BASE}/api/futures/daily?code=${code}`;
    if (rangeMonths > 0) {
      const start = end.subtract(rangeMonths, 'month');
      url += `&start=${start.format('YYYY-MM-DD')}&end=${end.format('YYYY-MM-DD')}`;
    } else {
      url += `&start=2018-01-01&end=${end.format('YYYY-MM-DD')}`;
    }
    fetch(url, { credentials: 'include' })
      .then(r => r.json())
      .then(j => {
        setSymbol(j.data?.symbol || null);
        setBars(j.data?.bars || []);
      })
      .finally(() => setLoading(false));
  }, [code, rangeMonths]);

  // 顶部信息栏：最新一根 K + 涨跌 + 振幅
  const lastBar = bars[bars.length - 1];
  const prevBar = bars.length >= 2 ? bars[bars.length - 2] : lastBar;

  const stats = useMemo(() => {
    if (!lastBar || !prevBar) return null;
    const change = lastBar.close - prevBar.close;
    const changePct = prevBar.close > 0 ? (change / prevBar.close) * 100 : 0;
    const amplitude = prevBar.close > 0 ? ((lastBar.high - lastBar.low) / prevBar.close) * 100 : 0;
    const ytdBar = bars.find(b => dayjs(b.date).year() === dayjs().year());
    const ytdPct = ytdBar && ytdBar.close > 0 ? ((lastBar.close - ytdBar.close) / ytdBar.close) * 100 : 0;
    return { change, changePct, amplitude, ytdPct };
  }, [bars, lastBar, prevBar]);

  if (loading && bars.length === 0) {
    return <div style={{ textAlign: 'center', padding: 80 }}><Spin size="large" /></div>;
  }
  if (!symbol || !lastBar || !stats) {
    return <Empty description="品种不存在或暂无数据" style={{ marginBlock: 80 }} />;
  }

  const trend = trendColor(stats.change);
  const sign = stats.change >= 0 ? '+' : '';

  return (
    <div>
      <Breadcrumb
        style={{ marginBottom: 12, fontSize: 13 }}
        items={[
          { title: <Link to="/futures">原料行情</Link> },
          { title: symbol.nameCn },
        ]}
      />

      {/* 顶部价格信息栏（同花顺风格） */}
      <Card styles={{ body: { padding: '16px 20px' } }} style={{ marginBottom: 12 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 16, flexWrap: 'wrap' }}>
          <div>
            <div style={{ display: 'flex', alignItems: 'baseline', gap: 10 }}>
              <Title level={3} style={{ margin: 0 }}>{symbol.nameCn}</Title>
              <Tag style={{ borderRadius: 999 }}>{exchangeLabel[symbol.exchange] || symbol.exchange}</Tag>
              <Text type="secondary" style={{ fontSize: 13 }}>{symbol.code}</Text>
            </div>
            <div style={{ marginTop: 4 }}>
              <Tag color="blue" style={{ borderRadius: 999, fontSize: 11 }}>{symbol.businessTag}</Tag>
              <Text type="secondary" style={{ fontSize: 12 }}>·  {symbol.unit}  ·  {dayjs(lastBar.date).format('YYYY-MM-DD')}</Text>
            </div>
          </div>

          {/* 价格 + 涨跌 */}
          <div style={{ marginInlineStart: 'auto', textAlign: 'right' }}>
            <div style={{ fontSize: 32, fontWeight: 700, color: trend, lineHeight: 1, fontVariantNumeric: 'tabular-nums' }}>
              {lastBar.close.toLocaleString()}
            </div>
            <div style={{ marginTop: 4, fontSize: 14, color: trend, fontWeight: 500 }}>
              {stats.change > 0 ? <CaretUpOutlined /> : stats.change < 0 ? <CaretDownOutlined /> : null}
              {sign}{stats.change.toFixed(2)}  ({sign}{stats.changePct.toFixed(2)}%)
            </div>
          </div>
        </div>

        {/* 指标横向栏 */}
        <div style={{
          marginTop: 14, paddingTop: 14, borderTop: '1px solid #f1f5f9',
          display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(110px, 1fr))', gap: 14,
        }}>
          <Metric label="今开" value={lastBar.open.toLocaleString()} />
          <Metric label="昨收" value={prevBar.close.toLocaleString()} />
          <Metric label="最高" value={lastBar.high.toLocaleString()} color={upColor} />
          <Metric label="最低" value={lastBar.low.toLocaleString()} color={downColor} />
          <Metric label="振幅" value={stats.amplitude.toFixed(2) + '%'} />
          <Metric label="成交量" value={(lastBar.volume / 10000).toFixed(1) + ' 万手'} />
          <Metric label="持仓量" value={(lastBar.openInterest / 10000).toFixed(1) + ' 万手'} />
          <Metric label="年初至今" value={(stats.ytdPct > 0 ? '+' : '') + stats.ytdPct.toFixed(2) + '%'} color={trendColor(stats.ytdPct)} />
        </div>
      </Card>

      {/* 图表卡片 */}
      <Card styles={{ body: { padding: 12 } }}>
        {/* 控制栏 */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 16, marginBottom: 12, flexWrap: 'wrap' }}>
          <div>
            <Text type="secondary" style={{ marginInlineEnd: 8, fontSize: 12 }}>周期</Text>
            <Radio.Group
              size="small"
              value={period}
              onChange={e => setPeriod(e.target.value)}
              optionType="button"
              buttonStyle="solid"
              options={periodOptions}
            />
          </div>
          <div>
            <Text type="secondary" style={{ marginInlineEnd: 8, fontSize: 12 }}>指标</Text>
            <Radio.Group
              size="small"
              value={indicator}
              onChange={e => setIndicator(e.target.value)}
              optionType="button"
              options={indicatorOptions}
            />
          </div>
          <div style={{ marginInlineStart: 'auto' }}>
            <Text type="secondary" style={{ marginInlineEnd: 8, fontSize: 12 }}>区间</Text>
            <Radio.Group
              size="small"
              value={rangeMonths}
              onChange={e => setRangeMonths(e.target.value)}
              optionType="button"
              options={rangeOptions}
            />
          </div>
        </div>

        <KlineChart
          bars={bars}
          period={period}
          indicator={indicator}
          height={580}
          unit={symbol.unit}
          title={symbol.nameCn}
        />
      </Card>

      <div style={{ marginTop: 12, color: '#94a3b8', fontSize: 12, textAlign: 'center' }}>
        数据来源：新浪财经期货 · {symbol.code} 主连合约 · 收盘后 17:30 自动更新
      </div>
    </div>
  );
};

const Metric: React.FC<{ label: string; value: string; color?: string }> = ({ label, value, color }) => (
  <div>
    <div style={{ fontSize: 11, color: '#94a3b8', marginBottom: 4 }}>{label}</div>
    <div style={{ fontSize: 14, fontWeight: 600, color: color || '#0f172a', fontVariantNumeric: 'tabular-nums' }}>{value}</div>
  </div>
);

export default FuturesDetail;
