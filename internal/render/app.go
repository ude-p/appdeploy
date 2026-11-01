package render

import (
	"fmt"
	"maps"
	"slices"

	"github.com/ude-p/appdeploy/internal/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func buildApps(model config.Model, namespace string) ([]client.Object, error) {
	objects := make([]client.Object, 0, len(model.Apps)*2)
	for _, app := range model.Apps {
		if app.Scope != "" && app.Scope != namespace {
			continue
		}
		labels := map[string]string{"app.kubernetes.io/name": app.Name}
		replicas := int32(1)
		if model.Defaults != nil && model.Defaults.Replicas != nil {
			replicas = *model.Defaults.Replicas
		}
		if app.Replicas != nil {
			replicas = *app.Replicas
		}

		container, volumes, err := buildContainer(model, app)
		if err != nil {
			return nil, fmt.Errorf("app %q: %w", app.Name, err)
		}
		resources, err := inlineResources(model, namespace, app.Name, app.Env)
		if err != nil {
			return nil, fmt.Errorf("app %q: %w", app.Name, err)
		}
		objects = append(objects, resources...)
		objects = append(objects, &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: namespace},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec: corev1.PodSpec{
						NodeSelector: nodeSelector(model, app.NodeSelector),
						Containers: []corev1.Container{container},
						Volumes:    volumes,
					},
				},
			},
		})
		if len(app.ServicePorts()) > 0 {
			objects = append(objects, buildService(app, namespace))
		}
	}
	return objects, nil
}

func buildContainer(model config.Model, app config.App) (corev1.Container, []corev1.Volume, error) {
	container := corev1.Container{
		Name:            app.Name,
		Image:           app.Image,
		ImagePullPolicy: corev1.PullAlways,
		Command:         app.Command,
		Args:            app.Args,
	}
	envFrom, env, err := buildEnv(app.Name, app.Env)
	if err != nil {
		return corev1.Container{}, nil, err
	}
	container.EnvFrom = envFrom
	container.Env = env

	ports := app.ServicePorts()
	for _, port := range ports {
		container.Ports = append(container.Ports, corev1.ContainerPort{ContainerPort: port})
	}

	resources := app.Resources
	if resources == nil && model.Defaults != nil {
		resources = model.Defaults.Resources
	}
	if resources != nil {
		cpu, err := resource.ParseQuantity(resources.CPU)
		if err != nil {
			return corev1.Container{}, nil, fmt.Errorf("parse CPU: %w", err)
		}
		memory, err := resource.ParseQuantity(resources.Memory)
		if err != nil {
			return corev1.Container{}, nil, fmt.Errorf("parse memory: %w", err)
		}
		container.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: memory},
			Limits:   corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: memory},
		}
	}

	configureProbes(&container, app.Health, ports)

	mounts, volumes, err := buildMounts(app.Mounts)
	if err != nil {
		return corev1.Container{}, nil, err
	}
	container.VolumeMounts = mounts

	return container, volumes, nil
}

func buildEnv(workloadName string, env *config.Env) ([]corev1.EnvFromSource, []corev1.EnvVar, error) {
	envFrom := make([]corev1.EnvFromSource, 0, 4)
	values := map[string]string{}
	if env != nil {
		if env.Config != nil {
			envFrom = append(envFrom, corev1.EnvFromSource{
				ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: workloadName + "-config"}},
			})
		}
		if env.Secret != nil {
			envFrom = append(envFrom, corev1.EnvFromSource{
				SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: workloadName + "-secret"}},
			})
		}
		if config.HasExpression(env.FromConfig) {
			name, err := referenceName(env.FromConfig)
			if err != nil {
				return nil, nil, err
			}
			envFrom = append(envFrom, corev1.EnvFromSource{
				ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: name}},
			})
		}
		if config.HasExpression(env.FromSecret) {
			name, err := referenceName(env.FromSecret)
			if err != nil {
				return nil, nil, err
			}
			envFrom = append(envFrom, corev1.EnvFromSource{
				SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: name}},
			})
		}
		var err error
		values, err = config.StringValues(env.Values, "from_config", "from_secret")
		if err != nil {
			return nil, nil, err
		}
	}
	fields := make([]corev1.EnvVar, 0, len(values)+len(config.DownwardEnv))
	for _, name := range slices.Sorted(maps.Keys(values)) {
		fields = append(fields, corev1.EnvVar{Name: name, Value: values[name]})
	}
	fields = append(fields, downwardEnv()...)
	return envFrom, fields, nil
}

func buildMounts(mounts []config.Mount) ([]corev1.VolumeMount, []corev1.Volume, error) {
	volumeMounts := make([]corev1.VolumeMount, 0, len(mounts))
	volumes := make([]corev1.Volume, 0, len(mounts))
	for _, mount := range mounts {
		var name string
		var source corev1.VolumeSource
		var err error
		switch {
		case config.HasExpression(mount.Volume):
			name, err = referenceName(mount.Volume)
			source.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{ClaimName: name}
		case config.HasExpression(mount.Config):
			name, err = referenceName(mount.Config)
			source.ConfigMap = &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: name}}
		case config.HasExpression(mount.Secret):
			name, err = referenceName(mount.Secret)
			source.Secret = &corev1.SecretVolumeSource{SecretName: name}
		}
		if err != nil {
			return nil, nil, err
		}
		volumeMounts = append(volumeMounts, corev1.VolumeMount{Name: name, MountPath: mount.Path})
		volumes = append(volumes, corev1.Volume{Name: name, VolumeSource: source})
	}
	return volumeMounts, volumes, nil
}

func buildService(app config.App, namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: namespace},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": app.Name},
			Ports:    servicePorts(app.ServicePorts()),
		},
	}
}
