package main

// 合思商旅 行程/订单管理业务对象同步 (跑哥 2026-06-11: 详情要看行程消费信息+支付方式)
// 数据源: /api/openapi/v2.1/datalink/TRAVEL_MANAGEMENT/searchOrders (按业务对象分页)
// 平台「行程管理」(ID01FilmFC62En) 下 21 个业务对象 (订单-火车/飞机/酒店/用车/餐饮... + 行程-*)
// 字段是 E_<父对象ID>_中文名 的扁平结构, 同步时剥前缀按中文名取关键列, 全量存 raw_json
// 用法: sync-hesi-orders        增量(近90天创建的单, 幂等覆盖)
//       sync-hesi-orders 0      全量
//       sync-hesi-orders 30     近30天

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"

	_ "github.com/go-sql-driver/mysql"
)

const (
	hesiBase         = "https://app.ekuaibao.com"
	travelPlatformID = "ID01FilmFC62En" // 行程管理平台 (2026-06-11 getPlatform 实查)
)

var httpCli = &http.Client{Timeout: 60 * time.Second}

func main() {
	unlock := importutil.AcquireLock("sync-hesi-orders")
	defer unlock()

	days := 90
	if len(os.Args) >= 2 {
		if v, err := strconv.Atoi(os.Args[1]); err == nil {
			days = v
		}
	}

	cfg, err := config.Load(`C:\Users\Administrator\bi-dashboard\server\config.json`)
	if err != nil {
		log.Fatalf("配置: %v", err)
	}
	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("数据库: %v", err)
	}
	defer db.Close()
	if err := ensureTable(db); err != nil {
		log.Fatalf("建表: %v", err)
	}

	token, err := getToken(cfg.Hesi.AppKey, cfg.Hesi.Secret)
	if err != nil {
		log.Fatalf("合思token: %v", err)
	}

	entities, err := listEntities(token)
	if err != nil {
		log.Fatalf("业务对象列表: %v", err)
	}

	startDate := ""
	if days > 0 {
		startDate = time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04:05")
	}

	total := 0
	for id, name := range entities {
		n, err := syncEntity(db, token, id, name, startDate)
		if err != nil {
			log.Printf("[%s] 同步失败: %v", name, err)
			continue
		}
		if n > 0 {
			fmt.Printf("[%s] 入库/更新 %d 条\n", name, n)
		}
		total += n
	}
	fmt.Printf("\n商旅行程/订单同步完成: 共 %d 条 (范围: %s)\n", total, map[bool]string{true: "全量", false: "近" + strconv.Itoa(days) + "天"}[days <= 0])
}

func getToken(appKey, secret string) (string, error) {
	b, _ := json.Marshal(map[string]string{"appKey": appKey, "appSecurity": secret})
	resp, err := httpCli.Post(hesiBase+"/api/openapi/v1/auth/getAccessToken", "application/json", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var r struct {
		Value struct {
			AccessToken string `json:"accessToken"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &r); err != nil || r.Value.AccessToken == "" {
		return "", fmt.Errorf("token 解析失败: %.200s", string(data))
	}
	return r.Value.AccessToken, nil
}

func listEntities(token string) (map[string]string, error) {
	resp, err := httpCli.Get(hesiBase + "/api/openapi/v2/datalink/entity/$" + travelPlatformID + "?accessToken=" + token)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var r struct {
		Items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("解析失败: %.200s", string(data))
	}
	m := map[string]string{}
	for _, it := range r.Items {
		m[it.ID] = it.Name
	}
	return m, nil
}

var fieldRe = regexp.MustCompile(`^E_[0-9A-Za-z]+_(.+)$`)

func syncEntity(db *sql.DB, token, entityID, entityName, startDate string) (int, error) {
	count := 0
	for start := 0; ; start += 100 {
		body := map[string]interface{}{"entityId": entityID, "start": start, "count": 100}
		if startDate != "" {
			body["startDate"] = startDate
		}
		bb, _ := json.Marshal(body)
		resp, err := httpCli.Post(hesiBase+"/api/openapi/v2.1/datalink/TRAVEL_MANAGEMENT/searchOrders?accessToken="+token,
			"application/json", bytes.NewReader(bb))
		if err != nil {
			return count, err
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var r struct {
			Items struct {
				Total int `json:"total"`
				Data  []struct {
					DataLink map[string]interface{} `json:"dataLink"`
				} `json:"data"`
			} `json:"items"`
		}
		if err := json.Unmarshal(data, &r); err != nil {
			return count, fmt.Errorf("解析失败: %.200s", string(data))
		}
		for _, d := range r.Items.Data {
			if err := saveOrder(db, entityID, entityName, d.DataLink); err != nil {
				log.Printf("[%s] 单条入库失败: %v", entityName, err)
				continue
			}
			count++
		}
		if len(r.Items.Data) < 100 {
			return count, nil
		}
	}
}

// fStr/fMoney/fPerson 从剥前缀后的字段 map 取值
func fStr(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func fMoney(m map[string]interface{}, key string) sql.NullFloat64 {
	mm, ok := m[key].(map[string]interface{})
	if !ok {
		return sql.NullFloat64{}
	}
	s, _ := mm["standard"].(string)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: f, Valid: true}
}

func fPerson(m map[string]interface{}, key string) string {
	switch v := m[key].(type) {
	case map[string]interface{}:
		n, _ := v["name"].(string)
		return n
	case []interface{}:
		names := []string{}
		for _, p := range v {
			if pm, ok := p.(map[string]interface{}); ok {
				if n, _ := pm["name"].(string); n != "" {
					names = append(names, n)
				}
			}
		}
		return strings.Join(names, "+")
	}
	return ""
}

// fCity 城市字段是 JSON 字符串 [{"key":"982","label":"句容"}] → 取 label
func fCity(m map[string]interface{}, key string) string {
	s := fStr(m, key)
	if s == "" || !strings.Contains(s, "label") {
		return s
	}
	var arr []map[string]interface{}
	if json.Unmarshal([]byte(s), &arr) == nil {
		labels := []string{}
		for _, a := range arr {
			if l, _ := a["label"].(string); l != "" {
				labels = append(labels, l)
			}
		}
		return strings.Join(labels, "/")
	}
	return s
}

func saveOrder(db *sql.DB, entityID, entityName string, dl map[string]interface{}) error {
	dataID, _ := dl["id"].(string)
	if dataID == "" {
		return fmt.Errorf("缺 dataLink.id")
	}
	// 剥 E_<父对象>_ 前缀, 按中文字段名平铺
	f := map[string]interface{}{}
	for k, v := range dl {
		if m := fieldRe.FindStringSubmatch(k); m != nil {
			f[m[1]] = v
		}
	}
	// 车次/舱型: 火车用 火车车次/火车坐席, 飞机用 航班号?/航班舱型, 取非空者
	tripNo := fStr(f, "火车车次")
	if tripNo == "" {
		tripNo = fStr(f, "航班号")
	}
	seat := fStr(f, "火车坐席")
	if seat == "" {
		seat = fStr(f, "航班舱型")
	}
	// raw_json 列是 MEDIUMTEXT(16MB), 不截断 — 按字节截会把 UTF-8 汉字砍半导致 1366 入库失败
	rawJSON, _ := json.Marshal(f)

	_, err := db.Exec(`INSERT INTO hesi_travel_order
		(data_id, entity_id, entity_name, code, name, order_no, ticket_no, req_code,
		 pay_method, corp_pay, personal_pay, order_amount, reimburse_status, over_standard,
		 travel_type, trip_no, seat, depart_station, arrive_station, depart_city, arrive_city,
		 traveler, order_state, order_type, book_platform, depart_time, arrive_time, raw_json)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		 pay_method=VALUES(pay_method), corp_pay=VALUES(corp_pay), personal_pay=VALUES(personal_pay),
		 order_amount=VALUES(order_amount), reimburse_status=VALUES(reimburse_status),
		 over_standard=VALUES(over_standard), order_state=VALUES(order_state), order_type=VALUES(order_type),
		 req_code=VALUES(req_code), trip_no=VALUES(trip_no), seat=VALUES(seat),
		 depart_time=VALUES(depart_time), arrive_time=VALUES(arrive_time), raw_json=VALUES(raw_json)`,
		dataID, entityID, entityName,
		fStr(f, "code"), fStr(f, "name"), fStr(f, "订单号"), fStr(f, "票号"), fStr(f, "申请单编号"),
		fStr(f, "支付方式"), fMoney(f, "企业支付"), fMoney(f, "个人支付"), fMoney(f, "订单金额"),
		fStr(f, "报销状态"), fStr(f, "是否超标"), fStr(f, "出行类型"),
		tripNo, seat, fStr(f, "出发车站"), fStr(f, "到达车站"),
		fCity(f, "出发地"), fCity(f, "到达地"), fPerson(f, "出行人"),
		fStr(f, "订单状态"), fStr(f, "订单类型"), fStr(f, "订票平台"),
		fStr(f, "出发时间"), fStr(f, "到达时间"), string(rawJSON))
	return err
}

func ensureTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS hesi_travel_order (
		id BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT '主键',
		data_id VARCHAR(64) NOT NULL COMMENT '合思业务对象数据ID(dataLink.id)',
		entity_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '业务对象模板ID',
		entity_name VARCHAR(32) NOT NULL DEFAULT '' COMMENT '业务对象名(火车/飞机/酒店/用车/行程等)',
		code VARCHAR(128) NOT NULL DEFAULT '' COMMENT '业务对象编号',
		name VARCHAR(255) NOT NULL DEFAULT '' COMMENT '名称(一般是 出发-到达)',
		order_no VARCHAR(64) NOT NULL DEFAULT '' COMMENT '订单号',
		ticket_no VARCHAR(64) NOT NULL DEFAULT '' COMMENT '票号',
		req_code VARCHAR(32) NOT NULL DEFAULT '' COMMENT '关联申请单编号(S26xxxx, 挂回出差申请单)',
		pay_method VARCHAR(16) NOT NULL DEFAULT '' COMMENT '支付方式(企业支付/个人支付)',
		corp_pay DECIMAL(12,2) NULL COMMENT '企业支付金额',
		personal_pay DECIMAL(12,2) NULL COMMENT '个人支付金额',
		order_amount DECIMAL(12,2) NULL COMMENT '订单金额',
		reimburse_status VARCHAR(16) NOT NULL DEFAULT '' COMMENT '报销状态(未报销/已报销)',
		over_standard VARCHAR(8) NOT NULL DEFAULT '' COMMENT '是否超标(合思自判)',
		travel_type VARCHAR(16) NOT NULL DEFAULT '' COMMENT '出行类型(因公/因私)',
		trip_no VARCHAR(32) NOT NULL DEFAULT '' COMMENT '车次/航班号',
		seat VARCHAR(32) NOT NULL DEFAULT '' COMMENT '坐席/舱型',
		depart_station VARCHAR(64) NOT NULL DEFAULT '' COMMENT '出发车站',
		arrive_station VARCHAR(64) NOT NULL DEFAULT '' COMMENT '到达车站',
		depart_city VARCHAR(64) NOT NULL DEFAULT '' COMMENT '出发城市',
		arrive_city VARCHAR(64) NOT NULL DEFAULT '' COMMENT '到达城市',
		traveler VARCHAR(64) NOT NULL DEFAULT '' COMMENT '出行人(多人+连接)',
		order_state VARCHAR(16) NOT NULL DEFAULT '' COMMENT '订单状态(出票/退票/改签)',
		order_type VARCHAR(16) NOT NULL DEFAULT '' COMMENT '订单类型',
		book_platform VARCHAR(32) NOT NULL DEFAULT '' COMMENT '订票平台(合思商城等)',
		depart_time VARCHAR(32) NOT NULL DEFAULT '' COMMENT '出发时间(原样文本)',
		arrive_time VARCHAR(32) NOT NULL DEFAULT '' COMMENT '到达时间(原样文本)',
		raw_json MEDIUMTEXT COMMENT '剥前缀后的全量字段JSON(备查)',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '入库时间',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
		UNIQUE KEY uk_data_id (data_id),
		KEY idx_req_code (req_code),
		KEY idx_trip (trip_no)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='合思商旅行程/订单业务对象(支付方式/报销状态/超标, 详情行程消费信息用)'`)
	return err
}
