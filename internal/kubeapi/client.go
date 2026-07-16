package kubeapi

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"

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
	if nodeLimit < 1 {
		return ClusterInfo{}, errors.New("kubernetes_request_invalid: node limit must be positive")
	}
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
	truncated := nodes.Continue != "" || int64(len(nodes.Items)) > nodeLimit
	items := nodes.Items
	if int64(len(items)) > nodeLimit {
		items = items[:nodeLimit]
	}
	mapped := mapNodes(items)
	return ClusterInfo{
		Version:   mapServerVersion(serverVersion),
		NodeCount: len(mapped),
		Truncated: truncated,
		Nodes:     mapped,
	}, nil
}

func (c *Client) PodList(ctx context.Context, namespace string, limit int64) (PodList, error) {
	if limit < 1 {
		return PodList{}, errors.New("kubernetes_request_invalid: Pod limit must be positive")
	}
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
	truncated := pods.Continue != "" || int64(len(pods.Items)) > limit
	items := pods.Items
	if int64(len(items)) > limit {
		items = items[:limit]
	}
	mapped := mapPods(items)
	return PodList{
		Namespace: namespace,
		Count:     len(mapped),
		Truncated: truncated,
		Pods:      mapped,
	}, nil
}

func (c *Client) PodInspect(ctx context.Context, namespace, name string, eventLimit int64) (PodInspect, error) {
	if eventLimit < 1 {
		return PodInspect{}, errors.New("kubernetes_request_invalid: event limit must be positive")
	}
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
	truncated := events.Continue != "" || int64(len(events.Items)) > eventLimit
	items := events.Items
	if int64(len(items)) > eventLimit {
		items = items[:eventLimit]
	}
	return mapPodInspect(pod, items, truncated), nil
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
	return errors.New("kubernetes_api_error: Kubernetes API request failed")
}
