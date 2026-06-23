import React, { useEffect, useMemo, useState } from 'react';
import { Modal, Input, Checkbox, Switch, Button, Tag, Empty, Typography, message, Popconfirm } from 'antd';
import { CloseOutlined, HolderOutlined } from '@ant-design/icons';

const { Text } = Typography;

export type CfColMeta = { key: string; label: string; fmt?: string; group?: string };
export type CfPreset = { id: number; name: string; keys: string[] };

// 数值类列(可数值排序)：金额/数量/率/比值都算
const CF_NUM_FMTS = new Set(['money', 'cost', 'int', 'rate', 'roi', 'num2']);

// cfColSorter 按列类型生成表头排序函数(千帆/乘风共用)：
//   数值列→按数值排；笔记创建时间(text 但 YYYY-MM-DD HH:MM:SS 字典序=时间序)→按字符串排；
//   其余文字维度列(作者/类型/品类/品牌等)→不排(返回 undefined)。
export const cfColSorter = (key: string, fmt?: string): ((a: any, b: any) => number) | undefined => {
  if (fmt && CF_NUM_FMTS.has(fmt)) return (a, b) => (Number(a[key]) || 0) - (Number(b[key]) || 0);
  if (key === 'createTime') return (a, b) => String(a[key] || '').localeCompare(String(b[key] || ''));
  return undefined;
};

type Props = {
  open: boolean;
  columns: CfColMeta[];          // 全部可选指标
  value: string[];               // 当前已选(有序)
  defaultKeys: string[];         // 恢复默认 = 全部
  presets: CfPreset[];
  onOk: (keys: string[]) => void;
  onCancel: () => void;
  onSavePreset: (name: string, keys: string[]) => void;
  onDeletePreset: (id: number) => void;
};

// 仿小红书千帆「自定义指标」：左侧搜索+分组勾选，右侧已选(拖拽排序/删除)，底部存常用方案
const CfMetricPicker: React.FC<Props> = ({ open, columns, value, defaultKeys, presets, onOk, onCancel, onSavePreset, onDeletePreset }) => {
  const [selected, setSelected] = useState<string[]>(value);
  const [search, setSearch] = useState('');
  const [saveOn, setSaveOn] = useState(false);
  const [presetName, setPresetName] = useState('');
  const [dragIdx, setDragIdx] = useState<number | null>(null);

  // 打开时用外部当前选择初始化草稿
  useEffect(() => {
    if (open) { setSelected(value); setSearch(''); setSaveOn(false); setPresetName(''); }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const labelOf = useMemo(() => {
    const m: Record<string, string> = {};
    columns.forEach((c) => { m[c.key] = c.label; });
    return m;
  }, [columns]);

  // 按分组聚合(保留首次出现顺序)，并按搜索过滤
  const groups = useMemo(() => {
    const order: string[] = [];
    const map: Record<string, CfColMeta[]> = {};
    const kw = search.trim().toLowerCase();
    columns.forEach((c) => {
      if (kw && !c.label.toLowerCase().includes(kw)) return;
      const g = c.group || '其他指标';
      if (!map[g]) { map[g] = []; order.push(g); }
      map[g].push(c);
    });
    return order.map((g) => ({ name: g, items: map[g] }));
  }, [columns, search]);

  const selSet = useMemo(() => new Set(selected), [selected]);

  const toggleKey = (key: string) => {
    setSelected((prev) => (prev.includes(key) ? prev.filter((k) => k !== key) : [...prev, key]));
  };

  // 组全选/取消：全选则移除该组，否则补齐缺的(按组内顺序追加到末尾)
  const toggleGroup = (items: CfColMeta[]) => {
    const keys = items.map((i) => i.key);
    const allIn = keys.every((k) => selSet.has(k));
    if (allIn) {
      setSelected((prev) => prev.filter((k) => !keys.includes(k)));
    } else {
      setSelected((prev) => [...prev, ...keys.filter((k) => !prev.includes(k))]);
    }
  };

  const removeKey = (key: string) => setSelected((prev) => prev.filter((k) => k !== key));

  // 拖拽排序已选项
  const onDrop = (toIdx: number) => {
    if (dragIdx === null || dragIdx === toIdx) { setDragIdx(null); return; }
    setSelected((prev) => {
      const next = [...prev];
      const [moved] = next.splice(dragIdx, 1);
      next.splice(toIdx, 0, moved);
      return next;
    });
    setDragIdx(null);
  };

  const handleOk = () => {
    if (!selected.length) { message.warning('至少选择一个指标'); return; }
    if (saveOn) {
      const nm = presetName.trim();
      if (!nm) { message.warning('请填写常用方案名称'); return; }
      onSavePreset(nm, selected);
    }
    onOk(selected);
  };

  return (
    <Modal
      title="自定义指标"
      open={open}
      onCancel={onCancel}
      onOk={handleOk}
      okText="确定"
      cancelText="取消"
      width={880}
      destroyOnClose
      styles={{ body: { paddingTop: 12 } }}
    >
      {/* 常用方案 */}
      {presets.length > 0 && (
        <div style={{ marginBottom: 12 }}>
          <Text type="secondary" style={{ marginRight: 8 }}>常用方案：</Text>
          {presets.map((p) => (
            <Tag key={p.id} style={{ marginBottom: 4, paddingRight: 4 }}>
              <span style={{ cursor: 'pointer' }} onClick={() => setSelected(p.keys.filter((k) => labelOf[k]))}>
                {p.name}
              </span>
              <Popconfirm title="删除该常用方案?" okText="删除" cancelText="取消" onConfirm={() => onDeletePreset(p.id)}>
                <CloseOutlined style={{ marginLeft: 6, cursor: 'pointer' }} />
              </Popconfirm>
            </Tag>
          ))}
        </div>
      )}

      <div style={{ display: 'flex', gap: 16, height: 440 }}>
        {/* 左：搜索 + 分组勾选 */}
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', border: '1px solid #f0f0f0', borderRadius: 8 }}>
          <div style={{ padding: 12, borderBottom: '1px solid #f0f0f0' }}>
            <Input.Search placeholder="搜索指标" allowClear value={search} onChange={(e) => setSearch(e.target.value)} />
          </div>
          <div style={{ flex: 1, overflowY: 'auto', padding: '4px 12px 12px' }}>
            {groups.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有匹配的指标" />
            ) : (
              groups.map((g) => {
                const keys = g.items.map((i) => i.key);
                const checkedCnt = keys.filter((k) => selSet.has(k)).length;
                return (
                  <div key={g.name} style={{ marginTop: 12 }}>
                    <div style={{ background: '#fafafa', padding: '6px 8px', borderRadius: 4, marginBottom: 6 }}>
                      <Checkbox
                        checked={checkedCnt === keys.length}
                        indeterminate={checkedCnt > 0 && checkedCnt < keys.length}
                        onChange={() => toggleGroup(g.items)}
                      >
                        <Text strong>{g.name}</Text>
                        <Text type="secondary" style={{ marginLeft: 6 }}>({checkedCnt}/{keys.length})</Text>
                      </Checkbox>
                    </div>
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', rowGap: 6, columnGap: 8, paddingLeft: 8 }}>
                      {g.items.map((c) => (
                        <Checkbox key={c.key} checked={selSet.has(c.key)} onChange={() => toggleKey(c.key)}>
                          {c.label}
                        </Checkbox>
                      ))}
                    </div>
                  </div>
                );
              })
            )}
          </div>
        </div>

        {/* 右：已选(拖拽排序) */}
        <div style={{ width: 300, display: 'flex', flexDirection: 'column', border: '1px solid #f0f0f0', borderRadius: 8 }}>
          <div style={{ padding: 12, borderBottom: '1px solid #f0f0f0', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Text strong>已选 {selected.length}/{columns.length}</Text>
            <span>
              <Button type="link" size="small" style={{ padding: '0 4px' }} onClick={() => setSelected(defaultKeys)}>恢复默认</Button>
              <Button type="link" size="small" style={{ padding: '0 4px' }} onClick={() => setSelected([])}>清空</Button>
            </span>
          </div>
          <div style={{ flex: 1, overflowY: 'auto', padding: 8 }}>
            {selected.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="未选指标" />
            ) : (
              selected.map((k, idx) => (
                <div
                  key={k}
                  draggable
                  onDragStart={() => setDragIdx(idx)}
                  onDragOver={(e) => e.preventDefault()}
                  onDrop={() => onDrop(idx)}
                  style={{
                    display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                    padding: '4px 8px', marginBottom: 4, borderRadius: 4,
                    background: dragIdx === idx ? '#e6f4ff' : '#fafafa', cursor: 'move',
                  }}
                >
                  <span style={{ display: 'flex', alignItems: 'center', gap: 6, overflow: 'hidden' }}>
                    <HolderOutlined style={{ color: '#bbb' }} />
                    <Text ellipsis style={{ maxWidth: 210 }}>{labelOf[k] || k}</Text>
                  </span>
                  <CloseOutlined style={{ color: '#999', cursor: 'pointer' }} onClick={() => removeKey(k)} />
                </div>
              ))
            )}
          </div>
        </div>
      </div>

      {/* 底：保存为常用 */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginTop: 12 }}>
        <Switch size="small" checked={saveOn} onChange={setSaveOn} />
        <Text>保存为常用方案</Text>
        <Input
          placeholder="方案名称"
          disabled={!saveOn}
          maxLength={10}
          value={presetName}
          onChange={(e) => setPresetName(e.target.value)}
          style={{ width: 200 }}
          suffix={<Text type="secondary">{presetName.length}/10</Text>}
        />
      </div>
    </Modal>
  );
};

export default CfMetricPicker;
