class_name L10nCatalog
extends RefCounted

## Loads either a flat target JSON object or a Godot CSV translation file.
##
## Flat JSON: {"key": "translated text", ...} for one locale.
## Godot CSV: first header cell is "keys" (or "key"), following header cells
##            are locale names, for example: keys,en,ja

var source_path: String = ""
var source_format: String = ""
var locale: String = ""
var available_locales: PackedStringArray = []
var warnings: PackedStringArray = []
var last_error: String = ""

var _columns: Dictionary = {}
var _key_order: PackedStringArray = []


func load_path(path: String, requested_locale: String = "") -> bool:
	_reset()
	source_path = path
	match path.get_extension().to_lower():
		"json":
			return _read_flat_json(path, requested_locale)
		"csv":
			return _read_csv(path, requested_locale)
		_:
			return _fail("unsupported catalog extension for %s (expected .json or .csv)" % path)


func load_flat_json(path: String, requested_locale: String = "") -> bool:
	_reset()
	source_path = path
	return _read_flat_json(path, requested_locale)


func load_csv_file(path: String, requested_locale: String = "") -> bool:
	_reset()
	source_path = path
	return _read_csv(path, requested_locale)


## Builds a single-locale catalog from an in-memory map. Useful for tests and
## for embedding a small generated catalog at runtime.
func load_mapping(texts: Dictionary, mapping_locale: String = "en") -> bool:
	_reset()
	var normalized: Dictionary = {}
	for raw_key in texts.keys():
		if typeof(raw_key) != TYPE_STRING:
			return _fail("catalog keys must be strings")
		if String(raw_key).is_empty():
			return _fail("catalog keys must not be empty")
		var raw_value: Variant = texts[raw_key]
		if typeof(raw_value) != TYPE_STRING:
			return _fail("catalog value for '%s' is not a string" % String(raw_key))
		normalized[String(raw_key)] = String(raw_value)
	var chosen_locale := mapping_locale.strip_edges()
	if chosen_locale.is_empty():
		chosen_locale = "en"
	return _install_columns(
		{chosen_locale: normalized},
		PackedStringArray([chosen_locale]),
		chosen_locale,
		"",
		"mapping"
	)


func select_locale(requested_locale: String) -> bool:
	last_error = ""
	var wanted := requested_locale.strip_edges()
	if wanted.is_empty():
		last_error = "locale is empty"
		return false

	if _columns.has(wanted):
		locale = wanted
		return true

	var wanted_key := _locale_key(wanted)
	for available in available_locales:
		if _locale_key(String(available)) == wanted_key:
			locale = String(available)
			return true

	last_error = "locale '%s' is not in the catalog (available: %s)" % [
		wanted, ", ".join(available_locales)
	]
	return false


func has_locale(requested_locale: String) -> bool:
	var wanted := requested_locale.strip_edges()
	if wanted.is_empty():
		return false
	if _columns.has(wanted):
		return true
	var wanted_key := _locale_key(wanted)
	for available in available_locales:
		if _locale_key(String(available)) == wanted_key:
			return true
	return false


func has_key(key: String) -> bool:
	if locale.is_empty():
		return false
	var column: Dictionary = _columns.get(locale, {})
	return column.has(key)


func get_text(key: String) -> String:
	if locale.is_empty():
		return ""
	var column: Dictionary = _columns.get(locale, {})
	if not column.has(key):
		return ""
	return String(column[key])


func keys() -> PackedStringArray:
	return _key_order


func key_count() -> int:
	return _key_order.size()


func _read_flat_json(path: String, requested_locale: String) -> bool:
	var text := _read_text(path)
	if not last_error.is_empty():
		return false
	text = _strip_bom(text)

	var json := JSON.new()
	var parse_error := json.parse(text)
	if parse_error != OK:
		return _fail("JSON parse error in %s at line %d: %s" % [
			path, json.get_error_line(), json.get_error_message()
		])

	var data: Variant = json.data
	if typeof(data) != TYPE_DICTIONARY:
		return _fail("flat target JSON must be an object of key -> string: %s" % path)

	var normalized: Dictionary = {}
	var dictionary: Dictionary = data
	for raw_key in dictionary.keys():
		if typeof(raw_key) != TYPE_STRING:
			return _fail("JSON catalog keys must be strings: %s" % path)
		if String(raw_key).is_empty():
			return _fail("JSON catalog keys must not be empty: %s" % path)
		var raw_value: Variant = dictionary[raw_key]
		if typeof(raw_value) != TYPE_STRING:
			return _fail("JSON value for '%s' is not a string (flat target JSON only): %s" % [
				String(raw_key), path
			])
		normalized[String(raw_key)] = String(raw_value)

	var chosen_locale := requested_locale.strip_edges()
	if chosen_locale.is_empty():
		chosen_locale = "en"
	return _install_columns(
		{chosen_locale: normalized},
		PackedStringArray([chosen_locale]),
		chosen_locale,
		path,
		"json"
	)


func _read_csv(path: String, requested_locale: String) -> bool:
	var file := FileAccess.open(path, FileAccess.READ)
	if file == null:
		return _fail("cannot open catalog %s: %s" % [
			path, error_string(FileAccess.get_open_error())
		])

	var header := file.get_csv_line(",")
	if header.is_empty():
		file.close()
		return _fail("CSV catalog is empty: %s" % path)

	header[0] = _strip_bom(header[0]).strip_edges()
	var first_header := header[0].to_lower()
	if first_header != "keys" and first_header != "key":
		file.close()
		return _fail("Godot CSV must start with a 'keys' header cell: %s" % path)

	var locale_names: PackedStringArray = []
	var locale_indexes: Array[int] = []
	for column_index in range(1, header.size()):
		var locale_name := header[column_index].strip_edges()
		if locale_name.is_empty():
			warnings.append("empty locale header at column %d; skipped" % (column_index + 1))
			continue
		if locale_names.has(locale_name):
			warnings.append("duplicate locale header '%s'; first column is used" % locale_name)
			continue
		locale_names.append(locale_name)
		locale_indexes.append(column_index)

	if locale_names.is_empty():
		file.close()
		return _fail("CSV catalog has no locale columns: %s" % path)

	var columns: Dictionary = {}
	for locale_name in locale_names:
		columns[locale_name] = {}

	var first_column: Dictionary = columns[locale_names[0]]
	var data_row := 0
	while not file.eof_reached():
		var row := file.get_csv_line(",")
		if row.is_empty():
			continue
		if row.size() == 1 and row[0].strip_edges().is_empty():
			continue
		data_row += 1

		var key := _strip_bom(row[0])
		if key.is_empty():
			warnings.append("row %d has an empty key; skipped" % data_row)
			continue
		if first_column.has(key):
			warnings.append("duplicate key '%s'; the later row wins" % key)

		for column_index in range(locale_names.size()):
			var locale_name := locale_names[column_index]
			var value := ""
			var value_index := locale_indexes[column_index]
			if value_index < row.size():
				value = row[value_index]
			else:
				warnings.append("row '%s' is missing column '%s'; using an empty string" % [
					key, locale_name
				])
			var column: Dictionary = columns[locale_name]
			column[key] = value
	file.close()

	var chosen_locale := requested_locale.strip_edges()
	if chosen_locale.is_empty():
		chosen_locale = locale_names[0]
	return _install_columns(columns, locale_names, chosen_locale, path, "csv")


func _install_columns(
		columns: Dictionary,
		locale_names: PackedStringArray,
		requested_locale: String,
		path: String,
		format: String
) -> bool:
	_columns = columns
	available_locales = locale_names
	source_path = path
	source_format = format
	_key_order = PackedStringArray()

	if locale_names.is_empty():
		return _fail("catalog has no locales")

	var first_column: Dictionary = _columns[locale_names[0]]
	for key in first_column.keys():
		_key_order.append(String(key))

	if not select_locale(requested_locale):
		return false
	return true


func _read_text(path: String) -> String:
	var file := FileAccess.open(path, FileAccess.READ)
	if file == null:
		_fail("cannot open catalog %s: %s" % [path, error_string(FileAccess.get_open_error())])
		return ""
	var text := file.get_as_text()
	file.close()
	return text


func _reset() -> void:
	source_path = ""
	source_format = ""
	locale = ""
	available_locales = PackedStringArray()
	warnings = PackedStringArray()
	last_error = ""
	_columns.clear()
	_key_order = PackedStringArray()


func _fail(message: String) -> bool:
	last_error = message
	return false


func _locale_key(value: String) -> String:
	return value.strip_edges().to_lower().replace("-", "_")


func _strip_bom(value: String) -> String:
	if value.begins_with(String.chr(0xFEFF)):
		return value.substr(1)
	return value
