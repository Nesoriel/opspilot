package kubeapi

import (
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
)

type ClusterInfo struct {
	Version   ServerVersion `json:"version"`
	NodeCount int           `json:"node_count"`
	Truncated bool          `json:"truncated"`
	Nodes     []NodeSummary `json:"nodes"`
}

type ServerVersion struct {
	GitVersion string `json:"git_version"`
	Major      string `json:"major,omitempty"`
	Minor      string `json:"minor,omitempty"`
	Platform   string `json:"platform,omitempty"`
}

type NodeSummary struct {
	Name                    string             `json:"name"`
	Ready                   bool               `json:"ready"`
	ReadyStatus             string             `json:"ready_status"`
	Unschedulable           bool               `json:"unschedulable"`
	KubeletVersion          string             `json:"kubelet_version,omitempty"`
	ContainerRuntimeVersion string             `json:"container_runtime_version,omitempty"`
	OperatingSystem         string             `json:"operating_system,omitempty"`
	Architecture            string             `json:"architecture,omitempty"`
	OSImage                 string             `json:"os_image,omitempty"`
	KernelVersion           string             `json:"kernel_version,omitempty"`
	Capacity                ResourceSummary    `json:"capacity"`
	Allocatable             ResourceSummary    `json:"allocatable"`
	Conditions              []ConditionSummary `json:"conditions,omitempty"`
}

type ResourceSummary struct {
	CPU              string `json:"cpu,omitempty"`
	Memory           string `json:"memory,omitempty"`
	Pods             string `json:"pods,omitempty"`
	EphemeralStorage string `json:"ephemeral_storage,omitempty"`
}

type ConditionSummary struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	LastTransitionTime string `json:"last_transition_time,omitempty"`
}

type PodList struct {
	Namespace string       `json:"namespace"`
	Count     int          `json:"count"`
	Truncated bool         `json:"truncated"`
	Pods      []PodSummary `json:"pods"`
}

type PodSummary struct {
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	CreatedAt       string `json:"created_at,omitempty"`
	Phase           string `json:"phase"`
	Reason          string `json:"reason,omitempty"`
	Ready           bool   `json:"ready"`
	NodeName        string `json:"node_name,omitempty"`
	PodIP           string `json:"pod_ip,omitempty"`
	QoSClass        string `json:"qos_class,omitempty"`
	RestartCount    int32  `json:"restart_count"`
	ContainersReady int    `json:"containers_ready"`
	ContainersTotal int    `json:"containers_total"`
	InitReady       int    `json:"init_ready"`
	InitTotal       int    `json:"init_total"`
}

type PodInspect struct {
	Summary               PodSummary             `json:"summary"`
	ServiceAccountName    string                 `json:"service_account_name,omitempty"`
	PriorityClassName     string                 `json:"priority_class_name,omitempty"`
	RestartPolicy         string                 `json:"restart_policy,omitempty"`
	DNSPolicy             string                 `json:"dns_policy,omitempty"`
	SchedulerName         string                 `json:"scheduler_name,omitempty"`
	HostNetwork           bool                   `json:"host_network"`
	Owners                []OwnerSummary         `json:"owners,omitempty"`
	Conditions            []ConditionSummary     `json:"conditions,omitempty"`
	Containers            []ContainerSpecSummary `json:"containers,omitempty"`
	InitContainers        []ContainerSpecSummary `json:"init_containers,omitempty"`
	ContainerStatuses     []ContainerStatus      `json:"container_statuses,omitempty"`
	InitContainerStatuses []ContainerStatus      `json:"init_container_statuses,omitempty"`
	EventCount            int                    `json:"event_count"`
	EventsTruncated       bool                   `json:"events_truncated"`
	Events                []EventSummary         `json:"events,omitempty"`
}

type OwnerSummary struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Controller bool   `json:"controller"`
}

type ContainerSpecSummary struct {
	Name                   string          `json:"name"`
	Image                  string          `json:"image"`
	Resources              ResourceRequest `json:"resources"`
	Privileged             *bool           `json:"privileged,omitempty"`
	ReadOnlyRootFilesystem *bool           `json:"read_only_root_filesystem,omitempty"`
	RunAsNonRoot           *bool           `json:"run_as_non_root,omitempty"`
}

type ResourceRequest struct {
	RequestCPU    string `json:"request_cpu,omitempty"`
	RequestMemory string `json:"request_memory,omitempty"`
	LimitCPU      string `json:"limit_cpu,omitempty"`
	LimitMemory   string `json:"limit_memory,omitempty"`
}

type ContainerStatus struct {
	Name                  string `json:"name"`
	Image                 string `json:"image"`
	Ready                 bool   `json:"ready"`
	Started               *bool  `json:"started,omitempty"`
	RestartCount          int32  `json:"restart_count"`
	State                 string `json:"state"`
	Reason                string `json:"reason,omitempty"`
	ExitCode              int32  `json:"exit_code,omitempty"`
	Signal                int32  `json:"signal,omitempty"`
	StartedAt             string `json:"started_at,omitempty"`
	FinishedAt            string `json:"finished_at,omitempty"`
	LastTerminationReason string `json:"last_termination_reason,omitempty"`
	LastExitCode          int32  `json:"last_exit_code,omitempty"`
}

type EventSummary struct {
	Type         string `json:"type"`
	Reason       string `json:"reason"`
	Count        int32  `json:"count"`
	LastObserved string `json:"last_observed,omitempty"`
}

func mapServerVersion(info *version.Info) ServerVersion {
	if info == nil {
		return ServerVersion{}
	}
	return ServerVersion{GitVersion: info.GitVersion, Major: info.Major, Minor: info.Minor, Platform: info.Platform}
}

func mapNodes(nodes []corev1.Node) []NodeSummary {
	result := make([]NodeSummary, 0, len(nodes))
	for _, node := range nodes {
		readyStatus := string(corev1.ConditionUnknown)
		ready := false
		conditions := make([]ConditionSummary, 0, len(node.Status.Conditions))
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady {
				readyStatus = string(condition.Status)
				ready = condition.Status == corev1.ConditionTrue
			}
			conditions = append(conditions, mapNodeCondition(condition))
		}
		sort.Slice(conditions, func(i, j int) bool { return conditions[i].Type < conditions[j].Type })
		result = append(result, NodeSummary{
			Name:                    node.Name,
			Ready:                   ready,
			ReadyStatus:             readyStatus,
			Unschedulable:           node.Spec.Unschedulable,
			KubeletVersion:          node.Status.NodeInfo.KubeletVersion,
			ContainerRuntimeVersion: node.Status.NodeInfo.ContainerRuntimeVersion,
			OperatingSystem:         node.Status.NodeInfo.OperatingSystem,
			Architecture:            node.Status.NodeInfo.Architecture,
			OSImage:                 node.Status.NodeInfo.OSImage,
			KernelVersion:           node.Status.NodeInfo.KernelVersion,
			Capacity:                mapResources(node.Status.Capacity),
			Allocatable:             mapResources(node.Status.Allocatable),
			Conditions:              conditions,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func mapNodeCondition(condition corev1.NodeCondition) ConditionSummary {
	return ConditionSummary{
		Type:               string(condition.Type),
		Status:             string(condition.Status),
		Reason:             condition.Reason,
		LastTransitionTime: formatTime(condition.LastTransitionTime),
	}
}

func mapPodCondition(condition corev1.PodCondition) ConditionSummary {
	return ConditionSummary{
		Type:               string(condition.Type),
		Status:             string(condition.Status),
		Reason:             condition.Reason,
		LastTransitionTime: formatTime(condition.LastTransitionTime),
	}
}

func mapResources(resources corev1.ResourceList) ResourceSummary {
	result := ResourceSummary{}
	if value, found := resources[corev1.ResourceCPU]; found {
		result.CPU = value.String()
	}
	if value, found := resources[corev1.ResourceMemory]; found {
		result.Memory = value.String()
	}
	if value, found := resources[corev1.ResourcePods]; found {
		result.Pods = value.String()
	}
	if value, found := resources[corev1.ResourceEphemeralStorage]; found {
		result.EphemeralStorage = value.String()
	}
	return result
}

func mapPods(pods []corev1.Pod) []PodSummary {
	result := make([]PodSummary, 0, len(pods))
	for index := range pods {
		result = append(result, mapPodSummary(&pods[index]))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func mapPodSummary(pod *corev1.Pod) PodSummary {
	if pod == nil {
		return PodSummary{}
	}
	ready := false
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			ready = condition.Status == corev1.ConditionTrue
			break
		}
	}
	containersReady := 0
	restarts := int32(0)
	for _, status := range pod.Status.ContainerStatuses {
		if status.Ready {
			containersReady++
		}
		restarts += status.RestartCount
	}
	initReady := 0
	for _, status := range pod.Status.InitContainerStatuses {
		if status.Ready || status.State.Terminated != nil && status.State.Terminated.ExitCode == 0 {
			initReady++
		}
		restarts += status.RestartCount
	}
	return PodSummary{
		Namespace:       pod.Namespace,
		Name:            pod.Name,
		CreatedAt:       formatTime(pod.CreationTimestamp),
		Phase:           string(pod.Status.Phase),
		Reason:          pod.Status.Reason,
		Ready:           ready,
		NodeName:        pod.Spec.NodeName,
		PodIP:           pod.Status.PodIP,
		QoSClass:        string(pod.Status.QOSClass),
		RestartCount:    restarts,
		ContainersReady: containersReady,
		ContainersTotal: len(pod.Spec.Containers),
		InitReady:       initReady,
		InitTotal:       len(pod.Spec.InitContainers),
	}
}

func mapPodInspect(pod *corev1.Pod, events []corev1.Event, eventsTruncated bool) PodInspect {
	if pod == nil {
		return PodInspect{}
	}
	conditions := make([]ConditionSummary, 0, len(pod.Status.Conditions))
	for _, condition := range pod.Status.Conditions {
		conditions = append(conditions, mapPodCondition(condition))
	}
	sort.Slice(conditions, func(i, j int) bool { return conditions[i].Type < conditions[j].Type })

	owners := make([]OwnerSummary, 0, len(pod.OwnerReferences))
	for _, owner := range pod.OwnerReferences {
		owners = append(owners, OwnerSummary{Kind: owner.Kind, Name: owner.Name, Controller: owner.Controller != nil && *owner.Controller})
	}
	sort.Slice(owners, func(i, j int) bool {
		if owners[i].Kind != owners[j].Kind {
			return owners[i].Kind < owners[j].Kind
		}
		return owners[i].Name < owners[j].Name
	})

	return PodInspect{
		Summary:               mapPodSummary(pod),
		ServiceAccountName:    pod.Spec.ServiceAccountName,
		PriorityClassName:     pod.Spec.PriorityClassName,
		RestartPolicy:         string(pod.Spec.RestartPolicy),
		DNSPolicy:             string(pod.Spec.DNSPolicy),
		SchedulerName:         pod.Spec.SchedulerName,
		HostNetwork:           pod.Spec.HostNetwork,
		Owners:                owners,
		Conditions:            conditions,
		Containers:            mapContainerSpecs(pod.Spec.Containers),
		InitContainers:        mapContainerSpecs(pod.Spec.InitContainers),
		ContainerStatuses:     mapContainerStatuses(pod.Status.ContainerStatuses),
		InitContainerStatuses: mapContainerStatuses(pod.Status.InitContainerStatuses),
		EventCount:            len(events),
		EventsTruncated:       eventsTruncated,
		Events:                aggregateEvents(events),
	}
}

func mapContainerSpecs(containers []corev1.Container) []ContainerSpecSummary {
	result := make([]ContainerSpecSummary, 0, len(containers))
	for _, container := range containers {
		summary := ContainerSpecSummary{
			Name:  container.Name,
			Image: container.Image,
			Resources: ResourceRequest{
				RequestCPU:    quantityString(container.Resources.Requests, corev1.ResourceCPU),
				RequestMemory: quantityString(container.Resources.Requests, corev1.ResourceMemory),
				LimitCPU:      quantityString(container.Resources.Limits, corev1.ResourceCPU),
				LimitMemory:   quantityString(container.Resources.Limits, corev1.ResourceMemory),
			},
		}
		if container.SecurityContext != nil {
			summary.Privileged = container.SecurityContext.Privileged
			summary.ReadOnlyRootFilesystem = container.SecurityContext.ReadOnlyRootFilesystem
			summary.RunAsNonRoot = container.SecurityContext.RunAsNonRoot
		}
		result = append(result, summary)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func mapContainerStatuses(statuses []corev1.ContainerStatus) []ContainerStatus {
	result := make([]ContainerStatus, 0, len(statuses))
	for _, status := range statuses {
		summary := ContainerStatus{
			Name:         status.Name,
			Image:        status.Image,
			Ready:        status.Ready,
			Started:      status.Started,
			RestartCount: status.RestartCount,
		}
		switch {
		case status.State.Waiting != nil:
			summary.State = "waiting"
			summary.Reason = status.State.Waiting.Reason
		case status.State.Running != nil:
			summary.State = "running"
			summary.StartedAt = formatTime(status.State.Running.StartedAt)
		case status.State.Terminated != nil:
			summary.State = "terminated"
			summary.Reason = status.State.Terminated.Reason
			summary.ExitCode = status.State.Terminated.ExitCode
			summary.Signal = status.State.Terminated.Signal
			summary.StartedAt = formatTime(status.State.Terminated.StartedAt)
			summary.FinishedAt = formatTime(status.State.Terminated.FinishedAt)
		default:
			summary.State = "unknown"
		}
		if status.LastTerminationState.Terminated != nil {
			summary.LastTerminationReason = status.LastTerminationState.Terminated.Reason
			summary.LastExitCode = status.LastTerminationState.Terminated.ExitCode
		}
		result = append(result, summary)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func aggregateEvents(events []corev1.Event) []EventSummary {
	type key struct{ eventType, reason string }
	aggregated := make(map[key]EventSummary)
	for _, event := range events {
		eventType := strings.TrimSpace(event.Type)
		reason := strings.TrimSpace(event.Reason)
		if eventType == "" {
			eventType = "Unknown"
		}
		if reason == "" {
			reason = "Unknown"
		}
		count := event.Count
		if count <= 0 {
			count = 1
		}
		lastObserved := eventLastObserved(event)
		item := aggregated[key{eventType: eventType, reason: reason}]
		item.Type = eventType
		item.Reason = reason
		item.Count += count
		if lastObserved > item.LastObserved {
			item.LastObserved = lastObserved
		}
		aggregated[key{eventType: eventType, reason: reason}] = item
	}
	result := make([]EventSummary, 0, len(aggregated))
	for _, item := range aggregated {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Type != result[j].Type {
			return result[i].Type < result[j].Type
		}
		return result[i].Reason < result[j].Reason
	})
	return result
}

func eventLastObserved(event corev1.Event) string {
	if !event.EventTime.IsZero() {
		return event.EventTime.UTC().Format(time.RFC3339)
	}
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.UTC().Format(time.RFC3339)
	}
	return formatTime(event.CreationTimestamp)
}

func quantityString(resources corev1.ResourceList, name corev1.ResourceName) string {
	if value, found := resources[name]; found {
		return value.String()
	}
	return ""
}

func formatTime(value metav1.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
