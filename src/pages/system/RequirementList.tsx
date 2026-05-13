import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button, Card, Col, DatePicker, Input, Modal, Row, Select, Statistic, Table, Tabs, Tag, Typography, message,
} from 'antd';
import RequirementKanban from './RequirementKanban';
import type { ColumnsType } from 'antd/es/table';
import {
  ClockCircleOutlined, CheckCircleOutlined,
  RocketOutlined, FieldTimeOutlined, SearchOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import { API_BASE } from '../../config';

const STATUS_HEX_MAP: Record<string, string> = {
  pending: '#f59e0b',
  accepted: '#1677ff',
  scheduled: '#8b5cf6',
  in_progress: '#06b6d4',
  done: '#16a34a',
  shelved: '#94a3b8',
  rejected: '#ef4444',
};

const STATUS_LABEL: Record<string, string> = {
  pending: '待评估', accepted: '已接受', scheduled: '排期中', in_progress: '开发中',
  done: '已完成', shelved: '已搁置', rejected: '已拒绝',
};

const PRIORITY_HEX: Record<string, string> = {
  P0: '#dc2626', P1: '#ea580c', P2: '#1677ff', P3: '#94a3b8',
};

interface RequirementItem {
  id: number;
  title: string;
  content: string;
  submitterUserId: number;
  submitterName: string;
  submitterDept: string;
  priority: string;
  status: string;
  targetVersion: string;
  expectedDate: string | null;
  actualDate: string | null;
  tag: string;
  adminRemark: string;
  createdAt: string;
  updatedAt: string;
}

interface StatsData {
  status: Record<string, number>;
  priority: Record<string, number>;
}

const StatusChip: React.FC<{ status: string }> = ({ status }) => {
  const hex = STATUS_HEX_MAP[status] || '#94a3b8';
  return (
    <span style={{
      display: 'inline-block', padding: '1px 8px',
      background: `${hex}12`, color: hex, border: `1px solid ${hex}30`,
      borderRadius: 3, fontWeight: 500, fontSize: 12, lineHeight: '20px',
    }}>{STATUS_LABEL[status] || status}</span>
  );
};

const PriorityChip: React.FC<{ p: string }> = ({ p }) => {
  const hex = PRIORITY_HEX[p] || '#94a3b8';
  return (
    <span style={{
      display: 'inline-block', padding: '1px 8px',
      background: `${hex}12`, color: hex, border: `1px solid ${hex}30`,
      borderRadius: 3, fontWeight: 600, fontSize: 12, lineHeight: '20px',
    }}>{p}</span>
  );
};

const RequirementList: React.FC = () => {
  const [view, setView] = useState<'list' | 'kanban' | 'gantt'>('list');
  const [list, setList] = useState<RequirementItem[]>([]);
  const [total, setTotal] = useState(0);
  const [stats, setStats] = useState<StatsData>({ status: {}, priority: {} });
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize] = useState(50);
  const [statusFilter, setStatusFilter] = useState('');
  const [priorityFilter, setPriorityFilter] = useState('');
  const [searchText, setSearchText] = useState('');
  const [detail, setDetail] = useState<RequirementItem | null>(null);
  const [editStatus, setEditStatus] = useState('');
  const [editPriority, setEditPriority] = useState('');
  const [editTargetVersion, setEditTargetVersion] = useState('');
  const [editExpectedDate, setEditExpectedDate] = useState<string | null>(null);
  const [editTag, setEditTag] = useState('');
  const [editRemark, setEditRemark] = useState('');
  const [saving, setSaving] = useState(false);
  const [isAdmin, setIsAdmin] = useState(false);

  const fetchList = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) });
      if (statusFilter) params.set('status', statusFilter);
      if (priorityFilter) params.set('priority', priorityFilter);
      const res = await fetch(`${API_BASE}/api/requirements/list?${params}`, { credentials: 'include' });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const json = await res.json();
      const data = json.data || json;
      setList(data.list || []);
      setTotal(data.total || 0);
      setIsAdmin(!!data.isAdmin);
    } catch (err) {
      message.error('获取需求列表失败');
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, statusFilter, priorityFilter]);

  const fetchStats = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/api/requirements/stats`, { credentials: 'include' });
      if (res.ok) {
        const json = await res.json();
        setStats(json.data || json);
      }
    } catch { /* silent */ }
  }, []);

  useEffect(() => { fetchList(); }, [fetchList]);
  useEffect(() => { fetchStats(); }, [fetchStats, list]);

  const filteredList = useMemo(() => {
    const q = searchText.trim().toLowerCase();
    if (!q) return list;
    return list.filter(i =>
      i.title.toLowerCase().includes(q) ||
      i.content.toLowerCase().includes(q) ||
      i.submitterName.toLowerCase().includes(q) ||
      (i.tag || '').toLowerCase().includes(q)
    );
  }, [list, searchText]);

  const openDetail = (item: RequirementItem) => {
    setDetail(item);
    setEditStatus(item.status);
    setEditPriority(item.priority);
    setEditTargetVersion(item.targetVersion || '');
    setEditExpectedDate(item.expectedDate || null);
    setEditTag(item.tag || '');
    setEditRemark(item.adminRemark || '');
  };

  const handleSave = async () => {
    if (!detail) return;
    setSaving(true);
    try {
      const body: Record<string, any> = {};
      if (editStatus !== detail.status) body.status = editStatus;
      if (editPriority !== detail.priority) body.priority = editPriority;
      if (editTargetVersion !== (detail.targetVersion || '')) body.targetVersion = editTargetVersion;
      if (editExpectedDate !== (detail.expectedDate || null)) body.expectedDate = editExpectedDate || '';
      if (editTag !== (detail.tag || '')) body.tag = editTag;
      if (editRemark !== (detail.adminRemark || '')) body.adminRemark = editRemark;
      if (Object.keys(body).length === 0) {
        message.info('没有改动');
        setSaving(false);
        return;
      }
      const res = await fetch(`${API_BASE}/api/requirements/${detail.id}`, {
        method: 'PUT', credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!res.ok) {
        const j = await res.json().catch(() => ({}));
        throw new Error(j.msg || `HTTP ${res.status}`);
      }
      message.success('保存成功');
      setDetail(null);
      fetchList();
    } catch (err) {
      message.error('保存失败: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      setSaving(false);
    }
  };

  const columns: ColumnsType<RequirementItem> = [
    {
      title: '优先级', dataIndex: 'priority', width: 70,
      render: (p: string) => <PriorityChip p={p} />,
    },
    {
      title: '标题', dataIndex: 'title',
      render: (t: string, r) => (
        <div>
          <Button type="link" style={{ padding: 0 }} onClick={() => openDetail(r)}>{t}</Button>
          {r.tag && <Tag style={{ marginLeft: 6 }}>{r.tag}</Tag>}
        </div>
      ),
    },
    {
      title: '提交人', dataIndex: 'submitterName', width: 100,
      render: (n: string, r) => (
        <span>{n}{r.submitterDept ? <span style={{ color: '#94a3b8' }}> · {r.submitterDept}</span> : null}</span>
      ),
    },
    { title: '状态', dataIndex: 'status', width: 80, render: (s: string) => <StatusChip status={s} /> },
    {
      title: '排期版本', dataIndex: 'targetVersion', width: 110,
      render: (v: string) => v ? <Tag color="blue">{v}</Tag> : <span style={{ color: '#cbd5e1' }}>未排</span>,
    },
    {
      title: '预计上线', dataIndex: 'expectedDate', width: 110,
      render: (d: string | null) => d || <span style={{ color: '#cbd5e1' }}>-</span>,
    },
    { title: '提交时间', dataIndex: 'createdAt', width: 150 },
  ];

  // 4 张活跃状态 KPI 卡 + 1 张已完成
  const kpiCards = [
    { key: 'pending', label: '待评估', icon: <ClockCircleOutlined /> },
    { key: 'accepted', label: '已接受', icon: <CheckCircleOutlined /> },
    { key: 'scheduled', label: '排期中', icon: <FieldTimeOutlined /> },
    { key: 'in_progress', label: '开发中', icon: <RocketOutlined /> },
  ];

  return (
    <div>
      <Tabs
        activeKey={view}
        onChange={(k) => setView(k as any)}
        items={[
          { key: 'list', label: '列表' },
          { key: 'kanban', label: '看板' },
          { key: 'gantt', label: '甘特图' },
        ]}
        size="small"
        style={{ marginBottom: 12 }}
      />
      {view === 'kanban' && <RequirementKanban />}
      {view === 'gantt' && (
        <div style={{ padding: 40, textAlign: 'center', color: '#94a3b8' }}>
          甘特图视图即将上线（v1.62.0 收尾前完成）
        </div>
      )}
      {view === 'list' && (
      <>
      <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
        {kpiCards.map(card => {
          const hex = STATUS_HEX_MAP[card.key];
          const n = stats.status[card.key] || 0;
          return (
            <Col xs={12} sm={6} key={card.key}>
              <Card
                className="bi-stat-card"
                style={{
                  ['--accent-color' as any]: hex,
                  cursor: 'pointer',
                  outline: statusFilter === card.key ? `2px solid ${hex}` : 'none',
                }}
                bodyStyle={{ padding: 16 }}
                onClick={() => { setPage(1); setStatusFilter(statusFilter === card.key ? '' : card.key); }}
              >
                <Statistic
                  title={<><span style={{ color: hex, marginRight: 6 }}>{card.icon}</span>{card.label}</>}
                  value={n}
                  valueStyle={{ color: n > 0 ? hex : '#94a3b8', fontSize: 22, fontWeight: n > 0 ? 700 : 400 }}
                />
              </Card>
            </Col>
          );
        })}
      </Row>

      {/* 已完成/搁置/拒绝 次要 KPI 行 */}
      <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
        {['done', 'shelved', 'rejected'].map(key => {
          const hex = STATUS_HEX_MAP[key];
          const n = stats.status[key] || 0;
          return (
            <Col xs={8} key={key}>
              <Card
                className="bi-stat-card"
                style={{
                  ['--accent-color' as any]: hex,
                  cursor: 'pointer',
                  outline: statusFilter === key ? `2px solid ${hex}` : 'none',
                }}
                bodyStyle={{ padding: 12 }}
                onClick={() => { setPage(1); setStatusFilter(statusFilter === key ? '' : key); }}
              >
                <Statistic
                  title={<span style={{ color: hex, fontSize: 13 }}>{STATUS_LABEL[key]}</span>}
                  value={n}
                  valueStyle={{ color: hex, fontSize: 18 }}
                />
              </Card>
            </Col>
          );
        })}
      </Row>

      <Card
        className="bi-card"
        title="需求列表"
        extra={
          <div style={{ display: 'flex', gap: 8 }}>
            <Input
              placeholder="搜索 标题/内容/提交人/标签"
              prefix={<SearchOutlined style={{ color: '#94a3b8' }} />}
              allowClear
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              style={{ width: 240 }}
            />
            <Select
              placeholder="优先级"
              allowClear
              value={priorityFilter || undefined}
              onChange={v => { setPriorityFilter(v || ''); setPage(1); }}
              style={{ width: 100 }}
              options={[
                { label: 'P0', value: 'P0' }, { label: 'P1', value: 'P1' },
                { label: 'P2', value: 'P2' }, { label: 'P3', value: 'P3' },
              ]}
            />
          </div>
        }
      >
        {(statusFilter || priorityFilter) && (
          <div style={{ marginBottom: 12 }}>
            {statusFilter && (
              <Tag closable onClose={() => setStatusFilter('')} color="blue">
                状态: {STATUS_LABEL[statusFilter]}
              </Tag>
            )}
            {priorityFilter && (
              <Tag closable onClose={() => setPriorityFilter('')} color="orange">
                优先级: {priorityFilter}
              </Tag>
            )}
          </div>
        )}
        <Table
          rowKey="id"
          loading={loading}
          dataSource={filteredList}
          columns={columns}
          size="small"
          pagination={{
            current: page, pageSize, total,
            onChange: (p) => setPage(p),
            showTotal: (t) => `共 ${t} 条`,
          }}
        />
      </Card>

      <Modal
        title={detail ? `需求详情 · ${detail.title}` : ''}
        open={!!detail}
        onCancel={() => setDetail(null)}
        width={680}
        footer={isAdmin ? [
          <Button key="cancel" onClick={() => setDetail(null)}>关闭</Button>,
          <Button key="save" type="primary" loading={saving} onClick={handleSave}>保存</Button>,
        ] : [
          <Button key="close" onClick={() => setDetail(null)}>关闭</Button>,
        ]}
      >
        {detail && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
              <PriorityChip p={detail.priority} />
              <StatusChip status={detail.status} />
              {detail.targetVersion && <Tag color="blue">{detail.targetVersion}</Tag>}
              {detail.tag && <Tag>{detail.tag}</Tag>}
              <span style={{ color: '#94a3b8', fontSize: 12, marginLeft: 'auto' }}>
                {detail.submitterName} · {detail.submitterDept} · 提交于 {detail.createdAt}
              </span>
            </div>
            {detail.content && (
              <div style={{ background: '#f8fafc', padding: 12, borderRadius: 4, whiteSpace: 'pre-wrap' }}>
                {detail.content}
              </div>
            )}

            {isAdmin && (
              <>
                <Row gutter={12}>
                  <Col span={8}>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>状态</Typography.Text>
                    <Select
                      value={editStatus}
                      onChange={setEditStatus}
                      style={{ width: '100%', marginTop: 4 }}
                      options={Object.entries(STATUS_LABEL).map(([v, l]) => ({ label: l, value: v }))}
                    />
                  </Col>
                  <Col span={8}>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>优先级</Typography.Text>
                    <Select
                      value={editPriority}
                      onChange={setEditPriority}
                      style={{ width: '100%', marginTop: 4 }}
                      options={['P0', 'P1', 'P2', 'P3'].map(p => ({ label: p, value: p }))}
                    />
                  </Col>
                  <Col span={8}>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>标签</Typography.Text>
                    <Input
                      value={editTag}
                      onChange={e => setEditTag(e.target.value)}
                      placeholder="BI / 合思 / 钉钉 / RPA..."
                      style={{ marginTop: 4 }}
                    />
                  </Col>
                </Row>
                <Row gutter={12}>
                  <Col span={12}>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>排期版本号</Typography.Text>
                    <Input
                      value={editTargetVersion}
                      onChange={e => setEditTargetVersion(e.target.value)}
                      placeholder="v1.62.0"
                      style={{ marginTop: 4 }}
                    />
                  </Col>
                  <Col span={12}>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>预计上线日期</Typography.Text>
                    <DatePicker
                      value={editExpectedDate ? dayjs(editExpectedDate) : null}
                      onChange={d => setEditExpectedDate(d ? d.format('YYYY-MM-DD') : null)}
                      style={{ width: '100%', marginTop: 4 }}
                    />
                  </Col>
                </Row>
                <div>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>跑哥备注</Typography.Text>
                  <Input.TextArea
                    value={editRemark}
                    onChange={e => setEditRemark(e.target.value)}
                    rows={3}
                    style={{ marginTop: 4 }}
                  />
                </div>
              </>
            )}
          </div>
        )}
      </Modal>
      </>
      )}
    </div>
  );
};

export default RequirementList;
