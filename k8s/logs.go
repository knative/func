package k8s

import (
	"bytes"
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
)

// GetPodLogs returns logs from a specified Container in a Pod, if container is empty string,
// then the first container in the pod is selected.
func GetPodLogs(ctx context.Context, namespace, podName, containerName string) (string, error) {
	podLogOpts := corev1.PodLogOptions{}
	if containerName != "" {
		podLogOpts.Container = containerName
	}

	client, namespace, _ := NewClientAndResolvedNamespace(namespace)
	request := client.CoreV1().Pods(namespace).GetLogs(podName, &podLogOpts)

	containerLogStream, err := request.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer containerLogStream.Close()

	buffer := new(bytes.Buffer)
	_, err = io.Copy(buffer, containerLogStream)
	if err != nil {
		return "", err
	}

	return buffer.String(), nil
}
