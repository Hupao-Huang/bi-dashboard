import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Col,
  Form,
  Input,
  Modal,
  Popconfirm,
  Result,
  Row,
  Select,
  Space,
  Statistic,
  Steps,
  Switch,
  Table,
  Tag,
  Typography,
  Upload,
  message,
} from 'antd';
import type { UploadFile } from 'antd';
import { DeleteOutlined, UploadOutlined, SearchOutlined, TeamOutlined, CheckCircleOutlined, StopOutlined, ClockCircleOutlined, SyncOutlined } from '@ant-design/icons';
import { useLocation } from 'react-router-dom';
import { API_BASE } from '../../config';

const ROLE_COLOR_MAP: Record<string, string> = {
  super_admin: 'red',
  management: 'gold',
  dept_manager: 'cyan',
  operator: 'blue',
  finance: 'purple',
  customer_service: 'green',
  ecommerce_manager: 'geekblue',
  social_manager: 'magenta',
  offline_manager: 'orange',
  distribution_manager: 'lime',
  procurement: 'volcano',
};

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

const roleColor = (code: string): string => ROLE_COLOR_MAP[code] || 'blue';
const roleHex = (code: string): string => ROLE_HEX_MAP[code] || '#64748b';

type MetaOption = {
  label: string;
  value: string;
};

type RoleOption = {
  code: string;
  name: string;
};

type UserItem = {
  id: number;
  lastLoginAt: string;
  phone?: string;
  realName: string;
  remark?: string;
  roles: string[];
  status: string;
  username: string;
};

type AccessData = {
  dataScopes: {
    depts: string[];
    domains: string[];
    platforms: string[];
    shops: string[];
    warehouses: string[];
  };
  realName: string;
  roleCodes: string[];
  status: string;
  userId: number;
  username: string;
};

type MetaData = {
  depts: MetaOption[];
  domains: MetaOption[];
  platforms: MetaOption[];
  roles: RoleOption[];
  shops: MetaOption[];
  warehouses: MetaOption[];
};

const parseData = async <T,>(response: Response): Promise<T> => {
  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.msg || '请求失败');
  }
  return body.data as T;
};

const UserAccessPage: React.FC = () => {
  const [messageApi, contextHolder] = message.useMessage();
  const [meta, setMeta] = useState<MetaData | null>(null);
  const [users, setUsers] = useState<UserItem[]>([]);
  const [selectedUserId, setSelectedUserId] = useState<number | null>(null);
  const [access, setAccess] = useState<AccessData | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [passwordSaving, setPasswordSaving] = useState(false);
  const [createForm] = Form.useForm();
  const [accessForm] = Form.useForm();
  const [passwordForm] = Form.useForm();

  const [searchText, setSearchText] = useState('');
  const location = useLocation();
  const [statusFilter, setStatusFilter] = useState<string>(() => {
    const s = new URLSearchParams(location.search).get('status');
    return s === 'pending' ? 'pending' : '';
  });

  // 批量导入
  const [batchOpen, setBatchOpen] = useState(false);
  const [batchStep, setBatchStep] = useState(0); // 0=上传配置 1=预览 2=结果
  const [batchFile, setBatchFile] = useState<UploadFile[]>([]);
  const [batchPassword, setBatchPassword] = useState('');
  const [batchRoles, setBatchRoles] = useState<string[]>([]);
  const [batchPreview, setBatchPreview] = useState<{ total: number; valid: number; errors: Array<{ row: number; realName: string; error: string }>; preview: Array<{ row: number; realName: string; phone: string; department: string; username: string; valid: boolean; error?: string }> } | null>(null);
  const [batchResult, setBatchResult] = useState<{ imported: number; total: number; errors: Array<{ row: number; realName: string; error: string }> } | null>(null);
  const [batchLoading, setBatchLoading] = useState(false);

  const loadUsers = useCallback(async () => {
    const data = await parseData<{ list: UserItem[] }>(await fetch(`${API_BASE}/api/admin/users`));
    setUsers(data.list);
    return data.list;
  }, []);

  const loadMeta = useCallback(async () => {
    const data = await parseData<MetaData>(await fetch(`${API_BASE}/api/admin/meta`));
    setMeta(data);
  }, []);

  const loadAccess = useCallback(async (userId: number) => {
    const data = await parseData<AccessData>(await fetch(`${API_BASE}/api/admin/users/${userId}/access`));
    setAccess(data);
    accessForm.setFieldsValue({
      domains: data.dataScopes.domains,
      depts: data.dataScopes.depts,
      platforms: data.dataScopes.platforms,
      roleCodes: data.roleCodes,
      shops: data.dataScopes.shops,
      warehouses: data.dataScopes.warehouses,
      status: data.status === 'active',
    });
  }, [accessForm]);

  useEffect(() => {
    (async () => {
      try {
        await loadMeta();
        const loadedUsers = await loadUsers();
        if (loadedUsers.length > 0) {
          const firstId = loadedUsers[0].id;
          setSelectedUserId(firstId);
          await loadAccess(firstId);
        }
      } catch (error) {
        messageApi.error(error instanceof Error ? error.message : '加载失败');
      } finally {
        setLoading(false);
      }
    })();
  }, [loadAccess, loadMeta, loadUsers, messageApi]);

  const roleNameMap = useMemo(() => {
    const entries = (meta?.roles || []).map(role => [role.code, role.name]);
    return Object.fromEntries(entries) as Record<string, string>;
  }, [meta]);

  const stats = useMemo(() => ({
    total: users.length,
    active: users.filter(u => u.status === 'active').length,
    disabled: users.filter(u => u.status === 'disabled').length,
    pending: users.filter(u => u.status === 'pending').length,
  }), [users]);

  const filteredUsers = useMemo(() => {
    const q = searchText.trim().toLowerCase();
    return users.filter(u => {
      if (statusFilter && u.status !== statusFilter) return false;
      if (!q) return true;
      return (
        u.realName.toLowerCase().includes(q) ||
        u.username.toLowerCase().includes(q) ||
        (u.phone || '').toLowerCase().includes(q)
      );
    });
  }, [users, searchText, statusFilter]);

  const handleSelectUser = async (user: UserItem) => {
    setSelectedUserId(user.id);
    try {
      await loadAccess(user.id);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '加载用户权限失败');
    }
  };

  const handleSaveAccess = async (values: any) => {
    if (!selectedUserId) return;
    setSaving(true);
    try {
      await parseData(
        await fetch(`${API_BASE}/api/admin/users/${selectedUserId}/access`, {
          body: JSON.stringify({
            dataScopes: {
              depts: values.depts || [],
              domains: values.domains || [],
              platforms: values.platforms || [],
              shops: values.shops || [],
              warehouses: values.warehouses || [],
            },
            roleCodes: values.roleCodes || [],
          }),
          headers: { 'Content-Type': 'application/json' },
          method: 'PUT',
        }),
      );
      await parseData(
        await fetch(`${API_BASE}/api/admin/users/${selectedUserId}/status`, {
          body: JSON.stringify({ status: values.status ? 'active' : 'disabled' }),
          headers: { 'Content-Type': 'application/json' },
          method: 'PUT',
        }),
      );
      await loadUsers();
      await loadAccess(selectedUserId);
      messageApi.success('权限已保存');
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '保存失败');
    } finally {
      setSaving(false);
    }
  };

  const handleCreateUser = async (values: any) => {
    try {
      await parseData(
        await fetch(`${API_BASE}/api/admin/users`, {
          body: JSON.stringify({
            dataScopes: {
              depts: values.depts || [],
              domains: values.domains || [],
              platforms: values.platforms || [],
              shops: values.shops || [],
              warehouses: values.warehouses || [],
            },
            password: values.password,
            phone: values.phone || '',
            realName: values.realName,
            roleCodes: values.roleCodes || [],
            status: values.status ? 'active' : 'disabled',
            username: values.username,
          }),
          headers: { 'Content-Type': 'application/json' },
          method: 'POST',
        }),
      );
      setCreateOpen(false);
      createForm.resetFields();
      const loadedUsers = await loadUsers();
      const latestUser = loadedUsers[loadedUsers.length - 1];
      if (latestUser) {
        setSelectedUserId(latestUser.id);
        await loadAccess(latestUser.id);
      }
      messageApi.success('用户已创建');
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '创建失败');
    }
  };

  const handleResetPassword = async (values: { password: string }) => {
    if (!selectedUserId) return;
    setPasswordSaving(true);
    try {
      await parseData(
        await fetch(`${API_BASE}/api/admin/users/${selectedUserId}/password`, {
          body: JSON.stringify(values),
          headers: { 'Content-Type': 'application/json' },
          method: 'PUT',
        }),
      );
      passwordForm.resetFields();
      messageApi.success('密码已重置');
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '重置密码失败');
    } finally {
      setPasswordSaving(false);
    }
  };

  const handleDeleteUser = async (userId: number) => {
    try {
      await parseData(
        await fetch(`${API_BASE}/api/admin/users/${userId}`, {
          method: 'DELETE',
        }),
      );
      const loadedUsers = await loadUsers();
      if (selectedUserId === userId) {
        if (loadedUsers.length > 0) {
          const nextUser = loadedUsers[0];
          setSelectedUserId(nextUser.id);
          await loadAccess(nextUser.id);
        } else {
          setSelectedUserId(null);
          setAccess(null);
          accessForm.resetFields();
        }
      }
      messageApi.success('用户已删除');
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '删除用户失败');
    }
  };

  const handleBatchPreview = async () => {
    if (!batchFile.length || !batchFile[0].originFileObj) {
      messageApi.error('请上传Excel文件');
      return;
    }
    if (!batchPassword) {
      messageApi.error('请设置初始密码');
      return;
    }
    setBatchLoading(true);
    try {
      const formData = new FormData();
      formData.append('file', batchFile[0].originFileObj);
      formData.append('password', batchPassword);
      formData.append('roleCodes', JSON.stringify(batchRoles));
      formData.append('dryRun', 'true');
      const res = await fetch(`${API_BASE}/api/admin/users/batch`, {
        method: 'POST',
        credentials: 'include',
        body: formData,
      });
      const body = await res.json();
      if (!res.ok) throw new Error(body.msg || '校验失败');
      setBatchPreview(body.data);
      setBatchStep(1);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '校验失败');
    } finally {
      setBatchLoading(false);
    }
  };

  const handleBatchImport = async () => {
    if (!batchFile.length || !batchFile[0].originFileObj) return;
    setBatchLoading(true);
    try {
      const formData = new FormData();
      formData.append('file', batchFile[0].originFileObj);
      formData.append('password', batchPassword);
      formData.append('roleCodes', JSON.stringify(batchRoles));
      formData.append('dryRun', 'false');
      const res = await fetch(`${API_BASE}/api/admin/users/batch`, {
        method: 'POST',
        credentials: 'include',
        body: formData,
      });
      const body = await res.json();
      if (!res.ok) throw new Error(body.msg || '导入失败');
      setBatchResult(body.data);
      setBatchStep(2);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '导入失败');
    } finally {
      setBatchLoading(false);
    }
  };

  const handleBatchClose = () => {
    setBatchOpen(false);
    setBatchStep(0);
    setBatchFile([]);
    setBatchPassword('');
    setBatchRoles([]);
    setBatchPreview(null);
    setBatchResult(null);
    if (batchResult && batchResult.imported > 0) {
      void loadUsers();
    }
  };

  const userColumns = [
    {
      title: '账号',
      dataIndex: 'username',
      key: 'username',
      render: (_: string, record: UserItem) => {
        const role = record.roles[0];
        const accent = role ? roleHex(role) : '#cbd5e1';
        const isSelected = selectedUserId === record.id;
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
                {record.realName}
              </Typography.Text>
              <Typography.Text type="secondary" style={{ display: 'block', fontSize: 12, fontVariantNumeric: 'tabular-nums' }}>
                @{record.username}
              </Typography.Text>
            </div>
          </div>
        );
      },
    },
    {
      title: '手机号',
      dataIndex: 'phone',
      key: 'phone',
      width: 110,
      responsive: ['lg' as const],
      render: (phone: string) => phone || '-',
    },
    {
      title: '角色',
      dataIndex: 'roles',
      key: 'roles',
      render: (roles: string[]) => (
        <Space size={[4, 4]} wrap>
          {roles.length > 0 ? roles.map(role => (
            <Tag key={role} color={roleColor(role)} style={{ marginInlineEnd: 0 }}>
              {roleNameMap[role] || role}
            </Tag>
          )) : <Tag style={{ marginInlineEnd: 0 }}>未分配</Tag>}
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 78,
      responsive: ['xxl' as const],
      render: (status: string) => (
        <Tag color={status === 'active' ? 'green' : status === 'pending' ? 'orange' : 'default'} style={{ marginInlineEnd: 0 }}>
          {status === 'active' ? '启用' : status === 'pending' ? '待审批' : '停用'}
        </Tag>
      ),
    },
    {
      title: '上次登录',
      dataIndex: 'lastLoginAt',
      key: 'lastLoginAt',
      width: 160,
      responsive: ['xxl' as const],
      render: (value: string) => value || '-',
    },
  ];

  const currentUser = users.find(user => user.id === selectedUserId) || null;

  return (
    <div>
      {contextHolder}

      <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
        <Col xs={12} sm={6}>
          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: '#1e40af' }} bodyStyle={{ padding: 16 }}>
            <Statistic title={<><TeamOutlined style={{ marginRight: 6 }} />总用户</>} value={stats.total} valueStyle={{ color: '#1e40af', fontSize: 22 }} />
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: '#16a34a' }} bodyStyle={{ padding: 16 }}>
            <Statistic title={<><CheckCircleOutlined style={{ marginRight: 6, color: '#16a34a' }} />已启用</>} value={stats.active} valueStyle={{ color: '#16a34a', fontSize: 22 }} />
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: '#94a3b8' }} bodyStyle={{ padding: 16 }}>
            <Statistic title={<><StopOutlined style={{ marginRight: 6, color: '#94a3b8' }} />已停用</>} value={stats.disabled} valueStyle={{ color: '#64748b', fontSize: 22 }} />
          </Card>
        </Col>
        <Col xs={12} sm={6}>
          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: '#f59e0b' }} bodyStyle={{ padding: 16 }}>
            <Statistic
              title={<><ClockCircleOutlined style={{ marginRight: 6, color: '#f59e0b' }} />待审批</>}
              value={stats.pending}
              valueStyle={{ color: stats.pending > 0 ? '#f59e0b' : '#94a3b8', fontSize: 22, fontWeight: stats.pending > 0 ? 700 : 400 }}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]}>
        <Col xs={24} xl={12} xxl={10}>
          <Card
            className="bi-card"
            title="用户列表"
            extra={<Space>
              <Button
                icon={<SyncOutlined />}
                onClick={async () => {
                  const hide = message.loading('正在同步全员钉钉真名…', 0);
                  try {
                    const res = await fetch(`${API_BASE}/api/admin/sync-all-dingtalk-names`, { method: 'POST', credentials: 'include' });
                    const data = await res.json().catch(() => ({}));
                    hide();
                    if (res.ok) {
                      const d = data.data || data;
                      message.success(`同步完成: 成功 ${d.success}/${d.total}${d.failed?.length ? `, 失败 ${d.failed.length}` : ''}`);
                      loadUsers();
                    } else {
                      message.error(data.msg || data.error || '同步失败');
                    }
                  } catch {
                    hide();
                    message.error('网络错误');
                  }
                }}
              >同步钉钉真名</Button>
              <Button onClick={() => setBatchOpen(true)} icon={<UploadOutlined />}>批量导入</Button>
              <Button type="primary" onClick={() => setCreateOpen(true)}>新增用户</Button>
            </Space>}
          >
            <Input
              placeholder="搜索 姓名 / 账号 / 手机号"
              prefix={<SearchOutlined style={{ color: '#94a3b8' }} />}
              allowClear
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              style={{ marginBottom: 12 }}
            />
            {statusFilter === 'pending' && (
              <Tag
                color="orange"
                closable
                onClose={() => setStatusFilter('')}
                style={{ marginBottom: 12 }}
              >
                仅看待审批 · {stats.pending}
              </Tag>
            )}
            <Table
              rowKey="id"
              loading={loading}
              dataSource={filteredUsers}
              columns={userColumns}
              pagination={{ pageSize: 20, hideOnSinglePage: true, size: 'small' }}
              size="small"
              onRow={record => {
                const role = record.roles[0];
                const accent = role ? roleHex(role) : '#cbd5e1';
                const isSelected = selectedUserId === record.id;
                return {
                  onClick: () => { void handleSelectUser(record); },
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
        <Col xs={24} xl={12} xxl={14}>
          <Card className="bi-card" title="用户配置">
            {access && meta && currentUser ? (
              <>
                {currentUser.status === 'pending' && (
                  <Alert
                    type="warning"
                    showIcon
                    style={{ marginBottom: 16 }}
                    message="待审批用户"
                    description={currentUser.remark ? `权限申请说明：${currentUser.remark}` : '该用户通过钉钉扫码注册，请分配角色后启用账号'}
                  />
                )}

                <Form form={accessForm} layout="vertical" onFinish={handleSaveAccess}>
                  {(() => {
                    const primaryRole = access.roleCodes[0];
                    const accent = primaryRole ? roleHex(primaryRole) : '#64748b';
                    const primaryName = primaryRole ? roleNameMap[primaryRole] || primaryRole : '未分配角色';
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
                              {access.realName}
                            </span>
                            <span style={{ fontSize: 12, color: '#94a3b8', fontVariantNumeric: 'tabular-nums' }}>
                              @{access.username}
                            </span>
                          </div>
                          <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12, color: '#64748b' }}>
                            <span style={{
                              display: 'inline-block',
                              padding: '1px 8px',
                              background: `${accent}12`,
                              color: accent,
                              border: `1px solid ${accent}30`,
                              borderRadius: 3,
                              fontWeight: 500,
                              fontSize: 11,
                              letterSpacing: '0.02em',
                            }}>
                              {primaryName}
                            </span>
                            {currentUser.phone && (
                              <span style={{ fontVariantNumeric: 'tabular-nums' }}>
                                {currentUser.phone}
                              </span>
                            )}
                          </div>
                        </div>
                        <Form.Item name="status" valuePropName="checked" style={{ marginBottom: 0 }}>
                          <Switch checkedChildren="启用" unCheckedChildren="停用" />
                        </Form.Item>
                      </div>
                    );
                  })()}

                  <Form.Item label="分配角色" name="roleCodes">
                    <Select
                      mode="multiple"
                      options={meta.roles.map(role => ({ label: role.name, value: role.code }))}
                      placeholder="请选择角色（可多选）"
                    />
                  </Form.Item>
                  <Row gutter={12}>
                    <Col xs={24} md={12}>
                      <Form.Item label="可见部门" name="depts">
                        <Select mode="multiple" allowClear options={meta.depts} placeholder="不选=按角色默认" maxTagCount="responsive" />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item label="可见仓库" name="warehouses">
                        <Select mode="multiple" allowClear options={meta.warehouses} placeholder="不选=不限制" maxTagCount="responsive" />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item label="可见平台" name="platforms">
                        <Select mode="multiple" allowClear options={meta.platforms} placeholder="不选=不限制" maxTagCount="responsive" />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item label="可见店铺" name="shops">
                        <Select mode="multiple" allowClear options={meta.shops} placeholder="不选=不限制" maxTagCount="responsive" />
                      </Form.Item>
                    </Col>
                    <Col xs={24}>
                      <Form.Item label="可见业务域" name="domains">
                        <Select mode="multiple" allowClear options={meta.domains} placeholder="不选=不限制" maxTagCount="responsive" />
                      </Form.Item>
                    </Col>
                  </Row>
                  <Form.Item style={{ marginBottom: 0 }}>
                    <Button type="primary" htmlType="submit" loading={saving}>保存</Button>
                  </Form.Item>
                </Form>

                <div style={{ marginTop: 24, paddingTop: 16, borderTop: '1px solid #f0f0f0' }}>
                  <Typography.Text strong style={{ display: 'block', marginBottom: 12 }}>重置密码</Typography.Text>
                  <Form
                    form={passwordForm}
                    layout="inline"
                    onFinish={handleResetPassword}
                  >
                    <Form.Item
                      name="password"
                      rules={[
                        { required: true, message: '请输入新密码' },
                        { min: 6, message: '至少 6 位' },
                      ]}
                    >
                      <Input.Password placeholder="输入新密码" style={{ width: 240 }} />
                    </Form.Item>
                    <Popconfirm
                      title="确认重置密码？"
                      description={`将把 ${access.realName} (@${access.username}) 的密码改成你输入的新密码`}
                      onConfirm={() => passwordForm.submit()}
                      okText="确认重置"
                      cancelText="取消"
                    >
                      <Button loading={passwordSaving} danger>重置密码</Button>
                    </Popconfirm>
                  </Form>
                </div>

                <div style={{ marginTop: 16, padding: '12px 16px', background: '#fff1f0', border: '1px solid #ffccc7', borderRadius: 8 }}>
                  <Space size={12} align="center">
                    <Typography.Text strong style={{ color: '#cf1322' }}>危险操作</Typography.Text>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>删除后该账号无法恢复</Typography.Text>
                    <Popconfirm
                      title="确定删除该用户？"
                      description={`@${access.username} (${access.realName}) 将被永久删除`}
                      onConfirm={() => handleDeleteUser(access.userId)}
                      okText="确认删除"
                      okButtonProps={{ danger: true }}
                      cancelText="取消"
                    >
                      <Button danger size="small" icon={<DeleteOutlined />}>删除该用户</Button>
                    </Popconfirm>
                  </Space>
                </div>
              </>
            ) : (
              <Typography.Text type="secondary">请选择左侧用户。</Typography.Text>
            )}
          </Card>
        </Col>
      </Row>

      <Modal
        open={createOpen}
        title="新增用户"
        onCancel={() => setCreateOpen(false)}
        onOk={() => createForm.submit()}
        okText="创建"
        destroyOnHidden
      >
        <Form
          form={createForm}
          layout="vertical"
          initialValues={{ status: true }}
          onFinish={handleCreateUser}
        >
          <Row gutter={12}>
            <Col span={12}>
              <Form.Item name="username" label="账号" rules={[{ required: true, message: '请输入账号' }]}>
                <Input placeholder="例如 zhangsan" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="realName" label="姓名" rules={[{ required: true, message: '请输入姓名' }]}>
                <Input placeholder="例如 张三" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="phone" label="手机号">
                <Input placeholder="11 位手机号（选填）" maxLength={11} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="password" label="初始密码" rules={[{ required: true, message: '请输入初始密码' }, { min: 6, message: '至少 6 位' }]}>
                <Input.Password placeholder="请输入初始密码" />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="roleCodes" label="角色">
            <Select mode="multiple" options={(meta?.roles || []).map(role => ({ label: role.name, value: role.code }))} placeholder="可多选" />
          </Form.Item>
          <Row gutter={12}>
            <Col span={12}>
              <Form.Item name="depts" label="可见部门">
                <Select mode="multiple" allowClear options={meta?.depts || []} maxTagCount="responsive" placeholder="不选=按角色默认" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="warehouses" label="可见仓库">
                <Select mode="multiple" allowClear options={meta?.warehouses || []} maxTagCount="responsive" placeholder="不选=不限制" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="platforms" label="可见平台">
                <Select mode="multiple" allowClear options={meta?.platforms || []} maxTagCount="responsive" placeholder="不选=不限制" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="shops" label="可见店铺">
                <Select mode="multiple" allowClear options={meta?.shops || []} maxTagCount="responsive" placeholder="不选=不限制" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="domains" label="可见业务域">
                <Select mode="multiple" allowClear options={meta?.domains || []} maxTagCount="responsive" placeholder="不选=不限制" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="status" label="立即启用" valuePropName="checked">
                <Switch checkedChildren="启用" unCheckedChildren="停用" />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>

      {/* 批量导入 Modal */}
      <Modal
        open={batchOpen}
        title="批量导入用户"
        onCancel={handleBatchClose}
        footer={null}
        width={800}
        destroyOnHidden
      >
        <Steps current={batchStep} size="small" style={{ marginBottom: 24 }} items={[
          { title: '上传配置' },
          { title: '预览校验' },
          { title: '导入结果' },
        ]} />

        {batchStep === 0 && (
          <Form layout="vertical">
            <Form.Item label="钉钉通讯录 Excel" required>
              <Upload
                accept=".xlsx,.xls"
                maxCount={1}
                fileList={batchFile}
                onChange={({ fileList }) => setBatchFile(fileList)}
                beforeUpload={() => false}
              >
                <Button icon={<UploadOutlined />}>选择文件</Button>
              </Upload>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                Excel需包含"姓名"和"手机号"列，手机号将作为登录账号
              </Typography.Text>
            </Form.Item>
            <Form.Item label="统一初始密码" required>
              <Input.Password
                value={batchPassword}
                onChange={e => setBatchPassword(e.target.value)}
                placeholder="至少8位，含大小写字母和数字"
              />
            </Form.Item>
            <Form.Item label="分配角色">
              <Select
                mode="multiple"
                value={batchRoles}
                onChange={setBatchRoles}
                options={(meta?.roles || []).map(role => ({ label: role.name, value: role.code }))}
                placeholder="选择要分配的角色（可多选）"
              />
            </Form.Item>
            <div style={{ textAlign: 'right' }}>
              <Space>
                <Button onClick={handleBatchClose}>取消</Button>
                <Button type="primary" loading={batchLoading} onClick={handleBatchPreview}
                  disabled={!batchFile.length || !batchPassword}>
                  预览校验
                </Button>
              </Space>
            </div>
          </Form>
        )}

        {batchStep === 1 && batchPreview && (
          <>
            <Alert
              style={{ marginBottom: 16 }}
              type={(batchPreview.errors?.length ?? 0) > 0 ? 'warning' : 'success'}
              message={`共 ${batchPreview.total} 条数据，有效 ${batchPreview.valid} 条${(batchPreview.errors?.length ?? 0) > 0 ? `，错误 ${batchPreview.errors.length} 条` : ''}`}
            />
            <Table
              rowKey="row"
              size="small"
              dataSource={batchPreview.preview}
              pagination={{ pageSize: 10, size: 'small' }}
              scroll={{ y: 320 }}
              rowClassName={record => record.valid ? '' : 'batch-row-error'}
              columns={[
                { title: '行号', dataIndex: 'row', width: 60 },
                { title: '姓名', dataIndex: 'realName', width: 80 },
                { title: '手机号', dataIndex: 'phone', width: 120 },
                { title: '部门', dataIndex: 'department', width: 120 },
                { title: '登录账号', dataIndex: 'username', width: 120 },
                { title: '状态', dataIndex: 'valid', width: 120, render: (valid: boolean, record: any) =>
                  valid ? <Tag color="green">有效</Tag> : <Tag color="red">{record.error}</Tag>
                },
              ]}
            />
            <div style={{ textAlign: 'right', marginTop: 16 }}>
              <Space>
                <Button onClick={() => setBatchStep(0)}>返回修改</Button>
                <Button type="primary" loading={batchLoading} onClick={handleBatchImport}
                  disabled={batchPreview.valid === 0}>
                  确认导入 {batchPreview.valid} 人
                </Button>
              </Space>
            </div>
          </>
        )}

        {batchStep === 2 && batchResult && (
          <>
            <Result
              status={batchResult.imported > 0 ? 'success' : 'error'}
              title={`成功导入 ${batchResult.imported} 个用户`}
              subTitle={batchResult.errors?.length > 0 ? `${batchResult.errors.length} 条失败` : undefined}
            />
            {batchResult.errors?.length > 0 && (
              <Table
                rowKey="row"
                size="small"
                dataSource={batchResult.errors}
                pagination={false}
                columns={[
                  { title: '行号', dataIndex: 'row', width: 60 },
                  { title: '姓名', dataIndex: 'realName', width: 100 },
                  { title: '错误', dataIndex: 'error' },
                ]}
              />
            )}
            <div style={{ textAlign: 'right', marginTop: 16 }}>
              <Button type="primary" onClick={handleBatchClose}>完成</Button>
            </div>
          </>
        )}
      </Modal>
    </div>
  );
};

export default UserAccessPage;
