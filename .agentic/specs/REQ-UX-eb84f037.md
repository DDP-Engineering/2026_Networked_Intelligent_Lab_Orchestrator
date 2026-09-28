# REQ-UX-eb84f037 — Control top-level and nested navigation expansion

## Behavior

Each authenticated session starts with Instruments and System collapsed and every nested group collapsed. Expanding a section reveals its authorized roots; expanding a nested group reveals its children. Toggle actions do not select pages or close tabs. Selecting a destination visible within an expanded section opens or activates its workspace and exposes nested ancestors as necessary. Switching to an already open tab does not implicitly expand a closed top-level section; About being active does not open System. Closing a section leaves its nested expansion choices intact within the session. A new authenticated session starts from the collapsed state. A read/refresh during the same session does not reset choices.

## Data / interface

Each section has a visible translated name, a keyboard-activatable button with an accessible name and `aria-expanded`, and an associated child list while expanded. Nested toggle controls likewise expose their expanded state. The same expansion model is shared by wide rail, compact rail and constrained drawer. Inventory eligibility and authorization filter destinations, not the existence of section controls.

## Examples

On entry with About active and a ready Camera device, neither section's children are shown. Expanding Instruments reveals Camera, still collapsed; expanding Camera reveals its ready device. After opening the device and collapsing Instruments, its tab stays active and the section stays closed. Expanding System reveals About without selecting it. A failed inventory refresh may remove visible instrument destinations but leaves both section controls, tabs and expansion choices intact.

## Acceptance

Component and browser tests exercise Enter and Space, `aria-expanded`, translated accessible names and keyboard focus in each layout; starting state, nested activation, tab switching, session change and refresh must follow the state transitions above.

## Assumptions and open decisions

The existing page registry remains the source of permission/readiness-filtered children; navigation expansion is session-local and not saved as a preference. The exact ownership of state between App and the shell is a design decision, provided refresh-induced remounts preserve it.
