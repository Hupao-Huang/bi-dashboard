package main

import (
  "fmt"
  "strings"
  "github.com/xuri/excelize/v2"
)

func headerMapData(header, data []string) map[string]string {
  m := map[string]string{}
  for i, h := range header {
    key := strings.TrimSpace(h)
    if key == "" { continue }
    val := ""
    if i < len(data) { val = strings.TrimSpace(data[i]) }
    m[key] = val
  }
  return m
}

func main(){
  path := `Z:\信息部\RPA_集团数据看板\抖音\2026\20260409\松鲜鲜官方旗舰店\抖音_20260409_松鲜鲜官方旗舰店_飞鸽_客服表现.xlsx`
  f, err := excelize.OpenFile(path)
  if err != nil { panic(err) }
  defer f.Close()
  rows, _ := f.GetRows(f.GetSheetName(0))
  fmt.Println("rows", len(rows))
  for i:=0; i<len(rows) && i<8; i++ {
    r := rows[i]
    label := ""
    if len(r)>1 { label = r[1] }
    fmt.Printf("row %d len=%d col0=%q col1=%q\n", i, len(r), func() string { if len(r)>0 { return r[0] }; return "" }(), label)
  }
  header := rows[0]
  data := rows[1]
  avg := rows[1]
  for i:=1; i<len(rows); i++ {
    r := rows[i]
    if len(r)>1 {
      l := strings.TrimSpace(r[1])
      if strings.Contains(l, "店铺汇总值") { data = r }
      if strings.Contains(l, "店铺平均值") { avg = r }
    }
  }
  m := headerMapData(header, data)
  a := headerMapData(header, avg)
  fmt.Println("summary 全天首响时长=", m["全天首响时长"], "全天平响时长=", m["全天平响时长"], "询单转化率=", m["询单转化率"])
  fmt.Println("avg 全天首响时长=", a["全天首响时长"], "全天平响时长=", a["全天平响时长"], "询单转化率=", a["询单转化率"])
}
