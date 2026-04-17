# Prometheus 接入示例

本文档给出 `orm/v2` 输出 `MetricSample` 后，如何在业务层适配为 Prometheus 指标。

本仓库不直接依赖 Prometheus，但建议在接入层封装一个采集器。

## 适用场景

- 你已经在服务内使用 `prometheus.Registerer`
- 你希望把 `Client.Metrics()` 或 `Cluster.Metrics()` 暴露给 `/metrics`

## 参考实现

```go
package metrics

import (
	"sync"

	orm "github.com/gtkit/orm/v2"
	"github.com/prometheus/client_golang/prometheus"
)

type ORMMetricSource interface {
	Metrics() []orm.MetricSample
}

type ORMCollector struct {
	source ORMMetricSource
	mu     sync.Mutex
	descs  map[string]*prometheus.Desc
}

func NewORMCollector(source ORMMetricSource) *ORMCollector {
	return &ORMCollector{
		source: source,
		descs:  make(map[string]*prometheus.Desc),
	}
}

func (c *ORMCollector) Describe(ch chan<- *prometheus.Desc) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, sample := range c.source.Metrics() {
		desc := c.getDesc(sample)
		ch <- desc
	}
}

func (c *ORMCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, sample := range c.source.Metrics() {
		desc := c.getDesc(sample)
		labelValues := make([]string, 0, len(sample.Labels))
		labelNames := orderedLabelNames(sample.Labels)
		for _, name := range labelNames {
			labelValues = append(labelValues, sample.Labels[name])
		}

		metric, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, sample.Value, labelValues...)
		if err != nil {
			continue
		}
		ch <- metric
	}
}

func (c *ORMCollector) getDesc(sample orm.MetricSample) *prometheus.Desc {
	if desc, ok := c.descs[sample.Name]; ok {
		return desc
	}

	labelNames := orderedLabelNames(sample.Labels)
	desc := prometheus.NewDesc(sample.Name, "orm/v2 指标", labelNames, nil)
	c.descs[sample.Name] = desc
	return desc
}

func orderedLabelNames(labels map[string]string) []string {
	names := make([]string, 0, len(labels))
	if _, ok := labels["name"]; ok {
		names = append(names, "name")
	}
	if _, ok := labels["role"]; ok {
		names = append(names, "role")
	}
	for key := range labels {
		if key == "name" || key == "role" {
			continue
		}
		names = append(names, key)
	}
	return names
}
```

## 使用方式

```go
collector := metrics.NewORMCollector(cluster)
prometheus.MustRegister(collector)
```

## 建议

- 单节点可直接注册 `client`
- 集群模式优先注册 `cluster`
- 如果你还要统计死锁重试次数，可把 `WithTxRetryObserver(...)` 接到独立 counter / histogram
