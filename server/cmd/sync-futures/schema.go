package main

import (
	"database/sql"
	"log"
)

// 启动时自动建表 + 灌品种字典（首次部署最省心）
// 字段说明全用中文，跟跑哥的数据库规范保持一致

const schemaSQL = `
CREATE TABLE IF NOT EXISTS futures_symbol (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  symbol_code VARCHAR(20) NOT NULL COMMENT '新浪期货代码,主连合约,如M0/Y0/SR0/L0',
  name_cn VARCHAR(50) NOT NULL COMMENT '中文名,如豆粕',
  exchange VARCHAR(10) NOT NULL COMMENT '交易所:DCE大商所/CZCE郑商所/SHFE上期所/INE上期能源/CFFEX中金所',
  category VARCHAR(20) NOT NULL COMMENT '分类:material主要原料/package包材原料/macro大宗商品概览',
  unit VARCHAR(20) DEFAULT NULL COMMENT '计价单位,如元/吨',
  business_tag VARCHAR(100) DEFAULT NULL COMMENT '关联业务,如调味品原料/塑料瓶包材',
  sort_order INT NOT NULL DEFAULT 0 COMMENT '排序,小的在前',
  is_enabled TINYINT NOT NULL DEFAULT 1 COMMENT '是否启用',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_symbol_code (symbol_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='期货品种字典';

CREATE TABLE IF NOT EXISTS futures_price_daily (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  symbol_code VARCHAR(20) NOT NULL COMMENT '品种代码',
  trade_date DATE NOT NULL COMMENT '交易日',
  open_price DECIMAL(14,4) NOT NULL DEFAULT 0 COMMENT '开盘价',
  high_price DECIMAL(14,4) NOT NULL DEFAULT 0 COMMENT '最高价',
  low_price DECIMAL(14,4) NOT NULL DEFAULT 0 COMMENT '最低价',
  close_price DECIMAL(14,4) NOT NULL DEFAULT 0 COMMENT '收盘价',
  volume BIGINT NOT NULL DEFAULT 0 COMMENT '成交量',
  open_interest BIGINT NOT NULL DEFAULT 0 COMMENT '持仓量',
  settle_price DECIMAL(14,4) NOT NULL DEFAULT 0 COMMENT '结算价(预留,新浪源暂无)',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_symbol_date (symbol_code, trade_date),
  KEY idx_date (trade_date),
  KEY idx_symbol (symbol_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='期货日线';
`

// 16 个品种主连合约字典（新浪源代码以0结尾 = 主力连续合约）
type symbolSeed struct {
	Code        string
	NameCN      string
	Exchange    string
	Category    string
	Unit        string
	BusinessTag string
	SortOrder   int
}

var seedSymbols = []symbolSeed{
	// 主要原料：调味品行业核心
	{"M0", "豆粕", "DCE", "material", "元/吨", "酱油/味精原料", 11},
	{"Y0", "豆油", "DCE", "material", "元/吨", "食用油", 12},
	{"P0", "棕榈油", "DCE", "material", "元/吨", "食用油", 13},
	{"OI0", "菜籽油", "CZCE", "material", "元/吨", "食用油", 14},
	{"RM0", "菜籽粕", "CZCE", "material", "元/吨", "饲料/调味品", 15},
	{"SR0", "白糖", "CZCE", "material", "元/吨", "调味用糖", 16},
	{"C0", "玉米", "DCE", "material", "元/吨", "淀粉/淀粉糖", 17},

	// 包材原料
	{"L0", "聚乙烯", "DCE", "package", "元/吨", "塑料瓶/塑料袋", 21},
	{"PP0", "聚丙烯", "DCE", "package", "元/吨", "瓶盖/塑料瓶", 22},
	{"FG0", "玻璃", "CZCE", "package", "元/吨", "玻璃瓶", 23},
	{"SP0", "纸浆", "SHFE", "package", "元/吨", "包装纸箱", 24},

	// 大宗商品概览
	{"SC0", "原油", "INE", "macro", "元/桶", "宏观参考", 31},
	{"AU0", "黄金", "SHFE", "macro", "元/克", "避险/通胀对冲", 32},
	{"RB0", "螺纹钢", "SHFE", "macro", "元/吨", "建筑/工业景气", 33},
	{"CU0", "铜", "SHFE", "macro", "元/吨", "工业景气", 34},
	{"IF0", "沪深300", "CFFEX", "macro", "指数点", "大盘股指", 35},
}

func ensureSchema(db *sql.DB) {
	// 1. 建表
	for _, stmt := range splitSQLStatements(schemaSQL) {
		if _, err := db.Exec(stmt); err != nil {
			log.Fatalf("建表失败: %v\nSQL: %s", err, stmt)
		}
	}
	log.Println("schema 检查完成（CREATE IF NOT EXISTS）")

	// 2. 灌品种字典（INSERT IGNORE 不覆盖已有）
	stmt, err := db.Prepare(`INSERT IGNORE INTO futures_symbol
		(symbol_code, name_cn, exchange, category, unit, business_tag, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		log.Fatalf("准备品种字典插入语句失败: %v", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, s := range seedSymbols {
		res, err := stmt.Exec(s.Code, s.NameCN, s.Exchange, s.Category, s.Unit, s.BusinessTag, s.SortOrder)
		if err != nil {
			log.Printf("灌入 %s 失败: %v", s.Code, err)
			continue
		}
		if n, _ := res.RowsAffected(); n > 0 {
			inserted++
		}
	}
	if inserted > 0 {
		log.Printf("品种字典：新增 %d 条（已有的跳过）", inserted)
	} else {
		log.Println("品种字典：全部已存在，无新增")
	}
}

func splitSQLStatements(s string) []string {
	out := []string{}
	cur := ""
	for _, ch := range s {
		cur += string(ch)
		if ch == ';' {
			trimmed := trimSpace(cur)
			if trimmed != "" {
				out = append(out, trimmed)
			}
			cur = ""
		}
	}
	if trimmed := trimSpace(cur); trimmed != "" {
		out = append(out, trimmed)
	}
	return out
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r' || s[start] == ';') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r' || s[end-1] == ';') {
		end--
	}
	return s[start:end]
}
