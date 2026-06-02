package main

import (
	"fmt"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/dns"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func buildDNS(ctx *pulumi.Context, c nuonConfig, clusterName string, vpc vpcLayer, labels pulumi.StringMap) (public, internal *dns.ManagedZone, err error) {
	if c.enableNuonDNS && c.publicDomain != "" {
		public, err = dns.NewManagedZone(ctx, "public", &dns.ManagedZoneArgs{
			Project:      pulumi.String(c.projectID),
			Name:         pulumi.Sprintf("%s-public", clusterName),
			DnsName:      pulumi.Sprintf("%s.", c.publicDomain),
			Description:  pulumi.Sprintf("Public DNS zone for %s", clusterName),
			Labels:       labels,
			ForceDestroy: pulumi.Bool(true),
		})
		if err != nil {
			return nil, nil, fmt.Errorf("create public dns zone: %w", err)
		}
	}

	if c.internalDomain != "" {
		internal, err = dns.NewManagedZone(ctx, "internal", &dns.ManagedZoneArgs{
			Project:      pulumi.String(c.projectID),
			Name:         pulumi.Sprintf("%s-internal", clusterName),
			DnsName:      pulumi.Sprintf("%s.", c.internalDomain),
			Visibility:   pulumi.String("private"),
			ForceDestroy: pulumi.Bool(true),
			Labels:       labels,
			PrivateVisibilityConfig: &dns.ManagedZonePrivateVisibilityConfigArgs{
				Networks: dns.ManagedZonePrivateVisibilityConfigNetworkArray{
					&dns.ManagedZonePrivateVisibilityConfigNetworkArgs{
						NetworkUrl: vpc.networkSelfLink,
					},
				},
			},
		})
		if err != nil {
			return nil, nil, fmt.Errorf("create internal dns zone: %w", err)
		}
	}

	return public, internal, nil
}
