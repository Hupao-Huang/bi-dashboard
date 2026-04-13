import React, { useEffect, useState, useCallback } from 'react';
import { Badge, Button, Drawer, List, Tag, Modal, Typography, Empty, Tooltip } from 'antd';
import { BellOutlined, PushpinOutlined } from '@ant-design/icons';
import { API_BASE } from '../config';

const { Paragraph } = Typography;

interface Notice {
  id: number;
  title: string;
  content: string;
  type: string;
  isPinned: boolean;
  createdBy: string;
  createdAt: string;
}

const typeConfig: Record<string, { color: string; label: string }> = {
  update: { color: 'blue', label: '更新' },
  notice: { color: 'green', label: '通知' },
  maintenance: { color: 'orange', label: '维护' },
};

const NoticeBell: React.FC = () => {
  const [notices, setNotices] = useState<Notice[]>([]);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [detailNotice, setDetailNotice] = useState<Notice | null>(null);
  const [readIds, setReadIds] = useState<number[]>(() => {
    try {
      return JSON.parse(localStorage.getItem('notice_read_ids') || '[]');
    } catch { return []; }
  });

  const fetchNotices = useCallback(() => {
    fetch(`${API_BASE}/api/notices`, { credentials: 'include' })
      .then(res => res.json())
      .then(res => {
        if (res.data?.notices) setNotices(res.data.notices);
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    fetchNotices();
    const timer = setInterval(fetchNotices, 5 * 60 * 1000);
    return () => clearInterval(timer);
  }, [fetchNotices]);

  // 首次登录弹窗：显示最新一条未读公告
  useEffect(() => {
    if (notices.length > 0) {
      const unread = notices.filter(n => !readIds.includes(n.id));
      if (unread.length > 0 && !sessionStorage.getItem('notice_popup_shown')) {
        setDetailNotice(unread[0]);
        sessionStorage.setItem('notice_popup_shown', '1');
      }
    }
  }, [notices, readIds]);

  const unreadCount = notices.filter(n => !readIds.includes(n.id)).length;

  const markRead = (id: number) => {
    if (!readIds.includes(id)) {
      const newIds = [...readIds, id];
      setReadIds(newIds);
      localStorage.setItem('notice_read_ids', JSON.stringify(newIds));
    }
  };

  const markAllRead = () => {
    const allIds = notices.map(n => n.id);
    setReadIds(allIds);
    localStorage.setItem('notice_read_ids', JSON.stringify(allIds));
  };

  return (
    <>
      <Tooltip title="系统公告">
        <Badge count={unreadCount} size="small" offset={[-2, 2]}>
          <Button
            type="text"
            icon={<BellOutlined />}
            onClick={() => setDrawerOpen(true)}
            style={{ color: '#64748b', fontSize: 16 }}
          />
        </Badge>
      </Tooltip>

      <Drawer
        title={<div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>系统公告</span>
          {unreadCount > 0 && (
            <Button type="link" size="small" onClick={markAllRead}>全部已读</Button>
          )}
        </div>}
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        width={400}
      >
        {notices.length === 0 ? (
          <Empty description="暂无公告" />
        ) : (
          <List
            dataSource={notices}
            renderItem={(item) => {
              const isRead = readIds.includes(item.id);
              const tc = typeConfig[item.type] || typeConfig.notice;
              return (
                <List.Item
                  style={{
                    cursor: 'pointer',
                    background: isRead ? 'transparent' : '#f0f5ff',
                    borderRadius: 6,
                    marginBottom: 8,
                    padding: '10px 12px',
                  }}
                  onClick={() => {
                    markRead(item.id);
                    setDrawerOpen(false);
                    setDetailNotice(item);
                  }}
                >
                  <div style={{ width: '100%' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4 }}>
                      <Tag color={tc.color} style={{ margin: 0 }}>{tc.label}</Tag>
                      {item.isPinned && <PushpinOutlined style={{ color: '#f5222d' }} />}
                      <span style={{ fontWeight: isRead ? 400 : 600, flex: 1 }}>{item.title}</span>
                    </div>
                    <div style={{ color: '#999', fontSize: 12 }}>
                      {item.createdBy} · {item.createdAt}
                    </div>
                  </div>
                </List.Item>
              );
            }}
          />
        )}
      </Drawer>

      <Modal
        title={detailNotice ? (
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Tag color={typeConfig[detailNotice.type]?.color || 'blue'}>
              {typeConfig[detailNotice.type]?.label || '通知'}
            </Tag>
            {detailNotice.title}
          </div>
        ) : ''}
        open={!!detailNotice}
        onCancel={() => setDetailNotice(null)}
        footer={null}
        width={520}
      >
        {detailNotice && (
          <>
            <div style={{ color: '#999', fontSize: 12, marginBottom: 12 }}>
              发布人：{detailNotice.createdBy} · {detailNotice.createdAt}
            </div>
            <Paragraph style={{ whiteSpace: 'pre-wrap', fontSize: 14, lineHeight: 1.8 }}>
              {detailNotice.content}
            </Paragraph>
          </>
        )}
      </Modal>
    </>
  );
};

export default NoticeBell;
