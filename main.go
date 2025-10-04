package main

import ( "bufio" "context" "crypto/tls" "flag" "fmt" "io" "net" "net/http" "os" "strings" "sync" "sync/atomic" "time" )

// Simple CLI stresser // Usage examples: //  build: go build -o stresser main.go //  default flags: ./stresser //  use flag: ./stresser -url http://example.com -c 100 -n 1000 //  positional (optional): ./stresser http://example.com -c 100 //  IP only positional (will be used as http://<ip>:80 if no scheme/port): ./stresser 192.0.2.5

var ( urlFlag       string concurrency   int totalRequests int method        string bodyFile      string headerFlags   headerList timeoutSec    int insecure      bool rateLimit     int )

type headerList []string

func (h *headerList) String() string { return fmt.Sprint(*h) } func (h *headerList) Set(value string) error { *h = append(*h, value) return nil }

func init() { flag.StringVar(&urlFlag, "url", "http://localhost:8000", "Target URL (or use positional arg)") flag.IntVar(&concurrency, "c", 50, "Number of concurrent workers (goroutines)") flag.IntVar(&totalRequests, "n", 1000, "Total number of requests to send") flag.StringVar(&method, "X", "GET", "HTTP method to use (GET, POST, PUT, ...)") flag.StringVar(&bodyFile, "body", "", "File path to use as request body (optional)") flag.Var(&headerFlags, "H", "Custom header, can be repeated (e.g. -H 'Key: Value')") flag.IntVar(&timeoutSec, "timeout", 10, "Timeout per request in seconds") flag.BoolVar(&insecure, "insecure", false, "Skip TLS certificate verification (for testing)") flag.IntVar(&rateLimit, "rate", 0, "Max requests per second (0 = unlimited)") }

type Stats struct { totalSent    int64 totalSuccess int64 totalFail    int64 minLatNs     int64 maxLatNs     int64 sumLatNs     int64 }

func main() { flag.Parse()

// Positional override: if user passes an argument, use it as target
posArgs := flag.Args()
if len(posArgs) > 0 {
	// take only first positional arg as URL/IP
	candidate := posArgs[0]
	// if it looks like an IP without scheme, prepend http://
	if strings.HasPrefix(candidate, "http://") || strings.HasPrefix(candidate, "https://") {
		urlFlag = candidate
	} else if strings.Contains(candidate, ":") { // may contain port (ip:port)
		urlFlag = "http://" + candidate
	} else if net.ParseIP(candidate) != nil {
		// bare IP
		urlFlag = "http://" + candidate
	} else {
		// assume hostname without scheme
		urlFlag = "http://" + candidate
	}
}

if totalRequests <= 0 {
	fmt.Println("n must be > 0")
	return
}
if concurrency <= 0 {
	concurrency = 1
}

var body []byte
var err error
if bodyFile != "" {
	body, err = os.ReadFile(bodyFile)
	if err != nil {
		fmt.Printf("Failed to read body file: %v\n", err)
		return
	}
}

// Prepare headers
headers := make(http.Header)
for _, h := range headerFlags {
	parts := strings.SplitN(h, ":", 2)
	if len(parts) == 2 {
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		headers.Add(key, val)
	}
}

tr := &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   time.Duration(timeoutSec) * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          1000,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	TLSClientConfig:       &tls.Config{InsecureSkipVerify: insecure},
}

client := &http.Client{
	Transport: tr,
	Timeout:   time.Duration(timeoutSec) * time.Second,
}

fmt.Printf("Target: %s | Method: %s | Concurrency: %d | Total requests: %d\n", urlFlag, method, concurrency, totalRequests)
if len(headers) > 0 {
	fmt.Println("Custom headers:")
	for k, v := range headers {
		fmt.Printf("  %s: %s\n", k, strings.Join(v, ", "))
	}
}
if rateLimit > 0 {
	fmt.Printf("Rate limit: %d req/s\n", rateLimit)
}

jobs := make(chan int, totalRequests)
var wg sync.WaitGroup

stats := &Stats{minLatNs: int64(1<<63 - 1)}

var limiter <-chan time.Time
if rateLimit > 0 {
	interval := time.Duration(1e9 / rateLimit)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	limiter = ticker.C
}

wg.Add(concurrency)
for i := 0; i < concurrency; i++ {
	go func(id int) {
		defer wg.Done()
		for range jobs {
			if limiter != nil {
				<-limiter
			}
			sendRequest(client, method, urlFlag, headers, body, stats)
		}
	}(i)
}

startAll := time.Now()
for i := 0; i < totalRequests; i++ {
	jobs <- i
}
close(jobs)

// Progress
go func() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for range t.C {
		sent := atomic.LoadInt64(&stats.totalSent)
		fmt.Printf("\rSent: %d/%d | Success: %d | Fail: %d", sent, totalRequests, atomic.LoadInt64(&stats.totalSuccess), atomic.LoadInt64(&stats.totalFail))
		if sent >= int64(totalRequests) {
			return
		}
	}
}()

wg.Wait()
elapsed := time.Since(startAll)

// Summary
fmt.Println("\n\n=== Summary ===")
fmt.Printf("Total requests: %d\n", totalRequests)
fmt.Printf("Time taken: %v\n", elapsed)
sent := atomic.LoadInt64(&stats.totalSent)
success := atomic.LoadInt64(&stats.totalSuccess)
fail := atomic.LoadInt64(&stats.totalFail)
fmt.Printf("Sent: %d | Success: %d | Fail: %d\n", sent, success, fail)
if success > 0 {
	avgNs := atomic.LoadInt64(&stats.sumLatNs) / success
	fmt.Printf("Min latency: %v\n", time.Duration(atomic.LoadInt64(&stats.minLatNs)))
	fmt.Printf("Max latency: %v\n", time.Duration(atomic.LoadInt64(&stats.maxLatNs)))
	fmt.Printf("Avg latency: %v\n", time.Duration(avgNs))
}

}

func sendRequest(client *http.Client, method, url string, headers http.Header, body []byte, stats *Stats) { atomic.AddInt64(&stats.totalSent, 1) ctx, cancel := context.WithTimeout(context.Background(), client.Timeout) defer cancel()

reqBody := io.NopCloser(strings.NewReader(""))
if len(body) > 0 {
	reqBody = io.NopCloser(strings.NewReader(string(body)))
}
req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
if err != nil {
	atomic.AddInt64(&stats.totalFail, 1)
	return
}
for k, vv := range headers {
	for _, v := range vv {
		req.Header.Add(k, v)
	}
}

start := time.Now()
resp, err := client.Do(req)
lat := time.Since(start)
updateLatency(stats, lat)

if err != nil {
	atomic.AddInt64(&stats.totalFail, 1)
	return
}
_, _ = io.Copy(io.Discard, bufio.NewReader(resp.Body))
resp.Body.Close()

if resp.StatusCode >= 200 && resp.StatusCode < 400 {
	atomic.AddInt64(&stats.totalSuccess, 1)
} else {
	atomic.AddInt64(&stats.totalFail, 1)
}

}

func updateLatency(stats *Stats, d time.Duration) { ns := d.Nanoseconds() atomic.AddInt64(&stats.sumLatNs, ns) for { oldMin := atomic.LoadInt64(&stats.minLatNs) if ns >= oldMin { break } if atomic.CompareAndSwapInt64(&stats.minLatNs, oldMin, ns) { break } } for { oldMax := atomic.LoadInt64(&stats.maxLatNs) if ns <= oldMax { break } if atomic.CompareAndSwapInt64(&stats.maxLatNs, oldMax, ns) { break } } }

