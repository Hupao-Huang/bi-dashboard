package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/jackyun"
	"encoding/json"
	"fmt"
	"log"
)

func main() {
	cfg, _ := config.Load("config.json")
	client := jackyun.NewClient(cfg.JackYun.AppKey, cfg.JackYun.Secret, cfg.JackYun.APIURL)
	biz := map[string]interface{}{"pageIndex": 0, "pageSize": 1, "goodsNo": "03030091"}
	resp, err := client.Call("erp.storage.goodslist", biz)
	if err != nil {
		log.Fatal(err)
	}
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
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
	json.Unmarshal(dataBytes, &result)
	if len(result.GoodsList) > 0 {
		b, _ := json.MarshalIndent(result.GoodsList[0], "", "  ")
		fmt.Println(string(b))
	}
}
