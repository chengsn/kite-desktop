package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"k8s.io/klog/v2"
)

type Client struct {
	client promAPI
}

type promAPI interface {
	Config(ctx context.Context) (v1.ConfigResult, error)
	Query(ctx context.Context, query string, ts time.Time, opts ...v1.Option) (model.Value, v1.Warnings, error)
	QueryRange(ctx context.Context, query string, r v1.Range, opts ...v1.Option) (model.Value, v1.Warnings, error)
}

type ResourceMetrics struct {
	CPURequest    float64
	CPUTotal      float64
	MemoryRequest float64
	MemoryTotal   float64
}

// UsageDataPoint represents a single time point in usage metrics
type UsageDataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// ResourceUsageHistory contains historical usage data for a resource
type ResourceUsageHistory struct {
	CPU        []UsageDataPoint `json:"cpu"`
	Memory     []UsageDataPoint `json:"memory"`
	NetworkIn  []UsageDataPoint `json:"networkIn"`
	NetworkOut []UsageDataPoint `json:"networkOut"`
	DiskRead   []UsageDataPoint `json:"diskRead"`
	DiskWrite  []UsageDataPoint `json:"diskWrite"`
	Warnings   []string         `json:"warnings,omitempty"`
}

// PodMetrics contains metrics for a specific pod
type PodMetrics struct {
	CPU        []UsageDataPoint `json:"cpu"`
	Memory     []UsageDataPoint `json:"memory"`
	NetworkIn  []UsageDataPoint `json:"networkIn"`
	NetworkOut []UsageDataPoint `json:"networkOut"`
	DiskRead   []UsageDataPoint `json:"diskRead"`
	DiskWrite  []UsageDataPoint `json:"diskWrite"`
	CPUUnit    MetricUnit       `json:"cpuUnit,omitempty"`
	MemoryUnit MetricUnit       `json:"memoryUnit,omitempty"`
	Fallback   bool             `json:"fallback"`
	Source     string           `json:"source,omitempty"`
	Warnings   []string         `json:"warnings,omitempty"`
}

// MetricUnit describes what the values of a series represent, so the UI can
// pick the matching axis format and title.
type MetricUnit string

const (
	// UnitPercent means the series is a utilization ratio scaled to 0-100.
	UnitPercent MetricUnit = "percent"
	// UnitCores means the series holds absolute CPU cores.
	UnitCores MetricUnit = "cores"
	// UnitMB means the series holds absolute memory in megabytes.
	UnitMB MetricUnit = "mb"
)

type PodCurrentMetrics struct {
	PodName   string  `json:"podName"`
	Namespace string  `json:"namespace"`
	CPU       float64 `json:"cpu"`    // CPU cores
	Memory    float64 `json:"memory"` // Memory in MB
}

func NewClientWithRoundTripper(prometheusURL string, rt http.RoundTripper) (*Client, error) {
	if prometheusURL == "" {
		return nil, fmt.Errorf("prometheus URL cannot be empty")
	}
	client, err := api.NewClient(api.Config{
		Address:      prometheusURL,
		RoundTripper: rt,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating prometheus client: %w", err)
	}

	v1api := v1.NewAPI(client)
	return &Client{
		client: v1api,
	}, nil
}

type seriesCandidate struct {
	query   string
	source  string
	unit    MetricUnit
	warning string
}

// seriesResult is the outcome of the first candidate that returned data. It is
// never nil on a nil error, so callers can read Data/Warnings unconditionally.
type seriesResult struct {
	Data     []UsageDataPoint
	Source   string
	Unit     MetricUnit
	Warnings []string
}

func joinMatchers(matchers []string) string {
	return strings.Join(matchers, ",")
}

func workloadMatchers(namespace, podNamePrefix, container string, withContainerLabel bool) []string {
	matchers := []string{}
	if withContainerLabel {
		matchers = append(matchers, `container!="POD"`, `container!=""`)
		if container != "" {
			matchers = append(matchers, fmt.Sprintf(`container="%s"`, container))
		}
	} else {
		matchers = append(matchers, `pod!=""`)
	}
	if podNamePrefix != "" {
		matchers = append(matchers, fmt.Sprintf(`pod=~"%s.*"`, podNamePrefix))
	}
	if namespace != "" {
		matchers = append(matchers, fmt.Sprintf(`namespace="%s"`, namespace))
	}
	return matchers
}

func nodeScopedMatchers(nodeLabel, instance string) []string {
	if instance == "" {
		return nil
	}
	return []string{fmt.Sprintf(`%s="%s"`, nodeLabel, instance)}
}

func warningForCandidate(primary, candidate string) string {
	if candidate == primary {
		return ""
	}
	switch candidate {
	case "node":
		return "using node-level metrics because container-level metrics are unavailable"
	case "pod":
		return "using pod-level aggregate metrics because container labels are unavailable"
	default:
		return ""
	}
}

func (c *Client) queryRangeFirstAvailable(ctx context.Context, start, end time.Time, step time.Duration, candidates ...seriesCandidate) (*seriesResult, error) {
	var errs []string
	for _, candidate := range candidates {
		if candidate.query == "" {
			continue
		}
		data, err := c.queryRange(ctx, candidate.query, start, end, step)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", candidate.source, err))
			continue
		}
		if len(data) == 0 {
			continue
		}
		var warnings []string
		if candidate.warning != "" {
			warnings = append(warnings, candidate.warning)
		}
		return &seriesResult{
			Data:     data,
			Source:   candidate.source,
			Unit:     candidate.unit,
			Warnings: warnings,
		}, nil
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	// No candidate produced data: report an empty result rather than nil so
	// callers never have to guard against a missing series.
	return &seriesResult{}, nil
}

// GetResourceUsageHistory fetches historical usage data for CPU and Memory
func (c *Client) GetResourceUsageHistory(ctx context.Context, instance string, duration string, nodeLabel string) (*ResourceUsageHistory, error) {
	var step time.Duration
	var timeRange time.Duration

	switch duration {
	case "30m":
		timeRange = 30 * time.Minute
		step = 1 * time.Minute
	case "1h":
		timeRange = 1 * time.Hour
		step = 2 * time.Minute
	case "24h":
		timeRange = 24 * time.Hour
		step = 30 * time.Minute
	default:
		return nil, fmt.Errorf("unsupported duration: %s", duration)
	}

	now := time.Now()
	start := now.Add(-timeRange)

	var warnings []string

	nodeMatchers := nodeScopedMatchers(nodeLabel, instance)
	cpuAllocatableMatchers := append([]string{`resource="cpu"`}, nodeMatchers...)
	memoryAllocatableMatchers := append([]string{`resource="memory"`}, nodeMatchers...)

	cpuSeries, err := c.queryRangeFirstAvailable(ctx, start, now, step,
		seriesCandidate{
			query:   fmt.Sprintf(`sum(rate(node_cpu_seconds_total{%s}[5m])) / sum(rate(node_cpu_seconds_total{%s}[5m])) * 100`, joinMatchers(append([]string{`mode!="idle"`}, nodeMatchers...)), joinMatchers(nodeMatchers)),
			source:  "node",
			unit:    UnitPercent,
			warning: warningForCandidate("node", "node"),
		},
		seriesCandidate{
			query:   fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s}[1m])) / sum(kube_node_status_allocatable{%s}) * 100`, joinMatchers(append(workloadMatchers("", "", "", true), nodeMatchers...)), joinMatchers(cpuAllocatableMatchers)),
			source:  "container",
			unit:    UnitPercent,
			warning: warningForCandidate("node", "container"),
		},
		seriesCandidate{
			query:   fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s}[1m])) / sum(kube_node_status_allocatable{%s}) * 100`, joinMatchers(append(workloadMatchers("", "", "", false), nodeMatchers...)), joinMatchers(cpuAllocatableMatchers)),
			source:  "pod",
			unit:    UnitPercent,
			warning: warningForCandidate("node", "pod"),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("error querying CPU usage: %w", err)
	}
	warnings = append(warnings, cpuSeries.Warnings...)

	memorySeries, err := c.queryRangeFirstAvailable(ctx, start, now, step,
		seriesCandidate{
			query:   fmt.Sprintf(`(1 - sum(node_memory_MemAvailable_bytes{%s}) / sum(node_memory_MemTotal_bytes{%s})) * 100`, joinMatchers(nodeMatchers), joinMatchers(nodeMatchers)),
			source:  "node",
			unit:    UnitPercent,
			warning: warningForCandidate("node", "node"),
		},
		seriesCandidate{
			query:   fmt.Sprintf(`sum(container_memory_working_set_bytes{%s}) / sum(kube_node_status_allocatable{%s}) * 100`, joinMatchers(append(workloadMatchers("", "", "", true), nodeMatchers...)), joinMatchers(memoryAllocatableMatchers)),
			source:  "container",
			unit:    UnitPercent,
			warning: warningForCandidate("node", "container"),
		},
		seriesCandidate{
			query:   fmt.Sprintf(`sum(container_memory_working_set_bytes{%s}) / sum(kube_node_status_allocatable{%s}) * 100`, joinMatchers(append(workloadMatchers("", "", "", false), nodeMatchers...)), joinMatchers(memoryAllocatableMatchers)),
			source:  "pod",
			unit:    UnitPercent,
			warning: warningForCandidate("node", "pod"),
		},
		seriesCandidate{
			query:   fmt.Sprintf(`sum(container_memory_usage_bytes{%s}) / sum(kube_node_status_allocatable{%s}) * 100`, joinMatchers(append(workloadMatchers("", "", "", true), nodeMatchers...)), joinMatchers(memoryAllocatableMatchers)),
			source:  "container",
			unit:    UnitPercent,
			warning: warningForCandidate("node", "container"),
		},
		seriesCandidate{
			query:   fmt.Sprintf(`sum(container_memory_usage_bytes{%s}) / sum(kube_node_status_allocatable{%s}) * 100`, joinMatchers(append(workloadMatchers("", "", "", false), nodeMatchers...)), joinMatchers(memoryAllocatableMatchers)),
			source:  "pod",
			unit:    UnitPercent,
			warning: warningForCandidate("node", "pod"),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("error querying Memory usage: %w", err)
	}
	warnings = append(warnings, memorySeries.Warnings...)

	networkInSeries, err := c.queryRangeFirstAvailable(ctx, start, now, step,
		seriesCandidate{
			query:   fmt.Sprintf(`sum(rate(node_network_receive_bytes_total{%s}[1m]))`, joinMatchers(append([]string{`device!="lo"`}, nodeMatchers...))),
			source:  "node",
			warning: warningForCandidate("node", "node"),
		},
		seriesCandidate{
			query:   fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[1m]))`, joinMatchers(nodeMatchers)),
			source:  "container",
			warning: warningForCandidate("node", "container"),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("error querying Network incoming bytes: %w", err)
	}
	warnings = append(warnings, networkInSeries.Warnings...)

	networkOutSeries, err := c.queryRangeFirstAvailable(ctx, start, now, step,
		seriesCandidate{
			query:   fmt.Sprintf(`sum(rate(node_network_transmit_bytes_total{%s}[1m]))`, joinMatchers(append([]string{`device!="lo"`}, nodeMatchers...))),
			source:  "node",
			warning: warningForCandidate("node", "node"),
		},
		seriesCandidate{
			query:   fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[1m]))`, joinMatchers(nodeMatchers)),
			source:  "container",
			warning: warningForCandidate("node", "container"),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("error querying Network outgoing bytes: %w", err)
	}
	warnings = append(warnings, networkOutSeries.Warnings...)

	if len(cpuSeries.Data) == 0 && len(memorySeries.Data) == 0 {
		return nil, fmt.Errorf("resource usage history is unavailable from Prometheus")
	}

	return &ResourceUsageHistory{
		CPU:        cpuSeries.Data,
		Memory:     memorySeries.Data,
		NetworkIn:  networkInSeries.Data,
		NetworkOut: networkOutSeries.Data,
		Warnings:   warnings,
	}, nil
}

func (c *Client) queryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]UsageDataPoint, error) {
	r := v1.Range{
		Start: start,
		End:   end,
		Step:  step,
	}

	result, warnings, err := c.client.QueryRange(ctx, query, r)
	if err != nil {
		klog.Error("queryRange", "error", err)
		return nil, err
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}

	var dataPoints []UsageDataPoint

	switch result.Type() {
	case model.ValMatrix:
		matrix := result.(model.Matrix)
		if len(matrix) > 0 {
			for _, sample := range matrix[0].Values {
				dataPoints = append(dataPoints, UsageDataPoint{
					Timestamp: sample.Timestamp.Time(),
					Value:     float64(sample.Value),
				})
			}
		}
	default:
		return nil, fmt.Errorf("unexpected result type: %s", result.Type())
	}

	return dataPoints, nil
}

// HealthCheck verifies if Prometheus is accessible
func (c *Client) HealthCheck(ctx context.Context) error {
	_, err := c.client.Config(ctx)
	return err
}

// Query executes an instant query against Prometheus
func (c *Client) Query(ctx context.Context, query string, ts time.Time, opts ...v1.Option) (model.Value, v1.Warnings, error) {
	return c.client.Query(ctx, query, ts, opts...)
}

// QueryRange executes a range query against Prometheus
func (c *Client) QueryRange(ctx context.Context, query string, r v1.Range, opts ...v1.Option) (model.Value, v1.Warnings, error) {
	return c.client.QueryRange(ctx, query, r, opts...)
}

// workloadLimitMatchers builds the kube-state-metrics matchers for a container's
// resource limit/request. It reuses workloadMatchers so the denominator selects
// exactly the same containers as the usage numerator; resource is "cpu" or
// "memory".
func workloadLimitMatchers(namespace, podNamePrefix, container, resource string, withContainerLabel bool) []string {
	return append(
		[]string{fmt.Sprintf(`resource="%s"`, resource)},
		workloadMatchers(namespace, podNamePrefix, container, withContainerLabel)...,
	)
}

// utilizationCandidates builds the CPU/memory utilization queries for one
// workload scope. Utilization is usage divided by the container limit, falling
// back to the request when no limit is set, so the series reads as a 0-100
// percentage. When neither is configured the query returns nothing and the
// caller falls back to the absolute series.
func utilizationCandidates(usageExpr string, namespace, podNamePrefix, container, resource string, withContainerLabel bool) []seriesCandidate {
	source := "prometheus-container"
	warning := ""
	if !withContainerLabel {
		source = "prometheus-pod"
		warning = warningForCandidate("prometheus-container", "pod")
	}

	usage := fmt.Sprintf(usageExpr, joinMatchers(workloadMatchers(namespace, podNamePrefix, container, withContainerLabel)))
	limit := fmt.Sprintf(`sum(kube_pod_container_resource_limits{%s})`, joinMatchers(workloadLimitMatchers(namespace, podNamePrefix, container, resource, withContainerLabel)))
	request := fmt.Sprintf(`sum(kube_pod_container_resource_requests{%s})`, joinMatchers(workloadLimitMatchers(namespace, podNamePrefix, container, resource, withContainerLabel)))

	// Prefer limit, then request. Both operands are summed over the same
	// selector so the vector match is 1:1 and needs no grouping.
	return []seriesCandidate{
		{
			query:   fmt.Sprintf(`%s / (%s > 0) * 100`, usage, limit),
			source:  source,
			unit:    UnitPercent,
			warning: warning,
		},
		{
			query:   fmt.Sprintf(`%s / (%s > 0) * 100`, usage, request),
			source:  source,
			unit:    UnitPercent,
			warning: warning,
		},
	}
}

// getCPUUsage returns CPU utilization as a percentage of the container limit
// (or request), falling back to absolute cores when the selected container
// declares neither.
func (c *Client) getCPUUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) (*seriesResult, error) {
	now := time.Now()
	start := now.Add(-timeRange)

	candidates := utilizationCandidates(
		`sum(rate(container_cpu_usage_seconds_total{%s}[1m]))`,
		namespace, podNamePrefix, container, "cpu", true,
	)
	// Absolute fallback for the selected container. It must come before the
	// pod-level utilization candidates: when the container declares no limit
	// or request, its own absolute usage is still more accurate than a
	// pod-wide aggregate.
	candidates = append(candidates, seriesCandidate{
		query:  fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s}[1m]))`, joinMatchers(workloadMatchers(namespace, podNamePrefix, container, true))),
		source: "prometheus-container",
		unit:   UnitCores,
	})
	candidates = append(candidates, utilizationCandidates(
		`sum(rate(container_cpu_usage_seconds_total{%s}[1m]))`,
		namespace, podNamePrefix, "", "cpu", false,
	)...)
	candidates = append(candidates, seriesCandidate{
		query:   fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s}[1m]))`, joinMatchers(workloadMatchers(namespace, podNamePrefix, "", false))),
		source:  "prometheus-pod",
		unit:    UnitCores,
		warning: warningForCandidate("prometheus-container", "pod"),
	})

	return c.queryRangeFirstAvailable(ctx, start, now, step, candidates...)
}

// getMemoryUsage returns memory utilization as a percentage of the container
// limit (or request), falling back to absolute megabytes when the selected
// container declares neither.
func (c *Client) getMemoryUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) (*seriesResult, error) {
	now := time.Now()
	start := now.Add(-timeRange)

	candidates := utilizationCandidates(
		`sum(container_memory_working_set_bytes{%s})`,
		namespace, podNamePrefix, container, "memory", true,
	)
	candidates = append(candidates, utilizationCandidates(
		`sum(container_memory_usage_bytes{%s})`,
		namespace, podNamePrefix, container, "memory", true,
	)...)
	// Absolute fallback for the selected container, before any pod-level
	// aggregate candidate. A pod-wide percentage is not the selected
	// container's utilization.
	candidates = append(candidates,
		seriesCandidate{
			query:  fmt.Sprintf(`sum(container_memory_working_set_bytes{%s}) / 1024 / 1024`, joinMatchers(workloadMatchers(namespace, podNamePrefix, container, true))),
			source: "prometheus-container",
			unit:   UnitMB,
		},
		seriesCandidate{
			query:  fmt.Sprintf(`sum(container_memory_usage_bytes{%s}) / 1024 / 1024`, joinMatchers(workloadMatchers(namespace, podNamePrefix, container, true))),
			source: "prometheus-container",
			unit:   UnitMB,
		},
	)
	candidates = append(candidates, utilizationCandidates(
		`sum(container_memory_working_set_bytes{%s})`,
		namespace, podNamePrefix, "", "memory", false,
	)...)
	candidates = append(candidates, utilizationCandidates(
		`sum(container_memory_usage_bytes{%s})`,
		namespace, podNamePrefix, "", "memory", false,
	)...)
	candidates = append(candidates,
		seriesCandidate{
			query:   fmt.Sprintf(`sum(container_memory_working_set_bytes{%s}) / 1024 / 1024`, joinMatchers(workloadMatchers(namespace, podNamePrefix, "", false))),
			source:  "prometheus-pod",
			unit:    UnitMB,
			warning: warningForCandidate("prometheus-container", "pod"),
		},
		seriesCandidate{
			query:   fmt.Sprintf(`sum(container_memory_usage_bytes{%s}) / 1024 / 1024`, joinMatchers(workloadMatchers(namespace, podNamePrefix, "", false))),
			source:  "prometheus-pod",
			unit:    UnitMB,
			warning: warningForCandidate("prometheus-container", "pod"),
		},
	)

	return c.queryRangeFirstAvailable(ctx, start, now, step, candidates...)
}

func (c *Client) getNetworkInUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) (*seriesResult, error) {
	now := time.Now()
	start := now.Add(-timeRange)
	return c.queryRangeFirstAvailable(ctx, start, now, step,
		seriesCandidate{
			query:   fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[1m]))`, joinMatchers(workloadMatchers(namespace, podNamePrefix, container, true))),
			source:  "prometheus-container",
			warning: "",
		},
		seriesCandidate{
			query:   fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[1m]))`, joinMatchers(workloadMatchers(namespace, podNamePrefix, "", false))),
			source:  "prometheus-pod",
			warning: warningForCandidate("prometheus-container", "pod"),
		},
	)
}

func (c *Client) getNetworkOutUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) (*seriesResult, error) {
	now := time.Now()
	start := now.Add(-timeRange)
	return c.queryRangeFirstAvailable(ctx, start, now, step,
		seriesCandidate{
			query:   fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[1m]))`, joinMatchers(workloadMatchers(namespace, podNamePrefix, container, true))),
			source:  "prometheus-container",
			warning: "",
		},
		seriesCandidate{
			query:   fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[1m]))`, joinMatchers(workloadMatchers(namespace, podNamePrefix, "", false))),
			source:  "prometheus-pod",
			warning: warningForCandidate("prometheus-container", "pod"),
		},
	)
}

func (c *Client) getDiskReadUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) (*seriesResult, error) {
	now := time.Now()
	start := now.Add(-timeRange)
	return c.queryRangeFirstAvailable(ctx, start, now, step,
		seriesCandidate{
			query:   fmt.Sprintf(`sum(rate(container_fs_reads_bytes_total{%s}[1m]))`, joinMatchers(workloadMatchers(namespace, podNamePrefix, container, true))),
			source:  "prometheus-container",
			warning: "",
		},
		seriesCandidate{
			query:   fmt.Sprintf(`sum(rate(container_fs_reads_bytes_total{%s}[1m]))`, joinMatchers(workloadMatchers(namespace, podNamePrefix, "", false))),
			source:  "prometheus-pod",
			warning: warningForCandidate("prometheus-container", "pod"),
		},
	)
}

func (c *Client) getDiskWriteUsage(ctx context.Context, namespace, podNamePrefix, container string, timeRange, step time.Duration) (*seriesResult, error) {
	now := time.Now()
	start := now.Add(-timeRange)
	return c.queryRangeFirstAvailable(ctx, start, now, step,
		seriesCandidate{
			query:   fmt.Sprintf(`sum(rate(container_fs_writes_bytes_total{%s}[1m]))`, joinMatchers(workloadMatchers(namespace, podNamePrefix, container, true))),
			source:  "prometheus-container",
			warning: "",
		},
		seriesCandidate{
			query:   fmt.Sprintf(`sum(rate(container_fs_writes_bytes_total{%s}[1m]))`, joinMatchers(workloadMatchers(namespace, podNamePrefix, "", false))),
			source:  "prometheus-pod",
			warning: warningForCandidate("prometheus-container", "pod"),
		},
	)
}

func FillMissingDataPoints(timeRange time.Duration, step time.Duration, existing []UsageDataPoint) []UsageDataPoint {
	if len(existing) == 0 {
		return existing
	}

	startTime := time.Now().Add(-timeRange)
	firstTime := existing[0].Timestamp

	if firstTime.Sub(startTime) <= step {
		return existing
	}

	result := []UsageDataPoint{}
	for t := startTime.Add(step); t.Before(firstTime); t = t.Add(step) {
		result = append(result, UsageDataPoint{
			Timestamp: t,
			Value:     0.0,
		})
	}

	return append(result, existing...)
}

// GetPodMetrics fetches metrics for a specific pod
func (c *Client) GetPodMetrics(ctx context.Context, namespace, podName, container string, duration string) (*PodMetrics, error) {
	var step time.Duration
	var timeRange time.Duration

	switch duration {
	case "30m":
		timeRange = 30 * time.Minute
		step = 15 * time.Second
	case "1h":
		timeRange = 1 * time.Hour
		step = 1 * time.Minute
	case "24h":
		timeRange = 24 * time.Hour
		step = 5 * time.Minute
	default:
		return nil, fmt.Errorf("unsupported duration: %s", duration)
	}

	var warnings []string
	cpuSeries, err := c.getCPUUsage(ctx, namespace, podName, container, timeRange, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod CPU usage: %w", err)
	}
	warnings = append(warnings, cpuSeries.Warnings...)

	memorySeries, err := c.getMemoryUsage(ctx, namespace, podName, container, timeRange, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod Memory usage: %w", err)
	}
	warnings = append(warnings, memorySeries.Warnings...)

	networkInSeries, err := c.getNetworkInUsage(ctx, namespace, podName, container, timeRange, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod Network incoming usage: %w", err)
	}
	warnings = append(warnings, networkInSeries.Warnings...)

	networkOutSeries, err := c.getNetworkOutUsage(ctx, namespace, podName, container, timeRange, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod Network outgoing usage: %w", err)
	}
	warnings = append(warnings, networkOutSeries.Warnings...)

	diskReadSeries, err := c.getDiskReadUsage(ctx, namespace, podName, container, timeRange, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod Disk read usage: %w", err)
	}
	warnings = append(warnings, diskReadSeries.Warnings...)

	diskWriteSeries, err := c.getDiskWriteUsage(ctx, namespace, podName, container, timeRange, step)
	if err != nil {
		return nil, fmt.Errorf("error querying pod Disk write usage: %w", err)
	}
	warnings = append(warnings, diskWriteSeries.Warnings...)

	if len(cpuSeries.Data) == 0 && len(memorySeries.Data) == 0 {
		return nil, fmt.Errorf("pod metrics are unavailable from Prometheus")
	}

	source := cpuSeries.Source
	if source == "" {
		source = memorySeries.Source
	}

	return &PodMetrics{
		CPU:        FillMissingDataPoints(timeRange, step, cpuSeries.Data),
		Memory:     FillMissingDataPoints(timeRange, step, memorySeries.Data),
		NetworkIn:  FillMissingDataPoints(timeRange, step, networkInSeries.Data),
		NetworkOut: FillMissingDataPoints(timeRange, step, networkOutSeries.Data),
		DiskRead:   FillMissingDataPoints(timeRange, step, diskReadSeries.Data),
		DiskWrite:  FillMissingDataPoints(timeRange, step, diskWriteSeries.Data),
		CPUUnit:    cpuSeries.Unit,
		MemoryUnit: memorySeries.Unit,
		Fallback:   false,
		Source:     source,
		Warnings:   warnings,
	}, nil
}
