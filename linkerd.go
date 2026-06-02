package main

import (
	"fmt"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helm "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const linkerdEgressNetworkName = "all-egress"

// buildLinkerd installs cert-manager + linkerd and the egress network, mirroring
// linkerd.tf. Gated by enable_linkerd in the caller.
func buildLinkerd(ctx *pulumi.Context, prov *kubernetes.Provider, gke gkeCluster) error {
	clusterDeps := pulumi.DependsOn([]pulumi.Resource{gke.cluster, gke.nodePool})

	certManager, err := helm.NewRelease(ctx, "cert-manager", &helm.ReleaseArgs{
		Name:            pulumi.String("cert-manager"),
		Chart:           pulumi.String("cert-manager"),
		Version:         pulumi.String("v1.17.2"),
		Namespace:       pulumi.String("cert-manager"),
		CreateNamespace: pulumi.Bool(true),
		WaitForJobs:     pulumi.Bool(true),
		RepositoryOpts: &helm.RepositoryOptsArgs{
			Repo: pulumi.String("https://charts.jetstack.io"),
		},
		Values: pulumi.Map{
			"crds": pulumi.Map{"enabled": pulumi.Bool(true)},
			"startupapicheck": pulumi.Map{
				"timeout":      pulumi.String("5m"),
				"backoffLimit": pulumi.Int(20),
			},
		},
	}, pulumi.Provider(prov), clusterDeps)
	if err != nil {
		return fmt.Errorf("cert-manager release: %w", err)
	}

	bootstrapIssuer, err := apiextensions.NewCustomResource(ctx, "selfsigned-bootstrap", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("cert-manager.io/v1"),
		Kind:       pulumi.String("ClusterIssuer"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("selfsigned-bootstrap")},
		OtherFields: kubernetes.UntypedArgs{
			"spec": map[string]interface{}{"selfSigned": map[string]interface{}{}},
		},
	}, pulumi.Provider(prov), pulumi.DependsOn([]pulumi.Resource{certManager}))
	if err != nil {
		return fmt.Errorf("bootstrap issuer: %w", err)
	}

	caCert, err := apiextensions.NewCustomResource(ctx, "public-issuer-ca", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("cert-manager.io/v1"),
		Kind:       pulumi.String("Certificate"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("public-issuer-ca"),
			Namespace: pulumi.String("cert-manager"),
		},
		OtherFields: kubernetes.UntypedArgs{
			"spec": map[string]interface{}{
				"isCA":       true,
				"commonName": "public-issuer",
				"secretName": "public-issuer-ca-tls",
				"issuerRef": map[string]interface{}{
					"name": "selfsigned-bootstrap",
					"kind": "ClusterIssuer",
				},
				"privateKey": map[string]interface{}{"algorithm": "ECDSA", "size": 256},
			},
		},
	}, pulumi.Provider(prov), pulumi.DependsOn([]pulumi.Resource{bootstrapIssuer}))
	if err != nil {
		return fmt.Errorf("public issuer ca cert: %w", err)
	}

	_, err = apiextensions.NewCustomResource(ctx, "public-issuer", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("cert-manager.io/v1"),
		Kind:       pulumi.String("ClusterIssuer"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("public-issuer")},
		OtherFields: kubernetes.UntypedArgs{
			"spec": map[string]interface{}{
				"ca": map[string]interface{}{"secretName": "public-issuer-ca-tls"},
			},
		},
	}, pulumi.Provider(prov), pulumi.DependsOn([]pulumi.Resource{caCert}))
	if err != nil {
		return fmt.Errorf("public issuer: %w", err)
	}

	certs, err := generateLinkerdCerts()
	if err != nil {
		return fmt.Errorf("generate linkerd certs: %w", err)
	}

	linkerdCRDs, err := helm.NewRelease(ctx, "linkerd-crds", &helm.ReleaseArgs{
		Name:            pulumi.String("linkerd-crds"),
		Chart:           pulumi.String("linkerd-crds"),
		Version:         pulumi.String("2026.2.1"),
		Namespace:       pulumi.String("linkerd"),
		CreateNamespace: pulumi.Bool(true),
		RepositoryOpts: &helm.RepositoryOptsArgs{
			Repo: pulumi.String("https://helm.linkerd.io/edge"),
		},
		Values: pulumi.Map{
			"installGatewayAPI": pulumi.Bool(false),
		},
	}, pulumi.Provider(prov), clusterDeps)
	if err != nil {
		return fmt.Errorf("linkerd-crds release: %w", err)
	}

	controlPlane, err := helm.NewRelease(ctx, "linkerd-control-plane", &helm.ReleaseArgs{
		Name:      pulumi.String("linkerd-control-plane"),
		Chart:     pulumi.String("linkerd-control-plane"),
		Version:   pulumi.String("2026.2.1"),
		Namespace: pulumi.String("linkerd"),
		RepositoryOpts: &helm.RepositoryOptsArgs{
			Repo: pulumi.String("https://helm.linkerd.io/edge"),
		},
		Values: pulumi.Map{
			"identityTrustAnchorsPEM": pulumi.String(certs.trustAnchorPEM),
			"identity": pulumi.Map{
				"issuer": pulumi.Map{
					"tls": pulumi.Map{
						"crtPEM": pulumi.String(certs.issuerCertPEM),
						"keyPEM": pulumi.String(certs.issuerKeyPEM),
					},
				},
			},
		},
	}, pulumi.Provider(prov), pulumi.DependsOn([]pulumi.Resource{linkerdCRDs}))
	if err != nil {
		return fmt.Errorf("linkerd-control-plane release: %w", err)
	}

	egressNS, err := corev1.NewNamespace(ctx, "linkerd-egress", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String("linkerd-egress"),
			Annotations: pulumi.StringMap{
				"linkerd.io/inject": pulumi.String("enabled"),
			},
		},
	}, pulumi.Provider(prov), pulumi.DependsOn([]pulumi.Resource{controlPlane}))
	if err != nil {
		return fmt.Errorf("linkerd-egress namespace: %w", err)
	}

	_, err = apiextensions.NewCustomResource(ctx, "all-egress", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("policy.linkerd.io/v1alpha1"),
		Kind:       pulumi.String("EgressNetwork"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(linkerdEgressNetworkName),
			Namespace: pulumi.String("linkerd-egress"),
		},
		OtherFields: kubernetes.UntypedArgs{
			"spec": map[string]interface{}{
				"trafficPolicy": "Allow",
				"networks": []interface{}{
					map[string]interface{}{
						"cidr": "0.0.0.0/0",
						"except": []interface{}{
							"10.0.0.0/8",
							"172.16.0.0/12",
							"192.168.0.0/16",
						},
					},
				},
			},
		},
	}, pulumi.Provider(prov), pulumi.DependsOn([]pulumi.Resource{egressNS}))
	if err != nil {
		return fmt.Errorf("egress network: %w", err)
	}

	return nil
}
