package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalogio"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/orderio"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	"github.com/skeletg/skelet/apps/backend/core/lib/dbconn"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// Runs the same registry the dashboard drives, straight through rather than as a job - an operator at a
// terminal already has somewhere to stream to, so the object store is not in the loop
var DataCmd = &cobra.Command{
	Use:   "data",
	Short: "Bulk import and export",
}

var (
	dataChannel string
	dataLocale  string
	dataFormat  string
	dataOut     string
	dataDryRun  bool
	dataOptions []string
)

var dataListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the registered resources and their columns",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDataList,
}

var dataExportCmd = &cobra.Command{
	Use:   "export <resource>",
	Short: "Export a resource",
	Args:  cobra.ExactArgs(1),
	RunE:  runDataExport,
}

var dataImportCmd = &cobra.Command{
	Use:   "import <resource> <file>",
	Short: "Import a resource (upsert on its identifier columns)",
	Args:  cobra.ExactArgs(2),
	RunE:  runDataImport,
}

var dataTemplateCmd = &cobra.Command{
	Use:   "template <resource>",
	Short: "Print a header-only file with one example row",
	Args:  cobra.ExactArgs(1),
	RunE:  runDataTemplate,
}

func init() {
	for _, c := range []*cobra.Command{dataExportCmd, dataImportCmd, dataTemplateCmd} {
		c.Flags().StringVar(&dataFormat, "format", "csv", "csv or jsonl")
	}
	for _, c := range []*cobra.Command{dataExportCmd, dataImportCmd} {
		c.Flags().StringVar(&dataChannel, "channel", "", "channel slug the run targets")
		c.Flags().StringVar(&dataLocale, "locale", "en", "locale names are read and written under")
		c.Flags().StringArrayVar(&dataOptions, "opt", nil, "resource option as name=value, repeatable")
	}
	dataExportCmd.Flags().StringVarP(&dataOut, "out", "o", "", "output file (default: stdout)")
	dataImportCmd.Flags().BoolVar(&dataDryRun, "dry-run", false, "validate and report counts without writing")
	for _, c := range []*cobra.Command{dataListCmd, dataExportCmd, dataImportCmd, dataTemplateCmd} {
		c.SilenceUsage = true // a failed row is a data outcome, not a usage error - the help text is noise there
	}

	DataCmd.AddCommand(dataListCmd, dataExportCmd, dataImportCmd, dataTemplateCmd)
}

func runDataList(_ *cobra.Command, args []string) error {
	svcs, err := newDataServices()
	if err != nil {
		return err
	}
	defer svcs.cleanup()

	if len(args) == 1 {
		res, err := svcs.registry.Get(args[0])
		if err != nil {
			return err
		}
		printResource(res)
		return nil
	}
	group := ""
	for _, res := range svcs.registry.List() {
		if res.Group != group {
			group = res.Group
			fmt.Printf("\n%s\n", strings.ToUpper(group))
		}
		fmt.Printf("  %-20s %-22s %s\n", res.Slug, directions(res), res.Description)
	}
	fmt.Println()
	return nil
}

func directions(res dataio.Resource) string {
	switch {
	case res.CanImport() && res.CanExport():
		return "import + export"
	case res.CanImport():
		return "import only"
	default:
		return "export only"
	}
}

func printResource(res dataio.Resource) {
	fmt.Printf("%s (%s)\n%s\n\nCOLUMNS\n", res.Label, directions(res), res.Description)
	for _, f := range res.Fields {
		flags := []string{}
		if f.Identifier {
			flags = append(flags, "key")
		}
		if f.Required {
			flags = append(flags, "required")
		}
		if f.ReadOnly {
			flags = append(flags, "read-only")
		}
		fmt.Printf("  %-22s %-10s %-22s %s\n", f.Name, f.Type, strings.Join(flags, ","), f.Description)
	}
	if len(res.Options) > 0 {
		fmt.Println("\nOPTIONS")
		for _, o := range res.Options {
			fmt.Printf("  --opt %s=<%s>%s  %s\n", o.Name, o.Type, requiredMark(o.Required), o.Description)
		}
	}
	if len(res.DynamicPrefixes) > 0 {
		fmt.Printf("\nAlso accepts columns prefixed: %s\n", strings.Join(res.DynamicPrefixes, ", "))
	}
}

func requiredMark(required bool) string {
	if required {
		return " (required)"
	}
	return ""
}

func runDataExport(_ *cobra.Command, args []string) error {
	svcs, err := newDataServices()
	if err != nil {
		return err
	}
	defer svcs.cleanup()
	ctx := context.Background()

	run, err := svcs.run(ctx, args[0])
	if err != nil {
		return err
	}
	var w io.Writer = os.Stdout
	if dataOut != "" {
		f, err := os.Create(dataOut)
		if err != nil {
			return fmt.Errorf("create %s: %w", dataOut, err)
		}
		defer f.Close()
		w = f
	}
	res, err := svcs.registry.Export(ctx, run, w)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "exported %d rows, %d columns\n", res.Rows, len(res.Columns))
	return nil
}

func runDataImport(_ *cobra.Command, args []string) error {
	svcs, err := newDataServices()
	if err != nil {
		return err
	}
	defer svcs.cleanup()
	ctx := context.Background()

	run, err := svcs.run(ctx, args[0])
	if err != nil {
		return err
	}
	run.Opts.DryRun = dataDryRun

	f, err := os.Open(args[1])
	if err != nil {
		return fmt.Errorf("open %s: %w", args[1], err)
	}
	defer f.Close()

	res, err := svcs.registry.Import(ctx, run, f)
	if err != nil {
		return err
	}
	if dataDryRun {
		fmt.Println("DRY RUN - nothing was written")
	}
	for _, warning := range res.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}
	fmt.Printf("created: %d, updated: %d, skipped: %d, failed: %d\n", res.Created, res.Updated, res.Skipped, res.Failed)
	for _, e := range res.Errors {
		fmt.Fprintf(os.Stderr, "  line %d (%s): %s\n", e.Line, e.Ref, e.Message)
	}
	if res.Truncated {
		fmt.Fprintln(os.Stderr, "  ... more errors than were reported")
	}
	if len(res.Errors) > 0 {
		return fmt.Errorf("%d row(s) failed", len(res.Errors))
	}
	return nil
}

func runDataTemplate(_ *cobra.Command, args []string) error {
	svcs, err := newDataServices()
	if err != nil {
		return err
	}
	defer svcs.cleanup()
	return svcs.registry.Template(context.Background(), args[0], dataio.Format(dataFormat), os.Stdout)
}

type dataServices struct {
	registry *dataio.Registry
	channel  *channel.Service
	cleanup  func()
}

// Builds the same Run the job runner would, so a CLI import and a dashboard import are the same code path
func (s *dataServices) run(ctx context.Context, resource string) (dataio.Run, error) {
	values := map[string]string{}
	for _, raw := range dataOptions {
		name, value, ok := strings.Cut(raw, "=")
		if !ok {
			return dataio.Run{}, fmt.Errorf("--opt wants name=value, got %q", raw)
		}
		values[strings.TrimSpace(name)] = strings.TrimSpace(value)
	}
	opts := dataio.NewOpts(values)
	opts.Locale = dataLocale
	if dataChannel != "" {
		ch, err := s.channel.GetBySlug(ctx, dataChannel)
		if err != nil {
			return dataio.Run{}, fmt.Errorf("channel %q: %w", dataChannel, err)
		}
		opts.ChannelID = ch.ID
	}
	return dataio.Run{Resource: resource, Format: dataio.Format(dataFormat), Opts: opts}, nil
}

// Only the resources whose services a CLI process can stand up cheaply - customers, promotions and destinations
// pull in most of the graph (mailers, consent, the event bus), which this command does not justify
func newDataServices() (*dataServices, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}
	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	queries := db.New(conn)
	txMgr := txrunner.New(conn)
	localeSvc := locale.NewService(locale.NewRepository(queries))
	channelSvc := channel.NewService(channel.NewRepository(queries, txMgr))
	channelSvc.SetLocaleValidator(localeSvc)
	catalogSvc := catalog.NewService(catalog.Deps{
		Repo:    catalog.NewRepository(queries, txMgr),
		Tx:      txMgr,
		Channel: channelSvc,
		Locales: localeSvc,
	})
	inventorySvc := inventory.NewService(inventory.NewRepository(queries, txMgr), txMgr)
	taxSvc := tax.NewService(tax.NewRepository(queries))
	taxSvc.SetTx(txMgr)

	reg := dataio.NewRegistry()
	reg.Register(catalogio.Resource(queries, catalogSvc))
	reg.Register(catalogio.CategoryResource(catalogSvc))
	reg.Register(orderio.OrderLineResource(queries))
	reg.Register(inventory.LevelResource(queries, inventorySvc, cliSKULookup{catalogSvc}))
	reg.Register(tax.RateResource(taxSvc))

	return &dataServices{registry: reg, channel: channelSvc, cleanup: func() { conn.Close() }}, nil
}

type cliSKULookup struct{ cat *catalog.Service }

func (l cliSKULookup) ProductIDBySKU(ctx context.Context, sku string) (uuid.UUID, error) {
	p, err := l.cat.GetProductBySKU(ctx, sku)
	if err != nil {
		return uuid.Nil, err
	}
	return p.ID, nil
}
