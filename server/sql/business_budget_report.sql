-- 业务预决算报表（v0.58）
-- 跑哥 2026-04-30：每月一份快照，财务在前端增量上传
-- 数据源：Desktop/业务报表/YYYY年业务预决算报表.xlsx (4 年×多月快照)

CREATE TABLE IF NOT EXISTS business_budget_report (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  snapshot_year SMALLINT NOT NULL COMMENT '快照年份（财务出此报表的时间点）',
  snapshot_month TINYINT NOT NULL COMMENT '快照月份 1-12（财务出此报表的时间点）',
  year SMALLINT NOT NULL COMMENT '报表覆盖的业务年份（通常等于 snapshot_year）',
  channel VARCHAR(50) NOT NULL COMMENT '一级渠道：总/电商/私域/分销/社媒/线下/国际零售/中后台/经营指标',
  sub_channel VARCHAR(50) NOT NULL DEFAULT '' COMMENT '二级子渠道：TOC/TOB/礼品/非礼品/自营/小红书/视频号/华东/华南/重客等；一级 sheet 留空',
  subject VARCHAR(150) NOT NULL COMMENT '科目名（含原始缩进字符）',
  subject_level TINYINT NOT NULL DEFAULT 1 COMMENT '层级 1=一级科目 2=二级 3=三级（按 xlsx 缩进识别）',
  subject_category VARCHAR(30) NOT NULL DEFAULT '' COMMENT '分组：GMV数据/财务数据 等',
  parent_subject VARCHAR(150) NOT NULL DEFAULT '' COMMENT '父科目名',
  sort_order INT NOT NULL DEFAULT 0 COMMENT '在原 xlsx sheet 中的行序',
  period_month TINYINT NOT NULL COMMENT '数据所属期间 0=合计/年初 1-12=月份',
  budget_year_start DECIMAL(20,4) DEFAULT NULL COMMENT '预算-年初（仅 period_month=0 有意义）',
  ratio_year_start DECIMAL(10,6) DEFAULT NULL COMMENT '占比-年初',
  budget DECIMAL(20,4) DEFAULT NULL COMMENT '当月或合计预算',
  ratio_budget DECIMAL(10,6) DEFAULT NULL COMMENT '占比-预算',
  actual DECIMAL(20,4) DEFAULT NULL COMMENT '当月或合计实际',
  ratio_actual DECIMAL(10,6) DEFAULT NULL COMMENT '占比-实际',
  achievement_rate DECIMAL(10,6) DEFAULT NULL COMMENT '达成率=实际/预算（仅 period_month=0 有）',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_bbr (snapshot_year, snapshot_month, channel, sub_channel, subject, period_month),
  INDEX idx_snapshot (snapshot_year, snapshot_month),
  INDEX idx_year_chan (year, channel, sub_channel),
  INDEX idx_subject (subject)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='业务预决算报表（每月一份快照，预算 vs 实际 vs 达成率）';

-- 导入日志表（复用 finance_import_log 风格）
CREATE TABLE IF NOT EXISTS business_budget_import_log (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  snapshot_year SMALLINT NOT NULL,
  snapshot_month TINYINT NOT NULL,
  year SMALLINT NOT NULL,
  source_file VARCHAR(500) NOT NULL COMMENT '源 xlsx 文件名',
  rows_inserted INT NOT NULL DEFAULT 0,
  rows_updated INT NOT NULL DEFAULT 0,
  rows_deleted INT NOT NULL DEFAULT 0,
  imported_by VARCHAR(50) NOT NULL DEFAULT 'admin',
  status ENUM('preview','success','failed') NOT NULL DEFAULT 'success',
  error_msg TEXT DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_snapshot (snapshot_year, snapshot_month),
  INDEX idx_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='业务预决算报表导入日志';
