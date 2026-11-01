package render

import (
	"fmt"

	"github.com/ude-p/appdeploy/internal/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	defaultCPURequest    = "100m"
	defaultMemoryRequest = "128Mi"
	defaultCPULimit      = "1"
	defaultMemoryLimit   = "1Gi"
)

func resolveContainerResources(configured *config.Resources, model config.Model) *config.Resources {
	if configured != nil {
		return configured
	}
	if model.Defaults != nil {
		return model.Defaults.Resources
	}
	return nil
}

func containerResources(configured *config.Resources) (corev1.ResourceRequirements, error) {
	requests := config.Resources{CPU: defaultCPURequest, Memory: defaultMemoryRequest}
	limits := config.Resources{CPU: defaultCPULimit, Memory: defaultMemoryLimit}
	if configured != nil {
		requests, limits = *configured, *configured
	}

	cpuRequest, err := resource.ParseQuantity(requests.CPU)
	if err != nil {
		return corev1.ResourceRequirements{}, fmt.Errorf("parse CPU request: %w", err)
	}
	memoryRequest, err := resource.ParseQuantity(requests.Memory)
	if err != nil {
		return corev1.ResourceRequirements{}, fmt.Errorf("parse memory request: %w", err)
	}
	cpuLimit, err := resource.ParseQuantity(limits.CPU)
	if err != nil {
		return corev1.ResourceRequirements{}, fmt.Errorf("parse CPU limit: %w", err)
	}
	memoryLimit, err := resource.ParseQuantity(limits.Memory)
	if err != nil {
		return corev1.ResourceRequirements{}, fmt.Errorf("parse memory limit: %w", err)
	}

	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: cpuRequest, corev1.ResourceMemory: memoryRequest},
		Limits:   corev1.ResourceList{corev1.ResourceCPU: cpuLimit, corev1.ResourceMemory: memoryLimit},
	}, nil
}
