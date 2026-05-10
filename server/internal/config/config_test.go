package config

// config_test.go — Load + DSN 测试
// 已 Read config.go 全文 (98 行).

import (
	"os"
	"path/filepath"
	"testing"
)

// === DatabaseConfig.DSN (line 33-35) ===

func TestDatabaseConfigDSN(t *testing.T) {
	d := &DatabaseConfig{
		User: "root", Password: "pass123", Host: "127.0.0.1", Port: 3306, DBName: "bi_dashboard",
	}
	want := "root:pass123@tcp(127.0.0.1:3306)/bi_dashboard?charset=utf8mb4&parseTime=True&loc=Local"
	if got := d.DSN(); got != want {
		t.Errorf("DSN()=\n  got %q\n want %q", got, want)
	}
}

func TestDatabaseConfigDSNHandlesEmpty(t *testing.T) {
	// 空字段应不 panic, 拼成边界字符串
	d := &DatabaseConfig{}
	got := d.DSN()
	if got == "" {
		t.Error("空 DSN 不应返空 string")
	}
	// 含基础 mysql DSN 结构
	if !contains2(got, "tcp(") {
		t.Errorf("DSN 应含 tcp() 结构: %s", got)
	}
}

// === Load (line 75-98) ===

func TestLoadValidJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	content := `{
		"server": {"port": 9090},
		"database": {"host": "db.example", "port": 3307, "user": "u", "password": "p", "dbname": "x"}
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load err: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Port should be 9090, got %d", cfg.Server.Port)
	}
	if cfg.Database.Host != "db.example" {
		t.Errorf("Host should be db.example, got %q", cfg.Database.Host)
	}
}

func TestLoadDefaultsWhenZero(t *testing.T) {
	// 源码 line 91-96: server.port=0 → 8080, database.port=0 → 3306
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	content := `{"server":{},"database":{"host":"h","user":"u","password":"p","dbname":"d"}}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load err: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Port=0 应 fallback 到 8080, got %d", cfg.Server.Port)
	}
	if cfg.Database.Port != 3306 {
		t.Errorf("Database.Port=0 应 fallback 到 3306, got %d", cfg.Database.Port)
	}
}

func TestLoadInvalidJSONReturnsError(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Error("invalid JSON 应返 error")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("/path/that/does/not/exist.json"); err == nil {
		t.Error("不存在文件应返 error")
	}
}

func contains2(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
