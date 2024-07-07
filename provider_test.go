package route53

import (
	"testing"
)

func TestProvider_shouldWaitForDeletePropagation(t *testing.T) {
	type fields struct {
		WaitForPropagation       bool
		WaitForDeletePropagation waitForDeletePropagationOption
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "backwards compatability - should wait",
			fields: fields{
				WaitForPropagation: true,
			},
			want: true,
		},
		{
			name: "backwards compatability - should not wait",
			fields: fields{
				WaitForPropagation: false,
			},
			want: false,
		},
		{
			name: "opposite from WaitForPropagation - should wait",
			fields: fields{
				WaitForPropagation:       false,
				WaitForDeletePropagation: AlwaysWaitForDeletePropagation,
			},
			want: true,
		},
		{
			name: "opposite from WaitForPropagation - should wait",
			fields: fields{
				WaitForPropagation:       true,
				WaitForDeletePropagation: NeverWaitForDeletePropagation,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Provider{
				WaitForPropagation:       tt.fields.WaitForPropagation,
				WaitForDeletePropagation: tt.fields.WaitForDeletePropagation,
			}
			if got := p.shouldWaitForDeletePropagation(); got != tt.want {
				t.Errorf("Provider.shouldWaitForDeletePropagation() = %v, want %v", got, tt.want)
			}
		})
	}
}
