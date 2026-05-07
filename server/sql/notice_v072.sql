-- v0.72 公告
UPDATE notices SET is_pinned=0 WHERE is_pinned=1;

INSERT INTO notices (title, content, is_pinned, created_by, created_at) VALUES (
'v0.72 同步按钮三重防御 — 不会再"自动重新同步"',
'跑哥反馈: v0.71 进度条上线后, 同步完成又自动重新同步。

【根因排查】
经过日志和代码审计, 这种现象只可能在以下情况出现:
- 浏览器加载的还是旧版前端 (Ctrl+F5 没强制刷新缓存)
- 跑哥关闭进度条后又点了一次按钮 (无意识)
- 异步状态更新瞬间被绕过

【v0.72 三重防御】

1️⃣ 同步按钮 disabled 范围扩大
   - 之前: 同步进行中 disabled
   - 现在: 同步进行中 + 进度框还没关闭 都 disabled
   - 跑哥必须先关闭进度框, 才能再次点击

2️⃣ 30 秒冷却期 (cooldown)
   - 上次同步完成后 30 秒内, 即使绕过前面所有防御, 后端也会拒绝
   - 防止双击/异步竞态/快速重试导致的意外触发

3️⃣ 后端互斥锁 (v0.71 已有)
   - bi-server 进程级锁, 同一时刻最多 1 个同步在跑
   - 第二个请求会被 429 拒绝

【三重防御组合后】
即使跑哥连点 10 次同步按钮:
- 第 1 次: ✓ 启动同步
- 第 2-N 次 (Modal 弹出期间): ✗ 按钮 disabled, 无反应
- Modal 关闭后 30 秒内再点: ✗ 提示"上次同步刚完成, X 秒后再试"
- 30 秒后: ✓ 可启动新一轮

【请跑哥操作】
1. 浏览器按 Ctrl + Shift + R 强制刷新 (这次 v0.72 build hash 不同, 会自动加载新版)
2. 重新点击"立即同步全部 YS 数据"
3. 进度条全程可见, 完成后点关闭, 30 秒内不会再自动跑',
1, 'system', NOW());

SELECT id, title, is_pinned, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i') AS 时间 FROM notices WHERE is_pinned=1;
