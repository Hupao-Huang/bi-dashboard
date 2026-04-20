import React, { useEffect, useState, useCallback } from 'react';
import { Table, Input, Typography, Tag, Empty } from 'antd';
import { SearchOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';
import PageLoading from '../../components/PageLoading';

const { Title, Text } = Typography;

interface ColumnInfo {
  column_name: string;
  column_type: string;
  is_nullable: string;
  column_key: string;
  column_default: string | null;
  column_comment: string;
}

interface TableInfo {
  table_name: string;
  table_comment: string;
  columns: ColumnInfo[];
}

type DBDictData = Record<string, TableInfo>;

const indexTagColor: Record<string, string> = {
  PRI: 'red',
  UNI: 'orange',
  MUL: 'blue',
};

const DBDictionary: React.FC = () => {
  const [dictData, setDictData] = useState<DBDictData>({});
  const [loading, setLoading] = useState(false);
  const [tableList, setTableList] = useState<string[]>([]);
  const [selectedTable, setSelectedTable] = useState<string | null>(null);
  const [tableSearch, setTableSearch] = useState('');

  const fetchData = useCallback(() => {
    setLoading(true);
    fetch(`${API_BASE}/api/admin/docs/db-dict`, { credentials: 'include' })
      .then(res => res.json())
      .then(res => {
        const arr: TableInfo[] = res.data || res || [];
        const d: DBDictData = {};
        arr.forEach((t: TableInfo) => { d[t.table_name] = t; });
        setDictData(d);
        const tables = arr.map((t: TableInfo) => t.table_name);
        setTableList(tables);
        if (tables.length > 0 && !selectedTable) {
          setSelectedTable(tables[0]);
        }
        setLoading(false);
      })
      .catch(err => { console.warn('DBDictionary fetch:', err); setLoading(false); });
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => { fetchData(); }, [fetchData]);

  const filteredTables = tableSearch
    ? tableList.filter(t =>
        t.toLowerCase().includes(tableSearch.toLowerCase()) ||
        (dictData[t]?.table_comment || '').toLowerCase().includes(tableSearch.toLowerCase())
      )
    : tableList;

  const selectedInfo = selectedTable ? dictData[selectedTable] : null;

  const columns = [
    {
      title: '字段名',
      dataIndex: 'column_name',
      key: 'column_name',
      width: 200,
      render: (v: string) => <span style={{ fontSize: 13 }}>{v}</span>,
    },
    {
      title: '类型',
      dataIndex: 'column_type',
      key: 'column_type',
      width: 160,
      render: (v: string) => <Text type="secondary" style={{ fontSize: 12 }}>{v}</Text>,
    },
    {
      title: '可空',
      dataIndex: 'is_nullable',
      key: 'is_nullable',
      width: 70,
      render: (v: string) => v === 'YES'
        ? <Tag color="default">YES</Tag>
        : <Tag color="blue">NO</Tag>,
    },
    {
      title: '索引',
      dataIndex: 'column_key',
      key: 'column_key',
      width: 70,
      render: (v: string) => v
        ? <Tag color={indexTagColor[v] || 'default'}>{v}</Tag>
        : <span style={{ color: '#bbb' }}>-</span>,
    },
    {
      title: '默认值',
      dataIndex: 'column_default',
      key: 'column_default',
      width: 120,
      render: (v: string | null) => v !== null && v !== undefined
        ? <span style={{ fontSize: 12 }}>{v}</span>
        : <span style={{ color: '#bbb' }}>NULL</span>,
    },
    {
      title: '备注',
      dataIndex: 'column_comment',
      key: 'column_comment',
      ellipsis: true,
      render: (v: string) => v || <span style={{ color: '#bbb' }}>-</span>,
    },
  ];

  if (loading && tableList.length === 0) return <PageLoading />;

  return (
    <div style={{ display: 'flex', gap: 0, height: 'calc(100vh - 180px)', background: '#fff', borderRadius: 8, overflow: 'hidden', border: '1px solid #f0f0f0' }}>
      {/* 左侧表列表 */}
      <div style={{ width: 280, flexShrink: 0, borderRight: '1px solid #f0f0f0', display: 'flex', flexDirection: 'column' }}>
        <div style={{ padding: '12px 12px 8px', borderBottom: '1px solid #f0f0f0' }}>
          <div style={{ fontWeight: 600, marginBottom: 8, fontSize: 14 }}>数据库表</div>
          <Input
            placeholder="搜索表名/备注"
            prefix={<SearchOutlined />}
            allowClear
            size="small"
            value={tableSearch}
            onChange={e => setTableSearch(e.target.value)}
          />
        </div>
        <div style={{ flex: 1, overflowY: 'auto' }}>
          {filteredTables.length === 0 && (
            <div style={{ padding: 24, color: '#bbb', textAlign: 'center', fontSize: 13 }}>无匹配表</div>
          )}
          {filteredTables.map(t => {
            const info = dictData[t];
            const isSelected = selectedTable === t;
            return (
              <div
                key={t}
                onClick={() => setSelectedTable(t)}
                style={{
                  padding: '8px 14px',
                  cursor: 'pointer',
                  background: isSelected ? '#e6f4ff' : 'transparent',
                  borderLeft: isSelected ? '3px solid #1677ff' : '3px solid transparent',
                  transition: 'background 0.15s',
                }}
              >
                <div style={{ fontSize: 13, color: isSelected ? '#1677ff' : '#333', fontWeight: isSelected ? 600 : 400, wordBreak: 'break-all' }}>
                  {t}
                </div>
                {info?.table_comment && (
                  <div style={{ fontSize: 11, color: '#999', marginTop: 2, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                    {info.table_comment}
                  </div>
                )}
              </div>
            );
          })}
        </div>
        <div style={{ padding: '8px 12px', borderTop: '1px solid #f0f0f0', color: '#999', fontSize: 12 }}>
          共 {filteredTables.length} / {tableList.length} 张表
        </div>
      </div>

      {/* 右侧字段详情 */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        {selectedInfo ? (
          <>
            <div style={{ padding: '14px 20px', borderBottom: '1px solid #f0f0f0', flexShrink: 0 }}>
              <Title level={5} style={{ margin: 0 }}>
                {selectedTable}
              </Title>
              {selectedInfo.table_comment && (
                <Text type="secondary" style={{ fontSize: 13 }}>{selectedInfo.table_comment}</Text>
              )}
              <Text style={{ fontSize: 12, color: '#999', marginLeft: selectedInfo.table_comment ? 12 : 0 }}>
                {selectedInfo.columns?.length ?? 0} 个字段
              </Text>
            </div>
            <div style={{ flex: 1, overflow: 'auto', padding: '0 0 0 0' }}>
              <Table
                dataSource={selectedInfo.columns || []}
                columns={columns}
                rowKey="column_name"
                size="small"
                pagination={false}
                loading={loading}
                scroll={{ x: 820 }}
                style={{ minHeight: '100%' }}
              />
            </div>
          </>
        ) : (
          <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <Empty description="请从左侧选择一张表" />
          </div>
        )}
      </div>
    </div>
  );
};

export default DBDictionary;
