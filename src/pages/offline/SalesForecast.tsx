import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Button, Card, DatePicker, Empty, Input, InputNumber, message, Popconfirm, Space, Spin, Switch, Table, Tag, Tooltip } from 'antd';
import { ReloadOutlined, SaveOutlined, SearchOutlined } from '@ant-design/icons';
import dayjs, { Dayjs } from 'dayjs';
import { API_BASE } from '../../config';

interface ForecastItem {
  sku_code: string;
  goods_name: string;
  suggestions: Record<string, number>;
  forecasts: Record<string, number>;
  base_avgs?: Record<string, number>;
  seasonal_factor?: number;
  recent_season_avg?: number;
  seasonal_replaced?: boolean;
}

const SalesForecast: React.FC = () => {
  const defaultYM = dayjs().add(1, 'month');
  const [ym, setYm] = useState<Dayjs>(defaultYM);
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
      const res = await fetch(`${API_BASE}/api/offline/sales-forecast?ym=${ymStr}&range=recent6m`, {
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
  }, [ymStr]);

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

  const handlePredict = () => {
    // 用最新算法建议值填表 (覆盖所有), 点保存才落库
    const next: Record<string, Record<string, number>> = {};
    items.forEach(it => {
      regions.forEach(region => {
        const sug = it.suggestions?.[region];
        if (sug && sug > 0) {
          next[it.sku_code] = { ...(next[it.sku_code] || {}), [region]: sug };
        }
      });
    });
    setUserValues(next);
    message.success('已用最新算法预测填表,点"保存"落库');
  };

  const handleClear = async () => {
    // 1. 调后端清空 DB
    try {
      const res = await fetch(`${API_BASE}/api/offline/sales-forecast/clear?ym=${ymStr}`, {
        method: 'POST',
        credentials: 'include',
      });
      const json = await res.json();
      if (!res.ok) {
        message.error(`清空失败: ${json.msg || res.status}`);
        return;
      }
      // 2. 清前端
      setUserValues({});
      message.success(json.data?.message || '已清空');
      // 3. 重新拉数据 (forecasts 字段会清空)
      fetchData();
    } catch (e: any) {
      message.error(`清空失败: ${e.message}`);
    }
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
        title: '货品名',
        dataIndex: 'goods_name',
        key: 'goods_name',
        fixed: 'left' as const,
        width: 240,
        ellipsis: true,
        render: (v: string) => <Tooltip title={v}>{v || '(未命名)'}</Tooltip>,
      },
      {
        title: '货品编码',
        dataIndex: 'sku_code',
        key: 'sku_code',
        fixed: 'left' as const,
        width: 110,
        render: (v: string) => <span style={{ color: '#64748b' }}>{v}</span>,
      },
      {
        title: `${predictMonthLabel}季节`,
        key: '_seasonal',
        fixed: 'left' as const,
        width: 110,
        align: 'center' as const,
        render: (_: any, row: ForecastItem) => {
          const f = row.seasonal_factor ?? 1;
          const replaced = !!row.seasonal_replaced;
          const sourceText = replaced
            ? `品类替代 — 该 SKU 该月历史数据被营销/促销污染 (同月 2 年波动 >30%), 改用调味品同类货品中位数代表客观规律`
            : `单品自身 — 该 SKU 该月历史稳定 (同月 2 年波动 ≤30%), 用真实季节规律`;
          let factorTag: React.ReactNode;
          if (f >= 1.2) factorTag = <Tag color="orange" style={{ margin: 0 }}>×{f.toFixed(2)}</Tag>;
          else if (f <= 0.8) factorTag = <Tag color="blue" style={{ margin: 0 }}>×{f.toFixed(2)}</Tag>;
          else factorTag = <Tag color="default" style={{ margin: 0 }}>×{f.toFixed(2)}</Tag>;
          const sourceTag = replaced
            ? <Tag color="warning" style={{ margin: 0 }}>替代</Tag>
            : <Tag color="success" style={{ margin: 0 }}>客观</Tag>;
          return (
            <Tooltip title={`${predictMonthLabel}系数 ${f.toFixed(2)} · ${sourceText}`}>
              <Space size={2}>{factorTag}{sourceTag}</Space>
            </Tooltip>
          );
        },
      },
    ];
    regions.forEach(region => {
      cols.push({
        title: region,
        key: region,
        width: 90,
        align: 'center' as const,
        render: (_: any, row: ForecastItem) => {
          const userVal = userValues[row.sku_code]?.[region];
          const suggest = row.suggestions?.[region];
          const base = row.base_avgs?.[region];
          const factor = row.seasonal_factor ?? 1;
          const recentAvg = row.recent_season_avg ?? 1;
          const placeholder = suggest && suggest > 0 ? `建议 ${suggest}` : '—';
          // Tooltip 完整公式: 基础 ÷ 近3月系数均 × 预测月系数 ≈ 建议值
          const tooltipTitle =
            base && base > 0 && suggest
              ? (
                <div>
                  <div>近3月均 {base.toFixed(1)} 件</div>
                  <div>÷ 近3月季节系数均 {recentAvg.toFixed(2)}</div>
                  <div>× {predictMonthLabel}系数 {factor.toFixed(2)}</div>
                  <div>≈ 建议 {suggest} 件</div>
                  {userVal !== undefined && userVal !== suggest ? (
                    <div style={{ marginTop: 4, color: '#faad14' }}>当前已填 {userVal}, 跟新算法建议 {suggest} 有差异</div>
                  ) : null}
                </div>
              )
              : null;
          // cell 已填值跟新建议差异 ≥ 20% 加视觉提示
          const hasDiff = userVal !== undefined && userVal !== null && suggest && suggest > 0
            && Math.abs(userVal - suggest) / suggest >= 0.2;
          const input = (
            <InputNumber
              value={userVal ?? null}
              onChange={val => handleCellChange(row.sku_code, region, val as number | null)}
              min={0}
              step={1}
              precision={0}
              placeholder={placeholder}
              style={{ width: '100%', ...(hasDiff ? { background: '#fff7e6' } : {}) }}
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
    <>
    <Alert
      type="info"
      showIcon
      closable
      style={{ marginBottom: 12 }}
      message="销量预测算法说明"
      description={
        <div style={{ lineHeight: 1.8 }}>
          <div><b>核心公式</b>:预测值 = 近 3 月均 ÷ 近 3 月季节系数均 × 预测月季节系数 × 大区同比 × 大区环比</div>
          <div><b>5 层智能叠加</b>:</div>
          <ol style={{ marginTop: 4, marginBottom: 4, paddingLeft: 20 }}>
            <li><b>季节系数</b> — 用过去 24 个月销量推算每个 SKU 的 12 个月份系数(&gt;1 旺/&lt;1 淡)</li>
            <li><b>春节滑动修正</b> — 1/2 月只看"春节落点跟预测年同月"的历史年, 避免囤货月跟假月混算</li>
            <li><b>客观度判定</b> — 单 SKU 同月 2 年波动 &gt;30% 视为营销污染, 改用同品类货品的月份系数中位数(代表客观规律)</li>
            <li><b>大区同比</b> — 大区近 3 月销量 ÷ 去年同期 = 同比增长率, clamp ±30% 防异常, 捕获年度业务扩张</li>
            <li><b>大区环比</b> — 大区近 1 月 ÷ 近 3 月均 = 短期趋势加速度, clamp ±8%, 春节季月份(近3月含1/2/3) 自动跳过防带飞</li>
          </ol>
          <div><b>过滤范围</b>:只算成品(调味料/酱油/调味汁/干制面/素蚝油/酱类/醋/汤底/番茄沙司/糖), 包材/广宣品自动排除</div>
          <div><b>回测 2026-04 全国精度</b>:95.8% (误差 -4.2%, 算法 33.7 万件 vs 实际 35.2 万件)</div>
          <div><b>使用步骤</b>:右上"清空" → "预测" → 业务手填微调 → "保存"</div>
        </div>
      }
    />
    <Card size="small">
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12, flexWrap: 'wrap', marginBottom: 12 }}>
        <Space size="middle" wrap>
          <span style={{ fontWeight: 600 }}>预测月份</span>
          <DatePicker
            size="small"
            picker="month"
            value={ym}
            onChange={d => d && setYm(d)}
            allowClear={false}
            format="YYYY-MM"
          />
          {holidayContext && <Tag color="gold">含 {holidayContext} 假期</Tag>}
          <Input
            size="small"
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
        <Space size={8}>
          <Popconfirm
            title={`清空 ${ymStr} 全部预测?`}
            description="数据库会删掉该月所有 SKU 大区预测,不可恢复"
            onConfirm={handleClear}
            okText="清空"
            cancelText="取消"
            okButtonProps={{ danger: true }}
          >
            <Button size="small" danger>清空</Button>
          </Popconfirm>
          <Button size="small" onClick={handlePredict}>预测</Button>
          <Button size="small" type="primary" icon={<SaveOutlined />} loading={saving} onClick={handleSave}>
            保存
          </Button>
        </Space>
      </div>
      <Spin spinning={loading}>
        {filteredItems.length === 0 && !loading ? (
          <Empty description={items.length === 0 ? '该月份范围内暂无有销量的 SKU' : '没有匹配关键词的货品'} />
        ) : (
          <Table
            rowKey="sku_code"
            columns={columns}
            dataSource={filteredItems}
            pagination={{ pageSize: 50, showSizeChanger: true, pageSizeOptions: [20, 50, 100, 200] }}
            scroll={{ x: 240 + 110 + 110 + regions.length * 90 + 110 }}
            size="small"
          />
        )}
      </Spin>
    </Card>
    </>
  );
};

export default SalesForecast;
