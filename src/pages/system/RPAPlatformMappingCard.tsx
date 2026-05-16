// 影刀任务映射管理: 11 个 BI 平台 → 影刀子应用的对应关系
//
// 跑哥可视化改: 下拉选影刀子应用, 改启用状态, 改备注
// 数据来源: /api/admin/rpa/platform-mapping (映射表) + /api/admin/yingdao/tasks (影刀任务列表, 5min 缓存)
import React, { useEffect, useState, useCallback } from 'react';
import { Card, Table, Select, Input, Switch, Button, Tag, Space, message, Tooltip, Typography } from 'antd';
import { SaveOutlined, ReloadOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';

const { Text } = Typography;

interface PlatformMapping {
  platform: string;
  robot_uuid: string;
  robot_name: string;
  account_name: string;
  enabled: number;
  remark: string;
  updated_at: string;
}

interface YingDaoSubApp {
  robotUuid: string;
  robotName: string;
}

interface YingDaoClient {
  robotClientUuid: string;
  robotClientName: string;
  status: string; // idle / running / offline
  clientIp: string;
  machineName: string;
}

// 状态显示映射 (跟影刀控制台机器人管理一致)
const CLIENT_STATUS_LABEL: Record<string, { label: string; color: string }> = {
  idle:    { label: '空闲', color: 'green'  },
  running: { label: '运行中', color: 'gold' },
  offline: { label: '离线', color: 'default' },
};

const RPAPlatformMappingCard: React.FC = () => {
  const [mappings, setMappings] = useState<PlatformMapping[]>([]);
  const [yingdaoApps, setYingdaoApps] = useState<YingDaoSubApp[]>([]);
  const [yingdaoClients, setYingdaoClients] = useState<YingDaoClient[]>([]);
  const [loading, setLoading] = useState(false);
  const [edited, setEdited] = useState<Record<string, Partial<PlatformMapping>>>({});
  const [savingPlat, setSavingPlat] = useState<string>('');

  // 拉映射 + 拉影刀任务列表 + 拉影刀机器人列表 (3 个并行)
  const fetchAll = useCallback(async () => {
    setLoading(true);
    try {
      const [mapRes, appsRes, clientsRes] = await Promise.all([
        fetch(`${API_BASE}/api/admin/rpa/platform-mapping`, { credentials: 'include' }).then(r => r.json()),
        fetchYingDaoApps(),
        fetchYingDaoClients(),
      ]);
      if (mapRes.code === 200) setMappings(mapRes.data || []);
      setYingdaoApps(appsRes);
      setYingdaoClients(clientsRes);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchAll(); }, [fetchAll]);

  const handleEdit = (platform: string, field: keyof PlatformMapping, value: any) => {
    setEdited(prev => ({
      ...prev,
      [platform]: { ...(prev[platform] || {}), [field]: value },
    }));
  };

  const handleSave = async (row: PlatformMapping) => {
    const merged = { ...row, ...(edited[row.platform] || {}) };
    setSavingPlat(row.platform);
    try {
      const res = await fetch(`${API_BASE}/api/admin/rpa/platform-mapping/update`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(merged),
      });
      const j = await res.json();
      if (j.code === 200) {
        message.success(`已保存 ${row.platform}`);
        setEdited(prev => { const next = { ...prev }; delete next[row.platform]; return next; });
        fetchAll();
      } else {
        message.error(j.msg || '保存失败');
      }
    } finally {
      setSavingPlat('');
    }
  };

  const columns = [
    {
      title: '平台',
      dataIndex: 'platform',
      width: 110,
      render: (v: string) => <Tag color="blue">{v}</Tag>,
    },
    {
      title: '影刀子应用',
      key: 'robot_uuid',
      width: 320,
      render: (_: any, row: PlatformMapping) => {
        const value = edited[row.platform]?.robot_uuid ?? row.robot_uuid;
        return (
          <Select
            value={value}
            style={{ width: '100%' }}
            showSearch
            optionFilterProp="label"
            placeholder="选择影刀子应用"
            options={yingdaoApps.map(a => ({ value: a.robotUuid, label: a.robotName }))}
            onChange={v => {
              const app = yingdaoApps.find(a => a.robotUuid === v);
              handleEdit(row.platform, 'robot_uuid', v);
              if (app) handleEdit(row.platform, 'robot_name', app.robotName);
            }}
          />
        );
      },
    },
    {
      title: '机器人账号',
      key: 'account_name',
      width: 280,
      render: (_: any, row: PlatformMapping) => {
        const value = edited[row.platform]?.account_name ?? row.account_name;
        // 排序: 空闲优先, 运行中其次, 离线最后
        const sortedClients = [...yingdaoClients].sort((a, b) => {
          const order: Record<string, number> = { idle: 0, running: 1, offline: 2 };
          return (order[a.status] ?? 9) - (order[b.status] ?? 9);
        });
        return (
          <Select
            value={value || undefined}
            style={{ width: '100%' }}
            showSearch
            allowClear
            optionFilterProp="label"
            placeholder="选机器人账号"
            options={sortedClients.map(c => {
              const meta = CLIENT_STATUS_LABEL[c.status] || { label: c.status, color: 'default' };
              return {
                value: c.robotClientName,
                // label 用于搜索匹配
                label: `${c.robotClientName} ${meta.label} ${c.machineName}`,
                // 自定义渲染
                clientData: c,
                statusMeta: meta,
              };
            })}
            optionRender={(opt) => {
              const c = (opt.data as any).clientData as YingDaoClient;
              const meta = (opt.data as any).statusMeta as { label: string; color: string };
              return (
                <Space size={6}>
                  <Tag color={meta.color} style={{ marginRight: 0, minWidth: 50, textAlign: 'center' }}>{meta.label}</Tag>
                  <span>{c.robotClientName}</span>
                  <Text type="secondary" style={{ fontSize: 12 }}>{c.machineName}</Text>
                </Space>
              );
            }}
            onChange={v => handleEdit(row.platform, 'account_name', v || '')}
          />
        );
      },
    },
    {
      title: '启用',
      key: 'enabled',
      width: 80,
      align: 'center' as const,
      render: (_: any, row: PlatformMapping) => {
        const value = edited[row.platform]?.enabled ?? row.enabled;
        return (
          <Switch
            checked={value === 1}
            onChange={c => handleEdit(row.platform, 'enabled', c ? 1 : 0)}
          />
        );
      },
    },
    {
      title: '备注',
      key: 'remark',
      render: (_: any, row: PlatformMapping) => {
        const value = edited[row.platform]?.remark ?? row.remark;
        return (
          <Input
            value={value}
            placeholder="可选 (如临时换流程的说明)"
            onChange={e => handleEdit(row.platform, 'remark', e.target.value)}
          />
        );
      },
    },
    {
      title: '更新时间',
      dataIndex: 'updated_at',
      width: 150,
      render: (v: string) => <Text type="secondary" style={{ fontSize: 12 }}>{v}</Text>,
    },
    {
      title: '操作',
      key: 'op',
      width: 90,
      align: 'center' as const,
      render: (_: any, row: PlatformMapping) => {
        const dirty = !!edited[row.platform];
        return (
          <Button
            size="small"
            type={dirty ? 'primary' : 'default'}
            icon={<SaveOutlined />}
            disabled={!dirty}
            loading={savingPlat === row.platform}
            onClick={() => handleSave(row)}
          >
            保存
          </Button>
        );
      },
    },
  ];

  return (
    <Card
      title={
        <Space>
          <span>影刀任务映射</span>
          <Tag color="purple">RPA 监控页"立即同步本平台"按钮的来源</Tag>
        </Space>
      }
      extra={
        <Space size={8}>
          {yingdaoClients.length > 0 && (
            <Space size={4}>
              {(['idle', 'running', 'offline'] as const).map(s => {
                const cnt = yingdaoClients.filter(c => c.status === s).length;
                const meta = CLIENT_STATUS_LABEL[s];
                return cnt > 0 ? (
                  <Tag key={s} color={meta.color}>{meta.label} {cnt}</Tag>
                ) : null;
              })}
            </Space>
          )}
          <Tooltip title="重新拉取影刀任务+机器人 (绕过 5 分钟缓存)">
            <Button size="small" icon={<ReloadOutlined />} onClick={async () => {
              await Promise.all([
                fetch(`${API_BASE}/api/admin/yingdao/sub-apps?refresh=1`, { credentials: 'include' }),
                fetch(`${API_BASE}/api/admin/yingdao/clients?refresh=1`, { credentials: 'include' }),
              ]);
              fetchAll();
            }}>
              刷新
            </Button>
          </Tooltip>
        </Space>
      }
      style={{ marginBottom: 16 }}
    >
      <Table
        dataSource={mappings}
        columns={columns}
        rowKey="platform"
        size="small"
        pagination={false}
        loading={loading}
      />
      <div style={{ marginTop: 8 }}>
        <Text type="secondary" style={{ fontSize: 12 }}>
          💡 改完点"保存"按钮才生效。一个 BI 平台对应 1 个影刀子应用，触发同步会让对应应用跑一遍。
        </Text>
      </div>
    </Card>
  );
};

// 拉"集团数据看板"任务下的子应用 (12 个: 唯品会/天猫/.../同步webhook)
// 后端 /api/admin/yingdao/sub-apps 默认返回这个 schedule 下的 robotList
async function fetchYingDaoApps(): Promise<YingDaoSubApp[]> {
  try {
    const res = await fetch(`${API_BASE}/api/admin/yingdao/sub-apps`, { credentials: 'include' });
    const j = await res.json();
    if (j.code !== 200) return [];
    const apps = (j.data || []) as Array<{ robotUuid: string; robotName: string }>;
    return apps.filter(a => a.robotUuid && a.robotName);
  } catch {
    return [];
  }
}

// 拉影刀全量机器人列表 (含 idle/running/offline 实时状态)
async function fetchYingDaoClients(): Promise<YingDaoClient[]> {
  try {
    const res = await fetch(`${API_BASE}/api/admin/yingdao/clients`, { credentials: 'include' });
    const j = await res.json();
    if (j.code !== 200) return [];
    return (j.data || []) as YingDaoClient[];
  } catch {
    return [];
  }
}

export default RPAPlatformMappingCard;
