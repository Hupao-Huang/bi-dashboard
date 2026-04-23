-- 财务报表 v2：DROP 老表 + 新建 3 张表
-- 2026-04-22

DROP TABLE IF EXISTS finance_report;

CREATE TABLE finance_report (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  year SMALLINT NOT NULL COMMENT '年份',
  month TINYINT NOT NULL COMMENT '月份 1-12，0=合计',
  department VARCHAR(30) NOT NULL COMMENT '渠道：汇总/电商/社媒/线下/分销/私域/国际零售/即时零售/糙有力量/中台',
  sub_channel VARCHAR(50) NOT NULL DEFAULT '' COMMENT '子渠道：天猫/拼多多/京东自营/有赞/微店等，仅 GMV 子渠道行使用',
  subject_code VARCHAR(50) NOT NULL COMMENT '稳定科目代码，跨年兼容（如 REV_MAIN/COST_GOODS/NET_PROFIT）',
  subject_name VARCHAR(100) NOT NULL COMMENT '科目显示名',
  subject_category VARCHAR(30) NOT NULL COMMENT '科目分组：GMV/收入/成本/毛利/销售费用/运营利润/管理费用/研发费用/利润总额/营业外/净利润',
  subject_level TINYINT NOT NULL COMMENT '层级：1=分组行 2=一级科目 3=子科目',
  parent_code VARCHAR(50) NOT NULL DEFAULT '' COMMENT '父级科目代码（层级 3 用于指向层级 2）',
  sort_order INT NOT NULL DEFAULT 0 COMMENT '在报表中的展示顺序',
  amount DECIMAL(18,4) NOT NULL DEFAULT 0 COMMENT '金额',
  ratio DECIMAL(10,6) DEFAULT NULL COMMENT '占比销售（Excel 的占比销售列原值）',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_report (year, month, department, sub_channel, subject_code),
  INDEX idx_dept_year (department, year, month),
  INDEX idx_subject (subject_code, year, month),
  INDEX idx_category (subject_category)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='财务管理损益表明细';

DROP TABLE IF EXISTS finance_subject_dict;

CREATE TABLE finance_subject_dict (
  subject_code VARCHAR(50) PRIMARY KEY COMMENT '稳定科目代码',
  subject_name VARCHAR(100) NOT NULL COMMENT '标准科目名',
  subject_category VARCHAR(30) NOT NULL COMMENT '科目分组',
  subject_level TINYINT NOT NULL COMMENT '层级 1/2/3',
  parent_code VARCHAR(50) NOT NULL DEFAULT '' COMMENT '父级代码',
  display_order INT NOT NULL DEFAULT 0 COMMENT '显示顺序',
  aliases JSON DEFAULT NULL COMMENT '历史别名数组：如 ["减：管理费用","减：管理费用（不可控成本）"]',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_category (subject_category, display_order),
  INDEX idx_level (subject_level)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='财务科目字典';

DROP TABLE IF EXISTS finance_import_log;

CREATE TABLE finance_import_log (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  year SMALLINT NOT NULL COMMENT '导入年份',
  filename VARCHAR(255) NOT NULL COMMENT '上传文件名',
  file_size BIGINT DEFAULT 0 COMMENT '文件字节数',
  md5 VARCHAR(32) DEFAULT '' COMMENT '文件MD5',
  sheet_count INT DEFAULT 0 COMMENT '处理的sheet数',
  row_count INT DEFAULT 0 COMMENT '实际入库行数',
  unmapped_subjects JSON DEFAULT NULL COMMENT '未匹配到字典的科目列表，留给财务确认',
  status VARCHAR(20) NOT NULL DEFAULT 'success' COMMENT 'success/partial/failed',
  error_msg TEXT COMMENT '错误信息',
  user_id INT DEFAULT 0 COMMENT '上传人 user_id，0=命令行',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_year_created (year, created_at),
  INDEX idx_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='财务报表导入日志';
