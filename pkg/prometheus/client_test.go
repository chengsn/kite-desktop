package prometheus

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type fakePromAPI struct {
	queryRangeResponses map[string]model.Value
	queryRangeCalls     []v1.Range
	queryRangeQueries   []string
}

func (f *fakePromAPI) Config(context.Context) (v1.ConfigResult, error) {
	return v1.ConfigResult{}, nil
}

func (f *fakePromAPI) Query(context.Context, string, time.Time, ...v1.Option) (model.Value, v1.Warnings, error) {
	return &model.String{}, nil, nil
}

func (f *fakePromAPI) QueryRange(_ context.Context, query string, r v1.Range, _ ...v1.Option) (model.Value, v1.Warnings, error) {
	f.queryRangeCalls = append(f.queryRangeCalls, r)
	f.queryRangeQueries = append(f.queryRangeQueries, query)
	if value, ok := f.queryRangeResponses[query]; ok {
		return value, nil, nil
	}
	return model.Matrix{}, nil, nil
}

func matrixValue(ts time.Time, value float64) model.Value {
	return model.Matrix{
		&model.SampleStream{
			Values: []model.SamplePair{
				{
					Timestamp: model.TimeFromUnixNano(ts.UnixNano()),
					Value:     model.SampleValue(value),
				},
			},
		},
	}
}

func TestQueryRange(t *testing.T) {
	now := time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)
	api := &fakePromAPI{
		queryRangeResponses: map[string]model.Value{
			"query": matrixValue(now, 1.5),
			"bad":   &model.String{Value: "unexpected"},
		},
	}
	client := &Client{client: api}

	got, err := client.queryRange(context.Background(), "query", now.Add(-time.Minute), now, 30*time.Second)
	if err != nil {
		t.Fatalf("queryRange() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("queryRange() len = %d, want 1", len(got))
	}
	if !got[0].Timestamp.Equal(now) {
		t.Fatalf("queryRange() timestamp = %v, want %v", got[0].Timestamp, now)
	}
	if got[0].Value != 1.5 {
		t.Fatalf("queryRange() value = %v, want 1.5", got[0].Value)
	}

	if _, err := client.queryRange(context.Background(), "bad", now.Add(-time.Minute), now, 30*time.Second); err == nil || !strings.Contains(err.Error(), "unexpected result type") {
		t.Fatalf("queryRange() error = %v, want unexpected result type", err)
	}
}

func TestFillMissingDataPoints(t *testing.T) {
	if got := FillMissingDataPoints(time.Minute, time.Second, nil); got != nil {
		t.Fatalf("FillMissingDataPoints() nil = %#v, want nil", got)
	}

	now := time.Now()
	step := time.Minute
	timeRange := 10 * time.Minute
	existing := []UsageDataPoint{
		{
			Timestamp: now.Add(-timeRange + 3*step),
			Value:     42,
		},
	}

	got := FillMissingDataPoints(timeRange, step, existing)
	if len(got) != 3 {
		t.Fatalf("FillMissingDataPoints() len = %d, want 3", len(got))
	}
	if got[0].Value != 0 || got[1].Value != 0 || got[2].Value != 42 {
		t.Fatalf("FillMissingDataPoints() values = %#v, want two zeros then original point", got)
	}
	if !got[2].Timestamp.Equal(existing[0].Timestamp) {
		t.Fatalf("FillMissingDataPoints() last timestamp = %v, want %v", got[2].Timestamp, existing[0].Timestamp)
	}
}

func TestGetResourceUsageHistory(t *testing.T) {
	sampleTime := time.Date(2026, 3, 27, 11, 30, 0, 0, time.UTC)
	api := &fakePromAPI{queryRangeResponses: map[string]model.Value{}}
	client := &Client{client: api}

	cpuQuery := `sum(rate(node_cpu_seconds_total{mode!="idle",node="node-a"}[5m])) / sum(rate(node_cpu_seconds_total{node="node-a"}[5m])) * 100`
	memoryQuery := `(1 - sum(node_memory_MemAvailable_bytes{node="node-a"}) / sum(node_memory_MemTotal_bytes{node="node-a"})) * 100`
	networkInQuery := `sum(rate(node_network_receive_bytes_total{device!="lo",node="node-a"}[1m]))`
	networkOutQuery := `sum(rate(node_network_transmit_bytes_total{device!="lo",node="node-a"}[1m]))`
	for _, query := range []string{cpuQuery, memoryQuery, networkInQuery, networkOutQuery} {
		api.queryRangeResponses[query] = matrixValue(sampleTime, 1)
	}

	got, err := client.GetResourceUsageHistory(context.Background(), "node-a", "1h", "node")
	if err != nil {
		t.Fatalf("GetResourceUsageHistory() error = %v", err)
	}
	if len(got.CPU) != 1 || len(got.Memory) != 1 || len(got.NetworkIn) != 1 || len(got.NetworkOut) != 1 {
		t.Fatalf("GetResourceUsageHistory() lengths = %#v", got)
	}
	if len(api.queryRangeCalls) != 4 {
		t.Fatalf("GetResourceUsageHistory() query calls = %d, want 4", len(api.queryRangeCalls))
	}
	if api.queryRangeCalls[0].Step != 2*time.Minute {
		t.Fatalf("GetResourceUsageHistory() step = %v, want 2m", api.queryRangeCalls[0].Step)
	}
	for i := 1; i < len(api.queryRangeCalls); i++ {
		if api.queryRangeCalls[i] != api.queryRangeCalls[0] {
			t.Fatalf("GetResourceUsageHistory() range %d = %#v, want %#v", i, api.queryRangeCalls[i], api.queryRangeCalls[0])
		}
	}
}

func TestGetResourceUsageHistoryFallsBackToPodMetrics(t *testing.T) {
	sampleTime := time.Date(2026, 3, 27, 11, 30, 0, 0, time.UTC)
	api := &fakePromAPI{queryRangeResponses: map[string]model.Value{}}
	client := &Client{client: api}

	cpuQuery := `sum(rate(container_cpu_usage_seconds_total{pod!=""}[1m])) / sum(kube_node_status_allocatable{resource="cpu"}) * 100`
	memoryQuery := `sum(container_memory_working_set_bytes{pod!=""}) / sum(kube_node_status_allocatable{resource="memory"}) * 100`
	api.queryRangeResponses[cpuQuery] = matrixValue(sampleTime, 2)
	api.queryRangeResponses[memoryQuery] = matrixValue(sampleTime, 3)

	got, err := client.GetResourceUsageHistory(context.Background(), "", "30m", "node")
	if err != nil {
		t.Fatalf("GetResourceUsageHistory() error = %v", err)
	}
	if len(got.CPU) != 1 || len(got.Memory) != 1 {
		t.Fatalf("GetResourceUsageHistory() cpu/memory lengths = %#v", got)
	}
	if len(got.NetworkIn) != 0 || len(got.NetworkOut) != 0 {
		t.Fatalf("GetResourceUsageHistory() network lengths = %#v, want empty", got)
	}
	if len(got.Warnings) == 0 {
		t.Fatalf("GetResourceUsageHistory() warnings = %#v, want non-empty", got.Warnings)
	}
}

func TestGetResourceUsageHistoryUnsupportedDuration(t *testing.T) {
	client := &Client{client: &fakePromAPI{}}

	if _, err := client.GetResourceUsageHistory(context.Background(), "", "bad", "node"); err == nil || !strings.Contains(err.Error(), "unsupported duration") {
		t.Fatalf("GetResourceUsageHistory() error = %v, want unsupported duration", err)
	}
}

// utilizationQuery mirrors how the client combines a usage series with its
// limit/request denominator.
func utilizationQuery(usage, bound string) string {
	return usage + ` / (` + bound + ` > 0) * 100`
}

// containerMatchers is the matcher list for a named container in a pod prefix.
const containerMatchers = `container!="POD",container!="",container="api",pod=~"web.*",namespace="default"`

// podMatchers is the matcher list used when container labels are unavailable.
const podMatchers = `pod!="",pod=~"web.*",namespace="default"`

func TestGetPodMetrics(t *testing.T) {
	sampleTime := time.Date(2026, 3, 27, 11, 0, 0, 0, time.UTC)
	api := &fakePromAPI{queryRangeResponses: map[string]model.Value{}}
	client := &Client{client: api}

	// The pod declares limits, so CPU and memory report utilization and the
	// network/disk series are unchanged.
	queries := []string{
		utilizationQuery(
			fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s}[1m]))`, containerMatchers),
			fmt.Sprintf(`sum(kube_pod_container_resource_limits{resource="cpu",%s})`, containerMatchers),
		),
		utilizationQuery(
			fmt.Sprintf(`sum(container_memory_working_set_bytes{%s})`, containerMatchers),
			fmt.Sprintf(`sum(kube_pod_container_resource_limits{resource="memory",%s})`, containerMatchers),
		),
		fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[1m]))`, containerMatchers),
		fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[1m]))`, containerMatchers),
		fmt.Sprintf(`sum(rate(container_fs_reads_bytes_total{%s}[1m]))`, containerMatchers),
		fmt.Sprintf(`sum(rate(container_fs_writes_bytes_total{%s}[1m]))`, containerMatchers),
	}
	for _, query := range queries {
		api.queryRangeResponses[query] = matrixValue(sampleTime, 2)
	}

	got, err := client.GetPodMetrics(context.Background(), "default", "web", "api", "30m")
	if err != nil {
		t.Fatalf("GetPodMetrics() error = %v", err)
	}
	if got.Fallback {
		t.Fatalf("GetPodMetrics() fallback = true, want false")
	}
	if got.Source != "prometheus-container" {
		t.Fatalf("GetPodMetrics() source = %q, want prometheus-container", got.Source)
	}
	if got.CPUUnit != UnitPercent || got.MemoryUnit != UnitPercent {
		t.Fatalf("GetPodMetrics() units = %q/%q, want percent/percent", got.CPUUnit, got.MemoryUnit)
	}
	if len(api.queryRangeCalls) != len(queries) {
		t.Fatalf("GetPodMetrics() query calls = %d, want %d", len(api.queryRangeCalls), len(queries))
	}
	if api.queryRangeCalls[0].Step != 15*time.Second {
		t.Fatalf("GetPodMetrics() step = %v, want 15s", api.queryRangeCalls[0].Step)
	}
	for i, query := range queries {
		if api.queryRangeQueries[i] != query {
			t.Fatalf("GetPodMetrics() query %d = %q, want %q", i, api.queryRangeQueries[i], query)
		}
	}
}

func TestGetPodMetricsPrefersRequestOverAbsolute(t *testing.T) {
	sampleTime := time.Date(2026, 3, 27, 11, 0, 0, 0, time.UTC)
	api := &fakePromAPI{queryRangeResponses: map[string]model.Value{}}
	client := &Client{client: api}

	// No limit is declared, so the request becomes the denominator. The
	// absolute series is registered too, to prove it is not preferred.
	cpuUsage := fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s}[1m]))`, containerMatchers)
	memUsage := fmt.Sprintf(`sum(container_memory_working_set_bytes{%s})`, containerMatchers)
	queries := []string{
		utilizationQuery(cpuUsage, fmt.Sprintf(`sum(kube_pod_container_resource_requests{resource="cpu",%s})`, containerMatchers)),
		utilizationQuery(memUsage, fmt.Sprintf(`sum(kube_pod_container_resource_requests{resource="memory",%s})`, containerMatchers)),
		fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[1m]))`, containerMatchers),
		fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[1m]))`, containerMatchers),
		fmt.Sprintf(`sum(rate(container_fs_reads_bytes_total{%s}[1m]))`, containerMatchers),
		fmt.Sprintf(`sum(rate(container_fs_writes_bytes_total{%s}[1m]))`, containerMatchers),
	}
	for _, query := range queries {
		api.queryRangeResponses[query] = matrixValue(sampleTime, 2)
	}
	api.queryRangeResponses[cpuUsage] = matrixValue(sampleTime, 99)
	api.queryRangeResponses[memUsage+` / 1024 / 1024`] = matrixValue(sampleTime, 99)

	got, err := client.GetPodMetrics(context.Background(), "default", "web", "api", "30m")
	if err != nil {
		t.Fatalf("GetPodMetrics() error = %v", err)
	}
	if got.CPUUnit != UnitPercent || got.MemoryUnit != UnitPercent {
		t.Fatalf("GetPodMetrics() units = %q/%q, want percent/percent", got.CPUUnit, got.MemoryUnit)
	}
	if got.CPU[0].Value != 2 {
		t.Fatalf("GetPodMetrics() cpu value = %v, want the request-relative 2", got.CPU[0].Value)
	}
}

func TestGetPodMetricsPrefersContainerAbsoluteOverPodPercent(t *testing.T) {
	sampleTime := time.Date(2026, 3, 27, 11, 0, 0, 0, time.UTC)
	api := &fakePromAPI{queryRangeResponses: map[string]model.Value{}}
	client := &Client{client: api}

	// The selected container has no limit/request, but a sibling container
	// does. The container's own absolute usage must win over the pod-wide
	// percentage, which would otherwise describe the whole pod.
	cpuAbsolute := fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s}[1m]))`, containerMatchers)
	memAbsolute := fmt.Sprintf(`sum(container_memory_working_set_bytes{%s}) / 1024 / 1024`, containerMatchers)
	api.queryRangeResponses[utilizationQuery(
		fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s}[1m]))`, podMatchers),
		fmt.Sprintf(`sum(kube_pod_container_resource_limits{resource="cpu",%s})`, podMatchers),
	)] = matrixValue(sampleTime, 99)
	api.queryRangeResponses[utilizationQuery(
		fmt.Sprintf(`sum(container_memory_working_set_bytes{%s})`, podMatchers),
		fmt.Sprintf(`sum(kube_pod_container_resource_limits{resource="memory",%s})`, podMatchers),
	)] = matrixValue(sampleTime, 99)
	api.queryRangeResponses[cpuAbsolute] = matrixValue(sampleTime, 2)
	api.queryRangeResponses[memAbsolute] = matrixValue(sampleTime, 3)
	for _, query := range []string{
		fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[1m]))`, containerMatchers),
		fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[1m]))`, containerMatchers),
		fmt.Sprintf(`sum(rate(container_fs_reads_bytes_total{%s}[1m]))`, containerMatchers),
		fmt.Sprintf(`sum(rate(container_fs_writes_bytes_total{%s}[1m]))`, containerMatchers),
	} {
		api.queryRangeResponses[query] = matrixValue(sampleTime, 4)
	}

	got, err := client.GetPodMetrics(context.Background(), "default", "web", "api", "30m")
	if err != nil {
		t.Fatalf("GetPodMetrics() error = %v", err)
	}
	if got.CPUUnit != UnitCores || got.MemoryUnit != UnitMB {
		t.Fatalf("GetPodMetrics() units = %q/%q, want cores/mb", got.CPUUnit, got.MemoryUnit)
	}
	if got.Source != "prometheus-container" {
		t.Fatalf("GetPodMetrics() source = %q, want prometheus-container", got.Source)
	}
	if got.CPU[0].Value != 2 || got.Memory[0].Value != 3 {
		t.Fatalf("GetPodMetrics() values = %v/%v, want the container absolute 2/3", got.CPU[0].Value, got.Memory[0].Value)
	}
}

func TestGetPodMetricsFallsBackToAbsoluteWithoutLimits(t *testing.T) {
	sampleTime := time.Date(2026, 3, 27, 11, 0, 0, 0, time.UTC)
	api := &fakePromAPI{queryRangeResponses: map[string]model.Value{}}
	client := &Client{client: api}

	// Nothing in kube-state-metrics: every utilization query comes back empty,
	// so the client reports absolute cores and megabytes.
	queries := []string{
		fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s}[1m]))`, containerMatchers),
		fmt.Sprintf(`sum(container_memory_working_set_bytes{%s}) / 1024 / 1024`, containerMatchers),
		fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[1m]))`, containerMatchers),
		fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[1m]))`, containerMatchers),
		fmt.Sprintf(`sum(rate(container_fs_reads_bytes_total{%s}[1m]))`, containerMatchers),
		fmt.Sprintf(`sum(rate(container_fs_writes_bytes_total{%s}[1m]))`, containerMatchers),
	}
	for _, query := range queries {
		api.queryRangeResponses[query] = matrixValue(sampleTime, 2)
	}

	got, err := client.GetPodMetrics(context.Background(), "default", "web", "api", "30m")
	if err != nil {
		t.Fatalf("GetPodMetrics() error = %v", err)
	}
	if got.CPUUnit != UnitCores || got.MemoryUnit != UnitMB {
		t.Fatalf("GetPodMetrics() units = %q/%q, want cores/mb", got.CPUUnit, got.MemoryUnit)
	}
	if got.CPU[0].Value != 2 || got.Memory[0].Value != 2 {
		t.Fatalf("GetPodMetrics() values = %v/%v, want the absolute 2/2", got.CPU[0].Value, got.Memory[0].Value)
	}
}

func TestGetPodMetricsFallsBackToPodAggregate(t *testing.T) {
	sampleTime := time.Date(2026, 3, 27, 11, 0, 0, 0, time.UTC)
	api := &fakePromAPI{queryRangeResponses: map[string]model.Value{}}
	client := &Client{client: api}

	api.queryRangeResponses[utilizationQuery(
		fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s}[1m]))`, podMatchers),
		fmt.Sprintf(`sum(kube_pod_container_resource_limits{resource="cpu",%s})`, podMatchers),
	)] = matrixValue(sampleTime, 2)
	api.queryRangeResponses[utilizationQuery(
		fmt.Sprintf(`sum(container_memory_working_set_bytes{%s})`, podMatchers),
		fmt.Sprintf(`sum(kube_pod_container_resource_limits{resource="memory",%s})`, podMatchers),
	)] = matrixValue(sampleTime, 3)
	api.queryRangeResponses[fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[1m]))`, podMatchers)] = matrixValue(sampleTime, 4)
	api.queryRangeResponses[fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[1m]))`, podMatchers)] = matrixValue(sampleTime, 4)
	api.queryRangeResponses[fmt.Sprintf(`sum(rate(container_fs_reads_bytes_total{%s}[1m]))`, podMatchers)] = matrixValue(sampleTime, 4)
	api.queryRangeResponses[fmt.Sprintf(`sum(rate(container_fs_writes_bytes_total{%s}[1m]))`, podMatchers)] = matrixValue(sampleTime, 4)

	got, err := client.GetPodMetrics(context.Background(), "default", "web", "api", "30m")
	if err != nil {
		t.Fatalf("GetPodMetrics() error = %v", err)
	}
	if got.Source != "prometheus-pod" {
		t.Fatalf("GetPodMetrics() source = %q, want prometheus-pod", got.Source)
	}
	if got.CPUUnit != UnitPercent || got.MemoryUnit != UnitPercent {
		t.Fatalf("GetPodMetrics() units = %q/%q, want percent/percent", got.CPUUnit, got.MemoryUnit)
	}
	if len(got.Warnings) == 0 {
		t.Fatalf("GetPodMetrics() warnings = %#v, want non-empty", got.Warnings)
	}
}

func TestGetPodMetricsUnsupportedDuration(t *testing.T) {
	client := &Client{client: &fakePromAPI{}}

	if _, err := client.GetPodMetrics(context.Background(), "default", "web", "api", "bad"); err == nil || !strings.Contains(err.Error(), "unsupported duration") {
		t.Fatalf("GetPodMetrics() error = %v, want unsupported duration", err)
	}
}
