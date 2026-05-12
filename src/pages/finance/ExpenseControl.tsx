import React, { useState, useEffect, useCallback } from 'react';
import { Card, Table, Tag, Select, Input, DatePicker, Row, Col, Statistic, Button, Modal, Descriptions, Tabs, Badge, Tooltip, message } from 'antd';
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

const ExpenseControl: React.FC = () => {
  const [stats, setStats] = useState<StatsData | null>(null);
  const [flows, setFlows] = useState<FlowItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [formType, setFormType] = useState<string | undefined>(undefined);
  const [state, setState] = useState<string | undefined>(undefined);
  const [invoiceStatus, setInvoiceStatus] = useState<string | undefined>(undefined);
  const [keyword, setKeyword] = useState('');
  const [approver, setApprover] = useState(''); // v1.58.2: 当前审批人 LIKE 搜
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null] | null>(null);
  const [detailModal, setDetailModal] = useState<{ visible: boolean; flowId: string }>({ visible: false, flowId: '' });
  const [detailData, setDetailData] = useState<any>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [attachUrls, setAttachUrls] = useState<any>(null);
  const [attachLoading, setAttachLoading] = useState(false);

  const fetchStats = useCallback(async () => {
    try {
      const res = await fetch(`${API}/stats`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200) setStats(json.data);
    } catch (e) { /* ignore */ }
  }, []);

  const fetchFlows = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams();
      params.set('page', String(page));
      params.set('pageSize', String(pageSize));
      if (formType) params.set('formType', formType);
      if (state) params.set('state', state);
      if (invoiceStatus) params.set('invoiceStatus', invoiceStatus);
      if (keyword) params.set('keyword', keyword);
      if (approver) params.set('approver', approver);
      if (dateRange?.[0]) params.set('startDate', dateRange[0].format('YYYY-MM-DD'));
      if (dateRange?.[1]) params.set('endDate', dateRange[1].format('YYYY-MM-DD'));

      const res = await fetch(`${API}/flows?${params}`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200) {
        setFlows(json.data.items || []);
        setTotal(json.data.total || 0);
      }
    } catch (e) {
      message.error('获取单据列表失败');
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, formType, state, invoiceStatus, keyword, approver, dateRange]);

  useEffect(() => { fetchStats(); }, [fetchStats]);
  useEffect(() => { fetchFlows(); }, [fetchFlows]);

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
    setPage(1);
    if (page === 1) {
      void fetchFlows();
    }
  }, [fetchFlows, page]);

  const resetFilters = useCallback(() => {
    const noFilter = !formType && !state && !invoiceStatus && !keyword && !approver && !dateRange;
    setFormType(undefined);
    setState(undefined);
    setInvoiceStatus(undefined);
    setKeyword('');
    setApprover('');
    setDateRange(null);
    setPage(1);
    if (page === 1 && noFilter) {
      void fetchFlows();
    }
    void fetchStats();
  }, [dateRange, fetchFlows, fetchStats, formType, invoiceStatus, keyword, approver, page, state]);

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

  const quickFilter = (f: string, s: string, inv: string) => {
    setFormType(f || undefined);
    setState(s || undefined);
    setInvoiceStatus(inv || undefined);
    setPage(1);
  };
  const statCards = [
    { title: '单据总数', value: stats?.totalFlows || 0, prefix: <FileTextOutlined />, accentColor: '#1e40af', onClick: () => quickFilter('', '', '') },
    { title: '报销单', value: stats?.totalExpense || 0, prefix: <DollarOutlined />, accentColor: '#10b981', onClick: () => quickFilter('expense', '', '') },
    { title: '款付票未到', value: stats?.paidNoInvoice || 0, prefix: <WarningOutlined />, accentColor: '#ef4444', onClick: () => quickFilter('', 'paid', 'noExist') },
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
      title: '状态', dataIndex: 'state', width: 90,
      render: (v) => {
        const m = stateMap[v];
        return m ? <Tag color={m.color}>{m.label}</Tag> : v;
      },
    },
    {
      title: '当前审批', width: 160,
      render: (_, record) => {
        // v1.58.0: 优先显示真实审批人 (合思 approveStates 接口)
        if (record.currentApproverName && record.currentStageName) {
          return (
            <Tooltip title={`节点: ${record.currentStageName}  审批人: ${record.currentApproverName}${record.currentApproverCode ? ' (' + record.currentApproverCode + ')' : ''}`}>
              <div><strong>{record.currentApproverName}</strong></div>
              <div style={{fontSize:11, color:'#666'}}>{record.currentStageName}</div>
            </Tooltip>
          );
        }
        // fallback: 已结束单据 (合思接口不返审批人) → 显示上一步
        if (record.state === 'paid' || record.state === 'archived' || record.state === 'rejected') {
          return record.preApprovedNode
            ? <Tooltip title={`已结束 (上一步: ${record.preApprovedNode})`}><span style={{color:'#999'}}>已结束</span></Tooltip>
            : <span style={{color:'#999'}}>-</span>;
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

  return (
    <div>
      {/* v1.57.1 顶部工具栏: 立即同步 + 看日志 */}
      <div style={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center', gap: 8, marginBottom: 12 }}>
        <Button icon={<FileTextOutlined />} onClick={openLogModal}>看同步日志</Button>
        <Button type="primary" icon={<SyncOutlined spin={syncing} />} loading={syncing} onClick={handleSync}>立即同步合思</Button>
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
            <Select value={state} onChange={v => { setState(v); setPage(1); }} allowClear placeholder="状态" style={{ width: 110 }}>
              <Option value="paid">已支付</Option>
              <Option value="archived">已归档</Option>
              <Option value="approving">审批中</Option>
              <Option value="paying">待支付</Option>
              <Option value="rejected">已驳回</Option>
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
            <Input prefix={<SearchOutlined />} placeholder="搜索编码/标题" value={keyword}
              onChange={e => setKeyword(e.target.value)}
              onPressEnter={triggerQuery}
              allowClear style={{ width: 200 }} />
          </Col>
          <Col>
            <Input prefix={<SearchOutlined />} placeholder="当前审批人" value={approver}
              onChange={e => setApprover(e.target.value)}
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
