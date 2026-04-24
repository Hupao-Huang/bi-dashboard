import React, { useState } from 'react';
import { Tabs } from 'antd';
import RPAMapping from './RPAMapping';
import RPAMonitor from './RPAMonitor';

const RPAManagement: React.FC = () => {
  const [activeKey, setActiveKey] = useState('mapping');

  return (
    <Tabs
      activeKey={activeKey}
      onChange={setActiveKey}
      items={[
        { key: 'mapping', label: 'RPA文件映射', children: <RPAMapping /> },
        { key: 'monitor', label: 'RPA监控', children: <RPAMonitor /> },
      ]}
      style={{ margin: '-20px -20px 0', padding: '20px 20px 0' }}
    />
  );
};

export default RPAManagement;
