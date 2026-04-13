import React from 'react';
import { Card } from 'antd';
import { WarningOutlined } from '@ant-design/icons';

const DailyAlerts: React.FC = () => (
  <Card>
    <div style={{ textAlign: 'center', padding: '80px 0', color: '#94a3b8' }}>
      <WarningOutlined style={{ fontSize: 48, marginBottom: 16 }} />
      <div style={{ fontSize: 18, fontWeight: 600, color: '#1e293b', marginBottom: 8 }}>每日预警</div>
      <div style={{ color: '#64748b', fontSize: 13, marginTop: 8 }}>拆单-缺货预警 / 库存预警 / 发货时效预警 / 跨仓发货预警</div>
      <div style={{ marginTop: 4 }}>功能开发中，敬请期待</div>
    </div>
  </Card>
);

export default DailyAlerts;
