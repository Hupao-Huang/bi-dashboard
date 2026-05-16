// 影刀机器人(client)列表接口
//
// 走 winrobot360 域名 (跟 schedule/list 同). 用于 BI 后台 RPA 文件映射页"机器人账号"下拉.
// 文档官方 SPA 没列, 接口出处: https://github.com/ying-dao/yingdao_mcp_server src/yingdao/openApiService.ts
package yingdao

import "context"

// RobotClientListItem 机器人 (client/list 返回项)
//
// status 取值: idle (空闲可用) / running (运行中) / offline (离线)
// 跟控制台机器人管理页面状态一一对应.
type RobotClientListItem struct {
	RobotClientListItemUuid string `json:"robotClientUuid"`
	RobotClientListItemName string `json:"robotClientName"` // 账号名, 如 lhx@sxx
	Status          string `json:"status"`          // idle / running / offline
	WindowsUserName string `json:"windowsUserName"`
	ClientIp        string `json:"clientIp"`
	MachineName     string `json:"machineName"`
	ClientVersion   string `json:"clientVersion"`
	CreateTime      string `json:"createTime"`
}

// ListClients 列影刀机器人 (一次最多 100, 公司机器人很少, 一次拉完即可)
func (c *Client) ListClients(ctx context.Context, page, size int) ([]RobotClientListItem, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 100
	}
	body := map[string]int{"page": page, "size": size}
	wrapped := struct {
		Data []RobotClientListItem `json:"data"`
	}{}
	if err := c.doJSON(ctx, "POST", c.BizURL, "/oapi/dispatch/v2/client/list", body, &wrapped); err != nil {
		return nil, err
	}
	return wrapped.Data, nil
}
