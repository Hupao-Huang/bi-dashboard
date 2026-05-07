-- v0.64 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.64 采购计划新增"其他"Tab + 分类口径优化',
'采购计划页面从 2 个 Tab 扩展到 3 个 Tab:

📦 成品/半成品采购计划 — 销售业务用
🏷 原材料/包材采购计划 — 生产业务用
🎁 其他采购计划 (含广宣品) — 营销/赠品业务用 (本次新增)

【🎁 其他 Tab 新增内容】
按用友 BIP 大类 05 圈定的品种, 共有月销的 81 个 SKU 显示:
  · 广宣品 (赠品/促销物料)
  · 周边品
  · 物流易耗品
  · 其它

数据口径: 用吉客云的库存 + 月销售量计算 (跟成品 Tab 同口径)
目标备货: 45 天
范围: 7 个核心仓 (跟成品 Tab 一致)

【📦 成品/半成品 Tab 调整】
之前: 不小心把 191 个广宣品/周边品错混进来 (因为它们在吉客云被标成成品)
现在: 通过用友 BIP 分类反查, 把广宣品/周边品分流到"其他"Tab, 成品 Tab 数据更纯净

【🏷 原材料/包材 Tab 调整】
之前: 用排除法过滤(写一长串排除名单), 容易遗漏
现在: 严格限定用友 BIP 大类 01(原材料) + 02(包材), 数据范围更明确

【虚拟品黑名单调整】
之前: 黑名单包含运费虚拟品 + 1 个具体广宣 SKU
现在: 只保留运费虚拟品, 那个广宣 SKU 已自动归到"其他"Tab

跑哥使用建议: 多刷一下"其他"Tab, 看广宣品数量/库存/月销是否符合实际, 有问题随时反馈',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
