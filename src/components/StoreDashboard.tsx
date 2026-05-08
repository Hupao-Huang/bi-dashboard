import React, { useEffect, useState, useCallback, useMemo, useRef } from 'react';
import { Row, Col, Card, Table, Statistic, Spin, Select, Tabs, Tag, Empty, Popover, Progress } from 'antd';
import ReactECharts from './Chart';
import DateFilter from './DateFilter';
import PageLoading from './PageLoading';
import GoodsChannelExpand from './GoodsChannelExpand';
import StoreVipSection from './StoreVipSection';
import StorePddSection from './StorePddSection';
import StoreSProductsSection from './StoreSProductsSection';
import StoreTmallCsSection from './StoreTmallCsSection';
import StoreJdSection from './StoreJdSection';
import { API_BASE, DATA_END_DATE, DATA_START_DATE } from '../config';
import { getNiceAxisInterval } from '../chartTheme';

interface Props {
  dept: string;
  title: string;
  color: string;
}

// 平台Tab从后端动态获取

// 天猫店铺名(用来判断是否展示运营数据)
const TMALL_SHOPS = ['松鲜鲜调味品旗舰店', '松鲜鲜挚先专卖店', '糙能农场旗舰店'];
const ALL_SHOPS_VALUE = '__all_shops__';

const formatDate = (d: string) => { if (!d) return ''; const parts = d.split('-'); return parts.length >= 3 ? `${parts[1]}-${parts[2].slice(0,2)}` : d; };
const fmtMoney = (v: number) => v >= 10000 ? (v/10000).toFixed(1)+'万' : v.toLocaleString();
const createAbortController = (ref: React.MutableRefObject<AbortController | null>) => {
  ref.current?.abort();
  const controller = new AbortController();
  ref.current = controller;
  return controller;
};
const isAbortError = (error: unknown) => (error as Error)?.name === 'AbortError';
const supportsOpsShop = (shopName: string) => {
  if (shopName.includes('唯品会') || shopName.includes('拼多多') || shopName.includes('京东')) return true;
  return shopName.includes('天猫') && TMALL_SHOPS.some(s => shopName.includes(s));
};

const StoreDashboard: React.FC<Props> = ({ dept, color }) => {
  // 线下部门用"大区"术语（线下没有电商平台，每家店就是一个大区）
  const unit = dept === 'offline' ? '大区' : '店铺';
  const [platformTabs, setPlatformTabs] = useState<{key: string; label: string}[]>([]);
  const [platform, setPlatform] = useState('all');
  const [shopList, setShopList] = useState<any[]>([]);
  const [selectedShop, setSelectedShop] = useState<string>('');
  const [shopDetail, setShopDetail] = useState<any>(null);
  const [tmallOps, setTmallOps] = useState<any>(null);
  const [vipOps, setVipOps] = useState<any>(null);
  const [pddOps, setPddOps] = useState<any>(null);
  const [jdOps, setJdOps] = useState<any>(null);
  const [tmallcsOps, setTmallcsOps] = useState<any>(null);
  const [sProducts, setSProducts] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [startDate, setStartDate] = useState(DATA_START_DATE);
  const [endDate, setEndDate] = useState(DATA_END_DATE);
  const shopListAbortRef = useRef<AbortController | null>(null);
  const shopDetailAbortRef = useRef<AbortController | null>(null);
  const tmallOpsAbortRef = useRef<AbortController | null>(null);
  const vipOpsAbortRef = useRef<AbortController | null>(null);
  const pddOpsAbortRef = useRef<AbortController | null>(null);
  const jdOpsAbortRef = useRef<AbortController | null>(null);
  const tmallcsOpsAbortRef = useRef<AbortController | null>(null);
  const isAllShops = selectedShop === ALL_SHOPS_VALUE;

  // 判断当前店铺是否是天猫店(有运营数据)
  const isTmallShop = useMemo(() => {
    return !isAllShops && selectedShop.includes('天猫') && TMALL_SHOPS.some(s => selectedShop.includes(s));
  }, [isAllShops, selectedShop]);

  // 判断是否是唯品会店铺
  const isVipShop = useMemo(() => !isAllShops && selectedShop.includes('唯品会'), [isAllShops, selectedShop]);

  // 判断是否是拼多多店铺
  const isPddShop = useMemo(() => !isAllShops && selectedShop.includes('拼多多'), [isAllShops, selectedShop]);

  // 判断是否是京东店铺
  const isJdShop = useMemo(() => !isAllShops && selectedShop.includes('京东'), [isAllShops, selectedShop]);

  // 判断是否是天猫超市店铺
  const isTmallcsShop = useMemo(() => !isAllShops && selectedShop.includes('天猫超市'), [isAllShops, selectedShop]);

  // 加载店铺列表
  const fetchShopList = useCallback((s: string, e: string, plat: string) => {
    const controller = createAbortController(shopListAbortRef);
    setLoading(true);
    const platParam = plat ? `&platform=${plat}` : '';
    fetch(`${API_BASE}/api/department?dept=${dept}&start=${s}&end=${e}${platParam}`, { signal: controller.signal })
      .then(res => res.json())
      .then(res => {
        if (controller.signal.aborted) return;
        // 更新平台Tab（从后端动态获取）
        const plats = res.data?.platforms || [];
        if (plats.length > 0 && platformTabs.length === 0) {
          const allTab = { key: 'all', label: '全部' };
          if (dept === 'distribution') {
            // 分销部门只显示"全部"
            setPlatformTabs([allTab]);
          } else {
            setPlatformTabs([allTab, ...plats]);
          }
          // 首次加载默认选"全部"
          if (!plat) {
            setPlatform('all');
            setLoading(false);
            return; // platform变化会触发重新加载
          }
        }

        const shops = res.data?.shops || [];
        setShopList(shops);
        if (shops.length > 0) {
          const found = selectedShop === ALL_SHOPS_VALUE || shops.find((sh: any) => sh.shopName === selectedShop);
          if (!found) setSelectedShop(ALL_SHOPS_VALUE);
        } else if (selectedShop !== ALL_SHOPS_VALUE) {
          setSelectedShop(ALL_SHOPS_VALUE);
        }
        setLoading(false);
      })
      .catch(err => {
        if (isAbortError(err)) return;
        setLoading(false);
      });
  }, [dept, selectedShop, platformTabs.length]);

  // 加载店铺详情
  const fetchShopDetail = useCallback((shopName: string, s: string, e: string, plat: string) => {
    if (!shopName) return;
    const controller = createAbortController(shopDetailAbortRef);
    setDetailLoading(true);
    const platParam = plat !== 'all' ? `&platform=${plat}` : '';
    const shopParam = shopName === ALL_SHOPS_VALUE ? '' : `&shop=${encodeURIComponent(shopName)}`;
    fetch(`${API_BASE}/api/department?dept=${dept}&start=${s}&end=${e}${shopParam}${platParam}`, { signal: controller.signal })
      .then(res => res.json())
      .then(res => {
        if (controller.signal.aborted) return;
        setShopDetail(res.data);
        setDetailLoading(false);
      })
      .catch(err => {
        if (isAbortError(err)) return;
        setDetailLoading(false);
      });
  }, [dept]);

  // 加载天猫运营数据
  const fetchTmallOps = useCallback((shopName: string, s: string, e: string) => {
    const controller = createAbortController(tmallOpsAbortRef);
    // 从店铺名中提取天猫店名(去掉ds-天猫-前缀)
    let tmallName = shopName;
    const m = shopName.match(/ds-天猫[^-]*-(.+)/);
    if (m) tmallName = m[1];
    // 也试试直接匹配
    const matched = TMALL_SHOPS.find(ts => shopName.includes(ts));
    if (matched) tmallName = matched;

    fetch(`${API_BASE}/api/tmall/ops?shop=${encodeURIComponent(tmallName)}&start=${s}&end=${e}`, { signal: controller.signal })
      .then(res => res.json())
      .then(res => {
        if (controller.signal.aborted) return;
        setTmallOps(res.data);
      })
      .catch(err => {
        if (isAbortError(err)) return;
        setTmallOps(null);
      });
  }, []);

  // 加载拼多多运营数据
  const fetchPddOps = useCallback((shopName: string, s: string, e: string) => {
    const controller = createAbortController(pddOpsAbortRef);
    let pddName = shopName;
    const pm = shopName.match(/ds-拼多多-(.+)/);
    if (pm) pddName = pm[1];
    fetch(`${API_BASE}/api/pdd/ops?shop=${encodeURIComponent(pddName)}&start=${s}&end=${e}`, { signal: controller.signal })
      .then(res => res.json())
      .then(res => {
        if (controller.signal.aborted) return;
        setPddOps(res.data);
      })
      .catch(err => {
        if (isAbortError(err)) return;
        setPddOps(null);
      });
  }, []);

  // 加载京东运营数据
  const fetchJdOps = useCallback((shopName: string, s: string, e: string) => {
    const controller = createAbortController(jdOpsAbortRef);
    let jdName = shopName;
    const jm = shopName.match(/ds-京东-(.+)/);
    if (jm) jdName = jm[1];
    fetch(`${API_BASE}/api/jd/ops?shop=${encodeURIComponent(jdName)}&start=${s}&end=${e}`, { signal: controller.signal })
      .then(res => res.json())
      .then(res => {
        if (controller.signal.aborted) return;
        setJdOps(res.data);
      })
      .catch(err => {
        if (isAbortError(err)) return;
        setJdOps(null);
      });
  }, []);

  // 加载S品渠道数据
  const fetchSProducts = useCallback((s: string, e: string, plat: string, shop: string) => {
    const platParam = plat && plat !== 'all' ? `&platform=${plat}` : '';
    const shopParam = shop && shop !== ALL_SHOPS_VALUE ? `&shop=${encodeURIComponent(shop)}` : '';
    fetch(`${API_BASE}/api/s-products?dept=${dept}&start=${s}&end=${e}${platParam}${shopParam}`)
      .then(res => res.json())
      .then(res => setSProducts(res.data))
      .catch(err => { console.warn('StoreDashboard s-products:', err); setSProducts(null); });
  }, [dept]);

  // 加载天猫超市运营数据
  const fetchTmallcsOps = useCallback((s: string, e: string) => {
    const controller = createAbortController(tmallcsOpsAbortRef);
    fetch(`${API_BASE}/api/tmallcs/ops?start=${s}&end=${e}`, { signal: controller.signal })
      .then(res => res.json())
      .then(res => {
        if (controller.signal.aborted) return;
        setTmallcsOps(res.data);
      })
      .catch(err => {
        if (isAbortError(err)) return;
        setTmallcsOps(null);
      });
  }, []);

  // 加载唯品会运营数据
  const fetchVipOps = useCallback((shopName: string, s: string, e: string) => {
    const controller = createAbortController(vipOpsAbortRef);
    // 提取唯品会店名
    let vipName = shopName;
    const vm = shopName.match(/ds-唯品会-(.+)/);
    if (vm) vipName = vm[1];
    fetch(`${API_BASE}/api/vip/ops?shop=${encodeURIComponent(vipName)}&start=${s}&end=${e}`, { signal: controller.signal })
      .then(res => res.json())
      .then(res => {
        if (controller.signal.aborted) return;
        setVipOps(res.data);
      })
      .catch(err => {
        if (isAbortError(err)) return;
        setVipOps(null);
      });
  }, []);

  useEffect(() => { fetchShopList(startDate, endDate, platform); }, [fetchShopList, startDate, endDate, platform]);
  useEffect(() => () => {
    shopListAbortRef.current?.abort();
    shopDetailAbortRef.current?.abort();
    tmallOpsAbortRef.current?.abort();
    vipOpsAbortRef.current?.abort();
    pddOpsAbortRef.current?.abort();
    jdOpsAbortRef.current?.abort();
    tmallcsOpsAbortRef.current?.abort();
  }, []);
  useEffect(() => {
    if (selectedShop) {
      fetchShopDetail(selectedShop, startDate, endDate, platform);
      if (isTmallShop) fetchTmallOps(selectedShop, startDate, endDate);
      else setTmallOps(null);
      if (isVipShop) fetchVipOps(selectedShop, startDate, endDate);
      else setVipOps(null);
      if (isPddShop) fetchPddOps(selectedShop, startDate, endDate);
      else setPddOps(null);
      if (isJdShop) fetchJdOps(selectedShop, startDate, endDate);
      else setJdOps(null);
      if (isTmallcsShop) fetchTmallcsOps(startDate, endDate);
      else setTmallcsOps(null);
      // 电商部门加载S品数据
      if (dept === 'ecommerce') fetchSProducts(startDate, endDate, platform, selectedShop);
    }
  }, [fetchShopDetail, fetchTmallOps, fetchVipOps, fetchPddOps, fetchJdOps, fetchTmallcsOps, fetchSProducts, selectedShop, startDate, endDate, platform, isTmallShop, isVipShop, isPddShop, isJdShop, isTmallcsShop, dept, isAllShops]);

  const currentShop = useMemo(() => {
    if (isAllShops) {
      return shopList.reduce((acc, shop) => ({
        sales: acc.sales + (shop.sales || 0),
        qty: acc.qty + (shop.qty || 0),
      }), { sales: 0, qty: 0 });
    }
    return shopList.find(s => s.shopName === selectedShop) || { sales: 0, qty: 0 };
  }, [isAllShops, selectedShop, shopList]);
  const opsSupportedShops = useMemo(
    () => shopList.filter(shop => supportsOpsShop(shop.shopName)).map(shop => shop.shopName),
    [shopList],
  );
  const shopSelectOptions = useMemo(() => {
    const totalSales = shopList.reduce((sum, shop) => sum + (shop.sales || 0), 0);
    return [
      { value: ALL_SHOPS_VALUE, label: `全部${unit}（${shopList.length}家，¥${totalSales.toLocaleString()}）` },
      ...shopList.map(s => ({
        value: s.shopName,
        label: `${s.shopName}（¥${s.sales?.toLocaleString()}${supportsOpsShop(s.shopName) ? '，含运营数据' : ''}）`,
      })),
    ];
  }, [shopList]);
  const opsHint = useMemo(() => {
    if (!shopList.length) return '';
    // 线下部门没有"运营数据"概念（纯 ERP 销售数据），不显示该提示
    if (dept === 'offline') return '';
    if (!opsSupportedShops.length) return '当前平台暂无运营数据店铺';
    return `运营数据店铺：${opsSupportedShops.length}家`;
  }, [opsSupportedShops, shopList.length, dept]);
  const avgOrderValue = currentShop.qty > 0 ? currentShop.sales / currentShop.qty : 0;
  // 趋势图用扩展数据，商品/品牌用原始数据
  // 后端趋势数据已自动扩展，直接用shopDetail.daily
  const daily = shopDetail?.daily || [];
  // 判断趋势是否被扩展了
  const trendRange = shopDetail?.trendRange;
  const isExpanded = trendRange && trendRange.start !== shopDetail?.dateRange?.start;
  const inSelectedRange = useCallback((d: string) => d >= startDate && d <= endDate, [startDate, endDate]);
  // KPI 卡片辅助信息
  const daysInRange = daily.filter((d: any) => inSelectedRange(d.date)).length || 1;
  const dailyAvgQty = currentShop.qty > 0 ? Math.round(currentShop.qty / daysInRange) : 0;
  const shopRank = !isAllShops && selectedShop ? shopList.findIndex((s: any) => s.shopName === selectedShop) + 1 : 0;
  // 目标数据（仅 offline 有）
  const regionTargets: Record<string, number> = shopDetail?.regionTargets || {};
  const targetAmount = isAllShops
    ? Object.values(regionTargets).reduce((s: number, v: number) => s + v, 0)
    : (regionTargets[selectedShop] || 0);
  const targetPct = targetAmount > 0 ? currentShop.sales / targetAmount * 100 : 0;
  const targetPctColor = targetPct >= 100 ? '#10b981' : targetPct >= 80 ? '#f59e0b' : '#ef4444';
  const avgShopSales = shopList.length > 0 ? currentShop.sales / shopList.length : 0;
  const achievedCount = shopList.filter((s: any) => regionTargets[s.shopName] && s.sales >= regionTargets[s.shopName]).length;
  const goods = shopDetail?.goods || [];
  const brands = shopDetail?.brands || [];
  const trendSplits = 5;
  const trendSalesMax = Math.max(...daily.map((d: any) => d.sales || 0), 1);
  const trendQtyMax = Math.max(...daily.map((d: any) => d.qty || 0), 1);
  const salesInterval = getNiceAxisInterval(trendSalesMax, trendSplits);
  const qtyInterval = getNiceAxisInterval(trendQtyMax, trendSplits);

  // ========== 经营数据图表（选中范围内高亮，范围外淡色） ==========
  const trendOption = {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['销售额', '销量'], top: 0 },
    grid: { left: 80, right: 80, top: 40, bottom: 40 },
    xAxis: { type: 'category' as const, data: daily.map((d: any) => formatDate(d.date)),
      axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: daily.length > 15 ? 45 : 0 } },
    yAxis: [
      { type: 'value' as const, name: '销售额', min: 0, max: salesInterval * trendSplits, interval: salesInterval, axisLabel: { formatter: (v: number) => v >= 10000 ? (v/10000).toFixed(0)+'万' : String(v) } },
      { type: 'value' as const, name: '销量', min: 0, max: qtyInterval * trendSplits, interval: qtyInterval, position: 'right' as const },
    ],
    series: [
      { name: '销售额', type: 'bar', barMaxWidth: 20, barGap: '30%',
        data: daily.map((d: any) => ({ value: d.sales, itemStyle: { color: isExpanded && !inSelectedRange(d.date) ? color + '40' : color } })),
      },
      { name: '销量', type: 'line', yAxisIndex: 1, smooth: true, data: daily.map((d: any) => d.qty), itemStyle: { color: '#ea580c' } },
    ],
  };

  const indexedGoods = goods.map((g: any, i: number) => ({ ...g, _rank: i + 1 }));
  const goodsColumns = [
    { title: '#', dataIndex: '_rank', key: 'rank', width: 40 },
    { title: '编码', dataIndex: 'goodsNo', key: 'goodsNo', width: 110 },
    { title: '商品名称', dataIndex: 'goodsName', key: 'goodsName', ellipsis: true },
    { title: '销售额', dataIndex: 'sales', key: 'sales', width: 120, sorter: (a: any, b: any) => a.sales - b.sales,
      render: (v: number) => `¥${v?.toLocaleString()}` },
    { title: '销量', dataIndex: 'qty', key: 'qty', width: 80, render: (v: number) => v?.toLocaleString() },
    { title: '客单价', key: 'avgPrice', width: 100,
      render: (_: any, r: any) => r.qty > 0 ? `¥${(r.sales/r.qty).toFixed(2)}` : '-' },
  ];

  const brandOption = {
    tooltip: { trigger: 'item' as const, formatter: '{b}: ¥{c} ({d}%)' },
    legend: { bottom: 0, type: 'scroll' as const },
    series: [{
      type: 'pie', radius: ['35%', '65%'],
      label: { show: true, formatter: '{b}\n{d}%', fontSize: 11, lineHeight: 15 },
      data: brands.map((b: any) => ({ value: b.sales, name: b.brand || '未知' })),
    }],
  };

  // ========== 天猫运营数据图表 ==========
  const traffic = useMemo(() => tmallOps?.traffic || [], [tmallOps]);
  const campaigns = useMemo(() => tmallOps?.campaigns || [], [tmallOps]);
  const cps = useMemo(() => tmallOps?.cps || [], [tmallOps]);
  const members = useMemo(() => tmallOps?.members || [], [tmallOps]);
  const selectedTraffic = useMemo(() => traffic.filter((t: any) => inSelectedRange(t.date)), [traffic, inSelectedRange]);
  const selectedCampaigns = useMemo(() => campaigns.filter((c: any) => inSelectedRange(c.date)), [campaigns, inSelectedRange]);
  const selectedCps = useMemo(() => cps.filter((c: any) => inSelectedRange(c.date)), [cps, inSelectedRange]);
  const selectedMembers = useMemo(() => members.filter((m: any) => inSelectedRange(m.date)), [members, inSelectedRange]);
  const tmallGoodsTop = useMemo(() => tmallOps?.goodsTop || [], [tmallOps]);
  const tmallBrandDaily = useMemo(() => tmallOps?.brandDaily || [], [tmallOps]);
  const tmallCrowdDaily = useMemo(() => tmallOps?.crowdDaily || [], [tmallOps]);
  const tmallIndustry = useMemo(() => tmallOps?.industry || [], [tmallOps]);
  const tmallRepurchase = useMemo(() => tmallOps?.repurchase || [], [tmallOps]);

  // 流量转化汇总
  const trafficSummary = useMemo(() => {
    if (!selectedTraffic.length) return { visitors: 0, payBuyers: 0, payAmount: 0, convRate: 0, uvValue: 0, cartBuyers: 0 };
    const sum = selectedTraffic.reduce((acc: any, t: any) => ({
      visitors: acc.visitors + t.visitors,
      payBuyers: acc.payBuyers + t.payBuyers,
      payAmount: acc.payAmount + t.payAmount,
      cartBuyers: acc.cartBuyers + t.cartBuyers,
    }), { visitors: 0, payBuyers: 0, payAmount: 0, cartBuyers: 0 });
    return {
      ...sum,
      convRate: sum.visitors > 0 ? (sum.payBuyers / sum.visitors * 100) : 0,
      uvValue: sum.visitors > 0 ? sum.payAmount / sum.visitors : 0,
    };
  }, [selectedTraffic]);

  // 流量转化趋势
  const trafficOption = {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['访客数', '支付买家数', '转化率'], top: 0 },
    grid: { left: 80, right: 80, top: 40, bottom: 40 },
    xAxis: { type: 'category' as const, data: traffic.map((t: any) => formatDate(t.date)),
      axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: traffic.length > 15 ? 45 : 0 } },
    yAxis: [
      { type: 'value' as const, name: '人数', min: 0 },
      { type: 'value' as const, name: '转化率%', min: 0, position: 'right' as const, axisLabel: { formatter: '{value}%' } },
    ],
    series: [
      { name: '访客数', type: 'bar', barWidth: 8,
        data: traffic.map((t: any) => ({ value: t.visitors, itemStyle: { color: isExpanded && !inSelectedRange(t.date) ? 'rgba(24,144,255,0.25)' : '#1e40af' } })) },
      { name: '支付买家数', type: 'bar', barWidth: 8,
        data: traffic.map((t: any) => ({ value: t.payBuyers, itemStyle: { color: isExpanded && !inSelectedRange(t.date) ? 'rgba(82,196,26,0.25)' : '#10b981' } })) },
      { name: '转化率', type: 'line', yAxisIndex: 1, smooth: true, data: traffic.map((t: any) => (t.payConvRate * 100).toFixed(2)),
        itemStyle: { color: '#faad14' }, lineStyle: { width: 2 } },
    ],
  };

  // 推广花费汇总
  const adSummary = useMemo(() => {
    const cpcTotal = selectedCampaigns.reduce((s: number, c: any) => s + c.cost, 0);
    const cpcPay = selectedCampaigns.reduce((s: number, c: any) => s + c.payAmount, 0);
    const cpsCommission = selectedCps.reduce((s: number, c: any) => s + c.payCommission, 0);
    const cpsPay = selectedCps.reduce((s: number, c: any) => s + c.payAmount, 0);
    return {
      cpcCost: cpcTotal,
      cpcPayAmount: cpcPay,
      cpcROI: cpcTotal > 0 ? cpcPay / cpcTotal : 0,
      cpsCommission,
      cpsPayAmount: cpsPay,
      totalCost: cpcTotal + cpsCommission,
    };
  }, [selectedCampaigns, selectedCps]);

  // 会员趋势
  const memberOption = {
    tooltip: { trigger: 'axis' as const },
    legend: { data: ['会员支付额', '支付会员数', '复购率'], top: 0 },
    grid: { left: 80, right: 80, top: 40, bottom: 40 },
    xAxis: { type: 'category' as const, data: members.map((m: any) => formatDate(m.date)),
      axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: members.length > 15 ? 45 : 0 } },
    yAxis: [
      { type: 'value' as const, name: '金额/人数', min: 0, axisLabel: { formatter: (v: number) => fmtMoney(v) } },
      { type: 'value' as const, name: '复购率%', min: 0, position: 'right' as const, axisLabel: { formatter: '{value}%' } },
    ],
    series: [
      { name: '会员支付额', type: 'bar', barWidth: 8,
        data: members.map((m: any) => ({ value: m.memberPayAmt, itemStyle: { color: isExpanded && !inSelectedRange(m.date) ? 'rgba(114,46,209,0.25)' : '#7c3aed' } })) },
      { name: '支付会员数', type: 'line', data: members.map((m: any) => m.paidMemberCnt), itemStyle: { color: '#1e40af' } },
      { name: '复购率', type: 'line', yAxisIndex: 1, smooth: true, data: members.map((m: any) => (m.repurchaseRate * 100).toFixed(2)),
        itemStyle: { color: '#faad14' }, lineStyle: { type: 'dashed' as const } },
    ],
  };

  // 会员汇总
  const memberSummary = useMemo(() => {
    if (!selectedMembers.length) return { totalMember: 0, payAmt: 0, paidCnt: 0, avgRepurchase: 0 };
    const last = selectedMembers[selectedMembers.length - 1];
    return {
      totalMember: last.totalMemberCnt,
      payAmt: selectedMembers.reduce((s: number, m: any) => s + m.memberPayAmt, 0),
      paidCnt: selectedMembers.reduce((s: number, m: any) => s + m.paidMemberCnt, 0),
      avgRepurchase: selectedMembers.reduce((s: number, m: any) => s + m.repurchaseRate, 0) / selectedMembers.length * 100,
    };
  }, [selectedMembers]);

  if (loading) return <PageLoading />;

  return (
    <div>
      <DateFilter start={startDate} end={endDate} onChange={(s, e) => { setStartDate(s); setEndDate(e); }} />

      {/* 平台Tab + 店铺选择器 */}
      <Card className="bi-filter-card" style={{ marginBottom: 16 }}>
        <Tabs
          activeKey={platform}
          onChange={(key) => { setPlatform(key); setSelectedShop(ALL_SHOPS_VALUE); }}
          items={platformTabs.map(t => ({ key: t.key, label: t.label }))}
          style={{ marginBottom: 12 }}
        />
        <Row align="middle" gutter={16}>
          <Col>
            <span style={{ fontWeight: 500, marginRight: 8 }}>选择{unit}：</span>
            <Select
              value={selectedShop}
              onChange={setSelectedShop}
              style={{ width: 400 }}
              showSearch
              filterOption={(input, option) => (option?.label as string || '').toLowerCase().includes(input.toLowerCase())}
              options={shopSelectOptions}
            />
          </Col>
          <Col>
            {isAllShops && <Tag color="blue">{dept === 'offline' ? '部门汇总' : '平台汇总'}</Tag>}
            {isTmallShop && <Tag color="orange">天猫运营数据</Tag>}
            {isVipShop && <Tag color="purple">唯品会运营数据</Tag>}
            {isPddShop && <Tag color="red">拼多多运营数据</Tag>}
            {isJdShop && <Tag color="red">京东运营数据</Tag>}
            {isTmallcsShop && <Tag color="cyan">天猫超市运营数据</Tag>}
            {isAllShops && dept !== 'offline' && <span style={{ color: '#8c8c8c', fontSize: 12 }}>运营数据仅支持单店查看</span>}
          </Col>
        </Row>
        {opsHint && (
          <div style={{ marginTop: 12, color: '#8c8c8c', fontSize: 12 }}>
            {opsSupportedShops.length > 0 ? (
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                <span>{opsHint}</span>
                <Popover
                  trigger="hover"
                  placement="bottomLeft"
                  content={(
                    <div style={{ maxWidth: 360 }}>
                      <div style={{ marginBottom: 8, color: '#595959', fontWeight: 500 }}>支持运营数据的店铺</div>
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                        {opsSupportedShops.map(shop => (
                          <Tag key={shop} color="blue" style={{ marginInlineEnd: 0 }}>
                            {shop}
                          </Tag>
                        ))}
                      </div>
                    </div>
                  )}
                >
                  <Tag color="blue" style={{ marginInlineEnd: 0, cursor: 'pointer' }}>查看名单</Tag>
                </Popover>
              </div>
            ) : (
              opsHint
            )}
          </div>
        )}
      </Card>

      {detailLoading ? <Spin style={{ display: 'block', margin: '40px auto' }} /> : (
        <>
          {isExpanded && <div style={{ color: '#999', fontSize: 12, marginBottom: 8 }}>深色柱为选中日期，浅色柱为趋势参考</div>}
          {/* ========== 第一块：经营数据（所有平台） ========== */}
          <Card title={`经营数据${isExpanded ? '（蓝色区域为选中日期）' : ''}`} style={{ marginBottom: 16 }}
            styles={{ header: { background: 'linear-gradient(90deg, #f0f5ff 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
            <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
              {[
                { title: '销售额', value: currentShop.sales, precision: 2, prefix: '¥', accentColor: color,
                  hint: currentShop.sales >= 10000 ? `≈ ${(currentShop.sales / 10000).toFixed(1)}万` : '' },
                { title: '货品数', value: currentShop.qty, accentColor: '#10b981' },
                { title: '客单价', value: avgOrderValue, precision: 2, prefix: '¥', accentColor: '#1e40af',
                  hint: avgOrderValue >= 10000 ? `≈ ${(avgOrderValue / 10000).toFixed(1)}万` : '' },
                { title: `${unit}数量`, value: shopList.length, suffix: '家', accentColor: '#7c3aed',
                  hintNode: shopRank > 0 ? <span style={{ color: shopRank <= 3 ? '#10b981' : '#64748b', fontWeight: shopRank <= 3 ? 600 : 400 }}>排名 #{shopRank}/{shopList.length}</span> : null },
              ].map((card: any, idx: number) => (
                <Col xs={12} sm={6} key={card.title}>
                  <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor, height: '100%' }}>
                    <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
                    <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>{card.hintNode || card.hint || ' '}</div>
                    {idx === 0 && targetAmount > 0 && (
                      <div style={{ marginTop: 8, borderTop: '1px solid #f1f5f9', paddingTop: 8 }}>
                        <Progress
                          percent={parseFloat(Math.min(targetPct, 999).toFixed(1))}
                          strokeColor={targetPctColor}
                          trailColor="#f1f5f9"
                          format={p => <span style={{ color: targetPctColor, fontWeight: 700, fontSize: 13 }}>{p}%</span>}
                          size="small"
                        />
                        <div style={{ fontSize: 11, color: '#94a3b8', marginTop: 2 }}>
                          目标 ￥{(targetAmount / 10000).toFixed(1)}万
                        </div>
                      </div>
                    )}
                    {idx === 1 && targetAmount > 0 && (
                      <div style={{ marginTop: 8, borderTop: '1px solid #f1f5f9', paddingTop: 8 }}>
                        <div style={{ fontSize: 15, fontWeight: 600, color: '#334155' }}>
                          {dailyAvgQty.toLocaleString()} <span style={{ fontSize: 11, color: '#94a3b8', fontWeight: 400 }}>件/日</span>
                        </div>
                        <div style={{ fontSize: 11, color: '#94a3b8', marginTop: 2 }}>{daysInRange} 天平均</div>
                      </div>
                    )}
                    {idx === 2 && targetAmount > 0 && (
                      <div style={{ marginTop: 8, borderTop: '1px solid #f1f5f9', paddingTop: 8 }}>
                        <div style={{ fontSize: 15, fontWeight: 600, color: '#334155' }}>
                          ￥{(avgShopSales / 10000).toFixed(1)}万 <span style={{ fontSize: 11, color: '#94a3b8', fontWeight: 400 }}>店均</span>
                        </div>
                        <div style={{ fontSize: 11, color: '#94a3b8', marginTop: 2 }}>单店平均销售额</div>
                      </div>
                    )}
                    {idx === 3 && targetAmount > 0 && (
                      <div style={{ marginTop: 8, borderTop: '1px solid #f1f5f9', paddingTop: 8 }}>
                        <div style={{ fontSize: 15, fontWeight: 600, color: achievedCount > 0 ? '#10b981' : '#94a3b8' }}>
                          {achievedCount} / {shopList.length} <span style={{ fontSize: 11, color: '#94a3b8', fontWeight: 400 }}>达标</span>
                        </div>
                        <div style={{ fontSize: 11, color: '#94a3b8', marginTop: 2 }}>完成率 ≥100% 的大区数</div>
                      </div>
                    )}
                  </Card>
                </Col>
              ))}
            </Row>

            <ReactECharts lazyUpdate={true} option={trendOption} style={{ height: 300 }} />

            <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
              <Col xs={24} lg={14}>
                <Card className="bi-table-card" title="商品销售排行" size="small" type="inner">
                  <Table dataSource={indexedGoods} columns={goodsColumns} rowKey="goodsNo" pagination={{ pageSize: 10 }} size="small"
                    summary={(pageData) => {
                      const totalSales = pageData.reduce((s, r: any) => s + (r.sales || 0), 0);
                      const totalQty = pageData.reduce((s, r: any) => s + (r.qty || 0), 0);
                      return (
                        <Table.Summary fixed>
                          <Table.Summary.Row style={{ fontWeight: 600, background: '#fafafa' }}>
                            <Table.Summary.Cell index={0} />
                            <Table.Summary.Cell index={1} />
                            <Table.Summary.Cell index={2}>合计</Table.Summary.Cell>
                            <Table.Summary.Cell index={3}>¥{totalSales.toLocaleString()}</Table.Summary.Cell>
                            <Table.Summary.Cell index={4}>{totalQty.toLocaleString()}</Table.Summary.Cell>
                            <Table.Summary.Cell index={5} />
                          </Table.Summary.Row>
                        </Table.Summary>
                      );
                    }}
                  />
                </Card>
              </Col>
              <Col xs={24} lg={10}>
                <Card title="品牌销售占比" size="small" type="inner">
                  <ReactECharts lazyUpdate={true} option={brandOption} style={{ height: 350 }} />
                </Card>
              </Col>
            </Row>
          </Card>

          {/* ========== 第二块：流量转化（天猫独有） ========== */}
          {isTmallShop && (
            <Card title="流量转化" style={{ marginBottom: 16 }}
              styles={{ header: { background: 'linear-gradient(90deg, #fff7e6 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
              {traffic.length > 0 ? (
                <>
                  <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
                    {[
                      { title: '访客数', value: trafficSummary.visitors, accentColor: '#1e40af' },
                      { title: '支付买家数', value: trafficSummary.payBuyers, accentColor: '#10b981' },
                      { title: '支付金额', value: trafficSummary.payAmount, precision: 2, prefix: '¥', accentColor: '#ef4444' },
                      { title: '加购人数', value: trafficSummary.cartBuyers, accentColor: '#06b6d4' },
                      { title: '支付转化率', value: trafficSummary.convRate, precision: 2, suffix: '%', accentColor: '#f59e0b' },
                      { title: 'UV价值', value: trafficSummary.uvValue, precision: 2, prefix: '¥', accentColor: '#7c3aed' },
                    ].map((card) => (
                      <Col xs={12} sm={4} key={card.title}>
                        <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                          <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
                    <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>{card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}</div>
                        </Card>
                      </Col>
                    ))}
                  </Row>
                  <ReactECharts lazyUpdate={true} option={trafficOption} style={{ height: 300 }} />
                </>
              ) : <Empty description="暂无流量数据" />}
            </Card>
          )}

          {/* ========== 第三块：推广摘要（天猫独有，详情见营销费用页面） ========== */}
          {isTmallShop && (
            <Card title="推广摘要" extra={<span style={{ color: '#999', fontSize: 12 }}>详细数据请查看「营销费用」页面</span>}
              style={{ marginBottom: 16 }}
              styles={{ header: { background: 'linear-gradient(90deg, #fff1f0 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
              {(campaigns.length > 0 || cps.length > 0) ? (
                <Row gutter={[16, 16]}>
                  {[
                    { title: 'CPC花费', value: adSummary.cpcCost, precision: 2, prefix: '¥', accentColor: '#ef4444' },
                    { title: 'CPC成交额', value: adSummary.cpcPayAmount, precision: 2, prefix: '¥', accentColor: '#10b981' },
                    { title: 'CPC ROI', value: adSummary.cpcROI, precision: 2, accentColor: '#1e40af' },
                    { title: 'CPS佣金', value: adSummary.cpsCommission, precision: 2, prefix: '¥', accentColor: '#f59e0b' },
                    { title: 'CPS成交额', value: adSummary.cpsPayAmount, precision: 2, prefix: '¥', accentColor: '#06b6d4' },
                    { title: '推广总花费', value: adSummary.totalCost, precision: 2, prefix: '¥', accentColor: '#dc2626' },
                  ].map((card) => (
                    <Col xs={12} sm={4} key={card.title}>
                      <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                        <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} />
                    <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>{card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}</div>
                      </Card>
                    </Col>
                  ))}
                </Row>
              ) : <Empty description="暂无推广数据" />}
            </Card>
          )}

          {/* ========== 第四块：会员复购（天猫独有） ========== */}
          {isTmallShop && (
            <Card title="会员复购" style={{ marginBottom: 16 }}
              styles={{ header: { background: 'linear-gradient(90deg, #f9f0ff 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
              {members.length > 0 ? (
                <>
                  <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
                    {[
                      { title: '会员总数', value: memberSummary.totalMember, accentColor: '#7c3aed' },
                      { title: '会员支付额', value: memberSummary.payAmt, precision: 2, prefix: '¥', accentColor: '#1e40af' },
                      { title: '支付会员数', value: memberSummary.paidCnt, accentColor: '#10b981' },
                      { title: '平均复购率', value: memberSummary.avgRepurchase, precision: 2, suffix: '%', accentColor: '#f59e0b' },
                    ].map((card) => (
                      <Col xs={12} sm={6} key={card.title}>
                        <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                          <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
                    <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>{card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}</div>
                        </Card>
                      </Col>
                    ))}
                  </Row>
                  <ReactECharts lazyUpdate={true} option={memberOption} style={{ height: 300 }} />
                </>
              ) : <Empty description="暂无会员数据" />}
            </Card>
          )}

          {/* ========== 天猫商品TOP10 ========== */}
          {isTmallShop && tmallGoodsTop.length > 0 && (
            <Card className="bi-table-card" title="商品销售TOP10（生意参谋）" style={{ marginBottom: 16 }}
              styles={{ header: { background: 'linear-gradient(90deg, #fef3c7 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
              <Table dataSource={tmallGoodsTop} rowKey="productName" size="small" pagination={false} scroll={{ x: 700 }}
                columns={[
                  { title: '商品', dataIndex: 'productName', key: 'name', ellipsis: true, width: 200 },
                  { title: '访客', dataIndex: 'visitors', key: 'visitors', sorter: (a: any, b: any) => a.visitors - b.visitors },
                  { title: '加购人数', dataIndex: 'cartBuyers', key: 'cart', sorter: (a: any, b: any) => a.cartBuyers - b.cartBuyers },
                  { title: '支付件数', dataIndex: 'payQty', key: 'payQty' },
                  { title: '支付金额', dataIndex: 'payAmount', key: 'payAmt', render: (v: number) => `¥${v?.toLocaleString()}`, sorter: (a: any, b: any) => a.payAmount - b.payAmount },
                  { title: '转化率', dataIndex: 'payConvRate', key: 'conv' },
                  { title: '退款金额', dataIndex: 'refundAmount', key: 'refund', render: (v: number) => v > 0 ? `¥${v?.toLocaleString()}` : '-' },
                ]}
              />
            </Card>
          )}

          {/* ========== 天猫品牌数据（数据银行） ========== */}
          {isTmallShop && tmallBrandDaily.length > 0 && (
            <Card title="品牌数据（数据银行）" style={{ marginBottom: 16 }}
              styles={{ header: { background: 'linear-gradient(90deg, #dbeafe 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
              <ReactECharts lazyUpdate={true} style={{ height: 300 }} option={{
                tooltip: { trigger: 'axis' },
                legend: { data: ['会员成交额', '客户量', '忠诚量', '兴趣量'], top: 0 },
                grid: { left: 60, right: 60, top: 40, bottom: 40 },
                xAxis: { type: 'category', data: tmallBrandDaily.map((d: any) => d.date?.slice(5)) },
                yAxis: [
                  { type: 'value', name: '金额' },
                  { type: 'value', name: '人数', position: 'right' },
                ],
                series: [
                  { name: '会员成交额', type: 'bar', data: tmallBrandDaily.map((d: any) => d.memberPayAmt), itemStyle: { color: '#1e40af' } },
                  { name: '客户量', type: 'line', yAxisIndex: 1, data: tmallBrandDaily.map((d: any) => d.customerVolume), itemStyle: { color: '#10b981' } },
                  { name: '忠诚量', type: 'line', yAxisIndex: 1, data: tmallBrandDaily.map((d: any) => d.loyalVolume), itemStyle: { color: '#f59e0b' } },
                  { name: '兴趣量', type: 'line', yAxisIndex: 1, data: tmallBrandDaily.map((d: any) => d.interestVolume), itemStyle: { color: '#ef4444' } },
                ],
              }} />
            </Card>
          )}

          {/* ========== 天猫人群覆盖（达摩盘） ========== */}
          {isTmallShop && tmallCrowdDaily.length > 0 && (
            <Card title="人群覆盖（达摩盘）" style={{ marginBottom: 16 }}
              styles={{ header: { background: 'linear-gradient(90deg, #fce7f3 0%, #fff 100%)', fontWeight: 600, fontSize: 16 } }}>
              <ReactECharts lazyUpdate={true} style={{ height: 300 }} option={{
                tooltip: { trigger: 'axis' },
                legend: { data: ['覆盖人数', 'TA浓度', '成交额'], top: 0 },
                grid: { left: 60, right: 60, top: 40, bottom: 40 },
                xAxis: { type: 'category', data: tmallCrowdDaily.map((d: any) => d.date?.slice(5)) },
                yAxis: [
                  { type: 'value', name: '人数' },
                  { type: 'value', name: '金额', position: 'right' },
                ],
                series: [
                  { name: '覆盖人数', type: 'bar', data: tmallCrowdDaily.map((d: any) => d.coverage), itemStyle: { color: '#7c3aed' } },
                  { name: 'TA浓度', type: 'line', data: tmallCrowdDaily.map((d: any) => d.concentrate), itemStyle: { color: '#be123c' } },
                  { name: '成交额', type: 'line', yAxisIndex: 1, data: tmallCrowdDaily.map((d: any) => d.payAmount), itemStyle: { color: '#10b981' } },
                ],
              }} />
            </Card>
          )}

          {/* ========== 天猫行业月报+复购月报（集客） ========== */}
          {isTmallShop && (tmallIndustry.length > 0 || tmallRepurchase.length > 0) && (
            <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
              {tmallIndustry.length > 0 && (
                <Col xs={24} lg={12}>
                  <Card title="行业月报（集客）" styles={{ header: { fontWeight: 600, fontSize: 15 } }}>
                    <Table dataSource={tmallIndustry} rowKey={(r: any) => `${r.month}-${r.category}`} size="small" pagination={false}
                      columns={[
                        { title: '月份', dataIndex: 'month', key: 'month', width: 80 },
                        { title: '品类', dataIndex: 'category', key: 'cat', ellipsis: true },
                        { title: '新客占比', dataIndex: 'newRatio', key: 'nr', render: (v: number) => v ? `${v}%` : '-' },
                        { title: '新客销售占比', dataIndex: 'newSalesRatio', key: 'nsr', render: (v: number) => v ? `${v}%` : '-' },
                        { title: '新客30天复购', dataIndex: 'newRepurchase30d', key: 'nr30', render: (v: number) => v ? `${v}%` : '-' },
                        { title: '客单价', dataIndex: 'unitPrice', key: 'up', render: (v: number) => v ? `¥${v}` : '-' },
                      ]}
                    />
                  </Card>
                </Col>
              )}
              {tmallRepurchase.length > 0 && (
                <Col xs={24} lg={12}>
                  <Card title="复购分析（集客）" styles={{ header: { fontWeight: 600, fontSize: 15 } }}>
                    <Table dataSource={tmallRepurchase} rowKey={(r: any) => `${r.month}-${r.category}`} size="small" pagination={false}
                      columns={[
                        { title: '月份', dataIndex: 'month', key: 'month', width: 80 },
                        { title: '品类', dataIndex: 'category', key: 'cat', ellipsis: true },
                        { title: '30天复购', dataIndex: 'shopRepurchase30d', key: 'r30', render: (v: number) => v ? `${v}%` : '-' },
                        { title: '180天复购', dataIndex: 'shopRepurchase180d', key: 'r180', render: (v: number) => v ? `${v}%` : '-' },
                        { title: '360天复购', dataIndex: 'shopRepurchase360d', key: 'r360', render: (v: number) => v ? `${v}%` : '-' },
                        { title: '流失复购率', dataIndex: 'lostRepurchaseRate', key: 'lost', render: (v: number) => v ? `${v}%` : '-' },
                      ]}
                    />
                  </Card>
                </Col>
              )}
            </Row>
          )}

          {/* ========== 唯品会运营数据 ========== */}
          {isVipShop && <StoreVipSection vipOps={vipOps} inSelectedRange={inSelectedRange} isExpanded={isExpanded} />}

          {/* ========== 京东运营数据 (店铺经营+客户分析+新老客+行业热词+促销) ========== */}
          {isJdShop && <StoreJdSection jdOps={jdOps} inSelectedRange={inSelectedRange} isExpanded={isExpanded} />}

          {/* ========== 拼多多运营数据 ========== */}
          {isPddShop && <StorePddSection pddOps={pddOps} inSelectedRange={inSelectedRange} isExpanded={isExpanded} />}

          {/* ========== S品渠道销售分析（电商部门） ========== */}
          <StoreSProductsSection sProducts={sProducts} dept={dept} platform={platform} isAllShops={isAllShops} inSelectedRange={inSelectedRange} isExpanded={isExpanded} />

          {/* ========== 天猫超市运营数据 ========== */}
          {isTmallcsShop && <StoreTmallCsSection tmallcsOps={tmallcsOps} inSelectedRange={inSelectedRange} isExpanded={isExpanded} />}
        </>
      )}
    </div>
  );
};

export default StoreDashboard;

