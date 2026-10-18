
## 2024-05-18 - Dynamic Tab Titles & Inline Error Clearing
**Learning:** We discovered that real-time multi-device apps frequently cause users to open multiple tabs for different rooms, making tab identification difficult. Also, relying strictly on submit events for clearing validation states (`aria-invalid`) feels slow and leaves users stuck with persistent error indicators.
**Action:** Next time we have dynamic "rooms" or "channels" that determine the primary focus of a screen, proactively update `document.title` to reflect this state so multitasking is easier. Additionally, immediately clear inline form validation states (e.g. `aria-invalid`) on `input` events rather than waiting for the next submission attempt.
## 2026-10-08 - Improved light mode color contrast
**Learning:** Functional colors on translucent glass backgrounds in light mode can suffer from poor contrast if not intentionally darkened. Default functional tones that work on dark themes or solid backgrounds may fail WCAG contrast on glass overlays.
**Action:** When designing or adjusting themes using glassmorphism, ensure functional colors (like accent, success, warning, error) are mapped to darker variants in the `prefers-color-scheme: light` block to maintain optimal readability against frosted backgrounds.
## 2024-10-24 - Redundant ARIA Labels on Visual Indicators
**Learning:** When a visual indicator (like a colored dot) is placed directly next to descriptive text (like "실시간 연결됨"), adding a `role="status"` and `aria-label` to the indicator causes screen readers to read the status twice.
**Action:** Use `aria-hidden="true"` on visual indicators that accompany visible text to prevent redundant screen reader announcements. Add `aria-label` only when the indicator is the *sole* representation of the status.
## 2024-10-10 - Preserving Keyboard Focus During Polled DOM Replacements
**Learning:** When using polled Server-Sent Events (SSE) or WebSockets to re-render portions of the DOM continuously (e.g. `innerHTML` replacement), keyboard users lose focus if the active element is destroyed and recreated, severely breaking keyboard navigation accessibility.
**Action:** Before replacing DOM structures containing interactive elements that might have focus (like `<summary>`), temporarily cache an identifier for the currently focused element (using `document.activeElement`). After the DOM replacement completes, query for the newly created identical element and restore its focus utilizing `.focus({ preventScroll: true })` to maintain uninterrupted keyboard accessibility and prevent confusing scroll-jumping behaviors.

## 2024-10-24 - Screen Reader Spam from Rapid Polling DOM Updates
**Learning:** For rapidly changing percentage indicators like live battery bars, screen readers will continuously announce text-content changes which causes severe spam and prevents reading other content. Exposing `.level-pct` raw text creates unmanageable auditory noise, whereas emojis used decoratively can add to that noise if not explicitly hidden (e.g., reading "두 눈" or "플러그").
**Action:** Use `role="progressbar"` with `aria-valuenow` (and optionally update it dynamically with `setAttribute` in requestAnimationFrame) for continuous ranges, and explicitly hide noisy decorative text/emojis and the text-content of the percentage indicator using `aria-hidden="true"`.

## 2026-10-12 - Hide decorative asterisks from screen readers
**Learning:** Decorative asterisks denoting required fields in forms can cause auditory clutter ("별표" or "asterisk") for screen reader users when announcing the input label. They should be explicitly hidden.
**Action:** Add `aria-hidden="true"` to visual required indicators (`*`) within labels, ensuring the input itself correctly relies on `aria-required="true"` or the `required` attribute for accessibility.
## 2024-05-24 - Room ID Sharing and Loading UX
**Learning:** The lack of an optimistic loading state leaves users wondering if connection is working, and the absence of a copy button creates friction in a multi-device tool.
**Action:** Added an immediate visual loading state before SSE connects and a clipboard copy button with clear accessible confirmation feedback.
## 2024-11-20 - Disable Autocorrect on ID Fields for Mobile UX
**Learning:** Mobile keyboards often aggressively autocapitalize the first letter or autocorrect user inputs. For exact string identifiers (like Room IDs), this behavior creates significant friction, causing users to accidentally join the wrong room or receive validation errors.
**Action:** When creating text inputs that function as IDs, tokens, or usernames, always apply `autocapitalize="off"`, `autocorrect="off"`, and `spellcheck="false"` to prevent unwanted mobile keyboard interventions.
## 2024-11-20 - Synchronize Tooltips on Dynamic Icon Buttons
**Learning:** When dynamically updating the visual state of an icon-only button (like a "Copy" icon temporarily changing to a "Copied!" checkmark), updating only the `aria-label` attribute leaves sighted users seeing stale tooltips on hover, reducing clarity.
**Action:** Always synchronize the `title` attribute alongside the `aria-label` attribute on dynamic icon-only buttons to ensure native browser tooltips accurately reflect the updated state for sighted users.
