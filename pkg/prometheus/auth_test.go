package prometheus

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// basicAuthEncoding mirrors what http.Request.SetBasicAuth produces.
func basicAuthEncoding(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

type recordingRoundTripper struct {
	gotAuthorization string
	calls            int
}

func (r *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.calls++
	r.gotAuthorization = req.Header.Get("Authorization")
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     make(http.Header),
	}, nil
}

func TestBasicAuthConfigured(t *testing.T) {
	tests := []struct {
		name string
		auth BasicAuth
		want bool
	}{
		{name: "empty", auth: BasicAuth{}, want: false},
		{name: "username only", auth: BasicAuth{Username: "admin"}, want: true},
		{name: "password only", auth: BasicAuth{Password: "secret"}, want: true},
		{name: "both", auth: BasicAuth{Username: "admin", Password: "secret"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.auth.Configured(); got != tt.want {
				t.Fatalf("Configured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewBasicAuthRoundTripperSetsAuthorizationHeader(t *testing.T) {
	inner := &recordingRoundTripper{}
	rt := NewBasicAuthRoundTripper(BasicAuth{Username: "admin", Password: "s3cret"}, inner)

	req := httptest.NewRequest(http.MethodGet, "http://prometheus.example.com/api/v1/query", nil)
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}

	want := "Basic " + basicAuthEncoding("admin", "s3cret")
	if inner.gotAuthorization != want {
		t.Fatalf("Authorization = %q, want %q", inner.gotAuthorization, want)
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatalf("original request was mutated: Authorization = %q", req.Header.Get("Authorization"))
	}
}

func TestNewBasicAuthRoundTripperKeepsExistingAuthorization(t *testing.T) {
	inner := &recordingRoundTripper{}
	rt := NewBasicAuthRoundTripper(BasicAuth{Username: "admin", Password: "s3cret"}, inner)

	req := httptest.NewRequest(http.MethodGet, "http://prometheus.example.com/api/v1/query", nil)
	req.Header.Set("Authorization", "Bearer kubeconfig-token")
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}

	if inner.gotAuthorization != "Bearer kubeconfig-token" {
		t.Fatalf("Authorization = %q, want the existing header to win", inner.gotAuthorization)
	}
}

func TestNewBasicAuthRoundTripperWithoutCredentialsPassesThrough(t *testing.T) {
	inner := &recordingRoundTripper{}
	rt := NewBasicAuthRoundTripper(BasicAuth{}, inner)

	if rt != http.RoundTripper(inner) {
		t.Fatalf("expected the inner RoundTripper to be returned unchanged")
	}

	req := httptest.NewRequest(http.MethodGet, "http://prometheus.example.com/api/v1/query", nil)
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if inner.gotAuthorization != "" {
		t.Fatalf("Authorization = %q, want empty", inner.gotAuthorization)
	}
}

func TestNewBasicAuthRoundTripperDefaultsTransport(t *testing.T) {
	rt := NewBasicAuthRoundTripper(BasicAuth{Username: "admin", Password: "s3cret"}, nil)

	authRT, ok := rt.(*basicAuthRoundTripper)
	if !ok {
		t.Fatalf("RoundTripper type = %T, want *basicAuthRoundTripper", rt)
	}
	if authRT.rt != http.RoundTripper(http.DefaultTransport) {
		t.Fatalf("inner transport = %T, want http.DefaultTransport", authRT.rt)
	}
}

func TestNewClientWithRoundTripperSendsBasicAuth(t *testing.T) {
	var gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"yaml":""}}`))
	}))
	defer server.Close()

	rt := NewBasicAuthRoundTripper(BasicAuth{Username: "admin", Password: "s3cret"}, http.DefaultTransport)
	client, err := NewClientWithRoundTripper(server.URL, rt)
	if err != nil {
		t.Fatalf("NewClientWithRoundTripper() error = %v", err)
	}

	if err := client.HealthCheck(t.Context()); err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}

	want := "Basic " + basicAuthEncoding("admin", "s3cret")
	if gotAuthorization != want {
		t.Fatalf("Authorization = %q, want %q", gotAuthorization, want)
	}
}
