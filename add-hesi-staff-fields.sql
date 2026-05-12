ALTER TABLE users
  ADD COLUMN hesi_staff_id VARCHAR(80) NULL COMMENT '合思员工staffId(用于精确匹配审批人)' AFTER dingtalk_userid,
  ADD COLUMN hesi_real_name VARCHAR(50) NULL COMMENT '合思系统真实姓名(可能与BI看板real_name不同)' AFTER hesi_staff_id,
  ALGORITHM=INSTANT;
