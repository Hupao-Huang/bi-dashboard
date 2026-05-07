import React, { useEffect, useState } from 'react';
import { Button, Card, Form, Input, Modal, Select, Switch, Table, Tag, message, Popconfirm, Space } from 'antd';
import { PlusOutlined, PushpinOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';

const { TextArea } = Input;

interface Notice {
  id: number;
  title: string;
  content: string;
  type: string;
  isPinned: boolean;
  isActive: boolean;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
}

const typeOptions = [
  { value: 'update', label: '功能更新', color: 'blue' },
  { value: 'notice', label: '通知', color: 'green' },
  { value: 'maintenance', label: '维护公告', color: 'orange' },
];

const NoticesPage: React.FC = () => {
  const [notices, setNotices] = useState<Notice[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<Notice | null>(null);
  const [form] = Form.useForm();

  const fetchNotices = () => {
    setLoading(true);
    fetch(`${API_BASE}/api/admin/notices`, { credentials: 'include' })
      .then(res => res.json())
      .then(res => { setNotices(res.data?.notices || []); setLoading(false); })
      .catch(err => { console.warn('Notices fetch:', err); setLoading(false); });
  };

  useEffect(() => { fetchNotices(); }, []);

  const handleCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ type: 'update', isPinned: false });
    setModalOpen(true);
  };

  const handleEdit = (record: Notice) => {
    setEditing(record);
    form.setFieldsValue({
      title: record.title,
      content: record.content,
      type: record.type,
      isPinned: record.isPinned,
    });
    setModalOpen(true);
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      const url = editing
        ? `${API_BASE}/api/admin/notices/${editing.id}`
        : `${API_BASE}/api/admin/notices/create`;
      const method = editing ? 'PUT' : 'POST';
      const res = await fetch(url, {
        method,
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(values),
      });
      const data = await res.json();
      if (res.ok) {
        message.success(editing ? '更新成功' : '创建成功');
        setModalOpen(false);
        fetchNotices();
      } else {
        message.error(data.msg || data.error || '操作失败');
      }
    } catch (e) {
      console.warn('Notices submit:', e);
    }
  };

  const handleDelete = async (id: number) => {
    try {
      const res = await fetch(`${API_BASE}/api/admin/notices/${id}`, {
        method: 'DELETE',
        credentials: 'include',
      });
      const data = await res.json().catch(() => ({}));
      if (res.ok) {
        message.success('删除成功');
        fetchNotices();
      } else {
        message.error(data.msg || data.error || '删除失败');
      }
    } catch (e) {
      console.warn('Notices delete:', e);
      message.error('网络错误，删除失败');
    }
  };

  const handleToggleActive = async (record: Notice) => {
    try {
      const res = await fetch(`${API_BASE}/api/admin/notices/${record.id}`, {
        method: 'PUT',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ isActive: !record.isActive }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        message.error(data.msg || data.error || '切换状态失败');
      }
      fetchNotices();
    } catch (e) {
      console.warn('Notices toggleActive:', e);
      message.error('网络错误，切换状态失败');
    }
  };

  const handleTogglePin = async (record: Notice) => {
    try {
      const res = await fetch(`${API_BASE}/api/admin/notices/${record.id}`, {
        method: 'PUT',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ isPinned: !record.isPinned }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        message.error(data.msg || data.error || '切换置顶失败');
      }
      fetchNotices();
    } catch (e) {
      console.warn('Notices togglePin:', e);
      message.error('网络错误，切换置顶失败');
    }
  };

  const columns = [
    {
      title: '状态', dataIndex: 'isActive', key: 'isActive', width: 70,
      render: (v: boolean, record: Notice) => (
        <Switch size="small" checked={v} onChange={() => handleToggleActive(record)} />
      ),
    },
    {
      title: '置顶', dataIndex: 'isPinned', key: 'isPinned', width: 60,
      render: (v: boolean, record: Notice) => (
        <PushpinOutlined
          style={{ color: v ? '#f5222d' : '#d9d9d9', cursor: 'pointer', fontSize: 16 }}
          onClick={() => handleTogglePin(record)}
        />
      ),
    },
    {
      title: '类型', dataIndex: 'type', key: 'type', width: 90,
      render: (v: string) => {
        const t = typeOptions.find(o => o.value === v);
        return <Tag color={t?.color || 'blue'}>{t?.label || v}</Tag>;
      },
    },
    { title: '标题', dataIndex: 'title', key: 'title', ellipsis: true },
    {
      title: '内容', dataIndex: 'content', key: 'content', ellipsis: true, width: 300,
      render: (v: string) => <span style={{ color: '#666' }}>{v?.slice(0, 60)}{v?.length > 60 ? '...' : ''}</span>,
    },
    { title: '发布人', dataIndex: 'createdBy', key: 'createdBy', width: 80 },
    { title: '创建时间', dataIndex: 'createdAt', key: 'createdAt', width: 140 },
    {
      title: '操作', key: 'action', width: 120,
      render: (_: any, record: Notice) => (
        <Space>
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => handleEdit(record)}>编辑</Button>
          <Popconfirm title="确认删除？" onConfirm={() => handleDelete(record.id)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Card
      title="公告管理"
      extra={<Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>发布公告</Button>}
    >
      <Table
        dataSource={notices}
        columns={columns}
        rowKey="id"
        loading={loading}
        size="small"
        pagination={{ pageSize: 20 }}
      />

      <Modal
        title={editing ? '编辑公告' : '发布公告'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={handleSubmit}
        okText={editing ? '保存' : '发布'}
        width={600}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="title" label="标题" rules={[{ required: true, message: '请输入标题' }]}>
            <Input placeholder="公告标题" maxLength={200} />
          </Form.Item>
          <Form.Item name="type" label="类型">
            <Select options={typeOptions} />
          </Form.Item>
          <Form.Item name="content" label="内容" rules={[{ required: true, message: '请输入内容' }]}>
            <TextArea rows={8} placeholder="公告内容，支持换行" />
          </Form.Item>
          <Form.Item name="isPinned" label="置顶" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};

export default NoticesPage;
