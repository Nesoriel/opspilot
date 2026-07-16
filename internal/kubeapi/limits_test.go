package kubeapi

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestClientEnforcesLocalNodePodAndEventLimits(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "operations", Name: "web", UID: types.UID("pod-uid")}}
	clientset := fake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-c"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-b"}},
		pod,
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "operations", Name: "worker-b"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "operations", Name: "worker-a"}},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Namespace: "operations", Name: "event-c"}, InvolvedObject: corev1.ObjectReference{UID: pod.UID}, Type: "Warning", Reason: "ReasonC"},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Namespace: "operations", Name: "event-a"}, InvolvedObject: corev1.ObjectReference{UID: pod.UID}, Type: "Warning", Reason: "ReasonA"},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Namespace: "operations", Name: "event-b"}, InvolvedObject: corev1.ObjectReference{UID: pod.UID}, Type: "Warning", Reason: "ReasonB"},
	)
	clientset.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: "v1.36.2"}
	client := NewForClient(clientset, "operations")

	cluster, err := client.ClusterInfo(context.Background(), 2)
	if err != nil {
		t.Fatalf("cluster info: %v", err)
	}
	if cluster.NodeCount != 2 || len(cluster.Nodes) != 2 || !cluster.Truncated {
		t.Fatalf("node limit was not enforced: %#v", cluster)
	}

	pods, err := client.PodList(context.Background(), "operations", 2)
	if err != nil {
		t.Fatalf("pod list: %v", err)
	}
	if pods.Count != 2 || len(pods.Pods) != 2 || !pods.Truncated {
		t.Fatalf("Pod limit was not enforced: %#v", pods)
	}

	inspected, err := client.PodInspect(context.Background(), "operations", "web", 2)
	if err != nil {
		t.Fatalf("pod inspect: %v", err)
	}
	if inspected.EventCount != 2 || !inspected.EventsTruncated {
		t.Fatalf("event limit was not enforced: %#v", inspected)
	}
}

func TestClientRejectsNonPositiveInternalLimits(t *testing.T) {
	client := NewForClient(fake.NewSimpleClientset(), "default")
	if _, err := client.ClusterInfo(context.Background(), 0); err == nil {
		t.Fatal("expected node limit error")
	}
	if _, err := client.PodList(context.Background(), "default", 0); err == nil {
		t.Fatal("expected Pod limit error")
	}
	if _, err := client.PodInspect(context.Background(), "default", "pod", 0); err == nil {
		t.Fatal("expected event limit error")
	}
}
