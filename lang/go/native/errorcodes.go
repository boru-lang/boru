package native

import (
	core "github.com/boru-lang/boru/core/go"
)

// Error-code registration for the language layer — the codes every built-in
// word, module and capability attaches. The mechanism, the naming rule and
// the stability contract all live in eng/go/errorcodes.go; this file is the
// lang half of the layered registration described there.
//
// The list is complete for this layer and is kept complete by the
// bidirectional gate in test/go/docexamples/errorcodes_test.go: a code minted
// by a site under lang/go but missing here fails the gate, and so does an
// entry here that no site mints. That is the point — before it existed,
// REFERENCE.md documented seven codes nothing could produce, and a `case` arm
// on any of them silently never fired.
//
// Codes the KERNEL owns are not repeated here even where a lang site raises
// them (`type_error`, `undefined_word`, `index_out_of_range`, and nine more).
// Re-registering one is refused as a double-owned code, deliberately: a lang
// site raising `type_error` is raising the kernel's error, not defining its
// own, and two layers each claiming a code is the drift the enumeration
// exists to catch.
//
// Ownership id: the language layer registers under OwnerLang so tooling can
// tell a kernel code from a built-in-word code — `boru explain` wants to say
// which layer a failure came from, and that is not derivable from the name.
var langErrorCodes = []string{
	"afn_error", "already_running", "append_error", "args_error",
	"arith_error", "as_error", "assertion_failure", "at_error",
	"await_error", "bad-encoding", "bad_widget", "behave_error",
	"binary_error", "body_error", "bytes_error", "cancel-interval_error",
	"cancel-timeout_error", "capability_not_installed", "case_error", "check_error",
	"check_prop_error", "class_error", "close_error", "closed",
	"codec_error", "compile_failed", "const_error", "context_error",
	"convert_error", "copy_error", "create_error", "debug_error",
	"decode_error", "default_outside_gen", "del_error", "do_error",
	"each_error", "eachrank_error", "emit_bad_signature", "emit_error",
	"emit_no_natural", "emit_registry_frozen", "emit_unknown_lang", "emit_unsupported",
	"env_error", "error_error", "expand_error", "expected-byte",
	"export_error",
	"exit_error", "exposes_error", "extends_error", "extends_outside_gen",
	"fetch_error",
	"filter_error", "flex_error", "flush_error", "fn_error",
	"fnpred_invalid_spec",
	"fnsig_invalid_spec", "fold_error", "foldaxis_error", "gen_error",
	"gen_unsupported_constructor", "get_error", "getr_error", "if_error",
	"import_error", "inner_error", "insert_at_error", "internal",
	"interval_error", "iota_error", "is_tty_error", "join_error",
	"key_error",
	"link_error", "list_error", "load_error", "lock_error",
	"log_error", "macro_not_expandable", "math_error", "merge_error",
	"mini_bad_signature", "mini_error", "mini_eval_error", "mini_parse_error",
	"mini_registry_frozen", "mini_syntax_error", "mini_unknown_lang", "mmap_error",
	"model_bad_action", "model_bad_handle", "model_bad_spec", "model_io",
	"model_not_built", "module_body_executed_in_check", "module_error", "mount_error",
	"move_error", "net_error", "no_backend", "no_match",
	"node_error", "not_found", "not_implemented", "not_sendable",
	"open_error", "outer_error", "overload", "pad_error",
	"parse_bad_abnf", "parse_bad_action", "parse_bad_grammar", "parse_bad_matcher",
	"parse_bad_rule", "parse_bad_signature", "parse_bad_source", "parse_bad_spec",
	"parse_bad_token", "parse_error", "parse_file_unsupported", "parse_grammar_done",
	"parse_registry_frozen", "parse_syntax_error", "parse_unknown_lang", "patrun_error",
	"permission_denied", "pop_error", "process_error", "push_error",
	"raise_error", "rand_error", "range_error", "reach_error",
	"read_error", "receive_error", "record_error", "referent_error",
	"refine_error", "reify_error", "remove_at_error", "remove_error",
	"replicate_error", "reshape_error", "scan_error", "sealed_field",
	"seek_error", "service_error", "set_error", "setpath_error",
	"shift_error", "sink-exists", "sleep_error", "space_error",
	"span-mismatch", "spawn_error", "splice_error", "stat_error",
	"string_option_error",
	"static_warning", "surface_error", "surface_unsatisfied", "tall_error",
	"tany_error", "temp_error", "terminal", "test_cover_bad_source",
	"test_cover_no_source", "timeout", "timeout_error", "touch_error",
	"transport", "transpose_error", "tui_error", "unaligned",
	"unconditional_raise", "undef_error", "unknown-sink", "unlock_error",
	"unmount_error", "unpack_error", "unquote_error", "unreachable_branch",
	"unreachable_signature", "unshift_error", "unwatch_error", "update_error",
	"user_error", "value_error", "var_error", "vault",
	"vault_error", "vault_usage", "walk_error", "watch_error", "window_error",
	"write_error", "xml_attr_error", "xml_elem_error", "xml_text_error",
}

var _ = registerLangErrorCodes()

func registerLangErrorCodes() struct{} {
	core.RegisterErrorCodes(OwnerLang, langErrorCodes...)
	return struct{}{}
}
