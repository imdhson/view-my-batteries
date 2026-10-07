## 2024-10-03 - Smooth Percentage CSS Fix
**Learning:** Using `transition: width 0.5s` on a bar while updating it constantly with JavaScript in a `requestAnimationFrame` loop causes extreme jitter as CSS fights the JS.
**Action:** Remove the CSS transition property on elements whose state is being manually driven at 60fps by Javascript interpolation logic. Additionally, apply `font-variant-numeric: tabular-nums` to numbers updating rapidly to prevent layout shifts.
## 2026-10-04 - Semantic Forms Improve Accessibility
**Learning:** Custom keydown enter listeners are poor practice compared to wrapping inputs and buttons in semantic `<form>` tags and relying on native `submit` events. Not only does this fix screen reader form submission expectations, it enables robust mobile virtual keyboard features like 'Go' button submissions.
**Action:** Always prefer native semantic HTML `<form>` elements with `submit` listeners instead of attaching individual `keydown` event listeners directly to input fields.
## 2026-10-05 - Native Summary Element Focus Styling
**Learning:** Native `<summary>` elements inside `<details>` accordions often lose their default focus visibility when placed inside highly stylized containers, making keyboard navigation difficult for accessibility users. They also default to a slightly unpolished focus box.
**Action:** When using `<summary>`, explicitly define `:focus-visible` styles (sharing styles with buttons or inputs) and add a small border-radius for visual polish to ensure keyboard users can clearly see the focus state.
## 2024-10-06 - Light Mode Contrast with Glass UI
**Learning:** Bright functional colors (like #3ddc84 for green or #ffc247 for yellow) that look great on a dark background become completely illegible when placed over a luminous, white-based "glass" background in light mode, violating accessibility contrast guidelines.
**Action:** When implementing a light mode theme with a translucent glass background, explicitly define darker variants for functional colors (accent, green, yellow, red) inside the `@media (prefers-color-scheme: light)` block to maintain sufficient text contrast.
