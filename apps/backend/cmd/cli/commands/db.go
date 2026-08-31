package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/skeletg/skelet/apps/backend/internal/backup"
	"github.com/spf13/cobra"
)

// `db backup` targets the BACKUP_S3_* env only on purpose - a runtime-editable destination would be an exfiltration channel; local dev snapshots are a different tool (bin/db-export / bin/db-import)
var DbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database operations (backups to dedicated storage)",
}

var dbBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Stream a pg_dump into the configured dedicated object storage",
	RunE:  runDbBackup,
}

var dbBackupsCmd = &cobra.Command{
	Use:   "backups",
	Short: "List backups in the configured bucket (oldest first)",
	RunE:  runDbBackups,
}

var (
	dbRestoreYes  bool
	dbRestoreJobs int
)

var dbRestoreCmd = &cobra.Command{
	Use:   "restore <key>",
	Short: "Stream a backup from the bucket into pg_restore (DESTRUCTIVE - drops and recreates objects)",
	Long: "Restores one dump listed by `db backups` into the matching database: skelet-signals-* keys\n" +
		"go to SIGNALS_DB_URL, everything else to DATABASE_URL. Runs pg_restore --clean --if-exists,\n" +
		"so existing objects are dropped first - stop the app before restoring, and restore the\n" +
		"commerce and signals dumps of the SAME timestamp together.\n" +
		"The dump spools to a temp file first (set TMPDIR to a volume for very large dumps);\n" +
		"--jobs parallelizes the data/index phases and is the difference between hours and\n" +
		"minutes on a big database.",
	Args: cobra.ExactArgs(1),
	RunE: runDbRestore,
}

func init() {
	DbCmd.AddCommand(dbBackupCmd)
	DbCmd.AddCommand(dbBackupsCmd)
	dbRestoreCmd.Flags().BoolVar(&dbRestoreYes, "yes", false, "confirm overwriting the live database")
	dbRestoreCmd.Flags().IntVar(&dbRestoreJobs, "jobs", 1, "parallel pg_restore workers (data + index phases)")
	DbCmd.AddCommand(dbRestoreCmd)
}

// Env-only: destination + creds come from BACKUP_* vars, no DB connection needed
func newBackupService() *backup.Service {
	return backup.NewService(os.Getenv("DATABASE_URL"), os.Getenv("SIGNALS_DB_URL"))
}

func runDbRestore(cmd *cobra.Command, args []string) error {
	if !dbRestoreYes {
		return fmt.Errorf("restore overwrites the live database - rerun with --yes to confirm")
	}
	ctx := context.Background()
	svc := newBackupService()

	if err := svc.Restore(ctx, args[0], dbRestoreJobs); err != nil {
		return err
	}
	fmt.Printf("restored %s\n", args[0])
	return nil
}

func runDbBackup(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc := newBackupService()

	res, err := svc.Run(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("uploaded %s (%d bytes)\n", res.Key, res.Bytes)
	if res.SignalsKey != "" {
		fmt.Printf("uploaded %s (%d bytes)\n", res.SignalsKey, res.SignalsBytes)
	}
	for _, p := range res.Pruned {
		fmt.Printf("pruned %s\n", p)
	}
	return nil
}

func runDbBackups(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc := newBackupService()

	keys, err := svc.List(ctx)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		fmt.Println("no backups in the bucket")
		return nil
	}
	for _, k := range keys {
		fmt.Println(k)
	}
	return nil
}
