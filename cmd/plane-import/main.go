// Command plane-import loads a Plane export contract (JSONL produced from the
// Plane database) into a Windshift workspace, preserving item numbers,
// authors, and timestamps. See internal/planeimport for the fidelity contract.
//
// Usage:
//
//	plane-import --connection "$POSTGRES_CONNECTION_STRING" \
//	  --workspace VECTIS --contract vectis.jsonl --users users.json \
//	  --alias hakerx1@gmail.com=oskars_germovs@inbox.lv [--dry-run] [--batch 200]
//
// By default the command runs the Windshift migration catalog first (that is
// how plane_import_mappings gets created). --skip-migrate skips it when the
// target schema is already managed by the application deployment.
//
// The JSON report goes to stdout; progress lines to stderr. The run is
// idempotent: reruns skip every source uuid already in plane_import_mappings
// and fail loudly on any number collision the importer does not own.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"windshift/internal/database"
	"windshift/internal/planeimport"
)

type aliasList struct {
	aliases map[string]string
}

func (a *aliasList) String() string {
	parts := make([]string, 0, len(a.aliases))
	for k, v := range a.aliases {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func (a *aliasList) Set(value string) error {
	eq := strings.Index(value, "=")
	if eq <= 0 {
		return fmt.Errorf("alias must be planeEmail=windshiftEmail, got %q", value)
	}
	if a.aliases == nil {
		a.aliases = map[string]string{}
	}
	a.aliases[strings.ToLower(strings.TrimSpace(value[:eq]))] = strings.ToLower(strings.TrimSpace(value[eq+1:]))
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "plane-import:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		connection = flag.String("connection", envOr("POSTGRES_CONNECTION_STRING", ""), "Windshift Postgres connection string (default $POSTGRES_CONNECTION_STRING)")
		workspace  = flag.String("workspace", "", "destination workspace key (required)")
		wsSlug     = flag.String("workspace-slug", "", "source Plane workspace slug; when set it must match the contract meta (guards against a wrong dump file)")
		contract   = flag.String("contract", "", "path to the JSONL import contract (required)")
		usersPath  = flag.String("users", "", "path to the users sidecar JSON array (optional)")
		dryRun     = flag.Bool("dry-run", false, "parse, map, and report without writing")
		strict     = flag.Bool("strict", false, "fail when an author cannot be resolved")
		skipMig    = flag.Bool("skip-migrate", false, "do not run Initialize()/migrations: use against a deployment whose schema the app manages")
		batchSize  = flag.Int("batch", 200, "issues committed per transaction")
		aliases    aliasList
	)
	flag.Var(&aliases, "alias", "planeEmail=windshiftEmail author alias (repeatable)")
	flag.Parse()

	if *workspace == "" || *contract == "" {
		flag.Usage()
		return fmt.Errorf("--workspace and --contract are required")
	}
	if *connection == "" {
		return fmt.Errorf("no connection string (--connection or $POSTGRES_CONNECTION_STRING)")
	}

	db, err := database.NewPostgresDB(*connection, 4)
	if err != nil {
		return fmt.Errorf("open windshift database: %w", err)
	}
	if !*skipMig {
		// Initialize() applies every catalog migration (including this
		// importer's plane_import_mappings). Against a live deployment that
		// means the schema moves to this binary's version — forward-only, the
		// same upgrade the app itself runs at boot. --skip-migrate instead
		// requires the operator to have applied those migrations already.
		if err := db.Initialize(); err != nil {
			return fmt.Errorf("initialize windshift database: %w", err)
		}
	}
	defer func() { _ = db.Close() }()

	loader := planeimport.NewLoader(db, planeimport.Options{
		WorkspaceKey: *workspace,
		SourceSlug:   *wsSlug,
		ContractPath: *contract,
		UsersPath:    *usersPath,
		Aliases:      aliases.aliases,
		Strict:       *strict,
		DryRun:       *dryRun,
		BatchSize:    *batchSize,
	})
	if err := loader.LoadContract(); err != nil {
		return err
	}
	if err := loader.Resolve(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "plane-import: %d issues, %d comments in contract; %s destination\n",
		loader.IssueCount(), loader.CommentCount(), *workspace)

	report, err := loader.Run(context.Background())
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
