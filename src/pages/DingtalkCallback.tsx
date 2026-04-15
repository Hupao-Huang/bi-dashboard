import React, { useCallback, useEffect, useState } from 'react';
import { Button, Card, Input, Result, Space, Spin, Typography } from 'antd';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { getFirstAllowedRoute } from '../navigation';
import { API_BASE } from '../config';

const DingtalkCallback: React.FC = () => {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const { refresh } = useAuth();
  const code = searchParams.get('authCode') || searchParams.get('code') || '';
  const state = searchParams.get('state') || '';
  const isBind = state === 'bind';

  const [status, setStatus] = useState<'loading' | 'pending' | 'remark' | 'success' | 'error'>('loading');
  const [message, setMessage] = useState('');
  const [remark, setRemark] = useState('');
  const [pendingToken, setPendingToken] = useState('');
  const [department, setDepartment] = useState('');
  const [nickName, setNickName] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const doLogin = useCallback(async (remarkText?: string) => {
    try {
      const res = await fetch(`${API_BASE}/api/auth/dingtalk/login`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code, remark: remarkText || '' }),
      });
      const body = await res.json();

      if (!res.ok) {
        setStatus('error');
        setMessage(body.msg || '登录失败');
        return;
      }

      if (body.data?.pending) {
        setStatus('pending');
        setMessage(body.data.message || '注册申请已提交');
        return;
      }

      await refresh();
      const payload = body.data;
      const target = getFirstAllowedRoute((permission?: string) => {
        if (!permission) return true;
        if (payload?.isSuperAdmin || payload?.roles?.includes('super_admin')) return true;
        return payload?.permissions?.includes(permission) ?? false;
      }) || '/overview';
      navigate(target, { replace: true });
    } catch {
      setStatus('error');
      setMessage('网络错误，请重试');
    }
  }, [code, navigate, refresh]);

  const doBind = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/api/user/dingtalk`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code }),
      });
      const body = await res.json();

      if (!res.ok) {
        setStatus('error');
        setMessage(body.msg || body.error || '绑定失败');
        return;
      }

      setStatus('success');
      setMessage(body.data?.nick ? `已绑定钉钉账号：${body.data.nick}` : '钉钉绑定成功');
    } catch {
      setStatus('error');
      setMessage('网络错误，请重试');
    }
  }, [code]);

  useEffect(() => {
    if (!code) {
      setStatus('error');
      setMessage('授权码缺失，请重新扫码');
      return;
    }

    if (isBind) {
      doBind();
    } else {
      const tryLogin = async () => {
        try {
          const res = await fetch(`${API_BASE}/api/auth/dingtalk/login`, {
            method: 'POST',
            credentials: 'include',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ code, remark: '' }),
          });
          const body = await res.json();

          if (!res.ok) {
            setStatus('error');
            setMessage(body.msg || '登录失败');
            return;
          }

          if (body.data?.needRemark) {
            if (body.data.pendingToken) setPendingToken(body.data.pendingToken);
            if (body.data.department) setDepartment(body.data.department);
            if (body.data.nick) setNickName(body.data.nick);
            setStatus('remark');
            return;
          }

          if (body.data?.pending) {
            setStatus('pending');
            setMessage(body.data.message || '注册申请已提交');
            return;
          }

          await refresh();
          const payload = body.data;
          const target = getFirstAllowedRoute((permission?: string) => {
            if (!permission) return true;
            if (payload?.isSuperAdmin || payload?.roles?.includes('super_admin')) return true;
            return payload?.permissions?.includes(permission) ?? false;
          }) || '/overview';
          navigate(target, { replace: true });
        } catch {
          setStatus('error');
          setMessage('网络错误，请重试');
        }
      };
      tryLogin();
    }
  }, [code, isBind, navigate, refresh, doBind]);

  const handleSubmitRemark = async () => {
    setSubmitting(true);
    setStatus('loading');
    try {
      const res = await fetch(`${API_BASE}/api/auth/dingtalk/login`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ pendingToken, remark }),
      });
      const body = await res.json();
      if (!res.ok) {
        setStatus('error');
        setMessage(body.msg || '注册失败，请重新扫码');
        return;
      }
      setStatus('pending');
      setMessage(body.data?.message || '注册申请已提交，请等待管理员审批');
    } catch {
      setStatus('error');
      setMessage('网络错误，请重试');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', background: '#f5f7fa', padding: 24 }}>
      <Card style={{ maxWidth: 480, width: '100%', borderRadius: 16 }} styles={{ body: { padding: 32 } }}>
        {status === 'loading' && (
          <div style={{ textAlign: 'center', padding: 40 }}>
            <Spin size="large" />
            <div style={{ marginTop: 16, color: '#666' }}>{isBind ? '正在绑定钉钉...' : '正在验证钉钉身份...'}</div>
          </div>
        )}

        {status === 'success' && (
          <Result
            status="success"
            title="绑定成功"
            subTitle={message}
            extra={<Button type="primary" onClick={() => navigate('/profile', { replace: true })}>返回个人中心</Button>}
          />
        )}

        {status === 'remark' && (
          <div>
            <Result
              status="info"
              title={nickName ? `${nickName}，欢迎使用松鲜鲜工作台` : '欢迎使用松鲜鲜工作台'}
              subTitle="您是新用户，请填写以下信息，提交后等待管理员审批"
            />
            <div style={{ maxWidth: 360, margin: '0 auto' }}>
              {department && (
                <div style={{ padding: '10px 12px', borderRadius: 8, background: '#f0f5ff', border: '1px solid #d6e4ff', marginBottom: 16 }}>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>钉钉通讯录部门</Typography.Text>
                  <div style={{ fontWeight: 600, color: '#1d39c4', marginTop: 2 }}>{department}</div>
                </div>
              )}
              <Typography.Text strong>岗位及数据需求 <Typography.Text type="danger">*</Typography.Text></Typography.Text>
              <Input.TextArea
                rows={3}
                value={remark}
                onChange={e => setRemark(e.target.value)}
                placeholder={"请填写你的岗位和需要查看的数据范围\n例如：运营岗，需要查看天猫和京东店铺数据"}
                style={{ marginTop: 8, marginBottom: 16 }}
              />
              <Button type="primary" block loading={submitting} disabled={!remark.trim()} onClick={handleSubmitRemark}>
                提交注册申请
              </Button>
            </div>
          </div>
        )}

        {status === 'pending' && (
          <Result
            status="success"
            title="注册申请已提交"
            subTitle={message}
            extra={<Button onClick={() => navigate('/login', { replace: true })}>返回登录页</Button>}
          />
        )}

        {status === 'error' && (
          <Result
            status="error"
            title={isBind ? '绑定失败' : '登录失败'}
            subTitle={message}
            extra={
              <Space>
                <Button onClick={() => navigate(isBind ? '/profile' : '/login', { replace: true })}>
                  {isBind ? '返回个人中心' : '返回登录页'}
                </Button>
                <Button type="primary" onClick={() => window.location.reload()}>重试</Button>
              </Space>
            }
          />
        )}
      </Card>
    </div>
  );
};

export default DingtalkCallback;
