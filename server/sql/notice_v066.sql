-- v0.66 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.66 采购计划页面顶部精简',
'采购计划页面顶部"建议采购量公式 + 数据来源"那段大段说明文字, 已精简删除。

【保留】状态分布快速 KPI:
  断货 / 紧急 / 偏低 / 正常 / 积压 各档计数
  当前 Tab 总 SKU 数

【删除原因】
列表里每个字段(当前库存/日均销售/可售天数/在途/建议采购量)右上角都有 ⓘ 图标, 鼠标移上去能看到完整公式 + 数据来源, 不需要在顶部重复占用屏幕空间。

页面更紧凑, 列表数据看得更多。',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
