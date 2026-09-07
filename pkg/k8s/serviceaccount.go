package k8s

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetServiceAccount(ctx context.Context, c *Client, referencedServiceAccount, namespace string) error {
	k8sClient, err := c.Clientset()
	if err != nil {
		return err
	}
	_, err = k8sClient.CoreV1().ServiceAccounts(namespace).Get(ctx, referencedServiceAccount, metav1.GetOptions{})
	if err != nil {
		return err
	}
	return nil
}
