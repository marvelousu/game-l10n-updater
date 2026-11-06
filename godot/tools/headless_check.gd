extends SceneTree

## Example automated runner. It checks the demo scene with the seeded catalog,
## expects exactly the intentionally narrow Japanese button to be clipped, and
## expects the fitting English strings to pass. Unexpected results exit 1.
##
## Default:
##   godot --headless --path godot --script res://tools/headless_check.gd -- \
##     --report-path D:/tmp/demo-report.json
##
## External catalog (report generation only, no seeded expectation):
##   godot --headless --path godot --script res://tools/headless_check.gd -- \
##     --catalog D:/tmp/catalog_en.json --locale en \
##     --report-path D:/tmp/report.json

const DEMO_SCENE := preload("res://demo/main.tscn")


func _initialize() -> void:
	call_deferred("_run")


func _run() -> void:
	var options := L10nCli.parse(L10nCli.app_args())
	if bool(options["help"]):
		print(L10nCli.USAGE)
		quit(0)
		return

	var catalog_path := String(options["catalog_path"])
	var requested_locale := String(options["locale"])
	var report_path := String(options["report_path"])
	var capture_path := String(options["capture_path"])
	var check_glyphs := bool(options["check_glyphs"])
	var external_catalog := not catalog_path.is_empty()

	var unknown: Array[String] = options["unknown"]
	if not unknown.is_empty():
		push_warning("Ignoring unknown options: %s" % ", ".join(unknown))

	var demo := DEMO_SCENE.instantiate()
	demo.set("runner_owned", true)
	demo.set("check_glyphs", check_glyphs)
	demo.set("auto_check", false)
	if external_catalog:
		demo.set("catalog_path_override", catalog_path)
		demo.set("locale_override", requested_locale)
	elif not requested_locale.is_empty():
		demo.set("locale_override", requested_locale)

	get_root().add_child(demo)
	await process_frame
	await process_frame
	await process_frame

	var load_path := catalog_path
	var load_locale := requested_locale
	if not external_catalog:
		load_path = "res://demo/translations.csv"
		if load_locale.is_empty():
			load_locale = "ja"

	var catalog := L10nCatalog.new()
	if not catalog.load_path(load_path, load_locale):
		printerr("Catalog error: %s" % catalog.last_error)
		quit(2)
		return

	var checker := L10nTextChecker.new()
	var issues: Array[Dictionary] = []
	var checked := 0
	var ja_report: Dictionary = {}
	var en_report: Dictionary = {}

	if external_catalog:
		var report := await checker.check_scene(
			demo, catalog, catalog.locale,
			{"check_glyphs": check_glyphs, "settle_frames": 2}
		)
		if not checker.last_error.is_empty():
			printerr("Check error: %s" % checker.last_error)
			quit(2)
			return
		_append_issues(issues, report)
		checked += int(report["checked"])
	else:
		ja_report = await checker.check_scene(
			demo, catalog, "ja",
			{"check_glyphs": check_glyphs, "settle_frames": 2}
		)
		if not checker.last_error.is_empty():
			printerr("Japanese check error: %s" % checker.last_error)
			quit(2)
			return
		en_report = await checker.check_scene(
			demo, catalog, "en",
			{"check_glyphs": check_glyphs, "settle_frames": 2}
		)
		if not checker.last_error.is_empty():
			printerr("English check error: %s" % checker.last_error)
			quit(2)
			return
		_append_issues(issues, ja_report)
		_append_issues(issues, en_report)
		checked += int(ja_report["checked"]) + int(en_report["checked"])

	var combined := L10nReport.build(issues, checked)
	if report_path.is_empty():
		report_path = "user://l10n_text_check/headless-demo-report.json"
	var write_error := L10nReport.write(combined, report_path)
	if not write_error.is_empty():
		printerr(write_error)
		quit(2)
		return

	print("L10n report: %s" % ProjectSettings.globalize_path(report_path))
	print("L10n check: checked=%d issues=%d catalog=%s locale=%s" % [
		checked, issues.size(), catalog.source_path, catalog.locale
	])

	var failures := 0
	if external_catalog:
		print("External catalog mode: report generated; seeded demo expectations were not enforced.")
	else:
		failures += _check_seeded_expectations(ja_report, en_report, check_glyphs)
	if not capture_path.is_empty():
		if DisplayServer.get_name() == "headless":
			print("Screenshot skipped: headless display server; the JSON report is still valid.")
		else:
			var capture_error: String = await demo.call("capture_screenshot", capture_path)
			if not capture_error.is_empty():
				printerr(capture_error)
				failures += 1

	demo.queue_free()
	await process_frame

	if failures > 0:
		printerr("RESULT: FAIL (%d unexpected outcome(s))" % failures)
		quit(1)
		return

	if external_catalog:
		print("RESULT: PASS (external catalog report generated; no seeded expectations enforced)")
	else:
		print("RESULT: PASS (one intentionally clipped button detected in ja; fitting en strings passed)")
	quit(0)


func _append_issues(target: Array[Dictionary], report: Dictionary) -> void:
	var report_issues: Array = report["issues"]
	for raw_issue in report_issues:
		var issue: Dictionary = raw_issue
		target.append(issue)


func _check_seeded_expectations(
		ja_report: Dictionary,
		en_report: Dictionary,
		ignore_missing_glyphs: bool
) -> int:
	var failures := 0
	var ja_issues: Array = ja_report["issues"]
	var en_issues: Array = en_report["issues"]
	if ignore_missing_glyphs:
		ja_issues = _without_kind(ja_issues, "missing_glyph")
		en_issues = _without_kind(en_issues, "missing_glyph")
		print("Glyph-check mode: font-chain missing_glyph issues are advisory and excluded from the seeded layout gate.")

	if int(ja_report["checked"]) <= 0 or int(en_report["checked"]) <= 0:
		printerr("FAIL: expected non-zero checked counts for both locales.")
		failures += 1

	if ja_issues.size() != 1:
		printerr("FAIL: expected exactly one Japanese issue, got %d." % ja_issues.size())
		_print_issues(ja_issues)
		failures += 1
	else:
		var issue: Dictionary = ja_issues[0]
		if String(issue["key"]) != "demo.tight_action":
			printerr("FAIL: expected clipped key demo.tight_action, got %s." % String(issue["key"]))
			failures += 1
		if String(issue["locale"]) != "ja":
			printerr("FAIL: expected locale ja, got %s." % String(issue["locale"]))
			failures += 1
		var kind := String(issue["kind"])
		if kind != "clipped" and kind != "overflow":
			printerr("FAIL: expected clipped/overflow, got %s." % kind)
			failures += 1

	if not en_issues.is_empty():
		printerr("FAIL: expected the fitting English strings to pass, got %d issue(s)." % en_issues.size())
		_print_issues(en_issues)
		failures += 1

	return failures


func _print_issues(issues: Array) -> void:
	for raw_issue in issues:
		var issue: Dictionary = raw_issue
		printerr("  [%s] %s %s: %s" % [
			String(issue["kind"]),
			String(issue["locale"]),
			String(issue["key"]),
			String(issue["message"]),
		])


func _without_kind(issues: Array, ignored_kind: String) -> Array:
	var filtered: Array = []
	for raw_issue in issues:
		var issue: Dictionary = raw_issue
		if String(issue["kind"]) != ignored_kind:
			filtered.append(issue)
	return filtered
