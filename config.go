package main

import (
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

type masterAuthorizedNetwork struct {
	CidrBlock   string `json:"cidr_block"`
	DisplayName string `json:"display_name"`
}

type nuonConfig struct {
	nuonID    string
	region    string
	projectID string

	clusterName                 string
	nodeMachineType             string
	nodeMinCount                int
	nodeMaxCount                int
	releaseChannel              string
	deletionProtection          bool
	clusterEndpointPublicAccess bool

	network           string
	subnetwork        string
	subnetCIDR        string
	podsCIDRRange     string
	servicesCIDRRange string

	enableNuonDNS  bool
	publicDomain   string
	internalDomain string

	enableLinkerd bool

	additionalNamespaces     []string
	masterAuthorizedNetworks []masterAuthorizedNetwork

	labels map[string]string
	tags   map[string]string
}

func loadConfig(cfg *config.Config) nuonConfig {
	c := nuonConfig{
		nuonID:    cfg.Require("nuon_id"),
		region:    cfg.Require("region"),
		projectID: cfg.Require("project_id"),

		clusterName:                 cfg.Get("cluster_name"),
		nodeMachineType:             getDefault(cfg, "node_machine_type", "e2-standard-4"),
		nodeMinCount:                getIntDefault(cfg, "node_min_count", 1),
		nodeMaxCount:                getIntDefault(cfg, "node_max_count", 10),
		releaseChannel:              getDefault(cfg, "release_channel", "REGULAR"),
		deletionProtection:          cfg.GetBool("deletion_protection"),
		clusterEndpointPublicAccess: getBoolDefault(cfg, "cluster_endpoint_public_access", true),

		network:           cfg.Get("network"),
		subnetwork:        cfg.Get("subnetwork"),
		subnetCIDR:        getDefault(cfg, "subnet_cidr", "10.0.0.0/20"),
		podsCIDRRange:     getDefault(cfg, "pods_cidr_range", "10.1.0.0/16"),
		servicesCIDRRange: getDefault(cfg, "services_cidr_range", "10.2.0.0/20"),

		enableLinkerd: getBoolDefault(cfg, "enable_linkerd", true),
	}

	enableDNS := cfg.Get("enable_nuon_dns")
	c.enableNuonDNS = enableDNS == "true" || enableDNS == "1"
	c.publicDomain = normalizeDomain(cfg.Get("public_root_domain"))
	c.internalDomain = normalizeDomain(cfg.Get("internal_root_domain"))

	_ = cfg.TryObject("additional_namespaces", &c.additionalNamespaces)
	_ = cfg.TryObject("master_authorized_networks", &c.masterAuthorizedNetworks)
	_ = cfg.TryObject("labels", &c.labels)
	_ = cfg.TryObject("tags", &c.tags)

	return c
}

func (c nuonConfig) computedClusterName() string {
	name := c.clusterName
	if name == "" {
		name = "n-" + c.nuonID
	}
	if len(name) > 38 {
		name = name[:38]
	}
	return name
}

func (c nuonConfig) createVPC() bool {
	return c.network == ""
}

func (c nuonConfig) namespaces() []string {
	return append([]string{c.nuonID}, c.additionalNamespaces...)
}

func normalizeDomain(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".")
	s = strings.TrimPrefix(s, ".")
	return s
}

func getDefault(cfg *config.Config, key, def string) string {
	if v := cfg.Get(key); v != "" {
		return v
	}
	return def
}

func getIntDefault(cfg *config.Config, key string, def int) int {
	if v, err := cfg.TryInt(key); err == nil {
		return v
	}
	return def
}

func getBoolDefault(cfg *config.Config, key string, def bool) bool {
	if v, err := cfg.TryBool(key); err == nil {
		return v
	}
	return def
}
