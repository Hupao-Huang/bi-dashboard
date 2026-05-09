import React from 'react';
import type { MenuProps } from 'antd';
import {
  AlertOutlined,
  ApartmentOutlined,
  AppstoreOutlined,
  BarChartOutlined,
  CalendarOutlined,
  CarOutlined,
  ClockCircleOutlined,
  CommentOutlined,
  CustomerServiceOutlined,
  CrownOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  DollarOutlined,
  EyeOutlined,
  FileTextOutlined,
  FundOutlined,
  FlagOutlined,
  GlobalOutlined,
  LineChartOutlined,
  MonitorOutlined,
  NodeIndexOutlined,
  NotificationOutlined,
  RiseOutlined,
  SafetyCertificateOutlined,
  ScheduleOutlined,
  ShareAltOutlined,
  ShopOutlined,
  ShoppingCartOutlined,
  SettingOutlined,
  SyncOutlined,
  TeamOutlined,
  TrademarkOutlined,
  VideoCameraOutlined,
  WarningOutlined,
} from '@ant-design/icons';

type MenuItem = Required<MenuProps>['items'][number];

export type MenuDefinition = {
  children?: MenuDefinition[];
  icon?: React.ReactNode;
  key: string;
  label: string;
  permission?: string;
};

const deptMenuChildren = (prefix: string, permissions: { preview: string; store: string; product: string }, unitLabel: '店铺' | '大区' = '店铺'): MenuDefinition[] => [
  { key: `${prefix}/store-preview`, icon: <EyeOutlined />, label: `${unitLabel}数据预览`, permission: permissions.preview },
  { key: `${prefix}/store-dashboard`, icon: <BarChartOutlined />, label: `${unitLabel}看板`, permission: permissions.store },
  { key: `${prefix}/product-dashboard`, icon: <AppstoreOutlined />, label: '货品看板', permission: permissions.product },
];

export const menuDefinitions: MenuDefinition[] = [
  { key: '/overview', icon: <DashboardOutlined />, label: '综合看板', permission: 'overview:view' },
  { key: '/brand', icon: <TrademarkOutlined />, label: '品牌中心', permission: 'brand:view' },
  {
    key: '/ecommerce',
    icon: <ShoppingCartOutlined />,
    label: '电商部门',
    permission: 'ecommerce:view',
    children: [
      ...deptMenuChildren('/ecommerce', {
        preview: 'ecommerce.store_preview:view',
        store: 'ecommerce.store_dashboard:view',
        product: 'ecommerce.product_dashboard:view',
      }),
      { key: '/ecommerce/marketing-cost', icon: <DollarOutlined />, label: '营销费用', permission: 'ecommerce.marketing_cost:view' },
      { key: '/ecommerce/special-channel-allot', icon: <DollarOutlined />, label: '特殊渠道调拨对账', permission: 'ecommerce.special_channel_allot:view' },
    ],
  },
  {
    key: '/social',
    icon: <GlobalOutlined />,
    label: '社媒部门',
    permission: 'social:view',
    children: [
      ...deptMenuChildren('/social', {
        preview: 'social.store_preview:view',
        store: 'social.store_dashboard:view',
        product: 'social.product_dashboard:view',
      }),
      { key: '/social/feigua', icon: <VideoCameraOutlined />, label: '飞瓜看板', permission: 'social.feigua:view' },
      { key: '/social/marketing', icon: <RiseOutlined />, label: '营销看板', permission: 'social.marketing:view' },
    ],
  },
  {
    key: '/offline',
    icon: <ShopOutlined />,
    label: '线下部门',
    permission: 'offline:view',
    children: [
      ...deptMenuChildren('/offline', {
        preview: 'offline.store_preview:view',
        store: 'offline.store_dashboard:view',
        product: 'offline.product_dashboard:view',
      }, '大区'),
      { key: '/offline/high-value-customers', icon: <CrownOutlined />, label: '高价值客户', permission: 'offline.high_value_customers:view' },
      { key: '/offline/turnover-expiry', icon: <SyncOutlined />, label: '周转率及临期', permission: 'offline.turnover_expiry:view' },
      { key: '/offline/ka-monthly', icon: <ScheduleOutlined />, label: 'KA月度统计', permission: 'offline.ka_monthly:view' },
      { key: '/offline/target-manage', icon: <FlagOutlined />, label: '目标管理', permission: 'offline.target:view' },
    ],
  },
  {
    key: '/distribution',
    icon: <ShareAltOutlined />,
    label: '分销部门',
    permission: 'distribution:view',
    children: [
      ...deptMenuChildren('/distribution', {
        preview: 'distribution.store_preview:view',
        store: 'distribution.store_dashboard:view',
        product: 'distribution.product_dashboard:view',
      }),
      { key: '/distribution/customer-analysis', icon: <CrownOutlined />, label: '客户分析', permission: 'distribution.customer_analysis:view' },
      { key: '/distribution/customer-list-manage', icon: <TeamOutlined />, label: '客户名单管理', permission: 'distribution.customer_list:edit' },
    ],
  },
  {
    key: '/instant-retail',
    icon: <ShoppingCartOutlined />,
    label: '即时零售部',
    permission: 'instant_retail:view',
    children: deptMenuChildren('/instant-retail', {
      preview: 'instant_retail.store_preview:view',
      store: 'instant_retail.store_dashboard:view',
      product: 'instant_retail.product_dashboard:view',
    }),
  },
  {
    key: '/finance',
    icon: <FundOutlined />,
    label: '财务部门',
    permission: 'finance:view',
    children: [
      { key: '/finance/overview', icon: <DashboardOutlined />, label: '利润总览', permission: 'finance.overview:view' },
      { key: '/finance/product-profit', icon: <RiseOutlined />, label: '产品利润统计', permission: 'finance.product_profit:view' },
      { key: '/finance/report', icon: <FileTextOutlined />, label: '财务报表', permission: 'finance.report:view' },
    ],
  },
  {
    key: '/customer',
    icon: <CustomerServiceOutlined />,
    label: '客服部门',
    permission: 'customer:view',
    children: [
      { key: '/customer/overview', icon: <DashboardOutlined />, label: '客服总览', permission: 'customer.overview:view' },
    ],
  },
  {
    key: '/supply-chain',
    icon: <NodeIndexOutlined />,
    label: '供应链管理',
    permission: 'supply_chain:view',
    children: [
      { key: '/supply-chain/plan-dashboard', icon: <DashboardOutlined />, label: '计划看板', permission: 'supply_chain.plan_dashboard:view' },
      { key: '/supply-chain/purchase-plan', icon: <ShoppingCartOutlined />, label: '采购计划', permission: 'supply_chain.plan_dashboard:view' },
      { key: '/supply-chain/inventory-warning', icon: <AlertOutlined />, label: '库存预警', permission: 'supply_chain.inventory_warning:view' },
      { key: '/supply-chain/logistics-analysis', icon: <CarOutlined />, label: '快递仓储分析', permission: 'supply_chain.logistics_analysis:view' },
      { key: '/supply-chain/daily-alerts', icon: <WarningOutlined />, label: '每日预警', permission: 'supply_chain.daily_alerts:view' },
      { key: '/supply-chain/monthly-billing', icon: <FileTextOutlined />, label: '月度账单分析', permission: 'supply_chain.monthly_billing:view' },
    ],
  },
  {
    key: '/system',
    icon: <SettingOutlined />,
    label: '系统设置',
    permission: 'user.manage',
    children: [
      { key: '/system/access', icon: <TeamOutlined />, label: '用户管理', permission: 'user.manage' },
      { key: '/system/roles', icon: <SafetyCertificateOutlined />, label: '角色管理', permission: 'role.manage' },
      { key: '/system/feedback', icon: <CommentOutlined />, label: '反馈管理', permission: 'feedback.manage' },
      { key: '/system/notices', icon: <NotificationOutlined />, label: '公告管理', permission: 'notice.manage' },
      { key: '/system/channels', icon: <ApartmentOutlined />, label: '渠道管理', permission: 'channel.manage' },
      { key: '/system/rpa', icon: <FileTextOutlined />, label: 'RPA管理', permission: 'role.manage' },
      { key: '/system/db-dict', icon: <DatabaseOutlined />, label: '数据库字典', permission: 'role.manage' },
      { key: '/system/ops', icon: <MonitorOutlined />, label: '运维监控', permission: 'role.manage' },
    ],
  },
];

export const pageTitleMap: Record<string, string> = {
  '/overview': '综合看板',
  '/ecommerce': '电商部门',
  '/ecommerce/store-preview': '店铺数据预览',
  '/ecommerce/store-dashboard': '店铺看板',
  '/ecommerce/product-dashboard': '货品看板',
  '/ecommerce/marketing-cost': '营销费用',
  '/ecommerce/special-channel-allot': '特殊渠道调拨对账',
  '/social': '社媒部门',
  '/social/store-preview': '店铺数据预览',
  '/social/store-dashboard': '店铺看板',
  '/social/product-dashboard': '货品看板',
  '/social/feigua': '飞瓜看板',
  '/social/marketing': '营销看板',
  '/offline': '线下部门',
  '/offline/store-preview': '大区数据预览',
  '/offline/store-dashboard': '大区看板',
  '/offline/product-dashboard': '货品看板',
  '/offline/high-value-customers': '高价值客户',
  '/offline/turnover-expiry': '周转率及临期',
  '/offline/ka-monthly': 'KA月度统计',
  '/offline/target-manage': '目标管理',
  '/distribution': '分销部门',
  '/distribution/store-preview': '店铺数据预览',
  '/distribution/store-dashboard': '店铺看板',
  '/distribution/product-dashboard': '货品看板',
  '/distribution/customer-analysis': '客户分析',
  '/distribution/customer-list-manage': '客户名单管理',
  '/instant-retail': '即时零售部',
  '/instant-retail/store-preview': '店铺数据预览',
  '/instant-retail/store-dashboard': '店铺看板',
  '/instant-retail/product-dashboard': '货品看板',
  '/finance': '财务部门',
  '/finance/overview': '利润总览',
  '/finance/department-profit': '部门利润分析',
  '/finance/monthly-profit': '月度利润统计',
  '/finance/product-profit': '产品利润统计',
  '/finance/report': '财务报表',
  '/customer': '客服部门',
  '/customer/overview': '客服总览',
  '/supply-chain': '供应链管理',
  '/supply-chain/plan-dashboard': '计划看板',
  '/supply-chain/purchase-plan': '采购计划',
  '/supply-chain/inventory-warning': '库存预警',
  '/supply-chain/logistics-analysis': '快递仓储分析',
  '/supply-chain/daily-alerts': '每日预警',
  '/supply-chain/monthly-billing': '月度账单分析',
  '/brand': '品牌中心',
  '/system': '系统设置',
  '/system/access': '用户管理',
  '/system/roles': '角色管理',
  '/system/feedback': '反馈管理',
  '/system/notices': '公告管理',
  '/system/channels': '渠道管理',
  '/system/rpa': 'RPA管理',
  '/system/db-dict': '数据库字典',
  '/system/ops': '运维监控',
  '/profile': '个人中心',
  '/forbidden': '无权限',
};

export const deptLabelMap: Record<string, string> = {
  '/ecommerce': '电商部门',
  '/social': '社媒部门',
  '/offline': '线下部门',
  '/distribution': '分销部门',
  '/instant-retail': '即时零售部',
  '/finance': '财务部门',
  '/customer': '客服部门',
  '/supply-chain': '供应链管理',
  '/system': '系统设置',
};

export const routePermissions: Array<{ path: string; permission: string }> = [
  { path: '/overview', permission: 'overview:view' },
  { path: '/brand', permission: 'brand:view' },
  { path: '/ecommerce', permission: 'ecommerce:view' },
  { path: '/ecommerce/store-preview', permission: 'ecommerce.store_preview:view' },
  { path: '/ecommerce/store-dashboard', permission: 'ecommerce.store_dashboard:view' },
  { path: '/ecommerce/product-dashboard', permission: 'ecommerce.product_dashboard:view' },
  { path: '/ecommerce/marketing-cost', permission: 'ecommerce.marketing_cost:view' },
  { path: '/ecommerce/special-channel-allot', permission: 'ecommerce.special_channel_allot:view' },
  { path: '/social', permission: 'social:view' },
  { path: '/social/store-preview', permission: 'social.store_preview:view' },
  { path: '/social/store-dashboard', permission: 'social.store_dashboard:view' },
  { path: '/social/product-dashboard', permission: 'social.product_dashboard:view' },
  { path: '/social/feigua', permission: 'social.feigua:view' },
  { path: '/social/marketing', permission: 'social.marketing:view' },
  { path: '/offline', permission: 'offline:view' },
  { path: '/offline/store-preview', permission: 'offline.store_preview:view' },
  { path: '/offline/store-dashboard', permission: 'offline.store_dashboard:view' },
  { path: '/offline/product-dashboard', permission: 'offline.product_dashboard:view' },
  { path: '/offline/high-value-customers', permission: 'offline.high_value_customers:view' },
  { path: '/offline/turnover-expiry', permission: 'offline.turnover_expiry:view' },
  { path: '/offline/ka-monthly', permission: 'offline.ka_monthly:view' },
  { path: '/offline/target-manage', permission: 'offline.target:view' },
  { path: '/distribution', permission: 'distribution:view' },
  { path: '/distribution/store-preview', permission: 'distribution.store_preview:view' },
  { path: '/distribution/store-dashboard', permission: 'distribution.store_dashboard:view' },
  { path: '/distribution/product-dashboard', permission: 'distribution.product_dashboard:view' },
  { path: '/distribution/customer-analysis', permission: 'distribution.customer_analysis:view' },
  { path: '/distribution/customer-list-manage', permission: 'distribution.customer_list:edit' },
  { path: '/instant-retail', permission: 'instant_retail:view' },
  { path: '/instant-retail/store-preview', permission: 'instant_retail.store_preview:view' },
  { path: '/instant-retail/store-dashboard', permission: 'instant_retail.store_dashboard:view' },
  { path: '/instant-retail/product-dashboard', permission: 'instant_retail.product_dashboard:view' },
  { path: '/finance/overview', permission: 'finance.overview:view' },
  { path: '/finance/department-profit', permission: 'finance.department_profit:view' },
  { path: '/finance/monthly-profit', permission: 'finance.monthly_profit:view' },
  { path: '/finance/product-profit', permission: 'finance.product_profit:view' },
  { path: '/customer/overview', permission: 'customer.overview:view' },
  { path: '/supply-chain/plan-dashboard', permission: 'supply_chain.plan_dashboard:view' },
  { path: '/supply-chain/purchase-plan', permission: 'supply_chain.plan_dashboard:view' },
  { path: '/supply-chain/inventory-warning', permission: 'supply_chain.inventory_warning:view' },
  { path: '/supply-chain/logistics-analysis', permission: 'supply_chain.logistics_analysis:view' },
  { path: '/supply-chain/daily-alerts', permission: 'supply_chain.daily_alerts:view' },
  { path: '/supply-chain/monthly-billing', permission: 'supply_chain.monthly_billing:view' },
  { path: '/system/access', permission: 'user.manage' },
  { path: '/system/roles', permission: 'role.manage' },
  { path: '/system/feedback', permission: 'feedback.manage' },
  { path: '/system/notices', permission: 'notice.manage' },
  { path: '/system/channels', permission: 'channel.manage' },
  { path: '/system/rpa', permission: 'role.manage' },
  { path: '/system/db-dict', permission: 'role.manage' },
  { path: '/system/ops', permission: 'role.manage' },
];

const filterMenuDefinitions = (
  definitions: MenuDefinition[],
  hasPermission: (permission?: string) => boolean,
): MenuDefinition[] => definitions.reduce<MenuDefinition[]>((acc, definition) => {
  const children = definition.children ? filterMenuDefinitions(definition.children, hasPermission) : undefined;
  const visible = hasPermission(definition.permission) || Boolean(children?.length);
  if (!visible) return acc;
  acc.push({ ...definition, children });
  return acc;
}, []);

const toMenuItems = (definitions: MenuDefinition[]): MenuItem[] => definitions.map(definition => ({
  key: definition.key,
  icon: definition.icon,
  label: definition.label,
  children: definition.children ? toMenuItems(definition.children) : undefined,
}));

export const buildMenuItems = (hasPermission: (permission?: string) => boolean): MenuItem[] => (
  toMenuItems(filterMenuDefinitions(menuDefinitions, hasPermission))
);

export const getDefaultOpenKeys = (pathname: string): string[] => {
  const dept = Object.keys(deptLabelMap).find(key => pathname.startsWith(key));
  return dept ? [dept] : [];
};

export const getFirstAllowedRoute = (hasPermission: (permission?: string) => boolean): string | null => {
  const match = routePermissions.find(route => hasPermission(route.permission));
  return match?.path || null;
};
