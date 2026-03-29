package metrics

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		output  string
		want    float64
		wantErr bool
	}{
		{
			name:    "val_bpb standard format",
			pattern: `^val_bpb:\s+([\d.]+)`,
			output:  "step 00953 (100.0%)\n---\nval_bpb:          0.997900\ntraining_seconds: 300.1\n",
			want:    0.997900,
		},
		{
			name:    "accuracy metric",
			pattern: `^accuracy:\s+([\d.]+)`,
			output:  "accuracy: 0.95\nloss: 0.05\n",
			want:    0.95,
		},
		{
			name:    "metric not found",
			pattern: `^val_bpb:\s+([\d.]+)`,
			output:  "training complete\nno metric here\n",
			wantErr: true,
		},
		{
			name:    "multiple lines",
			pattern: `^score:\s+([\d.]+)`,
			output:  "info: running\nscore: 42.5\ninfo: done\n",
			want:    42.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser(tt.pattern, "minimize")
			got, err := p.Parse(tt.output)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("got %.6f, want %.6f", got, tt.want)
			}
		})
	}
}

func TestIsBetter(t *testing.T) {
	minParser := NewParser("", "minimize")
	maxParser := NewParser("", "maximize")

	if !minParser.IsBetter(0.5, 1.0) {
		t.Error("minimize: 0.5 should be better than 1.0")
	}
	if minParser.IsBetter(1.0, 0.5) {
		t.Error("minimize: 1.0 should not be better than 0.5")
	}

	if !maxParser.IsBetter(1.0, 0.5) {
		t.Error("maximize: 1.0 should be better than 0.5")
	}
	if maxParser.IsBetter(0.5, 1.0) {
		t.Error("maximize: 0.5 should not be better than 1.0")
	}
}

func TestIsBetterByThreshold(t *testing.T) {
	p := NewParser("", "minimize")

	if !p.IsBetterByThreshold(0.90, 1.0, 0.05) {
		t.Error("0.90 is 0.10 better than 1.0, should pass threshold 0.05")
	}
	if p.IsBetterByThreshold(0.96, 1.0, 0.05) {
		t.Error("0.96 is 0.04 better than 1.0, should NOT pass threshold 0.05")
	}
}
