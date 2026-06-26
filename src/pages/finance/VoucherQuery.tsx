import React, { useEffect, useState, useCallback, useRef, useMemo } from 'react';
import { Card, Table, Select, DatePicker, InputNumber, Button, Space, Tag, message, Typography, Alert } from 'antd';
import { SearchOutlined } from '@ant-design/icons';
import dayjs, { Dayjs } from 'dayjs';
import { API_BASE } from '../../config';
import AccbookPicker from '../../components/AccbookPicker';
import { TableLayout, loadLayout, saveLayout, reorder, clampWidth } from './voucherTableLayout';

// 财务-凭证查询 (2026-06-16, 仅超管)
// 实时查用友 YS 凭证, 不入库。选账簿+账期查凭证头, 点开看分录明细(借贷)。
const { RangePicker } = DatePicker;
const { Text } = Typography;

interface Accbook { code: string; name: string; }
interface BookMeta { code: string; name: string; recordCount: number; fetched: number; error?: string; }
interface VoucherLine {
  recordNumber: string;
  description: string;
  subjectCode: string;
  subjectName: string;
  auxiliary: string;
  debit: number;
  credit: number;
}
interface VoucherRow {
  accbookCode: string;
  accbookName: string;
  id: string;
  period: string;
  voucherNo: string;
  voucherType: string;
  description: string;
  totalDebit: number;
  totalCredit: number;
  srcSystem: string;
  maker: string;
  auditor: string;
  tallyman: string;
  status: string;
  makeTime: string;
  attached: string;
  lines: VoucherLine[];
}

// 金额格式化: 0 显示空(像真实凭证借贷一侧留空), 非 0 千分位两位小数
const fmtAmount = (n: number, blankZero = false): string => {
  if (blankZero && (!n || n === 0)) return '';
  return (n || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
};

const statusColor = (s: string): string => {
  if (s === '已记账') return 'green';
  if (s === '保存') return 'gold';
  return 'default';
};

const VOUCHER_LAYOUT_KEY = 'bi_voucher_table_layout_v1';

// 凭证表格列定义（静态；列宽/顺序在运行时由 layout 覆盖）
const VOUCHER_BASE_COLUMNS = [
  { title: '账簿', key: 'accbookName', dataIndex: 'accbookName', width: 160, ellipsis: true, render: (v: string) => v || '-' },
  { title: '账期', key: 'period', dataIndex: 'period', width: 90 },
  { title: '凭证字号', key: 'voucherNo', dataIndex: 'voucherNo', width: 100 },
  { title: '类型', key: 'voucherType', dataIndex: 'voucherType', width: 100 },
  { title: '摘要', key: 'description', dataIndex: 'description', width: 280, ellipsis: true },
  { title: '借方合计', key: 'totalDebit', dataIndex: 'totalDebit', width: 130, align: 'right' as const, render: (v: number) => fmtAmount(v) },
  { title: '贷方合计', key: 'totalCredit', dataIndex: 'totalCredit', width: 130, align: 'right' as const, render: (v: number) => fmtAmount(v) },
  { title: '来源', key: 'srcSystem', dataIndex: 'srcSystem', width: 100 },
  { title: '制单人', key: 'maker', dataIndex: 'maker', width: 90 },
  { title: '状态', key: 'status', dataIndex: 'status', width: 90, render: (s: string) => (s ? <Tag color={statusColor(s)}>{s}</Tag> : '-') },
  { title: '制单日期', key: 'makeTime', dataIndex: 'makeTime', width: 110 },
];
const VOUCHER_COL_KEYS = VOUCHER_BASE_COLUMNS.map((c) => c.key);
const VOUCHER_DEFAULT_WIDTHS: Record<string, number> = Object.fromEntries(
  VOUCHER_BASE_COLUMNS.map((c) => [c.key, c.width]),
);

// 可拖宽 + 可拖序的表头单元格（props 由 column.onHeaderCell 注入）
const ResizableTitle: React.FC<any> = (props) => {
  const { columnKey, colWidth, onColResize, onColReorder, dragKeyRef, children, style, ...rest } = props;
  const resizingRef = useRef(false);

  // 没有 columnKey 的列（如展开图标列）按普通表头渲染
  if (!columnKey) return <th {...rest} style={style}>{children}</th>;

  const startResize = (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    resizingRef.current = true;
    const startX = e.clientX;
    const startW = colWidth || 100;
    const onMove = (ev: MouseEvent) => onColResize(columnKey, startW + (ev.clientX - startX));
    const onUp = () => {
      resizingRef.current = false;
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
    };
    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
  };

  return (
    <th
      {...rest}
      style={{ ...style, position: 'relative' }}
      draggable
      onDragStart={(e) => {
        if (resizingRef.current) { e.preventDefault(); return; }
        dragKeyRef.current = columnKey;
        e.dataTransfer.effectAllowed = 'move';
      }}
      onDragOver={(e) => e.preventDefault()}
      onDrop={(e) => {
        e.preventDefault();
        const from = dragKeyRef.current;
        if (from && from !== columnKey) onColReorder(from, columnKey);
        dragKeyRef.current = null;
      }}
    >
      {children}
      <span
        onMouseDown={startResize}
        onClick={(e) => e.stopPropagation()}
        style={{ position: 'absolute', right: -4, top: 0, height: '100%', width: 8, cursor: 'col-resize', userSelect: 'none', zIndex: 1 }}
      />
    </th>
  );
};

const VoucherQuery: React.FC = () => {
  const abortRef = useRef<AbortController | null>(null);

  const [accbooks, setAccbooks] = useState<Accbook[]>([]);
  const [accbookCodes, setAccbookCodes] = useState<string[]>([]);
  const [period, setPeriod] = useState<[Dayjs, Dayjs]>([dayjs(), dayjs()]);
  const [status, setStatus] = useState<string>('');
  const [billMin, setBillMin] = useState<number | null>(null);
  const [billMax, setBillMax] = useState<number | null>(null);

  const [rows, setRows] = useState<VoucherRow[]>([]);
  const [truncated, setTruncated] = useState(false);
  const [pageIndex, setPageIndex] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [loading, setLoading] = useState(false);
  const [queried, setQueried] = useState(false);

  // 表格列布局（可拖宽 + 可拖序，持久化到 localStorage，每浏览器各自）
  const dragKeyRef = useRef<string | null>(null);
  const [layout, setLayout] = useState<TableLayout>(() =>
    loadLayout(VOUCHER_LAYOUT_KEY, VOUCHER_COL_KEYS, VOUCHER_DEFAULT_WIDTHS));
  useEffect(() => { saveLayout(VOUCHER_LAYOUT_KEY, layout); }, [layout]);
  const handleColResize = useCallback((key: string, w: number) => {
    setLayout((prev) => ({ ...prev, widths: { ...prev.widths, [key]: clampWidth(w) } }));
  }, []);
  const handleColReorder = useCallback((fromKey: string, toKey: string) => {
    setLayout((prev) => ({
      ...prev,
      order: reorder(prev.order, prev.order.indexOf(fromKey), prev.order.indexOf(toKey)),
    }));
  }, []);
  const resetLayout = () => setLayout({ order: VOUCHER_COL_KEYS.slice(), widths: { ...VOUCHER_DEFAULT_WIDTHS } });

  // 加载账簿下拉
  useEffect(() => {
    fetch(`${API_BASE}/api/finance/voucher/accbooks`, { credentials: 'include' })
      .then(res => res.json())
      .then(res => {
        const list: Accbook[] = res.data || [];
        setAccbooks(list);
        // 默认选"浙江松鲜鲜"账簿, 找不到就第一个
        const def = list.find(a => a.name.includes('浙江松鲜鲜自然调味品')) || list[0];
        if (def) setAccbookCodes([def.code]);
      })
      .catch(() => message.error('账簿加载失败, 请确认用友连接正常'));
  }, []);

  const fetchVouchers = useCallback((full: boolean) => {
    if (!accbookCodes.length) { message.warning('请先选择账簿'); return; }
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    fetch(`${API_BASE}/api/finance/voucher/list`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      signal: ctrl.signal,
      body: JSON.stringify({
        accbookCodes,
        periodStart: period[0].format('YYYY-MM'),
        periodEnd: period[1].format('YYYY-MM'),
        voucherStatus: status,
        billcodeMin: billMin || 0,
        billcodeMax: billMax || 0,
        full,
      }),
    })
      .then(res => res.json())
      .then(res => {
        if (res.code !== 0 && res.code !== 200) {
          message.error(res.message || '查询失败');
          setRows([]); setTruncated(false); setLoading(false); return;
        }
        const d = res.data || {};
        setRows(d.list || []);
        setTruncated(!!d.truncated);
        setPageIndex(1);
        setLoading(false);
        setQueried(true);
        // 单本账失败提示
        (d.books || []).filter((b: BookMeta) => b.error).forEach((b: BookMeta) =>
          message.warning(`账簿「${b.name}」查询失败:${b.error}`));
      })
      .catch((e: any) => {
        if (e?.name !== 'AbortError') { message.error('查询失败'); setLoading(false); }
      });
  }, [accbookCodes, period, status, billMin, billMax]);

  // 点查询直接拉全部 (full=true): 逐本翻页拉到底, 受后端兜底闸 voucherMaxCalls/voucherMaxRows 约束
  const onSearch = () => fetchVouchers(true);

  // 按 layout（顺序 + 宽度）派生当前列；onHeaderCell 注入拖宽/拖序所需 props
  const orderedColumns = useMemo(() => {
    const byKey: Record<string, any> = {};
    VOUCHER_BASE_COLUMNS.forEach((c) => { byKey[c.key] = c; });
    return layout.order.map((k) => byKey[k]).filter(Boolean).map((col) => {
      const w = layout.widths[col.key] ?? col.width;
      return {
        ...col,
        width: w,
        onHeaderCell: () => ({
          columnKey: col.key,
          colWidth: w,
          onColResize: handleColResize,
          onColReorder: handleColReorder,
          dragKeyRef,
        }),
      };
    });
  }, [layout, handleColResize, handleColReorder]);
  // 各列宽之和 → 横滚宽度（列固定宽后需要横滚）
  const totalWidth = useMemo(
    () => layout.order.reduce((s, k) => s + (layout.widths[k] || 0), 0),
    [layout],
  );

  const lineColumns = [
    { title: '行号', dataIndex: 'recordNumber', width: 60 },
    { title: '摘要', dataIndex: 'description', ellipsis: true },
    {
      title: '科目', key: 'subject', width: 280,
      render: (_: any, l: VoucherLine) => (
        <span>{l.subjectCode ? <Text type="secondary">{l.subjectCode}</Text> : null} {l.subjectName}</span>
      ),
    },
    { title: '辅助核算', dataIndex: 'auxiliary', width: 220, ellipsis: true, render: (v: string) => v || '-' },
    {
      title: '借方金额', dataIndex: 'debit', width: 130, align: 'right' as const,
      render: (v: number) => fmtAmount(v, true),
    },
    {
      title: '贷方金额', dataIndex: 'credit', width: 130, align: 'right' as const,
      render: (v: number) => fmtAmount(v, true),
    },
  ];

  return (
    <Card title="凭证查询" extra={<Tag color="purple">仅超管</Tag>}>
      <div style={{ marginBottom: 16 }}>
        <Space size="middle" wrap>
          <span>
            账簿：
            <AccbookPicker
              books={accbooks}
              value={accbookCodes}
              onChange={setAccbookCodes}
              style={{ minWidth: 320, maxWidth: 560 }}
            />
          </span>
          <span>
            账期：
            <RangePicker
              picker="month"
              value={period}
              allowClear={false}
              onChange={(v) => { if (v && v[0] && v[1]) setPeriod([v[0], v[1]]); }}
            />
          </span>
          <span>
            状态：
            <Select
              style={{ width: 120 }}
              value={status}
              onChange={setStatus}
              options={[
                { label: '全部', value: '' },
                { label: '保存', value: '01' },
                { label: '已记账', value: '04' },
              ]}
            />
          </span>
          <span>
            凭证号：
            <InputNumber placeholder="起" min={1} style={{ width: 80 }} value={billMin} onChange={setBillMin} />
            <span style={{ margin: '0 4px' }}>~</span>
            <InputNumber placeholder="止" min={1} style={{ width: 80 }} value={billMax} onChange={setBillMax} />
          </span>
          <Button type="primary" icon={<SearchOutlined />} onClick={onSearch} loading={loading}>
            查询
          </Button>
          <Button onClick={resetLayout}>恢复默认列</Button>
        </Space>
      </div>

      {queried && truncated && (
        <Alert
          style={{ marginBottom: 12 }}
          type="warning"
          showIcon
          message="结果太多已截断,请缩小账期、减少账簿或缩小凭证号范围后重试"
        />
      )}

      <Table<VoucherRow>
        rowKey={(r) => `${r.accbookCode}-${r.id}`}
        size="small"
        loading={loading}
        columns={orderedColumns as any}
        components={{ header: { cell: ResizableTitle } }}
        dataSource={rows}
        locale={{ emptyText: queried ? '该条件下没有凭证' : '请选择账簿和账期后点查询' }}
        scroll={{ x: totalWidth }}
        expandable={{
          expandedRowRender: (record) => (
            <Table<VoucherLine>
              rowKey={(l) => `${record.id}-${l.recordNumber}`}
              size="small"
              columns={lineColumns}
              dataSource={record.lines}
              pagination={false}
              scroll={{ x: 900 }}
            />
          ),
          rowExpandable: (record) => (record.lines?.length || 0) > 0,
        }}
        pagination={{
          current: pageIndex,
          pageSize,
          showSizeChanger: true,
          pageSizeOptions: ['20', '50', '100'],
          showTotal: (t) => `共 ${t} 张凭证`,
          onChange: (p, ps) => { setPageIndex(p); setPageSize(ps); },
        }}
      />
    </Card>
  );
};

export default VoucherQuery;
