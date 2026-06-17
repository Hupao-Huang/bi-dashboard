// 合思机器人 审批规则展示 (只读)
//
// 樊雪娇日常报销单的判定规则在后端规则引擎 server/internal/handler/hesi_audit_rules.go,
// 驱动待审批列表右下角的 AI 建议。这里把判定规则写出来给人看 (只读)。
// 改了后端规则记得同步这份文案。只写"判定要求", 不写不满足后果。
//
// 原 v1.60 用户自定义规则 CRUD 编辑器 (空表格 + 添加规则 + 干跑警告) 已下线:
// 它不真审批、长期空着易误导, 跑哥 2026-06-05 决定移除, 只留这份内置规则说明。

import React from 'react';
import { Card, Table, Typography, Space } from 'antd';

// ====== 樊雪娇日常报销单 审批判定规则 (系统内置, 只读展示) ======
const FXJ_AUDIT_RULES: { cat: string; rule: string }[] = [
  { cat: '部门', rule: '发起人部门必须是末级部门（部门底下不能再挂下级）' },
  { cat: '部门', rule: '报销 / 费用承担部门也必须是末级部门' },
  { cat: '收款', rule: '收款方式必须是银行账户' },
  { cat: '主体', rule: '报销的所属公司要和提交人的合同公司一致（分公司签的合同，可用主公司主体报销）' },
  { cat: '先申请后报销', rule: '业务招待费要关联「招待费用申请单」，且报销金额不超过申请额度' },
  { cat: '先申请后报销', rule: '固定资产要关联「固定资产申请单」，且金额不超过申请额度' },
  { cat: '先申请后报销', rule: '交通及差旅费要关联「出差申请单」，且金额不超过申请额度' },
  { cat: '差旅标准', rule: '火车票按发票上的坐席自动判：二等座及以下（含硬座、软座、硬卧、二等卧、无座等）通过；一等座、商务座、特等座、一等卧建议驳回；软卧、高级软卧、动卧或坐席识别不出来的转人工核' },
  { cat: '差旅标准', rule: '飞机（经济舱）、客车、汽车的座位等级票面上看不到，统一转人工核' },
  { cat: '差旅标准', rule: '住宿单晚价不超过「城市档次 × 职级」标准（两人同住可上浮 20%）' },
  { cat: '差旅标准', rule: '出差补贴不超过「出差天数 × 职级每天标准」' },
  { cat: '发票', rule: '发票要齐全，且抬头、税号、金额、开票时间都对得上（有 3 种情形可免发票）' },
  { cat: '私车公用', rule: '按行车记录自动核账：报销金额不超过系统按里程算出的补助（油 ¥0.7/公里、电 ¥0.6/公里）即通过，超了建议驳回；行车记录缺失才转人工核' },
  { cat: '消费事由', rule: '消费事由要写清楚（50 字以内即可）' },
  { cat: '必填项', rule: '品牌中心 / 研发中心必须选、附件要传齐、报销单的支付金额要一致' },
  { cat: '健康证及体检', rule: '金额不超过 100 元，且发票开票时间在 6 个月内，才可以通过' },
  { cat: '线下(世创/世用)', rule: '交通及差旅费之外的费用类型，非专用发票（普票等）必须在「付款截图」上传付款凭证等附件（差旅票本身不要求）' },
  { cat: '线下(世创/世用)', rule: '除补贴、样品、业务宣传费、私车公用外，明细金额必须与发票金额一分不差' },
  { cat: '线下(世创/世用)', rule: '出差补贴：集团经理及以上（含大区经理/总监）120 元/天（50 餐补+70 交通补），其他员工 100 元/天；当天报了私车公用的只算 50 餐补' },
  { cat: '线下(世创/世用)', rule: '私车公用以系统算出的报销金额为准，油费发票合计不低于该金额即可' },
  { cat: '差旅防重复', rule: '出差申请单里公司已经企业支付的车票（同车次、同乘车人、同票价），不能再放进报销发票里二次报销；票价不同视为另一张票，正常通过' },
  { cat: '补贴', rule: '出差补贴明细里选择的日期，必须落在关联出差申请单的出差起止日期范围内' },
  { cat: '发票', rule: '费用类型为广告费时，发票上的项目名称必须包含"广告"或"推广"（如开成"印刷品"等其他项目会建议驳回）' },
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
  return (
    <Card title="樊雪娇 · 报销审批判定规则（系统内置）" style={{ marginBottom: 16 }}>
      <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
        以下是机器人对樊雪娇日常报销单的判定规则，机器人据此给出待审批列表里的 AI 建议。单据满足全部规则才会建议通过，否则会提示驳回或人工核。
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
    </Card>
  );
};

export default HesiBotRules;
