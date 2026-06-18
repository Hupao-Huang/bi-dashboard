// v1.59.0 个人中心 → 合思机器人 Tab
// MVP: 只读"我的待审批" 单据列表. 后续 v1.60.0 加规则编辑, v1.61.0 dry run, v1.62.0 真自动审批.
// v1.62.x: 字段/详情对齐费控管理 (单据模板/创建时间/发票/附件 + 点击查看明细 Modal 4 Tab)

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Alert, Badge, Button, Card, DatePicker, Empty, Input, Modal,
  Radio, Select, Space, Statistic, Table, Tag, Tooltip, message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  CheckOutlined, ClockCircleOutlined, CloseOutlined,
  EyeOutlined,
  ReloadOutlined, SearchOutlined, UserOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import { API_BASE } from '../../config';
import HesiBotRules from './HesiBotRules';
import HesiBotRulesZhangJun from './HesiBotRulesZhangJun';
import HesiFlowDetailModal from '../../components/HesiFlowDetailModal';

// AI 审批建议规则卡的展示名单 (与后端 profile_hesi_pending.go 的审批人名单保持一致; 跑哥 2026-06-18 扩)
const DAILY_EXPENSE_APPROVERS = ['樊雪娇', '金海侠', '周翻翻', '张勇']; // 日常报销单规则卡
const PAYMENT_APPROVERS = ['张俊', '苏安妮']; // 对外付款单规则卡

interface PendingItem {
  flowId: string;
  code: string;
  ownerName?: string; // 发起人姓名
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
  const [suggestionFilter, setSuggestionFilter] = useState<string[]>([]); // AI 建议筛选 (agree/reject/manual/none)
  // 批量审批: 多选行 → 一次提交 (跑哥 6/11 需求, 跟 AI 建议筛选连用: 筛"建议同意"→全选→批量同意)
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);
  const [batchOpen, setBatchOpen] = useState(false);
  const [batchAction, setBatchAction] = useState<'agree' | 'reject'>('agree');
  const [batchComment, setBatchComment] = useState('');
  const [batchLoading, setBatchLoading] = useState(false);
  // 驳回选项 (对应合思驳回弹窗两模块): 驳回节点(单据级, 仅单笔驳回可选; 空=提交人) + 重审路径
  const [rejectTo, setRejectTo] = useState('');
  const [resubmitMethod, setResubmitMethod] = useState<'TO_REJECTOR' | 'FROM_START'>('TO_REJECTOR');
  const [batchResubmitMethod, setBatchResubmitMethod] = useState<'TO_REJECTOR' | 'FROM_START'>('TO_REJECTOR');
  const [rejectNodes, setRejectNodes] = useState<{ id: string; name: string }[]>([]);
  const [rejectNodesLoading, setRejectNodesLoading] = useState(false);
  const [liveSyncing, setLiveSyncing] = useState(false); // 刷新=现场同步合思中
  // 异步审批队列
  type QueueItem = { id: number; flowId: string; flowCode: string; action: string; status: string; errorMsg?: string; createdAt: string; finishedAt?: string };
  const [activeQueue, setActiveQueue] = useState<QueueItem[]>([]);
  // P1 乐观更新: 跑哥点完"通过/驳回"立即标该单为"已提交, 等生效" (flowId → 提交时间戳)
  // 解除时机: 该单从后端列表消失(合思流转确认完成) 或 超 3 分钟兜底(防失败单永久卡"已提交")
  const [optimisticApproved, setOptimisticApproved] = useState<Map<string, number>>(new Map());
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
        const fetched: PendingItem[] = json.data.items || [];
        setItems(fetched);
        setWarning(json.data.warning || '');
        // 单子已从后端列表消失 = 合思流转确认完成 → 解除乐观标记 (顺带停掉加速轮询)
        setOptimisticApproved(prev => {
          if (prev.size === 0) return prev;
          const present = new Set(fetched.map(i => i.flowId));
          const out = new Map<string, number>();
          prev.forEach((ts, fid) => { if (present.has(fid)) out.set(fid, ts); });
          return out.size === prev.size ? prev : out;
        });
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
  // 切换查看的审批人时清空勾选 (跨人勾选没有意义且容易误提交)
  useEffect(() => { setSelectedRowKeys([]); }, [selectedApprover]);

  const fetchQueue = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/api/hesi-bot/approve/queue`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200 && json.data) {
        const next: QueueItem[] = json.data.active || [];
        setActiveQueue(next);
        // 乐观标记兜底清理: 队列跑完后合思还要异步流转几秒~2分钟(后端确认后单子才从列表消失),
        // 这期间保留"已提交"; 超 3 分钟还没消失(大概率失败)恢复"审批"按钮可重试
        setOptimisticApproved(prev => {
          if (prev.size === 0) return prev;
          const stillActive = new Set(next.map(q => q.flowId));
          const now = Date.now();
          const out = new Map<string, number>();
          prev.forEach((ts, fid) => {
            if (stillActive.has(fid) || now - ts < 180000) out.set(fid, ts);
          });
          return out.size === prev.size ? prev : out;
        });
      }
    } catch { /* silent */ }
  }, []);

  // 跑哥 6/11: 去掉平时的 30s 自动刷新 (干扰浏览), 改手动"刷新"按钮
  // 仅审批提交后队列未清空时短暂轮询 5s 一次 (等 worker 审完把单子从列表撤掉), 队列空即停
  const activeQueueHasFlows = activeQueue.length > 0 || optimisticApproved.size > 0;
  useEffect(() => {
    fetchQueue();
    if (!activeQueueHasFlows) return;
    const t = setInterval(() => {
      fetchQueue();
      fetchPending(selectedApproverRef.current);
    }, 5000);
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
        const matchOwner = (it.ownerName || '').toLowerCase().includes(q);
        if (!matchCode && !matchTitle && !matchOwner) return false;
      }
      if (formTypeFilter.length > 0 && !formTypeFilter.includes(it.formType)) return false;
      if (suggestionFilter.length > 0 && !suggestionFilter.includes(it.suggestion?.action || 'none')) return false;
      if (dateRange && (dateRange[0] || dateRange[1]) && it.submitDate) {
        const ts = Number(it.submitDate);
        if (dateRange[0] && ts < dateRange[0].startOf('day').valueOf()) return false;
        if (dateRange[1] && ts > dateRange[1].endOf('day').valueOf()) return false;
      }
      return true;
    });
  }, [items, searchText, formTypeFilter, dateRange, suggestionFilter]);

  const totalAmount = filteredItems.reduce((sum, item) => sum + (getMoney(item) || 0), 0);
  const hasFilter = !!searchText || formTypeFilter.length > 0 || suggestionFilter.length > 0 || (!!dateRange && !!(dateRange[0] || dateRange[1]));
  // AI 建议列数据驱动: 任一单有 suggestion 才显示 (日常报销/付款单审批人才有建议, 其他人无 → 列自动隐藏)
  const showAuditCol = items.some(it => !!it.suggestion);

  // 批量审批: 已勾选的单据 + 金额合计
  const selectedSet = useMemo(() => new Set(selectedRowKeys.map(String)), [selectedRowKeys]);
  const selectedItems = useMemo(() => items.filter(it => selectedSet.has(it.flowId)), [items, selectedSet]);
  const selectedAmount = selectedItems.reduce((sum, it) => sum + (getMoney(it) || 0), 0);

  // 刷新 = 先去合思现场拉当前审批人的实时待办对齐本地, 再重读列表
  // 同步失败(如该审批人匹配不到合思员工)降级为只读本地, 给个提示不挡路
  const handleLiveRefresh = async () => {
    setLiveSyncing(true);
    try {
      const url = selectedApprover
        ? `${PROFILE_API}/hesi-pending/sync?approver=${encodeURIComponent(selectedApprover)}`
        : `${PROFILE_API}/hesi-pending/sync`;
      const res = await fetch(url, { method: 'POST', credentials: 'include' });
      const json = await res.json();
      if (res.ok && json.code === 200 && json.data) {
        const d = json.data;
        message.success(`已对齐合思实时待办: ${d.pulled} 单在审${d.added ? `, 新到 ${d.added} 单` : ''}${d.removed ? `, 撤走 ${d.removed} 单` : ''}`);
      } else {
        message.info(`${json.msg || '现场同步失败'}, 已改为只刷新本地数据`);
      }
    } catch {
      message.info('现场同步失败, 已改为只刷新本地数据');
    }
    setLiveSyncing(false);
    fetchPending(selectedApprover);
  };

  const handleBatchSubmit = async () => {
    if (batchAction === 'reject' && !batchComment.trim()) {
      message.warning('驳回必须填写备注');
      return;
    }
    setBatchLoading(true);
    let ok = 0;
    const fails: string[] = [];
    for (const it of selectedItems) {
      try {
        const res = await fetch(`${API_BASE}/api/hesi-bot/approve`, {
          method: 'POST',
          credentials: 'include',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            flowId: it.flowId,
            action: batchAction,
            comment: batchComment.trim(),
            // 批量驳回: 节点是单据级没法跨单统一选, 固定驳回至提交人; 重审路径可选
            ...(batchAction === 'reject' ? { rejectTo: '', resubmitMethod: batchResubmitMethod } : {}),
          }),
        });
        const json = await res.json();
        if (!res.ok || json.code !== 200) throw new Error(json.msg || `HTTP ${res.status}`);
        ok += 1;
        const fid = it.flowId;
        setOptimisticApproved(prev => {
          const next = new Map(prev);
          next.set(fid, Date.now());
          return next;
        });
      } catch (e) {
        fails.push(`${it.code}: ${e instanceof Error ? e.message : String(e)}`);
      }
    }
    setBatchLoading(false);
    setBatchOpen(false);
    setSelectedRowKeys([]);
    if (fails.length === 0) {
      message.success(`已批量提交 ${ok} 单, 机器人按合思限流分批处理(每分钟一批最多10单), 单子会陆续从列表消失`);
    } else {
      message.warning(`提交成功 ${ok} 单, 失败 ${fails.length} 单: ${fails[0]}${fails.length > 1 ? ' 等' : ''}`);
    }
    fetchQueue();
    fetchPending(selectedApprover);
  };

  const openApproveModal = (item: PendingItem) => {
    setApproveTarget(item);
    setApproveAction('agree');
    setApproveComment('');
    setRejectTo('');
    setResubmitMethod('TO_REJECTOR');
    setRejectNodes([]);
    // 预拉该单已审过的节点 (驳回节点下拉选项; 失败只剩"提交人"默认项, 不挡审批)
    setRejectNodesLoading(true);
    fetch(`${API_BASE}/api/hesi-bot/reject-nodes?flowId=${encodeURIComponent(item.flowId)}`, { credentials: 'include' })
      .then(res => res.json())
      .then(json => { if (json.code === 200 && json.data) setRejectNodes(json.data.nodes || []); })
      .catch(() => { /* 拉不到节点就只有提交人选项 */ })
      .finally(() => setRejectNodesLoading(false));
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
          ...(approveAction === 'reject' ? { rejectTo, resubmitMethod } : {}),
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
        const next = new Map(prev);
        next.set(submittedFlowId, Date.now());
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
      title: '发起人', dataIndex: 'ownerName', width: 90,
      render: (v: string | undefined) => v || '-',
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
              <div style={{ fontSize: 11, color: 'var(--text-secondary)' }}>{record.approverName}</div>
            </Tooltip>
          );
        }
        return v || <span style={{ color: 'var(--text-tertiary)' }}>-</span>;
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
        // 排队/提交/等生效 对用户是同一件事"在办", 统一一个标签, 悬停看具体阶段 (跑哥 6/11 反馈样式不统一)
        let phase = '';
        if (inQueue) {
          phase = inQueue.status === 'running' ? '正在提交合思' : '排队等提交 (合思 60s 限流)';
        } else if (isOptimistic) {
          phase = '已提交合思, 等流转生效 (约 1-2 分钟)';
        }
        const statusTag: React.ReactNode = phase ? (
          <Tooltip title={phase}>
            <Tag color="processing" icon={<ClockCircleOutlined />}>处理中</Tag>
          </Tooltip>
        ) : null;
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

      {/* 日常报销单判定规则卡, 查看日常报销审批人(樊雪娇/金海侠/周翻翻/张勇)待审批时展示 (跑哥 6/11; 6/18 扩) */}
      {DAILY_EXPENSE_APPROVERS.some((n) => ((selectedApprover || '') + (queryName || '') + (realName || '')).includes(n)) && <HesiBotRules />}
      {/* 对外付款单/预付款单判定规则卡, 查看付款单审批人(张俊/苏安妮)待审批时展示 (跑哥 2026-06-17; 6/18 扩) */}
      {PAYMENT_APPROVERS.some((n) => ((selectedApprover || '') + (queryName || '') + (realName || '')).includes(n)) && <HesiBotRulesZhangJun />}

      {/* 管理员视角切换 */}
      {isAdmin && (
        <Card size="small" style={{ marginBottom: 16, background: '#fafafa' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <UserOutlined style={{ color: '#1677ff' }} />
            <span style={{ color: 'var(--text-secondary)' }}>管理员视角 · 查看谁的待审批:</span>
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
              prefix={<SearchOutlined style={{ color: 'var(--text-tertiary)' }} />}
              placeholder="搜索 单据编码 / 标题 / 发起人"
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
            {showAuditCol && (
              <Select
                mode="multiple"
                placeholder="AI 建议"
                value={suggestionFilter}
                onChange={setSuggestionFilter}
                style={{ minWidth: 160 }}
                allowClear
                maxTagCount="responsive"
                options={[
                  { value: 'agree', label: '建议同意' },
                  { value: 'reject', label: '建议驳回' },
                  { value: 'manual', label: '转人工' },
                  { value: 'none', label: '无建议' },
                ]}
              />
            )}
            {hasFilter && (
              <Button size="small" onClick={() => { setSearchText(''); setFormTypeFilter([]); setDateRange(null); setSuggestionFilter([]); }}>
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
            <Tooltip title="去合思现场拉取当前审批人的实时待办(新到的单/被别处审掉的单都会对齐), 然后刷新清单. 秒级完成.">
              <Button icon={<ReloadOutlined />} onClick={handleLiveRefresh} loading={loading || liveSyncing} style={{ marginLeft: 'auto' }}>
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
          <>
            {selectedRowKeys.length > 0 && (
              <Alert
                type="info"
                style={{ marginBottom: 12 }}
                message={
                  <Space wrap>
                    <span>已选 <strong>{selectedRowKeys.length}</strong> 单 · 金额合计 ¥{selectedAmount.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</span>
                    <Button
                      type="primary"
                      size="small"
                      onClick={() => { setBatchAction('agree'); setBatchComment(''); setBatchOpen(true); }}
                    >
                      批量审批
                    </Button>
                    <Button size="small" onClick={() => setSelectedRowKeys([])}>取消选择</Button>
                  </Space>
                }
              />
            )}
            <Table<PendingItem>
              columns={columns}
              dataSource={filteredItems}
              rowKey="flowId"
              loading={loading}
              pagination={{ pageSize: 20, showSizeChanger: false }}
              size="middle"
              scroll={{ x: 'max-content' /* max-content: 表宽=真实列宽和(自动含条件显示的AI建议列), 防 fixed:right 操作列贴右压到邻列(底层冒上层); 勿改回写死数字, 否则加列/改宽后又会小于真实列宽和而复发 */ }}
              locale={{ emptyText: hasFilter ? '当前筛选下无匹配单据' : '暂无数据' }}
              rowSelection={{
                selectedRowKeys,
                onChange: setSelectedRowKeys,
                columnWidth: 42,
                fixed: true,
                // 排队中/已提交的单不可再勾 (跟单行"审批"按钮禁用口径一致)
                getCheckboxProps: (record) => ({
                  disabled: !!activeQueue.find(q => q.flowId === record.flowId) || optimisticApproved.has(record.flowId),
                }),
              }}
              rowClassName={(record) => {
                // P1 乐观更新: 已提交/队列中的单行整体置灰, 跑哥一眼看出"这单不必再点"
                const inQueue = activeQueue.find(q => q.flowId === record.flowId);
                return (inQueue || optimisticApproved.has(record.flowId)) ? 'hesi-row-pending-approve' : '';
              }}
            />
          </>
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
              <div style={{ color: 'var(--text-tertiary)', fontSize: 12 }}>
                {formTypeMap[approveTarget.formType]?.label || approveTarget.formType}
                {(() => {
                  const m = getMoney(approveTarget);
                  return m ? ` · ¥${m.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}` : '';
                })()}
                {approveTarget.stageName ? ` · 当前节点: ${approveTarget.stageName}` : ''}
              </div>
            </div>
            <div>
              <div style={{ marginBottom: 6, fontSize: 13, color: 'var(--text-tertiary)' }}>审批结果</div>
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
            {approveAction === 'reject' && (
              <>
                <div>
                  <div style={{ marginBottom: 6, fontSize: 13, color: 'var(--text-tertiary)' }}>驳回节点 (单子退给谁处理)</div>
                  <Select
                    value={rejectTo}
                    onChange={setRejectTo}
                    loading={rejectNodesLoading}
                    style={{ width: '100%' }}
                    options={[
                      { value: '', label: '提交人 (默认)' },
                      ...rejectNodes.map(n => ({ value: n.id, label: n.name })),
                    ]}
                  />
                </div>
                <div>
                  <div style={{ marginBottom: 6, fontSize: 13, color: 'var(--text-tertiary)' }}>重审路径 (改完重新提交后从哪开始审)</div>
                  <Radio.Group value={resubmitMethod} onChange={(e) => setResubmitMethod(e.target.value)}>
                    <Radio value="TO_REJECTOR">从当前节点开始审批 (默认, 前面已审过的不重审)</Radio>
                    <Radio value="FROM_START">从提交人节点开始审批 (重走完整流程)</Radio>
                  </Radio.Group>
                </div>
              </>
            )}
            <div>
              <div style={{ marginBottom: 6, fontSize: 13, color: 'var(--text-tertiary)' }}>
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

      {/* 批量审批 Modal */}
      <Modal
        title={`批量审批 · 已选 ${selectedItems.length} 单`}
        open={batchOpen}
        onCancel={() => setBatchOpen(false)}
        onOk={handleBatchSubmit}
        okText={`确定提交 ${selectedItems.length} 单`}
        cancelText="取消"
        confirmLoading={batchLoading}
        width={560}
        destroyOnHidden
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          <div style={{ background: '#f8fafc', padding: 10, borderRadius: 4, fontSize: 13, maxHeight: 160, overflowY: 'auto' }}>
            <div style={{ marginBottom: 6, color: 'var(--text-tertiary)' }}>
              金额合计 ¥{selectedAmount.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}
            </div>
            {selectedItems.map(it => (
              <div key={it.flowId} style={{ fontSize: 12, color: 'var(--text-secondary)' }}>
                {it.code} · {it.title}{(() => { const m = getMoney(it); return m ? ` · ¥${m.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}` : ''; })()}
              </div>
            ))}
          </div>
          <div>
            <div style={{ marginBottom: 6, fontSize: 13, color: 'var(--text-tertiary)' }}>审批结果 (整批统一)</div>
            <Radio.Group
              value={batchAction}
              onChange={(e) => setBatchAction(e.target.value)}
              optionType="button"
              buttonStyle="solid"
            >
              <Radio.Button value="agree"><CheckOutlined /> 同意</Radio.Button>
              <Radio.Button value="reject"><CloseOutlined /> 驳回</Radio.Button>
            </Radio.Group>
          </div>
          {batchAction === 'reject' && (
            <div>
              <div style={{ marginBottom: 6, fontSize: 13, color: 'var(--text-tertiary)' }}>
                重审路径 (整批统一; 批量驳回固定退给各单提交人, 想退到指定环节请单笔驳回)
              </div>
              <Radio.Group value={batchResubmitMethod} onChange={(e) => setBatchResubmitMethod(e.target.value)}>
                <Radio value="TO_REJECTOR">从当前节点开始审批 (默认, 前面已审过的不重审)</Radio>
                <Radio value="FROM_START">从提交人节点开始审批 (重走完整流程)</Radio>
              </Radio.Group>
            </div>
          )}
          <div>
            <div style={{ marginBottom: 6, fontSize: 13, color: 'var(--text-tertiary)' }}>
              备注{batchAction === 'reject' ? <span style={{ color: '#ef4444' }}> (驳回必填, 整批共用)</span> : ' (可选, 整批共用)'}
            </div>
            <Input.TextArea
              value={batchComment}
              onChange={(e) => setBatchComment(e.target.value)}
              rows={3}
              maxLength={500}
              showCount
              placeholder={batchAction === 'reject' ? '请写明驳回理由(将提交到合思, 整批共用同一条)' : '可选, 会一同提交到合思'}
            />
          </div>
          <Alert
            type="warning"
            showIcon
            message="整批提交到合思后无法撤销; 机器人按合思限流分批处理(每分钟一批最多10单), 单子会陆续从列表消失"
            style={{ fontSize: 12 }}
          />
        </div>
      </Modal>

      {/* 单据详情弹窗 — 共享组件 HesiFlowDetailModal, 跟费控管理(finance/ExpenseControl)共用一套 */}
      {/* 改一处两边同步 (跑哥要求"这两个一直保持一致") */}
      <HesiFlowDetailModal
        open={detailModal.visible}
        flowId={detailModal.flowId}
        onClose={() => setDetailModal({ visible: false, flowId: '' })}
        flowDetailUrl={(id) => `${PROFILE_API}/hesi-flow-detail?flowId=${id}`}
        attachmentUrlsUrl={(id) => `${PROFILE_API}/hesi-attachment-urls?flowId=${id}`}
        approvalFlowUrl={(id) => `${PROFILE_API}/hesi-approval-flow?flowId=${id}`}
      />
    </div>
  );
};

export default HesiBot;
