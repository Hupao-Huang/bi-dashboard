import React, { useState, useEffect, useCallback } from 'react';
import { Card, Table, DatePicker, Space, message, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import PageLoading from '../../components/PageLoading';
import { API_BASE } from '../../config';
import ChannelMapManager from './ChannelMapManager';
import {
  pct, kg1, int0, num0, perOrderStr, isSummaryChannel,
  type ChannelRow, type GoodsRow, type ComboRow, type ChannelStat,
} from './salesDailyReportColumns';

const { Title, Text } = Typography;

interface ReportData {
  date: string;
  channels: ChannelRow[];
  goods: GoodsRow[];
  combos: ComboRow[];
}

// 渠道块列: 平台/渠道 固定, 当日 + 当月累计 两组并排(对齐 Excel)。占比按各自组的总单数算。
const channelCols = (todayGrand: number, monthGrand: number): ColumnsType<ChannelRow> => {
  const grp = (s: (r: ChannelRow) => ChannelStat, grand: number, prefix: string, dm: string): ColumnsType<ChannelRow> => [
    { title: dm + '发货量', key: prefix + 'o', align: 'right', render: (_: unknown, r: ChannelRow) => num0(s(r).orders) },
    { title: dm + '发货件数', key: prefix + 'b', align: 'right', render: (_: unknown, r: ChannelRow) => num0(s(r).bottles) },
    { title: '占比', key: prefix + 'r', align: 'right', render: (_: unknown, r: ChannelRow) => pct(grand ? s(r).orders / grand : 0) },
    { title: '单件比', key: prefix + 'p', align: 'right', render: (_: unknown, r: ChannelRow) => perOrderStr(s(r).bottles, s(r).orders) },
    { title: '单均重量(kg)', key: prefix + 'w', align: 'right', render: (_: unknown, r: ChannelRow) => perOrderStr(s(r).weightKg, s(r).orders, 1) },
  ];
  return [
    { title: '平台', dataIndex: 'platform', key: 'platform', fixed: 'left', width: 70,
      render: (v: string, r: ChannelRow) => (isSummaryChannel(r.channel) ? '' : v) },
    { title: '渠道', dataIndex: 'channel', key: 'channel', fixed: 'left', width: 100,
      render: (v: string) => (isSummaryChannel(v) ? <b>{v}</b> : v) },
    { title: '当日', children: grp(r => r.today, todayGrand, 't', '日') },
    { title: '当月累计', children: grp(r => r.month, monthGrand, 'm', '月') },
  ];
};

const goodsCols: ColumnsType<GoodsRow> = [
  { title: '货品', dataIndex: 'goodsName', key: 'goodsName', fixed: 'left', width: 240 },
  { title: '箱规', dataIndex: 'boxQty', key: 'boxQty', align: 'right', width: 60,
    render: (v: number) => (v > 0 ? int0(v) : '—') },
  { title: '当日', children: [
    { title: '日发货量', key: 'to', align: 'right', render: (_: unknown, r: GoodsRow) => num0(r.today.orders) },
    { title: '日发货件数', key: 'tb', align: 'right', render: (_: unknown, r: GoodsRow) => num0(r.today.bottles) },
    { title: '发货箱数', key: 'tx', align: 'right', render: (_: unknown, r: GoodsRow) => int0(r.today.boxes) },
    { title: '发货托数', key: 'tp', align: 'right', render: (_: unknown, r: GoodsRow) => (r.today.pallets > 0 ? r.today.pallets.toFixed(2) : '—') },
  ] },
  { title: '当月累计', children: [
    { title: '月发货量', key: 'mo', align: 'right', render: (_: unknown, r: GoodsRow) => num0(r.month.orders) },
    { title: '月发货件数', key: 'mb', align: 'right', render: (_: unknown, r: GoodsRow) => num0(r.month.bottles) },
    { title: '发货箱数', key: 'mx', align: 'right', render: (_: unknown, r: GoodsRow) => int0(r.month.boxes) },
    { title: '发货托数', key: 'mp', align: 'right', render: (_: unknown, r: GoodsRow) => (r.month.pallets > 0 ? r.month.pallets.toFixed(2) : '—') },
  ] },
];

const comboCols: ColumnsType<ComboRow> = [
  { title: '货品组合', dataIndex: 'display', key: 'display', fixed: 'left', width: 320 },
  { title: '当日', children: [
    { title: '日发货量', key: 'to', align: 'right', render: (_: unknown, r: ComboRow) => num0(r.today.orders) },
    { title: '日发货件数', key: 'tb', align: 'right', render: (_: unknown, r: ComboRow) => num0(r.today.bottles) },
    { title: '重量(kg)', key: 'tw', align: 'right', render: (_: unknown, r: ComboRow) => kg1(r.today.weightKg) },
  ] },
  { title: '当月累计', children: [
    { title: '月发货量', key: 'mo', align: 'right', render: (_: unknown, r: ComboRow) => num0(r.month.orders) },
    { title: '月发货件数', key: 'mb', align: 'right', render: (_: unknown, r: ComboRow) => num0(r.month.bottles) },
    { title: '重量(kg)', key: 'mw', align: 'right', render: (_: unknown, r: ComboRow) => kg1(r.month.weightKg) },
  ] },
];

const grandOrders = (rows: ChannelRow[], which: 'today' | 'month'): number => {
  const g = rows.find(r => r.channel === '总计');
  return g ? g[which].orders : 0;
};

const SalesDailyReport: React.FC = () => {
  const [date, setDate] = useState<string>('');
  const [data, setData] = useState<ReportData | null>(null);
  const [loading, setLoading] = useState(true);

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

  const channels = data?.channels || [];
  const goods = data?.goods || [];
  const combos = data?.combos || [];

  return (
    <div style={{ padding: 16 }}>
      <Space style={{ marginBottom: 16 }} wrap>
        <Title level={4} style={{ margin: 0 }}>销售日报</Title>
        <DatePicker
          value={date ? dayjs(date) : null}
          onChange={(d) => { const s = d ? d.format('YYYY-MM-DD') : ''; setDate(s); fetchData(s); }}
          allowClear={false}
        />
        <Text type="secondary">发货口径 · 仅统计 4 个成品仓 · 只算销售单 · TOP10 按当月累计排</Text>
      </Space>

      <Card title="渠道汇总" size="small" style={{ marginBottom: 16 }} loading={loading}
        extra={<ChannelMapManager onSaved={() => fetchData(date)} />}>
        <Table
          rowKey={(r) => r.platform + '|' + r.channel}
          columns={channelCols(grandOrders(channels, 'today'), grandOrders(channels, 'month'))}
          dataSource={channels}
          pagination={false}
          size="small"
          scroll={{ x: 'max-content' }}
        />
      </Card>

      <Card title="TOP10 单品" size="small" style={{ marginBottom: 16 }} loading={loading}>
        <Table rowKey="goodsNo" columns={goodsCols} dataSource={goods} pagination={false} size="small" scroll={{ x: 'max-content' }} />
      </Card>

      <Card title="TOP10 货品组合" size="small" loading={loading}>
        <Table rowKey="display" columns={comboCols} dataSource={combos} pagination={false} size="small" scroll={{ x: 'max-content' }} />
      </Card>
    </div>
  );
};

export default SalesDailyReport;
