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
// ref = 后端 hesi_audit_rules.go 里正在跑的规则编号, 页面"规则号"列直接显示, 改后端时照编号同步本表。
const FXJ_AUDIT_RULES: { ref: string; cat: string; rule: string }[] = [
  { ref: '2', cat: '部门', rule: '报销 / 费用承担部门不能选公司，必须选到公司下面的具体部门' },
  { ref: '3', cat: '收款', rule: '收款方式必须是银行账户' },
  { ref: '4', cat: '主体', rule: '报销的所属公司要和提交人的合同公司一致（分公司签的合同，可用主公司主体报销）' },
  { ref: '5', cat: '先申请后报销', rule: '业务招待费要关联「招待费用申请单」，且报销金额不超过申请额度' },
  { ref: '6', cat: '先申请后报销', rule: '固定资产要关联「固定资产申请单」，且金额不超过申请额度' },
  { ref: '7-1', cat: '先申请后报销', rule: '交通及差旅费（含私车公用、过路费）都要关联「出差申请单」，且报销金额不超过申请额度；线下世创/世用的交通差旅费全部无需关联' },
  { ref: '7-2', cat: '差旅标准', rule: '火车票按发票上的坐席自动判：二等座及以下（含硬座、软座、硬卧、二等卧、无座等）通过；一等座、商务座、特等座、一等卧建议驳回；软卧、高级软卧、动卧或坐席识别不出来的转人工核' },
  { ref: '7-2', cat: '差旅标准', rule: '飞机（经济舱）、客车、汽车的座位等级票面上看不到，统一转人工核' },
  { ref: '7-3', cat: '差旅标准', rule: '住宿单晚价不超过「城市档次 × 职级」标准（两人同住可上浮 20%）' },
  { ref: '11', cat: '差旅标准', rule: '出差补贴不超过「出差天数 × 职级每天标准」' },
  { ref: '8-1·8-2·8-3', cat: '发票', rule: '发票要齐全，抬头、税号要和报销主体对得上，每条明细的报销金额不超过发票合计；遇到定额发票等票面金额识别不出来的，转人工核' },
  { ref: '8-4', cat: '发票', rule: '发票开票时间离交单 1 个月内可直接通过，1-3 个月转人工核，超过 3 个月建议驳回（健康证及体检类放宽到 6 个月）' },
  { ref: '10', cat: '发票', rule: '这几类可以免发票：研发样品、出差补贴、私车公用、特殊截图说明' },
  { ref: '12-1', cat: '私车公用', rule: '按行车记录自动核账：报销金额不超过系统按里程算出的补助（油 ¥0.7/公里、电 ¥0.6/公里）即通过，超了建议驳回；行车记录缺失才转人工核' },
  { ref: '12-2', cat: '消费事由', rule: '消费事由要写清楚（50 字以内即可）' },
  { ref: '13', cat: '必填项', rule: '品牌中心 / 研发中心必须选、附件要传齐、报销单的支付金额要一致' },
  { ref: '14', cat: '健康证及体检', rule: '金额不超过 100 元，且发票开票时间在 6 个月内，才可以通过' },
  { ref: '15-1.2', cat: '线下(世创/世用)', rule: '交通及差旅费之外的费用类型，非专用发票（普票等）必须上传付款凭证：放在费用明细的「付款截图」字段、或单据附件里，二选一即可（差旅票本身不要求）' },
  { ref: '15-2', cat: '线下(世创/世用)', rule: '除补贴、样品、业务宣传费、私车公用外，明细金额必须与发票金额一分不差' },
  { ref: '15-3', cat: '线下(世创/世用)', rule: '出差补贴：集团经理及以上（含大区经理/总监）120 元/天（50 餐补+70 交通补），其他员工 100 元/天；当天报了私车公用的只算 50 餐补' },
  { ref: '15-4', cat: '线下(世创/世用)', rule: '私车公用以系统算出的报销金额为准，油费发票合计不低于该金额即可' },
  { ref: '16', cat: '差旅防重复', rule: '出差申请单里公司已经企业支付的车票（同车次、同乘车人、同票价），不能再放进报销发票里二次报销；票价不同视为另一张票，正常通过' },
  { ref: '17', cat: '补贴', rule: '出差补贴明细里选择的日期，必须落在关联出差申请单的出差起止日期范围内' },
  { ref: '18', cat: '发票', rule: '广告费与"广告/推广"发票双向对应：① 费用类型为广告费时，发票项目名称必须含"广告"或"推广"（开成"印刷品"等会建议驳回）；② 反过来，发票项目名称含"广告"或"推广"但费用类型不是广告费的，也建议驳回' },
];

// 住宿标准 (¥/晚, 城市档次 × 职级; 两人同住按职位高者标准上浮 20%)
const FXJ_HOTEL_STD = [
  { level: '总裁', t1: 1200, t2: 1000, t3: 1000, other: 800 },
  { level: '副总裁', t1: 1000, t2: 800, t3: 800, other: 600 },
  { level: '集团总监', t1: 500, t2: 400, t3: 400, other: 300 },
  { level: '集团经理', t1: 450, t2: 350, t3: 350, other: 300 },
  { level: '主管及其他', t1: 400, t2: 300, t3: 300, other: 300 },
];

// 线下（世创/世用）住宿标准 (¥/晚, 3 档城市 一线/二线/国内其他 × 3 档职级; 樊雪娇 2026-06-17)
// 新一线城市按二线算; 同住上浮 20% 同集团
const FXJ_HOTEL_STD_OFFLINE = [
  { level: '集团总监', t1: 500, t2: 400, other: 300 },
  { level: '大区经理及以上', t1: 450, t2: 350, other: 280 },
  { level: '其他员工', t1: 350, t2: 280, other: 230 },
];

// 出差补贴标准 (¥/天)
const FXJ_SUBSIDY_STD = [
  { level: '总裁', v: 200 }, { level: '副总裁', v: 150 }, { level: '集团总监', v: 100 },
  { level: '集团经理', v: 80 }, { level: '主管及其他', v: 60 },
];

const HesiBotRules: React.FC = () => {
  return (
    <Card title="日常报销单 · 审批判定规则（系统内置）" style={{ marginBottom: 16 }}>
      <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
        以下是机器人对日常报销单的判定规则，机器人据此给出待审批列表里的 AI 建议。单据满足全部规则才会建议通过，否则会提示驳回或人工核。
      </Typography.Paragraph>
      <Table
        columns={[
          { title: '规则号', dataIndex: 'ref', width: 112, align: 'center' as const },
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
          <Typography.Text strong>住宿标准 · 集团（每晚上限，城市档次 × 职级）</Typography.Text>
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
          <Typography.Text strong>住宿标准 · 线下世创/世用（每晚上限）</Typography.Text>
          <Table
            style={{ marginTop: 8 }}
            columns={[
              { title: '职级', dataIndex: 'level' },
              { title: '一线', dataIndex: 't1', align: 'right' as const },
              { title: '二线', dataIndex: 't2', align: 'right' as const },
              { title: '国内其他', dataIndex: 'other', align: 'right' as const },
            ]}
            dataSource={FXJ_HOTEL_STD_OFFLINE}
            rowKey="level"
            pagination={false}
            size="small"
          />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>注：新一线城市按二线算；两人同住上浮 20%。</Typography.Text>
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
