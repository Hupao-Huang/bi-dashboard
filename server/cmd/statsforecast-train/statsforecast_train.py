"""
StatsForecast 训练脚本 — 用 AutoARIMA + AutoETS + Theta 集成
按大区训练月度销量模型, 输出 UPSERT 到 offline_sales_forecast_statsforecast
"""
import mysql.connector
import pandas as pd
import numpy as np
from statsforecast import StatsForecast
from statsforecast.models import AutoARIMA, AutoETS, AutoTheta
from dateutil.relativedelta import relativedelta
import warnings
warnings.filterwarnings('ignore')
import logging
logging.basicConfig(level=logging.WARNING)

def _bi_cfg():
    """读 server/config.json (与后端 Go 共用一份凭证), walk-up 自动找文件"""
    import os, json
    here = os.path.dirname(os.path.abspath(__file__))
    for up in range(5):
        for cand in [os.path.join(here, *(['..'] * up), 'server', 'config.json'),
                     os.path.join(here, *(['..'] * up), 'config.json')]:
            if os.path.exists(cand):
                return json.load(open(cand, encoding='utf-8'))
    raise RuntimeError('未找到 server/config.json, 请按 server/.env.example 配置凭证')


_d = _bi_cfg()['database']
DB = dict(host=_d['host'], port=_d['port'], user=_d['user'], password=_d['password'], database=_d['dbname'])

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

def fetch_monthly_region(conn):
    sql = f"""
    SELECT ds, unique_id, SUM(y) AS y FROM (
      SELECT DATE_FORMAT(stat_date, '%Y-%m-01') AS ds,
             {REGION_MAP_SQL} AS unique_id,
             goods_qty AS y
      FROM sales_goods_summary
      WHERE department='offline' AND {CATE_FILTER}
    ) t
    WHERE unique_id IS NOT NULL
    GROUP BY ds, unique_id
    ORDER BY ds, unique_id
    """
    df = pd.read_sql(sql, conn)
    df['ds'] = pd.to_datetime(df['ds'])
    df['y'] = df['y'].astype(float)
    return df

def main():
    conn = mysql.connector.connect(**DB)
    print('[StatsForecast] 拉月级历史...')
    df = fetch_monthly_region(conn)
    last_complete_month = df['ds'].max()
    # 排除当月 (数据不全)
    df = df[df['ds'] < last_complete_month]
    print(f'[StatsForecast] 训练数据: {df["ds"].min().date()} ~ {df["ds"].max().date()}, {df["unique_id"].nunique()} 大区, {len(df)} 行')

    # 训练 — AutoARIMA + AutoETS + AutoTheta 集成
    season_length = 12  # 年度季节性
    sf = StatsForecast(
        models=[
            AutoARIMA(season_length=season_length),
            AutoETS(season_length=season_length),
            AutoTheta(season_length=season_length),
        ],
        freq='MS',  # Month Start
        n_jobs=-1,
    )
    print('[StatsForecast] 训练 + 预测 12 个月...')
    forecast = sf.forecast(df=df, h=12, level=[80])
    print(f'[StatsForecast] 预测完成, {len(forecast)} 行')

    # 集成 = 3 个模型预测均值
    forecast['ensemble'] = forecast[['AutoARIMA', 'AutoETS', 'AutoTheta']].mean(axis=1)
    forecast['ym'] = forecast['ds'].dt.strftime('%Y-%m')

    rows = []
    for _, r in forecast.iterrows():
        rows.append(dict(
            ym=r['ym'],
            region=r['unique_id'],
            forecast_qty=max(0, int(round(r['ensemble']))),
            arima_qty=max(0, int(round(r['AutoARIMA']))),
            ets_qty=max(0, int(round(r['AutoETS']))),
            theta_qty=max(0, int(round(r['AutoTheta']))),
        ))

    cur = conn.cursor()
    cur.executemany(
        """INSERT INTO offline_sales_forecast_statsforecast
           (ym, region, forecast_qty, arima_qty, ets_qty, theta_qty)
           VALUES (%(ym)s, %(region)s, %(forecast_qty)s, %(arima_qty)s, %(ets_qty)s, %(theta_qty)s)
           ON DUPLICATE KEY UPDATE
             forecast_qty=VALUES(forecast_qty),
             arima_qty=VALUES(arima_qty),
             ets_qty=VALUES(ets_qty),
             theta_qty=VALUES(theta_qty),
             trained_at=NOW()""",
        rows
    )
    conn.commit()
    print(f'[StatsForecast] UPSERT 完成: {len(rows)} 行')
    cur.close()
    conn.close()

if __name__ == '__main__':
    main()
