import React from 'react';
import { Button, Result } from 'antd';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { getFirstAllowedRoute } from '../navigation';

const NotFoundPage: React.FC = () => {
  const navigate = useNavigate();
  const { hasPermission } = useAuth();

  return (
    <Result
      status="404"
      title="页面不存在"
      subTitle="地址可能已变更或被删除，请检查后重试。"
      extra={(
        <Button type="primary" onClick={() => navigate(getFirstAllowedRoute(hasPermission) || '/', { replace: true })}>
          返回首页
        </Button>
      )}
    />
  );
};

export default NotFoundPage;
