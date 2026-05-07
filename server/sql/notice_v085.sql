-- v0.85 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.85 系统页错误提示完善',
'本次修复内容:

【系统页面错误兜底完善】
1. 个人中心: 解绑钉钉 / 修改密码 / 保存资料 / 上传头像 — 操作失败时现在会显示具体错误原因, 不再静默
2. 公告管理: 删除 / 切换启用状态 / 切换置顶 — 操作失败时现在会显示提示, 不再以为成功

【后端协议适配】
所有失败提示按后端实际协议读取错误信息(优先 msg 字段, 兼容旧 error 字段)

不影响日常使用, 不动定时任务',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
