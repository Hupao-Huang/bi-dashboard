-- v0.90 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.90 后端代码结构重构完成(Sprint 4 阶段一收官)',
'本次完成内容:

【代码结构】
后端 dashboard.go 主文件由 3926 行重构到只剩 49 行核心配置, 拆出 12 个独立模块:
- 综合看板 / 部门详情 / S 品分析 / 营销费用
- 6 个平台独立看板 (天猫 / 拼多多 / 京东 / 唯品会 / 飞瓜 / 客服总览)
- 缓存模块 / 公共 helpers

【效果】
- 主文件减小 99% (3926 → 49 行)
- 每个看板模块独立维护, 改动互不影响
- 行为零改动: 所有接口数字 / 路由地址 / 缓存策略全部不变

后续: 数据导入工具公共代码抽取(下次版本)

不影响日常使用, 不动定时任务',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
