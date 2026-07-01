import React, { useState, useEffect, useCallback } from 'react';
import { Card, Table, DatePicker, Space, message, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import PageLoading from '../../components/PageLoading';
import { API_BASE } from '../../config';
import ChannelMapManager from './ChannelMapManager';
import PackSpecManager from './PackSpecManager';
import './SalesDailyReport.css';
import {
  pct, int0, num0, perOrderStr, isSummaryChannel, yoy, signPct,
  type ChannelRow, type GoodsRow, type ComboRow,
} from './salesDailyReportColumns';

const { Title, Text } = Typography;

interface ReportData {
  date: string;
  channels: ChannelRow[];
  goods: GoodsRow[];
  combos: ComboRow[];
}

// 环比单元格: 涨绿跌红(语义色, 非装饰)
const yoyCell = (cur: number, prev: number) => {
  const v = yoy(cur, prev);
  const color = v === null ? undefined : v >= 0 ? '#3f8600' : '#cf1322';
  return <span style={{ color }}>{signPct(v)}</span>;
};

// 渠道块列(照 Excel): 平台/渠道 ‖ 当日[...] ‖ 当月[...]
// 平台名只在该平台的「合计」行显示(明细行留空), 避免每行重复。当月组加 sdr-month-col 底色区分当日。
const channelCols = (todayGrand: number, monthGrand: number): ColumnsType<ChannelRow> => [
  { title: '平台', dataIndex: 'platform', key: 'platform', fixed: 'left', width: 64,
    render: (v: string, r: ChannelRow) => {
      // 只在「X合计」行显示平台名(加粗), 明细行/总计行留空
      if (r.channel.endsWith('合计')) return <b>{v}</b>;
      return '';
    } },
  { title: '渠道', dataIndex: 'channel', key: 'channel', fixed: 'left', width: 90,
    render: (v: string) => (isSummaryChannel(v) ? <b>{v}</b> : v) },
  { title: '当日', children: [
    { title: '日发货量', key: 'to', align: 'right', render: (_: unknown, r: ChannelRow) => num0(r.today.orders) },
    { title: '日发货量占比', key: 'tr', align: 'right', render: (_: unknown, r: ChannelRow) => pct(todayGrand ? r.today.orders / todayGrand : 0) },
    { title: '日发货量环比', key: 'ty', align: 'right', render: (_: unknown, r: ChannelRow) => yoyCell(r.today.orders, r.prevOrders) },
    { title: '日发货件数', key: 'tb', align: 'right', render: (_: unknown, r: ChannelRow) => num0(r.today.bottles) },
    { title: '单件比', key: 'tp', align: 'right', render: (_: unknown, r: ChannelRow) => perOrderStr(r.today.bottles, r.today.orders) },
    { title: '单均重量(kg)', key: 'tw', align: 'right', render: (_: unknown, r: ChannelRow) => perOrderStr(r.today.weightKg, r.today.orders, 1) },
  ] },
  { title: '当月累计', className: 'sdr-month-col', children: [
    { title: '月发货量', key: 'mo', align: 'right', className: 'sdr-month-col', render: (_: unknown, r: ChannelRow) => num0(r.month.orders) },
    { title: '月发货量占比', key: 'mr', align: 'right', className: 'sdr-month-col', render: (_: unknown, r: ChannelRow) => pct(monthGrand ? r.month.orders / monthGrand : 0) },
    { title: '月发货件数', key: 'mb', align: 'right', className: 'sdr-month-col', render: (_: unknown, r: ChannelRow) => num0(r.month.bottles) },
    { title: '单件比', key: 'mp', align: 'right', className: 'sdr-month-col', render: (_: unknown, r: ChannelRow) => perOrderStr(r.month.bottles, r.month.orders) },
    { title: '单均重量(kg)', key: 'mw', align: 'right', className: 'sdr-month-col', render: (_: unknown, r: ChannelRow) => perOrderStr(r.month.weightKg, r.month.orders, 1) },
  ] },
];

// 单品块列(照 Excel): 货品 ‖ 当日[日发货件数·日发货量占比·日发货量环比·箱规·发货箱数·发货托数] ‖ 当月[月发货件数·月发货量占比·日均发货箱数·月发货箱数·月发货托数]
// 单品占比分母=渠道总计发货件数; 单品环比按发货件数; 日均发货箱数=月发货箱数÷本月天数
const goodsCols = (todayBottleGrand: number, monthBottleGrand: number, dayOfMonth: number): ColumnsType<GoodsRow> => [
  { title: '货品', dataIndex: 'goodsName', key: 'goodsName', fixed: 'left', width: 230,
    render: (v: string, r: GoodsRow) => (
      <div>
        <div>{v}</div>
        <div style={{ color: 'rgba(0,0,0,0.45)', fontSize: 12 }}>{r.goodsNo}</div>
      </div>
    ) },
  { title: '当日', children: [
    { title: '日发货件数', key: 'tb', align: 'right', render: (_: unknown, r: GoodsRow) => num0(r.today.bottles) },
    { title: '日发货量占比', key: 'tr', align: 'right', render: (_: unknown, r: GoodsRow) => pct(todayBottleGrand ? r.today.bottles / todayBottleGrand : 0) },
    { title: '日发货量环比', key: 'ty', align: 'right', render: (_: unknown, r: GoodsRow) => yoyCell(r.today.bottles, r.prevBottles) },
    { title: '箱规', dataIndex: 'boxQty', key: 'boxQty', align: 'right', render: (v: number) => (v > 0 ? int0(v) : '—') },
    { title: '发货箱数', key: 'tx', align: 'right', render: (_: unknown, r: GoodsRow) => int0(r.today.boxes) },
    { title: '发货托数', key: 'tp', align: 'right', render: (_: unknown, r: GoodsRow) => (r.today.pallets > 0 ? r.today.pallets.toFixed(2) : '—') },
  ] },
  { title: '当月累计', className: 'sdr-month-col', children: [
    { title: '月发货件数', key: 'mb', align: 'right', className: 'sdr-month-col', render: (_: unknown, r: GoodsRow) => num0(r.month.bottles) },
    { title: '月发货量占比', key: 'mr', align: 'right', className: 'sdr-month-col', render: (_: unknown, r: GoodsRow) => pct(monthBottleGrand ? r.month.bottles / monthBottleGrand : 0) },
    { title: '日均发货箱数', key: 'mavg', align: 'right', className: 'sdr-month-col', render: (_: unknown, r: GoodsRow) => (dayOfMonth ? (r.month.boxes / dayOfMonth).toFixed(2) : '—') },
    { title: '月发货箱数', key: 'mx', align: 'right', className: 'sdr-month-col', render: (_: unknown, r: GoodsRow) => int0(r.month.boxes) },
    { title: '月发货托数', key: 'mp', align: 'right', className: 'sdr-month-col', render: (_: unknown, r: GoodsRow) => (r.month.pallets > 0 ? r.month.pallets.toFixed(2) : '—') },
  ] },
];

// 组合块列(照 Excel): 货品组合 ‖ 当日[日发货量·占比·环比·日发货件数·单件比·单均重量] ‖ 当月[月发货量·占比·月发货件数·单件比·单均重量]
// 组合占比分母=渠道总计发货量(订单数); 环比=当日发货量÷前一发货日
const comboCols = (todayGrand: number, monthGrand: number): ColumnsType<ComboRow> => [
  { title: '货品组合', dataIndex: 'display', key: 'display', fixed: 'left', width: 300 },
  { title: '当日', children: [
    { title: '日发货量', key: 'to', align: 'right', render: (_: unknown, r: ComboRow) => num0(r.today.orders) },
    { title: '日发货量占比', key: 'tr', align: 'right', render: (_: unknown, r: ComboRow) => pct(todayGrand ? r.today.orders / todayGrand : 0) },
    { title: '日发货量环比', key: 'ty', align: 'right', render: (_: unknown, r: ComboRow) => yoyCell(r.today.orders, r.prevOrders) },
    { title: '日发货件数', key: 'tb', align: 'right', render: (_: unknown, r: ComboRow) => num0(r.today.bottles) },
    { title: '单件比', key: 'tp', align: 'right', render: (_: unknown, r: ComboRow) => perOrderStr(r.today.bottles, r.today.orders) },
    { title: '单均重量(kg)', key: 'tw', align: 'right', render: (_: unknown, r: ComboRow) => perOrderStr(r.today.weightKg, r.today.orders, 1) },
  ] },
  { title: '当月累计', className: 'sdr-month-col', children: [
    { title: '月发货量', key: 'mo', align: 'right', className: 'sdr-month-col', render: (_: unknown, r: ComboRow) => num0(r.month.orders) },
    { title: '月发货量占比', key: 'mr', align: 'right', className: 'sdr-month-col', render: (_: unknown, r: ComboRow) => pct(monthGrand ? r.month.orders / monthGrand : 0) },
    { title: '月发货件数', key: 'mb', align: 'right', className: 'sdr-month-col', render: (_: unknown, r: ComboRow) => num0(r.month.bottles) },
    { title: '单件比', key: 'mp', align: 'right', className: 'sdr-month-col', render: (_: unknown, r: ComboRow) => perOrderStr(r.month.bottles, r.month.orders) },
    { title: '单均重量(kg)', key: 'mw', align: 'right', className: 'sdr-month-col', render: (_: unknown, r: ComboRow) => perOrderStr(r.month.weightKg, r.month.orders, 1) },
  ] },
];

// 从渠道总计行取分母(订单数 / 发货件数)
const grandFrom = (rows: ChannelRow[], which: 'today' | 'month', field: 'orders' | 'bottles'): number => {
  const g = rows.find(r => r.channel === '总计');
  return g ? g[which][field] : 0;
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
  // 占比分母(渠道总计行) + 本月天数(日均箱数用 = 所选日是本月第几天)
  const todayOrdersGrand = grandFrom(channels, 'today', 'orders');
  const monthOrdersGrand = grandFrom(channels, 'month', 'orders');
  const todayBottleGrand = grandFrom(channels, 'today', 'bottles');
  const monthBottleGrand = grandFrom(channels, 'month', 'bottles');
  const dayOfMonth = date ? dayjs(date).date() : 0;

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
          columns={channelCols(todayOrdersGrand, monthOrdersGrand)}
          dataSource={channels}
          pagination={false}
          size="small"
          scroll={{ x: 'max-content' }}
          rowClassName={(r) => (r.channel === '总计' ? 'sdr-grand-row' : r.channel.endsWith('合计') ? 'sdr-subtotal-row' : '')}
        />
      </Card>

      <Card title="TOP10 单品" size="small" style={{ marginBottom: 16 }} loading={loading}
        extra={<PackSpecManager onSaved={() => fetchData(date)} />}>
        <Table rowKey="goodsNo" columns={goodsCols(todayBottleGrand, monthBottleGrand, dayOfMonth)} dataSource={goods} pagination={false} size="small" scroll={{ x: 'max-content' }} />
      </Card>

      <Card title="TOP10 货品组合" size="small" loading={loading}>
        <Table rowKey="display" columns={comboCols(todayOrdersGrand, monthOrdersGrand)} dataSource={combos} pagination={false} size="small" scroll={{ x: 'max-content' }} />
      </Card>
    </div>
  );
};

export default SalesDailyReport;
