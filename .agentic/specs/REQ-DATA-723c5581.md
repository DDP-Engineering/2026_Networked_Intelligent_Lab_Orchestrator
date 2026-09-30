# REQ-DATA-723c5581 — Modbus map interchange

## Behavior

JSON export is the canonical version-1 map object with all supported point
fields. CSV is a versioned UTF-8 point interchange with one header and one row
per point. Preview parses the complete bounded document, reports row/field
errors and target conflicts, and returns a candidate only when it is valid.
Import repeats validation and uses one expected-revision transaction to publish
the metadata; failure leaves the prior revision unchanged. No import path
connects, polls, writes, or calls the runner.

## Data / interface

At most 1 MiB, 4096 points and 32 KiB per CSV record are accepted. JSON is a
strict object without unknown fields or trailing data. CSV header order is:
`schema_version,map_id,map_revision,map_owner_id,map_name,point_id,point_name,
description,area,address_base,address,type,width,order,unit,scale,offset,
precision,access,write_min,write_max,labels,padding`. `labels` is an object
encoded as compact JSON in one CSV field; blank limits mean JSON null. Export
uses `address_base=zero`, while import also accepts `one` and `reference` and
normalizes them with the explicit area. Extended references may overlap familiar
prefixes for other areas, since the selected area determines the subtraction base.
An empty map uses one metadata row
with blank point columns. Every row repeats identical map metadata; a mismatch
is a row error. CSV parser obeys RFC 4180 quoting and rejects malformed UTF-8.

Text cells whose first non-space character is `=`, `+`, `-` or `@`, or whose
first character is `~`, gain one leading `~` on export. Import removes exactly
one leading `~`; this reversible escape prevents spreadsheet formula
interpretation while preserving original names/units/descriptions/labels.

Import mode is `create`, `replace` or `merge`, with `target_id` and
`expected_revision` (0 for create). The importing actor becomes owner of a new
map; replace/merge retain the existing creator and require creator/admin
authority. Replace substitutes every point. Merge retains unmentioned points
and replaces matching point IDs only when `accept_conflicts=true`; preview
lists those IDs as conflicts. Existing target on create, stale revision, or
unaccepted merge conflict makes `can_apply=false`. A successful import returns
the saved map with its new revision. Preview never persists anything.

`ImportPreview` is `{format,mode,target_id,can_apply,row_errors,conflicts,
candidate}` where each error has `row,field,code`. A syntactically bad JSON
document is reported as row 0. CSV row numbers are one-based including the
header. Limits fail before allocating an unbounded document.

## Examples

- Exporting a map named `=SUM(1,2)` yields a CSV metadata cell
  `~=SUM(1,2)`; importing restores the exact name.
- A holding point at reference 40001 imports to wire address 0. A base-1
  address of zero is a row error; no map row is changed.
- Merge preview lists an existing point ID `speed`; importing without
  `accept_conflicts` refuses publication. Importing with it and the current
  revision changes only the named point and increments the map revision once.

## Acceptance

JSON and CSV round trips preserve all map metadata and point fields, including
empty maps and delimiter/formula-leading text. Mixed invalid CSV rows yield
row-local errors; a failed import leaves SQLite unchanged. An import/open
fixture observes zero private runner calls and zero new live sessions.
