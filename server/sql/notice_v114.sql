-- v1.14 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v1.14 反馈通知闭环 - 钉钉双向打通',
'本次完成内容:

【反馈通知 - 双向闭环】
v1.13 把后端框架做好后, 跑哥本次开通了钉钉应用的两个权限,
本版完成全链路联调 + 双向通知逻辑。

方向 1: 管理员回复 → 提交人收到钉钉私聊
- 管理员在反馈管理页给一条反馈写回复并保存后,
  系统自动通过钉钉推送一条消息给原提交人:
  "{姓名}, 你提交的反馈"{标题}"已有新回复:\n\n{回复内容}\n\n— 来自 {管理员}"

方向 2: 员工新提反馈 → 跑哥收到钉钉私聊
- 任何员工通过右上角 / 左侧底部 "问题反馈" 提交反馈后,
  系统自动推送一条消息给 admin (跑哥):
  "【BI 看板·新反馈】{姓名} 提了一条反馈:
   《{标题}》
   {内容前 80 字}
   来源页面: {URL}
   前往反馈管理处理 → /system/feedback"

【接收范围】
本版仅推送给 admin (跑哥本人)。其他 super_admin 角色用户暂不广播,
后续如需扩展通知给多管理员可调整 SQL 范围。

【技术细节】
- BI 看板 db 里 dingtalk_userid 存的是 UnionId, 推送前自动调用钉钉
  user/getbyunionid 接口转换为企业内 staffId, 再调用 chatbotToOne API
- access_token 缓存 7000s, UnionId→staffId 一对一映射进程内常驻缓存
- 推送是异步的, 不影响保存 / 提交反馈接口的响应速度
- 提交人未绑定钉钉时跳过, 不阻断流程

【依赖的 hermes-agent 钉钉应用权限】
- 企业机器人主动消息 (chatbotToOne)
- 通讯录管理 - 成员信息读取 (qyapi_get_member, 用于 UnionId 转 staffId)
两个权限都已开通生效。

不影响数据口径与定时任务运行',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS '时间'
FROM notices WHERE is_pinned=1;
