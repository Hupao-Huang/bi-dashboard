ALTER TABLE users
  ADD COLUMN dingtalk_real_name VARCHAR(50) NULL COMMENT '钉钉通讯录真名(企业认证姓名,可能与real_name昵称不同)' AFTER dingtalk_userid,
  ALGORITHM=INSTANT;
