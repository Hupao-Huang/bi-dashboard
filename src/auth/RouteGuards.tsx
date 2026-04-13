import React from 'react';
import { Navigate, Outlet, useLocation } from 'react-router-dom';
import { Spin } from 'antd';
import { useAuth } from './AuthContext';
import { getFirstAllowedRoute } from '../navigation';

const loadingView = (
  <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
    <Spin size="large" />
  </div>
);

export const RequireAuth: React.FC<{ children?: React.ReactNode; permission?: string }> = ({ children, permission }) => {
  const { hasPermission, isAuthenticated, loading } = useAuth();
  const location = useLocation();

  if (loading) return loadingView;
  if (!isAuthenticated) {
    return <Navigate to="/login" replace state={{ from: `${location.pathname}${location.search}` }} />;
  }
  if (permission && !hasPermission(permission)) {
    const fallback = getFirstAllowedRoute(hasPermission) || '/forbidden';
    return <Navigate to={fallback === location.pathname ? '/forbidden' : fallback} replace />;
  }
  return <>{children || <Outlet />}</>;
};

export const PublicOnlyRoute: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { hasPermission, isAuthenticated, loading } = useAuth();

  if (loading) return loadingView;
  if (isAuthenticated) {
    return <Navigate to={getFirstAllowedRoute(hasPermission) || '/forbidden'} replace />;
  }
  return <>{children}</>;
};
