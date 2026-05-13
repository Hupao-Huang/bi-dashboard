import React, { useCallback, useEffect, useState } from 'react';
import { Card, Tag, Empty, Spin, message } from 'antd';
import { API_BASE } from '../../config';

const STATUS_HEX_MAP: Record<string, string> = {
  pending: '#f59e0b',
  accepted: '#1677ff',
  scheduled: '#8b5cf6',
  in_progress: '#06b6d4',
  done: '#16a34a',
};

const STATUS_LABEL: Record<string, string> = {
  pending: '待评估', accepted: '已接受', scheduled: '排期中', in_progress: '开发中', done: '已完成',
};

const PRIORITY_HEX: Record<string, string> = {
  P0: '#dc2626', P1: '#ea580c', P2: '#1677ff', P3: '#94a3b8',
};

interface RequirementItem {
  id: number;
  title: string;
  submitterName: string;
  submitterDept: string;
  priority: string;
  status: string;
  targetVersion: string;
  expectedDate: string | null;
  tag: string;
}

const COLUMNS = ['pending', 'accepted', 'scheduled', 'in_progress', 'done'];

const RequirementKanban: React.FC = () => {
  const [list, setList] = useState<RequirementItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [isAdmin, setIsAdmin] = useState(false);
  const [draggingId, setDraggingId] = useState<number | null>(null);
  const [hoverCol, setHoverCol] = useState<string | null>(null);

  const fetchList = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/requirements/list?pageSize=200`, { credentials: 'include' });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const json = await res.json();
      const data = json.data || json;
      setList((data.list || []).filter((i: RequirementItem) => COLUMNS.includes(i.status)));
      setIsAdmin(!!data.isAdmin);
    } catch {
      message.error('加载看板失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchList(); }, [fetchList]);

  const handleDrop = async (status: string) => {
    if (!isAdmin) {
      message.warning('只有管理员可以拖拽改状态');
      setDraggingId(null);
      setHoverCol(null);
      return;
    }
    if (!draggingId) return;
    const item = list.find(i => i.id === draggingId);
    if (!item || item.status === status) {
      setDraggingId(null);
      setHoverCol(null);
      return;
    }
    // 乐观更新
    setList(prev => prev.map(i => i.id === draggingId ? { ...i, status } : i));
    try {
      const res = await fetch(`${API_BASE}/api/requirements/${draggingId}`, {
        method: 'PUT', credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status }),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      message.success(`已移至「${STATUS_LABEL[status]}」`);
    } catch {
      message.error('状态更新失败，已回滚');
      fetchList();
    } finally {
      setDraggingId(null);
      setHoverCol(null);
    }
  };

  const renderCard = (item: RequirementItem) => {
    const priHex = PRIORITY_HEX[item.priority] || '#94a3b8';
    return (
      <div
        key={item.id}
        draggable={isAdmin}
        onDragStart={() => setDraggingId(item.id)}
        onDragEnd={() => { setDraggingId(null); setHoverCol(null); }}
        style={{
          background: 'white',
          padding: 10,
          marginBottom: 8,
          borderRadius: 4,
          border: '1px solid #e5e7eb',
          cursor: isAdmin ? 'grab' : 'default',
          opacity: draggingId === item.id ? 0.5 : 1,
          fontSize: 13,
          boxShadow: '0 1px 2px rgba(0,0,0,0.04)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 6 }}>
          <span style={{
            display: 'inline-block', padding: '0 6px',
            background: `${priHex}12`, color: priHex, border: `1px solid ${priHex}30`,
            borderRadius: 3, fontWeight: 600, fontSize: 11,
          }}>{item.priority}</span>
          {item.tag && <Tag style={{ margin: 0, fontSize: 11 }}>{item.tag}</Tag>}
          {item.targetVersion && <Tag color="blue" style={{ margin: 0, fontSize: 11 }}>{item.targetVersion}</Tag>}
        </div>
        <div style={{ fontWeight: 500, marginBottom: 4, lineHeight: 1.4 }}>{item.title}</div>
        <div style={{ color: '#94a3b8', fontSize: 11 }}>
          {item.submitterName}{item.submitterDept ? ` · ${item.submitterDept}` : ''}
        </div>
      </div>
    );
  };

  return (
    <div>
      {!isAdmin && (
        <div style={{ marginBottom: 12, padding: 8, background: '#fffbeb', border: '1px solid #fde68a', borderRadius: 4, fontSize: 12, color: '#92400e' }}>
          只有管理员可以拖拽卡片改状态，您可在「列表」视图查看自己提的需求
        </div>
      )}
      <Spin spinning={loading}>
        <div style={{ display: 'flex', gap: 12, overflowX: 'auto', paddingBottom: 8 }}>
          {COLUMNS.map(status => {
            const items = list.filter(i => i.status === status);
            const hex = STATUS_HEX_MAP[status];
            const isHover = hoverCol === status;
            return (
              <div
                key={status}
                onDragOver={e => { e.preventDefault(); if (hoverCol !== status) setHoverCol(status); }}
                onDragLeave={() => setHoverCol(null)}
                onDrop={() => handleDrop(status)}
                style={{
                  flex: '0 0 240px',
                  background: isHover ? `${hex}08` : '#f8fafc',
                  borderRadius: 6,
                  padding: 12,
                  border: isHover ? `2px dashed ${hex}` : '2px dashed transparent',
                  transition: 'all 120ms',
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 10 }}>
                  <span style={{ width: 8, height: 8, borderRadius: '50%', background: hex }} />
                  <span style={{ fontWeight: 600, color: '#0f172a' }}>{STATUS_LABEL[status]}</span>
                  <span style={{ color: '#94a3b8', fontSize: 12 }}>{items.length}</span>
                </div>
                {items.length === 0 ? (
                  <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={<span style={{ color: '#cbd5e1', fontSize: 12 }}>暂无</span>} />
                ) : items.map(renderCard)}
              </div>
            );
          })}
        </div>
      </Spin>
    </div>
  );
};

export default RequirementKanban;
