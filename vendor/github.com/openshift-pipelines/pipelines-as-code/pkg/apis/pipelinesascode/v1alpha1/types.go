package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Repository is the representation of a repo
type Repository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RepositorySpec        `json:"spec"`
	Status []RepositoryRunStatus `json:"pipelinerun_status,omitempty"`
}

type RepositoryRunStatus struct {
	duckv1.Status `json:",inline"`

	// PipelineRunName is the name of the PipelineRun
	// +optional
	PipelineRunName string `json:"pipelineRunName,omitempty"`

	// StartTime is the time the PipelineRun is actually started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is the time the PipelineRun completed.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// SHA is the name of the SHA that has been tested
	// +optional
	SHA *string `json:"sha,omitempty"`

	// SHA the URL of the SHA to view it
	// +optional
	SHAURL *string `json:"sha_url,omitempty"`

	// Title is the title of the commit SHA that has been tested
	// +optional
	Title *string `json:"title,omitempty"`

	// LogURL is the full url to this run long
	// +optional
	LogURL *string `json:"logurl,omitempty"`

	// TargetBranch is the target branch of that run
	// +optional
	TargetBranch *string `json:"target_branch,omitempty"`

	// EventType is the event type of that run
	// +optional
	EventType *string `json:"event_type,omitempty"`

	// CollectedTaskInfos is the information about tasks
	CollectedTaskInfos *map[string]TaskInfos `json:"failure_reason,omitempty"`
}

type TaskInfos struct {
	Name           string
	Message        string
	LogSnippet     string
	Reason         string
	CompletionTime *metav1.Time
}

// RepositorySpec is the spec of a repo
type RepositorySpec struct {
	ConcurrencyLimit *int         `json:"concurrency_limit,omitempty"`
	URL              string       `json:"url"`
	GitProvider      *GitProvider `json:"git_provider,omitempty"`
	Incomings        *[]Incoming  `json:"incoming,omitempty"`
}

type Incoming struct {
	Type    string   `json:"type"`
	Secret  Secret   `json:"secret"`
	Targets []string `json:"targets,omitempty"`
}

type GitProvider struct {
	URL           string  `json:"url,omitempty"`
	User          string  `json:"user,omitempty"`
	Secret        *Secret `json:"secret,omitempty"`
	WebhookSecret *Secret `json:"webhook_secret,omitempty"`
	Type          string  `json:"type,omitempty"`
}

type Secret struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RepositoryList is the list of Repositories
type RepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Repository `json:"items"`
}
