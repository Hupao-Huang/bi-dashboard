import React from 'react';
import { Card } from 'antd';
import { ScheduleOutlined } from '@ant-design/icons';

const KAMonthly: React.FC = () => (
  <Card>
    <div style={{ textAlign: 'center', padding: '80px 0', color: 'var(--text-tertiary)' }}>
      <ScheduleOutlined style={{ fontSize: 48, marginBottom: 16 }} />
      <div style={{ fontSize: 18, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 8 }}>KA月度统计</div>
      <div>功能开发中，敬请期待</div>
    </div>
  </Card>
);

export default KAMonthly;
