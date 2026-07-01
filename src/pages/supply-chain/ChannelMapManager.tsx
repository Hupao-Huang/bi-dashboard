import React, { useState, useEffect, useCallback } from 'react';
import { Modal, Table, Input, Select, Button, Space, message, Tag, Checkbox } from 'antd';
import { ApartmentOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { API_BASE } from '../../config';
import { useAuth } from '../../auth/AuthContext';

// 销售日报「渠道对应关系表」维护器(跑哥 2026-07-01):
// 店铺名来自吉客云(sales_channel, 只读不可改), 业务手动选每个店铺的 渠道 + 平台。
// 弹窗列出吉客云全部店铺, 未配的排前面高亮。改了立即影响渠道汇总口径, 保存走二次确认。
const EDIT_PERM = 'supply_chain.sales_daily_report:edit';
const PLATFORMS = ['社媒', '电商', '其他'];

interface Row {
  shopName: string;   // 吉客云店铺名(只读)
  channel: string;    // 业务填
  platform: string;   // 业务选
  mapped: boolean;    // 是否已配
  cateName: string;   // 吉客云渠道分类(参考)
  _uid: string;
  _dirty?: boolean;
}

interface Props {
  onSaved?: () => void;
}

const ChannelMapManager: React.FC<Props> = ({ onSaved }) => {
  const { hasPermission } = useAuth();
  const canEdit = hasPermission(EDIT_PERM);
  const [open, setOpen] = useState(false);
  const [rows, setRows] = useState<Row[]>([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [kw, setKw] = useState('');
  const [onlyUnmapped, setOnlyUnmapped] = useState(false);

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

  const setCell = (uid: string, key: keyof Row, val: string) => {
    setRows(prev => prev.map(r => (r._uid === uid ? { ...r, [key]: val, _dirty: true } : r)));
  };

  const dirtyRows = rows.filter(r => r._dirty);

  const save = () => {
    // 只提交 渠道+平台 都填了的脏行(店铺来自吉客云一定有)
    const toSave = dirtyRows.filter(r => r.channel.trim() && r.platform.trim());
    const incomplete = dirtyRows.filter(r => !r.channel.trim() || !r.platform.trim());
    if (incomplete.length) { message.error(`有 ${incomplete.length} 个店铺渠道或平台没填全`); return; }
    if (!toSave.length) { message.info('没有要保存的改动'); return; }
    setSaving(true);
    fetch(`${API_BASE}/api/supply-chain/channel-map/save`, {
      method: 'POST', credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ rows: toSave.map(r => ({ shopName: r.shopName, channel: r.channel.trim(), platform: r.platform })) }),
    })
      .then(r => r.json())
      .then(j => {
        if (j.code === 200) { message.success(`已保存 ${j.data?.saved ?? toSave.length} 条`); load(); onSaved?.(); }
        else message.error(j.msg || '保存失败');
      })
      .catch(err => message.error(`保存失败: ${err instanceof Error ? err.message : String(err)}`))
      .finally(() => setSaving(false));
  };

  const unmappedCount = rows.filter(r => !r.mapped).length;
  let filtered = rows;
  if (onlyUnmapped) filtered = filtered.filter(r => !r.mapped);
  if (kw.trim()) {
    const k = kw.trim().toLowerCase();
    filtered = filtered.filter(r => (r.shopName + r.channel + r.platform + r.cateName).toLowerCase().includes(k));
  }

  const columns: ColumnsType<Row> = [
    {
      title: '店铺名(吉客云)', dataIndex: 'shopName', key: 'shopName',
      render: (v: string, r: Row) => (
        <span>{v} {!r.mapped && <Tag color="orange">未配</Tag>}</span>
      ),
    },
    { title: '吉客云分类', dataIndex: 'cateName', key: 'cateName', width: 100,
      render: (v: string) => v || <span style={{ color: 'rgba(0,0,0,0.25)' }}>—</span> },
    {
      title: '渠道', dataIndex: 'channel', key: 'channel', width: 170,
      render: (v: string, r: Row) => canEdit
        ? <Input value={v} size="small" placeholder="如 抖音/天猫/分销" onChange={e => setCell(r._uid, 'channel', e.target.value)} />
        : v,
    },
    {
      title: '平台', dataIndex: 'platform', key: 'platform', width: 120,
      render: (v: string, r: Row) => canEdit
        ? <Select value={v || undefined} size="small" style={{ width: '100%' }} placeholder="选平台" options={PLATFORMS.map(p => ({ label: p, value: p }))} onChange={val => setCell(r._uid, 'platform', val)} />
        : v,
    },
  ];

  return (
    <>
      <Button size="small" icon={<ApartmentOutlined />} onClick={() => setOpen(true)}>渠道对应关系</Button>
      <Modal
        title="渠道对应关系表(吉客云店铺 → 渠道 → 平台)"
        open={open}
        onCancel={() => setOpen(false)}
        width={880}
        footer={
          canEdit ? [
            <Button key="cancel" onClick={() => setOpen(false)}>关闭</Button>,
            <Button key="save" type="primary" loading={saving} disabled={dirtyRows.length === 0} onClick={save}>
              保存改动{dirtyRows.length ? `(${dirtyRows.length})` : ''}
            </Button>,
          ] : [<Button key="cancel" onClick={() => setOpen(false)}>关闭</Button>]
        }
      >
        <Space style={{ marginBottom: 12, width: '100%', justifyContent: 'space-between' }}>
          <Input.Search placeholder="搜店铺/渠道/平台/分类" allowClear style={{ width: 260 }} onChange={e => setKw(e.target.value)} />
          <Space>
            <Checkbox checked={onlyUnmapped} onChange={e => setOnlyUnmapped(e.target.checked)}>只看未配({unmappedCount})</Checkbox>
            {!canEdit && <Tag>只读(无编辑权限)</Tag>}
          </Space>
        </Space>
        <div style={{ marginBottom: 8, color: 'rgba(0,0,0,0.45)', fontSize: 12 }}>店铺名来自吉客云自动带出, 只需给每个店铺选渠道和平台。未配的店铺不计入渠道汇总。</div>
        <Table
          rowKey="_uid"
          columns={columns}
          dataSource={filtered}
          loading={loading}
          size="small"
          rowClassName={(r) => (r._dirty ? 'ant-table-row-selected' : '')}
          pagination={{ pageSize: 8, showSizeChanger: false, showTotal: (t) => `共 ${t} 条` }}
          scroll={{ y: 360 }}
        />
      </Modal>
    </>
  );
};

export default ChannelMapManager;
