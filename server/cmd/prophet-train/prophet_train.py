"""
Prophet 训练脚本 — 大区级月度销量预测, 结果 UPSERT 到 offline_sales_forecast_prophet
SKU 级分摊由 Go 后端运行时算 (用历史占比).

跑法: python prophet_train.py [起始预测月 YYYY-MM]
默认从当月起算未来 12 个月.
"""
import sys
import mysql.connector
import pandas as pd
from prophet import Prophet
from datetime import datetime
from dateutil.relativedelta import relativedelta
import warnings
warnings.filterwarnings('ignore')
import logging
logging.getLogger('cmdstanpy').setLevel(logging.WARNING)
logging.getLogger('prophet').setLevel(logging.WARNING)

DB = dict(host='127.0.0.1', port=3306, user='root', password='Hch123456', database='bi_dashboard')

REGIONS = ['华北大区', '华东大区', '华中大区', '华南大区', '西南大区',
           '西北大区', '东北大区', '山东大区', '重客']

REGION_MAP_SQL = """
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

# 中国春节 (上下窗口 = 囤货/恢复期)
HOLIDAYS = pd.DataFrame([
    {'holiday': 'spring_festival', 'ds': pd.Timestamp('2024-02-10'), 'lower_window': -30, 'upper_window': 7},
    {'holiday': 'spring_festival', 'ds': pd.Timestamp('2025-01-29'), 'lower_window': -30, 'upper_window': 7},
    {'holiday': 'spring_festival', 'ds': pd.Timestamp('2026-02-17'), 'lower_window': -30, 'upper_window': 7},
    {'holiday': 'spring_festival', 'ds': pd.Timestamp('2027-02-06'), 'lower_window': -30, 'upper_window': 7},
    {'holiday': 'national_day', 'ds': pd.Timestamp('2024-10-01'), 'lower_window': -3, 'upper_window': 7},
    {'holiday': 'national_day', 'ds': pd.Timestamp('2025-10-01'), 'lower_window': -3, 'upper_window': 7},
    {'holiday': 'national_day', 'ds': pd.Timestamp('2026-10-01'), 'lower_window': -3, 'upper_window': 7},
    {'holiday': 'mid_autumn', 'ds': pd.Timestamp('2024-09-17'), 'lower_window': -7, 'upper_window': 3},
    {'holiday': 'mid_autumn', 'ds': pd.Timestamp('2025-10-06'), 'lower_window': -7, 'upper_window': 3},
    {'holiday': 'mid_autumn', 'ds': pd.Timestamp('2026-09-25'), 'lower_window': -7, 'upper_window': 3},
])

def fetch_daily_region(conn):
    sql = f"""
    SELECT stat_date AS ds, {REGION_MAP_SQL} AS region, SUM(goods_qty) AS y
    FROM sales_goods_summary
    WHERE department='offline' AND {CATE_FILTER}
    GROUP BY stat_date, region HAVING region IS NOT NULL
    ORDER BY stat_date, region
    """
    df = pd.read_sql(sql, conn)
    df['ds'] = pd.to_datetime(df['ds'])
    df['y'] = df['y'].astype(float)
    return df

def train_and_predict(daily_df, region, future_months):
    df = daily_df[daily_df['region'] == region][['ds', 'y']].copy()
    if len(df) < 60:
        print(f"  {region}: 数据不足 ({len(df)} 天), 跳过")
        return []
    m = Prophet(
        yearly_seasonality=True,
        weekly_seasonality=True,
        daily_seasonality=False,
        holidays=HOLIDAYS,
        changepoint_prior_scale=0.05,
        seasonality_mode='multiplicative',
    )
    m.fit(df)
    last_date = df['ds'].max()
    future_end = (last_date + relativedelta(months=future_months+1)).replace(day=1) - pd.Timedelta(days=1)
    future = m.make_future_dataframe(periods=(future_end - last_date).days, freq='D')
    fc = m.predict(future)
    fc = fc[fc['ds'] > last_date]
    fc['ym'] = fc['ds'].dt.strftime('%Y-%m')
    monthly = fc.groupby('ym').agg(
        forecast_qty=('yhat','sum'),
        yhat_lower=('yhat_lower','sum'),
        yhat_upper=('yhat_upper','sum'),
    ).reset_index()
    return [
        dict(ym=r['ym'], region=region,
             forecast_qty=max(0, round(r['forecast_qty'])),
             yhat_lower=max(0, round(r['yhat_lower'])),
             yhat_upper=max(0, round(r['yhat_upper'])))
        for _, r in monthly.iterrows()
    ]

def main():
    future_months = 12
    conn = mysql.connector.connect(**DB)
    print(f"[Prophet] 拉历史日数据...")
    daily = fetch_daily_region(conn)
    print(f"[Prophet] 历史 {len(daily)} 行 / {daily['ds'].min().date()} ~ {daily['ds'].max().date()}")

    all_rows = []
    for region in REGIONS:
        print(f"[Prophet] 训练 {region}...")
        rows = train_and_predict(daily, region, future_months)
        all_rows.extend(rows)
        print(f"  -> 输出 {len(rows)} 月")

    print(f"\n[Prophet] 共 {len(all_rows)} 条 UPSERT 数据库")
    cur = conn.cursor()
    cur.executemany(
        """INSERT INTO offline_sales_forecast_prophet
           (ym, region, forecast_qty, forecast_yhat_lower, forecast_yhat_upper)
           VALUES (%(ym)s, %(region)s, %(forecast_qty)s, %(yhat_lower)s, %(yhat_upper)s)
           ON DUPLICATE KEY UPDATE
             forecast_qty=VALUES(forecast_qty),
             forecast_yhat_lower=VALUES(forecast_yhat_lower),
             forecast_yhat_upper=VALUES(forecast_yhat_upper),
             trained_at=NOW()""",
        all_rows
    )
    conn.commit()
    print(f"[Prophet] UPSERT 完成")
    cur.close()
    conn.close()

if __name__ == '__main__':
    main()
