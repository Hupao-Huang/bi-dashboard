// v1.59.0 个人中心 → 合思机器人 Tab
// MVP: 只读"我的待审批" 单据列表. 后续 v1.60.0 加规则编辑, v1.61.0 dry run, v1.62.0 真自动审批.

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Button, Card, DatePicker, Empty, Input, Modal, Radio, Select, Statistic, Table, Tag, Tooltip, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { CheckOutlined, ClockCircleOutlined, CloseOutlined, ReloadOutlined, RobotOutlined, SearchOutlined, UserOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { API_BASE } from '../../config';
import HesiBotRules from './HesiBotRules';

interface PendingItem {
  flowId: string;
  code: string;
  title: string;
  formType: string;
  state: string;
  stageName: string | null;
  approverName: string | null;
  payMoney: number | null;
  expenseMoney: number | null;
  loanMoney: number | null;
  submitDate: number | null;
  submitterId: string | null;
  departmentId: string | null;
  suggestion?: { action: 'agree' | 'reject' | 'manual'; reasons: string[] };
}

const formTypeMap: Record<string, { label: string; color: string }> = {
  expense: { label: '报销单', color: 'blue' },
  loan: { label: '借款单', color: 'orange' },
  requisition: { label: '申请单', color: 'green' },
  custom: { label: '通用审批', color: 'purple' },
  payment: { label: '付款单', color: 'red' },
  receipt: { label: '收款单', color: 'cyan' },
};

const stateMap: Record<string, { label: string; color: string }> = {
  approving: { label: '审批中', color: 'processing' },
  paying: { label: '待支付', color: 'warning' },
  pending: { label: '提交中', color: 'processing' },
};

const HesiBot: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [realName, setRealName] = useState('');
  const [queryName, setQueryName] = useState('');
  const [isAdmin, setIsAdmin] = useState(false);
  const [warning, setWarning] = useState('');
  const [items, setItems] = useState<PendingItem[]>([]);
  // v1.59.3: 管理员可切换查看别人的待审批
  const [approverOptions, setApproverOptions] = useState<{name:string, count:number}[]>([]);
  const [selectedApprover, setSelectedApprover] = useState<string | undefined>(undefined);
  // v1.63: 手动审批
  const [approveTarget, setApproveTarget] = useState<PendingItem | null>(null);
  const [approveAction, setApproveAction] = useState<'agree' | 'reject'>('agree');
  const [approveComment, setApproveComment] = useState('');
  const [approveLoading, setApproveLoading] = useState(false);
  // v1.63: 搜索筛选
  const [searchText, setSearchText] = useState('');
  const [formTypeFilter, setFormTypeFilter] = useState<string[]>([]);
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null] | null>(null);
  // v1.62.x: 异步审批队列
  type QueueItem = { id: number; flowId: string; flowCode: string; action: string; status: string; errorMsg?: string; createdAt: string; finishedAt?: string };
  const [activeQueue, setActiveQueue] = useState<QueueItem[]>([]);
  const [recentQueue, setRecentQueue] = useState<QueueItem[]>([]);
  const [queueTotal, setQueueTotal] = useState({ queued: 0, running: 0 });

  const fetchPending = useCallback(async (approver?: string) => {
    setLoading(true);
    try {
      const url = approver
        ? `${API_BASE}/api/profile/hesi-pending?approver=${encodeURIComponent(approver)}`
        : `${API_BASE}/api/profile/hesi-pending`;
      const res = await fetch(url, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200 && json.data) {
        setRealName(json.data.realName || '');
        setQueryName(json.data.queryName || '');
        setIsAdmin(!!json.data.isAdmin);
        setItems(json.data.items || []);
        setWarning(json.data.warning || '');
      } else {
        message.error(json.msg || '加载失败');
      }
    } catch (e) {
      message.error('网络错误');
    } finally {
      setLoading(false);
    }
  }, []);

  const fetchApproverOptions = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/api/profile/hesi-approvers`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200 && json.data) {
        setApproverOptions(json.data.items || []);
      }
    } catch {}
  }, []);

  useEffect(() => { fetchPending(); }, [fetchPending]);
  useEffect(() => { if (isAdmin) fetchApproverOptions(); }, [isAdmin, fetchApproverOptions]);

  const fetchQueue = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/api/hesi-bot/approve/queue`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200 && json.data) {
        setActiveQueue(json.data.active || []);
        setRecentQueue(json.data.recent || []);
        setQueueTotal({ queued: json.data.totalQueued || 0, running: json.data.totalRunning || 0 });
      }
    } catch { /* silent */ }
  }, []);

  // 30s polling 队列 + 待审批列表
  useEffect(() => {
    fetchQueue();
    const t = setInterval(() => {
      fetchQueue();
      fetchPending(selectedApprover);
    }, 30000);
    return () => clearInterval(t);
  }, [fetchQueue, fetchPending, selectedApprover]);

  const getMoney = (item: PendingItem) => {
    if (item.payMoney && item.payMoney > 0) return item.payMoney;
    if (item.expenseMoney && item.expenseMoney > 0) return item.expenseMoney;
    if (item.loanMoney && item.loanMoney > 0) return item.loanMoney;
    return null;
  };

  const filteredItems = useMemo(() => {
    const q = searchText.trim().toLowerCase();
    return items.filter(it => {
      if (q) {
        const matchCode = (it.code || '').toLowerCase().includes(q);
        const matchTitle = (it.title || '').toLowerCase().includes(q);
        if (!matchCode && !matchTitle) return false;
      }
      if (formTypeFilter.length > 0 && !formTypeFilter.includes(it.formType)) return false;
      if (dateRange && (dateRange[0] || dateRange[1]) && it.submitDate) {
        const ts = Number(it.submitDate);
        if (dateRange[0] && ts < dateRange[0].startOf('day').valueOf()) return false;
        if (dateRange[1] && ts > dateRange[1].endOf('day').valueOf()) return false;
      }
      return true;
    });
  }, [items, searchText, formTypeFilter, dateRange]);

  const totalAmount = filteredItems.reduce((sum, item) => sum + (getMoney(item) || 0), 0);
  const hasFilter = searchText || formTypeFilter.length > 0 || (dateRange && (dateRange[0] || dateRange[1]));

  const openApproveModal = (item: PendingItem) => {
    setApproveTarget(item);
    setApproveAction('agree');
    setApproveComment('');
  };

  const handleApproveSubmit = async () => {
    if (!approveTarget) return;
    if (approveAction === 'reject' && !approveComment.trim()) {
      message.warning('驳回必须填写备注');
      return;
    }
    setApproveLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/hesi-bot/approve`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          flowId: approveTarget.flowId,
          action: approveAction,
          comment: approveComment.trim(),
        }),
      });
      const json = await res.json();
      if (!res.ok || json.code !== 200) {
        throw new Error(json.msg || `HTTP ${res.status}`);
      }
      const d = json.data || {};
      const wait = d.estimateSeconds || 0;
      const pos = d.position || 1;
      const waitText = wait < 60 ? `约 ${wait} 秒` : `约 ${Math.round(wait / 60)} 分钟`;
      message.success(`已加入审批队列, 排第 ${pos} 位, 预计 ${waitText}后处理 (合思 60s 限流)`);
      setApproveTarget(null);
      fetchQueue();
      fetchPending(selectedApprover);
    } catch (e) {
      message.error('审批失败: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      setApproveLoading(false);
    }
  };

  const columns: ColumnsType<PendingItem> = [
    { title: '单据编码', dataIndex: 'code', width: 130, fixed: 'left' },
    {
      title: '标题', dataIndex: 'title', ellipsis: true,
      render: (title) => <Tooltip title={title}>{title}</Tooltip>,
    },
    {
      title: '类型', dataIndex: 'formType', width: 90,
      render: (v) => {
        const m = formTypeMap[v];
        return m ? <Tag color={m.color}>{m.label}</Tag> : v;
      },
    },
    {
      title: '状态', dataIndex: 'state', width: 90,
      render: (v) => {
        const m = stateMap[v];
        return m ? <Tag color={m.color}>{m.label}</Tag> : v;
      },
    },
    { title: '当前节点', dataIndex: 'stageName', width: 130 },
    {
      title: '金额', width: 120, align: 'right',
      render: (_, record) => {
        const money = getMoney(record);
        return money ? `¥${money.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}` : '-';
      },
    },
    {
      title: '提交日期', dataIndex: 'submitDate', width: 110,
      render: (v) => v ? dayjs(Number(v)).format('YYYY-MM-DD') : '-',
    },
    {
      title: 'AI 建议', width: 130, align: 'center',
      render: (_, record) => {
        const s = record.suggestion;
        if (!s) return <span style={{ color: '#cbd5e1' }}>-</span>;
        const cfg = {
          agree: { color: 'success', label: '建议同意', icon: <CheckOutlined /> },
          reject: { color: 'error', label: '建议驳回', icon: <CloseOutlined /> },
          manual: { color: 'warning', label: '转人工', icon: <ClockCircleOutlined /> },
        }[s.action] || { color: 'default', label: s.action, icon: null };
        return (
          <Tooltip title={<div>{s.reasons.map((r, i) => <div key={i}>· {r}</div>)}</div>}>
            <Tag color={cfg.color} icon={cfg.icon}>{cfg.label}</Tag>
          </Tooltip>
        );
      },
    },
    {
      title: '操作', width: 110, fixed: 'right', align: 'center',
      render: (_, record) => {
        const inQueue = activeQueue.find(q => q.flowId === record.flowId);
        if (inQueue) {
          const color = inQueue.status === 'running' ? 'processing' : 'default';
          const label = inQueue.status === 'running' ? '处理中' : '排队中';
          return <Tag color={color}>{label}</Tag>;
        }
        return (
          <Button size="small" type="primary" onClick={() => openApproveModal(record)}>
            审批
          </Button>
        );
      },
    },
  ];

  return (
    <div>
      {/* 顶部说明 + 后续路线 */}
      <HesiBotRules />

      <Alert
        type="info"
        showIcon
        icon={<RobotOutlined />}
        message="合思机器人 — 我的待审批"
        description={
          <div>
            <div>
              {isAdmin && queryName !== realName
                ? <span>正在查看 <strong>{queryName}</strong> 的待审批单据 (管理员视角)</span>
                : <span>当前显示<strong>等你审批</strong>的合思单据 (按你的真实姓名"{realName || '未设置'}"模糊匹配)</span>
              }
            </div>
            <div style={{ marginTop: 6, color: '#666', fontSize: 12 }}>
              路线图: v1.60 加规则编辑器 → v1.61 干跑模式(匹配但不审批, 看效果) → v1.62 真自动审批(需财务/合规批准).
            </div>
          </div>
        }
        style={{ marginBottom: 16 }}
      />

      {/* v1.59.3: 管理员视角切换 — 普通用户看不到这块 */}
      {isAdmin && (
        <Card size="small" style={{ marginBottom: 16, background: '#fafafa' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <UserOutlined style={{ color: '#1677ff' }} />
            <span style={{ color: '#666' }}>管理员视角 · 查看谁的待审批:</span>
            <Select
              showSearch
              allowClear
              placeholder={`默认看自己(${realName})`}
              style={{ minWidth: 240 }}
              value={selectedApprover}
              onChange={(v) => { setSelectedApprover(v); fetchPending(v); }}
              options={approverOptions.map(o => ({
                value: o.name,
                label: `${o.name} (${o.count} 单)`,
              }))}
              filterOption={(input, option) =>
                (option?.value as string)?.toLowerCase().includes(input.toLowerCase())
              }
            />
            {selectedApprover && (
              <Button size="small" onClick={() => { setSelectedApprover(undefined); fetchPending(); }}>
                看自己
              </Button>
            )}
          </div>
        </Card>
      )}

      {warning && (
        <Alert type="warning" showIcon message={warning} style={{ marginBottom: 16 }} />
      )}

      {/* 搜索筛选 */}
      {items.length > 0 && (
        <Card size="small" style={{ marginBottom: 12 }}>
          <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center' }}>
            <Input
              prefix={<SearchOutlined style={{ color: '#94a3b8' }} />}
              placeholder="搜索 单据编码 / 标题"
              value={searchText}
              onChange={e => setSearchText(e.target.value)}
              allowClear
              style={{ width: 260 }}
            />
            <Select
              mode="multiple"
              placeholder="单据类型"
              value={formTypeFilter}
              onChange={setFormTypeFilter}
              style={{ minWidth: 200 }}
              allowClear
              maxTagCount="responsive"
              options={Object.entries(formTypeMap).map(([v, m]) => ({ value: v, label: m.label }))}
            />
            <DatePicker.RangePicker
              value={dateRange as any}
              onChange={(v) => setDateRange(v as any)}
              placeholder={['提交起', '提交止']}
              style={{ width: 280 }}
            />
            {hasFilter && (
              <Button size="small" onClick={() => { setSearchText(''); setFormTypeFilter([]); setDateRange(null); }}>
                清空筛选
              </Button>
            )}
          </div>
        </Card>
      )}

      {/* 统计 */}
      {items.length > 0 && (
        <Card size="small" style={{ marginBottom: 16 }}>
          <div style={{ display: 'flex', gap: 32, alignItems: 'center' }}>
            <Statistic
              title={
                hasFilter
                  ? `筛选后 (共 ${items.length} 单)`
                  : (selectedApprover ? `${queryName} 待审批` : '等我审批')
              }
              value={filteredItems.length}
              suffix="单"
            />
            <Statistic title="涉及金额合计" value={totalAmount} precision={2} prefix="¥" />
            <Button icon={<ReloadOutlined />} onClick={() => fetchPending(selectedApprover)} loading={loading} style={{ marginLeft: 'auto' }}>
              刷新
            </Button>
          </div>
        </Card>
      )}

      {/* 表格 */}
      <Card>
        {items.length === 0 && !loading ? (
          <Empty
            description={
              queryName
                ? `没有等 ${queryName} 审批的合思单据 ✓`
                : '请先到"个人信息"页设置真实姓名'
            }
          />
        ) : (
          <Table<PendingItem>
            columns={columns}
            dataSource={filteredItems}
            rowKey="flowId"
            loading={loading}
            pagination={{ pageSize: 20, showSizeChanger: false }}
            size="middle"
            scroll={{ x: 800 }}
            locale={{ emptyText: hasFilter ? '当前筛选下无匹配单据' : '暂无数据' }}
          />
        )}
      </Card>

      <Modal
        title={approveTarget ? `审批单据 · ${approveTarget.code}` : ''}
        open={!!approveTarget}
        onCancel={() => setApproveTarget(null)}
        onOk={handleApproveSubmit}
        okText="确定"
        cancelText="取消"
        confirmLoading={approveLoading}
        width={520}
        destroyOnHidden
      >
        {approveTarget && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <div style={{ background: '#f8fafc', padding: 10, borderRadius: 4, fontSize: 13 }}>
              <div style={{ marginBottom: 4 }}><strong>{approveTarget.title}</strong></div>
              <div style={{ color: '#64748b', fontSize: 12 }}>
                {formTypeMap[approveTarget.formType]?.label || approveTarget.formType}
                {(() => {
                  const m = getMoney(approveTarget);
                  return m ? ` · ¥${m.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}` : '';
                })()}
                {approveTarget.stageName ? ` · 当前节点: ${approveTarget.stageName}` : ''}
              </div>
            </div>
            <div>
              <div style={{ marginBottom: 6, fontSize: 13, color: '#64748b' }}>审批结果</div>
              <Radio.Group
                value={approveAction}
                onChange={(e) => setApproveAction(e.target.value)}
                optionType="button"
                buttonStyle="solid"
              >
                <Radio.Button value="agree"><CheckOutlined /> 同意</Radio.Button>
                <Radio.Button value="reject"><CloseOutlined /> 驳回</Radio.Button>
              </Radio.Group>
            </div>
            <div>
              <div style={{ marginBottom: 6, fontSize: 13, color: '#64748b' }}>
                备注{approveAction === 'reject' ? <span style={{ color: '#ef4444' }}> (驳回必填)</span> : ' (可选)'}
              </div>
              <Input.TextArea
                value={approveComment}
                onChange={(e) => setApproveComment(e.target.value)}
                rows={3}
                maxLength={500}
                showCount
                placeholder={approveAction === 'reject' ? '请写明驳回理由(将提交到合思)' : '可选,会一同提交到合思'}
              />
            </div>
            <Alert
              type="warning"
              showIcon
              message="审批操作会直接提交到合思系统,无法撤销,请确认"
              style={{ fontSize: 12 }}
            />
          </div>
        )}
      </Modal>
    </div>
  );
};

export default HesiBot;
