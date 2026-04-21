import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Card, Empty, Table, Tabs, Tooltip } from 'antd';
import { InfoCircleOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import DateFilter from '../../components/DateFilter';
import PageLoading from '../../components/PageLoading';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../../config';

// 带悬停提示的列标题
const inquiryLagTitle = (
  <span>
    询单人数
    <Tooltip title="生意参谋业绩询单数据由 RPA 采集，通常存在 T-3 左右延迟（例：4-20 采集的是 4-17 的数据）。近 3 日空值为正常现象。">
      <InfoCircleOutlined style={{ marginLeft: 4, color: '#94a3b8', fontSize: 12 }} />
    </Tooltip>
  </span>
);

interface ShopStat {
  platform: string;
  shopName: string;
  consultUsers: number;
  inquiryUsers: number;
  salesAmount: number;
  avgFirstRespSeconds: number;
  avgResponseSeconds: number;
  avgSatisfactionRate: number;
  avgConvRate: number;
}

interface OverviewData {
  shopRanking: ShopStat[];
}

interface ApiResponse<T> {
  code: number;
  data: T;
}

const fmtCurrency = (v: number) => (v >= 10000
  ? `¥${(v / 10000).toFixed(2)}万`
  : `¥${v.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`);
const fmtNum = (v: number) => `${Math.round(v).toLocaleString()}`;
const fmtRate = (v: number) => `${v.toFixed(2)}%`;
const fmtSeconds = (v: number) => `${v.toFixed(1)}s`;
const platformTabs = ['天猫', '抖音', '京东', '拼多多', '快手', '小红书'];

const CustomerOverview: React.FC = () => {
  const abortRef = useRef<AbortController | null>(null);
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState<OverviewData | null>(null);
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);
  const [activePlatform, setActivePlatform] = useState<string>('天猫');
  const requestSeqRef = useRef(0);
  const isJD = activePlatform === '京东';
  const isPDD = activePlatform === '拼多多';
  const isXHS = activePlatform === '小红书';
  const isKS = activePlatform === '快手';
  const isTmall = activePlatform === '天猫';
  const responseLabel = isJD ? '平响(秒)' : '新平响(秒)';
  const satisfactionLabel = isJD ? '客服满意率' : '满意率';
  const consultLabel = isJD ? '售前接待人数' : '询单人数';
  const convLabel = isJD ? '下单转化率' : '询单转化率';

  const fetchData = useCallback(async (start: string, end: string, platform: string) => {
    const reqId = ++requestSeqRef.current;
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    try {
      const query = `${API_BASE}/api/customer/overview?start=${start}&end=${end}&platform=${encodeURIComponent(platform)}&_ts=${Date.now()}`;
      const res = await fetch(query, { credentials: 'include', cache: 'no-store' });
      const json = await res.json() as ApiResponse<OverviewData> | OverviewData;
      if (reqId !== requestSeqRef.current) return;
      const nextData = (json as ApiResponse<OverviewData>).data || (json as OverviewData);
      setData(nextData);
    } catch {
      if (reqId !== requestSeqRef.current) return;
      setData(null);
    } finally {
      if (reqId === requestSeqRef.current) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    fetchData(startDate, endDate, activePlatform);
  }, [activePlatform, endDate, fetchData, startDate]);

  const tableData = useMemo(() => (data?.shopRanking || [])
    .filter((item) => item.platform === activePlatform)
    .filter((item) => (
      item.consultUsers > 0
      || item.salesAmount > 0
      || item.avgFirstRespSeconds > 0
      || item.avgResponseSeconds > 0
      || item.avgSatisfactionRate > 0
      || item.avgConvRate > 0
    )), [activePlatform, data]);

  const columns: ColumnsType<ShopStat> = useMemo(() => {
    if (isKS) {
      return [
        { title: '店铺', dataIndex: 'shopName', key: 'shopName', width: 320, ellipsis: true },
        { title: '好评率', dataIndex: 'avgSatisfactionRate', key: 'avgSatisfactionRate', render: (v: number) => fmtRate(v || 0) },
        { title: '3分钟人工回复率', dataIndex: 'avgResponseSeconds', key: 'avgResponseSeconds', render: (v: number) => fmtRate(v || 0) },
        { title: '客服销售额', dataIndex: 'salesAmount', key: 'salesAmount', render: (v: number) => fmtCurrency(v || 0) },
        { title: '询单人数', dataIndex: 'consultUsers', key: 'consultUsers', render: (v: number) => fmtNum(v || 0) },
        { title: '询单转化率', dataIndex: 'avgConvRate', key: 'avgConvRate', render: (v: number) => fmtRate(v || 0) },
      ];
    }
    if (isXHS) {
      return [
        { title: '店铺', dataIndex: 'shopName', key: 'shopName', width: 320, ellipsis: true },
        { title: '好评率', dataIndex: 'avgSatisfactionRate', key: 'avgSatisfactionRate', render: (v: number) => fmtRate(v || 0) },
        { title: '3分钟人工回复率', dataIndex: 'avgResponseSeconds', key: 'avgResponseSeconds', render: (v: number) => fmtRate(v || 0) },
        { title: '客服销售额', dataIndex: 'salesAmount', key: 'salesAmount', render: (v: number) => fmtCurrency(v || 0) },
        { title: '询单转化率', dataIndex: 'avgConvRate', key: 'avgConvRate', render: (v: number) => fmtRate(v || 0) },
      ];
    }
    if (isPDD) {
      return [
        { title: '店铺', dataIndex: 'shopName', key: 'shopName', width: 300, ellipsis: true },
        { title: '三分钟回复率', dataIndex: 'avgSatisfactionRate', key: 'avgSatisfactionRate', render: (v: number) => fmtRate(v || 0) },
        { title: '客服销售额', dataIndex: 'salesAmount', key: 'salesAmount', render: (v: number) => fmtCurrency(v || 0) },
        { title: '询单人数', dataIndex: 'consultUsers', key: 'consultUsers', render: (v: number) => fmtNum(v || 0) },
        { title: '询单转化率', dataIndex: 'avgConvRate', key: 'avgConvRate', render: (v: number) => fmtRate(v || 0) },
      ];
    }
    if (isTmall) {
      return [
        { title: '店铺', dataIndex: 'shopName', key: 'shopName', width: 260, ellipsis: true },
        { title: '首响(秒)', dataIndex: 'avgFirstRespSeconds', key: 'avgFirstRespSeconds', render: (v: number) => fmtSeconds(v || 0) },
        { title: responseLabel, dataIndex: 'avgResponseSeconds', key: 'avgResponseSeconds', render: (v: number) => fmtSeconds(v || 0) },
        { title: satisfactionLabel, dataIndex: 'avgSatisfactionRate', key: 'avgSatisfactionRate', render: (v: number) => fmtRate(v || 0) },
        { title: '客服销售额', dataIndex: 'salesAmount', key: 'salesAmount', render: (v: number) => fmtCurrency(v || 0) },
        { title: '咨询人数', dataIndex: 'consultUsers', key: 'consultUsers', render: (v: number) => fmtNum(v || 0) },
        { title: inquiryLagTitle, dataIndex: 'inquiryUsers', key: 'inquiryUsers', render: (v: number) => fmtNum(v || 0) },
        { title: convLabel, dataIndex: 'avgConvRate', key: 'avgConvRate', render: (v: number) => fmtRate(v || 0) },
      ];
    }
    return [
      { title: '店铺', dataIndex: 'shopName', key: 'shopName', width: 280, ellipsis: true },
      { title: '首响(秒)', dataIndex: 'avgFirstRespSeconds', key: 'avgFirstRespSeconds', render: (v: number) => fmtSeconds(v || 0) },
      { title: responseLabel, dataIndex: 'avgResponseSeconds', key: 'avgResponseSeconds', render: (v: number) => fmtSeconds(v || 0) },
      { title: satisfactionLabel, dataIndex: 'avgSatisfactionRate', key: 'avgSatisfactionRate', render: (v: number) => fmtRate(v || 0) },
      { title: '客服销售额', dataIndex: 'salesAmount', key: 'salesAmount', render: (v: number) => fmtCurrency(v || 0) },
      { title: consultLabel, dataIndex: 'consultUsers', key: 'consultUsers', render: (v: number) => fmtNum(v || 0) },
      { title: convLabel, dataIndex: 'avgConvRate', key: 'avgConvRate', render: (v: number) => fmtRate(v || 0) },
    ];
  }, [consultLabel, convLabel, isKS, isPDD, isTmall, isXHS, responseLabel, satisfactionLabel]);

  if (loading && !data) return <PageLoading />;

  return (
    <div>
      <DateFilter start={startDate} end={endDate} onChange={(start, end) => { setStartDate(start); setEndDate(end); }} />
      <Card style={{ marginBottom: 12 }}>
        <Tabs
          activeKey={activePlatform}
          onChange={setActivePlatform}
          items={platformTabs.map((platform) => ({ key: platform, label: platform }))}
        />
      </Card>
      <Card title={`${activePlatform}客服数据明细`}>
        {tableData.length === 0 ? (
          <Empty description={`暂无${activePlatform}客服数据`} />
        ) : (
          <Table<ShopStat>
            rowKey={(row) => row.shopName}
            size="small"
            pagination={false}
            dataSource={tableData}
            columns={columns}
            scroll={{ x: 1100 }}
          />
        )}
      </Card>
    </div>
  );
};

export default CustomerOverview;
