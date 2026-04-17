import React, { useEffect, useState, useCallback } from 'react';
import { Card, Table, Select, Input, Tag, Space } from 'antd';
import { SearchOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';
import PageLoading from '../../components/PageLoading';

// 平台对应的 Tag 颜色
const PLATFORM_COLORS: Record<string, string> = {
  '吉客云': 'blue',
  '抖音': 'volcano',
  '快手': 'orange',
  '拼多多': 'magenta',
  '天猫': 'red',
  '京东': 'geekblue',
  '小红书': 'pink',
  '微信': 'green',
  '线下': 'cyan',
  '分销': 'purple',
};

const getPlatformColor = (platform: string): string => {
  if (PLATFORM_COLORS[platform]) return PLATFORM_COLORS[platform];
  // 根据字符串哈希分配颜色（对未知平台保持稳定颜色）
  const colors = ['blue', 'green', 'orange', 'purple', 'cyan', 'magenta', 'gold', 'lime', 'geekblue', 'volcano'];
  let hash = 0;
  for (let i = 0; i < platform.length; i++) {
    hash = (hash * 31 + platform.charCodeAt(i)) % colors.length;
  }
  return colors[Math.abs(hash)];
};

interface RPAMappingRow {
  id: number;
  platform: string;
  table_name: string;
  rpa_file_keyword: string;
  file_format: string;
  import_tool: string;
}

const RPAMapping: React.FC = () => {
  const [data, setData] = useState<RPAMappingRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [platforms, setPlatforms] = useState<string[]>([]);
  const [filterPlatform, setFilterPlatform] = useState<string | undefined>(undefined);
  const [keyword, setKeyword] = useState('');

  const fetchData = useCallback(() => {
    setLoading(true);
    fetch(`${API_BASE}/api/admin/docs/rpa-mapping`, { credentials: 'include' })
      .then(res => res.json())
      .then(res => {
        const rows: RPAMappingRow[] = res.data || res || [];
        setData(rows);
        // 提取所有唯一平台
        const platSet = Array.from(new Set(rows.map((r: RPAMappingRow) => r.platform).filter(Boolean)));
        setPlatforms(platSet);
        setLoading(false);
      })
      .catch(() => setLoading(false));
  }, []);

  useEffect(() => { fetchData(); }, [fetchData]);

  // 前端过滤
  const filtered = data.filter(row => {
    if (filterPlatform && row.platform !== filterPlatform) return false;
    if (keyword) {
      const kw = keyword.toLowerCase();
      return (
        (row.platform || '').toLowerCase().includes(kw) ||
        (row.table_name || '').toLowerCase().includes(kw) ||
        (row.rpa_file_keyword || '').toLowerCase().includes(kw) ||
        (row.file_format || '').toLowerCase().includes(kw) ||
        (row.import_tool || '').toLowerCase().includes(kw)
      );
    }
    return true;
  });

  const columns = [
    {
      title: '序号',
      key: 'index',
      width: 55,
      render: (_: any, __: any, index: number) => index + 1,
    },
    {
      title: '平台',
      dataIndex: 'platform',
      key: 'platform',
      width: 110,
      render: (v: string) => v ? <Tag color={getPlatformColor(v)}>{v}</Tag> : <span style={{ color: '#bbb' }}>-</span>,
    },
    {
      title: '数据库表名',
      dataIndex: 'table_name',
      key: 'table_name',
      width: 260,
      ellipsis: true,
    },
    {
      title: 'RPA文件名关键字',
      dataIndex: 'rpa_file_keyword',
      key: 'rpa_file_keyword',
      width: 240,
      ellipsis: true,
    },
    {
      title: '格式',
      dataIndex: 'file_format',
      key: 'file_format',
      width: 70,
    },
    {
      title: '导入工具',
      dataIndex: 'import_tool',
      key: 'import_tool',
      width: 150,
    },
  ];

  if (loading && data.length === 0) return <PageLoading />;

  return (
    <Card title="RPA文件映射">
      <Space style={{ marginBottom: 16 }} wrap>
        <Select
          placeholder="按平台筛选"
          allowClear
          style={{ width: 160 }}
          value={filterPlatform}
          onChange={v => setFilterPlatform(v)}
          options={platforms.map(p => ({ value: p, label: p }))}
        />
        <Input
          placeholder="搜索表名/关键字/工具"
          prefix={<SearchOutlined />}
          allowClear
          style={{ width: 260 }}
          value={keyword}
          onChange={e => setKeyword(e.target.value)}
        />
      </Space>
      <Table
        dataSource={filtered}
        columns={columns}
        rowKey={(r, i) => r.id ?? i ?? Math.random()}
        loading={loading}
        size="small"
        pagination={false}
        scroll={{ y: 500, x: 890 }}
        rowClassName={(r: RPAMappingRow) => `rpa-row-${(r.platform || 'unknown').replace(/[^a-zA-Z0-9]/g, '')}`}
      />
    </Card>
  );
};

export default RPAMapping;
