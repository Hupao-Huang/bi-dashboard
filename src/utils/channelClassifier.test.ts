// channelClassifier.test.ts — getPlatform / getDepartment 单元测试
// 已 Read channelClassifier.ts 全文 (跟 GoodsChannelExpand.tsx 抽出的逻辑一致)
// 防御本次 v1.46.0 视频号 bug + 历史 dept 分类规则

import { getPlatform, getDepartment } from './channelClassifier';

describe('getPlatform', () => {
  describe('电商部门 (ds- 前缀)', () => {
    it('天猫超市 / 天猫 / 京东 / 拼多多 / 唯品会', () => {
      expect(getPlatform('ds-天猫超市-松鲜鲜旗舰店')).toBe('天猫超市');
      expect(getPlatform('ds-天猫-松鲜鲜调味品旗舰店')).toBe('天猫');
      expect(getPlatform('ds-京东-松鲜鲜调味油旗舰店')).toBe('京东');
      expect(getPlatform('ds-拼多多-松鲜鲜旗舰店')).toBe('拼多多');
      expect(getPlatform('ds-唯品会-松鲜鲜旗舰店')).toBe('唯品会');
    });
    it('天猫优先级: ds-天猫超市 不能误判成 ds-天猫-', () => {
      // 顺序敏感: getPlatform 先检查 ds-天猫超市 再 ds-天猫-
      expect(getPlatform('ds-天猫超市-XX')).toBe('天猫超市');
      expect(getPlatform('ds-天猫超市-寄售')).toBe('天猫超市');
    });
  });

  describe('即时零售部 (js- 前缀)', () => {
    it('js- 直接归即时零售平台', () => {
      expect(getPlatform('js-即时零售-盒马')).toBe('即时零售');
      expect(getPlatform('js-朴朴-XXX')).toBe('即时零售');
    });
  });

  describe('社媒部门', () => {
    it('抖音 / 快手 / 小红书', () => {
      expect(getPlatform('社媒-抖音-松鲜鲜旗舰店')).toBe('抖音');
      expect(getPlatform('社媒-快手-松鲜鲜官方旗舰店')).toBe('快手');
      expect(getPlatform('社媒-小红书-松鲜鲜旗舰店')).toBe('小红书');
    });
    it('视频号 / 视频小店 都识别 (本次 v1.46.0 修的 bug 防御)', () => {
      expect(getPlatform('社媒-视频小店-悦楚专卖店')).toBe('视频号');
      expect(getPlatform('社媒-视频号-XX')).toBe('视频号');
      expect(getPlatform('微信视频号小店-X')).toBe('视频号');
    });
    it('有赞 / 微店 / 微信销售 (本次 v1.46.0 顺手补的)', () => {
      expect(getPlatform('sy-有赞-松鲜鲜官方旗舰店')).toBe('有赞');
      expect(getPlatform('sy-微店-X')).toBe('微店');
      expect(getPlatform('sy-微信销售分销客户')).toBe('微信销售');
    });
  });

  describe('分销 / 线下', () => {
    it('分销渠道', () => {
      expect(getPlatform('分销一组-名义初品')).toBe('分销');
      expect(getPlatform('分销八组-礼品')).toBe('分销');
    });
    it('线下渠道', () => {
      expect(getPlatform('线下渠道销售中心-华北大区')).toBe('线下');
    });
  });

  describe('未匹配', () => {
    it('未识别归"其他"', () => {
      expect(getPlatform('随意未知 shop')).toBe('其他');
      expect(getPlatform('')).toBe('其他');
      expect(getPlatform('品牌中心-小红书')).toBe('小红书'); // 注意: 含小红书优先匹配, 即使是品牌中心
    });
  });

  describe('优先级顺序', () => {
    it('ds-/js- 前缀优先于 includes 关键字', () => {
      // ds-天猫超市 含"天猫超市" 但因 startsWith 优先, 不会落到包含逻辑
      expect(getPlatform('ds-天猫超市-X')).toBe('天猫超市');
    });
    it('微信销售 优先于 分销 (源码顺序)', () => {
      // shopName="sy-微信销售分销客户" 同时含"微信销售"和"分销", 按源码先后, 先匹配微信销售
      expect(getPlatform('sy-微信销售分销客户')).toBe('微信销售');
    });
  });
});

describe('getDepartment', () => {
  it('js-即时零售- 优先归 即时零售部', () => {
    expect(getDepartment('js-即时零售-盒马')).toBe('即时零售部');
    // js-即时零售 不能落到电商部门 (顺序敏感)
  });

  it('其他 ds-/js- 归电商部门', () => {
    expect(getDepartment('ds-天猫-X')).toBe('电商部门');
    expect(getDepartment('ds-京东-X')).toBe('电商部门');
    expect(getDepartment('js-朴朴-X')).toBe('电商部门'); // 注意: js- 但不是即时零售前缀
  });

  it('社媒部门: 社媒- 前缀 或 含抖音/快手/小红书/视频号/有赞/微店/飞瓜', () => {
    expect(getDepartment('社媒-X')).toBe('社媒部门');
    expect(getDepartment('随便-抖音-X')).toBe('社媒部门'); // 含抖音
    expect(getDepartment('sy-有赞-X')).toBe('社媒部门');
    expect(getDepartment('社媒-抖音-飞瓜')).toBe('社媒部门');
    expect(getDepartment('微信视频号小店')).toBe('社媒部门'); // 含视频号
  });

  it('分销部门 (含分销关键字, 但社媒优先)', () => {
    expect(getDepartment('分销一组-名义初品')).toBe('分销部门');
    // 注意: '社媒-...分销' 要看顺序
    expect(getDepartment('社媒-分销-X')).toBe('社媒部门'); // 社媒- 前缀优先
  });

  it('线下部门 (含线下/大区)', () => {
    expect(getDepartment('线下渠道销售中心-华北大区')).toBe('线下部门');
    expect(getDepartment('xx大区')).toBe('线下部门');
  });

  it('未匹配归"其他"', () => {
    expect(getDepartment('未知 shop')).toBe('其他');
    expect(getDepartment('')).toBe('其他');
  });

  it('priority: js-即时零售 > 其他 js- (顺序敏感测试)', () => {
    // 源码: 先 if startsWith('js-即时零售'), 再 if startsWith('js-')
    expect(getDepartment('js-即时零售-盒马')).toBe('即时零售部');
    expect(getDepartment('js-其他-X')).toBe('电商部门');
  });
});
