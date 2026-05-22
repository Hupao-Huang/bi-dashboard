-- v1.73.0 W2 — BI 智能助手对话持久化 3 张表
-- 设计文档: docs/ai-assistant-design.md §4.3

CREATE TABLE IF NOT EXISTS ai_chat_session (
  id BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '会话ID',
  user_id BIGINT NOT NULL COMMENT '提问用户ID(users.id)',
  title VARCHAR(200) DEFAULT NULL COMMENT '会话标题(第一句话截前100字)',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '最后活跃时间',
  KEY idx_user_updated (user_id, updated_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI 智能助手会话';

CREATE TABLE IF NOT EXISTS ai_chat_message (
  id BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '消息ID',
  session_id BIGINT NOT NULL COMMENT '所属会话ID',
  role ENUM('user','assistant','system') NOT NULL COMMENT '消息角色',
  question TEXT DEFAULT NULL COMMENT '用户问题(role=user)',
  answer TEXT DEFAULT NULL COMMENT 'AI 回答(role=assistant)',
  intent_json TEXT DEFAULT NULL COMMENT '识别出的意图 JSON',
  source_type VARCHAR(20) DEFAULT NULL COMMENT '数据来源类型: api/sql/unknown',
  source_api VARCHAR(500) DEFAULT NULL COMMENT '调用的内部接口路径(路由命中时)',
  source_sql TEXT DEFAULT NULL COMMENT '生成的 SQL(fallback 时)',
  raw_data MEDIUMTEXT DEFAULT NULL COMMENT '原始数据 JSON(展开查看用)',
  confidence DECIMAL(4,2) DEFAULT NULL COMMENT '置信度 0.00-1.00',
  llm_model VARCHAR(50) DEFAULT NULL COMMENT '使用的 LLM 模型(glm-4.7 等)',
  llm_tokens INT DEFAULT 0 COMMENT '消耗的 token 数(成本统计)',
  duration_ms INT DEFAULT 0 COMMENT '总耗时毫秒',
  warning VARCHAR(500) DEFAULT NULL COMMENT '警告文案(置信度低/数据延迟等)',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  KEY idx_session (session_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI 智能助手消息';

CREATE TABLE IF NOT EXISTS ai_chat_feedback (
  id BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '反馈ID',
  message_id BIGINT NOT NULL COMMENT '所属消息ID',
  user_id BIGINT NOT NULL COMMENT '反馈用户ID',
  thumb TINYINT NOT NULL COMMENT '1=👍 -1=👎',
  comment TEXT DEFAULT NULL COMMENT '可选文字反馈',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '反馈时间',
  UNIQUE KEY uk_message_user (message_id, user_id),
  KEY idx_thumb (thumb, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI 回答反馈(👍/👎)';
