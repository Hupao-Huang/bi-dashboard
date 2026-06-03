import React, { useMemo } from 'react';
import { Row, Col, Statistic, List, Tag, Empty, Spin, Typography } from 'antd';
import Chart from './Chart';
import { pageTitleMap } from '../navigation';

const { Text } = Typography;

// 后端 /api/user/activity 与 /api/admin/user-activity 返回结构
export interface ActivityStats { today: number; last7d: number; last30d: number; total: number; activeDays: number; }
export interface ActivityHeatCell { date: string; count: number; }
export interface ActivityRecord { action: string; resource: string; detail: string; createdAt: string; }
export interface UserActivity { stats: ActivityStats; heatmap: ActivityHeatCell[]; recent: ActivityRecord[]; }

// 操作类型 → 中文标签 + 颜色（用 antd 语义色，不自造装饰色）
const ACTION_TAG: Record<string, string> = {
  page_view: '查看', login: '登录', logout: '退出', export: '导出', permission_change: '权限', hesi_approve: '合思',
};
const ACTION_COLOR: Record<string, string> = {
  page_view: 'blue', login: 'green', logout: 'default', export: 'gold', permission_change: 'volcano', hesi_approve: 'purple',
};

// 一条操作翻译成大白话；页面路径复用 navigation 的 pageTitleMap
function describe(rec: ActivityRecord): string {
  switch (rec.action) {
    case 'page_view': return `查看了 ${pageTitleMap[rec.resource] || rec.resource || '页面'}`;
    case 'login': return '登录系统';
    case 'logout': return '退出登录';
    case 'export': return rec.resource ? `导出 ${pageTitleMap[rec.resource] || rec.resource}` : '导出数据';
    case 'permission_change': return '变更了权限';
    case 'hesi_approve': return '合思单据审批';
    default: return rec.action;
  }
}

const fmtDate = (d: Date) => {
  const m = `${d.getMonth() + 1}`.padStart(2, '0');
  const day = `${d.getDate()}`.padStart(2, '0');
  return `${d.getFullYear()}-${m}-${day}`;
};

const UserActivityPanel: React.FC<{ data: UserActivity | null; loading?: boolean }> = ({ data, loading }) => {
  const heatOption = useMemo(() => {
    const today = new Date();
    const end = fmtDate(today);
    const start = fmtDate(new Date(today.getTime() - 181 * 86400000));
    const cells = data?.heatmap || [];
    const maxCount = Math.max(1, ...cells.map(c => c.count));
    return {
      tooltip: { trigger: 'item', formatter: (p: any) => `${p.value[0]}　${p.value[1]} 次操作` },
      visualMap: {
        min: 0, max: maxCount, calculable: false, orient: 'horizontal',
        left: 'center', bottom: 4, itemWidth: 14, itemHeight: 10,
        text: ['多', '少'], textStyle: { fontSize: 11, color: '#999' },
        inRange: { color: ['#eef1f4', '#cfe0f3', '#9bbde8', '#5b8ed6', '#1e40af'] },
      },
      calendar: {
        top: 24, left: 36, right: 16, bottom: 40, cellSize: ['auto', 14],
        range: [start, end], splitLine: { show: false },
        itemStyle: { color: '#eef1f4', borderColor: '#fff', borderWidth: 2 },
        yearLabel: { show: false },
        monthLabel: { nameMap: ['1月', '2月', '3月', '4月', '5月', '6月', '7月', '8月', '9月', '10月', '11月', '12月'], color: '#999', fontSize: 11 },
        dayLabel: { firstDay: 1, nameMap: ['日', '一', '二', '三', '四', '五', '六'], color: '#999', fontSize: 11 },
      },
      series: [{ type: 'heatmap', coordinateSystem: 'calendar', data: cells.map(c => [c.date, c.count]) }],
    };
  }, [data]);

  return (
    <Spin spinning={!!loading}>
      {/* 统计条：antd Statistic 默认样式，不自改字号/颜色 */}
      <Row gutter={16}>
        <Col flex="1"><Statistic title="今日操作" value={data?.stats?.today ?? 0} /></Col>
        <Col flex="1"><Statistic title="近 7 天" value={data?.stats?.last7d ?? 0} /></Col>
        <Col flex="1"><Statistic title="近 30 天" value={data?.stats?.last30d ?? 0} /></Col>
        <Col flex="1"><Statistic title="总操作" value={data?.stats?.total ?? 0} /></Col>
        <Col flex="1"><Statistic title="活跃天数" value={data?.stats?.activeDays ?? 0} /></Col>
      </Row>

      <div style={{ marginTop: 20 }}>
        <Text strong>近半年操作热力图</Text>
        <Chart option={heatOption} style={{ height: 200, width: '100%' }} />
      </div>

      <div style={{ marginTop: 12 }}>
        <Text strong>最近操作记录</Text>
        {data?.recent && data.recent.length > 0 ? (
          <List
            size="small"
            dataSource={data.recent}
            renderItem={(rec) => (
              <List.Item>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, width: '100%' }}>
                  <Tag color={ACTION_COLOR[rec.action] || 'default'}>{ACTION_TAG[rec.action] || rec.action}</Tag>
                  <span style={{ flex: 1, minWidth: 0 }}>{describe(rec)}</span>
                  <Text type="secondary" style={{ fontSize: 12 }}>{rec.createdAt}</Text>
                </div>
              </List.Item>
            )}
          />
        ) : (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无操作记录" />
        )}
      </div>
    </Spin>
  );
};

export default UserActivityPanel;
