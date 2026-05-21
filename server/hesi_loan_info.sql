-- v1.70.5: 合思借款包(预付款单核销追踪)
-- 一张预付款单出纳付款后会生成一个借款包(loanInfo), 借款人后续报销单
-- 通过 WRITEOFF 还款记录核销借款包余额 remain, remain 归零后单据 archived
-- 接口: GET /api/openapi/v2/loanInfo/{flowId}
-- 参考: ekuaibao/open-platform-docs flows/get-flow-byLoanInfoId.md

CREATE TABLE IF NOT EXISTS hesi_loan_info (
  loan_info_id   VARCHAR(64)  NOT NULL                COMMENT '借款包ID(合思唯一标识)',
  flow_id        VARCHAR(64)  NOT NULL                COMMENT '对应预付款单flowId',
  total          DECIMAL(14,2) DEFAULT 0              COMMENT '借款包总金额',
  reserved       DECIMAL(14,2) DEFAULT 0              COMMENT '占用金额(未确认还款的金额)',
  remain         DECIMAL(14,2) DEFAULT 0              COMMENT '余额(剩余待核销金额, 0=已核销完)',
  repayment      DECIMAL(14,2) DEFAULT 0              COMMENT '确认已核销金额',
  state          VARCHAR(20)   DEFAULT NULL           COMMENT 'REPAID=待还款 PAID=已还清',
  owner_id       VARCHAR(64)   DEFAULT NULL           COMMENT '借款包所属人员工ID',
  corporation_id VARCHAR(64)   DEFAULT NULL           COMMENT '企业ID',
  loan_date      BIGINT        DEFAULT NULL           COMMENT '借款日期(unix ms)',
  repayment_date BIGINT        DEFAULT NULL           COMMENT '还款日期(unix ms)',
  active         TINYINT       DEFAULT 1              COMMENT '是否有效',
  raw_json       MEDIUMTEXT    DEFAULT NULL           COMMENT '原始接口返回',
  gmt_sync       DATETIME      DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '同步时间',
  PRIMARY KEY (loan_info_id),
  UNIQUE KEY uk_flow (flow_id),
  KEY idx_state (state),
  KEY idx_remain (remain)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='合思借款包(预付款单核销追踪) v1.70.5';
