import React, { useCallback, useEffect, useState } from 'react';
import { Avatar, Button, Card, Col, Descriptions, Form, Input, message, Popconfirm, Row, Tag, Upload } from 'antd';
import { CameraOutlined, DingtalkOutlined, LockOutlined, SaveOutlined, SyncOutlined, UserOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';

const Profile: React.FC = () => {
  const [profile, setProfile] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [pwSaving, setPwSaving] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [form] = Form.useForm();
  const [pwForm] = Form.useForm();

  const handleSyncDingtalk = async () => {
    setSyncing(true);
    try {
      const res = await fetch(`${API_BASE}/api/profile/sync-dingtalk`, {
        method: 'POST', credentials: 'include',
      });
      const data = await res.json().catch(() => ({}));
      if (res.ok) {
        const realName = data.data?.realName || data.realName;
        message.success(`已同步钉钉真名: ${realName}`);
        fetchProfile();
      } else {
        message.error(data.msg || data.error || '同步失败');
      }
    } catch {
      message.error('网络错误');
    } finally {
      setSyncing(false);
    }
  };

  const fetchProfile = useCallback(() => {
    fetch(`${API_BASE}/api/user/profile`, { credentials: 'include' })
      .then(res => res.json())
      .then(res => {
        const data = res.data || res;
        setProfile(data);
        form.setFieldsValue({
          realName: data.realName,
          phone: data.phone,
          email: data.email,
        });
        setLoading(false);
      })
      .catch(err => { console.warn('Profile fetch:', err); setLoading(false); });
  }, [form]);

  useEffect(() => { fetchProfile(); }, [fetchProfile]);

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      const res = await fetch(`${API_BASE}/api/user/profile`, {
        method: 'PUT',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(values),
      });
      const data = await res.json();
      if (res.ok) {
        message.success('保存成功');
        fetchProfile();
      } else {
        message.error(data.msg || data.error || '保存失败');
      }
    } catch {} finally {
      setSaving(false);
    }
  };

  const handleAvatarUpload = async (file: File) => {
    const formData = new FormData();
    formData.append('avatar', file);
    const res = await fetch(`${API_BASE}/api/user/avatar`, {
      method: 'POST',
      credentials: 'include',
      body: formData,
    });
    const data = await res.json();
    if (res.ok) {
      message.success('头像更新成功');
      fetchProfile();
    } else {
      message.error(data.msg || data.error || '上传失败');
    }
    return false;
  };

  if (loading || !profile) return null;

  const roleLabelMap: Record<string, string> = {
    super_admin: '超级管理员',
    admin: '管理员',
    dept_manager: '部门主管',
    operator: '运营',
    viewer: '查看者',
  };

  return (
    <Row gutter={24}>
      <Col xs={24} md={8}>
        <Card style={{ textAlign: 'center' }}>
          <div style={{ position: 'relative', display: 'inline-block', marginBottom: 16 }}>
            <Avatar
              size={100}
              src={profile.avatar ? `${API_BASE}${profile.avatar}` : undefined}
              icon={!profile.avatar ? <UserOutlined /> : undefined}
              style={{ backgroundColor: profile.avatar ? undefined : '#1e40af' }}
            />
            <Upload
              showUploadList={false}
              accept=".jpg,.jpeg,.png,.gif,.webp"
              beforeUpload={(file) => { handleAvatarUpload(file); return false; }}
            >
              <Button
                shape="circle"
                size="small"
                icon={<CameraOutlined />}
                style={{
                  position: 'absolute',
                  bottom: 0,
                  right: 0,
                  background: '#fff',
                  border: '1px solid #d9d9d9',
                  boxShadow: '0 2px 4px rgba(0,0,0,0.1)',
                }}
              />
            </Upload>
          </div>
          <h2 style={{ margin: '8px 0 4px' }}>{profile.realName || profile.username}</h2>
          <div style={{ color: '#999', marginBottom: 12 }}>@{profile.username}</div>
          <div>
            {(profile.roles || []).map((r: string) => (
              <Tag key={r} color="blue">{roleLabelMap[r] || r}</Tag>
            ))}
          </div>
        </Card>

        <Card title="账号信息" style={{ marginTop: 16 }}>
          <Descriptions column={1} size="small">
            <Descriptions.Item label="用户名">{profile.username}</Descriptions.Item>
            <Descriptions.Item label="钉钉昵称">{profile.realName || '-'}</Descriptions.Item>
            <Descriptions.Item label="钉钉真名">
              {profile.dingtalkRealName ? (
                <span><strong>{profile.dingtalkRealName}</strong></span>
              ) : (
                <Tag>未同步</Tag>
              )}
              {profile.dingtalkBound && (
                <Button
                  size="small" type="link" icon={<SyncOutlined spin={syncing} />}
                  onClick={handleSyncDingtalk} loading={syncing}
                  style={{ marginLeft: 8 }}
                >
                  从钉钉同步
                </Button>
              )}
            </Descriptions.Item>
            {profile.hesiRealName && (
              <Descriptions.Item label="合思真名">{profile.hesiRealName}</Descriptions.Item>
            )}
            <Descriptions.Item label="最近登录">{profile.lastLoginAt || '-'}</Descriptions.Item>
            <Descriptions.Item label="钉钉">
              {profile.dingtalkBound ? (
                <span>
                  <Tag color="blue"><DingtalkOutlined /> 已绑定</Tag>
                  <Button
                    size="small"
                    type="link"
                    danger
                    onClick={async () => {
                      try {
                        const res = await fetch(`${API_BASE}/api/user/dingtalk`, {
                          method: 'POST',
                          credentials: 'include',
                          headers: { 'Content-Type': 'application/json' },
                          body: JSON.stringify({ action: 'unbind' }),
                        });
                        const data = await res.json().catch(() => ({}));
                        if (res.ok) {
                          message.success('已解绑钉钉');
                          fetchProfile();
                        } else {
                          message.error(data.msg || data.error || '解绑失败');
                        }
                      } catch {
                        message.error('网络错误，解绑失败');
                      }
                    }}
                  >
                    解绑
                  </Button>
                </span>
              ) : (
                <Button
                  size="small"
                  icon={<DingtalkOutlined />}
                  onClick={async () => {
                    const res = await fetch(`${API_BASE}/api/auth/dingtalk/url?state=bind`, { credentials: 'include' });
                    const body = await res.json();
                    if (body.data?.url) {
                      window.location.href = body.data.url;
                    }
                  }}
                >
                  绑定钉钉
                </Button>
              )}
            </Descriptions.Item>
          </Descriptions>
        </Card>
      </Col>

      <Col xs={24} md={16}>
        <Card title="个人信息">
          <Form form={form} layout="vertical" style={{ maxWidth: 500 }}>
            <Form.Item label="钉钉昵称" tooltip="钉钉聊天里显示的名字，可在钉钉个人设置里修改">
              <Input value={profile.realName || '-'} disabled />
            </Form.Item>
            <Form.Item label="真实名字" tooltip="钉钉企业通讯录里的实名">
              <Input
                value={profile.dingtalkRealName || '未同步'}
                disabled
                suffix={
                  profile.dingtalkBound ? (
                    <Button
                      type="link" size="small" icon={<SyncOutlined spin={syncing} />}
                      onClick={handleSyncDingtalk} loading={syncing}
                      style={{ padding: 0, height: 'auto' }}
                    >
                      从钉钉同步
                    </Button>
                  ) : null
                }
              />
            </Form.Item>
            <Form.Item label="合思名字" tooltip="合思系统里的实名（用于审批匹配）">
              <Input value={profile.hesiRealName || '未绑定'} disabled />
            </Form.Item>
            <Form.Item name="realName" label="真实姓名（BI 看板用）" tooltip={profile.dingtalkBound ? '已绑定钉钉，姓名由钉钉同步' : undefined}>
              <Input placeholder="请输入真实姓名" maxLength={20} disabled={profile.dingtalkBound} />
            </Form.Item>
            <Form.Item name="phone" label="手机号" tooltip={profile.dingtalkBound ? '已绑定钉钉，手机号由钉钉同步' : undefined}
              rules={[{ pattern: /^1\d{10}$/, message: '请输入正确的手机号' }]}>
              <Input placeholder="请输入手机号" maxLength={11} disabled={profile.dingtalkBound} />
            </Form.Item>
            <Form.Item name="email" label="邮箱"
              rules={[{ type: 'email', message: '请输入正确的邮箱' }]}>
              <Input placeholder="请输入邮箱" />
            </Form.Item>
            <Form.Item>
              <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={handleSave}>
                保存修改
              </Button>
            </Form.Item>
          </Form>
        </Card>

        <Card title="修改密码" style={{ marginTop: 16 }}>
          <Form form={pwForm} layout="vertical" style={{ maxWidth: 500 }}>
            <Form.Item name="oldPassword" label="当前密码" rules={[{ required: true, message: '请输入当前密码' }]}>
              <Input.Password prefix={<LockOutlined />} placeholder="请输入当前密码" />
            </Form.Item>
            <Form.Item name="newPassword" label="新密码"
              rules={[
                { required: true, message: '请输入新密码' },
                { min: 8, message: '密码至少8位' },
                { pattern: /^(?=.*[a-z])(?=.*[A-Z])(?=.*\d)/, message: '需包含大写字母、小写字母和数字' },
              ]}>
              <Input.Password prefix={<LockOutlined />} placeholder="请输入新密码" />
            </Form.Item>
            <Form.Item name="confirmPassword" label="确认新密码"
              dependencies={['newPassword']}
              rules={[
                { required: true, message: '请确认新密码' },
                ({ getFieldValue }) => ({
                  validator(_, value) {
                    if (!value || getFieldValue('newPassword') === value) return Promise.resolve();
                    return Promise.reject(new Error('两次密码不一致'));
                  },
                }),
              ]}>
              <Input.Password prefix={<LockOutlined />} placeholder="请再次输入新密码" />
            </Form.Item>
            <Form.Item>
              <Button type="primary" icon={<LockOutlined />} loading={pwSaving} onClick={async () => {
                try {
                  const values = await pwForm.validateFields();
                  setPwSaving(true);
                  const res = await fetch(`${API_BASE}/api/auth/change-password`, {
                    method: 'POST',
                    credentials: 'include',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ oldPassword: values.oldPassword, newPassword: values.newPassword }),
                  });
                  const data = await res.json();
                  if (res.ok) {
                    message.success('密码修改成功');
                    pwForm.resetFields();
                  } else {
                    message.error(data.msg || data.error || '修改失败');
                  }
                } catch {} finally {
                  setPwSaving(false);
                }
              }}>
                修改密码
              </Button>
            </Form.Item>
          </Form>
        </Card>
      </Col>
    </Row>
  );
};

export default Profile;
