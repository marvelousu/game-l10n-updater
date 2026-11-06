@tool
extends EditorPlugin


func _enter_tree() -> void:
	# The addon is runtime-only. Enabling it is optional; the class_name
	# scripts are available to a project even when this editor plugin is off.
	pass


func _exit_tree() -> void:
	pass
