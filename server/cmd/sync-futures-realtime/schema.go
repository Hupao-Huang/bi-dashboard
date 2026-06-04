package main

import (
	"database/sql"
	"log"
)

// 启动时自动建快照表(CREATE IF NOT EXISTS, 首次部署免手动建表)
// 每个品种一行, 盘中被实时覆盖; 字段注释全中文, 与库内规范一致
const realtimeSchemaSQL = `
CREATE TABLE IF NOT EXISTS futures_quote_realtime (
  symbol_code VARCHAR(20) NOT NULL COMMENT '品种代码,关联futures_symbol',
  quote_date DATE NOT NULL COMMENT '行情所属交易日(新浪源)',
  quote_time VARCHAR(8) NOT NULL DEFAULT '' COMMENT '最新报价时间HH:MM:SS(新浪源,用于判定是否盘中实时)',
  last_price DECIMAL(14,4) NOT NULL DEFAULT 0 COMMENT '最新价(现价)',
  open_price DECIMAL(14,4) NOT NULL DEFAULT 0 COMMENT '今开盘',
  high_price DECIMAL(14,4) NOT NULL DEFAULT 0 COMMENT '当日最高',
  low_price DECIMAL(14,4) NOT NULL DEFAULT 0 COMMENT '当日最低',
  prev_settle DECIMAL(14,4) NOT NULL DEFAULT 0 COMMENT '昨结算价(盘中涨跌的计算基准)',
  volume BIGINT NOT NULL DEFAULT 0 COMMENT '当日累计成交量',
  open_interest BIGINT NOT NULL DEFAULT 0 COMMENT '当前持仓量',
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '本地入库时间',
  PRIMARY KEY (symbol_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='期货盘中实时快照(每品种一行,新浪源,每5分钟更新)';
`

func ensureRealtimeSchema(db *sql.DB) {
	if _, err := db.Exec(realtimeSchemaSQL); err != nil {
		log.Fatalf("建实时快照表失败: %v", err)
	}
	log.Println("快照表 schema 检查完成(CREATE IF NOT EXISTS)")
}
