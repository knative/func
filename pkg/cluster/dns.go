package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// configureMagicDNS patches CoreDNS to resolve the configured domain (e.g.,
// localtest.me) to the cluster node's IP addresses.
func configureMagicDNS(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	start := time.Now()
	status(out, "Configuring Magic DNS")

	ipv4, ipv6, err := getKindNodeIPs(ctx, cfg)
	if err != nil {
		return fmt.Errorf("getting cluster node IPs for DNS: %w", err)
	}

	if err := patchCoreDNSConfigMap(ctx, cfg, out, ipv4, ipv6); err != nil {
		return err
	}
	if err := patchCoreDNSDeployment(ctx, cfg, out); err != nil {
		return err
	}

	// Deployment patch triggers a rolling restart. Sleep so the new pods
	// enter NotReady before we wait for Ready — otherwise the old pods
	// still satisfy the condition and we return before the restart lands.
	if err := wait(ctx, 5*time.Second); err != nil {
		return err
	}
	if err := run(ctx, out, "",
		cfg.kubectl(), "wait", "pod",
		"--for=condition=Ready", "-l", "!job-name",
		"-n", "kube-system", "--timeout=60s"); err != nil {
		return fmt.Errorf("waiting for coredns: %w", err)
	}

	success(out, "Magic DNS", time.Since(start))
	return nil
}

// patchCoreDNSConfigMap writes a new Corefile and an example.db zone file
// (containing A/AAAA records for cfg.Domain) into the coredns ConfigMap.
// CoreDNS's `reload` plugin picks the new data up within ~30s.
func patchCoreDNSConfigMap(ctx context.Context, cfg ClusterConfig, out io.Writer, ipv4, ipv6 string) error {
	var records strings.Builder
	if ipv4 != "" {
		fmt.Fprintf(&records, "%s.\tIN\tA\t%s\n*.%s.\tIN\tA\t%s\n", cfg.Domain, ipv4, cfg.Domain, ipv4)
	}
	if ipv6 != "" {
		fmt.Fprintf(&records, "%s.\tIN\tAAAA\t%s\n*.%s.\tIN\tAAAA\t%s\n", cfg.Domain, ipv6, cfg.Domain, ipv6)
	}

	corefile := fmt.Sprintf(corefileTemplate, cfg.Domain)
	exampleDB := fmt.Sprintf(exampleDBTemplate, cfg.Domain, cfg.Domain, records.String())

	patch, err := json.Marshal(map[string]any{
		"data": map[string]string{
			"Corefile":   corefile,
			"example.db": exampleDB,
		},
	})
	if err != nil {
		return fmt.Errorf("marshaling corefile patch: %w", err)
	}
	if err := run(ctx, out, string(patch),
		cfg.kubectl(), "patch", "cm/coredns", "-n", "kube-system", "--patch-file", "/dev/stdin"); err != nil {
		return fmt.Errorf("patching coredns configmap: %w", err)
	}
	return nil
}

// patchCoreDNSDeployment applies a strategic-merge patch to mount both the
// Corefile and example.db keys from the coredns ConfigMap. The patch triggers
// a pod restart, which picks up the fresh ConfigMap contents.
func patchCoreDNSDeployment(ctx context.Context, cfg ClusterConfig, out io.Writer) error {
	if err := run(ctx, out, corednsDeployPatch,
		cfg.kubectl(), "patch", "deploy/coredns", "-n", "kube-system", "--patch-file", "/dev/stdin"); err != nil {
		return fmt.Errorf("patching coredns deployment: %w", err)
	}
	return nil
}

// corefileTemplate is the CoreDNS Corefile. %s is the magic-DNS domain whose
// zone data lives in example.db (mounted alongside the Corefile by
// corednsDeployPatch).
const corefileTemplate = `.:53 {
    errors
    health {
       lameduck 5s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
       pods insecure
       fallthrough in-addr.arpa ip6.arpa
       ttl 30
    }
    file /etc/coredns/example.db %s
    prometheus :9153
    forward . /etc/resolv.conf {
       max_concurrent 1000
    }
    cache 30
    loop
    reload
    loadbalance
}
`

// exampleDBTemplate is a BIND zone file: header comment, SOA, then the A/AAAA
// records. The three %s are domain, domain, records.
const exampleDBTemplate = "; %s test file\n" +
	"%s.\tIN\tSOA\tsns.dns.icann.org. noc.dns.icann.org. 2015082541 7200 3600 1209600 3600\n" +
	"%s"

// corednsDeployPatch mounts the Corefile and example.db keys from the
// coredns ConfigMap into /etc/coredns.
const corednsDeployPatch = `{
  "spec": {
    "template": {
      "spec": {
        "$setElementOrder/volumes": [
          {
            "name": "config-volume"
          }
        ],
        "volumes": [
          {
            "$retainKeys": [
              "configMap",
              "name"
            ],
            "configMap": {
              "items": [
                {
                  "key": "Corefile",
                  "path": "Corefile"
                },
                {
                  "key": "example.db",
                  "path": "example.db"
                }
              ]
            },
            "name": "config-volume"
          }
        ]
      }
    }
  }
}`
