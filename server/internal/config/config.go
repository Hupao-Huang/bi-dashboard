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
	YingDao      YingDaoConfig  `json:"yingdao"`
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

// YingDaoConfig 影刀 RPA 开放 API 配置
// AuthURL 取 token 用 (api.yingdao.com)
// BizURL 业务接口用 (api.winrobot360.com), 列任务/任务详情接口走这个域名
// DefaultAccount 启动应用时关联的机器人账号 (如 lhx@sxx)
type YingDaoConfig struct {
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	AuthURL         string `json:"auth_url"`
	BizURL          string `json:"biz_url"`
	DefaultAccount  string `json:"default_account"`
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
	applyEnvOverrides(&cfg)
	return &cfg, nil
}

// applyEnvOverrides 用环境变量覆盖 config.json 中的敏感字段
//
// 生产环境推荐: 把 config.json 留为模板 (无密码), 真实凭证通过环境变量注入
// 优先级: 环境变量 > config.json
//
// 命名规则: BI_<SECTION>_<FIELD>
// 例: BI_DB_PASSWORD / BI_JACKYUN_SECRET / BI_DINGTALK_CLIENT_SECRET
func applyEnvOverrides(cfg *Config) {
	// Database
	if v := os.Getenv("BI_DB_HOST"); v != "" {
		cfg.Database.Host = v
	}
	if v := os.Getenv("BI_DB_USER"); v != "" {
		cfg.Database.User = v
	}
	if v := os.Getenv("BI_DB_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}
	if v := os.Getenv("BI_DB_NAME"); v != "" {
		cfg.Database.DBName = v
	}

	// JackYun (主)
	if v := os.Getenv("BI_JACKYUN_APPKEY"); v != "" {
		cfg.JackYun.AppKey = v
	}
	if v := os.Getenv("BI_JACKYUN_SECRET"); v != "" {
		cfg.JackYun.Secret = v
	}

	// JackYun Trade (新 AppKey, 销售单专用)
	if v := os.Getenv("BI_JACKYUN_TRADE_APPKEY"); v != "" {
		cfg.JackYunTrade.AppKey = v
	}
	if v := os.Getenv("BI_JACKYUN_TRADE_SECRET"); v != "" {
		cfg.JackYunTrade.Secret = v
	}

	// DingTalk
	if v := os.Getenv("BI_DINGTALK_WEBHOOK_TOKEN"); v != "" {
		cfg.DingTalk.WebhookToken = v
	}
	if v := os.Getenv("BI_DINGTALK_WEBHOOK_SECRET"); v != "" {
		cfg.DingTalk.WebhookSecret = v
	}
	if v := os.Getenv("BI_DINGTALK_CLIENT_ID"); v != "" {
		cfg.DingTalk.ClientID = v
	}
	if v := os.Getenv("BI_DINGTALK_CLIENT_SECRET"); v != "" {
		cfg.DingTalk.ClientSecret = v
	}
	if v := os.Getenv("BI_DINGTALK_NOTIFY_APP_KEY"); v != "" {
		cfg.DingTalk.NotifyAppKey = v
	}
	if v := os.Getenv("BI_DINGTALK_NOTIFY_APP_SECRET"); v != "" {
		cfg.DingTalk.NotifyAppSecret = v
	}

	// Hesi
	if v := os.Getenv("BI_HESI_APPKEY"); v != "" {
		cfg.Hesi.AppKey = v
	}
	if v := os.Getenv("BI_HESI_SECRET"); v != "" {
		cfg.Hesi.Secret = v
	}

	// YonSuite
	if v := os.Getenv("BI_YONSUITE_APPKEY"); v != "" {
		cfg.YonSuite.AppKey = v
	}
	if v := os.Getenv("BI_YONSUITE_APPSECRET"); v != "" {
		cfg.YonSuite.AppSecret = v
	}

	// Webhook
	if v := os.Getenv("BI_WEBHOOK_SECRET"); v != "" {
		cfg.Webhook.Secret = v
	}

	// YingDao
	if v := os.Getenv("BI_YINGDAO_ACCESS_KEY_ID"); v != "" {
		cfg.YingDao.AccessKeyID = v
	}
	if v := os.Getenv("BI_YINGDAO_ACCESS_KEY_SECRET"); v != "" {
		cfg.YingDao.AccessKeySecret = v
	}
}
