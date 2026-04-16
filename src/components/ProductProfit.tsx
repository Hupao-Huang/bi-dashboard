import React, { useEffect, useState, useCallback, useRef } from 'react';
import { DEPT_COLORS } from '../chartTheme';
import { Row, Col, Card, Table, Statistic, Spin } from 'antd';
import dayjs from 'dayjs';
import ReactECharts from './Chart';
import DateFilter from './DateFilter';
import PageLoading from './PageLoading';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../config';

interface Props {
  dept: string;
}


const ProductProfit: React.FC<Props> = ({ dept  }) => {
  const abortRef = useRef<AbortController | null>(null);
  const [data, setData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);

  const color = DEPT_COLORS[dept] || '#4f46e5';

  const fetchData = useCallback((s: string, e: string) => {
    abortRef.current?.abort();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setLoading(true);
    fetch(`${API_BASE}/api/department?dept=${dept}&start=${s}&end=${e}`, { signal: ctrl.signal })
      .then(res => res.json())
      .then(res => { setData(res.data); setLoading(false); })
      .catch((e: any) => { if (e?.name !== 'AbortError') setLoading(false); });
  }, [dept]);

  useEffect(() => { fetchData(startDate, endDate); }, [fetchData, startDate, endDate]);

  const handleDateChange = (s: string, e: string) => {
    setStartDate(s);
    setEndDate(e);
  };

  if (loading) return <PageLoading />;
  if (!data) return <div>加载失败</div>;

  const goods = data.goods || [];
  const brands = data.brands || [];

  // 商品销售额 vs 销量 气泡图
  const scatterOption = {
    tooltip: {
      trigger: 'item' as const,
      formatter: (p: any) => {
        const d = p.data;
        return `${d[2]}<br/>销售额: ¥${d[0]?.toLocaleString()}<br/>销量: ${d[1]}件`;
      },
    },
    grid: { left: 80, right: 40, top: 30, bottom: 50 },
    xAxis: {
      type: 'value' as const,
      name: '销售额',
      axisLabel: { formatter: (v: number) => v >= 10000 ? (v / 10000).toFixed(0) + '万' : String(v) },
    },
    yAxis: {
      type: 'value' as const,
      name: '销量(件)',
      min: 0,
    },
    series: [{
      type: 'scatter',
      symbolSize: 10,
      data: goods.slice(0, 50).map((g: any) => [
        g.sales,
        g.qty,
        g.goodsName || g.goodsNo,
      ]),
      itemStyle: { color },
    }],
  };

  // 品牌销售对比（仅销售额）
  const brandSalesOption = {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['销售额', '销量'], top: 0 },
    grid: { left: 120, right: 40, top: 40, bottom: 20 },
    xAxis: {
      type: 'value' as const,
      axisLabel: { formatter: (v: number) => v >= 10000 ? (v / 10000).toFixed(0) + '万' : String(v) },
    },
    yAxis: {
      type: 'category' as const,
      data: brands.slice(0, 10).map((b: any) => b.brand || '未知').reverse(),
    },
    series: [
      {
        name: '销售额',
        type: 'bar',
        data: brands.slice(0, 10).map((b: any) => b.sales).reverse(),
        itemStyle: { color },
        barWidth: 14,
      },
      {
        name: '销量',
        type: 'bar',
        data: brands.slice(0, 10).map((b: any) => b.qty || 0).reverse(),
        itemStyle: { color: '#c7d2fe' },
        barWidth: 14,
      },
    ],
  };

  // 产品统计表格
  const indexedGoods = goods.map((g: any, i: number) => ({ ...g, _rank: i + 1 }));
  const goodsColumns = [
    { title: '排名', dataIndex: '_rank', key: 'rank', width: 50 },
    { title: '编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 110 },
    { title: '商品名称', dataIndex: 'goodsName', key: 'goodsName', ellipsis: true },
    { title: '品牌', dataIndex: 'brand', key: 'brand', width: 80 },
    { title: '销售额', dataIndex: 'sales', key: 'sales', width: 110, render: (v: number) => `¥${v?.toLocaleString()}` },
    { title: '销量', dataIndex: 'qty', key: 'qty', width: 70, render: (v: number) => v?.toLocaleString() },
    { title: '客单价', key: 'avgPrice', width: 100, render: (_: any, row: any) => row.qty > 0 ? `¥${(row.sales / row.qty).toFixed(2)}` : '-' },
  ];

  const totalSales = goods.reduce((s: number, g: any) => s + (g.sales || 0), 0);
  const totalQty = goods.reduce((s: number, g: any) => s + (g.qty || 0), 0);
  const avgOrderValue = totalQty > 0 ? totalSales / totalQty : 0;
  const statCards = [
    { title: '总销售额', value: totalSales, precision: 2, prefix: '¥', accentColor: color },
    { title: '总销量', value: totalQty, suffix: '件', accentColor: '#10b981' },
    { title: '综合客单价', value: avgOrderValue, precision: 2, prefix: '¥', accentColor: '#4f46e5' },
    { title: 'SKU种类数', value: goods.length, suffix: '种', accentColor: '#8b5cf6' },
  ];

  return (
    <div>
      <DateFilter start={startDate} end={endDate} onChange={handleDateChange} />

      {/* 汇总 */}
      <Row gutter={[16, 16]}>
        {statCards.map((card) => (
          <Col xs={24} sm={6} key={card.title}>
            <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
              <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
            </Card>
          </Col>
        ))}
      </Row>

      {/* 散点图 + 品牌对比 */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={12}>
          <Card title="商品销售额 vs 销量分布">
            <ReactECharts lazyUpdate={true} option={scatterOption} style={{ height: 380 }} />
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card title="品牌销售对比 TOP10">
            <ReactECharts lazyUpdate={true} option={brandSalesOption} style={{ height: 380 }} />
          </Card>
        </Col>
      </Row>

      {/* 产品统计表 */}
      <Card className="bi-table-card" title="产品销售统计明细" style={{ marginTop: 16 }}>
        <Table
          dataSource={indexedGoods}
          columns={goodsColumns}
          rowKey="goodsNo"
          pagination={{ pageSize: 20, showSizeChanger: true }}
          size="small"
        />
      </Card>
    </div>
  );
};

export default ProductProfit;
