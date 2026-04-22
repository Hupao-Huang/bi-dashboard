"""audit.py - 对每张按 IDX 硬编码索引读取的 op_ 表做抽样核对。
按 shop_name 精确找 RPA 文件。
"""
import os, sys
import openpyxl
import pymysql

DB = dict(host='127.0.0.1', port=3306, user='root', password='Hch123456', database='bi_dashboard', charset='utf8mb4')
RPA = r'Z:\信息部\RPA_集团数据看板'

# (table, RPA-平台, 文件名关键字, db-shop字段名, 店铺匹配方式: exact/contains)
TABLES = [
    ('op_pdd_cs_service_daily', '拼多多', '客服_服务数据', 'shop_name', 'contains'),
    ('op_pdd_cs_sales_daily', '拼多多', '客服_销售数据', 'shop_name', 'contains'),
    ('op_pdd_shop_daily', '拼多多', '店铺销售数据', 'shop_name', 'contains'),
    ('op_pdd_goods_daily', '拼多多', '店铺商品概况', 'shop_name', 'contains'),
    ('op_pdd_service_overview', '拼多多', '店铺服务概况', 'shop_name', 'contains'),
    ('op_pdd_campaign_daily', '拼多多', None, 'shop_name', 'contains'),  # 需要按 promo_type 匹配文件名
    ('op_vip_shop_daily', '唯品会', '销售数据_经营', 'shop_name', 'contains'),
    ('op_vip_cancel', '唯品会', '取消金额', 'shop_name', 'contains'),
    ('op_vip_weixiangke', '唯品会', '唯享客', 'shop_name', 'contains'),
    ('op_jd_customer_daily', '京东', '客户数据_洞察', 'shop_name', 'exact'),
    ('op_jd_promo_sku_daily', '京东', '营销数据_便宜包邮', 'shop_name', 'exact'),
    ('op_jd_promo_daily', '京东', '营销数据', 'shop_name', 'exact'),
    ('op_jd_industry_rank', '京东', '行业_飙升榜', 'shop_name', 'exact'),
    ('op_douyin_funnel_daily', '抖音', '核心漏斗', 'shop_name', 'contains'),
    ('op_douyin_dist_promote_hourly', '抖音分销', '随心推', 'account_name', 'contains'),
    ('op_tmall_member_daily', '天猫', '达摩盘', 'shop_name', 'exact'),
    ('op_tmall_crowd_daily', '天猫', '数据银行', 'shop_name', 'exact'),
    ('op_tmall_service_inquiry', '天猫', '生意参谋_业绩询单', 'shop_name', 'exact'),
    ('op_tmall_service_consult', '天猫', '生意参谋_咨询接待', 'shop_name', 'exact'),
    ('op_tmall_service_avgprice', '天猫', '生意参谋_客单价客服', 'shop_name', 'exact'),
    ('op_tmall_service_evaluation', '天猫', '生意参谋_接待评价', 'shop_name', 'exact'),
    ('op_tmall_cs_shop_daily', '天猫', '生意参谋_店铺销售数据', 'shop_name', 'exact'),
    ('op_tmall_cs_industry_keyword', '天猫', '集客_行业数据', None, None),  # 无 shop_name
    ('op_jd_cs_workload_daily', '京东', '客服_工作量', 'shop_name', 'contains'),
    ('op_jd_cs_sales_perf_daily', '京东', '客服_销售业绩', 'shop_name', 'contains'),
]

def q(conn, sql):
    c = conn.cursor(pymysql.cursors.DictCursor)
    c.execute(sql)
    return c.fetchall()

def find_rpa(platform, pattern, date, shop=None, shop_match='exact'):
    yyyy = date[:4]
    base = os.path.join(RPA, platform, yyyy, date.replace('-', ''))
    if not os.path.isdir(base):
        # 可能没有年份分层
        base = os.path.join(RPA, platform, date.replace('-', ''))
        if not os.path.isdir(base):
            return None, f'日期目录不存在: {base}'
    candidates = []
    for sd in os.listdir(base):
        sp = os.path.join(base, sd)
        if not os.path.isdir(sp):
            continue
        # shop 过滤
        if shop and shop_match == 'exact' and sd != shop:
            continue
        if shop and shop_match == 'contains' and shop not in sd and sd not in shop:
            continue
        for fn in os.listdir(sp):
            if pattern and pattern in fn and (fn.endswith('.xlsx') or fn.endswith('.xls')):
                candidates.append(os.path.join(sp, fn))
    if not candidates:
        return None, f'没匹配文件: pattern={pattern} shop={shop} match={shop_match}'
    return candidates[0], None

def dump_excel(path, max_rows=5):
    try:
        wb = openpyxl.load_workbook(path, data_only=True)
        ws = wb.active
        rows = list(ws.iter_rows(values_only=True, max_row=max_rows))
        return rows
    except Exception as e:
        return [('ERR', str(e))]

def main():
    conn = pymysql.connect(**DB)
    print('# Audit: DB vs Excel for IDX-based import functions\n')
    for tbl, platform, pattern, shop_field, shop_match in TABLES:
        try:
            rows = q(conn, f"SELECT * FROM {tbl} WHERE stat_date BETWEEN '2026-04-01' AND '2026-04-15' ORDER BY stat_date DESC LIMIT 1")
        except Exception as e:
            print(f'## {tbl}  (查询失败: {e})\n')
            continue
        if not rows:
            print(f'## {tbl}  (无 4月 数据)\n')
            continue
        row = rows[0]
        stat_date = str(row.get('stat_date', ''))
        shop = row.get(shop_field) if shop_field else None
        print(f'## {tbl}  stat_date={stat_date}  shop={shop}')
        # 过滤掉大字段打印
        tidy = {k:v for k,v in row.items() if k not in ('id','updated_at')}
        print(f'  DB: {tidy}')
        if pattern is None:
            print(f'  (pattern 未指定，跳过 Excel 比对)\n')
            continue
        rpa, err = find_rpa(platform, pattern, stat_date, shop, shop_match)
        if not rpa:
            print(f'  {err}\n')
            continue
        print(f'  RPA: {os.path.basename(rpa)}')
        excel = dump_excel(rpa, max_rows=6)
        for i, r in enumerate(excel):
            vs = list(r)[:13]
            # 截断长 cell
            vs = [str(v)[:60] if v is not None else '' for v in vs]
            print(f'    row{i}: {vs}')
        print()

if __name__ == '__main__':
    main()
