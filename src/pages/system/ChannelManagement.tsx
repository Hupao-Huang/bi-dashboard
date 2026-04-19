import React, { useEffect, useState, useCallback } from 'react';
import { Card, Table, Select, Input, Button, Tag, message, Space, Row, Col, Statistic, Modal } from 'antd';
import { SyncOutlined, SearchOutlined, WarningOutlined, SaveOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';

const DEPT_OPTIONS = [
  { value: '', label: '未分配' },
  { value: 'ecommerce', label: '电商' },
  { value: 'social', label: '社媒' },
  { value: 'offline', label: '线下' },
  { value: 'distribution', label: '分销' },
];

const DEPT_MAP: Record<string, { label: string; color: string }> = {
  ecommerce: { label: '电商', color: 'blue' },
  social: { label: '社媒', color: 'green' },
  offline: { label: '线下', color: 'orange' },
  distribution: { label: '分销', color: 'purple' },
};

const ChannelManagement: React.FC = () => {
  const [channels, setChannels] = useState<any[]>([]);
  const [platforms, setPlatforms] = useState<string[]>([]);
  const [total, setTotal] = useState(0);
  const [unmappedCount, setUnmappedCount] = useState(0);
  const [loading, setLoading] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [keyword, setKeyword] = useState('');
  const [filterDept, setFilterDept] = useState<string | undefined>(undefined);
  const [filterPlat, setFilterPlat] = useState<string | undefined>(undefined);
  const [pageSize, setPageSize] = useState(50);
  const [editingDepts, setEditingDepts] = useState<Record<number, string>>({});
  const [savingIds, setSavingIds] = useState<Set<number>>(new Set());

  const fetchData = useCallback(() => {
    setLoading(true);
    const params = new URLSearchParams();
    if (keyword) params.set('keyword', keyword);
    if (filterDept !== undefined && filterDept !== '') params.set('department', filterDept);
    if (filterPlat) params.set('platform', filterPlat);
    fetch(`${API_BASE}/api/admin/channels?${params}`, { credentials: 'include' })
      .then(res => res.json())
      .then(res => {
        const d = res.data || res;
        setChannels(d.channels || []);
        setPlatforms(d.platforms || []);
        setTotal(d.total || 0);
        setUnmappedCount(d.unmappedCount || 0);
        setLoading(false);
      })
      .catch(() => setLoading(false));
  }, [keyword, filterDept, filterPlat]);

  useEffect(() => { fetchData(); }, [fetchData]);

  const handleDeptEdit = (id: number, dept: string) => {
    setEditingDepts(prev => ({ ...prev, [id]: dept }));
  };

  const handleSave = (id: number) => {
    const dept = editingDepts[id];
    if (dept === undefined) return;
    setSavingIds(prev => new Set(prev).add(id));
    fetch(`${API_BASE}/api/admin/channels/${id}`, {
      method: 'PUT',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ department: dept }),
    })
      .then(res => res.json())
      .then(res => {
        if (res.data?.message || res.message) {
          message.success('保存成功');
          setEditingDepts(prev => { const n = { ...prev }; delete n[id]; return n; });
          fetchData();
        } else {
          message.error(res.error || '保存失败');
        }
      })
      .catch(() => message.error('保存失败'))
      .finally(() => setSavingIds(prev => { const n = new Set(prev); n.delete(id); return n; }));
  };

  const handleSync = () => {
    Modal.confirm({
      title: '从吉客云同步渠道',
      content: '将从吉客云拉取最新的渠道数据，已有的部门映射会保留。确认同步？',
      onOk: () => {
        setSyncing(true);
        fetch(`${API_BASE}/api/admin/channels/sync`, { method: 'POST', credentials: 'include' })
          .then(res => res.json())
          .then(res => {
            setSyncing(false);
            if (res.data?.message || res.message) {
              message.success('同步完成');
              fetchData();
            } else {
              message.error(res.error || '同步失败');
            }
          })
          .catch(() => { setSyncing(false); message.error('同步失败'); });
      },
    });
  };

  const columns = [
    {
      title: '渠道名称', dataIndex: 'channelName', key: 'name', width: 260, ellipsis: true,
      render: (v: string, r: any) => (
        <span>
          {v}
          {!r.department && <WarningOutlined style={{ color: '#f5222d', marginLeft: 6 }} />}
        </span>
      ),
    },
    {
      title: '平台', dataIndex: 'onlinePlatName', key: 'plat', width: 120,
      render: (v: string) => v || <span style={{ color: '#bbb' }}>-</span>,
    },
    { title: '分类', dataIndex: 'cateName', key: 'cate', width: 100 },
    { title: '吉客云部门', dataIndex: 'departName', key: 'jkyDept', width: 140, ellipsis: true },
    { title: '公司', dataIndex: 'companyName', key: 'company', width: 160, ellipsis: true },
    { title: '负责人', dataIndex: 'responsibleUser', key: 'user', width: 100 },
    {
      title: 'BI部门', dataIndex: 'department', key: 'dept', width: 200,
      render: (v: string, r: any) => {
        const editVal = editingDepts[r.id];
        const isEdited = editVal !== undefined && editVal !== (v || '');
        const displayVal = editVal !== undefined ? editVal : (v || '');
        return (
          <Space size={4}>
            <Select
              size="small"
              value={displayVal}
              style={{ width: 100 }}
              onChange={(val: string) => handleDeptEdit(r.id, val)}
              options={DEPT_OPTIONS}
              status={!displayVal ? 'warning' : undefined}
            />
            {isEdited && (
              <Button
                type="primary"
                size="small"
                icon={<SaveOutlined />}
                loading={savingIds.has(r.id)}
                onClick={() => handleSave(r.id)}
              >
                保存
              </Button>
            )}
          </Space>
        );
      },
      filters: [
        { text: '电商', value: 'ecommerce' },
        { text: '社媒', value: 'social' },
        { text: '线下', value: 'offline' },
        { text: '分销', value: 'distribution' },
        { text: '未分配', value: '' },
      ],
      onFilter: (value: any, record: any) => (record.department || '') === value,
    },
    {
      title: '部门标签', key: 'deptTag', width: 80,
      render: (_: any, r: any) => {
        const d = DEPT_MAP[r.department];
        return d ? <Tag color={d.color}>{d.label}</Tag> : <Tag color="red">未分配</Tag>;
      },
    },
  ];

  return (
    <div>
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={8}>
          <Card><Statistic title="总渠道数" value={total} /></Card>
        </Col>
        <Col xs={8}>
          <Card><Statistic title="已映射" value={total - unmappedCount} valueStyle={{ color: '#52c41a' }} /></Card>
        </Col>
        <Col xs={8}>
          <Card><Statistic title="未映射" value={unmappedCount} valueStyle={{ color: unmappedCount > 0 ? '#f5222d' : '#52c41a' }} /></Card>
        </Col>
      </Row>

      <Card
        title="渠道管理"
        extra={
          <Button type="primary" icon={<SyncOutlined spin={syncing} />} loading={syncing} onClick={handleSync}>
            从吉客云同步
          </Button>
        }
      >
        <Space style={{ marginBottom: 16 }} wrap>
          <Input
            placeholder="搜索渠道名称/编码/负责人"
            prefix={<SearchOutlined />}
            allowClear
            style={{ width: 260 }}
            value={keyword}
            onChange={e => setKeyword(e.target.value)}
            onPressEnter={fetchData}
          />
          <Select
            placeholder="按BI部门筛选"
            allowClear
            style={{ width: 160 }}
            value={filterDept}
            onChange={v => setFilterDept(v)}
            options={[
              { value: 'unmapped', label: '未分配' },
              { value: 'ecommerce', label: '电商' },
              { value: 'social', label: '社媒' },
              { value: 'offline', label: '线下' },
              { value: 'distribution', label: '分销' },
            ]}
          />
          <Select
            placeholder="按平台筛选"
            allowClear
            style={{ width: 160 }}
            value={filterPlat}
            onChange={v => setFilterPlat(v)}
            options={platforms.map(p => ({ value: p, label: p }))}
          />
          <Button onClick={fetchData}>查询</Button>
        </Space>
        <Table
          dataSource={channels}
          columns={columns}
          rowKey="id"
          loading={loading}
          size="small"
          pagination={{ pageSize, showSizeChanger: true, pageSizeOptions: ['50', '100', '233'], showTotal: t => `共 ${t} 条`, onShowSizeChange: (_, size) => setPageSize(size) }}
          scroll={{ x: 1200, y: 600 }}
          rowClassName={(r: any) => !r.department ? 'channel-unmapped-row' : ''}
        />
      </Card>

      <style>{`
        .channel-unmapped-row { background: #fff2f0 !important; }
        .channel-unmapped-row:hover td { background: #ffebe8 !important; }
      `}</style>
    </div>
  );
};

export default ChannelManagement;
