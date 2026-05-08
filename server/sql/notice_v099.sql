-- v0.99 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.99 客服+广告 11 张运营表中文注释补全',
'本次完成内容:

【数据库注释规范化】
原: 11 张运营数据表(京东/抖音/快手/小红书 客服+广告+分销+商品+直播)有 200+ 字段无中文注释,
    新人看字段名要靠英文猜业务含义.
现: 一次补齐共 251 字段中文注释, 涵盖:
- 抖音商品/直播/广告素材日数据
- 抖音分销账户/素材/商品日数据
- 抖音飞鸽客服日数据
- 京东客服工作量/销售业绩日数据
- 快手客服考核日数据
- 小红书客服分析日数据

【影响】
- 数据库 SHOW CREATE TABLE / 上云 schema.sql 完整可读
- 行为零改动: 字段类型/索引/UK 完全不变, 仅加 COMMENT
- 客服 KPI 字段含义按 RPA 表头/平台通用术语对应, 不重新定义业务口径

不影响日常使用, 不动定时任务',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
