// v1.73.0 W2: BI 智能助手浮窗组件
// 设计文档: docs/ai-assistant-design.md §4.2
// 用法: 在 MainLayout 全局挂一次 <AIChatWidget />

import React, { useState, useRef, useEffect } from 'react';
import { Button, Input, Avatar, Tooltip, message, Spin, Tag } from 'antd';
import {
  MessageOutlined, CloseOutlined, SendOutlined,
  UserOutlined, RobotOutlined, LikeOutlined, DislikeOutlined,
  LikeFilled, DislikeFilled,
} from '@ant-design/icons';
import { API_BASE } from '../config';

interface Message {
  id?: number;
  role: 'user' | 'assistant';
  content: string;
  sourceAPI?: string;
  confidence?: number;
  llmTokens?: number;
  durationMs?: number;
  warning?: string;
  feedback?: 1 | -1 | null;
}

const STORAGE_KEY = 'bi-ai-chat-session';

const AIChatWidget: React.FC = () => {
  const [open, setOpen] = useState(false);
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [sessionId, setSessionId] = useState<number | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  // 加载 localStorage 历史 (只缓存当前会话 ID, 消息从后端拉)
  useEffect(() => {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved) {
      try {
        const obj = JSON.parse(saved);
        if (obj.sessionId) setSessionId(obj.sessionId);
      } catch { /* ignore */ }
    }
  }, []);

  // 打开浮窗时, 如果有 sessionId 就拉历史消息
  useEffect(() => {
    if (open && sessionId && messages.length === 0) {
      fetch(`${API_BASE}/api/ai-assistant/messages?sessionId=${sessionId}`, { credentials: 'include' })
        .then(r => r.json())
        .then(j => {
          if (j.code === 200 && Array.isArray(j.data?.items)) {
            const msgs: Message[] = j.data.items.map((it: any) => ({
              id: it.id,
              role: it.role,
              content: it.role === 'user' ? it.question : it.answer,
              sourceAPI: it.sourceAPI,
              confidence: it.confidence,
              llmTokens: it.llmTokens,
              durationMs: it.durationMs,
              warning: it.warning,
            }));
            setMessages(msgs);
          }
        })
        .catch(() => { /* ignore */ });
    }
  }, [open, sessionId, messages.length]);

  // 滚到底
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages, loading]);

  const handleSend = async () => {
    const q = input.trim();
    if (!q || loading) return;
    setInput('');
    setMessages(m => [...m, { role: 'user', content: q }]);
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/ai-assistant/ask`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ question: q, sessionId }),
      });
      const j = await res.json();
      if (j.code === 200) {
        const d = j.data;
        if (d.sessionId && d.sessionId !== sessionId) {
          setSessionId(d.sessionId);
          localStorage.setItem(STORAGE_KEY, JSON.stringify({ sessionId: d.sessionId }));
        }
        setMessages(m => [...m, {
          id: d.messageId,
          role: 'assistant',
          content: d.answer,
          sourceAPI: d.sourceAPI,
          confidence: d.confidence,
          llmTokens: d.llmTokens,
          durationMs: d.durationMs,
          warning: d.warning,
        }]);
      } else {
        message.error(j.msg || 'AI 助手响应失败');
        setMessages(m => [...m, { role: 'assistant', content: `❌ ${j.msg || '响应失败'}` }]);
      }
    } catch (err) {
      message.error(`AI 助手调用失败: ${err instanceof Error ? err.message : String(err)}`);
      setMessages(m => [...m, { role: 'assistant', content: `❌ 网络异常, 请重试` }]);
    } finally {
      setLoading(false);
    }
  };

  const handleFeedback = async (msgIdx: number, thumb: 1 | -1) => {
    const msg = messages[msgIdx];
    if (!msg.id) return;
    try {
      const res = await fetch(`${API_BASE}/api/ai-assistant/feedback`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ messageId: msg.id, thumb }),
      });
      const j = await res.json();
      if (j.code === 200) {
        setMessages(m => m.map((x, i) => i === msgIdx ? { ...x, feedback: thumb } : x));
        message.success(thumb === 1 ? '感谢反馈 👍' : '已记录, 我们会改进 👎');
      }
    } catch { /* ignore */ }
  };

  const handleNewSession = () => {
    setMessages([]);
    setSessionId(null);
    localStorage.removeItem(STORAGE_KEY);
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  if (!open) {
    return (
      <Tooltip title="BI 智能助手 (问数)" placement="left">
        <Button
          type="primary"
          shape="circle"
          size="large"
          icon={<MessageOutlined />}
          onClick={() => setOpen(true)}
          style={{
            position: 'fixed', right: 24, bottom: 24, zIndex: 9999,
            width: 56, height: 56, boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
          }}
        />
      </Tooltip>
    );
  }

  return (
    <div style={{
      position: 'fixed', right: 24, bottom: 24, zIndex: 9999,
      width: 380, height: 560, background: '#fff', borderRadius: 12,
      boxShadow: '0 4px 24px rgba(0,0,0,0.18)', display: 'flex', flexDirection: 'column',
      overflow: 'hidden',
    }}>
      {/* Header */}
      <div style={{ padding: '12px 16px', borderBottom: '1px solid #f0f0f0',
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        background: '#fafafa' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <Avatar size={28} icon={<RobotOutlined />} style={{ background: '#1677ff' }} />
          <span style={{ fontWeight: 500 }}>BI 智能助手</span>
        </div>
        <div style={{ display: 'flex', gap: 4 }}>
          <Button size="small" type="text" onClick={handleNewSession}>新会话</Button>
          <Button size="small" type="text" icon={<CloseOutlined />} onClick={() => setOpen(false)} />
        </div>
      </div>

      {/* 消息列表 */}
      <div ref={scrollRef} style={{ flex: 1, padding: 12, overflowY: 'auto', background: '#f7f8fa' }}>
        {messages.length === 0 && !loading && (
          <div style={{ textAlign: 'center', color: '#999', padding: 40, fontSize: 13 }}>
            <RobotOutlined style={{ fontSize: 32, color: '#1677ff', marginBottom: 12, display: 'block' }} />
            <div>您好, 我能查 BI 看板数据.</div>
            <div style={{ marginTop: 12, color: '#666' }}>试试问:</div>
            <div style={{ marginTop: 8, color: '#1677ff', cursor: 'pointer' }} onClick={() => setInput('上月电商部销售多少')}>
              · 上月电商部销售多少
            </div>
            <div style={{ marginTop: 4, color: '#1677ff', cursor: 'pointer' }} onClick={() => setInput('本月哪个店卖最差')}>
              · 本月哪个店卖最差
            </div>
            <div style={{ marginTop: 4, color: '#1677ff', cursor: 'pointer' }} onClick={() => setInput('这周对比上周')}>
              · 这周对比上周
            </div>
          </div>
        )}
        {messages.map((m, i) => (
          <div key={i} style={{ marginBottom: 12, display: 'flex',
            justifyContent: m.role === 'user' ? 'flex-end' : 'flex-start', gap: 8 }}>
            {m.role === 'assistant' && (
              <Avatar size={28} icon={<RobotOutlined />} style={{ background: '#1677ff', flexShrink: 0 }} />
            )}
            <div style={{ maxWidth: '76%' }}>
              <div style={{
                background: m.role === 'user' ? '#1677ff' : '#fff',
                color: m.role === 'user' ? '#fff' : '#000',
                padding: '8px 12px', borderRadius: 8, fontSize: 13, lineHeight: 1.6,
                wordBreak: 'break-word', boxShadow: '0 1px 2px rgba(0,0,0,0.04)',
              }}>
                {m.content}
                {m.warning && (
                  <div style={{ marginTop: 6, fontSize: 11, color: '#faad14' }}>⚠️ {m.warning}</div>
                )}
              </div>
              {m.role === 'assistant' && m.sourceAPI && (
                <div style={{ marginTop: 4, fontSize: 11, color: '#999' }}>
                  <span title={m.sourceAPI}>🔍 {m.sourceAPI.length > 50 ? m.sourceAPI.slice(0, 50) + '...' : m.sourceAPI}</span>
                  {m.confidence !== undefined && (
                    <Tag style={{ marginLeft: 4, fontSize: 10 }} color={m.confidence > 0.8 ? 'green' : 'orange'}>
                      置信度 {Math.round(m.confidence * 100)}%
                    </Tag>
                  )}
                  {m.durationMs !== undefined && <span style={{ marginLeft: 4 }}>{m.durationMs}ms</span>}
                  {m.llmTokens !== undefined && m.llmTokens > 0 && <span style={{ marginLeft: 4 }}>· {m.llmTokens} tokens</span>}
                </div>
              )}
              {m.role === 'assistant' && m.id && (
                <div style={{ marginTop: 4, display: 'flex', gap: 8 }}>
                  <Button size="small" type="text" icon={m.feedback === 1 ? <LikeFilled style={{ color: '#52c41a' }} /> : <LikeOutlined />} onClick={() => handleFeedback(i, 1)} />
                  <Button size="small" type="text" icon={m.feedback === -1 ? <DislikeFilled style={{ color: '#ff4d4f' }} /> : <DislikeOutlined />} onClick={() => handleFeedback(i, -1)} />
                </div>
              )}
            </div>
            {m.role === 'user' && (
              <Avatar size={28} icon={<UserOutlined />} style={{ background: '#52c41a', flexShrink: 0 }} />
            )}
          </div>
        ))}
        {loading && (
          <div style={{ display: 'flex', justifyContent: 'flex-start', gap: 8, marginBottom: 12 }}>
            <Avatar size={28} icon={<RobotOutlined />} style={{ background: '#1677ff' }} />
            <div style={{ background: '#fff', padding: '8px 12px', borderRadius: 8 }}>
              <Spin size="small" /> <span style={{ marginLeft: 8, fontSize: 12, color: '#999' }}>思考中...</span>
            </div>
          </div>
        )}
      </div>

      {/* 输入框 */}
      <div style={{ padding: 12, borderTop: '1px solid #f0f0f0', background: '#fff' }}>
        <div style={{ display: 'flex', gap: 8 }}>
          <Input.TextArea
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="问 BI 数据 (Enter 发, Shift+Enter 换行)"
            autoSize={{ minRows: 1, maxRows: 3 }}
            disabled={loading}
            style={{ flex: 1, fontSize: 13 }}
          />
          <Button type="primary" icon={<SendOutlined />} onClick={handleSend} loading={loading} disabled={!input.trim()} />
        </div>
      </div>
    </div>
  );
};

export default AIChatWidget;
