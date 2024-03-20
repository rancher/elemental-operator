/*
Copyright © 2022 - 2024 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kubectl

import "time"

// Pod is a collection of containers that can run on a host. This resource is created
// by clients and scheduled onto hosts.
type Pod struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind,omitempty"`
	Metadata   Metadata  `yaml:"metadata"`
	Spec       PodSpec   `yaml:"spec"`
	Status     PodStatus `yaml:"status,omitempty"`
}

// PodStatus has all the pod status information
type PodStatus struct {
	Phase             string            `yaml:"phase,omitempty"`
	Conditions        []PodCondition    `yaml:"conditions,omitempty"`
	Message           string            `yaml:"message,omitempty"`
	Reason            string            `yaml:"reason,omitempty"`
	ContainerStatuses []ContainerStatus `yaml:"containerStatuses,omitempty" protobuf:"bytes,8,rep,name=containerStatuses"`
}

// ContainerState is the known state of a container in a pod
type ContainerState struct {
	Waiting    *ContainerStateWaiting    `yaml:"waiting,omitempty" protobuf:"bytes,1,opt,name=waiting"`
	Running    *ContainerStateRunning    `yaml:"running,omitempty" protobuf:"bytes,2,opt,name=running"`
	Terminated *ContainerStateTerminated `yaml:"terminated,omitempty" protobuf:"bytes,3,opt,name=terminated"`
}

// ContainerStateWaiting is a waiting state of a container.
type ContainerStateWaiting struct {
	Reason  string `yaml:"reason,omitempty"`
	Message string `yaml:"message,omitempty"`
}

// ContainerStateRunning is a running state of a container.
type ContainerStateRunning struct {
	StartedAt time.Time `yaml:"startedAt,omitempty"`
}

// ContainerStateTerminated is a terminated state of a container.
type ContainerStateTerminated struct {
	ExitCode    int32     `yaml:"exitCode"`
	Signal      int32     `yaml:"signal,omitempty"`
	Reason      string    `yaml:"reason,omitempty"`
	Message     string    `yaml:"message,omitempty"`
	StartedAt   time.Time `yaml:"startedAt,omitempty"`
	FinishedAt  time.Time `yaml:"finishedAt,omitempty"`
	ContainerID string    `yaml:"containerID,omitempty"`
}

// ContainerStatus contains details for the current status of this container.
type ContainerStatus struct {
	Name                 string         `yaml:"name"`
	State                ContainerState `yaml:"state,omitempty"`
	LastTerminationState ContainerState `yaml:"lastState,omitempty"`
	Ready                bool           `yaml:"ready"`
	RestartCount         int32          `yaml:"restartCount"`
	Image                string         `yaml:"image"`
	ImageID              string         `yaml:"imageID"`
	ContainerID          string         `yaml:"containerID,omitempty"`
	Started              *bool          `yaml:"started,omitempty"`
}

// PodCondition is the pod condition status
type PodCondition struct {
	Type string `yaml:"type"`
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#pod-conditions
	Status  string `yaml:"status"`
	Reason  string `yaml:"reason,omitempty"`
	Message string `yaml:"message,omitempty"`
}

// Metadata is metadata attached to any object
type Metadata struct {
	Name            string            `yaml:"name"`
	GenerateName    string            `yaml:"generateName"`
	ResourceVersion string            `yaml:"resourceVersion"`
	Labels          map[string]string `yaml:"labels"`
	Annotations     map[string]string `yaml:"annotations"`
	UID             string            `yaml:"uid"`
	Namespace       string            `yaml:"namespace"`
}

// PodSpec is a description of a pod.
type PodSpec struct {
	Containers   []Container       `yaml:"containers"`
	NodeSelector map[string]string `yaml:"nodeSelector"`
}

// Container is a single application container that you want to run within a pod.
type Container struct {
	Name    string   `yaml:"name"`
	Image   string   `yaml:"image,omitempty"`
	Command []string `yaml:"command,omitempty"`
	Args    []string `yaml:"args,omitempty"`
	Env     []EnvVar `yaml:"envs,omitempty"`
}

// EnvVar represents an environment variable present in a Container.
type EnvVar struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value,omitempty"`
}
