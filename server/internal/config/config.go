package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Server   ServerConfig   `json:"server"`
	Database DatabaseConfig `json:"database"`
	JackYun  JackYunConfig  `json:"jackyun"`
	JackYunTrade JackYunConfig `json:"jackyun_trade"`
	DingTalk DingTalkConfig `json:"dingtalk"`
}

type ServerConfig struct {
	Port int `json:"port"`
}

type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"dbname"`
}

func (d *DatabaseConfig) DSN() string {
	return d.User + ":" + d.Password + "@tcp(" + d.Host + ":" + itoa(d.Port) + ")/" + d.DBName + "?charset=utf8mb4&parseTime=True&loc=Local"
}

type JackYunConfig struct {
	AppKey string `json:"appkey"`
	Secret string `json:"secret"`
	APIURL string `json:"api_url"`
}

type DingTalkConfig struct {
	WebhookToken  string `json:"webhook_token"`
	WebhookSecret string `json:"webhook_secret"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil && !filepath.IsAbs(path) {
		if exePath, exeErr := os.Executable(); exeErr == nil {
			exeConfigPath := filepath.Join(filepath.Dir(exePath), path)
			data, err = os.ReadFile(exeConfigPath)
		}
	}
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	// defaults
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 3306
	}
	return &cfg, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
