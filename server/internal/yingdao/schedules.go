// 影刀任务管理接口 (用于 BI 后台维护映射表 UI 拉影刀任务下拉)
//
// 列任务 / 任务详情, 走 winrobot360 域名
package yingdao

import "context"

// Schedule 影刀任务 (列表项)
type Schedule struct {
	ScheduleUuid string `json:"scheduleUuid"`
	ScheduleName string `json:"scheduleName"`
	ScheduleType string `json:"scheduleType"`
	Enabled      bool   `json:"enabled"`
	CreateTime   string `json:"createTime"`
	UpdateTime   string `json:"updateTime"`
}

// ScheduleListReq 列任务请求
type ScheduleListReq struct {
	Key     string `json:"key,omitempty"`     // 任务名称模糊
	Enabled *bool  `json:"enabled,omitempty"` // 是否启用过滤
	Page    int    `json:"page,omitempty"`
	Size    int    `json:"size,omitempty"`
}

// RobotInScheduleDetail 任务详情下的子应用
type RobotInScheduleDetail struct {
	RobotUuid    string `json:"robotUuid"`
	RobotName    string `json:"robotName"`
	SupportParam bool   `json:"supportParam"`
}

// RobotClient 任务关联的机器人
type RobotClient struct {
	Uuid             string `json:"uuid"`
	RobotClientName  string `json:"robotClientName"`
}

// ScheduleDetail 任务详情 (核心拿子应用 robotList)
type ScheduleDetail struct {
	ScheduleUuid    string                  `json:"scheduleUuid"`
	ScheduleName    string                  `json:"scheduleName"`
	ScheduleType    string                  `json:"scheduleType"`
	RobotClientList []RobotClient           `json:"robotClientList"`
	RobotList       []RobotInScheduleDetail `json:"robotList"`
}

// ListSchedules 拉影刀全量任务 (一次最多 500)
func (c *Client) ListSchedules(ctx context.Context, page, size int) ([]Schedule, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 500
	}
	req := ScheduleListReq{Page: page, Size: size}
	var arr []Schedule
	wrapped := struct {
		Data []Schedule `json:"data"`
	}{}
	if err := c.doJSON(ctx, "POST", c.BizURL, "/oapi/dispatch/v2/schedule/list", req, &wrapped); err != nil {
		return nil, err
	}
	arr = wrapped.Data
	return arr, nil
}

// GetScheduleDetail 拉单个任务详情 (含子应用 robotList)
func (c *Client) GetScheduleDetail(ctx context.Context, scheduleUuid string) (*ScheduleDetail, error) {
	body := map[string]string{"scheduleUuid": scheduleUuid}
	var resp ScheduleDetail
	wrapped := struct {
		Data *ScheduleDetail `json:"data"`
	}{Data: &resp}
	if err := c.doJSON(ctx, "POST", c.BizURL, "/oapi/dispatch/v2/schedule/detail", body, &wrapped); err != nil {
		return nil, err
	}
	return &resp, nil
}
