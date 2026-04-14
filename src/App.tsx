import React, { Suspense, lazy } from 'react';
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { ConfigProvider, Spin } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import dayjs from 'dayjs';
import 'dayjs/locale/zh-cn';
import MainLayout from './layouts/MainLayout';
import { AuthProvider, useAuth } from './auth/AuthContext';
import { PublicOnlyRoute, RequireAuth } from './auth/RouteGuards';
import { getFirstAllowedRoute } from './navigation';

const OverviewPage = lazy(() => import('./pages/overview'));
const EcommercePage = lazy(() => import('./pages/ecommerce'));
const EcommerceStorePreview = lazy(() => import('./pages/ecommerce/StorePreview'));
const EcommerceStoreDashboard = lazy(() => import('./pages/ecommerce/StoreDashboard'));
const EcommerceProductDashboard = lazy(() => import('./pages/ecommerce/ProductDashboard'));
const EcommerceMarketingCost = lazy(() => import('./pages/ecommerce/MarketingCost'));
const SocialPage = lazy(() => import('./pages/social'));
const SocialStorePreview = lazy(() => import('./pages/social/StorePreview'));
const SocialStoreDashboard = lazy(() => import('./pages/social/StoreDashboard'));
const SocialProductDashboard = lazy(() => import('./pages/social/ProductDashboard'));
const SocialFeiguaDashboard = lazy(() => import('./pages/social/FeiguaDashboard'));
const SocialMarketingDashboard = lazy(() => import('./pages/social/MarketingDashboard'));
const OfflinePage = lazy(() => import('./pages/offline'));
const OfflineStorePreview = lazy(() => import('./pages/offline/StorePreview'));
const OfflineStoreDashboard = lazy(() => import('./pages/offline/StoreDashboard'));
const OfflineProductDashboard = lazy(() => import('./pages/offline/ProductDashboard'));
const HighValueCustomers = lazy(() => import('./pages/offline/HighValueCustomers'));
const TurnoverExpiry = lazy(() => import('./pages/offline/TurnoverExpiry'));
const KAMonthly = lazy(() => import('./pages/offline/KAMonthly'));
const DistributionPage = lazy(() => import('./pages/distribution'));
const DistributionStorePreview = lazy(() => import('./pages/distribution/StorePreview'));
const DistributionStoreDashboard = lazy(() => import('./pages/distribution/StoreDashboard'));
const DistributionProductDashboard = lazy(() => import('./pages/distribution/ProductDashboard'));
const FinanceOverview = lazy(() => import('./pages/finance/Overview'));
const FinanceDepartmentProfit = lazy(() => import('./pages/finance/DepartmentProfit'));
const FinanceMonthlyProfit = lazy(() => import('./pages/finance/MonthlyProfit'));
const FinanceProductProfit = lazy(() => import('./pages/finance/ProductProfit'));
const FinanceExpenseControl = lazy(() => import('./pages/finance/ExpenseControl'));
const CustomerOverview = lazy(() => import('./pages/customer/Overview'));
const InventoryWarning = lazy(() => import('./pages/supply-chain/InventoryWarning'));
const PlanDashboard = lazy(() => import('./pages/supply-chain/PlanDashboard'));
const LogisticsAnalysis = lazy(() => import('./pages/supply-chain/LogisticsAnalysis'));
const DailyAlerts = lazy(() => import('./pages/supply-chain/DailyAlerts'));
const MonthlyBilling = lazy(() => import('./pages/supply-chain/MonthlyBilling'));
const LoginPage = lazy(() => import('./pages/Login'));
const ForbiddenPage = lazy(() => import('./pages/Forbidden'));
const UserAccessPage = lazy(() => import('./pages/system/UserAccess'));
const RoleAccessPage = lazy(() => import('./pages/system/RoleAccess'));
const TaskMonitorPage = lazy(() => import('./pages/system/TaskMonitor'));
const FeedbackPage = lazy(() => import('./pages/system/Feedback'));
const NoticesPage = lazy(() => import('./pages/system/Notices'));
const ProfilePage = lazy(() => import('./pages/system/Profile'));
const ChannelManagementPage = lazy(() => import('./pages/system/ChannelManagement'));

dayjs.locale('zh-cn');

const routeFallback = (
  <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
    <Spin size="large" />
  </div>
);

const LandingPage: React.FC = () => {
  const { hasPermission } = useAuth();
  return <Navigate to={getFirstAllowedRoute(hasPermission) || '/forbidden'} replace />;
};

const guard = (permission: string, element: React.ReactNode) => (
  <RequireAuth permission={permission}>{element}</RequireAuth>
);

const App: React.FC = () => (
  <ConfigProvider locale={zhCN}>
    <BrowserRouter>
      <AuthProvider>
        <Suspense fallback={routeFallback}>
          <Routes>
            <Route path="/login" element={<PublicOnlyRoute><LoginPage /></PublicOnlyRoute>} />
            <Route element={<RequireAuth><MainLayout /></RequireAuth>}>
              <Route path="/" element={<LandingPage />} />
              <Route path="/forbidden" element={<ForbiddenPage />} />

              <Route path="/overview" element={guard('overview:view', <OverviewPage />)} />

              <Route path="/ecommerce" element={guard('ecommerce:view', <EcommercePage />)} />
              <Route path="/ecommerce/store-preview" element={guard('ecommerce.store_preview:view', <EcommerceStorePreview />)} />
              <Route path="/ecommerce/store-dashboard" element={guard('ecommerce.store_dashboard:view', <EcommerceStoreDashboard />)} />
              <Route path="/ecommerce/product-dashboard" element={guard('ecommerce.product_dashboard:view', <EcommerceProductDashboard />)} />
              <Route path="/ecommerce/marketing-cost" element={guard('ecommerce.marketing_cost:view', <EcommerceMarketingCost />)} />

              <Route path="/social" element={guard('social:view', <SocialPage />)} />
              <Route path="/social/store-preview" element={guard('social.store_preview:view', <SocialStorePreview />)} />
              <Route path="/social/store-dashboard" element={guard('social.store_dashboard:view', <SocialStoreDashboard />)} />
              <Route path="/social/product-dashboard" element={guard('social.product_dashboard:view', <SocialProductDashboard />)} />
              <Route path="/social/feigua" element={guard('social.feigua:view', <SocialFeiguaDashboard />)} />
              <Route path="/social/marketing" element={guard('social.marketing:view', <SocialMarketingDashboard />)} />

              <Route path="/offline" element={guard('offline:view', <OfflinePage />)} />
              <Route path="/offline/store-preview" element={guard('offline.store_preview:view', <OfflineStorePreview />)} />
              <Route path="/offline/store-dashboard" element={guard('offline.store_dashboard:view', <OfflineStoreDashboard />)} />
              <Route path="/offline/product-dashboard" element={guard('offline.product_dashboard:view', <OfflineProductDashboard />)} />
              <Route path="/offline/high-value-customers" element={guard('offline.high_value_customers:view', <HighValueCustomers />)} />
              <Route path="/offline/turnover-expiry" element={guard('offline.turnover_expiry:view', <TurnoverExpiry />)} />
              <Route path="/offline/ka-monthly" element={guard('offline.ka_monthly:view', <KAMonthly />)} />

              <Route path="/distribution" element={guard('distribution:view', <DistributionPage />)} />
              <Route path="/distribution/store-preview" element={guard('distribution.store_preview:view', <DistributionStorePreview />)} />
              <Route path="/distribution/store-dashboard" element={guard('distribution.store_dashboard:view', <DistributionStoreDashboard />)} />
              <Route path="/distribution/product-dashboard" element={guard('distribution.product_dashboard:view', <DistributionProductDashboard />)} />

              <Route path="/finance/overview" element={guard('finance.overview:view', <FinanceOverview />)} />
              <Route path="/finance/department-profit" element={guard('finance.department_profit:view', <FinanceDepartmentProfit />)} />
              <Route path="/finance/monthly-profit" element={guard('finance.monthly_profit:view', <FinanceMonthlyProfit />)} />
              <Route path="/finance/product-profit" element={guard('finance.product_profit:view', <FinanceProductProfit />)} />
              <Route path="/finance/expense-control" element={guard('finance.expense:view', <FinanceExpenseControl />)} />
              <Route path="/customer/overview" element={guard('customer.overview:view', <CustomerOverview />)} />

              <Route path="/supply-chain/inventory-warning" element={guard('supply_chain.inventory_warning:view', <InventoryWarning />)} />
              <Route path="/supply-chain/plan-dashboard" element={guard('supply_chain.plan_dashboard:view', <PlanDashboard />)} />
              <Route path="/supply-chain/logistics-analysis" element={guard('supply_chain.logistics_analysis:view', <LogisticsAnalysis />)} />
              <Route path="/supply-chain/daily-alerts" element={guard('supply_chain.daily_alerts:view', <DailyAlerts />)} />
              <Route path="/supply-chain/monthly-billing" element={guard('supply_chain.monthly_billing:view', <MonthlyBilling />)} />
              <Route path="/supply-chain/purchase-plan" element={guard('supply_chain.purchase_plan:view', <div style={{ textAlign: 'center', padding: 80, color: '#94a3b8' }}>采购计划 - 开发中</div>)} />
              <Route path="/brand" element={guard('brand:view', <div style={{ textAlign: 'center', padding: 80, color: '#94a3b8' }}>品牌中心 - 待开发</div>)} />
              <Route path="/system/access" element={guard('user.manage', <UserAccessPage />)} />
              <Route path="/system/roles" element={guard('role.manage', <RoleAccessPage />)} />
              <Route path="/system/tasks" element={guard('role.manage', <TaskMonitorPage />)} />
              <Route path="/system/feedback" element={guard('feedback.manage', <FeedbackPage />)} />
              <Route path="/system/notices" element={guard('notice.manage', <NoticesPage />)} />
              <Route path="/system/channels" element={guard('channel.manage', <ChannelManagementPage />)} />
              <Route path="/profile" element={<ProfilePage />} />
            </Route>
          </Routes>
        </Suspense>
      </AuthProvider>
    </BrowserRouter>
  </ConfigProvider>
);

export default App;
