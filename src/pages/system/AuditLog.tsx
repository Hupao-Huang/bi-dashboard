import React, { useEffect, useState } from 'react';
import { Table, Tag, Input, Select, DatePicker, Card, Typography, Row, Col, Button } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import { API_BASE } from '../../config';
import { useAuth } from '../../auth/AuthContext';

const { RangePicker } = DatePicker;
const { Title } = Typography;

const actionLabels: Record<string, { label: string; color: string }> = {
  login: { label: '登录', color: 'green' },
  logout: { label: '退出', color: 'default' },
  page_view: { label: '浏览', color: 'blue' },
  export: { label: '导出', color: 'orange' },
  permission_change: { label: '权限变更', color: 'red' },
};

const AuditLog: React.FC = () => {
  const { session } = useAuth();
  const [data, setData] = useState<any[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [loading, setLoading] = useState(false);
  const [action, setAction] = useState<string>('');
  const [username, setUsername] = useState('');
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null] | null>(null);

  const fetchData = async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams({
        page: String(page),
        pageSize: String(pageSize),
      });
      if (action) params.set('action', action);
      if (username) params.set('username', username);
      if (dateRange && dateRange[0]) params.set('startDate', dateRange[0].format('YYYY-MM-DD'));
      if (dateRange && dateRange[1]) params.set('endDate', dateRange[1].format('YYYY-MM-DD'));

      const res = await fetch(`${API_BASE}/api/admin/audit-logs?${params}`, { credentials: 'include' });
      if (!res.ok) throw new Error();
      const json = await res.json();
      const payload = json.data || json;
      setData(payload.list || []);
      setTotal(payload.total || 0);
    } catch {
      setData([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (session) fetchData();
  }, [session, page, pageSize]);

  const columns: ColumnsType<any> = [
    {
      title: '时间',
      dataIndex: 'createdAt',
      width: 170,
      render: (v: string) => <span style={{ fontSize: 12, color: '#64748b' }}>{v}</span>,
    },
    {
      title: '用户',
      width: 160,
      render: (_: any, r: any) => (
        <div>
          <div style={{ fontWeight: 500, fontSize: 13 }}>{r.realName || r.username}</div>
          {r.realName && <div style={{ fontSize: 11, color: '#94a3b8' }}>@{r.username}</div>}
        </div>
      ),
    },
    {
      title: '操作',
      dataIndex: 'action',
      width: 100,
      render: (v: string) => {
        const info = actionLabels[v] || { label: v, color: 'default' };
        return <Tag color={info.color}>{info.label}</Tag>;
      },
    },
    {
      title: '资源',
      dataIndex: 'resource',
      ellipsis: true,
    },
    {
      title: '详情',
      dataIndex: 'detail',
      ellipsis: true,
      render: (v: string) => {
        if (!v) return '-';
        try {
          const obj = JSON.parse(v);
          return <span style={{ fontSize: 12, color: '#64748b' }}>{JSON.stringify(obj, null, 0)}</span>;
        } catch {
          return <span style={{ fontSize: 12, color: '#64748b' }}>{v}</span>;
        }
      },
    },
    {
      title: 'IP',
      dataIndex: 'ip',
      width: 130,
      render: (v: string) => <span style={{ fontSize: 12, fontFamily: 'monospace', color: '#94a3b8' }}>{v || '-'}</span>,
    },
  ];

  return (
    <div style={{ padding: 0 }}>
      <Title level={4} style={{ marginBottom: 20 }}>审计日志</Title>

      <Card className="bi-filter-card" style={{ marginBottom: 16 }}>
        <Row align="middle" gutter={[16, 12]} wrap>
          <Col>
            <span style={{ fontWeight: 500, marginRight: 8 }}>操作类型：</span>
            <Select
              placeholder="全部"
              allowClear
              style={{ width: 150 }}
              value={action || undefined}
              onChange={v => setAction(v || '')}
            >
              {Object.entries(actionLabels).map(([k, v]) => (
                <Select.Option key={k} value={k}>{v.label}</Select.Option>
              ))}
            </Select>
          </Col>
          <Col>
            <span style={{ fontWeight: 500, marginRight: 8 }}>用户：</span>
            <Input
              placeholder="用户名搜索"
              allowClear
              style={{ width: 180 }}
              value={username}
              onChange={e => setUsername(e.target.value)}
              onPressEnter={fetchData}
            />
          </Col>
          <Col>
            <span style={{ fontWeight: 500, marginRight: 8 }}>时间范围：</span>
            <RangePicker
              value={dateRange}
              onChange={v => setDateRange(v)}
            />
          </Col>
          <Col>
            <Button type="primary" onClick={fetchData}>查询</Button>
          </Col>
        </Row>
      </Card>

      <Table
        dataSource={data}
        columns={columns}
        rowKey="id"
        loading={loading}
        size="small"
        scroll={{ x: 900 }}
        pagination={{
          current: page,
          pageSize,
          total,
          showSizeChanger: true,
          showTotal: t => `共 ${t} 条`,
          onChange: (p, ps) => { setPage(p); setPageSize(ps); },
        }}
      />
    </div>
  );
};

export default AuditLog;
