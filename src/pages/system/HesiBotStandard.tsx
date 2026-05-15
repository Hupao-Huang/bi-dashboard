// 报销单审核标准说明 (只读展示, 数据源: 张俊 Excel v2026.5.13)
// 在合思机器人页展示完整审批规则全貌, 让审批人看清机器人按啥标准在判
// 不做 CRUD, 纯参考型展示

import React from 'react';
import { Card, Collapse, Table, Tag, Alert, Space } from 'antd';
import type { ColumnsType } from 'antd/es/table';

type Capability = 'auto' | 'auto-todo' | 'lookup' | 'ocr' | 'manual';

interface RuleRow {
  key: string;
  no: string;
  item: string;
  std: string;
  capability: Capability;
  capNote?: string;
}

const CAP_TAG: Record<Capability, { color: string; label: string }> = {
  'auto':      { color: 'success',    label: '已自动 ✓' },
  'auto-todo': { color: 'processing', label: '可自动·待开发' },
  'lookup':    { color: 'warning',    label: '需关联查询' },
  'ocr':       { color: 'error',      label: '需 OCR / AI 识别' },
  'manual':    { color: 'default',    label: '人工复核' },
};

// 板块 1: 基本信息 (6 项)
const SEC1_BASIC: RuleRow[] = [
  {
    key: 'b1', no: '①', item: '所属公司',
    std: '杭州松鲜鲜自然调味品有限公司 (与提交人合同主体一致, 配花名册)',
    capability: 'auto',
    capNote: '法人实体黑名单校验 (合思 48 家主体, 仅 1 家海外主体转人工)',
  },
  {
    key: 'b2', no: '②', item: '报销事由',
    std: '简单概述, 不能空',
    capability: 'auto-todo',
    capNote: '字段非空校验, 待补到规则引擎',
  },
  {
    key: 'b3', no: '③', item: '支付时间',
    std: '与提交月份一致 (跨月仅审批延迟可放过)',
    capability: 'auto-todo',
    capNote: '月份对比, 待补',
  },
  {
    key: 'b4', no: '④', item: '提交人部门',
    std: '必须末级部门',
    capability: 'lookup',
    capNote: '需查部门树确认是否末级',
  },
  {
    key: 'b5', no: '⑤', item: '费用承担部门',
    std: '必须末级部门',
    capability: 'lookup',
    capNote: '需查部门树',
  },
  {
    key: 'b6', no: '⑥', item: '收款信息',
    std: '必须银行账户, 微信 / 支付宝 / 钉钉付款一律驳回',
    capability: 'auto-todo',
    capNote: '字段类型校验, 待补',
  },
];

// 板块 2: 费用明细 · 集团版 (10 项)
const SEC2_DETAIL_HQ: RuleRow[] = [
  {
    key: 'd1', no: '①', item: '费用类型映射',
    std: '依据 "合思费用-费用类型.xlsx" 字典映射',
    capability: 'lookup',
    capNote: '需录入字典表',
  },
  {
    key: 'd2', no: '②', item: '业务招待费',
    std: '关联招待费用申请单, 报销金额 ≤ 申请金额',
    capability: 'lookup',
    capNote: '需查关联申请单 + 金额对比',
  },
  {
    key: 'd3', no: '③', item: '固定资产',
    std: '关联固定资产申请单, 报销金额 ≤ 申请金额',
    capability: 'lookup',
    capNote: '同 ②',
  },
  {
    key: 'd4', no: '④', item: '交通 / 差旅',
    std: '关联出差申请单; 住宿按 5 级城市标准 — 总裁/副总/总监/经理/主管 各档位 (一线 1200 → 主管 300 等)',
    capability: 'lookup',
    capNote: '复杂多层判断 + 花名册职位',
  },
  {
    key: 'd5', no: '⑤', item: '发票一致性',
    std: '抬头 / 税号 / 金额 与报销一致, 开票时间 ≤ 1 个月 (可放宽 3 个月)',
    capability: 'ocr',
    capNote: '需 OCR 识别发票',
  },
  {
    key: 'd6', no: '⑥', item: '报销金额 vs 发票金额',
    std: '报销金额 ≤ 发票金额 (增值税专票 / 机票 / 高铁 / 打车票 需附支付截图)',
    capability: 'auto-todo',
    capNote: '字段对比, 待补',
  },
  {
    key: 'd7', no: '⑦', item: '无票场景',
    std: '研发样品 / 出差补贴 / 特殊情形 (需备注说明)',
    capability: 'lookup',
    capNote: '类型 + 备注校验',
  },
  {
    key: 'd8', no: '⑧', item: '出差补贴',
    std: '总裁 200 / 副总 150 / 总监 100 / 经理 80 / 主管及以下 60 元/天',
    capability: 'lookup',
    capNote: '需花名册查职位',
  },
  {
    key: 'd9', no: '⑨', item: '自驾报销',
    std: '油车 0.7 元/KM, 电车 0.6 元/KM, 过路 / 停车凭票报销',
    capability: 'ocr',
    capNote: '地图截图 OCR',
  },
  {
    key: 'd10', no: '⑩', item: '消费事由长度',
    std: '原标准 ≤ 10 字, 放宽到 ≤ 50 字; 不能空; 不能含"合计 / 小计"',
    capability: 'auto',
    capNote: '已实现: 空 → 驳回 / 含合计小计 → 驳回 / > 50 字 → 转人工',
  },
];

// 板块 3: 费用明细 · 线下版差异
const SEC3_DETAIL_OFFLINE: { title: string; std: string }[] = [
  {
    title: '出差补贴 (大区经理 / 总监)',
    std: '50 元餐补 + 70 元交通 / 天',
  },
  {
    title: '出差补贴 (其他线下职级)',
    std: '50 元餐补 + 50 元交通 / 天',
  },
  {
    title: '私车公用日',
    std: '仅 50 元餐补 (无交通补贴)',
  },
  {
    title: '其余规则',
    std: '与集团版相同',
  },
];

// 板块 4: 其他 (全人工)
const SEC4_OTHER: string[] = [
  '备注栏: 选填',
  '品牌中心: 备注必选',
  '研发中心: 备注必选',
  '附件完整性',
  '金额复核',
];

const ruleColumns: ColumnsType<RuleRow> = [
  { title: '#',    dataIndex: 'no',   width: 50, align: 'center' },
  { title: '项',   dataIndex: 'item', width: 160 },
  { title: '审批标准', dataIndex: 'std' },
  {
    title: '机器人能力',
    dataIndex: 'capability',
    width: 220,
    render: (cap: Capability, r) => {
      const t = CAP_TAG[cap];
      return (
        <Space direction="vertical" size={2}>
          <Tag color={t.color}>{t.label}</Tag>
          {r.capNote && <span style={{ fontSize: 12 }}>{r.capNote}</span>}
        </Space>
      );
    },
  },
];

const HesiBotStandard: React.FC = () => {
  return (
    <Card title="报销单审核标准 (张俊 v2026.5.13)" style={{ marginBottom: 16 }}>
      <Alert
        type="info"
        showIcon
        message="只读展示, 给审批人看完整规则全貌"
        description={
          <div>
            <div>规则源自张俊提供的《报销单审核.xlsx》, 共 4 板块 21 条 + 5 项人工复核.</div>
            <div style={{ marginTop: 4 }}>
              "机器人能力" 列说明: <Tag color="success">已自动</Tag>合思机器人扫描时已能判;
              <Tag color="processing">可自动·待开发</Tag>规则简单, 后续会补到引擎;
              <Tag color="warning">需关联查询</Tag>要查花名册 / 部门树 / 字典;
              <Tag color="error">需 OCR / AI 识别</Tag>需要图片识别能力;
              <Tag color="default">人工复核</Tag>张俊本人审.
            </div>
          </div>
        }
        style={{ marginBottom: 16 }}
      />

      <Collapse
        defaultActiveKey={['sec1', 'sec2', 'sec3', 'sec4']}
        items={[
          {
            key: 'sec1',
            label: `三、基本信息 (共 ${SEC1_BASIC.length} 项)`,
            children: (
              <Table<RuleRow>
                columns={ruleColumns}
                dataSource={SEC1_BASIC}
                pagination={false}
                size="middle"
              />
            ),
          },
          {
            key: 'sec2',
            label: `四、费用明细 · 集团版 (共 ${SEC2_DETAIL_HQ.length} 项)`,
            children: (
              <Table<RuleRow>
                columns={ruleColumns}
                dataSource={SEC2_DETAIL_HQ}
                pagination={false}
                size="middle"
              />
            ),
          },
          {
            key: 'sec3',
            label: `四、费用明细 · 线下版 (差异 ${SEC3_DETAIL_OFFLINE.length} 项)`,
            children: (
              <Table
                columns={[
                  { title: '差异项', dataIndex: 'title', width: 240 },
                  { title: '标准', dataIndex: 'std' },
                ]}
                dataSource={SEC3_DETAIL_OFFLINE.map((r, i) => ({ key: i, ...r }))}
                pagination={false}
                size="middle"
              />
            ),
          },
          {
            key: 'sec4',
            label: `五、其他 (人工复核 ${SEC4_OTHER.length} 项)`,
            children: (
              <ul style={{ paddingLeft: 20, marginBottom: 0 }}>
                {SEC4_OTHER.map((s, i) => <li key={i}>{s}</li>)}
              </ul>
            ),
          },
        ]}
      />
    </Card>
  );
};

export default HesiBotStandard;
