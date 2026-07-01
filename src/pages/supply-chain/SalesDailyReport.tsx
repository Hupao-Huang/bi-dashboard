import React, { useState, useEffect, useCallback } from 'react';
import { Card, Table, DatePicker, Space, Segmented, message, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import PageLoading from '../../components/PageLoading';
import { API_BASE } from '../../config';
import ChannelMapManager from './ChannelMapManager';
import {
  pct, kg1, int0, num0, perOrderStr, isSummaryChannel,
  type ChannelRow, type GoodsRow, type ComboRow,
} from './salesDailyReportColumns';

const { Title, Text } = Typography;

interface ReportData {
  date: string;
  channelToday: ChannelRow[]; channelMonth: ChannelRow[];
  goodsToday: GoodsRow[]; goodsMonth: GoodsRow[];
  comboToday: ComboRow[]; comboMonth: ComboRow[];
}

// 渠道块列(占比按传入的总单数算)
const channelCols = (grandOrders: number): ColumnsType<ChannelRow> => [
  { title: '平台', dataIndex: 'platform', key: 'platform',
    render: (v: string, r: ChannelRow) => (isSummaryChannel(r.channel) ? '' : v) },
  { title: '渠道', dataIndex: 'channel', key: 'channel',
    render: (v: string) => (isSummaryChannel(v) ? <b>{v}</b> : v) },
  { title: '发货单数', dataIndex: 'orders', key: 'orders', align: 'right', render: num0 },
  { title: '发货量(单瓶)', dataIndex: 'bottles', key: 'bottles', align: 'right', render: num0 },
  { title: '占比', key: 'ratio', align: 'right',
    render: (_: unknown, r: ChannelRow) => pct(grandOrders ? r.orders / grandOrders : 0) },
  { title: '单件比', key: 'ppo', align: 'right',
    render: (_: unknown, r: ChannelRow) => perOrderStr(r.bottles, r.orders) },
  { title: '单均重量(kg)', key: 'wpo', align: 'right',
    render: (_: unknown, r: ChannelRow) => perOrderStr(r.weightKg, r.orders, 1) },
];

const goodsCols: ColumnsType<GoodsRow> = [
  { title: '货品', dataIndex: 'goodsName', key: 'goodsName' },
  { title: '发货单数', dataIndex: 'orders', key: 'orders', align: 'right', render: num0 },
  { title: '发货量(单瓶)', dataIndex: 'bottles', key: 'bottles', align: 'right', render: num0 },
  { title: '发货箱数', dataIndex: 'boxes', key: 'boxes', align: 'right', render: int0 },
  { title: '发货托数', dataIndex: 'pallets', key: 'pallets', align: 'right',
    render: (v: number) => (v > 0 ? v.toFixed(2) : '—') },
];

const comboCols: ColumnsType<ComboRow> = [
  { title: '货品组合', dataIndex: 'display', key: 'display' },
  { title: '订单数', dataIndex: 'orders', key: 'orders', align: 'right', render: num0 },
  { title: '发货量(单瓶)', dataIndex: 'bottles', key: 'bottles', align: 'right', render: num0 },
  { title: '重量(kg)', dataIndex: 'weightKg', key: 'weightKg', align: 'right', render: kg1 },
];

const grandOf = (rows: ChannelRow[]): number => {
  const g = rows.find(r => r.channel === '总计');
  return g ? g.orders : 0;
};

const SalesDailyReport: React.FC = () => {
  const [date, setDate] = useState<string>('');
  const [data, setData] = useState<ReportData | null>(null);
  const [loading, setLoading] = useState(true);
  const [scope, setScope] = useState<'today' | 'month'>('today'); // 当日 / 当月累计

  const fetchData = useCallback((d: string) => {
    setLoading(true);
    const q = d ? `?date=${d}` : '';
    fetch(`${API_BASE}/api/supply-chain/sales-daily-report${q}`, { credentials: 'include' })
      .then(r => r.json())
      .then(j => {
        if (j.code === 200) {
          setData(j.data);
          if (!d && j.data?.date) setDate(j.data.date);
        } else {
          message.error(j.msg || '加载失败');
        }
      })
      .catch(err => message.error(`加载失败: ${err instanceof Error ? err.message : String(err)}`))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => { fetchData(''); }, [fetchData]);

  if (loading && !data) return <PageLoading />;

  const ch = scope === 'today' ? data?.channelToday : data?.channelMonth;
  const gd = scope === 'today' ? data?.goodsToday : data?.goodsMonth;
  const cb = scope === 'today' ? data?.comboToday : data?.comboMonth;

  return (
    <div style={{ padding: 16 }}>
      <Space style={{ marginBottom: 16 }} wrap>
        <Title level={4} style={{ margin: 0 }}>销售日报</Title>
        <DatePicker
          value={date ? dayjs(date) : null}
          onChange={(d) => { const s = d ? d.format('YYYY-MM-DD') : ''; setDate(s); fetchData(s); }}
          allowClear={false}
        />
        <Segmented
          value={scope}
          onChange={(v) => setScope(v as 'today' | 'month')}
          options={[{ label: '当日', value: 'today' }, { label: '当月累计', value: 'month' }]}
        />
        <Text type="secondary">发货口径 · 仅统计 4 个成品仓 · 只算销售单</Text>
      </Space>

      <Card title="渠道汇总" size="small" style={{ marginBottom: 16 }} loading={loading}
        extra={<ChannelMapManager onSaved={() => fetchData(date)} />}>
        <Table
          rowKey={(r) => r.platform + '|' + r.channel}
          columns={channelCols(grandOf(ch || []))}
          dataSource={ch || []}
          pagination={false}
          size="small"
        />
      </Card>

      <Card title="TOP10 单品" size="small" style={{ marginBottom: 16 }} loading={loading}>
        <Table rowKey="goodsNo" columns={goodsCols} dataSource={gd || []} pagination={false} size="small" />
      </Card>

      <Card title="TOP10 货品组合" size="small" loading={loading}>
        <Table rowKey="display" columns={comboCols} dataSource={cb || []} pagination={false} size="small" />
      </Card>
    </div>
  );
};

export default SalesDailyReport;
