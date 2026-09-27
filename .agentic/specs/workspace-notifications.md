# Workspace notifications

General action feedback in plugin and system workspaces uses MessageBanner and
the shared NotificationProvider. Info, Warning and Error prefixes accompany
theme-aware colors. A 44-pixel keyboard-operable dismiss target sits on the left.
Callouts occupy an upper-right overlay below the page header, at most 26rem wide, constrained to the
workspace and to 40% of its height with scrolling for a burst of messages.

Info expires after five seconds; hovering or focusing pauses expiry. Warnings
and errors remain until dismissed, their source resolves, or the page changes.
Identical mounted messages coalesce. Dismissal lasts for the current occurrence;
changed content or a new mount creates a new occurrence. Polling identical text
does not restore a dismissed message. No notification data is persisted.

Approval actions, irreversible-change warnings, field errors, progress and
configuration explanations remain in context. Dismissal never changes backend
state. Fullscreen instruments retain the notification area in the fullscreen
element. Dialog feedback remains in its dialog.

Instrument header actions align to the right of their title and status, wrapping
on constrained screens. Serial RX/TX share the terminal toolbar; connection owner
and expiry remain available under a compact connection-details disclosure.
