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
  Steps,
  Switch,
  Table,
  Tag,
  Typography,
  Upload,
  message,
} from 'antd';
import type { UploadFile } from 'antd';
import { DeleteOutlined, UploadOutlined } from '@ant-design/icons';
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
      title: '手机号',
      dataIndex: 'phone',
      key: 'phone',
      width: 120,
      render: (phone: string) => phone || '-',
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
        <Tag color={status === 'active' ? 'green' : status === 'pending' ? 'orange' : 'default'} style={{ marginInlineEnd: 0 }}>
          {status === 'active' ? '启用' : status === 'pending' ? '待审批' : '停用'}
        </Tag>
      ),
    },
    ...(users.some(u => u.status === 'pending') ? [{
      title: '申请备注',
      dataIndex: 'remark',
      key: 'remark',
      ellipsis: true,
      render: (remark: string, record: UserItem) => record.status === 'pending' && remark
        ? <Typography.Text style={{ fontSize: 12, color: '#d46b08' }}>{remark}</Typography.Text>
        : null,
    }] : []),
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
            extra={<Space><Button onClick={() => setBatchOpen(true)} icon={<UploadOutlined />}>批量导入</Button><Button type="primary" onClick={() => setCreateOpen(true)}>新增用户</Button></Space>}
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

                <div style={{ display: 'flex', alignItems: 'center', gap: 16, padding: '12px 16px', background: '#fafafa', borderRadius: 8, marginBottom: 20 }}>
                  <div style={{ width: 48, height: 48, borderRadius: 12, background: 'linear-gradient(135deg, #4f6bff 0%, #7aa2ff 100%)', color: '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 18, fontWeight: 700, flexShrink: 0 }}>
                    {(access.realName || '?').slice(0, 1)}
                  </div>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <Typography.Title level={5} style={{ marginBottom: 0 }}>{access.realName}</Typography.Title>
                    <Typography.Text type="secondary">@{access.username}{currentUser.phone ? ` · ${currentUser.phone}` : ''}</Typography.Text>
                  </div>
                  <Form form={accessForm} onFinish={handleSaveAccess} style={{ marginBottom: 0 }}>
                    <Form.Item name="status" valuePropName="checked" style={{ marginBottom: 0 }}>
                      <Switch checkedChildren="启用" unCheckedChildren="停用" />
                    </Form.Item>
                  </Form>
                </div>

                <Form form={accessForm} layout="vertical" onFinish={handleSaveAccess}>
                  <Form.Item label="分配角色" name="roleCodes">
                    <Select
                      mode="multiple"
                      options={meta.roles.map(role => ({ label: role.name, value: role.code }))}
                      placeholder="请选择角色（可多选）"
                    />
                  </Form.Item>
                  <Form.Item style={{ marginBottom: 0 }}>
                    <Button type="primary" htmlType="submit" loading={saving}>
                      保存
                    </Button>
                  </Form.Item>
                </Form>

                <div style={{ marginTop: 24, paddingTop: 16, borderTop: '1px solid #f0f0f0' }}>
                  <Typography.Text strong style={{ display: 'block', marginBottom: 12 }}>重置密码</Typography.Text>
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
