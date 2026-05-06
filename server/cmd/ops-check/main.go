package main

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "os"
    "sort"

    _ "github.com/go-sql-driver/mysql"
)

type Config struct {
    Database struct {
        Host     string `json:"host"`
        Port     int    `json:"port"`
        User     string `json:"user"`
        Password string `json:"password"`
        DBName   string `json:"dbname"`
    } `json:"database"`
}

func main() {
    b, err := os.ReadFile("config.json")
    if err != nil { panic(err) }
    var cfg Config
    if err := json.Unmarshal(b, &cfg); err != nil { panic(err) }

    dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
        cfg.Database.User, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName)
    db, err := sql.Open("mysql", dsn)
    if err != nil { panic(err) }
    defer db.Close()

    rows, err := db.Query(`
        SELECT table_name
        FROM information_schema.tables
        WHERE table_schema = ? AND table_name LIKE 'op\\_%'
        ORDER BY table_name
    `, cfg.Database.DBName)
    if err != nil { panic(err) }
    defer rows.Close()

    var tables []string
    for rows.Next() {
        var t string
        if err := rows.Scan(&t); err != nil { panic(err) }
        tables = append(tables, t)
    }
    sort.Strings(tables)

    fmt.Println("table|max_date|rows_on_max|shops_on_max")
    for _, t := range tables {
        var hasStatDate int
        err := db.QueryRow(`
            SELECT COUNT(*)
            FROM information_schema.columns
            WHERE table_schema=? AND table_name=? AND column_name='stat_date'
        `, cfg.Database.DBName, t).Scan(&hasStatDate)
        if err != nil { panic(err) }
        if hasStatDate == 0 {
            continue
        }

        var maxDate sql.NullString
        q1 := fmt.Sprintf("SELECT DATE_FORMAT(MAX(stat_date), '%%Y-%%m-%%d') FROM %s", t)
        if err := db.QueryRow(q1).Scan(&maxDate); err != nil {
            fmt.Printf("%s|ERR:%v|0|0\n", t, err)
            continue
        }

        if !maxDate.Valid || maxDate.String == "" {
            fmt.Printf("%s|NULL|0|0\n", t)
            continue
        }

        var cnt int
        q2 := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE stat_date = ?", t)
        if err := db.QueryRow(q2, maxDate.String).Scan(&cnt); err != nil {
            fmt.Printf("%s|%s|ERR:%v|0\n", t, maxDate.String, err)
            continue
        }

        var hasShop int
        if err := db.QueryRow(`
            SELECT COUNT(*)
            FROM information_schema.columns
            WHERE table_schema=? AND table_name=? AND column_name='shop_name'
        `, cfg.Database.DBName, t).Scan(&hasShop); err != nil { panic(err) }

        shops := 0
        if hasShop > 0 {
            q3 := fmt.Sprintf("SELECT COUNT(DISTINCT shop_name) FROM %s WHERE stat_date = ?", t)
            if err := db.QueryRow(q3, maxDate.String).Scan(&shops); err != nil {
                fmt.Printf("%s|%s|%d|ERR:%v\n", t, maxDate.String, cnt, err)
                continue
            }
        }

        fmt.Printf("%s|%s|%d|%d\n", t, maxDate.String, cnt, shops)
    }
}
