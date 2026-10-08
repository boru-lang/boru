package vault

import (
	"crypto/aes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/boru-lang/boru/cmd/go/internal/auth"
	"github.com/boru-lang/boru/cmd/go/internal/pathutil"

	"github.com/boru-lang/boru/cmd/go/internal/wire"
)

// Export bundles let a vault be moved between machines and backends:
// they carry alias metadata and the secret values, re-encrypted under a
// passphrase you choose, independent of the host keystore. A `file`
// vault is already portable as two files; an export bundle is how you
// move secrets *out of* an OS-keychain or 1Password vault to a
// different OS.
//
// On-disk envelope (self-describing, per ADR-009):
//
//	"BORUX" | format(1 byte) | salt(16) | nonce(12) | ciphertext|tag
//
// The header bytes + salt are bound in as AEAD additional data, so the
// format byte cannot be downgraded by tampering. The plaintext is the
// JSON exportBundle below, which carries its own schema version.
const (
	// EnvExportPassphrase supplies the bundle passphrase non-interactively
	// for both export and import.
	EnvExportPassphrase = "BORU_VAULT_EXPORT_PASSPHRASE"

	// exportMagic is the only bundle magic this binary WRITES; every magic
	// it can read comes from wire.ExportMagics(), which includes the ones
	// earlier, differently-named releases wrote.
	exportMagic = wire.ExportMagic
	// exportEnvelopeFormat versions the crypto envelope (KDF/cipher/layout).
	exportEnvelopeFormat = 1
	// exportVersion versions the inner JSON schema.
	//
	//	v1 — aliases + values.
	//	v2 — bundles also carry the custom provider presets the exported
	//	     aliases reference, so a custom-backed alias still brokers after
	//	     import instead of falling back to the URL-less generic preset.
	//	     An older boru refuses a v2 bundle (Version > exportVersion) rather
	//	     than importing aliases whose provider tag it cannot satisfy.
	exportVersion = 2
)

// exportBundle is the decrypted payload of an export.
type exportBundle struct {
	Version    int           `json:"version"`
	ExportedAt string        `json:"exported_at"`
	Aliases    []exportAlias `json:"aliases"`
	// CustomProviders are the operator-defined presets referenced by
	// Aliases (built-in presets are compiled into the target, so only
	// store-defined ones travel). Empty on a v1 bundle.
	CustomProviders []Provider `json:"custom_providers,omitempty"`
}

// exportAlias is one secret plus the metadata worth carrying with it.
type exportAlias struct {
	Name        string   `json:"name"`
	Provider    string   `json:"provider,omitempty"`
	Namespace   string   `json:"namespace,omitempty"`
	Source      string   `json:"source,omitempty"`
	IPWhitelist []string `json:"ip_whitelist,omitempty"`
	Value       string   `json:"value"`
}

// isExportBundle reports whether data is an encrypted export bundle, by
// its magic header. Used to dispatch `vault import` between bundles and
// .env files.
func isExportBundle(data []byte) bool {
	_, ok := exportMagicLen(data)
	return ok
}

// exportMagicLen matches data's leading magic against every spelling this
// binary can read, returning the MATCHED magic's length so the reader can
// locate the format byte. The spellings differ in length, so the offset
// must come from here rather than from len(exportMagic).
func exportMagicLen(data []byte) (int, bool) {
	return wire.MatchPrefix(data, wire.ExportMagics(), 0)
}

// --- export ----------------------------------------------------------------

func runExport(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "write the encrypted bundle to this file (default: stdout)")
	namespace := fs.String("namespace", "", "only export aliases in this namespace (':' = root only)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	// A leading ~ that the shell did not expand (e.g. --out=~/bundle.borux)
	// must resolve under the home folder, not a literal "~" directory.
	outPath := pathutil.ExpandTilde(*out, homeDir)
	nsFilter, nsFiltered, err := normalizeNSFilter(*namespace)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s.Locked {
		fmt.Fprintln(stderr, "error: vault is locked; run `boru vault unlock`")
		return 1
	}

	selected, err := selectExportAliases(s, fs.Args(), nsFilter, nsFiltered)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if len(selected) == 0 {
		fmt.Fprintln(stderr, "error: no matching aliases to export")
		return 1
	}

	// Never spew a binary bundle onto an interactive terminal.
	if outPath == "" && isTerminalWriter(stdout) {
		fmt.Fprintln(stderr, "error: refusing to write a binary bundle to the terminal; use --out=FILE or redirect stdout")
		return 1
	}

	// Prompts go to stderr, never stdout: in the default mode the bundle
	// IS stdout (e.g. `boru vault export > vault.borux`), so a prompt on
	// stdout would corrupt the file — and the user wouldn't see it on
	// their terminal either.
	sess, err := authenticate(s, homeDir, stdin, stderr, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer sess.Close()
	if err := c7requireScope(sess, OpRead); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	bundle := exportBundle{Version: exportVersion, ExportedAt: time.Now().UTC().Format(time.RFC3339)}
	for _, a := range selected {
		v, err := sess.getValue(a.Name, valueNamespace(a.Name))
		if err != nil {
			fmt.Fprintf(stderr, "error: reading %s: %s\n", a.Name, err)
			return 1
		}
		bundle.Aliases = append(bundle.Aliases, exportAlias{
			Name: a.Name, Provider: a.Provider, Namespace: a.Namespace, Source: a.Source,
			IPWhitelist: a.IPWhitelist, Value: v,
		})
	}
	// Carry the custom provider presets the exported aliases reference so
	// they resolve on the target; built-ins are compiled in there, so only
	// store-defined presets travel. Sorted for a stable payload.
	seenProv := map[string]bool{}
	for _, a := range selected {
		if a.Provider == "" || builtinProvider(a.Provider) || seenProv[a.Provider] {
			continue
		}
		if p, _ := s.FindCustomProvider(a.Provider); p != nil {
			bundle.CustomProviders = append(bundle.CustomProviders, *p)
			seenProv[a.Provider] = true
		}
	}
	sort.Slice(bundle.CustomProviders, func(i, j int) bool {
		return bundle.CustomProviders[i].Name < bundle.CustomProviders[j].Name
	})

	pass, err := exportSealPassphrase(stdin, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if pass == "" {
		fmt.Fprintln(stderr, "error: export passphrase must not be empty; the bundle holds your secrets in re-encrypted form")
		return 1
	}

	plain, err := jsonMarshal(bundle)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	blob, err := sealExport(plain, pass)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	if outPath != "" {
		if err := writeFileAtomic(outPath, blob, 0600); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		fmt.Fprintf(stderr, "exported %d secret(s) to %s\n", len(bundle.Aliases), outPath)
	} else {
		if _, err := stdout.Write(blob); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		fmt.Fprintf(stderr, "exported %d secret(s)\n", len(bundle.Aliases))
	}
	_ = appendAudit(homeDir, AuditEvent{
		Action: "vault.export", Outcome: "ok",
		Reason: fmt.Sprintf("aliases=%d", len(bundle.Aliases)),
	})
	return 0
}

// selectExportAliases returns the aliases to export: the referenced
// ones if any are given (each resolved through the namespace rule, so
// bare names follow the active default), otherwise all, filtered by
// namespace when one was specified ("" with filtered=true means root
// only).
func selectExportAliases(s *Store, names []string, nsFilter string, nsFiltered bool) ([]Alias, error) {
	var out []Alias
	if len(names) > 0 {
		for _, ref := range names {
			a, _, err := findAliasRef(s, ref)
			if err != nil {
				return nil, err
			}
			if !nsFiltered || aliasNamespace(*a) == nsFilter {
				out = append(out, *a)
			}
		}
		return out, nil
	}
	for _, a := range s.SortedAliases() {
		if !nsFiltered || aliasNamespace(a) == nsFilter {
			out = append(out, a)
		}
	}
	return out, nil
}

// exportSealPassphrase reads the bundle passphrase from the environment
// or, failing that, prompts for it twice (with confirmation). promptW
// is where the prompts are written — stderr for export, so they never
// land in a bundle written to stdout.
func exportSealPassphrase(stdin io.Reader, promptW io.Writer) (string, error) {
	if p := os.Getenv(EnvExportPassphrase); p != "" {
		return p, nil
	}
	ir := auth.NewInputReader(stdin)
	p1, err := ir.ReadPassword("Set export passphrase: ", promptW)
	if err != nil {
		return "", err
	}
	p2, err := ir.ReadPassword("Confirm export passphrase: ", promptW)
	if err != nil {
		return "", err
	}
	if p1 != p2 {
		return "", errors.New("passphrases did not match")
	}
	return p1, nil
}

// --- import (bundle path) --------------------------------------------------

// importBundle decrypts an export bundle and writes its secrets into the
// current vault, regardless of which backend produced it. Existing
// aliases are skipped unless overwrite is set.
func importBundle(data []byte, fromStdin bool, homeDir string, stdin io.Reader, stdout, stderr io.Writer, prefix, namespaceOverride string, overwrite bool) int {
	pass := os.Getenv(EnvExportPassphrase)
	if pass == "" {
		if fromStdin {
			fmt.Fprintf(stderr, "error: reading the bundle from stdin, so set %s (stdin cannot also carry the passphrase prompt)\n", EnvExportPassphrase)
			return 1
		}
		ir := auth.NewInputReader(stdin)
		p, err := ir.ReadPassword("Export passphrase: ", stderr) // prompt to stderr; stdout carries import results
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		pass = p
	}

	plain, err := openExport(data, pass)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	var bundle exportBundle
	if err := json.Unmarshal(plain, &bundle); err != nil {
		fmt.Fprintf(stderr, "error: parsing bundle: %s\n", err)
		return 1
	}
	if bundle.Version > exportVersion {
		fmt.Fprintf(stderr, "error: bundle is schema version %d but this boru understands up to %d; upgrade boru\n", bundle.Version, exportVersion)
		return 1
	}

	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s.Locked {
		fmt.Fprintln(stderr, "error: vault is locked; run `boru vault unlock`")
		return 1
	}
	krStdin := stdin
	if fromStdin {
		krStdin = nil
	}
	sess, err := authenticate(s, homeDir, krStdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer sess.Close()
	if err := requireScope(sess, OpWrite); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	// Bundle names are stored-form, so they are kept as-is — the default
	// namespace never silently rewrites them. --namespace explicitly
	// remaps every imported alias into the given namespace (":" = root);
	// --prefix applies to the base name, inside the namespace.
	remap := false
	remapNS := ""
	switch namespaceOverride {
	case "":
	case rootNamespaceRef:
		remap = true
	default:
		if !validNamespaceName(namespaceOverride) {
			fmt.Fprintf(stderr, "error: invalid namespace %q\n", namespaceOverride)
			return 1
		}
		remap, remapNS = true, namespaceOverride
	}

	type imp struct {
		name, provider, metaNS string
		ipwl                   []string
	}
	var done []imp
	imported, skipped := 0, 0
	for _, a := range bundle.Aliases {
		ns, base := splitAlias(a.Name)
		if remap {
			ns = remapNS
		}
		name := prefix + base
		if ns != "" {
			name = ns + ":" + name
		}
		if !validAlias(name) {
			fmt.Fprintf(stderr, "warning: skipping invalid alias %q\n", name)
			continue
		}
		if a.Value == "" {
			fmt.Fprintf(stderr, "warning: skipping %s (empty value)\n", name)
			continue
		}
		if existing, _ := s.FindAlias(name); existing != nil && !overwrite {
			fmt.Fprintf(stderr, "skipping existing alias %s (use --overwrite to replace)\n", name)
			skipped++
			continue
		}
		if err := writeSecret(homeDir, sess, name, ns, a.Value); err != nil {
			fmt.Fprintf(stderr, "error: storing %s: %s\n", name, err)
			return 1
		}
		// Metadata namespace: derived from the final name; a root-level
		// name keeps the bundle's legacy tag unless root was an explicit
		// remap target.
		metaNS := ns
		if ns == "" && !remap {
			metaNS = a.Namespace
		}
		done = append(done, imp{name: name, provider: a.Provider, metaNS: metaNS, ipwl: a.IPWhitelist})
		_ = appendAudit(homeDir, AuditEvent{
			Action: "vault.import", Alias: name, Provider: a.Provider,
			Outcome: "ok", Reason: "source=bundle",
		})
		fmt.Fprintf(stdout, "imported %s\n", name)
		imported++
	}
	// Restore the custom presets the bundle carried alongside the aliases,
	// in the same store mutation. A name already present is kept unless
	// --overwrite; an entry with an un-mintable name (a tampered bundle)
	// is skipped so it can never be smuggled in.
	restoredProv := 0
	if err := mutateStore(homeDir, func(s *Store) error {
		for _, d := range done {
			s.UpsertAlias(Alias{Name: d.name, Provider: d.provider, Namespace: d.metaNS, Source: "import:bundle", IPWhitelist: d.ipwl})
		}
		for _, p := range bundle.CustomProviders {
			if !validCustomProviderName(p.Name) {
				fmt.Fprintf(stderr, "warning: skipping invalid custom provider %q in bundle\n", p.Name)
				continue
			}
			if _, idx := s.FindCustomProvider(p.Name); idx >= 0 && !overwrite {
				fmt.Fprintf(stderr, "skipping existing custom provider %q (use --overwrite to replace)\n", p.Name)
				continue
			}
			s.UpsertCustomProvider(p)
			restoredProv++
		}
		return nil
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if restoredProv > 0 {
		fmt.Fprintf(stdout, "restored %d custom provider(s)\n", restoredProv)
	}
	sealIntegrity(homeDir, sess)
	fmt.Fprintf(stdout, "imported %d secret(s)", imported)
	if skipped > 0 {
		fmt.Fprintf(stdout, ", skipped %d existing", skipped)
	}
	fmt.Fprintln(stdout)
	return 0
}

// --- bundle envelope crypto ------------------------------------------------

// sealExport encrypts plain under passphrase into the self-describing
// bundle envelope.
func sealExport(plain []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, keyringSaltLen)
	if _, err := randRead(salt); err != nil {
		return nil, err
	}
	key, err := c7scryptKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := gcmFromBlock(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := randRead(nonce); err != nil {
		return nil, err
	}
	header := append([]byte(exportMagic), byte(exportEnvelopeFormat))
	aad := append(append([]byte{}, header...), salt...)
	ct := gcm.Seal(nil, nonce, plain, aad)
	out := make([]byte, 0, len(header)+len(salt)+len(nonce)+len(ct))
	out = append(out, header...)
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

// openExport validates and decrypts a bundle envelope. A newer envelope
// format than this binary understands is reported clearly.
func openExport(blob []byte, passphrase string) ([]byte, error) {
	magicLen, ok := exportMagicLen(blob)
	if !ok || len(blob) < magicLen+1 {
		return nil, errors.New("vault: not a boru vault export bundle")
	}
	format := int(blob[magicLen])
	if format > exportEnvelopeFormat {
		return nil, fmt.Errorf("vault: export bundle is format %d but this boru understands up to %d; upgrade boru", format, exportEnvelopeFormat)
	}
	if format != 1 {
		return nil, fmt.Errorf("vault: unknown export bundle format %d", format)
	}
	off := magicLen + 1
	if len(blob) < off+keyringSaltLen+keyringNonceLen+16 {
		return nil, errors.New("vault: export bundle is truncated")
	}
	salt := blob[off : off+keyringSaltLen]
	nonce := blob[off+keyringSaltLen : off+keyringSaltLen+keyringNonceLen]
	ct := blob[off+keyringSaltLen+keyringNonceLen:]
	key, err := c7scryptKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := gcmFromBlock(block)
	if err != nil {
		return nil, err
	}
	aad := append(append([]byte{}, blob[:off]...), salt...)
	plain, err := gcm.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, errors.New("vault: wrong export passphrase or corrupt bundle")
	}
	return plain, nil
}

// isTerminalWriter reports whether w is an interactive terminal, so the
// exporter can refuse to dump a binary bundle onto it.
func isTerminalWriter(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return isTerminal(int(f.Fd()))
	}
	return false
}
