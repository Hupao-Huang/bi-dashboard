import React, { useState, useEffect, useCallback } from 'react';
import { Modal, Table, Tag, InputNumber, Button, Space, Badge, Tabs, Alert, message } from 'antd';
import { TableOutlined } from '@ant-design/icons';
import { API_BASE } from '../config';
import { useAuth } from '../auth/AuthContext';

// 特殊渠道(京东/猫超/朴朴)价格表维护 —— 右上角「价格表」按钮, 点开弹窗按渠道分 Tab 填价/改价。
// 保存需二次确认; 后端 save-price 自动: 存价 → 重算该商品已发调拨单金额 → 清看板缓存 → 记操作日志。
// 同一货品在不同渠道可填不同价(价格表 UK = 渠道+商品), 各渠道分开互不影响。
// 有"改价"权限(ecommerce.special_channel_allot:edit, 默认仅超管)才显示输入框, 否则弹窗只读。

const EDIT_PERM = 'ecommerce.special_channel_allot:edit';

export interface MissingRow {
  channelKey: string;
  goodsNo: string;
  barcode: string;
  goodsName: string;
  allocateCnt: number;
  qtyTotal: number;
}

interface PriceRow {
  channelKey: string;
  goodsNo: string;
  barcode: string;
  goodsName: string;
  price: number;
  source: string;
  updatedAt: string;
}

// 弹窗表格一行: 缺价(price=null) 或 已配价
interface SkuRow {
  channelKey: string;
  goodsNo: string;
  barcode: string;
  goodsName: string;
  price: number | null;
  qtyTotal?: number;
  updatedAt?: string;
}

interface Props {
  dept: 'ecommerce' | 'instant_retail';
  missing: MissingRow[];
  onSaved: () => void; // 父页刷新 KPI / 单据列表
}

const DEPT_CHANNELS: Record<string, string[]> = {
  ecommerce: ['京东', '猫超'],
  instant_retail: ['朴朴', '小象', '叮咚'], // 2026-06-05 加小象/叮咚 (跟后端 priceChannelsByDept 一致)
};

const keyOf = (r: { channelKey: string; goodsNo: string }) => r.channelKey + '|' + r.goodsNo;

const SpecialChannelPriceManager: React.FC<Props> = ({ dept, missing, onSaved }) => {
  const { hasPermission } = useAuth();
  const canEdit = hasPermission(EDIT_PERM);
  const channels = DEPT_CHANNELS[dept] || ['京东', '猫超', '朴朴'];

  const [open, setOpen] = useState(false);
  const [prices, setPrices] = useState<PriceRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState<Record<string, number | null>>({});
  const [savingKey, setSavingKey] = useState<string>('');
  const [activeCh, setActiveCh] = useState<string>(channels[0]);

  const fetchPrices = useCallback(() => {
    setLoading(true);
    fetch(`${API_BASE}/api/special-channel-allot/prices?dept=${dept}`, { credentials: 'include' })
      .then(r => r.json())
      .then(j => { if (j.code === 200) setPrices(j.data?.list || []); })
      .catch(() => { /* 列表拉取失败不打断 */ })
      .finally(() => setLoading(false));
  }, [dept]);

  useEffect(() => { if (open) fetchPrices(); }, [open, fetchPrices]);

  // 某渠道的行 = 缺价(missing, 在前) + 已配价(prices)
  const rowsOf = (ch: string): SkuRow[] => {
    const miss: SkuRow[] = missing.filter(m => m.channelKey === ch).map(m => ({
      channelKey: m.channelKey, goodsNo: m.goodsNo, barcode: m.barcode, goodsName: m.goodsName,
      price: null, qtyTotal: m.qtyTotal,
    }));
    const priced: SkuRow[] = prices.filter(p => p.channelKey === ch).map(p => ({
      channelKey: p.channelKey, goodsNo: p.goodsNo, barcode: p.barcode, goodsName: p.goodsName,
      price: p.price, updatedAt: p.updatedAt,
    }));
    return [...miss, ...priced];
  };

  const doSave = (row: SkuRow, priceVal: number) => {
    const k = keyOf(row);
    setSavingKey(k);
    fetch(`${API_BASE}/api/special-channel-allot/save-price`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        channelKey: row.channelKey, goodsNo: row.goodsNo,
        barcode: row.barcode || '', goodsName: row.goodsName || '', price: priceVal,
      }),
    })
      .then(r => r.json())
      .then(j => {
        if (j.code === 200) {
          message.success(`已保存:[${row.channelKey}] ${row.goodsName || row.goodsNo} 单价 ${priceVal},补算了 ${j.data?.updatedRows ?? 0} 条调拨明细`);
          setInputs(prev => { const n = { ...prev }; delete n[k]; return n; });
          fetchPrices();
          onSaved();
        } else {
          message.error(j.msg || '保存失败');
        }
      })
      .catch(err => message.error(`保存失败:${err instanceof Error ? err.message : String(err)}`))
      .finally(() => setSavingKey(''));
  };

  // 二次确认后再保存
  const onSaveClick = (row: SkuRow) => {
    const k = keyOf(row);
    const v = inputs[k];
    if (!v || v <= 0) { message.warning('请先填一个大于 0 的单价'); return; }
    Modal.confirm({
      title: '确认保存价格',
      content: `把 [${row.channelKey}] ${row.goodsName || row.goodsNo} 单价设为 ¥${v}?保存后这商品已发的调拨单销售额会立即按新价重算,综合 / 部门 / 计划看板也跟着更新。`,
      okText: '确定保存',
      cancelText: '再想想',
      onOk: () => doSave(row, v),
    });
  };

  const columns = [
    { title: '商品编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 120 },
    { title: '名称', dataIndex: 'goodsName', key: 'goodsName', ellipsis: true },
    {
      title: '状态', key: 'status', width: 80,
      render: (_: unknown, r: SkuRow) => r.price == null ? <Tag color="red">缺价</Tag> : <Tag color="green">已配</Tag>,
    },
    {
      title: '当前单价', dataIndex: 'price', key: 'price', width: 90, align: 'right' as const,
      render: (p: number | null) => p == null ? <span style={{ color: '#bbb' }}>—</span> : p.toFixed(2),
    },
    ...(canEdit ? [{
      title: '新单价 / 操作', key: 'op', width: 200, align: 'right' as const,
      render: (_: unknown, r: SkuRow) => {
        const k = keyOf(r);
        return (
          <Space>
            <InputNumber
              size="small" min={0.01} max={100000} step={0.1}
              placeholder={r.price != null ? r.price.toFixed(2) : '单价'}
              value={inputs[k]}
              onChange={(v) => setInputs(prev => ({ ...prev, [k]: v }))}
              style={{ width: 90 }}
            />
            <Button size="small" type={r.price == null ? 'primary' : 'default'} loading={savingKey === k} onClick={() => onSaveClick(r)}>
              保存
            </Button>
          </Space>
        );
      },
    }] : []),
  ];

  return (
    <>
      <Badge count={missing.length} size="small" offset={[-2, 2]}>
        <Button icon={<TableOutlined />} onClick={() => setOpen(true)}>价格表</Button>
      </Badge>
      <Modal
        title="特殊渠道价格表维护"
        open={open}
        onCancel={() => setOpen(false)}
        footer={null}
        width={900}
      >
        <Alert
          type={canEdit ? 'info' : 'warning'}
          showIcon
          style={{ marginBottom: 12 }}
          message={canEdit
            ? `同一个货品在各渠道(${channels.join(' / ')})可以填不同的价,各渠道分开互不影响。填上单价点保存(需二次确认),这商品已发的调拨单销售额会当场重算,综合 / 部门 / 计划看板也跟着更新。`
            : '只读:你没有改价权限。如需改价请联系管理员。'}
        />
        <Tabs
          activeKey={activeCh}
          onChange={setActiveCh}
          items={channels.map(ch => {
            const cnt = missing.filter(m => m.channelKey === ch).length;
            return {
              key: ch,
              label: cnt > 0 ? <Badge count={cnt} size="small" offset={[10, 0]}>{ch}</Badge> : ch,
              children: (
                <Table
                  dataSource={rowsOf(ch)}
                  columns={columns}
                  rowKey={keyOf}
                  size="small"
                  loading={loading}
                  pagination={false}
                  scroll={{ y: 440 }}
                />
              ),
            };
          })}
        />
      </Modal>
    </>
  );
};

export default SpecialChannelPriceManager;
