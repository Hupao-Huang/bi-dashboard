import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Card, DatePicker, Empty, Input, InputNumber, message, Radio, Space, Spin, Switch, Table, Tag, Tooltip } from 'antd';
import { ReloadOutlined, SaveOutlined, SearchOutlined } from '@ant-design/icons';
import dayjs, { Dayjs } from 'dayjs';
import { API_BASE } from '../../config';

type RangeMode = 'recent6m' | 'all';

interface ForecastItem {
  sku_code: string;
  goods_name: string;
  suggestions: Record<string, number>;
  forecasts: Record<string, number>;
  base_avgs?: Record<string, number>;
  seasonal_factor?: number;
}

const SalesForecast: React.FC = () => {
  const defaultYM = dayjs().add(1, 'month');
  const [ym, setYm] = useState<Dayjs>(defaultYM);
  const [rangeMode, setRangeMode] = useState<RangeMode>('recent6m');
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [regions, setRegions] = useState<string[]>([]);
  const [items, setItems] = useState<ForecastItem[]>([]);
  const [holidayContext, setHolidayContext] = useState('');
  // userValues[sku][region] = 用户填的数字; 未填则不存
  const [userValues, setUserValues] = useState<Record<string, Record<string, number>>>({});
  const [keyword, setKeyword] = useState('');
  const [hideEmpty, setHideEmpty] = useState(true);

  const ymStr = ym.format('YYYY-MM');

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/offline/sales-forecast?ym=${ymStr}&range=${rangeMode}`, {
        credentials: 'include',
      });
      const json = await res.json();
      if (!res.ok) {
        message.error(`加载失败：${json.msg || res.status}`);
        return;
      }
      const data = json.data || {};
      setRegions(data.regions || []);
      setHolidayContext(data.holiday_context || '');
      const list: ForecastItem[] = data.items || [];
      setItems(list);
      // 初始化 userValues = 已保存的预测
      const uv: Record<string, Record<string, number>> = {};
      list.forEach(it => {
        if (it.forecasts && Object.keys(it.forecasts).length > 0) {
          uv[it.sku_code] = { ...it.forecasts };
        }
      });
      setUserValues(uv);
    } catch (e: any) {
      message.error(`请求失败：${e.message}`);
    } finally {
      setLoading(false);
    }
  }, [ymStr, rangeMode]);

  useEffect(() => { fetchData(); }, [fetchData]);

  const handleCellChange = (sku: string, region: string, val: number | null) => {
    setUserValues(prev => {
      const next = { ...prev };
      if (val === null || val === undefined) {
        if (next[sku]) {
          const cp = { ...next[sku] };
          delete cp[region];
          if (Object.keys(cp).length === 0) {
            delete next[sku];
          } else {
            next[sku] = cp;
          }
        }
      } else {
        next[sku] = { ...(next[sku] || {}), [region]: val };
      }
      return next;
    });
  };

  const handleApplySuggestions = () => {
    // 把所有"未填"的格子按建议值填上
    const next: Record<string, Record<string, number>> = { ...userValues };
    items.forEach(it => {
      regions.forEach(region => {
        if (next[it.sku_code]?.[region] !== undefined) return; // 已有用户值,跳过
        const sug = it.suggestions?.[region];
        if (sug && sug > 0) {
          next[it.sku_code] = { ...(next[it.sku_code] || {}), [region]: sug };
        }
      });
    });
    setUserValues(next);
    message.success('已用系统建议值填充空格');
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const payload: any[] = [];
      Object.keys(userValues).forEach(sku => {
        const item = items.find(it => it.sku_code === sku);
        Object.keys(userValues[sku]).forEach(region => {
          payload.push({
            sku_code: sku,
            goods_name: item?.goods_name || '',
            region,
            forecast_qty: userValues[sku][region],
          });
        });
      });
      if (payload.length === 0) {
        message.warning('没有要保存的预测数据');
        setSaving(false);
        return;
      }
      const res = await fetch(`${API_BASE}/api/offline/sales-forecast/save`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ym: ymStr, items: payload }),
      });
      const json = await res.json();
      if (res.ok) {
        message.success(json.data?.message || '保存成功');
      } else {
        message.error(json.msg || '保存失败');
      }
    } catch (e: any) {
      message.error(`网络错误：${e.message}`);
    } finally {
      setSaving(false);
    }
  };

  const filteredItems = useMemo(() => {
    let list = items;
    if (hideEmpty) {
      list = list.filter(it => {
        const hasSuggest = it.suggestions && Object.values(it.suggestions).some(v => v > 0);
        const hasForecast = it.forecasts && Object.keys(it.forecasts).length > 0;
        const hasUserInput = userValues[it.sku_code] && Object.keys(userValues[it.sku_code]).length > 0;
        return hasSuggest || hasForecast || hasUserInput;
      });
    }
    if (keyword.trim()) {
      const kw = keyword.trim().toLowerCase();
      list = list.filter(it =>
        it.goods_name?.toLowerCase().includes(kw) || it.sku_code?.toLowerCase().includes(kw)
      );
    }
    return list;
  }, [items, keyword, hideEmpty, userValues]);

  const predictMonthLabel = ym.format('M月');

  const columns = useMemo(() => {
    const cols: any[] = [
      {
        title: '货品',
        dataIndex: 'goods_name',
        key: 'goods_name',
        fixed: 'left' as const,
        width: 220,
        render: (v: string, row: ForecastItem) => {
          const f = row.seasonal_factor ?? 1;
          let factorBadge: React.ReactNode = null;
          if (f >= 1.2) {
            factorBadge = <Tag color="orange" style={{ marginInlineStart: 4 }}>×{f.toFixed(2)}</Tag>;
          } else if (f <= 0.8) {
            factorBadge = <Tag color="blue" style={{ marginInlineStart: 4 }}>×{f.toFixed(2)}</Tag>;
          }
          return (
            <Tooltip title={f !== 1 ? `${predictMonthLabel}季节系数 ${f.toFixed(2)} (${f >= 1.2 ? '旺季' : f <= 0.8 ? '淡季' : '中性'})` : null}>
              <div>
                <div style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{v || '(未命名)'}</div>
                <div style={{ color: '#94a3b8' }}>{row.sku_code}{factorBadge}</div>
              </div>
            </Tooltip>
          );
        },
      },
    ];
    regions.forEach(region => {
      cols.push({
        title: region,
        key: region,
        width: 100,
        align: 'center' as const,
        render: (_: any, row: ForecastItem) => {
          const userVal = userValues[row.sku_code]?.[region];
          const suggest = row.suggestions?.[region];
          const base = row.base_avgs?.[region];
          const factor = row.seasonal_factor ?? 1;
          const placeholder = suggest && suggest > 0 ? `建议 ${suggest}` : '—';
          const tooltipTitle =
            base && base > 0 && suggest
              ? `近3月均 ${base.toFixed(1)} 件 × ${predictMonthLabel}系数 ${factor.toFixed(2)} ≈ ${suggest} 件`
              : null;
          const input = (
            <InputNumber
              value={userVal ?? null}
              onChange={val => handleCellChange(row.sku_code, region, val as number | null)}
              min={0}
              step={1}
              precision={0}
              placeholder={placeholder}
              style={{ width: '100%' }}
              variant="borderless"
            />
          );
          return tooltipTitle ? <Tooltip title={tooltipTitle}>{input}</Tooltip> : input;
        },
      });
    });
    // 线下总计列 - 9 大区填值合计 (实时跟随用户输入)
    cols.push({
      title: '线下总计',
      key: '_offline_total',
      width: 110,
      fixed: 'right' as const,
      align: 'center' as const,
      render: (_: any, row: ForecastItem) => {
        const uv = userValues[row.sku_code] || {};
        let total = 0;
        regions.forEach(r => { total += uv[r] || 0; });
        return total > 0 ? <Tag color="blue">{total.toLocaleString()}</Tag> : <span style={{ color: '#bfbfbf' }}>—</span>;
      },
    });
    return cols;
  }, [regions, userValues, predictMonthLabel]);

  const filledCount = useMemo(() => {
    let c = 0;
    Object.values(userValues).forEach(m => { c += Object.keys(m).length; });
    return c;
  }, [userValues]);

  return (
    <Card
      title={
        <Space size="middle" wrap>
          <span style={{ fontWeight: 600 }}>预测月份</span>
          <DatePicker
            picker="month"
            value={ym}
            onChange={d => d && setYm(d)}
            allowClear={false}
            format="YYYY-MM"
          />
          {holidayContext && <Tag color="gold">含 {holidayContext} 假期</Tag>}
          <span style={{ fontWeight: 600 }}>SKU 范围</span>
          <Radio.Group value={rangeMode} onChange={e => setRangeMode(e.target.value)}>
            <Radio.Button value="recent6m">近 6 个月有销量</Radio.Button>
            <Radio.Button value="all">全部（近 12 月有销量）</Radio.Button>
          </Radio.Group>
          <Input
            allowClear
            prefix={<SearchOutlined />}
            placeholder="搜货品名或 SKU"
            value={keyword}
            onChange={e => setKeyword(e.target.value)}
            style={{ width: 200 }}
          />
          <Space size={6}>
            <Switch checked={hideEmpty} onChange={setHideEmpty} size="small" />
            <span>仅看有销量历史的 SKU</span>
          </Space>
          <Tag color="blue">已填 {filledCount} 格</Tag>
        </Space>
      }
      extra={
        <Space>
          <Button icon={<ReloadOutlined />} onClick={fetchData}>重新加载</Button>
          <Button onClick={handleApplySuggestions}>一键填充建议值</Button>
          <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={handleSave}>
            保存
          </Button>
        </Space>
      }
    >
      <Spin spinning={loading}>
        {filteredItems.length === 0 && !loading ? (
          <Empty description={items.length === 0 ? '该月份范围内暂无有销量的 SKU' : '没有匹配关键词的货品'} />
        ) : (
          <Table
            rowKey="sku_code"
            columns={columns}
            dataSource={filteredItems}
            pagination={{ pageSize: 50, showSizeChanger: true, pageSizeOptions: [20, 50, 100, 200] }}
            scroll={{ x: 220 + regions.length * 100 + 110 }}
            size="small"
          />
        )}
      </Spin>
    </Card>
  );
};

export default SalesForecast;
