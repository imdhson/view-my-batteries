
## 2024-05-18 - Dynamic Tab Titles & Inline Error Clearing
**Learning:** We discovered that real-time multi-device apps frequently cause users to open multiple tabs for different rooms, making tab identification difficult. Also, relying strictly on submit events for clearing validation states (`aria-invalid`) feels slow and leaves users stuck with persistent error indicators.
**Action:** Next time we have dynamic "rooms" or "channels" that determine the primary focus of a screen, proactively update `document.title` to reflect this state so multitasking is easier. Additionally, immediately clear inline form validation states (e.g. `aria-invalid`) on `input` events rather than waiting for the next submission attempt.
## 2026-10-08 - Improved light mode color contrast
**Learning:** Functional colors on translucent glass backgrounds in light mode can suffer from poor contrast if not intentionally darkened. Default functional tones that work on dark themes or solid backgrounds may fail WCAG contrast on glass overlays.
**Action:** When designing or adjusting themes using glassmorphism, ensure functional colors (like accent, success, warning, error) are mapped to darker variants in the `prefers-color-scheme: light` block to maintain optimal readability against frosted backgrounds.
