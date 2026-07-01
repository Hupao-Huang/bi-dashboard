package main

import (
	"strings"
	"testing"

	"bi-dashboard/internal/jackyun"
)

func TestParseOrderTypes(t *testing.T) {
	t.Setenv("SYNC_ORDER_TYPES", "")

	got := parseOrderTypes("1, 2,2,,8")
	want := []string{"1", "2", "8"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("parseOrderTypes flag = %v, want %v", got, want)
	}
}

func TestParseOrderTypesUsesEnvAndDefault(t *testing.T) {
	t.Setenv("SYNC_ORDER_TYPES", "9,10")
	got := parseOrderTypes("")
	if strings.Join(got, ",") != "9,10" {
		t.Fatalf("parseOrderTypes env = %v, want [9 10]", got)
	}

	t.Setenv("SYNC_ORDER_TYPES", "")
	got = parseOrderTypes("")
	if strings.Join(got, ",") != "1,2,7,8,9,10,12" {
		t.Fatalf("parseOrderTypes default = %v", got)
	}
}

func TestInsertPlaceholderCountMatchesArgs(t *testing.T) {
	item := jackyun.SalesSummaryItem{}
	args := summaryInsertArgs("2026-06-30", "8", item, "社媒部门")
	placeholderCount := strings.Count(insertSQL(), "?")
	if placeholderCount != len(args) {
		t.Fatalf("insertSQL placeholders=%d args=%d", placeholderCount, len(args))
	}
}

func TestSummaryInsertArgsUsesRequestedOrderTypeWhenAPIOmitsIt(t *testing.T) {
	args := summaryInsertArgs("2026-06-30", "12", jackyun.SalesSummaryItem{}, "")
	if got := args[37]; got != "12" {
		t.Fatalf("trade_order_type arg = %v, want 12", got)
	}
}

func TestSummaryInsertArgsKeepsRequestedOrderType(t *testing.T) {
	item := jackyun.SalesSummaryItem{TradeOrderType: jackyun.FlexString("99")}
	args := summaryInsertArgs("2026-06-30", "8", item, "")
	if got := args[37]; got != "8" {
		t.Fatalf("trade_order_type arg = %v, want requested type 8", got)
	}
}
