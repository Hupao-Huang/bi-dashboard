#!/usr/bin/env python
"""生成 ys_purchase_orders ALTER TABLE SQL + field mapping (v0.45.2 字段全量补齐)"""
import pymysql, json, re
from collections import defaultdict

COMMENT_MAP = {
    'amount_payable': '应付金额', 'audit_date': '审核日期', 'audit_time': '审核时间', 'auditor': '审核人',
    'batchno': '批次号', 'bmake_pu_arrivalorder': '流程到货单标记', 'body_parallel': '行并发字段(JSON)',
    'close_is_collaboration': '关账是否协同', 'close_time': '关账时间', 'closer': '关账人',
    'department': '部门ID', 'department_name': '部门名称', 'direct_shipment': '直发标记',
    'discount_rate': '折扣率', 'exch_rate': '汇率', 'exch_rate_date': '汇率日期',
    'exch_rate_ops': '汇率操作类型', 'exch_rate_type': '汇率类型', 'exch_rate_type_name': '汇率类型名称',
    'has_payment_schedules': '是否有付款计划', 'head_parallel': '表头并发字段(JSON)',
    'invaliddate': '失效日期', 'is_arrivalplan': '是否到货计划', 'is_despatch': '是否发货',
    'is_end_trade': '是否结束交易', 'is_exe_detailed_reconciliation': '是否执行详细对账',
    'is_feedback': '是否反馈', 'is_line_feedback': '是否行反馈', 'is_max_limit_price': '是否最高限价',
    'is_vmi': '是否VMI库存', 'lineclose_time': '行关账时间', 'linecloser': '行关账人',
    'lineno': '行号', 'listarrival_close': '到货关账', 'listdiscount_tax_type': '扣税类别',
    'listpayment_close': '付款关账', 'listprice_source': '价格来源', 'listticket_collection_close': '收票关账',
    'listwarehousing_close': '入库关账', 'memo': '备注', 'modifier': '修改人', 'modify_time': '修改时间',
    'nat_currency_name': '本币名称', 'nat_tax': '本币税额',
    'operator': '操作员ID', 'operator_name': '操作员名称', 'ori_tax': '原币税额',
    'payment_process': '付款流程', 'phased_invoice': '分期开票', 'print_count': '打印次数',
    'producedate': '生产日期', 'product_pu_type': '物料采购类型',
    'productsku': 'SKU id', 'productsku_c_code': 'SKU编码', 'productsku_c_name': 'SKU名称',
    'productsku_model_description': 'SKU规格说明',
    'purchase_orders_core_order_code': '核心订单编码', 'purchase_orders_firstsource': '首源单据类型',
    'purchase_orders_firstsourceid': '首源单据ID', 'purchase_orders_firstupcode': '首源上游编码',
    'purchase_orders_is_do_logistics': '是否启动物流', 'purchase_orders_is_gift_product': '是否赠品',
    'purchase_orders_is_logistics_related': '是否物流关联',
    'purchase_orders_material_class_code': '物料分类编码', 'purchase_orders_material_class_id': '物料分类ID',
    'purchase_orders_material_class_name': '物料分类名称', 'purchase_orders_memo': '行备注',
    'purchase_orders_nat_money': '本币无税金额', 'purchase_orders_nat_sum': '本币含税金额',
    'purchase_orders_nat_tax': '本币税额', 'purchase_orders_nat_tax_unit_price': '本币含税单价',
    'purchase_orders_nat_unit_price': '本币无税单价', 'purchase_orders_payment_stauts': '付款状态',
    'purchase_orders_price_qty': '计价数量', 'purchase_orders_pur_uom_code': '采购单位编码',
    'purchase_orders_pur_uom_name': '采购单位名称', 'purchase_orders_reserveid': '保留ID',
    'purchase_orders_source': '源单据类型(行级)', 'purchase_orders_sub_qty': '采购数量(辅单位)',
    'purchase_orders_total_confirm_in_qty': '累计确认入库数量',
    'purchase_orders_total_confirm_in_subqty': '累计确认入库辅单位数量',
    'purchase_orders_total_in_subqty': '累计入库辅单位数量',
    'purchase_orders_total_invoice_money': '累计开票金额', 'purchase_orders_total_invoice_qty': '累计开票数量',
    'purchase_orders_total_recieve_subqty': '累计到货辅单位数量',
    'purchase_orders_total_return_and_return_in_qty': '累计退货+退入数量',
    'purchase_orders_total_return_and_return_in_sub_qty': '累计退货+退入辅单位数量',
    'purchase_orders_total_return_in_qty': '累计退入数量',
    'purchase_orders_total_return_in_sub_qty': '累计退入辅单位数量',
    'purchase_orders_total_return_qty': '累计退货数量',
    'purchase_orders_total_return_sub_qty': '累计退货辅单位数量',
    'purchase_orders_upcode': '上游单据编码(如入库单CGRK)', 'purchase_orders_warehouse': '仓库ID',
    'purchase_orders_warehouse_name': '仓库名称', 'purchase_orders_weigh_finish': '是否称重完成',
    'recieve_date': '到货日期(算lead_time关键字段)', 'returncount': '退货次数',
    'row_close': '行关闭', 'source': '源单据(表头)', 'source_up_lineno': '上游行号',
    'sourceid': '源单据ID', 'tax_amount_payable': '应付税额', 'tax_rate': '税率', 'taxitems': '税项',
    'total_in_no_tax_money': '累计入库无税金额', 'total_invoice_no_tax_money': '累计开票无税金额',
    'trade_throw_version': '交易抛出版本', 'up_lineno': '上游行号',
    'vendor_correspondingorg': '供应商对应组织', 'verifystate': '审批状态', 'warehouse': '仓库ID(表头)',
}


def to_snake(s):
    s = re.sub(r'([A-Z]+)([A-Z][a-z])', r'\1_\2', s)
    s = re.sub(r'([a-z\d])([A-Z])', r'\1_\2', s)
    return s.lower()


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


def main():
    conn = pymysql.connect(host='127.0.0.1', port=3306, user='root', password='Hch123456',
                           database='bi_dashboard', charset='utf8mb4')
    cur = conn.cursor()
    cur.execute("SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA='bi_dashboard' AND TABLE_NAME='ys_purchase_orders'")
    existing = {r[0] for r in cur.fetchall()}
    cur.execute("SELECT raw_json FROM ys_purchase_orders WHERE raw_json IS NOT NULL")
    records = [json.loads(r[0]) for r in cur.fetchall()]

    fields = defaultdict(lambda: {'types': set(), 'max_str_len': 0, 'has_decimal': False,
                                  'samples': [], 'max_abs': 0})
    for rec in records:
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

    new_cols_sql = []
    field_mapping = {}
    for orig in sorted(fields.keys()):
        if re.match(r'^item\d+$|^define\d+$', orig):
            continue
        snake = to_snake(orig)
        if snake in existing:
            continue
        sql_type = infer_type(orig, fields[orig])
        cmt = COMMENT_MAP.get(snake, snake)
        new_cols_sql.append(f"ADD COLUMN `{snake}` {sql_type} DEFAULT NULL COMMENT '{cmt}'")
        field_mapping[snake] = (orig, sql_type)

    alter = 'ALTER TABLE ys_purchase_orders\n  ' + ',\n  '.join(new_cols_sql) + ';\n'
    with open(r'C:\Users\Administrator\bi-dashboard\server\sql\ys_alter_v0_45_2.sql', 'w', encoding='utf-8') as f:
        f.write(alter)
    print(f'写入 ys_alter_v0_45_2.sql: {len(new_cols_sql)} 列, {len(alter)} 字节')

    with open(r'C:\Users\Administrator\bi-dashboard\server\cmd\sync-yonsuite-purchase\field_mapping.txt', 'w', encoding='utf-8') as f:
        for snake, (orig, t) in sorted(field_mapping.items()):
            f.write(f'{snake}|{orig}|{t}\n')
    print(f'写入 field_mapping.txt: {len(field_mapping)} 行')

    print('\nLead time 关键字段已纳入: recieve_date, audit_date, modify_time, close_time, lineclose_time')


if __name__ == '__main__':
    main()
