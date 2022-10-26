package k8s

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

func GetPersistentVolumeClaim(ctx context.Context, name, namespaceOverride string) (*corev1.PersistentVolumeClaim, error) {
	client, namespace, err := NewClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return nil, err
	}

	return client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
}

func CreatePersistentVolumeClaim(ctx context.Context, name, namespaceOverride string, labels map[string]string, annotations map[string]string, accessMode corev1.PersistentVolumeAccessMode, resourceRequest resource.Quantity) (err error) {
	client, namespace, err := NewClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{accessMode},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{},
			},
		},
	}
	pvc.Spec.Resources.Requests[corev1.ResourceStorage] = resourceRequest

	_, err = client.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
	return
}

func DeletePersistentVolumeClaims(ctx context.Context, namespaceOverride string, listOptions metav1.ListOptions) (err error) {
	client, namespace, err := NewClientAndResolvedNamespace(namespaceOverride)
	if err != nil {
		return
	}

	return client.CoreV1().PersistentVolumeClaims(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, listOptions)
}

var TarImage = "quay.io/boson/alpine-socat:1.7.4.3-r1-non-root"

// UploadToVolume uploads files (passed in form of tar stream) into volume.
func UploadToVolume(ctx context.Context, content io.Reader, claimName, namespace string) error {
	return runWithVolumeMounted(ctx, TarImage, []string{"tar", "-xmf", "-"}, content, claimName, namespace)
}

func getClientSetAndRestConfig(namespace string) (kubernetes.Interface, *restclient.Config, string, error) {
	cliConf := GetClientConfig()
	restConf, err := cliConf.ClientConfig()
	if err != nil {
		return nil, nil, "", fmt.Errorf("cannot get client config: %w", err)
	}
	restConf.WarningHandler = restclient.NoWarnings{}

	err = setConfigDefaults(restConf)
	if err != nil {
		return nil, nil, "", fmt.Errorf("cannot set config defaults: %w", err)
	}

	client, err := kubernetes.NewForConfig(restConf)
	if err != nil {
		return nil, nil, "", fmt.Errorf("cannot create k8s client: %w", err)
	}

	namespace, err = GetNamespace(namespace)
	if err != nil {
		return nil, nil, "", fmt.Errorf("cannot get namespace: %w", err)
	}

	return client, restConf, namespace, nil
}

func podWithVolumeMounted(podName, claimName, podImage string, podCommand []string) *corev1.Pod {
	const volumeMntPoint = "/tmp/volume_mnt"
	const pVol = "p-vol"
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Labels:      nil,
			Annotations: nil,
		},
		Spec: corev1.PodSpec{
			SecurityContext: defaultPodSecurityContext(),
			Containers: []corev1.Container{
				{
					Name:       podName,
					Image:      podImage,
					Stdin:      true,
					StdinOnce:  true,
					WorkingDir: volumeMntPoint,
					Command:    podCommand,
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      pVol,
							MountPath: volumeMntPoint,
						},
					},
					SecurityContext: defaultSecurityContext(client),
				},
			},
			Volumes: []corev1.Volume{{
				Name: pVol,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: claimName,
					},
				},
			}},
			DNSPolicy:     corev1.DNSClusterFirst,
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
}

// Runs a pod with given image, command and stdin
// while having the volume mounted and working directory set to it.
func runWithVolumeMounted(ctx context.Context, podImage string, podCommand []string, podInput io.Reader, claimName, namespace string) error {
	var err error

	client, restConf, namespace, err := getClientSetAndRestConfig(namespace)
	if err != nil {
		return err
	}

	podName := "volume-uploader-" + rand.String(5)

	pods := client.CoreV1().Pods(namespace)

	defer func() {
		_ = pods.Delete(ctx, podName, metav1.DeleteOptions{})
	}()

	pod := podWithVolumeMounted(podName, claimName, podImage, podCommand)
	localCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ready := podReady(localCtx, client.CoreV1(), podName, namespace)

	_, err = pods.Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("cannot create pod: %w", err)
	}

	select {
	case err = <-ready:
	case <-ctx.Done():
		err = ctx.Err()
	case <-time.After(time.Minute * 5):
		err = errors.New("timeout waiting for pod to start")
	}

	if err != nil {
		return fmt.Errorf("cannot start the pod: %w", err)
	}

	nameSelector := fields.OneTermEqualSelector("metadata.name", podName).String()
	listOpts := metav1.ListOptions{
		FieldSelector: nameSelector,
		Watch:         true,
	}
	watcher, err := pods.Watch(localCtx, listOpts)
	if err != nil {
		return fmt.Errorf("cannot set up the watcher: %w", err)
	}
	defer watcher.Stop()
	termCh := make(chan corev1.ContainerStateTerminated, 1)
	go func() {
		for event := range watcher.ResultChan() {
			p, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}
			if len(p.Status.ContainerStatuses) > 0 {
				termState := event.Object.(*corev1.Pod).Status.ContainerStatuses[0].State.Terminated
				if termState != nil {
					termCh <- *termState
					break
				}
			}
		}
	}()

	var outBuff tsBuff
	err = attach(client.CoreV1().RESTClient(), restConf, podName, namespace, podInput, &outBuff, &outBuff)
	if err != nil {
		return fmt.Errorf("cannot attach stdio to the pod: %w", err)
	}

	termState := <-termCh
	if termState.ExitCode != 0 {
		cmdOut := strings.Trim(outBuff.String(), "\n")
		err = fmt.Errorf("the command failed: exitcode=%d, out=%q", termState.ExitCode, cmdOut)
	}

	return err
}

// thread safe buffer for logging
type tsBuff struct {
	buff bytes.Buffer
	mu   sync.Mutex
}

func (t *tsBuff) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buff.String()
}

func (t *tsBuff) Write(p []byte) (n int, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buff.Write(p)
}

func ServeRsyncOnVolume(ctx context.Context, listener net.Listener, claimName, namespace string) error {
	client, restConf, namespace, err := getClientSetAndRestConfig(namespace)
	if err != nil {
		return err
	}

	podName := "volume-rsync-" + rand.String(5)
	volumeHelperImage := "quay.io/mvasek/volume-helper:latest"
	pod := podWithVolumeMounted(podName, claimName, volumeHelperImage, []string{"socat", "-u", "-", "OPEN:/dev/null"})

	pods := client.CoreV1().Pods(namespace)

	defer func() {
		_ = pods.Delete(ctx, podName, metav1.DeleteOptions{})
	}()

	localCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	ready := podReady(localCtx, client.CoreV1(), podName, namespace)

	_, err = pods.Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("cannot create pod: %w", err)
	}

	select {
	case err = <-ready:
	case <-ctx.Done():
		err = ctx.Err()
	case <-time.After(time.Minute * 5):
		err = errors.New("timeout waiting for pod to start")
	}

	if err != nil {
		return fmt.Errorf("cannot start the pod: %w", err)
	}

	detachChan := make(chan struct{})
	defer close(detachChan)
	go func() {
		_ = attach(client.CoreV1().RESTClient(), restConf, podName, namespace, emptyBlockingReader(detachChan), io.Discard, io.Discard)
	}()

	handleRsyncConnection := func(conn net.Conn) error {
		defer conn.Close()
		errOut := &tsBuff{}
		err := exec(client.CoreV1().RESTClient(), restConf,
			podName, namespace,
			[]string{"socat", "-", "EXEC:rsync --daemon"},
			conn, conn, errOut,
		)
		if err != nil {
			return fmt.Errorf("errow while exec (err out: %q): %w", errOut.String(), err)
		}
		return nil
	}

	var eg errgroup.Group
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}
			return fmt.Errorf("cannot accpet connection: %w", err)

		}
		eg.Go(func() error { return handleRsyncConnection(conn) })
	}

	err = eg.Wait()
	if err != nil {
		err = fmt.Errorf("error while handling connection(s): %w", err)
	}
	return err
}
