package k8s

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
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
	socatImage = "quay.io/boson/alpine-socat:1.7.4.3-r"
)

// NewInClusterDialer creates context dialer that will dial TCP connections via POD running in k8s cluster.
// This is useful when accessing k8s services that are not exposed outside cluster (e.g. openshift image registry).
//
// Usage:
//
//     dialer, err := k8s.NewInClusterDialer(ctx)
//     if err != nil {
//         return err
//     }
//     defer dialer.Close()
//
//     transport := &http.Transport{
//         DialContext: dialer.DialContext,
//     }
//
//     var client = http.Client{
//         Transport: transport,
//     }
func NewInClusterDialer(ctx context.Context) (*contextDialer, error) {
	c := &contextDialer{
		detachChan: make(chan struct{}),
	}
	err := c.startDialerPod(ctx)
	if err != nil {
		return nil, err
	}
	return c, nil
}

type contextDialer struct {
	coreV1     v1.CoreV1Interface
	restConf   *restclient.Config
	podName    string
	namespace  string
	detachChan chan struct{}
}

func (c *contextDialer) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	if !(network == "tcp" || network == "tcp4" || network == "tcp6") {
		return nil, fmt.Errorf("unsupported network: %q", network)
	}

	execDone := make(chan struct{})
	pr, pw, conn := newConn(execDone)

	go func() {
		defer close(execDone)
		errOut := bytes.NewBuffer(nil)
		err := c.exec(addr, pr, pw, errOut)
		if err != nil {
			err = fmt.Errorf("failed to exec in pod: %w (stderr: %q)", err, errOut.String())
			_ = pr.CloseWithError(err)
			_ = pw.CloseWithError(err)
		}
	}()

	return conn, nil
}

func (c *contextDialer) Close() error {
	// closing the channel will cause stdin of the attached container to return EOF
	// as a result the pod exits -- it transits to Completed state
	close(c.detachChan)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()
	delOpts := metaV1.DeleteOptions{}

	return c.coreV1.Pods(c.namespace).Delete(ctx, c.podName, delOpts)
}

func (c *contextDialer) startDialerPod(ctx context.Context) (err error) {
	cliConf := GetClientConfig()
	c.restConf, err = cliConf.ClientConfig()
	if err != nil {
		return
	}

	err = setConfigDefaults(c.restConf)
	if err != nil {
		return
	}

	client, err := kubernetes.NewForConfig(c.restConf)
	if err != nil {
		return
	}
	c.coreV1 = client.CoreV1()

	c.namespace, err = GetNamespace("")
	if err != nil {
		return
	}

	pods := client.CoreV1().Pods(c.namespace)

	c.podName = "in-cluster-dialer-" + rand.String(5)

	defer func() {
		if err != nil {
			c.Close()
		}
	}()

	pod := &coreV1.Pod{
		ObjectMeta: metaV1.ObjectMeta{
			Name:        c.podName,
			Labels:      nil,
			Annotations: nil,
		},
		Spec: coreV1.PodSpec{
			Containers: []coreV1.Container{
				{
					Name:      c.podName,
					Image:     socatImage,
					Stdin:     true,
					StdinOnce: true,
					Args:      []string{"-u", "-", "OPEN:/dev/null,append"},
				},
			},
			DNSPolicy:     coreV1.DNSClusterFirst,
			RestartPolicy: coreV1.RestartPolicyNever,
		},
	}
	creatOpts := metaV1.CreateOptions{}

	ready := c.podReady(ctx)

	_, err = pods.Create(ctx, pod, creatOpts)
	if err != nil {
		return
	}

	select {
	case err = <-ready:
	case <-ctx.Done():
		err = ctx.Err()
	case <-time.After(time.Minute * 1):
		err = errors.New("timeout")
	}

	if err != nil {
		return fmt.Errorf("failed to start dialer container: %w", err)
	}

	// attaching to the stdin to automatically Complete the pod on exit
	go func() {
		_ = c.attach(emptyBlockingReader(c.detachChan), io.Discard, io.Discard)
	}()

	return nil
}

// reader that returns no data and blocks until
// the channel is closed or data are sent to the channel
type emptyBlockingReader chan struct{}

func (e emptyBlockingReader) Read(p []byte) (n int, err error) {
	<-e
	return 0, io.EOF
}

func (c *contextDialer) exec(hostPort string, in io.Reader, out, errOut io.Writer) error {

	restClient := c.coreV1.RESTClient()
	req := restClient.Post().
		Resource("pods").
		Name(c.podName).
		Namespace(c.namespace).
		SubResource("exec")
	req.VersionedParams(&coreV1.PodExecOptions{
		Command:   []string{"socat", "-", fmt.Sprintf("TCP:%s", hostPort)},
		Container: c.podName,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(c.restConf, "POST", req.URL())
	if err != nil {
		return err
	}

	return executor.Stream(remotecommand.StreamOptions{
		Stdin:  in,
		Stdout: out,
		Stderr: errOut,
		Tty:    false,
	})
}

func (c *contextDialer) attach(in io.Reader, out, errOut io.Writer) error {

	restClient := c.coreV1.RESTClient()
	req := restClient.Post().
		Resource("pods").
		Name(c.podName).
		Namespace(c.namespace).
		SubResource("attach")
	req.VersionedParams(&coreV1.PodAttachOptions{
		Container: c.podName,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(c.restConf, "POST", req.URL())
	if err != nil {
		return err
	}

	return executor.Stream(remotecommand.StreamOptions{
		Stdin:  in,
		Stdout: out,
		Stderr: errOut,
		Tty:    false,
	})
}

func (c *contextDialer) podReady(ctx context.Context) (errChan <-chan error) {
	d := make(chan error)
	errChan = d

	pods := c.coreV1.Pods(c.namespace)

	nameSelector := fields.OneTermEqualSelector("metadata.name", c.podName).String()
	listOpts := metaV1.ListOptions{
		Watch:         true,
		FieldSelector: nameSelector,
	}
	watcher, err := pods.Watch(ctx, listOpts)
	if err != nil {
		return
	}

	go func() {
		defer watcher.Stop()
		ch := watcher.ResultChan()
		for event := range ch {
			pod := event.Object.(*coreV1.Pod)

			if event.Type == watch.Modified {
				for _, status := range pod.Status.ContainerStatuses {
					if status.Ready {
						d <- nil
						return
					}
					if status.State.Waiting != nil {
						switch status.State.Waiting.Reason {
						case "ErrImagePull",
							"CreateContainerError",
							"CreateContainerConfigError",
							"InvalidImageName",
							"CrashLoopBackOff",
							"ImagePullBackOff":
							d <- fmt.Errorf("reason: %v, message: %v",
								status.State.Waiting.Reason,
								status.State.Waiting.Message)
							return
						default:
							continue
						}
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

type addr struct{}

func (a addr) Network() string {
	return "pod-stdio"
}

func (a addr) String() string {
	return "pod-stdio"
}

type conn struct {
	pr       *io.PipeReader
	pw       *io.PipeWriter
	execDone <-chan struct{}
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
	<-c.execDone
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

func newConn(execDone <-chan struct{}) (*io.PipeReader, *io.PipeWriter, conn) {
	pr0, pw0 := io.Pipe()
	pr1, pw1 := io.Pipe()
	rwc := conn{pr: pr0, pw: pw1, execDone: execDone}
	return pr1, pw0, rwc
}

func NewLazyInitInClusterDialer() *lazyInitInClusterDialer {
	return &lazyInitInClusterDialer{}
}

type lazyInitInClusterDialer struct {
	contextDialer *contextDialer
	initErr       error
	o             sync.Once
}

func (l *lazyInitInClusterDialer) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	l.o.Do(func() {
		l.contextDialer, l.initErr = NewInClusterDialer(ctx)
	})
	if l.initErr != nil {
		return nil, l.initErr
	}
	return l.contextDialer.DialContext(ctx, network, addr)
}

func (l *lazyInitInClusterDialer) Close() error {
	if l.contextDialer != nil {
		return l.contextDialer.Close()
	}
	return nil
}
