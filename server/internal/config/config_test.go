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

// === applyEnvOverrides (敏感字段环境变量覆盖) ===

func TestApplyEnvOverridesAll(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg.json")
	os.WriteFile(path, []byte(`{
		"database":{"host":"json-host","user":"json-user","password":"json-pass","dbname":"json-db","port":3306},
		"jackyun":{"appkey":"json-jk","secret":"json-jks"},
		"jackyun_trade":{"appkey":"json-jkt","secret":"json-jkts"},
		"dingtalk":{"webhook_token":"json-wt","webhook_secret":"json-ws","client_id":"json-cid","client_secret":"json-cs","notify_app_key":"json-nak","notify_app_secret":"json-nas"},
		"hesi":{"appkey":"json-hk","secret":"json-hs"},
		"yonsuite":{"appkey":"json-yk","appsecret":"json-ys"},
		"webhook":{"secret":"json-whs"}
	}`), 0644)

	envVars := map[string]string{
		"BI_DB_HOST":                    "env-host",
		"BI_DB_USER":                    "env-user",
		"BI_DB_PASSWORD":                "env-pass",
		"BI_DB_NAME":                    "env-db",
		"BI_JACKYUN_APPKEY":             "env-jk",
		"BI_JACKYUN_SECRET":             "env-jks",
		"BI_JACKYUN_TRADE_APPKEY":       "env-jkt",
		"BI_JACKYUN_TRADE_SECRET":       "env-jkts",
		"BI_DINGTALK_WEBHOOK_TOKEN":     "env-wt",
		"BI_DINGTALK_WEBHOOK_SECRET":    "env-ws",
		"BI_DINGTALK_CLIENT_ID":         "env-cid",
		"BI_DINGTALK_CLIENT_SECRET":     "env-cs",
		"BI_DINGTALK_NOTIFY_APP_KEY":    "env-nak",
		"BI_DINGTALK_NOTIFY_APP_SECRET": "env-nas",
		"BI_HESI_APPKEY":                "env-hk",
		"BI_HESI_SECRET":                "env-hs",
		"BI_YONSUITE_APPKEY":            "env-yk",
		"BI_YONSUITE_APPSECRET":         "env-ys",
		"BI_WEBHOOK_SECRET":             "env-whs",
	}
	for k, v := range envVars {
		os.Setenv(k, v)
		defer os.Unsetenv(k)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	checks := []struct {
		name, got, want string
	}{
		{"db.host", cfg.Database.Host, "env-host"},
		{"db.user", cfg.Database.User, "env-user"},
		{"db.password", cfg.Database.Password, "env-pass"},
		{"db.dbname", cfg.Database.DBName, "env-db"},
		{"jackyun.appkey", cfg.JackYun.AppKey, "env-jk"},
		{"jackyun.secret", cfg.JackYun.Secret, "env-jks"},
		{"jackyun_trade.appkey", cfg.JackYunTrade.AppKey, "env-jkt"},
		{"jackyun_trade.secret", cfg.JackYunTrade.Secret, "env-jkts"},
		{"dingtalk.webhook_token", cfg.DingTalk.WebhookToken, "env-wt"},
		{"dingtalk.webhook_secret", cfg.DingTalk.WebhookSecret, "env-ws"},
		{"dingtalk.client_id", cfg.DingTalk.ClientID, "env-cid"},
		{"dingtalk.client_secret", cfg.DingTalk.ClientSecret, "env-cs"},
		{"dingtalk.notify_app_key", cfg.DingTalk.NotifyAppKey, "env-nak"},
		{"dingtalk.notify_app_secret", cfg.DingTalk.NotifyAppSecret, "env-nas"},
		{"hesi.appkey", cfg.Hesi.AppKey, "env-hk"},
		{"hesi.secret", cfg.Hesi.Secret, "env-hs"},
		{"yonsuite.appkey", cfg.YonSuite.AppKey, "env-yk"},
		{"yonsuite.appsecret", cfg.YonSuite.AppSecret, "env-ys"},
		{"webhook.secret", cfg.Webhook.Secret, "env-whs"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %q want %q", c.name, c.got, c.want)
		}
	}
}

// 没设环境变量 → JSON 值不被覆盖
func TestApplyEnvOverridesFallbackToJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg.json")
	os.WriteFile(path, []byte(`{
		"database":{"host":"json-only","port":3306,"user":"u","password":"p","dbname":"d"},
		"webhook":{"secret":"json-secret"}
	}`), 0644)

	// 清掉可能干扰的环境变量
	os.Unsetenv("BI_DB_HOST")
	os.Unsetenv("BI_WEBHOOK_SECRET")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Database.Host != "json-only" {
		t.Errorf("无 env 时应保留 json 值, got %q", cfg.Database.Host)
	}
	if cfg.Webhook.Secret != "json-secret" {
		t.Errorf("无 env 时应保留 json 值, got %q", cfg.Webhook.Secret)
	}
}
