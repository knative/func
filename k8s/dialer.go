package k8s

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	socatImage = "quay.io/mvasek/socat:alpine"
)

type ContextDialer interface {
	DialContext(ctx context.Context, network string, addr string) (net.Conn, error)
	Close() error
}

// NewInClusterDialer creates context dialer that will dial TCP connections via POD running in k8s cluster.
// This is useful when accessing k8s services that are not exposed outside cluster (e.g. openshift image registry).
//
// Usage:
//
//     dialer := k8s.NewInClusterDialer()
//     defer dialer.Close()
//
//     transport := &http.Transport{
//         DialContext: dialer.DialContext,
//     }
//
//     var client = http.Client{
//         Transport: transport,
//     }
func NewInClusterDialer() ContextDialer {
	return &contextDialer{}
}

type contextDialer struct {
	lck      sync.Mutex
	cleanUps []func()
}

func (c *contextDialer) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	if !(network == "tcp" || network == "tcp4" || network == "tcp6") {
		return nil, fmt.Errorf("unsupported network: %q", network)
	}
	pr, pw, conn := newConn()
	err, attach, removePod := runDialerPod(ctx, addr)
	if err != nil {
		return nil, err
	}

	go func() {
		c.lck.Lock()
		defer c.lck.Unlock()
		c.cleanUps = append(c.cleanUps, removePod)
	}()

	attachErrChan := make(chan error)
	go func() {
		attachErrChan <- attach(pr, pw, os.Stderr)
	}()

	conn.cleanUp = func() {
		err = <-attachErrChan
		fmt.Fprintf(os.Stderr, "failed to attach to dialer pod: %v", err)
		removePod()
	}
	return conn, nil
}

func (c *contextDialer) Close() error {
	c.lck.Lock()
	defer c.lck.Unlock()
	for _, fn := range c.cleanUps {
		fn()
	}
	return nil
}

type addr struct{}

func (a addr) Network() string {
	return "pod-stdio"
}

func (a addr) String() string {
	return "pod-stdio"
}

type conn struct {
	pr      *io.PipeReader
	pw      *io.PipeWriter
	cleanUp func()
}

func (c conn) Read(b []byte) (n int, err error) {
	return c.pr.Read(b)
}

func (c conn) Write(b []byte) (n int, err error) {
	return c.pw.Write(b)
}

func (c conn) Close() error {
	err := c.pw.Close()
	if err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}
	c.cleanUp()
	err = c.pr.Close()
	if err != nil {
		return fmt.Errorf("failed to close reader: %w", err)
	}
	return nil
}

func (c conn) LocalAddr() net.Addr {
	return addr{}
}

func (c conn) RemoteAddr() net.Addr {
	return addr{}
}

func (c conn) SetDeadline(t time.Time) error { return nil }

func (c conn) SetReadDeadline(t time.Time) error { return nil }

func (c conn) SetWriteDeadline(t time.Time) error { return nil }

func newConn() (*io.PipeReader, *io.PipeWriter, conn) {
	pr0, pw0 := io.Pipe()
	pr1, pw1 := io.Pipe()
	rwc := conn{pr: pr0, pw: pw1}
	return pr1, pw0, rwc
}

type attachFn = func(in io.Reader, out, errOut io.Writer) error
type removeFn = func()

func runDialerPod(ctx context.Context, hostPort string) (err error, attach attachFn, removePod removeFn) {
	cliConf := GetClientConfig()
	restConf, err := cliConf.ClientConfig()
	if err != nil {
		return
	}

	err = setConfigDefaults(restConf)
	if err != nil {
		return
	}

	client, err := kubernetes.NewForConfig(restConf)
	if err != nil {
		return
	}

	namespace, err := GetNamespace("")
	if err != nil {
		return
	}

	pods := client.CoreV1().Pods(namespace)

	podName := "registry-dialer-" + rand.String(5)

	removePod = func() {
		delOpts := metaV1.DeleteOptions{}
		err = pods.Delete(ctx, podName, delOpts)
		if err != nil {
			return
		}
	}
	defer func() {
		if err != nil {
			removePod()
		}
	}()

	pod := &coreV1.Pod{
		ObjectMeta: metaV1.ObjectMeta{
			Name:        podName,
			Labels:      nil,
			Annotations: nil,
		},
		Spec: coreV1.PodSpec{
			Containers: []coreV1.Container{
				{
					Name:      podName,
					Image:     socatImage,
					Stdin:     true,
					StdinOnce: true,
					Args:      []string{"-", fmt.Sprintf("TCP:%s", hostPort)},
				},
			},
			DNSPolicy:     coreV1.DNSClusterFirst,
			RestartPolicy: coreV1.RestartPolicyNever,
		},
	}
	creatOpts := metaV1.CreateOptions{}

	ready := podReady(ctx, pods, pod)

	_, err = pods.Create(ctx, pod, creatOpts)
	if err != nil {
		return
	}
	<-ready

	restClient := client.CoreV1().RESTClient()
	req := restClient.Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("attach")
	req.VersionedParams(&coreV1.PodAttachOptions{
		Container: podName,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(restConf, "POST", req.URL())
	if err != nil {
		return
	}

	attach = func(in io.Reader, out, errOut io.Writer) error {
		return exec.Stream(remotecommand.StreamOptions{
			Stdin:  in,
			Stdout: out,
			Stderr: errOut,
			Tty:    false,
		})
	}
	return nil, attach, removePod
}

// returns a channel that is closed when pod is ready
func podReady(ctx context.Context, pods v1.PodInterface, pod *coreV1.Pod) (done <-chan struct{}) {
	d := make(chan struct{})
	done = d

	nameSelector := fields.OneTermEqualSelector("metadata.name", pod.Name).String()
	listOpts := metaV1.ListOptions{
		Watch:           true,
		FieldSelector:   nameSelector,
		ResourceVersion: pod.ResourceVersion,
	}
	watcher, err := pods.Watch(ctx, listOpts)
	if err != nil {
		return
	}

	go func() {
		defer watcher.Stop()
		defer close(d)

		ch := watcher.ResultChan()
		for event := range ch {
			if event.Type == watch.Modified {
				pod := event.Object.(*coreV1.Pod)
				for _, condition := range pod.Status.Conditions {
					if condition.Type == coreV1.ContainersReady && condition.Status == coreV1.ConditionTrue {
						return
					}
				}
			}
		}
	}()

	return
}

func setConfigDefaults(config *restclient.Config) error {
	gv := coreV1.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/api"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	if config.UserAgent == "" {
		config.UserAgent = restclient.DefaultKubernetesUserAgent()
	}

	return nil
}
