#!/usr/bin/env python
"""一气呵成: 拉委外订单全量 → 扫字段 → 生成 ys_subcontract_orders CREATE TABLE + fields.go

用法: python gen_ys_subcontract.py
输出:
  - server/sql/ys_subcontract_create.sql
  - server/cmd/sync-yonsuite-subcontract/fields.go (208 字段风格)
"""
import hmac, hashlib, base64, time, requests, json, re
from collections import defaultdict
from urllib.parse import quote
import os

APPKEY = '87d4f181d550426db299890a7abb6ddd'
SECRET = '7944bc4fc4b2cd2f79ea972140a1078c19c21e09'
BASE = 'https://c3.yonyoucloud.com'

# 中文 COMMENT 映射 (按字段名规则推断)
COMMENT_RULES = [
    (r'^code$', '委外订单号'), (r'^id$', '订单主表id'),
    (r'^vouchdate$', '单据日期'), (r'^createTime$', '创建时间'),
    (r'^pubts$', '时间戳'), (r'^creator$', '创建人'),
    (r'^auditDate$', '审核日期'), (r'^auditTime$', '审核时间'), (r'^auditor$', '审核人'),
    (r'^modifyTime$', '修改时间'), (r'^modifier$', '修改人'),
    (r'^closeTime$', '关闭时间'), (r'^closer$', '关闭人'),
    (r'^status$', '订单状态(0开立/1已审核/2已关闭/3审核中/4已锁定/5已开工)'),
    (r'^bizstatus$', '流程状态'), (r'^arriveStatus$', '到货状态'),
    (r'^OrderProduct_id$', '订单行id'),
    (r'^OrderProduct_materialCode$', '制造物料编码(成品)'),
    (r'^OrderProduct_materialName$', '制造物料名称(成品)'),
    (r'^OrderProduct_materialId$', '制造物料id'),
    (r'^OrderProduct_bomId$', 'BOM ID(关联物料清单)'),
    (r'^OrderProduct_subcontractQuantityMU$', '委外数量(主单位)'),
    (r'^OrderProduct_subcontractQuantityPU$', '委外数量(计价单位)'),
    (r'^OrderProduct_subcontractQuantitySU$', '委外数量(辅单位)'),
    (r'^OrderProduct_deliveryDate$', '⭐ 交货日期(算在途委外何时到关键)'),
    (r'^OrderProduct_mainUnitName$', '主单位名称'),
    (r'^OrderProduct_priceUnitName$', '计价单位名称'),
    (r'^OrderProduct_subcontractUnitName$', '委外单位名称'),
    (r'^OrderProduct_productId$', '物料id(辅料/包材)'),
    (r'^OrderProduct_lineClose$', '行关闭'),
    (r'^OrderProduct_isHold$', '是否暂挂'),
    (r'^OrderProduct_changeRate$', '换算率'),
    (r'^OrderProduct_version$', '版本'),
    (r'^subcontractVendor.*', '委外供应商'),
    (r'^vendor.*', '供应商'),
    (r'^transType.*', '交易类型'),
    (r'^org.*', '组织'),
    (r'.*Quantity.*MU$', '主单位数量'),
    (r'.*Quantity.*PU$', '计价单位数量'),
    (r'.*Quantity.*SU$', '辅单位数量'),
    (r'.*Quantity.*$', '数量'),
    (r'.*recipientQuantity$', '应领数量'),
    (r'.*auxiliary.*Quantity.*', '辅料数量'),
    (r'.*Money$|.*Sum$|.*Tax$|.*Amount.*', '金额/税额'),
    (r'.*Price.*', '单价'),
    (r'.*Rate$', '汇率/费率'),
    (r'.*Date$', '日期'), (r'.*Time$', '时间'),
    (r'^is[A-Z].*', '是否标记'), (r'^has[A-Z].*', '是否有'),
    (r'^bmake.*', '流程标记'),
    (r'.*Code$', '编码'), (r'.*Name$', '名称'),
    (r'.*_name$', '名称'), (r'.*_code$', '编码'),
    (r'.*Id$|.*_id$', 'ID'),
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
    if not types:
        return 'VARCHAR(64)'
    if 'json' in types:
        return 'JSON'
    if 'bool' in types and len(types) == 1:
        return 'TINYINT(1)'
    if types & {'int', 'float'}:
        if info['has_decimal']:
            return 'DECIMAL(28,8)' if info['max_abs'] > 1e9 else 'DECIMAL(20,8)'
        return 'BIGINT' if info['max_abs'] > 2_147_483_647 else 'INT'
    if 'str' in types:
        L = info['max_str_len']
        if any(re.match(r'\d{4}-\d{2}-\d{2}', s) for s in info['samples']):
            return 'DATETIME' if any(':' in s for s in info['samples']) else 'DATE'
        if L <= 16:
            return 'VARCHAR(64)'
        if L <= 64:
            return 'VARCHAR(128)'
        if L <= 200:
            return 'VARCHAR(255)'
        return 'VARCHAR(1000)' if L <= 500 else 'TEXT'
    return 'VARCHAR(255)'


def col_to_getter(col_type):
    t = col_type.upper()
    if t.startswith('DECIMAL'):
        return 'getFloat'
    if t == 'TINYINT(1)':
        return 'getBool'
    if t in ('BIGINT', 'INT'):
        return 'getInt64' if t == 'BIGINT' else 'getInt'
    if t in ('DATETIME', 'DATE'):
        return 'getTime'
    if t == 'JSON':
        return 'getJSON'
    return 'getStr'


def fetch_all_subcontract():
    """拉所有委外订单"""
    ts = str(int(time.time() * 1000))
    sig_str = f'appKey{APPKEY}timestamp{ts}'
    sig = base64.b64encode(hmac.new(SECRET.encode(), sig_str.encode(), hashlib.sha256).digest()).decode()
    auth_url = f'{BASE}/iuap-api-auth/open-auth/selfAppAuth/base/v1/getAccessToken?appKey={APPKEY}&timestamp={ts}&signature={quote(sig)}'
    token = requests.get(auth_url, timeout=30).json()['data']['access_token']
    api = f'{BASE}/iuap-api-gateway/yonbip/mfg/subcontractorder/list?access_token={quote(token)}'

    all_recs = []
    page = 1
    while True:
        body = {"pageIndex": page, "pageSize": 500, "isShowMaterial": False}
        r = requests.post(api, json=body, headers={"Content-Type": "application/json"}, timeout=60).json()
        rl = r.get('data', {}).get('recordList') or []
        if not rl:
            break
        all_recs.extend(rl)
        rc = r.get('data', {}).get('recordCount', 0)
        print(f'page {page}: got {len(rl)} (total recordCount={rc}, accumulated {len(all_recs)})')
        if len(rl) < 500:
            break
        page += 1
        time.sleep(1.2)
    return all_recs


def main():
    recs = fetch_all_subcontract()
    print(f'\n=== 总抓取 {len(recs)} 条 ===')

    fields = defaultdict(lambda: {'types': set(), 'max_str_len': 0, 'has_decimal': False,
                                  'samples': [], 'max_abs': 0})
    for rec in recs:
        for k, v in rec.items():
            f = fields[k]
            if v is None:
                f['types'].add('null')
                continue
            if isinstance(v, bool):
                f['types'].add('bool')
            elif isinstance(v, int) and not isinstance(v, bool):
                f['types'].add('int')
                f['max_abs'] = max(f['max_abs'], abs(v))
            elif isinstance(v, float):
                f['types'].add('float')
                f['has_decimal'] = True
                f['max_abs'] = max(f['max_abs'], abs(v))
            elif isinstance(v, str):
                f['types'].add('str')
                f['max_str_len'] = max(f['max_str_len'], len(v))
            elif isinstance(v, (dict, list)):
                f['types'].add('json')
            if len(f['samples']) < 3:
                sv = str(v)[:40]
                if sv not in f['samples']:
                    f['samples'].append(sv)

    print(f'字段总数: {len(fields)}')

    # 生成 CREATE TABLE
    cols_def = ['pk BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT \'自增主键\'']
    field_list = []
    for orig in sorted(fields.keys()):
        if re.match(r'^item\d+$|^define\d+$', orig):
            continue
        snake = to_snake(orig)
        if snake in ('pk', 'created_at', 'updated_at', 'raw_json'):
            continue
        sql_type = infer_type(orig, fields[orig])
        cmt = get_comment(orig)
        cols_def.append(f"`{snake}` {sql_type} DEFAULT NULL COMMENT '{cmt}'")
        field_list.append((snake, orig, sql_type))

    cols_def.append('raw_json JSON DEFAULT NULL COMMENT \'完整 record 原始 JSON 备份\'')
    cols_def.append('created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT \'入库时间\'')
    cols_def.append('updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT \'更新时间\'')

    # UK: id 是主表订单, OrderProduct_id 是行 id
    has_op_id = 'order_product_id' in [f[0] for f in field_list]
    if has_op_id:
        uk = 'UNIQUE KEY uk_id_line (id, order_product_id)'
    else:
        uk = 'UNIQUE KEY uk_id (id)'
    cols_def.append(uk)
    cols_def.append('KEY idx_vouchdate (vouchdate)')
    cols_def.append('KEY idx_code (code)')
    cols_def.append('KEY idx_status (status)')

    create_sql = (
        'CREATE TABLE IF NOT EXISTS ys_subcontract_orders (\n  '
        + ',\n  '.join(cols_def)
        + "\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='YS用友委外订单(订单行级)';\n"
    )
    with open(r'C:\Users\Administrator\bi-dashboard\server\sql\ys_subcontract_create.sql', 'w', encoding='utf-8') as f:
        f.write(create_sql)
    print(f'写入 ys_subcontract_create.sql ({len(field_list)} 业务字段, has_op_id={has_op_id})')

    # 生成 fields.go
    lines = ['// Code generated by gen_ys_subcontract.py - DO NOT EDIT']
    lines.append('package main')
    lines.append('')
    lines.append('var ysSubcontractFields = []ysField{')
    for snake, orig, sql_type in field_list:
        lines.append(f'\t{{"{snake}", "{orig}", {col_to_getter(sql_type)}}},')
    lines.append('}')
    out_path = r'C:\Users\Administrator\bi-dashboard\server\cmd\sync-yonsuite-subcontract\fields.go'
    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    with open(out_path, 'w', encoding='utf-8') as f:
        f.write('\n'.join(lines) + '\n')
    print(f'写入 fields.go ({len(field_list)} 字段)')


if __name__ == '__main__':
    main()
