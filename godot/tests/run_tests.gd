extends SceneTree

## Automated headless tests. Run from the repository root:
##   godot --headless --path godot --import
##   godot --headless --path godot --script res://tests/run_tests.gd
##
## All generated files live under user:// (or an absolute temp path provided by
## the caller); no fixture output is written into the tracked source tree.

const DEMO_SCENE := preload("res://demo/main.tscn")
var TEMP_DIR: String = ""

var failures := 0
var checks := 0


func _initialize() -> void:
	var base := OS.get_environment("GAMEL10N_TEST_TMP")
	if base.is_empty():
		base = OS.get_environment("TEMP")
	if base.is_empty():
		base = OS.get_environment("TMPDIR")
	if base.is_empty():
		base = "/tmp"
	TEMP_DIR = base.path_join("gamel10n-tests-%d-%d" % [OS.get_process_id(), Time.get_ticks_usec()])
	call_deferred("_run")


func _run() -> void:
	DirAccess.make_dir_recursive_absolute(ProjectSettings.globalize_path(TEMP_DIR))
	print("L10n text check tests")

	await _test_catalog_mapping()
	await _test_catalog_json_and_csv()
	await _test_label_overflow_and_fit()
	await _test_explicit_multiline_label()
	await _test_wrapped_label()
	await _test_wrapped_button()
	await _test_richtext_label()
	await _test_preserves_text_and_locale()
	await _test_catalog_locale_restore()
	await _test_unsupported_control()
	await _test_missing_key()
	await _test_report_writer()
	await _test_glyph_helper()
	await _test_demo_scene()

	_cleanup()
	if failures > 0:
		printerr("RESULT: FAIL (%d failure(s), %d check(s))" % [failures, checks])
		quit(1)
		return
	print("RESULT: PASS (%d check(s))" % checks)
	quit(0)


func _test_catalog_mapping() -> void:
	print("- catalog mapping")
	var catalog := L10nCatalog.new()
	_expect(catalog.load_mapping({
		"greet": "Hello",
		"uni": "日本語",
	}, "en"), "in-memory mapping loads")
	_expect(catalog.get_text("greet") == "Hello", "mapping returns a value")
	_expect(catalog.get_text("uni") == "日本語", "mapping preserves Unicode")
	_expect(catalog.has_key("greet"), "mapping has a known key")
	_expect(not catalog.has_key("missing"), "mapping rejects an unknown key")
	_expect(not catalog.load_mapping({"bad": 42}, "en"), "non-string mapping value is rejected")
	_expect(not catalog.load_mapping({"": "empty key"}, "en"), "empty mapping key is rejected")
	await process_frame


func _test_catalog_json_and_csv() -> void:
	print("- catalog JSON and CSV")
	var json_path := TEMP_DIR + "/catalog.json"
	var csv_path := TEMP_DIR + "/catalog.csv"
	_write_text(json_path, "{\"greet\":\"Hello\",\"uni\":\"日本語\",\"comma\":\"one, two\"}\n")
	_write_text(
		csv_path,
		"keys,en,ja\ngreet,Hello,こんにちは\ncomma,\"one, two\",\"一つ、二つ\"\n"
	)

	var catalog := L10nCatalog.new()
	_expect(catalog.load_flat_json(json_path, "en"), "flat JSON loads")
	_expect(catalog.source_format == "json", "flat JSON format is recorded")
	_expect(catalog.get_text("comma") == "one, two", "flat JSON keeps a comma")
	var json_ja := L10nCatalog.new()
	_expect(json_ja.load_flat_json(json_path, "ja"), "flat JSON can be labelled with a locale")
	_expect(json_ja.locale == "ja", "flat JSON locale label is retained")

	var csv_catalog := L10nCatalog.new()
	_expect(csv_catalog.load_csv_file(csv_path, "ja"), "Godot CSV loads")
	_expect(csv_catalog.source_format == "csv", "CSV format is recorded")
	_expect(csv_catalog.get_text("greet") == "こんにちは", "CSV selects the requested locale column")
	_expect(csv_catalog.select_locale("en"), "CSV can switch locale columns")
	_expect(csv_catalog.get_text("greet") == "Hello", "CSV English column is available")
	_expect(csv_catalog.get_text("comma") == "one, two", "CSV quoted comma is preserved")
	_expect(csv_catalog.available_locales.size() == 2, "CSV reports both locales")
	_expect(not csv_catalog.select_locale("fr"), "missing CSV locale is rejected")
	_expect(not csv_catalog.last_error.is_empty(), "missing CSV locale has an error message")

	var bad_json := TEMP_DIR + "/bad.json"
	_write_text(bad_json, "[1, 2, 3]\n")
	var bad_catalog := L10nCatalog.new()
	_expect(not bad_catalog.load_flat_json(bad_json, "en"), "non-object JSON is rejected")
	_expect(not bad_catalog.last_error.is_empty(), "bad JSON has an error message")
	await process_frame


func _test_label_overflow_and_fit() -> void:
	print("- label overflow and fit")
	var root := await _add_root(Vector2(600, 400))
	var overflow := _make_label("OverflowLabel", Vector2(10, 10), Vector2(120, 30), "placeholder", "test.overflow")
	overflow.clip_text = true
	root.add_child(overflow)
	var fit := _make_label("FitLabel", Vector2(10, 60), Vector2(300, 30), "placeholder", "test.fit")
	root.add_child(fit)
	await process_frame

	var catalog := L10nCatalog.new()
	catalog.load_mapping({
		"test.overflow": "This translated string is definitely too wide for the narrow label.",
		"test.fit": "Short",
	}, "en")
	var checker := L10nTextChecker.new()
	var report := await checker.check_scene(root, catalog, "en", {"settle_frames": 2})

	_expect(int(report["checked"]) == 2, "two opted-in labels were checked")
	var issues: Array = report["issues"]
	_expect(issues.size() == 1, "only the too-narrow label is reported")
	if issues.size() == 1:
		var issue: Dictionary = issues[0]
		_expect(String(issue["key"]) == "test.overflow", "overflow issue has the expected key")
		_expect(String(issue["node_path"]) == "OverflowLabel", "overflow issue has a stable node path")
		_expect(String(issue["kind"]) == "clipped", "clip_text Label is reported as clipped")
		_expect(String(issue["locale"]) == "en", "overflow issue has the checked locale")
	_expect(overflow.text == "placeholder", "overflow Label text was restored")
	_expect(fit.text == "placeholder", "fitting Label text was restored")

	root.queue_free()
	await process_frame


func _test_wrapped_label() -> void:
	print("- wrapped label")
	var root := await _add_root(Vector2(600, 400))
	var wrapped_fit := _make_label("WrappedFit", Vector2(10, 10), Vector2(260, 90), "placeholder", "test.wrap_fit")
	wrapped_fit.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
	root.add_child(wrapped_fit)
	var wrapped_over := _make_label("WrappedOver", Vector2(10, 120), Vector2(260, 30), "placeholder", "test.wrap_over")
	wrapped_over.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
	root.add_child(wrapped_over)
	await process_frame

	var text := "A short wrapped note with enough room for two lines and no clipped text."
	var catalog := L10nCatalog.new()
	catalog.load_mapping({
		"test.wrap_fit": text,
		"test.wrap_over": text,
	}, "en")
	var checker := L10nTextChecker.new()
	var report := await checker.check_scene(root, catalog, "en", {"settle_frames": 2})
	var issues: Array = report["issues"]
	_expect(issues.size() == 1, "wrapped fit passes while the short-height one is reported")
	if issues.size() != 1:
		printerr("wrapped issues: %s" % str(issues))
	if issues.size() == 1:
		var issue: Dictionary = issues[0]
		_expect(String(issue["key"]) == "test.wrap_over", "wrapped issue identifies the short-height Label")
		_expect(String(issue["kind"]) == "overflow", "unclipped wrapped Label is reported as overflow")

	root.queue_free()
	await process_frame


func _test_wrapped_button() -> void:
	print("- wrapped button")
	var root := await _add_root(Vector2(600, 400))
	var wrapped_fit := Button.new()
	wrapped_fit.name = "WrappedButtonFit"
	wrapped_fit.position = Vector2(10, 10)
	wrapped_fit.size = Vector2(220, 110)
	wrapped_fit.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
	wrapped_fit.text = "placeholder"
	wrapped_fit.set_meta("l10n_key", "test.button_wrap_fit")
	root.add_child(wrapped_fit)
	var wrapped_over := Button.new()
	wrapped_over.name = "WrappedButtonOver"
	wrapped_over.position = Vector2(10, 140)
	wrapped_over.size = Vector2(220, 30)
	wrapped_over.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
	wrapped_over.text = "placeholder"
	wrapped_over.set_meta("l10n_key", "test.button_wrap_over")
	root.add_child(wrapped_over)
	await process_frame

	var text := "Wrapped button note with a couple of lines."
	var catalog := L10nCatalog.new()
	catalog.load_mapping({
		"test.button_wrap_fit": text,
		"test.button_wrap_over": text,
	}, "en")
	var checker := L10nTextChecker.new()
	var report := await checker.check_scene(root, catalog, "en", {"settle_frames": 2})
	var issues: Array = report["issues"]
	_expect(issues.size() == 2, "both wrapped Buttons are explicitly unsupported")
	for raw_issue in issues:
		var issue: Dictionary = raw_issue
		_expect(String(issue["kind"]) == "unsupported_mode", "wrapped Button is not silently passed")

	root.queue_free()
	await process_frame


func _test_richtext_label() -> void:
	print("- RichTextLabel")
	var root := await _add_root(Vector2(600, 400))
	var no_scroll := RichTextLabel.new()
	no_scroll.name = "RichNoScroll"
	no_scroll.position = Vector2(10, 10)
	no_scroll.size = Vector2(220, 50)
	no_scroll.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
	no_scroll.scroll_active = false
	no_scroll.text = "placeholder"
	no_scroll.set_meta("l10n_key", "test.rich_no_scroll")
	root.add_child(no_scroll)
	var with_scroll := RichTextLabel.new()
	with_scroll.name = "RichWithScroll"
	with_scroll.position = Vector2(10, 120)
	with_scroll.size = Vector2(220, 50)
	with_scroll.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
	with_scroll.scroll_active = true
	with_scroll.text = "placeholder"
	with_scroll.set_meta("l10n_key", "test.rich_scroll")
	root.add_child(with_scroll)
	await process_frame

	var long_text := "Rich text content that is deliberately much longer than the measured rectangle so that the content height exceeds the available height in both configurations."
	var catalog := L10nCatalog.new()
	catalog.load_mapping({
		"test.rich_no_scroll": long_text,
		"test.rich_scroll": long_text,
	}, "en")
	var checker := L10nTextChecker.new()
	var report := await checker.check_scene(root, catalog, "en", {"settle_frames": 2})
	var issues: Array = report["issues"]
	_expect(issues.size() == 2, "RichTextLabel fixtures produce two explicit results")
	var kinds: Dictionary = {}
	for raw_issue in issues:
		var issue: Dictionary = raw_issue
		kinds[String(issue["kind"])] = true
	_expect(kinds.has("clipped"), "non-scrolling RichTextLabel overflow is clipped")
	_expect(kinds.has("unsupported_mode"), "scrolling RichTextLabel overflow is unsupported_mode")

	root.queue_free()
	await process_frame


func _test_preserves_text_and_locale() -> void:
	print("- text and locale preservation")
	var root := await _add_root(Vector2(600, 200))
	var label := _make_label("Preserved", Vector2(10, 10), Vector2(300, 30), "original text", "test.preserve")
	root.add_child(label)
	await process_frame

	var catalog := L10nCatalog.new()
	catalog.load_mapping({"test.preserve": "translated text"}, "en")
	var checker := L10nTextChecker.new()
	TranslationServer.set_locale("fr")
	var report := await checker.check_scene(root, catalog, "en", {"settle_frames": 2})
	_expect(int(report["checked"]) == 1, "preservation fixture was checked")
	_expect(label.text == "original text", "original Label text is restored")
	_expect(TranslationServer.get_locale() == "fr", "TranslationServer locale is preserved")
	_expect(label.auto_translate_mode == Node.AUTO_TRANSLATE_MODE_INHERIT, "auto-translate mode is restored")
	TranslationServer.set_locale("en")

	root.queue_free()
	await process_frame


func _test_catalog_locale_restore() -> void:
	print("- catalog locale restore")
	var root := await _add_root(Vector2(600, 200))
	var label := _make_label("CatalogLocale", Vector2(10, 10), Vector2(300, 30), "original", "greet")
	root.add_child(label)
	await process_frame

	var csv_path := TEMP_DIR + "/locale-restore.csv"
	_write_text(csv_path, "keys,en,ja\ngreet,Hello,こんにちは\n")
	var catalog := L10nCatalog.new()
	_expect(catalog.load_csv_file(csv_path, "en"), "two-locale catalog loads in English")
	var checker := L10nTextChecker.new()
	var report := await checker.check_scene(root, catalog, "ja", {"settle_frames": 1})
	_expect(int(report["checked"]) == 1, "catalog-locale fixture was checked")
	_expect(catalog.locale == "en", "catalog locale is restored after checking another locale")
	_expect(label.text == "original", "catalog-locale fixture text is restored")

	root.queue_free()
	await process_frame


func _test_unsupported_control() -> void:
	print("- unsupported opted-in control")
	var root := await _add_root(Vector2(400, 200))
	var texture := TextureRect.new()
	texture.name = "UnsupportedTexture"
	texture.position = Vector2(10, 10)
	texture.size = Vector2(100, 100)
	texture.set_meta("l10n_key", "test.texture")
	root.add_child(texture)
	await process_frame

	var catalog := L10nCatalog.new()
	catalog.load_mapping({"test.texture": "not measured"}, "en")
	var checker := L10nTextChecker.new()
	var report := await checker.check_scene(root, catalog, "en", {"settle_frames": 1})
	var issues: Array = report["issues"]
	_expect(int(report["checked"]) == 1, "unsupported opted-in node is counted")
	_expect(issues.size() == 1, "unsupported opted-in node is not a false pass")
	if issues.size() == 1:
		_expect(String(issues[0]["kind"]) == "unsupported_control", "unsupported node kind is explicit")

	root.queue_free()
	await process_frame

	var button_root := await _add_root(Vector2(400, 200))
	var check_box := CheckBox.new()
	check_box.name = "UnsupportedCheckBox"
	check_box.position = Vector2(10, 10)
	check_box.size = Vector2(160, 40)
	check_box.text = "Check"
	check_box.set_meta("l10n_key", "test.checkbox")
	button_root.add_child(check_box)
	await process_frame

	var button_catalog := L10nCatalog.new()
	button_catalog.load_mapping({"test.checkbox": "Check translated"}, "en")
	var button_checker := L10nTextChecker.new()
	var button_report := await button_checker.check_scene(
		button_root, button_catalog, "en", {"settle_frames": 1}
	)
	var button_issues: Array = button_report["issues"]
	_expect(button_issues.size() == 1, "Button subclass is not silently passed")
	if button_issues.size() == 1:
		_expect(String(button_issues[0]["kind"]) == "unsupported_mode", "Button subclass mode is explicit")

	button_root.queue_free()
	await process_frame

	var hidden_root := await _add_root(Vector2(400, 200))
	var hidden := _make_label("HiddenLabel", Vector2(10, 10), Vector2(200, 30), "hidden", "test.hidden")
	hidden.visible = false
	hidden_root.add_child(hidden)
	await process_frame

	var hidden_catalog := L10nCatalog.new()
	hidden_catalog.load_mapping({"test.hidden": "hidden translated"}, "en")
	var hidden_checker := L10nTextChecker.new()
	var hidden_report := await hidden_checker.check_scene(
		hidden_root, hidden_catalog, "en", {"settle_frames": 1}
	)
	var hidden_issues: Array = hidden_report["issues"]
	_expect(hidden_issues.size() == 1, "hidden opted-in node is not a false pass")
	if hidden_issues.size() == 1:
		_expect(String(hidden_issues[0]["kind"]) == "unsupported_mode", "hidden node mode is explicit")

	hidden_root.queue_free()
	await process_frame


func _test_missing_key() -> void:
	print("- missing key")
	var root := await _add_root(Vector2(400, 200))
	var label := _make_label("MissingKeyLabel", Vector2(10, 10), Vector2(300, 30), "original missing", "test.absent")
	root.add_child(label)
	await process_frame

	var catalog := L10nCatalog.new()
	catalog.load_mapping({"test.present": "present"}, "en")
	var checker := L10nTextChecker.new()
	var report := await checker.check_scene(root, catalog, "en", {"settle_frames": 1})
	var issues: Array = report["issues"]
	_expect(int(report["checked"]) == 1, "missing-key node is counted")
	_expect(issues.size() == 1, "missing key is not a false pass")
	if issues.size() == 1:
		_expect(String(issues[0]["kind"]) == "missing_key", "missing key kind is explicit")
		_expect(String(issues[0]["key"]) == "test.absent", "missing key is reported")
	_expect(label.text == "original missing", "missing-key node text is untouched")

	root.queue_free()
	await process_frame


func _test_report_writer() -> void:
	print("- report writer")
	var report := L10nReport.build([
		L10nReport.issue("sample.key", "en", "Root/Label", "clipped", "message"),
	], 1)
	_expect(not L10nReport.write(report, "res://generated-report.json").is_empty(), "writer refuses res://")
	_expect(not FileAccess.file_exists("res://generated-report.json"), "refused write creates no source file")

	var user_path := TEMP_DIR + "/report.json"
	var error := L10nReport.write(report, user_path)
	_expect(error.is_empty(), "writer writes a user:// report")
	var parsed: Variant = JSON.parse_string(FileAccess.get_file_as_string(user_path))
	_expect(typeof(parsed) == TYPE_DICTIONARY, "written report is valid JSON")
	if typeof(parsed) == TYPE_DICTIONARY:
		var data: Dictionary = parsed
		_expect(int(data["schema_version"]) == 1, "report schema_version is 1")
		_expect(int(data["checked"]) == 1, "report checked count is preserved")
		var issues: Array = data["issues"]
		_expect(issues.size() == 1, "report issue array is preserved")
		if issues.size() == 1:
			var issue: Dictionary = issues[0]
			_expect(String(issue["key"]) == "sample.key", "report issue key is preserved")
			_expect(String(issue["node_path"]) == "Root/Label", "report issue node path is preserved")

	var absolute_path := ProjectSettings.globalize_path(TEMP_DIR + "/absolute-report.json")
	var absolute_error := L10nReport.write(report, absolute_path)
	_expect(absolute_error.is_empty(), "writer accepts an absolute report path")
	_expect(FileAccess.file_exists(absolute_path), "absolute report exists")
	await process_frame


func _test_glyph_helper() -> void:
	print("- glyph helper")
	var private_use := String.chr(0xE000)
	var missing := L10nTextChecker.find_missing_glyphs(ThemeDB.fallback_font, private_use)
	_expect(missing.size() == 1, "fallback font reports a private-use glyph missing")
	_expect(L10nTextChecker.find_missing_glyphs(ThemeDB.fallback_font, "Hello").is_empty(), "fallback font covers ASCII")
	await process_frame


func _test_demo_scene() -> void:
	print("- demo scene expectations")
	var demo := DEMO_SCENE.instantiate()
	demo.set("runner_owned", true)
	get_root().add_child(demo)
	await process_frame
	await process_frame
	await process_frame

	var catalog := L10nCatalog.new()
	_expect(catalog.load_path("res://demo/translations.csv", "ja"), "demo CSV loads")
	var checker := L10nTextChecker.new()
	var opted_in := checker.collect_opted_in_nodes(demo).size()
	_expect(opted_in == 6, "demo has six opted-in nodes")

	var ja_report := await checker.check_scene(demo, catalog, "ja", {"settle_frames": 2})
	var en_report := await checker.check_scene(demo, catalog, "en", {"settle_frames": 2})
	_expect(int(ja_report["checked"]) == opted_in, "Japanese check covers every opted-in node")
	_expect(int(en_report["checked"]) == opted_in, "English check covers every opted-in node")

	var ja_issues: Array = ja_report["issues"]
	var en_issues: Array = en_report["issues"]
	_expect(ja_issues.size() == 1, "demo reports exactly one Japanese issue")
	if ja_issues.size() != 1:
		printerr("demo ja issues: %s" % str(ja_issues))
	if ja_issues.size() == 1:
		var issue: Dictionary = ja_issues[0]
		_expect(String(issue["key"]) == "demo.tight_action", "demo issue is the intentionally narrow button")
		_expect(String(issue["kind"]) == "clipped", "demo narrow button is clipped")
		_expect(String(issue["node_path"]).ends_with("TightButton"), "demo issue path ends at TightButton")
	_expect(en_issues.is_empty(), "fitting English demo strings pass")
	if not en_issues.is_empty():
		printerr("demo en issues: %s" % str(en_issues))

	demo.queue_free()
	await process_frame


func _add_root(size: Vector2) -> Control:
	var root := Control.new()
	root.name = "TestRoot"
	root.position = Vector2.ZERO
	root.size = size
	get_root().add_child(root)
	await process_frame
	return root


func _make_label(
		node_name: String,
		position: Vector2,
		size: Vector2,
		text: String,
		key: String
) -> Label:
	var label := Label.new()
	label.name = node_name
	label.position = position
	label.size = size
	label.text = text
	label.set_meta("l10n_key", key)
	return label


func _write_text(path: String, text: String) -> void:
	var file := FileAccess.open(path, FileAccess.WRITE)
	if file == null:
		_expect(false, "could not create test file %s" % path)
		return
	file.store_string(text)
	file.close()


func _expect(condition: bool, message: String) -> void:
	checks += 1
	if not condition:
		failures += 1
		printerr("FAIL: " + message)


func _cleanup() -> void:
	var directory := DirAccess.open(TEMP_DIR)
	if directory == null:
		return
	directory.list_dir_begin()
	var entry := directory.get_next()
	while entry != "":
		if not directory.current_is_dir():
			directory.remove(entry)
		entry = directory.get_next()
	directory.list_dir_end()
	DirAccess.remove_absolute(ProjectSettings.globalize_path(TEMP_DIR))


func _test_explicit_multiline_label() -> void:
	var root: Control = await _add_root(Vector2(300, 200))
	var label := _make_label("Multiline", Vector2.ZERO, Vector2(120, 80), "original", "multiline")
	label.clip_text = true
	root.add_child(label)
	var catalog := L10nCatalog.new()
	_expect(catalog.load_mapping({"multiline": "Short line\nShort line"}, "en"), "multiline catalog loads")
	var checker := L10nTextChecker.new()
	var report := await checker.check_scene(root, catalog, "en")
	_expect(report["issues"].is_empty(), "each explicit line fits without a false width overflow")
	root.queue_free()
	await process_frame
