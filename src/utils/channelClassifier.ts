// 渠道分类器 — 从 shop_name 推断平台和部门
// 抽自 src/components/GoodsChannelExpand.tsx, 加 jest test 覆盖.
//
// 后端对应 platLabelMap (server/internal/handler/dashboard_department.go:381),
// memory feedback_test_and_verify L2 教训: 前后端映射应一致, 视频小店 → 视频号 防御.

export function getPlatform(shopName: string): string {
  if (shopName.startsWith('ds-天猫超市')) return '天猫超市';
  if (shopName.startsWith('ds-天猫-')) return '天猫';
  if (shopName.startsWith('ds-京东-')) return '京东';
  if (shopName.startsWith('ds-拼多多-')) return '拼多多';
  if (shopName.startsWith('ds-唯品会-')) return '唯品会';
  if (shopName.startsWith('js-')) return '即时零售';
  if (shopName.includes('抖音')) return '抖音';
  if (shopName.includes('快手')) return '快手';
  if (shopName.includes('小红书')) return '小红书';
  if (shopName.includes('视频小店') || shopName.includes('视频号')) return '视频号';
  if (shopName.includes('有赞')) return '有赞';
  if (shopName.includes('微店')) return '微店';
  if (shopName.includes('微信销售')) return '微信销售';
  if (shopName.includes('分销')) return '分销';
  if (shopName.includes('线下')) return '线下';
  return '其他';
}

export function getDepartment(shopName: string): string {
  // v1.02: js- 前缀拆出即时零售部 (跟数据库 department='instant_retail' 对齐)
  if (shopName.startsWith('js-即时零售')) return '即时零售部';
  if (shopName.startsWith('ds-') || shopName.startsWith('js-')) return '电商部门';
  if (
    shopName.startsWith('社媒-') ||
    shopName.includes('抖音') ||
    shopName.includes('快手') ||
    shopName.includes('小红书') ||
    shopName.includes('视频号') ||
    shopName.includes('有赞') ||
    shopName.includes('微店') ||
    shopName.includes('飞瓜')
  ) {
    return '社媒部门';
  }
  if (shopName.includes('分销')) return '分销部门';
  if (shopName.includes('线下') || shopName.includes('大区')) return '线下部门';
  return '其他';
}
