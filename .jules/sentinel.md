## 2024-10-03 - Missing Input Length Limits (DoS Risk) in JSON Decoding
**Vulnerability:** Unbounded JSON payload reading via `json.NewDecoder(r.Body)` in `/api/leave` endpoint.
**Learning:** Even for simple requests like "leave room", if the payload size is unbounded, an attacker can send a large request body to consume memory, resulting in a Denial of Service.
**Prevention:** Always wrap `r.Body` with `http.MaxBytesReader` before passing it to `json.NewDecoder` or `io.ReadAll`, setting a reasonable limit (e.g. `1<<10` or 1KB for short payloads).

## 2026-10-04 - [Missing Default HTTP Security Headers]
**Vulnerability:** Standard Go `net/http` servers do not set essential HTTP security headers like `X-Content-Type-Options`, `X-Frame-Options`, or `X-XSS-Protection` by default. This leaves the application susceptible to MIME-type sniffing, Clickjacking, and cross-site scripting attacks on older browsers.
**Learning:** This repo lacked basic defense-in-depth protection because standard Go web apps rely entirely on manual configuration of middleware to insert required HTTP security headers.
**Prevention:** Always implement a security middleware in any Go standard library `http.Server` to append baseline HTTP security headers (`nosniff`, `DENY` etc.) wrapping the main request multiplexer.
## 2026-10-05 - Add Content Security Policy
**Vulnerability:** Missing Content Security Policy (CSP) headers, allowing potential Cross-Site Scripting (XSS) and unrestricted content loading.
**Learning:** The initial setup used standard security headers like X-XSS-Protection but lacked a robust CSP to restrict external scripts, styles, and connections.
**Prevention:** Enforce a `Content-Security-Policy` header in web applications by default (`default-src 'self'`) and selectively allow required inline styles and scripts if unavoidable (`unsafe-inline`).

## 2026-10-06 - Missing Escaping for Time Strings and Colors in UI
**Vulnerability:** Attributes like `data-time-connected`, `data-time-ago`, and styling like `color` in HTML templates were being directly interpolated without being escaped. Although currently safe due to their expected format and the backend implementation, it leaves an XSS risk if the sources for these become controllable or complex.
**Learning:** Dynamic attributes or values rendered client-side must always be escaped because changes to backend data formatting or time inputs could lead to injection.
**Prevention:** Always wrap all interpolated dynamic variables, including time strings, numbers, and generated strings like colors, in `escapeHtml` or equivalent functions during manual DOM rendering string generation.
## 2024-05-18 - Missing escapeHtml() wrapper around strings and colors injected in HTML
**Vulnerability:** XSS vulnerability where generated strings from helper functions (like `formatDuration` or `formatAgo`) and dynamically chosen values like CSS colors (`color`) were not wrapped in `escapeHtml()` when concatenated into strings and written to the DOM via `innerHTML`.
**Learning:** Even seemingly benign dynamic values (like time representations or CSS var names/colors derived from conditionals) must be escaped to prevent injection, as changes to those functions could introduce malicious inputs that then bypass existing XSS protections.
**Prevention:** Always wrap all interpolated variable expressions in `escapeHtml()`, unless they are hard-coded static strings or explicitly sanitized immediately prior.
## 2024-10-07 - Add missing HTML escaping for duration strings
**Vulnerability:** XSS vulnerability where generated strings from helper functions (like `formatDuration` or `formatAgo`) were not wrapped in `escapeHtml()` when concatenated into strings and written to the DOM.
**Learning:** Even seemingly benign dynamic values (like time representations) must be escaped to prevent injection, as changes to those functions could introduce malicious inputs that bypass existing XSS protections.
**Prevention:** Always wrap all interpolated variable expressions in `escapeHtml()`, unless they are hard-coded static strings or explicitly sanitized immediately prior.

## 2026-10-09 - Missing Content-Type Validation in JSON Endpoints
**Vulnerability:** The JSON API endpoints (`/api/battery`, `/api/leave`) accepted POST requests without validating the `Content-Type` header, making them vulnerable to "Simple Request" Cross-Site Request Forgery (CSRF) via simple HTML forms using `text/plain`.
**Learning:** `json.NewDecoder` parses valid JSON regardless of the declared HTTP `Content-Type`. Standard web forms cannot send cross-origin `application/json` without triggering a CORS preflight, but they can send `text/plain`. If an API blindly accepts it, CSRF is possible even without CORS misconfigurations.
**Prevention:** Always enforce `Content-Type: application/json` on all POST endpoints designed to consume JSON by checking `strings.HasPrefix(r.Header.Get("Content-Type"), "application/json")` to trigger a mandatory CORS preflight for cross-origin requests.

## 2024-10-10 - Fix Goroutine Leak on Empty Rooms (DoS Risk)
**Vulnerability:** A goroutine leak existed in the `broadcaster` function because it continuously blocked listening on `broadcastCh` and wasn't terminated when a room became completely empty, causing memory/CPU consumption for each newly created and then emptied room. Attackers could repeatedly join and leave random rooms to exhaust server resources resulting in a Denial of Service.
**Learning:** In Go, unbounded goroutine creation without a termination mechanism can quickly cause DoS on server applications (specifically WebSockets/SSE handlers that maintain long-lived state). When managing room state in-memory, you must actively track and tear down background tasks when they become unneeded (e.g. rooms becoming empty).
**Prevention:** Ensure SSE broadcast channels are explicitly closed and the associated map keys are deleted when the room size drops to zero, which allows `for range ch` loops to terminate and free resources.

## 2024-10-11 - Missing HTTP Server Timeouts (Slowloris DoS Risk)
**Vulnerability:** The standard Go `http.Server` instance lacked `ReadTimeout` and `IdleTimeout` configurations, making it vulnerable to resource exhaustion from slow clients (Slowloris attacks) keeping connections open indefinitely.
**Learning:** Default Go HTTP servers do not enforce read or idle timeouts. An attacker can send requests extremely slowly (e.g., 1 byte every few seconds) or just hold connections open, exhausting the server's file descriptors and memory. While `ReadHeaderTimeout` was set, it only protects against slow headers, not slow bodies.
**Prevention:** Always configure `ReadTimeout` and `IdleTimeout` on exposed Go HTTP servers. When dealing with SSE (which needs long-lived connections for writing), `ReadTimeout` ensures the request reading phase bounds memory/time, while the connection remains open for writing as long as headers are flushed.
## 2023-10-12 - Strict escaping for numeric dynamic data
**Vulnerability:** XSS risk due to unescaped numeric dynamic data (`pct`) being interpolated into the `aria-valuenow` attribute.
**Learning:** Even data that is typically numeric (like battery percentage) should be strictly escaped when interpolated into HTML templates. A malicious actor could theoretically manipulate the `pct` value if it were derived from external or untrusted sources, leading to an XSS injection.
**Prevention:** Always wrap all dynamic data, including numbers, in `escapeHtml()` when concatenating strings to generate HTML elements or attributes.

## 2024-10-13 - Missing Map Capacity Limits (Resource Exhaustion DoS Risk)
**Vulnerability:** Unbounded map growth in `s.rooms`, `s.presence`, and `s.subs` allowed attackers to exhaust server memory by creating infinite numbers of rooms, devices, or SSE connections, leading to a Denial of Service.
**Learning:** In-memory state tracking for dynamic entities (like chat rooms or live devices) must always have enforced upper bounds. Standard Maps (`map[string]T`) in Go will grow indefinitely until the host crashes due to OOM (Out Of Memory).
**Prevention:** Always implement explicit capacity checks (e.g., `len(s.rooms) >= maxRooms`) before adding new entries to globally shared in-memory collections, and return an appropriate error (e.g., HTTP 503) when capacity is reached.

## 2024-10-14 - Missing Map Capacity Limits per Room (Resource Exhaustion DoS Risk)
**Vulnerability:** Unbounded map growth in `s.rooms[roomID]` allowed attackers to exhaust server memory by continuously adding an infinite number of unique devices to a single room via the `/api/battery` endpoint, leading to a Denial of Service.
**Learning:** While global map capacity limits were in place (e.g., `maxRooms`), nested maps inside global collections (like devices within a room) must also have explicitly enforced upper bounds (`maxDevices`) on insertion. Standard Maps (`map[string]*Device`) in Go will grow indefinitely otherwise.
**Prevention:** Always implement explicit capacity checks (e.g., `if len(room) >= maxDevices`) before adding new entries to globally shared, nested in-memory collections. Return an appropriate failure response when capacity is reached.

## 2023-10-27 - [Server-Sent Events (SSE) Sensitive Data Caching Risk]
**Vulnerability:** The `/api/stream` SSE endpoint used `Cache-Control: no-cache`. While this prevents stale data in normal HTTP requests, browsers may still write chunked SSE streams to the local disk cache when using `no-cache`, potentially exposing sensitive real-time battery/device data locally.
**Learning:** `no-cache` forces revalidation but doesn't strictly prevent storage of the response payload.
**Prevention:** Always use `Cache-Control: no-store` on endpoints serving sensitive real-time streaming data (like SSE) to guarantee chunks are never written to disk by the browser.
