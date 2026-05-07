import React, { useEffect, useState, useCallback } from 'react';
import { Card, Row, Col, Table, Statistic, Tag, DatePicker, Button, Modal, Alert, Space, Tabs, Empty } from 'antd';
import { ReloadOutlined, ExclamationCircleOutlined } from '@ant-design/icons';
import dayjs, { Dayjs } from 'dayjs';
import PageLoading from '../../components/PageLoading';
import { API_BASE } from '../../config';

const { RangePicker } = DatePicker;

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

const inStatusLabel = (s: number): { text: string; color: string } => {
  switch (s) {
    case 0: return { text: '未审核', color: 'default' };
    case 1: return { text: '入库等待', color: 'orange' };
    case 2: return { text: '部分入库', color: 'gold' };
    case 3: return { text: '入库完成', color: 'green' };
    default: return { text: `${s}`, color: 'default' };
  }
};

const fmtMoney = (n: number) => n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
const fmtWan = (n: number) => `≈${(n / 10000).toFixed(1)}万`;

const SpecialChannelAllot: React.FC = () => {
  const [range, setRange] = useState<[Dayjs, Dayjs]>([dayjs().subtract(60, 'day'), dayjs()]);
  const [summary, setSummary] = useState<ChannelSummary[]>([]);
  const [orders, setOrders] = useState<OrderRow[]>([]);
  const [missing, setMissing] = useState<MissingRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [activeChannel, setActiveChannel] = useState<string>('京东');
  const [detailOpen, setDetailOpen] = useState(false);
  const [detailNo, setDetailNo] = useState('');
  const [details, setDetails] = useState<DetailRow[]>([]);
  const [detailLoading, setDetailLoading] = useState(false);

  const fetchAll = useCallback(() => {
    setLoading(true);
    const start = range[0].format('YYYY-MM-DD');
    const end = range[1].format('YYYY-MM-DD');
    fetch(`${API_BASE}/api/special-channel-allot/summary?start=${start}&end=${end}`, { credentials: 'include' })
      .then(r => r.json())
      .then(j => {
        if (j.code === 200) {
          setSummary(j.data?.summary || []);
          setOrders(j.data?.orders || []);
          setMissing(j.data?.missing || []);
        }
      })
      .finally(() => setLoading(false));
  }, [range]);

  useEffect(() => { fetchAll(); }, [fetchAll]);

  const openDetail = (allocateNo: string) => {
    setDetailNo(allocateNo);
    setDetailOpen(true);
    setDetailLoading(true);
    fetch(`${API_BASE}/api/special-channel-allot/details?allocate_no=${encodeURIComponent(allocateNo)}`, { credentials: 'include' })
      .then(r => r.json())
      .then(j => {
        if (j.code === 200) {
          setDetails(j.data?.list || []);
        }
      })
      .finally(() => setDetailLoading(false));
  };

  if (loading && orders.length === 0) return <PageLoading />;

  const channelOrders = orders.filter(o => o.channelKey === activeChannel);
  const channelSummary = summary.find(s => s.channelKey === activeChannel);

  const orderColumns = [
    { title: '调拨单号', dataIndex: 'allocateNo', key: 'allocateNo', width: 200,
      render: (v: string) => <a onClick={() => openDetail(v)}>{v}</a> },
    { title: '入库仓', dataIndex: 'inWarehouseName', key: 'inWarehouseName', width: 220, ellipsis: true },
    { title: '入库状态', dataIndex: 'inStatus', key: 'inStatus', width: 110,
      render: (s: number) => { const l = inStatusLabel(s); return <Tag color={l.color}>{l.text}</Tag>; } },
    { title: '创建时间', dataIndex: 'gmtCreate', key: 'gmtCreate', width: 140 },
    { title: '入库完成时间', dataIndex: 'gmtModified', key: 'gmtModified', width: 140 },
    { title: '销售统计日', dataIndex: 'statDate', key: 'statDate', width: 110,
      render: (v: string) => v || <span style={{ color: '#bbb' }}>—</span> },
    { title: 'SKU 数', dataIndex: 'skuCount', key: 'skuCount', width: 80, align: 'right' as const },
    { title: '销售额(Excel价)', dataIndex: 'excelSales', key: 'excelSales', width: 150, align: 'right' as const,
      render: (v: number) => <strong style={{ color: '#1677ff' }}>{fmtMoney(v)}</strong>, sorter: (a: OrderRow, b: OrderRow) => a.excelSales - b.excelSales },
    { title: '吉客云接口金额', dataIndex: 'apiSales', key: 'apiSales', width: 140, align: 'right' as const,
      render: (v: number) => <span style={{ color: '#999' }}>{fmtMoney(v)}</span> },
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
      render: (v: number) => <span style={{ color: '#999' }}>{v.toFixed(2)}</span> },
    { title: '接口金额', dataIndex: 'apiAmount', key: 'apiAmount', width: 110, align: 'right' as const,
      render: (v: number) => <span style={{ color: '#999' }}>{fmtMoney(v)}</span> },
  ];

  const missingColumns = [
    { title: '渠道', dataIndex: 'channelKey', key: 'channelKey', width: 80 },
    { title: '商品编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 130 },
    { title: '条码', dataIndex: 'barcode', key: 'barcode', width: 160 },
    { title: '名称', dataIndex: 'goodsName', key: 'goodsName', ellipsis: true },
    { title: '出现单数', dataIndex: 'allocateCnt', key: 'allocateCnt', width: 90, align: 'right' as const },
    { title: '累计调拨量', dataIndex: 'qtyTotal', key: 'qtyTotal', width: 110, align: 'right' as const },
  ];

  return (
    <div style={{ padding: 16 }}>
      <Card className="bi-filter-card" style={{ marginBottom: 16 }} bodyStyle={{ padding: '12px 16px' }}>
        <Row align="middle" gutter={16}>
          <Col>
            <Space>
              <span>时间范围:</span>
              <RangePicker
                value={range}
                onChange={(v) => v && setRange([v[0]!, v[1]!])}
                allowClear={false}
              />
              <Button icon={<ReloadOutlined />} onClick={fetchAll} loading={loading}>刷新</Button>
            </Space>
          </Col>
          <Col flex="auto" style={{ textAlign: 'right', color: '#888' }}>
            🔍 特殊渠道按"调拨入库完成"算销售额(在途数据展示但不计入"已完成销售额")
          </Col>
        </Row>
      </Card>

      {/* 顶部 3 个渠道 KPI */}
      <Row gutter={16} style={{ marginBottom: 16 }}>
        {summary.map(s => (
          <Col span={8} key={s.channelKey}>
            <Card className="bi-stat-card" style={{ height: '100%' }}>
              <div style={{ fontWeight: 600, fontSize: 14, marginBottom: 8 }}>
                {s.channelKey} <span style={{ color: '#999', fontWeight: 400, fontSize: 12 }}>· {s.channelName}</span>
              </div>
              <Row gutter={16}>
                <Col span={12}>
                  <Statistic
                    title="✅ 已入库完成"
                    value={s.completedSales}
                    precision={2}
                    valueStyle={{ color: '#3f8600', fontSize: 22 }}
                  />
                  <div style={{ color: '#999', fontSize: 12 }}>
                    {s.completedOrders} 单 · {fmtWan(s.completedSales)}
                  </div>
                </Col>
                <Col span={12}>
                  <Statistic
                    title="⏳ 在途待入"
                    value={s.pendingSales}
                    precision={2}
                    valueStyle={{ color: '#faad14', fontSize: 22 }}
                  />
                  <div style={{ color: '#999', fontSize: 12 }}>
                    {s.pendingOrders} 单 · {fmtWan(s.pendingSales)}
                  </div>
                </Col>
              </Row>
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
          description="价格表里漏配了这些商品，请联系数据组补上后会自动同步"
          style={{ marginBottom: 16 }}
        />
      )}

      {/* 中部 Tab: 3 个渠道分别看 */}
      <Card>
        <Tabs activeKey={activeChannel} onChange={setActiveChannel}
          items={summary.map(s => ({
            key: s.channelKey,
            label: <span>{s.channelKey} <Tag>{s.totalOrders} 单</Tag></span>,
          }))}
        />

        {channelSummary && (
          <Row gutter={16} style={{ marginBottom: 16 }}>
            <Col span={6}><Statistic title="单数(全部)" value={channelSummary.totalOrders} /></Col>
            <Col span={6}><Statistic title="销售额(全部)" value={channelSummary.totalSales} precision={2} /></Col>
            <Col span={6}><Statistic title="已入库完成销售额" value={channelSummary.completedSales} precision={2} valueStyle={{ color: '#3f8600' }} /></Col>
            <Col span={6}><Statistic title="在途销售额" value={channelSummary.pendingSales} precision={2} valueStyle={{ color: '#faad14' }} /></Col>
          </Row>
        )}

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

      {/* 缺失 SKU 清单 */}
      {missing.length > 0 && (
        <Card title="价格表缺失 SKU 清单(待维护)" style={{ marginTop: 16 }}>
          <Table
            dataSource={missing}
            columns={missingColumns}
            rowKey={(r) => r.channelKey + '|' + r.goodsNo}
            size="small"
            pagination={false}
          />
        </Card>
      )}

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
                  <Table.Summary.Cell index={7} align="right"><span style={{ color: '#999' }}>{fmtMoney(totalApi)}</span></Table.Summary.Cell>
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
