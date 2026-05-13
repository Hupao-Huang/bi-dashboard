"""
LightGBM 销量预测回测 — M5 比赛冠军方案 (机器学习 + 特征工程)
全局模型: 9 大区共享参数, 用 region 特征区分
特征:
  - 滞后特征: y_lag_1/2/3/6/12
  - 移动均值: rolling_3/6/12 mean
  - 同比: y_yoy (y[t-12])
  - 时间特征: month, year, days_to_spring_festival
  - 大区: region_id (label encoded)
口径 (跟 Prophet/StatsForecast/baseline 完全对齐):
  - 月级聚合, department='offline', 10 个调料品类白名单
"""
import mysql.connector
import pandas as pd
import numpy as np
import lightgbm as lgb
from datetime import datetime
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

# 春节日期 (公历) - 用于算"距春节天数"特征
SPRING_FESTIVAL = {
    2023: '2023-01-22', 2024: '2024-02-10',
    2025: '2025-01-29', 2026: '2026-02-17', 2027: '2027-02-06',
}


def days_to_spring(ds: pd.Timestamp) -> int:
    """该月 1 号距离当年/明年春节的天数 (负数表示已过)"""
    year = ds.year
    nearest = None
    for y in (year, year + 1, year - 1):
        if y not in SPRING_FESTIVAL:
            continue
        sf = pd.Timestamp(SPRING_FESTIVAL[y])
        d = (sf - ds).days
        if nearest is None or abs(d) < abs(nearest):
            nearest = d
    return nearest if nearest is not None else 0


def fetch_monthly(conn):
    """拉全部历史月度大区销量"""
    sql = f"""
    SELECT YEAR(stat_date) AS yr, MONTH(stat_date) AS mo,
           {REGION_MAP} AS region, SUM(goods_qty) AS y
    FROM sales_goods_summary
    WHERE department='offline' AND {CATE_FILTER}
    GROUP BY YEAR(stat_date), MONTH(stat_date), region
    HAVING region IS NOT NULL
    ORDER BY yr, mo, region
    """
    df = pd.read_sql(sql, conn)
    df['ds'] = pd.to_datetime(df['yr'].astype(str) + '-' + df['mo'].astype(str).str.zfill(2) + '-01')
    df['y'] = df['y'].astype(float)
    return df[['ds', 'region', 'y']].sort_values(['region', 'ds']).reset_index(drop=True)


def build_features(df):
    """构造滞后/移动均值/同比/时间特征"""
    out = []
    for region, g in df.groupby('region'):
        g = g.set_index('ds').asfreq('MS').reset_index()  # 补齐月份, 缺失置 NaN
        g['region'] = region
        g['region_id'] = REGION_ID.get(region, -1)
        # 滞后特征
        for lag in (1, 2, 3, 6, 12):
            g[f'y_lag_{lag}'] = g['y'].shift(lag)
        # 移动均值 (用滞后值, 不偷看)
        for w in (3, 6, 12):
            g[f'roll_{w}_mean'] = g['y'].shift(1).rolling(w, min_periods=1).mean()
        # 同比
        g['y_yoy'] = g['y'].shift(12)
        # 时间特征
        g['month'] = g['ds'].dt.month
        g['year'] = g['ds'].dt.year
        g['days_to_sf'] = g['ds'].apply(days_to_spring)
        g['is_pre_sf'] = ((g['days_to_sf'] >= 0) & (g['days_to_sf'] <= 30)).astype(int)
        g['is_post_sf'] = ((g['days_to_sf'] < 0) & (g['days_to_sf'] >= -7)).astype(int)
        out.append(g)
    return pd.concat(out, ignore_index=True)


FEATURE_COLS = [
    'region_id', 'month', 'days_to_sf', 'is_pre_sf', 'is_post_sf',
    'y_lag_1', 'y_lag_2', 'y_lag_3', 'y_lag_6', 'y_lag_12',
    'roll_3_mean', 'roll_6_mean', 'roll_12_mean', 'y_yoy',
]


def upsert_backtest(conn, ym, train_end, region, fc, actual):
    if fc is None or actual == 0:
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
        (ym, 'lightgbm', region, round(fc), round(actual), err_pct, abs_err, train_end),
    )
    cur.close()


def backtest_one_month(features_full, predict_ds, train_end_ds):
    """单月回测: 用 train_end 之前样本训练, 预测 predict_ds 那月"""
    train = features_full[features_full['ds'] <= train_end_ds].copy()
    test = features_full[features_full['ds'] == predict_ds].copy()
    # 训练样本必须 y 有值且关键特征有值
    train = train.dropna(subset=['y', 'y_lag_1'])
    if train.empty or test.empty:
        return {}

    X_tr = train[FEATURE_COLS].fillna(0)
    y_tr = train['y']
    X_te = test[FEATURE_COLS].fillna(0)

    model = lgb.LGBMRegressor(
        n_estimators=300,
        learning_rate=0.05,
        num_leaves=31,
        min_child_samples=5,
        feature_fraction=0.9,
        bagging_fraction=0.9,
        bagging_freq=3,
        verbose=-1,
        random_state=42,
    )
    model.fit(X_tr, y_tr)
    yhat = model.predict(X_te)
    return dict(zip(test['region'].values, yhat))


def fetch_actual_monthly(conn, year_month):
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


def main():
    conn = mysql.connector.connect(**DB)
    raw = fetch_monthly(conn)
    print(f"拉到月度数据 {len(raw)} 行 ({raw['ds'].min()} ~ {raw['ds'].max()})")
    features = build_features(raw)

    # 支持 --months=YYYY-MM,YYYY-MM,... 默认上月
    import argparse
    p = argparse.ArgumentParser()
    p.add_argument('--months', default='')
    args = p.parse_args()
    if args.months.strip():
        target_months = [m.strip() for m in args.months.split(',') if m.strip()]
    else:
        today = pd.Timestamp.now()
        target_months = [(today - pd.offsets.MonthBegin(1)).strftime('%Y-%m')]
    backtest_months = [(ym, (pd.Timestamp(f"{ym}-01") - pd.Timedelta(days=1)).strftime('%Y-%m-%d'))
                       for ym in target_months]
    print(f"LightGBM 回测月份: {target_months}")
    for ym, train_end in backtest_months:
        predict_ds = pd.Timestamp(f"{ym}-01")
        train_end_ds = pd.Timestamp(train_end)
        # 因为 build_features 已经做了 shift, train_end_ds 这个月的样本 y 已经能用了 → 训练截止应包括到训练月份末
        # 这里我们要保证 X 只用截至 train_end 的信息, 所以用 ds <= train_end_ds 选月份(月初日期)
        # 但 train_end 是月末日期, train_end_ds 月初也算; 简单处理: 训练用 ds < predict_ds 即 ds <= train_end 月的所有样本
        train_cutoff = predict_ds - pd.offsets.MonthBegin(1)
        forecasts = backtest_one_month(features, predict_ds, train_cutoff)
        actuals = fetch_actual_monthly(conn, ym)
        print(f"\n== {ym} (训练截至 {train_end}) ==")
        for region in REGIONS:
            fc = forecasts.get(region)
            a = actuals.get(region, 0)
            if fc is not None and a > 0:
                err = (fc - a) / a * 100
                print(f"  {region}: LGB={fc:8.0f} vs 实际={a:8.0f}  误差 {err:+.1f}%")
                upsert_backtest(conn, ym, train_end, region, fc, a)
            else:
                print(f"  {region}: LGB={fc} 实际={a} 跳过")
        conn.commit()
    print("\n[OK] LightGBM 回测已 UPSERT 入 offline_sales_forecast_backtest (algo=lightgbm)")
    conn.close()


if __name__ == '__main__':
    main()
