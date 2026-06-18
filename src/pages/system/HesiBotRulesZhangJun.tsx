// 合思机器人 审批规则展示 (只读) — 张俊 对外付款单 / 预付款单
//
// 张俊付款审批的判定规则在后端规则引擎 server/internal/handler/hesi_audit_payment_rules.go (AuditPayment),
// 驱动待审批列表里的 AI 建议。这里把判定规则写出来给人看 (只读)。
// 跟樊雪娇那份 (HesiBotRules.tsx) 同一套展示风格: 只写"判定要求", 不写不满足后果; 业务大白话, 不写字段名/表名。
// 改了后端规则记得同步这份文案。
//
// 当前已启用 14 条 (A1/A2/A3/A5/A6/A7/A8/A13/A17/A18/B1/B2/B3/B4)。
// 品牌中心/研发必选、检测费、装修费、核销明细核对、无票费用 等规则待财务确认口径后补充。

import React from 'react';
import { Card, Table, Typography } from 'antd';

// ====== 张俊 对外付款单 / 预付款单 审批判定规则 (系统内置, 只读展示) ======
// ref = 后端 hesi_audit_payment_rules.go (AuditPayment) 里正在跑的规则编号, 页面"规则号"列直接显示, 改后端时照编号同步本表。
const ZJ_AUDIT_RULES: { ref: string; cat: string; rule: string }[] = [
  { ref: 'A1', cat: '所属公司', rule: '所属公司（法人实体）必须填写' },
  { ref: 'A13', cat: '所属公司', rule: '付款事由涉及"阳光天际"时，所属公司必须是杭州松鲜鲜悦伍食品科技有限公司' },
  { ref: 'A2', cat: '部门', rule: '申请部门必须是末级部门（部门底下不能再挂下级）' },
  { ref: 'A6', cat: '收款', rule: '收款方式必须是银行账户（填"其他"等非银行账户会建议驳回）；新建收款方时银行账户信息要填齐全' },
  { ref: 'A7', cat: '收款', rule: '收款方不能与付款的所属公司同名（避免自己付给自己）' },
  { ref: 'A5', cat: '事由', rule: '付款事由 / 费用明细的消费事由必须填写，且不能出现"合计、小计"等笼统字样' },
  { ref: 'A3', cat: '客户', rule: '客户信息要选择（非线下业务统一选虚拟客户）' },
  { ref: 'A8', cat: '费用明细', rule: '至少要有一行费用明细（预付款单"先款后票"在发票未到前可以暂无明细）' },
  { ref: 'B2', cat: '发票', rule: '发票上的购买方必须是本付款单的所属公司（不一致会建议驳回）' },
  { ref: 'B1', cat: '发票', rule: '收款方名称要与发票上的开票方一致（对不上转人工核）' },
  { ref: 'B3', cat: '发票', rule: '发票张数要与单据填写的份数对得上（对不上转人工核）' },
  { ref: 'A18', cat: '金额', rule: '付款金额不能超过发票金额合计（超出转人工核）' },
  { ref: 'A17', cat: '合同', rule: '对外付款超过 2 万元，建议上传盖章版合同（未上传只提醒，不驳回）' },
  { ref: 'B4', cat: '防重复', rule: '同一收款方、相同金额若此前已付过，会提示疑似重复付款并附上原单号，请人工核对（付款单、预付款单都查）' },
];

const HesiBotRulesZhangJun: React.FC = () => {
  return (
    <Card title="对外付款单 · 审批判定规则（系统内置）" style={{ marginBottom: 16 }}>
      <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
        以下是机器人对付款单 / 预付款单的判定规则，据此给出待审批列表里的 AI 建议（建议通过 / 转人工核 / 建议驳回）。这是辅助参考，最终由审批人审批，机器人不替你点审批。
      </Typography.Paragraph>
      <Table
        columns={[
          { title: '规则号', dataIndex: 'ref', width: 90, align: 'center' as const },
          { title: '类别', dataIndex: 'cat', width: 120 },
          { title: '判定规则', dataIndex: 'rule' },
        ]}
        dataSource={ZJ_AUDIT_RULES}
        rowKey={(r) => r.rule}
        pagination={false}
        size="small"
      />
      <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginTop: 12, marginBottom: 0 }}>
        说明：付款单（票到付款 / 票到核销）走全部规则；预付款单（先款后票）因发票尚未到，发票相关项暂不适用。品牌中心 / 研发中心必选项、检测费、装修费、核销明细核对、无票费用等规则正在补充中。
      </Typography.Paragraph>
    </Card>
  );
};

export default HesiBotRulesZhangJun;
