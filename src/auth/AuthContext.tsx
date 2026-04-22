import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { API_BASE } from '../config';

type AuthUser = {
  id: number;
  realName: string;
  username: string;
};

type DataScopes = {
  depts: string[];
  domains: string[];
  platforms: string[];
  shops: string[];
  warehouses: string[];
};

type AuthPayload = {
  dataScopes: DataScopes;
  isSuperAdmin: boolean;
  mustChangePassword: boolean;
  permissions: string[];
  roles: string[];
  user: AuthUser;
};

type LoginParams = {
  password: string;
  username: string;
  remember?: boolean;
  captchaId?: string;
  captchaAnswer?: number;
};

type AuthContextValue = {
  hasPermission: (permission?: string) => boolean;
  isAuthenticated: boolean;
  loading: boolean;
  login: (params: LoginParams) => Promise<AuthPayload>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
  session: AuthPayload | null;
};

const AuthContext = createContext<AuthContextValue | null>(null);

const parseResponse = async (response: Response) => {
  const body = await response.json().catch(err => { console.warn('parseResponse json:', err); return {}; });
  if (!response.ok) {
    throw new Error(body.msg || '请求失败');
  }
  return body.data as AuthPayload;
};

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [session, setSession] = useState<AuthPayload | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const response = await fetch(`${API_BASE}/api/auth/me`, { credentials: 'include' });
      const data = await parseResponse(response);
      setSession(data);
    } catch {
      setSession(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const login = useCallback(async (params: LoginParams) => {
    const response = await fetch(`${API_BASE}/api/auth/login`, {
      body: JSON.stringify(params),
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      method: 'POST',
    });
    const data = await parseResponse(response);
    setSession(data);
    return data;
  }, []);

  const logout = useCallback(async () => {
    try {
      await fetch(`${API_BASE}/api/auth/logout`, { method: 'POST', credentials: 'include' });
    } finally {
      setSession(null);
    }
  }, []);

  const hasPermission = useCallback((permission?: string) => {
    if (!permission) return true;
    if (!session) return false;
    if (session.isSuperAdmin || session.roles.includes('super_admin')) return true;
    return session.permissions.includes(permission);
  }, [session]);

  const value = useMemo<AuthContextValue>(() => ({
    hasPermission,
    isAuthenticated: Boolean(session),
    loading,
    login,
    logout,
    refresh,
    session,
  }), [hasPermission, loading, login, logout, refresh, session]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within AuthProvider');
  }
  return context;
};
