## 2024-10-03 - Floating Point Precision with Math.floor()
**Learning:** Using `Math.floor(x * 100)` for percentages like `0.29 * 100` results in `28` because `0.29 * 100` in Javascript evaluates to `28.999999999999996`.
**Action:** Use `Math.round(x * 100)` or add an epsilon `Math.floor(x * 100 + 0.1)` instead of blindly flooring when converting decimal fractions to integers in JavaScript.

## 2024-10-04 - RequestAnimationFrame DOM Operations
**Learning:** Querying the DOM (e.g., `document.querySelectorAll`) and writing to the DOM (e.g., setting `.textContent` or `.style.width`) inside a `requestAnimationFrame` loop that runs 60 times a second can cause significant performance bottlenecks and layout thrashing, especially when the values haven't actually changed.
**Action:** Always cache DOM elements outside of the animation loop (e.g., update the cache only when the DOM structure changes), and compare calculated values against a cached `lastVal` to ensure DOM writes only happen when strictly necessary.

## 2024-10-05 - Periodic DOM Re-rendering for Timestamps
**Learning:** Re-rendering an entire DOM tree periodically (e.g., using `setInterval` with `render()`) just to update relative timestamps causes significant layout thrashing and forces complex state management (like tracking which details panels are open).
**Action:** Instead, embed the raw timestamp in `data-*` attributes and use `querySelectorAll` to update only the `textContent` of those specific nodes efficiently.

## 2024-10-25 - Expensive JS Browser API Caching
**Learning:** Calling APIs like `Intl.DateTimeFormat().resolvedOptions().timeZone` and iterating over multiple regexes for `navigator.userAgent` on a timer loop can take several milliseconds per execution, causing performance hiccups on low-end devices.
**Action:** Always lazily evaluate and cache these expensive values (`cachedUA`, `cachedTimezone`) in variables within the Javascript client if they are unlikley to change during the session. Clear the cache only when specific async hints (`navigator.userAgentData.getHighEntropyValues`) resolve.

## 2024-05-24 - Object Allocation in String.replace Callback
**Learning:** Creating object literals inside the callback function of `String.prototype.replace` forces the JavaScript engine to allocate a new object for every single match found. In a rendering function (`render()`) that calls `escapeHtml()` repeatedly on many fields, this creates unnecessary GC thrashing.
**Action:** Always extract static mapping objects outside of the `replace` callback and reuse the same instance, allowing the JS engine to optimize the property lookup without allocations.
## 2024-10-08 - Avoid Inline Callbacks in Frequent Replacements
**Learning:** Inline callback functions passed to `String.prototype.replace` in highly-frequent rendering loops (like `escapeHtml` called for every variable in every DOM update) cause unnecessary object allocations and garbage collection overhead.
**Action:** Extract the callback function alongside the static map to prevent re-allocation on every invocation.
## 2024-10-26 - Keep sorting out of lock critical sections
**Learning:** The entirely in-memory backend uses a single global `sync.Mutex` for all rooms. Keeping O(N log N) sorting logic inside the locked section causes unnecessary contention and could block other incoming connections and status updates.
**Action:** Unlock the mutex as early as possible after copying the state before performing CPU-intensive work like `sort.Slice()`.
## 2024-10-10 - Unnecessary RegExp Extraction
**Learning:** Avoid extracting inline literal regular expressions (e.g., `/[&<>"]/g`) into variables in JavaScript specifically for performance. Modern JavaScript engines (like V8) already perform inline regex literal caching natively, making this extraction a micro-optimization with no measurable impact.
**Action:** Do not optimize inline regular expressions in JavaScript engines unless it can be proven they are recompiled unnecessarily in specific browsers. Look for more impactful backend optimizations first.
