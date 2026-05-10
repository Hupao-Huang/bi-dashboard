package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bi-dashboard/internal/finance"
)

// 预览缓存 TTL：30 分钟
const financePreviewTTL = 30 * time.Minute

func financePreviewDir() string {
	d := filepath.Join(os.TempDir(), "bi-finance-preview")
	os.MkdirAll(d, 0755)
	return d
}

func newPreviewToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// previewPayload 缓存到磁盘的内容：包含 result 和元数据
type previewPayload struct {
	Year       int                  `json:"year"`
	Mode       string               `json:"mode"`
	Filename   string               `json:"filename"`
	UserID     int                  `json:"userId"`
	UploadedAt time.Time            `json:"uploadedAt"`
	Result     *finance.ParseResult `json:"result"`
}

var financeImportMu sync.Mutex

type FinCell struct {
	Amount float64  `json:"amount"`
	Ratio  *float64 `json:"ratio,omitempty"`
}

type FinSeries struct {
	RangeTotal FinCell            `json:"rangeTotal"`
	Cells      map[string]FinCell `json:"cells"` // key = "YYYY-M"
}

type FinChannelSeries struct {
	Channel string    `json:"channel"`
	Series  FinSeries `json:"series"`
}

type FinReportRow struct {
	Code       string             `json:"code"`
	Name       string             `json:"name"`
	Level      int                `json:"level"`
	Parent     string             `json:"parent"`
	Category   string             `json:"category"`
	SubChannel string             `json:"subChannel,omitempty"`
	SortOrder  int                `json:"sortOrder"`
	Total      FinSeries          `json:"total"`               // 跨选中渠道的总（电商+社媒之和）
	ByChannel  []FinChannelSeries `json:"byChannel,omitempty"` // 各渠道明细；仅在 channels>1 时返回
}

func sortIntsAsc(a []int) {
	for i := 0; i < len(a); i++ {
		for j := i + 1; j < len(a); j++ {
			if a[i] > a[j] {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	s := strings.Repeat("?,", n)
	return s[:len(s)-1]
}

func nullStr(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
}

func trimStrings(a []string) []string {
	var r []string
	for _, s := range a {
		s = strings.TrimSpace(s)
		if s != "" {
			r = append(r, s)
		}
	}
	return r
}
