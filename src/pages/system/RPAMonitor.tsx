import React, { useEffect, useState, useCallback, useRef, useMemo } from 'react';
import {
  Card,
  Table,
  Button,
  Tabs,
  Drawer,
  Tag,
  Tooltip,
  Spin,
  Typography,
  Space,
  Progress,
  Radio,
  Input,
  Modal,
  message,
  Badge,
  List,
  Empty,
} from 'antd';
import { ReloadOutlined, CheckCircleOutlined, WarningOutlined, CloseCircleOutlined, MinusCircleOutlined, SearchOutlined, SyncOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';
import RPASyncModal from './RPASyncModal';

const { Text } = Typography;

// ─── Types ───────────────────────────────────────────────────────────────────

type ItemStatus = 'complete' | 'partial' | 'missing' | 'no_dir';

interface DataItem {
  name: string;
  status: ItemStatus;
}

interface CellDetail {
  platform: string;
  store: string;
  date: string;
  status: ItemStatus;
  items: DataItem[];
}

interface DateStoreEntry {
  status: ItemStatus;
  items: DataItem[];
}

interface DateMeta {
  dbImported: boolean;
  fileStatus: ItemStatus; // complete / partial / missing
}

interface PlatformData {
  name: string;
  stores: string[];
  dates: string[];
  /** grid[date][store] = DateStoreEntry */
  grid: Record<string, Record<string, DateStoreEntry>>;
  dateMeta: Record<string, DateMeta>;
  completeness: number; // 0-100
}

interface ScanResult {
  scannedAt: string;
  scanning: boolean;
  platforms: PlatformData[];
}

// ─── Constants ───────────────────────────────────────────────────────────────

const STATUS_COLOR: Record<ItemStatus, string> = {
  complete: '#10b981',
  partial: '#f59e0b',
  missing: '#ef4444',
  no_dir: '#94a3b8',
};

const STATUS_ICON: Record<ItemStatus, React.ReactNode> = {
  complete: <CheckCircleOutlined style={{ color: '#10b981' }} />,
  partial: <WarningOutlined style={{ color: '#f59e0b' }} />,
  missing: <CloseCircleOutlined style={{ color: '#ef4444' }} />,
  no_dir: <MinusCircleOutlined style={{ color: '#94a3b8' }} />,
};

const STATUS_LABEL: Record<ItemStatus, string> = {
  complete: '完整',
  partial: '部分',
  missing: '缺失',
  no_dir: '无目录',
};

// ─── Cell component ───────────────────────────────────────────────────────────

interface CellProps {
  entry: DateStoreEntry | undefined;
  onClick: () => void;
}

const StatusCell: React.FC<CellProps> = ({ entry, onClick }) => {
  if (!entry) {
    return <span style={{ color: '#e2e8f0' }}>-</span>;
  }
  const icon = STATUS_ICON[entry.status];
  return (
    <Tooltip title={STATUS_LABEL[entry.status]}>
      <span
        style={{ cursor: 'pointer', fontSize: 16 }}
        onClick={onClick}
      >
        {icon}
      </span>
    </Tooltip>
  );
};

// ─── Platform tab content ─────────────────────────────────────────────────────

interface ActiveTask {
  trigger_id: number;
  platform: string;
  robot_name: string;
  run_date: string;
  trigger_user: string;
  started_at: string;
  elapsed_sec: number;
}

interface PlatformPanelProps {
  platform: PlatformData;
  onImport: (date: string, platform: string) => void;
  onSync: (platform: string, date: string) => void;
  onBatchSync: (platform: string, dates: string[]) => void;
  syncingDates?: string[]; // 当前正在同步的日期列表 (按钮 loading + disable)
}

const PlatformPanel: React.FC<PlatformPanelProps> = ({ platform, onImport, onSync, onBatchSync, syncingDates }) => {
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [cellDetail, setCellDetail] = useState<CellDetail | null>(null);
  const [dateFilter, setDateFilter] = useState<'all' | 'issues'>('all');
  const [selectedDates, setSelectedDates] = useState<string[]>([]);

  const openDetail = (date: string, store: string) => {
    const entry = platform.grid[date]?.[store];
    if (!entry) return;
    setCellDetail({ platform: platform.name, store, date, status: entry.status, items: entry.items });
    setDrawerOpen(true);
  };

  // 一行算"异常" = (导入状态!=已导入) 或 (任意店铺单元格!=完整)
  const isIssueDate = (date: string): boolean => {
    const meta = platform.dateMeta[date];
    const imported = meta?.dbImported;
    const fileComplete = meta?.fileStatus === 'complete';
    if (!imported || !fileComplete) return true;
    // 导入状态 OK 但是某店铺缺数据也算异常
    for (const store of platform.stores) {
      const entry = platform.grid[date]?.[store];
      if (entry && entry.status !== 'complete') return true;
    }
    return false;
  };
  const issueCount = platform.dates.filter(isIssueDate).length;

  // Build columns: first col = date, then DB status, then one col per store
  const columns = [
    {
      title: '日期',
      dataIndex: 'date',
      key: 'date',
      fixed: 'left' as const,
      width: 110,
      render: (v: string) => <Text style={{ fontSize: 12 }}>{v}</Text>,
    },
    {
      title: '导入状态',
      key: 'dbImported',
      fixed: 'left' as const,
      width: 110,
      align: 'center' as const,
      render: (_: any, row: { date: string }) => {
        const meta = platform.dateMeta[row.date];
        const imported = meta?.dbImported;
        const fileComplete = meta?.fileStatus === 'complete';
        if (imported && fileComplete) return <Tag color="success" style={{ fontSize: 11, margin: 0 }}>已导入</Tag>;
        if (imported && !fileComplete) return (
          <Space size={4}>
            <Tag color="warning" style={{ fontSize: 11, margin: 0 }}>需更新</Tag>
            <Button type="link" size="small" onClick={() => onImport(row.date, platform.name)} style={{ fontSize: 11, padding: 0 }}>导入</Button>
          </Space>
        );
        return (
          <Space size={4}>
            <Tag color="error" style={{ fontSize: 11, margin: 0 }}>未导入</Tag>
            <Button type="link" size="small" onClick={() => onImport(row.date, platform.name)} style={{ fontSize: 11, padding: 0 }}>导入</Button>
          </Space>
        );
      },
    },
    {
      title: '同步影刀',
      key: 'sync',
      fixed: 'left' as const,
      width: 100,
      align: 'center' as const,
      render: (_: any, row: { date: string }) => {
        const syncing = syncingDates?.includes(row.date) ?? false;
        return (
          <Tooltip title={syncing ? `${platform.name} ${row.date} 已在同步中` : `触发影刀采集 ${platform.name} ${row.date} 数据`}>
            <Button
              size="small"
              type="link"
              icon={<SyncOutlined spin={syncing} />}
              loading={syncing}
              disabled={syncing}
              onClick={() => onSync(platform.name, row.date)}
              style={{ fontSize: 11, padding: 0 }}
            >
              {syncing ? '同步中' : '同步'}
            </Button>
          </Tooltip>
        );
      },
    },
    ...platform.stores.map(store => ({
      title: (
        <Tooltip title={store}>
          <span style={{ fontSize: 11 }}>{store}</span>
        </Tooltip>
      ),
      key: store,
      width: 100,
      align: 'center' as const,
      render: (_: any, row: { date: string }) => (
        <StatusCell
          entry={platform.grid[row.date]?.[store]}
          onClick={() => openDetail(row.date, store)}
        />
      ),
    })),
  ];

  const dataSource = platform.dates
    .filter(date => dateFilter === 'all' || isIssueDate(date))
    .map(date => ({ key: date, date }));

  // Compute per-store completeness for summary row
  const storeCompleteness = platform.stores.map(store => {
    const entries = platform.dates.map(d => platform.grid[d]?.[store]).filter(Boolean) as DateStoreEntry[];
    if (entries.length === 0) return null;
    const complete = entries.filter(e => e.status === 'complete').length;
    return Math.round((complete / entries.length) * 100);
  });

  return (
    <div>
      {/* Overall completeness */}
      <div style={{ marginBottom: 16, display: 'flex', alignItems: 'center', gap: 16 }}>
        <Text strong>平台完整率：</Text>
        <Progress
          percent={platform.completeness}
          size="small"
          style={{ width: 220 }}
          strokeColor={platform.completeness >= 80 ? '#10b981' : platform.completeness >= 50 ? '#f59e0b' : '#ef4444'}
        />
        <Text type="secondary" style={{ fontSize: 12 }}>
          （各店铺近30天数据。点表格里"同步影刀"按钮可补采指定日期的数据）
        </Text>
      </div>

      {/* Per-store completeness badges + 状态筛选 */}
      <div style={{ marginBottom: 12, display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
        <Space wrap style={{ flex: 1 }}>
          {platform.stores.map((store, idx) => {
            const pct = storeCompleteness[idx];
            if (pct === null) return null;
            const color = pct >= 80 ? '#10b981' : pct >= 50 ? '#f59e0b' : '#ef4444';
            return (
              <div key={store} style={{ display: 'flex', alignItems: 'center', gap: 4, background: '#f8fafc', border: '1px solid #e2e8f0', borderRadius: 6, padding: '3px 8px' }}>
                <span style={{ fontSize: 12, color: '#475569', maxWidth: 120, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{store}</span>
                <span style={{ fontSize: 12, fontWeight: 600, color }}>{pct}%</span>
              </div>
            );
          })}
        </Space>
        <Radio.Group
          value={dateFilter}
          onChange={e => setDateFilter(e.target.value)}
          size="small"
          optionType="button"
          buttonStyle="solid"
        >
          <Radio.Button value="all">全部 ({platform.dates.length})</Radio.Button>
          <Radio.Button value="issues">只看异常 ({issueCount})</Radio.Button>
        </Radio.Group>
        {selectedDates.length > 0 && (
          <Space>
            <Button
              type="primary"
              size="small"
              icon={<SyncOutlined />}
              onClick={() => {
                onBatchSync(platform.name, selectedDates);
                setSelectedDates([]);
              }}
            >
              批量同步 ({selectedDates.length})
            </Button>
            <Button size="small" onClick={() => setSelectedDates([])}>取消选择</Button>
          </Space>
        )}
      </div>

      {/* Date grid table */}
      <Table
        columns={columns}
        dataSource={dataSource}
        rowKey="date"
        size="small"
        pagination={false}
        scroll={{ x: Math.max(500, 220 + platform.stores.length * 100), y: 520 }}
        virtual
        bordered
        rowSelection={{
          selectedRowKeys: selectedDates,
          onChange: keys => setSelectedDates(keys as string[]),
          getCheckboxProps: (row: any) => ({
            disabled: syncingDates?.includes(row.date) ?? false,
          }),
          columnWidth: 40,
          fixed: true,
        }}
      />

      {/* Detail drawer */}
      <Drawer
        title={
          <Space>
            {cellDetail && STATUS_ICON[cellDetail.status]}
            <span>{cellDetail ? `${cellDetail.store} · ${cellDetail.date}` : ''}</span>
          </Space>
        }
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        size={360}
        destroyOnHidden
      >
        {cellDetail && (
          <div>
            <div style={{ marginBottom: 12 }}>
              <Text type="secondary" style={{ fontSize: 12 }}>平台：{cellDetail.platform}</Text>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {cellDetail.items.length === 0 ? (
                <Text type="secondary">无数据项信息</Text>
              ) : (
                cellDetail.items.map(item => (
                  <div
                    key={item.name}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 8,
                      padding: '6px 10px',
                      borderRadius: 6,
                      background: item.status === 'complete' ? '#f0fdf4' : item.status === 'partial' ? '#fffbeb' : item.status === 'missing' ? '#fef2f2' : '#f8fafc',
                      border: `1px solid ${STATUS_COLOR[item.status]}33`,
                    }}
                  >
                    {STATUS_ICON[item.status]}
                    <Text style={{ flex: 1, fontSize: 13 }}>{item.name}</Text>
                    <Tag color={item.status === 'complete' ? 'success' : item.status === 'partial' ? 'warning' : item.status === 'missing' ? 'error' : 'default'}>
                      {STATUS_LABEL[item.status]}
                    </Tag>
                  </div>
                ))
              )}
            </div>
          </div>
        )}
      </Drawer>
    </div>
  );
};

// ─── Legend ───────────────────────────────────────────────────────────────────

const Legend: React.FC = () => (
  <Space size={16} style={{ fontSize: 12, color: '#64748b' }}>
    {(['complete', 'partial', 'missing', 'no_dir'] as ItemStatus[]).map(s => (
      <Space key={s} size={4}>
        {STATUS_ICON[s]}
        <span>{STATUS_LABEL[s]}</span>
      </Space>
    ))}
  </Space>
);

// ─── Transform backend response into PlatformData ────────────────────────────

function transformData(raw: any): ScanResult {
  const platforms: PlatformData[] = (raw.platforms || []).map((p: any) => {
    const storeSet = new Set<string>();
    const dates: string[] = [];
    const grid: Record<string, Record<string, DateStoreEntry>> = {};
    const dateMeta: Record<string, DateMeta> = {};

    for (const d of (p.dates || [])) {
      const dateKey = d.formatted_date || d.date;
      dates.push(dateKey);
      grid[dateKey] = {};
      dateMeta[dateKey] = { dbImported: !!d.db_imported, fileStatus: (d.status || 'missing') as ItemStatus };
      for (const s of (d.stores || [])) {
        storeSet.add(s.name);
        const items: DataItem[] = [
          ...(s.completed_items || []).map((name: string) => ({ name, status: 'complete' as ItemStatus })),
          ...(s.missing_items || []).map((name: string) => ({ name, status: 'missing' as ItemStatus })),
        ];
        grid[dateKey][s.name] = {
          status: s.status as ItemStatus,
          items,
        };
      }
    }

    return {
      name: p.name,
      stores: Array.from(storeSet),
      dates,
      grid,
      dateMeta,
      completeness: Math.round((p.completeness || 0) * 100),
    };
  });

  return {
    scannedAt: raw.scanned_at || '',
    scanning: raw.scanning || false,
    platforms,
  };
}

// ─── Main page ────────────────────────────────────────────────────────────────

const RPAMonitor: React.FC = () => {
  const [data, setData] = useState<ScanResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/admin/rpa-scan`, { credentials: 'include' });
      const json = await res.json();
      const raw = json.data ?? json;
      setData(transformData(raw));
    } catch {
    } finally {
      setLoading(false);
    }
  }, []);

  const handleRefresh = useCallback(async () => {
    setRefreshing(true);
    try {
      await fetch(`${API_BASE}/api/admin/rpa-scan/refresh`, { method: 'POST', credentials: 'include' });
      // Poll until scanning is false
      let attempts = 0;
      const poll = async () => {
        attempts++;
        const res = await fetch(`${API_BASE}/api/admin/rpa-scan`, { credentials: 'include' });
        const json = await res.json();
        const result: ScanResult = transformData(json.data ?? json);
        setData(result);
        if (result.scanning && attempts < 60) {
          setTimeout(poll, 2000);
        } else {
          setRefreshing(false);
        }
      };
      await poll();
    } catch {
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    fetchData();
    // Auto-refresh every 5 minutes
    timerRef.current = setInterval(fetchData, 5 * 60 * 1000);
    return () => {
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, [fetchData]);

  const platforms = data?.platforms ?? [];
  const [filterStatus, setFilterStatus] = useState<'all' | 'issues' | 'missing' | 'no_dir'>('all');
  const [keyword, setKeyword] = useState('');
  const [keywordInput, setKeywordInput] = useState('');
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [importModalOpen, setImportModalOpen] = useState(false);
  const [importProg, setImportProg] = useState<any>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const importCtrlRef = useRef<AbortController | null>(null);

  const startImport = async (dateStr: string, platformName: string) => {
    const d = dateStr.replace(/-/g, '');
    setImportProg({ running: true, date: d, platform: platformName, total: 0, current: 0, results: [] });
    setImportModalOpen(true);
    importCtrlRef.current?.abort();
    importCtrlRef.current = new AbortController();
    const signal = importCtrlRef.current.signal;
    try {
      const res = await fetch(`${API_BASE}/api/admin/rpa-scan/import`, {
        method: 'POST', credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ date: d, platform: platformName }),
        signal,
      });
      const json = await res.json();
      // 后端错误统一返回 {code, msg}，非 200 视为失败
      if (!res.ok || (json.code && json.code !== 200)) {
        message.error(json.msg || json.error || '导入请求失败');
        setImportModalOpen(false);
        return;
      }
      pollRef.current = setInterval(async () => {
        try {
          const pr = await fetch(`${API_BASE}/api/admin/rpa-scan/import-progress`, { credentials: 'include', signal });
          const json = await pr.json();
          // 后端统一包 {code, data}，这里要取 .data 才是真实 progress
          const prog = json.data || json;
          // 后端若返回只含 running 的空对象（服务重启等场景），合并进当前 state 保留 total/platform/results
          setImportProg((prev: any) => ({ ...(prev || {}), ...prog }));
          if (!prog.running) {
            if (pollRef.current) clearInterval(pollRef.current);
            pollRef.current = null;
            message.success('导入完成');
            fetchData();
            // 停止后让用户看 1.5 秒结果再自动关闭，避免弹窗永久卡在界面上
            setTimeout(() => setImportModalOpen(false), 1500);
          }
        } catch (err) {
          if ((err as Error)?.name === 'AbortError' && pollRef.current) {
            clearInterval(pollRef.current);
            pollRef.current = null;
          }
        }
      }, 1000);
    } catch (err) {
      if ((err as Error)?.name !== 'AbortError') message.error('导入请求失败');
      setImportModalOpen(false);
    }
  };

  useEffect(() => {
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
      importCtrlRef.current?.abort();
    };
  }, []);

  // === 影刀 RPA 同步触发 ===
  const [activeSync, setActiveSync] = useState<{ triggerId: number; platform: string; date: string; robotName: string } | null>(null);
  const [syncModalOpen, setSyncModalOpen] = useState(false);
  const [activeTasks, setActiveTasks] = useState<ActiveTask[]>([]);
  const [taskDrawerOpen, setTaskDrawerOpen] = useState(false);
  const activeTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const fetchActiveTasks = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/api/admin/rpa/active-tasks`, { credentials: 'include' });
      const j = await res.json();
      if (j.code === 200) setActiveTasks(j.data || []);
    } catch {}
  }, []);

  // 5s 一次轮询活跃任务列表
  useEffect(() => {
    fetchActiveTasks();
    activeTimerRef.current = setInterval(fetchActiveTasks, 5000);
    return () => {
      if (activeTimerRef.current) clearInterval(activeTimerRef.current);
    };
  }, [fetchActiveTasks]);

  const handleSync = useCallback(async (platform: string, date: string) => {
    try {
      const res = await fetch(`${API_BASE}/api/admin/rpa/trigger`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ platform, date }),
      });
      const j = await res.json();
      if (j.code !== 200) {
        message.error(j.msg || j.error || `触发失败：${platform} ${date}`);
        return;
      }
      setActiveSync({
        triggerId: j.data.trigger_id,
        platform,
        date,
        robotName: j.data.robot_name || '',
      });
      setSyncModalOpen(true);
      message.success(`已触发影刀采集 - ${platform} ${date}`);
    } catch {
      message.error('触发同步请求失败（网络错误）');
    }
  }, []);

  const handleSyncClose = useCallback(() => {
    setSyncModalOpen(false);
    setActiveSync(null);
  }, []);

  const handleSyncDone = useCallback(() => {
    // 单次同步完成只刷新活跃任务列表 (轻量), 不全量重拉 RPA 扫描数据
    // 原因: 全量数据 700KB+ 1000+ 行渲染慢, 跑哥批量同步时多个 done 叠加更卡
    // 跑哥要看最新 RPA 文件状态主动点 "刷新" 按钮 (或等 5 分钟自动刷)
    fetchActiveTasks();
  }, [fetchActiveTasks]);

  // 批量同步: 调后端 batch-trigger 接口入队, 立即返回. 后端 goroutine 串行执行,
  // 跑哥可关浏览器, 不影响. 进度看右下角浮窗.
  const handleBatchSync = useCallback(async (platform: string, dates: string[]) => {
    if (dates.length === 0) return;
    try {
      const res = await fetch(`${API_BASE}/api/admin/rpa/batch-trigger`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ platform, dates }),
      });
      const j = await res.json();
      if (j.code === 200 && j.data?.batch_id) {
        message.success(j.data.message || `已加入后台队列 ${dates.length} 个日期`);
      } else {
        message.error(j.msg || '批量入队失败');
      }
    } catch {
      message.error('批量入队请求失败 (网络错误)');
    }
    fetchActiveTasks();
  }, [fetchActiveTasks]);

  // 从 Drawer 点某个 active task → 重新打开 Modal 看进度
  const handleResumeTask = useCallback((task: ActiveTask) => {
    setActiveSync({
      triggerId: task.trigger_id,
      platform: task.platform,
      date: task.run_date,
      robotName: task.robot_name,
    });
    setSyncModalOpen(true);
    setTaskDrawerOpen(false);
  }, []);

  // 问题汇总：把所有平台/日期/店铺/缺失文件展平
  const issueRows = useMemo(() => {
    const rows: { platform: string; date: string; store: string; status: ItemStatus; missingItems: string[]; dbImported: boolean; fileStatus: ItemStatus }[] = [];
    for (const p of platforms) {
      for (const date of p.dates) {
        const meta = p.dateMeta[date];
        const imported = meta?.dbImported ?? false;
        const fileStatus = meta?.fileStatus ?? 'missing';
        for (const store of p.stores) {
          const entry = p.grid[date]?.[store];
          if (!entry) continue;
          if (entry.status === 'complete' && imported && fileStatus === 'complete') continue;
          const missingItems = entry.items.filter(i => i.status !== 'complete').map(i => i.name);
          rows.push({ platform: p.name, date, store, status: entry.status, missingItems, dbImported: imported, fileStatus });
        }
      }
    }
    return rows.sort((a, b) => b.date.localeCompare(a.date));
  }, [platforms]);

  const filteredIssues = useMemo(() => {
    return issueRows.filter(r => {
      if (filterStatus === 'missing' && r.status !== 'missing') return false;
      if (filterStatus === 'no_dir' && r.status !== 'no_dir') return false;
      if (filterStatus === 'issues' && r.status === 'complete') return false;
      if (keyword) {
        const kw = keyword.toLowerCase();
        return r.platform.includes(kw) || r.date.includes(kw) || r.store.toLowerCase().includes(kw) ||
          r.missingItems.some(m => m.toLowerCase().includes(kw));
      }
      return true;
    });
  }, [issueRows, filterStatus, keyword]);

  const issueCols = [
    { title: '平台', dataIndex: 'platform', key: 'platform', width: 100, render: (v: string) => <Tag color="blue">{v}</Tag> },
    { title: '日期', dataIndex: 'date', key: 'date', width: 110 },
    { title: '店铺', dataIndex: 'store', key: 'store', width: 200, ellipsis: true },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 90,
      render: (v: ItemStatus) => (
        <Space size={4}>{STATUS_ICON[v]}<span style={{ fontSize: 12 }}>{STATUS_LABEL[v]}</span></Space>
      ),
    },
    {
      title: '导入状态', key: 'dbImported', width: 120,
      render: (_: any, row: any) => {
        const imported = row.dbImported;
        const fileComplete = row.fileStatus === 'complete';
        if (imported && fileComplete) return <Tag color="success" style={{ fontSize: 11 }}>已导入</Tag>;
        if (imported && !fileComplete) return (
          <Space size={4}>
            <Tag color="warning" style={{ fontSize: 11, margin: 0 }}>需更新</Tag>
            <Button type="link" size="small" onClick={() => startImport(row.date, row.platform)} style={{ fontSize: 11, padding: 0 }}>导入</Button>
          </Space>
        );
        return (
          <Space size={4}>
            <Tag color="error" style={{ fontSize: 11, margin: 0 }}>未导入</Tag>
            <Button type="link" size="small" onClick={() => startImport(row.date, row.platform)} style={{ fontSize: 11, padding: 0 }}>导入</Button>
          </Space>
        );
      },
    },
    {
      title: '缺失文件', key: 'missingItems',
      render: (_: any, row: any) => {
        if (row.missingItems.length === 0 && row.status === 'complete') return <span style={{ color: '#10b981', fontSize: 12 }}>文件完整</span>;
        if (row.missingItems.length === 0) return <span style={{ color: '#94a3b8', fontSize: 12 }}>无文件信息</span>;
        return <span style={{ fontSize: 12, color: '#ef4444' }}>{row.missingItems.join('、')}</span>;
      },
    },
  ];

  const tabItems = [
    {
      key: '__issues__',
      label: (
        <Space size={4}>
          <WarningOutlined style={{ color: '#f59e0b' }} />
          <span>问题汇总</span>
          {issueRows.length > 0 && <Tag color="error" style={{ fontSize: 11, padding: '0 4px' }}>{issueRows.length}</Tag>}
        </Space>
      ),
      children: (
        <div>
          <Space style={{ marginBottom: 12 }} wrap>
            <Radio.Group value={filterStatus} onChange={e => setFilterStatus(e.target.value)} size="small">
              <Radio.Button value="all">全部问题 ({issueRows.length})</Radio.Button>
              <Radio.Button value="missing">缺失 ({issueRows.filter(r => r.status === 'missing').length})</Radio.Button>
              <Radio.Button value="no_dir">无目录 ({issueRows.filter(r => r.status === 'no_dir').length})</Radio.Button>
            </Radio.Group>
            <Input
              placeholder="搜索平台/日期/店铺/文件"
              prefix={<SearchOutlined />}
              allowClear
              size="small"
              style={{ width: 220 }}
              value={keywordInput}
              onChange={e => {
                const v = e.target.value;
                setKeywordInput(v);
                if (debounceRef.current) clearTimeout(debounceRef.current);
                debounceRef.current = setTimeout(() => setKeyword(v), 300);
              }}
              onPressEnter={() => {
                if (debounceRef.current) clearTimeout(debounceRef.current);
                setKeyword(keywordInput);
              }}
            />
          </Space>
          <Table
            dataSource={filteredIssues}
            columns={issueCols}
            rowKey={(r: any) => `${r.platform}-${r.date}-${r.store}-${r.status}-${r.fileStatus}`}
            size="small"
            pagination={false}
            scroll={{ x: 900, y: 500 }}
            virtual
          />
        </div>
      ),
    },
    ...platforms.map(p => ({
    key: p.name,
    label: (
      <Space size={4}>
        <span>{p.name}</span>
        <Tag
          color={p.completeness >= 80 ? 'success' : p.completeness >= 50 ? 'warning' : 'error'}
          style={{ fontSize: 11, lineHeight: '16px', padding: '0 4px' }}
        >
          {p.completeness}%
        </Tag>
      </Space>
    ),
    children: (
      <PlatformPanel
        platform={p}
        onImport={startImport}
        onSync={handleSync}
        onBatchSync={handleBatchSync}
        syncingDates={activeTasks.filter(t => t.platform === p.name).map(t => t.run_date)}
      />
    ),
  })),
];

  return (
    <div>
      <Card
        title={
          <Space size={16}>
            <span style={{ fontWeight: 600, fontSize: 16 }}>RPA数据采集监控</span>
            {(loading || refreshing) && (
              <Space size={6}>
                <Spin size="small" />
                <span style={{ fontSize: 13, color: '#64748b' }}>
                  {refreshing ? '正在扫描...' : '加载中...'}
                </span>
              </Space>
            )}
            {data?.scannedAt && !loading && !refreshing && (
              <span style={{ fontSize: 12, color: '#94a3b8', fontWeight: 400 }}>
                最后扫描时间：{data.scannedAt}
              </span>
            )}
            {activeTasks.length > 0 && (
              <Tooltip title="点击查看后台正在跑的同步任务">
                <Badge
                  count={activeTasks.length}
                  overflowCount={999}
                  size="small"
                  style={{ marginRight: 12 }}
                >
                  <Button
                    size="small"
                    type="primary"
                    icon={<SyncOutlined spin />}
                    onClick={() => setTaskDrawerOpen(true)}
                  >
                    正在同步
                  </Button>
                </Badge>
              </Tooltip>
            )}
          </Space>
        }
        extra={
          <Button
            type="primary"
            icon={<ReloadOutlined spin={refreshing} />}
            loading={refreshing}
            onClick={handleRefresh}
            disabled={loading || refreshing}
          >
            刷新
          </Button>
        }
        styles={{ body: { paddingTop: 12 } }}
      >
        {/* Legend */}
        <div style={{ marginBottom: 16 }}>
          <Legend />
        </div>

        {/* Platform tabs */}
        {loading && !data ? (
          <div style={{ textAlign: 'center', padding: 60 }}>
            <Spin size="large" />
          </div>
        ) : platforms.length === 0 ? (
          <div style={{ textAlign: 'center', padding: 60, color: '#94a3b8' }}>
            暂无数据，请点击「刷新」开始扫描
          </div>
        ) : (
          <Tabs
            type="card"
            size="small"
            items={tabItems}
            style={{ marginTop: 4 }}
            destroyOnHidden
          />
        )}
      </Card>

      <style>{`
        .ant-tabs-card > .ant-tabs-nav .ant-tabs-tab {
          padding: 4px 12px;
        }
      `}</style>

      {/* 导入进度弹窗 */}
      <Modal
        title={importProg ? `导入 ${importProg.platform || ''} ${importProg.date ? `${importProg.date.slice(0,4)}-${importProg.date.slice(4,6)}-${importProg.date.slice(6)}` : ''}` : '导入进度'}
        open={importModalOpen}
        footer={importProg?.running ? null : <Button type="primary" onClick={() => setImportModalOpen(false)}>关闭</Button>}
        closable={!importProg?.running}
        onCancel={() => { if (!importProg?.running) setImportModalOpen(false); }}
        width={480}
      >
        {importProg && (
          <div>
            <Progress
              percent={(importProg.total || 0) > 0 ? Math.round(((importProg.current || 0) / importProg.total) * 100) : 0}
              status={importProg.running ? 'active' : 'success'}
              style={{ marginBottom: 16 }}
            />
            <div style={{ fontSize: 13, color: '#64748b', marginBottom: 12 }}>
              {importProg.running
                ? `正在执行: ${importProg.current_tool || ''} (${importProg.current || 0}/${importProg.total || 0})`
                : `全部完成 (${importProg.total || 0}个工具)`
              }
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              {(importProg.results || []).map((r: any, i: number) => (
                <div key={i} style={{
                  display: 'flex', alignItems: 'center', gap: 8, padding: '4px 8px', borderRadius: 4,
                  background: r.status === '成功' ? '#f0fdf4' : r.status === 'running' ? '#eff6ff' : r.status === 'pending' ? '#f8fafc' : '#fef2f2',
                }}>
                  {r.status === '成功' && <CheckCircleOutlined style={{ color: '#10b981' }} />}
                  {r.status === 'running' && <Spin size="small" />}
                  {r.status === 'pending' && <MinusCircleOutlined style={{ color: '#94a3b8' }} />}
                  {(r.status === '失败' || r.status === '超时') && <CloseCircleOutlined style={{ color: '#ef4444' }} />}
                  <span style={{ flex: 1, fontSize: 12 }}>{r.tool}</span>
                  <span style={{ fontSize: 11, color: '#64748b' }}>{r.detail || r.status}</span>
                </div>
              ))}
            </div>
          </div>
        )}
      </Modal>

      {/* 影刀 RPA 同步进度 Modal */}
      <RPASyncModal
        triggerId={activeSync?.triggerId ?? null}
        platform={activeSync?.platform ?? ''}
        robotName={activeSync?.robotName ?? ''}
        date={activeSync?.date}
        open={syncModalOpen}
        onClose={handleSyncClose}
        onDone={handleSyncDone}
      />

      {/* 后台正在跑的任务列表 Drawer */}
      <Drawer
        title={
          <Space>
            <SyncOutlined spin />
            <span>正在跑的同步任务</span>
            <Tag color="blue">{activeTasks.length} 个</Tag>
          </Space>
        }
        open={taskDrawerOpen}
        onClose={() => setTaskDrawerOpen(false)}
        width={520}
      >
        {activeTasks.length === 0 ? (
          <Empty description="当前没有正在跑的任务" />
        ) : (
          <List
            dataSource={activeTasks}
            renderItem={t => (
              <List.Item
                actions={[
                  <Button key="view" type="primary" size="small" onClick={() => handleResumeTask(t)}>
                    查看进度
                  </Button>,
                ]}
              >
                <List.Item.Meta
                  title={
                    <Space>
                      <Tag color="blue">{t.platform}</Tag>
                      <span>{t.run_date}</span>
                    </Space>
                  }
                  description={
                    <Space size={12} wrap>
                      <Text type="secondary" style={{ fontSize: 12 }}>{t.robot_name}</Text>
                      <Text type="secondary" style={{ fontSize: 12 }}>由 {t.trigger_user} 触发</Text>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        已用时 {Math.floor(t.elapsed_sec / 60)} 分 {t.elapsed_sec % 60} 秒
                      </Text>
                    </Space>
                  }
                />
              </List.Item>
            )}
          />
        )}
        <div style={{ marginTop: 16, fontSize: 12, color: '#94a3b8' }}>
          💡 同步任务在后台跑，跑完通过钉钉通知。点"查看进度"可以重新打开 Modal 看实时日志。
        </div>
      </Drawer>
    </div>
  );
};

export default RPAMonitor;
