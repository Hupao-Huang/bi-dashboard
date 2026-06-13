import React, { useState, useEffect, useCallback } from 'react';
import { Button, Card, Table, Tabs, Typography, Tooltip, Empty, message, Modal, Input, Tag, Popconfirm, Switch, Space } from 'antd';
import { DownloadOutlined, EditOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import DateFilter from '../../components/DateFilter';
import { API_BASE, DATA_START_DATE, DATA_END_DATE } from '../../config';
import { useAuth } from '../../auth/AuthContext';

const { Text } = Typography;

// 平台 Tab 顺序与客服总览一致
const PLATFORM_ORDER = ['天猫', '抖音', '京东', '拼多多', '快手', '小红书'];
const ALL_SHOPS = '__all__';

// 与后端 comment.go CommentList 返回的 item 对齐
interface CommentItem {
  platform: string;
  shopName: string;
  date: string;
  orderNo: string;
  product: string;         // 显示名(改后或原始)
  productOriginal: string; // RPA 原始名
  edited: boolean;         // 是否被客服改过
  content: string;
  score: number | null;
  scoreText: string;
  hash: string;            // content_hash, 改名/恢复用
}

const orderPlatforms = (pfs: string[]) => [...pfs].sort((a, b) => {
  const ia = PLATFORM_ORDER.indexOf(a);
  const ib = PLATFORM_ORDER.indexOf(b);
  return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib);
});

const CommentData: React.FC = () => {
  const { hasPermission } = useAuth();
  const canEdit = hasPermission('customer.comment:edit'); // 改名/删除仅管理员
  const [platforms, setPlatforms] = useState<string[]>([]);
  const [shops, setShops] = useState<string[]>([]);
  const [activePlatform, setActivePlatform] = useState('');
  const [activeShop, setActiveShop] = useState(ALL_SHOPS);
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);
  const [list, setList] = useState<CommentItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [loading, setLoading] = useState(false);
  const [showDeleted, setShowDeleted] = useState(false); // 显示已删除视图(仅管理员)

  // 改名弹窗
  const [editOpen, setEditOpen] = useState(false);
  const [editHash, setEditHash] = useState('');
  const [editOriginal, setEditOriginal] = useState('');
  const [editName, setEditName] = useState('');
  const [editIsEdited, setEditIsEdited] = useState(false);
  const [editSaving, setEditSaving] = useState(false);

  // 初次: 拉平台列表, 默认选第一个
  useEffect(() => {
    (async () => {
      try {
        const res = await fetch(`${API_BASE}/api/customer/comment-options`, { credentials: 'include' });
        const json = await res.json();
        if (res.ok && json.data) {
          const pfs: string[] = json.data.platforms || [];
          setPlatforms(pfs);
          if (pfs.length && !activePlatform) setActivePlatform(orderPlatforms(pfs)[0]);
        }
      } catch {
        /* 选项加载失败不阻塞 */
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 平台切换: 拉该平台店铺 + 店铺重置为全部
  useEffect(() => {
    if (!activePlatform) return;
    (async () => {
      try {
        const res = await fetch(`${API_BASE}/api/customer/comment-options?platform=${encodeURIComponent(activePlatform)}`, { credentials: 'include' });
        const json = await res.json();
        if (res.ok && json.data) setShops(json.data.shops || []);
      } catch {
        /* ignore */
      }
    })();
    setActiveShop(ALL_SHOPS);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activePlatform]);

  const fetchList = useCallback(async (p: number, ps: number) => {
    if (!activePlatform) return;
    setLoading(true);
    try {
      const params = new URLSearchParams();
      params.set('platform', activePlatform);
      if (activeShop && activeShop !== ALL_SHOPS) params.set('shop', activeShop);
      params.set('date_from', startDate);
      params.set('date_to', endDate);
      if (showDeleted) params.set('show_deleted', '1');
      params.set('page', String(p));
      params.set('page_size', String(ps));
      const res = await fetch(`${API_BASE}/api/customer/comments?${params.toString()}`, { credentials: 'include' });
      const json = await res.json();
      if (res.ok && json.data) {
        setList(json.data.list || []);
        setTotal(json.data.total || 0);
        setPage(json.data.page || p);
        setPageSize(ps);
      } else {
        message.error(json.msg || json.error || '查询失败');
      }
    } catch {
      message.error('网络错误，查询失败');
    } finally {
      setLoading(false);
    }
  }, [activePlatform, activeShop, startDate, endDate, showDeleted]);

  // 平台 / 店铺 / 日期变 → 回第 1 页查询
  useEffect(() => {
    fetchList(1, pageSize);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activePlatform, activeShop, startDate, endDate, showDeleted]);

  const handleExport = () => {
    if (!activePlatform) return;
    const params = new URLSearchParams();
    params.set('platform', activePlatform);
    if (activeShop && activeShop !== ALL_SHOPS) params.set('shop', activeShop);
    params.set('date_from', startDate);
    params.set('date_to', endDate);
    window.open(`${API_BASE}/api/customer/comments/export?${params.toString()}`, '_blank');
  };

  const openEdit = (record: CommentItem) => {
    setEditHash(record.hash);
    setEditOriginal(record.productOriginal);
    setEditName(record.product);
    setEditIsEdited(record.edited);
    setEditOpen(true);
  };

  const doSaveRename = async () => {
    if (!editName.trim()) { message.warning('请输入新的商品名称'); return; }
    setEditSaving(true);
    try {
      const res = await fetch(`${API_BASE}/api/customer/comments/rename`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ hash: editHash, name: editName.trim() }),
      });
      const json = await res.json();
      if (res.ok) {
        message.success('已修改商品名称');
        setEditOpen(false);
        fetchList(page, pageSize);
      } else {
        message.error(json.msg || json.error || '保存失败');
      }
    } catch {
      message.error('网络错误，保存失败');
    } finally {
      setEditSaving(false);
    }
  };

  const doRestore = async () => {
    setEditSaving(true);
    try {
      const res = await fetch(`${API_BASE}/api/customer/comments/restore`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ hash: editHash }),
      });
      const json = await res.json();
      if (res.ok) {
        message.success('已恢复为 RPA 原始名称');
        setEditOpen(false);
        fetchList(page, pageSize);
      } else {
        message.error(json.msg || json.error || '恢复失败');
      }
    } catch {
      message.error('网络错误，恢复失败');
    } finally {
      setEditSaving(false);
    }
  };

  const doDelete = async (hash: string) => {
    try {
      const res = await fetch(`${API_BASE}/api/customer/comments/delete`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ hash }),
      });
      const json = await res.json();
      if (res.ok) {
        message.success('已删除（原始数据仍保留）');
        fetchList(page, pageSize);
      } else {
        message.error(json.msg || json.error || '删除失败');
      }
    } catch {
      message.error('网络错误，删除失败');
    }
  };

  const doUndelete = async (hash: string) => {
    try {
      const res = await fetch(`${API_BASE}/api/customer/comments/undelete`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ hash }),
      });
      const json = await res.json();
      if (res.ok) {
        message.success('已恢复到正常列表');
        fetchList(page, pageSize);
      } else {
        message.error(json.msg || json.error || '恢复失败');
      }
    } catch {
      message.error('网络错误，恢复失败');
    }
  };

  const columns: ColumnsType<CommentItem> = [
    { title: '店铺', dataIndex: 'shopName', key: 'shop', width: 180, ellipsis: true },
    { title: '时间', dataIndex: 'date', key: 'date', width: 110, render: (v: string) => v || '-' },
    { title: '订单编号', dataIndex: 'orderNo', key: 'orderNo', width: 215, render: (v: string) => <span style={{ whiteSpace: 'nowrap' }}>{v}</span> },
    {
      title: '商品名称', dataIndex: 'product', key: 'product', width: 450,
      render: (v: string, record: CommentItem) => (
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <Tooltip title={record.edited ? `RPA原始：${record.productOriginal}` : v} placement="topLeft">
            <span style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{v}</span>
          </Tooltip>
          {record.edited && <Tag color="blue" style={{ margin: 0, flexShrink: 0 }}>已改</Tag>}
          <Tooltip title="修改商品名称">
            <EditOutlined style={{ flexShrink: 0, cursor: 'pointer', color: '#1677ff' }} onClick={() => openEdit(record)} />
          </Tooltip>
        </div>
      ),
    },
    {
      title: '评价内容', dataIndex: 'content', key: 'content',
      render: (v: string) => (
        <Tooltip title={v} placement="topLeft">
          <span style={{ display: 'block', maxWidth: 380, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{v}</span>
        </Tooltip>
      ),
    },
    { title: '评分', dataIndex: 'scoreText', key: 'score', width: 100, render: (v: string) => v || '-' },
    {
      title: '操作', key: 'op', width: 90, fixed: 'right' as const,
      render: (_: unknown, record: CommentItem) => (showDeleted ? (
        <Popconfirm
          title="恢复这条评价？"
          description="放回正常列表"
          onConfirm={() => doUndelete(record.hash)}
          okText="恢复"
          cancelText="取消"
        >
          <Button size="small" type="link">恢复</Button>
        </Popconfirm>
      ) : (
        <Popconfirm
          title="删除这条评价？"
          description="原始数据保留，仅从列表隐藏"
          onConfirm={() => doDelete(record.hash)}
          okText="删除"
          cancelText="取消"
          okButtonProps={{ danger: true }}
        >
          <Button size="small" type="link" danger>删除</Button>
        </Popconfirm>
      )),
    },
  ];

  const shopTabItems = [
    { key: ALL_SHOPS, label: '全部' },
    ...shops.map((s) => ({ key: s, label: s })),
  ];

  return (
    <div>
      <DateFilter start={startDate} end={endDate} onChange={(s, e) => { setStartDate(s); setEndDate(e); }} />
      <Card className="bi-filter-card" style={{ marginBottom: 16 }}>
        <Tabs
          activeKey={activePlatform}
          onChange={setActivePlatform}
          items={orderPlatforms(platforms).map((p) => ({ key: p, label: p }))}
        />
      </Card>
      <Card title={`${activePlatform || ''}评论明细`}>
        <Tabs
          activeKey={activeShop}
          onChange={setActiveShop}
          size="small"
          items={shopTabItems}
          style={{ marginBottom: 8 }}
        />
        <div style={{ marginBottom: 12, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <Space>
            <Text type="secondary">共 {total} 条评价</Text>
            {canEdit && (
              <Switch
                checkedChildren="显示已删除"
                unCheckedChildren="正常视图"
                checked={showDeleted}
                onChange={(v) => setShowDeleted(v)}
              />
            )}
          </Space>
          <Button icon={<DownloadOutlined />} onClick={handleExport} disabled={!activePlatform || total === 0}>
            导出 Excel
          </Button>
        </div>
        {list.length === 0 && !loading ? (
          <Empty description={`暂无${activePlatform}评价数据`} />
        ) : (
          <Table
            size="small"
            rowKey={(r) => r.hash}
            columns={columns}
            dataSource={list}
            loading={loading}
            scroll={{ x: 1325 }}
            pagination={{
              current: page,
              pageSize,
              total,
              showSizeChanger: true,
              pageSizeOptions: ['20', '50', '100', '200'],
              showTotal: (t) => `共 ${t} 条`,
              onChange: (p, ps) => fetchList(p, ps),
            }}
          />
        )}
      </Card>

      <Modal
        title="修改商品名称"
        open={editOpen}
        onCancel={() => setEditOpen(false)}
        maskClosable={false}
        footer={[
          <Button key="cancel" onClick={() => setEditOpen(false)}>取消</Button>,
          (editIsEdited && canEdit) ? (
            <Button key="restore" danger loading={editSaving} onClick={doRestore}>恢复原始</Button>
          ) : null,
          <Button key="ok" type="primary" loading={editSaving} onClick={doSaveRename}>保存</Button>,
        ]}
      >
        <div style={{ marginBottom: 8 }}>
          <Text type="secondary">RPA 原始名称（永久保留，不会被改动）：</Text>
          <div style={{ marginTop: 4, color: 'var(--text-tertiary)' }}>{editOriginal}</div>
        </div>
        <div>
          <Text type="secondary">改成：</Text>
          <Input.TextArea
            value={editName}
            onChange={(e) => setEditName(e.target.value)}
            autoSize={{ minRows: 2, maxRows: 4 }}
            placeholder="输入新的商品名称"
            style={{ marginTop: 4 }}
            maxLength={255}
          />
        </div>
      </Modal>
    </div>
  );
};

export default CommentData;
