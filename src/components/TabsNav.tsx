import React, { useCallback, useEffect, useState } from 'react';
import { Tabs, Dropdown } from 'antd';
import type { MenuProps } from 'antd';
import { useLocation, useNavigate } from 'react-router-dom';
import { pageTitleMap, deptLabelMap, deptShortMap } from '../navigation';

// 浏览器式多页签: 打开过的页面排成一行标签, 可切换 / 关闭 / 关其他 / 关全部, 刷新后保持。
// 不做 keep-alive (BI 看板满屏 ECharts, display:none 缓存会让图表尺寸错乱), 切换=正常路由重渲染。

interface TabItem { path: string; title: string; }

const HOME_PATH = '/overview';
const STORAGE_KEY = 'bi_open_tabs_v1';
// 不收进标签页的路由 (落地重定向 / 登录 / 钉钉回调)
const SKIP = new Set(['/', '/login']);

// 选中标签更醒目: 浅蓝底 + 顶部主题蓝条 + 蓝字加粗 (主题色 #1e40af, 跟侧边栏选中态一致)
const TABS_CSS = `
.bi-tabsnav .ant-tabs-tab.ant-tabs-tab-active {
  background: rgba(30, 64, 175, 0.08) !important;
  border-top: 2px solid #1e40af !important;
}
.bi-tabsnav .ant-tabs-tab.ant-tabs-tab-active .ant-tabs-tab-btn {
  color: #1e40af !important;
  font-weight: 600 !important;
}
`;

// 标签名 = 「短部门名·页面名」, 跨部门同名页(店铺看板/货品看板等)靠部门前缀区分。
// 模块落地页本身(path===根)和无部门归属的页(综合看板/个人中心)不加前缀, 用自己的名字。
function titleOf(path: string): string {
  const root = Object.keys(deptShortMap).find((p) => path === p || path.startsWith(p + '/'));
  const leaf = pageTitleMap[path];
  if (!root) return leaf || '页面';
  if (path === root) return leaf || deptLabelMap[root] || deptShortMap[root];
  return `${deptShortMap[root]}·${leaf || '页面'}`;
}

const TabsNav: React.FC = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const path = location.pathname;
  const skip = SKIP.has(path) || path.startsWith('/dingtalk');

  const [tabs, setTabs] = useState<TabItem[]>(() => {
    try {
      const saved = JSON.parse(sessionStorage.getItem(STORAGE_KEY) || 'null');
      // 标签名按当前路径重算(不信存档里的旧 title), 改了命名规则后已开标签也立即生效
      if (Array.isArray(saved) && saved.length) {
        return saved.filter((t) => t && t.path).map((t) => ({ path: t.path, title: titleOf(t.path) }));
      }
    } catch { /* ignore */ }
    return [{ path: HOME_PATH, title: titleOf(HOME_PATH) }];
  });

  // 当前路由进标签 (已存在则只切换激活)
  useEffect(() => {
    if (skip) return;
    setTabs((prev) => (prev.some((t) => t.path === path) ? prev : [...prev, { path, title: titleOf(path) }]));
  }, [path, skip]);

  // 持久化, 刷新保持
  useEffect(() => {
    try { sessionStorage.setItem(STORAGE_KEY, JSON.stringify(tabs)); } catch { /* ignore */ }
  }, [tabs]);

  const closeTab = useCallback((target: string) => {
    setTabs((prev) => {
      if (prev.length <= 1) return prev; // 至少留一个
      const idx = prev.findIndex((t) => t.path === target);
      const next = prev.filter((t) => t.path !== target);
      // 关掉的是当前页 → 跳到相邻标签
      if (target === path && next.length) {
        navigate(next[Math.min(idx, next.length - 1)].path);
      }
      return next;
    });
  }, [path, navigate]);

  const closeOthers = useCallback((keep: string) => {
    setTabs((prev) => prev.filter((t) => t.path === keep));
    if (path !== keep) navigate(keep);
  }, [path, navigate]);

  const closeAll = useCallback(() => {
    setTabs([{ path: HOME_PATH, title: titleOf(HOME_PATH) }]);
    navigate(HOME_PATH);
  }, [navigate]);

  if (tabs.length === 0) return null;

  const ctxMenu = (target: string): MenuProps['items'] => [
    { key: 'close', label: '关闭', disabled: tabs.length <= 1, onClick: () => closeTab(target) },
    { key: 'others', label: '关闭其他', disabled: tabs.length <= 1, onClick: () => closeOthers(target) },
    { key: 'all', label: '关闭全部', onClick: closeAll },
  ];

  return (
    <div className="bi-tabsnav" style={{ background: '#fff', borderBottom: '1px solid #f0f0f0', padding: '4px 12px 0', flexShrink: 0 }}>
      <style>{TABS_CSS}</style>
      <Tabs
        type="editable-card"
        hideAdd
        size="small"
        activeKey={path}
        onChange={(k) => navigate(k)}
        onEdit={(target, action) => { if (action === 'remove') closeTab(target as string); }}
        tabBarStyle={{ margin: 0 }}
        items={tabs.map((t) => ({
          key: t.path,
          label: (
            <Dropdown menu={{ items: ctxMenu(t.path) }} trigger={['contextMenu']}>
              <span>{t.title}</span>
            </Dropdown>
          ),
          closable: tabs.length > 1,
        }))}
      />
    </div>
  );
};

export default TabsNav;
