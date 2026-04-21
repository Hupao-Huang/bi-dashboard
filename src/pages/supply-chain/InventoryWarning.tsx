import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Row, Col, Card, Table, Select, Input, Tag } from 'antd';
import {
  StopOutlined,
  AlertOutlined,
  ExclamationCircleOutlined,
  InboxOutlined,
  PauseCircleOutlined,
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
    { title: '商品名称', dataIndex: 'goodsName', key: 'goodsName', width: 280, ellipsis: true },
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
        </div>

        <Table
          dataSource={items}
          columns={columns}
          rowKey={(r) => r.goodsNo + (r.warehouse || '')}
          expandable={isAggMode ? {
            expandedRowRender,
            rowExpandable: (r) => !!(r.warehouses && r.warehouses.length > 0),
          } : undefined}
          pagination={{ pageSize: 50, showSizeChanger: true, pageSizeOptions: ['50', '100', '200'], showTotal: t => `共 ${t} 条` }}
          size="small"
          scroll={{ x: 900 }}
          loading={loading}
        />
      </Card>
    </div>
  );
};

export default InventoryWarning;
