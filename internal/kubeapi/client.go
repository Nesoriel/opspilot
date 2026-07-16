package kubeapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
)

type Reader interface {
	DefaultNamespace() string
	ClusterInfo(ctx context.Context, nodeLimit int64) (ClusterInfo, error)
	PodList(ctx context.Context, namespace string, limit int64) (PodList, error)
	PodInspect(ctx context.Context, namespace, name string, eventLimit int64) (PodInspect, error)
}

type clientFactory func() (kubernetes.Interface, string, error)

type Client struct {
	once             sync.Once
	factory          clientFactory
	client           kubernetes.Interface
	defaultNamespace string
	initErr          error
}

func New(config Config) *Client {
	return &Client{factory: func() (kubernetes.Interface, string, error) {
		restConfig, namespace, err := loadRESTConfig(config)
		if err != nil {
			return nil, "", err
		}
		client, err := kubernetes.NewForConfig(restConfig)
		if err != nil {
			return nil, "", errors.New("kubernetes_config_invalid: Kubernetes client could not be created")
		}
		return client, namespace, nil
	}}
}

func NewForClient(client kubernetes.Interface, namespace string) *Client {
	return &Client{factory: func() (kubernetes.Interface, string, error) {
		if client == nil {
			return nil, "", errors.New("kubernetes_config_invalid: Kubernetes client is nil")
		}
		if strings.TrimSpace(namespace) == "" {
			namespace = defaultNamespace
		}
		return client, namespace, nil
	}}
}

func (c *Client) initialize() {
	c.once.Do(func() {
		if c.factory == nil {
			c.initErr = errors.New("kubernetes_config_invalid: client factory is nil")
			return
		}
		c.client, c.defaultNamespace, c.initErr = c.factory()
	})
}

func (c *Client) backend() (kubernetes.Interface, error) {
	c.initialize()
	if c.initErr != nil {
		return nil, c.initErr
	}
	if c.client == nil {
		return nil, errors.New("kubernetes_config_invalid: Kubernetes client is unavailable")
	}
	return c.client, nil
}

func (c *Client) DefaultNamespace() string {
	c.initialize()
	if strings.TrimSpace(c.defaultNamespace) == "" {
		return defaultNamespace
	}
	return c.defaultNamespace
}

func (c *Client) ClusterInfo(ctx context.Context, nodeLimit int64) (ClusterInfo, error) {
	if err := ctx.Err(); err != nil {
		return ClusterInfo{}, classifyAPIError(err)
	}
	client, err := c.backend()
	if err != nil {
		return ClusterInfo{}, err
	}
	serverVersion, err := client.Discovery().ServerVersion()
	if err != nil {
		return ClusterInfo{}, classifyAPIError(err)
	}
	if err := ctx.Err(); err != nil {
		return ClusterInfo{}, classifyAPIError(err)
	}
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: nodeLimit})
	if err != nil {
		return ClusterInfo{}, classifyAPIError(err)
	}
	mapped := mapNodes(nodes.Items)
	return ClusterInfo{
		Version:   mapServerVersion(serverVersion),
		NodeCount: len(mapped),
		Truncated: nodes.Continue != "",
		Nodes:     mapped,
	}, nil
}

func (c *Client) PodList(ctx context.Context, namespace string, limit int64) (PodList, error) {
	client, err := c.backend()
	if err != nil {
		return PodList{}, err
	}
	queryNamespace := namespace
	if namespace == "*" {
		queryNamespace = metav1.NamespaceAll
	}
	pods, err := client.CoreV1().Pods(queryNamespace).List(ctx, metav1.ListOptions{Limit: limit})
	if err != nil {
		return PodList{}, classifyAPIError(err)
	}
	mapped := mapPods(pods.Items)
	return PodList{
		Namespace: namespace,
		Count:     len(mapped),
		Truncated: pods.Continue != "",
		Pods:      mapped,
	}, nil
}

func (c *Client) PodInspect(ctx context.Context, namespace, name string, eventLimit int64) (PodInspect, error) {
	client, err := c.backend()
	if err != nil {
		return PodInspect{}, err
	}
	pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return PodInspect{}, classifyAPIError(err)
	}
	selector := fields.OneTermEqualSelector("involvedObject.uid", string(pod.UID)).String()
	events, err := client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: selector,
		Limit:         eventLimit,
	})
	if err != nil {
		return PodInspect{}, classifyAPIError(err)
	}
	return mapPodInspect(pod, events.Items, events.Continue != ""), nil
}

func classifyAPIError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.Canceled):
		return errors.New("kubernetes_canceled: Kubernetes API request was canceled")
	case errors.Is(err, context.DeadlineExceeded), apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return errors.New("kubernetes_timeout: Kubernetes API request timed out")
	case apierrors.IsNotFound(err):
		return errors.New("kubernetes_not_found: requested Kubernetes resource was not found")
	case apierrors.IsForbidden(err):
		return errors.New("kubernetes_forbidden: access was denied by Kubernetes RBAC")
	case apierrors.IsUnauthorized(err):
		return errors.New("kubernetes_unauthorized: Kubernetes credentials were rejected")
	case apierrors.IsTooManyRequests(err):
		return errors.New("kubernetes_rate_limited: Kubernetes API rate limit was exceeded")
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return errors.New("kubernetes_timeout: Kubernetes API request timed out")
	}
	return fmt.Errorf("kubernetes_api_error: Kubernetes API request failed")
}

func eventReasonKey(event corev1.Event) string {
	return strings.TrimSpace(event.Type) + "/" + strings.TrimSpace(event.Reason)
}
