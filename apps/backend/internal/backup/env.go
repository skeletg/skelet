package backup

import (
	"os"
	"strconv"
	"strings"
)

// The destination and credentials are DELIBERATELY environment-only, never admin config - a runtime-editable destination lets anyone with settings access redirect the nightly full dump to a bucket they control, a complete-database exfiltration channel that bypasses every per-channel scope and PII gate, so env pins backups to the deployment trust boundary
// BACKUP_S3_BUCKET is what enables backups at all, and BACKUP_ENCRYPTION_PASSPHRASE encrypts dumps with age - strongly recommended, but losing the passphrase loses the backups
// BACKUP_HEARTBEAT_URL is a dead-man's switch, pinged only after a fully successful run - the scheduler alerts on job FAILURES, so the one thing it structurally cannot report is the job never running at all (app down, or nothing ever claimed it), and a monitor watching for pings to stop covers exactly that
type envConfig struct {
	Bucket       string
	Endpoint     string
	Region       string
	Prefix       string
	Keep         int
	PathStyle    bool
	AccessKey    string
	SecretKey    string
	Passphrase   string
	HeartbeatURL string
}

func loadEnv() envConfig {
	cfg := envConfig{
		Bucket:       strings.TrimSpace(os.Getenv("BACKUP_S3_BUCKET")),
		Endpoint:     strings.TrimSpace(os.Getenv("BACKUP_S3_ENDPOINT_URL")),
		Region:       strings.TrimSpace(os.Getenv("BACKUP_S3_REGION")),
		Prefix:       os.Getenv("BACKUP_S3_PREFIX"),
		Keep:         30,
		PathStyle:    true,
		AccessKey:    os.Getenv("BACKUP_S3_ACCESS_KEY_ID"),
		SecretKey:    os.Getenv("BACKUP_S3_SECRET_ACCESS_KEY"),
		Passphrase:   os.Getenv("BACKUP_ENCRYPTION_PASSPHRASE"),
		HeartbeatURL: strings.TrimSpace(os.Getenv("BACKUP_HEARTBEAT_URL")),
	}
	if cfg.Prefix == "" {
		cfg.Prefix = "db/"
	}
	if n, err := strconv.Atoi(os.Getenv("BACKUP_KEEP")); err == nil && n >= 1 {
		cfg.Keep = n
	}
	if v, err := strconv.ParseBool(os.Getenv("BACKUP_S3_PATH_STYLE")); err == nil {
		cfg.PathStyle = v
	}
	return cfg
}
