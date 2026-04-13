import React, { useCallback, useEffect, useState } from 'react';
import { Avatar, Button, Card, Col, Descriptions, Form, Input, message, Row, Tag, Upload } from 'antd';
import { CameraOutlined, LockOutlined, SaveOutlined, UserOutlined } from '@ant-design/icons';
import { API_BASE } from '../../config';

const Profile: React.FC = () => {
  const [profile, setProfile] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [pwSaving, setPwSaving] = useState(false);
  const [form] = Form.useForm();
  const [pwForm] = Form.useForm();

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
      .catch(() => setLoading(false));
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
        message.error(data.error || '保存失败');
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
      message.error(data.error || '上传失败');
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
              style={{ backgroundColor: profile.avatar ? undefined : '#4f46e5' }}
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
            <Descriptions.Item label="最近登录">{profile.lastLoginAt || '-'}</Descriptions.Item>
          </Descriptions>
        </Card>
      </Col>

      <Col xs={24} md={16}>
        <Card title="个人信息">
          <Form form={form} layout="vertical" style={{ maxWidth: 500 }}>
            <Form.Item name="realName" label="真实姓名">
              <Input placeholder="请输入真实姓名" maxLength={20} />
            </Form.Item>
            <Form.Item name="phone" label="手机号"
              rules={[{ pattern: /^1\d{10}$/, message: '请输入正确的手机号' }]}>
              <Input placeholder="请输入手机号" maxLength={11} />
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
                  const res = await fetch(`${API_BASE}/api/change-password`, {
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
                    message.error(data.error || '修改失败');
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
