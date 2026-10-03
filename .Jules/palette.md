## 2024-10-03 - Smooth Percentage CSS Fix
**Learning:** Using `transition: width 0.5s` on a bar while updating it constantly with JavaScript in a `requestAnimationFrame` loop causes extreme jitter as CSS fights the JS.
**Action:** Remove the CSS transition property on elements whose state is being manually driven at 60fps by Javascript interpolation logic. Additionally, apply `font-variant-numeric: tabular-nums` to numbers updating rapidly to prevent layout shifts.
