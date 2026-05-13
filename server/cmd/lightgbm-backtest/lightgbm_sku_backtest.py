"""
LightGBM 销量预测回测 SKU 级 — M5 比赛冠军标配
全局模型: 训练样本 = SKU × 大区 × 月 (~4 万行), 比大区合计版多 200 倍
预测后 SUM 到大区维度入库, 跟其他算法可比 (algo='lightgbm_sku')

特征 (SKU 级):
  - 滞后: y_lag_1/2/3/6/12 (该 SKU×大区序列)
  - 移动均值: rolling_3/6/12 mean (用滞后值, 不偷看)
  - 同比: y_yoy (y[t-12])
  - 时间: month, days_to_sf, is_pre/post_sf
  - SKU 编码: sku_id (label encoded)
  - 大区编码: region_id
  - 品类编码: cate_id
  - 大区×品类: region_cate_id (交叉特征)
  - SKU 历史均值: sku_history_mean (训练截至前的全 SKU 均值)

口径 (跟其他算法对齐):
  - department='offline' + 10 个调料品类白名单
"""
import mysql.connector
import pandas as pd
import numpy as np
import lightgbm as lgb
import argparse
import warnings
warnings.filterwarnings('ignore')

DB = dict(host='127.0.0.1', port=3306, user='root', password='Hch123456', database='bi_dashboard')

REGION_MAP = """
    CASE
      WHEN shop_name LIKE '%华东大区%' THEN '华东大区'
      WHEN shop_name LIKE '%华北大区%' THEN '华北大区'
      WHEN shop_name LIKE '%华南大区%' THEN '华南大区'
      WHEN shop_name LIKE '%华中大区%' THEN '华中大区'
      WHEN shop_name LIKE '%西北大区%' THEN '西北大区'
      WHEN shop_name LIKE '%西南大区%' THEN '西南大区'
      WHEN shop_name LIKE '%东北大区%' THEN '东北大区'
      WHEN shop_name LIKE '%山东大区%' OR shop_name LIKE '%山东省区%' THEN '山东大区'
      WHEN shop_name LIKE '%重客系统%' THEN '重客'
      ELSE NULL END
"""

CATE_FILTER = "cate_name IN ('调味料','酱油','调味汁','干制面','素蚝油','酱类','醋','汤底','番茄沙司','糖')"

REGIONS = ['华北大区', '华东大区', '华中大区', '华南大区', '西南大区',
           '西北大区', '东北大区', '山东大区', '重客']
REGION_ID = {r: i for i, r in enumerate(REGIONS)}

# ============ 中国传统节日 + 电商节日历 (公历) ============
# 调味料/食品行业最相关 — 按业务相关度精选, 不加冷门节日避免过拟合
HOLIDAYS = {
    'spring_festival': {  # 春节: 大头, 全年最高峰
        2023: '2023-01-22', 2024: '2024-02-10',
        2025: '2025-01-29', 2026: '2026-02-17', 2027: '2027-02-06',
    },
    'mid_autumn': {  # 中秋: 月饼/礼盒/调味料礼包
        2023: '2023-09-29', 2024: '2024-09-17',
        2025: '2025-10-06', 2026: '2026-09-25', 2027: '2027-09-15',
    },
    'dragon_boat': {  # 端午: 粽子/咸鸭蛋/调味料
        2023: '2023-06-22', 2024: '2024-06-10',
        2025: '2025-05-31', 2026: '2026-06-19', 2027: '2027-06-09',
    },
    'national_day': {  # 国庆: 餐饮高峰 + 礼盒
        2023: '2023-10-01', 2024: '2024-10-01',
        2025: '2025-10-01', 2026: '2026-10-01', 2027: '2027-10-01',
    },
    'labor_day': {  # 五一: 餐饮高峰
        2023: '2023-05-01', 2024: '2024-05-01',
        2025: '2025-05-01', 2026: '2026-05-01', 2027: '2027-05-01',
    },
    'laba': {  # 腊八: 北方腊月头, 备年货起点
        2023: '2023-12-30', 2024: '2024-01-18',
        2025: '2025-01-07', 2026: '2026-01-26', 2027: '2027-01-15',
    },
    'xiao_nian': {  # 小年 (北方腊月廿三): 春节前最后一波囤货
        2023: '2023-01-14', 2024: '2024-02-02',
        2025: '2025-01-22', 2026: '2026-02-10', 2027: '2027-01-30',
    },
    'winter_solstice': {  # 冬至: 北方吃饺子, 调味料用
        2023: '2023-12-22', 2024: '2024-12-21',
        2025: '2025-12-21', 2026: '2026-12-22', 2027: '2027-12-22',
    },
    'double11': {  # 双11: 调味料囤货大节
        2023: '2023-11-11', 2024: '2024-11-11',
        2025: '2025-11-11', 2026: '2026-11-11', 2027: '2027-11-11',
    },
    '618': {  # 618: 京东大促, 调味料囤货
        2023: '2023-06-18', 2024: '2024-06-18',
        2025: '2025-06-18', 2026: '2026-06-18', 2027: '2027-06-18',
    },
}


def days_to_holiday(ds: pd.Timestamp, holiday_dates: dict) -> int:
    """计算 ds 距离最近一次 holiday 的天数 (正=未到, 负=已过)"""
    nearest = None
    for y in (ds.year - 1, ds.year, ds.year + 1):
        if y not in holiday_dates:
            continue
        d = (pd.Timestamp(holiday_dates[y]) - ds).days
        if nearest is None or abs(d) < abs(nearest):
            nearest = d
    return nearest if nearest is not None else 999


def month_overlap_days(ds: pd.Timestamp, holiday_dates: dict, window_before: int, window_after: int) -> int:
    """计算该月与 holiday±window 区间的重叠天数 (0-31)"""
    month_start = ds
    month_end = month_start + pd.offsets.MonthEnd(0)
    total = 0
    for y in (ds.year - 1, ds.year, ds.year + 1):
        if y not in holiday_dates:
            continue
        h = pd.Timestamp(holiday_dates[y])
        win_start = h - pd.Timedelta(days=window_before)
        win_end = h + pd.Timedelta(days=window_after)
        # 该月与窗口的交集
        s = max(month_start, win_start)
        e = min(month_end, win_end)
        if s <= e:
            total += (e - s).days + 1
    return total


# 保留旧函数兼容
def days_to_spring(ds: pd.Timestamp) -> int:
    return days_to_holiday(ds, HOLIDAYS['spring_festival'])


def fetch_sku_monthly(conn):
    """拉 SKU × 大区 月度销量 (4 万行级别)"""
    sql = f"""
    SELECT YEAR(stat_date) AS yr, MONTH(stat_date) AS mo,
           goods_no AS sku_code, cate_name,
           {REGION_MAP} AS region,
           SUM(goods_qty) AS y
    FROM sales_goods_summary
    WHERE department='offline' AND {CATE_FILTER}
      AND goods_no IS NOT NULL AND goods_no <> ''
    GROUP BY YEAR(stat_date), MONTH(stat_date), goods_no, cate_name, region
    HAVING region IS NOT NULL
    """
    df = pd.read_sql(sql, conn)
    df['ds'] = pd.to_datetime(df['yr'].astype(str) + '-' + df['mo'].astype(str).str.zfill(2) + '-01')
    df['y'] = df['y'].astype(float)
    return df[['ds', 'sku_code', 'cate_name', 'region', 'y']].sort_values(
        ['sku_code', 'region', 'ds']).reset_index(drop=True)


def build_sku_features(df):
    """构造 SKU × 大区 序列的特征 (每个 SKU×region 是独立时间序列)"""
    out = []
    # 全部月份范围
    min_ds, max_ds = df['ds'].min(), df['ds'].max()
    all_months = pd.date_range(min_ds, max_ds, freq='MS')

    # SKU label encoding
    sku_list = sorted(df['sku_code'].unique())
    sku_id_map = {s: i for i, s in enumerate(sku_list)}
    # 品类 label encoding
    cate_list = sorted(df['cate_name'].dropna().unique())
    cate_id_map = {c: i for i, c in enumerate(cate_list)}

    sku_cate = df.drop_duplicates('sku_code').set_index('sku_code')['cate_name'].to_dict()

    for (sku, region), g in df.groupby(['sku_code', 'region']):
        g = g.set_index('ds')['y'].reindex(all_months, fill_value=0.0).rename_axis('ds').reset_index()
        g.columns = ['ds', 'y']
        g['sku_code'] = sku
        g['region'] = region
        g['sku_id'] = sku_id_map[sku]
        g['region_id'] = REGION_ID[region]
        cate = sku_cate.get(sku, '')
        g['cate_id'] = cate_id_map.get(cate, -1)
        g['region_cate_id'] = g['region_id'] * 100 + g['cate_id']
        for lag in (1, 2, 3, 6, 12):
            g[f'y_lag_{lag}'] = g['y'].shift(lag)
        for w in (3, 6, 12):
            g[f'roll_{w}_mean'] = g['y'].shift(1).rolling(w, min_periods=1).mean()
        g['y_yoy'] = g['y'].shift(12)
        g['month'] = g['ds'].dt.month
        # 仅春节特征 (验证: 加多节日 overlap 反而退步 4.2pp, 见 commit 历史)
        # HOLIDAYS 字典/days_to_holiday/month_overlap_days 工具函数保留, 留给后续旬级实验用
        g['days_to_sf'] = g['ds'].apply(days_to_spring)
        g['is_pre_sf'] = ((g['days_to_sf'] >= 0) & (g['days_to_sf'] <= 30)).astype(int)
        g['is_post_sf'] = ((g['days_to_sf'] < 0) & (g['days_to_sf'] >= -7)).astype(int)
        out.append(g)
    full = pd.concat(out, ignore_index=True)
    return full, sku_id_map, cate_id_map


FEATURE_COLS = [
    'sku_id', 'region_id', 'cate_id', 'region_cate_id',
    'month', 'days_to_sf', 'is_pre_sf', 'is_post_sf',
    'y_lag_1', 'y_lag_2', 'y_lag_3', 'y_lag_6', 'y_lag_12',
    'roll_3_mean', 'roll_6_mean', 'roll_12_mean', 'y_yoy',
]
CAT_COLS = ['sku_id', 'region_id', 'cate_id', 'region_cate_id', 'month']


def backtest_one_month(features_full, predict_ds):
    """单月回测: 用 ds < predict_ds 训练, 预测 predict_ds 那月"""
    train = features_full[features_full['ds'] < predict_ds].copy()
    test = features_full[features_full['ds'] == predict_ds].copy()
    train = train.dropna(subset=['y_lag_1'])  # 至少要有一阶滞后
    if train.empty or test.empty:
        return pd.DataFrame()

    X_tr = train[FEATURE_COLS].fillna(0)
    y_tr = train['y']
    X_te = test[FEATURE_COLS].fillna(0)

    model = lgb.LGBMRegressor(
        n_estimators=500,
        learning_rate=0.05,
        num_leaves=63,
        min_child_samples=20,
        feature_fraction=0.85,
        bagging_fraction=0.85,
        bagging_freq=3,
        verbose=-1,
        random_state=42,
    )
    model.fit(X_tr, y_tr, categorical_feature=CAT_COLS)
    yhat = model.predict(X_te)
    yhat = np.maximum(yhat, 0)  # 销量不能负
    test = test.copy()
    test['yhat'] = yhat
    return test[['sku_code', 'region', 'yhat']]


def fetch_actual_region_monthly(conn, year_month):
    """拉指定月份每大区的实际销量 (大区合计, 跟其他算法口径一致)"""
    first_day = f"{year_month}-01"
    next_month = pd.to_datetime(first_day) + pd.offsets.MonthBegin(1)
    last_day = (next_month - pd.Timedelta(days=1)).strftime('%Y-%m-%d')
    sql = f"""
    SELECT {REGION_MAP} AS region, SUM(goods_qty) AS qty
    FROM sales_goods_summary
    WHERE department='offline' AND stat_date BETWEEN %s AND %s AND {CATE_FILTER}
    GROUP BY region HAVING region IS NOT NULL
    """
    df = pd.read_sql(sql, conn, params=(first_day, last_day))
    return dict(zip(df['region'], df['qty'].astype(float)))


def upsert_backtest(conn, algo, ym, train_end, region, fc, actual):
    if actual == 0:
        return
    err_pct = round((fc - actual) / actual * 100, 2)
    abs_err = abs(err_pct)
    cur = conn.cursor()
    cur.execute(
        """INSERT INTO offline_sales_forecast_backtest
           (ym, algo, region, forecast_qty, actual_qty, err_pct, abs_err_pct, train_end_date, run_at)
           VALUES (%s,%s,%s,%s,%s,%s,%s,%s,NOW())
           ON DUPLICATE KEY UPDATE
             forecast_qty=VALUES(forecast_qty),
             actual_qty=VALUES(actual_qty),
             err_pct=VALUES(err_pct),
             abs_err_pct=VALUES(abs_err_pct),
             train_end_date=VALUES(train_end_date),
             run_at=NOW()""",
        (ym, algo, region, round(fc), round(actual), err_pct, abs_err, train_end),
    )
    cur.close()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--months', default='', help='逗号分隔回测月份, 默认上月')
    args = parser.parse_args()
    if args.months.strip():
        target_months = [m.strip() for m in args.months.split(',') if m.strip()]
    else:
        today = pd.Timestamp.now()
        target_months = [(today - pd.offsets.MonthBegin(1)).strftime('%Y-%m')]

    conn = mysql.connector.connect(**DB)
    print('拉 SKU 级月度数据 ...')
    raw = fetch_sku_monthly(conn)
    print(f'  SKU 数={raw["sku_code"].nunique()}, 大区数={raw["region"].nunique()}, '
          f'月数={raw["ds"].nunique()}, 总行数={len(raw)}')

    print('构造特征 (这一步占用最多内存) ...')
    features, sku_map, cate_map = build_sku_features(raw)
    print(f'  特征表行数={len(features)} (理论上限 {raw["sku_code"].nunique()}×9×{raw["ds"].nunique()})')

    for ym in target_months:
        predict_ds = pd.Timestamp(f'{ym}-01')
        train_end = (predict_ds - pd.Timedelta(days=1)).strftime('%Y-%m-%d')
        print(f'\n== {ym} (训练截至 {train_end}) ==')
        preds = backtest_one_month(features, predict_ds)
        if preds.empty:
            print('  ⚠️ 当月无可预测样本')
            continue
        # 聚合到大区
        region_pred = preds.groupby('region')['yhat'].sum().to_dict()
        actuals = fetch_actual_region_monthly(conn, ym)
        for region in REGIONS:
            fc = region_pred.get(region, 0)
            a = actuals.get(region, 0)
            if a > 0:
                err = (fc - a) / a * 100
                print(f'  {region}: LGB-SKU={fc:8.0f} vs 实际={a:8.0f}  误差 {err:+6.1f}%')
                upsert_backtest(conn, 'lightgbm_sku', ym, train_end, region, fc, a)
            else:
                print(f'  {region}: 实际=0, 跳过')
        conn.commit()
    print('\n[OK] LightGBM SKU 级回测已 UPSERT 入 offline_sales_forecast_backtest (algo=lightgbm_sku)')
    conn.close()


if __name__ == '__main__':
    main()
