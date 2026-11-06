# Godot l10n text check

Reusable Godot 4.x runtime addon and isolated demo for the `game-l10n-updater`
display-check stage. It replaces approximate rune-length checks with measurements
made from the actual theme font and Control rectangle after layout frames.

Only explicitly opted-in nodes are checked:

```gdscript
node.set_meta("l10n_key", "ui.save")          # required
node.set_meta("l10n_check_glyphs", true)      # optional, per-node glyph scan
```

Supported control types are `Label`, plain `Button`, and `RichTextLabel`.
An opted-in node of another type is reported as `unsupported_control`, not
silently passed. Button subclasses that add icons or decoration (for example
`CheckBox`, `CheckButton`, `OptionButton`, `MenuButton`) are reported as
`unsupported_mode` until their extra layout is modeled.

No external addon, downloaded art, or downloaded font is required. The demo uses
Godot's default theme.

## Requirements and tested environment

- Godot 4.3+ is the intended API level.
- Verified locally with Godot **4.7.2.stable.mono** in headless mode on Windows.
- No model download, API key, or network call is used by this subtree.
- Replace `godot` in the commands below with your installed Godot executable.

## Commands

Run from the repository root.

### 1. Import a fresh checkout

A fresh checkout needs an import pass before scene/SceneTree tests can resolve
`class_name` registrations and the demo scene.

```powershell
godot --headless --path godot --import
```

This creates `godot/.godot/`, which is ignored by the repository.

### 2. Automated tests

```powershell
godot --headless --path godot --script res://tests/run_tests.gd
```

The tests use explicit failure counts and exit 1 on any failure. They do not use
`assert` as the sole gate. They cover:

- flat JSON and Godot CSV loading, locale selection, quoted commas, Unicode;
- single-line Label overflow and a fitting string;
- wrapped Label height handling (fit vs. too-short);
- restoration of original node text, auto-translate mode, and
  `TranslationServer` locale;
- unsupported opted-in controls;
- report schema, absolute report paths, and refusal to write under `res://`;
- missing-glyph helper behavior;
- the demo scene: exactly one intentionally clipped Japanese button and zero
  issues for the fitting English strings.

Generated test files go in a unique child directory of `GAMEL10N_TEST_TMP`,
`TEMP`, `TMPDIR`, or `/tmp`, and are removed by the test. Use `--log-file` to
choose an explicit location for Godot logs.

### 3. Headless runner (example acceptance check)

```powershell
godot --headless --path godot --script res://tools/headless_check.gd -- `
  --report-path "$env:TEMP/l10n-demo-report.json"
```

The default mode checks Japanese and English, writes a combined report, expects
exactly one clipped issue (`demo.tight_action`, Japanese), expects the English
strings to pass, and exits 1 for unexpected outcomes.

`--check-glyphs` adds font-chain glyph findings to the report. On a headless
default theme font, CJK may be reported as font-chain missing even though the
system fallback renders it (visible in a real screenshot). The runner therefore
excludes `missing_glyph` from its seeded layout gate when that opt-in flag is
present; the report still contains the findings for review.

External catalog mode generates a report and does not enforce the seeded demo
expectation, because an arbitrary catalog may not contain the demo keys:

```powershell
godot --headless --path godot --script res://tools/headless_check.gd -- `
  --catalog "$env:TEMP/catalog_en.json" --locale en `
  --report-path "$env:TEMP/l10n-external-report.json"
```

The external catalog must contain the demo keys if you want meaningful demo
results. To check a different game scene, integrate the addon into that project
as described below.

### 4. Demo UI

```powershell
godot --path godot
```

The demo is a 1000x650 Control scene with:

- two language switching buttons (English / Japanese);
- normal fitting labels and one intentionally too-narrow translated button;
- a visible report summary and issue list;
- stable node names and `metadata/l10n_key` opt-in metadata;
- a visible PLaMo notice.

`Run display check` applies the current catalog strings, waits for layout, checks
only the opted-in nodes, prints the report summary/list, and restores the current
displayed text. The sample's Japanese and English strings are hand-written
fixtures; they are **not** presented as model output.

### 5. Demo check-only mode

```powershell
godot --headless --path godot -- --check-only `
  --report-path "$env:TEMP/l10n-demo-report.json"
```

The demo quits after writing the report. Pass app arguments after `--`.
Without `--report-path`, the report goes to
`user://l10n_text_check/demo-report-<locale>.json`.

External flat JSON:

```powershell
godot --headless --path godot -- --check-only `
  --catalog "$env:TEMP/catalog_en.json" --locale en `
  --report-path "$env:TEMP/l10n-external-report.json"
```

External Godot CSV (the selected column is chosen by `--locale`):

```powershell
godot --headless --path godot -- --check-only `
  --catalog "$env:TEMP/translations.csv" --locale ja `
  --report-path "$env:TEMP/l10n-csv-report.json"
```

### 6. Optional screenshot

Screenshots are real rendered frames only. Headless runs skip capture with a clear
message and still write a valid JSON report.

```powershell
godot --path godot -- --check-only `
  --report-path "$env:TEMP/l10n-demo-report.json" `
  --capture-path "$env:TEMP/l10n-demo.png"
```

A screenshot is optional evidence. It does not prove that all scenes or all
controls were exercised.

## Report format

```json
{
  "schema_version": 1,
  "issues": [
    {
      "key": "demo.tight_action",
      "locale": "ja",
      "node_path": "Layout/Content/Columns/Left/ButtonRow/TightButton",
      "kind": "clipped",
      "message": "Button text is 190.0 px wide but only 172.0 px is available (clip_text=true)."
    }
  ],
  "checked": 12
}
```

- `schema_version` is always `1`.
- `node_path` is relative to the scanned root; `.` means the root itself. Use
  stable node names for stable paths.
- `checked` is the number of opted-in node checks in this call, including nodes
  that produced `missing_key` or `unsupported_control`. Call the checker once per
  locale to check multiple languages.
- `kind` values:
  - `clipped` — text or content is clipped/trimmed by the control;
  - `overflow` — text draws beyond the measured content rectangle;
  - `missing_glyph` — font chain reports no glyph (opt-in);
  - `missing_key` — the catalog has no entry for the opted-in key;
  - `unsupported_control` — opted-in node type is not measured;
  - `unsupported_mode` — a layout mode that cannot be verified safely, such as a
    scrollable RichTextLabel whose content exceeds its rectangle.
- `message` is human-readable and includes measured pixel values where useful.

The report is deliberately limited to the subtree and locale that were checked.
Unvisited scenes are not certified.

## Catalog formats

### Flat target JSON

One locale per file; the locale label is supplied by `--locale` (default `en`):

```json
{
  "ui.title": "Save complete",
  "ui.action": "Continue"
}
```

Values must be strings. Nested locale maps are rejected with an error.

### Godot CSV

First header cell is `keys` (or `key`); following cells are locales:

```csv
keys,en,ja
ui.title,Save complete,保存しました
ui.action,"Continue, now","このまま続行する"
```

Quoted commas are supported. Select a column with `--locale`. An unknown locale
is an explicit error listing available columns.

## Addon integration

1. Copy `godot/addons/l10n_text_check/` into the game's `addons/` directory.
   Enabling the editor plugin is optional; the runtime `class_name` classes are
   available after import.
2. Opt in only the controls that belong to the localization display contract by
   setting `metadata/l10n_key` on stable, named nodes. Do not opt in decorative
   or purely dynamic report UI unless it is also part of the checked contract.
3. Apply translations through the game's normal localization path. If you want
   the helper to apply catalog strings directly, use `L10nCatalog` and
   `L10nTextChecker.apply_catalog(root, catalog)`.
4. After the relevant scene is in the tree and layout has settled, run the
   checker. The checker itself waits `settle_frames` (default 2) and then restores
   original text and auto-translate mode:

```gdscript
var catalog := L10nCatalog.new()
if not catalog.load_path("res://i18n/en.json", "en"):
    push_error(catalog.last_error)
    return

var checker := L10nTextChecker.new()
var report: Dictionary = await checker.check_scene(
    get_tree().current_scene,
    catalog,
    "en",
    {"settle_frames": 2, "check_glyphs": true}
)
if not checker.last_error.is_empty():
    push_error(checker.last_error)
    return

var write_error := L10nReport.write(report, "user://reports/display-en.json")
if not write_error.is_empty():
    push_error(write_error)
```

5. Import the JSON report into `game-l10n-updater` with the CLI's display-report
   import command. Run the checker per scene and locale; do not infer coverage
   from a screenshot or from one scene's report.

`L10nReport.write` refuses `res://` paths so generated reports cannot accidentally
land in tracked source. Use `user://` or an absolute path outside the repository.

## What is actually measured

- **Label:** `get_line_count()` / `get_visible_line_count()`, required height from
  the effective font height plus line spacing, and either `Font.get_string_size`
  (no wrapping) or `Font.get_multiline_string_size` at the available width
  (wrapping). `clip_text` and `text_overrun_behavior` decide `clipped` vs.
  `overflow`.
- **Button:** effective theme font, stylebox content margins, icon width and
  `h_separation`, then text width/height against the remaining content rectangle.
  `clip_text` decides `clipped` vs. `overflow`.
- **RichTextLabel:** content width/height against the content rectangle. If
  scrolling is active and content exceeds the rectangle, the result is
  `unsupported_mode` rather than a false pass.
- **Missing glyph:** `Font.has_char` against the effective theme font and
  `ThemeDB.fallback_font`. This is opt-in (global `check_glyphs` or per-node
  `l10n_check_glyphs`) because system fallback fonts cannot be enumerated
  reliably. The report says "font chain reports no glyph"; it does not claim that
  a system fallback definitely failed to render.
- **Preservation:** original node text, `auto_translate_mode`, and
  `TranslationServer` locale are restored after each check. Switching languages
  in the demo intentionally changes the displayed locale; the checker itself
  does not leave a different locale behind.

## Tested vs. unsupported coverage

Verified in the automated tests and headless runner:

- flat JSON and Godot CSV loading, locale switching, Unicode, quoted commas;
- Label overflow and fitting strings;
- wrapped Label with enough height and with too little height;
- Button clipping caused by a deliberately narrow translated button;
- RichTextLabel non-scrolling overflow and the scrollable `unsupported_mode`
  result;
- text/locale restoration;
- explicit unsupported controls (`unsupported_control`);
- report schema and absolute/user:// writing;
- glyph helper detection for an uncovered private-use code point;
- one deliberately clipped Japanese button and fitting English strings in the
  demo scene.

Not covered / limitations:

- all scenes and all controls in a project — only the scanned subtree and locale
  are checked;
- nodes without `l10n_key` metadata;
- custom-drawn text, `LineEdit`, `TextEdit`, `Label3D`, 3D worlds, and game HUDs
  rendered outside this Control contract;
- RichTextLabel glyph coverage, BBCode-aware per-span font selection, and full
  scrollable-content verification (scrollable overflow is reported as
  `unsupported_mode`);
- Buttons using `expand_icon` (reported as `unsupported_mode`);
- Buttons using `autowrap_mode` (reported as `unsupported_mode`);
- Button subclasses that draw extra icons/decorations (`CheckBox`,
  `CheckButton`, `OptionButton`, `MenuButton`, etc.) — reported as
  `unsupported_mode` rather than measured with a false pass;
- fonts or translations loaded after the check, animations, scene transitions,
  and text changed by gameplay before/after the check;
- hidden opted-in nodes are reported as `unsupported_mode` (not silently passed)
  because they are not rendered;
- system fallback rendering of missing glyphs;
- treating a screenshot as evidence for unvisited scenes.

## PLaMo notice and licensing

The sample strings in `demo/translations.csv` and the demo JSON files are
hand-written fixtures. They are not PLaMo model output and should not be cited as
model output.

Real catalogs used with the parent workflow may contain translations produced by
the PLaMo model. Follow the PLaMo model terms and the notices/terms linked by the
parent project documentation (see the repository's `THIRD_PARTY_NOTICES.md` and
`README.md`), and do not imply that this demo generated them. This subtree makes
no network or model calls.

The addon and demo are provided under the repository's MIT license. No external
downloaded art or fonts are included.
