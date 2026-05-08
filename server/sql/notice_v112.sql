-- v1.12 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v1.12 布局微调 - AI 工具箱与公告回到右上角',
'本次完成内容:

【布局拆分调整】
v1.11 把 4 件套都搬到左侧底部, 跑哥使用后觉得 AI 工具箱 和 公告铃铛
更适合放回页面右上角(操作时手往右上抬一下更顺手), 而 问题反馈 和
用户头像菜单 留在左侧底部更合理(不常用功能集中在视觉边缘)。

最终布局:
- 右上角: AI 工具箱 + 公告铃铛 (50 红点未读数)
- 左侧底部: 问题反馈 + 用户头像菜单(改密/退出)

不影响数据口径与定时任务运行',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS '时间'
FROM notices WHERE is_pinned=1;
