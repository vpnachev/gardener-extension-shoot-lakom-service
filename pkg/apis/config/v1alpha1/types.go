// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	healthcheckconfigv1alpha1 "github.com/gardener/gardener/extensions/pkg/apis/config/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Configuration contains information about the Lakom service configuration.
type Configuration struct {
	metav1.TypeMeta `json:",inline"`

	// HealthCheckConfig is the config for the health check controller.
	// +optional
	HealthCheckConfig *healthcheckconfigv1alpha1.HealthCheckConfig `json:"healthCheckConfig,omitempty"`
	// CosignPublicKeys is the cosign public keys used to verify image signatures.
	CosignPublicKeys []string `json:"cosignPublicKeys,omitempty"`
	// FailurePolicy is the failure policy used to configure the failurePolicy of the lakom admission webhooks.
	// +optional
	FailurePolicy *string `json:"failurePolicy,omitempty"`
	// DebugConfig contains debug configurations for the controller.
	// +optional
	DebugConfig *DebugConfig `json:"debugConfig,omitempty"`
}

// DebugConfig contains debug configurations for the controller.
type DebugConfig struct {
	// EnableProfiling enables profiling via web interface host:port/debug/pprof/.
	EnableProfiling bool `json:"enableProfiling"`
	// EnableContentionProfiling enables lock contention profiling, if
	// enableProfiling is true.
	EnableContentionProfiling bool `json:"enableContentionProfiling"`
}
