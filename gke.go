package main

import (
	"fmt"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type gkeCluster struct {
	cluster  *container.Cluster
	nodePool *container.NodePool
}

func buildGKE(ctx *pulumi.Context, c nuonConfig, clusterName string, vpc vpcLayer, labels pulumi.StringMap) (gkeCluster, error) {
	ipAlloc := &container.ClusterIpAllocationPolicyArgs{}
	if c.createVPC() {
		ipAlloc.ClusterSecondaryRangeName = pulumi.String("pods")
		ipAlloc.ServicesSecondaryRangeName = pulumi.String("services")
	}

	clusterArgs := &container.ClusterArgs{
		Project:               pulumi.String(c.projectID),
		Name:                  pulumi.String(clusterName),
		Location:              pulumi.String(c.region),
		RemoveDefaultNodePool: pulumi.Bool(true),
		InitialNodeCount:      pulumi.Int(1),
		DeletionProtection:    pulumi.Bool(c.deletionProtection),
		Network:               vpc.network,
		Subnetwork:            vpc.subnetwork,
		IpAllocationPolicy:    ipAlloc,
		ReleaseChannel: &container.ClusterReleaseChannelArgs{
			Channel: pulumi.String(c.releaseChannel),
		},
		PrivateClusterConfig: &container.ClusterPrivateClusterConfigArgs{
			EnablePrivateNodes:    pulumi.Bool(true),
			EnablePrivateEndpoint: pulumi.Bool(!c.clusterEndpointPublicAccess),
			MasterIpv4CidrBlock:   pulumi.String("172.16.0.0/28"),
		},
		WorkloadIdentityConfig: &container.ClusterWorkloadIdentityConfigArgs{
			WorkloadPool: pulumi.Sprintf("%s.svc.id.goog", c.projectID),
		},
		BinaryAuthorization: &container.ClusterBinaryAuthorizationArgs{
			EvaluationMode: pulumi.String("DISABLED"),
		},
		GatewayApiConfig: &container.ClusterGatewayApiConfigArgs{
			Channel: pulumi.String("CHANNEL_STANDARD"),
		},
		ResourceLabels: labels,
	}

	if len(c.masterAuthorizedNetworks) > 0 {
		cidrs := container.ClusterMasterAuthorizedNetworksConfigCidrBlockArray{}
		for _, n := range c.masterAuthorizedNetworks {
			cidrs = append(cidrs, &container.ClusterMasterAuthorizedNetworksConfigCidrBlockArgs{
				CidrBlock:   pulumi.String(n.CidrBlock),
				DisplayName: pulumi.String(n.DisplayName),
			})
		}
		clusterArgs.MasterAuthorizedNetworksConfig = &container.ClusterMasterAuthorizedNetworksConfigArgs{
			CidrBlocks: cidrs,
		}
	}

	cluster, err := container.NewCluster(ctx, "cluster", clusterArgs)
	if err != nil {
		return gkeCluster{}, fmt.Errorf("create gke cluster: %w", err)
	}

	nodePool, err := container.NewNodePool(ctx, "node-pool", &container.NodePoolArgs{
		Project:  pulumi.String(c.projectID),
		Name:     pulumi.String("main"),
		Cluster:  cluster.Name,
		Location: pulumi.String(c.region),
		Autoscaling: &container.NodePoolAutoscalingArgs{
			MinNodeCount:   pulumi.Int(c.nodeMinCount),
			MaxNodeCount:   pulumi.Int(c.nodeMaxCount),
			LocationPolicy: pulumi.String("BALANCED"),
		},
		Management: &container.NodePoolManagementArgs{
			AutoRepair:  pulumi.Bool(true),
			AutoUpgrade: pulumi.Bool(true),
		},
		NodeConfig: &container.NodePoolNodeConfigArgs{
			MachineType: pulumi.String(c.nodeMachineType),
			WorkloadMetadataConfig: &container.NodePoolNodeConfigWorkloadMetadataConfigArgs{
				Mode: pulumi.String("GKE_METADATA"),
			},
			ShieldedInstanceConfig: &container.NodePoolNodeConfigShieldedInstanceConfigArgs{
				EnableSecureBoot:          pulumi.Bool(false),
				EnableIntegrityMonitoring: pulumi.Bool(true),
			},
			OauthScopes: pulumi.StringArray{
				pulumi.String("https://www.googleapis.com/auth/cloud-platform"),
			},
			Labels: labels,
		},
	})
	if err != nil {
		return gkeCluster{}, fmt.Errorf("create node pool: %w", err)
	}

	return gkeCluster{cluster: cluster, nodePool: nodePool}, nil
}
