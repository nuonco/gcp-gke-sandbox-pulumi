package main

import (
	"fmt"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/compute"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type vpcLayer struct {
	network         pulumi.StringInput
	networkSelfLink pulumi.StringInput
	subnetwork      pulumi.StringInput
}

func buildVPC(ctx *pulumi.Context, c nuonConfig, clusterName string) (vpcLayer, error) {
	if !c.createVPC() {
		existing, err := compute.LookupNetwork(ctx, &compute.LookupNetworkArgs{
			Name:    c.network,
			Project: &c.projectID,
		})
		if err != nil {
			return vpcLayer{}, fmt.Errorf("look up existing vpc %q: %w", c.network, err)
		}
		return vpcLayer{
			network:         pulumi.String(existing.Id),
			networkSelfLink: pulumi.String(existing.SelfLink),
			subnetwork:      pulumi.String(c.subnetwork),
		}, nil
	}

	net, err := compute.NewNetwork(ctx, "vpc", &compute.NetworkArgs{
		Project:               pulumi.String(c.projectID),
		Name:                  pulumi.Sprintf("%s-vpc", clusterName),
		AutoCreateSubnetworks: pulumi.Bool(false),
	})
	if err != nil {
		return vpcLayer{}, fmt.Errorf("create vpc: %w", err)
	}

	subnet, err := compute.NewSubnetwork(ctx, "gke", &compute.SubnetworkArgs{
		Project:     pulumi.String(c.projectID),
		Name:        pulumi.Sprintf("%s-gke-subnet", clusterName),
		Region:      pulumi.String(c.region),
		Network:     net.ID(),
		IpCidrRange: pulumi.String(c.subnetCIDR),
		SecondaryIpRanges: compute.SubnetworkSecondaryIpRangeArray{
			&compute.SubnetworkSecondaryIpRangeArgs{
				RangeName:   pulumi.String("pods"),
				IpCidrRange: pulumi.String(c.podsCIDRRange),
			},
			&compute.SubnetworkSecondaryIpRangeArgs{
				RangeName:   pulumi.String("services"),
				IpCidrRange: pulumi.String(c.servicesCIDRRange),
			},
		},
		PrivateIpGoogleAccess: pulumi.Bool(true),
	})
	if err != nil {
		return vpcLayer{}, fmt.Errorf("create subnet: %w", err)
	}

	router, err := compute.NewRouter(ctx, "router", &compute.RouterArgs{
		Project: pulumi.String(c.projectID),
		Name:    pulumi.Sprintf("%s-router", clusterName),
		Region:  pulumi.String(c.region),
		Network: net.ID(),
	})
	if err != nil {
		return vpcLayer{}, fmt.Errorf("create router: %w", err)
	}

	_, err = compute.NewRouterNat(ctx, "nat", &compute.RouterNatArgs{
		Project:                       pulumi.String(c.projectID),
		Name:                          pulumi.Sprintf("%s-nat", clusterName),
		Router:                        router.Name,
		Region:                        pulumi.String(c.region),
		NatIpAllocateOption:           pulumi.String("AUTO_ONLY"),
		SourceSubnetworkIpRangesToNat: pulumi.String("ALL_SUBNETWORKS_ALL_IP_RANGES"),
		LogConfig: &compute.RouterNatLogConfigArgs{
			Enable: pulumi.Bool(false),
			Filter: pulumi.String("ERRORS_ONLY"),
		},
	})
	if err != nil {
		return vpcLayer{}, fmt.Errorf("create nat: %w", err)
	}

	return vpcLayer{
		network:         net.ID(),
		networkSelfLink: net.SelfLink,
		subnetwork:      subnet.ID(),
	}, nil
}
