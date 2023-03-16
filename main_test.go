package main

import "testing"

func Test_getGitRepo(t *testing.T) {
	type args struct {
		repoPath string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "HTTP Git clone config",
			args: args{
				repoPath: "testdata/knada-git-sync.config",
			},
			want:    "github.com/navikt/knada-git-sync",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getGitRepo(tt.args.repoPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("getGitRepo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getGitRepo() got = %v, want %v", got, tt.want)
			}
		})
	}
}
