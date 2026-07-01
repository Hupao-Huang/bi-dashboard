package main

// channelPlatform 渠道 → 平台(照销售日报版式的三大平台分组)
var channelPlatform = map[string]string{
	"抖音": "社媒", "视频小店": "社媒", "小红书": "社媒", "快手": "社媒",
	"拼多多": "电商", "天猫": "电商", "京东": "电商", "唯品会": "电商",
	"分销": "其他", "私域": "其他", "线下": "其他", "新零售": "其他", "其它": "其他",
}

// platformOf 渠道→平台,未知渠道归「其他」
func platformOf(channel string) string {
	if p, ok := channelPlatform[channel]; ok {
		return p
	}
	return "其他"
}
