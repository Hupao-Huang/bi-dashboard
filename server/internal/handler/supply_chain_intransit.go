package handler

import (
	"log"
	"net/http"
	"strings"
)

func (h *DashboardHandler) GetInTransitDetail(w http.ResponseWriter, r *http.Request) {
	if writeScopeError(w, requireDomainAccess(r, "supply_chain")) {
		return
	}
	goodsNo := strings.TrimSpace(r.URL.Query().Get("goodsNo"))
	if goodsNo == "" {
		writeError(w, http.StatusBadRequest, "goodsNo required")
		return
	}

	type purchaseOrder struct {
		Code         string  `json:"code"`         // YS 订单号
		VendorName   string  `json:"vendorName"`   // 供应商
		OrgName      string  `json:"orgName"`      // 组织
		VouchDate    string  `json:"vouchDate"`    // 开单日期
		ArriveDate   string  `json:"arriveDate"`   // 计划到货日期
		TotalQty     float64 `json:"totalQty"`     // 订单总量
		IncomingQty  float64 `json:"incomingQty"`  // 已入库
		InTransitQty float64 `json:"inTransitQty"` // 未入库(在途)
		StatusText   string  `json:"statusText"`   // 业务大白话状态
	}

	type subcontractOrder struct {
		Code         string  `json:"code"`
		VendorName   string  `json:"vendorName"`
		OrgName      string  `json:"orgName"`
		VouchDate    string  `json:"vouchDate"`
		ArriveDate   string  `json:"arriveDate"`
		TotalQty     float64 `json:"totalQty"`
		IncomingQty  float64 `json:"incomingQty"`
		InTransitQty float64 `json:"inTransitQty"`
		StatusText   string  `json:"statusText"`
	}

	purchaseOrders := []purchaseOrder{}
	subcontractOrders := []subcontractOrder{}

	// 1. 在途采购订单 (双路径桥接)
	purchaseSQL := `SELECT
		p.code,
		IFNULL(p.vendor_name, '') AS vendor_name,
		IFNULL(p.org_name, '') AS org_name,
		IFNULL(DATE_FORMAT(p.vouchdate, '%Y-%m-%d'), '') AS vouch_date,
		IFNULL(DATE_FORMAT(p.recieve_date, '%Y-%m-%d'), '') AS arrive_date,
		IFNULL(p.qty, 0) AS total_qty,
		IFNULL(p.total_in_qty, 0) AS incoming_qty,
		IFNULL(p.qty, 0) - IFNULL(p.total_in_qty, 0) AS in_transit_qty,
		CASE p.purchase_orders_in_wh_status
		  WHEN 2 THEN '已审核未入库'
		  WHEN 3 THEN '部分入库'
		  ELSE CONCAT('状态码', p.purchase_orders_in_wh_status)
		END AS status_text
		FROM ys_purchase_orders p
		WHERE p.purchase_orders_in_wh_status IN (2,3)
		  AND p.qty > IFNULL(p.total_in_qty, 0)
		  AND p.org_name != '安徽香松自然调味品有限公司'
		  AND (
		    p.product_c_code IN (SELECT sku_code FROM goods WHERE goods_no = ? AND sku_code IS NOT NULL AND sku_code != '')
		    OR p.product_c_code = ?
		  )
		ORDER BY p.vouchdate DESC, p.code`
	if rows, err := h.DB.Query(purchaseSQL, goodsNo, goodsNo); err == nil {
		defer rows.Close()
		for rows.Next() {
			var o purchaseOrder
			if err := rows.Scan(&o.Code, &o.VendorName, &o.OrgName, &o.VouchDate, &o.ArriveDate,
				&o.TotalQty, &o.IncomingQty, &o.InTransitQty, &o.StatusText); err == nil {
				purchaseOrders = append(purchaseOrders, o)
			}
		}
	} else {
		log.Printf("in-transit purchase query err: %v", err)
	}

	// 2. 在途委外订单 (双路径桥接)
	subcontractSQL := `SELECT
		s.code,
		IFNULL(s.subcontract_vendor_name, '') AS vendor_name,
		IFNULL(s.org_name, '') AS org_name,
		IFNULL(DATE_FORMAT(s.vouchdate, '%Y-%m-%d'), '') AS vouch_date,
		IFNULL(DATE_FORMAT(s.order_product_delivery_date, '%Y-%m-%d'), '') AS arrive_date,
		IFNULL(s.order_product_subcontract_quantity_mu, 0) AS total_qty,
		IFNULL(s.order_product_incoming_quantity, 0) AS incoming_qty,
		IFNULL(s.order_product_subcontract_quantity_mu, 0) - IFNULL(s.order_product_incoming_quantity, 0) AS in_transit_qty,
		CASE s.status
		  WHEN 0 THEN '草稿'
		  WHEN 1 THEN '已审核未入库'
		  WHEN 3 THEN '部分入库'
		  WHEN 4 THEN '已完成'
		  ELSE CONCAT('状态码', s.status)
		END AS status_text
		FROM ys_subcontract_orders s
		WHERE s.status NOT IN (2)
		  AND s.order_product_subcontract_quantity_mu > IFNULL(s.order_product_incoming_quantity, 0)
		  AND s.org_name != '安徽香松自然调味品有限公司'
		  AND (
		    s.order_product_material_code IN (SELECT sku_code FROM goods WHERE goods_no = ? AND sku_code IS NOT NULL AND sku_code != '')
		    OR s.order_product_material_code = ?
		  )
		ORDER BY s.vouchdate DESC, s.code`
	if rows, err := h.DB.Query(subcontractSQL, goodsNo, goodsNo); err == nil {
		defer rows.Close()
		for rows.Next() {
			var o subcontractOrder
			if err := rows.Scan(&o.Code, &o.VendorName, &o.OrgName, &o.VouchDate, &o.ArriveDate,
				&o.TotalQty, &o.IncomingQty, &o.InTransitQty, &o.StatusText); err == nil {
				subcontractOrders = append(subcontractOrders, o)
			}
		}
	} else {
		log.Printf("in-transit subcontract query err: %v", err)
	}

	writeJSON(w, map[string]interface{}{
		"goodsNo":           goodsNo,
		"purchaseOrders":    purchaseOrders,
		"subcontractOrders": subcontractOrders,
	})
}
