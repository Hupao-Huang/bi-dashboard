import React, { useState } from 'react';
import { Card, Steps, Upload, Button, Table, Tag, message, Typography, Space, Alert, Modal, Checkbox } from 'antd';
import { InboxOutlined, CloudUploadOutlined } from '@ant-design/icons';
import type { UploadProps } from 'antd';
import { API_BASE } from '../../config';

// 工具箱-新增采购订单: 上传 Excel → 预览(翻译/算价/查错) → 确认建单(写用友, 不可逆)。
const { Paragraph, Text } = Typography;

interface PreviewRow {
  rowNo: number;
  orderIndex: number;
  orgName: string; orgCode: string;
  vendorName: string; vendorCode: string;
  productCode: string; productName: string;
  unitCode: string; taxitemsCode: string;
  taxRatePct: number; taxRateName: string;
  qty: number; taxInclPrice: number;
  oriSum: number; oriMoney: number; oriTax: number;
  arriveDate: string;
  problems: string[];
  warnings: string[];
}
interface OrderSummary {
  orgCode: string; orgName: string;
  vendorCode: string; vendorName: string;
  vouchDate: string; lineCount: number; totalSum: number; hasProblem: boolean;
}
interface CommitResult {
  orgCode: string; vendorName: string; vouchDate: string; lineCount: number;
  ok: boolean; skipped: boolean; orderId: string; error?: string;
}

const fmtMoney = (n: number) => (n || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

const YonbipPurchaseOrder: React.FC = () => {
  const [step, setStep] = useState(0);
  const [rows, setRows] = useState<PreviewRow[]>([]);
  const [orders, setOrders] = useState<OrderSummary[]>([]);
  const [token, setToken] = useState('');
  const [uploading, setUploading] = useState(false);
  const [committing, setCommitting] = useState(false);
  const [results, setResults] = useState<CommitResult[]>([]);
  const [force, setForce] = useState(false);
  const [selectedOrder, setSelectedOrder] = useState<number | null>(null); // 点订单筛明细

  const reset = () => {
    setStep(0); setRows([]); setOrders([]); setToken(''); setResults([]); setForce(false); setSelectedOrder(null);
  };

  const doUpload = async (file: File) => {
    setUploading(true);
    setSelectedOrder(null);
    message.open({ key: 'po-up', type: 'loading', content: '正在解析并查用友翻译编码（组织/物料/供应商），通常 5–20 秒，请别关页面…', duration: 0 });
    const fd = new FormData();
    fd.append('file', file);
    try {
      const res = await fetch(`${API_BASE}/api/yonbip/po-preview`, {
        method: 'POST', credentials: 'include', body: fd,
      });
      const json = await res.json();
      if (json.code !== 0 && json.code !== 200) {
        message.error(json.message || json.error || '预览失败');
        return;
      }
      const d = json.data || {};
      setRows(d.rows || []);
      setOrders(d.orders || []);
      setToken(d.token || '');
      setStep(1);
      message.success(`解析完成: ${(d.rows || []).length} 行明细, ${(d.orders || []).length} 张订单`);
    } catch {
      message.error('网络错误, 上传失败');
    } finally {
      message.destroy('po-up');
      setUploading(false);
    }
  };

  const buildableCount = orders.filter(o => !o.hasProblem).length;

  const doCommit = () => {
    Modal.confirm({
      title: `确认建 ${buildableCount} 张采购订单到用友?`,
      content: '写入用友不可逆(建成开立态, 需在用友删除)。有问题的订单会自动跳过。',
      okText: '确认建单', cancelText: '再看看',
      onOk: async () => {
        setCommitting(true);
        try {
          const res = await fetch(`${API_BASE}/api/yonbip/po-commit`, {
            method: 'POST', credentials: 'include',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ token, force }),
          });
          const json = await res.json();
          if (json.code !== 0 && json.code !== 200) {
            message.error(json.message || json.error || '建单失败');
            return;
          }
          setResults(json.data?.results || []);
          setStep(2);
          const ok = (json.data?.results || []).filter((r: CommitResult) => r.ok).length;
          message.success(`建单完成: 成功 ${ok} 张`);
        } catch {
          message.error('网络错误, 建单失败');
        } finally {
          setCommitting(false);
        }
      },
    });
  };

  const uploadProps: UploadProps = {
    accept: '.xlsx',
    showUploadList: false,
    beforeUpload: (file) => { doUpload(file as File); return false; },
  };

  const previewCols = [
    { title: '行', dataIndex: 'rowNo', width: 50 },
    {
      title: '组织', dataIndex: 'orgName', width: 160, ellipsis: true,
      render: (v: string, r: PreviewRow) => <span>{v} {r.orgCode ? <Text type="secondary">({r.orgCode})</Text> : <Tag color="red">无</Tag>}</span>,
    },
    {
      title: '供应商', dataIndex: 'vendorName', width: 200, ellipsis: true,
      render: (v: string, r: PreviewRow) => <span>{v} {r.vendorCode ? <Text type="secondary">({r.vendorCode})</Text> : <Tag color="red">无</Tag>}</span>,
    },
    {
      title: '物料', dataIndex: 'productCode', width: 200, ellipsis: true,
      render: (v: string, r: PreviewRow) => <span>{v} {r.productName} {r.unitCode ? <Text type="secondary">[{r.unitCode}]</Text> : <Tag color="red">查不到</Tag>}</span>,
    },
    { title: '数量', dataIndex: 'qty', width: 70, align: 'right' as const },
    { title: '含税单价', dataIndex: 'taxInclPrice', width: 85, align: 'right' as const, render: (v: number) => v },
    {
      title: '税率', dataIndex: 'taxRatePct', width: 70, align: 'right' as const,
      render: (v: number, r: PreviewRow) => v ? <span title={r.taxRateName}>{v}%</span> : '-',
    },
    { title: '含税金额', dataIndex: 'oriSum', width: 95, align: 'right' as const, render: (v: number) => fmtMoney(v) },
    { title: '无税', dataIndex: 'oriMoney', width: 95, align: 'right' as const, render: (v: number) => fmtMoney(v) },
    { title: '税额', dataIndex: 'oriTax', width: 85, align: 'right' as const, render: (v: number) => fmtMoney(v) },
    { title: '计划到货', dataIndex: 'arriveDate', width: 105, render: (v: string) => v ? v.slice(0, 10) : '-' },
    {
      title: '检查', key: 'check', width: 220,
      render: (_: any, r: PreviewRow) => (
        <span>
          {r.problems && r.problems.length
            ? <Tag color="red">{r.problems.join('; ')}</Tag>
            : (!r.warnings || !r.warnings.length) && <Tag color="green">OK</Tag>}
          {r.warnings && r.warnings.map((w, i) => <Tag color="orange" key={i}>{w}</Tag>)}
        </span>
      ),
    },
  ];

  const orderCols = [
    { title: '组织', dataIndex: 'orgCode', width: 90 },
    { title: '供应商', dataIndex: 'vendorName', ellipsis: true },
    { title: '日期', dataIndex: 'vouchDate', width: 110, render: (v: string) => v ? v.slice(0, 10) : '-' },
    { title: '行数', dataIndex: 'lineCount', width: 60, align: 'right' as const },
    { title: '含税合计', dataIndex: 'totalSum', width: 120, align: 'right' as const, render: (v: number) => fmtMoney(v) },
    { title: '状态', dataIndex: 'hasProblem', width: 100, render: (v: boolean) => v ? <Tag color="red">有问题不建</Tag> : <Tag color="green">可建</Tag> },
  ];

  const resultCols = [
    { title: '组织', dataIndex: 'orgCode', width: 90 },
    { title: '供应商', dataIndex: 'vendorName', ellipsis: true },
    { title: '日期', dataIndex: 'vouchDate', width: 110, render: (v: string) => v ? v.slice(0, 10) : '-' },
    { title: '行数', dataIndex: 'lineCount', width: 60, align: 'right' as const },
    {
      title: '结果', key: 'r', width: 120,
      render: (_: any, r: CommitResult) => r.ok ? <Tag color="green">成功</Tag> : r.skipped ? <Tag color="orange">已跳过(防重)</Tag> : <Tag color="red">失败</Tag>,
    },
    { title: '用友订单号/id', dataIndex: 'orderId', width: 200, render: (v: string) => v || '-' },
    { title: '说明', dataIndex: 'error', ellipsis: true, render: (v: string) => v || '-' },
  ];

  return (
    <Card title={<span>新增采购订单 <Tag color="purple">YS工具</Tag></span>}>
      <Steps
        current={step} style={{ marginBottom: 20, maxWidth: 600 }}
        items={[{ title: '上传模板' }, { title: '预览核对' }, { title: '建单结果' }]}
      />

      {step === 0 && (
        <div>
          <Paragraph type="secondary">
            上传采购申请 Excel(.xlsx)。系统会自动把组织/供应商/物料名称翻译成用友编码、算好价税,先给你预览,确认无误再建单。
            列: 采购组织 / 交易类型 / 单据日期 / 供货供应商 / 物料编码 / 物料名称 / 采购数量 / 采购单位名称 / 主计量 / 含税单价 / 含税金额 / 税率 / 计划到货日期。
          </Paragraph>
          <Upload.Dragger {...uploadProps} disabled={uploading} style={{ maxWidth: 520 }}>
            <p className="ant-upload-drag-icon"><InboxOutlined /></p>
            <p className="ant-upload-text">{uploading ? '正在解析并查用友翻译编码…（通常 5–20 秒，请别关页面）' : '点击或拖拽 .xlsx 文件到此上传'}</p>
          </Upload.Dragger>
        </div>
      )}

      {step === 1 && (
        <div>
          <Space style={{ marginBottom: 12 }} wrap>
            <Button onClick={reset}>重新上传</Button>
            <Button type="primary" icon={<CloudUploadOutlined />} loading={committing}
              disabled={buildableCount === 0} onClick={doCommit}>
              确认建单（可建 {buildableCount} / {orders.length} 张）
            </Button>
            <Checkbox checked={force} onChange={e => setForce(e.target.checked)}>
              强制重发(跳过10分钟防重)
            </Checkbox>
          </Space>
          {orders.some(o => o.hasProblem) && (
            <Alert type="warning" showIcon style={{ marginBottom: 12 }}
              message="有订单存在红色问题(组织/供应商/物料查不到、数量异常等), 这些订单会自动跳过, 只建绿色的。" />
          )}
          <Card size="small" type="inner"
            title={<span>订单汇总（{orders.length} 张）<Text type="secondary" style={{ fontWeight: 400, marginLeft: 8 }}>点某行只看该订单的明细</Text></span>}
            style={{ marginBottom: 16 }}>
            <Table rowKey={(_, i) => `o${i}`} size="small" columns={orderCols} dataSource={orders}
              pagination={false} scroll={{ x: 700 }}
              rowClassName={(_, i) => (i === selectedOrder ? 'po-row-sel' : '')}
              onRow={(_, i) => ({
                style: { cursor: 'pointer' },
                onClick: () => setSelectedOrder(selectedOrder === i ? null : (i ?? null)),
              })} />
          </Card>
          <Card size="small" type="inner"
            title={
              <span>
                明细行（{selectedOrder === null ? rows.length : rows.filter(r => r.orderIndex === selectedOrder).length} 行）
                {selectedOrder !== null && (
                  <>
                    <Text type="secondary" style={{ fontWeight: 400, marginLeft: 8 }}>
                      已筛选: {orders[selectedOrder]?.vendorName}
                    </Text>
                    <Button size="small" type="link" onClick={() => setSelectedOrder(null)}>显示全部</Button>
                  </>
                )}
              </span>
            }>
            <Table rowKey="rowNo" size="small" columns={previewCols}
              dataSource={selectedOrder === null ? rows : rows.filter(r => r.orderIndex === selectedOrder)}
              pagination={false} scroll={{ x: 1550, y: 440 }} sticky={false}
              rowClassName={(r) => (r.problems && r.problems.length ? 'po-row-bad' : '')} />
          </Card>
        </div>
      )}

      {step === 2 && (
        <div>
          <Space style={{ marginBottom: 12 }}>
            <Button type="primary" onClick={reset}>再传一张</Button>
          </Space>
          <Alert type="info" showIcon style={{ marginBottom: 12 }}
            message="建成的采购订单为'开立'态(未审核), 需要在用友里审核或删除。" />
          <Table rowKey={(_, i) => `r${i}`} size="small" columns={resultCols} dataSource={results}
            pagination={false} scroll={{ x: 900 }} />
        </div>
      )}
      <style>{`.po-row-bad td { background: #fff1f0 !important; }
        .po-row-sel td { background: #e6f4ff !important; }`}</style>
    </Card>
  );
};

export default YonbipPurchaseOrder;
