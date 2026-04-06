package main

import "testing"

func TestParseSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    uint64
		wantErr bool
	}{
		{name: "bytes", in: "1024", want: 1024},
		{name: "gib", in: "2G", want: 2 << 30},
		{name: "overflow", in: "18446744073709551615T", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseSize(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseSize(%q) expected error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSize(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("parseSize(%q)=%d want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseChunkSizeMiB(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      int
		want    uint64
		wantErr bool
	}{
		{name: "valid", in: 4, want: 4 << 20},
		{name: "zero", in: 0, wantErr: true},
		{name: "negative", in: -1, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseChunkSizeMiB(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseChunkSizeMiB(%d) expected error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseChunkSizeMiB(%d) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("parseChunkSizeMiB(%d)=%d want %d", tt.in, got, tt.want)
			}
		})
	}
}

