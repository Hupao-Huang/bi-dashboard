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
	YonSuite     YonSuiteConfig `json:"yonsuite"`
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

	// 通知机器人凭证（用于反馈回复 push 给用户，走 chatbotToOne API）
	// 复用 hermes-agent 钉钉应用，需在该应用启用 "企业机器人主动消息" 权限
	// 留空时通知功能自动禁用，不影响其他功能
	NotifyAppKey    string `json:"notify_app_key"`
	NotifyAppSecret string `json:"notify_app_secret"`
	NotifyRobotCode string `json:"notify_robot_code"` // 机器人 robotCode (一般等于 AppKey)
}

type HesiConfig struct {
	AppKey string `json:"appkey"`
	Secret string `json:"secret"`
}

// YonSuiteConfig 用友 YonBIP 开放平台配置
// BaseURL 示例: https://c3.yonyoucloud.com（不带尾斜杠，client 内部拼路径）
type YonSuiteConfig struct {
	AppKey    string `json:"appkey"`
	AppSecret string `json:"appsecret"`
	BaseURL   string `json:"base_url"`
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
