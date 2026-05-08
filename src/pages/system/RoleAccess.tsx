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
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';

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

const PermissionMatrix: React.FC<{
  groups: PermissionGroup[];
  onChange?: (value: string[]) => void;
  readOnly?: boolean;
  value?: string[];
}> = ({ groups, onChange, readOnly = false, value = [] }) => {
  const selected = useMemo(() => new Set(value), [value]);

  return (
    <Space orientation="vertical" size={12} style={{ width: '100%' }}>
      {groups.map(group => (
        <Card key={group.key} size="small" title={group.label} styles={{ body: { padding: '12px 16px' } }}>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 10 }}>
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
        </Card>
      ))}
    </Space>
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
      render: (_: string, record: RoleItem) => (
        <div>
          <Typography.Text strong style={{ color: selectedRoleId === record.id ? '#4338ca' : '#0f172a' }}>
            {record.name}
          </Typography.Text>
          <Typography.Text type="secondary" style={{ display: 'block', fontSize: 12 }}>
            {record.code}
          </Typography.Text>
        </div>
      ),
    },
    {
      title: '类型',
      dataIndex: 'builtin',
      key: 'builtin',
      width: 92,
      render: (builtin: boolean) => (
        <Tag color={builtin ? 'blue' : 'default'} style={{ marginInlineEnd: 0 }}>
          {builtin ? '内置' : '自定义'}
        </Tag>
      ),
    },
    {
      title: '授权',
      key: 'stats',
      width: 120,
      render: (_: unknown, record: RoleItem) => (
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          {record.permissionCount} 权限 / {record.userCount} 用户
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

  return (
    <div>
      {contextHolder}
      <Row gutter={[16, 16]}>
        <Col xs={24} xl={9}>
          <Card className="bi-card" title="角色列表" styles={{ body: { maxHeight: 'calc(100vh - 180px)', overflowY: 'auto' } }} extra={
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
              rowClassName={record => (record.id === selectedRoleId ? 'ant-table-row-selected' : '')}
              onRow={record => ({
                onClick: () => { void handleSelectRole(record); },
                style: { cursor: 'pointer' },
              })}
            />
          </Card>
        </Col>
        <Col xs={24} xl={15}>
          <Card className="bi-card" title="角色配置" styles={{ body: { maxHeight: 'calc(100vh - 180px)', overflowY: 'auto' } }}>
            {meta && roleDetail && currentRole ? (
              <>
                <div style={{ marginBottom: 20 }}>
                  <Space size={8} wrap>
                    <Typography.Title level={5} style={{ marginBottom: 0 }}>
                      {roleDetail.name}
                    </Typography.Title>
                    <Tag color={roleDetail.builtin ? 'blue' : 'default'} style={{ marginInlineEnd: 0 }}>
                      {roleDetail.builtin ? '内置角色' : '自定义角色'}
                    </Tag>
                    {isReadOnly && (
                      <Tag color="gold" style={{ marginInlineEnd: 0 }}>
                        只读
                      </Tag>
                    )}
                  </Space>
                  <Typography.Text type="secondary" style={{ display: 'block', marginTop: 4 }}>
                    角色编码：{roleDetail.code}
                  </Typography.Text>
                  {roleDetail.description && (
                    <Typography.Text type="secondary" style={{ display: 'block', marginTop: 4 }}>
                      {roleDetail.description}
                    </Typography.Text>
                  )}
                </div>

                <Form<RoleFormValues> form={form} layout="vertical" onFinish={handleSave}>
                  <Row gutter={16}>
                    <Col xs={24} md={12}>
                      <Form.Item name="name" label="角色名称" rules={[{ required: true, message: '请输入角色名称' }]}>
                        <Input disabled={isReadOnly} placeholder="请输入角色名称" />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item name="description" label="角色说明">
                        <Input placeholder="补充角色适用范围" disabled={isReadOnly} />
                      </Form.Item>
                    </Col>
                  </Row>

                  <Typography.Title level={5} style={{ marginBottom: 12 }}>1. 功能模块权限</Typography.Title>
                  <Form.Item name="permissions" label="模块与动作">
                    <PermissionMatrix groups={permissionLayers.moduleGroups} readOnly={isReadOnly} />
                  </Form.Item>

                  <Typography.Title level={5} style={{ marginBottom: 12, marginTop: 8 }}>2. 数据范围权限</Typography.Title>
                  <Form.Item name="depts" label="默认可见部门">
                    <Select mode="multiple" options={meta.depts} placeholder="不选表示不限制部门" maxTagCount="responsive" disabled={isReadOnly} />
                  </Form.Item>

                  <Form.Item name="platforms" label="默认可见平台">
                    <Select mode="multiple" options={meta.platforms} placeholder="不选表示不限制平台" maxTagCount="responsive" disabled={isReadOnly} />
                  </Form.Item>

                  <Form.Item name="shops" label="默认可见店铺">
                    <Select
                      mode="multiple"
                      showSearch
                      optionFilterProp="label"
                      options={meta.shops}
                      placeholder="不选表示不限制店铺"
                      maxTagCount="responsive"
                      disabled={isReadOnly}
                    />
                  </Form.Item>

                  <Form.Item name="warehouses" label="默认可见仓库">
                    <Select
                      mode="multiple"
                      showSearch
                      optionFilterProp="label"
                      options={meta.warehouses}
                      placeholder="不选表示不限制仓库"
                      maxTagCount="responsive"
                      disabled={isReadOnly}
                    />
                  </Form.Item>

                  <Form.Item name="domains" label="默认数据域">
                    <Select mode="multiple" options={meta.domains} placeholder="不选表示不限制数据域" maxTagCount="responsive" disabled={isReadOnly} />
                  </Form.Item>

                  <Typography.Title level={5} style={{ marginBottom: 12, marginTop: 8 }}>3. 字段权限</Typography.Title>
                  {permissionLayers.fieldGroups.length > 0 ? (
                    <Form.Item label="敏感字段可见性">
                      <PermissionMatrix
                        groups={permissionLayers.fieldGroups}
                        readOnly={isReadOnly}
                        value={watchedPermissions}
                        onChange={handlePermissionsChange}
                      />
                    </Form.Item>
                  ) : (
                    <Typography.Text type="secondary" style={{ display: 'block', marginBottom: 16 }}>
                      当前没有可配置的字段权限。
                    </Typography.Text>
                  )}

                  {isReadOnly && (
                    <Typography.Text type="secondary" style={{ display: 'block', marginBottom: 16 }}>
                      超级管理员角色固定拥有全部权限，默认范围也不会生效，因此这里保持只读。
                    </Typography.Text>
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
