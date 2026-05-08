-- v0.87 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.87 库存出入库流水去重防漏 + 同步工具健壮性',
'本次修复内容:

【出入库流水表 — 防止重复累加 + 防 API 失败空窗】
1. 修复了出入库流水重复 169 行的历史问题 (吉客云接口同步时偶发返回重复行造成)
2. 现在重跑同步即使遇到接口异常, 已有数据也不会被清空 (之前是先删后拉, 中途失败会空窗一段时间)

【吉客云库存同步 — 防止漏拉数据】
3. 库存同步翻页规则修正 — 修复极端情况下可能跳过部分库存的潜在漏洞

【同步工具错误提示】
4. 6 个 RPA 数据导入工具 (天猫超市/抖音/京东/拼多多/唯品会/抖音分销) 配置加载失败时现在会立刻提示错误并停止, 不再静默继续跑空轮

不影响日常使用, 不动定时任务',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
