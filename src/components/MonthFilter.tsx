import React, { useEffect, useState } from 'react';
import { DatePicker, Space, Button } from 'antd';
import { CalendarOutlined } from '@ant-design/icons';
import dayjs, { Dayjs } from 'dayjs';
import quarterOfYear from 'dayjs/plugin/quarterOfYear';
import { createPortal } from 'react-dom';

dayjs.extend(quarterOfYear);

interface Props {
  start: string;
  end: string;
  onChange: (start: string, end: string) => void;
}

// 月份筛选器 - 用于月度视角的看板
// 输入只能选到月份，传出的 start/end 都是月初/月末的日期字符串
const MonthFilter: React.FC<Props> = ({ start, end, onChange }) => {
  const [portalTarget, setPortalTarget] = useState<HTMLElement | null>(null);
  const today = dayjs();

  useEffect(() => {
    setPortalTarget(document.getElementById('bi-toolbar-slot'));
  }, []);

  const curMonthStart = dayjs(start);
  const curMonthEnd = dayjs(end);

  const shortcuts = [
    { label: '本月', start: today.startOf('month'), end: today.endOf('month') },
    { label: '上月', start: today.subtract(1, 'month').startOf('month'), end: today.subtract(1, 'month').endOf('month') },
    { label: '近3月', start: today.subtract(2, 'month').startOf('month'), end: today.endOf('month') },
    { label: '近6月', start: today.subtract(5, 'month').startOf('month'), end: today.endOf('month') },
    { label: '近12月', start: today.subtract(11, 'month').startOf('month'), end: today.endOf('month') },
    { label: '今年', start: today.startOf('year'), end: today.endOf('month') },
  ];

  const getActiveLabel = (): string | null => {
    const matched = shortcuts.find(s =>
      s.start.format('YYYY-MM-DD') === start && s.end.format('YYYY-MM-DD') === end
    );
    return matched ? matched.label : null;
  };

  const activeLabel = getActiveLabel();

  const handleRange = (dates: [Dayjs | null, Dayjs | null] | null) => {
    if (dates && dates[0] && dates[1]) {
      onChange(dates[0].startOf('month').format('YYYY-MM-DD'), dates[1].endOf('month').format('YYYY-MM-DD'));
    }
  };

  const handleShortcut = (s: Dayjs, e: Dayjs) => {
    onChange(s.format('YYYY-MM-DD'), e.format('YYYY-MM-DD'));
  };

  const content = (
    <div className="bi-date-filter">
      <CalendarOutlined style={{ color: '#94a3b8', fontSize: 14, marginRight: 2 }} />
      <span style={{ fontWeight: 500, color: '#64748b', marginRight: 4, fontSize: 13 }}>月份</span>
      <DatePicker.RangePicker
        picker="month"
        value={[curMonthStart, curMonthEnd]}
        onChange={handleRange as any}
        disabledDate={(current) => current && current.isAfter(today, 'month')}
        size="small"
        style={{ width: 220 }}
      />
      <div style={{ width: 1, height: 20, background: '#e2e8f0', margin: '0 4px' }} />
      <Space size={4} wrap>
        {shortcuts.map(s => (
          <Button
            key={s.label}
            size="small"
            type={activeLabel === s.label ? 'primary' : 'default'}
            onClick={() => handleShortcut(s.start, s.end)}
          >
            {s.label}
          </Button>
        ))}
      </Space>
    </div>
  );

  return portalTarget ? createPortal(content, portalTarget) : content;
};

export default MonthFilter;
