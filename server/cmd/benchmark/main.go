package main

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var client = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        500,
		MaxIdleConnsPerHost: 500,
		MaxConnsPerHost:     500,
		IdleConnTimeout:     90 * time.Second,
	},
}

// 简易压测工具
// 用法: benchmark.exe <url> <concurrency> <total>
// 例: benchmark.exe "http://127.0.0.1:8080/api/overview?start=2026-04-01&end=2026-04-21" 100 1000
func main() {
	if len(os.Args) < 4 {
		fmt.Println("用法: benchmark.exe <url> <concurrency> <total>")
		os.Exit(1)
	}
	url := os.Args[1]
	concurrency := parseInt(os.Args[2], 100)
	total := parseInt(os.Args[3], 1000)

	cookie := os.Getenv("COOKIE")

	fmt.Printf("压测: %s\n并发: %d, 总请求: %d\n\n", url, concurrency, total)

	var wg sync.WaitGroup
	latencies := make([]time.Duration, 0, total)
	var mu sync.Mutex
	var success, failed int64
	ch := make(chan struct{}, concurrency)
	start := time.Now()

	for i := 0; i < total; i++ {
		wg.Add(1)
		ch <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-ch }()
			req, _ := http.NewRequest("GET", url, nil)
			if cookie != "" {
				req.Header.Set("Cookie", cookie)
			}
			t0 := time.Now()
			resp, err := client.Do(req)
			dur := time.Since(t0)
			if err != nil {
				atomic.AddInt64(&failed, 1)
				return
			}
			resp.Body.Close()
			if resp.StatusCode >= 400 {
				atomic.AddInt64(&failed, 1)
			} else {
				atomic.AddInt64(&success, 1)
			}
			mu.Lock()
			latencies = append(latencies, dur)
			mu.Unlock()
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	pct := func(p float64) time.Duration {
		if len(latencies) == 0 {
			return 0
		}
		idx := int(float64(len(latencies)) * p / 100)
		if idx >= len(latencies) {
			idx = len(latencies) - 1
		}
		return latencies[idx]
	}
	qps := float64(success) / elapsed.Seconds()

	fmt.Printf("完成: %v\n", elapsed)
	fmt.Printf("QPS: %.1f\n", qps)
	fmt.Printf("成功: %d, 失败: %d\n", success, failed)
	fmt.Printf("P50: %v\nP90: %v\nP99: %v\n最大: %v\n",
		pct(50), pct(90), pct(99), latencies[len(latencies)-1])
}

func parseInt(s string, def int) int {
	v := 0
	_, err := fmt.Sscanf(s, "%d", &v)
	if err != nil {
		return def
	}
	return v
}
