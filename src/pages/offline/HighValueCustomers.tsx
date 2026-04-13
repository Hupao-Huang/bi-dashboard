import React from 'react';
import { Card } from 'antd';
import { CrownOutlined } from '@ant-design/icons';

const HighValueCustomers: React.FC = () => (
  <Card>
    <div style={{ textAlign: 'center', padding: '80px 0', color: '#94a3b8' }}>
      <CrownOutlined style={{ fontSize: 48, marginBottom: 16 }} />
      <div style={{ fontSize: 18, fontWeight: 600, color: '#1e293b', marginBottom: 8 }}>高价值客户</div>
      <div>功能开发中，敬请期待</div>
    </div>
  </Card>
);

export default HighValueCustomers;
