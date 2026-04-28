#!/usr/bin/env python
"""一气呵成: 拉材料出库单全量 → 扫字段 → 生成 ys_material_out CREATE TABLE + fields.go"""
import hmac, hashlib, base64, time, requests, json, re
from collections import defaultdict
from urllib.parse import quote
import os

APPKEY = '87d4f181d550426db299890a7abb6ddd'
SECRET = '7944bc4fc4b2cd2f79ea972140a1078c19c21e09'
BASE = 'https://c3.yonyoucloud.com'

COMMENT_RULES = [
    (r'^code$', '单据编号(CLCK 开头)'), (r'^id$', '主表id'),
    (r'^vouchdate$', '单据日期'), (r'^createTime$', '创建时间'),
    (r'^pubts$', '时间戳'), (r'^creator$', '创建人'),
    (r'^auditTime$', '审核时间'), (r'^auditor$', '审核人'),
    (r'^modifyTime$', '修改时间'), (r'^modifier$', '修改人'),
    (r'^bustype_name$', '交易类型(委外发料/销售出库等)'),
    (r'^bustype$', '交易类型id'),
    (r'^stockOrg.*name$', '库存组织名称'),
    (r'^stockOrg.*$', '库存组织'),
    (r'^accountOrg.*name$', '核算组织名称'),
    (r'^accountOrg.*$', '核算组织'),
    (r'^materOuts_id$', '出库行id'),
    (r'^materOuts_product$', '物料id'),
    (r'^materOuts_productsku$', 'SKU id'),
    (r'^materOuts_qty$', '出库数量(主单位)'),
    (r'^materOuts_subQty$', '出库数量(辅单位)'),
    (r'^materOuts_(.+)$', r'\1'),
    (r'^product_cCode$', '物料编码'),
    (r'^product_cName$', '物料名称'),
    (r'^warehouse_name$', '仓库名称'),
    (r'^warehouse$', '仓库id'),
    (r'^batchno$', '批次号'),
    (r'^upcode.*', '上游单据编号(委外入库单 WWRK)'),
    (r'^firstsource.*', '首源单据'),
    (r'^source.*', '源单据'),
    (r'^bodyItem$', '行自定义项(JSON)'),
    (r'^bodyParallel$', '行并发字段(JSON)'),
    (r'^headParallel$', '表头并发字段(JSON)'),
    (r'^define\d+$', '自定义项'),
    (r'^currency.*name$', '币种名称'),
    (r'^currency.*$', '币种'),
    (r'^nat.*$', '本币'),
    (r'.*Money$|.*Sum$|.*Tax$|.*amount.*', '金额/税额'),
    (r'.*Price.*', '单价'),
    (r'.*Rate$', '汇率/费率'),
    (r'.*Date$', '日期'), (r'.*Time$', '时间'),
    (r'^is[A-Z].*', '是否标记'), (r'^has[A-Z].*', '是否有'),
    (r'^bmake.*', '流程标记'),
    (r'.*Code$', '编码'), (r'.*Name$', '名称'),
    (r'.*_name$', '名称'), (r'.*_code$', '编码'),
    (r'.*Id$|.*_id$', 'ID'),
    (r'.*Status$|.*status$', '状态'),
]


def to_snake(s):
    s = re.sub(r'([A-Z]+)([A-Z][a-z])', r'\1_\2', s)
    s = re.sub(r'([a-z\d])([A-Z])', r'\1_\2', s)
    return s.lower()


def get_comment(orig):
    for pat, cmt in COMMENT_RULES:
        m = re.match(pat, orig)
        if m:
            try:
                return m.expand(cmt) if '\\' in cmt else cmt
            except Exception:
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


def fetch_all_materialout():
    ts = str(int(time.time() * 1000))
    sig_str = f'appKey{APPKEY}timestamp{ts}'
    sig = base64.b64encode(hmac.new(SECRET.encode(), sig_str.encode(), hashlib.sha256).digest()).decode()
    auth_url = f'{BASE}/iuap-api-auth/open-auth/selfAppAuth/base/v1/getAccessToken?appKey={APPKEY}&timestamp={ts}&signature={quote(sig)}'
    token = requests.get(auth_url, timeout=30).json()['data']['access_token']
    api = f'{BASE}/iuap-api-gateway/yonbip/scm/materialout/list?access_token={quote(token)}'
    all_recs = []
    page = 1
    while True:
        body = {"pageIndex": page, "pageSize": 500, "isSum": False}
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
    recs = fetch_all_materialout()
    print(f'\n=== 总抓取 {len(recs)} 条 ===')

    # 看 bustype_name 分布 (区分委外发料/销售出库等)
    from collections import Counter
    bustypes = Counter(r.get('bustype_name', '?') for r in recs)
    print(f'bustype_name 分布: {dict(bustypes)}')

    fields = defaultdict(lambda: {'types': set(), 'max_str_len': 0, 'has_decimal': False,
                                  'samples': [], 'max_abs': 0})
    for rec in recs:
        for k, v in rec.items():
            f = fields[k]
            if v is None:
                f['types'].add('null'); continue
            if isinstance(v, bool):
                f['types'].add('bool')
            elif isinstance(v, int) and not isinstance(v, bool):
                f['types'].add('int'); f['max_abs'] = max(f['max_abs'], abs(v))
            elif isinstance(v, float):
                f['types'].add('float'); f['has_decimal'] = True; f['max_abs'] = max(f['max_abs'], abs(v))
            elif isinstance(v, str):
                f['types'].add('str'); f['max_str_len'] = max(f['max_str_len'], len(v))
            elif isinstance(v, (dict, list)):
                f['types'].add('json')
            if len(f['samples']) < 3:
                sv = str(v)[:40]
                if sv not in f['samples']:
                    f['samples'].append(sv)

    print(f'字段总数: {len(fields)}')

    cols_def = ['pk BIGINT PRIMARY KEY AUTO_INCREMENT COMMENT \'自增主键\'']
    field_list = []
    for orig in sorted(fields.keys()):
        if re.match(r'^item\d+$', orig):
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

    snake_set = {f[0] for f in field_list}
    if 'mater_outs_id' in snake_set:
        uk = 'UNIQUE KEY uk_id_line (id, mater_outs_id)'
    elif 'materouts_id' in snake_set:
        uk = 'UNIQUE KEY uk_id_line (id, materouts_id)'
    else:
        uk = 'UNIQUE KEY uk_id (id)'
    cols_def.append(uk)
    cols_def.append('KEY idx_vouchdate (vouchdate)')
    cols_def.append('KEY idx_code (code)')
    if 'product_c_code' in snake_set:
        cols_def.append('KEY idx_product_c_code (product_c_code)')
    if 'bustype' in snake_set:
        cols_def.append('KEY idx_bustype (bustype)')

    create_sql = (
        'CREATE TABLE IF NOT EXISTS ys_material_out (\n  '
        + ',\n  '.join(cols_def)
        + "\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='YS用友材料出库单(委外发料明细)';\n"
    )
    with open(r'C:\Users\Administrator\bi-dashboard\server\sql\ys_material_out_create.sql', 'w', encoding='utf-8') as f:
        f.write(create_sql)
    print(f'写入 ys_material_out_create.sql ({len(field_list)} 字段, UK: {uk})')

    lines = ['// Code generated by gen_ys_materialout.py - DO NOT EDIT']
    lines.append('package main')
    lines.append('')
    lines.append('var ysMaterialOutFields = []ysField{')
    for snake, orig, sql_type in field_list:
        lines.append(f'\t{{"{snake}", "{orig}", {col_to_getter(sql_type)}}},')
    lines.append('}')
    out_path = r'C:\Users\Administrator\bi-dashboard\server\cmd\sync-yonsuite-materialout\fields.go'
    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    with open(out_path, 'w', encoding='utf-8') as f:
        f.write('\n'.join(lines) + '\n')
    print(f'写入 fields.go ({len(field_list)} 字段)')


if __name__ == '__main__':
    main()
