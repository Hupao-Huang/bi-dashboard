package main

import "testing"

func TestPlatformOf(t *testing.T) {
	cases := map[string]string{
		"抖音": "社媒", "视频小店": "社媒", "小红书": "社媒", "快手": "社媒",
		"拼多多": "电商", "天猫": "电商", "京东": "电商", "唯品会": "电商",
		"分销": "其他", "私域": "其他", "线下": "其他", "新零售": "其他", "其它": "其他",
		"没见过的渠道": "其他",
	}
	for in, want := range cases {
		if got := platformOf(in); got != want {
			t.Errorf("platformOf(%q)=%q want %q", in, got, want)
		}
	}
}
