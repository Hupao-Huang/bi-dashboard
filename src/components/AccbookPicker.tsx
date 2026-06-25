import React, { useMemo, useState } from 'react';
import { Modal, Input, Button, Checkbox, Empty, theme } from 'antd';
import { SearchOutlined, CloseOutlined, DownOutlined } from '@ant-design/icons';
import { AccbookOption, filterBooks, toggleCode, mergeAll, triggerLabelOf } from './accbookPickerLogic';

// 账簿选择器 (2026-06-25) — 仿用友 YS 弹窗式账簿选择
// 左侧全账簿勾选 + 右侧已选面板, 顶部搜索 + 全选/清空, 点「确定」才生效(「取消」还原)。
// 纯展示组件: 输入 账簿列表 + 已选编码, 输出新的已选编码数组, 不与后端交互。
// 纯逻辑(过滤/勾选/全选/文案)拆到 ./accbookPickerLogic 便于单测。
export type { AccbookOption };

interface AccbookPickerProps {
  books: AccbookOption[];
  value: string[];
  onChange: (codes: string[]) => void;
  placeholder?: string;
  style?: React.CSSProperties;
}

const AccbookPicker: React.FC<AccbookPickerProps> = ({
  books,
  value,
  onChange,
  placeholder = '选择账簿(可多选)',
  style,
}) => {
  const { token } = theme.useToken();
  const [open, setOpen] = useState(false);
  const [keyword, setKeyword] = useState('');
  const [draft, setDraft] = useState<string[]>([]); // 弹窗内临时选择, 「确定」才提交

  const nameOf = useMemo(() => {
    const m = new Map<string, string>();
    books.forEach((b) => m.set(b.code, b.name));
    return m;
  }, [books]);

  const filtered = useMemo(() => filterBooks(books, keyword), [books, keyword]);
  const draftSet = useMemo(() => new Set(draft), [draft]);

  const openModal = () => {
    setDraft(value);
    setKeyword('');
    setOpen(true);
  };
  const confirm = () => {
    onChange(draft);
    setOpen(false);
  };
  const toggle = (code: string) => setDraft((prev) => toggleCode(prev, code));
  const selectAllFiltered = () => setDraft((prev) => mergeAll(prev, filtered));
  const clearAll = () => setDraft([]);

  const boxStyle: React.CSSProperties = {
    maxHeight: 360,
    overflowY: 'auto',
    border: `1px solid ${token.colorBorder}`,
    borderRadius: token.borderRadiusLG,
    padding: 8,
  };

  return (
    <>
      <Button
        onClick={openModal}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          textAlign: 'left',
          ...style,
        }}
      >
        <span
          style={{
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            flex: 1,
            color: value.length === 0 ? 'var(--text-tertiary)' : undefined,
          }}
        >
          {triggerLabelOf(value, nameOf, placeholder)}
        </span>
        <DownOutlined style={{ marginLeft: 8, color: 'var(--text-tertiary)', fontSize: 12 }} />
      </Button>

      <Modal
        title="选择账簿"
        open={open}
        onOk={confirm}
        onCancel={() => setOpen(false)}
        okText="确定"
        cancelText="取消"
        width={720}
        destroyOnHidden
      >
        <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
          <Input
            allowClear
            prefix={<SearchOutlined />}
            placeholder="搜索 编码 / 名称"
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            style={{ flex: 1 }}
          />
          <Button onClick={selectAllFiltered}>全选</Button>
          <Button onClick={clearAll} disabled={draft.length === 0}>
            清空
          </Button>
        </div>

        <div style={{ display: 'flex', gap: 12 }}>
          {/* 左: 全账簿勾选列表 */}
          <div style={{ ...boxStyle, flex: 1 }}>
            {filtered.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="无匹配账簿" />
            ) : (
              filtered.map((b) => (
                <div key={b.code} style={{ padding: '4px 4px' }}>
                  <Checkbox checked={draftSet.has(b.code)} onChange={() => toggle(b.code)}>
                    <span style={{ color: 'var(--text-tertiary)' }}>{b.code}</span> {b.name}
                  </Checkbox>
                </div>
              ))
            )}
          </div>

          {/* 右: 已选面板 */}
          <div style={{ ...boxStyle, width: 260 }}>
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 8,
              }}
            >
              <span>已选 ({draft.length})</span>
              <Button
                type="link"
                size="small"
                onClick={clearAll}
                disabled={draft.length === 0}
                style={{ padding: 0 }}
              >
                清空
              </Button>
            </div>
            {draft.length === 0 ? (
              <div style={{ color: 'var(--text-tertiary)', fontSize: 12, padding: '8px 0' }}>
                未选择账簿
              </div>
            ) : (
              draft.map((code) => (
                <div
                  key={code}
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    padding: '4px 0',
                  }}
                >
                  <span
                    style={{
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                      marginRight: 8,
                    }}
                  >
                    <span style={{ color: 'var(--text-tertiary)' }}>{code}</span>{' '}
                    {nameOf.get(code) || ''}
                  </span>
                  <CloseOutlined
                    onClick={() => toggle(code)}
                    style={{ cursor: 'pointer', color: 'var(--text-tertiary)', flexShrink: 0 }}
                  />
                </div>
              ))
            )}
          </div>
        </div>
      </Modal>
    </>
  );
};

export default AccbookPicker;
