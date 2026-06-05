// 合思单据详情 Modal — 费控管理 / 合思机器人 共享组件
//
// 为什么抽出来: 费控管理(finance/ExpenseControl) 和 合思机器人(system/HesiBot) 都要看同一份
// 合思单据详情(基本信息/费用明细/发票/附件/凭证). 以前两边各画一套, 改一边漏一边 →
// 跑哥要求"这两个一直保持一致". 抽成一个组件后, 详情长什么样只此一处, 两边引用, 永不漂移.
//
// 两个页面唯一的差别是后端接口路径(费控 /api/hesi/*, 机器人 /api/profile/*), 但返回数据结构
// 完全相同(机器人侧 GetMyHesiFlowDetail 鉴权后直接 delegate 给 GetHesiFlowDetail).
// 所以接口地址用 props 传进来即可.

import React, { useEffect, useState } from 'react';
import { Modal, Descriptions, Tabs, Tag, Table, Typography, Tooltip, Button, Image, message } from 'antd';
import { CheckCircleOutlined, WarningOutlined, PaperClipOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';

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

const formatTime = (ts: number | null) => {
  if (!ts) return '-';
  return dayjs(ts).format('YYYY-MM-DD HH:mm');
};

// 费用明细展开行 - 把合思 API 原始字段 (raw_json.feeTypeForm) 全部展示
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

export interface HesiFlowDetailModalProps {
  open: boolean;
  flowId: string;
  onClose: () => void;
  /** 单据详情接口完整 URL — 费控: `${API}/flow-detail?flowId=x`; 机器人: `${PROFILE_API}/hesi-flow-detail?flowId=x` */
  flowDetailUrl: (flowId: string) => string;
  /** 在线附件链接接口完整 URL — 费控: `${API}/attachment-urls?flowId=x`; 机器人: `${PROFILE_API}/hesi-attachment-urls?flowId=x` */
  attachmentUrlsUrl: (flowId: string) => string;
}

const HesiFlowDetailModal: React.FC<HesiFlowDetailModalProps> = ({ open, flowId, onClose, flowDetailUrl, attachmentUrlsUrl }) => {
  const [detailData, setDetailData] = useState<any>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [attachUrls, setAttachUrls] = useState<any>(null);
  const [attachLoading, setAttachLoading] = useState(false);
  // 单张发票原件预览弹窗
  const [invoicePreview, setInvoicePreview] = useState<{ visible: boolean; file: any; title: string }>({ visible: false, file: null, title: '' });
  // 凭证明细子弹窗
  const [voucherModalOpen, setVoucherModalOpen] = useState(false);

  const loadAttachUrls = async (fid: string) => {
    setAttachLoading(true);
    try {
      const res = await fetch(attachmentUrlsUrl(fid), { credentials: 'include' });
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

  // open + flowId 变化时拉详情, 并自动拉在线附件链接(同步的附件表可能为空, 以合思在线接口为准)
  useEffect(() => {
    if (!open || !flowId) {
      setDetailData(null);
      setAttachUrls(null);
      return;
    }
    let cancelled = false;
    setDetailData(null);
    setAttachUrls(null);
    setDetailLoading(true);
    (async () => {
      try {
        const res = await fetch(flowDetailUrl(flowId), { credentials: 'include' });
        const json = await res.json();
        if (cancelled) return;
        if (json.code === 200) {
          setDetailData(json.data);
          void loadAttachUrls(flowId);
        } else {
          message.error(json.msg || '获取单据详情失败');
        }
      } catch (e) {
        if (!cancelled) message.error('获取单据详情失败');
      } finally {
        if (!cancelled) setDetailLoading(false);
      }
    })();
    return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, flowId]);

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
          <iframe src={`${url}#toolbar=0&navpanes=0&view=Fit`} title={name} style={{ width: '100%', height: 360, border: '1px solid #eee', borderRadius: 6 }} />
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
    <>
      {/* 详情弹窗 */}
      <Modal
        title={detailData?.flow?.code ? `${detailData.flow.code} - ${detailData.flow.title}` : '单据详情'}
        open={open}
        onCancel={onClose}
        footer={null}
        width="90%"
        style={{ top: 24, maxWidth: 1600 }}
        styles={{ body: { maxHeight: 'calc(100vh - 140px)', overflowY: 'auto' } }}
        destroyOnHidden
      >
        {detailLoading ? <div style={{ textAlign: 'center', padding: 40 }}>加载中...</div> : detailData && (
          <Tabs defaultActiveKey="basic" items={[
            // Tab 自适应显示规则 (基于 17 模板真实数据统计)
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
                  {/* 4 个金额/凭证/支付字段按 form_type 自适应
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
                          'FULL_DIGITAl_PAPER': '全电纸质发票',
                          'FULL_DIGITAl_PAPER_NORMAL': '全电纸质普票',
                          'DIGITAL_NORMAL': '电子普通发票',
                          'SPECIAL_VAT': '增值税专票',
                          'NORMAL_VAT': '增值税普票',
                          'NORMAL_ELECTRONIC': '电子普票',
                          'SPECIAL_ELECTRONIC': '电子专票',
                          'PAPER_NORMAL': '纸质普票',
                          'PAPER_SPECIAL': '纸质专票',
                          'PAPER_FEE': '通行费发票',
                          'ELECTRONIC_PAPER_FEE': '通行费电子发票',
                          'ELECTRONIC_PAPER_CAR': '机动车销售发票(电子)',
                          'ELECTRONIC_TRAIN_INVOICE': '电子火车票',
                          'ELECTRONIC_AIRCRAFT_INVOICE': '电子机票行程单',
                          'BLOCK_CHAIN': '区块链电子发票',
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
                    onClick={() => loadAttachUrls(flowId)}
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
              <iframe src={`${url}#toolbar=0&navpanes=0&view=Fit`} title={name} style={{ width: '100%', height: '78vh', border: '1px solid #eee', borderRadius: 6 }} />
              <div style={{ marginTop: 8, textAlign: 'center' }}>
                <a href={url} target="_blank" rel="noopener noreferrer">PDF 显示不全？点此在新窗口打开 ↗</a>
              </div>
            </div>
          );
          return <a href={url} target="_blank" rel="noopener noreferrer">{name || '打开文件'} ↗</a>;
        })()}
      </Modal>

      {/* 凭证明细子弹窗 (从详情 Modal 的"凭证状态"行的"查看凭证"按钮触发) */}
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
    </>
  );
};

export default HesiFlowDetailModal;
