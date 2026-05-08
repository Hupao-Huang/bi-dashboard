-- v1.13 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v1.13 反馈通知闭环 - 后端框架就位',
'本次完成内容:

【反馈通知闭环 — 框架就位】
之前管理员回复反馈后, 提交人毫无感知, 反馈像扔进黑洞。本次后端骨架已经
做好, 等钉钉应用权限开通后即可生效, 不需要再改任何代码。

技术能力:
- 当管理员在反馈管理页面 回复+保存 后, 系统自动通过钉钉 推送一条文本
  消息给原提交人(直接私聊)
- 消息内容: "你提交的反馈X已有新回复: ... — 来自 [管理员姓名]"
- 推送依赖用户已绑定钉钉(扫码登录过, users.dingtalk_userid 不为空),
  未绑定的用户跳过, 不阻断流程
- 异步发送, 不影响保存按钮的响应速度

【启用条件】(待跑哥操作)
1. 钉钉开发者后台 → 找到 hermes-agent 钉钉应用 → 权限管理
   → 添加 "企业机器人主动消息" 能力
2. 重置应用 ClientSecret (memory 标记过原 secret 已暴露)
3. 把 NotifyAppKey/NotifyAppSecret/NotifyRobotCode 三个字段加到
   server/config.json 的 dingtalk 节里
4. 重启 bi-server

启用后日志会显示 "DingTalk notifier ready"。

【现状】
未配置凭证时, 启动日志显示 "DingTalk notifier disabled" — 通知功能优雅
关闭, 反馈管理页其他功能 100% 不受影响。

不影响数据口径与定时任务运行',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS '时间'
FROM notices WHERE is_pinned=1;
