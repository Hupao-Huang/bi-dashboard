import React from 'react';
import { Spin } from 'antd';

const PageLoading: React.FC<{ minHeight?: React.CSSProperties['minHeight'] }> = ({ minHeight = 'calc(100vh - 220px)' }) => (
  <div style={{ minHeight, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
    <Spin size="large" />
  </div>
);

export default PageLoading;
