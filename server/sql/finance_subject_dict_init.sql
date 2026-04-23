-- 财务科目字典初始化
-- 2026-04-22
-- Level 1 分组行 / Level 2 一级科目 / Level 3 子科目
-- GMV 子渠道统一用 GMV_SUB，由 sub_channel 字段区分具体平台

TRUNCATE TABLE finance_subject_dict;

-- ========== Level 1 分组 ==========
INSERT INTO finance_subject_dict (subject_code, subject_name, subject_category, subject_level, parent_code, display_order) VALUES
  ('GMV_GROUP',  'GMV数据',   'GMV',   1, '', 100),
  ('FIN_GROUP',  '财务数据',  '财务',  1, '', 200);

-- ========== Level 2 一级科目 ==========
-- 所有 sheet 都有
INSERT INTO finance_subject_dict (subject_code, subject_name, subject_category, subject_level, parent_code, display_order) VALUES
  ('GMV_TOTAL',     'GMV合计',       'GMV',         2, 'GMV_GROUP', 110),
  ('REV_MAIN',      '一、营业收入',  '收入',        2, 'FIN_GROUP', 210),
  ('COST_MAIN',     '减：营业成本',  '成本',        2, 'FIN_GROUP', 220),
  ('PROFIT_GROSS',  '营业毛利',      '毛利',        2, 'FIN_GROUP', 230),
  ('SALES_EXP',     '减：销售费用',  '销售费用',    2, 'FIN_GROUP', 240),
  ('PROFIT_OP',     '运营利润',      '运营利润',    2, 'FIN_GROUP', 250),
  ('MGMT_EXP',      '减：管理费用',  '管理费用',    2, 'FIN_GROUP', 260),
  ('NET_PROFIT',    '二：净利润',    '净利润',      2, 'FIN_GROUP', 290);

-- 只汇总表有
INSERT INTO finance_subject_dict (subject_code, subject_name, subject_category, subject_level, parent_code, display_order) VALUES
  ('RETURN',        '售退',              'GMV',         2, 'GMV_GROUP', 120),
  ('REV_TOTAL',     '营业额合计',        'GMV',         2, 'GMV_GROUP', 130),
  ('RND_EXP',       '减：研发费用',      '研发费用',    2, 'FIN_GROUP', 270),
  ('PROFIT_TOTAL',  '利润总额',          '利润总额',    2, 'FIN_GROUP', 275),
  ('NON_REV',       '加：营业外收入',    '营业外',      2, 'FIN_GROUP', 280),
  ('NON_EXP',       '减：营业外支出',    '营业外',      2, 'FIN_GROUP', 281),
  ('LOSS_SCRAP',    '其中：报废损失',    '营业外',      2, 'FIN_GROUP', 282),
  ('TAX_SURCHARGE', '税金及附加',        '税费',        2, 'FIN_GROUP', 285),
  ('TAX_INCOME',    '所得税费用',        '税费',        2, 'FIN_GROUP', 288),
  ('VAT_EXTRA',     '补充数据（增值税）','税费',        2, 'FIN_GROUP', 295);

-- Level 2 别名
UPDATE finance_subject_dict SET aliases = JSON_ARRAY('减：管理费用（不可控成本）') WHERE subject_code = 'MGMT_EXP';
UPDATE finance_subject_dict SET aliases = JSON_ARRAY('营业利润')                    WHERE subject_code = 'PROFIT_TOTAL';

-- ========== Level 3 GMV 子渠道（统一用 GMV_SUB） ==========
INSERT INTO finance_subject_dict (subject_code, subject_name, subject_category, subject_level, parent_code, display_order) VALUES
  ('GMV_SUB', '子渠道GMV', 'GMV', 3, 'GMV_TOTAL', 115);

-- ========== Level 3 营业成本 COST_MAIN 子项 ==========
INSERT INTO finance_subject_dict (subject_code, subject_name, subject_category, subject_level, parent_code, display_order) VALUES
  ('COST_MAIN.商品成本',        '商品成本',        '成本', 3, 'COST_MAIN', 221),
  ('COST_MAIN.赠品及样品',      '赠品及样品',      '成本', 3, 'COST_MAIN', 222),
  ('COST_MAIN.分佣佣金',        '分佣佣金',        '成本', 3, 'COST_MAIN', 223),
  ('COST_MAIN.KA进场及条码费用','KA进场及条码费用','成本', 3, 'COST_MAIN', 224),
  ('COST_MAIN.仓储物流费用',    '仓储物流费用',    '成本', 3, 'COST_MAIN', 225),
  ('COST_MAIN.物流费用',        '物流费用',        '成本', 3, 'COST_MAIN', 226),
  ('COST_MAIN.临时工费用',      '临时工费用',      '成本', 3, 'COST_MAIN', 227),
  ('COST_MAIN.发货耗材成本',    '发货耗材成本',    '成本', 3, 'COST_MAIN', 228);

UPDATE finance_subject_dict SET aliases = JSON_ARRAY('1、物流费用')       WHERE subject_code = 'COST_MAIN.物流费用';
UPDATE finance_subject_dict SET aliases = JSON_ARRAY('2、临时工费用')     WHERE subject_code = 'COST_MAIN.临时工费用';
UPDATE finance_subject_dict SET aliases = JSON_ARRAY('3、发货耗材成本')   WHERE subject_code = 'COST_MAIN.发货耗材成本';

-- ========== Level 3 销售费用 SALES_EXP 子项 ==========
INSERT INTO finance_subject_dict (subject_code, subject_name, subject_category, subject_level, parent_code, display_order) VALUES
  ('SALES_EXP.人工成本',       '人工成本',       '销售费用', 3, 'SALES_EXP', 241),
  ('SALES_EXP.福利费用',       '福利费用',       '销售费用', 3, 'SALES_EXP', 242),
  ('SALES_EXP.平台推广费',     '平台推广费',     '销售费用', 3, 'SALES_EXP', 243),
  ('SALES_EXP.平台佣金及手续费','平台佣金及手续费','销售费用',3, 'SALES_EXP', 244),
  ('SALES_EXP.KA陈列及物料费用','KA陈列及物料费用','销售费用',3, 'SALES_EXP', 245),
  ('SALES_EXP.KA临时促销员费用','KA临时促销员费用','销售费用',3, 'SALES_EXP', 246),
  ('SALES_EXP.广告宣传费用',   '广告宣传费用',   '销售费用', 3, 'SALES_EXP', 247),
  ('SALES_EXP.品牌费用',       '品牌费用',       '销售费用', 3, 'SALES_EXP', 248),
  ('SALES_EXP.业务费用',       '业务费用',       '销售费用', 3, 'SALES_EXP', 249),
  ('SALES_EXP.房租',           '房租',           '销售费用', 3, 'SALES_EXP', 250),
  ('SALES_EXP.售后费用',       '售后费用',       '销售费用', 3, 'SALES_EXP', 251),
  ('SALES_EXP.其他',           '其他',           '销售费用', 3, 'SALES_EXP', 252);

UPDATE finance_subject_dict SET aliases = JSON_ARRAY('品牌费用（集团）') WHERE subject_code = 'SALES_EXP.品牌费用';

-- ========== Level 3 管理费用 MGMT_EXP 子项（渠道 sheet 粗分版） ==========
INSERT INTO finance_subject_dict (subject_code, subject_name, subject_category, subject_level, parent_code, display_order) VALUES
  ('MGMT_EXP.品牌基础建设费用','品牌基础建设费用','管理费用',3, 'MGMT_EXP', 261),
  ('MGMT_EXP.供应链基础费用',  '供应链基础费用',  '管理费用',3, 'MGMT_EXP', 262),
  ('MGMT_EXP.总部管理费用',    '总部管理费用',    '管理费用',3, 'MGMT_EXP', 263),
  ('MGMT_EXP.客服相关费用',    '客服相关费用',    '管理费用',3, 'MGMT_EXP', 264);

-- ========== Level 3 管理费用 MGMT_EXP 子项（汇总表细分版） ==========
INSERT INTO finance_subject_dict (subject_code, subject_name, subject_category, subject_level, parent_code, display_order) VALUES
  ('MGMT_EXP.人工成本',     '人工成本',     '管理费用', 3, 'MGMT_EXP', 265),
  ('MGMT_EXP.福利费用',     '福利费用',     '管理费用', 3, 'MGMT_EXP', 266),
  ('MGMT_EXP.房租',         '房租',         '管理费用', 3, 'MGMT_EXP', 267),
  ('MGMT_EXP.办公成本',     '办公成本',     '管理费用', 3, 'MGMT_EXP', 268),
  ('MGMT_EXP.仓储物流费用', '仓储物流费用', '管理费用', 3, 'MGMT_EXP', 269),
  ('MGMT_EXP.差旅费用',     '差旅费用',     '管理费用', 3, 'MGMT_EXP', 270),
  ('MGMT_EXP.招待费用',     '招待费用',     '管理费用', 3, 'MGMT_EXP', 271),
  ('MGMT_EXP.研发费用',     '研发费用',     '管理费用', 3, 'MGMT_EXP', 272),
  ('MGMT_EXP.车辆费用',     '车辆费用',     '管理费用', 3, 'MGMT_EXP', 273),
  ('MGMT_EXP.系统使用费用', '系统使用费用', '管理费用', 3, 'MGMT_EXP', 274),
  ('MGMT_EXP.中介费用',     '中介费用',     '管理费用', 3, 'MGMT_EXP', 275),
  ('MGMT_EXP.累计折旧',     '累计折旧',     '管理费用', 3, 'MGMT_EXP', 276),
  ('MGMT_EXP.检测费用',     '检测费用',     '管理费用', 3, 'MGMT_EXP', 277),
  ('MGMT_EXP.财务费用',     '财务费用',     '管理费用', 3, 'MGMT_EXP', 278),
  ('MGMT_EXP.其他',         '其他',         '管理费用', 3, 'MGMT_EXP', 279);

UPDATE finance_subject_dict SET aliases = JSON_ARRAY('人工成本1') WHERE subject_code = 'MGMT_EXP.人工成本';
UPDATE finance_subject_dict SET aliases = JSON_ARRAY('福利费用1') WHERE subject_code = 'MGMT_EXP.福利费用';
UPDATE finance_subject_dict SET aliases = JSON_ARRAY('其他1')     WHERE subject_code = 'MGMT_EXP.其他';

SELECT COUNT(*) AS total, subject_level, COUNT(*) AS cnt FROM finance_subject_dict GROUP BY subject_level;
