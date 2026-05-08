-- v0.93 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.93 数据正确性 + 系统稳定性修复(HIGH ROI 6 项)',
'本次完成内容:

【数据修正】
- 抖音自营直播页"总场次/趋势"双主播翻倍问题已修
  注: 之后看到的场次数字可能比之前小, 这是正确的(去重了双主播一场)

【访问权限】
- 特殊渠道对账页加了访问权限, 只有授权用户能看
  注: 之前任意登录用户都能进, 现已锁定财务相关角色

【内部修复】
- 修了 RPA 自动同步触发按钮的调度异常问题(之前一直失败)
- 钉钉扫码注册超短账号场景崩溃问题修了
- 审计日志查询出错改为记录日志, 不再静默吞错
- 审计日志页代码结构优化, 翻页/搜索更稳定

不影响日常使用, 不动定时任务',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
