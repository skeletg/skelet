package config

// The settings the Signals suite reads, declared here because core renders and administers every one
// Their values live in the suite's database (Store: StoreSignals), which core reaches over the contract
func init() {
	// Lens - the beacon tier reads six of these per /t/config hit
	Register(Definition{Path: "signals/lens/enabled", Type: TypeBool, Default: "true", Label: "Web analytics enabled", Public: false, Store: StoreSignals})
	Register(Definition{Path: "signals/lens/sampling_rate", Type: TypeInt, Default: "100", Label: "Web analytics sampling rate (%)", Description: "Share of visitors recorded. Below 100 the totals are estimates, which is the trade for less storage on a busy store.", Public: false, Store: StoreSignals})
	Register(Definition{Path: "signals/lens/capture_moves", Type: TypeBool, Default: "true", Label: "Capture cursor-move heatmaps", Description: "Much the heaviest event stream - clicks and scroll depth are recorded either way.", Public: false, Store: StoreSignals})
	Register(Definition{Path: "signals/lens/retention_days", Scoping: ScopingGlobal, Type: TypeInt, Default: "90", Label: "Web analytics retention (days)", Public: false, Store: StoreSignals})
	// the per-day aggregates are a few rows per channel per day - keeping them long past the raw events is what preserves year-over-year trends
	Register(Definition{Path: "signals/lens/rollup_retention_days", Scoping: ScopingGlobal, Type: TypeInt, Default: "0", Label: "Daily traffic rollup retention (days, 0 = keep forever)", Public: false, Store: StoreSignals})
	Register(Definition{Path: "signals/lens/replay_enabled", Type: TypeBool, Default: "true", Label: "Session replay enabled", Public: false, Store: StoreSignals})
	Register(Definition{Path: "signals/lens/replay_sampling_rate", Type: TypeInt, Default: "10", Label: "Session replay sampling rate for anonymous visitors (%)", Description: "Signed-in customers are recorded per their own consent; this covers everyone else.", Public: false, Store: StoreSignals})
	Register(Definition{Path: "signals/lens/replay_max_minutes", Type: TypeInt, Default: "30", Label: "Session replay max recording length (minutes; 0 = no cap)", Public: false, Store: StoreSignals})
	Register(Definition{Path: "signals/lens/replay_retention_days", Scoping: ScopingGlobal, Type: TypeInt, Default: "30", Label: "Session replay retention (days)", Public: false, Store: StoreSignals})

	// Synapse's windows - lifecycle staging, engagement and the auto affinity segments
	// Set the lifecycle three ascending (new <= active <= at_risk), per the channel's repurchase cadence
	Register(Definition{Path: "signals/synapse/lifecycle_new_days", Type: TypeInt, Default: "30", Label: "Lifecycle: new within (days)", Public: false, Store: StoreSignals})
	Register(Definition{Path: "signals/synapse/lifecycle_active_days", Type: TypeInt, Default: "90", Label: "Lifecycle: active within (days)", Public: false, Store: StoreSignals})
	Register(Definition{Path: "signals/synapse/lifecycle_at_risk_days", Type: TypeInt, Default: "180", Label: "Lifecycle: at-risk within (days, lapsed beyond)", Public: false, Store: StoreSignals})
	Register(Definition{Path: "signals/synapse/return_window_days", Type: TypeInt, Default: "30", Label: "Engagement: return window (days)", Public: false, Store: StoreSignals})
	Register(Definition{Path: "signals/synapse/affinity_segment_min_actors", Type: TypeInt, Default: "5", Label: "Auto affinity segment: min customers per value", Public: false, Store: StoreSignals})
	Register(Definition{Path: "signals/synapse/affinity_segment_max_values", Type: TypeInt, Default: "20", Label: "Auto affinity segment: max values per attribute", Public: false, Store: StoreSignals})
	Register(Definition{Path: "signals/synapse/affinity_segment_window_days", Type: TypeInt, Default: "90", Label: "Auto affinity segment: affinity window (days)", Public: false, Store: StoreSignals})

	// Deliverability - the campaign send loop turns SMTP rejections into suppressions by these, and
	// core's bounce webhook reads the switch too, over the contract. Transactional mail is never gated
	Register(Definition{Path: "signals/deliverability/auto_suppress", Type: TypeBool, Default: "true", Label: "Auto-suppress on bounce / spam complaint", Description: "Stops mailing addresses that hard-bounced or complained. Off costs sender reputation.", Public: false, Store: StoreSignals})
	Register(Definition{Path: "signals/deliverability/soft_bounce_threshold", Type: TypeInt, Default: "3", Label: "Soft bounces before suppressing an address", Description: "A soft bounce is temporary (full mailbox, server down), so it takes several before the address is dropped.", Public: false, Store: StoreSignals})

	// The retention windows the suite's own sweeps read
	Register(Definition{Path: "signals/analytics/send_log_retention_months", Type: TypeInt, Default: "24", Label: "Email send-log retention (months, 0 = keep forever)", Public: false, Store: StoreSignals})
	// keep larger than any report window; 0 disables the prune (unbounded growth)
	Register(Definition{Path: "signals/analytics/spine_retention_days", Type: TypeInt, Default: "365", Label: "Analytics spine retention (days)", Description: "The raw per-event log behind every Signals report. Rollups outlive it, so trends survive a short window - individual events do not.", Public: false, Store: StoreSignals})
	// archive exports aged data to the BACKUP_S3_* destination (identity-stripped gzip JSONL) before deleting - with no destination configured the sweep fails closed and deletes nothing
	Register(Definition{Path: "signals/analytics/retention_action", Scoping: ScopingGlobal, Type: TypeSelect, Default: "prune", Label: "Retention action for aged analytics data", Description: "Prune deletes; archive writes identity-stripped copies to your backup bucket first.", Public: false, Store: StoreSignals, Options: []Option{
		{Value: "prune", Label: "Prune (delete)"},
		{Value: "archive", Label: "Archive to object storage, then delete"},
	}})
}
