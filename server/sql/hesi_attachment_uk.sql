-- v0.94 hesi_flow_attachment 加 UK 防重复
-- audit 报告里说的 1 条 "重复" 其实是同附件在不同 free_id 下, 业务正确
-- 真 UK 应该是 (flow_id, attachment_type, file_id, free_id)
-- free_id 有 50% 是 NULL, MySQL UK 对 NULL 允许多个, 加上 INSERT IGNORE 双重保险

ALTER TABLE hesi_flow_attachment
  ADD UNIQUE KEY uk_attachment (flow_id, attachment_type, file_id, free_id);

SHOW CREATE TABLE hesi_flow_attachment;
