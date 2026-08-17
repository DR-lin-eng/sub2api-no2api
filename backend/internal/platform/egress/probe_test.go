package egress

import "testing"

func TestExtractObservedIPv6(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "plain", body: "2001:db8::10\n", want: "2001:db8::10"},
		{name: "json ip", body: `{"ip":"2001:db8::11"}`, want: "2001:db8::11"},
		{name: "json origin", body: `{"origin":"2001:db8::12, 2001:db8::13"}`, want: "2001:db8::12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractObservedIPv6([]byte(tt.body))
			if err != nil || got.String() != tt.want {
				t.Fatalf("extractObservedIPv6() = %s, %v", got, err)
			}
		})
	}
}

func TestExtractObservedIPv6RejectsIPv4(t *testing.T) {
	if _, err := extractObservedIPv6([]byte("192.0.2.10")); err == nil {
		t.Fatal("extractObservedIPv6() unexpectedly accepted IPv4")
	}
}
