import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Button, Card, DatePicker, Empty, Input, InputNumber, message, Popconfirm, Popover, Radio, Space, Spin, Switch, Table, Tabs, Tag, Tooltip } from 'antd';
import { DownloadOutlined, ReloadOutlined, SaveOutlined, SearchOutlined } from '@ant-design/icons';
import dayjs, { Dayjs } from 'dayjs';
import * as XLSX from 'xlsx-js-style';
import { API_BASE } from '../../config';
import SalesForecastBacktest from './SalesForecastBacktest';
import Chart from '../../components/Chart';

// 组件级缓存 — 跨多次 hover 共享, 避免重复 fetch 同一 SKU
const skuTrendCache = new Map<string, { goods_name: string; items: { ym: string; qty: number }[] }>();

interface SkuTrendData { goods_name: string; items: { ym: string; qty: number }[]; }

const SkuTrendPopover: React.FC<{ skuCode: string; children: React.ReactNode }> = ({ skuCode, children }) => {
  const [data, setData] = useState<SkuTrendData | null>(null);
  const [loading, setLoading] = useState(false);
  const [open, setOpen] = useState(false);

  const fetchTrend = useCallback(async () => {
    if (skuTrendCache.has(skuCode)) {
      setData(skuTrendCache.get(skuCode)!);
      return;
    }
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/offline/sales-forecast/sku-trend?sku_code=${encodeURIComponent(skuCode)}`, { credentials: 'include' });
      const json = await res.json();
      if (json.code === 200 && json.data) {
        skuTrendCache.set(skuCode, json.data);
        setData(json.data);
      }
    } finally {
      setLoading(false);
    }
  }, [skuCode]);

  useEffect(() => { if (open) fetchTrend(); }, [open, fetchTrend]);

  const option = useMemo(() => {
    if (!data) return null;
    const months = data.items.map(p => p.ym);
    const values = data.items.map(p => Math.round(p.qty));
    return {
      grid: { top: 40, right: 32, bottom: 48, left: 72 },
      xAxis: { type: 'category', data: months, axisLabel: { fontSize: 11, rotate: 30 } },
      yAxis: { type: 'value', axisLabel: { fontSize: 11 } },
      tooltip: { trigger: 'axis', valueFormatter: (v: number) => `${v.toLocaleString()} 件` },
      series: [{
        type: 'line', data: values, smooth: true, symbol: 'circle', symbolSize: 6,
        lineStyle: { width: 2 }, areaStyle: { opacity: 0.15 },
        // 每个点显示数字标签 — 业务一眼看清每月销量
        label: {
          show: true,
          position: 'top',
          fontSize: 11,
          color: '#1e40af',
          formatter: (p: any) => Number(p.value).toLocaleString(),
        },
      }],
    };
  }, [data]);

  const content = (
    <div style={{ width: 720 }}>
      <div style={{ fontWeight: 600, marginBottom: 4, fontSize: 14 }}>{data?.goods_name || skuCode}</div>
      <div style={{ fontSize: 12, color: '#64748b', marginBottom: 8 }}>近 13 个月实际销量趋势 (大区合计)</div>
      {loading || !data ? (
        <div style={{ height: 360, display: 'flex', alignItems: 'center', justifyContent: 'center' }}><Spin /></div>
      ) : data.items.length === 0 ? (
        <Empty description="近 13 月无销售数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
      ) : (
        <Chart option={option!} style={{ height: 360, width: '100%' }} />
      )}
    </div>
  );

  return (
    <Popover
      content={content}
      trigger="hover"
      placement="right"
      mouseEnterDelay={0.3}
      onOpenChange={setOpen}
      getPopupContainer={() => document.body}
      overlayStyle={{ zIndex: 1080 }}
      autoAdjustOverflow
    >
      {children}
    </Popover>
  );
};

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
  const [effectiveReason, setEffectiveReason] = useState('');
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
      setEffectiveReason(data.effective_reason || '');
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

  const handleDownload = () => {
    const headers = ['货品名', '货品编码', `${predictMonthLabel}季节系数`, ...regions, '线下总计'];
    const aoa: any[][] = [headers];
    filteredItems.forEach(it => {
      const uv = userValues[it.sku_code] || {};
      const cells = regions.map(r => uv[r] ?? null);
      const total = regions.reduce((s, r) => s + (uv[r] || 0), 0);
      aoa.push([
        it.goods_name || '',
        it.sku_code,
        it.seasonal_factor ?? null,
        ...cells,
        total,
      ]);
    });
    // v1.60.3: 加 Antd 风格的视觉样式(表头蓝底白字 + 行边框 + 数值右对齐 + 合计列黄底加粗)
    const ws = XLSX.utils.aoa_to_sheet(aoa);
    // 列宽
    ws['!cols'] = [
      { wch: 32 }, // 货品名
      { wch: 14 }, // 货品编码
      { wch: 12 }, // 季节系数
      ...regions.map(() => ({ wch: 10 })),
      { wch: 12 }, // 线下总计
    ];
    // 行高(表头加高)
    ws['!rows'] = [{ hpt: 22 }, ...filteredItems.map(() => ({ hpt: 18 }))];

    const thinBorder = {
      top:    { style: 'thin', color: { rgb: 'D9D9D9' } },
      bottom: { style: 'thin', color: { rgb: 'D9D9D9' } },
      left:   { style: 'thin', color: { rgb: 'D9D9D9' } },
      right:  { style: 'thin', color: { rgb: 'D9D9D9' } },
    };
    const headerStyle = {
      font:      { bold: true, color: { rgb: 'FFFFFF' }, sz: 11 },
      fill:      { patternType: 'solid', fgColor: { rgb: '1677FF' } },
      alignment: { horizontal: 'center', vertical: 'center', wrapText: true },
      border:    thinBorder,
    };
    const textStyle = {
      alignment: { vertical: 'center' },
      border:    thinBorder,
    };
    const numberStyle = {
      alignment: { horizontal: 'right', vertical: 'center' },
      border:    thinBorder,
    };
    const totalColStyle = {
      font:      { bold: true },
      fill:      { patternType: 'solid', fgColor: { rgb: 'FFF7E6' } }, // 浅黄
      alignment: { horizontal: 'right', vertical: 'center' },
      border:    thinBorder,
    };

    const range = XLSX.utils.decode_range(ws['!ref'] as string);
    const lastCol = range.e.c; // 最后一列(线下总计)
    for (let R = range.s.r; R <= range.e.r; R++) {
      for (let C = range.s.c; C <= range.e.c; C++) {
        const addr = XLSX.utils.encode_cell({ r: R, c: C });
        if (!ws[addr]) ws[addr] = { v: '', t: 's' };
        if (R === 0) {
          ws[addr].s = headerStyle;
        } else if (C === lastCol) {
          ws[addr].s = totalColStyle;
        } else if (C >= 2) {
          ws[addr].s = numberStyle;
        } else {
          ws[addr].s = textStyle;
        }
      }
    }
    const wb = XLSX.utils.book_new();
    XLSX.utils.book_append_sheet(wb, ws, `${ymStr}销量预测`);
    // 用 write + Blob 手动触发, 避免 XLSX.writeFile 在某些浏览器丢文件名
    const wbout = XLSX.write(wb, { bookType: 'xlsx', type: 'array' });
    const blob = new Blob([wbout], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' });
    const filename = `销量预测_${ymStr}_${dayjs().format('YYYY-MM-DD_HHmm')}.xlsx`;
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.style.display = 'none';
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    message.success(`已导出 ${filteredItems.length} 行 Excel: ${filename}`);
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
        render: (v: string) => (
          <SkuTrendPopover skuCode={v}>
            <span style={{ color: '#1677ff', cursor: 'help', borderBottom: '1px dashed #94a3b8' }}>{v}</span>
          </SkuTrendPopover>
        ),
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
    // 线下总计列 - 9 大区填值合计 (实时跟随用户输入) + 偏离标记
    // 偏离 = (本月预测合计 - 近 3 月大区合计月均) / 近 3 月大区合计月均
    // |偏离| > 20% 红/橙 标记, 提醒业务复核
    cols.push({
      title: '线下总计',
      key: '_offline_total',
      width: 150,
      fixed: 'right' as const,
      align: 'center' as const,
      render: (_: any, row: ForecastItem) => {
        const uv = userValues[row.sku_code] || {};
        let total = 0;
        regions.forEach(r => { total += uv[r] || 0; });
        if (total === 0) return <span style={{ color: '#bfbfbf' }}>—</span>;

        // 计算偏离 (基于后端 base_avgs - 该 SKU × 大区 近 3 月销量均值)
        let baseTotal = 0;
        regions.forEach(r => { baseTotal += row.base_avgs?.[r] || 0; });
        const totalTag = <Tag color="blue">{total.toLocaleString()}</Tag>;
        if (baseTotal <= 0) return totalTag;

        const dev = (total - baseTotal) / baseTotal;
        const devPct = Math.round(dev * 100);
        if (Math.abs(devPct) < 20) return totalTag;

        const isHigh = devPct > 0;
        const tooltipText = `预测 ${total.toLocaleString()} / 近 3 月均 ${Math.round(baseTotal).toLocaleString()} / 偏离 ${devPct > 0 ? '+' : ''}${devPct}%`;
        return (
          <Tooltip title={tooltipText}>
            <div style={{ display: 'flex', gap: 4, justifyContent: 'center', alignItems: 'center', cursor: 'help' }}>
              {totalTag}
              <Tag color={Math.abs(devPct) >= 50 ? 'red' : isHigh ? 'orange' : 'gold'} style={{ marginInlineEnd: 0 }}>
                {isHigh ? '↑' : '↓'} {Math.abs(devPct)}%
              </Tag>
            </div>
          </Tooltip>
        );
      },
    });
    return cols;
  }, [regions, userValues, predictMonthLabel]);

  const filledCount = useMemo(() => {
    let c = 0;
    Object.values(userValues).forEach(m => { c += Object.keys(m).length; });
    return c;
  }, [userValues]);

  const forecastTabContent = (
    <>
    <Alert
      type="info"
      showIcon
      closable
      style={{ marginBottom: 12 }}
      message="销量预测算法说明"
      description={
        <div style={{ lineHeight: 1.8 }}>
          <div><b>内置公式 (五层叠加)</b>:近3月均 ÷ 近3月季节系数均 × 预测月季节系数 × 大区同比 × 大区环比</div>
          <ol style={{ marginTop: 4, marginBottom: 8, paddingLeft: 20 }}>
            <li><b>季节系数</b> — 24 月历史推 12 月份系数 (&gt;1 旺/&lt;1 淡)</li>
            <li><b>春节滑动修正</b> — 1/2 月按春节落点同月年份对齐</li>
            <li><b>客观度判定</b> — 单 SKU 2 年波动 &gt;30% 视为污染, 用品类中位数替代</li>
            <li><b>大区同比</b> — 近 3 月 ÷ 去年同期 (clamp ±30%)</li>
            <li><b>大区环比</b> — 近 1 月 ÷ 近 3 月均 (clamp ±8%, 春节季自动跳过)</li>
          </ol>
          <div><b>贝叶斯时序 (Prophet, Facebook 开源)</b>:贝叶斯加性模型,公式 = 趋势 + 季节性 + 节假日效应 + 残差.内置中国春节假期模型(±30 天囤货 + 假期断崖),日级训练. <span style={{color:'#dc2626'}}>当前回测表现差 (MAPE 99%)</span>.</div>
          <div><b>统计集成 (StatsForecast, Nixtla 开源)</b>:经典统计模型集成 = AutoARIMA + AutoETS + AutoTheta 三模型预测均值. M4 / M5 销量比赛冠军级方案, 月级训练. <span style={{color:'#16a34a'}}>当前回测 MAPE 37.7%</span>.</div>
          <div><b>智能路由 (默认)</b>: <span style={{color:'#16a34a'}}>数据驱动</span> — 优先看同月份历史 MAPE, 退而看全部历史平均 MAPE, 都没数据兜底按月份硬编码. 鼠标 hover "本月走 XX" Tag 可看选择理由.</div>
          <div style={{marginTop:4,color:'#64748b'}}>4 种算法切换时表格数字实时变化,SKU 间相对比例保留,大区合计对齐该算法预测.</div>
        </div>
      }
    />
    <Card size="small">
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12, flexWrap: 'wrap', marginBottom: 12 }}>
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
          <Radio.Group value={algo} onChange={e => setAlgo(e.target.value)}>
            <Tooltip title="看历史回测 MAPE 自动选最准算法">
              <Radio.Button value="auto">智能路由</Radio.Button>
            </Tooltip>
            <Tooltip title="近 3 月均 ÷ 季节系数 × 大区同比 × 大区环比">
              <Radio.Button value="builtin">内置公式</Radio.Button>
            </Tooltip>
            <Tooltip title="Facebook 开源贝叶斯加性模型, 含中国春节假期">
              <Radio.Button value="prophet">贝叶斯时序 (Prophet)</Radio.Button>
            </Tooltip>
            <Tooltip title="Nixtla 三模型集成 — AutoARIMA + AutoETS + AutoTheta 的均值">
              <Radio.Button value="statsforecast">统计集成 (StatsForecast)</Radio.Button>
            </Tooltip>
          </Radio.Group>
          {algo === 'auto' && effectiveAlgo && (
            <Tooltip title={effectiveReason || '智能路由根据历史回测 MAPE 选最准的算法'}>
              <Tag color="purple" style={{ cursor: 'help' }}>
                本月走 {effectiveAlgo === 'prophet' ? '贝叶斯时序' : effectiveAlgo === 'statsforecast' ? '统计集成' : effectiveAlgo}
              </Tag>
            </Tooltip>
          )}
          <Input
            allowClear
            prefix={<SearchOutlined />}
            placeholder="搜货品名或 SKU"
            value={keyword}
            onChange={e => setKeyword(e.target.value)}
            style={{ width: 220 }}
          />
          <Space size={6}>
            <Switch checked={hideEmpty} onChange={setHideEmpty} />
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
            <Button danger>清空</Button>
          </Popconfirm>
          <Button onClick={handlePredict}>预测</Button>
          <Button icon={<DownloadOutlined />} onClick={handleDownload}>下载</Button>
          <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={handleSave}>
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

  return (
    <Tabs
      defaultActiveKey="forecast"
      items={[
        { key: 'forecast', label: '销量预测', children: forecastTabContent },
        { key: 'backtest', label: '历史回测', children: <SalesForecastBacktest /> },
      ]}
    />
  );
};

export default SalesForecast;
