⚡ Stresser
by B-dev

A lightweight and simple HTTP stress-testing CLI tool written in Go.

> ⚠️ Warning: Only test servers you own or have explicit permission to test!




---

✨ Features

Simple CLI interface

URL or positional IP input

Custom HTTP methods (GET, POST, PUT, etc.)

Add custom headers and request body

TLS insecure option for testing self-signed certificates

Rate limiting (requests per second)

Progress output with basic latency stats (min/max/avg)



---

🛠️ Build

You need Go 1.21+ installed.

# Build the binary
go build -o stresser main.go


---

🚀 Usage

# Basic usage with flags
./stresser -url http://localhost:8000 -c 50 -n 1000

# Positional URL (optional)
./stresser http://localhost:8000 -c 50 -n 1000

# IP only (auto prepends http://)
./stresser 192.168.1.50 -c 50 -n 1000

# POST request with body and headers, rate limit 200 req/s
./stresser -url https://api.example.com/submit -X POST -body payload.json -H "Content-Type: application/json" -c 200 -n 5000 -rate 200

# Skip TLS verification (testing only)
./stresser -url https://self-signed.example -insecure -c 50 -n 2000


---

⚙️ Flags

-url  → Target URL (or use positional arg)

-c  → Concurrency (number of goroutines)

-n  → Total requests to send

-X  → HTTP method (GET, POST, PUT...)

-body  → Path to request body file (optional)

-H  → Custom header, repeatable (e.g. -H 'Key: Value')

-timeout  → Timeout per request in seconds

-insecure  → Skip TLS verification (for testing only)

-rate  → Max requests per second (0 = unlimited)



---

📊 Output

Shows progress every 2 seconds:

Sent: 150/1000 | Success: 145 | Fail: 5

And final summary:

=== Summary ===
Total requests: 1000
Time taken: 12.5s
Sent: 1000 | Success: 980 | Fail: 20
Min latency: 15ms
Max latency: 220ms
Avg latency: 48ms


---

💡 Notes

Lightweight tool for learning and small tests.

For production-level load testing, consider k6, vegeta, wrk, or hey.



---

📄 License

MIT

