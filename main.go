package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/artifactregistry"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "nuon")
		c := loadConfig(cfg)
		clusterName := c.computedClusterName()

		labels := pulumi.StringMap{
			"nuon-id":         pulumi.String(c.nuonID),
			"managed-by":      pulumi.String("nuon"),
			"sandbox-name":    pulumi.String("gcp-gke"),
			"sandbox-variant": pulumi.String("standard"),
		}
		mergeSanitized(labels, c.labels)

		vpc, err := buildVPC(ctx, c, clusterName)
		if err != nil {
			return err
		}

		repo, err := artifactregistry.NewRepository(ctx, "main", &artifactregistry.RepositoryArgs{
			Project:      pulumi.String(c.projectID),
			Location:     pulumi.String(c.region),
			RepositoryId: pulumi.String(clusterName),
			Format:       pulumi.String("DOCKER"),
			Labels:       labels,
		})
		if err != nil {
			return fmt.Errorf("create artifact registry repo: %w", err)
		}

		publicZone, internalZone, err := buildDNS(ctx, c, clusterName, vpc, labels)
		if err != nil {
			return err
		}

		gke, err := buildGKE(ctx, c, clusterName, vpc, labels)
		if err != nil {
			return err
		}

		k8sProvider, err := newK8sProvider(ctx, gke.cluster)
		if err != nil {
			return err
		}

		namespaces, err := buildNamespaces(ctx, c, k8sProvider, gke, labels)
		if err != nil {
			return err
		}

		if c.enableLinkerd {
			if err := buildLinkerd(ctx, k8sProvider, gke); err != nil {
				return err
			}
		}

		exportOutputs(ctx, c, clusterName, repo, vpc, gke, publicZone, internalZone, namespaces)
		return nil
	})
}

var labelSanitizer = regexp.MustCompile(`[/._]`)

func sanitizeLabel(s string) string {
	out := strings.ToLower(labelSanitizer.ReplaceAllString(s, "-"))
	if len(out) > 63 {
		out = out[:63]
	}
	return out
}

func mergeSanitized(dst pulumi.StringMap, src map[string]string) {
	for k, v := range src {
		dst[sanitizeLabel(k)] = pulumi.String(sanitizeLabel(v))
	}
}
