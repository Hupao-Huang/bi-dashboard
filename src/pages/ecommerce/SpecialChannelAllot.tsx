import React, { useEffect, useState, useCallback } from 'react';
import { Card, Row, Col, Table, Statistic, Tag, Modal, Alert, Tabs, Empty, message } from 'antd';
import { ExclamationCircleOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import PageLoading from '../../components/PageLoading';
import DateFilter from '../../components/DateFilter';
import { API_BASE } from '../../config';
import SpecialChannelPriceManager from '../../components/SpecialChannelPriceManager';

interface ChannelSummary {
  channelKey: string;
  channelName: string;
  completedOrders: number;
  completedSales: number;
  pendingOrders: number;
  pendingSales: number;
  totalOrders: number;
  totalSales: number;
}

interface OrderRow {
  allocateNo: string;
  channelKey: string;
  inWarehouseName: string;
  inStatus: number;
  status: number;
  gmtCreate: string;
  auditDate: string;
  gmtModified: string;
  statDate: string;
  skuCount: number;
  excelSales: number;
  apiSales: number;
}

interface DetailRow {
  goodsNo: string;
  skuBarcode: string;
  goodsName: string;
  skuName: string;
  skuCount: number;
  outCount: number;
  inCount: number;
  excelPrice: number;
  excelAmount: number;
  skuPrice: number;
  apiAmount: number;
  priceSource: string;
}

interface MissingRow {
  channelKey: string;
  goodsNo: string;
  barcode: string;
  goodsName: string;
  allocateCnt: number;
  qtyTotal: number;
}

// 单据状态 (status): 调拨销售按"审核通过"口径确认, 这里看单据审没审
const statusLabel = (s: number): { text: string; color: string } => {
  switch (s) {
    case 0: return { text: '草稿', color: 'default' };
    case 1: return { text: '待审', color: 'orange' };
    case 2: return { text: '已审', color: 'blue' };
    case 3: return { text: '已关闭', color: 'default' };
    case 10: return { text: '审中', color: 'gold' };
    case 20: return { text: '已完成', color: 'green' };
    default: return { text: `${s}`, color: 'default' };
  }
};

const fmtMoney = (n: number) => n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
const fmtWan = (n: number) => `≈${(n / 10000).toFixed(1)}万`;

const SpecialChannelAllot: React.FC = () => {
  const [startDate, setStartDate] = useState<string>(dayjs().subtract(60, 'day').format('YYYY-MM-DD'));
  const [endDate, setEndDate] = useState<string>(dayjs().subtract(1, 'day').format('YYYY-MM-DD'));
  const [summary, setSummary] = useState<ChannelSummary[]>([]);
  const [orders, setOrders] = useState<OrderRow[]>([]);
  const [missing, setMissing] = useState<MissingRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [activeChannel, setActiveChannel] = useState<string>('全部');
  const [detailOpen, setDetailOpen] = useState(false);
  const [detailNo, setDetailNo] = useState('');
  const [details, setDetails] = useState<DetailRow[]>([]);
  const [detailLoading, setDetailLoading] = useState(false);

  const fetchAll = useCallback(() => {
    setLoading(true);
    fetch(`${API_BASE}/api/special-channel-allot/summary?start=${startDate}&end=${endDate}&dept=ecommerce`, { credentials: 'include' })
      .then(r => r.json())
      .then(j => {
        if (j.code === 200) {
          setSummary(j.data?.summary || []);
          setOrders(j.data?.orders || []);
          setMissing(j.data?.missing || []);
        } else {
          message.error(j.msg || '加载失败');
        }
      })
      .catch(err => {
        // v1.72.0: 网络错误用户能看到提示, 不再"永远转圈"
        message.error(`加载失败: ${err instanceof Error ? err.message : String(err)}`);
      })
      .finally(() => setLoading(false));
  }, [startDate, endDate]);

  useEffect(() => { fetchAll(); }, [fetchAll]);

  const handleDateChange = (s: string, e: string) => {
    setStartDate(s);
    setEndDate(e);
  };

  const openDetail = (allocateNo: string) => {
    setDetailNo(allocateNo);
    setDetailOpen(true);
    setDetailLoading(true);
    fetch(`${API_BASE}/api/special-channel-allot/details?allocate_no=${encodeURIComponent(allocateNo)}`, { credentials: 'include' })
      .then(r => r.json())
      .then(j => {
        if (j.code === 200) {
          setDetails(j.data?.list || []);
        } else {
          message.error(j.msg || '加载详情失败');
        }
      })
      .catch(err => {
        // v1.72.0: catch 提示
        message.error(`加载详情失败: ${err instanceof Error ? err.message : String(err)}`);
      })
      .finally(() => setDetailLoading(false));
  };

  if (loading && orders.length === 0) return <PageLoading />;

  const isAll = activeChannel === '全部';
  const channelOrders = isAll ? orders : orders.filter(o => o.channelKey === activeChannel);
  const channelSummary = summary.find(s => s.channelKey === activeChannel);
  // 全部合集: 各渠道总单数 / 总销售额相加
  const allOrdersCnt = summary.reduce((sum, s) => sum + s.totalOrders, 0);
  const allSalesSum = summary.reduce((sum, s) => sum + s.totalSales, 0);
  const dispOrders = isAll ? allOrdersCnt : (channelSummary?.totalOrders ?? 0);
  const dispSales = isAll ? allSalesSum : (channelSummary?.totalSales ?? 0);

  const orderColumns = [
    ...(isAll ? [{ title: '渠道', dataIndex: 'channelKey', key: 'channelKey', width: 80,
      render: (v: string) => <Tag color="blue">{v}</Tag> }] : []),
    { title: '调拨单号', dataIndex: 'allocateNo', key: 'allocateNo', width: 200,
      render: (v: string) => <a onClick={() => openDetail(v)}>{v}</a> },
    { title: '入库仓', dataIndex: 'inWarehouseName', key: 'inWarehouseName', width: 220, ellipsis: true },
    { title: '单据状态', dataIndex: 'status', key: 'status', width: 100,
      render: (s: number) => { const l = statusLabel(s); return <Tag color={l.color}>{l.text}</Tag>; } },
    { title: '创建时间', dataIndex: 'gmtCreate', key: 'gmtCreate', width: 140 },
    { title: '审核时间', dataIndex: 'auditDate', key: 'auditDate', width: 140 },
    { title: '入库完成时间', dataIndex: 'gmtModified', key: 'gmtModified', width: 140 },
    { title: '销售统计日', dataIndex: 'statDate', key: 'statDate', width: 110,
      render: (v: string) => v || <span style={{ color: '#bbb' }}>—</span> },
    { title: 'SKU 数', dataIndex: 'skuCount', key: 'skuCount', width: 80, align: 'right' as const },
    { title: '销售额(Excel价)', dataIndex: 'excelSales', key: 'excelSales', width: 150, align: 'right' as const,
      render: (v: number) => <strong style={{ color: '#1677ff' }}>{fmtMoney(v)}</strong>, sorter: (a: OrderRow, b: OrderRow) => a.excelSales - b.excelSales },
    { title: '吉客云接口金额', dataIndex: 'apiSales', key: 'apiSales', width: 140, align: 'right' as const,
      render: (v: number) => <span style={{ color: '#64748b' }}>{fmtMoney(v)}</span> },
  ];

  const detailColumns = [
    { title: '商品编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 120 },
    { title: '条码', dataIndex: 'skuBarcode', key: 'skuBarcode', width: 140 },
    { title: '名称', dataIndex: 'goodsName', key: 'goodsName', width: 240, ellipsis: true },
    { title: '调拨数量', dataIndex: 'skuCount', key: 'skuCount', width: 90, align: 'right' as const,
      render: (v: number) => v.toFixed(0) },
    { title: 'Excel 单价', dataIndex: 'excelPrice', key: 'excelPrice', width: 100, align: 'right' as const,
      render: (v: number, r: DetailRow) => r.priceSource === 'missing'
        ? <Tag color="red">缺失</Tag>
        : <span>{v.toFixed(2)}</span> },
    { title: 'Excel 销售额', dataIndex: 'excelAmount', key: 'excelAmount', width: 120, align: 'right' as const,
      render: (v: number) => <strong style={{ color: '#1677ff' }}>{fmtMoney(v)}</strong> },
    { title: '接口单价', dataIndex: 'skuPrice', key: 'skuPrice', width: 90, align: 'right' as const,
      render: (v: number) => <span style={{ color: '#64748b' }}>{v.toFixed(2)}</span> },
    { title: '接口金额', dataIndex: 'apiAmount', key: 'apiAmount', width: 110, align: 'right' as const,
      render: (v: number) => <span style={{ color: '#64748b' }}>{fmtMoney(v)}</span> },
  ];

  return (
    <div style={{ padding: 16 }}>
      <DateFilter start={startDate} end={endDate} onChange={handleDateChange} />
      <div style={{ color: '#888', fontSize: 13, marginBottom: 12 }}>
        🔍 特殊渠道按"调拨审核通过"算销售额：单据审核通过即计入、按审核日归月（跟综合看板口径一致）
      </div>

      {/* 顶部 3 个渠道 KPI */}
      <Row gutter={16} style={{ marginBottom: 16 }}>
        {summary.map(s => (
          <Col span={8} key={s.channelKey}>
            <Card className="bi-stat-card" style={{ height: '100%' }}>
              <div style={{ fontWeight: 600, fontSize: 14, marginBottom: 8 }}>
                {s.channelKey} <span style={{ color: '#64748b', fontWeight: 400, fontSize: 12 }}>· {s.channelName}</span>
              </div>
              <Statistic
                title="销售额"
                value={s.totalSales}
                precision={2}
                valueStyle={{ color: '#3f8600', fontSize: 22 }}
              />
              <div style={{ color: '#64748b', fontSize: 12 }}>
                {s.totalOrders} 单 · {fmtWan(s.totalSales)}
              </div>
            </Card>
          </Col>
        ))}
      </Row>

      {missing.length > 0 && (
        <Alert
          type="warning"
          showIcon
          icon={<ExclamationCircleOutlined />}
          message={`价格表缺失 ${missing.length} 个商品(销售额暂按 0 计算)`}
          description="点右上角「价格表」按钮可直接填单价(有改价权限的话),填完销售额自动补算"
          style={{ marginBottom: 16 }}
        />
      )}

      {/* 中部 Tab: 3 个渠道分别看 */}
      <Card>
        <Tabs activeKey={activeChannel} onChange={setActiveChannel}
          tabBarExtraContent={<SpecialChannelPriceManager dept="ecommerce" missing={missing} onSaved={fetchAll} />}
          items={[
            { key: '全部', label: <span>全部合计 <Tag color="blue">{allOrdersCnt} 单</Tag></span> },
            ...summary.map(s => ({
              key: s.channelKey,
              label: <span>{s.channelKey} <Tag>{s.totalOrders} 单</Tag></span>,
            })),
          ]}
        />

        <Row gutter={16} style={{ marginBottom: 16 }}>
          <Col span={8}><Statistic title={isAll ? '总单数(全渠道)' : '单数'} value={dispOrders} /></Col>
          <Col span={8}><Statistic title={isAll ? '总销售额' : '销售额'} value={dispSales} precision={2} valueStyle={{ color: '#3f8600' }} /></Col>
        </Row>

        {channelOrders.length === 0 ? (
          <Empty description="该时间段无调拨单" />
        ) : (
          <Table
            dataSource={channelOrders}
            columns={orderColumns}
            rowKey="allocateNo"
            size="small"
            pagination={{ pageSize: 50, showSizeChanger: true }}
            scroll={{ x: 1300 }}
          />
        )}
      </Card>


      {/* 调拨单明细 Modal */}
      <Modal
        title={`调拨单明细 · ${detailNo}`}
        open={detailOpen}
        onCancel={() => setDetailOpen(false)}
        footer={null}
        width={1100}
      >
        {detailLoading ? (
          <div style={{ textAlign: 'center', padding: 40 }}>加载中...</div>
        ) : (
          <Table
            dataSource={details}
            columns={detailColumns}
            rowKey={(r) => r.goodsNo + '|' + r.skuBarcode}
            size="small"
            pagination={false}
            summary={(rows) => {
              const totalQty = rows.reduce((s, r) => s + r.skuCount, 0);
              const totalExcel = rows.reduce((s, r) => s + r.excelAmount, 0);
              const totalApi = rows.reduce((s, r) => s + r.apiAmount, 0);
              return (
                <Table.Summary.Row>
                  <Table.Summary.Cell index={0} colSpan={3} align="right"><strong>合计</strong></Table.Summary.Cell>
                  <Table.Summary.Cell index={3} align="right"><strong>{totalQty.toFixed(0)}</strong></Table.Summary.Cell>
                  <Table.Summary.Cell index={4} />
                  <Table.Summary.Cell index={5} align="right"><strong style={{ color: '#1677ff' }}>{fmtMoney(totalExcel)}</strong></Table.Summary.Cell>
                  <Table.Summary.Cell index={6} />
                  <Table.Summary.Cell index={7} align="right"><span style={{ color: '#64748b' }}>{fmtMoney(totalApi)}</span></Table.Summary.Cell>
                </Table.Summary.Row>
              );
            }}
          />
        )}
      </Modal>
    </div>
  );
};

export default SpecialChannelAllot;
