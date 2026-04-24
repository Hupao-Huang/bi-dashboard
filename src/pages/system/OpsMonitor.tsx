import React, { useState } from 'react';
import { Tabs } from 'antd';
import TaskMonitor from './TaskMonitor';
import AuditLog from './AuditLog';

const OpsMonitor: React.FC = () => {
  const [activeKey, setActiveKey] = useState('tasks');

  return (
    <Tabs
      activeKey={activeKey}
      onChange={setActiveKey}
      items={[
        { key: 'tasks', label: '任务监控', children: <TaskMonitor /> },
        { key: 'audit', label: '审计日志', children: <AuditLog /> },
      ]}
      style={{ margin: '-20px -20px 0', padding: '20px 20px 0' }}
    />
  );
};

export default OpsMonitor;
