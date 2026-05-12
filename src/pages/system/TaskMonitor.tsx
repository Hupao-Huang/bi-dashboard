import React, { useCallback, useEffect, useRef, useState } from 'react';
import {
  Button,
  Card,
  Col,
  Collapse,
  DatePicker,
  Input,
  InputNumber,
  Modal,
  Row,
  Space,
  Spin,
  Table,
  Tabs,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  CheckCircleOutlined,
  ClockCircleOutlined,
  CloseCircleOutlined,
  LoadingOutlined,
  ReloadOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import { API_BASE } from '../../config';

type TaskItem = {
  name: string;
  description: string;
  schedule: string;
  category: string;
  status: string;
  lastRun: string;
  lastFinish: string;
  duration: string;
  lastOutput: string;
  nextRun: string;
};

interface ManualTaskConfig {
  key: string;
  name: string;
  description: string;
  exe: string;
  params: { key: string; label: string; type: string; required: boolean; default: string }[];
}

interface RunningTaskInfo {
  id: string;
  key: string;
  name: string;
  status: 'running' | 'completed' | 'failed';
  startedAt: string;
  endedAt?: string;
  params: Record<string, string>;
  log: string;
}

const categoryOrder = ['sync', 'stock', 'ops', 'service'];

const statusConfig: Record<string, { color: string; label: string; icon: React.ReactNode }> = {
  success: { color: 'success', label: '成功', icon: <CheckCircleOutlined /> },
  failed: { color: 'error', label: '失败', icon: <CloseCircleOutlined /> },
  running: { color: 'processing', label: '运行中', icon: <SyncOutlined spin /> },
  waiting: { color: 'default', label: '等待中', icon: <ClockCircleOutlined /> },
};

const statusDotColors: Record<string, string> = {
  success: '#52c41a',
  failed: '#ff4d4f',
  running: '#1677ff',
  waiting: '#d9d9d9',
};

const runningStatusConfig: Record<string, { color: string; label: string }> = {
  running: { color: 'processing', label: '运行中' },
  completed: { color: 'success', label: '已完成' },
  failed: { color: 'error', label: '失败' },
};

const getLastOutputLines = (output: string, maxLines = 10): string => {
  if (!output) return '';
  const lines = output.split('\n');
  return lines.slice(-maxLines).join('\n');
};

const REFRESH_INTERVAL = 30_000;

const logStyle: React.CSSProperties = {
  background: '#1e293b',
  color: '#e2e8f0',
  borderRadius: 8,
  padding: 16,
  fontSize: 12,
  fontFamily: "'JetBrains Mono', 'Fira Code', 'Consolas', monospace",
  maxHeight: 300,
  overflow: 'auto',
  margin: 0,
  lineHeight: 1.6,
  whiteSpace: 'pre-wrap',
  wordBreak: 'break-all',
};

const TaskMonitor: React.FC = () => {
  const [tasks, setTasks] = useState<TaskItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);

  // 手动任务状态
  const [manualConfigs, setManualConfigs] = useState<ManualTaskConfig[]>([]);
  const [runningTasks, setRunningTasks] = useState<Record<string, RunningTaskInfo>>({});
  const [manualParams, setManualParams] = useState<Record<string, Record<string, string>>>({});
  const [startingTask, setStartingTask] = useState<string | null>(null);
  const [stoppingTask, setStoppingTask] = useState<string | null>(null);

  // 日志自动滚到底部的 ref map
  const logRefs = useRef<Record<string, HTMLPreElement | null>>({});

  // 实时日志查看 (v1.56.1)
  const [logModalOpen, setLogModalOpen] = useState(false);
  const [logModalKey, setLogModalKey] = useState<string>('');
  const [logModalName, setLogModalName] = useState<string>('');
  const [logModalText, setLogModalText] = useState<string>('');
  const [logModalLoading, setLogModalLoading] = useState(false);
  const logModalTimerRef = useRef<number | null>(null);
  const logModalBoxRef = useRef<HTMLPreElement | null>(null);

  // 哪些 task 有固定日志可看 (跟后端 fixedToolLogMap 一致)
  const LIVE_LOG_KEYS = ['sync-trades', 'sync-summary', 'snapshot-stock'];

  const fetchLiveLog = useCallback(async (key: string) => {
    try {
      const res = await fetch(`${API_BASE}/api/admin/sync-tools/log?key=${encodeURIComponent(key)}&lines=300`, {
        credentials: 'include',
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const json = await res.json();
      const text = (json.data?.lines || []).join('\n');
      setLogModalText(text || '(暂无日志)');
      // 自动滚到底
      setTimeout(() => {
        if (logModalBoxRef.current) {
          logModalBoxRef.current.scrollTop = logModalBoxRef.current.scrollHeight;
        }
      }, 50);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setLogModalText(`获取失败: ${msg}`);
    } finally {
      setLogModalLoading(false);
    }
  }, []);

  const openLogModal = useCallback((key: string, name: string) => {
    setLogModalKey(key);
    setLogModalName(name);
    setLogModalText('');
    setLogModalLoading(true);
    setLogModalOpen(true);
    fetchLiveLog(key);
    if (logModalTimerRef.current) {
      window.clearInterval(logModalTimerRef.current);
    }
    logModalTimerRef.current = window.setInterval(() => fetchLiveLog(key), 3000);
  }, [fetchLiveLog]);

  const closeLogModal = useCallback(() => {
    setLogModalOpen(false);
    if (logModalTimerRef.current) {
      window.clearInterval(logModalTimerRef.current);
      logModalTimerRef.current = null;
    }
  }, []);

  useEffect(() => {
    return () => {
      if (logModalTimerRef.current) {
        window.clearInterval(logModalTimerRef.current);
      }
    };
  }, []);

  const fetchTasks = useCallback(async (signal?: AbortSignal) => {
    try {
      const res = await fetch(`${API_BASE}/api/admin/tasks`, {
        credentials: 'include',
        signal,
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const json = await res.json();
      setTasks(json.data || []);
    } catch (err: unknown) {
      if ((err as Error)?.name === 'AbortError') return;
      const msg = err instanceof Error ? err.message : String(err);
      message.error(`获取任务数据失败: ${msg}`);
    } finally {
      setLoading(false);
    }
  }, []);

  const fetchRunning = useCallback(async (signal?: AbortSignal) => {
    try {
      const res = await fetch(`${API_BASE}/api/admin/tasks/running`, {
        credentials: 'include',
        signal,
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const json = await res.json();
      const d = json.data || json;
      setManualConfigs(d.configs || []);
      setRunningTasks(d.running || {});
    } catch (err: unknown) {
      if ((err as Error)?.name === 'AbortError') return;
      const msg = err instanceof Error ? err.message : String(err);
      message.error(`获取手动任务数据失败: ${msg}`);
    }
  }, []);

  const fetchAll = useCallback(async (signal?: AbortSignal) => {
    await Promise.all([fetchTasks(signal), fetchRunning(signal)]);
  }, [fetchTasks, fetchRunning]);

  useEffect(() => {
    const ctrl = new AbortController();
    fetchAll(ctrl.signal);
    const timer = setInterval(() => fetchAll(ctrl.signal), REFRESH_INTERVAL);
    return () => {
      clearInterval(timer);
      ctrl.abort();
    };
  }, [fetchAll]);

  // 初始化手动任务参数默认值
  useEffect(() => {
    if (manualConfigs.length === 0) return;
    setManualParams(prev => {
      const next = { ...prev };
      for (const cfg of manualConfigs) {
        if (!next[cfg.key]) {
          const defaults: Record<string, string> = {};
          for (const p of cfg.params) {
            defaults[p.key] = p.default || '';
          }
          next[cfg.key] = defaults;
        }
      }
      return next;
    });
  }, [manualConfigs]);

  // 日志自动滚到底部
  useEffect(() => {
    for (const [taskId, info] of Object.entries(runningTasks)) {
      if (info.log && logRefs.current[taskId]) {
        const el = logRefs.current[taskId];
        if (el) {
          el.scrollTop = el.scrollHeight;
        }
      }
    }
  }, [runningTasks]);

  const handleRefresh = async () => {
    setRefreshing(true);
    await fetchAll();
    setRefreshing(false);
  };

  const handleParamChange = (taskKey: string, paramKey: string, value: string) => {
    setManualParams(prev => ({
      ...prev,
      [taskKey]: {
        ...(prev[taskKey] || {}),
        [paramKey]: value,
      },
    }));
  };

  const handleRunTask = (config: ManualTaskConfig) => {
    const params = manualParams[config.key] || {};
    // 检查必填参数
    for (const p of config.params) {
      if (p.required && !params[p.key]) {
        message.warning(`请填写参数: ${p.label}`);
        return;
      }
    }

    Modal.confirm({
      title: '确认运行任务',
      content: `确定要运行「${config.name}」吗?`,
      okText: '运行',
      cancelText: '取消',
      onOk: async () => {
        setStartingTask(config.key);
        try {
          const res = await fetch(`${API_BASE}/api/admin/tasks/run`, {
            method: 'POST',
            credentials: 'include',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ task: config.key, params }),
          });
          if (!res.ok) {
            const text = await res.text();
            throw new Error(text || `HTTP ${res.status}`);
          }
          message.success(`任务「${config.name}」已启动`);
          await fetchRunning();
        } catch (err: unknown) {
          const msg = err instanceof Error ? err.message : String(err);
          message.error(`启动任务失败: ${msg}`);
        } finally {
          setStartingTask(null);
        }
      },
    });
  };

  const handleStopTask = (taskId: string, taskName: string) => {
    Modal.confirm({
      title: '确认停止任务',
      content: `确定要停止「${taskName}」吗?`,
      okText: '停止',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        setStoppingTask(taskId);
        try {
          const res = await fetch(`${API_BASE}/api/admin/tasks/stop`, {
            method: 'POST',
            credentials: 'include',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ taskId }),
          });
          if (!res.ok) {
            const text = await res.text();
            throw new Error(text || `HTTP ${res.status}`);
          }
          message.success(`任务「${taskName}」已停止`);
          await fetchRunning();
        } catch (err: unknown) {
          const msg = err instanceof Error ? err.message : String(err);
          message.error(`停止任务失败: ${msg}`);
        } finally {
          setStoppingTask(null);
        }
      },
    });
  };

  const isTaskRunning = (taskKey: string): boolean => {
    return Object.values(runningTasks).some(t => t.key === taskKey && t.status === 'running');
  };

  const formatDuration = (startedAt: string, endedAt?: string): string => {
    const start = dayjs(startedAt);
    const end = endedAt ? dayjs(endedAt) : dayjs();
    const diffSec = end.diff(start, 'second');
    if (diffSec < 60) return `${diffSec}秒`;
    if (diffSec < 3600) return `${Math.floor(diffSec / 60)}分${diffSec % 60}秒`;
    const hours = Math.floor(diffSec / 3600);
    const mins = Math.floor((diffSec % 3600) / 60);
    return `${hours}时${mins}分`;
  };

  const renderParamInput = (config: ManualTaskConfig, param: ManualTaskConfig['params'][0]) => {
    const value = manualParams[config.key]?.[param.key] || '';
    const disabled = isTaskRunning(config.key);

    switch (param.type) {
      case 'date':
        return (
          <DatePicker
            value={value ? dayjs(value) : null}
            onChange={(_d, dateStr) => handleParamChange(config.key, param.key, dateStr as string)}
            style={{ width: '100%' }}
            disabled={disabled}
            placeholder={param.label}
          />
        );
      case 'number':
        return (
          <InputNumber
            value={value ? Number(value) : undefined}
            onChange={v => handleParamChange(config.key, param.key, v != null ? String(v) : '')}
            style={{ width: '100%' }}
            disabled={disabled}
            placeholder={param.label}
          />
        );
      default:
        return (
          <Input
            value={value}
            onChange={e => handleParamChange(config.key, param.key, e.target.value)}
            disabled={disabled}
            placeholder={param.label}
          />
        );
    }
  };

  // 统计数据
  const totalCount = tasks.length;
  const runningCount = tasks.filter(t => t.status === 'running').length;
  const successCount = tasks.filter(t => t.status === 'success').length;
  const failedCount = tasks.filter(t => t.status === 'failed').length;

  // 按 category 分组
  const groupedTasks = categoryOrder.reduce<Record<string, TaskItem[]>>((acc, cat) => {
    const items = tasks.filter(t => t.category === cat);
    if (items.length > 0) acc[cat] = items;
    return acc;
  }, {});

  // 未归类的放到最后
  const knownCategories = new Set(categoryOrder);
  const uncategorized = tasks.filter(t => !knownCategories.has(t.category));
  if (uncategorized.length > 0) groupedTasks['other'] = uncategorized;

  // 运行中的任务列表
  const runningTaskList = Object.values(runningTasks).sort((a, b) =>
    new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime()
  );

  // 定时任务表格列定义
  const scheduledColumns: ColumnsType<TaskItem> = [
    {
      title: '状态',
      dataIndex: 'status',
      width: 60,
      align: 'center',
      render: (status: string) => {
        const cfg = statusConfig[status] || statusConfig.waiting;
        const dotColor = statusDotColors[status] || statusDotColors.waiting;
        return (
          <Tooltip title={cfg.label}>
            <span style={{
              display: 'inline-block',
              width: 10,
              height: 10,
              borderRadius: '50%',
              backgroundColor: dotColor,
              boxShadow: status === 'running' ? `0 0 6px ${dotColor}` : 'none',
            }} />
          </Tooltip>
        );
      },
    },
    {
      title: '任务名称',
      dataIndex: 'name',
      width: 130,
      render: (name: string) => (
        <Typography.Text strong style={{ fontSize: 14 }}>{name}</Typography.Text>
      ),
    },
    {
      title: '说明',
      dataIndex: 'description',
      ellipsis: true,
      render: (desc: string) => (
        <Typography.Text type="secondary" style={{ fontSize: 13 }}>{desc}</Typography.Text>
      ),
    },
    {
      title: '调度',
      dataIndex: 'schedule',
      render: (val: string) => (
        <Tag style={{ fontSize: 12, whiteSpace: 'nowrap' }}>{val}</Tag>
      ),
    },
    {
      title: '上次运行',
      dataIndex: 'lastRun',
      width: 160,
      render: (val: string) => (
        <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>{val || '-'}</span>
      ),
    },
    {
      title: '耗时',
      dataIndex: 'duration',
      width: 70,
      render: (val: string) => (
        <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>{val || '-'}</span>
      ),
    },
    {
      title: '下次运行',
      dataIndex: 'nextRun',
      width: 160,
      render: (val: string) => (
        <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>{val || '-'}</span>
      ),
    },
  ];

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 400 }}>
        <Spin size="large" />
      </div>
    );
  }

  return (
    <div>
      <Tabs
        type="card"
        tabBarExtraContent={
          <Tooltip title="刷新数据">
            <Button
              type="default"
              shape="circle"
              icon={<ReloadOutlined spin={refreshing} />}
              onClick={handleRefresh}
              loading={refreshing}
              style={{ marginRight: 4 }}
            />
          </Tooltip>
        }
        items={[
          {
            key: 'scheduled',
            label: '定时任务',
            children: (
              <div>
                {/* 统计卡片 */}
                <Row gutter={16} style={{ marginBottom: 20 }}>
                  <Col span={6}>
                    <Card
                      size="small"
                      style={{ background: 'var(--card-bg)', borderRadius: 'var(--card-radius)', boxShadow: 'var(--card-shadow)' }}
                      styles={{ body: { padding: '12px 16px' } }}
                    >
                      <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 4 }}>任务总数</div>
                      <div style={{ fontSize: 24, fontWeight: 600, color: 'var(--text-primary)' }}>{totalCount}</div>
                    </Card>
                  </Col>
                  <Col span={6}>
                    <Card
                      size="small"
                      style={{ background: 'var(--card-bg)', borderRadius: 'var(--card-radius)', boxShadow: 'var(--card-shadow)' }}
                      styles={{ body: { padding: '12px 16px' } }}
                    >
                      <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 4 }}>运行中</div>
                      <div style={{ fontSize: 24, fontWeight: 600, color: '#1677ff' }}>
                        <SyncOutlined spin={runningCount > 0} style={{ fontSize: 16, marginRight: 8 }} />
                        {runningCount}
                      </div>
                    </Card>
                  </Col>
                  <Col span={6}>
                    <Card
                      size="small"
                      style={{ background: 'var(--card-bg)', borderRadius: 'var(--card-radius)', boxShadow: 'var(--card-shadow)' }}
                      styles={{ body: { padding: '12px 16px' } }}
                    >
                      <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 4 }}>成功</div>
                      <div style={{ fontSize: 24, fontWeight: 600, color: '#52c41a' }}>
                        <CheckCircleOutlined style={{ fontSize: 16, marginRight: 8 }} />
                        {successCount}
                      </div>
                    </Card>
                  </Col>
                  <Col span={6}>
                    <Card
                      size="small"
                      style={{ background: 'var(--card-bg)', borderRadius: 'var(--card-radius)', boxShadow: 'var(--card-shadow)' }}
                      styles={{ body: { padding: '12px 16px' } }}
                    >
                      <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 4 }}>失败</div>
                      <div style={{ fontSize: 24, fontWeight: 600, color: '#ff4d4f' }}>
                        <CloseCircleOutlined style={{ fontSize: 16, marginRight: 8 }} />
                        {failedCount}
                      </div>
                    </Card>
                  </Col>
                </Row>

                {/* 定时任务表格 */}
                <Table<TaskItem>
                  columns={scheduledColumns}
                  dataSource={tasks}
                  rowKey="name"
                  pagination={false}
                  size="middle"
                  style={{
                    background: 'var(--card-bg)',
                    borderRadius: 'var(--card-radius)',
                    boxShadow: 'var(--card-shadow)',
                    overflow: 'hidden',
                  }}
                  expandable={{
                    expandedRowRender: (record) => {
                      const logContent = getLastOutputLines(record.lastOutput);
                      return logContent ? (
                        <pre style={{ ...logStyle, maxHeight: 200 }}>{logContent}</pre>
                      ) : (
                        <Typography.Text type="secondary">暂无日志输出</Typography.Text>
                      );
                    },
                    rowExpandable: () => true,
                  }}
                  locale={{ emptyText: '暂无任务数据' }}
                />

                {/* 底部刷新提示 */}
                <div style={{ textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 12, marginTop: 16 }}>
                  <LoadingOutlined style={{ marginRight: 6 }} />
                  每 30 秒自动刷新
                </div>
              </div>
            ),
          },
          {
            key: 'manual',
            label: '手动任务',
            children: (
              <Row gutter={20}>
                {/* 左侧：手动任务列表 */}
                <Col span={14}>
                  <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 12 }}>
                    任务列表
                  </div>
                  {manualConfigs.length > 0 ? (
                    <Row gutter={[12, 12]}>
                      {manualConfigs.map(config => {
                        const running = isTaskRunning(config.key);
                        const isStarting = startingTask === config.key;
                        const hasParams = config.params.length > 0;

                        return (
                          <Col span={12} key={config.key}>
                            <Card
                              style={{
                                background: 'var(--card-bg)',
                                borderRadius: 12,
                                boxShadow: 'var(--card-shadow)',
                                height: '100%',
                              }}
                              styles={{ body: { padding: '14px 16px' } }}
                            >
                              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: hasParams ? 10 : 0 }}>
                                <div style={{ flex: 1, minWidth: 0 }}>
                                  <Typography.Text strong style={{ fontSize: 14 }}>
                                    {config.name}
                                  </Typography.Text>
                                  <div>
                                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                                      {config.description}
                                    </Typography.Text>
                                  </div>
                                </div>
                                <Space size={4} style={{ marginLeft: 8, flexShrink: 0 }}>
                                  {LIVE_LOG_KEYS.includes(config.key) && (
                                    <Button
                                      size="small"
                                      onClick={() => openLogModal(config.key, config.name)}
                                    >
                                      看日志
                                    </Button>
                                  )}
                                  <Button
                                    type="primary"
                                    size="small"
                                    disabled={running}
                                    loading={isStarting}
                                    onClick={() => handleRunTask(config)}
                                  >
                                    {running ? '运行中' : '运行'}
                                  </Button>
                                </Space>
                              </div>
                              {hasParams && (
                                <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center' }}>
                                  {config.params.map(param => (
                                    <div key={param.key} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                                      <Typography.Text style={{ fontSize: 12, whiteSpace: 'nowrap', color: 'var(--text-secondary)' }}>
                                        {param.label}:
                                      </Typography.Text>
                                      <div style={{ width: 120 }}>
                                        {renderParamInput(config, param)}
                                      </div>
                                    </div>
                                  ))}
                                </div>
                              )}
                            </Card>
                          </Col>
                        );
                      })}
                    </Row>
                  ) : (
                    <Card style={{ textAlign: 'center', color: 'var(--text-tertiary)', padding: 40 }}>
                      暂无手动任务配置
                    </Card>
                  )}
                  <div style={{ textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 12, marginTop: 16 }}>
                    <LoadingOutlined style={{ marginRight: 6 }} />
                    每 30 秒自动刷新
                  </div>
                </Col>

                {/* 右侧：运行中的任务 */}
                <Col span={10}>
                  <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 12 }}>
                    运行记录
                  </div>
                  {runningTaskList.length > 0 ? (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                      {runningTaskList.map(task => {
                        const sCfg = runningStatusConfig[task.status] || runningStatusConfig.running;
                        const isStopping = stoppingTask === task.id;
                        const paramEntries = Object.entries(task.params || {});

                        return (
                          <Card
                            key={task.id}
                            style={{
                              background: 'var(--card-bg)',
                              borderRadius: 10,
                              boxShadow: 'var(--card-shadow)',
                            }}
                            styles={{ body: { padding: '12px 16px' } }}
                          >
                            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 6 }}>
                              <Space size={8} wrap>
                                <Tag color={sCfg.color} style={{ fontSize: 12 }}>{sCfg.label}</Tag>
                                <Typography.Text strong style={{ fontSize: 13 }}>
                                  {task.name}
                                </Typography.Text>
                                {paramEntries.length > 0 && (
                                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                                    ({paramEntries.map(([, v]) => v).join(' ~ ')})
                                  </Typography.Text>
                                )}
                              </Space>
                              {task.status === 'running' && (
                                <Button
                                  danger
                                  size="small"
                                  loading={isStopping}
                                  onClick={() => handleStopTask(task.id, task.name)}
                                >
                                  停止
                                </Button>
                              )}
                            </div>
                            <div style={{ fontSize: 12, color: 'var(--text-tertiary)', marginBottom: 4 }}>
                              {dayjs(task.startedAt).format('MM-DD HH:mm:ss')} | 耗时: {formatDuration(task.startedAt, task.endedAt)}
                            </div>
                            <Collapse
                              ghost
                              size="small"
                              defaultActiveKey={task.status === 'running' ? ['log'] : []}
                              items={[
                                {
                                  key: 'log',
                                  label: <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>日志</span>,
                                  children: task.log ? (
                                    <pre
                                      ref={el => { logRefs.current[task.id] = el; }}
                                      style={{ ...logStyle, maxHeight: 200 }}
                                    >
                                      {task.log}
                                    </pre>
                                  ) : (
                                    <pre style={{ ...logStyle, maxHeight: 200 }}>暂无日志输出</pre>
                                  ),
                                },
                              ]}
                            />
                          </Card>
                        );
                      })}
                    </div>
                  ) : (
                    <div style={{ textAlign: 'center', color: 'var(--text-tertiary)', padding: '40px 0', fontSize: 13 }}>
                      暂无运行记录
                    </div>
                  )}
                </Col>
              </Row>
            ),
          },
        ]}
      />

      <Modal
        title={`实时日志 - ${logModalName}`}
        open={logModalOpen}
        onCancel={closeLogModal}
        footer={[
          <Button key="refresh" icon={<ReloadOutlined />} onClick={() => fetchLiveLog(logModalKey)}>立即刷新</Button>,
          <Button key="close" type="primary" onClick={closeLogModal}>关闭</Button>,
        ]}
        width={900}
      >
        <div style={{ color: 'var(--text-tertiary)', fontSize: 12, marginBottom: 8 }}>
          每 3 秒自动刷新, 显示末尾 300 行
        </div>
        <Spin spinning={logModalLoading && !logModalText}>
          <pre
            ref={logModalBoxRef}
            style={{
              background: '#1e1e1e',
              color: '#d4d4d4',
              padding: 12,
              borderRadius: 6,
              height: 500,
              overflow: 'auto',
              fontSize: 12,
              margin: 0,
              fontFamily: 'Consolas, Monaco, monospace',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-all',
            }}
          >
            {logModalText || '加载中...'}
          </pre>
        </Spin>
      </Modal>
    </div>
  );
};

export default TaskMonitor;
