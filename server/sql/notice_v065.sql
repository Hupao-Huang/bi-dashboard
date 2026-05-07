-- v0.65 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.65 修复广宣品分类漏排除',
'v0.64 上线后, 跑哥发现仍有 2 个广宣品 SKU 错混在"成品/半成品"Tab 里:
  · 广宣品-天猫华为watch fit 4手表
  · 组装-20260428麦德龙255安心年年礼盒

【根因】这些 SKU 在吉客云端档案不全(用友编码字段为空), 导致系统分类判断走单一路径桥接失败, 漏掉了排除条件。

【修复】系统增加了一条直连兜底路径, 即使吉客云档案不全, 也能通过库存编码直接对到用友 BIP 端的分类。

【影响】
- 📦 成品/半成品 Tab: 前面这种"档案不全"的广宣品已自动从成品 Tab 移走
- 🎁 其他 Tab: 总 SKU 从 81 增加到 83, 多出来 2 个就是跑哥发现的这种漏掉的

【原理简化说明】
吉客云端有些 SKU 没维护用友编码字段, 系统过去依赖这个字段做分类匹配, 字段空就匹配失败。现在加了一条"按库存编码直接匹配用友档案"的备份路径, 双路径同时比对, 任何一种发现是 05 大类(广宣品)就分流到"其他"Tab。

跑哥使用建议: 多刷一下 3 个 Tab, 看是否还有"看着不像本类"的 SKU, 有问题随时反馈',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
