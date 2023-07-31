package main

import (
	"fmt"
	"github.com/alecthomas/kingpin/v2"
	"github.com/docker/go-units"
	golog "github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/ovh/go-ovh/ovh"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/prometheus/exporter-toolkit/web/kingpinflag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type Metrics struct {
	cloudProjectUsageStorageIncomingBandwidth         *prometheus.GaugeVec
	cloudProjectUsageStorageIncomingInternalBandwidth *prometheus.GaugeVec
	cloudProjectUsageStorageOutgoingBandwidth         *prometheus.GaugeVec
	cloudProjectUsageStorageOutgoingInternalBandwidth *prometheus.GaugeVec
	cloudProjectUsageStorageStored                    *prometheus.GaugeVec
}

type CloudProject struct {
	ProjectId   string `json:"project_id"`
	ProjectName string `json:"projectName"`
}

type Quantity struct {
	Unit  string  `json:"unit"`
	Value float64 `json:"value"`
}

type Bandwidth struct {
	Quantity   Quantity `json:"quantity"`
	TotalPrice float64  `json:"totalPrice"`
}

type Stored struct {
	Quantity   Quantity `json:"quantity"`
	TotalPrice float64  `json:"totalPrice"`
}

type Storage struct {
	BucketName                string    `json:"bucketName"`
	IncomingBandwidth         Bandwidth `json:"incomingBandwidth"`
	IncomingInternalBandwidth Bandwidth `json:"incomingInternalBandwidth"`
	OutgoingBandwidth         Bandwidth `json:"outgoingBandwidth"`
	OutgoingInternalBandwidth Bandwidth `json:"outgoingInternalBandwidth"`
	Stored                    Stored    `json:"stored"`
	Type                      string    `json:"type"`
	Region                    string    `json:"region"`
}

type HourlyUsage struct {
	Storage []Storage `json:"storage"`
}

type CloudProjectUsage struct {
	HourlyUsage HourlyUsage `json:"hourlyUsage"`
}

func GetPublicCloudProjects(client *ovh.Client) ([]CloudProject, error) {
	var projectIds []string
	var cloudProjects []CloudProject
	err := client.Get("/cloud/project", &projectIds)

	if err != nil {
		return nil, err
	}

	for _, id := range projectIds {
		var cloudProject CloudProject
		err := client.Get(fmt.Sprintf("/cloud/project/%s", id), &cloudProject)

		if err != nil {
			return nil, err
		}

		cloudProjects = append(cloudProjects, cloudProject)
	}

	return cloudProjects, nil
}

func NewMetrics(reg prometheus.Registerer, namespace string) *Metrics {
	m := &Metrics{
		cloudProjectUsageStorageIncomingBandwidth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "cloud_project_usage",
				Name:      "storage_incoming_bw",
				Help:      "Incoming bandwidth for OVH Cloud Project storage",
			}, []string{"project_name", "bucket_name", "region", "type"},
		),
		cloudProjectUsageStorageIncomingInternalBandwidth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "cloud_project_usage",
				Name:      "storage_incoming_internal_bw",
				Help:      "Incoming internal bandwidth for OVH Cloud Project storage",
			}, []string{"project_name", "bucket_name", "region", "type"},
		),
		cloudProjectUsageStorageOutgoingBandwidth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "cloud_project_usage",
				Name:      "storage_outgoing_bw",
				Help:      "Outgoing bandwidth for OVH Cloud Project storage",
			}, []string{"project_name", "bucket_name", "region", "type"},
		),
		cloudProjectUsageStorageOutgoingInternalBandwidth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "cloud_project_usage",
				Name:      "storage_outgoing_internal_bw",
				Help:      "Outgoing internal bandwidth for OVH Cloud Project storage",
			}, []string{"project_name", "bucket_name", "region", "type"},
		),
		cloudProjectUsageStorageStored: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "cloud_project_usage",
				Name:      "storage_stored",
				Help:      "Stored Data for OVH Cloud Project storage",
			}, []string{"project_name", "bucket_name", "region", "type"},
		),
	}
	reg.MustRegister(m.cloudProjectUsageStorageIncomingBandwidth)
	reg.MustRegister(m.cloudProjectUsageStorageIncomingInternalBandwidth)
	reg.MustRegister(m.cloudProjectUsageStorageOutgoingBandwidth)
	reg.MustRegister(m.cloudProjectUsageStorageOutgoingInternalBandwidth)
	reg.MustRegister(m.cloudProjectUsageStorageStored)
	return m
}

func RealQuantity(quantity *Quantity) int64 {
	realQuantity, err := units.FromHumanSize(
		fmt.Sprintf(
			"%f %s", quantity.Value,
			strings.TrimSuffix(quantity.Unit, "h"),
		),
	)
	if err != nil {
		logger.Log("error", err)
	}
	return realQuantity
}

func RecordCloudProjectMetrics(client *ovh.Client, metrics *Metrics, cloudProjects []CloudProject) {
	for _, cloudProject := range cloudProjects {
		var cloudProjectUsage CloudProjectUsage
		err := client.Get(fmt.Sprintf("/cloud/project/%s/usage/current", cloudProject.ProjectId), &cloudProjectUsage)
		if err != nil {
			logger.Log("error", err)
			continue
		}
		for _, storage := range cloudProjectUsage.HourlyUsage.Storage {
			if len(storage.BucketName) == 0 {
				continue
			}
			labels := prometheus.Labels{
				"project_name": cloudProject.ProjectName,
				"bucket_name":  storage.BucketName,
				"region":       storage.Region,
				"type":         storage.Type,
			}

			metrics.cloudProjectUsageStorageIncomingBandwidth.
				With(labels).
				Set(float64(RealQuantity(&storage.IncomingBandwidth.Quantity)))
			metrics.cloudProjectUsageStorageIncomingInternalBandwidth.
				With(labels).
				Set(float64(RealQuantity(&storage.IncomingInternalBandwidth.Quantity)))
			metrics.cloudProjectUsageStorageOutgoingBandwidth.
				With(labels).
				Set(float64(RealQuantity(&storage.OutgoingBandwidth.Quantity)))
			metrics.cloudProjectUsageStorageOutgoingInternalBandwidth.
				With(labels).
				Set(float64(RealQuantity(&storage.OutgoingInternalBandwidth.Quantity)))
			metrics.cloudProjectUsageStorageStored.
				With(labels).
				Set(float64(RealQuantity(&storage.Stored.Quantity)))
		}

	}
}

func RecordMetrics(client *ovh.Client, metrics *Metrics, cloudProjects []CloudProject) {
	go func() {
		for {
			RecordCloudProjectMetrics(client, metrics, cloudProjects)
			time.Sleep(1 * time.Minute)
		}
	}()
}

var (
	webConfig      = kingpinflag.AddFlags(kingpin.CommandLine, ":9162")
	apiEndpoint    = kingpin.Flag("api-endpoint", "OVH API endpoint. Can be either a URL or one of these aliases: ovh-eu, ovh-ca, ovh-us, kimsufi-eu, kimsufi-ca, soyoustart-eu, soyoustart-ca").Envar("OVH_EXPORTER_API_ENDPOINT").Required().String()
	apiAppKey      = kingpin.Flag("api-app-key", "OVH API app key").Envar("OVH_EXPORTER_API_APP_KEY").Required().String()
	apiAppSecret   = kingpin.Flag("api-app-secret", "OVH API app secret").Envar("OVH_EXPORTER_API_APP_SECRET").Required().String()
	apiConsumerKey = kingpin.Flag("api-consumer-key", "OVH API consumer key").Envar("OVH_EXPORTER_API_CONSUMER_KEY").Required().String()
	metricsPath    = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").Envar("OVH_EXPORTER_WEB_TELEMETRY_PATH").String()
	metricPrefix   = kingpin.Flag("metric-prefix", "A metric prefix can be used to have non-default (not \"ovh\") prefixes for each of the metrics").Default(namespace).Envar("OVH_EXPORTER_METRIC_PREFIX").String()

	logger = golog.NewNopLogger()
)

// Metric name parts.
const (
	// Namespace for all metrics.
	namespace = "ovh"
	// The name of the exporter.
	exporterName = "ovh_exporter"
)

func main() {
	kingpin.Version(version.Print(exporterName))
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger = promlog.New(promlogConfig)

	client, _ := ovh.NewClient(
		*apiEndpoint,
		*apiAppKey,
		*apiAppSecret,
		*apiConsumerKey,
	)

	cloudProjects, err := GetPublicCloudProjects(client)
	if err != nil {
		log.Fatal(err)
	}
	reg := prometheus.NewRegistry()

	m := NewMetrics(reg, *metricPrefix)

	RecordMetrics(client, m, cloudProjects)

	http.Handle(*metricsPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))

	if *metricsPath != "/" && *metricsPath != "" {
		landingConfig := web.LandingConfig{
			Name:        "OVH Exporter",
			Description: "Prometheus OVH API Exporter",
			Version:     version.Info(),
			Links: []web.LandingLinks{
				{
					Address: *metricsPath,
					Text:    "Metrics",
				},
			},
		}
		landingPage, err := web.NewLandingPage(landingConfig)
		if err != nil {
			level.Error(logger).Log("err", err)
			os.Exit(1)
		}
		http.Handle("/", landingPage)
	}
	srv := &http.Server{}
	if err := web.ListenAndServe(srv, webConfig, logger); err != nil {
		level.Error(logger).Log("msg", "Error running HTTP server", "err", err)
		os.Exit(1)
	}
}
