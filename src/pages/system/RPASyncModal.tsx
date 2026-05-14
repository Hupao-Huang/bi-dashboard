// 影刀 RPA 同步进度 Modal
//
// 跑哥点 "立即同步本平台" 按钮 → 后端调影刀 → 拿 trigger_id → 这个 Modal 打开
// 5 秒一次轮询 /api/admin/rpa/job-status, 显示状态 + 实时日志
// 完成 → toast + 自动关闭 + 触发外部 onDone (刷新主表)
import React, { useEffect, useRef, useState, useCallback } from 'react';
import { Modal, Progress, Statistic, Tag, Typography, Empty, Button, message } from 'antd';
import { SyncOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';

const { Text } = Typography;

interface LogItem {
  time: string;
  level: string;
  text: string;
  logId: number;
}

interface JobStatus {
  trigger_id: number;
  platform: string;
  robot_name: string;
  job_uuid: string;
  status: string;
  status_name: string;
  remark: string;
  start_time: string;
  end_time: string;
  elapsed_sec: number;
  logs: LogItem[];
  done: boolean;
}

interface Props {
  triggerId: number | null;
  platform: string;
  robotName: string;
  date?: string; // 业务日期 (YYYY-MM-DD), 仅展示用
  open: boolean;
  onClose: () => void;
  onDone?: () => void;
}

const POLL_INTERVAL_MS = 5000;

const RPASyncModal: React.FC<Props> = ({ triggerId, platform, robotName, date, open, onClose, onDone }) => {
  const [status, setStatus] = useState<JobStatus | null>(null);
  const [logs, setLogs] = useState<LogItem[]>([]);
  const [localElapsed, setLocalElapsed] = useState(0); // 本地秒表 1s 一跳, 不等后端 5s 轮询
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const tickRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const logsBoxRef = useRef<HTMLDivElement>(null);
  const doneRef = useRef(false);

  const stopPoll = useCallback(() => {
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const poll = useCallback(async () => {
    if (!triggerId) return;
    try {
      const res = await fetch(`${API_BASE}/api/admin/rpa/job-status?trigger_id=${triggerId}`, {
        credentials: 'include',
      });
      const j = await res.json();
      if (j.code !== 200) return;
      const d = j.data as JobStatus;
      setStatus(d);
      // 合并日志（按 logId 去重，留最近 300 条）
      if (d.logs && d.logs.length > 0) {
        setLogs(prev => {
          const seen = new Set(prev.map(l => l.logId));
          const fresh = d.logs.filter(l => !seen.has(l.logId));
          if (fresh.length === 0) return prev;
          const merged = [...prev, ...fresh].sort((a, b) => a.logId - b.logId);
          return merged.slice(-300);
        });
      }
      // 终态 → 停止轮询 + toast + 触发 onDone (不自动关 modal, 让跑哥看清结果再手动关)
      if (d.done && !doneRef.current) {
        doneRef.current = true;
        stopPoll();
        const ok = d.status === 'finish';
        if (ok) {
          message.success(`${platform} 同步完成（耗时 ${formatTime(d.elapsed_sec)}）`);
        } else {
          message.error(`${platform} 同步失败（${d.status_name || d.status}）`);
        }
        onDone?.();
      }
    } catch (e) {
      // 静默失败，下一轮再试
    }
  }, [triggerId, platform, stopPoll, onDone, onClose]);

  // 启动轮询
  useEffect(() => {
    if (!open || !triggerId) {
      stopPoll();
      return;
    }
    doneRef.current = false;
    setStatus(null);
    setLogs([]);
    setLocalElapsed(0);
    poll();
    timerRef.current = setInterval(poll, POLL_INTERVAL_MS);
    return stopPoll;
  }, [open, triggerId, poll, stopPoll]);

  // 后端 5s 一次返回 elapsed_sec, 用作秒表基准 + 1s 本地递增
  useEffect(() => {
    if (status?.elapsed_sec !== undefined) {
      setLocalElapsed(status.elapsed_sec);
    }
  }, [status?.elapsed_sec]);

  useEffect(() => {
    if (!open || !status || status.done) {
      if (tickRef.current) {
        clearInterval(tickRef.current);
        tickRef.current = null;
      }
      return;
    }
    tickRef.current = setInterval(() => setLocalElapsed(s => s + 1), 1000);
    return () => {
      if (tickRef.current) {
        clearInterval(tickRef.current);
        tickRef.current = null;
      }
    };
  }, [open, status?.done, status]);

  // 自动滚到日志最新
  useEffect(() => {
    if (logsBoxRef.current) {
      logsBoxRef.current.scrollTop = logsBoxRef.current.scrollHeight;
    }
  }, [logs.length]);

  const statusTagColor = (s: string) => {
    if (s === 'finish') return 'success';
    if (['error', 'fail', 'cancel'].includes(s)) return 'error';
    if (s === 'running') return 'processing';
    return 'warning';
  };

  return (
    <Modal
      title={
        <span>
          同步进度 · <Text strong>{platform}</Text>
          {date && <Text strong style={{ marginLeft: 8 }}>· {date}</Text>}
          {robotName && (
            <Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>（{robotName}）</Text>
          )}
        </span>
      }
      open={open}
      onCancel={onClose}
      width={720}
      maskClosable={false}
      footer={
        <Button onClick={onClose}>
          {status?.done ? '关闭' : '最小化（后台继续跑，跑完发钉钉通知）'}
        </Button>
      }
    >
      {!status ? (
        <div style={{ textAlign: 'center', padding: 40 }}>
          <Text type="secondary">正在启动影刀任务...</Text>
        </div>
      ) : (
        <>
          {/* 顶部 KPI */}
          <div style={{ display: 'flex', gap: 32, marginBottom: 16 }}>
            <Statistic title="状态" valueRender={() => (
              <Tag color={statusTagColor(status.status)} style={{ fontSize: 14, padding: '2px 10px' }}>
                {status.status_name || status.status}
              </Tag>
            )} value={0} />
            <Statistic title="已用时" value={formatTime(localElapsed)} />
            {status.start_time && (
              <Statistic title="开始时间" value={status.start_time} valueStyle={{ fontSize: 14 }} />
            )}
          </div>

          {status.remark && (
            <div style={{ marginBottom: 12 }}>
              <Text type="secondary" style={{ fontSize: 12 }}>影刀备注：{status.remark}</Text>
            </div>
          )}

          {/* 进度: 影刀 API 没真实进度百分比, 终态显示完整条, 进行中显示动画"不定进度"风格 */}
          {status.done ? (
            <Progress
              percent={100}
              status={status.status === 'error' || status.status === 'fail' ? 'exception' : 'success'}
              style={{ marginBottom: 20 }}
            />
          ) : (
            <div style={{
              marginBottom: 20,
              padding: '8px 12px',
              borderRadius: 4,
              background: '#f0f5ff',
              border: '1px solid #d6e4ff',
              fontSize: 13,
              color: '#1677ff',
              display: 'flex', alignItems: 'center', gap: 8,
            }}>
              <SyncOutlined spin />
              <span>影刀正在执行（影刀未提供进度百分比，请通过下方日志和已用时观察进度）</span>
            </div>
          )}

          {/* 日志区 */}
          <div style={{ marginBottom: 8 }}>
            <Text strong>影刀执行日志</Text>
            <Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>
              （5 秒刷新一次，共 {logs.length} 条）
            </Text>
          </div>
          <div
            ref={logsBoxRef}
            style={{
              maxHeight: 320,
              overflow: 'auto',
              background: '#fafafa',
              border: '1px solid #f0f0f0',
              borderRadius: 4,
              padding: 8,
            }}
          >
            {logs.length === 0 ? (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={status?.done ? '影刀本次未返回执行日志' : '暂无日志（机器人启动中...）'}
              />
            ) : (
              logs.map(l => (
                <div key={l.logId} style={{ display: 'flex', gap: 8, padding: '3px 0', fontSize: 12 }}>
                  <Text type="secondary" style={{ minWidth: 70 }}>{shortTime(l.time)}</Text>
                  <Tag
                    color={l.level === '错误' || l.level === 'error' ? 'red' : 'default'}
                    style={{ margin: 0, fontSize: 11, lineHeight: '18px', padding: '0 4px' }}
                  >
                    {l.level || '信息'}
                  </Tag>
                  <Text style={{ flex: 1 }}>{l.text}</Text>
                </div>
              ))
            )}
          </div>

          <div style={{ marginTop: 12, fontSize: 12 }}>
            <Text type="secondary">
              💡 这个窗口可以关掉，影刀会继续在后台跑。跑完会通过钉钉发通知，BI 数据也会自动刷新。
            </Text>
          </div>
        </>
      )}
    </Modal>
  );
};

function formatTime(sec: number): string {
  if (!sec || sec < 0) return '0 秒';
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  if (m === 0) return `${s} 秒`;
  return `${m} 分 ${s} 秒`;
}

function shortTime(t: string): string {
  // "03/20/2024 15:35:23" → "15:35:23"
  if (!t) return '';
  const parts = t.split(' ');
  return parts[parts.length - 1] || t;
}

export default RPASyncModal;
