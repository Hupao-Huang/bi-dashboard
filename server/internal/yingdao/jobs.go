// 影刀业务接口: 启动应用 / 查询应用结果 / 通知日志 / 轮询日志
//
// 所有接口都是 Authorization: Bearer <accessToken>
package yingdao

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// JobStartReq 启动应用请求
type JobStartReq struct {
	AccountName    string                 `json:"accountName,omitempty"`    // 机器人账号 (跟 robotClientGroupUuid 二选一)
	RobotUuid      string                 `json:"robotUuid"`                // 应用 UUID (必填)
	IdempotentUuid string                 `json:"idempotentUuid,omitempty"` // 幂等 UUID
	WaitTimeoutSeconds int                `json:"waitTimeoutSeconds,omitempty"` // 等待超时秒数, 默认 600
	RunTimeout     int                    `json:"runTimeout,omitempty"`     // 应用运行超时秒数
	Priority       string                 `json:"priority,omitempty"`       // low/middle/high
	Params         []JobParam             `json:"params,omitempty"`         // 应用入参
}

// JobParam 应用运行参数
type JobParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  string `json:"type"` // str/int/float/bool/file
}

// JobStartResp 启动应用响应 data
type JobStartResp struct {
	JobUuid        string `json:"jobUuid"`
	IdempotentFlag bool   `json:"idempotentFlag"`
}

// JobStatus 查询应用结果响应 data
type JobStatus struct {
	JobUuid          string `json:"jobUuid"`
	Status           string `json:"status"`           // waiting/running/finish/error/cancel/...
	StatusName       string `json:"statusName"`       // 中文状态描述
	Remark           string `json:"remark"`
	RobotClientUuid  string `json:"robotClientUuid"`
	RobotClientName  string `json:"robotClientName"`
	StartTime        string `json:"startTime"`
	EndTime          string `json:"endTime"`
	RobotUuid        string `json:"robotUuid"`
	RobotName        string `json:"robotName"`
	ScreenshotURL    string `json:"screenshotUrl"`
}

// LogNotifyReq 通知日志请求
type LogNotifyReq struct {
	JobUuid string `json:"jobUuid"`
	Page    int    `json:"page,omitempty"`
	Size    int    `json:"size,omitempty"`
}

// 影刀 notify log 响应 data 直接是 requestId 字符串 (不是 {requestId: "..."} 对象)
// 实测响应: {"data":"9a256ac6-ac30-4904-b501-ac3aac5b203b","code":200}

// LogQueryResp 轮询日志响应 data
type LogQueryResp struct {
	RequestID string    `json:"requestId"`
	Page      LogPage   `json:"page"`
	Logs      []LogItem `json:"logs"`
}

// LogPage 日志分页信息
type LogPage struct {
	Total int `json:"total"`
	Page  int `json:"page"`
	Size  int `json:"size"`
}

// LogItem 单条日志
type LogItem struct {
	Time  string `json:"time"`
	Level string `json:"level"`
	Text  string `json:"text"`
	LogID int64  `json:"logId"`
}

// StartJob 启动一个应用 (单 job)
// 自动用 client.DefaultAccount 作为 accountName, 可通过传 req.AccountName 覆盖
// 自动生成 idempotentUuid (32 字符 hex), 可通过传 req.IdempotentUuid 覆盖
func (c *Client) StartJob(ctx context.Context, req JobStartReq) (*JobStartResp, error) {
	if req.AccountName == "" {
		req.AccountName = c.DefaultAccount
	}
	if req.IdempotentUuid == "" {
		req.IdempotentUuid = newIdempotentUuid()
	}
	if req.WaitTimeoutSeconds == 0 {
		req.WaitTimeoutSeconds = 600 // 10 分钟排队
	}
	if req.RunTimeout == 0 {
		req.RunTimeout = 1800 // 30 分钟跑超时
	}
	if req.Priority == "" {
		req.Priority = "middle"
	}

	var resp JobStartResp
	wrapped := struct {
		Data *JobStartResp `json:"data"`
	}{Data: &resp}
	if err := c.doJSON(ctx, "POST", c.AuthURL, "/oapi/dispatch/v2/job/start", req, &wrapped); err != nil {
		return nil, err
	}
	return &resp, nil
}

// QueryJob 查询应用运行状态
func (c *Client) QueryJob(ctx context.Context, jobUuid string) (*JobStatus, error) {
	var resp JobStatus
	wrapped := struct {
		Data *JobStatus `json:"data"`
	}{Data: &resp}
	body := map[string]string{"jobUuid": jobUuid}
	if err := c.doJSON(ctx, "POST", c.AuthURL, "/oapi/dispatch/v2/job/query", body, &wrapped); err != nil {
		return nil, err
	}
	return &resp, nil
}

// NotifyLog 通知影刀准备日志, 拿到 requestId 后续 60s 内可轮询日志
func (c *Client) NotifyLog(ctx context.Context, jobUuid string, page, size int) (string, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 100
	}
	req := LogNotifyReq{JobUuid: jobUuid, Page: page, Size: size}
	wrapped := struct {
		Data string `json:"data"` // 影刀这里 data 直接是 requestId 字符串
	}{}
	if err := c.doJSON(ctx, "POST", c.AuthURL, "/oapi/dispatch/v2/job/log/notify", req, &wrapped); err != nil {
		return "", err
	}
	return wrapped.Data, nil
}

// QueryLog 轮询日志 (用 NotifyLog 拿到的 requestId)
// 返回 logs 可能为空, 调用方需要根据 code 判断是否继续轮询
// 80204002 = 处理中需要继续轮询 / 200 = 已拿到日志
func (c *Client) QueryLog(ctx context.Context, requestID string) (*LogQueryResp, error) {
	q := "?requestId=" + requestID
	var resp LogQueryResp
	wrapped := struct {
		Data *LogQueryResp `json:"data"`
	}{Data: &resp}
	if err := c.doJSON(ctx, "GET", c.AuthURL, "/oapi/dispatch/v2/job/log/query"+q, nil, &wrapped); err != nil {
		return nil, err
	}
	return &resp, nil
}

// newIdempotentUuid 生成 32 字符 hex 幂等 UUID
func newIdempotentUuid() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
