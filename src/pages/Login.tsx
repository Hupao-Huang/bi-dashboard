import React, { useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Form,
  Input,
  Space,
  Typography,
} from 'antd';
import {
  LockOutlined,
  SafetyCertificateOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { getFirstAllowedRoute } from '../navigation';
import SliderCaptcha from '../components/SliderCaptcha';

const pageStyle: React.CSSProperties = {
  minHeight: '100vh',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  padding: 24,
  background: '#f5f7fa',
};

const wrapStyle: React.CSSProperties = {
  width: '100%',
  maxWidth: 420,
};

const cardStyle: React.CSSProperties = {
  borderRadius: 20,
  borderColor: '#edf0f4',
  boxShadow: '0 12px 30px rgba(15, 23, 42, 0.06)',
};

const brandStyle: React.CSSProperties = {
  width: 44,
  height: 44,
  borderRadius: 14,
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  background: 'linear-gradient(135deg, #4f6bff 0%, #7aa2ff 100%)',
  color: '#fff',
  fontSize: 18,
  fontWeight: 700,
  boxShadow: '0 10px 20px rgba(79, 107, 255, 0.2)',
};

const noteStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 8,
  padding: '10px 12px',
  borderRadius: 12,
  background: '#f8fafc',
  border: '1px solid #edf0f4',
  color: '#667085',
  fontSize: 12,
  marginBottom: 20,
};

const LoginPage: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { login } = useAuth();
  const [form] = Form.useForm();
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [captchaOpen, setCaptchaOpen] = useState(false);

  const handleLoginClick = async () => {
    try {
      await form.validateFields();
      setError('');
      setCaptchaOpen(true);
    } catch {
      // 表单校验失败，不弹验证码
    }
  };

  const handleCaptchaSuccess = async (captchaId: string, captchaAnswer: number) => {
    setCaptchaOpen(false);
    setSubmitting(true);
    setError('');
    const values = form.getFieldsValue();
    try {
      const payload = await login({
        username: values.username.trim(),
        password: values.password,
        remember: values.remember,
        captchaId,
        captchaAnswer,
      });
      const target = typeof location.state?.from === 'string'
        ? location.state.from
        : (getFirstAllowedRoute((permission?: string) => {
          if (!permission) return true;
          if (payload.isSuperAdmin || payload.roles.includes('super_admin')) return true;
          return payload.permissions.includes(permission);
        }) || '/forbidden');
      navigate(target, { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : '登录失败');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div style={pageStyle}>
      <div style={wrapStyle}>
        <Card styles={{ body: { padding: 28 } }} style={cardStyle}>
          <Space direction="vertical" size={18} style={{ width: '100%' }}>
            <Space size={14} align="start">
              <div style={brandStyle}>BI</div>
              <div>
                <Typography.Title level={3} style={{ margin: 0, color: '#1f2937' }}>
                  登录系统
                </Typography.Title>
                <Typography.Paragraph style={{ margin: '6px 0 0', color: '#667085' }}>
                  使用公司账号进入松鲜鲜工作台。
                </Typography.Paragraph>
              </div>
            </Space>

            <div style={noteStyle}>
              <SafetyCertificateOutlined />
              <span>登录后按账号角色控制菜单、字段与数据范围。</span>
            </div>

            {error && <Alert type="error" showIcon message={error} />}

            <Form
              form={form}
              layout="vertical"
              autoComplete="off"
              initialValues={{ remember: true }}
            >
              <Form.Item label="账号" name="username" rules={[{ required: true, message: '请输入账号' }]}>
                <Input
                  size="large"
                  prefix={<UserOutlined />}
                  placeholder="请输入账号"
                />
              </Form.Item>

              <Form.Item label="密码" name="password" rules={[{ required: true, message: '请输入密码' }]}>
                <Input.Password
                  size="large"
                  prefix={<LockOutlined />}
                  placeholder="请输入密码"
                  onPressEnter={handleLoginClick}
                />
              </Form.Item>

              <div style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                gap: 12,
                margin: '-4px 0 8px',
                color: '#98a2b3',
                fontSize: 12,
              }}>
                <Form.Item name="remember" valuePropName="checked" noStyle>
                  <Checkbox>保持登录（7天）</Checkbox>
                </Form.Item>
                <span>如无法登录，请联系系统管理员。</span>
              </div>

              <Button
                type="primary"
                block
                size="large"
                loading={submitting}
                onClick={handleLoginClick}
                style={{ height: 44, marginTop: 8 }}
              >
                {submitting ? '登录中...' : '登 录'}
              </Button>
            </Form>
          </Space>
        </Card>
      </div>

      <SliderCaptcha
        open={captchaOpen}
        onSuccess={handleCaptchaSuccess}
        onCancel={() => setCaptchaOpen(false)}
      />
    </div>
  );
};

export default LoginPage;
