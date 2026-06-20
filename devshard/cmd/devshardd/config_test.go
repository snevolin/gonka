package main

import "testing"

func TestValidateBinaryLogVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		env     string
		link    string
		want    string
		wantErr bool
	}{
		{name: "standalone uses link stamp", env: "", link: "0.2.13-v2-r2", want: "0.2.13-v2-r2"},
		{name: "versiond match", env: "0.2.13-v2-r2", link: "0.2.13-v2-r2", want: "0.2.13-v2-r2"},
		{name: "mismatch", env: "0.2.12-v2-r1", link: "0.2.13-v2-r2", wantErr: true},
		{name: "env without link", env: "0.2.13-v2-r2", link: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateBinaryLogVersion(tt.env, tt.link)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
