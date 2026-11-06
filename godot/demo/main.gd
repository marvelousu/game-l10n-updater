extends Control

## Isolated demo driver. It is also the target of the headless SceneTree runner.
##
## Direct dialog use:
##   godot --path godot
##
## Automated report:
##   godot --headless --path godot -- --check-only --report-path D:/tmp/report.json
##
## The runner may set the public override fields before adding this scene.

const DEFAULT_CATALOG := "res://demo/translations.csv"
const DEFAULT_LOCALE := "ja"

var catalog_path_override: String = ""
var locale_override: String = ""
var report_path_override: String = ""
var capture_path_override: String = ""
var check_glyphs: bool = false
var auto_check: bool = false
var runner_owned: bool = false

var _catalog: L10nCatalog
var _checker: L10nTextChecker
var _current_locale: String = ""

@onready var _locale_en_button: Button = %LocaleEnButton
@onready var _locale_ja_button: Button = %LocaleJaButton
@onready var _run_button: Button = %RunCheckButton
@onready var _status_label: Label = %StatusLabel
@onready var _report_summary: Label = %ReportSummary
@onready var _report_list: RichTextLabel = %ReportList


func _ready() -> void:
	_catalog = L10nCatalog.new()
	_checker = L10nTextChecker.new()

	if not _read_cli():
		return

	_wire_buttons()
	if not _load_catalog():
		return
	_apply_current_locale()

	if auto_check:
		await _run_check_and_exit()


func _read_cli() -> bool:
	var options := L10nCli.parse(L10nCli.app_args())
	if bool(options["help"]):
		print(L10nCli.USAGE)
		get_tree().quit(0)
		return false

	if catalog_path_override.is_empty():
		catalog_path_override = String(options["catalog_path"])
	if locale_override.is_empty():
		locale_override = String(options["locale"])
	if report_path_override.is_empty():
		report_path_override = String(options["report_path"])
	if capture_path_override.is_empty():
		capture_path_override = String(options["capture_path"])
	check_glyphs = check_glyphs or bool(options["check_glyphs"])
	if not runner_owned:
		auto_check = auto_check or bool(options["check_only"])

	var unknown: Array[String] = options["unknown"]
	if not unknown.is_empty():
		push_warning("Ignoring unknown options: %s" % ", ".join(unknown))
	return true


func _wire_buttons() -> void:
	_locale_en_button.pressed.connect(_on_locale_en_pressed)
	_locale_ja_button.pressed.connect(_on_locale_ja_pressed)
	_run_button.pressed.connect(_on_run_check_pressed)


func _load_catalog() -> bool:
	var path := catalog_path_override
	if path.is_empty():
		path = DEFAULT_CATALOG
	var wanted_locale := locale_override
	if wanted_locale.is_empty() and catalog_path_override.is_empty():
		wanted_locale = DEFAULT_LOCALE

	if not _catalog.load_path(path, wanted_locale):
		_status_label.text = "Catalog error: %s" % _catalog.last_error
		push_error(_catalog.last_error)
		if auto_check:
			get_tree().quit(1)
		return false

	_current_locale = _catalog.locale
	if not _catalog.warnings.is_empty():
		push_warning("Catalog warnings: %s" % "; ".join(_catalog.warnings))
	return true


func _apply_current_locale() -> void:
	if not _catalog.select_locale(_current_locale):
		_status_label.text = "Locale error: %s" % _catalog.last_error
		return

	_current_locale = _catalog.locale
	TranslationServer.set_locale(_current_locale)
	var missing := _checker.apply_catalog(self, _catalog)
	_refresh_locale_buttons()
	_refresh_status(missing)
	_show_placeholder_report()


func _refresh_locale_buttons() -> void:
	_locale_en_button.disabled = not _catalog.has_locale("en")
	_locale_ja_button.disabled = not _catalog.has_locale("ja")
	_locale_en_button.tooltip_text = "Switch to the English catalog column."
	_locale_ja_button.tooltip_text = "日本語のカタログ列に切り替えます。"
	if not _catalog.has_locale("en"):
		_locale_en_button.tooltip_text = "English is not present in this catalog."
	if not _catalog.has_locale("ja"):
		_locale_ja_button.tooltip_text = "Japanese is not present in this catalog."


func _refresh_status(missing: Array[Dictionary]) -> void:
	var source_name := _catalog.source_path.get_file()
	if source_name.is_empty():
		source_name = "in-memory mapping"
	var opted_in := _checker.collect_opted_in_nodes(self).size()
	var suffix := ""
	if not missing.is_empty():
		suffix = " · %d missing key(s)" % missing.size()
	_status_label.text = "Catalog: %s (%s) · locale: %s · %d opted-in node(s)%s" % [
		source_name, _catalog.source_format.to_upper(), _current_locale, opted_in, suffix
	]
	_status_label.tooltip_text = _catalog.source_path


func _show_placeholder_report() -> void:
	_report_summary.text = "No check has run yet for locale '%s'." % _current_locale
	_report_list.text = "Press \"Run display check\" to measure the opted-in controls with the current theme font. Only opted-in nodes in this scene are checked."


func _on_locale_en_pressed() -> void:
	_switch_locale("en")


func _on_locale_ja_pressed() -> void:
	_switch_locale("ja")


func _switch_locale(wanted: String) -> void:
	if not _catalog.select_locale(wanted):
		_status_label.text = "Locale error: %s" % _catalog.last_error
		return
	_current_locale = _catalog.locale
	_apply_current_locale()


func _on_run_check_pressed() -> void:
	_run_button.disabled = true
	_report_summary.text = "Checking locale '%s'…" % _current_locale
	await _perform_check(true)
	_run_button.disabled = false


func _perform_check(write_to_override: bool = true) -> Dictionary:
	var report := await _checker.check_scene(
		self,
		_catalog,
		_current_locale,
		{"check_glyphs": check_glyphs, "settle_frames": 2}
	)

	if not _checker.last_error.is_empty():
		_status_label.text = "Check error: %s" % _checker.last_error
		push_error(_checker.last_error)
		return report

	_show_report(report)
	_status_label.text = "Checked %d opted-in node(s) for locale '%s'." % [
		int(report["checked"]), _current_locale
	]

	if write_to_override and not report_path_override.is_empty():
		var write_error := L10nReport.write(report, report_path_override)
		if write_error.is_empty():
			print("L10n report written: %s" % ProjectSettings.globalize_path(report_path_override))
		else:
			push_error(write_error)
			_status_label.text = "Report write error: %s" % write_error
	return report


func _show_report(report: Dictionary) -> void:
	var issues: Array = report["issues"]
	var issue_count := issues.size()
	_report_summary.text = "Checked %d opted-in node(s) for '%s': %d issue(s). Only this scene subtree and locale were exercised." % [
		int(report["checked"]), _current_locale, issue_count
	]

	if issues.is_empty():
		_report_list.text = "No clipped or overflowing text found in the checked nodes for this locale. Unvisited scenes are not certified."
		return

	var lines: PackedStringArray = []
	for raw_issue in issues:
		var issue: Dictionary = raw_issue
		lines.append("[%s] %s\n  %s\n  %s" % [
			String(issue["kind"]),
			String(issue["key"]),
			String(issue["node_path"]),
			String(issue["message"]),
		])
	_report_list.text = "\n\n".join(lines)


func _run_check_and_exit() -> void:
	var report := await _perform_check(false)
	if not _checker.last_error.is_empty():
		get_tree().quit(1)
		return

	var report_path := report_path_override
	if report_path.is_empty():
		report_path = "user://l10n_text_check/demo-report-%s.json" % _current_locale
	var write_error := L10nReport.write(report, report_path)
	if not write_error.is_empty():
		push_error(write_error)
		get_tree().quit(1)
		return

	var issues: Array = report["issues"]
	print("L10n report: %s" % ProjectSettings.globalize_path(report_path))
	print("L10n check: locale=%s checked=%d issues=%d" % [
		_current_locale, int(report["checked"]), issues.size()
	])

	if not capture_path_override.is_empty():
		if DisplayServer.get_name() == "headless":
			print("Screenshot skipped: headless display server; the JSON report is still valid.")
		else:
			var capture_error := await capture_screenshot(capture_path_override)
			if not capture_error.is_empty():
				push_error(capture_error)
				get_tree().quit(1)
				return
	get_tree().quit(0)


## Saves a real rendered frame only when a display server is available.
## Returns "" on success, otherwise a human-readable reason.
func capture_screenshot(path: String) -> String:
	if DisplayServer.get_name() == "headless":
		return "headless display server; no rendered frame is available"
	if path.strip_edges().is_empty():
		return "capture path is empty"
	var clean_path := path.strip_edges()
	if clean_path.begins_with("res://"):
		return "refusing to write a screenshot under res://: %s" % clean_path

	var parent_dir := clean_path.get_base_dir()
	if not parent_dir.is_empty():
		var absolute_dir := ProjectSettings.globalize_path(parent_dir)
		if not DirAccess.dir_exists_absolute(absolute_dir):
			var dir_error := DirAccess.make_dir_recursive_absolute(absolute_dir)
			if dir_error != OK:
				return "cannot create screenshot directory %s: %s" % [
					absolute_dir, error_string(dir_error)
				]

	await RenderingServer.frame_post_draw
	var viewport := get_viewport()
	if viewport == null:
		return "no viewport is available"
	var texture := viewport.get_texture()
	if texture == null:
		return "viewport texture is not available"
	var image := texture.get_image()
	if image == null:
		return "viewport image is not available"
	var save_error := image.save_png(clean_path)
	if save_error != OK:
		return "save_png failed for %s: %s" % [clean_path, error_string(save_error)]
	print("L10n screenshot: %s" % ProjectSettings.globalize_path(clean_path))
	return ""
