## 2024-10-03 - Missing Input Length Limits (DoS Risk) in JSON Decoding
**Vulnerability:** Unbounded JSON payload reading via `json.NewDecoder(r.Body)` in `/api/leave` endpoint.
**Learning:** Even for simple requests like "leave room", if the payload size is unbounded, an attacker can send a large request body to consume memory, resulting in a Denial of Service.
**Prevention:** Always wrap `r.Body` with `http.MaxBytesReader` before passing it to `json.NewDecoder` or `io.ReadAll`, setting a reasonable limit (e.g. `1<<10` or 1KB for short payloads).

## 2026-10-04 - [Missing Default HTTP Security Headers]
**Vulnerability:** Standard Go `net/http` servers do not set essential HTTP security headers like `X-Content-Type-Options`, `X-Frame-Options`, or `X-XSS-Protection` by default. This leaves the application susceptible to MIME-type sniffing, Clickjacking, and cross-site scripting attacks on older browsers.
**Learning:** This repo lacked basic defense-in-depth protection because standard Go web apps rely entirely on manual configuration of middleware to insert required HTTP security headers.
**Prevention:** Always implement a security middleware in any Go standard library `http.Server` to append baseline HTTP security headers (`nosniff`, `DENY` etc.) wrapping the main request multiplexer.
