package main

import "testing"

func Test_extractDate(t *testing.T) {
	type args struct {
		runID string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Enkel dato",
			args: args{
				runID: "manual__2023-02-13T131127.5671880000-27f960c46",
			},
			want:    "2023-02-13T131127",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractDate(tt.args.runID)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractDate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractDate() got = %v, want %v", got, tt.want)
			}
		})
	}
}
