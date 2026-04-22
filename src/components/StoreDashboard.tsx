import React, { useEffect, useState, useCallback, useMemo, useRef } from 'react';
import { Row, Col, Card, Table, Statistic, Spin, Select, Tabs, Tag, Empty, Popover } from 'antd';
import ReactECharts from './Chart';
import DateFilter from './DateFilter';
import PageLoading from './PageLoading';
import GoodsChannelExpand from './GoodsChannelExpand';
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
      { value: ALL_SHOPS_VALUE, label: `全部店铺（${shopList.length}家，¥${totalSales.toLocaleString()}）` },
      ...shopList.map(s => ({
        value: s.shopName,
        label: `${s.shopName}（¥${s.sales?.toLocaleString()}${supportsOpsShop(s.shopName) ? '，含运营数据' : ''}）`,
      })),
    ];
  }, [shopList]);
  const opsHint = useMemo(() => {
    if (!shopList.length) return '';
    if (!opsSupportedShops.length) return '当前平台暂无运营数据店铺';
    return `运营数据店铺：${opsSupportedShops.length}家`;
  }, [opsSupportedShops, shopList.length]);
  const avgOrderValue = currentShop.qty > 0 ? currentShop.sales / currentShop.qty : 0;
  // 趋势图用扩展数据，商品/品牌用原始数据
  // 后端趋势数据已自动扩展，直接用shopDetail.daily
  const daily = shopDetail?.daily || [];
  // 判断趋势是否被扩展了
  const trendRange = shopDetail?.trendRange;
  const isExpanded = trendRange && trendRange.start !== shopDetail?.dateRange?.start;
  const inSelectedRange = useCallback((d: string) => d >= startDate && d <= endDate, [startDate, endDate]);
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
            <span style={{ fontWeight: 500, marginRight: 8 }}>选择店铺：</span>
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
            {isAllShops && <Tag color="blue">平台汇总</Tag>}
            {isTmallShop && <Tag color="orange">天猫运营数据</Tag>}
            {isVipShop && <Tag color="purple">唯品会运营数据</Tag>}
            {isPddShop && <Tag color="red">拼多多运营数据</Tag>}
            {isJdShop && <Tag color="red">京东运营数据</Tag>}
            {isTmallcsShop && <Tag color="cyan">天猫超市运营数据</Tag>}
            {isAllShops && <span style={{ color: '#8c8c8c', fontSize: 12 }}>运营数据仅支持单店查看</span>}
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
            headStyle={{ background: 'linear-gradient(90deg, #f0f5ff 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
            <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
              {[
                { title: '销售额', value: currentShop.sales, precision: 2, prefix: '¥', accentColor: color },
                { title: '货品数', value: currentShop.qty, accentColor: '#10b981' },
                { title: '客单价', value: avgOrderValue, precision: 2, prefix: '¥', accentColor: '#1e40af' },
                { title: '店铺数量', value: shopList.length, suffix: '家', accentColor: '#7c3aed' },
              ].map((card) => (
                <Col xs={12} sm={6} key={card.title}>
                  <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                    <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
                    <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>{card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}</div>
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
              headStyle={{ background: 'linear-gradient(90deg, #fff7e6 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
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
              headStyle={{ background: 'linear-gradient(90deg, #fff1f0 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
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
              headStyle={{ background: 'linear-gradient(90deg, #f9f0ff 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
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
              headStyle={{ background: 'linear-gradient(90deg, #fef3c7 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
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
              headStyle={{ background: 'linear-gradient(90deg, #dbeafe 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
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
              headStyle={{ background: 'linear-gradient(90deg, #fce7f3 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
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
                  <Card title="行业月报（集客）" headStyle={{ fontWeight: 600, fontSize: 15 }}>
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
                  <Card title="复购分析（集客）" headStyle={{ fontWeight: 600, fontSize: 15 }}>
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
          {isVipShop && (() => {
            const vipDaily = vipOps?.daily || [];
            if (!vipDaily.length) return null;

            const vipSummary = {
              impressions: vipDaily.filter((d: any) => inSelectedRange(d.date)).reduce((s: number, d: any) => s + d.impressions, 0),
              detailUv: vipDaily.filter((d: any) => inSelectedRange(d.date)).reduce((s: number, d: any) => s + d.detailUv, 0),
              cartBuyers: vipDaily.filter((d: any) => inSelectedRange(d.date)).reduce((s: number, d: any) => s + d.cartBuyers, 0),
              payAmount: vipDaily.filter((d: any) => inSelectedRange(d.date)).reduce((s: number, d: any) => s + d.payAmount, 0),
              payCount: vipDaily.filter((d: any) => inSelectedRange(d.date)).reduce((s: number, d: any) => s + d.payCount, 0),
              visitors: vipDaily.filter((d: any) => inSelectedRange(d.date)).reduce((s: number, d: any) => s + d.visitors, 0),
            };
            const avgUvValue = vipSummary.detailUv > 0 ? vipSummary.payAmount / vipSummary.detailUv : 0;

            const vipTrafficOption = {
              tooltip: { trigger: 'axis' as const },
              legend: { data: ['曝光流量', '商详UV', '加购人数'], top: 0 },
              grid: { left: 80, right: 80, top: 40, bottom: 40 },
              xAxis: { type: 'category' as const, data: vipDaily.map((d: any) => formatDate(d.date)),
                axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: vipDaily.length > 15 ? 45 : 0 } },
              yAxis: [{ type: 'value' as const, name: '人数', min: 0 }],
              series: [
                { name: '曝光流量', type: 'bar', barWidth: 8,
                  data: vipDaily.map((d: any) => ({ value: d.impressions, itemStyle: { color: isExpanded && !inSelectedRange(d.date) ? 'rgba(114,46,209,0.25)' : '#7c3aed' } })) },
                { name: '商详UV', type: 'line', data: vipDaily.map((d: any) => d.detailUv), itemStyle: { color: '#1e40af' } },
                { name: '加购人数', type: 'line', data: vipDaily.map((d: any) => d.cartBuyers), itemStyle: { color: '#10b981' } },
              ],
            };

            return (
              <Card title="流量转化（唯品会）" style={{ marginBottom: 16 }}
                headStyle={{ background: 'linear-gradient(90deg, #f9f0ff 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
                <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
                  {[
                    { title: '曝光流量', value: vipSummary.impressions, accentColor: '#7c3aed' },
                    { title: '商详UV', value: vipSummary.detailUv, accentColor: '#1e40af' },
                    { title: '加购人数', value: vipSummary.cartBuyers, accentColor: '#10b981' },
                    { title: '销售额', value: vipSummary.payAmount, precision: 2, prefix: '¥', accentColor: '#ef4444' },
                    { title: '客户数', value: vipSummary.visitors, accentColor: '#06b6d4' },
                    { title: 'UV价值', value: avgUvValue, precision: 2, prefix: '¥', accentColor: '#f59e0b' },
                  ].map((card) => (
                    <Col xs={12} sm={4} key={card.title}>
                      <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                        <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} />
                    <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>{card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}</div>
                      </Card>
                    </Col>
                  ))}
                </Row>
                <ReactECharts lazyUpdate={true} option={vipTrafficOption} style={{ height: 300 }} />
              </Card>
            );
          })()}

          {/* ========== 京东运营数据 ========== */}
          {isJdShop && (() => {
            const jdShop = jdOps?.shop || [];
            const jdCustomer = jdOps?.customer || [];
            const hasData = jdShop.length > 0 || jdCustomer.length > 0;
            if (!hasData) return null;

            // 店铺经营汇总
            const shopInRange = jdShop.filter((d: any) => inSelectedRange(d.date));
            const shopSum = {
              visitors: shopInRange.reduce((s: number, d: any) => s + d.visitors, 0),
              pageViews: shopInRange.reduce((s: number, d: any) => s + d.pageViews, 0),
              payCustomers: shopInRange.reduce((s: number, d: any) => s + d.payCustomers, 0),
              payAmount: shopInRange.reduce((s: number, d: any) => s + d.payAmount, 0),
              payCount: shopInRange.reduce((s: number, d: any) => s + d.payCount, 0),
              avgConvRate: shopInRange.length > 0 ? shopInRange.reduce((s: number, d: any) => s + d.convRate, 0) / shopInRange.length : 0,
              avgUnitPrice: shopInRange.length > 0 ? shopInRange.reduce((s: number, d: any) => s + d.unitPrice, 0) / shopInRange.length : 0,
              avgUvValue: shopInRange.length > 0 ? shopInRange.reduce((s: number, d: any) => s + d.uvValue, 0) / shopInRange.length : 0,
            };

            // 客户分析汇总
            const custInRange = jdCustomer.filter((d: any) => inSelectedRange(d.date));
            const custSum = {
              browse: custInRange.reduce((s: number, d: any) => s + d.browseCustomers, 0),
              cart: custInRange.reduce((s: number, d: any) => s + d.cartCustomers, 0),
              order: custInRange.reduce((s: number, d: any) => s + d.orderCustomers, 0),
              pay: custInRange.reduce((s: number, d: any) => s + d.payCustomers, 0),
              repurchase: custInRange.reduce((s: number, d: any) => s + d.repurchaseCustomers, 0),
              lost: custInRange.reduce((s: number, d: any) => s + d.lostCustomers, 0),
            };

            // 店铺经营趋势图
            const jdShopOption = jdShop.length > 0 ? {
              tooltip: { trigger: 'axis' as const },
              legend: { data: ['成交金额', '访客数', '转化率'], top: 0 },
              grid: { left: 80, right: 80, top: 40, bottom: 40 },
              xAxis: { type: 'category' as const, data: jdShop.map((d: any) => formatDate(d.date)),
                axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: jdShop.length > 15 ? 45 : 0 } },
              yAxis: [
                { type: 'value' as const, name: '金额/人数', min: 0, axisLabel: { formatter: (v: number) => fmtMoney(v) } },
                { type: 'value' as const, name: '转化率%', min: 0, position: 'right' as const, axisLabel: { formatter: '{value}%' } },
              ],
              series: [
                { name: '成交金额', type: 'bar', barWidth: 8,
                  data: jdShop.map((d: any) => ({ value: d.payAmount, itemStyle: { color: isExpanded && !inSelectedRange(d.date) ? 'rgba(245,34,45,0.25)' : '#f5222d' } })) },
                { name: '访客数', type: 'line', data: jdShop.map((d: any) => d.visitors), itemStyle: { color: '#1e40af' } },
                { name: '转化率', type: 'line', yAxisIndex: 1, smooth: true, data: jdShop.map((d: any) => d.convRate.toFixed(2)),
                  itemStyle: { color: '#faad14' }, lineStyle: { type: 'dashed' as const } },
              ],
            } : null;

            // 客户分析趋势图
            const jdCustOption = jdCustomer.length > 0 ? {
              tooltip: { trigger: 'axis' as const },
              legend: { data: ['浏览客户', '加购客户', '成交客户', '复购客户'], top: 0 },
              grid: { left: 80, right: 80, top: 40, bottom: 40 },
              xAxis: { type: 'category' as const, data: jdCustomer.map((d: any) => formatDate(d.date)),
                axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: jdCustomer.length > 15 ? 45 : 0 } },
              yAxis: [{ type: 'value' as const, name: '人数', min: 0 }],
              series: [
                { name: '浏览客户', type: 'bar', barWidth: 8,
                  data: jdCustomer.map((d: any) => ({ value: d.browseCustomers, itemStyle: { color: isExpanded && !inSelectedRange(d.date) ? 'rgba(24,144,255,0.25)' : '#1e40af' } })) },
                { name: '加购客户', type: 'line', data: jdCustomer.map((d: any) => d.cartCustomers), itemStyle: { color: '#faad14' } },
                { name: '成交客户', type: 'line', data: jdCustomer.map((d: any) => d.payCustomers), itemStyle: { color: '#f5222d' } },
                { name: '复购客户', type: 'line', data: jdCustomer.map((d: any) => d.repurchaseCustomers), itemStyle: { color: '#10b981' } },
              ],
            } : null;

            return (
              <>
                {jdShop.length > 0 && (
                  <Card title="店铺经营（京东）" style={{ marginBottom: 16 }}
                    headStyle={{ background: 'linear-gradient(90deg, #fff1f0 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
                    <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
                      {[
                        { title: '访客数', value: shopSum.visitors, accentColor: '#1e40af' },
                        { title: '浏览量', value: shopSum.pageViews, accentColor: '#06b6d4' },
                        { title: '成交客户', value: shopSum.payCustomers, accentColor: '#ef4444' },
                        { title: '成交金额', value: shopSum.payAmount, precision: 2, prefix: '¥', accentColor: '#dc2626' },
                        { title: '成交单量', value: shopSum.payCount, accentColor: '#10b981' },
                        { title: '转化率', value: shopSum.avgConvRate, precision: 2, suffix: '%', accentColor: '#f59e0b' },
                        { title: '客单价', value: shopSum.avgUnitPrice, precision: 2, prefix: '¥', accentColor: '#7c3aed' },
                        { title: 'UV价值', value: shopSum.avgUvValue, precision: 2, prefix: '¥', accentColor: '#14b8a6' },
                      ].map((card) => (
                        <Col xs={12} sm={3} key={card.title}>
                          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                            <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
                    <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>{card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}</div>
                          </Card>
                        </Col>
                      ))}
                    </Row>
                    {jdShopOption && <ReactECharts lazyUpdate={true} option={jdShopOption} style={{ height: 300 }} />}
                  </Card>
                )}
                {jdCustomer.length > 0 && (
                  <Card title="客户分析（京东）" style={{ marginBottom: 16 }}
                    headStyle={{ background: 'linear-gradient(90deg, #e6f7ff 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
                    <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
                      {[
                        { title: '浏览客户', value: custSum.browse, accentColor: '#1e40af' },
                        { title: '加购客户', value: custSum.cart, accentColor: '#f59e0b' },
                        { title: '成交客户', value: custSum.pay, accentColor: '#ef4444' },
                        { title: '复购客户', value: custSum.repurchase, accentColor: '#10b981' },
                        { title: '流失客户', value: custSum.lost, accentColor: '#94a3b8' },
                      ].map((card) => (
                        <Col xs={12} sm={4} key={card.title}>
                          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                            <Statistic title={card.title} value={card.value} />
                    <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>{card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}</div>
                          </Card>
                        </Col>
                      ))}
                    </Row>
                    {jdCustOption && <ReactECharts lazyUpdate={true} option={jdCustOption} style={{ height: 300 }} />}
                  </Card>
                )}
              </>
            );
          })()}

          {/* ========== 京东新老客+行业热词+促销数据 ========== */}
          {isJdShop && (() => {
            const customerTypes = jdOps?.customerTypes || [];
            const keywords = jdOps?.keywords || [];
            const promos = jdOps?.promos || [];
            const promoSkus = jdOps?.promoSkus || [];
            if (!customerTypes.length && !keywords.length && !promos.length) return null;
            return (
              <>
                {customerTypes.length > 0 && (
                  <Card className="bi-table-card" title="新老客分析" style={{ marginBottom: 16 }}
                    headStyle={{ background: 'linear-gradient(90deg, #fef3c7 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
                    <Table dataSource={(() => {
                      const grouped: Record<string, any> = {};
                      customerTypes.forEach((c: any) => {
                        if (!grouped[c.customerType]) grouped[c.customerType] = { type: c.customerType, total: 0, pct: 0, conv: 0, price: 0, cnt: 0 };
                        grouped[c.customerType].total += c.payCustomers;
                        grouped[c.customerType].pct = c.payPct;
                        grouped[c.customerType].conv = c.convRate;
                        grouped[c.customerType].price += c.unitPrice;
                        grouped[c.customerType].cnt += 1;
                      });
                      return Object.values(grouped).map((g: any) => ({ ...g, price: g.cnt > 0 ? +(g.price / g.cnt).toFixed(2) : 0 }));
                    })()} rowKey="type" size="small" pagination={false}
                      columns={[
                        { title: '客户类型', dataIndex: 'type', key: 'type' },
                        { title: '支付人数', dataIndex: 'total', key: 'total', sorter: (a: any, b: any) => a.total - b.total },
                        { title: '占比', dataIndex: 'pct', key: 'pct', render: (v: number) => `${v}%` },
                        { title: '转化率', dataIndex: 'conv', key: 'conv', render: (v: number) => `${v}%` },
                        { title: '客单价', dataIndex: 'price', key: 'price', render: (v: number) => `¥${v}` },
                      ]}
                    />
                  </Card>
                )}
                {keywords.length > 0 && (
                  <Card className="bi-table-card" title="行业热词TOP20" style={{ marginBottom: 16 }}
                    headStyle={{ background: 'linear-gradient(90deg, #ecfdf5 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
                    <Table dataSource={keywords} rowKey="keyword" size="small" pagination={false} scroll={{ x: 600 }}
                      columns={[
                        { title: '关键词', dataIndex: 'keyword', key: 'kw', ellipsis: true, width: 150 },
                        { title: '搜索排名', dataIndex: 'searchRank', key: 'sr' },
                        { title: '竞争排名', dataIndex: 'competeRank', key: 'cr' },
                        { title: '点击排名', dataIndex: 'clickRank', key: 'clk' },
                        { title: '成交金额区间', dataIndex: 'payAmountRange', key: 'par', ellipsis: true },
                        { title: 'TOP品牌', dataIndex: 'topBrand', key: 'tb', ellipsis: true },
                      ]}
                    />
                  </Card>
                )}
                <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
                  {promos.length > 0 && (
                    <Col xs={24} lg={12}>
                      <Card className="bi-table-card" title="促销活动汇总" headStyle={{ fontWeight: 600, fontSize: 15 }}>
                        <Table dataSource={promos} rowKey="promoType" size="small" pagination={false}
                          columns={[
                            { title: '活动类型', dataIndex: 'promoType', key: 'type' },
                            { title: '支付金额', dataIndex: 'payAmount', key: 'amt', render: (v: number) => `¥${v?.toLocaleString()}`, sorter: (a: any, b: any) => a.payAmount - b.payAmount },
                            { title: '支付人数', dataIndex: 'payUsers', key: 'users' },
                            { title: '转化率', dataIndex: 'convRate', key: 'conv', render: (v: number) => `${v}%` },
                            { title: 'UV', dataIndex: 'uv', key: 'uv' },
                          ]}
                        />
                      </Card>
                    </Col>
                  )}
                  {promoSkus.length > 0 && (
                    <Col xs={24} lg={12}>
                      <Card className="bi-table-card" title="促销商品TOP10" headStyle={{ fontWeight: 600, fontSize: 15 }}>
                        <Table dataSource={promoSkus} rowKey={(r: any) => `${r.goodsName}-${r.promoType}`} size="small" pagination={false}
                          columns={[
                            { title: '商品', dataIndex: 'goodsName', key: 'name', ellipsis: true, width: 150 },
                            { title: '活动', dataIndex: 'promoType', key: 'type' },
                            { title: '支付金额', dataIndex: 'payAmount', key: 'amt', render: (v: number) => `¥${v?.toLocaleString()}`, sorter: (a: any, b: any) => a.payAmount - b.payAmount },
                            { title: '支付人数', dataIndex: 'payUsers', key: 'users' },
                            { title: 'UV', dataIndex: 'uv', key: 'uv' },
                          ]}
                        />
                      </Card>
                    </Col>
                  )}
                </Row>
              </>
            );
          })()}

          {/* ========== 拼多多运营数据 ========== */}
          {isPddShop && (() => {
            const pddShop = pddOps?.shop || [];
            const pddGoods = pddOps?.goods || [];
            const pddVideo = pddOps?.video || [];
            const hasData = pddShop.length > 0 || pddGoods.length > 0 || pddVideo.length > 0;
            if (!hasData) return null;

            // 店铺经营汇总（只算选中范围内）
            const shopInRange = pddShop.filter((d: any) => inSelectedRange(d.date));
            const shopSum = {
              payAmount: shopInRange.reduce((s: number, d: any) => s + d.payAmount, 0),
              payCount: shopInRange.reduce((s: number, d: any) => s + d.payCount, 0),
              payOrders: shopInRange.reduce((s: number, d: any) => s + d.payOrders, 0),
              avgConvRate: shopInRange.length > 0 ? shopInRange.reduce((s: number, d: any) => s + d.convRate, 0) / shopInRange.length : 0,
              avgUnitPrice: shopInRange.length > 0 ? shopInRange.reduce((s: number, d: any) => s + d.unitPrice, 0) / shopInRange.length : 0,
            };

            // 商品数据汇总
            const goodsInRange = pddGoods.filter((d: any) => inSelectedRange(d.date));
            const goodsSum = {
              visitors: goodsInRange.reduce((s: number, d: any) => s + d.goodsVisitors, 0),
              views: goodsInRange.reduce((s: number, d: any) => s + d.goodsViews, 0),
              collect: goodsInRange.reduce((s: number, d: any) => s + d.goodsCollect, 0),
              saleGoods: goodsInRange.length > 0 ? Math.round(goodsInRange.reduce((s: number, d: any) => s + d.saleGoodsCount, 0) / goodsInRange.length) : 0,
            };

            // 短视频数据汇总
            const videoInRange = pddVideo.filter((d: any) => inSelectedRange(d.date));
            const videoSum = {
              gmv: videoInRange.reduce((s: number, d: any) => s + d.totalGmv, 0),
              orders: videoInRange.reduce((s: number, d: any) => s + d.orderCount, 0),
              feedCount: videoInRange.reduce((s: number, d: any) => s + d.feedCount, 0),
              videoViews: videoInRange.reduce((s: number, d: any) => s + d.videoViewCnt, 0),
              goodsClicks: videoInRange.reduce((s: number, d: any) => s + d.goodsClickCnt, 0),
            };

            // 店铺经营趋势图
            const pddShopOption = pddShop.length > 0 ? {
              tooltip: { trigger: 'axis' as const },
              legend: { data: ['成交金额', '成交订单数', '转化率'], top: 0 },
              grid: { left: 80, right: 80, top: 40, bottom: 40 },
              xAxis: { type: 'category' as const, data: pddShop.map((d: any) => formatDate(d.date)),
                axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: pddShop.length > 15 ? 45 : 0 } },
              yAxis: [
                { type: 'value' as const, name: '金额', min: 0, axisLabel: { formatter: (v: number) => fmtMoney(v) } },
                { type: 'value' as const, name: '转化率%', min: 0, position: 'right' as const, axisLabel: { formatter: '{value}%' } },
              ],
              series: [
                { name: '成交金额', type: 'bar', barWidth: 8,
                  data: pddShop.map((d: any) => ({ value: d.payAmount, itemStyle: { color: isExpanded && !inSelectedRange(d.date) ? 'rgba(245,34,45,0.25)' : '#f5222d' } })) },
                { name: '成交订单数', type: 'line', data: pddShop.map((d: any) => d.payOrders), itemStyle: { color: '#1e40af' } },
                { name: '转化率', type: 'line', yAxisIndex: 1, smooth: true, data: pddShop.map((d: any) => d.convRate.toFixed(2)),
                  itemStyle: { color: '#faad14' }, lineStyle: { type: 'dashed' as const } },
              ],
            } : null;

            // 商品数据趋势图
            const pddGoodsOption = pddGoods.length > 0 ? {
              tooltip: { trigger: 'axis' as const },
              legend: { data: ['商品访客', '商品浏览', '收藏'], top: 0 },
              grid: { left: 80, right: 80, top: 40, bottom: 40 },
              xAxis: { type: 'category' as const, data: pddGoods.map((d: any) => formatDate(d.date)),
                axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: pddGoods.length > 15 ? 45 : 0 } },
              yAxis: [{ type: 'value' as const, name: '人数/次', min: 0 }],
              series: [
                { name: '商品访客', type: 'bar', barWidth: 8,
                  data: pddGoods.map((d: any) => ({ value: d.goodsVisitors, itemStyle: { color: isExpanded && !inSelectedRange(d.date) ? 'rgba(245,34,45,0.25)' : '#f5222d' } })) },
                { name: '商品浏览', type: 'line', data: pddGoods.map((d: any) => d.goodsViews), itemStyle: { color: '#1e40af' } },
                { name: '收藏', type: 'line', data: pddGoods.map((d: any) => d.goodsCollect), itemStyle: { color: '#10b981' } },
              ],
            } : null;

            // 短视频趋势图
            const pddVideoOption = pddVideo.length > 0 ? {
              tooltip: { trigger: 'axis' as const },
              legend: { data: ['GMV', '播放量', '商品点击'], top: 0 },
              grid: { left: 80, right: 80, top: 40, bottom: 40 },
              xAxis: { type: 'category' as const, data: pddVideo.map((d: any) => formatDate(d.date)),
                axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: pddVideo.length > 15 ? 45 : 0 } },
              yAxis: [
                { type: 'value' as const, name: '金额', min: 0, axisLabel: { formatter: (v: number) => fmtMoney(v) } },
                { type: 'value' as const, name: '次数', min: 0, position: 'right' as const },
              ],
              series: [
                { name: 'GMV', type: 'bar', barWidth: 8,
                  data: pddVideo.map((d: any) => ({ value: d.totalGmv, itemStyle: { color: isExpanded && !inSelectedRange(d.date) ? 'rgba(245,34,45,0.25)' : '#f5222d' } })) },
                { name: '播放量', type: 'line', yAxisIndex: 1, data: pddVideo.map((d: any) => d.videoViewCnt), itemStyle: { color: '#7c3aed' } },
                { name: '商品点击', type: 'line', yAxisIndex: 1, data: pddVideo.map((d: any) => d.goodsClickCnt), itemStyle: { color: '#10b981' } },
              ],
            } : null;

            return (
              <>
                {pddShop.length > 0 && (
                  <Card title="店铺经营（拼多多）" style={{ marginBottom: 16 }}
                    headStyle={{ background: 'linear-gradient(90deg, #fff1f0 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
                    <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
                      {[
                        { title: '成交金额', value: shopSum.payAmount, precision: 2, prefix: '¥', accentColor: '#ef4444' },
                        { title: '成交件数', value: shopSum.payCount, accentColor: '#10b981' },
                        { title: '成交订单数', value: shopSum.payOrders, accentColor: '#06b6d4' },
                        { title: '平均转化率', value: shopSum.avgConvRate, precision: 2, suffix: '%', accentColor: '#f59e0b' },
                        { title: '平均客单价', value: shopSum.avgUnitPrice, precision: 2, prefix: '¥', accentColor: '#1e40af' },
                      ].map((card) => (
                        <Col xs={12} sm={4} key={card.title}>
                          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                            <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
                    <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>{card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}</div>
                          </Card>
                        </Col>
                      ))}
                    </Row>
                    {pddShopOption && <ReactECharts lazyUpdate={true} option={pddShopOption} style={{ height: 300 }} />}
                  </Card>
                )}
                {pddGoods.length > 0 && (
                  <Card title="商品数据（拼多多）" style={{ marginBottom: 16 }}
                    headStyle={{ background: 'linear-gradient(90deg, #fff7e6 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
                    <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
                      {[
                        { title: '商品访客', value: goodsSum.visitors, accentColor: '#ef4444' },
                        { title: '商品浏览量', value: goodsSum.views, accentColor: '#1e40af' },
                        { title: '收藏用户', value: goodsSum.collect, accentColor: '#10b981' },
                        { title: '日均动销商品', value: goodsSum.saleGoods, accentColor: '#7c3aed' },
                      ].map((card) => (
                        <Col xs={12} sm={4} key={card.title}>
                          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                            <Statistic title={card.title} value={card.value} />
                    <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>{card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}</div>
                          </Card>
                        </Col>
                      ))}
                    </Row>
                    {pddGoodsOption && <ReactECharts lazyUpdate={true} option={pddGoodsOption} style={{ height: 300 }} />}
                  </Card>
                )}
                {pddVideo.length > 0 && (
                  <Card title="短视频数据（拼多多）" style={{ marginBottom: 16 }}
                    headStyle={{ background: 'linear-gradient(90deg, #f9f0ff 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
                    <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
                      {[
                        { title: '视频GMV', value: videoSum.gmv, precision: 2, prefix: '¥', accentColor: '#ef4444' },
                        { title: '订单数', value: videoSum.orders, accentColor: '#1e40af' },
                        { title: '发布作品', value: videoSum.feedCount, accentColor: '#06b6d4' },
                        { title: '播放量', value: videoSum.videoViews, accentColor: '#7c3aed' },
                        { title: '商品点击', value: videoSum.goodsClicks, accentColor: '#10b981' },
                      ].map((card) => (
                        <Col xs={12} sm={4} key={card.title}>
                          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                            <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} />
                    <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>{card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}</div>
                          </Card>
                        </Col>
                      ))}
                    </Row>
                    {pddVideoOption && <ReactECharts lazyUpdate={true} option={pddVideoOption} style={{ height: 300 }} />}
                  </Card>
                )}
              </>
            );
          })()}

          {/* ========== S品渠道销售分析（电商部门） ========== */}
          {dept === 'ecommerce' && sProducts && (() => {
            const shopRank = sProducts?.shopRank || [];
            const goodsRank = sProducts?.goodsRank || [];
            const sTrend = sProducts?.trend || [];
            const details = sProducts?.details || [];
            if (shopRank.length === 0 && goodsRank.length === 0) return null;

            // KPI汇总
            const totalSales = goodsRank.reduce((s: number, g: any) => s + g.sales, 0);
            const totalQty = goodsRank.reduce((s: number, g: any) => s + g.qty, 0);
            const skuCount = goodsRank.length;

            // 平台/渠道排行图
            const shopChartData = shopRank.slice(0, 15);
            const pieColors = ['#f59e0b', '#7c3aed', '#10b981', '#f43f5e', '#3b82f6', '#06b6d4', '#be123c', '#84cc16'];
            const shopBarOption = shopChartData.length > 0 ? {
              tooltip: { trigger: 'item' as const, formatter: '{b}: ¥{c} ({d}%)' },
              legend: { bottom: 0, type: 'scroll' as const },
              series: [{
                type: 'pie', radius: ['30%', '60%'], center: ['50%', '45%'],
                label: { show: true, formatter: '{b}\n{d}%', fontSize: 11 },
                data: shopChartData.map((s: any, i: number) => ({
                  name: s.shopName, value: s.sales,
                  itemStyle: { color: pieColors[i % pieColors.length] },
                })),
              }],
            } : null;

            // S品趋势图
            const sTrendOption = sTrend.length > 0 ? {
              tooltip: { trigger: 'axis' as const },
              legend: { data: ['销售额', '销量'], top: 0 },
              grid: { left: 80, right: 80, top: 40, bottom: 40 },
              xAxis: { type: 'category' as const, data: sTrend.map((t: any) => formatDate(t.date)),
                axisTick: { alignWithLabel: true },
                axisLabel: { fontSize: 11, interval: 0, rotate: sTrend.length > 15 ? 45 : 0 } },
              yAxis: (() => {
                const maxSales = Math.max(...sTrend.map((t: any) => t.sales), 1);
                const maxQty = Math.max(...sTrend.map((t: any) => t.qty), 1);
                const salesInterval = Math.ceil(maxSales / 5 / 1000) * 1000;
                const qtyInterval = Math.ceil(maxQty / 5 / 100) * 100;
                return [
                  { type: 'value' as const, name: '金额', min: 0, max: salesInterval * 5, interval: salesInterval, axisLabel: { formatter: (v: number) => fmtMoney(v) } },
                  { type: 'value' as const, name: '销量', min: 0, max: qtyInterval * 5, interval: qtyInterval, position: 'right' as const },
                ];
              })(),
              series: [
                { name: '销售额', type: 'bar', barWidth: 8,
                  data: sTrend.map((t: any) => ({ value: t.sales, itemStyle: { color: isExpanded && !inSelectedRange(t.date) ? 'rgba(245,158,11,0.25)' : '#f59e0b' } })) },
                { name: '销量', type: 'line', yAxisIndex: 1, data: sTrend.map((t: any) => t.qty), itemStyle: { color: '#7c3aed' } },
              ],
            } : null;

            // 按商品分组明细
            const goodsDetailMap: Record<string, any[]> = {};
            details.forEach((d: any) => {
              if (!goodsDetailMap[d.goodsName]) goodsDetailMap[d.goodsName] = [];
              goodsDetailMap[d.goodsName].push(d);
            });

            return (
              <>
                <Card title="S品渠道销售分析" style={{ marginBottom: 16 }}
                  headStyle={{ background: 'linear-gradient(90deg, #fffbeb 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
                  <Row gutter={16} style={{ marginBottom: 16 }}>
                    {[
                      { title: 'S品总销售额', value: totalSales, precision: 2, prefix: '¥', accentColor: '#1e40af' },
                      { title: 'S品总销量', value: totalQty, accentColor: '#10b981' },
                      { title: 'S品SKU数', value: skuCount, accentColor: '#7c3aed' },
                    ].map((card) => (
                      <Col span={8} key={card.title}>
                        <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                          <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} />
                    <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>{card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}</div>
                        </Card>
                      </Col>
                    ))}
                  </Row>
                  {sTrendOption && <ReactECharts lazyUpdate={true} option={sTrendOption} style={{ height: 300 }} />}
                </Card>

                <Row gutter={16}>
                  {isAllShops && shopBarOption && (
                    <Col span={10}>
                      <Card title={platform === 'all' || !platform ? 'S品平台排名' : 'S品渠道排行'} style={{ marginBottom: 16 }}
                        headStyle={{ background: 'linear-gradient(90deg, #fffbeb 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
                        <ReactECharts lazyUpdate={true} option={shopBarOption} style={{ height: 320 }} />
                      </Card>
                    </Col>
                  )}
                  {goodsRank.length > 0 && (
                    <Col span={isAllShops && shopBarOption ? 14 : 24}>
                      <Card className="bi-table-card" title={`S品单品排行（点击展开查看${platform === 'all' || !platform ? '平台' : '渠道'}分布）`} style={{ marginBottom: 16 }}
                        headStyle={{ background: 'linear-gradient(90deg, #fffbeb 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
                    <Table dataSource={goodsRank} pagination={false} size="small" rowKey="goodsNo"
                      expandable={{
                        expandedRowRender: (record: any) => {
                          const shopList = goodsDetailMap[record.goodsName] || [];
                          return <GoodsChannelExpand channels={shopList} />;
                        },
                        rowExpandable: (record: any) => (goodsDetailMap[record.goodsName] || []).length > 0,
                      }}
                      columns={[
                        { title: '商品名称', dataIndex: 'goodsName', ellipsis: true },
                        { title: '销售额', dataIndex: 'sales', width: 140, render: (v: number) => '¥' + v.toLocaleString(), align: 'right' as const, sorter: (a: any, b: any) => a.sales - b.sales, defaultSortOrder: 'descend' as const },
                        { title: '销量', dataIndex: 'qty', width: 100, render: (v: number) => v.toLocaleString(), align: 'right' as const },
                        { title: platform === 'all' || !platform ? '覆盖平台数' : '覆盖渠道数', dataIndex: 'shopCount', width: 110, align: 'right' as const },
                      ]}
                    />
                  </Card>
                    </Col>
                  )}
                </Row>
              </>
            );
          })()}

          {/* ========== 天猫超市运营数据 ========== */}
          {isTmallcsShop && (() => {
            const business = tmallcsOps?.business || [];
            const tmcsCampaigns = tmallcsOps?.campaigns || [];
            const keywords = tmallcsOps?.keywords || [];
            const ranks = tmallcsOps?.ranks || [];
            if (business.length === 0 && tmcsCampaigns.length === 0 && keywords.length === 0 && ranks.length === 0) return null;

            // 经营汇总（选中范围内）
            const bInRange = business.filter((d: any) => inSelectedRange(d.date));
            const bSum = {
              payAmount: bInRange.reduce((s: number, d: any) => s + d.payAmount, 0),
              paySubOrders: bInRange.reduce((s: number, d: any) => s + d.paySubOrders, 0),
              payQty: bInRange.reduce((s: number, d: any) => s + d.payQty, 0),
              payUsers: bInRange.reduce((s: number, d: any) => s + d.payUsers, 0),
              ipvUv: bInRange.reduce((s: number, d: any) => s + d.ipvUv, 0),
              avgPrice: bInRange.length > 0 ? bInRange.reduce((s: number, d: any) => s + d.avgPrice, 0) / bInRange.length : 0,
              avgConvRate: bInRange.length > 0 ? bInRange.reduce((s: number, d: any) => s + d.convRate, 0) / bInRange.length : 0,
            };

            // 经营趋势图
            const businessOption = business.length > 0 ? {
              tooltip: { trigger: 'axis' as const },
              legend: { data: ['支付金额', '支付用户数', '转化率'], top: 0 },
              grid: { left: 80, right: 80, top: 40, bottom: 40 },
              xAxis: { type: 'category' as const, data: business.map((d: any) => formatDate(d.date)),
                axisTick: { alignWithLabel: true }, axisLabel: { fontSize: 11, interval: 0, rotate: business.length > 15 ? 45 : 0 } },
              yAxis: [
                { type: 'value' as const, name: '金额', min: 0, axisLabel: { formatter: (v: number) => fmtMoney(v) } },
                { type: 'value' as const, name: '转化率%', min: 0, position: 'right' as const, axisLabel: { formatter: '{value}%' } },
              ],
              series: [
                { name: '支付金额', type: 'bar', barWidth: 8,
                  data: business.map((d: any) => ({ value: d.payAmount, itemStyle: { color: isExpanded && !inSelectedRange(d.date) ? 'rgba(19,194,194,0.25)' : '#13c2c2' } })) },
                { name: '支付用户数', type: 'line', data: business.map((d: any) => d.payUsers), itemStyle: { color: '#1e40af' } },
                { name: '转化率', type: 'line', yAxisIndex: 1, smooth: true,
                  data: business.map((d: any) => (d.convRate * 100).toFixed(2)),
                  itemStyle: { color: '#faad14' }, lineStyle: { type: 'dashed' as const } },
              ],
            } : null;

            // 按品类分组品牌排名（取每个品类前10）
            const categoryGroups: Record<string, any[]> = {};
            ranks.forEach((r: any) => {
              if (!categoryGroups[r.category]) categoryGroups[r.category] = [];
              if (categoryGroups[r.category].length < 10) categoryGroups[r.category].push(r);
            });

            return (
              <>
                {business.length > 0 && (
                  <Card title="经营概况（天猫超市）" style={{ marginBottom: 16 }}
                    headStyle={{ background: 'linear-gradient(90deg, #e6fffb 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
                    <Row gutter={16} style={{ marginBottom: 16 }}>
                      {[
                        { title: '支付金额', value: bSum.payAmount, precision: 2, prefix: '¥', accentColor: '#14b8a6' },
                        { title: '支付用户数', value: bSum.payUsers, accentColor: '#1e40af' },
                        { title: '支付子订单', value: bSum.paySubOrders, accentColor: '#06b6d4' },
                        { title: '支付件数', value: bSum.payQty, accentColor: '#10b981' },
                        { title: '客单价', value: bSum.avgPrice, precision: 2, prefix: '¥', accentColor: '#7c3aed' },
                        { title: '平均转化率', value: (bSum.avgConvRate * 100).toFixed(2), suffix: '%', accentColor: '#f59e0b' },
                      ].map((card) => (
                        <Col span={4} key={card.title}>
                          <Card className="bi-stat-card" style={{ ['--accent-color' as any]: card.accentColor }}>
                            <Statistic title={card.title} value={card.value} precision={card.precision} prefix={card.prefix} suffix={card.suffix} />
                    <div style={{ fontSize: 13, color: '#64748b', marginTop: 4, fontVariantNumeric: 'tabular-nums', fontWeight: 400, minHeight: '1.4em' }}>{card.value >= 10000 ? `≈ ${(card.value / 10000).toFixed(1)}万` : ' '}</div>
                          </Card>
                        </Col>
                      ))}
                    </Row>
                    {businessOption && <ReactECharts lazyUpdate={true} option={businessOption} style={{ height: 320 }} />}
                  </Card>
                )}

                {keywords.length > 0 && (
                  <Card className="bi-table-card" title="行业搜索热词TOP30（天猫超市）" style={{ marginBottom: 16 }}
                    headStyle={{ background: 'linear-gradient(90deg, #e6fffb 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
                    <Table dataSource={keywords} pagination={false} size="small" rowKey="keyword"
                      columns={[
                        { title: '排名', render: (_, __, i) => i + 1, width: 60 },
                        { title: '搜索词', dataIndex: 'keyword' },
                        { title: '搜索曝光热度', dataIndex: 'searchImpression', render: (v: number) => v.toFixed(2), align: 'right' as const },
                        { title: '引导成交热度', dataIndex: 'tradeHeat', render: (v: number) => v.toFixed(2), align: 'right' as const },
                        { title: '引导成交规模', dataIndex: 'tradeScale', render: (v: number) => v.toFixed(2), align: 'right' as const },
                        { title: '引导转化指数', dataIndex: 'convIndex', render: (v: number) => v.toFixed(2), align: 'right' as const },
                        { title: '引导访问热度', dataIndex: 'visitHeat', render: (v: number) => v.toFixed(2), align: 'right' as const },
                      ]}
                    />
                  </Card>
                )}

                {Object.keys(categoryGroups).length > 0 && (
                  <Card className="bi-table-card" title="市场品牌排名（天猫超市）" style={{ marginBottom: 16 }}
                    headStyle={{ background: 'linear-gradient(90deg, #e6fffb 0%, #fff 100%)', fontWeight: 600, fontSize: 16 }}>
                    {Object.entries(categoryGroups).map(([category, list]) => (
                      <div key={category} style={{ marginBottom: 16 }}>
                        <div style={{ fontWeight: 600, marginBottom: 8, color: '#13c2c2' }}>{category}</div>
                        <Table dataSource={list} pagination={false} size="small" rowKey="brandName"
                          columns={[
                            { title: '排名', render: (_, __, i) => i + 1, width: 60 },
                            { title: '品牌', dataIndex: 'brandName' },
                            { title: '成交热度', dataIndex: 'tradeHeat', render: (v: number) => v.toFixed(2), align: 'right' as const },
                            { title: '成交人气', dataIndex: 'tradePopularity', render: (v: number) => v.toFixed(2), align: 'right' as const },
                            { title: '访问热度', dataIndex: 'visitHeat', render: (v: number) => v.toFixed(2), align: 'right' as const },
                            { title: '转化指数', dataIndex: 'convIndex', render: (v: number) => v.toFixed(2), align: 'right' as const },
                            { title: '交易指数', dataIndex: 'tradeIndex', render: (v: number) => v.toFixed(2), align: 'right' as const },
                          ]}
                        />
                      </div>
                    ))}
                  </Card>
                )}
              </>
            );
          })()}
        </>
      )}
    </div>
  );
};

export default StoreDashboard;

