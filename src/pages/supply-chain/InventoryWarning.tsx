import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Row, Col, Card, Table, Select, Input, Tag, Button, message, Tooltip } from 'antd';
import {
  StopOutlined,
  AlertOutlined,
  ExclamationCircleOutlined,
  InboxOutlined,
  PauseCircleOutlined,
  InfoCircleOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import PageLoading from '../../components/PageLoading';
import { API_BASE } from '../../config';

const { Search } = Input;

interface StockChild {
  warehouse: string;
  usableQty: number;
  sellableDays: number;
  dailyAvg: number;
  monthQty: number;
}

interface StockItem {
  goodsNo: string;
  goodsName: string;
  category?: string;
  position?: string;
  warehouse?: string;
  usableQty: number;
  sellableDays: number;
  dailyAvg: number;
  monthQty: number;
  currentQty: number;
  whCount?: number;
  whStockout?: number;
  warehouses?: StockChild[];
}

interface Summary {
  total: number;
  stockout: number;
  urgent: number;
  low: number;
  overstock: number;
  dead: number;
}

const warningFilters = [
  { key: 'all', label: '全部' },
  { key: 'stockout', label: '断货' },
  { key: 'urgent', label: '即将断货(<7天)' },
  { key: 'low', label: '库存偏低(7-14天)' },
  { key: 'overstock', label: '库存积压(>90天)' },
  { key: 'dead', label: '滞销(零销量)' },
];

const getWarningTag = (days: number, monthQty: number, currentQty: number) => {
  if (monthQty > 0 && days <= 0) return <Tag color="red">断货</Tag>;
  if (monthQty > 0 && days < 7) return <Tag color="orange">即将断货</Tag>;
  if (monthQty > 0 && days <= 14) return <Tag color="gold">偏低</Tag>;
  if (monthQty > 0 && days <= 30) return <Tag color="blue">正常</Tag>;
  if (monthQty > 0 && days <= 90) return <Tag color="green">充足</Tag>;
  if (monthQty > 0 && days > 90) return <Tag color="purple">积压</Tag>;
  if (monthQty === 0 && currentQty > 0) return <Tag color="default">滞销</Tag>;
  return <Tag color="default">-</Tag>;
};

const InventoryWarning: React.FC = () => {
  const abortRef = useRef<AbortController | null>(null);
  const [loading, setLoading] = useState(true);
  const [items, setItems] = useState<StockItem[]>([]);
  const [summary, setSummary] = useState<Summary>({ total: 0, stockout: 0, urgent: 0, low: 0, overstock: 0, dead: 0 });
  const [whList, setWhList] = useState<string[]>([]);
  const [filter, setFilter] = useState('all');
  const [warehouse, setWarehouse] = useState('');
  const [keyword, setKeyword] = useState('');
  const [syncing, setSyncing] = useState(false);
  const [syncInfo, setSyncInfo] = useState<{ lastFinishedAt?: string; lastElapsedSec?: number; lastError?: string; elapsedSec?: number; startedAt?: string }>({});
  const pollRef = useRef<number | null>(null);
  const tickRef = useRef<number | null>(null);
  const wasRunningRef = useRef(false);
  const [, forceTick] = useState(0); // 仅用于 syncing 时每秒触发重渲染

  const fetchData = useCallback((w: string, f: string, kw: string) => {
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    const params = new URLSearchParams();
    if (w) params.set('warehouse', w);
    if (f && f !== 'all') params.set('warning', f);
    if (kw) params.set('keyword', kw);

    fetch(`${API_BASE}/api/stock/warning?${params}`, { signal: ctrl.signal })
      .then(res => res.json())
      .then(res => {
        if (res.code === 200 && res.data) {
          setSummary(res.data.summary || {});
          setItems(res.data.items || []);
          setWhList(res.data.warehouses || []);
        }
        setLoading(false);
      })
      .catch((e: any) => { if (e?.name !== 'AbortError') setLoading(false); });
  }, []);

  useEffect(() => { fetchData(warehouse, filter, keyword); }, [fetchData, warehouse, filter, keyword]);

  const stopPolling = () => {
    if (pollRef.current) {
      window.clearInterval(pollRef.current);
      pollRef.current = null;
    }
  };

  const checkSyncStatus = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/api/stock/sync-status`, { credentials: 'include' });
      const json = await res.json();
      if (json.code !== 200) return;
      const data = json.data || {};
      setSyncInfo({
        lastFinishedAt: data.lastFinishedAt,
        lastElapsedSec: data.lastElapsedSec,
        lastError: data.lastError,
        elapsedSec: data.elapsedSec,
        startedAt: data.startedAt,
      });
      if (data.running) {
        setSyncing(true);
        wasRunningRef.current = true;
        if (!pollRef.current) {
          pollRef.current = window.setInterval(() => { checkSyncStatus(); }, 10000);
        }
      } else {
        setSyncing(false);
        if (wasRunningRef.current) {
          // 从同步中 → 已完成：自动刷新表格 + 提示
          wasRunningRef.current = false;
          stopPolling();
          if (data.lastError) {
            message.error(`同步失败：${data.lastError}`);
          } else {
            message.success(`✓ 实时库存已同步（耗时 ${data.lastElapsedSec || 0}s），表格已刷新`);
          }
          fetchData(warehouse, filter, keyword);
        }
      }
    } catch {
      // ignore，下次轮询继续
    }
  }, [fetchData, warehouse, filter, keyword]);

  // 进页面查一次状态：如果发现服务端正在跑同步，自动锁住按钮 + 启动轮询
  useEffect(() => {
    checkSyncStatus();
    return () => stopPolling();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 同步中本地 tick：每秒重渲染让"已 Xs"实时跳动（不依赖 10 秒轮询）
  useEffect(() => {
    if (!syncing) {
      if (tickRef.current) {
        window.clearInterval(tickRef.current);
        tickRef.current = null;
      }
      return;
    }
    tickRef.current = window.setInterval(() => forceTick((x) => x + 1), 1000);
    return () => {
      if (tickRef.current) {
        window.clearInterval(tickRef.current);
        tickRef.current = null;
      }
    };
  }, [syncing]);

  // 实时已耗时秒数：优先用 startedAt 算（每秒跳），fallback 后端返回的 elapsedSec
  const liveElapsedSec = (() => {
    if (syncing && syncInfo.startedAt) {
      const ms = Date.now() - new Date(syncInfo.startedAt).getTime();
      return Math.max(0, Math.floor(ms / 1000));
    }
    return syncInfo.elapsedSec || 0;
  })();

  const handleSyncNow = () => {
    setSyncing(true);
    wasRunningRef.current = true;
    message.info('已触发实时同步（约 2-3 分钟），可关闭页面，后台继续');
    // fire-and-forget：触发后立即启动轮询，不等待 fetch 返回
    fetch(`${API_BASE}/api/stock/sync-now`, { method: 'POST', credentials: 'include' })
      .then((r) => r.json())
      .then((json) => {
        if (json.code === 409) {
          // 已有任务在跑（可能是定时任务）—— 仍然继续轮询
          message.warning(json.msg || '已有同步任务在跑，正在跟踪进度');
        } else if (json.code !== 200) {
          message.error(json.msg || '触发失败');
          setSyncing(false);
          wasRunningRef.current = false;
          stopPolling();
        }
        // code === 200：sync 完成，下一次轮询会拿到 running=false 触发刷新
      })
      .catch(() => {
        // 网络中断不要紧，依赖轮询拿状态
      });
    // 立即启动轮询（更快感知完成）
    stopPolling();
    pollRef.current = window.setInterval(() => { checkSyncStatus(); }, 10000);
    // 1 秒后再查一次（短延迟，让按钮立刻进入"同步中"状态显示已经几秒）
    window.setTimeout(() => { checkSyncStatus(); }, 1000);
  };

  // 格式化"上次同步：X 分钟前"
  const formatSyncAgo = (iso?: string) => {
    if (!iso) return '';
    const t = new Date(iso).getTime();
    const ago = Math.max(0, Math.floor((Date.now() - t) / 1000));
    if (ago < 60) return `${ago} 秒前`;
    if (ago < 3600) return `${Math.floor(ago / 60)} 分钟前`;
    if (ago < 86400) return `${Math.floor(ago / 3600)} 小时前`;
    return `${Math.floor(ago / 86400)} 天前`;
  };

  const isAggMode = !warehouse; // 未选仓库时按商品聚合

  const statCards = [
    { key: 'stockout', title: '断货', value: summary.stockout, color: '#ef4444', icon: <StopOutlined />, bg: '#fef2f2', desc: '可用库存≤0 且有销量' },
    { key: 'urgent', title: '即将断货', value: summary.urgent, color: '#ea580c', icon: <AlertOutlined />, bg: '#fff7ed', desc: '可售天数 < 7天' },
    { key: 'low', title: '库存偏低', value: summary.low, color: '#f59e0b', icon: <ExclamationCircleOutlined />, bg: '#fffbeb', desc: '可售天数 7~14天' },
    { key: 'overstock', title: '库存积压', value: summary.overstock, color: '#7c3aed', icon: <InboxOutlined />, bg: '#f5f3ff', desc: '可售天数 > 90天' },
    { key: 'dead', title: '滞销品', value: summary.dead, color: '#94a3b8', icon: <PauseCircleOutlined />, bg: '#f8fafc', desc: '30天零销量 但有库存' },
  ];

  // 聚合模式：展开行显示仓库明细
  const expandedRowRender = (record: StockItem) => {
    if (!record.warehouses || record.warehouses.length === 0) return null;
    const childCols = [
      { title: '仓库', dataIndex: 'warehouse', key: 'warehouse', width: 240 },
      {
        title: '可用库存', dataIndex: 'usableQty', key: 'usableQty', width: 100, align: 'right' as const,
        render: (v: number) => {
          const color = v <= 0 ? '#ef4444' : v < 100 ? '#f59e0b' : '#1e293b';
          return <span style={{ fontWeight: 600, color, fontVariantNumeric: 'tabular-nums' }}>{v.toLocaleString()}</span>;
        },
      },
      {
        title: '可售天数', dataIndex: 'sellableDays', key: 'sellableDays', width: 100, align: 'right' as const,
        render: (v: number, r: StockChild) => {
          if (r.monthQty === 0) return <span style={{ color: '#94a3b8' }}>-</span>;
          if (v <= 0) return <span style={{ color: '#ef4444', fontWeight: 700 }}>0</span>;
          const color = v < 7 ? '#ef4444' : v < 14 ? '#f59e0b' : v > 90 ? '#7c3aed' : '#1e293b';
          return <span style={{ fontWeight: 600, color, fontVariantNumeric: 'tabular-nums' }}>{v}</span>;
        },
      },
      {
        title: '日均销量', dataIndex: 'dailyAvg', key: 'dailyAvg', width: 90, align: 'right' as const,
        render: (v: number) => v > 0 ? v : '-',
      },
      {
        title: '近30天', dataIndex: 'monthQty', key: 'monthQty', width: 90, align: 'right' as const,
        render: (v: number) => v > 0 ? v.toLocaleString() : '-',
      },
    ];
    return (
      <Table
        dataSource={record.warehouses}
        columns={childCols}
        rowKey="warehouse"
        pagination={false}
        size="small"
        style={{ margin: '-4px 0' }}
      />
    );
  };

  // 主表列定义
  const columns: any[] = [
    {
      title: '预警', key: 'warning', width: 80, fixed: 'left' as const,
      render: (_: any, r: StockItem) => getWarningTag(r.sellableDays, r.monthQty, r.currentQty || r.usableQty),
    },
    { title: '商品编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 120 },
    { title: '商品名称', dataIndex: 'goodsName', key: 'goodsName', width: 240, ellipsis: true },
    {
      title: '商品分类', dataIndex: 'category', key: 'category', width: 130, ellipsis: true,
      filters: Array.from(new Set(items.map((i) => i.category).filter(Boolean))).map((c) => ({ text: c as string, value: c as string })),
      onFilter: (val: any, r: StockItem) => r.category === val,
      render: (v?: string) => v ? <span style={{ fontSize: 12 }}>{v}</span> : <span style={{ color: '#cbd5e1' }}>-</span>,
    },
    {
      title: '产品定位', dataIndex: 'position', key: 'position', width: 90, align: 'center' as const,
      filters: Array.from(new Set(items.map((i) => i.position).filter(Boolean))).map((p) => ({ text: p as string, value: p as string })),
      onFilter: (val: any, r: StockItem) => r.position === val,
      sorter: (a: StockItem, b: StockItem) => (a.position || 'Z').localeCompare(b.position || 'Z'),
      render: (v?: string) => {
        if (!v) return <span style={{ color: '#cbd5e1' }}>-</span>;
        const colorMap: Record<string, string> = { S: '#7c3aed', A: '#1e40af', B: '#16a34a', C: '#f59e0b', D: '#94a3b8' };
        const c = colorMap[v.toUpperCase()] || '#64748b';
        return <Tag color={c} style={{ marginRight: 0, fontWeight: 600 }}>{v}</Tag>;
      },
    },
  ];

  if (!isAggMode) {
    columns.push({ title: '仓库', dataIndex: 'warehouse', key: 'warehouse', width: 200, ellipsis: true });
  } else {
    columns.push({
      title: '仓库', key: 'whCount', width: 120, align: 'center' as const,
      render: (_: any, r: StockItem) => r.whCount && r.whCount > 1
        ? <span style={{ fontSize: 12 }}>
          <span style={{ color: '#1e40af' }}>{r.whCount}个仓</span>
          {(r.whStockout || 0) > 0 && <span style={{ color: '#ef4444', marginLeft: 4 }}>{r.whStockout}仓缺货</span>}
        </span>
        : <span style={{ color: '#94a3b8', fontSize: 12 }}>{r.warehouses?.[0]?.warehouse || '1个仓'}</span>,
    });
  }

  columns.push(
    {
      title: '可用库存', key: 'usableQty', width: 100, align: 'right' as const,
      sorter: (a: StockItem, b: StockItem) => a.usableQty - b.usableQty,
      render: (_: any, r: StockItem) => {
        const color = r.usableQty <= 0 ? '#ef4444' : r.usableQty < 100 ? '#f59e0b' : '#1e293b';
        return <span style={{ fontWeight: 600, color, fontVariantNumeric: 'tabular-nums' }}>{r.usableQty.toLocaleString()}</span>;
      },
    },
    {
      title: '可售天数', key: 'sellableDays', width: 100, align: 'right' as const,
      sorter: (a: StockItem, b: StockItem) => a.sellableDays - b.sellableDays,
      render: (_: any, r: StockItem) => {
        if (r.monthQty === 0 && (r.currentQty || 0) === 0 && r.usableQty <= 0) return '-';
        if (r.monthQty === 0) return <span style={{ color: '#94a3b8' }}>无销量</span>;
        if (r.sellableDays <= 0) return <span style={{ color: '#ef4444', fontWeight: 700 }}>0</span>;
        const color = r.sellableDays < 7 ? '#ef4444' : r.sellableDays < 14 ? '#f59e0b' : r.sellableDays > 90 ? '#7c3aed' : '#1e293b';
        return <span style={{ fontWeight: 600, color, fontVariantNumeric: 'tabular-nums' }}>{r.sellableDays}</span>;
      },
    },
    {
      title: '日均销量', key: 'dailyAvg', width: 90, align: 'right' as const,
      render: (_: any, r: StockItem) => r.dailyAvg > 0 ? r.dailyAvg : '-',
    },
    {
      title: '近30天销量', dataIndex: 'monthQty', key: 'monthQty', width: 110, align: 'right' as const,
      sorter: (a: StockItem, b: StockItem) => a.monthQty - b.monthQty,
      render: (v: number) => v > 0 ? v.toLocaleString() : '-',
    },
  );

  if (loading && items.length === 0) return <PageLoading />;

  return (
    <div>
      {/* 数据来源 + 指标说明 */}
      <div style={{ background: '#f8fafc', border: '1px solid #e2e8f0', borderRadius: 6, padding: '10px 14px', marginBottom: 12, fontSize: 12, color: '#64748b', lineHeight: '20px' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 8 }}>
          <div style={{ flex: 1 }}>
            <InfoCircleOutlined style={{ marginRight: 6, color: '#1e40af' }} />
            <span style={{ color: '#1e293b', fontWeight: 600 }}>数据来源：</span>
            南京委外成品仓、天津委外仓、西安仓库成品、松鲜鲜&大地密码云仓、长沙委外成品仓、安徽郎溪成品、南京分销虚拟仓（共 7 个仓库）
          </div>
          {syncInfo.lastFinishedAt && (
            <div style={{ whiteSpace: 'nowrap', color: syncInfo.lastError ? '#dc2626' : '#16a34a', fontSize: 11 }}>
              {syncInfo.lastError ? '⚠ 上次同步失败' : '✓ 上次同步'}：{formatSyncAgo(syncInfo.lastFinishedAt)}
              {!syncInfo.lastError && syncInfo.lastElapsedSec ? `（耗时 ${syncInfo.lastElapsedSec}s）` : ''}
            </div>
          )}
        </div>
        <div style={{ marginTop: 4 }}>
          <span style={{ color: '#1e293b', fontWeight: 600, marginLeft: 20 }}>核心公式：</span>
          可用库存 = 当前库存 − 锁定库存；&nbsp;&nbsp;日均销量 = 近 30 天销量 ÷ 30；&nbsp;&nbsp;可售天数 = 可用库存 ÷ 日均销量
        </div>
        <div style={{ marginTop: 4, marginLeft: 20 }}>
          <span style={{ color: '#ef4444', fontWeight: 600 }}>断货</span>：可用库存 ≤ 0 且近 30 天有销量&nbsp;&nbsp;|&nbsp;&nbsp;
          <span style={{ color: '#ea580c', fontWeight: 600 }}>即将断货</span>：可售天数 &lt; 7 天&nbsp;&nbsp;|&nbsp;&nbsp;
          <span style={{ color: '#f59e0b', fontWeight: 600 }}>库存偏低</span>：可售天数 7–14 天&nbsp;&nbsp;|&nbsp;&nbsp;
          <span style={{ color: '#7c3aed', fontWeight: 600 }}>库存积压</span>：可售天数 &gt; 90 天&nbsp;&nbsp;|&nbsp;&nbsp;
          <span style={{ color: '#94a3b8', fontWeight: 600 }}>滞销品</span>：近 30 天零销量 且当前库存 &gt; 0
        </div>
      </div>

      {/* 预警统计卡片 */}
      <Row gutter={[12, 12]}>
        {statCards.map(card => (
          <Col xs={12} sm={8} lg={4} xl={4} key={card.key}>
            <Card
              hoverable
              onClick={() => setFilter(filter === card.key ? 'all' : card.key)}
              style={{
                borderLeft: `3px solid ${card.color}`,
                background: filter === card.key ? card.bg : '#fff',
                cursor: 'pointer',
              }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 12, color: '#64748b', marginBottom: 4 }}>{card.title}</div>
                  <div style={{ fontSize: 24, fontWeight: 700, color: card.color, fontVariantNumeric: 'tabular-nums' }}>
                    {card.value}
                  </div>
                  <div style={{ fontSize: 11, color: '#b0b8c4', marginTop: 4, lineHeight: '14px' }}>{card.desc}</div>
                </div>
                <div style={{ fontSize: 24, color: card.color, opacity: 0.15 }}>{card.icon}</div>
              </div>
            </Card>
          </Col>
        ))}
        <Col xs={12} sm={8} lg={4} xl={4}>
          <Card style={{ borderLeft: '3px solid #1e40af' }}>
            <div style={{ fontSize: 12, color: '#64748b', marginBottom: 4 }}>成品SKU总数</div>
            <div style={{ fontSize: 24, fontWeight: 700, color: '#1e40af', fontVariantNumeric: 'tabular-nums' }}>
              {summary.total}
            </div>
            <div style={{ fontSize: 11, color: '#b0b8c4', marginTop: 4, lineHeight: '14px' }}>货品属性为成品的SKU</div>
          </Card>
        </Col>
      </Row>

      {/* 筛选栏 + 表格 */}
      <Card style={{ marginTop: 16 }}>
        <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center', marginBottom: 16 }}>
          <Select
            style={{ width: 240 }}
            placeholder="全部仓库（按商品聚合）"
            allowClear
            value={warehouse || undefined}
            onChange={v => setWarehouse(v || '')}
            options={whList.map(w => ({ label: w, value: w }))}
          />
          <Select
            style={{ width: 180 }}
            value={filter}
            onChange={v => setFilter(v)}
            options={warningFilters.map(f => ({ label: f.label, value: f.key }))}
          />
          <Search
            placeholder="搜索商品编码/名称"
            allowClear
            style={{ width: 240 }}
            onSearch={v => setKeyword(v)}
          />
          <span style={{ color: '#94a3b8', fontSize: 12, marginLeft: 'auto' }}>
            共 {items.length} {isAggMode ? '个商品' : '条记录'}
            {isAggMode && <span style={{ marginLeft: 8, color: '#b0b8c4' }}>点击行展开仓库明细</span>}
          </span>
          <Tooltip title={syncing
            ? `正在拉取吉客云全量库存（已 ${liveElapsedSec}s），约 2-3 分钟。可关闭页面，下次回来自动刷新。`
            : '立即从吉客云拉取最新库存（全量 SKU，约 2-3 分钟）。定时任务每小时自动跑一次。'}>
            <Button
              type="primary"
              icon={<ThunderboltOutlined />}
              loading={syncing}
              onClick={handleSyncNow}
            >
              {syncing ? `同步中（${liveElapsedSec}s）` : '实时刷新'}
            </Button>
          </Tooltip>
        </div>

        <Table
          dataSource={items}
          columns={columns}
          rowKey={(r) => r.goodsNo + (r.warehouse || '')}
          expandable={isAggMode ? {
            expandedRowRender,
            rowExpandable: (r) => !!(r.warehouses && r.warehouses.length > 0),
          } : undefined}
          pagination={{ defaultPageSize: 50, showSizeChanger: true, pageSizeOptions: ['50', '100', '200', '500'], showTotal: t => `共 ${t} 条` }}
          size="small"
          scroll={{ x: 900 }}
          loading={loading}
        />
      </Card>
    </div>
  );
};

export default InventoryWarning;
