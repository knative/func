package k8s

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	batchV1 "k8s.io/api/batch/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	batchv1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

var SocatImage = "quay.io/boson/alpine-socat:1.7.4.3-r1-non-root"

// NewInClusterDialer creates context dialer that will dial TCP connections via POD running in k8s cluster.
// This is useful when accessing k8s services that are not exposed outside cluster (e.g. openshift image registry).
//
// Usage:
//
//	dialer, err := k8s.NewInClusterDialer(ctx)
//	if err != nil {
//	    return err
//	}
//	defer dialer.Close()
//
//	transport := &http.Transport{
//	    DialContext: dialer.DialContext,
//	}
//
//	var client = http.Client{
//	    Transport: transport,
//	}
func NewInClusterDialer(ctx context.Context, clientConfig clientcmd.ClientConfig) (*contextDialer, error) {
	c := &contextDialer{
		clientConfig: clientConfig,
		detachChan:   make(chan struct{}),
	}
	err := c.startDialerPod(ctx)
	if err != nil {
		return nil, err
	}
	return c, nil
}

type contextDialer struct {
	coreV1       v1.CoreV1Interface
	clientConfig clientcmd.ClientConfig
	batchV1      batchv1.BatchV1Interface
	restConf     *restclient.Config
	jobName      string
	podName      string
	namespace    string
	detachChan   chan struct{}
}

func (c *contextDialer) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	if !(network == "tcp" || network == "tcp4" || network == "tcp6") {
		return nil, fmt.Errorf("unsupported network: %q", network)
	}

	ctrStdin, ctrStdout, conn := newConn()
	connectSuccess := make(chan struct{})
	connectFailure := make(chan error, 1)
	go func() {
		stderrBuff := bytes.NewBuffer(nil)
		ctrStderr := io.MultiWriter(stderrBuff, detectConnSuccess(connectSuccess))

		err := c.exec(addr, ctrStdin, ctrStdout, ctrStderr)
		if err != nil {
			stderrStr := stderrBuff.String()
			socatErr := tryParseSocatError(network, addr, stderrStr)
			if socatErr != nil {
				err = fmt.Errorf("socat error: %w", socatErr)
			} else {
				err = fmt.Errorf("failed to exec in pod: %w (stderr: %q)", err, stderrStr)
			}
		}
		_ = conn.closeWithError(err)
		connectFailure <- err
	}()

	select {
	case <-connectSuccess:
		return conn, nil
	case err := <-connectFailure:
		return nil, err
	case <-ctx.Done():
		_ = conn.closeWithError(ctx.Err())
		return nil, ctx.Err()
	}
}

var connSuccessfulRE = regexp.MustCompile("successfully connected")

// Creates io.Writer which closes connectSuccess channel when string "successfully connected" is written to it.
func detectConnSuccess(connectSuccess chan struct{}) io.Writer {
	pr, pw := io.Pipe()
	go func() {
		ok := connSuccessfulRE.MatchReader(bufio.NewReader(pr))
		if ok {
			close(connectSuccess)
		}
		_, _ = io.Copy(io.Discard, pr)
	}()
	return pw
}

var (
	connectionRefusedErrorRE = regexp.MustCompile(`E connect\(\d+, AF=\d+ (?P<hostport>[\[\]0-9.:a-z]+), \d+\): Connection refused`)
	nameResolutionErrorRE    = regexp.MustCompile(`E getaddrinfo\("(?P<hostname>[a-zA-z-.0-9]+)",.*\): Name does not resolve`)
)

// tries to detect common errors from `socat` stderr
func tryParseSocatError(network, address, stderr string) error {
	groups := nameResolutionErrorRE.FindStringSubmatch(stderr)
	if groups != nil {
		var name string
		if len(groups) > 1 {
			name = groups[1]
		}
		return &net.OpError{
			Op:     "dial",
			Net:    network,
			Source: nil,
			Addr:   nil,
			Err: &net.DNSError{
				Err:        "no such host",
				Name:       name,
				IsNotFound: true,
			},
		}
	}
	groups = connectionRefusedErrorRE.FindStringSubmatch(stderr)
	if groups != nil {
		var (
			addr net.IP
			port int
			zone string
		)
		if len(groups) > 1 {
			h, p, err := net.SplitHostPort(groups[1])
			if err == nil {
				addr = net.ParseIP(h)
				p, _ := strconv.ParseInt(p, 10, 16)
				port = int(p)
			}
		}
		return &net.OpError{
			Op:  "dial",
			Net: network,
			Addr: &net.TCPAddr{
				IP:   addr,
				Port: port,
				Zone: zone,
			},
			Err: &os.SyscallError{
				Syscall: "connect",
				Err:     syscall.ECONNREFUSED,
			},
		}
	}
	return nil
}

func (c *contextDialer) Close() error {
	// closing the channel will cause stdin of the attached container to return EOF
	// as a result the pod exits -- it transits to Completed state
	close(c.detachChan)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()

	pp := metaV1.DeletePropagationForeground
	delOpts := metaV1.DeleteOptions{
		PropagationPolicy: &pp,
	}

	return c.batchV1.Jobs(c.namespace).Delete(ctx, c.jobName, delOpts)
}

func (c *contextDialer) startDialerPod(ctx context.Context) (err error) {
	c.restConf, err = c.clientConfig.ClientConfig()
	if err != nil {
		return
	}
	c.restConf.WarningHandler = restclient.NoWarnings{}

	err = setConfigDefaults(c.restConf)
	if err != nil {
		return
	}

	client, err := kubernetes.NewForConfig(c.restConf)
	if err != nil {
		return
	}

	c.coreV1 = client.CoreV1()
	c.batchV1 = client.BatchV1()

	c.namespace, _, err = c.clientConfig.Namespace()
	if err != nil {
		return
	}

	jobs := client.BatchV1().Jobs(c.namespace)

	defer func() {
		if err != nil {
			c.Close()
		}
	}()

	c.jobName = "in-cluster-dialer-" + rand.String(5)

	job := &batchV1.Job{
		ObjectMeta: metaV1.ObjectMeta{
			Name: c.jobName,
		},
		Spec: batchV1.JobSpec{
			Template: coreV1.PodTemplateSpec{
				Spec: coreV1.PodSpec{
					Containers: []coreV1.Container{
						{
							Name:      "container",
							Image:     SocatImage,
							Stdin:     true,
							StdinOnce: true,
							Command:   []string{"socat", "-u", "-", "OPEN:/dev/null"},
						},
					},
					DNSPolicy:     coreV1.DNSClusterFirst,
					RestartPolicy: coreV1.RestartPolicyNever,
				},
			},
		},
	}

	creatOpts := metaV1.CreateOptions{}

	podChan, err := podReady(ctx, c.coreV1, c.jobName, c.namespace)
	if err != nil {
		return fmt.Errorf("cannot setup pod watch: %w", err)
	}

	_, err = jobs.Create(ctx, job, creatOpts)
	if err != nil {
		return
	}

	select {
	case poe := <-podChan:
		if poe.err != nil {
			return poe.err
		}
		c.podName = poe.pod.Name
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
		_ = attach(c.coreV1.RESTClient(), c.restConf, c.podName, c.namespace, emptyBlockingReader(c.detachChan), io.Discard, io.Discard)
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
		Command:   []string{"socat", "-dd", "-", fmt.Sprintf("TCP:%s", hostPort)},
		Container: "container",
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

func attach(restClient restclient.Interface, restConf *restclient.Config, podName, namespace string, in io.Reader, out, errOut io.Writer) error {
	req := restClient.Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("attach")
	req.VersionedParams(&coreV1.PodAttachOptions{
		Container: "container",
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(restConf, "POST", req.URL())
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

type podOrError struct {
	pod *coreV1.Pod
	err error
}

func podReady(ctx context.Context, core v1.CoreV1Interface, jobName, namespace string) (result <-chan podOrError, err error) {
	outChan := make(chan podOrError, 1)
	result = outChan

	pods := core.Pods(namespace)

	listOpts := metaV1.ListOptions{
		Watch:         true,
		LabelSelector: "job-name=" + jobName,
	}
	watcher, err := pods.Watch(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	go func() {
		defer watcher.Stop()
		watchChan := watcher.ResultChan()
		for event := range watchChan {
			pod, ok := event.Object.(*coreV1.Pod)
			if !ok {
				continue
			}

			if event.Type == watch.Modified {
				for _, status := range pod.Status.ContainerStatuses {
					if status.Ready {
						outChan <- podOrError{pod: pod}
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
							e := fmt.Errorf("reason: %v, message: %v",
								status.State.Waiting.Reason,
								status.State.Waiting.Message)
							outChan <- podOrError{err: e}
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
	pr  *io.PipeReader
	pw  *io.PipeWriter
	err atomic.Pointer[error]
}

func (c *conn) Read(b []byte) (n int, err error) {
	n, err = c.pr.Read(b)
	if errors.Is(err, io.ErrClosedPipe) {
		if p := c.err.Load(); p != nil {
			err = *p
		}
	}
	return
}

func (c *conn) Write(b []byte) (n int, err error) {
	n, err = c.pw.Write(b)
	if errors.Is(err, io.ErrClosedPipe) {
		if p := c.err.Load(); p != nil {
			err = *p
		}
	}
	return
}

func (c *conn) closeWithError(err error) error {
	if err == nil {
		err = net.ErrClosed
	}

	{
		e := err
		c.err.CompareAndSwap(nil, &e)
	}
	err = c.pw.CloseWithError(io.EOF)
	if err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}
	err = c.pr.CloseWithError(net.ErrClosed)
	if err != nil {
		return fmt.Errorf("failed to close reader: %w", err)
	}
	return nil
}

func (c *conn) Close() error {
	return c.closeWithError(nil)
}

func (c *conn) LocalAddr() net.Addr {
	return addr{}
}

func (c *conn) RemoteAddr() net.Addr {
	return addr{}
}

func (c *conn) SetDeadline(t time.Time) error { return nil }

func (c *conn) SetReadDeadline(t time.Time) error { return nil }

func (c *conn) SetWriteDeadline(t time.Time) error { return nil }

func newConn() (*io.PipeReader, *io.PipeWriter, *conn) {
	pr0, pw0 := io.Pipe()
	pr1, pw1 := io.Pipe()
	rwc := &conn{pr: pr0, pw: pw1}
	return pr1, pw0, rwc
}

func NewLazyInitInClusterDialer(clientConfig clientcmd.ClientConfig) *lazyInitInClusterDialer {
	return &lazyInitInClusterDialer{
		clientConfig: clientConfig,
	}
}

type lazyInitInClusterDialer struct {
	clientConfig  clientcmd.ClientConfig
	contextDialer *contextDialer
	initErr       error
	o             sync.Once
}

func (l *lazyInitInClusterDialer) DialContext(ctx context.Context, network string, addr string) (net.Conn, error) {
	l.o.Do(func() {
		l.contextDialer, l.initErr = NewInClusterDialer(ctx, l.clientConfig)
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
