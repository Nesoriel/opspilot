package kubeapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestClientMapsClusterPodsAndEvents(t *testing.T) {
	now := metav1.NewTime(time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC))
	controller := true
	started := true
	privileged := false
	readOnly := true
	runAsNonRoot := true

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-b"},
		Spec:       corev1.NodeSpec{Unschedulable: true},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:              resource.MustParse("8"),
				corev1.ResourceMemory:           resource.MustParse("16Gi"),
				corev1.ResourcePods:             resource.MustParse("110"),
				corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("7500m"),
				corev1.ResourceMemory: resource.MustParse("15Gi"),
				corev1.ResourcePods:   resource.MustParse("100"),
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse, Reason: "KubeletHasSufficientMemory", LastTransitionTime: now},
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: "KubeletReady", LastTransitionTime: now},
			},
			NodeInfo: corev1.NodeSystemInfo{
				KubeletVersion:          "v1.36.2",
				ContainerRuntimeVersion: "containerd://2.0.5",
				OperatingSystem:         "linux",
				Architecture:            "amd64",
				OSImage:                 "Debian GNU/Linux 13",
				KernelVersion:           "6.12.0",
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "operations",
			Name:              "web-0",
			UID:               types.UID("pod-uid"),
			CreationTimestamp: now,
			Labels:            map[string]string{"secret-label": "private-label"},
			Annotations:       map[string]string{"secret-annotation": "private-annotation"},
			OwnerReferences: []metav1.OwnerReference{{Kind: "StatefulSet", Name: "web", Controller: &controller}},
		},
		Spec: corev1.PodSpec{
			NodeName:           "node-b",
			ServiceAccountName: "web-service-account",
			PriorityClassName:  "normal",
			RestartPolicy:      corev1.RestartPolicyAlways,
			DNSPolicy:          corev1.DNSClusterFirst,
			SchedulerName:      "default-scheduler",
			Containers: []corev1.Container{{
				Name:    "web",
				Image:   "example/web:v2",
				Command: []string{"/bin/web", "--token=command-secret"},
				Args:    []string{"--password=argument-secret"},
				Env:     []corev1.EnvVar{{Name: "DATABASE_PASSWORD", Value: "environment-secret"}},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
					Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("512Mi")},
				},
				SecurityContext: &corev1.SecurityContext{Privileged: &privileged, ReadOnlyRootFilesystem: &readOnly, RunAsNonRoot: &runAsNonRoot},
			}},
			InitContainers: []corev1.Container{{Name: "migrate", Image: "example/migrate:v2", Env: []corev1.EnvVar{{Name: "TOKEN", Value: "init-secret"}}}},
			Volumes: []corev1.Volume{{Name: "credentials", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "database-credentials"}}}},
		},
		Status: corev1.PodStatus{
			Phase:  corev1.PodRunning,
			Reason: "Running",
			Message: "free-text pod message with pod-secret",
			PodIP:   "10.244.0.12",
			QOSClass: corev1.PodQOSBurstable,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse, Reason: "ContainersNotReady", Message: "condition-secret", LastTransitionTime: now}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "web",
				Image:        "example/web:v2",
				Ready:        false,
				Started:      &started,
				RestartCount: 3,
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff", Message: "waiting-secret"}},
				LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1, Message: "termination-secret"}},
			}},
			InitContainerStatuses: []corev1.ContainerStatus{{Name: "migrate", Image: "example/migrate:v2", Ready: true, State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed", ExitCode: 0}}}},
		},
	}
	events := []runtimeObject{
		{event: &corev1.Event{ObjectMeta: metav1.ObjectMeta{Namespace: "operations", Name: "event-1"}, InvolvedObject: corev1.ObjectReference{UID: pod.UID}, Type: "Warning", Reason: "BackOff", Message: "event-secret-one", Count: 2, LastTimestamp: now}},
		{event: &corev1.Event{ObjectMeta: metav1.ObjectMeta{Namespace: "operations", Name: "event-2"}, InvolvedObject: corev1.ObjectReference{UID: pod.UID}, Type: "Warning", Reason: "BackOff", Message: "event-secret-two", Count: 1, LastTimestamp: now}},
	}

	objects := []interface{}{node, pod}
	for _, item := range events {
		objects = append(objects, item.event)
	}
	clientset := fake.NewSimpleClientset(toRuntimeObjects(t, objects...)...)
	discovery := clientset.Discovery().(*fakediscovery.FakeDiscovery)
	discovery.FakedServerVersion = &version.Info{GitVersion: "v1.36.2", Major: "1", Minor: "36", Platform: "linux/amd64"}
	client := NewForClient(clientset, "operations")

	cluster, err := client.ClusterInfo(context.Background(), 100)
	if err != nil {
		t.Fatalf("cluster info: %v", err)
	}
	if cluster.Version.GitVersion != "v1.36.2" || cluster.NodeCount != 1 || !cluster.Nodes[0].Ready || !cluster.Nodes[0].Unschedulable {
		t.Fatalf("unexpected cluster result: %#v", cluster)
	}
	if cluster.Nodes[0].Capacity.CPU != "8" || cluster.Nodes[0].Allocatable.Memory != "15Gi" {
		t.Fatalf("unexpected node resources: %#v", cluster.Nodes[0])
	}

	listed, err := client.PodList(context.Background(), "operations", 100)
	if err != nil {
		t.Fatalf("pod list: %v", err)
	}
	if listed.Count != 1 || listed.Pods[0].RestartCount != 3 || listed.Pods[0].Ready {
		t.Fatalf("unexpected pod list: %#v", listed)
	}

	inspected, err := client.PodInspect(context.Background(), "operations", "web-0", 50)
	if err != nil {
		t.Fatalf("pod inspect: %v", err)
	}
	if inspected.Summary.Name != "web-0" || inspected.ServiceAccountName != "web-service-account" {
		t.Fatalf("unexpected inspect metadata: %#v", inspected)
	}
	if len(inspected.Events) != 1 || inspected.Events[0].Reason != "BackOff" || inspected.Events[0].Count != 3 {
		t.Fatalf("events were not aggregated: %#v", inspected.Events)
	}
	if inspected.ContainerStatuses[0].Reason != "CrashLoopBackOff" || inspected.ContainerStatuses[0].LastTerminationReason != "Error" {
		t.Fatalf("unexpected container status: %#v", inspected.ContainerStatuses)
	}

	payload, err := json.Marshal(inspected)
	if err != nil {
		t.Fatalf("marshal inspect: %v", err)
	}
	for _, secret := range []string{
		"private-label",
		"private-annotation",
		"command-secret",
		"argument-secret",
		"environment-secret",
		"init-secret",
		"database-credentials",
		"pod-secret",
		"condition-secret",
		"waiting-secret",
		"termination-secret",
		"event-secret-one",
		"event-secret-two",
	} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("sensitive value %q leaked into output: %s", secret, payload)
		}
	}
}

func TestClientIsLazyAndReturnsConfigErrorOnUse(t *testing.T) {
	client := New(Config{KubeconfigPath: "/definitely/missing/kubeconfig"})
	if namespace := client.DefaultNamespace(); namespace != defaultNamespace {
		t.Fatalf("default namespace = %q", namespace)
	}
	_, err := client.PodList(context.Background(), defaultNamespace, 10)
	if err == nil || !strings.Contains(err.Error(), "kubernetes_config_") {
		t.Fatalf("unexpected config error: %v", err)
	}
}

func TestClassifyAPIError(t *testing.T) {
	resource := schema.GroupResource{Group: "", Resource: "pods"}
	tests := []struct {
		err  error
		code string
	}{
		{context.Canceled, "kubernetes_canceled"},
		{context.DeadlineExceeded, "kubernetes_timeout"},
		{apierrors.NewNotFound(resource, "web"), "kubernetes_not_found"},
		{apierrors.NewForbidden(resource, "web", errors.New("denied")), "kubernetes_forbidden"},
		{apierrors.NewUnauthorized("bad token"), "kubernetes_unauthorized"},
		{apierrors.NewTooManyRequests("slow down", 1), "kubernetes_rate_limited"},
		{errors.New("raw server detail /secret/path"), "kubernetes_api_error"},
	}
	for _, test := range tests {
		result := classifyAPIError(test.err)
		if result == nil || !strings.Contains(result.Error(), test.code) {
			t.Fatalf("error %v classified as %v", test.err, result)
		}
		if strings.Contains(result.Error(), "/secret/path") || strings.Contains(result.Error(), "bad token") {
			t.Fatalf("raw error detail leaked: %v", result)
		}
	}
}

type runtimeObject struct {
	event *corev1.Event
}

func toRuntimeObjects(t *testing.T, values ...interface{}) []runtime.Object {
	t.Helper()
	result := make([]runtime.Object, 0, len(values))
	for _, value := range values {
		object, ok := value.(runtime.Object)
		if !ok {
			t.Fatalf("value does not implement runtime.Object: %T", value)
		}
		result = append(result, object)
	}
	return result
}
