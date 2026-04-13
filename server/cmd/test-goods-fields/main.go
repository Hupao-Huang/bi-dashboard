package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/jackyun"
	"encoding/json"
	"fmt"
	"log"
	"sort"
)

func main() {
	cfg, _ := config.Load("config.json")
	client := jackyun.NewClient(cfg.JackYun.AppKey, cfg.JackYun.Secret, cfg.JackYun.APIURL)
	biz := map[string]interface{}{"pageIndex": 0, "pageSize": 1}
	resp, err := client.Call("erp.storage.goodslist", biz)
	if err != nil {
		log.Fatal(err)
	}
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	fmt.Printf("code=%d msg=%s\n", resp.Code, resp.Msg)
	json.Unmarshal(resp.Result, &wrapper)
	var dataStr string
	var dataBytes []byte
	if err := json.Unmarshal(wrapper.Data, &dataStr); err == nil {
		dataBytes = []byte(dataStr)
	} else {
		dataBytes = wrapper.Data
	}
	var result struct {
		GoodsList []map[string]interface{} `json:"goodsList"`
	}
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		fmt.Println("raw data:", string(dataBytes[:500]))
		log.Fatal(err)
	}
	fmt.Printf("total items: %d\n", len(result.GoodsList))
	if len(result.GoodsList) > 0 {
		g := result.GoodsList[0]
		keys := make([]string, 0, len(g))
		for k := range g {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := g[k]
			if v != nil && fmt.Sprintf("%v", v) != "" && fmt.Sprintf("%v", v) != "0" && fmt.Sprintf("%v", v) != "<nil>" {
				fmt.Printf("%-30s = %v\n", k, v)
			}
		}
	}
}
