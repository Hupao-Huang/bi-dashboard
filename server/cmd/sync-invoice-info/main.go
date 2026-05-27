// sync-invoice-info 从"开票资料.pdf"同步法人实体抬头+税号到 hesi_legal_entity_invoice_info
// 用途: 规则 8 发票抬头/税号校验
// 异体字处理: PDF 提取常含 ⾃/⻝/⻄/⼴/⼯/⾷ 等 Kangxi/Radical Unicode, 用 NFKC normalize → 自/食/西/广/工
// 法人实体 ID 反查: 用 normalize 后公司名匹配 hesi_legal_entity_ys_accbook
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

type dbConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Dbname   string `json:"dbname"`
}

type config struct {
	Database dbConfig `json:"database"`
}

func loadDB() (*sql.DB, error) {
	data, err := os.ReadFile("config.json")
	if err != nil {
		return nil, err
	}
	var c config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true",
		c.Database.User, c.Database.Password, c.Database.Host, c.Database.Port, c.Database.Dbname)
	return sql.Open("mysql", dsn)
}

// extractFromPDF 用 Python pdfplumber + NFKC normalize 抽取 PDF 全文
func extractFromPDF(path string) (string, error) {
	// PDF 抽取的汉字含 Kangxi/CJK Radicals (U+2E80-U+2FFF):
	//   - NFKD/NFKC 把 U+2F00-2FBC 范围 (大部分) 自动转为标准汉字
	//   - U+2EXX 范围有 6 个不转, 需显式映射
	// 实查样本: ⻄ U+2EC4 / ⻝ U+2EDD / ⻘ U+2ED8 / ⺠ U+2EA0 / ⻰ U+2EF0 / ⻛ U+2EDB
	script := fmt.Sprintf(`import pdfplumber, unicodedata, sys
RADICAL_MAP = {
    '⻄': '西', '⻝': '食', '⻘': '青',
    '⺠': '民', '⻰': '龙', '⻛': '风',
    # 括号统一全角 (ys_accbook 用全角, NFKC 把全角→半角, 这里再转回全角)
    '(': '（', ')': '）',
}
def normalize(s):
    s = unicodedata.normalize('NFKC', s)
    return ''.join(RADICAL_MAP.get(c, c) for c in s)
with pdfplumber.open(r'%s') as pdf:
    out = []
    for p in pdf.pages:
        t = p.extract_text() or ''
        t = normalize(t)
        out.append(t)
sys.stdout.write('\n'.join(out))`, path)
	cmd := exec.Command("python", "-c", script)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

type invoiceEntry struct {
	Name        string
	TaxNo       string
	AddressPhone string
	Bank        string
	Account     string
	Remark      string // 上下文里如果出现"一般户/基本户"则留底
}

// parseEntries 从 PDF 文本抽取每个公司开票块
// 每块格式 (可能错行):
//   公司名称: XXX
//   税号: YYYYY (18 位 统一社会信用代码)
//   地址电话: ZZZ
//   银行: AAA
//   账号: BBB
func parseEntries(text string) []invoiceEntry {
	var entries []invoiceEntry
	lines := strings.Split(text, "\n")

	// 状态机抽取
	var cur invoiceEntry
	flush := func() {
		if cur.Name != "" && cur.TaxNo != "" {
			entries = append(entries, cur)
		}
		cur = invoiceEntry{}
	}
	getVal := func(line, key string) string {
		idx := strings.Index(line, key)
		if idx < 0 {
			return ""
		}
		v := strings.TrimSpace(line[idx+len(key):])
		v = strings.TrimLeft(v, ": ：")
		return strings.TrimSpace(v)
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		switch {
		case strings.Contains(line, "公司名称"):
			flush()
			cur.Name = getVal(line, "公司名称")
		case strings.HasPrefix(line, "税号") || strings.Contains(line, "税号"):
			if cur.Name == "" {
				continue
			}
			cur.TaxNo = getVal(line, "税号")
		case strings.HasPrefix(line, "地址电话") || strings.Contains(line, "地址电话"):
			cur.AddressPhone = getVal(line, "地址电话")
		case strings.HasPrefix(line, "银行") || strings.Contains(line, "银行：") || strings.Contains(line, "银行:"):
			cur.Bank = getVal(line, "银行")
		case strings.HasPrefix(line, "账号") || strings.Contains(line, "账号"):
			cur.Account = getVal(line, "账号")
		}
	}
	flush()
	return entries
}

func main() {
	pdfPath := `C:\Users\Administrator\Desktop\开票资料.pdf`

	db, err := loadDB()
	if err != nil {
		log.Fatalf("连数据库失败: %v", err)
	}
	defer db.Close()

	log.Printf("提取 PDF: %s", pdfPath)
	text, err := extractFromPDF(pdfPath)
	if err != nil {
		log.Fatalf("PDF 提取失败 (需 Python pdfplumber): %v", err)
	}

	entries := parseEntries(text)
	log.Printf("PDF 抽取: %d 公司+税号", len(entries))

	// 拉 ys_accbook 法人实体字典做 ID 反查
	entityMap := map[string]string{}
	rows, err := db.Query(`SELECT legal_entity_id, legal_entity_name FROM hesi_legal_entity_ys_accbook WHERE active=1`)
	if err != nil {
		log.Fatalf("拉法人实体失败: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		_ = rows.Scan(&id, &name)
		entityMap[name] = id
	}
	log.Printf("法人实体字典: %d 项", len(entityMap))

	// REPLACE INTO 灌表
	matched, unmatched := 0, 0
	dedupTax := map[string]bool{}
	for _, e := range entries {
		if dedupTax[e.TaxNo] {
			continue
		}
		dedupTax[e.TaxNo] = true

		legalEntityID, ok := entityMap[e.Name]
		if !ok {
			log.Printf("⚠ 法人实体未匹配: %s (税号 %s) — 跳过", e.Name, e.TaxNo)
			unmatched++
			continue
		}
		_, err := db.Exec(`REPLACE INTO hesi_legal_entity_invoice_info
			(legal_entity_id, legal_entity_name, invoice_title, tax_no, address_phone, bank_name, account_no, source)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'pdf')`,
			legalEntityID, e.Name, e.Name, e.TaxNo, e.AddressPhone, e.Bank, e.Account)
		if err != nil {
			log.Printf("写入失败 %s: %v", e.Name, err)
			continue
		}
		matched++
	}
	log.Printf("同步完成: 匹配 %d / 未匹配 %d", matched, unmatched)

	// 当前表覆盖
	var total int
	_ = db.QueryRow(`SELECT COUNT(*) FROM hesi_legal_entity_invoice_info WHERE active=1`).Scan(&total)
	log.Printf("hesi_legal_entity_invoice_info 当前 %d 条", total)
}
