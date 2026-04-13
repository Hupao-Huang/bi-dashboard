import React, { useEffect, useRef, useState } from 'react';
import { DatePicker, Space, Button, Dropdown } from 'antd';
import { CalendarOutlined } from '@ant-design/icons';
import type { MenuProps } from 'antd';
import dayjs, { Dayjs } from 'dayjs';
import quarterOfYear from 'dayjs/plugin/quarterOfYear';
import isoWeek from 'dayjs/plugin/isoWeek';
import { createPortal } from 'react-dom';

dayjs.extend(quarterOfYear);
dayjs.extend(isoWeek);

const { RangePicker } = DatePicker;

interface Props {
  start: string;
  end: string;
  onChange: (start: string, end: string) => void;
}

const DateFilter: React.FC<Props> = ({ start, end, onChange }) => {
  const lastClickedRef = useRef<string | null>(null);
  const [portalTarget, setPortalTarget] = useState<HTMLElement | null>(null);
  const today = dayjs();
  const yesterday = today.subtract(1, 'day');

  useEffect(() => {
    setPortalTarget(document.getElementById('bi-toolbar-slot'));
  }, []);

  const cap = (d: Dayjs, minDate?: Dayjs) => {
    const capped = d.isAfter(yesterday) ? yesterday : d;
    return minDate && capped.isBefore(minDate) ? minDate : capped;
  };

  const shortcuts = [
    { label: '昨日', start: yesterday, end: yesterday },
    { label: '前天', start: today.subtract(2, 'day'), end: today.subtract(2, 'day') },
    { label: '近3天', start: yesterday.subtract(2, 'day'), end: yesterday },
    { label: '近7天', start: yesterday.subtract(6, 'day'), end: yesterday },
    { label: '近30天', start: yesterday.subtract(29, 'day'), end: yesterday },
    { label: '近60天', start: yesterday.subtract(59, 'day'), end: yesterday },
    { label: '近90天', start: yesterday.subtract(89, 'day'), end: yesterday },
    { label: '本周', start: today.startOf('isoWeek' as any), end: cap(today.endOf('isoWeek' as any), today.startOf('isoWeek' as any)) },
    { label: '上周', start: today.subtract(1, 'week').startOf('isoWeek' as any), end: today.subtract(1, 'week').endOf('isoWeek' as any) },
    { label: '本月', start: today.startOf('month'), end: cap(today.endOf('month'), today.startOf('month')) },
    { label: '上月', start: today.subtract(1, 'month').startOf('month'), end: today.subtract(1, 'month').endOf('month') },
    { label: '本季', start: today.startOf('quarter' as any), end: cap(today.endOf('quarter' as any), today.startOf('quarter' as any)) },
    { label: '上季', start: today.subtract(1, 'quarter' as any).startOf('quarter' as any), end: today.subtract(1, 'quarter' as any).endOf('quarter' as any) },
  ];
  const primaryLabels = ['昨日', '近7天', '本周', '上周', '本月', '上月'];
  const primaryShortcuts = shortcuts.filter(s => primaryLabels.includes(s.label));
  const secondaryShortcuts = shortcuts.filter(s => !primaryLabels.includes(s.label));

  const getActiveLabel = (): string | null => {
    if (lastClickedRef.current) {
      const btn = shortcuts.find(s => s.label === lastClickedRef.current);
      if (btn && btn.start.format('YYYY-MM-DD') === start && btn.end.format('YYYY-MM-DD') === end) {
        return lastClickedRef.current;
      }
    }
    const matched = shortcuts.find(s =>
      s.start.format('YYYY-MM-DD') === start && s.end.format('YYYY-MM-DD') === end
    );
    return matched ? matched.label : null;
  };

  const activeLabel = getActiveLabel();
  const activeSecondary = secondaryShortcuts.find(s => s.label === activeLabel) || null;

  const handleRange = (dates: [Dayjs | null, Dayjs | null] | null) => {
    if (dates && dates[0] && dates[1]) {
      lastClickedRef.current = null;
      onChange(dates[0].format('YYYY-MM-DD'), dates[1].format('YYYY-MM-DD'));
    }
  };

  const handleShortcut = (label: string, s: Dayjs, e: Dayjs) => {
    lastClickedRef.current = label;
    onChange(s.format('YYYY-MM-DD'), e.format('YYYY-MM-DD'));
  };

  const moreMenu: MenuProps = {
    items: secondaryShortcuts.map(s => ({ key: s.label, label: s.label })),
    onClick: ({ key }) => {
      const shortcut = secondaryShortcuts.find(s => s.label === key);
      if (shortcut) handleShortcut(shortcut.label, shortcut.start, shortcut.end);
    },
  };

  const content = (
    <div className="bi-date-filter">
      <CalendarOutlined style={{ color: '#94a3b8', fontSize: 14, marginRight: 2 }} />
      <span style={{ fontWeight: 500, color: '#64748b', marginRight: 4, fontSize: 13 }}>时间</span>
      <RangePicker
        value={[dayjs(start), dayjs(end)]}
        onChange={handleRange as any}
        disabledDate={(current) => current && current.isAfter(yesterday, 'day')}
        size="small"
        style={{ width: 240 }}
      />
      <div style={{ width: 1, height: 20, background: '#e2e8f0', margin: '0 4px' }} />
      <Space size={4} wrap>
        {primaryShortcuts.map(s => (
          <Button
            key={s.label}
            size="small"
            type={activeLabel === s.label ? 'primary' : 'default'}
            onClick={() => handleShortcut(s.label, s.start, s.end)}
          >
            {s.label}
          </Button>
        ))}
        <Dropdown menu={moreMenu} trigger={['click']}>
          <Button size="small" type={activeSecondary ? 'primary' : 'default'}>
            {activeSecondary ? activeSecondary.label : '更多'}
          </Button>
        </Dropdown>
      </Space>
    </div>
  );

  return portalTarget ? createPortal(content, portalTarget) : content;
};

export default DateFilter;
