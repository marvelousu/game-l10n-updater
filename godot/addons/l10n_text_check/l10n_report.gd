class_name L10nReport
extends RefCounted

## Builds and writes the display-check report consumed by game-l10n-updater.
##
## Schema:
## {
##   "schema_version": 1,
##   "issues": [{"key": "...", "locale": "...", "node_path": "...",
##               "kind": "...", "message": "..."}],
##   "checked": 0
## }

const SCHEMA_VERSION: int = 1


static func build(issues: Array[Dictionary], checked: int) -> Dictionary:
	return {
		"schema_version": SCHEMA_VERSION,
		"issues": issues,
		"checked": checked,
	}


static func issue(
		key: String,
		locale: String,
		node_path: String,
		kind: String,
		message: String
) -> Dictionary:
	return {
		"key": key,
		"locale": locale,
		"node_path": node_path,
		"kind": kind,
		"message": message,
	}


## Writes a report. Returns "" on success, otherwise a human-readable error.
## Generated reports are refused under res:// so fixtures do not land in source.
static func write(report: Dictionary, path: String) -> String:
	if path.strip_edges().is_empty():
		return "report path is empty"
	var clean_path := path.strip_edges()
	if clean_path.begins_with("res://"):
		return "refusing to write a generated report under res://: %s" % clean_path

	var dir_path := clean_path.get_base_dir()
	if not dir_path.is_empty():
		var absolute_dir := ProjectSettings.globalize_path(dir_path)
		if not DirAccess.dir_exists_absolute(absolute_dir):
			var make_error := DirAccess.make_dir_recursive_absolute(absolute_dir)
			if make_error != OK:
				return "cannot create report directory %s: %s" % [
					absolute_dir, error_string(make_error)
				]

	var file := FileAccess.open(clean_path, FileAccess.WRITE)
	if file == null:
		return "cannot open report path %s: %s" % [
			clean_path, error_string(FileAccess.get_open_error())
		]
	file.store_string(JSON.stringify(report, "  ", false) + "\n")
	file.close()
	return ""
