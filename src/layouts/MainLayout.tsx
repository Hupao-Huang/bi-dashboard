import React, { useEffect, useMemo, useState } from 'react';
import { Avatar, Button, Drawer, Dropdown, Form, Grid, Input, Layout, Menu, Modal, Tooltip, Typography, message } from 'antd';
import {
  CommentOutlined,
  DownOutlined,
  KeyOutlined,
  MenuFoldOutlined,
  MenuOutlined,
  MenuUnfoldOutlined,
  RocketOutlined,
  UserOutlined,
  LogoutOutlined,
} from '@ant-design/icons';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { buildMenuItems, deptLabelMap, getDefaultOpenKeys, pageTitleMap } from '../navigation';
import { API_BASE } from '../config';
import AIToolboxDrawer from '../components/AIToolboxDrawer';
import FeedbackModal from '../components/FeedbackModal';
import NoticeBell from '../components/Noticebell';
import Watermark from '../components/Watermark';

const { Sider, Header, Content } = Layout;

const roleLabelMap: Record<string, string> = {
  super_admin: '超级管理员',
  management: '管理层',
  dept_manager: '部门负责人',
  operator: '运营',
  finance: '财务',
  supply_chain: '供应链',
};

const MainLayout: React.FC = () => {
  const [collapsed, setCollapsed] = useState(false);
  const [aiDrawerOpen, setAiDrawerOpen] = useState(false);
  const [feedbackOpen, setFeedbackOpen] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const screens = Grid.useBreakpoint();
  const isMobile = !screens.md; // <768px
  const navigate = useNavigate();
  const location = useLocation();
  const { hasPermission, logout, refresh, session } = useAuth();

  const currentPath = location.pathname === '/' ? '/overview' : location.pathname;
  const menuItems = useMemo(() => buildMenuItems(hasPermission), [hasPermission]);
  const pageTitle = pageTitleMap[currentPath] || '工作台';
  const deptPrefix = Object.keys(deptLabelMap).find(prefix => currentPath.startsWith(prefix + '/'));
  const deptLabel = deptPrefix ? deptLabelMap[deptPrefix] : null;
  const displayName = session?.user.realName || session?.user.username || '未登录用户';
  const username = session?.user.username || 'guest';
  const primaryRole = session?.roles?.[0];
  const roleLabel = primaryRole ? (roleLabelMap[primaryRole] || primaryRole) : '已登录';
  const avatarText = displayName.trim().slice(0, 1).toUpperCase();
  const [passwordOpen, setPasswordOpen] = useState(false);
  const [passwordLoading, setPasswordLoading] = useState(false);
  const [pwForm] = Form.useForm();

  const forceChange = session?.mustChangePassword ?? false;
  const [noPassword, setNoPassword] = useState(false);
  const needSetPassword = forceChange || noPassword;

  // 会话超时：2小时无操作自动登出 + 每5分钟检查session有效性
  useEffect(() => {
    const IDLE_MS = 2 * 60 * 60 * 1000;
    const CHECK_MS = 5 * 60 * 1000;
    let lastActivity = Date.now();
    const updateActivity = () => { lastActivity = Date.now(); };
    const events = ['mousemove', 'keydown', 'click', 'scroll', 'touchstart'];
    events.forEach(e => window.addEventListener(e, updateActivity, { passive: true }));
    const interval = setInterval(async () => {
      if (Date.now() - lastActivity > IDLE_MS) {
        await logout();
        navigate('/login', { replace: true });
        return;
      }
      try {
        const res = await fetch(`${API_BASE}/api/auth/me`, { credentials: 'include' });
        if (!res.ok) throw new Error();
      } catch {
        await logout();
        navigate('/login', { replace: true });
      }
    }, CHECK_MS);
    return () => {
      clearInterval(interval);
      events.forEach(e => window.removeEventListener(e, updateActivity));
    };
  }, [logout, navigate]);

  useEffect(() => {
    if (session) {
      fetch(`${API_BASE}/api/user/profile`, { credentials: 'include' })
        .then(res => res.json())
        .then(res => {
          const data = res.data || res;
          if (data.hasPassword === false) setNoPassword(true);
        })
        .catch(err => console.warn('MainLayout profile:', err));
    }
  }, [session]);

  useEffect(() => {
    if (needSetPassword) setPasswordOpen(true);
  }, [needSetPassword]);

  // 页面浏览审计
  useEffect(() => {
    if (!session || currentPath === '/overview') return;
    const controller = new AbortController();
    fetch(`${API_BASE}/api/audit/page-view`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: currentPath }),
      signal: controller.signal,
    }).catch(() => {});
    return () => controller.abort();
  }, [session, currentPath]);

  const handleChangePassword = async () => {
    try {
      const values = await pwForm.validateFields();
      setPasswordLoading(true);
      const res = await fetch(`${API_BASE}/api/auth/change-password`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ oldPassword: values.oldPassword, newPassword: values.newPassword }),
      });
      if (!res.ok) {
        const body = await res.json().catch(err => { console.warn('MainLayout change-password json:', err); return {}; });
        throw new Error(body.msg || '修改失败');
      }
      if (needSetPassword) {
        message.success('密码设置成功，现在可以用手机号+密码登录');
        pwForm.resetFields();
        setNoPassword(false);
        await refresh();
      } else {
        message.success('密码修改成功');
        setPasswordOpen(false);
        pwForm.resetFields();
      }
    } catch (err: unknown) {
      if (err instanceof Error && err.message !== 'Validation failed') {
        message.error(err.message);
      }
    } finally {
      setPasswordLoading(false);
    }
  };

  const userMenuItems = [
    { key: 'account', label: <span style={{ color: '#64748b' }}>账号：@{username}</span>, disabled: true },
    { key: 'role', label: <span style={{ color: '#64748b' }}>角色：{roleLabel}</span>, disabled: true },
    { type: 'divider' as const },
    { key: 'profile', label: '个人中心', icon: <UserOutlined /> },
    { key: 'change-password', label: '修改密码', icon: <KeyOutlined /> },
    { key: 'logout', label: '退出登录', icon: <LogoutOutlined /> },
  ];

  const siderWidth = 240;
  const collapsedWidth = 72;

  const menuContent = (
    <>
      <div style={{
        height: 56,
        padding: '0 16px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: collapsed ? 'center' : 'flex-start',
        gap: 10,
        borderBottom: '1px solid #f0f0f0',
        flexShrink: 0,
      }}>
        <div style={{
          width: 32,
          height: 32,
          borderRadius: 10,
          background: 'linear-gradient(135deg, #1e40af 0%, #f59e0b 100%)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontSize: 13,
          fontWeight: 800,
          color: '#fff',
          letterSpacing: -0.5,
          flexShrink: 0,
          boxShadow: '0 6px 14px rgba(30, 64, 175, 0.22)',
        }}>
          SXX
        </div>
        {(!collapsed || isMobile) && (
          <span style={{ color: '#1e293b', fontSize: 16, fontWeight: 700, letterSpacing: 0.3 }}>
            松鲜鲜工作台
          </span>
        )}
      </div>

      <div style={{ flex: 1, overflow: 'auto', marginTop: 8 }}>
        <Menu
          mode="inline"
          selectedKeys={[currentPath]}
          defaultOpenKeys={collapsed && !isMobile ? [] : getDefaultOpenKeys(currentPath)}
          items={menuItems}
          onClick={({ key }) => {
            navigate(key);
            if (isMobile) setMobileMenuOpen(false);
          }}
        />
      </div>

      {/* 侧边栏底部功能套区 (v1.15: 仅保留 用户菜单) */}
      <div style={{
        borderTop: '1px solid #f0f0f0',
        padding: '6px 0',
        display: 'flex',
        flexDirection: 'column',
        gap: 0,
        flexShrink: 0,
        background: '#fafbfc',
      }}>
        {/* 用户菜单 */}
        <Dropdown
          menu={{
            items: userMenuItems,
            onClick: async ({ key }) => {
              if (key === 'profile') { navigate('/profile'); return; }
              if (key === 'change-password') { setPasswordOpen(true); return; }
              if (key !== 'logout') return;
              await logout();
              navigate('/login', { replace: true });
            },
          }}
          trigger={['click']}
          placement="topRight"
        >
          <div style={{
            padding: collapsed && !isMobile ? '10px 0' : '10px 14px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: collapsed && !isMobile ? 'center' : 'flex-start',
            gap: 10,
            cursor: 'pointer',
            borderTop: '1px solid #e2e8f0',
            transition: 'background 0.15s',
          }}
            onMouseEnter={e => (e.currentTarget.style.background = '#f1f5f9')}
            onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
          >
            <Avatar size={28} style={{ background: '#e2e8f0', color: '#334155', fontWeight: 700, flexShrink: 0 }}>
              {avatarText}
            </Avatar>
            {(!collapsed || isMobile) && (
              <>
                <div style={{ flex: 1, minWidth: 0, lineHeight: 1.2 }}>
                  <div style={{ color: '#0f172a', fontSize: 13, fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {displayName}
                  </div>
                  <div style={{ color: '#94a3b8', fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {roleLabel} · @{username}
                  </div>
                </div>
                <DownOutlined style={{ fontSize: 10, color: '#cbd5e1', flexShrink: 0 }} />
              </>
            )}
          </div>
        </Dropdown>
      </div>
    </>
  );

  return (
    <Layout style={{ minHeight: '100vh' }}>
      {/* Mobile: Drawer sidebar */}
      {isMobile ? (
        <Drawer
          open={mobileMenuOpen}
          onClose={() => setMobileMenuOpen(false)}
          placement="left"
          width={260}
          styles={{ body: { padding: 0, display: 'flex', flexDirection: 'column', height: '100%' } }}
          closeIcon={null}
        >
          {menuContent}
        </Drawer>
      ) : (
        <Sider
          collapsible
          collapsed={collapsed}
          onCollapse={setCollapsed}
          width={siderWidth}
          collapsedWidth={collapsedWidth}
          trigger={null}
          className="bi-sider"
          style={{
            overflow: 'auto',
            height: '100vh',
            position: 'fixed',
            left: 0,
            top: 0,
            bottom: 0,
            zIndex: 10,
            display: 'flex',
            flexDirection: 'column',
          }}
        >
          {menuContent}
          <div
            onClick={() => setCollapsed(!collapsed)}
            style={{
              height: 44,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              cursor: 'pointer',
              color: '#94a3b8',
              borderTop: '1px solid #f0f0f0',
              transition: 'color 0.2s',
              flexShrink: 0,
            }}
            onMouseEnter={e => (e.currentTarget.style.color = '#1e40af')}
            onMouseLeave={e => (e.currentTarget.style.color = '#94a3b8')}
          >
            {collapsed ? <MenuUnfoldOutlined style={{ fontSize: 16 }} /> : <MenuFoldOutlined style={{ fontSize: 16 }} />}
          </div>
        </Sider>
      )}

      <Layout style={{
        marginLeft: isMobile ? 0 : (collapsed ? collapsedWidth : siderWidth),
        height: '100vh',
        overflow: 'hidden',
        transition: isMobile ? 'none' : 'margin-left 0.25s cubic-bezier(0.4, 0, 0.2, 1)',
      }}>
        <Header
          className="bi-header"
          style={{
            background: '#ffffff',
            padding: isMobile ? '0 12px' : '0 24px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            height: 56,
            lineHeight: '56px',
            flexShrink: 0,
            gap: 8,
          }}
        >
          {isMobile && (
            <Button
              type="text"
              icon={<MenuOutlined />}
              onClick={() => setMobileMenuOpen(true)}
              style={{ fontSize: 18, color: '#334155', marginRight: 4 }}
            />
          )}
          {deptLabel ? (
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <span style={{ fontSize: 14, color: '#64748b', fontWeight: 500 }}>{deptLabel}</span>
              <span style={{ color: '#cbd5e1', fontSize: 14, fontWeight: 300 }}>/</span>
              <h2 style={{ margin: 0, fontSize: 15, fontWeight: 600, color: '#0f172a', letterSpacing: 0.2 }}>{pageTitle}</h2>
            </div>
          ) : (
            <h2 style={{ margin: 0, fontSize: 15, fontWeight: 600, color: '#0f172a', letterSpacing: 0.2 }}>{pageTitle}</h2>
          )}

          <div style={{ display: 'flex', alignItems: 'center', gap: isMobile ? 4 : 12 }}>
            <Button
              type="text"
              icon={<RocketOutlined />}
              onClick={() => setAiDrawerOpen(true)}
              style={{ color: '#64748b', fontSize: 14 }}
            >
              {!isMobile && 'AI工具箱'}
            </Button>
            <NoticeBell />
            <Tooltip title="问题反馈">
              <Button
                type="text"
                icon={<CommentOutlined />}
                onClick={() => setFeedbackOpen(true)}
                style={{ color: '#64748b', fontSize: 16 }}
              />
            </Tooltip>
          </div>
        </Header>

        <div id="bi-toolbar-slot" className="bi-toolbar-slot" />

        <Content style={{ margin: 0, padding: isMobile ? 12 : 20, background: '#f5f7fa', flex: 1, overflow: 'auto', minHeight: 0 }}>
          <Outlet />
        </Content>
      </Layout>
      <AIToolboxDrawer open={aiDrawerOpen} onClose={() => setAiDrawerOpen(false)} />
      <FeedbackModal open={feedbackOpen} onClose={() => setFeedbackOpen(false)} />
      <Watermark text={`松鲜鲜工作台 · ${displayName}`} subtext={new Date().toLocaleDateString('zh-CN')} />
      <Modal
        title={noPassword ? '设置登录密码' : forceChange ? '首次登录，请修改密码' : '修改密码'}
        open={passwordOpen}
        onCancel={needSetPassword ? undefined : () => { setPasswordOpen(false); pwForm.resetFields(); }}
        onOk={handleChangePassword}
        confirmLoading={passwordLoading}
        okText={noPassword ? '设置密码' : '确认修改'}
        cancelText={needSetPassword ? undefined : '取消'}
        closable={!needSetPassword}
        mask={{ closable: !needSetPassword }}
        keyboard={!needSetPassword}
        footer={needSetPassword ? [
          <Button key="ok" type="primary" loading={passwordLoading} onClick={handleChangePassword}>{noPassword ? '设置密码' : '确认修改'}</Button>,
        ] : undefined}
        destroyOnHidden
      >
        {noPassword && (
          <Typography.Paragraph type="secondary" style={{ marginBottom: 16 }}>
            设置密码后即可使用手机号 + 密码登录，也可以继续使用钉钉扫码登录。
          </Typography.Paragraph>
        )}
        <Form form={pwForm} layout="vertical" style={{ marginTop: noPassword ? 0 : 16 }}>
          {!noPassword && (
            <Form.Item name="oldPassword" label="当前密码" rules={[{ required: true, message: '请输入当前密码' }]}>
              <Input.Password placeholder="请输入当前密码" />
            </Form.Item>
          )}
          <Form.Item name="newPassword" label="新密码" rules={[{ required: true, message: '请输入新密码' }, { min: 8, message: '至少8位' }, { pattern: /^(?=.*[a-z])(?=.*[A-Z])(?=.*\d)/, message: '需包含大写字母、小写字母和数字' }]}>
            <Input.Password placeholder="请输入新密码" />
          </Form.Item>
          <Form.Item
            name="confirmPassword"
            label="确认新密码"
            dependencies={['newPassword']}
            rules={[
              { required: true, message: '请确认新密码' },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  if (!value || getFieldValue('newPassword') === value) return Promise.resolve();
                  return Promise.reject(new Error('两次密码不一致'));
                },
              }),
            ]}
          >
            <Input.Password placeholder="再次输入新密码" />
          </Form.Item>
        </Form>
      </Modal>
    </Layout>
  );
};

export default MainLayout;
