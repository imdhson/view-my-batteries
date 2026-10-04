## 2024-10-03 - Floating Point Precision with Math.floor()
**Learning:** Using `Math.floor(x * 100)` for percentages like `0.29 * 100` results in `28` because `0.29 * 100` in Javascript evaluates to `28.999999999999996`.
**Action:** Use `Math.round(x * 100)` or add an epsilon `Math.floor(x * 100 + 0.1)` instead of blindly flooring when converting decimal fractions to integers in JavaScript.

## 2024-10-04 - RequestAnimationFrame DOM Operations
**Learning:** Querying the DOM (e.g., `document.querySelectorAll`) and writing to the DOM (e.g., setting `.textContent` or `.style.width`) inside a `requestAnimationFrame` loop that runs 60 times a second can cause significant performance bottlenecks and layout thrashing, especially when the values haven't actually changed.
**Action:** Always cache DOM elements outside of the animation loop (e.g., update the cache only when the DOM structure changes), and compare calculated values against a cached `lastVal` to ensure DOM writes only happen when strictly necessary.
