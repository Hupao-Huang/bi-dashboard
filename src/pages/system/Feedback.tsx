import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Col,
  Image,
  Input,
  Modal,
  Row,
  Select,
  Statistic,
  Table,
  Tooltip,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ClockCircleOutlined, SyncOutlined, CheckCircleOutlined, MinusCircleOutlined, LinkOutlined, SearchOutlined } from '@ant-design/icons';
import { useLocation } from 'react-router-dom';
import { API_BASE } from '../../config';
import { pageTitleMap } from '../../navigation';

const STATUS_HEX_MAP: Record<string, string> = {
  pending: '#f59e0b',
  processing: '#1677ff',
  resolved: '#16a34a',
  closed: '#94a3b8',
};
const statusHex = (s: string): string => STATUS_HEX_MAP[s] || '#94a3b8';

// 把 pageUrl 转成可读菜单名："/ecommerce/marketing-cost" → "营销费用"
const formatPageName = (url?: string): string => {
  if (!url) return '-';
  let path = url;
  try {
    if (/^https?:/i.test(url)) {
      path = new URL(url).pathname;
    }
  } catch {
    // ignore
  }
  return pageTitleMap[path] || url;
};

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

const statusConfig: Record<string, { label: string }> = {
  pending: { label: '待处理' },
  processing: { label: '处理中' },
  resolved: { label: '已解决' },
  closed: { label: '已关闭' },
};

const StatusChip: React.FC<{ status: string }> = ({ status }) => {
  const cfg = statusConfig[status] || statusConfig.pending;
  const hex = statusHex(status);
  return (
    <span style={{
      display: 'inline-block',
      padding: '1px 8px',
      background: `${hex}12`,
      color: hex,
      border: `1px solid ${hex}30`,
      borderRadius: 3,
      fontWeight: 500,
      fontSize: 12,
      lineHeight: '20px',
    }}>
      {cfg.label}
    </span>
  );
};

const Feedback: React.FC = () => {
  const location = useLocation();
  const initialStatus = useMemo(() => {
    const s = new URLSearchParams(location.search).get('status');
    return s && statusConfig[s] ? s : '';
  }, [location.search]);

  const [list, setList] = useState<FeedbackItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [statusFilter, setStatusFilter] = useState(initialStatus);
  const [searchText, setSearchText] = useState('');
  const [detailItem, setDetailItem] = useState<FeedbackItem | null>(null);
  const [originalReply, setOriginalReply] = useState('');
  const [originalStatus, setOriginalStatus] = useState('');
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
    const replyChanged = replyText !== originalReply;
    const statusChanged = replyStatus !== originalStatus;
    if (!replyChanged && !statusChanged) {
      message.info('没有改动，无需保存');
      return;
    }
    setUpdating(true);
    try {
      const body: Record<string, string> = {};
      if (replyChanged) body.reply = replyText;
      if (statusChanged) body.status = replyStatus;

      const res = await fetch(`${API_BASE}/api/feedback/${detailItem.id}`, {
        method: 'PUT',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!res.ok) {
        const data = await res.json().catch(err => { console.warn('Feedback reply json:', err); return {}; });
        throw new Error(data.msg || `HTTP ${res.status}`);
      }
      message.success('更新成功');
      setDetailItem(null);
      setReplyText('');
      setReplyStatus('');
      setOriginalReply('');
      setOriginalStatus('');
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
    setOriginalReply(item.reply || '');
    setOriginalStatus(item.status);
  };

  const stats = useMemo(() => ({
    pending: list.filter(i => i.status === 'pending').length,
    processing: list.filter(i => i.status === 'processing').length,
    resolved: list.filter(i => i.status === 'resolved').length,
    closed: list.filter(i => i.status === 'closed').length,
  }), [list]);

  const filteredList = useMemo(() => {
    const q = searchText.trim().toLowerCase();
    if (!q) return list;
    return list.filter(item =>
      item.title.toLowerCase().includes(q) ||
      item.content.toLowerCase().includes(q) ||
      (item.realName || '').toLowerCase().includes(q) ||
      (item.username || '').toLowerCase().includes(q)
    );
  }, [list, searchText]);

  const columns: ColumnsType<FeedbackItem> = [
    {
      title: '标题',
      dataIndex: 'title',
      width: 280,
      ellipsis: true,
      render: (title: string, record) => {
        const accent = statusHex(record.status);
        return (
          <div style={{ display: 'flex', alignItems: 'stretch', gap: 10, minHeight: 38 }}>
            <div style={{
              width: 3,
              background: accent,
              borderRadius: 2,
              flexShrink: 0,
              alignSelf: 'stretch',
            }} />
            <div style={{ minWidth: 0, flex: 1 }}>
              <Button
                type="link"
                onClick={() => openDetail(record)}
                style={{ padding: 0, height: 'auto', textAlign: 'left', color: '#0f172a', fontWeight: 500 }}
              >
                <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', display: 'inline-block', maxWidth: 220 }}>
                  {title}
                </span>
              </Button>
              <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 2 }}>
                {record.realName || record.username}
              </div>
            </div>
          </div>
        );
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 88,
      filters: [
        { text: '待处理', value: 'pending' },
        { text: '处理中', value: 'processing' },
        { text: '已解决', value: 'resolved' },
        { text: '已关闭', value: 'closed' },
      ],
      onFilter: (value, record) => record.status === value,
      render: (status: string) => <StatusChip status={status} />,
    },
    {
      title: '内容',
      dataIndex: 'content',
      ellipsis: true,
      render: (content: string) => (
        <Tooltip title={<div style={{ whiteSpace: 'pre-wrap', maxWidth: 400 }}>{content}</div>} placement="topLeft">
          <span style={{ color: '#475569', fontSize: 12 }}>{content}</span>
        </Tooltip>
      ),
    },
    {
      title: '提交页面',
      dataIndex: 'pageUrl',
      width: 150,
      ellipsis: true,
      responsive: ['lg' as const],
      render: (url: string) => {
        const name = formatPageName(url);
        return (
          <Tooltip title={url || '未知'}>
            <Typography.Text style={{ fontSize: 12, color: name === url || name === '-' ? '#94a3b8' : '#1e40af' }}>{name}</Typography.Text>
          </Tooltip>
        );
      },
    },
    {
      title: '附件',
      dataIndex: 'attachments',
      width: 90,
      responsive: ['xl' as const],
      render: (attachments: string[]) => {
        if (!attachments?.length) return <span style={{ color: '#cbd5e1' }}>—</span>;
        return (
          <div style={{ display: 'flex', gap: 4 }} onClick={(e) => e.stopPropagation()}>
            <Image.PreviewGroup>
              {attachments.slice(0, 2).map((url, i) => (
                <Image
                  key={i}
                  src={`${API_BASE}${url}`}
                  width={28}
                  height={28}
                  style={{ objectFit: 'cover', borderRadius: 3, border: '1px solid #e2e8f0' }}
                />
              ))}
              {attachments.length > 2 && (
                <span style={{ fontSize: 11, color: '#64748b', alignSelf: 'center', marginLeft: 2 }}>
                  +{attachments.length - 2}
                </span>
              )}
            </Image.PreviewGroup>
          </div>
        );
      },
    },
    {
      title: '提交时间',
      dataIndex: 'createdAt',
      width: 150,
      responsive: ['xl' as const],
      sorter: (a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime(),
      defaultSortOrder: 'descend' as const,
      render: (v: string) => <span style={{ fontSize: 12, color: '#64748b', fontVariantNumeric: 'tabular-nums' }}>{v}</span>,
    },
  ];

  return (
    <div>
      <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
        <Col xs={12} sm={6}>
          <Card
            className="bi-stat-card"
            style={{
              ['--accent-color' as any]: STATUS_HEX_MAP.pending,
              cursor: 'pointer',
              outline: statusFilter === 'pending' ? `2px solid ${STATUS_HEX_MAP.pending}` : 'none',
            }}
            bodyStyle={{ padding: 16 }}
            onClick={() => { setPage(1); setStatusFilter(statusFilter === 'pending' ? '' : 'pending'); }}
          >
            <Statistic
              title={<><ClockCircleOutlined style={{ marginRight: 6, color: STATUS_HEX_MAP.pending }} />待处理</>}
              value={stats.pending}
              valueStyle={{ color: stats.pending > 0 ? STATUS_HEX_MAP.pending : '#94a3b8', fontSize: 22, fontWeight: stats.pending > 0 ? 700 : 400 }}
            />
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card
            className="bi-stat-card"
            style={{
              ['--accent-color' as any]: STATUS_HEX_MAP.processing,
              cursor: 'pointer',
              outline: statusFilter === 'processing' ? `2px solid ${STATUS_HEX_MAP.processing}` : 'none',
            }}
            bodyStyle={{ padding: 16 }}
            onClick={() => { setPage(1); setStatusFilter(statusFilter === 'processing' ? '' : 'processing'); }}
          >
            <Statistic
              title={<><SyncOutlined style={{ marginRight: 6, color: STATUS_HEX_MAP.processing }} />处理中</>}
              value={stats.processing}
              valueStyle={{ color: STATUS_HEX_MAP.processing, fontSize: 22 }}
            />
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card
            className="bi-stat-card"
            style={{
              ['--accent-color' as any]: STATUS_HEX_MAP.resolved,
              cursor: 'pointer',
              outline: statusFilter === 'resolved' ? `2px solid ${STATUS_HEX_MAP.resolved}` : 'none',
            }}
            bodyStyle={{ padding: 16 }}
            onClick={() => { setPage(1); setStatusFilter(statusFilter === 'resolved' ? '' : 'resolved'); }}
          >
            <Statistic
              title={<><CheckCircleOutlined style={{ marginRight: 6, color: STATUS_HEX_MAP.resolved }} />已解决</>}
              value={stats.resolved}
              valueStyle={{ color: STATUS_HEX_MAP.resolved, fontSize: 22 }}
            />
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card
            className="bi-stat-card"
            style={{
              ['--accent-color' as any]: STATUS_HEX_MAP.closed,
              cursor: 'pointer',
              outline: statusFilter === 'closed' ? `2px solid ${STATUS_HEX_MAP.closed}` : 'none',
            }}
            bodyStyle={{ padding: 16 }}
            onClick={() => { setPage(1); setStatusFilter(statusFilter === 'closed' ? '' : 'closed'); }}
          >
            <Statistic
              title={<><MinusCircleOutlined style={{ marginRight: 6, color: STATUS_HEX_MAP.closed }} />已关闭</>}
              value={stats.closed}
              valueStyle={{ color: STATUS_HEX_MAP.closed, fontSize: 22 }}
            />
          </Card>
        </Col>
      </Row>

      <Card
        className="bi-card"
        title="反馈列表"
        extra={
          <div style={{ display: 'flex', gap: 8 }}>
            <Input
              placeholder="搜索 标题/内容/提交人"
              prefix={<SearchOutlined style={{ color: '#94a3b8' }} />}
              allowClear
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              style={{ width: 220 }}
            />
            <Select
              placeholder="筛选状态"
              allowClear
              value={statusFilter || undefined}
              onChange={v => { setStatusFilter(v || ''); setPage(1); }}
              style={{ width: 130 }}
              options={[
                { label: '待处理', value: 'pending' },
                { label: '处理中', value: 'processing' },
                { label: '已解决', value: 'resolved' },
                { label: '已关闭', value: 'closed' },
              ]}
            />
          </div>
        }
      >
        <Table<FeedbackItem>
          columns={columns}
          dataSource={filteredList}
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
          onRow={(record) => {
            const accent = statusHex(record.status);
            return {
              onClick: () => openDetail(record),
              style: {
                cursor: 'pointer',
                background: detailItem?.id === record.id ? `${accent}0d` : undefined,
                transition: 'background 120ms ease',
              },
            };
          }}
        />
      </Card>

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
        {detailItem && (() => {
          const accent = statusHex(detailItem.status);
          const pagePath = (() => {
            try {
              if (/^https?:/i.test(detailItem.pageUrl)) return new URL(detailItem.pageUrl).pathname;
              return detailItem.pageUrl;
            } catch { return detailItem.pageUrl; }
          })();
          return (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              <div style={{
                padding: '14px 18px',
                background: '#ffffff',
                border: '1px solid #e2e8f0',
                borderLeft: `4px solid ${accent}`,
                borderRadius: 6,
              }}>
                <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, marginBottom: 6, flexWrap: 'wrap' }}>
                  <span style={{ fontSize: 16, fontWeight: 600, color: '#0f172a', letterSpacing: '-0.01em' }}>
                    {detailItem.title}
                  </span>
                  <StatusChip status={detailItem.status} />
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12, color: '#64748b', flexWrap: 'wrap' }}>
                  <span>{detailItem.realName || detailItem.username}</span>
                  <span style={{ color: '#cbd5e1' }}>·</span>
                  <span style={{ fontVariantNumeric: 'tabular-nums' }}>{detailItem.createdAt}</span>
                  {detailItem.pageUrl && (
                    <>
                      <span style={{ color: '#cbd5e1' }}>·</span>
                      <a
                        href={pagePath}
                        target="_blank"
                        rel="noreferrer noopener"
                        style={{ color: accent, fontWeight: 500 }}
                      >
                        <LinkOutlined style={{ marginRight: 3 }} />{formatPageName(detailItem.pageUrl)}
                      </a>
                    </>
                  )}
                </div>
              </div>

              <div>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>问题描述</Typography.Text>
                <div style={{ whiteSpace: 'pre-wrap', background: '#f8fafc', padding: 12, borderRadius: 6, marginTop: 4, fontSize: 13, color: '#334155' }}>
                  {detailItem.content}
                </div>
              </div>

              {detailItem.attachments?.length > 0 && (
                <div>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>截图 ({detailItem.attachments.length})</Typography.Text>
                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 4 }}>
                    <Image.PreviewGroup>
                      {detailItem.attachments.map((url, i) => (
                        <Image
                          key={i}
                          src={`${API_BASE}${url}`}
                          width={88}
                          height={88}
                          style={{ objectFit: 'cover', borderRadius: 6, border: '1px solid #e2e8f0' }}
                        />
                      ))}
                    </Image.PreviewGroup>
                  </div>
                </div>
              )}

              <Row gutter={12}>
                <Col span={24}>
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
                </Col>
              </Row>

              <div>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>回复</Typography.Text>
                <Input.TextArea
                  value={replyText}
                  onChange={e => setReplyText(e.target.value)}
                  rows={3}
                  placeholder="输入回复内容（提交人后续将能看到）"
                  style={{ marginTop: 4 }}
                />
              </div>
            </div>
          );
        })()}
      </Modal>
    </div>
  );
};

export default Feedback;
