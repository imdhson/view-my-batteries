## 2024-10-03 - Smooth Percentage CSS Fix
**Learning:** Using `transition: width 0.5s` on a bar while updating it constantly with JavaScript in a `requestAnimationFrame` loop causes extreme jitter as CSS fights the JS.
**Action:** Remove the CSS transition property on elements whose state is being manually driven at 60fps by Javascript interpolation logic. Additionally, apply `font-variant-numeric: tabular-nums` to numbers updating rapidly to prevent layout shifts.
## 2026-10-04 - Semantic Forms Improve Accessibility
**Learning:** Custom keydown enter listeners are poor practice compared to wrapping inputs and buttons in semantic `<form>` tags and relying on native `submit` events. Not only does this fix screen reader form submission expectations, it enables robust mobile virtual keyboard features like 'Go' button submissions.
**Action:** Always prefer native semantic HTML `<form>` elements with `submit` listeners instead of attaching individual `keydown` event listeners directly to input fields.
