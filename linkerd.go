package main

import (
	"fmt"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helm "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-tls/sdk/v5/go/tls"
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

	// Linkerd mTLS certs via pulumi-tls so they stay stable in state across runs.
	anchorKey, err := tls.NewPrivateKey(ctx, "linkerd-trust-anchor", &tls.PrivateKeyArgs{
		Algorithm:  pulumi.String("ECDSA"),
		EcdsaCurve: pulumi.String("P256"),
	})
	if err != nil {
		return fmt.Errorf("linkerd trust anchor key: %w", err)
	}

	anchorCert, err := tls.NewSelfSignedCert(ctx, "linkerd-trust-anchor", &tls.SelfSignedCertArgs{
		PrivateKeyPem:   anchorKey.PrivateKeyPem,
		IsCaCertificate: pulumi.Bool(true),
		Subject: &tls.SelfSignedCertSubjectArgs{
			CommonName: pulumi.String("root.linkerd.cluster.local"),
		},
		ValidityPeriodHours: pulumi.Int(87600),
		AllowedUses:         pulumi.StringArray{pulumi.String("cert_signing"), pulumi.String("crl_signing")},
	})
	if err != nil {
		return fmt.Errorf("linkerd trust anchor cert: %w", err)
	}

	issuerKey, err := tls.NewPrivateKey(ctx, "linkerd-issuer", &tls.PrivateKeyArgs{
		Algorithm:  pulumi.String("ECDSA"),
		EcdsaCurve: pulumi.String("P256"),
	})
	if err != nil {
		return fmt.Errorf("linkerd issuer key: %w", err)
	}

	issuerReq, err := tls.NewCertRequest(ctx, "linkerd-issuer", &tls.CertRequestArgs{
		PrivateKeyPem: issuerKey.PrivateKeyPem,
		Subject: &tls.CertRequestSubjectArgs{
			CommonName: pulumi.String("identity.linkerd.cluster.local"),
		},
	})
	if err != nil {
		return fmt.Errorf("linkerd issuer cert request: %w", err)
	}

	issuerCert, err := tls.NewLocallySignedCert(ctx, "linkerd-issuer", &tls.LocallySignedCertArgs{
		CertRequestPem:      issuerReq.CertRequestPem,
		CaPrivateKeyPem:     anchorKey.PrivateKeyPem,
		CaCertPem:           anchorCert.CertPem,
		IsCaCertificate:     pulumi.Bool(true),
		ValidityPeriodHours: pulumi.Int(8760),
		AllowedUses:         pulumi.StringArray{pulumi.String("cert_signing")},
	})
	if err != nil {
		return fmt.Errorf("linkerd issuer cert: %w", err)
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
			"identityTrustAnchorsPEM": anchorCert.CertPem,
			"identity": pulumi.Map{
				"issuer": pulumi.Map{
					"tls": pulumi.Map{
						"crtPEM": issuerCert.CertPem,
						"keyPEM": issuerKey.PrivateKeyPem,
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
