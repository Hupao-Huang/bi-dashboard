import React, { useState, useEffect, useCallback } from 'react';
import { Modal, Table, Input, Select, Button, Space, Popconfirm, message, Tag } from 'antd';
import { PlusOutlined, ApartmentOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { API_BASE } from '../../config';
import { useAuth } from '../../auth/AuthContext';

// 销售日报「渠道对应关系表」维护器: 店铺→渠道→平台, 供应链角色可随时调整(跑哥 2026-07-01)。
// 改了立即影响渠道汇总口径, 保存走二次确认。平台三选一手动选。
const EDIT_PERM = 'supply_chain.sales_daily_report:edit';
const PLATFORMS = ['社媒', '电商', '其他'];

interface Row {
  shopName: string;
  channel: string;
  platform: string;
  _uid: string;     // 稳定唯一 key(已有行=shop:店铺名, 新增行=new:自增), 不用数组下标做 key
  _isNew?: boolean; // 新增未保存的行(shopName 可编辑)
  _dirty?: boolean; // 改过待保存
}

let newRowSeq = 0; // 新增行自增序号, 保证 rowKey 稳定不随 prepend 漂移

interface Props {
  onSaved?: () => void; // 保存后通知父组件刷新销售日报(口径变了)
}

const ChannelMapManager: React.FC<Props> = ({ onSaved }) => {
  const { hasPermission } = useAuth();
  const canEdit = hasPermission(EDIT_PERM);
  const [open, setOpen] = useState(false);
  const [rows, setRows] = useState<Row[]>([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [kw, setKw] = useState('');

  const load = useCallback(() => {
    setLoading(true);
    fetch(`${API_BASE}/api/supply-chain/channel-map`, { credentials: 'include' })
      .then(r => r.json())
      .then(j => {
        if (j.code === 200) setRows((j.data?.list || []).map((m: Row) => ({ ...m, _uid: 'shop:' + m.shopName })));
        else message.error(j.msg || '加载失败');
      })
      .catch(err => message.error(`加载失败: ${err instanceof Error ? err.message : String(err)}`))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => { if (open) load(); }, [open, load]);

  // 按 _uid 定位改哪行(不用数组下标, 搜索过滤/prepend 都不会错位)
  const setCell = (uid: string, key: keyof Row, val: string) => {
    setRows(prev => prev.map(r => (r._uid === uid ? { ...r, [key]: val, _dirty: true } : r)));
  };

  const addRow = () => {
    newRowSeq += 1;
    setRows(prev => [{ shopName: '', channel: '', platform: '电商', _uid: 'new:' + newRowSeq, _isNew: true, _dirty: true }, ...prev]);
  };

  const removeRow = (row: Row) => {
    if (row._isNew) {
      // 新增未保存的直接从前端去掉(按 _uid, 不受 prepend 影响)
      setRows(prev => prev.filter(r => r._uid !== row._uid));
      return;
    }
    fetch(`${API_BASE}/api/supply-chain/channel-map/delete`, {
      method: 'POST', credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ shopName: row.shopName }),
    })
      .then(r => r.json())
      .then(j => {
        if (j.code === 200) { message.success('已删除'); setRows(prev => prev.filter(r => r._uid !== row._uid)); onSaved?.(); }
        else message.error(j.msg || '删除失败');
      })
      .catch(err => message.error(`删除失败: ${err instanceof Error ? err.message : String(err)}`));
  };

  const dirtyRows = rows.filter(r => r._dirty);

  const save = () => {
    // 前端先校验
    for (const r of dirtyRows) {
      if (!r.shopName.trim()) { message.error('有店铺名没填'); return; }
      if (!r.channel.trim()) { message.error(`店铺「${r.shopName}」的渠道没填`); return; }
    }
    setSaving(true);
    fetch(`${API_BASE}/api/supply-chain/channel-map/save`, {
      method: 'POST', credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ rows: dirtyRows.map(r => ({ shopName: r.shopName.trim(), channel: r.channel.trim(), platform: r.platform })) }),
    })
      .then(r => r.json())
      .then(j => {
        if (j.code === 200) { message.success(`已保存 ${j.data?.saved ?? dirtyRows.length} 条`); load(); onSaved?.(); }
        else message.error(j.msg || '保存失败');
      })
      .catch(err => message.error(`保存失败: ${err instanceof Error ? err.message : String(err)}`))
      .finally(() => setSaving(false));
  };

  const filtered = kw.trim()
    ? rows.filter(r => (r.shopName + r.channel + r.platform).toLowerCase().includes(kw.trim().toLowerCase()))
    : rows;

  const columns: ColumnsType<Row> = [
    {
      title: '店铺名', dataIndex: 'shopName', key: 'shopName',
      render: (v: string, r: Row) => (canEdit && r._isNew)
        ? <Input value={v} placeholder="店铺全名(同吉客云)" onChange={e => setCell(r._uid, 'shopName', e.target.value)} />
        : v,
    },
    {
      title: '渠道', dataIndex: 'channel', key: 'channel', width: 180,
      render: (v: string, r: Row) => canEdit
        ? <Input value={v} placeholder="如 抖音/天猫/分销" onChange={e => setCell(r._uid, 'channel', e.target.value)} />
        : v,
    },
    {
      title: '平台', dataIndex: 'platform', key: 'platform', width: 130,
      render: (v: string, r: Row) => canEdit
        ? <Select value={v} style={{ width: '100%' }} options={PLATFORMS.map(p => ({ label: p, value: p }))} onChange={val => setCell(r._uid, 'platform', val)} />
        : v,
    },
  ];
  if (canEdit) {
    columns.push({
      title: '操作', key: 'op', width: 80,
      render: (_: unknown, r: Row) => (
        <Popconfirm title="删除这条映射?" onConfirm={() => removeRow(r)} okText="删除" cancelText="取消">
          <Button type="link" danger size="small">删除</Button>
        </Popconfirm>
      ),
    });
  }

  return (
    <>
      <Button size="small" icon={<ApartmentOutlined />} onClick={() => setOpen(true)}>渠道对应关系</Button>
      <Modal
        title="渠道对应关系表(店铺 → 渠道 → 平台)"
        open={open}
        onCancel={() => setOpen(false)}
        width={720}
        footer={
          canEdit ? [
            <Button key="cancel" onClick={() => setOpen(false)}>关闭</Button>,
            <Popconfirm
              key="save"
              title="确认保存?"
              description="改动会立即影响销售日报的渠道汇总口径"
              onConfirm={save}
              okText="保存" cancelText="再想想"
              disabled={dirtyRows.length === 0}
            >
              <Button type="primary" loading={saving} disabled={dirtyRows.length === 0}>
                保存改动{dirtyRows.length ? `(${dirtyRows.length})` : ''}
              </Button>
            </Popconfirm>,
          ] : [<Button key="cancel" onClick={() => setOpen(false)}>关闭</Button>]
        }
      >
        <Space style={{ marginBottom: 12, width: '100%', justifyContent: 'space-between' }}>
          <Input.Search placeholder="搜店铺/渠道/平台" allowClear style={{ width: 260 }} onChange={e => setKw(e.target.value)} />
          {canEdit && <Button icon={<PlusOutlined />} onClick={addRow}>新增映射</Button>}
          {!canEdit && <Tag>只读(无编辑权限)</Tag>}
        </Space>
        <Table
          rowKey="_uid"
          columns={columns}
          dataSource={filtered}
          loading={loading}
          size="small"
          pagination={{ pageSize: 20, showSizeChanger: false }}
        />
      </Modal>
    </>
  );
};

export default ChannelMapManager;
