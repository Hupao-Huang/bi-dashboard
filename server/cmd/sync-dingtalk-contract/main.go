// sync-dingtalk-contract: 同步钉钉花名册"合同公司"(劳动主体) 字段到本地表
// 用法: sync-dingtalk-contract
//
// 流程:
//   1. 调合思 staffs 拉全员 (id/name/cellphone) 880 条
//   2. 调钉钉 queryonjob 分页拿全员企业 userid
//   3. 调钉钉 v2/list 100 个一批拉 sys00-姓名/sys00-手机号/sys05-contractCompanyName
//   4. 用手机号桥接合思员工 ↔ 钉钉员工
//   5. UPSERT 进 hesi_employee_contract_company 表
//
// 业务用途: 费控管理"日常报销单"详情弹窗校验申请人填的"法人实体"
// 跟钉钉花名册"合同公司"是否一致 (v1.75.0 起新功能)
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"bi-dashboard/internal/config"
)

const (
	hesiAPIBase = "https://app.ekuaibao.com"
	dingAPIBase = "https://oapi.dingtalk.com"
)

var httpClient = &http.Client{Timeout: 60 * time.Second}

// ============================================================================
// 合思员工字典
// ============================================================================

type hesiStaff struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	Cellphone string `json:"cellphone"`
	Active    bool   `json:"active"`
}

func getHesiToken(cfg config.HesiConfig) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"appKey":      cfg.AppKey,
		"appSecurity": cfg.Secret,
	})
	resp, err := httpClient.Post(hesiAPIBase+"/api/openapi/v1/auth/getAccessToken", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var parsed struct {
		Value struct {
			AccessToken string `json:"accessToken"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if parsed.Value.AccessToken == "" {
		return "", fmt.Errorf("合思 token 为空")
	}
	return parsed.Value.AccessToken, nil
}

// fetchHesiStaffs 拉合思全员字典 (count=1000 上限, 当前 880 < 1000 一次拉完)
func fetchHesiStaffs(cfg config.HesiConfig) ([]hesiStaff, error) {
	token, err := getHesiToken(cfg)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/api/openapi/v2/staffs?accessToken=%s&start=0&count=1000", hesiAPIBase, token)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("合思 staffs HTTP %d: %s", resp.StatusCode, string(data[:min(200, len(data))]))
	}
	var parsed struct {
		Count int         `json:"count"`
		Items []hesiStaff `json:"items"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	return parsed.Items, nil
}

// ============================================================================
// 钉钉智能人事接口
// ============================================================================

func getDingToken(appKey, appSecret string) (string, error) {
	url := fmt.Sprintf("%s/gettoken?appkey=%s&appsecret=%s", dingAPIBase, appKey, appSecret)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var parsed struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if parsed.ErrCode != 0 {
		return "", fmt.Errorf("钉钉 gettoken errcode=%d msg=%s", parsed.ErrCode, parsed.ErrMsg)
	}
	return parsed.AccessToken, nil
}

// queryOnJobUseridList 分页拉钉钉在职员工 userid (status_list=2,3,5,-1)
// 2=正式 3=试用 5=待离职 -1=无状态
func queryOnJobUseridList(token string) ([]string, error) {
	var all []string
	offset := 0
	size := 50 // 钉钉上限
	for {
		body, _ := json.Marshal(map[string]interface{}{
			"status_list": "2,3,5,-1",
			"offset":      offset,
			"size":        size,
		})
		url := fmt.Sprintf("%s/topapi/smartwork/hrm/employee/queryonjob?access_token=%s", dingAPIBase, token)
		resp, err := httpClient.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("queryonjob HTTP: %w", err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var parsed struct {
			ErrCode int    `json:"errcode"`
			ErrMsg  string `json:"errmsg"`
			Result  struct {
				DataList   []string `json:"data_list"`
				NextCursor int      `json:"next_cursor"`
			} `json:"result"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("queryonjob unmarshal: %w body=%s", err, string(data[:min(200, len(data))]))
		}
		if parsed.ErrCode != 0 {
			return nil, fmt.Errorf("queryonjob errcode=%d msg=%s", parsed.ErrCode, parsed.ErrMsg)
		}
		all = append(all, parsed.Result.DataList...)
		log.Printf("[ding] queryonjob offset=%d 拉到 %d, 累计 %d (next_cursor=%d)", offset, len(parsed.Result.DataList), len(all), parsed.Result.NextCursor)
		// 翻页终止: 只看 next_cursor==0 (钉钉文档: 0=无更多)
		// 不能用 len<size 判断, 钉钉每页可能返不满 size 但仍有下一页 (5/26 翻车: 第三页返 47<50 但 next_cursor=538491 还有人)
		if parsed.Result.NextCursor == 0 {
			break
		}
		offset = parsed.Result.NextCursor
		time.Sleep(200 * time.Millisecond) // QPS 防撞
	}
	return all, nil
}

// dingEmployee 钉钉花名册一条员工 (只取 sys00 姓名/手机号 + sys05 合同公司/类型/到期日)
type dingEmployee struct {
	UserID              string
	Name                string
	Mobile              string
	ContractCompanyName string
	ContractType        string
	ContractEndTime     string // yyyy-MM-dd
}

// fetchDingEmployees 100 个 userid 一批查 v2/list, 解析 sys05 合同字段
func fetchDingEmployees(token string, agentID int64, userIDs []string) ([]dingEmployee, error) {
	var result []dingEmployee
	const batchSize = 100
	for i := 0; i < len(userIDs); i += batchSize {
		end := i + batchSize
		if end > len(userIDs) {
			end = len(userIDs)
		}
		body, _ := json.Marshal(map[string]interface{}{
			"userid_list": strings.Join(userIDs[i:end], ","),
			"agentid":     agentID,
			// 钉钉 field_code 是英文格式 (sys00-name 不是 "姓名")
			"field_filter_list": "sys00-name,sys00-mobile,sys05-contractCompanyName,sys05-contractType,sys05-nowContractEndTime",
		})
		url := fmt.Sprintf("%s/topapi/smartwork/hrm/employee/v2/list?access_token=%s", dingAPIBase, token)
		resp, err := httpClient.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("v2/list HTTP: %w", err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var parsed struct {
			ErrCode int    `json:"errcode"`
			ErrMsg  string `json:"errmsg"`
			SubMsg  string `json:"sub_msg"`
			Result  []struct {
				UserID        string `json:"userid"`
				FieldDataList []struct {
					FieldCode      string `json:"field_code"`
					FieldName      string `json:"field_name"`
					FieldValueList []struct {
						Value string `json:"value"`
						Label string `json:"label"`
					} `json:"field_value_list"`
				} `json:"field_data_list"`
			} `json:"result"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("v2/list unmarshal: %w body=%s", err, string(data[:min(300, len(data))]))
		}
		if parsed.ErrCode != 0 {
			return nil, fmt.Errorf("v2/list errcode=%d msg=%s sub=%s", parsed.ErrCode, parsed.ErrMsg, parsed.SubMsg)
		}
		for _, emp := range parsed.Result {
			e := dingEmployee{UserID: emp.UserID}
			for _, f := range emp.FieldDataList {
				if len(f.FieldValueList) == 0 {
					continue
				}
				v := f.FieldValueList[0].Value
				if v == "" {
					continue
				}
				switch f.FieldCode {
				case "sys00-name":
					e.Name = v
				case "sys00-mobile":
					// 钉钉返回 "+86-13357134296", 剥离国家码前缀对齐合思格式
					mob := strings.TrimSpace(v)
					if strings.HasPrefix(mob, "+86-") {
						mob = mob[4:]
					}
					if strings.HasPrefix(mob, "+86") {
						mob = mob[3:]
					}
					e.Mobile = mob
				case "sys05-contractCompanyName":
					e.ContractCompanyName = v
				case "sys05-contractType":
					e.ContractType = v
				case "sys05-nowContractEndTime":
					if len(v) >= 10 {
						e.ContractEndTime = v[:10] // yyyy-MM-dd
					}
				}
			}
			result = append(result, e)
		}
		log.Printf("[ding] v2/list batch %d-%d 拉到 %d", i, end, len(parsed.Result))
		time.Sleep(500 * time.Millisecond) // 钉钉 QPS 防撞
	}
	return result, nil
}

// ============================================================================
// 桥接 + 写库
// ============================================================================

type contractRow struct {
	HesiStaffID    string
	HesiName       string
	HesiCellphone  string
	DingUserID     string
	DingName       string
	ContractCompay string
	ContractType   string
	ContractEnd    sql.NullString
	MatchMethod    string
}

func matchAndBuild(hesi []hesiStaff, ding []dingEmployee) []contractRow {
	// 钉钉手机号 → 钉钉员工
	dingByMobile := make(map[string]*dingEmployee)
	for i := range ding {
		if ding[i].Mobile != "" {
			dingByMobile[ding[i].Mobile] = &ding[i]
		}
	}
	// 钉钉姓名 → 钉钉员工 (兜底, 手机号匹配不上时用)
	dingByName := make(map[string]*dingEmployee)
	for i := range ding {
		if ding[i].Name != "" {
			dingByName[ding[i].Name] = &ding[i]
		}
	}

	var rows []contractRow
	matchMobile, matchName, matchNone := 0, 0, 0
	for _, s := range hesi {
		row := contractRow{
			HesiStaffID:   s.ID,
			HesiName:      s.Name,
			HesiCellphone: strings.TrimSpace(s.Cellphone),
		}
		var d *dingEmployee
		if row.HesiCellphone != "" {
			if e, ok := dingByMobile[row.HesiCellphone]; ok {
				d = e
				row.MatchMethod = "mobile"
				matchMobile++
			}
		}
		if d == nil && s.Name != "" {
			if e, ok := dingByName[s.Name]; ok {
				d = e
				row.MatchMethod = "name"
				matchName++
			}
		}
		if d == nil {
			row.MatchMethod = "none"
			matchNone++
			rows = append(rows, row)
			continue
		}
		row.DingUserID = d.UserID
		row.DingName = d.Name
		row.ContractCompay = d.ContractCompanyName
		row.ContractType = d.ContractType
		if d.ContractEndTime != "" {
			row.ContractEnd = sql.NullString{String: d.ContractEndTime, Valid: true}
		}
		rows = append(rows, row)
	}
	log.Printf("[match] 合思员工 %d, 钉钉手机号匹配 %d, 姓名兜底 %d, 未匹配 %d", len(hesi), matchMobile, matchName, matchNone)
	return rows
}

func upsertContracts(db *sql.DB, rows []contractRow) error {
	stmt := `INSERT INTO hesi_employee_contract_company
		(hesi_staff_id, hesi_name, hesi_cellphone, dingtalk_userid, dingtalk_name,
		 contract_company_name, contract_type, contract_end_time, match_method, last_sync_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
		ON DUPLICATE KEY UPDATE
			hesi_name=VALUES(hesi_name),
			hesi_cellphone=VALUES(hesi_cellphone),
			dingtalk_userid=VALUES(dingtalk_userid),
			dingtalk_name=VALUES(dingtalk_name),
			contract_company_name=VALUES(contract_company_name),
			contract_type=VALUES(contract_type),
			contract_end_time=VALUES(contract_end_time),
			match_method=VALUES(match_method),
			last_sync_at=NOW()`
	for _, r := range rows {
		_, err := db.Exec(stmt,
			r.HesiStaffID,
			nullableStr(r.HesiName),
			nullableStr(r.HesiCellphone),
			nullableStr(r.DingUserID),
			nullableStr(r.DingName),
			nullableStr(r.ContractCompay),
			nullableStr(r.ContractType),
			r.ContractEnd,
			r.MatchMethod,
		)
		if err != nil {
			return fmt.Errorf("upsert %s: %w", r.HesiStaffID, err)
		}
	}
	return nil
}

func nullableStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ============================================================================
// main
// ============================================================================

func main() {
	start := time.Now()
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if cfg.DingTalk.NotifyAgentID == 0 {
		log.Fatalf("config.dingtalk.notify_agent_id 未配置, 智能人事接口需要 AgentId")
	}
	if cfg.DingTalk.NotifyAppKey == "" || cfg.DingTalk.NotifyAppSecret == "" {
		log.Fatalf("config.dingtalk.notify_app_key/secret 未配置")
	}

	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	// 1. 合思员工字典
	log.Println("[step 1] 拉合思 staffs 字典...")
	hesi, err := fetchHesiStaffs(cfg.Hesi)
	if err != nil {
		log.Fatalf("fetch hesi staffs: %v", err)
	}
	log.Printf("[step 1] 合思员工 %d 条", len(hesi))

	// 2. 钉钉 token
	log.Println("[step 2] 拉钉钉 access_token...")
	dingToken, err := getDingToken(cfg.DingTalk.NotifyAppKey, cfg.DingTalk.NotifyAppSecret)
	if err != nil {
		log.Fatalf("ding token: %v", err)
	}

	// 3. 钉钉在职 userid 列表
	log.Println("[step 3] 钉钉 queryonjob 拉全员 userid...")
	userIDs, err := queryOnJobUseridList(dingToken)
	if err != nil {
		log.Fatalf("queryonjob: %v", err)
	}
	log.Printf("[step 3] 钉钉在职员工 %d 人", len(userIDs))

	// 4. 钉钉花名册详情
	log.Println("[step 4] 钉钉 v2/list 拉合同公司字段...")
	dingEmps, err := fetchDingEmployees(dingToken, cfg.DingTalk.NotifyAgentID, userIDs)
	if err != nil {
		log.Fatalf("v2/list: %v", err)
	}
	log.Printf("[step 4] 钉钉员工详情 %d 条", len(dingEmps))

	// 5. 桥接
	log.Println("[step 5] 手机号桥接合思↔钉钉...")
	rows := matchAndBuild(hesi, dingEmps)

	// 6. 写库
	log.Println("[step 6] UPSERT hesi_employee_contract_company...")
	if err := upsertContracts(db, rows); err != nil {
		log.Fatalf("upsert: %v", err)
	}
	log.Printf("[step 6] 写入 %d 条, 耗时 %v", len(rows), time.Since(start))
}
