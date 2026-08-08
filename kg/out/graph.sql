BEGIN TRANSACTION;
CREATE TABLE bundle_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE input_files (path TEXT PRIMARY KEY, digest TEXT NOT NULL, chars INTEGER NOT NULL);
CREATE TABLE sources (id TEXT PRIMARY KEY, kind TEXT NOT NULL, locator TEXT NOT NULL, title TEXT, retrieved_at TEXT, content_hash TEXT NOT NULL, authority TEXT NOT NULL, metadata_json TEXT);
CREATE TABLE entities (id TEXT PRIMARY KEY, type TEXT NOT NULL, label TEXT NOT NULL, normalized_label TEXT NOT NULL, status TEXT NOT NULL);
CREATE TABLE entity_aliases (entity_id TEXT NOT NULL REFERENCES entities(id), alias TEXT NOT NULL);
CREATE TABLE entity_external_ids (entity_id TEXT NOT NULL REFERENCES entities(id), scheme TEXT NOT NULL, value TEXT NOT NULL);
CREATE TABLE entity_attributes (entity_id TEXT NOT NULL REFERENCES entities(id), key TEXT NOT NULL, value_json TEXT);
CREATE TABLE assertions (id TEXT PRIMARY KEY, subject_id TEXT NOT NULL REFERENCES entities(id), predicate TEXT NOT NULL, object_kind TEXT NOT NULL, object_entity_id TEXT REFERENCES entities(id), object_value_json TEXT, object_datatype TEXT, object_unit TEXT, object_language TEXT, confidence REAL NOT NULL, status TEXT NOT NULL, valid_from TEXT, valid_to TEXT, recorded_at TEXT NOT NULL, rule TEXT);
CREATE TABLE assertion_evidence (assertion_id TEXT NOT NULL REFERENCES assertions(id), source_id TEXT NOT NULL REFERENCES sources(id), locator TEXT NOT NULL, quote TEXT, extraction_method TEXT NOT NULL, extractor TEXT NOT NULL);
CREATE TABLE identity_decisions (id TEXT PRIMARY KEY, left_entity_id TEXT NOT NULL REFERENCES entities(id), right_entity_id TEXT NOT NULL REFERENCES entities(id), decision TEXT NOT NULL, confidence REAL NOT NULL, review_required INTEGER NOT NULL, supporting_json TEXT, conflicting_json TEXT);
CREATE TABLE validation_issues (id TEXT PRIMARY KEY, severity TEXT NOT NULL, rule TEXT NOT NULL, target_kind TEXT NOT NULL, target_id TEXT NOT NULL, message TEXT NOT NULL, suggested_correction TEXT, automatic_correction_safe INTEGER NOT NULL);
CREATE TABLE schema_proposals (id TEXT PRIMARY KEY, term_kind TEXT NOT NULL, term TEXT NOT NULL, definition TEXT NOT NULL, reason TEXT NOT NULL, status TEXT NOT NULL, domain_json TEXT, range_json TEXT, examples_json TEXT, possible_equivalents_json TEXT);
INSERT INTO bundle_meta VALUES ('schema_version', 'boru-kg/1');
INSERT INTO bundle_meta VALUES ('generated_at', '2026-08-07T00:00:00Z');
INSERT INTO bundle_meta VALUES ('input_digest_algorithm', 'fnv64');
INSERT INTO bundle_meta VALUES ('input_digest_combined', '2766248627240794074');
INSERT INTO input_files VALUES ('../AGENTS.md', '7741724709148957368', 9601);
INSERT INTO input_files VALUES ('../CLI.md', '147469389202745130', 79458);
INSERT INTO input_files VALUES ('../README.md', '7037787103551177539', 12216);
INSERT INTO input_files VALUES ('../basic/go/go.mod', '1012607248799602107', 457);
INSERT INTO input_files VALUES ('../calc/go/go.mod', '8123838604180080966', 605);
INSERT INTO input_files VALUES ('../check/go/go.mod', '5545877118704640253', 221);
INSERT INTO input_files VALUES ('../cmd/go/go.mod', '1168351960312314743', 4184);
INSERT INTO input_files VALUES ('../compiler/go/go.mod', '142593282390199928', 331);
INSERT INTO input_files VALUES ('../core/go/go.mod', '2316996521694161686', 98);
INSERT INTO input_files VALUES ('../design/BASIC-CHECK-CUT.0.md', '1575227922863509534', 8195);
INSERT INTO input_files VALUES ('../design/CORE-TS-COVERAGE.0.md', '7605485402373327537', 10411);
INSERT INTO input_files VALUES ('../design/DECLARATIVE-GRAMMAR.0.md', '4337381568175830188', 3240);
INSERT INTO input_files VALUES ('../design/ENG-COVERAGE-PARITY.0.md', '2541301273793164298', 20169);
INSERT INTO input_files VALUES ('../design/GO-TS-PARITY.0.md', '5136835591266948219', 12462);
INSERT INTO input_files VALUES ('../design/LANG-ENG-CONTENT-AUDIT.0.md', '4534620810048850672', 42667);
INSERT INTO input_files VALUES ('../design/ROOT-MODULE-FEASIBILITY.0.md', '6235317352461048001', 6313);
INSERT INTO input_files VALUES ('../design/TS-PARITY-AUDIT.0.md', '1213034318347000943', 5993);
INSERT INTO input_files VALUES ('../design/checker-compiler-completeness-review.0.md', '5862232790816677050', 39943);
INSERT INTO input_files VALUES ('../editors/tree-sitter/bindings/go/go.mod', '8359550297204300245', 134);
INSERT INTO input_files VALUES ('../eng/go/go.mod', '957440270513375970', 683);
INSERT INTO input_files VALUES ('../go.work', '4924620830580584237', 530);
INSERT INTO input_files VALUES ('../lang/go/go.mod', '9060255802713308676', 2387);
INSERT INTO input_files VALUES ('../parser/go/go.mod', '3554763054779601734', 350);
INSERT INTO input_files VALUES ('../test/go/go.mod', '7400423513495145622', 2503);
INSERT INTO input_files VALUES ('../test/solardemo/go.mod', '8784937342672483810', 59);
INSERT INTO input_files VALUES ('../tools/piecetool/go.mod', '3890078019736541119', 539);
INSERT INTO input_files VALUES ('../wpg/go.mod', '1909195114285173430', 2582);
INSERT INTO input_files VALUES ('<go tree: modules + packages>', '4995374428202383702', 501);
INSERT INTO input_files VALUES ('project/boru-project.jsonic', '7139861158690013628', 25451);
INSERT INTO sources VALUES ('src:agents', 'text', 'AGENTS.md', 'AGENTS.md agent guide', NULL, 'agents-2026-07', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:audit', 'text', 'design/LANG-ENG-CONTENT-AUDIT.0.md', 'lang/eng content audit — ADR-012 evidence', NULL, 'audit-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:basic-check-cut', 'text', 'design/BASIC-CHECK-CUT.0.md', 'removing basic''s dependency on check', NULL, 'basic-check-cut-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:cli-md', 'text', 'CLI.md', 'CLI.md subcommand reference', NULL, 'cli-md-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:completeness-review', 'text', 'design/checker-compiler-completeness-review.0.md', 'Type checker + bytecode compiler — completeness review', NULL, 'completeness-review-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:core-ts-coverage', 'text', 'design/CORE-TS-COVERAGE.0.md', 'core/ts coverage program', NULL, 'core-ts-coverage-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:coverage-parity', 'text', 'design/ENG-COVERAGE-PARITY.0.md', 'eng standalone coverage-parity program', NULL, 'coverage-parity-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:decl-grammar', 'text', 'design/DECLARATIVE-GRAMMAR.0.md', 'declarative grammar artifact for both parser twins', NULL, 'decl-grammar-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:go-tree', 'api', 'workspace tree', 'Go package discovery (filesystem walk)', NULL, 'go-tree', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:go-ts-parity', 'text', 'design/GO-TS-PARITY.0.md', 'full functional parity on core, parser and basic', NULL, 'go-ts-parity-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:go-work', 'text', 'go.work', 'Go workspace file', NULL, 'go-work', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:basic-go', 'text', 'basic/go/go.mod', 'basic/go go.mod', NULL, 'gomod-basic-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:calc-go', 'text', 'calc/go/go.mod', 'calc/go go.mod', NULL, 'gomod-calc-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:check-go', 'text', 'check/go/go.mod', 'check/go go.mod', NULL, 'gomod-check-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:cmd-go', 'text', 'cmd/go/go.mod', 'cmd/go go.mod', NULL, 'gomod-cmd-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:compiler-go', 'text', 'compiler/go/go.mod', 'compiler/go go.mod', NULL, 'gomod-compiler-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:core-go', 'text', 'core/go/go.mod', 'core/go go.mod', NULL, 'gomod-core-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:editors-tree-sitter-bindings-go', 'text', 'editors/tree-sitter/bindings/go/go.mod', 'editors/tree-sitter/bindings/go go.mod', NULL, 'gomod-editors-tree-sitter-bindings-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:eng-go', 'text', 'eng/go/go.mod', 'eng/go go.mod', NULL, 'gomod-eng-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:lang-go', 'text', 'lang/go/go.mod', 'lang/go go.mod', NULL, 'gomod-lang-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:parser-go', 'text', 'parser/go/go.mod', 'parser/go go.mod', NULL, 'gomod-parser-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:test-go', 'text', 'test/go/go.mod', 'test/go go.mod', NULL, 'gomod-test-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:test-solardemo', 'text', 'test/solardemo/go.mod', 'test/solardemo go.mod', NULL, 'gomod-test-solardemo', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:tools-piecetool', 'text', 'tools/piecetool/go.mod', 'tools/piecetool go.mod', NULL, 'gomod-tools-piecetool', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:wpg', 'text', 'wpg/go.mod', 'wpg go.mod', NULL, 'gomod-wpg', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:readme', 'text', 'README.md', 'boru README', NULL, 'readme-2026-07', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:root-module', 'text', 'design/ROOT-MODULE-FEASIBILITY.0.md', 'root module below core and parser, measured', NULL, 'root-module-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:ts-parity-audit', 'text', 'design/TS-PARITY-AUDIT.0.md', 'parser twin parity audit', NULL, 'ts-parity-audit-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO entities VALUES ('ent:Concept:2039420555596601682', 'Concept', 'secrets vault', 'secrets vault', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Concept:2039420555596601682', 'role', 'encrypted API-key / token store, driven by the boru vault subcommand');
INSERT INTO entities VALUES ('ent:Concept:3854395902791518463', 'Concept', 'boru language', 'boru language', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Concept:3854395902791518463', 'paradigm', 'concatenative, typed, word-based');
INSERT INTO entities VALUES ('ent:Concept:4587555710592773395', 'Concept', 'Forward arguments', 'forward arguments', 'accepted');
INSERT INTO entities VALUES ('ent:Concept:4841193570246608846', 'Concept', 'Value stack', 'value stack', 'accepted');
INSERT INTO entities VALUES ('ent:Concept:5837115061456563631', 'Concept', 'boru describe discovery', 'boru describe discovery', 'accepted');
INSERT INTO entities VALUES ('ent:Concept:6094411313845087998', 'Concept', 'vault wire protocol', 'vault wire protocol', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Concept:6094411313845087998', 'role', 'read-only, HashiCorp-style HTTP API for secret provision (boru vault serve), authenticated by capability tokens');
INSERT INTO entities VALUES ('ent:Concept:7376417356888575267', 'Concept', 'Executable language spec', 'executable language spec', 'accepted');
INSERT INTO entities VALUES ('ent:Document:1162242714758522750', 'Document', 'design/BASIC-CHECK-CUT.0.md', 'design/basic-check-cut.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:1162242714758522750', 'role', 'why basic no longer depends on check: the 23 symbols it used, which moved into core as pure primitives, which became a core-owned analysis seam, and the two gates that keep the edge from coming back');
INSERT INTO entities VALUES ('ent:Document:1344160336771235777', 'Document', 'EXPLANATION.md', 'explanation.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:203047846460430642', 'Document', 'design/DECLARATIVE-GRAMMAR.0.md', 'design/declarative-grammar.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:203047846460430642', 'role', 'the shared declarative tabnas grammar artifact (parser/go/grammar.json): contract, loader pair, and the batch-migration state');
INSERT INTO entities VALUES ('ent:Document:2574876380285708892', 'Document', 'design/ROOT-MODULE-FEASIBILITY.0.md', 'design/root-module-feasibility.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:2574876380285708892', 'role', 'the measured verdict on a shared module below core and parser: Value alone closes over 75.7% of core/go, so the cut as posed is a rename — includes the piecetool -closure method and the priced seam alternatives');
INSERT INTO entities VALUES ('ent:Document:3080274854606714513', 'Document', 'ADR.md', 'adr.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:3294415633888265368', 'Document', 'design/checker-compiler-completeness-review.0.md', 'design/checker-compiler-completeness-review.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:3294415633888265368', 'role', 'the 2026-08 checker/compiler completeness review and its §9 implementation record — the living index of the HOF-compilation graduations and the remaining frontier work');
INSERT INTO entities VALUES ('ent:Document:3521225411209893772', 'Document', 'design/LANG-ENG-CONTENT-AUDIT.0.md', 'design/lang-eng-content-audit.0.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:3904106568037504161', 'Document', 'AGENTS.md', 'agents.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:3955423872539901697', 'Document', 'HOWTO.md', 'howto.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:4163489813681141089', 'Document', 'README.md', 'readme.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:4790579719562719716', 'Document', 'design/CORE-TS-COVERAGE.0.md', 'design/core-ts-coverage.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:4790579719562719716', 'role', 'the coverage program for core/ts (the sibling of ENG-COVERAGE-PARITY, one layer down): the staged work list, the ratcheting TS_CORE_GATE_LINES floor, and the measurement that core/spec is a cross-engine SPEC and not a coverage instrument');
INSERT INTO entities VALUES ('ent:Document:481508614007064969', 'Document', 'REFERENCE.md', 'reference.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:4990200910103175455', 'Document', 'CLI.md', 'cli.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:5105101056062860425', 'Document', 'design/ENG-COVERAGE-PARITY.0.md', 'design/eng-coverage-parity.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:5105101056062860425', 'role', 'the standalone 100%/100% coverage program for eng/go and eng/ts: the ratcheting gate floors (make cover-gate-eng, make test-ts), the gap inventories, and the staged plans');
INSERT INTO entities VALUES ('ent:Document:520435226487613788', 'Document', 'design/ notes', 'design/ notes', 'accepted');
INSERT INTO entities VALUES ('ent:Document:5292060467150439417', 'Document', 'NUR.md', 'nur.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:6176355086953937469', 'Document', 'TUTORIAL.md', 'tutorial.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:6369673620858945660', 'Document', 'eng/go/CLAUDE.md', 'eng/go/claude.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:7770110494347118706', 'Document', 'lang/go/CLAUDE.md', 'lang/go/claude.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:8799605016341004740', 'Document', 'design/TS-PARITY-AUDIT.0.md', 'design/ts-parity-audit.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:8799605016341004740', 'role', 'the parser twin parity audit: the render defects the stream oracle found, the parser/spec corpus that replaced the self-referential battery, and the open divergences');
INSERT INTO entities VALUES ('ent:Document:8875579216819453768', 'Document', 'design/GO-TS-PARITY.0.md', 'design/go-ts-parity.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:8875579216819453768', 'role', 'the Go/TS parity program for core, parser and basic: the three shared TSV corpora and their two-runner rule, the capability table that forces core/ts before basic/ts, the divergence ledger, and the finding that an uncovered branch in one port is where a divergence hides');
INSERT INTO entities VALUES ('ent:Document:9071667555031388966', 'Document', 'parser/go/CLAUDE.md', 'parser/go/claude.md', 'accepted');
INSERT INTO entities VALUES ('ent:Organization:8832543031059216933', 'Organization', 'boru-lang', 'boru-lang', 'accepted');
INSERT INTO entities VALUES ('ent:Product:2046405728378673079', 'Product', 'kg knowledge-graph pipeline', 'kg knowledge-graph pipeline', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:2046405728378673079', 'path', 'kg');
INSERT INTO entities VALUES ('ent:Product:4032424380612892464', 'Product', 'boru-lang/boru repository', 'boru-lang/boru repository', 'accepted');
INSERT INTO entity_external_ids VALUES ('ent:Product:4032424380612892464', 'github', 'boru-lang/boru');
INSERT INTO entities VALUES ('ent:Product:4976123614970806523', 'Product', 'boru:cli module', 'boru:cli module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:4976123614970806523', 'language', 'boru');
INSERT INTO entity_attributes VALUES ('ent:Product:4976123614970806523', 'path', 'lang/go/modules/cli.boru');
INSERT INTO entities VALUES ('ent:Product:5770789618934008141', 'Product', 'lang/spec directory', 'lang/spec directory', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:5770789618934008141', 'path', 'lang/spec');
INSERT INTO entities VALUES ('ent:Product:8635404738244704660', 'Product', 'utils coreutils subset', 'utils coreutils subset', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:8635404738244704660', 'path', 'utils');
INSERT INTO entities VALUES ('ent:Product:9122085017676103232', 'Product', 'boru binary', 'boru binary', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:9122085017676103232', 'role', 'CLI, REPL, type checker, formatter, LSP, registry client, vault, supervisor');
INSERT INTO entities VALUES ('ent:SoftwareModule:1529704399546216258', 'SoftwareModule', 'wpg/serve package', 'wpg/serve package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:1529704399546216258', 'go_module', 'github.com/boru-lang/boru/wpg');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:1529704399546216258', 'parent_module', 'wpg');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:1529704399546216258', 'path', 'wpg/serve');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:1529704399546216258', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:1633412495372444656', 'SoftwareModule', 'lang/go/modules package', 'lang/go/modules package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:1633412495372444656', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:1633412495372444656', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:1633412495372444656', 'path', 'lang/go/modules');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:1633412495372444656', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:2013670336276694550', 'SoftwareModule', 'core/go module', 'core/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:2013670336276694550', 'go_module', 'github.com/boru-lang/boru/core/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:2013670336276694550', 'path', 'core/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:2013670336276694550', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:2013670336276694550', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:2224184413468547100', 'SoftwareModule', 'lang/go/test package', 'lang/go/test package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:2224184413468547100', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:2224184413468547100', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:2224184413468547100', 'path', 'lang/go/test');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:2224184413468547100', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:3568378724923050340', 'SoftwareModule', 'test/go/specgen package', 'test/go/specgen package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:3568378724923050340', 'go_module', 'github.com/boru-lang/boru/test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:3568378724923050340', 'parent_module', 'test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:3568378724923050340', 'path', 'test/go/specgen');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:3568378724923050340', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:3641482396164462740', 'SoftwareModule', 'test/go/docexamples package', 'test/go/docexamples package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:3641482396164462740', 'go_module', 'github.com/boru-lang/boru/test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:3641482396164462740', 'parent_module', 'test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:3641482396164462740', 'path', 'test/go/docexamples');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:3641482396164462740', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:4116347247488215916', 'SoftwareModule', 'test/go/covergate package', 'test/go/covergate package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4116347247488215916', 'go_module', 'github.com/boru-lang/boru/test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4116347247488215916', 'parent_module', 'test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4116347247488215916', 'path', 'test/go/covergate');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4116347247488215916', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:4125120573695539350', 'SoftwareModule', 'test/go/langspec package', 'test/go/langspec package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4125120573695539350', 'go_module', 'github.com/boru-lang/boru/test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4125120573695539350', 'parent_module', 'test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4125120573695539350', 'path', 'test/go/langspec');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4125120573695539350', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:4192460694199531608', 'SoftwareModule', 'wpg module', 'wpg module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4192460694199531608', 'go_module', 'github.com/boru-lang/boru/wpg');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4192460694199531608', 'path', 'wpg');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4192460694199531608', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4192460694199531608', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:425341189454841366', 'SoftwareModule', 'eng/go module', 'eng/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:425341189454841366', 'go_module', 'github.com/boru-lang/boru/eng/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:425341189454841366', 'path', 'eng/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:425341189454841366', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:425341189454841366', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:4361728672720029650', 'SoftwareModule', 'cmd/go module', 'cmd/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4361728672720029650', 'go_module', 'github.com/boru-lang/boru/cmd/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4361728672720029650', 'path', 'cmd/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4361728672720029650', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4361728672720029650', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:4559967244660037230', 'SoftwareModule', 'basic/go module', 'basic/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4559967244660037230', 'go_module', 'github.com/boru-lang/boru/basic/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4559967244660037230', 'path', 'basic/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4559967244660037230', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4559967244660037230', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:4589894982256403754', 'SoftwareModule', 'cmd/go/genhelp package', 'cmd/go/genhelp package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4589894982256403754', 'go_module', 'github.com/boru-lang/boru/cmd/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4589894982256403754', 'parent_module', 'cmd/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4589894982256403754', 'path', 'cmd/go/genhelp');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4589894982256403754', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:4598127138166596702', 'SoftwareModule', 'test/solardemo module', 'test/solardemo module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4598127138166596702', 'go_module', 'github.com/boru-lang/boru/test/solardemo');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4598127138166596702', 'path', 'test/solardemo');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4598127138166596702', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4598127138166596702', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:4598450785187489172', 'SoftwareModule', 'lang/go/capabilities package', 'lang/go/capabilities package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4598450785187489172', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4598450785187489172', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4598450785187489172', 'path', 'lang/go/capabilities');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4598450785187489172', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:4629987360876655630', 'SoftwareModule', 'lang/go/native package', 'lang/go/native package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4629987360876655630', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4629987360876655630', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4629987360876655630', 'path', 'lang/go/native');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4629987360876655630', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:4648368824093240216', 'SoftwareModule', 'editors/tree-sitter/bindings/go module', 'editors/tree-sitter/bindings/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4648368824093240216', 'go_module', 'github.com/tree-sitter/tree-sitter-boru');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4648368824093240216', 'path', 'editors/tree-sitter/bindings/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4648368824093240216', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4648368824093240216', 'workspace_member', 'false');
INSERT INTO entities VALUES ('ent:SoftwareModule:4765954660468613608', 'SoftwareModule', 'test/go/cliexamples package', 'test/go/cliexamples package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4765954660468613608', 'go_module', 'github.com/boru-lang/boru/test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4765954660468613608', 'parent_module', 'test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4765954660468613608', 'path', 'test/go/cliexamples');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4765954660468613608', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:5128065373905771552', 'SoftwareModule', 'eng/go/stackform package', 'eng/go/stackform package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5128065373905771552', 'go_module', 'github.com/boru-lang/boru/eng/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5128065373905771552', 'parent_module', 'eng/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5128065373905771552', 'path', 'eng/go/stackform');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5128065373905771552', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:5138375578915662736', 'SoftwareModule', 'test/go module', 'test/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5138375578915662736', 'go_module', 'github.com/boru-lang/boru/test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5138375578915662736', 'path', 'test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5138375578915662736', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5138375578915662736', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:5172256887243902980', 'SoftwareModule', 'test/go/vary package', 'test/go/vary package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5172256887243902980', 'go_module', 'github.com/boru-lang/boru/test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5172256887243902980', 'parent_module', 'test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5172256887243902980', 'path', 'test/go/vary');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5172256887243902980', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:559301050642427014', 'SoftwareModule', 'check/go module', 'check/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:559301050642427014', 'go_module', 'github.com/boru-lang/boru/check/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:559301050642427014', 'path', 'check/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:559301050642427014', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:559301050642427014', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:591370078981669488', 'SoftwareModule', 'test/go/engspec package', 'test/go/engspec package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:591370078981669488', 'go_module', 'github.com/boru-lang/boru/test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:591370078981669488', 'parent_module', 'test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:591370078981669488', 'path', 'test/go/engspec');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:591370078981669488', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:5919856352625365594', 'SoftwareModule', 'calc/go module', 'calc/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5919856352625365594', 'go_module', 'github.com/boru-lang/boru/calc/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5919856352625365594', 'path', 'calc/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5919856352625365594', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5919856352625365594', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:6519065918608370232', 'SoftwareModule', 'wpg/wasm package', 'wpg/wasm package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6519065918608370232', 'go_module', 'github.com/boru-lang/boru/wpg');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6519065918608370232', 'parent_module', 'wpg');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6519065918608370232', 'path', 'wpg/wasm');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6519065918608370232', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:6698255711474724668', 'SoftwareModule', 'eng/go/specfix package', 'eng/go/specfix package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6698255711474724668', 'go_module', 'github.com/boru-lang/boru/eng/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6698255711474724668', 'parent_module', 'eng/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6698255711474724668', 'path', 'eng/go/specfix');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6698255711474724668', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:6706536563979604982', 'SoftwareModule', 'parser/go module', 'parser/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6706536563979604982', 'go_module', 'github.com/boru-lang/boru/parser/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6706536563979604982', 'path', 'parser/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6706536563979604982', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6706536563979604982', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:6880687338933514154', 'SoftwareModule', 'compiler/go module', 'compiler/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6880687338933514154', 'go_module', 'github.com/boru-lang/boru/compiler/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6880687338933514154', 'path', 'compiler/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6880687338933514154', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6880687338933514154', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:7327774866203707838', 'SoftwareModule', 'tools/piecetool module', 'tools/piecetool module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:7327774866203707838', 'go_module', 'github.com/boru-lang/boru/tools/piecetool');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:7327774866203707838', 'path', 'tools/piecetool');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:7327774866203707838', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:7327774866203707838', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:8275629451197117420', 'SoftwareModule', 'lang/go module', 'lang/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8275629451197117420', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8275629451197117420', 'path', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8275629451197117420', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8275629451197117420', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:8281032769668059040', 'SoftwareModule', 'lang/go/policy package', 'lang/go/policy package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8281032769668059040', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8281032769668059040', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8281032769668059040', 'path', 'lang/go/policy');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8281032769668059040', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:8376380866775607244', 'SoftwareModule', 'lang/go/formatter package', 'lang/go/formatter package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8376380866775607244', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8376380866775607244', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8376380866775607244', 'path', 'lang/go/formatter');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8376380866775607244', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:8582068475984941492', 'SoftwareModule', 'lang/go/tuikit package', 'lang/go/tuikit package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8582068475984941492', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8582068475984941492', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8582068475984941492', 'path', 'lang/go/tuikit');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8582068475984941492', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:8650101979302333988', 'SoftwareModule', 'cmd/go/boru package', 'cmd/go/boru package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8650101979302333988', 'go_module', 'github.com/boru-lang/boru/cmd/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8650101979302333988', 'parent_module', 'cmd/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8650101979302333988', 'path', 'cmd/go/boru');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8650101979302333988', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:927371487649425292', 'SoftwareModule', 'lang/go/debugserve package', 'lang/go/debugserve package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:927371487649425292', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:927371487649425292', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:927371487649425292', 'path', 'lang/go/debugserve');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:927371487649425292', 'unit', 'go-package');
INSERT INTO assertions VALUES ('ast:1048763594130214961', 'ent:SoftwareModule:591370078981669488', 'part_of', 'entity', 'ent:SoftwareModule:5138375578915662736', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1048763594130214961', 'src:go-tree', 'test/go/engspec', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1126534628156470241', 'ent:SoftwareModule:5138375578915662736', 'depends_on', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1126534628156470241', 'src:gomod:test-go', 'require block', 'github.com/boru-lang/boru/parser/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1200607522753691950', 'ent:Document:5105101056062860425', 'supports', 'entity', 'ent:SoftwareModule:425341189454841366', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1200607522753691950', 'src:coverage-parity', 'The contract', 'eng proves itself with its OWN suite, on both implementations.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:128749266603685013', 'ent:SoftwareModule:6698255711474724668', 'part_of', 'entity', 'ent:SoftwareModule:425341189454841366', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:128749266603685013', 'src:go-tree', 'eng/go/specfix', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1291087365743607224', 'ent:SoftwareModule:8275629451197117420', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/lang/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1291087365743607224', 'src:gomod:lang-go', 'module directive', 'module github.com/boru-lang/boru/lang/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1374144917814367558', 'ent:Concept:3854395902791518463', 'has_attribute', 'literal', NULL, '"concatenative"', 'String', NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1374144917814367558', 'src:readme', 'intro', 'Underneath, boru is concatenative: words can equally take their arguments from a value stack', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1529088506745454642', 'ent:SoftwareModule:6880687338933514154', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1529088506745454642', 'src:go-work', 'use block', './compiler/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1536707264993433580', 'ent:SoftwareModule:4559967244660037230', 'depends_on', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1536707264993433580', 'src:gomod:basic-go', 'require block', 'github.com/boru-lang/boru/parser/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1731838044320190440', 'ent:Product:9122085017676103232', 'part_of', 'entity', 'ent:SoftwareModule:4361728672720029650', NULL, NULL, NULL, NULL, 0.97, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1731838044320190440', 'src:readme', 'Repository layout', 'cmd/go/ | The boru CLI / REPL', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1778398845817128450', 'ent:SoftwareModule:4629987360876655630', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1778398845817128450', 'src:go-tree', 'lang/go/native', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1803749198783100155', 'ent:SoftwareModule:5919856352625365594', 'depends_on', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1803749198783100155', 'src:gomod:calc-go', 'require block', 'github.com/boru-lang/boru/core/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1910960561653111690', 'ent:SoftwareModule:4765954660468613608', 'part_of', 'entity', 'ent:SoftwareModule:5138375578915662736', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1910960561653111690', 'src:go-tree', 'test/go/cliexamples', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:2099674608751247787', 'ent:SoftwareModule:5138375578915662736', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2099674608751247787', 'src:go-work', 'use block', './test/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:2132946225724718620', 'ent:SoftwareModule:7327774866203707838', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/tools/piecetool"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2132946225724718620', 'src:gomod:tools-piecetool', 'module directive', 'module github.com/boru-lang/boru/tools/piecetool', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:2174589708589527462', 'ent:SoftwareModule:5138375578915662736', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/test/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2174589708589527462', 'src:gomod:test-go', 'module directive', 'module github.com/boru-lang/boru/test/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:2190303907985753377', 'ent:Document:3955423872539901697', 'supports', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2190303907985753377', 'src:readme', 'Documentation', 'How-To Guides | You have a specific task and want a recipe.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2250649441720436972', 'ent:SoftwareModule:4598450785187489172', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2250649441720436972', 'src:go-tree', 'lang/go/capabilities', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:2308356135508106004', 'ent:Document:5292060467150439417', 'mentions', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2308356135508106004', 'src:readme', 'Documentation', 'Non-Uniformity Register | You want the recorded deviations from the language''s uniform rules, each pending, resolved, or explicitly allowed.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2313048313946518913', 'ent:Product:8635404738244704660', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2313048313946518913', 'src:agents', 'Repository layout', 'utils/ | A coreutils subset written in boru — real programs that prove the CLI story (argv, exit codes, streams, baked permissions).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2321418037316607481', 'ent:SoftwareModule:5919856352625365594', 'depends_on', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2321418037316607481', 'src:gomod:calc-go', 'require block', 'github.com/boru-lang/boru/parser/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:2457753349338579625', 'ent:Document:4990200910103175455', 'supports', 'entity', 'ent:Product:9122085017676103232', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2457753349338579625', 'src:readme', 'Documentation', 'CLI Reference | You want to drive the boru binary from the shell.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2510153559955756366', 'ent:Concept:7376417356888575267', 'part_of', 'entity', 'ent:Product:5770789618934008141', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2510153559955756366', 'src:agents', 'Task router', 'The executable language spec (the rows tests run against) | lang/spec/*.tsv', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2522351206637396085', 'ent:SoftwareModule:6880687338933514154', 'depends_on', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2522351206637396085', 'src:gomod:compiler-go', 'require block', 'github.com/boru-lang/boru/core/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:2546454704586227304', 'ent:SoftwareModule:5138375578915662736', 'depends_on', 'entity', 'ent:SoftwareModule:4559967244660037230', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2546454704586227304', 'src:gomod:test-go', 'require block', 'github.com/boru-lang/boru/basic/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:2618788706676923636', 'ent:Document:2574876380285708892', 'supports', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2618788706676923636', 'src:root-module', 'The finding', '**The finding: `Value` alone closes over 75.7% of `core/go`.**', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2650141132074711437', 'ent:Product:9122085017676103232', 'supports', 'entity', 'ent:Concept:5837115061456563631', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2650141132074711437', 'src:agents', 'First: let the tool document itself', 'The boru CLI documents both the language and itself, and that output is generated from the live engine', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2714957710081530623', 'ent:Document:3521225411209893772', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2714957710081530623', 'src:audit', 'title', 'Lang/Eng Content Audit — and the Types-Module Proposal', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2979279625592738698', 'ent:SoftwareModule:1529704399546216258', 'part_of', 'entity', 'ent:SoftwareModule:4192460694199531608', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2979279625592738698', 'src:go-tree', 'wpg/serve', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3002920817860690706', 'ent:SoftwareModule:8275629451197117420', 'depends_on', 'entity', 'ent:SoftwareModule:4559967244660037230', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3002920817860690706', 'src:gomod:lang-go', 'require block', 'github.com/boru-lang/boru/basic/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:303023300656590935', 'ent:Document:1344160336771235777', 'supports', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:303023300656590935', 'src:readme', 'Documentation', 'Explanation | You want to understand why boru is the way it is.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:311117795035296475', 'ent:SoftwareModule:8275629451197117420', 'depends_on', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:311117795035296475', 'src:gomod:lang-go', 'require block', 'github.com/boru-lang/boru/parser/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:314582934812456120', 'ent:Document:3904106568037504161', 'mentions', 'entity', 'ent:Concept:5837115061456563631', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:314582934812456120', 'src:agents', 'First: let the tool document itself', 'Before grepping source or guessing a word''s signature, ask the binary.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3155371871007005577', 'ent:Document:8875579216819453768', 'supports', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3155371871007005577', 'src:go-ts-parity', 'The measurement', 'the parity gap is the uncovered surface', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3178522596510652426', 'ent:SoftwareModule:6706536563979604982', 'depends_on', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3178522596510652426', 'src:gomod:parser-go', 'require block', 'github.com/boru-lang/boru/core/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3211344478610580799', 'ent:Concept:3854395902791518463', 'related_to', 'entity', 'ent:Concept:4587555710592773395', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3211344478610580799', 'src:readme', 'Forward arguments', 'the defining feature of the surface syntax is forward arguments', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3331627459435955194', 'ent:SoftwareModule:8275629451197117420', 'depends_on', 'entity', 'ent:SoftwareModule:6880687338933514154', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3331627459435955194', 'src:gomod:lang-go', 'require block', 'github.com/boru-lang/boru/compiler/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3384030660507785166', 'ent:Document:1162242714758522750', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3384030660507785166', 'src:basic-check-cut', 'title', 'BASIC-CHECK-CUT.0 — removing `basic`''s dependency on `check`', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3410841662781299382', 'ent:Document:4790579719562719716', 'supports', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3410841662781299382', 'src:core-ts-coverage', 'The corpus is not the instrument', 'keep growing `core/spec` as a cross-engine spec, and stop treating it', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3438862149494160814', 'ent:SoftwareModule:4589894982256403754', 'part_of', 'entity', 'ent:SoftwareModule:4361728672720029650', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3438862149494160814', 'src:go-tree', 'cmd/go/genhelp', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:351832070028345115', 'ent:SoftwareModule:4125120573695539350', 'part_of', 'entity', 'ent:SoftwareModule:5138375578915662736', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:351832070028345115', 'src:go-tree', 'test/go/langspec', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3612971925750437545', 'ent:SoftwareModule:8582068475984941492', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3612971925750437545', 'src:go-tree', 'lang/go/tuikit', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3656929081728712135', 'ent:SoftwareModule:7327774866203707838', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3656929081728712135', 'src:go-work', 'use block', './tools/piecetool', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3727255686445494517', 'ent:SoftwareModule:3568378724923050340', 'part_of', 'entity', 'ent:SoftwareModule:5138375578915662736', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3727255686445494517', 'src:go-tree', 'test/go/specgen', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3820981091835606929', 'ent:SoftwareModule:4361728672720029650', 'depends_on', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3820981091835606929', 'src:gomod:cmd-go', 'require block', 'github.com/boru-lang/boru/parser/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3821304020166363356', 'ent:Document:1162242714758522750', 'supports', 'entity', 'ent:SoftwareModule:4559967244660037230', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3821304020166363356', 'src:basic-check-cut', 'Status', '**Status:** COMPLETE (2026-08-08)', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3940080421122549792', 'ent:Document:203047846460430642', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3940080421122549792', 'src:decl-grammar', 'title', 'DECLARATIVE-GRAMMAR.0 — one grammar artifact for both parser twins', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4109835522011898628', 'ent:Concept:3854395902791518463', 'has_attribute', 'literal', NULL, '"typed, word-based query language"', 'String', NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4109835522011898628', 'src:readme', 'intro', 'boru is a typed, word-based query language.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4129485658791420324', 'ent:Concept:2039420555596601682', 'part_of', 'entity', 'ent:Product:9122085017676103232', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4129485658791420324', 'src:readme', 'Overview', 'a secrets vault', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4193417245179262389', 'ent:SoftwareModule:425341189454841366', 'depends_on', 'entity', 'ent:SoftwareModule:6880687338933514154', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4193417245179262389', 'src:gomod:eng-go', 'require block', 'github.com/boru-lang/boru/compiler/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4235718707717393125', 'ent:SoftwareModule:4648368824093240216', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4235718707717393125', 'src:go-tree', 'editors/tree-sitter/bindings/go/go.mod', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4367944145334365650', 'ent:SoftwareModule:4559967244660037230', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4367944145334365650', 'src:go-work', 'use block', './basic/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4503779949059601989', 'ent:SoftwareModule:425341189454841366', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/eng/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4503779949059601989', 'src:gomod:eng-go', 'module directive', 'module github.com/boru-lang/boru/eng/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4539802379148808939', 'ent:Document:3080274854606714513', 'mentions', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4539802379148808939', 'src:readme', 'Documentation', 'Architecture Design Record | You want the key architectural decisions and the reasoning behind them.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:459410346719323347', 'ent:Document:520435226487613788', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:459410346719323347', 'src:readme', 'Repository layout', 'Internal design notes and proposals.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4649598326942754342', 'ent:SoftwareModule:425341189454841366', 'depends_on', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4649598326942754342', 'src:gomod:eng-go', 'require block', 'github.com/boru-lang/boru/parser/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4713193859377278200', 'ent:SoftwareModule:5128065373905771552', 'part_of', 'entity', 'ent:SoftwareModule:425341189454841366', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4713193859377278200', 'src:go-tree', 'eng/go/stackform', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4722992453068743762', 'ent:SoftwareModule:6880687338933514154', 'depends_on', 'entity', 'ent:SoftwareModule:559301050642427014', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4722992453068743762', 'src:gomod:compiler-go', 'require block', 'github.com/boru-lang/boru/check/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4877857793539633559', 'ent:SoftwareModule:4361728672720029650', 'depends_on', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4877857793539633559', 'src:gomod:cmd-go', 'require block', 'github.com/boru-lang/boru/lang/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4932254227310747047', 'ent:SoftwareModule:6880687338933514154', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/compiler/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4932254227310747047', 'src:gomod:compiler-go', 'module directive', 'module github.com/boru-lang/boru/compiler/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4950098273738144333', 'ent:SoftwareModule:4598127138166596702', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/test/solardemo"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4950098273738144333', 'src:gomod:test-solardemo', 'module directive', 'module github.com/boru-lang/boru/test/solardemo', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5127918679616522004', 'ent:Document:8799605016341004740', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5127918679616522004', 'src:ts-parity-audit', 'title', 'TS-PARITY-AUDIT.0 — the parser battery does not agree with parser/go', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5214003688923155039', 'ent:SoftwareModule:6706536563979604982', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5214003688923155039', 'src:go-work', 'use block', './parser/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5245330609796646290', 'ent:SoftwareModule:425341189454841366', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5245330609796646290', 'src:go-work', 'use block', './eng/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:534808747601329006', 'ent:Document:6369673620858945660', 'supports', 'entity', 'ent:SoftwareModule:425341189454841366', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:534808747601329006', 'src:agents', 'Working in the code', 'Engine kernel — types, values, signatures, matching, the step loop, the parser bridge | eng/go/CLAUDE.md', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:535388861626259779', 'ent:Document:8799605016341004740', 'supports', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:535388861626259779', 'src:ts-parity-audit', 'the differential', 'After the fixes the differential is **0 divergences across 1,765 rows**.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5359122693730754691', 'ent:SoftwareModule:6519065918608370232', 'part_of', 'entity', 'ent:SoftwareModule:4192460694199531608', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5359122693730754691', 'src:go-tree', 'wpg/wasm', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5532600402363181981', 'ent:Document:7770110494347118706', 'supports', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5532600402363181981', 'src:agents', 'Working in the code', 'Language layer — native words, modules, registry, help/describe, capabilities | lang/go/CLAUDE.md', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5555847556064706927', 'ent:SoftwareModule:4192460694199531608', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5555847556064706927', 'src:go-work', 'use block', './wpg', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5569270398049625658', 'ent:Product:8635404738244704660', 'related_to', 'entity', 'ent:Product:4976123614970806523', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5569270398049625658', 'src:readme', 'Repository layout', 'utils/ | A coreutils subset written in boru (cat, wc, head, grep, …) — real programs, built with boru build.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5589670101945647091', 'ent:SoftwareModule:4648368824093240216', 'has_attribute', 'literal', NULL, '"github.com/tree-sitter/tree-sitter-boru"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5589670101945647091', 'src:gomod:editors-tree-sitter-bindings-go', 'module directive', 'module github.com/tree-sitter/tree-sitter-boru', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5661573044671554336', 'ent:SoftwareModule:425341189454841366', 'depends_on', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5661573044671554336', 'src:gomod:eng-go', 'require block', 'github.com/boru-lang/boru/core/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:569417614246256590', 'ent:SoftwareModule:5172256887243902980', 'part_of', 'entity', 'ent:SoftwareModule:5138375578915662736', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:569417614246256590', 'src:go-tree', 'test/go/vary', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5752370331192826008', 'ent:SoftwareModule:559301050642427014', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5752370331192826008', 'src:go-work', 'use block', './check/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5778588895625917877', 'ent:SoftwareModule:425341189454841366', 'depends_on', 'entity', 'ent:SoftwareModule:559301050642427014', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5778588895625917877', 'src:gomod:eng-go', 'require block', 'github.com/boru-lang/boru/check/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5799645053343114985', 'ent:SoftwareModule:5919856352625365594', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/calc/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5799645053343114985', 'src:gomod:calc-go', 'module directive', 'module github.com/boru-lang/boru/calc/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5826908977078226408', 'ent:SoftwareModule:5138375578915662736', 'depends_on', 'entity', 'ent:SoftwareModule:425341189454841366', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5826908977078226408', 'src:gomod:test-go', 'require block', 'github.com/boru-lang/boru/eng/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5861093039722048761', 'ent:Document:2574876380285708892', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5861093039722048761', 'src:root-module', 'title', 'ROOT-MODULE-FEASIBILITY.0 — measuring a shared module below core and parser', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5872086631131530189', 'ent:SoftwareModule:2013670336276694550', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5872086631131530189', 'src:go-work', 'use block', './core/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:6024405707462587556', 'ent:Document:481508614007064969', 'supports', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6024405707462587556', 'src:readme', 'Documentation', 'Reference | You need the precise behaviour of a syntax form, type, or word.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:6294168041362264130', 'ent:SoftwareModule:2013670336276694550', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/core/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6294168041362264130', 'src:gomod:core-go', 'module directive', 'module github.com/boru-lang/boru/core/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:6506454126246230063', 'ent:SoftwareModule:559301050642427014', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/check/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6506454126246230063', 'src:gomod:check-go', 'module directive', 'module github.com/boru-lang/boru/check/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:6658327612052881629', 'ent:SoftwareModule:8275629451197117420', 'depends_on', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6658327612052881629', 'src:gomod:lang-go', 'require block', 'github.com/boru-lang/boru/core/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:6799788586482649785', 'ent:Document:4790579719562719716', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6799788586482649785', 'src:core-ts-coverage', 'title', 'CORE-TS-COVERAGE.0 — taking core/ts from 62% to 100%', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:6971877089875160938', 'ent:SoftwareModule:559301050642427014', 'depends_on', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6971877089875160938', 'src:gomod:check-go', 'require block', 'github.com/boru-lang/boru/core/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:7011485005147552963', 'ent:SoftwareModule:4192460694199531608', 'supports', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7011485005147552963', 'src:readme', 'Install', 'A wasm-powered browser playground is bundled in docs/index.html', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7044130325114909254', 'ent:SoftwareModule:4192460694199531608', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/wpg"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7044130325114909254', 'src:gomod:wpg', 'module directive', 'module github.com/boru-lang/boru/wpg', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:7207675757765927631', 'ent:Document:3521225411209893772', 'supports', 'entity', 'ent:SoftwareModule:425341189454841366', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7207675757765927631', 'src:audit', '7A', 'the parser emits kernel STRUCTURAL MARKERS; the engine lowers each marker to word dispatches', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7212088808110577326', 'ent:SoftwareModule:8275629451197117420', 'depends_on', 'entity', 'ent:SoftwareModule:425341189454841366', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7212088808110577326', 'src:gomod:lang-go', 'require block', 'github.com/boru-lang/boru/eng/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:7294700582288177136', 'ent:Product:4032424380612892464', 'created_by', 'entity', 'ent:Organization:8832543031059216933', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7294700582288177136', 'src:readme', 'Install', 'git clone https://github.com/boru-lang/boru', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7447279695537175736', 'ent:Product:2046405728378673079', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7447279695537175736', 'src:agents', 'Repository layout', 'kg/ | The project knowledge graph: an evidence-backed boru pipeline and its generated bundle.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:747512178358509988', 'ent:SoftwareModule:5919856352625365594', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:747512178358509988', 'src:go-work', 'use block', './calc/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:758093515468550172', 'ent:Product:8635404738244704660', 'related_to', 'entity', 'ent:Product:9122085017676103232', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:758093515468550172', 'src:readme', 'Repository layout', 'real programs, built with boru build', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7616275222305495199', 'ent:SoftwareModule:8650101979302333988', 'part_of', 'entity', 'ent:SoftwareModule:4361728672720029650', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7616275222305495199', 'src:go-tree', 'cmd/go/boru', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:7661116843982840438', 'ent:Product:5770789618934008141', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7661116843982840438', 'src:readme', 'Repository layout', 'lang/spec/ | Engine spec TSV files (the language''s executable spec).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7825661200694944199', 'ent:SoftwareModule:4361728672720029650', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7825661200694944199', 'src:go-work', 'use block', './cmd/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:7828550185343127', 'ent:Document:4163489813681141089', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7828550185343127', 'src:agents', 'header', 'New to the repo? Skim README.md first.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7875690729105221344', 'ent:Concept:3854395902791518463', 'related_to', 'entity', 'ent:Concept:4841193570246608846', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7875690729105221344', 'src:readme', 'intro', 'words can equally take their arguments from a value stack, which is what makes point-free pipelines compose', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7916954283464366890', 'ent:SoftwareModule:4559967244660037230', 'depends_on', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7916954283464366890', 'src:gomod:basic-go', 'require block', 'github.com/boru-lang/boru/core/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8143463681370129906', 'ent:SoftwareModule:4192460694199531608', 'depends_on', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8143463681370129906', 'src:gomod:wpg', 'require block', 'github.com/boru-lang/boru/lang/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8189213360333637896', 'ent:Product:4976123614970806523', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8189213360333637896', 'src:agents', 'Repository layout', 'utils/ | A coreutils subset written in boru — real programs that prove the CLI story (argv, exit codes, streams, baked permissions).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8305882503258666818', 'ent:SoftwareModule:4361728672720029650', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/cmd/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8305882503258666818', 'src:gomod:cmd-go', 'module directive', 'module github.com/boru-lang/boru/cmd/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8360168369078766404', 'ent:Document:8875579216819453768', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8360168369078766404', 'src:go-ts-parity', 'title', 'GO-TS-PARITY.0 — full functional parity on core, parser and basic', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8427787091950146539', 'ent:SoftwareModule:4116347247488215916', 'part_of', 'entity', 'ent:SoftwareModule:5138375578915662736', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8427787091950146539', 'src:go-tree', 'test/go/covergate', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8559088752918568803', 'ent:SoftwareModule:5138375578915662736', 'depends_on', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8559088752918568803', 'src:gomod:test-go', 'require block', 'github.com/boru-lang/boru/lang/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8638473926594381359', 'ent:SoftwareModule:1633412495372444656', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8638473926594381359', 'src:go-tree', 'lang/go/modules', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8686166953339451140', 'ent:SoftwareModule:8281032769668059040', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8686166953339451140', 'src:go-tree', 'lang/go/policy', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8731733153397530894', 'ent:SoftwareModule:4598127138166596702', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8731733153397530894', 'src:go-work', 'use block', './test/solardemo', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8820902757097961904', 'ent:SoftwareModule:3641482396164462740', 'part_of', 'entity', 'ent:SoftwareModule:5138375578915662736', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8820902757097961904', 'src:go-tree', 'test/go/docexamples', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8905043332608038803', 'ent:SoftwareModule:927371487649425292', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8905043332608038803', 'src:go-tree', 'lang/go/debugserve', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8927535347315204632', 'ent:Concept:6094411313845087998', 'part_of', 'entity', 'ent:Concept:2039420555596601682', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8927535347315204632', 'src:cli-md', 'The vault', 'HTTP wire protocol for secret provision', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8933845283517661039', 'ent:SoftwareModule:8376380866775607244', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8933845283517661039', 'src:go-tree', 'lang/go/formatter', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8955705768046061429', 'ent:Document:203047846460430642', 'supports', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8955705768046061429', 'src:decl-grammar', 'The contract', 'parser/go/grammar.json is the single source of the boru grammar''s STRUCTURE, loaded by both parsers', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:897358123486782210', 'ent:Document:3294415633888265368', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:897358123486782210', 'src:completeness-review', 'title', 'Type checker + bytecode compiler — completeness review', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8982543179606202117', 'ent:SoftwareModule:8275629451197117420', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8982543179606202117', 'src:go-work', 'use block', './lang/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8989313594377163134', 'ent:SoftwareModule:2224184413468547100', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8989313594377163134', 'src:go-tree', 'lang/go/test', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:9059850742711593788', 'ent:SoftwareModule:6706536563979604982', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/parser/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:9059850742711593788', 'src:gomod:parser-go', 'module directive', 'module github.com/boru-lang/boru/parser/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:9138469699059997665', 'ent:SoftwareModule:4559967244660037230', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/basic/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:9138469699059997665', 'src:gomod:basic-go', 'module directive', 'module github.com/boru-lang/boru/basic/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:914658451831503592', 'ent:Document:6176355086953937469', 'supports', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:914658451831503592', 'src:readme', 'Documentation', 'Tutorial | You are new to boru and want to learn it step by step.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:965013337224509220', 'ent:SoftwareModule:8275629451197117420', 'depends_on', 'entity', 'ent:SoftwareModule:559301050642427014', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:965013337224509220', 'src:gomod:lang-go', 'require block', 'github.com/boru-lang/boru/check/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:999583482530779094', 'ent:Document:5105101056062860425', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:999583482530779094', 'src:coverage-parity', 'title', 'ENG-COVERAGE-PARITY.0 — the standalone 100%/100% program for the kernel twins', 'direct_record', 'kg-ingest');
COMMIT;