package parallel

// ChannelConfig holds per-channel configuration.
type ChannelConfig struct {
	ID          int
	WorktreeDir string
	BranchName  string
}
