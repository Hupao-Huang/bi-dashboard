// v1.59.0 个人中心 → 合思机器人 Tab
// MVP: 只读"我的待审批" 单据列表. 后续 v1.60.0 加规则编辑, v1.61.0 dry run, v1.62.0 真自动审批.
// v1.62.x: 字段/详情对齐费控管理 (单据模板/创建时间/发票/附件 + 点击查看明细 Modal 4 Tab)

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Alert, Badge, Button, Card, DatePicker, Empty, Input, Modal,
  Radio, Select, Statistic, Table, Tag, Tooltip, message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  CheckOutlined, ClockCircleOutlined, CloseOutlined,
  EyeOutlined, FileTextOutlined,
  ReloadOutlined, RobotOutlined, SearchOutlined, UserOutlined, WarningOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import { API_BASE } from '../../config';
import HesiBotRules from './HesiBotRules';
import HesiFlowDetailModal from '../../components/HesiFlowDetailModal';

interface PendingItem {
  flowId: string;
  code: string;
  title: string;
  formType: string;
  state: string;
  stageName: string | null;
  approverName: string | null;
  currentApproverCode: string | null;
  payMoney: number | null;
  expenseMoney: number | null;
  loanMoney: number | null;
  createTime: number | null;
  updateTime: number | null;
  submitDate: number | null;
  submitterId: string | null;
  departmentId: string | null;
  preApprovedNode: string | null;
  preApprovedTime: string | null;
  specificationId: string | null;
  specificationName: string;
  detailCount: number;
  invoiceExist: number;
  invoiceMissing: number;
  attachmentCount: number;
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
  draft: { label: '草稿', color: 'default' },
  pending: { label: '提交中', color: 'processing' },
  approving: { label: '审批中', color: 'processing' },
  rejected: { label: '已驳回', color: 'error' },
  paying: { label: '待支付', color: 'warning' },
  PROCESSING: { label: '支付中', color: 'warning' },
  paid: { label: '已支付', color: 'success' },
  archived: { label: '已归档', color: 'default' },
};

const PROFILE_API = `${API_BASE}/api/profile`;

const HesiBot: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [realName, setRealName] = useState('');
  const [queryName, setQueryName] = useState('');
  const [isAdmin, setIsAdmin] = useState(false);
  const [warning, setWarning] = useState('');
  const [items, setItems] = useState<PendingItem[]>([]);
  const [approverOptions, setApproverOptions] = useState<{ name: string; count: number }[]>([]);
  const [selectedApprover, setSelectedApprover] = useState<string | undefined>(undefined);
  // 手动审批
  const [approveTarget, setApproveTarget] = useState<PendingItem | null>(null);
  const [approveAction, setApproveAction] = useState<'agree' | 'reject'>('agree');
  const [approveComment, setApproveComment] = useState('');
  const [approveLoading, setApproveLoading] = useState(false);
  // 搜索筛选
  const [searchText, setSearchText] = useState('');
  const [formTypeFilter, setFormTypeFilter] = useState<string[]>([]);
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null] | null>(null);
  // 异步审批队列
  type QueueItem = { id: number; flowId: string; flowCode: string; action: string; status: string; errorMsg?: string; createdAt: string; finishedAt?: string };
  const [activeQueue, setActiveQueue] = useState<QueueItem[]>([]);
  // P1 乐观更新: 跑哥点完"通过/驳回"立即标该单为"已提交, 等生效", 不必等 worker 65s + 轮询 30s 后才看到反馈
  const [optimisticApproved, setOptimisticApproved] = useState<Set<string>>(new Set());
  // 详情 Modal (内容渲染交给共享组件 HesiFlowDetailModal)
  const [detailModal, setDetailModal] = useState<{ visible: boolean; flowId: string }>({ visible: false, flowId: '' });

  const selectedApproverRef = useRef(selectedApprover);
  useEffect(() => { selectedApproverRef.current = selectedApprover; }, [selectedApprover]);

  const fetchPending = useCallback(async (approver?: string) => {
    setLoading(true);
    try {
      const url = approver
        ? `${PROFILE_API}/hesi-pending?approver=${encodeURIComponent(approver)}`
        : `${PROFILE_API}/hesi-pending`;
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
      const res = await fetch(`${PROFILE_API}/hesi-approvers`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200 && json.data) {
        setApproverOptions(json.data.items || []);
      }
    } catch { /* silent */ }
  }, []);

  useEffect(() => { fetchPending(); }, [fetchPending]);
  useEffect(() => { if (isAdmin) fetchApproverOptions(); }, [isAdmin, fetchApproverOptions]);

  const fetchQueue = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/api/hesi-bot/approve/queue`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200 && json.data) {
        const next: QueueItem[] = json.data.active || [];
        setActiveQueue(next);
        // P1 配套清理: activeQueue 已不含该 flowId 说明 worker 跑完了, 清掉 optimistic 残留
        // 让 fetchPending 拿回的真实数据接管 (审批成功该单会从列表消失, 失败则恢复"审批"按钮)
        setOptimisticApproved(prev => {
          if (prev.size === 0) return prev;
          const stillActive = new Set(next.map(q => q.flowId));
          const out = new Set<string>();
          prev.forEach(fid => { if (stillActive.has(fid)) out.add(fid); });
          return out.size === prev.size ? prev : out;
        });
      }
    } catch { /* silent */ }
  }, []);

  // P0 加速轮询: 队列里有未完成单时 5s 一次 (跑哥审批后最快 ~70s 看到刷掉), 空闲时回到 30s
  const activeQueueHasFlows = activeQueue.length > 0 || optimisticApproved.size > 0;
  useEffect(() => {
    fetchQueue();
    const interval = activeQueueHasFlows ? 5000 : 30000;
    const t = setInterval(() => {
      fetchQueue();
      fetchPending(selectedApproverRef.current);
    }, interval);
    return () => clearInterval(t);
  }, [fetchQueue, fetchPending, activeQueueHasFlows]);

  const getMoney = (item: PendingItem) => {
    if (item.payMoney && item.payMoney > 0) return item.payMoney;
    if (item.expenseMoney && item.expenseMoney > 0) return item.expenseMoney;
    if (item.loanMoney && item.loanMoney > 0) return item.loanMoney;
    return null;
  };

  const formatTime = (ts: number | null | undefined) => {
    if (!ts) return '-';
    return dayjs(Number(ts)).format('YYYY-MM-DD HH:mm');
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
  const hasFilter = !!searchText || formTypeFilter.length > 0 || (!!dateRange && !!(dateRange[0] || dateRange[1]));
  // AI 建议规则当前仅适用于樊雪娇日常报销单 → 别人看到 items 无 suggestion → 列自动隐藏
  const showAuditCol = items.some(it => !!it.suggestion);

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
      // P1 乐观更新: 立即标该单"已提交", 不必等 worker + 轮询
      const submittedFlowId = approveTarget.flowId;
      setOptimisticApproved(prev => {
        const next = new Set(prev);
        next.add(submittedFlowId);
        return next;
      });
      setApproveTarget(null);
      fetchQueue();
      fetchPending(selectedApprover);
    } catch (e) {
      message.error('审批失败: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      setApproveLoading(false);
    }
  };

  // 详情数据/附件加载都在 HesiFlowDetailModal 组件内部完成, 这里只负责打开弹窗
  const showDetail = (flowId: string) => setDetailModal({ visible: true, flowId });

  const columns: ColumnsType<PendingItem> = [
    {
      title: '单据编码', dataIndex: 'code', width: 130, fixed: 'left',
      render: (code, record) => (
        <Button type="link" onClick={() => showDetail(record.flowId)} style={{ padding: 0, height: 'auto' }}>
          {code}
        </Button>
      ),
    },
    {
      title: '标题', dataIndex: 'title', width: 260, ellipsis: true,
      render: (title, record) => (
        <Tooltip title={title}>
          <Button type="link" onClick={() => showDetail(record.flowId)} style={{ padding: 0, height: 'auto' }}>
            {title}
          </Button>
        </Tooltip>
      ),
    },
    {
      title: '类型', dataIndex: 'formType', width: 90,
      render: (v) => {
        const m = formTypeMap[v];
        return m ? <Tag color={m.color}>{m.label}</Tag> : v;
      },
    },
    {
      title: '单据模板', dataIndex: 'specificationName', width: 140,
      render: (v: string, record) => {
        if (!v) return <span style={{ color: '#cbd5e1' }}>-</span>;
        return (
          <Tooltip title={record.specificationId || ''}>
            <Tag color="blue" style={{ cursor: 'help' }}>{v}</Tag>
          </Tooltip>
        );
      },
    },
    {
      title: '状态', dataIndex: 'state', width: 90,
      render: (v) => {
        const m = stateMap[v];
        return m ? <Tag color={m.color}>{m.label}</Tag> : v;
      },
    },
    {
      title: '当前节点', dataIndex: 'stageName', width: 130,
      render: (v: string | null, record) => {
        if (v && record.approverName) {
          return (
            <Tooltip title={`审批人: ${record.approverName}${record.currentApproverCode ? ' (' + record.currentApproverCode + ')' : ''}`}>
              <div><strong>{v}</strong></div>
              <div style={{ fontSize: 11, color: '#666' }}>{record.approverName}</div>
            </Tooltip>
          );
        }
        return v || <span style={{ color: '#999' }}>-</span>;
      },
    },
    {
      title: '金额', width: 120, align: 'right',
      render: (_, record) => {
        const money = getMoney(record);
        return money ? `¥${money.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}` : '-';
      },
    },
    {
      title: '发票', width: 100, align: 'center',
      render: (_, record) => {
        if (record.detailCount === 0) return '-';
        if (record.invoiceMissing > 0 && record.invoiceExist > 0) {
          return <Tag color="warning">部分({record.invoiceExist}/{record.detailCount})</Tag>;
        }
        if (record.invoiceMissing > 0) return <Tag color="error">未到</Tag>;
        return <Tag color="success">齐全</Tag>;
      },
    },
    {
      title: '附件', dataIndex: 'attachmentCount', width: 70, align: 'center',
      render: (v: number) => v > 0 ? <Badge count={v} style={{ backgroundColor: '#06b6d4' }} /> : '-',
    },
    {
      title: '创建时间', dataIndex: 'createTime', width: 150,
      render: (v: number | null) => formatTime(v),
    },
    {
      title: '提交日期', dataIndex: 'submitDate', width: 110,
      render: (v: number | null) => v ? dayjs(Number(v)).format('YYYY-MM-DD') : '-',
    },
    ...(showAuditCol ? [{
      title: 'AI 建议', width: 130, align: 'center' as const,
      render: (_: any, record: PendingItem) => {
        const s = record.suggestion;
        if (!s) return <span style={{ color: '#cbd5e1' }}>-</span>;
        const cfg = ({
          agree: { color: 'success', label: '建议同意', icon: <CheckOutlined /> },
          reject: { color: 'error', label: '建议驳回', icon: <CloseOutlined /> },
          manual: { color: 'warning', label: '转人工', icon: <ClockCircleOutlined /> },
        } as Record<string, { color: string; label: string; icon: React.ReactNode }>)[s.action]
          || { color: 'default', label: s.action, icon: null };
        return (
          <Tooltip title={<div>{s.reasons.map((r, i) => <div key={i}>· {r}</div>)}</div>}>
            <Tag color={cfg.color} icon={cfg.icon as any}>{cfg.label}</Tag>
          </Tooltip>
        );
      },
    }] : []),
    {
      title: '操作', width: 160, fixed: 'right', align: 'center',
      render: (_, record) => {
        const inQueue = activeQueue.find(q => q.flowId === record.flowId);
        const isOptimistic = optimisticApproved.has(record.flowId);
        // 状态优先级: 乐观提交瞬间 → 队列处理中 → 可审批
        let statusTag: React.ReactNode = null;
        if (inQueue) {
          statusTag = (
            <Tag color={inQueue.status === 'running' ? 'processing' : 'default'}>
              {inQueue.status === 'running' ? '处理中' : '排队中'}
            </Tag>
          );
        } else if (isOptimistic) {
          statusTag = <Tag color="processing">已提交,等生效</Tag>;
        }
        return (
          <div style={{ display: 'flex', gap: 4, justifyContent: 'center' }}>
            <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => showDetail(record.flowId)}>
              详情
            </Button>
            {statusTag ? statusTag : (
              <Button size="small" type="primary" onClick={() => openApproveModal(record)}>
                审批
              </Button>
            )}
          </div>
        );
      },
    },
  ];

  return (
    <div>
      {/* P1 乐观更新行样式: 已提交/队列中的单据整行置灰 */}
      <style>{`
        .hesi-row-pending-approve td { background-color: #f1f5f9 !important; color: #94a3b8 !important; }
        .hesi-row-pending-approve:hover td { background-color: #e2e8f0 !important; }
      `}</style>

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

      {/* 管理员视角切换 */}
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
            <Tooltip title="只从 BI 看板本地数据库重读一次清单, 不会去合思现拉. 想看合思最新状态请去 费控管理 → 立即同步合思.">
              <Button icon={<ReloadOutlined />} onClick={() => fetchPending(selectedApprover)} loading={loading} style={{ marginLeft: 'auto' }}>
                刷新
              </Button>
            </Tooltip>
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
            scroll={{ x: 1500 }}
            locale={{ emptyText: hasFilter ? '当前筛选下无匹配单据' : '暂无数据' }}
            rowClassName={(record) => {
              // P1 乐观更新: 已提交/队列中的单行整体置灰, 跑哥一眼看出"这单不必再点"
              const inQueue = activeQueue.find(q => q.flowId === record.flowId);
              return (inQueue || optimisticApproved.has(record.flowId)) ? 'hesi-row-pending-approve' : '';
            }}
          />
        )}
      </Card>

      {/* 审批 Modal */}
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

      {/* 单据详情弹窗 — 共享组件 HesiFlowDetailModal, 跟费控管理(finance/ExpenseControl)共用一套 */}
      {/* 改一处两边同步 (跑哥要求"这两个一直保持一致") */}
      <HesiFlowDetailModal
        open={detailModal.visible}
        flowId={detailModal.flowId}
        onClose={() => setDetailModal({ visible: false, flowId: '' })}
        flowDetailUrl={(id) => `${PROFILE_API}/hesi-flow-detail?flowId=${id}`}
        attachmentUrlsUrl={(id) => `${PROFILE_API}/hesi-attachment-urls?flowId=${id}`}
      />
    </div>
  );
};

export default HesiBot;
