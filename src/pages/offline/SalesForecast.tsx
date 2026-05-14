import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Button, Card, DatePicker, Empty, Input, InputNumber, message, Popconfirm, Popover, Space, Spin, Switch, Table, Tabs, Tag, Tooltip } from 'antd';
import { DownloadOutlined, ReloadOutlined, SaveOutlined, SearchOutlined } from '@ant-design/icons';
import dayjs, { Dayjs } from 'dayjs';
import * as XLSX from 'xlsx-js-style';
import { API_BASE } from '../../config';
import Chart from '../../components/Chart';
import ForecastBacktestCard from './ForecastBacktestCard';

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
  // v1.66 起删除算法切换 (algo/effectiveAlgo/effectiveReason), 只保留一个智能算法
  const [forecastSummary, setForecastSummary] = useState<{
    formulaText?: string; alpha?: number; beta?: number; gamma?: number;
    holidayFactor?: number; holidayContext?: string;
  } | null>(null);
  const [regionTrend, setRegionTrend] = useState<Record<string, number>>({});
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
      setForecastSummary(data.forecast_summary || null);
      setRegionTrend(data.region_trend || {});
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
          const trend = regionTrend[region];
          const tooltipTitle = base && base > 0 && suggest ? (
            <div style={{ lineHeight: 1.7 }}>
              <div>近3月均 {base.toFixed(1)} 件</div>
              <div>÷ 近3月季节系数均 {recentAvg.toFixed(2)}</div>
              <div>× {predictMonthLabel}系数 {factor.toFixed(2)}</div>
              {forecastSummary?.holidayFactor && forecastSummary.holidayFactor !== 1 && (
                <div>× 节假日因子 {forecastSummary.holidayFactor.toFixed(2)} ({forecastSummary.holidayContext})</div>
              )}
              {trend && trend !== 1 && (
                <div>× {region}近12月趋势 {trend.toFixed(2)} ({trend > 1 ? '上升' : '下降'})</div>
              )}
              <div style={{ marginTop: 4, paddingTop: 4, borderTop: '1px solid #475569' }}>
                ≈ 建议 <b>{suggest}</b> 件
              </div>
            </div>
          ) : null;
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
      message="销量预测·智能算法说明 (v1.66 起单一算法)"
      description={
        <div style={{ lineHeight: 1.8 }}>
          <div><b>核心公式</b>: 预测 = (α × 近3月均 + β × 同比 + γ × 环比) × 季节系数 × 节假日因子 × 大区12月趋势</div>
          <div style={{marginTop:6}}><b>权重按月份动态调整 (节假日驱动)</b>:</div>
          <ol style={{ marginTop: 4, marginBottom: 8, paddingLeft: 20 }}>
            <li><b>1月 (春节备货)</b> — α=20% / β=70% / γ=10%, 主用同比</li>
            <li><b>2月 (春节假期)</b> — α=10% / β=80% / γ=10%, 主用同比</li>
            <li><b>3-5月 (春节后)</b> — α=60% / β=30% / γ=10%, 主用近3月均</li>
            <li><b>6月 (618)</b> — α=40% / β=50% / γ=10%, 节假日因子 ×1.05</li>
            <li><b>7-8月 (平淡期)</b> — α=70% / β=20% / γ=10%, 主用近3月均</li>
            <li><b>9-10月 (中秋国庆)</b> — α=40% / β=50% / γ=10%, 节假日因子 ×1.05</li>
            <li><b>11月 (双11)</b> — α=30% / β=60% / γ=10%, 节假日因子 ×1.10</li>
            <li><b>12月 (年终)</b> — α=50% / β=40% / γ=10%</li>
          </ol>
          <div><b>4 个数据来源</b>:</div>
          <ol style={{ marginTop: 4, marginBottom: 8, paddingLeft: 20 }}>
            <li><b>近3月均</b> — 上月+前月+大前月销量平均, 短期趋势锚</li>
            <li><b>同比</b> — 去年同月销量, 春节月最稳</li>
            <li><b>环比</b> — 上月销量, 短期趋势校验</li>
            <li><b>节假日因子</b> — 618/双11/中秋国庆 适当上调</li>
          </ol>
          <div><b>大区12月趋势调整</b> — 每个大区拉近12月销量做线性回归,持续上升 ×1.05 / 持续下降 ×0.95 / 平稳 ×1.00.</div>
          <div style={{marginTop:4,color:'#64748b'}}>顶部"本月公式"Tag hover 可看具体权重和趋势调整明细. 鼠标移到表格 cell 也能看每个 SKU × 大区的完整计算链路.</div>
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
          {forecastSummary?.formulaText && (
            <Tooltip title={
              <div style={{ lineHeight: 1.7 }}>
                <div><b>本月预测公式</b></div>
                <div>{forecastSummary.formulaText}</div>
                <div style={{ marginTop: 6, fontSize: 12, color: '#cbd5e1' }}>
                  α (近3月均) = {((forecastSummary.alpha || 0) * 100).toFixed(0)}%<br />
                  β (同比)    = {((forecastSummary.beta || 0) * 100).toFixed(0)}%<br />
                  γ (环比)    = {((forecastSummary.gamma || 0) * 100).toFixed(0)}%<br />
                  节假日因子   = ×{(forecastSummary.holidayFactor || 1).toFixed(2)}
                </div>
                <div style={{ marginTop: 4, fontSize: 12, color: '#cbd5e1' }}>
                  另外按各大区近12月趋势 ×1.05 / ×0.95 / ×1.00 调整
                </div>
              </div>
            }>
              <Tag color="purple" style={{ cursor: 'help' }}>
                本月公式: {forecastSummary.formulaText}
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

  // v1.66.3 算法回测改成独立 Tab (跟销量预测并列)
  return (
    <Tabs
      defaultActiveKey="forecast"
      items={[
        { key: 'forecast', label: '销量预测', children: forecastTabContent },
        { key: 'backtest', label: '算法回测', children: <ForecastBacktestCard /> },
      ]}
    />
  );
};

export default SalesForecast;
