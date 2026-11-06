class_name L10nTextChecker
extends RefCounted

## Runtime display checker for explicitly opted-in Label, Button and
## RichTextLabel nodes.
##
## Opt in by setting metadata:
##   l10n_key = "catalog.key"          (required)
##   l10n_check_glyphs = true          (optional per-node glyph check)
##
## The checker temporarily applies catalog strings, waits for layout frames,
## inspects the real theme font and Control rect, then restores every node's
## original text and auto-translate mode. It never changes TranslationServer's
## locale permanently.

const TOLERANCE: float = 1.0
const DEFAULT_SETTLE_FRAMES: int = 2
const MAX_REPORTED_GLYPHS: int = 8

var last_error: String = ""


## Scans root (inclusive) and returns nodes that carry l10n_key metadata.
func collect_opted_in_nodes(root: Node) -> Array[Node]:
	var result: Array[Node] = []
	if root == null or not is_instance_valid(root):
		return result
	_collect(root, result)
	return result


## Applies the catalog's selected locale to opted-in nodes. This is a normal
## runtime helper; it does not restore text. Returns missing-key records.
func apply_catalog(root: Node, catalog: L10nCatalog) -> Array[Dictionary]:
	var missing: Array[Dictionary] = []
	if root == null or not is_instance_valid(root) or catalog == null:
		return missing
	for node in collect_opted_in_nodes(root):
		var key := _get_key(node)
		var node_path := _node_path(root, node)
		if key.is_empty() or not catalog.has_key(key):
			missing.append({"key": key, "node_path": node_path})
			continue
		if _supports(node):
			_set_text(node, catalog.get_text(key))
	return missing


## Checks one scene subtree for one locale. Await this function.
func check_scene(
		root: Node,
		catalog: L10nCatalog,
		requested_locale: String = "",
		options: Dictionary = {}
) -> Dictionary:
	last_error = ""
	if root == null or not is_instance_valid(root):
		last_error = "check root is not valid"
		return L10nReport.build([], 0)
	if catalog == null:
		last_error = "catalog is null"
		return L10nReport.build([], 0)

	var tree := root.get_tree()
	if tree == null:
		last_error = "check root is not inside a SceneTree"
		return L10nReport.build([], 0)

	var report_locale := catalog.locale
	var catalog_locale_before := catalog.locale
	if not requested_locale.strip_edges().is_empty():
		if not catalog.select_locale(requested_locale):
			last_error = catalog.last_error
			return L10nReport.build([], 0)
		report_locale = catalog.locale
	if report_locale.is_empty():
		last_error = "catalog has no selected locale"
		return L10nReport.build([], 0)

	var settle_frames := clampi(int(options.get("settle_frames", DEFAULT_SETTLE_FRAMES)), 1, 10)
	var check_glyphs := bool(options.get("check_glyphs", false))
	var locale_before := TranslationServer.get_locale()

	var issues: Array[Dictionary] = []
	var snapshots: Array[Dictionary] = []
	var checked := 0

	for node in collect_opted_in_nodes(root):
		var key := _get_key(node)
		var node_path := _node_path(root, node)
		checked += 1

		if key.is_empty():
			issues.append(L10nReport.issue(
				"", report_locale, node_path, "missing_key",
				"Opted-in node has empty or non-string l10n_key metadata."
			))
			continue
		if not catalog.has_key(key):
			issues.append(L10nReport.issue(
				key, report_locale, node_path, "missing_key",
				"Catalog '%s' has no entry for key '%s' in locale '%s'." % [
					catalog.source_path.get_file(), key, report_locale
				]
			))
			continue
		if not _supports(node):
			issues.append(L10nReport.issue(
				key, report_locale, node_path, "unsupported_control",
				"Opted-in node type '%s' is not measured; supported types are Label, plain Button and RichTextLabel." % node.get_class()
			))
			continue
		if node is CanvasItem and not (node as CanvasItem).is_visible_in_tree():
			issues.append(L10nReport.issue(
				key, report_locale, node_path, "unsupported_mode",
				"Opted-in node is hidden in the tree; it is not rendered, so no display result is claimed."
			))
			continue

		snapshots.append(_snapshot(node, key, node_path))
		_set_text(node, catalog.get_text(key))

	if not snapshots.is_empty():
		await _settle(tree, settle_frames)
		for snapshot in snapshots:
			var node := snapshot["node"] as Node
			if node == null or not is_instance_valid(node):
				continue
			var key := String(snapshot["key"])
			var node_path := String(snapshot["path"])
			issues.append_array(_inspect_node(node, key, report_locale, node_path, check_glyphs))

	_restore_snapshots(snapshots)
	if not snapshots.is_empty():
		await _settle(tree, 1)

	if TranslationServer.get_locale() != locale_before:
		TranslationServer.set_locale(locale_before)
	if not catalog_locale_before.is_empty() and catalog.locale != catalog_locale_before:
		catalog.select_locale(catalog_locale_before)

	return L10nReport.build(issues, checked)


## Font-reported missing glyph scan. Checks the effective theme font and
## ThemeDB's fallback font. System-level fallback fonts cannot be enumerated
## reliably, so callers should treat this as opt-in evidence, not proof.
static func find_missing_glyphs(font: Font, text: String) -> PackedInt32Array:
	var missing: PackedInt32Array = []
	if text.is_empty():
		return missing
	var fallback: Font = ThemeDB.fallback_font
	var seen: Dictionary = {}
	for index in text.length():
		var codepoint := text.unicode_at(index)
		if codepoint < 32 or codepoint == 127:
			continue
		if seen.has(codepoint):
			continue
		seen[codepoint] = true
		var covered := font != null and font.has_char(codepoint)
		if not covered and fallback != null:
			covered = fallback.has_char(codepoint)
		if not covered:
			missing.append(codepoint)
	return missing


func _collect(node: Node, result: Array[Node]) -> void:
	if node.has_meta("l10n_key"):
		result.append(node)
	for child in node.get_children():
		_collect(child, result)


func _supports(node: Node) -> bool:
	return node is Label or node is Button or node is RichTextLabel


func _get_key(node: Node) -> String:
	if not node.has_meta("l10n_key"):
		return ""
	var raw: Variant = node.get_meta("l10n_key")
	if typeof(raw) != TYPE_STRING:
		return ""
	return String(raw).strip_edges()


func _set_text(node: Node, text: String) -> void:
	if node is Label:
		(node as Label).text = text
	elif node is Button:
		(node as Button).text = text
	elif node is RichTextLabel:
		(node as RichTextLabel).text = text


func _read_text(node: Node) -> String:
	if node is Label:
		return (node as Label).text
	if node is Button:
		return (node as Button).text
	if node is RichTextLabel:
		return (node as RichTextLabel).text
	return ""


func _snapshot(node: Node, key: String, node_path: String) -> Dictionary:
	var snapshot: Dictionary = {
		"node": node,
		"key": key,
		"path": node_path,
		"text": _read_text(node),
		"auto_translate_mode": node.auto_translate_mode,
	}
	node.auto_translate_mode = Node.AUTO_TRANSLATE_MODE_DISABLED
	return snapshot


func _restore_snapshots(snapshots: Array[Dictionary]) -> void:
	for snapshot in snapshots:
		var node := snapshot["node"] as Node
		if node == null or not is_instance_valid(node):
			continue
		_set_text(node, String(snapshot["text"]))
		node.auto_translate_mode = int(snapshot["auto_translate_mode"])


func _inspect_node(
		node: Node,
		key: String,
		locale: String,
		node_path: String,
		check_glyphs: bool
) -> Array[Dictionary]:
	if node is Label:
		return _inspect_label(node as Label, key, locale, node_path, check_glyphs)
	if node is Button:
		return _inspect_button(node as Button, key, locale, node_path, check_glyphs)
	if node is RichTextLabel:
		return _inspect_richtext(node as RichTextLabel, key, locale, node_path)
	return []


func _inspect_label(
		label: Label,
		key: String,
		locale: String,
		node_path: String,
		check_glyphs: bool
) -> Array[Dictionary]:
	var result: Array[Dictionary] = []
	var text := label.text
	if text.is_empty():
		return result

	var font := _theme_font(label, &"font")
	var font_size := _theme_font_size(label, &"font_size")
	if font == null:
		return [L10nReport.issue(
			key, locale, node_path, "unsupported_mode",
			"Label has no resolvable theme font, so rendered width cannot be checked."
		)]

	var available := _content_size(label)
	var trims := _label_trims(label)
	var total_lines := label.get_line_count()
	var visible_lines := label.get_visible_line_count()
	var line_height := float(font.get_height(font_size))
	if label.has_theme_constant(&"line_spacing"):
		line_height += float(label.get_theme_constant(&"line_spacing"))
	var required_height := line_height * float(maxi(total_lines, 1))
	var vertically_too_small := required_height > available.y + TOLERANCE

	if total_lines > visible_lines or vertically_too_small:
		var kind := "clipped" if trims else "overflow"
		result.append(L10nReport.issue(
			key, locale, node_path, kind,
			"Label needs %.1f px of height for %d line(s) but has %.1f px (visible lines: %d, clip_text=%s)." % [
				required_height, total_lines, available.y, visible_lines, str(label.clip_text)
			]
		))
	elif label.autowrap_mode == TextServer.AUTOWRAP_OFF:
		var text_size := font.get_multiline_string_size(
			text, HORIZONTAL_ALIGNMENT_LEFT, -1, font_size
		)
		if text_size.x > available.x + TOLERANCE:
			var kind := "clipped" if trims else "overflow"
			result.append(L10nReport.issue(
				key, locale, node_path, kind,
				"Label text is %.1f px wide but only %.1f px is available (no wrapping)." % [
					text_size.x, available.x
				]
			))
	else:
		var wrap_width := available.x if available.x > 0.0 else -1.0
		var multiline_size := font.get_multiline_string_size(
			text, HORIZONTAL_ALIGNMENT_LEFT, wrap_width, font_size
		)
		if multiline_size.x > available.x + TOLERANCE:
			var kind := "clipped" if trims else "overflow"
			result.append(L10nReport.issue(
				key, locale, node_path, kind,
				"Wrapped Label has a %.1f px line but only %.1f px is available." % [
					multiline_size.x, available.x
				]
			))

	result.append_array(_glyph_issues(
		label, key, locale, node_path, text, font, check_glyphs
	))
	return result


func _inspect_button(
		button: Button,
		key: String,
		locale: String,
		node_path: String,
		check_glyphs: bool
) -> Array[Dictionary]:
	var result: Array[Dictionary] = []
	var text := button.text
	if text.is_empty():
		return result
	if button.get_class() != "Button":
		return [L10nReport.issue(
			key, locale, node_path, "unsupported_mode",
			"Button subclass '%s' draws extra decorations or icons that this checker does not model; use a plain Button or extend the measurement." % button.get_class()
		)]

	var font := _theme_font(button, &"font")
	var font_size := _theme_font_size(button, &"font_size")
	if font == null:
		return [L10nReport.issue(
			key, locale, node_path, "unsupported_mode",
			"Button has no resolvable theme font, so rendered width cannot be checked."
		)]

	var available := _content_size(button)
	var icon_width := 0.0
	if button.icon != null:
		if button.expand_icon:
			return [L10nReport.issue(
				key, locale, node_path, "unsupported_mode",
				"Button uses expand_icon; its final text area cannot be measured reliably."
			)]
		icon_width = float(button.icon.get_width())
		if button.has_theme_constant(&"icon_max_width"):
			var icon_max_width := int(button.get_theme_constant(&"icon_max_width"))
			if icon_max_width > 0:
				icon_width = minf(icon_width, float(icon_max_width))
	var separation := 0.0
	if icon_width > 0.0 and button.has_theme_constant(&"h_separation"):
		separation = float(button.get_theme_constant(&"h_separation"))
	var text_available := maxf(0.0, available.x - icon_width - separation)

	var autowrap_mode := _int_property(button, "autowrap_mode", TextServer.AUTOWRAP_OFF)
	if autowrap_mode != TextServer.AUTOWRAP_OFF:
		return [L10nReport.issue(
			key, locale, node_path, "unsupported_mode",
			"Button autowrap_mode=%d is not verified; use a single-line Button or check it in an engine-driven harness." % autowrap_mode
		)]
	var overrun_behavior := _int_property(
		button, "text_overrun_behavior", TextServer.OVERRUN_NO_TRIMMING
	)
	var trims := button.clip_text or overrun_behavior != TextServer.OVERRUN_NO_TRIMMING
	var text_size := font.get_string_size(
		text, HORIZONTAL_ALIGNMENT_LEFT, -1, font_size
	)

	if text_size.x > text_available + TOLERANCE:
		var kind := "clipped" if trims else "overflow"
		result.append(L10nReport.issue(
			key, locale, node_path, kind,
			"Button text is %.1f px wide but only %.1f px is available (autowrap=%d, clip_text=%s)." % [
				text_size.x, text_available, autowrap_mode, str(button.clip_text)
			]
		))
	elif text_size.y > available.y + TOLERANCE:
		var kind := "clipped" if trims else "overflow"
		result.append(L10nReport.issue(
			key, locale, node_path, kind,
			"Button text is %.1f px tall but only %.1f px is available (autowrap=%d)." % [
				text_size.y, available.y, autowrap_mode
			]
		))

	result.append_array(_glyph_issues(
		button, key, locale, node_path, text, font, check_glyphs
	))
	return result


func _inspect_richtext(
		richtext: RichTextLabel,
		key: String,
		locale: String,
		node_path: String
) -> Array[Dictionary]:
	var result: Array[Dictionary] = []
	var text := richtext.text
	if text.is_empty():
		return result

	var available := _content_size(richtext)
	var content_width := float(richtext.get_content_width())
	var content_height := float(richtext.get_content_height())
	var too_wide := (
		richtext.autowrap_mode == TextServer.AUTOWRAP_OFF
		and content_width > available.x + TOLERANCE
	)
	var too_tall := content_height > available.y + TOLERANCE

	if too_wide or too_tall:
		if richtext.scroll_active:
			result.append(L10nReport.issue(
				key, locale, node_path, "unsupported_mode",
				"RichTextLabel content (%.1fx%.1f px) exceeds its rect (%.1fx%.1f px) and scrolling is enabled; only the visible subset would render." % [
					content_width, content_height, available.x, available.y
				]
			))
		else:
			result.append(L10nReport.issue(
				key, locale, node_path, "clipped",
				"RichTextLabel content (%.1fx%.1f px) exceeds its rect (%.1fx%.1f px)." % [
					content_width, content_height, available.x, available.y
				]
			))
	return result


func _glyph_issues(
		node: Node,
		key: String,
		locale: String,
		node_path: String,
		text: String,
		font: Font,
		check_glyphs: bool
) -> Array[Dictionary]:
	var glyph_metadata: Variant = node.get_meta("l10n_check_glyphs", false)
	var per_node_glyphs := typeof(glyph_metadata) == TYPE_BOOL and bool(glyph_metadata)
	if not check_glyphs and not per_node_glyphs:
		return []
	var missing := find_missing_glyphs(font, text)
	if missing.is_empty():
		return []

	var labels: PackedStringArray = []
	var count := mini(missing.size(), MAX_REPORTED_GLYPHS)
	for index in count:
		labels.append("U+%04X" % missing[index])
	var suffix := ""
	if missing.size() > MAX_REPORTED_GLYPHS:
		suffix = " (+%d more)" % (missing.size() - MAX_REPORTED_GLYPHS)
	return [L10nReport.issue(
		key, locale, node_path, "missing_glyph",
		"Font chain reports no glyph for: %s%s. System fallback may still render these." % [
			", ".join(labels), suffix
		]
	)]


func _label_trims(label: Label) -> bool:
	if label.clip_text:
		return true
	return label.text_overrun_behavior != TextServer.OVERRUN_NO_TRIMMING


func _theme_font(control: Control, item: StringName) -> Font:
	var font: Font = control.get_theme_font(item)
	if font == null:
		font = ThemeDB.fallback_font
	return font


func _theme_font_size(control: Control, item: StringName) -> int:
	var size := control.get_theme_font_size(item)
	if size <= 0:
		size = ThemeDB.fallback_font_size
	return size


func _content_size(control: Control) -> Vector2:
	var width := control.size.x
	var height := control.size.y
	var style: StyleBox = null
	if control.has_theme_stylebox(&"normal"):
		style = control.get_theme_stylebox(&"normal")
	elif control.has_theme_stylebox(&"panel"):
		style = control.get_theme_stylebox(&"panel")
	if style != null:
		width -= style.get_margin(SIDE_LEFT) + style.get_margin(SIDE_RIGHT)
		height -= style.get_margin(SIDE_TOP) + style.get_margin(SIDE_BOTTOM)
	return Vector2(maxf(0.0, width), maxf(0.0, height))


func _node_path(root: Node, node: Node) -> String:
	if node == root:
		return "."
	return String(root.get_path_to(node))


func _int_property(object: Object, property_name: String, fallback: int) -> int:
	for property in object.get_property_list():
		if String(property["name"]) == property_name:
			return int(object.get(property_name))
	return fallback


func _settle(tree: SceneTree, frames: int) -> void:
	for _frame in frames:
		await tree.process_frame
