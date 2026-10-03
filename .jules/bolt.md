## 2024-10-03 - Floating Point Precision with Math.floor()
**Learning:** Using `Math.floor(x * 100)` for percentages like `0.29 * 100` results in `28` because `0.29 * 100` in Javascript evaluates to `28.999999999999996`.
**Action:** Use `Math.round(x * 100)` or add an epsilon `Math.floor(x * 100 + 0.1)` instead of blindly flooring when converting decimal fractions to integers in JavaScript.
