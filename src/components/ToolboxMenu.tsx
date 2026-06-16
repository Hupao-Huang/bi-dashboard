import React from 'react';
import { Button, Dropdown } from 'antd';
import type { MenuProps } from 'antd';
import { ToolOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';

// 业务小工具入口聚合。新增工具只在这里加一行: 路由 key + 工具名 label + 分组 group + 权限 permission。
// 没填 permission 的所有人可见; 填了的只有有权限的人能看到。整个工具箱里一个可见工具都没有时, 按钮不显示。
interface ToolItem {
  key: string;        // 点击后跳转的路由
  label: string;      // 工具显示名
  group: string;      // 所属分组 (下拉里的灰色分组标题)
  permission?: string;
}

const TOOLS: ToolItem[] = [
  { key: '/system/yonbip', label: '批量出库', group: 'YS工具', permission: 'system.yonbip:use' },
  { key: '/system/yonbip-purchase', label: '新增采购订单', group: 'YS工具', permission: 'system.yonbip:use' },
  { key: '/system/batch-convert', label: '批次转换', group: 'YS工具', permission: 'system.yonbip:use' },
];

interface ToolboxMenuProps {
  isMobile?: boolean;
}

const ToolboxMenu: React.FC<ToolboxMenuProps> = ({ isMobile }) => {
  const navigate = useNavigate();
  const { hasPermission } = useAuth();

  const visible = TOOLS.filter(t => !t.permission || hasPermission(t.permission));
  if (visible.length === 0) return null;

  // 按分组聚合成二级子菜单 (一级=分组名, 鼠标移上去展开二级工具项; 只有点工具项才跳转)
  const groups = Array.from(new Set(visible.map(t => t.group)));
  const items: MenuProps['items'] = groups.map(g => ({
    key: g,
    label: g,
    children: visible
      .filter(t => t.group === g)
      .map(t => ({ key: t.key, label: t.label })),
  }));

  return (
    <Dropdown
      menu={{ items, onClick: ({ key }) => navigate(key) }}
      placement="bottomRight"
      trigger={['click']}
    >
      <Button
        type="text"
        icon={<ToolOutlined />}
        style={{ color: 'var(--text-tertiary)', fontSize: 14 }}
      >
        {!isMobile && '工具箱'}
      </Button>
    </Dropdown>
  );
};

export default ToolboxMenu;
