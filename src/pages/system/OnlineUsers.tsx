// 在线用户页面 (严格在线: user_sessions.last_active_at 在最近 N 分钟内)
//
// 数据来源: GET /api/admin/online-users?minutes=N (默认 5)
// 自动刷新: 30s
import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Card, Table, Tag, Space, Select, Button, Tooltip, Typography } from 'antd';
import { ReloadOutlined, UserOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';

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

const OnlineUsers: React.FC = () => {
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

  // 首次加载 + minutes 变化时拉
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
      render: (_: any, row: OnlineUser) => {
        // 优先 real_name, 然后钉钉真名, 最后 username
        const name = row.real_name || row.dingtalk_real_name || row.username;
        return (
          <Space size={6}>
            <UserOutlined style={{ color: '#52c41a' }} />
            <strong>{name}</strong>
          </Space>
        );
      },
    },
    { title: '账号', dataIndex: 'username', key: 'username', width: 140 },
    {
      title: '部门',
      dataIndex: 'department',
      key: 'department',
      width: 120,
      render: (v: string) => v ? <Tag>{v}</Tag> : <Text type="secondary">-</Text>,
    },
    {
      title: '最近活动',
      key: 'last_active_at',
      width: 180,
      render: (_: any, row: OnlineUser) => (
        <Space direction="vertical" size={0}>
          <span>{row.last_active_at}</span>
          <Text type="secondary" style={{ fontSize: 12 }}>{fmtAgo(row.seconds_ago)}</Text>
        </Space>
      ),
    },
    { title: 'IP', dataIndex: 'ip', key: 'ip', width: 140 },
    {
      title: '设备',
      key: 'user_agent',
      width: 100,
      render: (_: any, row: OnlineUser) => fmtUA(row.user_agent),
    },
    {
      title: 'session 数',
      dataIndex: 'sessions_count',
      key: 'sessions_count',
      width: 100,
      align: 'center' as const,
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
            size="small"
            value={minutes}
            onChange={setMinutes}
            style={{ width: 110 }}
            options={[
              { value: 1, label: '近 1 分钟' },
              { value: 5, label: '近 5 分钟' },
              { value: 10, label: '近 10 分钟' },
              { value: 30, label: '近 30 分钟' },
              { value: 60, label: '近 1 小时' },
            ]}
          />
          {lastRefresh && (
            <Text type="secondary" style={{ fontSize: 12 }}>更新于 {lastRefresh}</Text>
          )}
          <Tooltip title="手动刷新 (页面每 30 秒自动刷)">
            <Button size="small" icon={<ReloadOutlined />} onClick={() => fetchData(minutes)} loading={loading} />
          </Tooltip>
        </Space>
      }
    >
      <Table
        dataSource={users}
        columns={columns}
        rowKey="id"
        size="small"
        pagination={false}
        loading={loading && users.length === 0}
        locale={{ emptyText: `最近 ${minutes} 分钟内没有用户操作` }}
      />
    </Card>
  );
};

export default OnlineUsers;
