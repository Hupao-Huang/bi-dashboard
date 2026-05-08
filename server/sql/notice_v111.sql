-- v1.11 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v1.11 全局布局调整 - 右上角功能下移到左侧',
'本次完成内容:

【右上角 4 件套整体下移到左侧底部】
之前页面右上角挤了 4 个功能图标 (AI 工具箱 / 公告铃铛 / 问题反馈 / 用户头像菜单),
看起来比较挤。本次全部下移到左侧菜单栏底部一个独立功能区里。

【展开状态下】
左侧菜单底部依次显示:
- 🚀 AI 工具箱 (一键打开 AI 助手抽屉)
- 🔔 公告 (带红色未读数, 点击查看公告列表)
- 💬 问题反馈 (一键打开反馈提交弹窗)
- 👤 虎跑 / 超级管理员·@admin (点击展开个人中心/改密码/退出登录)

【折叠状态下】
点击侧边栏底部 "<<" 折叠后, 4 个功能变成纯图标排列, 鼠标悬停显示文字 Tooltip,
不影响日常使用。

【页面顶部 Header 极简】
现在 Header 只剩面包屑 "系统设置 / 用户管理" 这种导航路径,
右侧整体清空, 视觉更干净, 数据展示区可见高度增加约 56px (Header 高度本身没变,
但右侧不再有抢眼的图标干扰阅读)。

【手机版同步】
手机端打开侧边栏抽屉时, 同样在底部看到这 4 件套, 不需要再去找右上角。

不影响数据口径与定时任务运行',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS '时间'
FROM notices WHERE is_pinned=1;
