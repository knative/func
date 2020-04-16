package kubectl

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/lkingland/faas/k8s"
)

const service = `
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
 name: {{ .Project }}
 namespace: default
 labels:
   # Will be exposed as this domain as per the knative service domains config.
   #  Configured in config-domain
   faas.domain: {{ .Domain }}
 annotations: 
   # Will be exposed as this specific subdomain rather than the autogernerated
   # name.namespace.
   # Configured in config-network
   faas.subdomain: {{ .Subdomain }}
   # TODO: test if the following forces the service to be cluster.local:
   # serving.knative.dev/visibility=cluster-local
spec:
 template:
  spec:
   containers: 
   - image: {{ .Image }}
     env: 
     - name: VERBOSE 
       value: "true"
`

// Deployer implemented using the kubectl binary.
type Deployer struct {
	// Verbose logging.
	Verbose bool
}

// NewDeployer creates an instance of the kubectl-based deployer.
func NewDeployer() *Deployer {
	return &Deployer{}
}

// Deploy the named image.
func (d *Deployer) Deploy(name, image string) (address string, err error) {
	// assert kubectl
	if _, err = exec.LookPath("kubectl"); err != nil {
		err = errors.New("please install 'kubectl'")
		return
	}

	// Parse the service template
	t := template.Must(template.New("service").Parse(service))

	// Create an autoremoved temp file for the service config.
	f, err := ioutil.TempFile("", "faas-service")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(f.Name())

	// Convert the project name proper (a valid domain) to how it is being
	// represented by appsody:  as a kubernetes and docker valid name (RFC1035 label)
	// for use as the service's deployed name.
	project, err := k8s.ToSubdomain(name)
	if err != nil {
		return
	}

	nn := strings.Split(name, ".")
	if len(nn) < 3 {
		err = errors.New(fmt.Sprintf("invalid service name '%v', must be at least three parts.\n", name))
	}
	subdomain := nn[0]
	domain := strings.Join(nn[1:], ".")

	// Minimum three parts expected: [service].[domain].[tld].
	// Since knative+kube does not support multi-level subdomains, we can simply
	// consider the first token to be the subdomain, and in the event of a
	// multi-level path, the config-domain map would have to include separate
	// entries for, ex x.example.com and y.x.example.com, with only the leading
	// node being considered the subdomain for

	// Write out the final service yaml
	err = t.Execute(f, map[string]string{
		"Project":   project,
		"Subdomain": subdomain,
		"Domain":    domain,
		"Image":     image,
	})

	cmd := exec.Command("kubectl", "apply", "-f", f.Name())

	// If verbose logging is enabled, echo appsody's chatty stdout.
	if d.Verbose {
		fmt.Println(cmd)
		cmd.Stdout = os.Stdout
	}

	// Capture stderr for echoing on failure.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Run the command, echoing captured stderr as well ass the cmd internal error.
	err = cmd.Run()
	if err != nil {
		// TODO: sanitize stderr from appsody, or submit a PR to remove duplicates etc.
		err = errors.New(fmt.Sprintf("%v. %v", string(stderr.Bytes()), err.Error()))
		return
	}

	// TODO: explicitly pull address:
	return "https://faas.example.com/", nil
}
