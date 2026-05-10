package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"
)

// ==================== 批量导入用户 ====================

type batchImportRow struct {
	Row        int    `json:"row"`
	RealName   string `json:"realName"`
	Phone      string `json:"phone"`
	Department string `json:"department"`
	EmployeeID string `json:"employeeId"`
	Username   string `json:"username"`
	Valid      bool   `json:"valid"`
	Error      string `json:"error,omitempty"`
}

func (h *DashboardHandler) AdminUsersBatchImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "文件过大或格式错误")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "请上传Excel文件")
		return
	}
	defer file.Close()

	password := strings.TrimSpace(r.FormValue("password"))
	roleCodesJSON := r.FormValue("roleCodes")
	dryRun := r.FormValue("dryRun") == "true"

	if password == "" {
		writeError(w, http.StatusBadRequest, "请设置初始密码")
		return
	}
	if err := validatePassword(password, ""); err != nil {
		writeError(w, http.StatusBadRequest, "初始密码不符合要求: "+err.Error())
		return
	}

	var roleCodes []string
	if roleCodesJSON != "" {
		if err := json.Unmarshal([]byte(roleCodesJSON), &roleCodes); err != nil {
			writeError(w, http.StatusBadRequest, "角色参数格式错误")
			return
		}
	}

	// 解析 Excel
	f, err := excelize.OpenReader(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无法读取Excel文件")
		return
	}
	defer f.Close()

	sheetName := f.GetSheetName(0)
	allRows, err := f.GetRows(sheetName)
	if err != nil || len(allRows) < 2 {
		writeError(w, http.StatusBadRequest, "Excel为空或无数据行")
		return
	}

	// 按表头名称匹配列
	headerMap := map[string]string{
		"姓名": "realName", "名字": "realName", "员工姓名": "realName",
		"手机号": "phone", "手机": "phone", "手机号码": "phone", "联系电话": "phone",
		"部门": "department", "所在部门": "department",
		"工号": "employeeId", "员工工号": "employeeId",
	}
	colIndex := map[string]int{}
	for i, cell := range allRows[0] {
		cell = strings.TrimSpace(cell)
		if field, ok := headerMap[cell]; ok {
			colIndex[field] = i
		}
	}

	if _, ok := colIndex["realName"]; !ok {
		writeError(w, http.StatusBadRequest, "Excel中未找到[姓名]列")
		return
	}
	if _, ok := colIndex["phone"]; !ok {
		writeError(w, http.StatusBadRequest, "Excel中未找到[手机号]列")
		return
	}

	// 解析数据行
	rows := make([]batchImportRow, 0, len(allRows)-1)
	for i := 1; i < len(allRows); i++ {
		row := allRows[i]
		getCol := func(field string) string {
			if idx, ok := colIndex[field]; ok && idx < len(row) {
				return strings.TrimSpace(row[idx])
			}
			return ""
		}

		realName := getCol("realName")
		phone := getCol("phone")
		department := getCol("department")
		employeeId := getCol("employeeId")

		if realName == "" && phone == "" {
			continue // 跳过空行
		}

		item := batchImportRow{
			Row:        i + 1,
			RealName:   realName,
			Phone:      phone,
			Department: department,
			EmployeeID: employeeId,
			Username:   phone,
			Valid:      true,
		}

		if realName == "" {
			item.Valid = false
			item.Error = "姓名为空"
		} else if phone == "" {
			item.Valid = false
			item.Error = "手机号为空"
		} else if len(phone) < 11 {
			item.Valid = false
			item.Error = "手机号格式不正确"
		}

		rows = append(rows, item)
	}

	if len(rows) == 0 {
		writeError(w, http.StatusBadRequest, "没有有效的数据行")
		return
	}

	// 查重：检查已存在的用户名
	usernames := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Valid {
			usernames = append(usernames, row.Username)
		}
	}
	existingUsers := map[string]bool{}
	if len(usernames) > 0 {
		userPlaceholders := strings.Repeat("?,", len(usernames))
		userPlaceholders = userPlaceholders[:len(userPlaceholders)-1]
		args := make([]interface{}, len(usernames))
		for i, u := range usernames {
			args[i] = u
		}
		existRows, err := h.DB.Query(
			fmt.Sprintf(`SELECT username FROM users WHERE username IN (%s)`, userPlaceholders),
			args...,
		)
		if err == nil {
			defer existRows.Close()
			for existRows.Next() {
				var u string
				existRows.Scan(&u)
				existingUsers[u] = true
			}
		}
	}

	// 检查导入列表内部重复
	seenPhones := map[string]int{}
	for i := range rows {
		if !rows[i].Valid {
			continue
		}
		if existingUsers[rows[i].Username] {
			rows[i].Valid = false
			rows[i].Error = "用户名已存在"
		} else if prevRow, dup := seenPhones[rows[i].Phone]; dup {
			rows[i].Valid = false
			rows[i].Error = fmt.Sprintf("手机号与第%d行重复", prevRow)
		} else {
			seenPhones[rows[i].Phone] = rows[i].Row
		}
	}

	// 统计
	validCount := 0
	errorItems := make([]batchImportRow, 0)
	for _, row := range rows {
		if row.Valid {
			validCount++
		} else {
			errorItems = append(errorItems, row)
		}
	}

	// 预览模式
	if dryRun {
		writeJSON(w, map[string]interface{}{
			"total":   len(rows),
			"valid":   validCount,
			"errors":  errorItems,
			"preview": rows,
		})
		return
	}

	if validCount == 0 {
		writeError(w, http.StatusBadRequest, "没有可导入的有效数据")
		return
	}

	// 正式导入：bcrypt 只算一次
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "密码加密失败")
		return
	}

	tx, err := h.DB.Begin()
	if writeDatabaseError(w, err) {
		return
	}
	defer tx.Rollback()

	imported := 0
	importErrors := make([]batchImportRow, 0)
	for i := range rows {
		if !rows[i].Valid {
			continue
		}

		result, err := tx.Exec(
			`INSERT INTO users (username, password_hash, real_name, phone, department, employee_id, must_change_password, status) VALUES (?, ?, ?, ?, ?, ?, 1, 'active')`,
			rows[i].Username, string(passwordHash), rows[i].RealName, rows[i].Phone, rows[i].Department, rows[i].EmployeeID,
		)
		if err != nil {
			rows[i].Valid = false
			rows[i].Error = "用户名已存在或数据重复"
			if !strings.Contains(err.Error(), "Duplicate") {
				rows[i].Error = "插入失败，请检查数据格式"
			}
			importErrors = append(importErrors, rows[i])
			continue
		}

		userID, _ := result.LastInsertId()
		if len(roleCodes) > 0 {
			if err := h.saveUserAccessTx(tx, userID, roleCodes, authDataScopes{}); err != nil {
				rows[i].Valid = false
				rows[i].Error = "分配角色失败，请联系管理员"
				importErrors = append(importErrors, rows[i])
				continue
			}
		}
		imported++
	}

	if err := tx.Commit(); err != nil {
		writeDatabaseError(w, err)
		return
	}

	writeJSON(w, map[string]interface{}{
		"total":    len(rows),
		"valid":    validCount,
		"imported": imported,
		"errors":   append(errorItems, importErrors...),
	})
}
