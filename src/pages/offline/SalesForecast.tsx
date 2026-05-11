import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Button, Card, DatePicker, Empty, Input, InputNumber, message, Popconfirm, Radio, Space, Spin, Switch, Table, Tag, Tooltip } from 'antd';
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
  const [algo, setAlgo] = useState<'auto' | 'builtin' | 'prophet' | 'statsforecast'>('auto');
  const [effectiveAlgo, setEffectiveAlgo] = useState('');
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
      const res = await fetch(`${API_BASE}/api/offline/sales-forecast?ym=${ymStr}&range=recent6m&algo=${algo}`, {
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
      setEffectiveAlgo(data.effective_algo || '');
      const list: ForecastItem[] = data.items || [];
      setItems(list);
      // 切换算法时 cell 跟随算法 suggestions 实时变化, 没 suggestion 才 fallback 到数据库 forecasts
      const uv: Record<string, Record<string, number>> = {};
      list.forEach(it => {
        const sug = it.suggestions || {};
        const fc = it.forecasts || {};
        const merged: Record<string, number> = {};
        Object.entries(sug).forEach(([r, q]) => { if (q > 0) merged[r] = q; });
        Object.entries(fc).forEach(([r, q]) => { if (merged[r] === undefined && q > 0) merged[r] = q; });
        if (Object.keys(merged).length > 0) uv[it.sku_code] = merged;
      });
      setUserValues(uv);
    } catch (e: any) {
      message.error(`请求失败：${e.message}`);
    } finally {
      setLoading(false);
    }
  }, [ymStr, algo]);

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
          // Cell 直接显示算法预测值 (没手填时用 suggest, 手填后用 userVal)
          const displayValue = userVal ?? (suggest && suggest > 0 ? suggest : null);
          const tooltipTitle =
            algo === 'builtin' && base && base > 0 && suggest
              ? (
                <div>
                  <div>近3月均 {base.toFixed(1)} 件</div>
                  <div>÷ 近3月季节系数均 {recentAvg.toFixed(2)}</div>
                  <div>× {predictMonthLabel}系数 {factor.toFixed(2)}</div>
                  <div>≈ 建议 {suggest} 件</div>
                </div>
              )
              : null;
          const input = (
            <InputNumber
              value={displayValue}
              onChange={val => handleCellChange(row.sku_code, region, val as number | null)}
              min={0}
              step={1}
              precision={0}
              placeholder="—"
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
    <>
    <Alert
      type="info"
      showIcon
      closable
      style={{ marginBottom: 12 }}
      message="销量预测算法说明"
      description={
        <div style={{ lineHeight: 1.8 }}>
          <div><b>内置算法 (五层叠加)</b>:近3月均 ÷ 近3月季节系数均 × 预测月季节系数 × 大区同比 × 大区环比</div>
          <ol style={{ marginTop: 4, marginBottom: 8, paddingLeft: 20 }}>
            <li><b>季节系数</b> — 24 月历史推 12 月份系数 (&gt;1 旺/&lt;1 淡)</li>
            <li><b>春节滑动修正</b> — 1/2 月按春节落点同月年份对齐</li>
            <li><b>客观度判定</b> — 单 SKU 2 年波动 &gt;30% 视为污染, 用品类中位数替代</li>
            <li><b>大区同比</b> — 近 3 月 ÷ 去年同期 (clamp ±30%)</li>
            <li><b>大区环比</b> — 近 1 月 ÷ 近 3 月均 (clamp ±8%, 春节季自动跳过)</li>
          </ol>
          <div><b>Prophet (Facebook 开源)</b>:贝叶斯加性模型,公式 = 趋势 + 季节性 + 节假日效应 + 残差.内置中国春节假期模型(±30 天囤货 + 假期断崖),日级训练. <span style={{color:'#16a34a'}}>春节月份精度最高</span>.</div>
          <div><b>StatsForecast (Nixtla 开源)</b>:经典统计模型集成 = AutoARIMA + AutoETS + AutoTheta 三模型预测均值. AutoARIMA 自动选择 ARIMA 参数, AutoETS 自动选择指数平滑模型, AutoTheta 基于趋势分解. 月级训练,<span style={{color:'#16a34a'}}>M5/M6 销量比赛冠军级方案</span>.</div>
          <div><b>智能(默认)</b>: 系统按预测月份自动选最优算法 — 1-2 月走 Prophet (春节碾压), 3-12 月走 StatsForecast (平稳月最准). 业务无需懂算法, 跟着月份切.</div>
          <div style={{marginTop:4,color:'#64748b'}}>4 种算法切换时表格数字实时变化,SKU 间相对比例保留,大区合计对齐该算法预测.</div>
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
          <Radio.Group size="small" value={algo} onChange={e => setAlgo(e.target.value)}>
            <Radio.Button value="auto" style={{ padding: '0 10px' }}>智能</Radio.Button>
            <Radio.Button value="builtin" style={{ padding: '0 10px' }}>内置</Radio.Button>
            <Radio.Button value="prophet" style={{ padding: '0 10px' }}>Prophet</Radio.Button>
            <Radio.Button value="statsforecast" style={{ padding: '0 10px' }}>StatsForecast</Radio.Button>
          </Radio.Group>
          {algo === 'auto' && effectiveAlgo && <Tag color="purple">本月走 {effectiveAlgo === 'prophet' ? 'Prophet' : effectiveAlgo === 'statsforecast' ? 'StatsForecast' : effectiveAlgo}</Tag>}
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
