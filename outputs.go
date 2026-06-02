package main

import (
	"strings"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/artifactregistry"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/dns"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func exportOutputs(
	ctx *pulumi.Context,
	c nuonConfig,
	clusterName string,
	repo *artifactregistry.Repository,
	vpc vpcLayer,
	gke gkeCluster,
	publicZone, internalZone *dns.ManagedZone,
	namespaces []*corev1.Namespace,
) {
	ctx.Export("account", pulumi.Map{
		"project_id": pulumi.String(c.projectID),
		"region":     pulumi.String(c.region),
	})

	ctx.Export("cluster", pulumi.Map{
		"name":                       gke.cluster.Name,
		"endpoint":                   pulumi.Sprintf("https://%s", gke.cluster.Endpoint),
		"certificate_authority_data": gke.cluster.MasterAuth.ClusterCaCertificate(),
		"location":                   gke.cluster.Location,
		"self_link":                  gke.cluster.SelfLink,
	})

	ctx.Export("gar", pulumi.Map{
		"repository_id":  repo.RepositoryId,
		"repository_url": pulumi.Sprintf("%s-docker.pkg.dev/%s/%s", c.region, c.projectID, clusterName),
		"registry_url":   pulumi.Sprintf("%s-docker.pkg.dev", c.region),
	})

	ctx.Export("vpc", pulumi.Map{
		"network":           vpc.network,
		"network_self_link": vpc.networkSelfLink,
		"subnetwork":        vpc.subnetwork,
	})

	ctx.Export("nuon_dns", buildDNSOutput(c, publicZone, internalZone))

	var nsNames pulumi.StringArray
	for _, ns := range namespaces {
		nsNames = append(nsNames, ns.Metadata.Name().Elem())
	}
	ctx.Export("namespaces", nsNames)

	ctx.Export("availability_zones", gke.nodePool.NodeLocations.ApplyT(func(zones []string) string {
		return strings.Join(zones, ",")
	}).(pulumi.StringOutput))

	if c.enableLinkerd {
		ctx.Export("linkerd", pulumi.Map{
			"all_egress_traffic": pulumi.String(linkerdEgressNetworkName),
		})
	} else {
		ctx.Export("linkerd", pulumi.Map(nil))
	}
}

func buildDNSOutput(c nuonConfig, public, internal *dns.ManagedZone) pulumi.Map {
	emptyDomain := pulumi.Map{
		"zone_id":     pulumi.String(""),
		"name":        pulumi.String(""),
		"nameservers": pulumi.StringArray{pulumi.String("")},
	}

	publicDomain := emptyDomain
	if c.enableNuonDNS && c.publicDomain != "" && public != nil {
		publicDomain = pulumi.Map{
			"zone_id":     public.ManagedZoneId,
			"name":        public.DnsName.ApplyT(trimTrailingDot).(pulumi.StringOutput),
			"nameservers": public.NameServers,
		}
	}

	internalDomain := emptyDomain
	if c.internalDomain != "" && internal != nil {
		internalDomain = pulumi.Map{
			"zone_id":     internal.ManagedZoneId,
			"name":        internal.DnsName.ApplyT(trimTrailingDot).(pulumi.StringOutput),
			"nameservers": internal.NameServers,
		}
	}

	return pulumi.Map{
		"enabled":         pulumi.Bool(c.enableNuonDNS),
		"public_domain":   publicDomain,
		"internal_domain": internalDomain,
	}
}

func trimTrailingDot(s string) string {
	return strings.TrimSuffix(s, ".")
}
