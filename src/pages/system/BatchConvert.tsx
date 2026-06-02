import React, { useState } from 'react';
import {
  Card, Input, InputNumber, Select, DatePicker, Button, Table, Tag, Space, Alert, Typography, message, Modal,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import dayjs, { Dayjs } from 'dayjs';
import { API_BASE } from '../../config';

const { Text } = Typography;

// 组织 / 库存状态常量, 与后端 yonbip_outbound.go(ybOrgPriority) + yonbip_convert.go(ybStatusDocName) 对齐。
// 改动需两边同步。
const YS_ORGS = [
  { id: '2451285875823214599', name: '浙江松鲜鲜自然调味品有限公司' },
  { id: '2451285927362822152', name: '杭州润松自然调味品有限公司' },
  { id: '2451285918772887559', name: '杭州华鲜高新技术有限公司' },
];
const YS_STATUSES = [
  { doc: '2448706971278246078', name: '合格' },
  { doc: '2448706971278246081', name: '不合格' },
  { doc: '2448706971278246082', name: '废品' },
];
const statusName = (doc: string) => YS_STATUSES.find((s) => s.doc === doc)?.name || doc || '-';

// 与后端 yonsuite.StockRow 对齐
interface StockRow {
  warehouse_code: string;
  warehouse_name: string;
  product_code: string;
  product_name: string;
  productsku_id: string;
  model: string;
  batchno: string;
  producedate: string;
  invaliddate: string;
  currentqty: number;
  availableqty: number;
  unit: string;
  unit_id: string;
  manageClass: string;
  status: string;
  stockStatusDoc: string;
  stockUnitId: string;
}

// 与后端 ybConvItem 对齐
interface ConvItem {
  type: 'batch' | 'status';
  org_id: string;
  warehouse_code: string;
  warehouse_name: string;
  product_code: string;
  product_name: string;
  productsku_id: string;
  unit_id: string;
  stockUnitId: string;
  qty: string;
  batchno: string;
  producedate: string;
  invaliddate: string;
  stockStatusDoc: string;
  to_batch: string;
  to_status_doc: string;
}

// 与后端 ybConvResult 对齐
interface ConvResult {
  type: string;
  product_code: string;
  from: string;
  to: string;
  qty: string;
  doc_code: string;
  audit_ok: boolean;
  error?: string;
}

const BatchConvertPage: React.FC = () => {
  // 查现货条件
  const [org, setOrg] = useState(YS_ORGS[0].id);
  const [productCode, setProductCode] = useState('');
  const [warehouseCode, setWarehouseCode] = useState('');
  const [batchno, setBatchno] = useState('');
  const [statusDoc, setStatusDoc] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [stockRows, setStockRows] = useState<StockRow[] | null>(null);

  // 待执行清单
  const [items, setItems] = useState<ConvItem[]>([]);
  const [vouchdate, setVouchdate] = useState<Dayjs>(dayjs());
  const [executing, setExecuting] = useState(false);
  const [results, setResults] = useState<ConvResult[] | null>(null);

  // 转换弹窗
  const [convOpen, setConvOpen] = useState(false);
  const [convType, setConvType] = useState<'batch' | 'status'>('batch');
  const [convRow, setConvRow] = useState<StockRow | null>(null);
  const [toBatch, setToBatch] = useState('');
  const [toStatus, setToStatus] = useState('');
  const [convQty, setConvQty] = useState<number | null>(null);

  const handleSearch = async () => {
    if (!productCode.trim() && !warehouseCode.trim()) {
      message.warning('至少填货品编码或仓库编码再查，避免全量拉取');
      return;
    }
    setLoading(true);
    setStockRows(null);
    try {
      const res = await fetch(`${API_BASE}/api/yonbip/convert-stock`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({
          org_id: org,
          product_code: productCode.trim(),
          warehouse_code: warehouseCode.trim(),
          batchno: batchno.trim(),
          status_doc: statusDoc,
        }),
      });
      const json = await res.json();
      if (res.ok && json.data?.rows) {
        setStockRows(json.data.rows);
        message.success(`查到 ${json.data.rows.length} 行现货`);
      } else {
        message.error(json.msg || json.error || '查库存失败');
      }
    } catch {
      message.error('网络错误，查库存失败');
    } finally {
      setLoading(false);
    }
  };

  const openConvert = (row: StockRow, type: 'batch' | 'status') => {
    setConvRow(row);
    setConvType(type);
    setToBatch('');
    setToStatus('');
    setConvQty(row.availableqty > 0 ? Math.floor(row.availableqty) : null);
    setConvOpen(true);
  };

  const confirmConvert = () => {
    if (!convRow) return;
    const qty = convQty ?? 0;
    if (qty <= 0) { message.error('数量必须大于 0'); return; }
    if (qty > convRow.availableqty) { message.error(`数量不能超过可用量 ${convRow.availableqty}`); return; }
    if (convType === 'batch') {
      if (!toBatch.trim()) { message.error('请填转换后批次'); return; }
      if (toBatch.trim() === convRow.batchno) { message.error('转换后批次不能和原批次相同'); return; }
    } else {
      if (!toStatus) { message.error('请选目标状态'); return; }
      if (toStatus === convRow.stockStatusDoc) { message.error('目标状态不能和当前状态相同'); return; }
    }
    const item: ConvItem = {
      type: convType,
      org_id: org,
      warehouse_code: convRow.warehouse_code,
      warehouse_name: convRow.warehouse_name,
      product_code: convRow.product_code,
      product_name: convRow.product_name,
      productsku_id: convRow.productsku_id,
      unit_id: convRow.unit_id,
      stockUnitId: convRow.stockUnitId,
      qty: String(qty),
      batchno: convRow.batchno,
      producedate: convRow.producedate,
      invaliddate: convRow.invaliddate,
      stockStatusDoc: convRow.stockStatusDoc,
      to_batch: convType === 'batch' ? toBatch.trim() : '',
      to_status_doc: convType === 'status' ? toStatus : '',
    };
    setItems((prev) => [...prev, item]);
    setConvOpen(false);
    message.success('已加入待执行清单');
  };

  const removeItem = (idx: number) => setItems((prev) => prev.filter((_, i) => i !== idx));

  const doExecute = async () => {
    if (executing) return; // 防连点重复提交
    setExecuting(true);
    try {
      const res = await fetch(`${API_BASE}/api/yonbip/convert-execute`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ vouchdate: vouchdate.format('YYYY-MM-DD'), items }),
      });
      const json = await res.json();
      if (res.ok && json.data?.results) {
        setResults(json.data.results);
        const fail = (json.data.results as ConvResult[]).filter((r) => r.error).length;
        // 执行完即清空清单: 转换单无幂等键, 防止再点"全部执行"把已成功的重发→重复建单、不可逆重复移库存。
        // 有失败的看下方结果, 需要重做的重新查现货再加。
        setItems([]);
        if (fail > 0) message.warning(`执行完成，${fail} 笔有问题，请看下方结果；清单已清空，需重做的请重新加`);
        else message.success('执行完成，全部成功');
      } else {
        message.error(json.msg || json.error || '执行失败');
      }
    } catch {
      message.error('网络错误，执行失败');
    } finally {
      setExecuting(false);
    }
  };

  const handleExecute = () => {
    if (items.length === 0) return;
    const nBatch = items.filter((i) => i.type === 'batch').length;
    const nStatus = items.length - nBatch;
    Modal.confirm({
      title: '确认执行（不可逆）',
      content: (
        <div>
          <p>即将在用友提交 <b>{items.length}</b> 笔转换并自动审核：批次转换 {nBatch} 笔、状态转换 {nStatus} 笔。</p>
          <p>单据日期：<b>{vouchdate.format('YYYY-MM-DD')}</b></p>
          <p><Text type="danger">会真改库存，不可撤回。确认执行？</Text></p>
        </div>
      ),
      okText: '确认执行',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: doExecute,
    });
  };

  const stockColumns: ColumnsType<StockRow> = [
    { title: '仓库', dataIndex: 'warehouse_name', key: 'wh', width: 180, ellipsis: true },
    {
      title: '货品', key: 'product', width: 200,
      render: (_, r) => <span>{r.product_code}{r.product_name ? <><br /><Text type="secondary">{r.product_name}</Text></> : null}</span>,
    },
    { title: '批次', dataIndex: 'batchno', key: 'batch', width: 130 },
    { title: '状态', dataIndex: 'status', key: 'status', width: 80 },
    { title: '可用量', dataIndex: 'availableqty', key: 'avail', width: 90, render: (v) => <Text strong>{v}</Text> },
    { title: '单位', dataIndex: 'unit', key: 'unit', width: 70 },
    {
      title: '操作', key: 'op', width: 170, fixed: 'right',
      render: (_, r) => (
        <Space size={4}>
          <Button size="small" onClick={() => openConvert(r, 'batch')}>批次转换</Button>
          <Button size="small" onClick={() => openConvert(r, 'status')}>状态转换</Button>
        </Space>
      ),
    },
  ];

  const itemColumns: ColumnsType<ConvItem> = [
    { title: '类型', dataIndex: 'type', key: 'type', width: 90, render: (t) => <Tag color={t === 'batch' ? 'blue' : 'magenta'}>{t === 'batch' ? '批次转换' : '状态转换'}</Tag> },
    { title: '货品', dataIndex: 'product_code', key: 'p', width: 130 },
    { title: '仓库', dataIndex: 'warehouse_name', key: 'wh', width: 160, ellipsis: true },
    {
      title: '转换', key: 'conv',
      render: (_, it) => it.type === 'batch'
        ? <span>批次 <Tag>{it.batchno || '-'}</Tag>→<Tag color="orange">{it.to_batch}</Tag></span>
        : <span>状态 <Tag>{statusName(it.stockStatusDoc)}</Tag>→<Tag color="orange">{statusName(it.to_status_doc)}</Tag>（批次 {it.batchno || '-'}）</span>,
    },
    { title: '数量', dataIndex: 'qty', key: 'qty', width: 80 },
    { title: '操作', key: 'op', width: 70, render: (_, _it, idx) => <Button size="small" type="link" danger onClick={() => removeItem(idx)}>删除</Button> },
  ];

  const resultColumns: ColumnsType<ConvResult> = [
    { title: '类型', dataIndex: 'type', key: 'type', width: 90, render: (t) => <Tag color={t === 'batch' ? 'blue' : 'magenta'}>{t === 'batch' ? '批次转换' : '状态转换'}</Tag> },
    { title: '货品', dataIndex: 'product_code', key: 'p', width: 130 },
    { title: '转换', key: 'conv', render: (_, r) => <span>{r.from} → {r.to}（{r.qty}）</span> },
    {
      title: '结果', key: 'res',
      render: (_, r) => r.error
        ? <Tag color="red">失败: {r.error}</Tag>
        : <Tag color={r.audit_ok ? 'green' : 'gold'}>{r.doc_code || '已建单'} {r.audit_ok ? '已审核' : '未审核'}</Tag>,
    },
  ];

  return (
    <div style={{ padding: 16 }}>
      <Card title="用友批次 / 状态转换">
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message="先查现货 → 在某行做批次转换或状态转换 → 加入清单 → 全部执行"
          description="点「全部执行」会真在用友建转换单、改库存、自动审核，不可撤回。"
        />

        {/* 查现货 */}
        <Space wrap style={{ marginBottom: 12 }}>
          <Select value={org} onChange={setOrg} style={{ width: 280 }} options={YS_ORGS.map((o) => ({ value: o.id, label: o.name }))} />
          <Input placeholder="货品编码" value={productCode} onChange={(e) => setProductCode(e.target.value)} style={{ width: 150 }} allowClear />
          <Input placeholder="仓库编码（可选）" value={warehouseCode} onChange={(e) => setWarehouseCode(e.target.value)} style={{ width: 160 }} allowClear />
          <Input placeholder="批次（可选）" value={batchno} onChange={(e) => setBatchno(e.target.value)} style={{ width: 140 }} allowClear />
          <Select
            placeholder="状态（可选）"
            value={statusDoc || undefined}
            onChange={(v) => setStatusDoc(v || '')}
            style={{ width: 130 }}
            allowClear
            options={YS_STATUSES.map((s) => ({ value: s.doc, label: s.name }))}
          />
          <Button type="primary" loading={loading} onClick={handleSearch}>查现货</Button>
        </Space>

        {stockRows && (
          <Table
            size="small"
            rowKey={(_, i) => `s${i}`}
            columns={stockColumns}
            dataSource={stockRows}
            pagination={false}
            scroll={{ x: 900, y: 360 }}
            style={{ marginBottom: 20 }}
          />
        )}

        {/* 待执行清单 */}
        <Typography.Title level={5}>待执行清单（{items.length}）</Typography.Title>
        {items.length > 0 ? (
          <>
            <Table size="small" rowKey={(_, i) => `i${i}`} columns={itemColumns} dataSource={items} pagination={false} style={{ marginBottom: 12 }} />
            <Space wrap>
              <Text strong>单据日期</Text>
              <DatePicker value={vouchdate} onChange={(d) => d && setVouchdate(d)} allowClear={false} />
              <Text type="secondary">跨月做账选当月最后一天，别用今天</Text>
              <Button danger loading={executing} onClick={handleExecute}>全部执行</Button>
            </Space>
          </>
        ) : (
          <Text type="secondary">还没加转换项。先查现货，在表格里点「批次转换」或「状态转换」。</Text>
        )}

        {/* 执行结果 */}
        {results && (
          <>
            <Typography.Title level={5} style={{ marginTop: 20 }}>执行结果</Typography.Title>
            <Table size="small" rowKey={(_, i) => `r${i}`} columns={resultColumns} dataSource={results} pagination={false} />
          </>
        )}
      </Card>

      {/* 转换弹窗 */}
      <Modal
        title={convType === 'batch' ? '批次转换' : '库存状态转换'}
        open={convOpen}
        onOk={confirmConvert}
        onCancel={() => setConvOpen(false)}
        okText="加入清单"
        cancelText="取消"
        destroyOnHidden
      >
        {convRow && (
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            <div>
              <Text type="secondary">货品：</Text>{convRow.product_code} {convRow.product_name}
            </div>
            <div>
              <Text type="secondary">仓库：</Text>{convRow.warehouse_name}
              <Text type="secondary">可用量：</Text><Text strong>{convRow.availableqty}</Text> {convRow.unit}
            </div>
            {convType === 'batch' ? (
              <>
                <div><Text type="secondary">原批次：</Text><Tag>{convRow.batchno || '-'}</Tag><Text type="secondary">（状态 {statusName(convRow.stockStatusDoc)} 不变）</Text></div>
                <div>
                  <Text>转换后批次 </Text><Text type="danger">*</Text>
                  <Input value={toBatch} onChange={(e) => setToBatch(e.target.value)} placeholder="新批次号" style={{ marginTop: 4 }} />
                </div>
              </>
            ) : (
              <>
                <div><Text type="secondary">当前状态：</Text><Tag>{statusName(convRow.stockStatusDoc)}</Tag><Text type="secondary">（批次 {convRow.batchno || '-'} 不变）</Text></div>
                <div>
                  <Text>目标状态 </Text><Text type="danger">*</Text>
                  <Select
                    value={toStatus || undefined}
                    onChange={setToStatus}
                    placeholder="选目标状态"
                    style={{ width: '100%', marginTop: 4 }}
                    options={YS_STATUSES.filter((s) => s.doc !== convRow.stockStatusDoc).map((s) => ({ value: s.doc, label: s.name }))}
                  />
                </div>
              </>
            )}
            <div>
              <Text>转换数量 </Text><Text type="danger">*</Text>
              <InputNumber value={convQty} onChange={setConvQty} min={0} max={convRow.availableqty} precision={0} style={{ width: '100%', marginTop: 4 }} />
            </div>
          </Space>
        )}
      </Modal>
    </div>
  );
};

export default BatchConvertPage;
