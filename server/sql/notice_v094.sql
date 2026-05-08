-- v0.94 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.94 数据正确性修复(合思去重 + 客服指标加权)',
'本次完成内容:

【合思费控-附件去重】
- 加唯一约束防止同一附件重复入库
- 现有数据已确认 0 真重复(之前审查发现的 1 条是同附件挂在不同费用类型下, 业务正确)
- 同步工具加双重保险, 重启服务时不再生重复

【客服总览-平均指标加权】
原: 各店各天的"平均响应/平均满意度/平均转化率"直接简单平均, 大店和小店权重一样
现: 改为按业务量加权
  - 首响/响应时长 → 按"咨询人次"加权 (大店权重更大)
  - 满意度 → 按"询单数"加权
  - 转化率 → 直接用"总支付人数 / 总咨询人数"(真加权)

【影响】
- 客服总览/趋势/平台分布/店铺分布 4 个维度的平均指标数字会变化
- 大店占比的指标更接近真实, 小店少量异常值不再拉高/拉低整体
- 没业务数据时自动回退原算法, 不会出现 0

不影响日常使用, 不动定时任务',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
