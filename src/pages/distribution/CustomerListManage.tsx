import React, { useCallback, useEffect, useState } from 'react';
import { Card, Table, Tag, Input, Select, Button, Modal, Form, Radio, message, Space, Typography, Row, Col } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { TeamOutlined, ImportOutlined, EditOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';

const { Title } = Typography;

type CustomerRow = {
  id: number;
  customerCode: string;
  customerName: string;
  grade: string;
  remark: string;
  firstOrderAt: string;
  lastOrderAt: string;
  totalAmount: number;
  totalOrders: number;
  createdAt: string;
  updatedAt: string;
};

const gradeColor = (g: string) => (g === 'S' ? 'red' : g === 'A' ? 'orange' : 'default');

const CustomerListManage: React.FC = () => {
  const [data, setData] = useState<CustomerRow[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [search, setSearch] = useState('');
  const [grade, setGrade] = useState<string>('');
  const [editTarget, setEditTarget] = useState<CustomerRow | null>(null);
  const [batchOpen, setBatchOpen] = useState(false);
  const [editForm] = Form.useForm();
  const [batchForm] = Form.useForm();

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams({
        page: String(page),
        pageSize: String(pageSize),
        sortBy: 'total_amount',
        sortOrder: 'desc',
      });
      if (search) params.set('search', search);
      if (grade) params.set('grade', grade);
      const res = await fetch(`${API_BASE}/api/distribution/customers/list?${params}`, { credentials: 'include' });
      const json = await res.json();
      const payload = json.data || json;
      setData(payload.list || []);
      setTotal(payload.total || 0);
    } catch (e) {
      message.error('加载失败');
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, search, grade]);

  useEffect(() => { fetchData(); /* eslint-disable-next-line */ }, [page, pageSize, grade]);

  const openEdit = (row: CustomerRow) => {
    setEditTarget(row);
    editForm.setFieldsValue({ grade: row.grade || '', remark: row.remark || '' });
  };

  const submitEdit = async () => {
    const values = await editForm.validateFields();
    if (!editTarget) return;
    try {
      const res = await fetch(`${API_BASE}/api/distribution/customers/grade`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({
          customerCode: editTarget.customerCode,
          grade: values.grade || '',
          remark: values.remark || '',
        }),
      });
      const json = await res.json();
      if (res.ok && (json.code === 200 || json.message)) {
        message.success('已保存');
        setEditTarget(null);
        fetchData();
      } else {
        message.error(json.error || json.msg || '保存失败');
      }
    } catch {
      message.error('保存失败');
    }
  };

  const submitBatch = async () => {
    const values = await batchForm.validateFields();
    const lines = String(values.codes || '').split(/[\n,，\s]+/).filter(Boolean);
    if (!lines.length) {
      message.warning('请输入至少一个客户编码');
      return;
    }
    const items = lines.map(code => ({ customerCode: code.trim(), grade: values.grade }));
    try {
      const res = await fetch(`${API_BASE}/api/distribution/customers/grade-batch`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ items, remark: values.remark || '' }),
      });
      const json = await res.json();
      const payload = json.data || json;
      message.success(`已更新 ${payload.updated || 0} 条${payload.notFound?.length ? `,未找到 ${payload.notFound.length} 条` : ''}`);
      setBatchOpen(false);
      batchForm.resetFields();
      fetchData();
    } catch {
      message.error('批量更新失败');
    }
  };

  const columns: ColumnsType<CustomerRow> = [
    { title: '客户编码', dataIndex: 'customerCode', width: 160, ellipsis: true, render: v => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v}</span> },
    { title: '客户名称', dataIndex: 'customerName', ellipsis: true },
    {
      title: '等级', dataIndex: 'grade', width: 80, align: 'center',
      render: (v: string) => v ? <Tag color={gradeColor(v)} style={{ fontWeight: 600 }}>{v}</Tag> : <span style={{ color: '#bbb' }}>-</span>,
    },
    {
      title: '累计销售额', dataIndex: 'totalAmount', width: 130, align: 'right',
      render: (v: number) => `¥${(v || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`,
    },
    { title: '订单数', dataIndex: 'totalOrders', width: 90, align: 'right' },
    { title: '首单', dataIndex: 'firstOrderAt', width: 110, render: v => v || '-' },
    { title: '末单', dataIndex: 'lastOrderAt', width: 110, render: v => v || '-' },
    { title: '备注', dataIndex: 'remark', ellipsis: true, render: v => v || <span style={{ color: '#bbb' }}>-</span> },
    {
      title: '操作', width: 80, fixed: 'right' as const,
      render: (_: any, r: CustomerRow) => <Button size="small" type="link" icon={<EditOutlined />} onClick={() => openEdit(r)}>标级</Button>,
    },
  ];

  return (
    <div>
      <Title level={4} style={{ marginBottom: 16 }}>
        <TeamOutlined /> 分销·客户名单管理
      </Title>

      <Card className="bi-filter-card" style={{ marginBottom: 16 }}>
        <Row gutter={[16, 12]} align="middle">
          <Col>
            <Input.Search
              placeholder="搜索客户名/编码"
              allowClear
              style={{ width: 280 }}
              value={search}
              onChange={e => setSearch(e.target.value)}
              onSearch={() => { setPage(1); fetchData(); }}
            />
          </Col>
          <Col>
            <span style={{ marginRight: 8, fontWeight: 500 }}>等级:</span>
            <Select
              value={grade || undefined}
              onChange={v => { setGrade(v || ''); setPage(1); }}
              allowClear
              placeholder="全部"
              style={{ width: 120 }}
              options={[
                { value: 'S', label: 'S 级' },
                { value: 'A', label: 'A 级' },
                { value: 'none', label: '未标级' },
              ]}
            />
          </Col>
          <Col flex="auto" style={{ textAlign: 'right' }}>
            <Space>
              <Button icon={<ImportOutlined />} onClick={() => setBatchOpen(true)}>批量标级</Button>
            </Space>
          </Col>
        </Row>
      </Card>

      <Card>
        <Table
          rowKey="id"
          columns={columns}
          dataSource={data}
          loading={loading}
          size="small"
          scroll={{ x: 1200 }}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: t => `共 ${t.toLocaleString()} 个客户`,
            onChange: (p, ps) => { setPage(p); setPageSize(ps); },
          }}
        />
      </Card>

      <Modal
        title={editTarget ? `标记等级 - ${editTarget.customerName}` : '标记等级'}
        open={!!editTarget}
        onOk={submitEdit}
        onCancel={() => setEditTarget(null)}
        width={520}
      >
        {editTarget && (
          <div style={{ marginBottom: 12, color: '#64748b', fontSize: 13 }}>
            客户编码: <span style={{ fontFamily: 'monospace' }}>{editTarget.customerCode}</span><br />
            累计销售额 ¥{editTarget.totalAmount.toLocaleString('zh-CN', { minimumFractionDigits: 2 })} · 订单数 {editTarget.totalOrders} 单
          </div>
        )}
        <Form form={editForm} layout="vertical">
          <Form.Item label="等级" name="grade">
            <Radio.Group>
              <Radio.Button value="S">S 级</Radio.Button>
              <Radio.Button value="A">A 级</Radio.Button>
              <Radio.Button value="">不标(清除)</Radio.Button>
            </Radio.Group>
          </Form.Item>
          <Form.Item label="备注" name="remark">
            <Input.TextArea rows={3} placeholder="选填" maxLength={500} showCount />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="批量标级"
        open={batchOpen}
        onOk={submitBatch}
        onCancel={() => { setBatchOpen(false); batchForm.resetFields(); }}
        width={620}
      >
        <Form form={batchForm} layout="vertical" initialValues={{ grade: 'S' }}>
          <Form.Item label="等级" name="grade" rules={[{ required: true }]}>
            <Radio.Group>
              <Radio.Button value="S">S 级</Radio.Button>
              <Radio.Button value="A">A 级</Radio.Button>
              <Radio.Button value="">清除</Radio.Button>
            </Radio.Group>
          </Form.Item>
          <Form.Item label="客户编码 (一行一个, 也支持逗号/空格分隔)" name="codes" rules={[{ required: true, message: '请粘贴客户编码' }]}>
            <Input.TextArea rows={8} placeholder="C202411089192&#10;C202210133840&#10;..." />
          </Form.Item>
          <Form.Item label="备注 (选填, 留空保留各自原备注)" name="remark">
            <Input placeholder="选填" maxLength={500} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default CustomerListManage;
