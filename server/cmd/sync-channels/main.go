package main

import (
	"bi-dashboard/internal/config"
	"bi-dashboard/internal/importutil"
	"bi-dashboard/internal/jackyun"
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

// 渠道分类 -> BI看板部门映射
// 私域归社媒，后续如需调整改这里即可
var cateToDepartment = map[string]string{
	"电商":   "ecommerce",
	"即时零售": "ecommerce",    // 即时零售归电商
	"社媒":   "social",
	"私域":   "social",       // 私域算社媒
	"分销":   "distribution",
	"线下":   "offline",
	// TODO: "品牌中心"和无分类的渠道暂未归类，需要跑哥后续确认
}

func mapDepartment(cateName string) string {
	for key, dept := range cateToDepartment {
		if strings.Contains(cateName, key) {
			return dept
		}
	}
	return "" // 未匹配的暂不归类
}

func main() {
	unlock := importutil.AcquireLock("sync-channels")
	defer unlock()

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := sql.Open("mysql", cfg.Database.DSN())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	client := jackyun.NewClient(cfg.JackYun.AppKey, cfg.JackYun.Secret, cfg.JackYun.APIURL)

	fmt.Println("正在从吉客云拉取销售渠道...")

	total := 0
	inserted := 0
	updated := 0
	unmapped := 0

	err = client.FetchChannels(func(channels []jackyun.Channel) error {
		for _, ch := range channels {
			total++
			channelId := ch.ChannelId.String()
			channelType := ch.ChannelType.String()
			typeName := jackyun.ChannelTypeName[channelType]
			department := mapDepartment(ch.CateName)

			if department == "" {
				unmapped++
				fmt.Printf("  [未映射] %s (%s) 分类=%s\n", ch.ChannelName, channelId, ch.CateName)
			}

			chargeType := sql.NullInt64{}
			if ch.ChargeType.String() != "" {
				v, _ := ch.ChargeType.Int64()
				chargeType = sql.NullInt64{Int64: v, Valid: true}
			}

			result, err := db.Exec(`
				INSERT INTO sales_channel
					(channel_id, channel_code, channel_name, channel_type, channel_type_name,
					 online_plat_code, online_plat_name, channel_depart_id, channel_depart_name,
					 cate_id, cate_name, company_id, company_name, company_code, depart_code,
					 warehouse_code, warehouse_name, link_man, link_tel, memo,
					 plat_shop_id, plat_shop_name, responsible_user, department,
					 charge_type, email, group_id, office_address, postcode,
					 city_id, city_name, country_id, country_name,
					 province_id, province_name, street_id, street_name,
					 town_id, town_name,
					 field1, field2, field3, field4, field5,
					 field6, field7, field8, field9, field10,
					 field11, field12, field13, field14, field15,
					 field16, field17, field18, field19, field20,
					 field21, field22, field23, field24, field25,
					 field26, field27, field28, field29, field30)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
						?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
						?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON DUPLICATE KEY UPDATE
					channel_code=VALUES(channel_code), channel_name=VALUES(channel_name),
					channel_type=VALUES(channel_type), channel_type_name=VALUES(channel_type_name),
					online_plat_code=VALUES(online_plat_code), online_plat_name=VALUES(online_plat_name),
					channel_depart_id=VALUES(channel_depart_id), channel_depart_name=VALUES(channel_depart_name),
					cate_id=VALUES(cate_id), cate_name=VALUES(cate_name),
					company_id=VALUES(company_id), company_name=VALUES(company_name),
					company_code=VALUES(company_code), depart_code=VALUES(depart_code),
					warehouse_code=VALUES(warehouse_code), warehouse_name=VALUES(warehouse_name),
					link_man=VALUES(link_man), link_tel=VALUES(link_tel), memo=VALUES(memo),
					plat_shop_id=VALUES(plat_shop_id), plat_shop_name=VALUES(plat_shop_name),
					responsible_user=VALUES(responsible_user), department=VALUES(department),
					charge_type=VALUES(charge_type), email=VALUES(email),
					group_id=VALUES(group_id), office_address=VALUES(office_address),
					postcode=VALUES(postcode), city_id=VALUES(city_id), city_name=VALUES(city_name),
					country_id=VALUES(country_id), country_name=VALUES(country_name),
					province_id=VALUES(province_id), province_name=VALUES(province_name),
					street_id=VALUES(street_id), street_name=VALUES(street_name),
					town_id=VALUES(town_id), town_name=VALUES(town_name),
					field1=VALUES(field1), field2=VALUES(field2), field3=VALUES(field3),
					field4=VALUES(field4), field5=VALUES(field5), field6=VALUES(field6),
					field7=VALUES(field7), field8=VALUES(field8), field9=VALUES(field9),
					field10=VALUES(field10), field11=VALUES(field11), field12=VALUES(field12),
					field13=VALUES(field13), field14=VALUES(field14), field15=VALUES(field15),
					field16=VALUES(field16), field17=VALUES(field17), field18=VALUES(field18),
					field19=VALUES(field19), field20=VALUES(field20), field21=VALUES(field21),
					field22=VALUES(field22), field23=VALUES(field23), field24=VALUES(field24),
					field25=VALUES(field25), field26=VALUES(field26), field27=VALUES(field27),
					field28=VALUES(field28), field29=VALUES(field29), field30=VALUES(field30)`,
				channelId, ch.ChannelCode, ch.ChannelName, channelType, typeName,
				ch.OnlinePlatTypeCode, ch.OnlinePlatTypeName,
				ch.ChannelDepartId.String(), ch.ChannelDepartName,
				ch.CateId.String(), ch.CateName,
				ch.CompanyId.String(), ch.CompanyName, ch.CompanyCode, ch.DepartCode,
				ch.WarehouseCode, ch.WarehouseName,
				ch.LinkMan, ch.LinkTel, ch.Memo,
				ch.PlatShopId, ch.PlatShopName, ch.ResponsibleUserName, department,
				chargeType, ch.Email, ch.GroupId.String(), ch.OfficeAddress, ch.Postcode,
				ch.CityId.String(), ch.CityName, ch.CountryId.String(), ch.CountryName,
				ch.ProvinceId.String(), ch.ProvinceName, ch.StreetId.String(), ch.StreetName,
				ch.TownId.String(), ch.TownName,
				ch.Field1, ch.Field2, ch.Field3, ch.Field4, ch.Field5,
				ch.Field6, ch.Field7, ch.Field8, ch.Field9, ch.Field10,
				ch.Field11, ch.Field12, ch.Field13, ch.Field14, ch.Field15,
				ch.Field16, ch.Field17, ch.Field18, ch.Field19, ch.Field20,
				ch.Field21, ch.Field22, ch.Field23, ch.Field24, ch.Field25,
				ch.Field26, ch.Field27, ch.Field28, ch.Field29, ch.Field30,
			)
			if err != nil {
				return fmt.Errorf("insert channel %s: %w", channelId, err)
			}

			rows, _ := result.RowsAffected()
			if rows == 1 {
				inserted++
			} else if rows == 2 { // ON DUPLICATE KEY UPDATE counts as 2
				updated++
			}
		}
		return nil
	})

	if err != nil {
		log.Fatalf("同步失败: %v", err)
	}

	fmt.Printf("\n同步完成！共 %d 个渠道，新增 %d，更新 %d，未映射 %d\n", total, inserted, updated, unmapped)

	// 打印映射统计
	rows, err := db.Query(`
		SELECT IFNULL(department,'未映射') as dept, COUNT(*) as cnt
		FROM sales_channel GROUP BY department ORDER BY cnt DESC`)
	if err == nil {
		defer rows.Close()
		fmt.Println("\n部门映射统计：")
		for rows.Next() {
			var dept string
			var cnt int
			rows.Scan(&dept, &cnt)
			label := dept
			if dept == "ecommerce" {
				label = "电商部门"
			} else if dept == "social" {
				label = "社媒部门"
			} else if dept == "distribution" {
				label = "分销部门"
			} else if dept == "offline" {
				label = "线下部门"
			}
			fmt.Printf("  %s (%s): %d 个渠道\n", label, dept, cnt)
		}
	}
}
