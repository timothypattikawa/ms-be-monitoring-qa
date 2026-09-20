package errors

import (
	"net/http"
	"testing"
)

func TestIs(t *testing.T) {
	errUpdateRepo := New(http.StatusInternalServerError, "01-00", "failed to update repository")
	errUpdateApproval := New(http.StatusInternalServerError, "02-00", "failed to update approval")
	errCreateApproval := New(http.StatusInternalServerError, "02-01", "failed to create approval")

	type args struct {
		err    error
		target error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "same",
			args: args{
				err:    errUpdateRepo,
				target: errUpdateRepo,
			},
			want: true,
		},
		{
			name: "same with underlying",
			args: args{
				err:    WithUnderlyingMsg(errUpdateRepo, "test"),
				target: errUpdateRepo,
			},
			want: true,
		},
		{
			name: "same with wrap",
			args: args{
				err:    Wrap(errUpdateRepo, "test"),
				target: errUpdateRepo,
			},
			want: true,
		},
		{
			name: "different code",
			args: args{
				err:    errUpdateApproval,
				target: errCreateApproval,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Is(tt.args.err, tt.args.target); got != tt.want {
				t.Errorf("Is() = %v, want %v", got, tt.want)
			}
		})
	}
}
