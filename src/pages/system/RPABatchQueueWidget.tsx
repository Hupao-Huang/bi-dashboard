// 右下角固定浮窗: 显示后端批量同步队列状态
// 5 秒轮询 /api/admin/rpa/batch-queue, Badge 显示 (running + pending) 总数
// 点击展开 Drawer 看每个 batch 的进度

import React, { useCallback, useEffect, useState } from 'react';
import { Button, Drawer, Empty, FloatButton, Space, Tag, Tooltip } from 'antd';
import { CloudSyncOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';

interface BatchItem {
  date: string;
  status: 'pending' | 'running' | 'finish' | 'error' | 'cancel' | 'timeout';
  trigger_id?: number;
  started_at?: string;
  finished_at?: string;
  err_msg?: string;
}

interface BatchState {
  batch_id: number;
  platform: string;
  user: string;
  started_at: string;
  items: BatchItem[];
}

interface QueueResp {
  batches: BatchState[];
  summary: { running: number; pending: number; finish: number; error: number; timeout: number };
}

const STATUS_TAG: Record<BatchItem['status'], { color: string; label: string }> = {
  pending: { color: 'default', label: '等待中' },
  running: { color: 'processing', label: '同步中' },
  finish:  { color: 'success', label: '已完成' },
  error:   { color: 'error', label: '失败' },
  cancel:  { color: 'warning', label: '取消' },
  timeout: { color: 'warning', label: '超时' },
};

const RPABatchQueueWidget: React.FC = () => {
  const [data, setData] = useState<QueueResp | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);

  const fetchQueue = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/api/admin/rpa/batch-queue`, { credentials: 'include' });
      const j = await res.json();
      if (j?.code === 200) setData(j.data);
    } catch {
      // 忽略, 5s 后重试
    }
  }, []);

  useEffect(() => {
    fetchQueue();
    const t = setInterval(fetchQueue, 5000);
    return () => clearInterval(t);
  }, [fetchQueue]);

  const summary = data?.summary || { running: 0, pending: 0, finish: 0, error: 0, timeout: 0 };
  const activeCount = summary.running + summary.pending;

  // 只在有运行中/等待中任务时才显示浮窗, 跑完自动藏起
  if (activeCount === 0) {
    if (drawerOpen) setDrawerOpen(false);
    return null;
  }

  return (
    <>
      <FloatButton
        icon={<CloudSyncOutlined spin={summary.running > 0} />}
        type={activeCount > 0 ? 'primary' : 'default'}
        onClick={() => setDrawerOpen(true)}
        tooltip={`后台同步队列: ${summary.running} 跑中 / ${summary.pending} 等待 / ${summary.finish} 已完成`}
        badge={{ count: activeCount, color: 'red' }}
        style={{ right: 24, bottom: 80 }}
      />

      <Drawer
        title="后台同步队列"
        placement="right"
        width={520}
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        extra={<Button size="small" onClick={fetchQueue}>刷新</Button>}
      >
        <Space wrap style={{ marginBottom: 16 }}>
          <Tag color="processing">同步中 {summary.running}</Tag>
          <Tag color="default">等待中 {summary.pending}</Tag>
          <Tag color="success">已完成 {summary.finish}</Tag>
          {summary.error > 0 && <Tag color="error">失败 {summary.error}</Tag>}
          {summary.timeout > 0 && <Tag color="warning">超时 {summary.timeout}</Tag>}
        </Space>

        {(!data || data.batches.length === 0) ? (
          <Empty description="暂无队列任务" />
        ) : (
          data.batches.map(batch => {
            const finishedCount = batch.items.filter(i =>
              i.status === 'finish' || i.status === 'error' || i.status === 'cancel' || i.status === 'timeout'
            ).length;
            return (
              <div key={batch.batch_id} style={{ marginBottom: 24, border: '1px solid #f0f0f0', borderRadius: 6, padding: 12 }}>
                <div style={{ marginBottom: 8 }}>
                  <Space>
                    <strong>{batch.platform}</strong>
                    <span>{finishedCount}/{batch.items.length}</span>
                    <span>{batch.user} · {batch.started_at}</span>
                  </Space>
                </div>
                <div>
                  {batch.items.map(it => {
                    const tag = STATUS_TAG[it.status];
                    return (
                      <div key={it.date} style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '4px 0', borderBottom: '1px solid #fafafa' }}>
                        <span style={{ minWidth: 100 }}>{it.date}</span>
                        <Tag color={tag.color}>{tag.label}</Tag>
                        {it.err_msg && (
                          <Tooltip title={it.err_msg}>
                            <span style={{ maxWidth: 220, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                              {it.err_msg}
                            </span>
                          </Tooltip>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>
            );
          })
        )}
      </Drawer>
    </>
  );
};

export default RPABatchQueueWidget;
