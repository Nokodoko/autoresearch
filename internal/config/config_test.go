package config

import (
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				Script:     "train.py",
				Metric:     "val_bpb",
				Direction:  "minimize",
				Parallel:   5,
				Timeout:    10 * time.Minute,
				LLMBackend: "claude",
			},
		},
		{
			name: "missing script",
			cfg: Config{
				Metric:     "val_bpb",
				Direction:  "minimize",
				Parallel:   5,
				Timeout:    10 * time.Minute,
				LLMBackend: "claude",
			},
			wantErr: true,
		},
		{
			name: "missing metric",
			cfg: Config{
				Script:     "train.py",
				Direction:  "minimize",
				Parallel:   5,
				Timeout:    10 * time.Minute,
				LLMBackend: "claude",
			},
			wantErr: true,
		},
		{
			name: "invalid direction",
			cfg: Config{
				Script:     "train.py",
				Metric:     "val_bpb",
				Direction:  "sideways",
				Parallel:   5,
				Timeout:    10 * time.Minute,
				LLMBackend: "claude",
			},
			wantErr: true,
		},
		{
			name: "invalid parallel",
			cfg: Config{
				Script:     "train.py",
				Metric:     "val_bpb",
				Direction:  "minimize",
				Parallel:   0,
				Timeout:    10 * time.Minute,
				LLMBackend: "claude",
			},
			wantErr: true,
		},
		{
			name: "invalid backend",
			cfg: Config{
				Script:     "train.py",
				Metric:     "val_bpb",
				Direction:  "minimize",
				Parallel:   5,
				Timeout:    10 * time.Minute,
				LLMBackend: "gemini",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDefaults(t *testing.T) {
	if DefaultParallel != 5 {
		t.Errorf("DefaultParallel: got %d, want 5", DefaultParallel)
	}
	if DefaultTimeout != 10*time.Minute {
		t.Errorf("DefaultTimeout: got %v, want 10m", DefaultTimeout)
	}
	if DefaultDirection != "minimize" {
		t.Errorf("DefaultDirection: got %s, want minimize", DefaultDirection)
	}
}
