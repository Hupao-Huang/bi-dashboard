import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Col,
  Form,
  Input,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import { DeleteOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';

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
  realName: string;
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

  const userColumns = [
    {
      title: '账号',
      dataIndex: 'username',
      key: 'username',
      render: (_: string, record: UserItem) => (
        <div>
          <Typography.Text strong style={{ color: selectedUserId === record.id ? '#4338ca' : '#0f172a' }}>
            {record.realName}
          </Typography.Text>
          <Typography.Text type="secondary" style={{ display: 'block', fontSize: 12 }}>
            @{record.username}
          </Typography.Text>
        </div>
      ),
    },
    {
      title: '角色',
      dataIndex: 'roles',
      key: 'roles',
      render: (roles: string[]) => (
        <Space size={[4, 4]} wrap>
          {roles.length > 0 ? roles.map(role => (
            <Tag key={role} color="blue" style={{ marginInlineEnd: 0 }}>
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
      width: 90,
      render: (status: string) => (
        <Tag color={status === 'active' ? 'green' : 'default'} style={{ marginInlineEnd: 0 }}>
          {status === 'active' ? '启用' : '停用'}
        </Tag>
      ),
    },
    {
      title: '上次登录',
      dataIndex: 'lastLoginAt',
      key: 'lastLoginAt',
      width: 170,
      render: (value: string) => value || '-',
    },
    {
      title: '',
      key: 'action',
      width: 48,
      render: (_: unknown, record: UserItem) => (
        <Popconfirm
          title="确定删除该用户？"
          description={`@${record.username} 将被永久删除`}
          onConfirm={(e) => { e?.stopPropagation(); void handleDeleteUser(record.id); }}
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
      ),
    },
  ];

  const currentUser = users.find(user => user.id === selectedUserId) || null;

  return (
    <div>
      {contextHolder}
      <Row gutter={[16, 16]}>
        <Col xs={24} xl={10}>
          <Card
            className="bi-card"
            title="用户列表"
            extra={<Button type="primary" onClick={() => setCreateOpen(true)}>新增用户</Button>}
          >
            <Table
              rowKey="id"
              loading={loading}
              dataSource={users}
              columns={userColumns}
              pagination={false}
              size="small"
              rowClassName={record => (record.id === selectedUserId ? 'ant-table-row-selected' : '')}
              onRow={record => ({
                onClick: () => { void handleSelectUser(record); },
                style: { cursor: 'pointer' },
              })}
            />
          </Card>
        </Col>
        <Col xs={24} xl={14}>
          <Card className="bi-card" title="权限配置">
            {access && meta && currentUser ? (
              <>
                <div style={{ marginBottom: 20 }}>
                  <Typography.Title level={5} style={{ marginBottom: 4 }}>{access.realName}</Typography.Title>
                  <Typography.Text type="secondary">@{access.username}</Typography.Text>
                </div>

                <Form form={accessForm} layout="vertical" onFinish={handleSaveAccess}>
                  <Row gutter={16}>
                    <Col span={16}>
                      <Form.Item label="角色" name="roleCodes">
                        <Select
                          mode="multiple"
                          options={meta.roles.map(role => ({ label: role.name, value: role.code }))}
                          placeholder="请选择角色"
                        />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item label="账号启用" name="status" valuePropName="checked">
                        <Switch checkedChildren="启用" unCheckedChildren="停用" />
                      </Form.Item>
                    </Col>
                  </Row>

                  <Form.Item label="可见部门" name="depts">
                    <Select mode="multiple" options={meta.depts} placeholder="不选表示不限制部门" maxTagCount="responsive" />
                  </Form.Item>

                  <Form.Item label="可见平台" name="platforms">
                    <Select mode="multiple" options={meta.platforms} placeholder="不选表示不限制平台" maxTagCount="responsive" />
                  </Form.Item>

                  <Form.Item label="可见店铺" name="shops">
                    <Select
                      mode="multiple"
                      showSearch
                      optionFilterProp="label"
                      options={meta.shops}
                      placeholder="不选表示不限制店铺"
                      maxTagCount="responsive"
                    />
                  </Form.Item>

                  <Form.Item label="可见仓库" name="warehouses">
                    <Select mode="multiple" options={meta.warehouses} placeholder="不选表示不限制仓库" maxTagCount="responsive" />
                  </Form.Item>

                  <Form.Item label="数据域" name="domains">
                    <Select mode="multiple" options={meta.domains} placeholder="不选表示不限制数据域" maxTagCount="responsive" />
                  </Form.Item>

                  <Button type="primary" htmlType="submit" loading={saving}>
                    保存权限
                  </Button>
                </Form>

                <Card size="small" title="重置密码" style={{ marginTop: 16 }}>
                  <Form form={passwordForm} layout="inline" onFinish={handleResetPassword}>
                    <Form.Item
                      name="password"
                      rules={[
                        { required: true, message: '请输入新密码' },
                        { min: 6, message: '至少 6 位' },
                      ]}
                    >
                      <Input.Password placeholder="输入新密码" style={{ width: 240 }} />
                    </Form.Item>
                    <Button htmlType="submit" loading={passwordSaving}>重置密码</Button>
                  </Form>
                </Card>
              </>
            ) : (
              <Typography.Text type="secondary">请选择左侧用户后再配置权限。</Typography.Text>
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
        destroyOnClose
      >
        <Form
          form={createForm}
          layout="vertical"
          initialValues={{ status: true }}
          onFinish={handleCreateUser}
        >
          <Form.Item name="username" label="账号" rules={[{ required: true, message: '请输入账号' }]}>
            <Input placeholder="例如 zhangsan" />
          </Form.Item>
          <Form.Item name="realName" label="姓名" rules={[{ required: true, message: '请输入姓名' }]}>
            <Input placeholder="例如 张三" />
          </Form.Item>
          <Form.Item name="password" label="初始密码" rules={[{ required: true, message: '请输入初始密码' }, { min: 6, message: '至少 6 位' }]}>
            <Input.Password placeholder="请输入初始密码" />
          </Form.Item>
          <Form.Item name="roleCodes" label="角色">
            <Select mode="multiple" options={(meta?.roles || []).map(role => ({ label: role.name, value: role.code }))} />
          </Form.Item>
          <Form.Item name="depts" label="可见部门">
            <Select mode="multiple" options={meta?.depts || []} maxTagCount="responsive" />
          </Form.Item>
          <Form.Item name="warehouses" label="可见仓库">
            <Select mode="multiple" options={meta?.warehouses || []} maxTagCount="responsive" />
          </Form.Item>
          <Form.Item name="status" label="立即启用" valuePropName="checked">
            <Switch checkedChildren="启用" unCheckedChildren="停用" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default UserAccessPage;
