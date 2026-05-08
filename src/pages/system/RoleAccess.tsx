import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Checkbox,
  Col,
  Form,
  Input,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import { DeleteOutlined, PlusOutlined, TeamOutlined, BookOutlined, ToolOutlined, UserOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';

const ROLE_HEX_MAP: Record<string, string> = {
  super_admin: '#cf1322',
  management: '#d48806',
  dept_manager: '#08979c',
  operator: '#1677ff',
  finance: '#722ed1',
  customer_service: '#389e0d',
  ecommerce_manager: '#0958d9',
  social_manager: '#c41d7f',
  offline_manager: '#d4380d',
  distribution_manager: '#7cb305',
  procurement: '#d4380d',
};

const roleHex = (code: string): string => ROLE_HEX_MAP[code] || '#64748b';

type MetaOption = {
  label: string;
  value: string;
};

type PermissionOption = {
  code: string;
  name: string;
  type: string;
};

type RoleItem = {
  builtin: boolean;
  code: string;
  description: string;
  id: number;
  name: string;
  permissionCount: number;
  userCount: number;
};

type RoleDetail = {
  builtin: boolean;
  code: string;
  dataScopes: {
    depts: string[];
    domains: string[];
    platforms: string[];
    shops: string[];
    warehouses: string[];
  };
  description: string;
  id: number;
  name: string;
  permissions: string[];
};

type MetaData = {
  depts: MetaOption[];
  domains: MetaOption[];
  permissions: PermissionOption[];
  platforms: MetaOption[];
  shops: MetaOption[];
  warehouses: MetaOption[];
};

type RoleFormValues = {
  depts: string[];
  description: string;
  domains: string[];
  name: string;
  permissions: string[];
  platforms: string[];
  shops: string[];
  warehouses: string[];
};

type PermissionGroup = {
  key: string;
  label: string;
  options: PermissionOption[];
};

const parseData = async <T,>(response: Response): Promise<T> => {
  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.msg || '请求失败');
  }
  return body.data as T;
};

const moduleGroupConfig: Record<string, { label: string; order: number }> = {
  overview: { label: '综合看板', order: 1 },
  brand: { label: '品牌中心', order: 2 },
  ecommerce: { label: '电商', order: 3 },
  social: { label: '社媒', order: 4 },
  offline: { label: '线下', order: 5 },
  distribution: { label: '分销', order: 6 },
  instant_retail: { label: '即时零售', order: 7 },
  finance: { label: '财务', order: 8 },
  customer: { label: '客服', order: 9 },
  supply_chain: { label: '供应链', order: 10 },
  action: { label: '系统动作', order: 11 },
  unclassified: { label: '未分类权限', order: 12 },
};

const getPermissionGroupKey = (permission: PermissionOption): string => {
  if (permission.type === 'field') return 'field';
  const code = permission.code || '';
  const dotIndex = code.indexOf('.');
  const colonIndex = code.indexOf(':');
  const splitIndex = dotIndex >= 0
    ? dotIndex
    : colonIndex;
  const prefix = splitIndex >= 0 ? code.slice(0, splitIndex) : code;
  if (moduleGroupConfig[prefix]) return prefix;
  if (permission.type === 'action') return 'action';
  return 'unclassified';
};

const togglePermission = (current: string[], code: string, checked: boolean) => {
  if (checked) {
    return Array.from(new Set([...current, code])).sort();
  }
  return current.filter(item => item !== code);
};

const GROUP_HEX_MAP: Record<string, string> = {
  overview: '#1e40af',
  brand: '#0891b2',
  ecommerce: '#0958d9',
  social: '#c41d7f',
  offline: '#16a34a',
  distribution: '#7cb305',
  instant_retail: '#0891b2',
  finance: '#722ed1',
  customer: '#389e0d',
  supply_chain: '#d48806',
  action: '#64748b',
  field: '#cf1322',
  unclassified: '#94a3b8',
};

const PermissionMatrix: React.FC<{
  groups: PermissionGroup[];
  onChange?: (value: string[]) => void;
  readOnly?: boolean;
  value?: string[];
}> = ({ groups, onChange, readOnly = false, value = [] }) => {
  const selected = useMemo(() => new Set(value), [value]);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      {groups.map(group => {
        const accent = GROUP_HEX_MAP[group.key] || '#64748b';
        const checkedInGroup = group.options.filter(o => selected.has(o.code)).length;
        return (
          <div key={group.key} style={{
            paddingLeft: 12,
            borderLeft: `3px solid ${accent}`,
          }}>
            <div style={{
              display: 'flex',
              alignItems: 'baseline',
              gap: 8,
              marginBottom: 8,
            }}>
              <span style={{ fontSize: 13, fontWeight: 600, color: '#0f172a', letterSpacing: '-0.01em' }}>
                {group.label}
              </span>
              <span style={{
                fontSize: 11,
                color: accent,
                background: `${accent}10`,
                border: `1px solid ${accent}30`,
                padding: '0 6px',
                borderRadius: 3,
                fontVariantNumeric: 'tabular-nums',
                fontWeight: 500,
              }}>
                {checkedInGroup}/{group.options.length}
              </span>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 6 }}>
              {group.options.map(option => (
                <Checkbox
                  key={option.code}
                  checked={selected.has(option.code)}
                  disabled={readOnly}
                  onChange={event => onChange?.(togglePermission(value, option.code, event.target.checked))}
                >
                  {option.name}
                </Checkbox>
              ))}
            </div>
          </div>
        );
      })}
    </div>
  );
};

const RoleAccessPage: React.FC = () => {
  const [messageApi, contextHolder] = message.useMessage();
  const [meta, setMeta] = useState<MetaData | null>(null);
  const [roles, setRoles] = useState<RoleItem[]>([]);
  const [selectedRoleId, setSelectedRoleId] = useState<number | null>(null);
  const [roleDetail, setRoleDetail] = useState<RoleDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm<RoleFormValues>();

  const loadMeta = useCallback(async () => {
    const data = await parseData<MetaData>(await fetch(`${API_BASE}/api/admin/meta`));
    setMeta(data);
  }, []);

  const loadRoles = useCallback(async () => {
    const data = await parseData<{ list: RoleItem[] }>(await fetch(`${API_BASE}/api/admin/roles`));
    setRoles(data.list);
    return data.list;
  }, []);

  const loadRoleDetail = useCallback(async (roleId: number) => {
    const data = await parseData<RoleDetail>(await fetch(`${API_BASE}/api/admin/roles/${roleId}`));
    setRoleDetail(data);
    form.setFieldsValue({
      depts: data.dataScopes.depts,
      description: data.description,
      domains: data.dataScopes.domains,
      name: data.name,
      permissions: data.permissions,
      platforms: data.dataScopes.platforms,
      shops: data.dataScopes.shops,
      warehouses: data.dataScopes.warehouses,
    });
  }, [form]);

  useEffect(() => {
    (async () => {
      try {
        await loadMeta();
        const loadedRoles = await loadRoles();
        if (loadedRoles.length > 0) {
          const firstId = loadedRoles[0].id;
          setSelectedRoleId(firstId);
          await loadRoleDetail(firstId);
        }
      } catch (error) {
        messageApi.error(error instanceof Error ? error.message : '加载失败');
      } finally {
        setLoading(false);
      }
    })();
  }, [loadMeta, loadRoleDetail, loadRoles, messageApi]);

  const handleSelectRole = async (role: RoleItem) => {
    setSelectedRoleId(role.id);
    try {
      await loadRoleDetail(role.id);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '加载角色失败');
    }
  };

  const handleSave = async (values: RoleFormValues) => {
    if (!selectedRoleId) return;
    setSaving(true);
    try {
      const saved = await parseData<RoleDetail>(
        await fetch(`${API_BASE}/api/admin/roles/${selectedRoleId}`, {
          body: JSON.stringify({
            dataScopes: {
              depts: values.depts || [],
              domains: values.domains || [],
              platforms: values.platforms || [],
              shops: values.shops || [],
              warehouses: values.warehouses || [],
            },
            description: values.description || '',
            name: values.name,
            permissions: values.permissions || [],
          }),
          headers: { 'Content-Type': 'application/json' },
          method: 'PUT',
        }),
      );
      setRoleDetail(saved);
      await loadRoles();
      messageApi.success('角色配置已保存');
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '保存失败');
    } finally {
      setSaving(false);
    }
  };

  const [createOpen, setCreateOpen] = useState(false);
  const [createForm] = Form.useForm();
  const [creating, setCreating] = useState(false);

  const handleCreate = async () => {
    try {
      const values = await createForm.validateFields();
      setCreating(true);
      const res = await fetch(`${API_BASE}/api/admin/roles`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(values),
      });
      if (!res.ok) {
        const body = await res.json().catch(err => { console.warn('RoleAccess json:', err); return {}; });
        throw new Error(body.msg || `HTTP ${res.status}`);
      }
      messageApi.success('角色创建成功');
      setCreateOpen(false);
      createForm.resetFields();
      const loadedRoles = await loadRoles();
      if (loadedRoles.length > 0) {
        const last = loadedRoles[loadedRoles.length - 1];
        setSelectedRoleId(last.id);
        await loadRoleDetail(last.id);
      }
    } catch (error) {
      if (error instanceof Error && error.message !== 'Validation failed') {
        messageApi.error(error.message);
      }
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async (roleId: number) => {
    try {
      const res = await fetch(`${API_BASE}/api/admin/roles/${roleId}`, {
        method: 'DELETE',
        credentials: 'include',
      });
      if (!res.ok) {
        const body = await res.json().catch(err => { console.warn('RoleAccess json:', err); return {}; });
        throw new Error(body.msg || `HTTP ${res.status}`);
      }
      messageApi.success('角色已删除');
      const loadedRoles = await loadRoles();
      if (selectedRoleId === roleId) {
        if (loadedRoles.length > 0) {
          setSelectedRoleId(loadedRoles[0].id);
          await loadRoleDetail(loadedRoles[0].id);
        } else {
          setSelectedRoleId(null);
          setRoleDetail(null);
        }
      }
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '删除失败');
    }
  };

  const permissionLayers = useMemo(() => {
    const moduleGrouped = new Map<string, PermissionGroup>();
    const fieldOptions: PermissionOption[] = [];

    (meta?.permissions || []).forEach(permission => {
      const key = getPermissionGroupKey(permission);
      if (key === 'field') {
        fieldOptions.push(permission);
        return;
      }

      const config = moduleGroupConfig[key] || moduleGroupConfig.unclassified;
      const group = moduleGrouped.get(key) || { key, label: config.label, options: [] };
      group.options.push(permission);
      moduleGrouped.set(key, group);
    });

    const moduleGroups = Array.from(moduleGrouped.values())
      .map(group => ({
        ...group,
        options: group.options.sort((a, b) => a.code.localeCompare(b.code, 'zh-CN')),
      }))
      .sort((a, b) => (moduleGroupConfig[a.key]?.order || 99) - (moduleGroupConfig[b.key]?.order || 99));

    const fieldGroups: PermissionGroup[] = fieldOptions.length > 0
      ? [{
          key: 'field',
          label: '字段权限',
          options: fieldOptions.sort((a, b) => a.code.localeCompare(b.code, 'zh-CN')),
        }]
      : [];

    return { moduleGroups, fieldGroups };
  }, [meta]);

  const watchedPermissions = Form.useWatch('permissions', form) || [];
  const handlePermissionsChange = useCallback((nextPermissions: string[]) => {
    form.setFieldsValue({ permissions: nextPermissions });
  }, [form]);

  const currentRole = roles.find(role => role.id === selectedRoleId) || null;
  const isReadOnly = roleDetail?.code === 'super_admin';

  const roleColumns = [
    {
      title: '角色',
      dataIndex: 'name',
      key: 'name',
      render: (_: string, record: RoleItem) => {
        const accent = roleHex(record.code);
        const isSelected = selectedRoleId === record.id;
        return (
          <div style={{ display: 'flex', alignItems: 'stretch', gap: 10, minHeight: 38 }}>
            <div style={{
              width: isSelected ? 4 : 3,
              background: accent,
              borderRadius: 2,
              flexShrink: 0,
              alignSelf: 'stretch',
              transition: 'width 120ms ease',
            }} />
            <div>
              <Typography.Text strong style={{ color: isSelected ? accent : '#0f172a' }}>
                {record.name}
              </Typography.Text>
              <Typography.Text type="secondary" style={{ display: 'block', fontSize: 12, fontVariantNumeric: 'tabular-nums' }}>
                {record.code}
              </Typography.Text>
            </div>
          </div>
        );
      },
    },
    {
      title: '类型',
      dataIndex: 'builtin',
      key: 'builtin',
      width: 84,
      responsive: ['xl' as const],
      render: (builtin: boolean) => (
        <Tag color={builtin ? 'blue' : 'default'} style={{ marginInlineEnd: 0 }}>
          {builtin ? '内置' : '自定义'}
        </Tag>
      ),
    },
    {
      title: '授权',
      key: 'stats',
      width: 130,
      responsive: ['xxl' as const],
      render: (_: unknown, record: RoleItem) => (
        <Typography.Text type="secondary" style={{ fontSize: 12, fontVariantNumeric: 'tabular-nums' }}>
          {record.permissionCount} 权限 · {record.userCount} 用户
        </Typography.Text>
      ),
    },
    {
      title: '',
      key: 'action',
      width: 40,
      render: (_: unknown, record: RoleItem) => (
        record.builtin ? null : (
          <Popconfirm
            title="确定删除该角色？"
            onConfirm={(e) => { e?.stopPropagation(); handleDelete(record.id); }}
            onCancel={(e) => e?.stopPropagation()}
            okText="删除"
            cancelText="取消"
          >
            <Button
              type="text"
              size="small"
              danger
              icon={<DeleteOutlined />}
              onClick={(e) => e.stopPropagation()}
            />
          </Popconfirm>
        )
      ),
    },
  ];

  const stats = useMemo(() => ({
    total: roles.length,
    builtin: roles.filter(r => r.builtin).length,
    custom: roles.filter(r => !r.builtin).length,
    inUse: roles.filter(r => r.userCount > 0).length,
  }), [roles]);

  return (
    <div>
      {contextHolder}

      <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
        <Col xs={12} sm={6}>
          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: '#1e40af' }} bodyStyle={{ padding: 16 }}>
            <Statistic title={<><TeamOutlined style={{ marginRight: 6 }} />总角色</>} value={stats.total} valueStyle={{ color: '#1e40af', fontSize: 22 }} />
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: '#1677ff' }} bodyStyle={{ padding: 16 }}>
            <Statistic title={<><BookOutlined style={{ marginRight: 6, color: '#1677ff' }} />内置角色</>} value={stats.builtin} valueStyle={{ color: '#1677ff', fontSize: 22 }} />
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: '#16a34a' }} bodyStyle={{ padding: 16 }}>
            <Statistic title={<><ToolOutlined style={{ marginRight: 6, color: '#16a34a' }} />自定义角色</>} value={stats.custom} valueStyle={{ color: '#16a34a', fontSize: 22 }} />
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: '#722ed1' }} bodyStyle={{ padding: 16 }}>
            <Statistic title={<><UserOutlined style={{ marginRight: 6, color: '#722ed1' }} />使用中</>} value={stats.inUse} valueStyle={{ color: '#722ed1', fontSize: 22 }} />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]}>
        <Col xs={24} xl={10} xxl={9}>
          <Card className="bi-card" title="角色列表" styles={{ body: { maxHeight: 'calc(100vh - 280px)', overflowY: 'auto' } }} extra={
            <Button type="primary" size="small" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
              新增
            </Button>
          }>
            <Table
              rowKey="id"
              loading={loading}
              dataSource={roles}
              columns={roleColumns}
              pagination={false}
              size="small"
              onRow={record => {
                const accent = roleHex(record.code);
                const isSelected = selectedRoleId === record.id;
                return {
                  onClick: () => { void handleSelectRole(record); },
                  style: {
                    cursor: 'pointer',
                    background: isSelected ? `${accent}0d` : undefined,
                    transition: 'background 120ms ease',
                  },
                };
              }}
            />
          </Card>
        </Col>
        <Col xs={24} xl={14} xxl={15}>
          <Card className="bi-card" title="角色配置" styles={{ body: { maxHeight: 'calc(100vh - 280px)', overflowY: 'auto' } }}>
            {meta && roleDetail && currentRole ? (
              <>
                {(() => {
                  const accent = roleHex(roleDetail.code);
                  return (
                    <div style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 16,
                      padding: '14px 18px',
                      background: '#ffffff',
                      border: '1px solid #e2e8f0',
                      borderLeft: `4px solid ${accent}`,
                      borderRadius: 6,
                      marginBottom: 20,
                    }}>
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, marginBottom: 4 }}>
                          <span style={{ fontSize: 18, fontWeight: 600, color: '#0f172a', letterSpacing: '-0.01em' }}>
                            {roleDetail.name}
                          </span>
                          <span style={{ fontSize: 12, color: '#94a3b8', fontVariantNumeric: 'tabular-nums' }}>
                            {roleDetail.code}
                          </span>
                        </div>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, color: '#64748b', flexWrap: 'wrap' }}>
                          <span style={{
                            display: 'inline-block',
                            padding: '1px 8px',
                            background: `${accent}12`,
                            color: accent,
                            border: `1px solid ${accent}30`,
                            borderRadius: 3,
                            fontWeight: 500,
                            fontSize: 11,
                          }}>
                            {roleDetail.builtin ? '内置角色' : '自定义角色'}
                          </span>
                          {isReadOnly && (
                            <span style={{
                              display: 'inline-block',
                              padding: '1px 8px',
                              background: '#fff7e6',
                              color: '#d48806',
                              border: '1px solid #ffd591',
                              borderRadius: 3,
                              fontWeight: 500,
                              fontSize: 11,
                            }}>
                              只读
                            </span>
                          )}
                          <span style={{ fontVariantNumeric: 'tabular-nums', color: '#64748b' }}>
                            {currentRole.permissionCount} 项权限 · {currentRole.userCount} 个用户在用
                          </span>
                        </div>
                        {roleDetail.description && (
                          <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 4 }}>
                            {roleDetail.description}
                          </div>
                        )}
                      </div>
                    </div>
                  );
                })()}

                <Form<RoleFormValues> form={form} layout="vertical" onFinish={handleSave}>
                  <Row gutter={16}>
                    <Col xs={24} md={12}>
                      <Form.Item
                        name="name"
                        label="角色名称"
                        rules={isReadOnly ? [] : [{ required: true, message: '请输入角色名称' }]}
                      >
                        <Input disabled={isReadOnly} placeholder="请输入角色名称" />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item name="description" label="角色说明">
                        <Input placeholder="补充角色适用范围" disabled={isReadOnly} />
                      </Form.Item>
                    </Col>
                  </Row>

                  <div style={{ marginTop: 8, marginBottom: 12, paddingBottom: 8, borderBottom: '1px solid #f1f5f9' }}>
                    <Typography.Text strong style={{ fontSize: 14, color: '#0f172a' }}>功能模块权限</Typography.Text>
                  </div>
                  <Form.Item name="permissions" style={{ marginBottom: 16 }}>
                    <PermissionMatrix groups={permissionLayers.moduleGroups} readOnly={isReadOnly} />
                  </Form.Item>

                  <div style={{ marginTop: 16, marginBottom: 12, paddingBottom: 8, borderBottom: '1px solid #f1f5f9' }}>
                    <Typography.Text strong style={{ fontSize: 14, color: '#0f172a' }}>数据范围权限</Typography.Text>
                  </div>
                  <Row gutter={12}>
                    <Col xs={24} md={12}>
                      <Form.Item name="depts" label="默认可见部门">
                        <Select mode="multiple" allowClear options={meta.depts} placeholder="不选=不限制" maxTagCount="responsive" disabled={isReadOnly} />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item name="warehouses" label="默认可见仓库">
                        <Select mode="multiple" allowClear showSearch optionFilterProp="label" options={meta.warehouses} placeholder="不选=不限制" maxTagCount="responsive" disabled={isReadOnly} />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item name="platforms" label="默认可见平台">
                        <Select mode="multiple" allowClear options={meta.platforms} placeholder="不选=不限制" maxTagCount="responsive" disabled={isReadOnly} />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item name="shops" label="默认可见店铺">
                        <Select mode="multiple" allowClear showSearch optionFilterProp="label" options={meta.shops} placeholder="不选=不限制" maxTagCount="responsive" disabled={isReadOnly} />
                      </Form.Item>
                    </Col>
                    <Col xs={24}>
                      <Form.Item name="domains" label="默认数据域">
                        <Select mode="multiple" allowClear options={meta.domains} placeholder="不选=不限制" maxTagCount="responsive" disabled={isReadOnly} />
                      </Form.Item>
                    </Col>
                  </Row>

                  {permissionLayers.fieldGroups.length > 0 && (
                    <>
                      <div style={{ marginTop: 16, marginBottom: 12, paddingBottom: 8, borderBottom: '1px solid #f1f5f9' }}>
                        <Typography.Text strong style={{ fontSize: 14, color: '#0f172a' }}>字段权限</Typography.Text>
                        <Typography.Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>
                          敏感字段可见性
                        </Typography.Text>
                      </div>
                      <Form.Item style={{ marginBottom: 16 }}>
                        <PermissionMatrix
                          groups={permissionLayers.fieldGroups}
                          readOnly={isReadOnly}
                          value={watchedPermissions}
                          onChange={handlePermissionsChange}
                        />
                      </Form.Item>
                    </>
                  )}

                  {isReadOnly && (
                    <div style={{
                      padding: '10px 14px',
                      background: '#fff7e6',
                      border: '1px solid #ffd591',
                      borderRadius: 6,
                      marginBottom: 16,
                      fontSize: 12,
                      color: '#d48806',
                    }}>
                      💡 超级管理员角色固定拥有全部权限，默认范围也不会生效，因此这里保持只读。
                    </div>
                  )}

                  <Button type="primary" htmlType="submit" loading={saving} disabled={isReadOnly}>
                    保存角色配置
                  </Button>
                </Form>
              </>
            ) : (
              <Typography.Text type="secondary">请选择左侧角色后再配置权限。</Typography.Text>
            )}
          </Card>
        </Col>
      </Row>
      <Modal
        title="新增角色"
        open={createOpen}
        onCancel={() => { setCreateOpen(false); createForm.resetFields(); }}
        onOk={handleCreate}
        confirmLoading={creating}
        okText="创建"
        cancelText="取消"
        destroyOnHidden
      >
        <Form form={createForm} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="name" label="角色名称" rules={[{ required: true, message: '请输入角色名称' }]}>
            <Input placeholder="如：电商运营" />
          </Form.Item>
          <Form.Item name="code" label="角色编码" rules={[{ required: true, message: '请输入角色编码' }]}>
            <Input placeholder="如：ecommerce_operator（英文下划线）" />
          </Form.Item>
          <Form.Item name="description" label="角色说明">
            <Input placeholder="补充角色适用范围" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default RoleAccessPage;
