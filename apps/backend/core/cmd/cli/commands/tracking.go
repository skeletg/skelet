package commands

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/skeletg/skelet/apps/backend/core/internal/configstore"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/core/internal/tracking"
	"github.com/skeletg/skelet/apps/backend/core/internal/webhook"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/dbconn"
	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Until the dashboard pages land, this is how a destination is set up and checked
var TrackingCmd = &cobra.Command{
	Use:   "tracking",
	Short: "Server-side tracking destinations: list, add, inspect deliveries, send a test event",
}

var (
	trackChannel string
	trackKind    string
	trackName    string
	trackSet     []string
	trackSecret  string
	trackLimit   int32
)

func init() {
	kinds := &cobra.Command{Use: "kinds", Short: "The platforms this build can send to, and the settings each needs", Args: cobra.NoArgs, RunE: runTrackingKinds}
	list := &cobra.Command{Use: "destinations", Short: "A channel's destinations", Args: cobra.NoArgs, RunE: runTrackingList}
	list.Flags().StringVar(&trackChannel, "channel", "", "channel id (required)")
	add := &cobra.Command{
		Use:   "add",
		Short: "Add a destination to a channel",
		Long:  "The secret is read from TRACKING_SECRET when --secret is not given, so it needn't land in shell history.",
		Example: `  skelet cli tracking add --channel <id> --kind ga4 --name "GA4" --set measurement_id=G-XXXX --set validate=true
  TRACKING_SECRET=... skelet cli tracking add --channel <id> --kind meta_capi --name Meta --set pixel_id=123 --set test_event_code=TEST1`,
		Args: cobra.NoArgs,
		RunE: runTrackingAdd,
	}
	add.Flags().StringVar(&trackChannel, "channel", "", "channel id (required)")
	add.Flags().StringVar(&trackKind, "kind", "", "kind, from `tracking kinds` (required)")
	add.Flags().StringVar(&trackName, "name", "", "a name for it (required)")
	add.Flags().StringArrayVar(&trackSet, "set", nil, "a setting as key=value, repeatable")
	add.Flags().StringVar(&trackSecret, "secret", "", "the secret setting, else TRACKING_SECRET")
	deliveries := &cobra.Command{Use: "deliveries <destination-id>", Short: "A destination's latest deliveries, newest first", Args: cobra.ExactArgs(1), RunE: runTrackingDeliveries}
	deliveries.Flags().Int32Var(&trackLimit, "limit", 20, "how many")
	test := &cobra.Command{Use: "test <destination-id>", Short: "Send one sample purchase now and print the platform's answer", Args: cobra.ExactArgs(1), RunE: runTrackingTest}
	TrackingCmd.AddCommand(kinds, list, add, deliveries, test)
}

func trackingService() (*tracking.Service, func(), error) {
	conn, err := dbconn.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		return nil, nil, fmt.Errorf("connect to database: %w", err)
	}
	q := db.New(conn)
	cfg := config.NewService(configstore.New(q))
	var sealer tracking.Sealer
	if rawKey := strings.TrimSpace(os.Getenv("CONFIG_ENCRYPTION_KEY")); rawKey != "" {
		cipher, err := secrets.NewCipher(rawKey)
		if err != nil {
			conn.Close()
			return nil, nil, fmt.Errorf("CONFIG_ENCRYPTION_KEY invalid: %w", err)
		}
		sealer = cipher
	}
	registerTrackingKinds()
	return tracking.NewService(q, sealer, cfg), func() { conn.Close() }, nil
}

var trackingKindsDone bool

func registerTrackingKinds() {
	if !trackingKindsDone {
		trackingKindsDone = true
		tracking.RegisterPluginKinds(sdk.Destinations(), plugin.Enabled)
	}
}

func runTrackingKinds(cmd *cobra.Command, _ []string) error {
	registerTrackingKinds()
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "KIND\tLABEL\tNEEDS\tSETTINGS")
	for _, k := range tracking.Kinds() {
		var fields []string
		for _, f := range k.Fields {
			name := f.Key
			if f.Secret {
				name += " (secret)"
			}
			if f.Required {
				name += "*"
			}
			fields = append(fields, name)
		}
		needs := make([]string, len(k.Purposes))
		for i, p := range k.Purposes {
			needs[i] = string(p)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", k.Kind, k.Label, strings.Join(needs, "+"), strings.Join(fields, ", "))
	}
	return w.Flush()
}

func runTrackingList(cmd *cobra.Command, _ []string) error {
	ch, err := uuid.Parse(trackChannel)
	if err != nil {
		return fmt.Errorf("--channel: %w", err)
	}
	svc, done, err := trackingService()
	if err != nil {
		return err
	}
	defer done()
	dests, err := svc.List(cmd.Context(), ch)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tKIND\tNAME\tENABLED\tEVENTS")
	for _, d := range dests {
		fmt.Fprintf(w, "%s\t%s\t%s\t%v\t%s\n", d.ID, d.Kind, d.Name, d.Enabled, strings.Join(d.Events, ","))
	}
	return w.Flush()
}

func runTrackingAdd(cmd *cobra.Command, _ []string) error {
	ch, err := uuid.Parse(trackChannel)
	if err != nil {
		return fmt.Errorf("--channel: %w", err)
	}
	cfg := map[string]string{}
	for _, kv := range trackSet {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return fmt.Errorf("--set %q: want key=value", kv)
		}
		cfg[k] = v
	}
	secret := trackSecret
	if secret == "" {
		secret = os.Getenv("TRACKING_SECRET")
	}
	svc, done, err := trackingService()
	if err != nil {
		return err
	}
	defer done()
	d, err := svc.Create(cmd.Context(), tracking.Destination{ChannelID: ch, Kind: trackKind, Name: trackName, Enabled: true, Config: cfg, Secret: secret})
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "added %s (%s) - check it with: skelet cli tracking test %s\n", d.ID, d.Kind, d.ID)
	return nil
}

func runTrackingDeliveries(cmd *cobra.Command, args []string) error {
	id, err := uuid.Parse(args[0])
	if err != nil {
		return err
	}
	svc, done, err := trackingService()
	if err != nil {
		return err
	}
	defer done()
	rows, err := svc.Deliveries(cmd.Context(), id, trackLimit)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "WHEN\tEVENT\tSTATUS\tATTEMPTS\tLAST ERROR")
	for _, d := range rows {
		fmt.Fprintf(w, "%s\t%s %s\t%s\t%d\t%s\n", d.CreatedAt.Format(time.DateTime), d.EventName, d.EventID, d.Status, d.Attempts, d.LastError)
	}
	return w.Flush()
}

func runTrackingTest(cmd *cobra.Command, args []string) error {
	id, err := uuid.Parse(args[0])
	if err != nil {
		return err
	}
	svc, done, err := trackingService()
	if err != nil {
		return err
	}
	defer done()
	client := &http.Client{Transport: webhook.SafeTransport(webhook.AllowPrivateTargetsFromEnv()), Timeout: 30 * time.Second}
	ev, err := svc.SendTest(cmd.Context(), client, id)
	if err != nil {
		return fmt.Errorf("test %s failed: %w", ev.ID, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "sent test purchase %s - accepted\n", ev.ID)
	return nil
}
