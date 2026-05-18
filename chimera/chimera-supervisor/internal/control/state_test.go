package control

import "testing"

func TestContractStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   Snapshot
		want string
	}{
		{
			name: "broker required unready",
			in: Snapshot{
				BrokerRequired: true,
				BrokerReady:    false,
			},
			want: "degraded",
		},
		{
			name: "vectorstore required unready",
			in: Snapshot{
				BrokerRequired:      true,
				BrokerReady:         true,
				VectorstoreRequired: true,
				VectorstoreReady:    false,
			},
			want: "degraded",
		},
		{
			name: "all required ready",
			in: Snapshot{
				BrokerRequired:      true,
				BrokerReady:         true,
				VectorstoreRequired: true,
				VectorstoreReady:    true,
			},
			want: "ok",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ContractStatus(tc.in); got != tc.want {
				t.Fatalf("ContractStatus()=%q want %q", got, tc.want)
			}
		})
	}
}
