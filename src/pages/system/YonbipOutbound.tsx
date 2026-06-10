import React, { useState } from 'react';
import {
  Card, Input, DatePicker, Button, Table, Tag, Space, Alert, Typography, message, Modal, Tabs, Row, Col,
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
interface YbConvSource { from_batch: string; qty: number; }
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

const YonbipOutboundPage: React.FC = () => {
  const [vouchdate, setVouchdate] = useState<Dayjs>(dayjs());
  const [flat, setFlat] = useState('');
  const [plans, setPlans] = useState<YbPlan[] | null>(null);
  const [results, setResults] = useState<YbResult[] | null>(null);
  const [planning, setPlanning] = useState(false);
  const [executing, setExecuting] = useState(false);
  const [mode, setMode] = useState<'flat' | 'grouped'>('flat');
  const [groups, setGroups] = useState<YbGroup[]>([{ id: 1, head: '', details: '' }]);

  const addGroup = () => setGroups((gs) => [...gs, { id: Math.max(0, ...gs.map((x) => x.id)) + 1, head: '', details: '' }]);
  const removeGroup = (id: number) => setGroups((gs) => {
    const n = gs.filter((x) => x.id !== id);
    return n.length ? n : [{ id: 1, head: '', details: '' }];
  });
  const updateGroup = (id: number, field: 'head' | 'details', val: string) =>
    setGroups((gs) => gs.map((x) => (x.id === id ? { ...x, [field]: val } : x)));

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
      setPlanning(false);
    }
  };

  const doExecute = async () => {
    if (!plans) return;
    setExecuting(true);
    try {
      const res = await fetch(`${API_BASE}/api/yonbip/export-execute`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({
          vouchdate: vouchdate.format('YYYY-MM-DD'),
          plans,
          group_by_bill: true,
        }),
      });
      const json = await res.json();
      if (res.ok && json.data?.results) {
        setResults(json.data.results);
        const ships = (json.data.results as YbResult[]).flatMap((r) => r.shipments);
        const shipUncertain = (s: YbShipLog) => !!s.uncertain || s.conversions.some((c) => c.uncertain);
        const uncertain = ships.filter(shipUncertain).length;
        const skip = ships.filter((s) => s.out_skipped).length;
        const fail = ships.filter((s) => s.error && !s.out_skipped && !shipUncertain(s)).length;
        if (uncertain === 0 && fail === 0) {
          setPlans(null); // 全部成功(或防重跳过), 清空计划
          if (skip > 0) message.warning(`执行完成，其中 ${skip} 笔10分钟内已提交过、已自动跳过防重复；计划已清空`);
          else message.success('执行完成，全部成功');
        } else if (uncertain > 0) {
          // 有"结果未知": 保留计划, 强提示去核对, 别盲目重做(可能已建单→重复)。
          Modal.error({
            title: `⚠ ${uncertain} 笔结果未知，可能已在用友建单`,
            content: '这几笔保存时网络中断、没拿到用友应答，出库单可能已经建成。请先去用友核对（下方标红“可能已建单”的就是），确认没有再重做，否则会重复出库！',
            okText: '我去核对',
          });
        } else {
          // 仅业务失败(用友明确拒单, 没建): 保留计划可修正重做; 已成功的再点会自动跳过防重。
          message.warning(`执行完成，${fail} 笔失败（已保留计划，可修正后再点执行；已成功的会自动跳过防重）`);
        }
      } else {
        // 非2xx(超时/服务器错误): 同样可能已部分提交到用友, 不谎报"失败"诱导重复操作。
        Modal.warning({
          title: '结果未知，请去用友核对',
          content: '服务器没返回正常结果，但本次可能已部分提交到用友。请先去用友核对，不要凭“失败”重复手工建单；如确需补提交，10分钟内系统会自动跳过已提交的。',
          okText: '我知道了',
        });
      }
    } catch (e) {
      // 连接中断/超时: 后端可能已部分或全部提交到用友(不会因前端断开而回滚)。
      // 不谎报"失败"误导用户重复手工建单; 后端有10分钟防重, 重发也不会重复建单。
      Modal.warning({
        title: '连接中断，结果未知',
        content: '出库执行可能已部分或全部提交到用友。请先去用友核对，不要凭“失败”重复手工建单。如确需补提交，10分钟内系统会自动跳过已提交的，不会重复。',
        okText: '我知道了',
      });
    } finally {
      setExecuting(false);
    }
  };

  const handleExecute = () => {
    if (!plans || plans.length === 0) return;
    const lineCount = plans.length;
    const shortfall = plans.filter((p) => p.remaining_qty > 0).length;
    Modal.confirm({
      title: '确认执行（不可逆）',
      content: (
        <div>
          <p>即将真实在用友提交 <b>{billCount(plans.map((p) => p.row))}</b> 张其他出库单（共 {lineCount} 行明细）的批次转换 + 出库单 + 自动审核。</p>
          <p>单据日期：<b>{vouchdate.format('YYYY-MM-DD')}</b></p>
          {shortfall > 0 && <p><Text type="warning">注意：有 {shortfall} 行存在缺口，只会出能凑齐的部分。</Text></p>}
          <p><Text type="danger">会真扣库存，不可撤回。确认执行？</Text></p>
        </div>
      ),
      okText: '确认执行',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: doExecute,
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
                {s.convert_qty > 0 && s.convert_sources.map((c, j) => (
                  <Tag key={j} color="orange" style={{ marginLeft: 4 }}>转 {c.from_batch}→{s.out_batch} {c.qty}</Tag>
                ))}
              </div>
            ))}
          </div>
        );
      },
    },
    { title: '已凑', dataIndex: 'fulfilled_qty', key: 'ful', width: 70, render: (v, p) => <Text type={p.remaining_qty === 0 ? 'success' : 'warning'}>{v}</Text> },
    { title: '缺口', dataIndex: 'remaining_qty', key: 'rem', width: 70, render: (v) => v > 0 ? <Text type="danger">{v}</Text> : <Text type="success">0</Text> },
  ];

  const resultColumns: ColumnsType<YbResult> = [
    { title: '单号', dataIndex: ['row', 'bill_no'], key: 'bill', width: 150 },
    { title: '货品', dataIndex: ['row', 'product_code'], key: 'product', width: 120 },
    { title: '需出', dataIndex: 'needed_qty', key: 'need', width: 70 },
    {
      title: '出库结果', key: 'res',
      render: (_, r) => (
        <div>
          {r.shipments.map((s, i) => (
            <div key={i}>
              {s.uncertain
                ? <Tag color="red"><b>⚠ 可能已建单，去用友核对后再重做</b> 出{s.qty}</Tag>
                : s.out_skipped
                  ? <Tag color="orange">已跳过（10分钟内已提交{s.out_doc_code ? `，单号 ${s.out_doc_code}` : ''}）出{s.qty}</Tag>
                  : s.error
                    ? <Tag color="red">失败: {s.error}</Tag>
                    : <Tag color={s.audit_ok ? 'green' : 'gold'}>{s.out_doc_code || '已建单'} {s.audit_ok ? '已审核' : '未审核'} 出{s.qty}</Tag>}
              {s.conversions.map((c, j) => (
                <Tag key={j} color={c.uncertain ? 'red' : (c.skipped ? 'orange' : (c.error ? 'red' : 'blue'))} style={{ marginLeft: 4 }}>
                  转{c.from_batch} {c.uncertain ? '⚠可能已建单' : (c.skipped ? '已跳过' : (c.audit_ok ? '✓' : (c.error || '?')))}
                </Tag>
              ))}
            </div>
          ))}
        </div>
      ),
    },
  ];

  return (
    <div style={{ padding: 16 }}>
      <Card title="用友批量出库">
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message="吉客云出库单 → 用友其他出库单（跨组织拆单 + 批次转换 + 自动审核）"
          description="实时查用友库存，量大时较慢；点「全部执行」会真在用友建单、扣库存、自动审核，不可撤回。"
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
                    onChange={(e) => setFlat(e.target.value)}
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

        <Space style={{ marginTop: 12 }}>
          <Button type="primary" loading={planning} onClick={handlePlan}>生成拆单计划</Button>
          <Button danger disabled={!plans || plans.length === 0} loading={executing} onClick={handleExecute}>全部执行</Button>
        </Space>

        {plans && (
          <Table
            style={{ marginTop: 16 }}
            size="small"
            rowKey={(_, i) => `p${i}`}
            columns={planColumns}
            dataSource={plans}
            pagination={false}
            scroll={{ y: 420 }}
          />
        )}

        {results && (
          <>
            <Typography.Title level={5} style={{ marginTop: 20 }}>执行结果</Typography.Title>
            <Table
              size="small"
              rowKey={(_, i) => `r${i}`}
              columns={resultColumns}
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
