// v1.59.0 个人中心 → 合思机器人 Tab
// MVP: 只读"我的待审批" 单据列表. 后续 v1.60.0 加规则编辑, v1.61.0 dry run, v1.62.0 真自动审批.
// v1.62.x: 字段/详情对齐费控管理 (单据模板/创建时间/发票/附件 + 点击查看明细 Modal 4 Tab)

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Alert, Badge, Button, Card, DatePicker, Descriptions, Empty, Input, Modal,
  Radio, Select, Statistic, Table, Tabs, Tag, Tooltip, Typography, message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  CheckCircleOutlined, CheckOutlined, ClockCircleOutlined, CloseOutlined,
  EyeOutlined, FileImageOutlined, FileTextOutlined, PaperClipOutlined,
  ReloadOutlined, RobotOutlined, SearchOutlined, UserOutlined, WarningOutlined,
} from '@ant-design/icons';
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
  // 详情 Modal
  const [detailModal, setDetailModal] = useState<{ visible: boolean; flowId: string }>({ visible: false, flowId: '' });
  const [detailData, setDetailData] = useState<any>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [attachUrls, setAttachUrls] = useState<any>(null);
  const [attachLoading, setAttachLoading] = useState(false);

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
        setActiveQueue(json.data.active || []);
      }
    } catch { /* silent */ }
  }, []);

  // 30s polling 队列 + 待审批列表
  useEffect(() => {
    fetchQueue();
    const t = setInterval(() => {
      fetchQueue();
      fetchPending(selectedApproverRef.current);
    }, 30000);
    return () => clearInterval(t);
  }, [fetchQueue, fetchPending]);

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
  // AI 建议规则当前仅适用于张俊, 别人不跑规则 → 后端返回的 items 都没 suggestion → 前端藏列
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
      setApproveTarget(null);
      fetchQueue();
      fetchPending(selectedApprover);
    } catch (e) {
      message.error('审批失败: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      setApproveLoading(false);
    }
  };

  const showDetail = async (flowId: string) => {
    setDetailModal({ visible: true, flowId });
    setDetailData(null);
    setDetailLoading(true);
    setAttachUrls(null);
    try {
      const res = await fetch(`${PROFILE_API}/hesi-flow-detail?flowId=${flowId}`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200) {
        setDetailData(json.data);
      } else {
        message.error(json.msg || '获取单据详情失败');
      }
    } catch (e) {
      message.error('获取单据详情失败');
    } finally {
      setDetailLoading(false);
    }
  };

  const loadAttachUrls = async (flowId: string) => {
    setAttachLoading(true);
    try {
      const res = await fetch(`${PROFILE_API}/hesi-attachment-urls?flowId=${flowId}`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200 && json.items) {
        setAttachUrls(json);
      } else {
        message.error(json.msg || '获取附件链接失败');
      }
    } catch (e) {
      message.error('获取附件链接失败');
    } finally {
      setAttachLoading(false);
    }
  };

  const renderAttachments = () => {
    if (!attachUrls?.items?.[0]?.attachmentList) return <div style={{ color: '#999' }}>无附件</div>;
    const list = attachUrls.items[0].attachmentList;
    const typeLabels: Record<string, string> = {
      'flow.body': '单据附件',
      'flow.free': '费用明细附件',
      'flow.approving': '审批附件',
      'flow.receipt': '回单',
    };
    return list.map((att: any, idx: number) => (
      <div key={idx} style={{ marginBottom: 16 }}>
        <h4 style={{ margin: '0 0 8px', color: '#06b6d4' }}>{typeLabels[att.type] || att.type}</h4>
        {att.attachmentUrls?.map((f: any, i: number) => (
          <div key={`a-${i}`} style={{ marginLeft: 16, marginBottom: 4 }}>
            <PaperClipOutlined style={{ marginRight: 4 }} />
            <a href={f.url} target="_blank" rel="noopener noreferrer">{f.fileName || f.key || '下载'}</a>
          </div>
        ))}
        {att.invoiceUrls?.map((f: any, i: number) => (
          <div key={`i-${i}`} style={{ marginLeft: 16, marginBottom: 4 }}>
            <FileImageOutlined style={{ color: '#ff4d4f', marginRight: 4 }} />
            <a href={f.url} target="_blank" rel="noopener noreferrer">
              {f.fileName || f.key || '发票'}
              {f.invoiceCode ? ` (${f.invoiceCode})` : ''}
            </a>
          </div>
        ))}
        {att.receiptUrls?.map((f: any, i: number) => (
          <div key={`r-${i}`} style={{ marginLeft: 16, marginBottom: 4 }}>
            <FileTextOutlined style={{ color: '#52c41a', marginRight: 4 }} />
            <a href={f.url} target="_blank" rel="noopener noreferrer">{f.key || '回单'}</a>
          </div>
        ))}
      </div>
    ));
  };

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
        return (
          <div style={{ display: 'flex', gap: 4, justifyContent: 'center' }}>
            <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => showDetail(record.flowId)}>
              详情
            </Button>
            {inQueue ? (
              <Tag color={inQueue.status === 'running' ? 'processing' : 'default'}>
                {inQueue.status === 'running' ? '处理中' : '排队中'}
              </Tag>
            ) : (
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

      {/* 详情 Modal (复用费控管理 4 Tab 布局) */}
      <Modal
        title={detailData?.flow?.code ? `${detailData.flow.code} - ${detailData.flow.title}` : '单据详情'}
        open={detailModal.visible}
        onCancel={() => {
          setDetailModal({ visible: false, flowId: '' });
          setDetailData(null);
          setAttachUrls(null);
        }}
        footer={null}
        width={900}
        destroyOnHidden
      >
        {detailLoading ? <div style={{ textAlign: 'center', padding: 40 }}>加载中...</div> : detailData && (
          <Tabs defaultActiveKey="basic" items={[
            {
              key: 'basic',
              label: '基本信息',
              children: (
                <Descriptions bordered size="small" column={2}>
                  <Descriptions.Item label="单据编码">{detailData.flow.code}</Descriptions.Item>
                  <Descriptions.Item label="单据类型">
                    {formTypeMap[detailData.flow.formType]?.label || detailData.flow.formType}
                  </Descriptions.Item>
                  <Descriptions.Item label="状态">
                    <Tag color={stateMap[detailData.flow.state]?.color}>
                      {stateMap[detailData.flow.state]?.label || detailData.flow.state}
                    </Tag>
                  </Descriptions.Item>
                  <Descriptions.Item label="支付金额">
                    {detailData.flow.payMoney ? `¥${detailData.flow.payMoney.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}` : '-'}
                  </Descriptions.Item>
                  <Descriptions.Item label="报销金额">
                    {detailData.flow.expenseMoney ? `¥${detailData.flow.expenseMoney.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}` : '-'}
                  </Descriptions.Item>
                  <Descriptions.Item label="凭证状态">{detailData.flow.voucherStatus || '-'}</Descriptions.Item>
                  <Descriptions.Item label="创建时间">{formatTime(detailData.flow.createTime)}</Descriptions.Item>
                  <Descriptions.Item label="提交时间">{formatTime(detailData.flow.submitDate)}</Descriptions.Item>
                  <Descriptions.Item label="支付时间">{formatTime(detailData.flow.payDate)}</Descriptions.Item>
                  <Descriptions.Item label="完成时间">{formatTime(detailData.flow.flowEndTime)}</Descriptions.Item>
                  <Descriptions.Item label="单据模板" span={2}>
                    {(() => {
                      const sid: string | null = detailData.flow.specificationId;
                      const sname: string | null = detailData.flow.specificationName;
                      if (!sid) return '-';
                      return (
                        <Tooltip title={sid}>
                          <Tag color="blue" style={{ cursor: 'help' }}>
                            {sname || '未匹配字典'}
                          </Tag>
                          <Typography.Text type="secondary" style={{ fontSize: 11, marginLeft: 8 }} copyable={{ text: sid }}>
                            ID: {sid.length > 40 ? sid.slice(0, 40) + '...' : sid}
                          </Typography.Text>
                        </Tooltip>
                      );
                    })()}
                  </Descriptions.Item>
                </Descriptions>
              ),
            },
            {
              key: 'details',
              label: `费用明细 (${detailData.details?.length || 0})`,
              children: (
                <Table
                  size="small"
                  dataSource={detailData.details || []}
                  rowKey={(r: any) => r.detailId || `${r.detailNo}-${r.amount}-${r.feeDate}`}
                  pagination={false}
                  columns={[
                    { title: '序号', dataIndex: 'detailNo', width: 60 },
                    {
                      title: '金额', dataIndex: 'amount', width: 120, align: 'right',
                      render: (v: number) => v ? `¥${v.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}` : '-',
                    },
                    {
                      title: '消费时间', dataIndex: 'feeDate', width: 120,
                      render: (v: number) => v ? dayjs(v).format('YYYY-MM-DD') : '-',
                    },
                    {
                      title: '发票', dataIndex: 'invoiceStatus', width: 80,
                      render: (v: string) => v === 'exist'
                        ? <Tag icon={<CheckCircleOutlined />} color="success">有</Tag>
                        : <Tag icon={<WarningOutlined />} color="error">无</Tag>,
                    },
                    { title: '消费原因', dataIndex: 'consumptionReasons', ellipsis: true },
                  ]}
                />
              ),
            },
            {
              key: 'invoices',
              label: `发票 (${detailData.invoices?.length || 0})`,
              children: (
                <Table
                  size="small"
                  dataSource={detailData.invoices || []}
                  rowKey={(r: any) => r.invoiceId || r.invoiceNumber || `${r.invoiceCode}-${r.totalAmount}`}
                  pagination={false}
                  scroll={{ x: 1200 }}
                  columns={[
                    { title: '发票号码', dataIndex: 'invoiceNumber', width: 200 },
                    {
                      title: '发票日期', dataIndex: 'invoiceDate', width: 110,
                      render: (v: number) => v ? dayjs(v).format('YYYY-MM-DD') : '-',
                    },
                    {
                      title: '价税合计', dataIndex: 'totalAmount', width: 120, align: 'right',
                      render: (v: number) => v ? `¥${v.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}` : '-',
                    },
                    {
                      title: '税额', dataIndex: 'taxAmount', width: 100, align: 'right',
                      render: (v: number) => v ? `¥${v.toFixed(2)}` : '-',
                    },
                    {
                      title: '发票类型', dataIndex: 'invoiceType', width: 140,
                      render: (v: string) => {
                        const m: Record<string, string> = {
                          'FULL_DIGITAl_SPECIAL': '全电专票',
                          'FULL_DIGITAl_NORMAL': '全电普票',
                          'SPECIAL_VAT': '增值税专票',
                          'NORMAL_VAT': '增值税普票',
                          'NORMAL_ELECTRONIC': '电子普票',
                          'SPECIAL_ELECTRONIC': '电子专票',
                        };
                        return m[v] || v || '-';
                      },
                    },
                    { title: '销售方', dataIndex: 'sellerName', width: 200, ellipsis: true },
                    {
                      title: '验真', dataIndex: 'isVerified', width: 60, align: 'center',
                      render: (v: number) => v ? <Tag color="success">是</Tag> : <Tag>否</Tag>,
                    },
                  ]}
                />
              ),
            },
            {
              key: 'attachments',
              label: `附件 (${detailData.attachments?.length || 0})`,
              children: (
                <div>
                  <Button
                    type="primary" size="small" icon={<PaperClipOutlined />}
                    loading={attachLoading}
                    onClick={() => loadAttachUrls(detailModal.flowId)}
                    style={{ marginBottom: 16 }}
                  >
                    获取附件下载链接（1小时有效）
                  </Button>
                  {attachUrls ? renderAttachments() : (
                    <Table
                      size="small"
                      dataSource={detailData.attachments || []}
                      rowKey={(r: any) => r.fileId || `${r.attachmentType}-${r.fileName}`}
                      pagination={false}
                      columns={[
                        {
                          title: '类型', dataIndex: 'attachmentType', width: 120,
                          render: (v: string) => {
                            const m: Record<string, string> = { 'flow.body': '单据附件', 'flow.free': '费用明细', 'flow.approving': '审批附件', 'flow.receipt': '回单' };
                            return m[v] || v;
                          },
                        },
                        { title: '文件名', dataIndex: 'fileName' },
                        {
                          title: '发票', dataIndex: 'isInvoice', width: 60, align: 'center',
                          render: (v: number) => v ? <Tag color="blue">是</Tag> : null,
                        },
                        { title: '发票号码', dataIndex: 'invoiceCode', width: 150 },
                      ]}
                    />
                  )}
                </div>
              ),
            },
          ]} />
        )}
      </Modal>
    </div>
  );
};

export default HesiBot;
