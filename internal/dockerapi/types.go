package dockerapi

import (
	"sort"
	"strings"
	"time"
)

type Version struct {
	EngineVersion string `json:"engine_version"`
	APIVersion    string `json:"api_version"`
	MinAPIVersion string `json:"min_api_version,omitempty"`
	GitCommit     string `json:"git_commit,omitempty"`
	GoVersion     string `json:"go_version,omitempty"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	KernelVersion string `json:"kernel_version,omitempty"`
	BuildTime     string `json:"build_time,omitempty"`
}

type EngineInfo struct {
	Version            Version  `json:"version"`
	Containers         int      `json:"containers"`
	ContainersRunning  int      `json:"containers_running"`
	ContainersPaused   int      `json:"containers_paused"`
	ContainersStopped  int      `json:"containers_stopped"`
	Images             int      `json:"images"`
	StorageDriver      string   `json:"storage_driver"`
	LoggingDriver      string   `json:"logging_driver"`
	CgroupDriver       string   `json:"cgroup_driver,omitempty"`
	CgroupVersion      string   `json:"cgroup_version,omitempty"`
	KernelVersion      string   `json:"kernel_version"`
	OperatingSystem    string   `json:"operating_system"`
	OSVersion          string   `json:"os_version,omitempty"`
	OSType             string   `json:"os_type"`
	Architecture       string   `json:"architecture"`
	CPUs               int      `json:"cpus"`
	MemoryBytes        int64    `json:"memory_bytes"`
	DefaultRuntime     string   `json:"default_runtime,omitempty"`
	LiveRestoreEnabled bool     `json:"live_restore_enabled"`
	ExperimentalBuild  bool     `json:"experimental_build"`
	SecurityOptions    []string `json:"security_options,omitempty"`
	WarningCount       int      `json:"warning_count"`
}

type ContainerListOptions struct {
	All   bool
	Limit int
}

type ContainerSummary struct {
	ID        string         `json:"id"`
	Names     []string       `json:"names"`
	Image     string         `json:"image"`
	ImageID   string         `json:"image_id,omitempty"`
	CreatedAt string         `json:"created_at"`
	State     string         `json:"state"`
	Status    string         `json:"status"`
	Health    *HealthSummary `json:"health,omitempty"`
	Ports     []Port         `json:"ports,omitempty"`
	Networks  []Network      `json:"networks,omitempty"`
}

type HealthSummary struct {
	Status        string `json:"status"`
	FailingStreak int    `json:"failing_streak"`
}

type Port struct {
	IP          string `json:"ip,omitempty"`
	PrivatePort uint16 `json:"private_port"`
	PublicPort  uint16 `json:"public_port,omitempty"`
	Type        string `json:"type"`
}

type Network struct {
	Name              string `json:"name"`
	IPAddress         string `json:"ip_address,omitempty"`
	GlobalIPv6Address string `json:"global_ipv6_address,omitempty"`
	Gateway           string `json:"gateway,omitempty"`
}

type ContainerInspect struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	ImageID         string             `json:"image_id"`
	ConfiguredImage string             `json:"configured_image,omitempty"`
	CreatedAt       string             `json:"created_at"`
	Driver          string             `json:"driver,omitempty"`
	Platform        string             `json:"platform,omitempty"`
	RestartCount    int                `json:"restart_count"`
	State           ContainerState     `json:"state"`
	Runtime         ContainerRuntime   `json:"runtime"`
	Resources       ContainerResources `json:"resources"`
	Networks        []Network          `json:"networks,omitempty"`
	Ports           []PortBinding      `json:"ports,omitempty"`
	Mounts          []Mount            `json:"mounts,omitempty"`
}

type ContainerState struct {
	Status       string         `json:"status"`
	Running      bool           `json:"running"`
	Paused       bool           `json:"paused"`
	Restarting   bool           `json:"restarting"`
	OOMKilled    bool           `json:"oom_killed"`
	Dead         bool           `json:"dead"`
	PID          int            `json:"pid,omitempty"`
	ExitCode     int            `json:"exit_code"`
	ErrorPresent bool           `json:"error_present"`
	StartedAt    string         `json:"started_at,omitempty"`
	FinishedAt   string         `json:"finished_at,omitempty"`
	Health       *HealthSummary `json:"health,omitempty"`
}

type ContainerRuntime struct {
	NetworkMode    string `json:"network_mode,omitempty"`
	RestartPolicy  string `json:"restart_policy,omitempty"`
	MaximumRetries int    `json:"maximum_retries,omitempty"`
	AutoRemove     bool   `json:"auto_remove"`
	Privileged     bool   `json:"privileged"`
	ReadonlyRootFS bool   `json:"readonly_rootfs"`
	User           string `json:"user,omitempty"`
	WorkingDir     string `json:"working_dir,omitempty"`
	StopSignal     string `json:"stop_signal,omitempty"`
}

type ContainerResources struct {
	MemoryBytes int64 `json:"memory_bytes,omitempty"`
	NanoCPUs    int64 `json:"nano_cpus,omitempty"`
	PIDsLimit   int64 `json:"pids_limit,omitempty"`
}

type PortBinding struct {
	ContainerPort string `json:"container_port"`
	HostIP        string `json:"host_ip,omitempty"`
	HostPort      string `json:"host_port,omitempty"`
}

type Mount struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Destination string `json:"destination"`
	ReadWrite   bool   `json:"read_write"`
}

type rawVersion struct {
	Version       string `json:"Version"`
	APIVersion    string `json:"ApiVersion"`
	MinAPIVersion string `json:"MinAPIVersion"`
	GitCommit     string `json:"GitCommit"`
	GoVersion     string `json:"GoVersion"`
	OS            string `json:"Os"`
	Arch          string `json:"Arch"`
	KernelVersion string `json:"KernelVersion"`
	BuildTime     string `json:"BuildTime"`
}

type rawInfo struct {
	Containers         int      `json:"Containers"`
	ContainersRunning  int      `json:"ContainersRunning"`
	ContainersPaused   int      `json:"ContainersPaused"`
	ContainersStopped  int      `json:"ContainersStopped"`
	Images             int      `json:"Images"`
	Driver             string   `json:"Driver"`
	LoggingDriver      string   `json:"LoggingDriver"`
	CgroupDriver       string   `json:"CgroupDriver"`
	CgroupVersion      string   `json:"CgroupVersion"`
	KernelVersion      string   `json:"KernelVersion"`
	OperatingSystem    string   `json:"OperatingSystem"`
	OSVersion          string   `json:"OSVersion"`
	OSType             string   `json:"OSType"`
	Architecture       string   `json:"Architecture"`
	NCPU               int      `json:"NCPU"`
	MemTotal           int64    `json:"MemTotal"`
	DefaultRuntime     string   `json:"DefaultRuntime"`
	LiveRestoreEnabled bool     `json:"LiveRestoreEnabled"`
	ExperimentalBuild  bool     `json:"ExperimentalBuild"`
	SecurityOptions    []string `json:"SecurityOptions"`
	Warnings           []string `json:"Warnings"`
}

type rawContainerSummary struct {
	ID              string `json:"Id"`
	Names           []string
	Image           string
	ImageID         string
	Created         int64
	State           string
	Status          string
	Health          *rawHealthSummary `json:"Health"`
	Ports           []rawPort
	NetworkSettings *struct {
		Networks map[string]rawNetwork `json:"Networks"`
	} `json:"NetworkSettings"`
}

type rawHealthSummary struct {
	Status        string `json:"Status"`
	FailingStreak int    `json:"FailingStreak"`
}

type rawPort struct {
	IP          string
	PrivatePort uint16
	PublicPort  uint16
	Type        string
}

type rawNetwork struct {
	IPAddress         string `json:"IPAddress"`
	GlobalIPv6Address string `json:"GlobalIPv6Address"`
	Gateway           string `json:"Gateway"`
}

type rawContainerInspect struct {
	ID           string `json:"Id"`
	Created      string
	Image        string
	Name         string
	RestartCount int
	Driver       string
	Platform     string
	State        *struct {
		Status     string
		Running    bool
		Paused     bool
		Restarting bool
		OOMKilled  bool
		Dead       bool
		Pid        int
		ExitCode   int
		Error      string
		StartedAt  string
		FinishedAt string
		Health     *rawHealthSummary
	}
	HostConfig *struct {
		NetworkMode   string
		RestartPolicy struct {
			Name              string
			MaximumRetryCount int
		}
		AutoRemove     bool
		Privileged     bool
		ReadonlyRootfs bool
		Memory         int64
		NanoCpus       int64
		PidsLimit      *int64
	}
	Config *struct {
		Image      string
		User       string
		WorkingDir string
		StopSignal string
	}
	NetworkSettings *struct {
		Ports    map[string][]rawPortBinding
		Networks map[string]rawNetwork
	}
	Mounts []struct {
		Type        string
		Name        string
		Destination string
		RW          bool
	}
}

type rawPortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

func mapEngineInfo(version Version, raw rawInfo) EngineInfo {
	securityOptions := append([]string(nil), raw.SecurityOptions...)
	sort.Strings(securityOptions)
	return EngineInfo{
		Version:            version,
		Containers:         raw.Containers,
		ContainersRunning:  raw.ContainersRunning,
		ContainersPaused:   raw.ContainersPaused,
		ContainersStopped:  raw.ContainersStopped,
		Images:             raw.Images,
		StorageDriver:      raw.Driver,
		LoggingDriver:      raw.LoggingDriver,
		CgroupDriver:       raw.CgroupDriver,
		CgroupVersion:      raw.CgroupVersion,
		KernelVersion:      raw.KernelVersion,
		OperatingSystem:    raw.OperatingSystem,
		OSVersion:          raw.OSVersion,
		OSType:             raw.OSType,
		Architecture:       raw.Architecture,
		CPUs:               raw.NCPU,
		MemoryBytes:        raw.MemTotal,
		DefaultRuntime:     raw.DefaultRuntime,
		LiveRestoreEnabled: raw.LiveRestoreEnabled,
		ExperimentalBuild:  raw.ExperimentalBuild,
		SecurityOptions:    securityOptions,
		WarningCount:       len(raw.Warnings),
	}
}

func mapContainerSummaries(raw []rawContainerSummary) []ContainerSummary {
	result := make([]ContainerSummary, 0, len(raw))
	for _, item := range raw {
		names := make([]string, 0, len(item.Names))
		for _, name := range item.Names {
			name = strings.TrimPrefix(name, "/")
			if name != "" {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		ports := make([]Port, 0, len(item.Ports))
		for _, port := range item.Ports {
			ports = append(ports, Port{IP: port.IP, PrivatePort: port.PrivatePort, PublicPort: port.PublicPort, Type: port.Type})
		}
		sort.Slice(ports, func(i, j int) bool {
			if ports[i].PrivatePort != ports[j].PrivatePort {
				return ports[i].PrivatePort < ports[j].PrivatePort
			}
			if ports[i].PublicPort != ports[j].PublicPort {
				return ports[i].PublicPort < ports[j].PublicPort
			}
			return ports[i].Type < ports[j].Type
		})
		result = append(result, ContainerSummary{
			ID:        item.ID,
			Names:     names,
			Image:     item.Image,
			ImageID:   item.ImageID,
			CreatedAt: time.Unix(item.Created, 0).UTC().Format(time.RFC3339),
			State:     item.State,
			Status:    item.Status,
			Health:    mapHealth(item.Health),
			Ports:     ports,
			Networks:  mapNetworks(item.NetworkSettings),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i].ID, result[j].ID
		if len(result[i].Names) > 0 {
			left = result[i].Names[0]
		}
		if len(result[j].Names) > 0 {
			right = result[j].Names[0]
		}
		return left < right
	})
	return result
}

func mapContainerInspect(raw rawContainerInspect) ContainerInspect {
	result := ContainerInspect{
		ID:           raw.ID,
		Name:         strings.TrimPrefix(raw.Name, "/"),
		ImageID:      raw.Image,
		CreatedAt:    raw.Created,
		Driver:       raw.Driver,
		Platform:     raw.Platform,
		RestartCount: raw.RestartCount,
	}
	if raw.State != nil {
		result.State = ContainerState{
			Status:       raw.State.Status,
			Running:      raw.State.Running,
			Paused:       raw.State.Paused,
			Restarting:   raw.State.Restarting,
			OOMKilled:    raw.State.OOMKilled,
			Dead:         raw.State.Dead,
			PID:          raw.State.Pid,
			ExitCode:     raw.State.ExitCode,
			ErrorPresent: strings.TrimSpace(raw.State.Error) != "",
			StartedAt:    raw.State.StartedAt,
			FinishedAt:   raw.State.FinishedAt,
			Health:       mapHealth(raw.State.Health),
		}
	}
	if raw.HostConfig != nil {
		result.Runtime = ContainerRuntime{
			NetworkMode:    raw.HostConfig.NetworkMode,
			RestartPolicy:  raw.HostConfig.RestartPolicy.Name,
			MaximumRetries: raw.HostConfig.RestartPolicy.MaximumRetryCount,
			AutoRemove:     raw.HostConfig.AutoRemove,
			Privileged:     raw.HostConfig.Privileged,
			ReadonlyRootFS: raw.HostConfig.ReadonlyRootfs,
		}
		result.Resources.MemoryBytes = raw.HostConfig.Memory
		result.Resources.NanoCPUs = raw.HostConfig.NanoCpus
		if raw.HostConfig.PidsLimit != nil {
			result.Resources.PIDsLimit = *raw.HostConfig.PidsLimit
		}
	}
	if raw.Config != nil {
		result.ConfiguredImage = raw.Config.Image
		result.Runtime.User = raw.Config.User
		result.Runtime.WorkingDir = raw.Config.WorkingDir
		result.Runtime.StopSignal = raw.Config.StopSignal
	}
	if raw.NetworkSettings != nil {
		result.Networks = mapNetworkMap(raw.NetworkSettings.Networks)
		for containerPort, bindings := range raw.NetworkSettings.Ports {
			if len(bindings) == 0 {
				result.Ports = append(result.Ports, PortBinding{ContainerPort: containerPort})
				continue
			}
			for _, binding := range bindings {
				result.Ports = append(result.Ports, PortBinding{ContainerPort: containerPort, HostIP: binding.HostIP, HostPort: binding.HostPort})
			}
		}
		sort.Slice(result.Ports, func(i, j int) bool {
			if result.Ports[i].ContainerPort != result.Ports[j].ContainerPort {
				return result.Ports[i].ContainerPort < result.Ports[j].ContainerPort
			}
			if result.Ports[i].HostIP != result.Ports[j].HostIP {
				return result.Ports[i].HostIP < result.Ports[j].HostIP
			}
			return result.Ports[i].HostPort < result.Ports[j].HostPort
		})
	}
	for _, mount := range raw.Mounts {
		result.Mounts = append(result.Mounts, Mount{Type: mount.Type, Name: mount.Name, Destination: mount.Destination, ReadWrite: mount.RW})
	}
	sort.Slice(result.Mounts, func(i, j int) bool {
		return result.Mounts[i].Destination < result.Mounts[j].Destination
	})
	return result
}

func mapHealth(raw *rawHealthSummary) *HealthSummary {
	if raw == nil {
		return nil
	}
	return &HealthSummary{Status: raw.Status, FailingStreak: raw.FailingStreak}
}

func mapNetworks(settings *struct {
	Networks map[string]rawNetwork `json:"Networks"`
}) []Network {
	if settings == nil {
		return nil
	}
	return mapNetworkMap(settings.Networks)
}

func mapNetworkMap(networks map[string]rawNetwork) []Network {
	result := make([]Network, 0, len(networks))
	for name, network := range networks {
		result = append(result, Network{Name: name, IPAddress: network.IPAddress, GlobalIPv6Address: network.GlobalIPv6Address, Gateway: network.Gateway})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
