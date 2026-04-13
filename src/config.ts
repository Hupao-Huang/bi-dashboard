import dayjs from 'dayjs';

// API 地址自动适配：localhost 访问用 localhost，局域网访问用当前 IP
export const API_BASE = `http://${window.location.hostname}:8080`;

// 数据截止日期：昨天（运营数据都是T+1，当天无数据）
export const DATA_END_DATE = dayjs().subtract(1, 'day').format('YYYY-MM-DD');

// 本月默认开始日期：月初，但如果月初就是今天（即1号），end会是上月底，需要保证start<=end
const monthStart = dayjs().startOf('month');
const yesterday = dayjs().subtract(1, 'day');
export const DATA_START_DATE = monthStart.isAfter(yesterday) ? yesterday.startOf('month').format('YYYY-MM-DD') : monthStart.format('YYYY-MM-DD');
