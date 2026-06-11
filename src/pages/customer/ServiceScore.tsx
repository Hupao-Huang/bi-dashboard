import React, { useState, useEffect, useMemo, useCallback } from 'react';
import { Card, Table, Tabs, Typography, Empty, Modal, Tag, Statistic, Row, Col, Tooltip, Spin } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import DateFilter from '../../components/DateFilter';
import Chart from '../../components/Chart';
import { API_BASE } from '../../config';

const { Text } = Typography;

// 平台 Tab 顺序 (跟客服总览风格一致, 京东系两个放一起)
const PLATFORM_ORDER = ['天猫', '抖音', 'POP', '京东自营', '拼多多', '快手', '小红书', '唯品会'];

// 与后端 service_score.go GetServiceScores 返回的 item 对齐
interface ScoreItem {
  date: string;
  platform: string;
  shopName: string;
  score1: number | null;
  score1Extra: number | null;
  score2: number | null;
  score3: number | null;
  score3Extra: number | null;
  target: number | null;
}

interface MetricDef {
  key: 'score1' | 'score1Extra' | 'score2' | 'score3' | 'score3Extra';
  label: string;
  pct?: boolean;     // 百分比显示 (库里存 0-1)
  isTarget?: boolean; // 跟"服务分目标"比较的列
}

// 三项分数含义按平台不同 (RPA 表头统一写物流/商品/服务, 实际语义客服部门确认)
const metricDefs = (platform: string): MetricDef[] => {
  if (platform === '京东自营') {
    return [
      { key: 'score1', label: '平均响应时间' },
      { key: 'score2', label: '应答率', pct: true },
      { key: 'score3', label: '满意度', pct: true },
    ];
  }
  if (platform === '拼多多') {
    return [
      { key: 'score1', label: '发货分' },
      { key: 'score1Extra', label: '物流分' },
      { key: 'score2', label: '商品分' },
      { key: 'score3', label: '服务分' },
      { key: 'score3Extra', label: '基础分' },
    ];
  }
  return [
    { key: 'score1', label: '物流分' },
    { key: 'score2', label: '商品分' },
    { key: 'score3', label: '服务分', isTarget: true },
  ];
};

const orderPlatforms = (pfs: string[]) => [...pfs].sort((a, b) => {
  const ia = PLATFORM_ORDER.indexOf(a);
  const ib = PLATFORM_ORDER.indexOf(b);
  return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib);
});

// 数值显示: null → -; 百分比 ×100; 其他原样去尾零
const fmtVal = (v: number | null | undefined, pct?: boolean): string => {
  if (v === null || v === undefined) return '-';
  if (pct) return `${parseFloat((v * 100).toFixed(1))}%`;
  return String(parseFloat(v.toFixed(3)));
};

const ServiceScore: React.FC = () => {
  const yesterday = dayjs().subtract(1, 'day');
  const [startDate, setStartDate] = useState(yesterday.subtract(13, 'day').format('YYYY-MM-DD'));
  const [endDate, setEndDate] = useState(yesterday.format('YYYY-MM-DD'));
  const [list, setList] = useState<ScoreItem[]>([]);
  const [latestDate, setLatestDate] = useState('');
  const [loading, setLoading] = useState(false);
  const [activePlatform, setActivePlatform] = useState('');

  // 趋势弹窗
  const [trendShop, setTrendShop] = useState('');
  const [trendList, setTrendList] = useState<ScoreItem[]>([]);
  const [trendLoading, setTrendLoading] = useState(false);

  const fetchScores = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(
        `${API_BASE}/api/customer/service-scores?date_from=${startDate}&date_to=${endDate}`,
        { credentials: 'include' },
      );
      const json = await res.json();
      if (res.ok && json.data) {
        setList(json.data.list || []);
        setLatestDate(json.data.latestDate || '');
      }
    } catch { /* 网络异常时保留旧数据 */ }
    setLoading(false);
  }, [startDate, endDate]);

  useEffect(() => { fetchScores(); }, [fetchScores]);

  // 按平台分组
  const byPlatform = useMemo(() => {
    const m = new Map<string, ScoreItem[]>();
    list.forEach((it) => {
      if (!m.has(it.platform)) m.set(it.platform, []);
      m.get(it.platform)!.push(it);
    });
    return m;
  }, [list]);

  const platforms = useMemo(() => orderPlatforms(Array.from(byPlatform.keys())), [byPlatform]);
  useEffect(() => {
    if (platforms.length && !platforms.includes(activePlatform)) setActivePlatform(platforms[0]);
  }, [platforms, activePlatform]);

  const platItems = byPlatform.get(activePlatform) || [];
  const metrics = metricDefs(activePlatform);
  const hasTarget = platItems.some((it) => it.target !== null);

  // 透视: 店铺 → 日期 → 数据
  const { shopRows, dates } = useMemo(() => {
    const shopMap = new Map<string, { shop: string; target: number | null; byDate: Map<string, ScoreItem> }>();
    const dateSet = new Set<string>();
    platItems.forEach((it) => {
      dateSet.add(it.date);
      if (!shopMap.has(it.shopName)) shopMap.set(it.shopName, { shop: it.shopName, target: it.target, byDate: new Map() });
      const row = shopMap.get(it.shopName)!;
      row.byDate.set(it.date, it);
      if (it.target !== null) row.target = it.target; // 目标取最新文件的值
    });
    return {
      shopRows: Array.from(shopMap.values()),
      dates: Array.from(dateSet).sort().reverse(), // 最新日期在左
    };
  }, [platItems]);

  // 达标统计 (最新有数日期, 仅有目标的平台)
  const latestStat = useMemo(() => {
    if (!dates.length) return { date: '', met: 0, miss: 0, missShops: [] as string[] };
    const d = dates[0];
    let met = 0; const missShops: string[] = [];
    shopRows.forEach((row) => {
      const it = row.byDate.get(d);
      if (!it || it.target === null || it.score3 === null) return;
      if (it.score3 >= it.target) met += 1; else missShops.push(row.shop);
    });
    return { date: d, met, miss: missShops.length, missShops };
  }, [shopRows, dates]);

  const columns: ColumnsType<typeof shopRows[number]> = useMemo(() => {
    const cols: ColumnsType<typeof shopRows[number]> = [
      {
        title: '店铺',
        dataIndex: 'shop',
        fixed: 'left',
        width: 200,
        render: (v: string) => (
          <Tooltip title="点击查看近30天走势">
            <a onClick={() => setTrendShop(v)}>{v}</a>
          </Tooltip>
        ),
      },
    ];
    if (hasTarget) {
      cols.push({
        title: '服务分目标',
        dataIndex: 'target',
        width: 90,
        fixed: 'left',
        render: (v: number | null) => fmtVal(v),
      });
    }
    // 日期组隔条纹背景 + 服务分列(各平台第3项主分)底色加粗, 让日期边界和重点列一眼可辨
    const zebraBg = '#fafafa';
    const focusBg = '#e6f4ff';
    dates.forEach((d, gi) => {
      const groupBg = gi % 2 === 1 ? zebraBg : undefined;
      cols.push({
        title: dayjs(d).format('MM-DD'),
        onHeaderCell: () => ({ style: groupBg ? { background: zebraBg } : {} }),
        children: metrics.map((m) => {
          const isFocus = m.key === 'score3';
          const bg = isFocus ? focusBg : groupBg;
          return {
            title: m.label,
            width: m.label.length >= 5 ? 96 : 76,
            onHeaderCell: () => ({ style: bg ? { background: bg, fontWeight: isFocus ? 600 : undefined } : {} }),
            onCell: () => ({ style: bg ? { background: bg } : {} }),
            render: (_: unknown, row: typeof shopRows[number]) => {
              const it = row.byDate.get(d);
              const v = it ? it[m.key] : null;
              const below = m.isTarget && it && v !== null && it.target !== null && v < it.target;
              const text = fmtVal(v, m.pct);
              if (below) return <Text type="danger" strong={isFocus}>{text}</Text>;
              return isFocus ? <Text strong>{text}</Text> : text;
            },
          };
        }),
      });
    });
    return cols;
  }, [dates, metrics, hasTarget]);

  // 趋势弹窗: 独立拉近30天 (不受页面日期范围影响, 单天筛选也能看走势)
  useEffect(() => {
    if (!trendShop) return;
    (async () => {
      setTrendLoading(true);
      try {
        const end = latestDate || dayjs().format('YYYY-MM-DD');
        const start = dayjs(end).subtract(29, 'day').format('YYYY-MM-DD');
        const res = await fetch(
          `${API_BASE}/api/customer/service-scores?date_from=${start}&date_to=${end}`,
          { credentials: 'include' },
        );
        const json = await res.json();
        if (res.ok && json.data) {
          setTrendList((json.data.list || []).filter(
            (it: ScoreItem) => it.platform === activePlatform && it.shopName === trendShop,
          ));
        }
      } catch { /* 弹窗内失败显示空 */ }
      setTrendLoading(false);
    })();
  }, [trendShop, activePlatform, latestDate]);

  const trendOption = useMemo(() => {
    if (!trendList.length) return null;
    const ds = trendList.map((it) => it.date).sort();
    const byDate = new Map(trendList.map((it) => [it.date, it]));
    const target = trendList.find((it) => it.target !== null)?.target ?? null;
    return {
      tooltip: { trigger: 'axis' as const },
      legend: { top: 0 },
      grid: { top: 40, left: 50, right: 20, bottom: 30 },
      xAxis: { type: 'category' as const, data: ds.map((d) => dayjs(d).format('MM-DD')) },
      yAxis: { type: 'value' as const, scale: true },
      series: metrics.map((m) => ({
        name: m.label,
        type: 'line' as const,
        connectNulls: true,
        data: ds.map((d) => {
          const v = byDate.get(d)?.[m.key];
          if (v === null || v === undefined) return null;
          return m.pct ? parseFloat((v * 100).toFixed(1)) : v;
        }),
        ...(m.isTarget && target !== null ? {
          markLine: {
            symbol: 'none',
            data: [{ yAxis: target, name: '目标' }],
            lineStyle: { type: 'dashed' as const },
            label: { formatter: `目标 ${target}` },
          },
        } : {}),
      })),
    };
  }, [trendList, metrics]);

  return (
    <div>
      <Card className="bi-filter-card" size="small" style={{ marginBottom: 12 }}>
        <DateFilter start={startDate} end={endDate} onChange={(s, e) => { setStartDate(s); setEndDate(e); }} />
      </Card>

      <Card size="small" style={{ marginBottom: 12 }}>
        <Row gutter={16}>
          <Col span={6}><Statistic title="店铺数" value={shopRows.length} /></Col>
          {hasTarget ? (
            <>
              <Col span={6}><Statistic title={`达标店铺 (${latestStat.date ? dayjs(latestStat.date).format('MM-DD') : '-'})`} value={latestStat.met} /></Col>
              <Col span={6}><Statistic title="未达标店铺" value={latestStat.miss} /></Col>
              <Col span={6}>
                {latestStat.missShops.length > 0 && (
                  <div>
                    <Text type="secondary">未达标名单</Text>
                    <div>{latestStat.missShops.map((s) => <Tag key={s} color="red">{s}</Tag>)}</div>
                  </div>
                )}
              </Col>
            </>
          ) : (
            <Col span={12}><Text type="secondary">该平台没有设置服务分目标, 仅展示每日数据</Text></Col>
          )}
        </Row>
      </Card>

      <Card size="small">
        <Tabs
          activeKey={activePlatform}
          onChange={setActivePlatform}
          items={platforms.map((p) => ({ key: p, label: p }))}
        />
        {shopRows.length === 0 ? (
          <Empty description="所选日期没有服务分数据" />
        ) : (
          <Table
            rowKey="shop"
            size="small"
            loading={loading}
            columns={columns}
            dataSource={shopRows}
            pagination={false}
            scroll={{ x: 'max-content' }}
            bordered
          />
        )}
      </Card>

      <Modal
        title={`${trendShop} — 近30天走势`}
        open={!!trendShop}
        onCancel={() => { setTrendShop(''); setTrendList([]); }}
        footer={null}
        width={860}
      >
        {trendLoading ? (
          <div style={{ textAlign: 'center', padding: 40 }}><Spin /></div>
        ) : trendOption ? (
          <Chart option={trendOption} style={{ height: 360 }} />
        ) : (
          <Empty description="暂无走势数据" />
        )}
      </Modal>
    </div>
  );
};

export default ServiceScore;
