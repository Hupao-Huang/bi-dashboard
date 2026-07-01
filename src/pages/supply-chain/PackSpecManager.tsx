import React, { useState, useEffect, useCallback } from 'react';
import { Modal, Table, Input, InputNumber, Button, Space, Popconfirm, message, Tag } from 'antd';
import { PlusOutlined, InboxOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { API_BASE } from '../../config';
import { useAuth } from '../../auth/AuthContext';

// 销售日报「箱规映射表」维护器: 货品编码→箱规→托规, 供应链角色可随时调整(跑哥 2026-07-01)。
// 箱规=1 表示该货品已是最小单位(发货件数=sell_count×1)。改了立即影响单品发货件数/箱数/托数口径。
const EDIT_PERM = 'supply_chain.sales_daily_report:edit';

interface Row {
  goodsNo: string;
  boxQty: number;
  palletBoxQty: number; // 0 = 未填
  _uid: string;
  _isNew?: boolean;
  _dirty?: boolean;
}

let newRowSeq = 0;

interface Props {
  onSaved?: () => void;
}

const PackSpecManager: React.FC<Props> = ({ onSaved }) => {
  const { hasPermission } = useAuth();
  const canEdit = hasPermission(EDIT_PERM);
  const [open, setOpen] = useState(false);
  const [rows, setRows] = useState<Row[]>([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [kw, setKw] = useState('');

  const load = useCallback(() => {
    setLoading(true);
    fetch(`${API_BASE}/api/supply-chain/pack-spec`, { credentials: 'include' })
      .then(r => r.json())
      .then(j => {
        if (j.code === 200) setRows((j.data?.list || []).map((m: Row) => ({ ...m, _uid: 'g:' + m.goodsNo })));
        else message.error(j.msg || '加载失败');
      })
      .catch(err => message.error(`加载失败: ${err instanceof Error ? err.message : String(err)}`))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => { if (open) load(); }, [open, load]);

  const setCell = (uid: string, key: keyof Row, val: string | number) => {
    setRows(prev => prev.map(r => (r._uid === uid ? { ...r, [key]: val, _dirty: true } : r)));
  };

  const addRow = () => {
    newRowSeq += 1;
    setRows(prev => [{ goodsNo: '', boxQty: 1, palletBoxQty: 0, _uid: 'new:' + newRowSeq, _isNew: true, _dirty: true }, ...prev]);
  };

  const removeRow = (row: Row) => {
    if (row._isNew) {
      setRows(prev => prev.filter(r => r._uid !== row._uid));
      return;
    }
    fetch(`${API_BASE}/api/supply-chain/pack-spec/delete`, {
      method: 'POST', credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ goodsNo: row.goodsNo }),
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
    for (const r of dirtyRows) {
      if (!r.goodsNo.trim()) { message.error('有货品编码没填'); return; }
      if (!(r.boxQty > 0)) { message.error(`货品「${r.goodsNo}」箱规要大于0(最小单位填1)`); return; }
    }
    setSaving(true);
    fetch(`${API_BASE}/api/supply-chain/pack-spec/save`, {
      method: 'POST', credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ rows: dirtyRows.map(r => ({ goodsNo: r.goodsNo.trim(), boxQty: r.boxQty, palletBoxQty: r.palletBoxQty || 0 })) }),
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
    ? rows.filter(r => r.goodsNo.toLowerCase().includes(kw.trim().toLowerCase()))
    : rows;

  const columns: ColumnsType<Row> = [
    {
      title: '货品编码', dataIndex: 'goodsNo', key: 'goodsNo',
      render: (v: string, r: Row) => (canEdit && r._isNew)
        ? <Input value={v} placeholder="货品编码(同吉客云)" onChange={e => setCell(r._uid, 'goodsNo', e.target.value)} />
        : v,
    },
    {
      title: '箱规(每箱数量)', dataIndex: 'boxQty', key: 'boxQty', width: 150,
      render: (v: number, r: Row) => canEdit
        ? <InputNumber min={1} value={v} style={{ width: '100%' }} onChange={val => setCell(r._uid, 'boxQty', Number(val) || 1)} />
        : v,
    },
    {
      title: '托规(每托箱数)', dataIndex: 'palletBoxQty', key: 'palletBoxQty', width: 150,
      render: (v: number, r: Row) => canEdit
        ? <InputNumber min={0} value={v} style={{ width: '100%' }} placeholder="空=不填" onChange={val => setCell(r._uid, 'palletBoxQty', Number(val) || 0)} />
        : (v > 0 ? v : '—'),
    },
  ];
  if (canEdit) {
    columns.push({
      title: '操作', key: 'op', width: 80,
      render: (_: unknown, r: Row) => (
        <Popconfirm title="删除这条箱规?" onConfirm={() => removeRow(r)} okText="删除" cancelText="取消">
          <Button type="link" danger size="small">删除</Button>
        </Popconfirm>
      ),
    });
  }

  return (
    <>
      <Button size="small" icon={<InboxOutlined />} onClick={() => setOpen(true)}>箱规维护</Button>
      <Modal
        title="箱规映射表(货品 → 箱规 → 托规)"
        open={open}
        onCancel={() => setOpen(false)}
        width={640}
        footer={
          canEdit ? [
            <Button key="cancel" onClick={() => setOpen(false)}>关闭</Button>,
            <Popconfirm
              key="save"
              title="确认保存?"
              description="改动会立即影响单品的发货件数/箱数/托数口径"
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
          <Input.Search placeholder="搜货品编码" allowClear style={{ width: 240 }} onChange={e => setKw(e.target.value)} />
          {canEdit ? <Button icon={<PlusOutlined />} onClick={addRow}>新增箱规</Button> : <Tag>只读(无编辑权限)</Tag>}
        </Space>
        <div style={{ marginBottom: 8, color: 'rgba(0,0,0,0.45)', fontSize: 12 }}>箱规=1 表示该货品已是最小单位(按袋/瓶发货,不折算)</div>
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

export default PackSpecManager;
