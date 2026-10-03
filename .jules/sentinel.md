## 2024-10-03 - Missing Input Length Limits (DoS Risk) in JSON Decoding
**Vulnerability:** Unbounded JSON payload reading via `json.NewDecoder(r.Body)` in `/api/leave` endpoint.
**Learning:** Even for simple requests like "leave room", if the payload size is unbounded, an attacker can send a large request body to consume memory, resulting in a Denial of Service.
**Prevention:** Always wrap `r.Body` with `http.MaxBytesReader` before passing it to `json.NewDecoder` or `io.ReadAll`, setting a reasonable limit (e.g. `1<<10` or 1KB for short payloads).
