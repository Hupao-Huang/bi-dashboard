// v1.59.0 个人中心 → 合思机器人 Tab
// MVP: 只读"我的待审批" 单据列表. 后续 v1.60.0 加规则编辑, v1.61.0 dry run, v1.62.0 真自动审批.

import React, { useCallback, useEffect, useState } from 'react';
import { Alert, Button, Card, Empty, Statistic, Table, Tag, Tooltip, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ReloadOutlined, RobotOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { API_BASE } from '../../config';

interface PendingItem {
  flowId: string;
  code: string;
  title: string;
  formType: string;
  state: string;
  stageName: string | null;
  approverName: string | null;
  payMoney: number | null;
  expenseMoney: number | null;
  loanMoney: number | null;
  submitDate: number | null;
  submitterId: string | null;
  departmentId: string | null;
}

const formTypeMap: Record<string, { label: string; color: string }> = {
  expense: { label: '报销单', color: 'blue' },
  loan: { label: '借款单', color: 'orange' },
  requisition: { label: '申请单', color: 'green' },
  custom: { label: '通用审批', color: 'purple' },
  payment: { label: '付款单', color: 'red' },
  receipt: { label: '收款单', color: 'cyan' },
};

const stateMap: Record<string, { label: string; color: string }> = {
  approving: { label: '审批中', color: 'processing' },
  paying: { label: '待支付', color: 'warning' },
  pending: { label: '提交中', color: 'processing' },
};

const HesiBot: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [realName, setRealName] = useState('');
  const [warning, setWarning] = useState('');
  const [items, setItems] = useState<PendingItem[]>([]);

  const fetchPending = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/profile/hesi-pending`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200 && json.data) {
        setRealName(json.data.realName || '');
        setItems(json.data.items || []);
        setWarning(json.data.warning || '');
      } else {
        message.error(json.msg || '加载失败');
      }
    } catch (e) {
      message.error('网络错误');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchPending(); }, [fetchPending]);

  const getMoney = (item: PendingItem) => {
    if (item.payMoney && item.payMoney > 0) return item.payMoney;
    if (item.expenseMoney && item.expenseMoney > 0) return item.expenseMoney;
    if (item.loanMoney && item.loanMoney > 0) return item.loanMoney;
    return null;
  };

  const totalAmount = items.reduce((sum, item) => sum + (getMoney(item) || 0), 0);

  const columns: ColumnsType<PendingItem> = [
    { title: '单据编码', dataIndex: 'code', width: 130, fixed: 'left' },
    {
      title: '标题', dataIndex: 'title', ellipsis: true,
      render: (title) => <Tooltip title={title}>{title}</Tooltip>,
    },
    {
      title: '类型', dataIndex: 'formType', width: 90,
      render: (v) => {
        const m = formTypeMap[v];
        return m ? <Tag color={m.color}>{m.label}</Tag> : v;
      },
    },
    {
      title: '状态', dataIndex: 'state', width: 90,
      render: (v) => {
        const m = stateMap[v];
        return m ? <Tag color={m.color}>{m.label}</Tag> : v;
      },
    },
    { title: '当前节点', dataIndex: 'stageName', width: 130 },
    {
      title: '金额', width: 120, align: 'right',
      render: (_, record) => {
        const money = getMoney(record);
        return money ? `¥${money.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}` : '-';
      },
    },
    {
      title: '提交日期', dataIndex: 'submitDate', width: 110,
      render: (v) => v ? dayjs(Number(v)).format('YYYY-MM-DD') : '-',
    },
  ];

  return (
    <div>
      {/* 顶部说明 + 后续路线 */}
      <Alert
        type="info"
        showIcon
        icon={<RobotOutlined />}
        message="合思机器人 (v1.59 MVP 只读模式)"
        description={
          <div>
            <div>当前显示<strong>等你审批</strong>的合思单据 (按你的真实姓名"{realName || '未设置'}"模糊匹配)。</div>
            <div style={{ marginTop: 6, color: '#666', fontSize: 12 }}>
              路线图: v1.60 加规则编辑器 → v1.61 干跑模式(匹配但不审批, 看效果) → v1.62 真自动审批(需财务/合规批准).
            </div>
          </div>
        }
        style={{ marginBottom: 16 }}
      />

      {warning && (
        <Alert type="warning" showIcon message={warning} style={{ marginBottom: 16 }} />
      )}

      {/* 统计 */}
      {items.length > 0 && (
        <Card size="small" style={{ marginBottom: 16 }}>
          <div style={{ display: 'flex', gap: 32, alignItems: 'center' }}>
            <Statistic title="等我审批" value={items.length} suffix="单" />
            <Statistic title="涉及金额合计" value={totalAmount} precision={2} prefix="¥" />
            <Button icon={<ReloadOutlined />} onClick={fetchPending} loading={loading} style={{ marginLeft: 'auto' }}>
              刷新
            </Button>
          </div>
        </Card>
      )}

      {/* 表格 */}
      <Card>
        {items.length === 0 && !loading ? (
          <Empty
            description={
              realName
                ? `没有等你(${realName})审批的合思单据 ✓`
                : '请先到"个人信息" Tab 设置真实姓名'
            }
          />
        ) : (
          <Table<PendingItem>
            columns={columns}
            dataSource={items}
            rowKey="flowId"
            loading={loading}
            pagination={{ pageSize: 20, showSizeChanger: false }}
            size="middle"
            scroll={{ x: 800 }}
          />
        )}
      </Card>
    </div>
  );
};

export default HesiBot;
