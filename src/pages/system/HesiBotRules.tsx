// v1.60.0 合思机器人规则管理 — 用户自定义自动审批条件
// 当前 dry_run 默认 true (匹配但不审批, v1.61 才扫描, v1.62 真审批)

import React, { useCallback, useEffect, useState } from 'react';
import {
  Alert, Button, Card, Collapse, Form, Input, InputNumber, Modal, Popconfirm,
  Select, Space, Switch, Table, Tag, Tooltip, Typography, message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { DeleteOutlined, EditOutlined, PlusOutlined, MinusCircleOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';

interface Condition {
  field: string;
  op: string;
  value: string | number;
}

interface Rule {
  id: number;
  userId: number;
  name: string;
  enabled: boolean;
  dryRun: boolean;
  actionType: 'agree' | 'reject';
  approveComment: string;
  maxAmount: number;
  conditions: Condition[] | string;
  priority: number;
  matchedCount: number;
  approvedCount: number;
  lastMatchedAt?: string | null;
  createdAt: string;
  updatedAt: string;
}

const FIELDS = [
  { key: 'formType', label: '单据类型', type: 'select', options: [
    { value: 'expense', label: '报销单' }, { value: 'loan', label: '借款单' },
    { value: 'requisition', label: '申请单' }, { value: 'custom', label: '通用审批' },
  ] },
  { key: 'payMoney', label: '支付金额', type: 'number' },
  { key: 'expenseMoney', label: '报销金额', type: 'number' },
  { key: 'loanMoney', label: '借款金额', type: 'number' },
  { key: 'title', label: '标题', type: 'string' },
  { key: 'stageName', label: '当前节点', type: 'string' },
];

const OP_BY_TYPE: Record<string, { value: string; label: string }[]> = {
  number: [
    { value: 'lt', label: '<' }, { value: 'lte', label: '≤' },
    { value: 'gt', label: '>' }, { value: 'gte', label: '≥' },
    { value: 'eq', label: '=' }, { value: 'ne', label: '≠' },
  ],
  string: [
    { value: 'eq', label: '等于' }, { value: 'ne', label: '不等于' },
    { value: 'contains', label: '包含' },
  ],
  select: [
    { value: 'eq', label: '等于' }, { value: 'ne', label: '不等于' },
  ],
};

const fieldType = (key: string) => FIELDS.find(f => f.key === key)?.type || 'string';
const fieldLabel = (key: string) => FIELDS.find(f => f.key === key)?.label || key;
const opLabel = (op: string) => Object.values(OP_BY_TYPE).flat().find(o => o.value === op)?.label || op;

// ====== 樊雪娇日常报销单 审批判定规则 (系统内置, 只读展示) ======
// 来源: server/internal/handler/hesi_audit_rules.go (后端规则引擎, 驱动列表右下角 AI 建议)
// 这里是给人看的判定标准说明, 改了后端规则记得同步这份文案。只写"判定要求", 不写不满足后果。
const FXJ_AUDIT_RULES: { cat: string; rule: string }[] = [
  { cat: '部门', rule: '发起人部门必须是末级部门（部门底下不能再挂下级）' },
  { cat: '部门', rule: '报销 / 费用承担部门也必须是末级部门' },
  { cat: '收款', rule: '收款方式必须是银行账户' },
  { cat: '主体', rule: '报销的所属公司要和提交人的合同公司一致（分公司签的合同，可用主公司主体报销）' },
  { cat: '先申请后报销', rule: '业务招待费要关联「招待费用申请单」，且报销金额不超过申请额度' },
  { cat: '先申请后报销', rule: '固定资产要关联「固定资产申请单」，且金额不超过申请额度' },
  { cat: '先申请后报销', rule: '交通及差旅费要关联「出差申请单」，且金额不超过申请额度' },
  { cat: '差旅标准', rule: '交通座位：火车二等座及以下合规；一等座、商务座属超标；卧铺及飞机、客车、汽车需人工核' },
  { cat: '差旅标准', rule: '住宿单晚价不超过「城市档次 × 职级」标准（两人同住可上浮 20%）' },
  { cat: '差旅标准', rule: '出差补贴不超过「出差天数 × 职级每天标准」' },
  { cat: '发票', rule: '发票要齐全，且抬头、税号、金额、开票时间都对得上（有 3 种情形可免发票）' },
  { cat: '消费事由', rule: '自驾出行需人工核；消费事由要写清楚（50 字以内即可）' },
  { cat: '必填项', rule: '品牌中心 / 研发中心必须选、附件要传齐、报销单的支付金额要一致' },
];

// 住宿标准 (¥/晚, 城市档次 × 职级; 两人同住按职位高者标准上浮 20%)
const FXJ_HOTEL_STD = [
  { level: '总裁', t1: 1200, t2: 1000, t3: 1000, other: 800 },
  { level: '副总裁', t1: 1000, t2: 800, t3: 800, other: 600 },
  { level: '集团总监', t1: 500, t2: 400, t3: 400, other: 300 },
  { level: '集团经理', t1: 450, t2: 350, t3: 350, other: 300 },
  { level: '主管及其他', t1: 400, t2: 300, t3: 300, other: 300 },
];

// 出差补贴标准 (¥/天)
const FXJ_SUBSIDY_STD = [
  { level: '总裁', v: 200 }, { level: '副总裁', v: 150 }, { level: '集团总监', v: 100 },
  { level: '集团经理', v: 80 }, { level: '主管及其他', v: 60 },
];

const HesiBotRules: React.FC = () => {
  const [rules, setRules] = useState<Rule[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<Rule | null>(null);
  const [form] = Form.useForm();

  const fetchRules = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/profile/hesi-rules`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200 && json.data) {
        const items = (json.data.items || []).map((r: any) => ({
          ...r,
          conditions: typeof r.conditions === 'string' ? JSON.parse(r.conditions || '[]') : (r.conditions || []),
        }));
        setRules(items);
      }
    } catch (e) {
      message.error('加载规则失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchRules(); }, [fetchRules]);

  const handleAdd = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({
      enabled: false,
      dryRun: true,
      actionType: 'agree',
      maxAmount: 1000,
      priority: 100,
      conditions: [{ field: 'formType', op: 'eq', value: 'expense' }],
    });
    setModalOpen(true);
  };

  const handleEdit = (rule: Rule) => {
    setEditing(rule);
    form.setFieldsValue({
      ...rule,
      conditions: Array.isArray(rule.conditions) ? rule.conditions : [],
    });
    setModalOpen(true);
  };

  const handleDelete = async (id: number) => {
    try {
      const res = await fetch(`${API_BASE}/api/profile/hesi-rules/${id}`, {
        method: 'DELETE', credentials: 'include',
      });
      if (res.ok) {
        message.success('删除成功');
        fetchRules();
      } else {
        const data = await res.json().catch(() => ({}));
        message.error(data.msg || '删除失败');
      }
    } catch {
      message.error('网络错误');
    }
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      const url = editing
        ? `${API_BASE}/api/profile/hesi-rules/${editing.id}`
        : `${API_BASE}/api/profile/hesi-rules`;
      const method = editing ? 'PUT' : 'POST';
      const res = await fetch(url, {
        method,
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(values),
      });
      const data = await res.json().catch(() => ({}));
      if (res.ok) {
        message.success(editing ? '更新成功' : '创建成功');
        setModalOpen(false);
        fetchRules();
      } else {
        message.error(data.msg || data.error || '保存失败');
      }
    } catch (e) {
      // form validate 失败已显示
    }
  };

  const columns: ColumnsType<Rule> = [
    {
      title: '规则名', dataIndex: 'name', width: 200,
      render: (name, r) => (
        <span>
          <strong>{name}</strong>
          {r.enabled
            ? <Tag color="success" style={{ marginLeft: 8 }}>{r.dryRun ? '干跑中' : '执行中'}</Tag>
            : <Tag style={{ marginLeft: 8 }}>停用</Tag>
          }
        </span>
      ),
    },
    {
      title: '动作', dataIndex: 'actionType', width: 80,
      render: (v) => v === 'agree' ? <Tag color="success">同意</Tag> : <Tag color="error">驳回</Tag>,
    },
    {
      title: '金额上限', dataIndex: 'maxAmount', width: 110, align: 'right',
      render: (v) => v > 0 ? `¥${Number(v).toLocaleString()}` : <Tag color="warning">无限制</Tag>,
    },
    {
      title: '条件', dataIndex: 'conditions', width: 280,
      render: (conds: Condition[]) => (
        <div style={{ fontSize: 12 }}>
          {(conds || []).map((c, i) => (
            <Tag key={i} style={{ marginBottom: 2 }}>
              {fieldLabel(c.field)} {opLabel(c.op)} {String(c.value)}
            </Tag>
          ))}
        </div>
      ),
    },
    { title: '优先级', dataIndex: 'priority', width: 80, align: 'center' },
    { title: '匹配/审批', width: 100, align: 'center',
      render: (_, r) => <span>{r.matchedCount} / {r.approvedCount}</span>,
    },
    {
      title: '操作', width: 140, fixed: 'right',
      render: (_, r) => (
        <Space>
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => handleEdit(r)}>编辑</Button>
          <Popconfirm title={`确定删除"${r.name}"?`} onConfirm={() => handleDelete(r.id)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Card
      title="我的审批规则"
      style={{ marginBottom: 16 }}
      extra={<Button type="primary" icon={<PlusOutlined />} onClick={handleAdd}>添加规则</Button>}
    >
      <Alert
        type="warning"
        showIcon
        message="当前规则不会真审批 (v1.60 阶段)"
        description="所有规则默认干跑模式: 满足条件会在 v1.61 (扫描+日志) 中记录,但不调合思 API 真审批; 真自动审批需 v1.62 + 财务/合规批准."
        style={{ marginBottom: 16 }}
      />

      {/* 樊雪娇 报销审批判定规则 — 系统内置只读展示 (内容来自后端规则引擎, 驱动右下角 AI 建议) */}
      <Collapse
        defaultActiveKey={['fxj']}
        style={{ marginBottom: 16 }}
        items={[{
          key: 'fxj',
          label: '樊雪娇 · 报销审批判定规则（系统内置，机器人据此给出右下角的 AI 建议）',
          children: (
            <div>
              <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
                以下是机器人对樊雪娇日常报销单的判定规则。单据满足全部规则才会建议通过，否则会提示驳回或人工核。
              </Typography.Paragraph>
              <Table
                columns={[
                  { title: '序号', width: 60, align: 'center' as const, render: (_: any, __: any, i: number) => i + 1 },
                  { title: '类别', dataIndex: 'cat', width: 120 },
                  { title: '判定规则', dataIndex: 'rule' },
                ]}
                dataSource={FXJ_AUDIT_RULES}
                rowKey={(r) => r.rule}
                pagination={false}
                size="small"
              />
              <Space align="start" wrap size={32} style={{ marginTop: 16 }}>
                <div>
                  <Typography.Text strong>住宿标准（每晚上限，城市档次 × 职级）</Typography.Text>
                  <Table
                    style={{ marginTop: 8 }}
                    columns={[
                      { title: '职级', dataIndex: 'level' },
                      { title: '一线', dataIndex: 't1', align: 'right' as const },
                      { title: '新一线', dataIndex: 't2', align: 'right' as const },
                      { title: '二线', dataIndex: 't3', align: 'right' as const },
                      { title: '其他', dataIndex: 'other', align: 'right' as const },
                    ]}
                    dataSource={FXJ_HOTEL_STD}
                    rowKey="level"
                    pagination={false}
                    size="small"
                  />
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>注：两人同住按职位高者标准上浮 20%。</Typography.Text>
                </div>
                <div>
                  <Typography.Text strong>出差补贴标准（每天）</Typography.Text>
                  <Table
                    style={{ marginTop: 8 }}
                    columns={[
                      { title: '职级', dataIndex: 'level' },
                      { title: '每天补贴', dataIndex: 'v', align: 'right' as const, render: (v: number) => `¥${v}` },
                    ]}
                    dataSource={FXJ_SUBSIDY_STD}
                    rowKey="level"
                    pagination={false}
                    size="small"
                  />
                </div>
              </Space>
            </div>
          ),
        }]}
      />

      <Table<Rule>
        columns={columns}
        dataSource={rules}
        rowKey="id"
        loading={loading}
        pagination={false}
        size="middle"
        locale={{ emptyText: '还没有规则, 点右上角"添加规则"开始' }}
        scroll={{ x: 1000 }}
      />

      <Modal
        title={editing ? `编辑规则: ${editing.name}` : '添加规则'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={handleSubmit}
        width={720}
        okText="保存"
        cancelText="取消"
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="规则名" rules={[{ required: true, max: 100 }]}>
            <Input placeholder="例: 小额报销自动同意" />
          </Form.Item>

          <Space size={32} style={{ width: '100%', marginBottom: 16 }}>
            <Form.Item name="enabled" label="启用机器人" valuePropName="checked" style={{ marginBottom: 0 }}>
              <Switch />
            </Form.Item>
            <Form.Item name="dryRun" label="干跑模式 (强烈推荐)" valuePropName="checked" tooltip="开=只匹配不真审批; 关=真调合思接口审批 (需 v1.62+)" style={{ marginBottom: 0 }}>
              <Switch />
            </Form.Item>
            <Form.Item name="actionType" label="动作" style={{ marginBottom: 0 }}>
              <Select style={{ width: 110 }} options={[
                { value: 'agree', label: '同意' },
                { value: 'reject', label: '驳回' },
              ]} />
            </Form.Item>
          </Space>

          <Space size={16} style={{ width: '100%', marginBottom: 16 }}>
            <Form.Item name="maxAmount" label="金额上限(护栏)" tooltip="超过此金额绝不自动, 0=不限制(不推荐)" style={{ marginBottom: 0 }}>
              <InputNumber style={{ width: 160 }} min={0} step={100} addonAfter="元" />
            </Form.Item>
            <Form.Item name="priority" label="优先级" tooltip="数字越小越先匹配 (多规则时)" style={{ marginBottom: 0 }}>
              <InputNumber style={{ width: 100 }} min={1} max={9999} />
            </Form.Item>
          </Space>

          <Form.Item name="approveComment" label="审批备注">
            <Input placeholder="例: 机器人自动审批 (会作为合思审批意见)" />
          </Form.Item>

          <Form.Item label="条件 (满足全部才生效)" required>
            <Form.List name="conditions">
              {(fields, { add, remove }) => (
                <>
                  {fields.map(({ key, name, ...rest }) => (
                    <Space key={key} align="baseline" style={{ display: 'flex', marginBottom: 8 }}>
                      <Form.Item {...rest} name={[name, 'field']} rules={[{ required: true }]} style={{ marginBottom: 0 }}>
                        <Select style={{ width: 130 }} placeholder="字段"
                          options={FIELDS.map(f => ({ value: f.key, label: f.label }))}
                          onChange={() => {
                            const all = form.getFieldValue('conditions');
                            all[name] = { ...all[name], op: undefined, value: undefined };
                            form.setFieldsValue({ conditions: all });
                          }}
                        />
                      </Form.Item>
                      <Form.Item shouldUpdate={(p, c) => p.conditions?.[name]?.field !== c.conditions?.[name]?.field} style={{ marginBottom: 0 }}>
                        {({ getFieldValue }) => {
                          const fkey = getFieldValue(['conditions', name, 'field']);
                          const ops = OP_BY_TYPE[fieldType(fkey)] || [];
                          return (
                            <Form.Item {...rest} name={[name, 'op']} rules={[{ required: true }]} style={{ marginBottom: 0 }}>
                              <Select style={{ width: 100 }} placeholder="操作符" options={ops} />
                            </Form.Item>
                          );
                        }}
                      </Form.Item>
                      <Form.Item shouldUpdate={(p, c) => p.conditions?.[name]?.field !== c.conditions?.[name]?.field} style={{ marginBottom: 0 }}>
                        {({ getFieldValue }) => {
                          const fkey = getFieldValue(['conditions', name, 'field']);
                          const ftype = fieldType(fkey);
                          const fdef = FIELDS.find(f => f.key === fkey);
                          if (ftype === 'select' && fdef?.options) {
                            return (
                              <Form.Item {...rest} name={[name, 'value']} rules={[{ required: true }]} style={{ marginBottom: 0 }}>
                                <Select style={{ width: 160 }} placeholder="值" options={fdef.options} />
                              </Form.Item>
                            );
                          }
                          if (ftype === 'number') {
                            return (
                              <Form.Item {...rest} name={[name, 'value']} rules={[{ required: true }]} style={{ marginBottom: 0 }}>
                                <InputNumber style={{ width: 160 }} placeholder="数值" />
                              </Form.Item>
                            );
                          }
                          return (
                            <Form.Item {...rest} name={[name, 'value']} rules={[{ required: true }]} style={{ marginBottom: 0 }}>
                              <Input style={{ width: 200 }} placeholder="文本" />
                            </Form.Item>
                          );
                        }}
                      </Form.Item>
                      <MinusCircleOutlined onClick={() => remove(name)} style={{ color: '#ff4d4f' }} />
                    </Space>
                  ))}
                  <Button type="dashed" onClick={() => add({ field: 'formType', op: 'eq', value: 'expense' })} icon={<PlusOutlined />}>
                    添加条件
                  </Button>
                </>
              )}
            </Form.List>
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};

export default HesiBotRules;
