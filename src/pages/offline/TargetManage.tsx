import React, { useCallback, useEffect, useState } from 'react';
import { Button, Card, InputNumber, message, Select, Spin, Table } from 'antd';
import { SaveOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';

const REGIONS = ['华北大区', '华东大区', '华中大区', '华南大区', '西南大区', '西北大区', '东北大区', '山东大区', '重客'];
const MONTHS = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12];

const TargetManage: React.FC = () => {
  const [year, setYear] = useState(new Date().getFullYear());
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  // targets[region][month] = value
  const [targets, setTargets] = useState<Record<string, Record<number, number>>>({});

  const fetchTargets = useCallback(async (y: number) => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/offline/targets?year=${y}`);
      const json = await res.json();
      if (!res.ok) {
        message.error(`加载失败：${json.msg || res.status}`);
        return;
      }
      const items = json.data?.items || [];
      const map: Record<string, Record<number, number>> = {};
      items.forEach((it: any) => {
        if (!map[it.region]) map[it.region] = {};
        map[it.region][it.month] = Number(it.target);
      });
      setTargets(map);
    } catch (e: any) {
      message.error(`请求失败：${e.message}`);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchTargets(year); }, [fetchTargets, year]);

  const handleChange = (region: string, month: number, value: number | null) => {
    setTargets(prev => ({
      ...prev,
      [region]: { ...(prev[region] || {}), [month]: value ?? 0 },
    }));
  };

  const handleSave = async () => {
    setSaving(true);
    const items: any[] = [];
    REGIONS.forEach(region => {
      MONTHS.forEach(month => {
        items.push({ month, region, target: targets[region]?.[month] ?? 0 });
      });
    });
    try {
      const res = await fetch(`${API_BASE}/api/offline/targets/save`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ year, items }),
      });
      const json = await res.json();
      if (res.ok) {
        message.success(json.data?.message || '保存成功');
      } else {
        message.error(json.msg || '保存失败');
      }
    } catch {
      message.error('网络错误');
    } finally {
      setSaving(false);
    }
  };

  const columns = [
    {
      title: '大区',
      dataIndex: 'region',
      key: 'region',
      fixed: 'left' as const,
      width: 100,
      render: (v: string) => <span style={{ fontWeight: 600 }}>{v}</span>,
    },
    ...MONTHS.map(m => ({
      title: `${m}月`,
      key: `month_${m}`,
      width: 130,
      render: (_: any, record: any) => (
        <InputNumber
          value={targets[record.region]?.[m] ?? undefined}
          onChange={val => handleChange(record.region, m, val)}
          formatter={v => v ? `¥${Number(v).toLocaleString()}` : ''}
          parser={(v: string | undefined) => Number((v || '').replace(/[¥,]/g, ''))}
          min={0}
          step={10000}
          style={{ width: '100%' }}
          placeholder="未设置"
          variant="borderless"
        />
      ),
    })),
  ];

  const dataSource = REGIONS.map(r => ({ region: r, key: r }));

  // 每月合计行
  const totalRow = {
    region: '合计',
    key: '__total__',
    ...Object.fromEntries(MONTHS.map(m => [
      `month_${m}`,
      REGIONS.reduce((s, r) => s + (targets[r]?.[m] ?? 0), 0),
    ])),
  };
  const totalColumns = [
    { title: '大区', dataIndex: 'region', key: 'region', fixed: 'left' as const, width: 100, render: (v: string) => <span style={{ fontWeight: 700, color: '#1e40af' }}>{v}</span> },
    ...MONTHS.map(m => ({
      title: `${m}月`,
      key: `month_${m}`,
      width: 130,
      dataIndex: `month_${m}`,
      render: (v: number) => v > 0 ? <span style={{ fontWeight: 600, color: '#1e40af' }}>¥{v.toLocaleString()}</span> : <span style={{ color: '#cbd5e1' }}>—</span>,
    })),
  ];

  const yearOptions = [2024, 2025, 2026, 2027].map(y => ({ label: `${y}年`, value: y }));

  return (
    <div style={{ padding: 24 }}>
      <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, alignItems: 'center', marginBottom: 12 }}>
        <Select size="small" options={yearOptions} value={year} onChange={setYear} style={{ width: 90 }} />
        <Button size="small" type="primary" icon={<SaveOutlined />} loading={saving} onClick={handleSave}>
          保存
        </Button>
      </div>

      <Spin spinning={loading}>
        <Card bodyStyle={{ padding: 0, overflowX: 'auto' }}>
          <Table
            dataSource={dataSource}
            columns={columns}
            pagination={false}
            size="small"
            scroll={{ x: 1700 }}
            rowKey="key"
          />
        </Card>
        <Card bodyStyle={{ padding: 0, overflowX: 'auto' }} style={{ marginTop: 8 }}>
          <Table
            dataSource={[totalRow]}
            columns={totalColumns}
            pagination={false}
            size="small"
            scroll={{ x: 1700 }}
            rowKey="key"
            showHeader={false}
          />
        </Card>
      </Spin>

      <div style={{ marginTop: 12, color: '#64748b', fontSize: 13 }}>
        提示：单位为元，留空表示未设置目标。修改后点击右上角"保存"生效。
      </div>
    </div>
  );
};

export default TargetManage;
