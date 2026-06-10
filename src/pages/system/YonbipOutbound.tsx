import React, { useState, useEffect, useMemo } from 'react';
import {
  Card, Input, DatePicker, Button, Table, Tag, Space, Alert, Typography, message, Modal, Tabs, Row, Col, Progress,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import dayjs, { Dayjs } from 'dayjs';
import { API_BASE } from '../../config';

const { TextArea } = Input;
const { Text, Paragraph } = Typography;

// 与后端 ybRow 对齐 (handler/yonbip_outbound.go)
interface YbRow {
  product_code: string;
  qty: string;
  target_batch: string;
  bustype?: string;
  bill_no: string;
  category: string;
  warehouse_name: string;
}
interface YbConvSource { from_batch: string; qty: number; stockStatusDoc?: string; }

// 库存状态 doc → 名字(与后端 ybStatusPriority 对齐)。出库一律合格, 不合格/废品要状态转换。
const YB_STATUS_NAME: Record<string, string> = {
  '2448706971278246078': '合格',
  '2448706971278246081': '不合格',
  '2448706971278246082': '废品',
};
const YB_QUALIFIED_DOC = '2448706971278246078';
interface YbShipment {
  org_name: string;
  warehouse_name: string;
  qty: number;
  convert_qty: number;
  convert_sources: YbConvSource[];
  out_batch: string;
}
interface YbPlan {
  row: YbRow;
  product_name: string;
  needed_qty: number;
  fulfilled_qty: number;
  remaining_qty: number;
  shipments: YbShipment[];
  bill_short?: boolean; // 所在单号缺货 → 整单不出
}
interface YbConvLog { from_batch: string; qty: number; doc_code: string; audit_ok: boolean; skipped?: boolean; uncertain?: boolean; error?: string; }
interface YbShipLog {
  org_name: string;
  warehouse_name: string;
  qty: number;
  conversions: YbConvLog[];
  out_doc_code: string;
  audit_ok: boolean;
  out_skipped?: boolean; // 防重: 同一张出库单10分钟内已提交过, 本次跳过
  uncertain?: boolean; // 出库单保存时网络中断没拿到应答, 单据可能已建成, 重做前必须核对
  bill_short?: boolean; // 所在单据缺货, 整单不出
  error?: string;
}
interface YbResult {
  row: YbRow;
  needed_qty: number;
  fulfilled_qty: number;
  remaining_qty: number;
  shipments: YbShipLog[];
}

// 平铺粘贴解析: 每行 6 列「单号 出库类型 仓库 货品编码 数量 批次」(Tab 或 2+ 空格分隔, 批次可留空)
function parseFlat(text: string): { rows: YbRow[]; bad: number } {
  const rows: YbRow[] = [];
  let bad = 0;
  for (const raw of text.split('\n')) {
    const line = raw.replace(/\r$/, '').trim();
    if (!line) continue;
    let parts = line.split(/\t+/);
    if (parts.length < 5) parts = line.split(/\s{2,}/);
    if (parts.length < 5) { bad++; continue; }
    const bill_no = (parts[0] || '').trim();
    const category = (parts[1] || '').trim();
    const warehouse_name = (parts[2] || '').trim();
    const product_code = (parts[3] || '').trim();
    const qty = (parts[4] || '').trim();
    const target_batch = (parts[5] || '').trim();
    if (!bill_no || !product_code || !qty) { bad++; continue; }
    rows.push({ bill_no, category, warehouse_name, product_code, qty, target_batch });
  }
  return { rows, bad };
}

// 分组录入: 一张吉客云单 = 单据头(单号 出库类型 仓库) + 明细(货品编码 数量 批次 多行)
interface YbGroup { id: number; head: string; details: string; }

function parseHeadLine(text: string): { bill_no: string; category: string; warehouse_name: string } | null {
  for (const raw of text.split('\n')) {
    const line = raw.replace(/\r$/, '').trim();
    if (!line) continue;
    let parts = line.split('\t');
    if (parts.length < 3) parts = line.split(/\s{2,}/);
    if (parts.length < 3) return null;
    return { bill_no: parts[0].trim(), category: parts[1].trim(), warehouse_name: parts.slice(2).join(' ').trim() };
  }
  return null;
}

function parseGroups(groups: YbGroup[]): { rows: YbRow[]; bad: number } {
  const rows: YbRow[] = [];
  let bad = 0;
  for (const g of groups) {
    const headText = g.head.trim();
    const detailText = g.details.trim();
    if (!headText && !detailText) continue;
    const head = headText ? parseHeadLine(headText) : null;
    if (!head || !head.bill_no || !head.warehouse_name || !detailText) { bad++; continue; }
    for (const raw of detailText.split('\n')) {
      const line = raw.replace(/\r$/, '').trim();
      if (!line) continue;
      let parts = line.split(/\t+/);
      if (parts.length < 2) parts = line.split(/\s{2,}/);
      if (parts.length < 2) parts = line.split(/\s+/);
      if (parts.length < 2) { bad++; continue; }
      const product_code = parts[0].trim();
      const qty = parts[1].trim();
      const target_batch = (parts[2] || '').trim();
      if (!product_code || !qty) { bad++; continue; }
      rows.push({ bill_no: head.bill_no, category: head.category, warehouse_name: head.warehouse_name, product_code, qty, target_batch });
    }
  }
  return { rows, bad };
}

// 草稿持久化: BI 看板切换标签页会卸载并重挂当前页, state 会丢。
// 把录入(flat/groups/mode/单据日期)+计划+结果存 sessionStorage, 切回来自动恢复, 别让跑哥白粘。
const DRAFT_KEY = 'yonbip_outbound_draft_v1';
const loadDraft = (): Record<string, unknown> => {
  try { return JSON.parse(sessionStorage.getItem(DRAFT_KEY) || '{}'); } catch { return {}; }
};

const YonbipOutboundPage: React.FC = () => {
  const draft0 = loadDraft();
  const [vouchdate, setVouchdate] = useState<Dayjs>(draft0.vouchdate ? dayjs(draft0.vouchdate as string) : dayjs());
  const [flat, setFlat] = useState((draft0.flat as string) || '');
  const [plans, setPlans] = useState<YbPlan[] | null>((draft0.plans as YbPlan[]) ?? null);
  const [results, setResults] = useState<YbResult[] | null>((draft0.results as YbResult[]) ?? null); // ②出库结果
  const [convResults, setConvResults] = useState<YbResult[] | null>((draft0.convResults as YbResult[]) ?? null); // ①批次转换结果
  const [planning, setPlanning] = useState(false);
  const [executing, setExecuting] = useState(false);
  const [progress, setProgress] = useState<{ done: number; total: number; label: string } | null>(null); // 执行实时进度(SSE)
  const [mode, setMode] = useState<'flat' | 'grouped'>((draft0.mode as 'flat' | 'grouped') || 'flat');
  const [groups, setGroups] = useState<YbGroup[]>((draft0.groups as YbGroup[]) || [{ id: 1, head: '', details: '' }]);

  // 录入/计划/结果一变就写回 sessionStorage(执行态 executing/progress 不存)。
  useEffect(() => {
    try {
      sessionStorage.setItem(DRAFT_KEY, JSON.stringify({
        flat, mode, groups, plans, results, convResults,
        vouchdate: vouchdate.format('YYYY-MM-DD'),
      }));
    } catch { /* 配额满等忽略, 不影响功能 */ }
  }, [flat, mode, groups, plans, results, convResults, vouchdate]);

  // 改了录入(=开始新一批)就清掉上一批的转换/出库结果; 复查(不改录入只点刷新)则保留。
  // setState 同为 null 时 React 自动 bail out, 频繁调用无害。
  const clearResults = () => { setConvResults(null); setResults(null); };

  const addGroup = () => { clearResults(); setGroups((gs) => [...gs, { id: Math.max(0, ...gs.map((x) => x.id)) + 1, head: '', details: '' }]); };
  const removeGroup = (id: number) => { clearResults(); setGroups((gs) => {
    const n = gs.filter((x) => x.id !== id);
    return n.length ? n : [{ id: 1, head: '', details: '' }];
  }); };
  const updateGroup = (id: number, field: 'head' | 'details', val: string) => {
    clearResults();
    setGroups((gs) => gs.map((x) => (x.id === id ? { ...x, [field]: val } : x)));
  };

  const billCount = (rs: { bill_no: string }[]) => new Set(rs.map((r) => r.bill_no)).size;

  const handlePlan = async () => {
    const { rows, bad } = mode === 'flat' ? parseFlat(flat) : parseGroups(groups);
    if (rows.length === 0) {
      message.error('没解析到有效行。每行 6 列：单号 出库类型 仓库 货品编码 数量 批次（批次可留空）');
      return;
    }
    setPlanning(true);
    setResults(null);
    setPlans(null);
    // 后端逐个编码 × 3 组织查用友库存, 每次调用受 1.1s 限流 → 量大时是分钟级, 给个预估别让人干等
    const distinctCodes = new Set(rows.map((r) => r.product_code)).size;
    const estSec = Math.ceil(distinctCodes * 3 * 1.4);
    const estText = estSec >= 90 ? `约 ${Math.ceil(estSec / 60)} 分钟` : `约 ${Math.max(estSec, 5)} 秒`;
    message.open({ key: 'yb-plan', type: 'loading', content: `正在查用友库存拆单（${distinctCodes} 个编码，预计${estText}），请别关页面…`, duration: 0 });
    try {
      const res = await fetch(`${API_BASE}/api/yonbip/export-plan`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ rows }),
      });
      const json = await res.json();
      if (res.ok && json.data?.plans) {
        setPlans(json.data.plans);
        message.success(`拆单完成：${billCount(rows)} 张吉客云单，${rows.length} 行${bad ? `（跳过 ${bad} 行）` : ''}`);
      } else {
        message.error(json.msg || json.error || '生成拆单计划失败');
      }
    } catch (e) {
      message.error('网络错误，生成拆单计划失败');
    } finally {
      message.destroy('yb-plan');
      setPlanning(false);
    }
  };

  // phase='convert' 只做批次转换; phase='out' 只做出库。中间靠【刷新计划】复查目标批次是否到货。
  // 走 SSE 流式(?stream=1): 后端每处理一笔推一条进度, 前端实时显示进度条; 最后一条 result 拿结果。
  const doExecute = async (phase: 'convert' | 'out') => {
    if (!plans) return;
    setExecuting(true);
    setProgress({ done: 0, total: 0, label: '连接中…' });
    try {
      const res = await fetch(`${API_BASE}/api/yonbip/export-execute?stream=1`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({
          vouchdate: vouchdate.format('YYYY-MM-DD'),
          plans,
          group_by_bill: true,
          phase,
        }),
      });
      if (!res.ok || !res.body) {
        Modal.warning({
          title: '结果未知，请去用友核对',
          content: '服务器没返回正常结果，但本次可能已部分提交到用友。请先去用友核对，不要凭“失败”重复手工建单；如确需补提交，10分钟内系统会自动跳过已提交的。',
          okText: '我知道了',
        });
        return;
      }
      // 读 SSE: progress 事件实时更新进度, result 事件拿最终结果
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buf = '';
      let rs: YbResult[] | null = null;
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += decoder.decode(value, { stream: true });
        let sep: number;
        while ((sep = buf.indexOf('\n\n')) >= 0) {
          const block = buf.slice(0, sep);
          buf = buf.slice(sep + 2);
          let ev = 'message';
          let data = '';
          block.split('\n').forEach((ln) => {
            if (ln.startsWith('event:')) ev = ln.slice(6).trim();
            else if (ln.startsWith('data:')) data += ln.slice(5).trim();
          });
          if (!data) continue;
          if (ev === 'progress') { try { setProgress(JSON.parse(data)); } catch { /* 忽略半包 */ } }
          else if (ev === 'result') { try { rs = (JSON.parse(data).results ?? []) as YbResult[]; } catch { /* 忽略 */ } }
        }
      }
      if (!rs) {
        Modal.warning({
          title: '结果未知，请去用友核对',
          content: '没收到完整执行结果，但本次可能已部分提交到用友。请先去用友核对，不要重复手工建单；如确需补提交，10分钟内系统会自动跳过已提交的。',
          okText: '我知道了',
        });
        return;
      }
      {
        if (phase === 'convert') {
          // 第①步: 批次转换。不清计划——接着要点【刷新计划】复查。
          setConvResults(rs);
          const convs = rs.flatMap((r) => r.shipments).flatMap((s) => s.conversions);
          const cUncertain = convs.filter((c) => c.uncertain).length;
          const cFail = convs.filter((c) => c.error && !c.skipped && !c.uncertain).length;
          if (cUncertain > 0) {
            Modal.error({
              title: `⚠ ${cUncertain} 笔转换结果未知，可能已在用友建单`,
              content: '这几笔保存时网络中断、没拿到用友应答，转换单可能已建成。请先去用友核对（下方标红的就是），别盲目重做，否则会重复建单。',
              okText: '我去核对',
            });
          } else if (cFail > 0) {
            message.warning(`转换完成（批次/状态），${cFail} 笔失败（看下方转换结果，修正后可再点①）`);
          } else {
            Modal.success({
              title: '转换完成（批次 / 状态）',
              content: '请点【生成 / 刷新计划】复查目标批次是否都到货、都是合格品了。用友库存刷新可能要等几秒，没立刻到货可稍等再刷新；计划里转换行（橙色批次 / 红色状态）消失，就能点②出库。',
              okText: '知道了',
            });
          }
        } else {
          // 第②步: 出库。
          setResults(rs);
          const ships = rs.flatMap((r) => r.shipments);
          const shipUncertain = (s: YbShipLog) => !!s.uncertain || s.conversions.some((c) => c.uncertain);
          const uncertain = ships.filter(shipUncertain).length;
          const skip = ships.filter((s) => s.out_skipped).length;
          const short = ships.filter((s) => s.bill_short).length; // 缺货整单未出, 不算失败
          const fail = ships.filter((s) => s.error && !s.out_skipped && !s.bill_short && !shipUncertain(s)).length;
          if (uncertain === 0 && fail === 0 && short === 0) {
            // 出库全成功: 清空计划 + 录入草稿(避免这批残留给下一次/同机下一人), 转换结果留着可看
            setPlans(null);
            setFlat('');
            setGroups([{ id: 1, head: '', details: '' }]);
            try { sessionStorage.removeItem(DRAFT_KEY); } catch { /* ignore */ }
            if (skip > 0) message.warning(`出库完成，其中 ${skip} 笔10分钟内已提交过、已自动跳过防重复；计划已清空`);
            else message.success('出库完成，全部成功');
          } else if (uncertain > 0) {
            Modal.error({
              title: `⚠ ${uncertain} 笔出库结果未知，可能已在用友建单`,
              content: '这几笔保存时网络中断、没拿到用友应答，出库单可能已经建成。请先去用友核对（下方标红“可能已建单”的就是），确认没有再重做，否则会重复出库！',
              okText: '我去核对',
            });
          } else if (fail > 0) {
            message.warning(`出库完成，${fail} 笔失败（已保留计划，可修正后再点②；已成功的会自动跳过防重）`);
          } else {
            // 只有缺货单据没出, 齐全的已出
            message.warning(`齐全单据已出库；有缺货单据整单未出（已保留计划，补齐库存后重新【生成 / 刷新计划】）`);
          }
        }
      }
    } catch (e) {
      // 连接中断/超时: 后端可能已部分或全部提交到用友(不会因前端断开而回滚)。
      // 不谎报"失败"误导用户重复手工建单; 后端有10分钟防重, 重发也不会重复建单。
      Modal.warning({
        title: '连接中断，结果未知',
        content: '执行可能已部分或全部提交到用友。请先去用友核对，不要凭“失败”重复手工建单。如确需补提交，10分钟内系统会自动跳过已提交的，不会重复。',
        okText: '我知道了',
      });
    } finally {
      setExecuting(false);
      setProgress(null);
    }
  };

  // 第①步: 只做批次转换 (不出库)。
  const handleExecuteConvert = () => {
    if (!plans || plans.length === 0) return;
    const convCount = plans.reduce(
      (n, p) => n + p.shipments.reduce((m, s) => m + (s.convert_sources?.length ?? 0), 0), 0);
    Modal.confirm({
      title: '确认执行转换（批次/状态，不可逆）',
      content: (
        <div>
          <p>第①步：在用友提交 <b>{convCount}</b> 笔转换 + 自动审核（把别的批次转成目标批次、把不合格/废品转成合格）。<b>本步不出库。</b></p>
          <p>单据日期：<b>{vouchdate.format('YYYY-MM-DD')}</b></p>
          <p><Text type="danger">会真改库存（含废品/不合格转合格），不可撤回。转换后请点【生成 / 刷新计划】复查再出库。</Text></p>
        </div>
      ),
      okText: '确认转换', cancelText: '取消', okButtonProps: { danger: true },
      onOk: () => { void doExecute('convert'); }, // 不 return promise: 确认框立即关, 交给进度弹窗
    });
  };

  // 第②步: 只做出库 (复查确认目标批次都到货后)。
  const handleExecuteOut = () => {
    if (!plans || plans.length === 0) return;
    const lineCount = plans.length;
    const shortfall = plans.filter((p) => p.remaining_qty > 0).length;
    Modal.confirm({
      title: '确认执行出库（不可逆）',
      content: (
        <div>
          <p>第②步：在用友提交 <b>{billCount(plans.map((p) => p.row))}</b> 张其他出库单（共 {lineCount} 行明细）+ 自动审核。<b>本步不再转换。</b></p>
          <p>单据日期：<b>{vouchdate.format('YYYY-MM-DD')}</b></p>
          {shortfall > 0 && <p><Text type="warning">注意：有 {shortfall} 行存在缺口，只会出能凑齐的部分。</Text></p>}
          <p><Text type="danger">会真扣库存，不可撤回。确认出库？</Text></p>
        </div>
      ),
      okText: '确认出库', cancelText: '取消', okButtonProps: { danger: true },
      onOk: () => { void doExecute('out'); }, // 不 return promise: 确认框立即关, 交给进度弹窗
    });
  };

  const planColumns: ColumnsType<YbPlan> = [
    { title: '单号', dataIndex: ['row', 'bill_no'], key: 'bill', width: 150 },
    {
      title: '货品', key: 'product', width: 200,
      render: (_, p) => (
        <span>{p.row.product_code}{p.product_name ? <><br /><Text type="secondary">{p.product_name}</Text></> : null}</span>
      ),
    },
    { title: '仓库', dataIndex: ['row', 'warehouse_name'], key: 'wh', width: 200, ellipsis: true },
    { title: '需出', dataIndex: 'needed_qty', key: 'need', width: 70 },
    {
      title: '拆单方案', key: 'ship',
      render: (_, p) => {
        if (!p.shipments.length) return <Text type="danger">无可用库存</Text>;
        return (
          <div>
            {p.shipments.map((s, i) => (
              <div key={i}>
                · {s.org_name} 出 <b>{s.qty}</b>{s.out_batch ? ` 批[${s.out_batch}]` : ''}
                {s.convert_qty > 0 && s.convert_sources.map((c, j) => {
                  const fromS = c.stockStatusDoc;
                  const isStatus = !!fromS && fromS !== YB_QUALIFIED_DOC; // 来源非合格=状态转换
                  return (
                    <Tag key={j} color={isStatus ? 'red' : 'orange'} style={{ marginLeft: 4 }}>
                      {isStatus ? `状态转 ${YB_STATUS_NAME[fromS] || fromS}→合格 ` : '批次转 '}
                      {c.from_batch}→{s.out_batch} {c.qty}
                    </Tag>
                  );
                })}
              </div>
            ))}
          </div>
        );
      },
    },
    { title: '已凑', dataIndex: 'fulfilled_qty', key: 'ful', width: 70, render: (v, p) => <Text type={p.remaining_qty === 0 ? 'success' : 'warning'}>{v}</Text> },
    {
      title: '缺口/出库', dataIndex: 'remaining_qty', key: 'rem', width: 110,
      render: (v, p) => p.bill_short
        ? <Text type="danger"><b>整单缺货不出</b></Text>
        : (v > 0 ? <Text type="danger">缺 {v}</Text> : <Text type="success">可出</Text>),
    },
  ];

  // ①批次转换结果: 只看每个 shipment 的 conversions
  const convResultColumns: ColumnsType<YbResult> = [
    { title: '单号', dataIndex: ['row', 'bill_no'], key: 'bill', width: 150 },
    { title: '货品', dataIndex: ['row', 'product_code'], key: 'product', width: 120 },
    {
      title: '转换结果（批次 / 状态）', key: 'conv',
      render: (_, r) => {
        const convs = r.shipments.flatMap((s) => s.conversions);
        if (!convs.length) return <Text type="secondary">无需转换</Text>;
        return (
          <div>
            {convs.map((c, j) => (
              <Tag key={j} color={c.uncertain ? 'red' : (c.skipped ? 'orange' : (c.error ? 'red' : 'green'))} style={{ marginBottom: 2 }}>
                转{c.from_batch} {c.qty}：{c.uncertain ? '⚠可能已建单,去核对' : (c.skipped ? '已跳过(10分钟内已提交)' : (c.error ? `失败 ${c.error}` : `${c.doc_code || '已建单'} ${c.audit_ok ? '已审核' : '未审核'}`))}
              </Tag>
            ))}
          </div>
        );
      },
    },
  ];

  // ②出库结果: 只看每个 shipment 的出库单(out)
  const outResultColumns: ColumnsType<YbResult> = [
    { title: '单号', dataIndex: ['row', 'bill_no'], key: 'bill', width: 150 },
    { title: '货品', dataIndex: ['row', 'product_code'], key: 'product', width: 120 },
    { title: '需出', dataIndex: 'needed_qty', key: 'need', width: 70 },
    {
      title: '出库结果', key: 'res',
      render: (_, r) => (
        <div>
          {r.shipments.map((s, i) => (
            <div key={i}>
              {s.bill_short
                ? <Tag color="red">整单缺货，未出库</Tag>
                : s.uncertain
                  ? <Tag color="red"><b>⚠ 可能已建单，去用友核对后再重做</b> 出{s.qty}</Tag>
                  : s.out_skipped
                    ? <Tag color="orange">已跳过（10分钟内已提交{s.out_doc_code ? `，单号 ${s.out_doc_code}` : ''}）出{s.qty}</Tag>
                    : s.error
                      ? <Tag color="red">失败: {s.error}</Tag>
                      : <Tag color={s.audit_ok ? 'green' : 'gold'}>{s.out_doc_code || '已建单'} {s.audit_ok ? '已审核' : '未审核'} 出{s.qty}</Tag>}
            </div>
          ))}
        </div>
      ),
    },
  ];

  // 计划里还有"批次转换"行=目标批次没凑齐→锁住出库, 逼先转换+复查。
  // 缺货单整单不出, 它的转换不算"待转换"(否则永远锁住②); 只看齐全单还有没有要转的。
  const hasConvert = !!plans && plans.some((p) => !p.bill_short && p.shipments.some((s) => (s.convert_sources?.length ?? 0) > 0));

  // 单据级缺货汇总(一眼看缺不缺, 不用下拉翻): 按单号聚合缺货量。
  const shortBills = useMemo(() => {
    if (!plans) return [];
    const m = new Map<string, number>();
    plans.forEach((p) => {
      if (p.bill_short) {
        const bn = p.row.bill_no || '(无单号)';
        m.set(bn, (m.get(bn) ?? 0) + p.remaining_qty);
      }
    });
    return Array.from(m.entries()).map(([bill, short]) => ({ bill, short }));
  }, [plans]);

  return (
    <div style={{ padding: 16 }}>
      <Card title="用友批量出库">
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message="吉客云出库单 → 用友其他出库单（跨组织拆单 + 批次/状态转换 + 自动审核）"
          description="两步走：①先执行转换（批次转换 + 不合格/废品转合格）→ 点【生成 / 刷新计划】复查目标批次都到货且合格 → ②再执行出库。出库一律出合格品。实时查用友库存，量大时较慢；执行会真在用友建单、扣库存、自动审核，不可撤回。"
        />

        <Space style={{ marginBottom: 16 }} wrap>
          <Text strong>单据日期</Text>
          <DatePicker value={vouchdate} onChange={(d) => d && setVouchdate(d)} allowClear={false} />
          <Text type="secondary">跨月做账：5 月的单选 5/31，别用今天</Text>
        </Space>

        <Tabs
          activeKey={mode}
          onChange={(k) => setMode(k as 'flat' | 'grouped')}
          items={[
            {
              key: 'flat',
              label: '一键平铺粘贴',
              children: (
                <>
                  <Paragraph type="secondary">
                    每行 6 列（单号 出库类型 仓库 货品编码 数量 批次），Tab 或多空格分隔，批次可留空
                  </Paragraph>
                  <TextArea
                    rows={8}
                    value={flat}
                    onChange={(e) => { clearResults(); setFlat(e.target.value); }}
                    placeholder={'CRK2026053120566\t其他出库\t南京委外成品仓-公司仓-委外\t03030207\t3\t20251114HT'}
                  />
                </>
              ),
            },
            {
              key: 'grouped',
              label: '分组录入（每单分开）',
              children: (
                <div>
                  {groups.map((g, idx) => (
                    <Card
                      key={g.id}
                      size="small"
                      title={`单据 #${idx + 1}`}
                      extra={<Button type="link" danger size="small" onClick={() => removeGroup(g.id)}>删除</Button>}
                      style={{ marginBottom: 12 }}
                    >
                      <Row gutter={12}>
                        <Col flex="320px">
                          <Paragraph type="secondary" style={{ marginBottom: 4 }}>单据头（一行：单号 出库类型 仓库）</Paragraph>
                          <TextArea
                            rows={5}
                            value={g.head}
                            onChange={(e) => updateGroup(g.id, 'head', e.target.value)}
                            placeholder={'CRK2026051215519\t其他出库\t南京委外包材仓-公司仓-委外'}
                          />
                        </Col>
                        <Col flex="auto">
                          <Paragraph type="secondary" style={{ marginBottom: 4 }}>明细（每行：货品编码 数量 批次，批次可留空）</Paragraph>
                          <TextArea
                            rows={5}
                            value={g.details}
                            onChange={(e) => updateGroup(g.id, 'details', e.target.value)}
                            placeholder={'03010147\t20\t20260425S\n03010294\t40\t20260127S\n05010234\t20\t'}
                          />
                        </Col>
                      </Row>
                    </Card>
                  ))}
                  <Button type="dashed" block onClick={addGroup}>+ 新增一张吉客云单</Button>
                </div>
              ),
            },
          ]}
        />

        <Space style={{ marginTop: 12 }} wrap>
          <Button type="primary" loading={planning} onClick={handlePlan}>生成 / 刷新计划（复查）</Button>
          <Button danger disabled={!plans || !hasConvert || executing} loading={executing} onClick={handleExecuteConvert}>① 执行转换（批次/状态）</Button>
          <Button danger disabled={!plans || hasConvert || executing} loading={executing} onClick={handleExecuteOut}>② 执行出库</Button>
        </Space>

        {/* 缺货汇总: 生成计划后一眼看缺不缺, 不用下拉一行行翻 */}
        {plans && shortBills.length > 0 && (
          <Alert
            style={{ marginTop: 12 }}
            type="error"
            showIcon
            message={`🔴 ${shortBills.length} 个单据缺货，整单不出（缺一行，整张单都不出）`}
            description={
              <div>
                {shortBills.map((s) => (
                  <Tag color="red" key={s.bill} style={{ marginBottom: 4 }}>{s.bill}（缺 {s.short}）</Tag>
                ))}
                <div style={{ marginTop: 4 }}>
                  <Text type="secondary">这些单据本次不会转换、也不会出库；补齐库存后重新【生成 / 刷新计划】即可。</Text>
                </div>
              </div>
            }
          />
        )}
        {plans && shortBills.length === 0 && (
          <Alert style={{ marginTop: 12 }} type="success" showIcon message="✅ 全部单据齐全，无缺货" />
        )}

        {plans && (
          <Alert
            style={{ marginTop: 12 }}
            type={hasConvert ? 'warning' : 'success'}
            showIcon
            message={hasConvert
              ? '目标批次/状态还差货：先点①执行转换（橙色=批次转换，红色=不合格/废品转合格）→ 再点【生成 / 刷新计划】复查 → 转换行消失后，②出库才会解锁'
              : '齐全单据的目标批次都够、且都是合格品：可以点②执行出库'}
          />
        )}

        {/* 执行中: 居中进度弹窗(挡住操作, 防止误点; 跑完自动关) */}
        <Modal
          open={executing}
          title="正在执行，请勿关闭页面"
          closable={false}
          maskClosable={false}
          keyboard={false}
          footer={null}
        >
          <Progress
            percent={progress && progress.total ? Math.round((progress.done / progress.total) * 100) : 0}
            status="active"
          />
          <div style={{ marginTop: 8 }}>
            <Text>{progress ? `正在执行 ${progress.done}/${progress.total || '?'}：${progress.label}` : '连接中…'}</Text>
          </div>
          <Paragraph type="secondary" style={{ marginTop: 8, marginBottom: 0 }}>
            用友限流每笔约 1 秒，大批量需要等一会儿，请勿关闭或刷新页面。
          </Paragraph>
        </Modal>

        {plans && (
          <Table
            style={{ marginTop: 16 }}
            size="small"
            rowKey={(_, i) => `p${i}`}
            columns={planColumns}
            dataSource={plans}
            pagination={false}
            scroll={{ y: 420 }}
            onRow={(p) => (p.bill_short ? { style: { background: '#fff1f0' } } : {})}
          />
        )}

        {convResults && (
          <>
            <Typography.Title level={5} style={{ marginTop: 20 }}>① 转换结果（批次 / 状态）</Typography.Title>
            <Table
              size="small"
              rowKey={(_, i) => `c${i}`}
              columns={convResultColumns}
              dataSource={convResults}
              pagination={false}
              scroll={{ y: 300 }}
            />
          </>
        )}

        {results && (
          <>
            <Typography.Title level={5} style={{ marginTop: 20 }}>② 出库结果</Typography.Title>
            <Table
              size="small"
              rowKey={(_, i) => `r${i}`}
              columns={outResultColumns}
              dataSource={results}
              pagination={false}
              scroll={{ y: 420 }}
            />
          </>
        )}
      </Card>
    </div>
  );
};

export default YonbipOutboundPage;
