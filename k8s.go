package main

import (
	"fmt"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/organizations"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// newK8sProvider builds a kubernetes provider authenticated to the freshly
// created GKE cluster using its endpoint + CA and a short-lived GCP token.
func newK8sProvider(ctx *pulumi.Context, cluster *container.Cluster) (*kubernetes.Provider, error) {
	clientCfg, err := organizations.GetClientConfig(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("get gcp client config: %w", err)
	}

	kubeconfig := pulumi.All(cluster.Endpoint, cluster.MasterAuth.ClusterCaCertificate()).ApplyT(
		func(args []interface{}) string {
			endpoint := args[0].(string)
			caCert := ""
			if args[1] != nil {
				caCert = *args[1].(*string)
			}
			return fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: gke
  cluster:
    server: https://%s
    certificate-authority-data: %s
contexts:
- name: gke
  context:
    cluster: gke
    user: gke
current-context: gke
users:
- name: gke
  user:
    token: %s
`, endpoint, caCert, clientCfg.AccessToken)
		}).(pulumi.StringOutput)

	return kubernetes.NewProvider(ctx, "gke", &kubernetes.ProviderArgs{
		Kubeconfig: kubeconfig,
	})
}

func buildNamespaces(ctx *pulumi.Context, c nuonConfig, prov *kubernetes.Provider, gke gkeCluster, labels pulumi.StringMap) ([]*corev1.Namespace, error) {
	var out []*corev1.Namespace
	for _, ns := range c.namespaces() {
		n, err := corev1.NewNamespace(ctx, ns, &corev1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:   pulumi.String(ns),
				Labels: labels,
			},
		}, pulumi.Provider(prov), pulumi.DependsOn([]pulumi.Resource{gke.cluster, gke.nodePool}))
		if err != nil {
			return nil, fmt.Errorf("create namespace %q: %w", ns, err)
		}
		out = append(out, n)
	}
	return out, nil
}
