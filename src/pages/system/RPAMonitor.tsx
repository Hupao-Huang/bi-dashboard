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
} from 'antd';
import { ReloadOutlined, CheckCircleOutlined, WarningOutlined, CloseCircleOutlined, MinusCircleOutlined, SearchOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';

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

interface PlatformData {
  name: string;
  stores: string[];
  dates: string[];
  /** grid[date][store] = DateStoreEntry */
  grid: Record<string, Record<string, DateStoreEntry>>;
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

interface PlatformPanelProps {
  platform: PlatformData;
}

const PlatformPanel: React.FC<PlatformPanelProps> = ({ platform }) => {
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [cellDetail, setCellDetail] = useState<CellDetail | null>(null);

  const openDetail = (date: string, store: string) => {
    const entry = platform.grid[date]?.[store];
    if (!entry) return;
    setCellDetail({ platform: platform.name, store, date, status: entry.status, items: entry.items });
    setDrawerOpen(true);
  };

  // Build columns: first col = date, then one col per store
  const columns = [
    {
      title: '日期',
      dataIndex: 'date',
      key: 'date',
      fixed: 'left' as const,
      width: 110,
      render: (v: string) => <Text style={{ fontSize: 12 }}>{v}</Text>,
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

  const dataSource = platform.dates.map(date => ({ key: date, date }));

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
        <Text type="secondary" style={{ fontSize: 12 }}>（各店铺近30天数据）</Text>
      </div>

      {/* Per-store completeness badges */}
      <Space wrap style={{ marginBottom: 12 }}>
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

      {/* Date grid table */}
      <Table
        columns={columns}
        dataSource={dataSource}
        rowKey="date"
        size="small"
        pagination={false}
        scroll={{ x: Math.max(500, 110 + platform.stores.length * 100), y: 520 }}
        bordered
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
        width={360}
        destroyOnClose
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

    for (const d of (p.dates || [])) {
      const dateKey = d.formatted_date || d.date;
      dates.push(dateKey);
      grid[dateKey] = {};
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

  // 问题汇总：把所有平台/日期/店铺/缺失文件展平
  const issueRows = useMemo(() => {
    const rows: { platform: string; date: string; store: string; status: ItemStatus; missingItems: string[] }[] = [];
    for (const p of platforms) {
      for (const date of p.dates) {
        for (const store of p.stores) {
          const entry = p.grid[date]?.[store];
          if (!entry) continue;
          if (entry.status === 'complete') continue;
          const missingItems = entry.items.filter(i => i.status !== 'complete').map(i => i.name);
          rows.push({ platform: p.name, date, store, status: entry.status, missingItems });
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
      title: '缺失文件', dataIndex: 'missingItems', key: 'missingItems',
      render: (items: string[]) => items.length === 0
        ? <span style={{ color: '#94a3b8', fontSize: 12 }}>无目录</span>
        : <span style={{ fontSize: 12, color: '#ef4444' }}>{items.join('、')}</span>,
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
            rowKey={(r, i) => `${r.platform}-${r.date}-${r.store}-${i}`}
            size="small"
            pagination={false}
            scroll={{ y: 500 }}
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
    children: <PlatformPanel platform={p} />,
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
        bodyStyle={{ paddingTop: 12 }}
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
          />
        )}
      </Card>

      <style>{`
        .ant-tabs-card > .ant-tabs-nav .ant-tabs-tab {
          padding: 4px 12px;
        }
      `}</style>
    </div>
  );
};

export default RPAMonitor;
