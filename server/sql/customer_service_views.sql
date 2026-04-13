-- 客服主题标准化视图（兼容层）
-- 原则：不改旧表名，通过标准命名 VIEW 供新报表/新接口使用
-- 命名：op_<platform>_custsvc_<subject>_daily

-- 天猫客服（旧表：op_tmall_service_*）
DROP VIEW IF EXISTS op_tmall_custsvc_inquiry_daily;
CREATE VIEW op_tmall_custsvc_inquiry_daily AS
SELECT * FROM op_tmall_service_inquiry;

DROP VIEW IF EXISTS op_tmall_custsvc_consult_daily;
CREATE VIEW op_tmall_custsvc_consult_daily AS
SELECT * FROM op_tmall_service_consult;

DROP VIEW IF EXISTS op_tmall_custsvc_avgprice_daily;
CREATE VIEW op_tmall_custsvc_avgprice_daily AS
SELECT * FROM op_tmall_service_avgprice;

DROP VIEW IF EXISTS op_tmall_custsvc_evaluation_daily;
CREATE VIEW op_tmall_custsvc_evaluation_daily AS
SELECT * FROM op_tmall_service_evaluation;

-- 拼多多客服（旧表：op_pdd_cs_*）
DROP VIEW IF EXISTS op_pdd_custsvc_service_daily;
CREATE VIEW op_pdd_custsvc_service_daily AS
SELECT * FROM op_pdd_cs_service_daily;

DROP VIEW IF EXISTS op_pdd_custsvc_sales_daily;
CREATE VIEW op_pdd_custsvc_sales_daily AS
SELECT * FROM op_pdd_cs_sales_daily;

-- 京东客户（当前已有规范表，增加同风格别名）
DROP VIEW IF EXISTS op_jd_custsvc_customer_daily;
CREATE VIEW op_jd_custsvc_customer_daily AS
SELECT * FROM op_jd_customer_daily;

DROP VIEW IF EXISTS op_jd_custsvc_customer_type_daily;
CREATE VIEW op_jd_custsvc_customer_type_daily AS
SELECT * FROM op_jd_customer_type_daily;
