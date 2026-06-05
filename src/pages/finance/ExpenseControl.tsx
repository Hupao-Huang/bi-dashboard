import React, { useState, useEffect, useCallback } from 'react';
import { Card, Table, Tag, Select, Input, DatePicker, Row, Col, Statistic, Button, Modal, Descriptions, Tabs, Typography, Badge, Tooltip, message, Image } from 'antd';
import {
  FileTextOutlined, DollarOutlined, WarningOutlined,
  CheckCircleOutlined, ClockCircleOutlined, SearchOutlined,
  PaperClipOutlined, FileImageOutlined, ReloadOutlined,
  EyeOutlined, SyncOutlined
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import { API_BASE } from '../../config';

const { RangePicker } = DatePicker;
const { Option } = Select;

const API = `${API_BASE}/api/hesi`;

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

interface FlowItem {
  flowId: string;
  code: string;
  title: string;
  formType: string;
  state: string;
  ownerId: string | null;
  departmentId: string | null;
  submitterId: string | null;
  payMoney: number | null;
  expenseMoney: number | null;
  loanMoney: number | null;
  createTime: number | null;
  updateTime: number | null;
  submitDate: number | null;
  payDate: number | null;
  flowEndTime: number | null;
  voucherNo: string | null;
  voucherStatus: string | null;
  detailCount: number;
  invoiceExist: number;
  invoiceMissing: number;
  attachmentCount: number;
  preApprovedNode?: string | null;  // v1.57.2: 上一步已审批节点名
  preApprovedTime?: string | null;  // v1.57.2: 上一步通过时间戳(毫秒, string 因 raw_json 提取)
  currentStageName?: string | null;    // v1.58.0: 当前审批节点 (来自合思 approveStates 接口)
  currentApproverName?: string | null; // v1.58.0: 当前审批人姓名
  currentApproverCode?: string | null; // v1.58.0: 当前审批人工号
  specificationId?: string | null;     // v1.62.x: 合思单据模板 ID
  specificationName?: string | null;   // v1.62.x: 合思单据模板真实名称 (字典查)
}

interface StatsData {
  totalFlows: number;
  totalExpense: number;
  totalLoan: number;
  totalRequisition: number;
  totalCustom: number;
  paidNoInvoice: number;
  approving: number;
  paying: number;
  totalAttachments: number;
  totalInvoiceFiles: number;
  stateDistribution: { state: string; count: number }[];
  typeDistribution: { formType: string; count: number }[];
}

// v1.74.5: 费用明细展开行 - 把合思 API 原始字段 (raw_json.feeTypeForm) 全部展示
// 不同费用类型字段不一样: 差旅有出发到达城市, 报销有付款截图, 出差补贴有天数+分类金额
const HESI_DETAIL_HIDDEN_KEYS = new Set([
  // 主表已展示, 不重复
  'feeTypeId', 'detailId', 'detailNo', 'specificationId',
  'amount', 'feeDate', 'invoice', 'consumptionReasons',
  // 发票/附件 Tab 已专题展示, 这里只在数组层 short summary
  'invoiceForm',
]);
const HESI_FIELD_LABELS: Record<string, string> = {
  feeDatePeriod: '消费日期段',
  attachments: '本明细附件',
  city: '城市',
  fromCity: '出发地',
  toCity: '目的地',
  linkDetailEntities: '关联明细',
};

function labelOfHesiField(key: string): string {
  if (key.startsWith('u_')) {
    const rest = key.slice(2);
    // u_ID_中文 模式: 取第一个 _ 后段, 如有中文用后段, 否则原样
    const m = rest.match(/^[A-Za-z0-9]+_(.+)$/);
    if (m && /[一-龥]/.test(m[1])) return m[1];
    return rest;
  }
  return HESI_FIELD_LABELS[key] || key;
}

function renderHesiValue(key: string, v: any, ctx?: { resolve?: (n: string) => any; preview?: (f: any) => void }): React.ReactNode {
  if (v === null || v === undefined || v === '') return '-';
  // 合思金额对象 (有 standard + standardSymbol)
  if (typeof v === 'object' && !Array.isArray(v) && 'standard' in v) {
    const sym = v.standardSymbol || '¥';
    const unit = v.standardUnit || '';
    return `${sym}${v.standard}${unit ? ' ' + unit : ''}`;
  }
  // 日期段 {start, end}
  if (typeof v === 'object' && !Array.isArray(v) && 'start' in v && 'end' in v) {
    return `${dayjs(v.start).format('YYYY-MM-DD')} ~ ${dayjs(v.end).format('YYYY-MM-DD')}`;
  }
  // 城市类: JSON 字符串 [{key, label}] / {key, label}
  if (typeof v === 'string' && (v.startsWith('[{') || v.startsWith('{')) && v.includes('label')) {
    try {
      const parsed = JSON.parse(v);
      if (Array.isArray(parsed)) return parsed.map((x: any) => x.label).filter(Boolean).join(' / ') || v;
      if (parsed.label) return parsed.label;
    } catch { /* 不是合法 JSON 就原样返 */ }
  }
  // 时间戳: key 含 Date/Time + value > 1e12
  if (typeof v === 'number' && v > 1e12 && /Date|Time/i.test(key)) {
    return dayjs(v).format('YYYY-MM-DD');
  }
  // 数组
  if (Array.isArray(v)) {
    if (v.length === 0) return '-';
    // 附件/文件类 (有 fileName/fileId)
    if (typeof v[0] === 'object' && (v[0].fileName || v[0].fileId)) {
      return (
        <ul style={{ margin: 0, paddingLeft: 18 }}>
          {v.map((f: any, i: number) => {
            const label = f.fileName || f.fileId;
            const file = ctx?.resolve?.(f.fileName || f.fileId);
            return <li key={i}>{file && ctx?.preview
              ? <a onClick={() => ctx.preview!(file)}>{label}</a>
              : label}</li>;
          })}
        </ul>
      );
    }
    return <Typography.Text type="secondary">{v.length} 项</Typography.Text>;
  }
  // 其他嵌套对象 → 短 JSON (可复制)
  if (typeof v === 'object') {
    const s = JSON.stringify(v);
    return <Typography.Text type="secondary" copyable={{ text: s }}>{s.length > 80 ? s.slice(0, 80) + '...' : s}</Typography.Text>;
  }
  // 纯合思 ID (ID01 开头, 字典未匹配) — 对用户无意义, 不显示原始 ID, 留空
  if (typeof v === 'string' && /^ID0[0-9A-Za-z]{8,}$/.test(v)) {
    return <Typography.Text type="secondary">-</Typography.Text>;
  }
  return String(v);
}

function renderHesiDetailExpand(record: any, ctx?: { resolve?: (n: string) => any; preview?: (f: any) => void }): React.ReactNode {
  const raw = record.rawJson;
  if (!raw) return <Typography.Text type="secondary">老数据未存原始字段</Typography.Text>;
  const form = raw.feeTypeForm || raw;
  if (!form || typeof form !== 'object') return <Typography.Text type="secondary">无更多信息</Typography.Text>;
  // 隐藏: 已展示字段 + 值是未匹配字典 ID 的字段(对用户无意义, 如预算费用 ID)
  const entries = Object.entries(form).filter(([k, v]) =>
    !HESI_DETAIL_HIDDEN_KEYS.has(k) && !(typeof v === 'string' && /^ID0[0-9A-Za-z]{8,}$/.test(v))
  );
  if (entries.length === 0) return <Typography.Text type="secondary">无更多明细字段</Typography.Text>;
  return (
    <Descriptions size="small" column={2} bordered>
      {entries.map(([k, v]) => (
        <Descriptions.Item key={k} label={labelOfHesiField(k)}>
          {renderHesiValue(k, v, ctx)}
        </Descriptions.Item>
      ))}
    </Descriptions>
  );
}

const ExpenseControl: React.FC = () => {
  const [stats, setStats] = useState<StatsData | null>(null);
  const [flows, setFlows] = useState<FlowItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [formType, setFormType] = useState<string | undefined>(undefined);
  const [state, setState] = useState<string | undefined>('active'); // v1.62.x 默认隐藏已结束
  const [invoiceStatus, setInvoiceStatus] = useState<string | undefined>(undefined);
  const [loanRepaid, setLoanRepaid] = useState<boolean>(false); // v1.70.5: 借款待还款 KPI 卡筛选
  const [keyword, setKeyword] = useState('');
  const [keywordInput, setKeywordInput] = useState(''); // v1.58.3: 输入框 draft, 点查询才提交到 keyword
  const [approver, setApprover] = useState(''); // v1.58.2: 当前审批人 LIKE 搜
  const [approverInput, setApproverInput] = useState(''); // v1.58.3: 输入框 draft
  const [specificationId, setSpecificationId] = useState<string | undefined>(undefined); // v1.63.x: 单据模板筛选
  const [specOptions, setSpecOptions] = useState<{ id: string; name: string; type: string }[]>([]);
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null] | null>(null);
  const [detailModal, setDetailModal] = useState<{ visible: boolean; flowId: string }>({ visible: false, flowId: '' });
  const [detailData, setDetailData] = useState<any>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [attachUrls, setAttachUrls] = useState<any>(null);
  const [attachLoading, setAttachLoading] = useState(false);
  // 单张发票原件预览弹窗
  const [invoicePreview, setInvoicePreview] = useState<{ visible: boolean; file: any; title: string }>({ visible: false, file: null, title: '' });
  // v1.75.8: 凭证明细从 Tab 改子弹窗
  const [voucherModalOpen, setVoucherModalOpen] = useState(false);

  const [lastSyncAt, setLastSyncAt] = useState<string | null>(null);

  const fetchStats = useCallback(async () => {
    try {
      const res = await fetch(`${API}/stats`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200) setStats(json.data);
    } catch (e) { /* ignore */ }
  }, []);

  const fetchLastSync = useCallback(async () => {
    try {
      const res = await fetch(`${API}/last-sync`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200 && json.data?.lastSyncAt) setLastSyncAt(json.data.lastSyncAt);
    } catch (e) { /* ignore */ }
  }, []);

  const fetchFlows = useCallback(async (silent: boolean = false) => {
    if (!silent) setLoading(true);
    try {
      const params = new URLSearchParams();
      params.set('page', String(page));
      params.set('pageSize', String(pageSize));
      if (formType) params.set('formType', formType);
      if (state) params.set('state', state);
      if (invoiceStatus) params.set('invoiceStatus', invoiceStatus);
      if (loanRepaid) params.set('loanRepaid', '1');
      if (keyword) params.set('keyword', keyword);
      if (approver) params.set('approver', approver);
      if (specificationId) params.set('specificationId', specificationId);
      if (dateRange?.[0]) params.set('startDate', dateRange[0].format('YYYY-MM-DD'));
      if (dateRange?.[1]) params.set('endDate', dateRange[1].format('YYYY-MM-DD'));

      const res = await fetch(`${API}/flows?${params}`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200) {
        setFlows(json.data.items || []);
        setTotal(json.data.total || 0);
      }
    } catch (e) {
      if (!silent) message.error('获取单据列表失败');
    } finally {
      if (!silent) setLoading(false);
    }
  }, [page, pageSize, formType, state, invoiceStatus, loanRepaid, keyword, approver, specificationId, dateRange]);

  useEffect(() => { fetchStats(); }, [fetchStats]);
  useEffect(() => { fetchFlows(false); }, [fetchFlows]);
  useEffect(() => {
    fetchLastSync();
    const t = setInterval(fetchLastSync, 60000); // 60s 自动刷新
    return () => clearInterval(t);
  }, [fetchLastSync]);
  // 30s 静默轮询列表 — 让合思机器人审批 / sync-hesi 跑完后费控管理也能看到新审批人, 不必手动刷新
  // silent=true 不闪 loading、不弹错误, 不打断用户筛选/翻页
  useEffect(() => {
    const t = setInterval(() => { fetchFlows(true); }, 30000);
    return () => clearInterval(t);
  }, [fetchFlows]);

  // v1.63.x 拉合思单据模板字典 (60s 服务端缓存, 页面打开拉一次)
  useEffect(() => {
    fetch(`${API}/specifications`, { credentials: 'include' })
      .then(r => r.json())
      .then(json => { if (json.code === 200) setSpecOptions(json.data?.items || []); })
      .catch(() => {});
  }, []);

  // v1.57.1: 立即同步 + 实时日志
  const [syncing, setSyncing] = useState(false);
  const [logModalOpen, setLogModalOpen] = useState(false);
  const [logText, setLogText] = useState('');
  const logTimerRef = React.useRef<number | null>(null);
  const logBoxRef = React.useRef<HTMLPreElement | null>(null);

  const handleSync = async () => {
    Modal.confirm({
      title: '立即同步合思单据',
      content: '将启动后台同步任务 (拉合思单据 + 报销 + 流程明细 + 附件), 大约 5-10 分钟. 期间页面数据不会立刻刷新, 完成后手动刷新本页即可.',
      okText: '启动',
      cancelText: '取消',
      onOk: async () => {
        setSyncing(true);
        try {
          const res = await fetch(`${API_BASE}/api/admin/tasks/run`, {
            method: 'POST',
            credentials: 'include',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ task: 'sync-hesi', params: {} }),
          });
          if (!res.ok) {
            const text = await res.text();
            throw new Error(text || `HTTP ${res.status}`);
          }
          message.success('合思同步已启动, 5-10 分钟后回来刷新看新数据 (点"看日志"查实时进度)');
        } catch (err) {
          const msg = err instanceof Error ? err.message : String(err);
          message.error(`启动失败: ${msg}`);
        } finally {
          setSyncing(false);
        }
      },
    });
  };

  const fetchSyncLog = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/api/admin/sync-tools/log?key=sync-hesi&lines=300`, {
        credentials: 'include',
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const json = await res.json();
      const text = (json.data?.lines || []).join('\n');
      setLogText(text || '(暂无日志, 还没跑过)');
      setTimeout(() => {
        if (logBoxRef.current) logBoxRef.current.scrollTop = logBoxRef.current.scrollHeight;
      }, 50);
    } catch (err) {
      setLogText(`获取失败: ${err instanceof Error ? err.message : String(err)}`);
    }
  }, []);

  const openLogModal = () => {
    setLogModalOpen(true);
    setLogText('加载中...');
    fetchSyncLog();
    if (logTimerRef.current) window.clearInterval(logTimerRef.current);
    logTimerRef.current = window.setInterval(fetchSyncLog, 3000);
  };

  const closeLogModal = () => {
    setLogModalOpen(false);
    if (logTimerRef.current) {
      window.clearInterval(logTimerRef.current);
      logTimerRef.current = null;
    }
  };

  useEffect(() => () => {
    if (logTimerRef.current) window.clearInterval(logTimerRef.current);
  }, []);

  const showDetail = async (flowId: string) => {
    setDetailModal({ visible: true, flowId });
    setDetailData(null);
    setDetailLoading(true);
    setAttachUrls(null);
    try {
      const res = await fetch(`${API}/flow-detail?flowId=${flowId}`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200) {
        setDetailData(json.data);
        // 总是自动拉在线附件链接 — 同步的附件表可能为空, 以合思在线接口为准
        void loadAttachUrls(flowId);
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
      const res = await fetch(`${API}/attachment-urls?flowId=${flowId}`, { credentials: 'include' });
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

  const triggerQuery = useCallback(() => {
    // v1.58.3: 把输入框 draft 提交到实际 state, 触发 useEffect 重查
    setKeyword(keywordInput);
    setApprover(approverInput);
    setPage(1);
    if (page === 1 && keywordInput === keyword && approverInput === approver) {
      void fetchFlows();
    }
  }, [fetchFlows, page, keywordInput, approverInput, keyword, approver]);

  const resetFilters = useCallback(() => {
    const noFilter = !formType && state === 'active' && !invoiceStatus && !loanRepaid && !keyword && !approver && !specificationId && !dateRange;
    setFormType(undefined);
    setState('active'); // v1.62.x 重置回默认"未结束"
    setInvoiceStatus(undefined);
    setLoanRepaid(false);
    setKeyword('');
    setKeywordInput('');
    setApprover('');
    setApproverInput('');
    setSpecificationId(undefined);
    setDateRange(null);
    setPage(1);
    if (page === 1 && noFilter) {
      void fetchFlows();
    }
    void fetchStats();
  }, [dateRange, fetchFlows, fetchStats, formType, invoiceStatus, keyword, approver, specificationId, page, state]);

  const getMoney = (item: FlowItem) => {
    // payMoney为0时优先用expenseMoney（合思线下支付的单据payMoney可能为0）
    if (item.payMoney && item.payMoney > 0) return item.payMoney;
    if (item.expenseMoney && item.expenseMoney > 0) return item.expenseMoney;
    if (item.loanMoney && item.loanMoney > 0) return item.loanMoney;
    return null;
  };

  const getPayTime = (item: FlowItem) => {
    // payDate为空时用flowEndTime作为替代（单据完成时间）
    if (item.payDate) return item.payDate;
    if (item.flowEndTime) return item.flowEndTime;
    return null;
  };

  const formatTime = (ts: number | null) => {
    if (!ts) return '-';
    return dayjs(ts).format('YYYY-MM-DD HH:mm');
  };

  const quickFilter = (f: string, s: string, inv: string, loanRepaidFlag: boolean = false) => {
    setFormType(f || undefined);
    setState(s || undefined);
    setInvoiceStatus(inv || undefined);
    setLoanRepaid(loanRepaidFlag);
    setPage(1);
  };
  const statCards = [
    { title: '单据总数', value: stats?.totalFlows || 0, prefix: <FileTextOutlined />, accentColor: '#1e40af', onClick: () => quickFilter('', '', '') },
    { title: '报销单', value: stats?.totalExpense || 0, prefix: <DollarOutlined />, accentColor: '#10b981', onClick: () => quickFilter('expense', '', '') },
    { title: '借款待还款', value: stats?.paidNoInvoice || 0, prefix: <WarningOutlined />, accentColor: '#ef4444', onClick: () => quickFilter('', '', '', true) },
    { title: '审批中', value: stats?.approving || 0, prefix: <ClockCircleOutlined />, accentColor: '#06b6d4', onClick: () => quickFilter('', 'approving', '') },
    { title: '待支付', value: stats?.paying || 0, prefix: <ClockCircleOutlined />, accentColor: '#faad14', onClick: () => quickFilter('', 'paying', '') },
    { title: '发票文件', value: stats?.totalInvoiceFiles || 0, prefix: <FileImageOutlined />, accentColor: '#7c3aed' },
  ];

  const columns: ColumnsType<FlowItem> = [
    {
      title: '单据编码', dataIndex: 'code', width: 130, fixed: 'left',
      render: (code, record) => (
        <Button type="link" onClick={() => showDetail(record.flowId)} style={{ padding: 0, height: 'auto' }}>
          {code}
        </Button>
      ),
    },
    {
      title: '标题', dataIndex: 'title', width: 280, ellipsis: true,
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
      render: (v: string | null, record: FlowItem) => {
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
      title: '当前审批', width: 160,
      render: (_, record) => {
        // v1.70.5: 终态单据 (已支付/已归档/被拒) 一律显示 "已结束"
        // 避免 sync 没追上时显示过期的"出纳支付/姚晓倩" 等误导信息
        if (record.state === 'paid' || record.state === 'archived' || record.state === 'rejected') {
          const tip = record.preApprovedNode
            ? `已结束 (上一步: ${record.preApprovedNode})`
            : '已结束';
          return <Tooltip title={tip}><span style={{color:'#999'}}>已结束</span></Tooltip>;
        }
        // v1.58.0: 进行中单据, 优先显示真实审批人 (合思 approveStates 接口)
        if (record.currentApproverName && record.currentStageName) {
          return (
            <Tooltip title={`节点: ${record.currentStageName}  审批人: ${record.currentApproverName}${record.currentApproverCode ? ' (' + record.currentApproverCode + ')' : ''}`}>
              <div><strong>{record.currentApproverName}</strong></div>
              <div style={{fontSize:11, color:'#666'}}>{record.currentStageName}</div>
            </Tooltip>
          );
        }
        // 进行中但还没拉到审批状态 — 显示上一步节点 (v1.57.2 老逻辑)
        if (record.preApprovedNode) {
          const t = record.preApprovedTime ? Number(record.preApprovedTime) : 0;
          const tStr = t > 0 ? dayjs(t).format('MM-DD HH:mm') : '';
          return (
            <Tooltip title={tStr ? `上一步 ${record.preApprovedNode} 于 ${tStr} 通过, 待下游` : record.preApprovedNode}>
              <span style={{color:'#666'}}>{record.preApprovedNode} ✓</span>
            </Tooltip>
          );
        }
        return <span style={{color:'#999'}}>未启动</span>;
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
        if (record.invoiceMissing > 0) {
          return <Tag color="error">未到</Tag>;
        }
        return <Tag color="success">齐全</Tag>;
      },
    },
    {
      title: '附件', dataIndex: 'attachmentCount', width: 70, align: 'center',
      render: (v) => v > 0 ? <Badge count={v} style={{ backgroundColor: '#06b6d4' }} /> : '-',
    },
    {
      title: '创建时间', dataIndex: 'createTime', width: 150,
      render: (v) => formatTime(v),
    },
    {
      title: '支付/完成时间', width: 150,
      render: (_, record) => formatTime(getPayTime(record)),
    },
    {
      title: '操作', width: 80, fixed: 'right', align: 'center',
      render: (_, record) => (
        <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => showDetail(record.flowId)}>
          详情
        </Button>
      ),
    },
  ];

  // 附件弹窗中的附件列表渲染
  // 单个文件预览: 图片→缩略图(点击放大), PDF→内嵌预览, 其它→点击打开
  const renderFilePreview = (f: any, key: string, fallbackName: string) => {
    const url: string = f.url || '';
    const name: string = f.fileName || f.key || (f.invoiceCode ? `发票 ${f.invoiceCode}` : fallbackName);
    const lower = `${name} ${url}`.toLowerCase();
    const isImg = /\.(jpg|jpeg|png|gif|webp|bmp)(\?|#|$)/.test(lower);
    const isPdf = /\.pdf(\?|#|$)/.test(lower);
    if (isImg) {
      return (
        <div key={key} style={{ width: 150 }}>
          <Image src={url} width={150} height={200} style={{ objectFit: 'cover', borderRadius: 6, border: '1px solid #eee' }} />
          <div style={{ fontSize: 11, color: '#888', marginTop: 2, wordBreak: 'break-all' }}>{name}</div>
        </div>
      );
    }
    if (isPdf) {
      return (
        <div key={key} style={{ width: 280 }}>
          <iframe src={`${url}#toolbar=0&navpanes=0&view=FitH`} title={name} style={{ width: '100%', height: 360, border: '1px solid #eee', borderRadius: 6 }} />
          <div style={{ fontSize: 11, marginTop: 2, wordBreak: 'break-all' }}>
            <a href={url} target="_blank" rel="noopener noreferrer">{name} ↗ 新窗口打开</a>
          </div>
        </div>
      );
    }
    return (
      <div key={key} style={{ width: 280 }}>
        <a href={url} target="_blank" rel="noopener noreferrer"><PaperClipOutlined style={{ marginRight: 4 }} />{name} ↗</a>
      </div>
    );
  };

  // 从在线附件链接里找某张发票的原件 (按 fileId=invoiceId 或 invoiceCode=发票号 匹配)
  const findInvoiceFile = (row: any) => {
    const list = attachUrls?.items?.[0]?.attachmentList || [];
    for (const att of list) {
      for (const f of (att.invoiceUrls || [])) {
        if (
          (row.invoiceId && f.fileId === row.invoiceId) ||
          (row.invoiceNumber && (f.invoiceCode === row.invoiceNumber || f.invoiceNumber === row.invoiceNumber)) ||
          (row.invoiceCode && f.invoiceCode === row.invoiceCode)
        ) {
          return f;
        }
      }
    }
    return null;
  };

  // 按文件名/fileId 从在线附件里找文件 (付款截图等明细附件用)
  const resolveAttachFile = (nameOrId: string) => {
    const list = attachUrls?.items?.[0]?.attachmentList || [];
    for (const att of list) {
      for (const grp of [att.invoiceUrls, att.attachmentUrls, att.receiptUrls]) {
        for (const f of (grp || [])) {
          if (f.fileId === nameOrId || f.fileName === nameOrId) return f;
        }
      }
    }
    return null;
  };
  const openFilePreview = (f: any) => setInvoicePreview({ visible: true, file: f, title: f.fileName || f.invoiceCode || '附件' });

  const renderAttachments = () => {
    if (!attachUrls?.items?.[0]?.attachmentList) return <div style={{ color: '#999' }}>无附件</div>;
    const list = attachUrls.items[0].attachmentList;
    const typeLabels: Record<string, string> = {
      'flow.body': '单据附件',
      'flow.free': '费用明细附件',
      'flow.approving': '审批附件',
      'flow.receipt': '回单',
    };
    return list.map((att: any, idx: number) => {
      const files = [
        ...(att.invoiceUrls || []).map((f: any) => ({ f, kind: 'i', fallback: '发票' })),
        ...(att.attachmentUrls || []).map((f: any) => ({ f, kind: 'a', fallback: '附件' })),
        ...(att.receiptUrls || []).map((f: any) => ({ f, kind: 'r', fallback: '回单' })),
      ];
      if (files.length === 0) return null;
      return (
        <div key={idx} style={{ marginBottom: 20 }}>
          <h4 style={{ margin: '0 0 10px', color: '#06b6d4' }}>{typeLabels[att.type] || att.type}</h4>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 16 }}>
            {files.map(({ f, kind, fallback }, i: number) => renderFilePreview(f, `${idx}-${kind}-${i}`, fallback))}
          </div>
        </div>
      );
    });
  };

  return (
    <div>
      {/* v1.57.1 顶部工具栏: 上次同步时间 + 立即同步 + 看日志 */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8, marginBottom: 12 }}>
        <div style={{ color: '#64748b', fontSize: 13, display: 'flex', alignItems: 'center', gap: 6 }}>
          <ClockCircleOutlined />
          <span>
            上次同步时间:{' '}
            <Typography.Text strong>
              {lastSyncAt || '未知'}
            </Typography.Text>
            <Tooltip title="后台定时任务每 15 分钟自动同步合思一次; 想立即同步请点右侧按钮 (5-10 分钟)">
              <span style={{ marginLeft: 6, color: '#94a3b8', cursor: 'help' }}>?</span>
            </Tooltip>
          </span>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <Button icon={<FileTextOutlined />} onClick={openLogModal}>看同步日志</Button>
          <Button type="primary" icon={<SyncOutlined spin={syncing} />} loading={syncing} onClick={handleSync}>立即同步合思</Button>
        </div>
      </div>

      {/* KPI 卡片 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        {statCards.map((card) => (
          <Col xs={12} sm={6} lg={4} key={card.title}>
            <Card
              className="bi-stat-card"
              hoverable={!!card.onClick}
              onClick={card.onClick}
              style={{ ['--accent-color' as any]: card.accentColor, cursor: card.onClick ? 'pointer' : 'default' }}
            >
              <Statistic title={card.title} value={card.value} prefix={card.prefix} />
            </Card>
          </Col>
        ))}
      </Row>

      {/* 筛选栏 */}
      <Card className="bi-filter-card" size="small" style={{ marginBottom: 16 }}>
        <Row gutter={[12, 12]} align="middle">
          <Col>
            <Select value={formType} onChange={v => { setFormType(v); setPage(1); }} allowClear placeholder="单据类型" style={{ width: 120 }}>
              <Option value="expense">报销单</Option>
              <Option value="loan">借款单</Option>
              <Option value="requisition">申请单</Option>
              <Option value="custom">通用审批</Option>
            </Select>
          </Col>
          <Col>
            <Select value={state} onChange={v => { setState(v); setPage(1); }} allowClear placeholder="状态(默认未结束)" style={{ width: 140 }}>
              <Option value="active">未结束 (默认)</Option>
              <Option value="approving">审批中</Option>
              <Option value="paying">待支付</Option>
              <Option value="paid">已支付</Option>
              <Option value="archived">已归档</Option>
              <Option value="rejected">已驳回</Option>
              <Option value="terminal">已结束 (合并)</Option>
              <Option value="draft">草稿</Option>
            </Select>
          </Col>
          <Col>
            <Select value={invoiceStatus} onChange={v => { setInvoiceStatus(v); setPage(1); }} allowClear placeholder="发票状态" style={{ width: 110 }}>
              <Option value="noExist">未到票</Option>
              <Option value="exist">已到票</Option>
            </Select>
          </Col>
          <Col>
            <Select
              value={specificationId}
              onChange={v => { setSpecificationId(v); setPage(1); }}
              allowClear
              showSearch
              placeholder="单据模板"
              style={{ width: 180 }}
              filterOption={(input, opt) =>
                (opt?.label as string)?.toLowerCase().includes(input.toLowerCase())
              }
              options={specOptions.map(s => ({ value: s.id, label: s.name }))}
            />
          </Col>
          <Col>
            <Input prefix={<SearchOutlined />} placeholder="搜索编码/标题" value={keywordInput}
              onChange={e => setKeywordInput(e.target.value)}
              onPressEnter={triggerQuery}
              allowClear style={{ width: 200 }} />
          </Col>
          <Col>
            <Input prefix={<SearchOutlined />} placeholder="当前审批人" value={approverInput}
              onChange={e => setApproverInput(e.target.value)}
              onPressEnter={triggerQuery}
              allowClear style={{ width: 140 }} />
          </Col>
          <Col>
            <RangePicker value={dateRange as any}
              onChange={(v) => { setDateRange(v as any); setPage(1); }}
              style={{ width: 240 }} />
          </Col>
          <Col>
            <Button type="primary" icon={<SearchOutlined />} onClick={triggerQuery}>查询</Button>
          </Col>
          <Col>
            <Button icon={<ReloadOutlined />} onClick={resetFilters}>重置</Button>
          </Col>
        </Row>
      </Card>

      {/* 数据表格 */}
      <Card className="bi-table-card" size="small">
        <Table
          columns={columns}
          dataSource={flows}
          rowKey="flowId"
          loading={loading}
          size="small"
          scroll={{ x: 1400 }}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showQuickJumper: true,
            pageSizeOptions: ['20', '50', '100'],
            showTotal: (t) => `共 ${t} 条`,
            onChange: (p, ps) => { setPage(p); setPageSize(ps); },
          }}
        />
      </Card>

      {/* 详情弹窗 */}
      <Modal
        title={detailData?.flow?.code ? `${detailData.flow.code} - ${detailData.flow.title}` : '单据详情'}
        open={detailModal.visible}
        onCancel={() => {
          setDetailModal({ visible: false, flowId: '' });
          setDetailData(null);
          setAttachUrls(null);
        }}
        footer={null}
        width="90%"
        style={{ top: 24, maxWidth: 1600 }}
        styles={{ body: { maxHeight: 'calc(100vh - 140px)', overflowY: 'auto' } }}
        destroyOnHidden
      >
        {detailLoading ? <div style={{ textAlign: 'center', padding: 40 }}>加载中...</div> : detailData && (
          <Tabs defaultActiveKey="basic" items={[
            // v1.75.2: Tab 自适应显示规则 (基于 17 模板真实数据统计)
            // - 基本信息: 永远显
            // - 费用明细: count > 0 才显 (申请单/借款单/商城类几乎无明细)
            // - 发票: count > 0 才显, 例外: form_type=expense 即使 0 也显作业务异常警告
            // - 附件: count > 0 才显
            {
              key: 'basic',
              label: '基本信息',
              children: (
                <Descriptions bordered size="small" column={2} labelStyle={{ width: 140, whiteSpace: 'nowrap' }}>
                  <Descriptions.Item label="单据编码">{detailData.flow.code}</Descriptions.Item>
                  <Descriptions.Item label="单据类型">
                    {formTypeMap[detailData.flow.formType]?.label || detailData.flow.formType}
                  </Descriptions.Item>
                  <Descriptions.Item label="公司（法人实体）" span={2}>
                    {detailData.flow.legalEntityName
                      ? detailData.flow.legalEntityName
                      : detailData.flow.legalEntityId
                        ? <Typography.Text type="secondary">ID: {detailData.flow.legalEntityId}（字典未匹配）</Typography.Text>
                        : '-'}
                    {detailData.flow.entityCheck === 'ok' && (
                      <Tooltip title={detailData.flow.entityCheckReason || '跟钉钉花名册的合同公司一致'}>
                        <Tag color="success" icon={<CheckCircleOutlined />} style={{ marginLeft: 8, cursor: 'help' }}>
                          已核对
                        </Tag>
                      </Tooltip>
                    )}
                    {detailData.flow.entityCheck === 'mismatch' && (
                      <Tooltip title={detailData.flow.entityCheckReason || '与钉钉花名册不一致'}>
                        <Tag color="error" icon={<WarningOutlined />} style={{ marginLeft: 8, cursor: 'help' }}>
                          主体可能选错 · 应为 {detailData.flow.entityCheckExpected}
                        </Tag>
                      </Tooltip>
                    )}
                    {detailData.flow.entityCheck === 'no_data' && (
                      <Tooltip title={detailData.flow.entityCheckReason || '钉钉花名册无合同公司数据'}>
                        <Tag color="default" style={{ marginLeft: 8, cursor: 'help' }}>
                          无法核对
                        </Tag>
                      </Tooltip>
                    )}
                  </Descriptions.Item>
                  <Descriptions.Item label={
                    <Tooltip title="这笔费是谁的 / 谁来背 (单据所有者). 99% 跟提交人一样, 个别助理代提交时才会不同, 找费用责任人看这个.">
                      <span style={{ cursor: 'help' }}>发起人</span>
                    </Tooltip>
                  }>
                    {detailData.flow.ownerName || (detailData.flow.ownerId
                      ? <Typography.Text type="secondary">未匹配</Typography.Text>
                      : '-')}
                  </Descriptions.Item>
                  <Descriptions.Item label={
                    <Tooltip title="实际在合思系统里点'提交'的那个人. 出差报销有时候会助理代提交, 这时跟发起人不同; 找操作单据的人看这个.">
                      <span style={{ cursor: 'help' }}>提交人</span>
                    </Tooltip>
                  }>
                    {detailData.flow.submitterName || (detailData.flow.submitterId
                      ? <Typography.Text type="secondary">未匹配</Typography.Text>
                      : '-')}
                  </Descriptions.Item>
                  <Descriptions.Item label="发起人部门">
                    {detailData.flow.ownerDepartmentName || '-'}
                    {detailData.flow.ownerDepartmentCheck === 'non-leaf' && (
                      <Tooltip title={detailData.flow.ownerDepartmentCheckReason || '该部门有下级'}>
                        <Tag color="error" icon={<WarningOutlined />} style={{ marginLeft: 8, cursor: 'help' }}>
                          非末级部门
                        </Tag>
                      </Tooltip>
                    )}
                  </Descriptions.Item>
                  <Descriptions.Item label="报销/借款部门">
                    {detailData.flow.departmentName || '-'}
                    {detailData.flow.departmentCheck === 'non-leaf' && (
                      <Tooltip title={detailData.flow.departmentCheckReason || '该部门有下级'}>
                        <Tag color="error" icon={<WarningOutlined />} style={{ marginLeft: 8, cursor: 'help' }}>
                          非末级部门
                        </Tag>
                      </Tooltip>
                    )}
                  </Descriptions.Item>
                  <Descriptions.Item label="状态">
                    <Tag color={stateMap[detailData.flow.state]?.color}>
                      {stateMap[detailData.flow.state]?.label || detailData.flow.state}
                    </Tag>
                  </Descriptions.Item>
                  {/* v1.75.5: 4 个金额/凭证/支付字段按 form_type 自适应
                      - expense 类 (报销) 始终显示 (即使空, 作为业务异常警告)
                      - 其他类 (申请/借款/商城) 仅 value 非空才显示 */}
                  {(detailData.flow.formType === 'expense' || detailData.flow.payMoney) && (
                    <Descriptions.Item label="支付金额">
                      {detailData.flow.payMoney ? `¥${detailData.flow.payMoney.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}` : '-'}
                    </Descriptions.Item>
                  )}
                  {(detailData.flow.formType === 'expense' || detailData.flow.expenseMoney) && (
                    <Descriptions.Item label="报销金额">
                      {detailData.flow.expenseMoney ? `¥${detailData.flow.expenseMoney.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}` : '-'}
                    </Descriptions.Item>
                  )}
                  {(detailData.flow.formType === 'expense' || detailData.flow.voucherStatus) && (
                    <Descriptions.Item label={
                      <Tooltip title="财务做账状态. 已生成 = 合思生成会计凭证后自动同步到用友, 财务做完账; 未生成 = 还没做账(单据审批完了但财务那边还没记到账本).">
                        <span style={{ cursor: 'help' }}>凭证状态</span>
                      </Tooltip>
                    }>
                      {detailData.flow.voucherStatus || '-'}
                      {detailData.voucherDetail && (
                        <Button
                          type="link"
                          size="small"
                          style={{ marginLeft: 8, padding: 0, height: 'auto' }}
                          onClick={() => setVoucherModalOpen(true)}
                        >
                          查看凭证
                        </Button>
                      )}
                    </Descriptions.Item>
                  )}
                  <Descriptions.Item label="创建时间" contentStyle={{ whiteSpace: 'nowrap' }}>{formatTime(detailData.flow.createTime)}</Descriptions.Item>
                  <Descriptions.Item label="提交时间" contentStyle={{ whiteSpace: 'nowrap' }}>{formatTime(detailData.flow.submitDate)}</Descriptions.Item>
                  {(detailData.flow.formType === 'expense' || detailData.flow.payDate) && (
                    <Descriptions.Item label="支付时间" contentStyle={{ whiteSpace: 'nowrap' }}>{formatTime(detailData.flow.payDate)}</Descriptions.Item>
                  )}
                  <Descriptions.Item label="完成时间" contentStyle={{ whiteSpace: 'nowrap' }}>{formatTime(detailData.flow.flowEndTime)}</Descriptions.Item>
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
                        </Tooltip>
                      );
                    })()}
                  </Descriptions.Item>
                  {detailData.flow.payeeId && (
                    <>
                      <Descriptions.Item label="收款户名">{detailData.flow.payeeName || '-'}</Descriptions.Item>
                      <Descriptions.Item label="收款方式">
                        {(() => {
                          const m: Record<string, { label: string; color: string }> = {
                            BANK: { label: '银行账户', color: 'success' },
                            OVERSEABANK: { label: '海外银行', color: 'blue' },
                            ALIPAY: { label: '支付宝', color: 'warning' },
                            WALLET: { label: '微信/钉钉钱包', color: 'warning' },
                            CHECK: { label: '支票', color: 'default' },
                            ACCEPTANCEBILL: { label: '承兑汇票', color: 'default' },
                            OTHER: { label: '其他', color: 'warning' },
                          };
                          const s = detailData.flow.payeeSort;
                          const cfg = m[s] || { label: s || '-', color: 'default' };
                          return <Tag color={cfg.color}>{cfg.label}</Tag>;
                        })()}
                      </Descriptions.Item>
                      <Descriptions.Item label="开户行" span={2}>{detailData.flow.payeeBank || '-'}</Descriptions.Item>
                      <Descriptions.Item label="收款账号" span={2}>{detailData.flow.payeeCardNo || '-'}</Descriptions.Item>
                    </>
                  )}
                </Descriptions>
              ),
            },
            ...((detailData.details?.length || 0) > 0 ? [{
              key: 'details',
              label: `费用明细 (${detailData.details?.length || 0})`,
              children: (
                <Table
                  size="small"
                  dataSource={detailData.details || []}
                  rowKey={(r: any) => r.detailId || `${r.detailNo}-${r.amount}-${r.feeDate}`}
                  pagination={false}
                  scroll={{ y: 480 }}
                  sticky
                  expandable={{
                    expandedRowRender: (record: any) => renderHesiDetailExpand(record, { resolve: resolveAttachFile, preview: openFilePreview }),
                    rowExpandable: (r: any) => {
                      const form = r.rawJson?.feeTypeForm || r.rawJson;
                      if (!form || typeof form !== 'object') return false;
                      return Object.keys(form).some((k) => !HESI_DETAIL_HIDDEN_KEYS.has(k));
                    },
                  }}
                  columns={[
                    {
                      title: '行号', width: 60,
                      render: (_: any, __: any, idx: number) => idx + 1,
                    },
                    {
                      title: '费用类型', dataIndex: 'feeTypeName', width: 140, ellipsis: true,
                      render: (v: string) => v || <Typography.Text type="secondary">-</Typography.Text>,
                    },
                    {
                      title: '金额', dataIndex: 'amount', width: 120, align: 'right',
                      render: (v: number) => v ? `¥${v.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}` : '-',
                    },
                    {
                      title: '消费时间', dataIndex: 'feeDate', width: 140,
                      render: (v: number, row: any) => {
                        if (v) return dayjs(v).format('YYYY-MM-DD');
                        // 差旅出差补贴类: feeDate 空, 用 feeDatePeriod
                        const p = row.rawJson?.feeTypeForm?.feeDatePeriod;
                        if (p?.start && p?.end) return `${dayjs(p.start).format('MM-DD')} ~ ${dayjs(p.end).format('MM-DD')}`;
                        return '-';
                      },
                    },
                    {
                      title: '发票', dataIndex: 'invoiceStatus', width: 100,
                      render: (v: string, row: any) => {
                        const cnt = row.invoiceCount || 0;
                        return v === 'exist'
                          ? <Tag icon={<CheckCircleOutlined />} color="success">{cnt > 0 ? `${cnt} 张` : '有'}</Tag>
                          : <Tag icon={<WarningOutlined />} color="error">无</Tag>;
                      },
                    },
                    {
                      title: '附件', width: 80, align: 'center',
                      render: (_: any, row: any) => {
                        const att = row.rawJson?.feeTypeForm?.attachments || [];
                        return att.length > 0 ? <Tag color="blue">{att.length}</Tag> : <Typography.Text type="secondary">-</Typography.Text>;
                      },
                    },
                    { title: '消费原因', dataIndex: 'consumptionReasons', ellipsis: true },
                  ]}
                />
              ),
            }] : []),
            ...(((detailData.invoices?.length || 0) > 0 || detailData.flow.formType === 'expense') ? [{
              key: 'invoices',
              label: `发票 (${detailData.invoices?.length || 0})`,
              children: (
                <Table
                  size="small"
                  dataSource={detailData.invoices || []}
                  rowKey={(r: any) => r.invoiceId || r.invoiceNumber || `${r.invoiceCode}-${r.totalAmount}`}
                  pagination={false}
                  scroll={{ x: 1310 }}
                  columns={[
                    {
                      title: '所属费用', dataIndex: 'feeTypeName', width: 150, fixed: 'left',
                      filters: Array.from(new Set((detailData.invoices || []).map((i: any) => i.feeTypeName).filter(Boolean)))
                        .map((n: any) => ({ text: n, value: n })),
                      onFilter: (val: any, r: any) => r.feeTypeName === val,
                      render: (v: string, r: any) => v
                        ? <span>{r.detailNo ? <Tag>#{r.detailNo}</Tag> : null}{v}</span>
                        : <Typography.Text type="secondary">—</Typography.Text>,
                    },
                    {
                      title: '发票号码', dataIndex: 'invoiceNumber', width: 200,
                      render: (v: string, r: any) => {
                        if (!v) return <Tag color="warning">未识别</Tag>;
                        const file = findInvoiceFile(r);
                        return file
                          ? <a onClick={() => setInvoicePreview({ visible: true, file, title: v })}>{v}</a>
                          : v;
                      },
                    },
                    {
                      title: '发票日期', dataIndex: 'invoiceDate', width: 110,
                      render: (v: number) => v ? dayjs(v).format('YYYY-MM-DD') : '-',
                    },
                    {
                      title: '价税合计', dataIndex: 'totalAmount', width: 120, align: 'right',
                      render: (v: number, r: any) => {
                        if (v) return `¥${v.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}`;
                        if (r.detailAmount) return <Typography.Text type="secondary">¥{r.detailAmount.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</Typography.Text>;
                        return '-';
                      },
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
                          'ELECTRONIC_TRAIN_INVOICE': '电子火车票',
                        };
                        return m[v] || v || '-';
                      },
                    },
                    {
                      title: '出行/座位', dataIndex: 'seatType', width: 230, ellipsis: true,
                      render: (v: string, r: any) => {
                        if (!v && !r.trainNo) return '-';
                        const over = ['一等座', '商务座', '特等座', '一等卧', '优选一等座'].includes(v);
                        const review = ['软卧', '动卧', '高级软卧'].includes(v);
                        const route = [r.trainNo, (r.fromStation || r.toStation) ? `${r.fromStation || ''}→${r.toStation || ''}` : ''].filter(Boolean).join(' ');
                        const full = `${v || ''} ${route}${r.passenger ? ' · ' + r.passenger : ''}`.trim();
                        return (
                          <div title={full} style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                            {v && <Tag color={over ? 'error' : review ? 'warning' : 'success'} style={{ marginInlineEnd: 4 }}>{v}</Tag>}
                            {route && <Typography.Text type="secondary" style={{ fontSize: 12 }}>{route}</Typography.Text>}
                          </div>
                        );
                      },
                    },
                    {
                      title: '销售方/明细原因', dataIndex: 'sellerName', width: 200, ellipsis: true,
                      render: (v: string, r: any) => v || (r.detailReason ? <Typography.Text type="secondary">{r.detailReason}</Typography.Text> : '-'),
                    },
                    {
                      title: '验真', dataIndex: 'isVerified', width: 60, align: 'center',
                      render: (v: number) => v ? <Tag color="success">是</Tag> : <Tag>否</Tag>,
                    },
                  ]}
                />
              ),
            }] : []),
            {
              key: 'attachments',
              label: '发票原件 / 附件',
              children: (
                <div>
                  <Button
                    size="small" icon={<PaperClipOutlined />}
                    loading={attachLoading}
                    onClick={() => loadAttachUrls(detailModal.flowId)}
                    style={{ marginBottom: 16 }}
                  >
                    {attachUrls ? '刷新链接（1小时有效）' : '加载发票/附件'}
                  </Button>
                  {attachLoading && !attachUrls ? <div style={{ color: '#999' }}>正在加载发票/附件…</div> : attachUrls ? renderAttachments() : (
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

      {/* 单张发票原件预览弹窗 (发票 tab 行内"查看"触发) */}
      <Modal
        open={invoicePreview.visible}
        title={`发票原件 · ${invoicePreview.title}`}
        footer={null}
        width="70%"
        style={{ top: 24, maxWidth: 1100 }}
        zIndex={1100}
        onCancel={() => setInvoicePreview({ visible: false, file: null, title: '' })}
      >
        {invoicePreview.file && (() => {
          const f = invoicePreview.file;
          const url: string = f.url || '';
          const name: string = f.fileName || '';
          const lower = `${name} ${url}`.toLowerCase();
          const isImg = /\.(jpg|jpeg|png|gif|webp|bmp)(\?|#|$)/.test(lower);
          const isPdf = /\.pdf(\?|#|$)/.test(lower);
          if (isImg) return <div style={{ textAlign: 'center' }}><Image src={url} preview={false} style={{ maxWidth: '100%', maxHeight: '72vh' }} /></div>;
          if (isPdf) return (
            <div>
              <iframe src={`${url}#toolbar=0&navpanes=0&view=FitH`} title={name} style={{ width: '100%', height: 620, border: '1px solid #eee', borderRadius: 6 }} />
              <div style={{ marginTop: 8, textAlign: 'center' }}>
                <a href={url} target="_blank" rel="noopener noreferrer">PDF 显示不全？点此在新窗口打开 ↗</a>
              </div>
            </div>
          );
          return <a href={url} target="_blank" rel="noopener noreferrer">{name || '打开文件'} ↗</a>;
        })()}
      </Modal>

      {/* v1.75.8: 凭证明细子弹窗 (从详情 Modal 的"凭证状态"行的"查看凭证"按钮触发) */}
      <Modal
        title={detailData?.voucherDetail?.header?.displayname
          ? `凭证明细 - ${detailData.voucherDetail.header.displayname}`
          : '凭证明细'}
        open={voucherModalOpen}
        onCancel={() => setVoucherModalOpen(false)}
        footer={null}
        width={1400}
        destroyOnHidden
      >
        {detailData?.voucherDetail && (
          <div>
            <Descriptions bordered size="small" column={2} style={{ marginBottom: 12 }}>
              <Descriptions.Item label="凭证号">
                <Tag color="purple">{detailData.voucherDetail.header?.displayname || '-'}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="会计期间">{detailData.voucherDetail.header?.period || '-'}</Descriptions.Item>
              <Descriptions.Item label="账簿">{detailData.voucherDetail.header?.accbook?.name || '-'}</Descriptions.Item>
              <Descriptions.Item label="凭证类型">{detailData.voucherDetail.header?.vouchertype?.name || '-'}</Descriptions.Item>
              <Descriptions.Item label="制单人">{detailData.voucherDetail.header?.maker?.name || '-'}</Descriptions.Item>
              <Descriptions.Item label="制单日期">{detailData.voucherDetail.header?.maketime || '-'}</Descriptions.Item>
              <Descriptions.Item label="借方合计">
                <Typography.Text strong>{detailData.voucherDetail.header?.totaldebit_org != null
                  ? `¥${Number(detailData.voucherDetail.header.totaldebit_org).toLocaleString('zh-CN', { minimumFractionDigits: 2 })}`
                  : '-'}</Typography.Text>
              </Descriptions.Item>
              <Descriptions.Item label="贷方合计">
                <Typography.Text strong>{detailData.voucherDetail.header?.totalcredit_org != null
                  ? `¥${Number(detailData.voucherDetail.header.totalcredit_org).toLocaleString('zh-CN', { minimumFractionDigits: 2 })}`
                  : '-'}</Typography.Text>
              </Descriptions.Item>
            </Descriptions>
            <Table
              size="small"
              dataSource={detailData.voucherDetail.body || []}
              rowKey={(r: any, i: number = 0) => r.id || `${r.recordnumber}-${i}`}
              pagination={false}
              columns={[
                { title: '行', dataIndex: 'recordnumber', width: 50, align: 'center' },
                {
                  title: '摘要', dataIndex: 'description', width: 400, ellipsis: true,
                  render: (v: string) => v
                    ? <Tooltip title={v}><span style={{ cursor: 'help' }}>{v}</span></Tooltip>
                    : '-',
                },
                {
                  title: '科目', width: 260, ellipsis: true,
                  render: (_: any, row: any) => row.accsubject
                    ? <Tooltip title={`${row.accsubject.code} ${row.accsubject.name}`}>
                        <span style={{ cursor: 'help' }}>{row.accsubject.code} {row.accsubject.name}</span>
                      </Tooltip>
                    : '-',
                },
                {
                  title: '借方', dataIndex: 'debit_org', width: 120, align: 'right',
                  render: (v: number) => v ? `¥${Number(v).toLocaleString('zh-CN', { minimumFractionDigits: 2 })}` : '-',
                },
                {
                  title: '贷方', dataIndex: 'credit_org', width: 120, align: 'right',
                  render: (v: number) => v ? `¥${Number(v).toLocaleString('zh-CN', { minimumFractionDigits: 2 })}` : '-',
                },
                {
                  title: '辅助核算', dataIndex: 'auxiliaryShow', width: 320, ellipsis: true,
                  render: (v: string) => v
                    ? <Tooltip title={v}><span style={{ cursor: 'help' }}>{v}</span></Tooltip>
                    : <Typography.Text type="secondary">-</Typography.Text>,
                },
              ]}
            />
          </div>
        )}
      </Modal>

      {/* v1.57.1 同步日志 modal */}
      <Modal
        title="合思同步日志"
        open={logModalOpen}
        onCancel={closeLogModal}
        footer={[
          <Button key="refresh" icon={<ReloadOutlined />} onClick={fetchSyncLog}>立即刷新</Button>,
          <Button key="close" type="primary" onClick={closeLogModal}>关闭</Button>,
        ]}
        width={900}
      >
        <div style={{ color: '#666', fontSize: 12, marginBottom: 8 }}>
          每 3 秒自动刷新, 显示末尾 300 行 (来自 sync-hesi.log)
        </div>
        <pre
          ref={logBoxRef}
          style={{
            background: '#1e1e1e', color: '#d4d4d4', padding: 12, borderRadius: 6,
            height: 500, overflow: 'auto', fontSize: 12, margin: 0,
            fontFamily: 'Consolas, Monaco, monospace', whiteSpace: 'pre-wrap', wordBreak: 'break-all',
          }}
        >
          {logText || '加载中...'}
        </pre>
      </Modal>
    </div>
  );
};

export default ExpenseControl;
