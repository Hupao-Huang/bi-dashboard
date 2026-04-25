package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Server       ServerConfig   `json:"server"`
	Database     DatabaseConfig `json:"database"`
	JackYun      JackYunConfig  `json:"jackyun"`
	JackYunTrade JackYunConfig  `json:"jackyun_trade"`
	DingTalk     DingTalkConfig `json:"dingtalk"`
	Hesi         HesiConfig     `json:"hesi"`
	Webhook      WebhookConfig  `json:"webhook"`
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
	return d.User + ":" + d.Password + "@tcp(" + d.Host + ":" + strconv.Itoa(d.Port) + ")/" + d.DBName + "?charset=utf8mb4&parseTime=True&loc=Local"
}

type JackYunConfig struct {
	AppKey string `json:"appkey"`
	Secret string `json:"secret"`
	APIURL string `json:"api_url"`
}

type DingTalkConfig struct {
	WebhookToken  string `json:"webhook_token"`
	WebhookSecret string `json:"webhook_secret"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	CallbackHost  string `json:"callback_host"` // OAuth 回调域名（如 http://192.168.200.48:3000），必须在钉钉应用后台白名单里
}

type HesiConfig struct {
	AppKey string `json:"appkey"`
	Secret string `json:"secret"`
}

type WebhookConfig struct {
	Secret string `json:"secret"`
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
