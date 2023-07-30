package scenario_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tzneal/ccmon/scenario"
)

func TestOpen(t *testing.T) {

	tests := []struct {
		name     string
		contents string
		want     *scenario.Scenario
		wantErr  bool
	}{
		{
			name:     "empty",
			contents: ``,
			want:     nil,
			wantErr:  true,
		},
		{
			name:     "no events",
			contents: `name:  test`,
			want:     nil,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scenario.Open(strings.NewReader(tt.contents))
			if (err != nil) != tt.wantErr {
				t.Errorf("Open() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Open() got = %v, want %v", got, tt.want)
			}
		})
	}
}
