-- v1.15 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v1.15 布局再调整 - 问题反馈回到右上角',
'本次完成内容:

【布局再次调整】
v1.12 把 AI 工具箱 + 公告铃铛 放回右上角, 反馈+用户菜单 留左下。
本次跑哥再调整: 问题反馈也搬回右上角, 左下只留用户头像菜单。

最终布局:
- 右上角 (3 件): AI 工具箱 + 公告铃铛(50 未读) + 问题反馈
- 左下角 (1 件): 用户头像菜单 (改密 / 退出登录)

不影响数据口径与定时任务运行',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS '时间'
FROM notices WHERE is_pinned=1;
