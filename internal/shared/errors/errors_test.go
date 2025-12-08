package errors

import "testing"

func TestIs(t *testing.T) {
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
				err:    ErrUpdateRepo,
				target: ErrUpdateRepo,
			},
			want: true,
		},
		{
			name: "same with underlying",
			args: args{
				err:    WithUnderlyingMsg(ErrUpdateRepo, "test"),
				target: ErrUpdateRepo,
			},
			want: true,
		},
		{
			name: "same with wrap",
			args: args{
				err:    Wrap(ErrUpdateRepo, "test"),
				target: ErrUpdateRepo,
			},
			want: true,
		},
		{
			name: "different code",
			args: args{
				err:    ErrUpdateApproval,
				target: ErrCreateApproval,
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
