"""
Prophet 销量预测回测 — 对比 2026 年 1/2/3 月预测 vs 实际
按"大区合计"维度回测 (而非 SKU × 大区 维度,先快速跑出来看效果)
"""
import mysql.connector
import pandas as pd
from prophet import Prophet
import warnings
warnings.filterwarnings('ignore')
import logging
logging.getLogger('cmdstanpy').setLevel(logging.WARNING)

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

# 中国春节日期 (Prophet holiday 用)
SPRING_FESTIVAL = pd.DataFrame({
    'holiday': 'spring_festival',
    'ds': pd.to_datetime(['2024-02-10', '2025-01-29', '2026-02-17', '2027-02-06']),
    'lower_window': -30,  # 春节前 30 天囤货期
    'upper_window': 7,    # 春节后 7 天恢复期
})

def fetch_daily_region(conn, end_date):
    """拉取每日 × 大区 销量"""
    sql = f"""
    SELECT stat_date AS ds, {REGION_MAP} AS region, SUM(goods_qty) AS y
    FROM sales_goods_summary
    WHERE department='offline' AND stat_date <= %s AND {CATE_FILTER}
    GROUP BY stat_date, region HAVING region IS NOT NULL
    ORDER BY stat_date, region
    """
    df = pd.read_sql(sql, conn, params=(end_date,))
    df['ds'] = pd.to_datetime(df['ds'])
    df['y'] = df['y'].astype(float)
    return df

def fetch_actual_monthly(conn, year_month):
    """拉指定月份各大区实际销量"""
    # 改 BETWEEN 范围, 避开 DATE_FORMAT 里 % 跟 mysql.connector pyformat 占位符冲突
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

def forecast_region(daily_df, region, predict_start, predict_end):
    """对单个大区跑 Prophet,返回预测期间合计"""
    df = daily_df[daily_df['region'] == region][['ds', 'y']].copy()
    if len(df) < 60:
        return None
    # 月级 / 周级聚合 — Prophet 日级 OK,但月级数据少
    m = Prophet(
        yearly_seasonality=True,
        weekly_seasonality=False,
        daily_seasonality=False,
        holidays=SPRING_FESTIVAL,
        changepoint_prior_scale=0.05,
    )
    m.fit(df)
    future = pd.DataFrame({'ds': pd.date_range(predict_start, predict_end, freq='D')})
    fc = m.predict(future)
    return float(fc['yhat'].sum())

def main():
    conn = mysql.connector.connect(**DB)
    results = []
    for ym, predict_start, predict_end, train_end in [
        ('2026-01', '2026-01-01', '2026-01-31', '2025-12-31'),
        ('2026-02', '2026-02-01', '2026-02-28', '2026-01-31'),
        ('2026-03', '2026-03-01', '2026-03-31', '2026-02-28'),
        ('2026-04', '2026-04-01', '2026-04-30', '2026-03-31'),
    ]:
        daily = fetch_daily_region(conn, train_end)
        actual = fetch_actual_monthly(conn, ym)
        for region in ['华北大区', '华东大区', '华中大区', '华南大区', '西南大区',
                       '西北大区', '东北大区', '山东大区', '重客']:
            fc = forecast_region(daily, region, predict_start, predict_end)
            results.append({
                'ym': ym,
                'region': region,
                'prophet_forecast': round(fc) if fc else None,
                'actual': round(actual.get(region, 0)),
            })
            print(f"  {ym} {region}: Prophet={round(fc) if fc else 'N/A':>8} vs 实际={round(actual.get(region, 0)):>8}")
    df = pd.DataFrame(results)
    df['err_pct'] = (df['prophet_forecast'] - df['actual']) / df['actual'] * 100
    df['err_pct'] = df['err_pct'].round(1)
    print('\n=== 汇总 ===')
    print(df.to_string(index=False))
    # 月级合计
    summary = df.groupby('ym').agg(prophet=('prophet_forecast','sum'), actual=('actual','sum'))
    summary['err_pct'] = ((summary['prophet']-summary['actual'])/summary['actual']*100).round(1)
    print('\n=== 月级汇总 ===')
    print(summary.to_string())
    conn.close()

if __name__ == '__main__':
    main()
