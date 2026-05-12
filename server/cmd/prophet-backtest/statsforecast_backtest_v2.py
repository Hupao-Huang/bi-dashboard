"""
StatsForecast 销量预测回测 v2 — 月级聚合 + season_length=12 (年度月周期)
关键调优: 日级 -> 月级聚合, 周级周期 -> 年度月周期, 减少噪声 + 捕捉年同期模式
"""
import mysql.connector
import pandas as pd
import numpy as np
from statsforecast import StatsForecast
from statsforecast.models import AutoARIMA, AutoETS, AutoTheta
import warnings
warnings.filterwarnings('ignore')
import logging
logging.getLogger('statsforecast').setLevel(logging.WARNING)

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


def fetch_monthly_region(conn, end_date):
    """月级聚合 - SELECT yr/mo 然后 Python 拼日期, 避开 ONLY_FULL_GROUP_BY"""
    sql = f"""
    SELECT YEAR(stat_date) AS yr, MONTH(stat_date) AS mo,
           {REGION_MAP} AS region, SUM(goods_qty) AS y
    FROM sales_goods_summary
    WHERE department='offline' AND stat_date <= %s AND {CATE_FILTER}
    GROUP BY YEAR(stat_date), MONTH(stat_date), region HAVING region IS NOT NULL
    ORDER BY yr, mo, region
    """
    df = pd.read_sql(sql, conn, params=(end_date,))
    df['ds'] = pd.to_datetime(df['yr'].astype(str) + '-' + df['mo'].astype(str).str.zfill(2) + '-01')
    df['y'] = df['y'].astype(float)
    return df[['ds', 'region', 'y']]


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


def statsforecast_predict_monthly(monthly_df, predict_ym):
    """月级 StatsForecast, 预测下一个月. 返回 dict[region]=该月预测值"""
    df = monthly_df.rename(columns={'region': 'unique_id'})[['unique_id', 'ds', 'y']]
    # 过滤数据太少的大区
    counts = df.groupby('unique_id').size()
    valid_uids = counts[counts >= 12].index  # 至少 12 个月历史
    df = df[df['unique_id'].isin(valid_uids)]
    if df.empty:
        return {}
    # 月级聚合 + season_length=12 (年度月周期)
    sf = StatsForecast(
        models=[
            AutoARIMA(season_length=12),
            AutoETS(season_length=12),
            AutoTheta(season_length=12),
        ],
        freq='MS',  # Month Start
        n_jobs=1,
    )
    sf.fit(df)
    fc = sf.predict(h=1)  # 预测下一个月
    fc['mean'] = fc[['AutoARIMA', 'AutoETS', 'AutoTheta']].mean(axis=1)
    grouped = fc.groupby('unique_id')['mean'].sum().to_dict()
    return grouped


def main():
    conn = mysql.connector.connect(**DB)
    results = []
    for ym, train_end in [
        ('2026-01', '2025-12-31'),
        ('2026-02', '2026-01-31'),
        ('2026-03', '2026-02-28'),
        ('2026-04', '2026-03-31'),
    ]:
        print(f"\n== {ym} (训练截至 {train_end}) ==")
        monthly = fetch_monthly_region(conn, train_end)
        actual = fetch_actual_monthly(conn, ym)
        forecasts = statsforecast_predict_monthly(monthly, ym)
        for region in ['华北大区', '华东大区', '华中大区', '华南大区', '西南大区',
                       '西北大区', '东北大区', '山东大区', '重客']:
            fc = forecasts.get(region)
            a = round(actual.get(region, 0))
            results.append({
                'ym': ym,
                'region': region,
                'sf_forecast': round(fc) if fc else None,
                'actual': a,
            })
            print(f"  {ym} {region}: SF={round(fc) if fc else 'N/A':>8} vs 实际={a:>8}")
    df = pd.DataFrame(results)
    df['err_pct'] = ((df['sf_forecast'] - df['actual']) / df['actual'] * 100).round(1)
    print('\n=== 汇总 (SKU×大区) ===')
    print(df.to_string(index=False))
    summary = df.groupby('ym').agg(sf=('sf_forecast', 'sum'), actual=('actual', 'sum'))
    summary['err_pct'] = ((summary['sf'] - summary['actual']) / summary['actual'] * 100).round(1)
    print('\n=== 月级汇总 (大区合计) ===')
    print(summary.to_string())
    # MAPE
    mape = summary['err_pct'].abs().mean()
    print(f"\n=== 平均绝对误差 (MAPE): {mape:.1f}% ===")
    conn.close()


if __name__ == '__main__':
    main()
