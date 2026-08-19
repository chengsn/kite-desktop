package prometheus

import "net/http"

// BasicAuth holds the credentials used to access a Prometheus endpoint that is
// protected by HTTP basic auth.
type BasicAuth struct {
	Username string
	Password string
}

// Configured reports whether any credential is set.
func (a BasicAuth) Configured() bool {
	return a.Username != "" || a.Password != ""
}

// NewBasicAuthRoundTripper wraps rt so that every request carries the supplied
// basic auth credentials. Requests that already have an Authorization header
// are passed through untouched, mirroring the behavior of the Kubernetes and
// Prometheus clients. When no credentials are configured rt is returned as is.
func NewBasicAuthRoundTripper(auth BasicAuth, rt http.RoundTripper) http.RoundTripper {
	if !auth.Configured() {
		return rt
	}
	if rt == nil {
		rt = http.DefaultTransport
	}

	return &basicAuthRoundTripper{auth: auth, rt: rt}
}

type basicAuthRoundTripper struct {
	auth BasicAuth
	rt   http.RoundTripper
}

func (t *basicAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("Authorization") != "" {
		return t.rt.RoundTrip(req)
	}

	// RoundTrippers must not mutate the request they are given.
	clone := req.Clone(req.Context())
	clone.SetBasicAuth(t.auth.Username, t.auth.Password)

	return t.rt.RoundTrip(clone)
}
