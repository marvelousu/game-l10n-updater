class_name L10nCli
extends RefCounted

## Small argument parser shared by the demo and the headless runner.

const USAGE: String = """L10n text check options:
  --report-path <path>   Write the report here (absolute, user://, res:// is refused).
  --catalog <path>       Flat target JSON or Godot CSV catalog.
  --locale <locale>      Locale column/name to check (for example: en, ja).
  --capture-path <path>  Save a rendered PNG when a display server is available.
  --check-glyphs         Also report font-reported missing glyphs (opt-in).
  --check-only           Run the check, write the report, then quit.
  --help                 Show this help.

Pass app arguments after -- so Godot does not consume them:
  godot --headless --path godot -- --check-only --report-path D:/tmp/report.json
"""


static func app_args() -> PackedStringArray:
	var user_args := OS.get_cmdline_user_args()
	if not user_args.is_empty():
		return user_args

	# Forgiving fallback for flags passed before "--". Only keep options this
	# tool understands; Godot's own --headless/--path/--script are ignored.
	var filtered: PackedStringArray = []
	var raw_args := OS.get_cmdline_args()
	var index := 0
	while index < raw_args.size():
		var arg := raw_args[index]
		if arg in ["--report-path", "--catalog", "--locale", "--capture-path"]:
			filtered.append(arg)
			if index + 1 < raw_args.size():
				filtered.append(raw_args[index + 1])
				index += 2
				continue
		elif arg in ["--check-only", "--check-glyphs", "--help", "-h"]:
			filtered.append(arg)
		index += 1
	return filtered


static func parse(argv: PackedStringArray) -> Dictionary:
	var unknown_options: Array[String] = []
	var options: Dictionary = {
		"report_path": "",
		"catalog_path": "",
		"locale": "",
		"capture_path": "",
		"check_only": false,
		"check_glyphs": false,
		"help": false,
		"unknown": unknown_options,
	}

	var index := 0
	while index < argv.size():
		var arg := argv[index]
		if arg in ["--report-path", "--catalog", "--locale", "--capture-path"]:
			if index + 1 >= argv.size():
				unknown_options.append("%s (missing value)" % arg)
				index += 1
				continue
			var value := argv[index + 1]
			match arg:
				"--report-path":
					options["report_path"] = value
				"--catalog":
					options["catalog_path"] = value
				"--locale":
					options["locale"] = value
				"--capture-path":
					options["capture_path"] = value
			index += 2
			continue

		match arg:
			"--check-only":
				options["check_only"] = true
			"--check-glyphs":
				options["check_glyphs"] = true
			"--help", "-h":
				options["help"] = true
			_:
				unknown_options.append(arg)
		index += 1

	options["unknown"] = unknown_options
	return options
