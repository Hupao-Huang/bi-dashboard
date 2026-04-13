import React, { useCallback, useEffect, useState } from 'react';
import {
  Badge,
  Button,
  Card,
  Image,
  Input,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ReloadOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';

interface FeedbackItem {
  id: number;
  userId: number;
  username: string;
  realName: string;
  title: string;
  content: string;
  pageUrl: string;
  attachments: string[];
  status: string;
  reply: string | null;
  repliedBy: string | null;
  createdAt: string;
  updatedAt: string;
}

const statusConfig: Record<string, { color: string; label: string }> = {
  pending: { color: 'warning', label: '待处理' },
  processing: { color: 'processing', label: '处理中' },
  resolved: { color: 'success', label: '已解决' },
  closed: { color: 'default', label: '已关闭' },
};

const Feedback: React.FC = () => {
  const [list, setList] = useState<FeedbackItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [statusFilter, setStatusFilter] = useState('');
  const [detailItem, setDetailItem] = useState<FeedbackItem | null>(null);
  const [replyText, setReplyText] = useState('');
  const [replyStatus, setReplyStatus] = useState('');
  const [updating, setUpdating] = useState(false);

  const fetchList = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams({
        page: String(page),
        pageSize: String(pageSize),
      });
      if (statusFilter) params.set('status', statusFilter);

      const res = await fetch(`${API_BASE}/api/feedback/list?${params}`, {
        credentials: 'include',
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const json = await res.json();
      const data = json.data || json;
      setList(data.list || []);
      setTotal(data.total || 0);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      message.error('获取反馈列表失败: ' + msg);
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, statusFilter]);

  useEffect(() => {
    fetchList();
  }, [fetchList]);

  const handleReply = async () => {
    if (!detailItem) return;
    if (!replyText && !replyStatus) {
      message.warning('请填写回复或选择状态');
      return;
    }
    setUpdating(true);
    try {
      const body: Record<string, string> = {};
      if (replyText) body.reply = replyText;
      if (replyStatus) body.status = replyStatus;

      const res = await fetch(`${API_BASE}/api/feedback/${detailItem.id}`, {
        method: 'PUT',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.msg || `HTTP ${res.status}`);
      }
      message.success('更新成功');
      setDetailItem(null);
      setReplyText('');
      setReplyStatus('');
      fetchList();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      message.error(msg);
    } finally {
      setUpdating(false);
    }
  };

  const openDetail = (item: FeedbackItem) => {
    setDetailItem(item);
    setReplyText(item.reply || '');
    setReplyStatus(item.status);
  };

  const columns: ColumnsType<FeedbackItem> = [
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      render: (status: string) => {
        const cfg = statusConfig[status] || statusConfig.pending;
        return <Tag color={cfg.color}>{cfg.label}</Tag>;
      },
    },
    {
      title: '标题',
      dataIndex: 'title',
      ellipsis: true,
      render: (title: string, record) => (
        <Button type="link" onClick={() => openDetail(record)} style={{ padding: 0, height: 'auto' }}>
          {title}
        </Button>
      ),
    },
    {
      title: '提交人',
      width: 120,
      render: (_: unknown, record) => (
        <span>{record.realName || record.username}</span>
      ),
    },
    {
      title: '页面',
      dataIndex: 'pageUrl',
      width: 180,
      ellipsis: true,
      render: (url: string) => (
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>{url || '-'}</Typography.Text>
      ),
    },
    {
      title: '附件',
      dataIndex: 'attachments',
      width: 60,
      align: 'center',
      render: (attachments: string[]) => (
        attachments?.length > 0 ? <Badge count={attachments.length} size="small" /> : '-'
      ),
    },
    {
      title: '提交时间',
      dataIndex: 'createdAt',
      width: 160,
    },
    {
      title: '操作',
      width: 80,
      render: (_: unknown, record) => (
        <Button type="link" size="small" onClick={() => openDetail(record)}>
          查看
        </Button>
      ),
    },
  ];

  return (
    <div>
      <Card
        style={{ marginBottom: 16, borderRadius: 'var(--card-radius)', boxShadow: 'var(--card-shadow)' }}
        styles={{ body: { padding: '12px 16px' } }}
      >
        <Space>
          <Select
            placeholder="筛选状态"
            allowClear
            value={statusFilter || undefined}
            onChange={v => { setStatusFilter(v || ''); setPage(1); }}
            style={{ width: 140 }}
            options={[
              { label: '待处理', value: 'pending' },
              { label: '处理中', value: 'processing' },
              { label: '已解决', value: 'resolved' },
              { label: '已关闭', value: 'closed' },
            ]}
          />
          <Button icon={<ReloadOutlined />} onClick={fetchList}>刷新</Button>
        </Space>
      </Card>

      <Table<FeedbackItem>
        columns={columns}
        dataSource={list}
        rowKey="id"
        loading={loading}
        pagination={{
          current: page,
          pageSize,
          total,
          onChange: (p, ps) => { setPage(p); setPageSize(ps); },
          showTotal: t => `共 ${t} 条`,
          showSizeChanger: true,
        }}
        style={{
          background: 'var(--card-bg)',
          borderRadius: 'var(--card-radius)',
          boxShadow: 'var(--card-shadow)',
          overflow: 'hidden',
        }}
      />

      <Modal
        title="反馈详情"
        open={!!detailItem}
        onCancel={() => setDetailItem(null)}
        width={640}
        footer={[
          <Button key="cancel" onClick={() => setDetailItem(null)}>关闭</Button>,
          <Button key="submit" type="primary" loading={updating} onClick={handleReply}>
            保存
          </Button>,
        ]}
      >
        {detailItem && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            <div>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>标题</Typography.Text>
              <div style={{ fontSize: 15, fontWeight: 600 }}>{detailItem.title}</div>
            </div>
            <div>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>提交人</Typography.Text>
              <div>{detailItem.realName || detailItem.username} · {detailItem.createdAt}</div>
            </div>
            <div>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>问题描述</Typography.Text>
              <div style={{ whiteSpace: 'pre-wrap', background: '#f8fafc', padding: 12, borderRadius: 8, marginTop: 4 }}>
                {detailItem.content}
              </div>
            </div>
            {detailItem.pageUrl && (
              <div>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>所在页面</Typography.Text>
                <div style={{ fontSize: 13 }}>{detailItem.pageUrl}</div>
              </div>
            )}
            {detailItem.attachments?.length > 0 && (
              <div>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>截图</Typography.Text>
                <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 4 }}>
                  <Image.PreviewGroup>
                    {detailItem.attachments.map((url, i) => (
                      <Image
                        key={i}
                        src={`${API_BASE}${url}`}
                        width={100}
                        height={100}
                        style={{ objectFit: 'cover', borderRadius: 6 }}
                      />
                    ))}
                  </Image.PreviewGroup>
                </div>
              </div>
            )}
            <div>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>状态</Typography.Text>
              <Select
                value={replyStatus}
                onChange={setReplyStatus}
                style={{ width: '100%', marginTop: 4 }}
                options={[
                  { label: '待处理', value: 'pending' },
                  { label: '处理中', value: 'processing' },
                  { label: '已解决', value: 'resolved' },
                  { label: '已关闭', value: 'closed' },
                ]}
              />
            </div>
            <div>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>回复</Typography.Text>
              <Input.TextArea
                value={replyText}
                onChange={e => setReplyText(e.target.value)}
                rows={3}
                placeholder="输入回复内容"
                style={{ marginTop: 4 }}
              />
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
};

export default Feedback;
