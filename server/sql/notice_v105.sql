-- v1.05 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v1.05 综合看板 KPI 卡分辨率适配修复',
'本次完成内容:

【综合看板顶部 3 个大卡修复】
之前在窗口宽度不够（笔记本/小屏/分屏）时,
"总销售额 / 总货品数 / 综合客单价" 卡片右上角的部门拆分标签
（电商部门 / 社媒部门 / 线下部门 / 分销部门 / 即时零售部）
会因为强制单行排列被切掉, 后面几个部门看不见。

本次修复后:
- 标签自动换行（窗口宽就一行排满, 窗口窄就排成 2-3 行）
- 不会再有部门数据被遮挡或截断
- 数字主显示区域保持原宽度, 不被挤压

【适用分辨率】
1920px 及以上: 5 个部门一行（理想态）
1600px (常见笔记本): 自动 3+2 两行
1366px 及以下: 自动 2-3 行

不影响数据口径与定时任务运行',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS '时间'
FROM notices WHERE is_pinned=1;
