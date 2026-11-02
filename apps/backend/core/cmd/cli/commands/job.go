package commands

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/scheduler"
	"github.com/skeletg/skelet/apps/backend/core/lib/dbconn"
)

var JobCmd = &cobra.Command{
	Use:   "job",
	Short: "Scheduled jobs - list them, or pull one forward to run on the next tick",
}

var jobListCmd = &cobra.Command{
	Use:   "list",
	Short: "Every scheduled job with its schedule and whether it is enabled",
	Args:  cobra.NoArgs,
	RunE:  runJobList,
}

var jobRunCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Run a job now - the app's scheduler claims it on its next tick, within its poll interval",
	Long: `Pulls the named job's next run forward to now. The running app or worker process
executes it, records the run like any other, and the regular schedule resumes after.

  skelet cli job run customer-roster-push
  skelet cli job run trait-compute`,
	Args: cobra.ExactArgs(1),
	RunE: runJobRun,
}

func init() {
	JobCmd.AddCommand(jobListCmd)
	JobCmd.AddCommand(jobRunCmd)
}

func jobDeps() (*scheduler.Service, func(), error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, nil, fmt.Errorf("DATABASE_URL is not set")
	}
	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to database: %w", err)
	}
	return scheduler.New(db.New(conn), false), func() { conn.Close() }, nil
}

func runJobList(cmd *cobra.Command, _ []string) error {
	jobs, closeFn, err := jobDeps()
	if err != nil {
		return err
	}
	defer closeFn()
	rows, err := jobs.List(context.Background())
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tKIND\tSCHEDULE\tENABLED")
	for _, j := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%v\n", j.Name, j.Kind, j.Schedule, j.Enabled)
	}
	return w.Flush()
}

func runJobRun(cmd *cobra.Command, args []string) error {
	jobs, closeFn, err := jobDeps()
	if err != nil {
		return err
	}
	defer closeFn()
	if err := jobs.RunNowByName(context.Background(), args[0]); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s is due now - the scheduler picks it up on its next tick\n", args[0])
	return nil
}
