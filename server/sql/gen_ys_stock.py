#!/usr/bin/env python
"""一气呵成: 拉 YS 现存量全量 → 扫字段 → 生成 ys_stock CREATE TABLE + fields.go"""
import hmac, hashlib, base64, time, requests, json, re
from collections import defaultdict
from urllib.parse import quote
import os

APPKEY = '87d4f181d550426db299890a7abb6ddd'
SECRET = '7944bc4fc4b2cd2f79ea972140a1078c19c21e09'
BASE = 'https://c3.yonyoucloud.com'

COMMENT_RULES = [
    (r'^id$', '现存量行 id (UK)'),
    (r'^product$', '物料 id'),
    (r'^product_code$', '物料编码 (关联 YS)'),
    (r'^product_name$', '物料名称'),
    (r'^product_modelDescription$', '物料规格说明'),
    (r'^product_unitName$', '主单位名称'),
    (r'^product_unitCode$', '主单位编码'),
    (r'^productsku$', 'SKU id'),
    (r'^warehouse$', '仓库 id'),
    (r'^warehouse_name$', '仓库名'),
    (r'^warehouse_code$', '仓库编码'),
    (r'^org$', '组织 id'),
    (r'^org_name$', '组织名'),
    (r'^org_code$', '组织编码'),
    (r'^batchno$', '批次号'),
    (r'^currentqty$', '⭐ 当前库存'),
    (r'^availableqty$', '⭐ 可用库存'),
    (r'^planavailableqty$', '计划可用库存'),
    (r'^stockStatusDoc.*name$', '库存状态名(合格/不合格)'),
    (r'^stockStatusDoc.*code$', '库存状态编码'),
    (r'^stockStatusDoc$', '库存状态 id'),
    (r'^statusType$', '状态类型'),
    (r'^manageClass.*name$', '管理类别名(易耗品/原料等)'),
    (r'^manageClass.*code$', '管理类别编码'),
    (r'^manageClass$', '管理类别 id'),
    (r'^inventoryowner$', '存货所有者'),
    (r'^custodian.*$', '保管人'),
    (r'^unit$', '主计量 id'),
    (r'^reserveid$', '保留 id'),
    (r'^sensitiveUID$', '敏感唯一标识'),
    # 在途/在订/出库各类
    (r'^inorderqty$', '在订量'),
    (r'^innoticeqty$', '到货预报'),
    (r'^outnoticeqty$', '出库预报'),
    (r'^arrivalorder$', '到货订单量'),
    (r'^applyorder$', '申请单量'),
    (r'^purchaseorder$', '采购订单量'),
    (r'^salesorder$', '销售订单量'),
    (r'^delivery$', '发货量'),
    (r'^returnorder$', '退货单量'),
    (r'^materialreq$', '材料需求'),
    (r'^pickingreq$', '领料需求'),
    (r'^retailTrade$', '零售交易'),
    (r'^preretailqty$', '预留零售'),
    (r'^reservedShipping$', '预留发货'),
    (r'^reservedArrival$', '预留到货'),
    (r'^tradeorder$', '交易订单'),
    (r'^tradedelivery$', '交易发货'),
    (r'^poout$', 'PO 出'),
    (r'^poin$', 'PO 入'),
    (r'^posubout$', 'PO 子出'),
    (r'^posubin$', 'PO 子入'),
    (r'^posubarrivalorder$', 'PO 子到货'),
    (r'^pofreport$', 'PO 反馈'),
    (r'^storenotice$', '入库通知'),
    (r'^transferapplyin$', '调拨申请入'),
    (r'^transferapplyout$', '调拨申请出'),
    (r'^morphologyconversionout$', '形态转换出'),
    (r'^aimequipcard$', '目标设备卡'),
]


def to_snake(s):
    s = re.sub(r'([A-Z]+)([A-Z][a-z])', r'\1_\2', s)
    s = re.sub(r'([a-z\d])([A-Z])', r'\1_\2', s)
    return s.lower()


def get_comment(orig):
    for pat, cmt in COMMENT_RULES:
        if re.match(pat, orig):
            return cmt
    return orig


def infer_type(name, info):
    types = info['types'] - {'null'}
    if not types: return 'VARCHAR(64)'
    if 'json' in types: return 'JSON'
    if 'bool' in types and len(types) == 1: return 'TINYINT(1)'
    if types & {'int', 'float'}:
        if info['has_decimal']:
            return 'DECIMAL(28,8)' if info['max_abs'] > 1e9 else 'DECIMAL(20,8)'
        return 'BIGINT' if info['max_abs'] > 2_147_483_647 else 'INT'
    if 'str' in types:
        L = info['max_str_len']
        if any(re.match(r'\d{4}-\d{2}-\d{2}', s) for s in info['samples']):
            return 'DATETIME' if any(':' in s for s in info['samples']) else 'DATE'
        if L <= 16: return 'VARCHAR(64)'
        if L <= 64: return 'VARCHAR(128)'
        if L <= 200: return 'VARCHAR(255)'
        return 'VARCHAR(1000)' if L <= 500 else 'TEXT'
    return 'VARCHAR(255)'


def col_to_getter(col_type):
    t = col_type.upper()
    if t.startswith('DECIMAL'): return 'getFloat'
    if t == 'TINYINT(1)': return 'getBool'
    if t in ('BIGINT', 'INT'): return 'getInt64' if t == 'BIGINT' else 'getInt'
    if t in ('DATETIME', 'DATE'): return 'getTime'
    if t == 'JSON': return 'getJSON'
    return 'getStr'


def fetch_all_stock():
    ts = str(int(time.time() * 1000))
    sig_str = f'appKey{APPKEY}timestamp{ts}'
    sig = base64.b64encode(hmac.new(SECRET.encode(), sig_str.encode(), hashlib.sha256).digest()).decode()
    auth_url = f'{BASE}/iuap-api-auth/open-auth/selfAppAuth/base/v1/getAccessToken?appKey={APPKEY}&timestamp={ts}&signature={quote(sig)}'
    token = requests.get(auth_url, timeout=30).json()['data']['access_token']
    api = f'{BASE}/iuap-api-gateway/yonbip/scm/stock/QueryCurrentStocksByCondition?access_token={quote(token)}'
    all_recs = []
    page = 1
    while True:
        body = {"pageIndex": page, "pageSize": 500}
        r = requests.post(api, json=body, headers={"Content-Type": "application/json"}, timeout=60).json()
        data = r.get('data')
        rl = data if isinstance(data, list) else (data.get('recordList', []) if isinstance(data, dict) else [])
        if not rl:
            break
        all_recs.extend(rl)
        print(f'page {page}: got {len(rl)} (累计 {len(all_recs)})')
        if len(rl) < 500: break
        page += 1
        time.sleep(1.2)
    return all_recs


def main():
    recs = fetch_all_stock()
    print(f'\n=== 总抓取 {len(recs)} 条 ===')
    fields = defaultdict(lambda: {'types': set(), 'max_str_len': 0, 'has_decimal': False, 'samples': [], 'max_abs': 0})
    for rec in recs:
        for k, v in rec.items():
            f = fields[k]
            if v is None: f['types'].add('null'); continue
            if isinstance(v, bool): f['types'].add('bool')
            elif isinstance(v, int) and not isinstance(v, bool):
                f['types'].add('int'); f['max_abs'] = max(f['max_abs'], abs(v))
            elif isinstance(v, float):
                f['types'].add('float'); f['has_decimal'] = True; f['max_abs'] = max(f['max_abs'], abs(v))
            elif isinstance(v, str):
                f['types'].add('str'); f['max_str_len'] = max(f['max_str_len'], len(v))
            elif isinstance(v, (dict, list)): f['types'].add('json')
            if len(f['samples']) < 3:
                sv = str(v)[:40]
                if sv not in f['samples']: f['samples'].append(sv)
    print(f'字段总数: {len(fields)}')

    cols_def = ['pk BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT \'自增主键\'']
    field_list = []
    for orig in sorted(fields.keys()):
        snake = to_snake(orig)
        if snake in ('pk', 'created_at', 'updated_at', 'raw_json'): continue
        sql_type = infer_type(orig, fields[orig])
        cmt = get_comment(orig)
        cols_def.append(f"`{snake}` {sql_type} DEFAULT NULL COMMENT '{cmt}'")
        field_list.append((snake, orig, sql_type))
    cols_def.append('raw_json JSON DEFAULT NULL COMMENT \'完整 record 备份\'')
    cols_def.append('created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP')
    cols_def.append('updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP')
    cols_def.append('UNIQUE KEY uk_id (id)')
    cols_def.append('KEY idx_product_code (product_code)')
    cols_def.append('KEY idx_warehouse (warehouse)')
    cols_def.append('KEY idx_org (org)')
    create_sql = ('CREATE TABLE IF NOT EXISTS ys_stock (\n  ' + ',\n  '.join(cols_def)
                  + "\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='YS用友现存量(行级: product×warehouse×batchno×status)';\n")
    with open(r'C:\Users\Administrator\bi-dashboard\server\sql\ys_stock_create.sql', 'w', encoding='utf-8') as f:
        f.write(create_sql)
    print(f'写入 ys_stock_create.sql ({len(field_list)} 字段)')

    lines = ['// Code generated by gen_ys_stock.py - DO NOT EDIT', 'package main', '',
             'var ysStockFields = []ysField{']
    for snake, orig, sql_type in field_list:
        lines.append(f'\t{{"{snake}", "{orig}", {col_to_getter(sql_type)}}},')
    lines.append('}')
    out_path = r'C:\Users\Administrator\bi-dashboard\server\cmd\sync-yonsuite-stock\fields.go'
    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    with open(out_path, 'w', encoding='utf-8') as f:
        f.write('\n'.join(lines) + '\n')
    print(f'写入 fields.go ({len(field_list)} 字段)')


if __name__ == '__main__':
    main()
