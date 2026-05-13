import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Empty, Spin, Tag, message } from 'antd';
import { API_BASE } from '../../config';

const STATUS_HEX_MAP: Record<string, string> = {
  pending: '#f59e0b',
  accepted: '#1677ff',
  scheduled: '#8b5cf6',
  in_progress: '#06b6d4',
  done: '#16a34a',
  shelved: '#94a3b8',
  rejected: '#ef4444',
};
const STATUS_LABEL: Record<string, string> = {
  pending: '待评估', accepted: '已接受', scheduled: '排期中', in_progress: '开发中', done: '已完成',
  shelved: '已搁置', rejected: '已拒绝',
};
const PRIORITY_HEX: Record<string, string> = {
  P0: '#dc2626', P1: '#ea580c', P2: '#1677ff', P3: '#94a3b8',
};

interface GanttRow {
  id: number;
  title: string;
  priority: string;
  status: string;
  targetVersion: string;
  expectedDate: string | null;
  actualDate: string | null;
  submitter: string;
}

interface GanttData {
  main: GanttRow[];
  shelved: GanttRow[];
}

const RequirementGantt: React.FC = () => {
  const [data, setData] = useState<GanttData>({ main: [], shelved: [] });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/requirements/gantt`, { credentials: 'include' });
      if (res.status === 403) {
        setError('只有管理员可以查看甘特图');
        return;
      }
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const json = await res.json();
      setData(json.data || json);
      setError('');
    } catch {
      message.error('加载甘特图失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchData(); }, [fetchData]);

  // 按 target_version 分组
  const grouped = useMemo(() => {
    const map = new Map<string, GanttRow[]>();
    data.main.forEach(r => {
      const key = r.targetVersion || '__unscheduled__';
      if (!map.has(key)) map.set(key, []);
      map.get(key)!.push(r);
    });
    // 排序：有版本号的按字典序，未排期放最前 (待规划)
    const entries = Array.from(map.entries());
    return entries.sort((a, b) => {
      if (a[0] === '__unscheduled__') return -1;
      if (b[0] === '__unscheduled__') return 1;
      return a[0].localeCompare(b[0]);
    });
  }, [data.main]);

  if (error) {
    return <div style={{ padding: 40, textAlign: 'center', color: '#94a3b8' }}>{error}</div>;
  }

  const renderCard = (r: GanttRow) => {
    const priHex = PRIORITY_HEX[r.priority] || '#94a3b8';
    const statHex = STATUS_HEX_MAP[r.status] || '#94a3b8';
    return (
      <div
        key={r.id}
        style={{
          display: 'flex', alignItems: 'center', gap: 10,
          padding: '8px 12px', marginBottom: 6,
          background: 'white', borderRadius: 4,
          borderLeft: `3px solid ${priHex}`,
          border: '1px solid #e5e7eb',
          fontSize: 13,
        }}
      >
        <span style={{
          display: 'inline-block', padding: '0 6px', minWidth: 28, textAlign: 'center',
          background: `${priHex}12`, color: priHex, border: `1px solid ${priHex}30`,
          borderRadius: 3, fontWeight: 600, fontSize: 11,
        }}>{r.priority}</span>
        <span style={{ flex: 1, fontWeight: 500 }}>{r.title}</span>
        <span style={{ color: '#64748b', fontSize: 11 }}>{r.submitter}</span>
        <span style={{
          display: 'inline-block', padding: '0 6px',
          background: `${statHex}12`, color: statHex, border: `1px solid ${statHex}30`,
          borderRadius: 3, fontSize: 11,
        }}>{STATUS_LABEL[r.status]}</span>
        {r.expectedDate && (
          <span style={{ color: '#94a3b8', fontSize: 11, minWidth: 80, textAlign: 'right' }}>
            {r.actualDate ? `✓ ${r.actualDate}` : `预 ${r.expectedDate}`}
          </span>
        )}
      </div>
    );
  };

  return (
    <Spin spinning={loading}>
      {grouped.length === 0 && data.shelved.length === 0 ? (
        <Empty description="暂无需求数据" />
      ) : (
        <>
          {grouped.map(([version, items]) => {
            const isUnscheduled = version === '__unscheduled__';
            return (
              <div key={version} style={{ marginBottom: 20 }}>
                <div style={{
                  display: 'flex', alignItems: 'center', gap: 10,
                  marginBottom: 8, paddingBottom: 6,
                  borderBottom: '2px solid #e5e7eb',
                }}>
                  {isUnscheduled ? (
                    <Tag color="default" style={{ fontSize: 14, padding: '2px 10px' }}>未排期</Tag>
                  ) : (
                    <Tag color="blue" style={{ fontSize: 14, padding: '2px 10px', fontWeight: 600 }}>{version}</Tag>
                  )}
                  <span style={{ color: '#94a3b8', fontSize: 12 }}>{items.length} 条</span>
                </div>
                {items.map(renderCard)}
              </div>
            );
          })}

          {data.shelved.length > 0 && (
            <div style={{ marginTop: 30, padding: 16, background: '#f8fafc', borderRadius: 6 }}>
              <div style={{
                display: 'flex', alignItems: 'center', gap: 10, marginBottom: 8,
                color: '#64748b', fontSize: 13, fontWeight: 600,
              }}>
                📦 暂缓清单（已搁置/已拒绝）· {data.shelved.length} 条
              </div>
              {data.shelved.map(renderCard)}
            </div>
          )}
        </>
      )}
    </Spin>
  );
};

export default RequirementGantt;
