import React, { useEffect, useState, useCallback } from 'react';
import { Card, DatePicker, Table, Statistic, Row, Col, Space, Empty, Spin, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import dayjs, { Dayjs } from 'dayjs';
import { API_BASE } from '../../config';

interface DayRow {
  date: string;
  tradeCount: number;
  goodsCount: number;
  packageCount: number;
}

interface AuditResp {
  month: string;
  rows: DayRow[];
  totalTradeCount: number;
  totalGoodsCount: number;
  totalPackageCount: number;
  tableExists: boolean;
}

const TradeAuditPage: React.FC = () => {
  const [month, setMonth] = useState<Dayjs>(dayjs());
  const [data, setData] = useState<AuditResp | null>(null);
  const [loading, setLoading] = useState(false);

  const fetchData = useCallback(async (m: Dayjs) => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/admin/trade-audit?month=${m.format('YYYY-MM')}`, {
        credentials: 'include',
      });
      const json = await res.json();
      if (res.ok && json.data) {
        setData(json.data);
      } else {
        message.error(json.msg || json.error || '加载失败');
        setData(null);
      }
    } catch (e) {
      message.error('网络错误');
      setData(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchData(month);
  }, [month, fetchData]);

  const columns: ColumnsType<DayRow> = [
    { title: '日期', dataIndex: 'date', key: 'date', width: 130 },
    {
      title: '销售单数', dataIndex: 'tradeCount', key: 'tradeCount', width: 130, align: 'right',
      render: (v: number) => v?.toLocaleString() ?? 0,
      sorter: (a, b) => a.tradeCount - b.tradeCount,
    },
    {
      title: '明细行数', dataIndex: 'goodsCount', key: 'goodsCount', width: 130, align: 'right',
      render: (v: number) => v?.toLocaleString() ?? 0,
      sorter: (a, b) => a.goodsCount - b.goodsCount,
    },
    {
      title: '包裹数', dataIndex: 'packageCount', key: 'packageCount', width: 130, align: 'right',
      render: (v: number) => v?.toLocaleString() ?? 0,
      sorter: (a, b) => a.packageCount - b.packageCount,
    },
  ];

  return (
    <div style={{ padding: 16 }}>
      <Card
        title="销售单核对"
        extra={
          <Space>
            <span>选择月份：</span>
            <DatePicker
              picker="month"
              value={month}
              onChange={(v) => v && setMonth(v)}
              allowClear={false}
              format="YYYY-MM"
            />
          </Space>
        }
      >
        <Spin spinning={loading}>
          {data && data.tableExists ? (
            <>
              <Row gutter={16} style={{ marginBottom: 16 }}>
                <Col span={8}>
                  <Card size="small">
                    <Statistic title={`${data.month} 销售单合计`} value={data.totalTradeCount} />
                  </Card>
                </Col>
                <Col span={8}>
                  <Card size="small">
                    <Statistic title="明细行数合计" value={data.totalGoodsCount} />
                  </Card>
                </Col>
                <Col span={8}>
                  <Card size="small">
                    <Statistic title="包裹数合计" value={data.totalPackageCount} />
                  </Card>
                </Col>
              </Row>
              <Table
                rowKey="date"
                dataSource={data.rows}
                columns={columns}
                pagination={false}
                bordered
                size="middle"
                summary={(pageData) => {
                  let trade = 0, goods = 0, pkg = 0;
                  pageData.forEach((r) => {
                    trade += r.tradeCount;
                    goods += r.goodsCount;
                    pkg += r.packageCount;
                  });
                  return (
                    <Table.Summary fixed>
                      <Table.Summary.Row>
                        <Table.Summary.Cell index={0}>合计</Table.Summary.Cell>
                        <Table.Summary.Cell index={1} align="right">{trade.toLocaleString()}</Table.Summary.Cell>
                        <Table.Summary.Cell index={2} align="right">{goods.toLocaleString()}</Table.Summary.Cell>
                        <Table.Summary.Cell index={3} align="right">{pkg.toLocaleString()}</Table.Summary.Cell>
                      </Table.Summary.Row>
                    </Table.Summary>
                  );
                }}
              />
              <div style={{ marginTop: 12, color: '#666', fontSize: 13 }}>
                按发货日期统计当月每天的销售单数、明细行数、包裹数，用于和吉客云后台逐日核对差异。
              </div>
            </>
          ) : (
            <Empty description={data ? `${data.month} 暂无销售数据` : '加载中…'} />
          )}
        </Spin>
      </Card>
    </div>
  );
};

export default TradeAuditPage;
