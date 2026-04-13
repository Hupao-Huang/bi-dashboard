import React from 'react';
import { Button, Result } from 'antd';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { getFirstAllowedRoute } from '../navigation';

const ForbiddenPage: React.FC = () => {
  const navigate = useNavigate();
  const { hasPermission } = useAuth();

  return (
    <Result
      status="403"
      title="当前账号暂无页面访问权限"
      subTitle="请联系管理员分配角色或数据范围。"
      extra={(
        <Button type="primary" onClick={() => navigate(getFirstAllowedRoute(hasPermission) || '/login', { replace: true })}>
          返回可访问页面
        </Button>
      )}
    />
  );
};

export default ForbiddenPage;
