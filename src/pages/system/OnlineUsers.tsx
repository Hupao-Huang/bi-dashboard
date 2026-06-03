// 在线用户页面
//   Tab 1「实时在线」: user_sessions.last_active_at 在最近 N 分钟内 (GET /api/admin/online-users, 30s 自动刷)
//   Tab 2「全员活动」: 所有用户的操作统计总表 + 点行下钻看个人活动面板 (GET /api/admin/users-activity / user-activity)
import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Card, Table, Tag, Space, Select, Button, Tooltip, Typography, Tabs, Drawer } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ReloadOutlined, UserOutlined, EyeOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';
import UserActivityPanel, { UserActivity } from '../../components/UserActivityPanel';

const { Text } = Typography;

interface OnlineUser {
  id: number;
  username: string;
  real_name: string;
  department: string;
  dingtalk_real_name: string;
  last_active_at: string;
  ip: string;
  user_agent: string;
  sessions_count: number;
  seconds_ago: number;
}

interface OnlineResp {
  code: number;
  data?: {
    minutes: number;
    count: number;
    users: OnlineUser[];
  };
}

// 全员活动总表一行
interface UserActivityRow {
  userId: number;
  username: string;
  realName: string;
  dingtalkRealName: string;
  department: string;
  status: string;
  total: number;
  today: number;
  last7d: number;
  last30d: number;
  activeDays: number;
  lastActiveAt: string;
}

// 几秒前 → 友好显示
const fmtAgo = (sec: number) => {
  if (sec < 60) return `${sec} 秒前`;
  const m = Math.floor(sec / 60);
  if (m < 60) return `${m} 分钟前`;
  const h = Math.floor(m / 60);
  return `${h} 小时前`;
};

// User-Agent 简化
const fmtUA = (ua: string) => {
  if (!ua) return '-';
  if (ua.includes('Edg/')) return 'Edge';
  if (ua.includes('Chrome/')) return 'Chrome';
  if (ua.includes('Firefox/')) return 'Firefox';
  if (ua.includes('Safari/')) return 'Safari';
  if (ua.includes('curl')) return 'curl';
  return '其他';
};

const displayName = (r: { realName?: string; dingtalkRealName?: string; username: string }) =>
  r.realName || r.dingtalkRealName || r.username;

// ============ Tab 1: 实时在线 ============
const OnlineTab: React.FC = () => {
  const [users, setUsers] = useState<OnlineUser[]>([]);
  const [loading, setLoading] = useState(false);
  const [minutes, setMinutes] = useState(5);
  const [lastRefresh, setLastRefresh] = useState<string>('');
  const timerRef = useRef<number | null>(null);

  const fetchData = useCallback(async (m: number) => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/admin/online-users?minutes=${m}`, { credentials: 'include' });
      const json: OnlineResp = await res.json();
      if (json.code === 200 && json.data) {
        setUsers(json.data.users);
        setLastRefresh(new Date().toLocaleTimeString('zh-CN', { hour12: false }));
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchData(minutes); }, [fetchData, minutes]);

  // 30s 自动刷新
  useEffect(() => {
    timerRef.current = window.setInterval(() => fetchData(minutes), 30_000);
    return () => { if (timerRef.current) window.clearInterval(timerRef.current); };
  }, [fetchData, minutes]);

  const columns = [
    {
      title: '姓名',
      key: 'name',
      width: 140,
      render: (_: any, row: OnlineUser) => (
        <Space size={6}>
          <UserOutlined style={{ color: '#52c41a' }} />
          <strong>{row.real_name || row.dingtalk_real_name || row.username}</strong>
        </Space>
      ),
    },
    { title: '账号', dataIndex: 'username', key: 'username', width: 140 },
    {
      title: '部门', dataIndex: 'department', key: 'department', width: 120,
      render: (v: string) => v ? <Tag>{v}</Tag> : <Text type="secondary">-</Text>,
    },
    {
      title: '最近活动', key: 'last_active_at', width: 180,
      render: (_: any, row: OnlineUser) => (
        <Space direction="vertical" size={0}>
          <span>{row.last_active_at}</span>
          <Text type="secondary" style={{ fontSize: 12 }}>{fmtAgo(row.seconds_ago)}</Text>
        </Space>
      ),
    },
    { title: 'IP', dataIndex: 'ip', key: 'ip', width: 140 },
    { title: '设备', key: 'user_agent', width: 100, render: (_: any, row: OnlineUser) => fmtUA(row.user_agent) },
    {
      title: 'session 数', dataIndex: 'sessions_count', key: 'sessions_count', width: 100, align: 'center' as const,
      render: (v: number) => v > 1 ? <Tag color="blue">{v}</Tag> : v,
    },
  ];

  return (
    <Card
      title={
        <Space>
          <span>在线用户</span>
          <Tag color="green">{users.length} 人在线</Tag>
          <Text type="secondary" style={{ fontSize: 12 }}>(最近 {minutes} 分钟有操作)</Text>
        </Space>
      }
      extra={
        <Space>
          <Text type="secondary" style={{ fontSize: 12 }}>口径:</Text>
          <Select
            size="small" value={minutes} onChange={setMinutes} style={{ width: 110 }}
            options={[
              { value: 1, label: '近 1 分钟' },
              { value: 5, label: '近 5 分钟' },
              { value: 10, label: '近 10 分钟' },
              { value: 30, label: '近 30 分钟' },
              { value: 60, label: '近 1 小时' },
            ]}
          />
          {lastRefresh && <Text type="secondary" style={{ fontSize: 12 }}>更新于 {lastRefresh}</Text>}
          <Tooltip title="手动刷新 (页面每 30 秒自动刷)">
            <Button size="small" icon={<ReloadOutlined />} onClick={() => fetchData(minutes)} loading={loading} />
          </Tooltip>
        </Space>
      }
    >
      <Table
        dataSource={users} columns={columns} rowKey="id" size="small" pagination={false}
        loading={loading && users.length === 0}
        locale={{ emptyText: `最近 ${minutes} 分钟内没有用户操作` }}
      />
    </Card>
  );
};

// ============ Tab 2: 全员活动 ============
const AllUsersActivityTab: React.FC = () => {
  const [rows, setRows] = useState<UserActivityRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [drawerUser, setDrawerUser] = useState<UserActivityRow | null>(null);
  const [detail, setDetail] = useState<UserActivity | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/admin/users-activity`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200 && json.data) setRows(json.data.users);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const reqIdRef = useRef(0);
  const openDetail = useCallback(async (row: UserActivityRow) => {
    const myReq = ++reqIdRef.current;
    setDrawerUser(row);
    setDetail(null);
    setDetailLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/admin/user-activity?userId=${row.userId}`, { credentials: 'include' });
      const json = await res.json();
      if (reqIdRef.current !== myReq) return; // 已被更新的点击取代, 丢弃这次旧响应
      setDetail(json.code === 200 && json.data ? json.data : null);
    } finally {
      if (reqIdRef.current === myReq) setDetailLoading(false);
    }
  }, []);

  const numSorter = (k: 'today' | 'last7d' | 'last30d' | 'total' | 'activeDays') =>
    (a: UserActivityRow, b: UserActivityRow) => a[k] - b[k];

  const columns: ColumnsType<UserActivityRow> = [
    {
      title: '姓名', key: 'name', width: 130, fixed: 'left',
      render: (_, r) => <strong>{displayName(r)}</strong>,
    },
    { title: '账号', dataIndex: 'username', key: 'username', width: 120 },
    {
      title: '部门', dataIndex: 'department', key: 'department', width: 110,
      render: (v: string) => v ? <Tag>{v}</Tag> : <Text type="secondary">-</Text>,
    },
    { title: '今日', dataIndex: 'today', key: 'today', width: 80, align: 'right', sorter: numSorter('today') },
    { title: '近 7 天', dataIndex: 'last7d', key: 'last7d', width: 90, align: 'right', sorter: numSorter('last7d') },
    { title: '近 30 天', dataIndex: 'last30d', key: 'last30d', width: 95, align: 'right', sorter: numSorter('last30d') },
    {
      title: '总操作', dataIndex: 'total', key: 'total', width: 90, align: 'right',
      sorter: numSorter('total'), defaultSortOrder: 'descend',
    },
    { title: '活跃天', dataIndex: 'activeDays', key: 'activeDays', width: 80, align: 'right', sorter: numSorter('activeDays') },
    {
      title: '最近活动', dataIndex: 'lastActiveAt', key: 'lastActiveAt', width: 160,
      render: (v: string) => v || <Text type="secondary">从未</Text>,
    },
    {
      title: '', key: 'op', width: 70, fixed: 'right',
      render: (_, r) => (
        <Button type="link" size="small" icon={<EyeOutlined />} onClick={(e) => { e.stopPropagation(); openDetail(r); }}>
          详情
        </Button>
      ),
    },
  ];

  return (
    <Card
      title={
        <Space>
          <span>全员操作活动</span>
          <Tag color="blue">{rows.length} 人</Tag>
          <Text type="secondary" style={{ fontSize: 12 }}>(点任意一行看 TA 的活动详情)</Text>
        </Space>
      }
      extra={<Button size="small" icon={<ReloadOutlined />} onClick={load} loading={loading}>刷新</Button>}
    >
      <Table
        dataSource={rows} columns={columns} rowKey="userId" size="small" loading={loading}
        scroll={{ x: 1010 }}
        pagination={{ pageSize: 20, hideOnSinglePage: true, showSizeChanger: false }}
        onRow={(r) => ({ onClick: () => openDetail(r), style: { cursor: 'pointer' } })}
      />
      <Drawer
        title={drawerUser ? `${displayName(drawerUser)} 的操作活动` : '操作活动'}
        width={780} open={!!drawerUser} onClose={() => setDrawerUser(null)} destroyOnClose
      >
        <UserActivityPanel data={detail} loading={detailLoading} />
      </Drawer>
    </Card>
  );
};

const OnlineUsers: React.FC = () => (
  <Tabs
    defaultActiveKey="online"
    items={[
      { key: 'online', label: '实时在线', children: <OnlineTab /> },
      { key: 'activity', label: '全员活动', children: <AllUsersActivityTab /> },
    ]}
  />
);

export default OnlineUsers;
